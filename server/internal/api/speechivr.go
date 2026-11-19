package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/ai"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Speech steps of voice menus: Asterisk records the caller into speechSpool and asks this FastAGI
// listener (on localhost only) which intent it was. The answer comes back as channel variables.

func (s *Server) speechSpool() string { return filepath.Join(s.Cfg.SpoolDir(), "speech") }

// agiAddr is where the FastAGI listener runs ("" turns speech steps off in the dialplan).
func (s *Server) agiAddr() string {
	if a, ok := os.LookupEnv("KIESTOON_AGI_LISTEN"); ok {
		return a
	}
	return "127.0.0.1:4573"
}

// transcribeShort is replaced in tests (there is no Whisper there).
var transcribeShort = func(s *Server, ctx context.Context, model, language, file string) (string, error) {
	return s.speechEngine().TranscribeNow(ctx, model, language, file)
}

var reSpeechFile = regexp.MustCompile(`^[0-9.]+-[0-9]+\.wav$`)

// runAGI serves FastAGI requests until ctx ends.
func (s *Server) runAGI(ctx context.Context) {
	addr := s.agiAddr()
	if addr == "" {
		return
	}
	_ = os.MkdirAll(s.speechSpool(), 0o2770)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		s.Log.Warn("speech menu listener", "addr", addr, "err", err)
		return
	}
	go func() { <-ctx.Done(); l.Close() }()
	go s.purgeSpeechSpool(ctx)
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go s.serveAGI(ctx, c)
	}
}

// purgeSpeechSpool removes recordings that were left behind (a hangup during recognition).
func (s *Server) purgeSpeechSpool(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		entries, _ := os.ReadDir(s.speechSpool())
		for _, e := range entries {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 10*time.Minute {
				_ = os.Remove(filepath.Join(s.speechSpool(), e.Name()))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) serveAGI(ctx context.Context, c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	r := bufio.NewReader(c)
	env := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ": "); ok {
			env[k] = v
		}
	}
	set := func(k, v string) bool {
		if _, err := fmt.Fprintf(c, "SET VARIABLE %s \"%s\"\n", k, v); err != nil {
			return false
		}
		reply, err := r.ReadString('\n')
		return err == nil && strings.HasPrefix(reply, "200")
	}
	switch env["agi_network_script"] {
	case "hotdesk":
		hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		// What the user typed comes as channel variables, so the PIN is not in the Asterisk log.
		get := func(k string) string {
			if _, err := fmt.Fprintf(c, "GET VARIABLE %s\n", k); err != nil {
				return ""
			}
			reply, err := r.ReadString('\n')
			if err != nil || !strings.HasPrefix(reply, "200 result=1 (") {
				return ""
			}
			v := strings.TrimPrefix(strings.TrimRight(reply, "\r\n"), "200 result=1 (")
			return strings.TrimSuffix(v, ")")
		}
		number, pin := "", ""
		if env["agi_arg_1"] == "login" {
			number, pin = get("KT_HDN"), get("KT_HDP")
		}
		set("KT_HD", s.hotdeskAGI(hctx, env["agi_arg_1"], env["agi_arg_2"], env["agi_arg_3"], number, pin))
		return
	case "speech":
	default:
		return
	}
	ivr, _ := strconv.ParseInt(env["agi_arg_1"], 10, 64)
	rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	n, label := s.recogniseSpeech(rctx, ivr, env["agi_arg_2"], env["agi_arg_3"])
	if n > 0 && set("KT_INTENT", strconv.Itoa(n)) {
		set("KT_INTENTL", label)
	}
}

var reIntentLabel = regexp.MustCompile(`[^\p{L}\p{N} -]+`)

// recogniseSpeech transcribes the recording and picks the intent: by keywords, else (when the step
// allows it) by the AI engine. It returns 0 when nothing fits. The recording is removed.
func (s *Server) recogniseSpeech(ctx context.Context, ivr int64, nodeID, file string) (int, string) {
	if filepath.Dir(file) != s.speechSpool() || !reSpeechFile.MatchString(filepath.Base(file)) {
		return 0, ""
	}
	defer os.Remove(file)
	v, err := s.PBX.IVR(ctx, ivr)
	if err != nil || v.Flow == nil {
		return 0, ""
	}
	node := v.Flow.Node(nodeID)
	if node == nil || node.Type != pbx.NodeSpeech || len(node.Intents) == 0 {
		return 0, ""
	}
	st := s.speechSettings(ctx)
	start := time.Now()
	said, err := transcribeShort(s, ctx, st.STT.Model, st.STT.Language, file)
	if err != nil {
		s.Log.Warn("speech menu", "ivr", ivr, "err", err)
		return 0, ""
	}
	n := pbx.MatchIntent(said, node.Intents)
	how := "keywords"
	if n == 0 && node.AI && said != "" {
		labels := make([]string, len(node.Intents))
		for i, in := range node.Intents {
			labels[i] = in.Label
			if len(in.Keywords) > 0 {
				labels[i] += " (" + strings.Join(in.Keywords, ", ") + ")"
			}
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		n, err = s.summarizer(cctx).Classify(cctx, s.aiSettings(ctx), said, labels)
		cancel()
		how = "ai"
		if err != nil && !errors.Is(err, ai.ErrNoKey) {
			s.Log.Warn("speech menu ai", "ivr", ivr, "err", err)
		}
	}
	s.Log.Info("speech menu", "ivr", ivr, "intent", n, "by", how, "ms", time.Since(start).Milliseconds())
	if n == 0 {
		return 0, ""
	}
	return n, strings.Join(strings.Fields(reIntentLabel.ReplaceAllString(node.Intents[n-1].Label, " ")), " ")
}
