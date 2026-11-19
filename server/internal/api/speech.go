package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/rhisijmuiden/kiestoon/server/internal/ai"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/speech"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

type speechState struct {
	once   sync.Once
	engine *speech.Engine
}

func (s *Server) speechEngine() *speech.Engine {
	s.speech.once.Do(func() {
		home := os.Getenv("KIESTOON_SPEECH_HOME")
		if home == "" {
			home = "/opt/kiestoon-speech"
		}
		s.speech.engine = &speech.Engine{Home: home, Dir: filepath.Join(s.Cfg.DataDir, "speech")}
	})
	return s.speech.engine
}

func (s *Server) speechSettings(ctx context.Context) speech.Settings {
	st := speech.Defaults()
	_ = store.GetJSON(ctx, s.DB, speech.SettingsKey, &st)
	return st
}

func (s *Server) speechGet(w http.ResponseWriter, r *http.Request) {
	e := s.speechEngine()
	tts, stt := e.Installed()
	type item struct {
		ID         string           `json:"id"`
		Name       string           `json:"name"`
		Language   string           `json:"language,omitempty"`
		Licence    string           `json:"licence,omitempty"`
		Speed      string           `json:"speed,omitempty"`
		Size       int64            `json:"size"`
		Downloaded bool             `json:"downloaded"`
		Progress   *speech.Download `json:"progress,omitempty"`
	}
	voices, models := []item{}, []item{}
	for i := range speech.Voices {
		v := &speech.Voices[i]
		voices = append(voices, item{ID: v.ID, Name: v.Name, Language: v.Language, Licence: v.Licence, Size: v.Size,
			Downloaded: e.HasVoice(v), Progress: e.Progress(v.ID)})
	}
	for i := range speech.Models {
		m := &speech.Models[i]
		models = append(models, item{ID: m.ID, Name: m.Name, Speed: m.Speed, Size: m.Size, Licence: "MIT",
			Downloaded: e.HasModel(m), Progress: e.Progress(m.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.speechSettings(r.Context()), "ttsInstalled": tts, "sttInstalled": stt,
		"voices": voices, "models": models})
}

func (s *Server) speechSave(w http.ResponseWriter, r *http.Request) {
	var st speech.Settings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := st.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, speech.SettingsKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "speech.saved", "")
	s.speechGet(w, r)
}

func (s *Server) speechDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.speechEngine().Start(req.ID); err != nil {
		fieldErr(w, "id")
		return
	}
	s.audit(r, "speech.download", req.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) speechRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if speech.FindVoice(id) == nil && speech.FindModel(id) == nil && speech.FindLLM(id) == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.speechEngine().Remove(id)
	s.audit(r, "speech.removed", id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// speechTTS speaks typed text for a prompt: the result is the same 8 kHz WAV as an uploaded prompt.
func (s *Server) speechTTS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text     string `json:"text"`
		Language string `json:"language"`
		Voice    string `json:"voice"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	st := s.speechSettings(r.Context())
	if !st.TTS.Enabled {
		writeErr(w, http.StatusConflict, "speech_not_ready")
		return
	}
	voice := req.Voice
	if voice == "" {
		voice = st.TTS.VoiceEN
		if req.Language == "nl" {
			voice = st.TTS.VoiceNL
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	audio, err := s.speechEngine().Speak(ctx, voice, req.Text)
	switch {
	case errors.Is(err, speech.ErrNotReady):
		writeErr(w, http.StatusConflict, "speech_not_ready")
		return
	case err != nil && err.Error() == "text":
		fieldErr(w, "text")
		return
	case err != nil:
		s.Log.Warn("text-to-speech", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "speech_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"audio": base64.StdEncoding.EncodeToString(audio)})
}

// ---- voicemail transcripts --------------------------------------------------

func (s *Server) transcriptDir() string { return filepath.Join(s.Cfg.DataDir, "speech", "transcripts") }

func sumHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// transcript returns the cached transcript of an audio file ("" when there is none).
func (s *Server) transcript(audio []byte) string {
	b, err := os.ReadFile(filepath.Join(s.transcriptDir(), sumHex(audio)+".txt"))
	if err != nil {
		return ""
	}
	return string(b)
}

// transcribe transcribes audio (cached by content, so mail and portal share the work). ok is
// false when speech-to-text is off or not ready.
func (s *Server) transcribe(ctx context.Context, audio []byte) (string, bool) {
	st := s.speechSettings(ctx)
	if !st.STT.Enabled || !st.STT.Voicemail {
		return "", false
	}
	sum := sumHex(audio)
	cache := filepath.Join(s.transcriptDir(), sum+".txt")
	if b, err := os.ReadFile(cache); err == nil {
		return string(b), true
	}
	tmp, err := os.CreateTemp("", "kt-vm-*.wav")
	if err != nil {
		return "", false
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.Write(audio)
	tmp.Close()
	text, err := s.speechEngine().Transcribe(ctx, st.STT.Model, st.STT.Language, tmp.Name())
	if err != nil {
		if !errors.Is(err, speech.ErrNotReady) {
			s.Log.Warn("voicemail transcription", "err", err)
		}
		return "", false
	}
	if err := os.MkdirAll(s.transcriptDir(), 0o750); err == nil {
		_ = os.WriteFile(cache, []byte(text), 0o640)
	}
	return text, true
}

// prepareMail adds the transcript to a voicemail mail from Asterisk (multipart with a WAV attachment).
func (s *Server) prepareMail(ctx context.Context, raw []byte) []byte {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil || msg.Header.Get("X-Asterisk-VM-Message-Num") == "" {
		return nil
	}
	st := s.speechSettings(ctx)
	if !st.STT.Enabled || !st.STT.Voicemail {
		return nil
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mt, "multipart/") || params["boundary"] == "" {
		return nil
	}
	type part struct {
		header textproto.MIMEHeader
		body   []byte
	}
	var parts []part
	var audio []byte
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		b, err := io.ReadAll(io.LimitReader(p, 60<<20))
		if err != nil {
			return nil
		}
		ct := strings.ToLower(p.Header.Get("Content-Type"))
		if strings.HasPrefix(ct, "audio/") && audio == nil && strings.EqualFold(p.Header.Get("Content-Transfer-Encoding"), "base64") {
			audio, _ = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(b)), ""))
		}
		parts = append(parts, part{p.Header, b})
	}
	if len(audio) == 0 {
		return nil
	}
	tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	text, ok := s.transcribe(tctx, audio)
	if !ok || text == "" {
		return nil
	}
	nl := s.defaultLanguage(ctx) == "nl"
	label := "Transcription (automatic)"
	if nl {
		label = "Transcriptie (automatisch)"
	}
	extra := label + ":\r\n" + text + "\r\n"
	if s.aiSettings(ctx).Voicemail {
		if sum := s.voicemailSummary(tctx, audio, text); sum != nil {
			extra = mailSummary(sum, nl) + "\r\n" + extra
		}
	}
	var out bytes.Buffer
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	sep := 4
	if end < 0 {
		end, sep = bytes.Index(raw, []byte("\n\n")), 2
	}
	if end < 0 {
		return nil
	}
	out.Write(raw[:end+sep])
	mw := multipart.NewWriter(&out)
	if err := mw.SetBoundary(params["boundary"]); err != nil {
		return nil
	}
	added := false
	for _, p := range parts {
		body := p.body
		if !added && strings.HasPrefix(strings.ToLower(p.header.Get("Content-Type")), "text/plain") {
			body = append(bytes.TrimRight(body, "\r\n"), []byte("\r\n\r\n"+extra)...)
			// multipart decoded quoted-printable for us; the text is now plain 8-bit.
			p.header.Del("Content-Transfer-Encoding")
			p.header.Set("Content-Transfer-Encoding", "8bit")
			added = true
		}
		pw, err := mw.CreatePart(p.header)
		if err != nil {
			return nil
		}
		_, _ = pw.Write(body)
	}
	_ = mw.Close()
	return out.Bytes()
}

// transcribeSpool transcribes new voicemail messages for the portal (when the mail did not already).
func (s *Server) transcribeSpool(ctx context.Context) {
	st := s.speechSettings(ctx)
	if !st.STT.Enabled || !st.STT.Voicemail {
		return
	}
	exts, err := s.PBX.Extensions(ctx)
	if err != nil {
		return
	}
	for _, e := range exts {
		if !e.Voicemail {
			continue
		}
		msgs, _ := s.Spool.Messages(e.Number)
		for _, m := range msgs {
			if ctx.Err() != nil {
				return
			}
			p, err := s.Spool.MessageAudio(e.Number, m.Folder, m.ID)
			if err != nil {
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(s.transcriptDir(), sumHex(b)+".txt")); err == nil {
				continue
			}
			if _, ok := s.transcribe(ctx, b); !ok {
				return
			}
		}
	}
}

// mailSummary is the AI summary block of a voicemail mail.
func mailSummary(sum *ai.Summary, nl bool) string {
	head, acts := "Summary (AI)", "To do"
	if nl {
		head, acts = "Samenvatting (AI)", "Te doen"
	}
	out := head + ":\r\n" + sum.Summary + "\r\n"
	if len(sum.Actions) > 0 {
		out += acts + ":\r\n"
		for _, a := range sum.Actions {
			out += "- " + a + "\r\n"
		}
	}
	return out
}
