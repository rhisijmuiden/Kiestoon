package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/ai"
	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/speech"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// AI: transcripts of recorded calls (local Whisper) and summaries of calls and voicemail (local
// model or Claude). Results are files in <data>/speech/ai, next to the voicemail transcripts, so
// backups and the HA standby carry them along.

func (s *Server) aiRoutes(mux *http.ServeMux, admin, recep, user func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/ai", admin(s.aiGet))
	mux.Handle("PUT /api/ai", admin(s.aiSave))
	mux.Handle("PUT /api/ai/key", admin(s.aiSetKey))
	mux.Handle("POST /api/ai/test", admin(s.aiTest))
	mux.Handle("GET /api/recordings/{name}/ai", recep(s.recordingAI))
	mux.Handle("POST /api/recordings/{name}/ai", recep(s.recordingAIRun))
	mux.Handle("GET /api/portal/recordings/{name}/ai", user(s.portalRecordingAI))
}

func (s *Server) aiSettings(ctx context.Context) ai.Settings {
	st := ai.Defaults()
	_ = store.GetJSON(ctx, s.DB, ai.SettingsKey, &st)
	return st
}

func (s *Server) aiKey(ctx context.Context) string {
	k, _ := store.GetSetting(ctx, s.DB, ai.KeyKey, "")
	return k
}

func (s *Server) summarizer(ctx context.Context) *ai.Summarizer {
	return &ai.Summarizer{Speech: s.speechEngine(), APIKey: s.aiKey(ctx), BaseURL: os.Getenv("KIESTOON_AI_BASE_URL")}
}

func (s *Server) aiGet(w http.ResponseWriter, r *http.Request) {
	e := s.speechEngine()
	type llm struct {
		speech.LLM
		Downloaded bool             `json:"downloaded"`
		Progress   *speech.Download `json:"progress,omitempty"`
	}
	llms := []llm{}
	for i := range speech.LLMs {
		m := speech.LLMs[i]
		llms = append(llms, llm{LLM: m, Downloaded: e.HasLLM(&m), Progress: e.Progress(m.ID)})
	}
	sst := s.speechSettings(r.Context())
	key := s.aiKey(r.Context())
	hint := ""
	if len(key) > 8 {
		hint = "…" + key[len(key)-4:]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": s.aiSettings(r.Context()), "llms": llms, "llmInstalled": e.LLMInstalled(), "cloudModels": ai.CloudModels,
		"keySet": key != "", "keyHint": hint, "sttReady": sst.STT.Enabled,
	})
}

func (s *Server) aiSave(w http.ResponseWriter, r *http.Request) {
	var st ai.Settings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := st.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, ai.SettingsKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "ai.saved", st.Engine)
	s.aiGet(w, r)
}

// aiSetKey stores or removes the Claude API key. It is never sent back.
func (s *Server) aiSetKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	key := strings.TrimSpace(req.Key)
	if key != "" && (!strings.HasPrefix(key, "sk-ant-") || len(key) > 300 || strings.ContainsAny(key, " \r\n\t")) {
		fieldErr(w, "key")
		return
	}
	if err := store.SetSetting(r.Context(), s.DB, ai.KeyKey, key); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if key == "" {
		s.audit(r, "ai.key.removed", "")
	} else {
		s.audit(r, "ai.key.set", "")
	}
	s.aiGet(w, r)
}

// aiTest summarises a sample text with the chosen engine, so an admin sees it works.
func (s *Server) aiTest(w http.ResponseWriter, r *http.Request) {
	var st ai.Settings
	if err := decode(r, &st); err != nil || st.Validate() != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sample := "Good morning, this is Mark de Vries from Bakkerij De Korrel. Our order of last Tuesday has not arrived yet. " +
		"Could you check where it is and call me back before three o'clock? My number is 0503141592. Thank you."
	if st.Language == "nl" || (st.Language == "auto" && s.defaultLanguage(r.Context()) == "nl") {
		sample = "Goedemorgen, u spreekt met Mark de Vries van Bakkerij De Korrel. Onze bestelling van afgelopen dinsdag is nog " +
			"niet binnen. Kunt u nagaan waar die blijft en mij voor drie uur terugbellen? Mijn nummer is 0503141592. Dank u wel."
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	sum, engine, err := s.summarizer(ctx).Summarize(ctx, st, ai.KindVoicemail, sample)
	if err != nil {
		code := "ai_failed"
		switch {
		case errors.Is(err, ai.ErrNoKey):
			code = "ai_no_key"
		case errors.Is(err, speech.ErrNotReady):
			code = "ai_not_ready"
		}
		writeJSON(w, http.StatusConflict, map[string]string{"error": code, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": sum, "engine": engine, "seconds": time.Since(start).Seconds()})
}

// ---- results ------------------------------------------------------------------

// aiResult is stored per recording (rec-<name>.json) and per voicemail audio (vm-<sha>.json).
type aiResult struct {
	Status     string      `json:"status"` // done | failed
	Transcript string      `json:"transcript,omitempty"`
	Summary    *ai.Summary `json:"summary,omitempty"`
	Engine     string      `json:"engine,omitempty"`
	Error      string      `json:"error,omitempty"`
	At         string      `json:"at"`
}

func (s *Server) aiDir() string { return filepath.Join(s.Cfg.DataDir, "speech", "ai") }

func (s *Server) aiLoad(key string) *aiResult {
	b, err := os.ReadFile(filepath.Join(s.aiDir(), key+".json"))
	if err != nil {
		return nil
	}
	var r aiResult
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

func (s *Server) aiStore(key string, r *aiResult) {
	if err := os.MkdirAll(s.aiDir(), 0o750); err != nil {
		return
	}
	r.At = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.Marshal(r)
	tmp := filepath.Join(s.aiDir(), "."+key+".tmp")
	if os.WriteFile(tmp, b, 0o640) == nil {
		_ = os.Rename(tmp, filepath.Join(s.aiDir(), key+".json"))
	}
}

// aiBrief is what lists show: status and topic.
type aiBrief struct {
	Status    string `json:"status"`
	Topic     string `json:"topic,omitempty"`
	Sentiment string `json:"sentiment,omitempty"`
}

func (s *Server) aiBriefOf(key string) *aiBrief {
	r := s.aiLoad(key)
	if r == nil {
		return nil
	}
	b := &aiBrief{Status: r.Status}
	if r.Summary != nil {
		b.Topic, b.Sentiment = r.Summary.Topic, r.Summary.Sentiment
	}
	return b
}

type recordingView struct {
	media.Recording
	AI *aiBrief `json:"ai,omitempty"`
}

func (s *Server) withAI(recs []media.Recording) []recordingView {
	out := make([]recordingView, 0, len(recs))
	for _, r := range recs {
		out = append(out, recordingView{Recording: r, AI: s.aiBriefOf("rec-" + r.Name)})
	}
	return out
}

func (s *Server) recordingAI(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Spool.RecordingPath(r.PathValue("name")); err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	res := s.aiLoad("rec-" + r.PathValue("name"))
	if res == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) portalRecordingAI(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	name := r.PathValue("name")
	if len(s.Spool.Recordings(s.loc(r.Context()), func(rec media.Recording) bool { return rec.Name == name && s.mine(e)(rec) })) == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.recordingAI(w, r)
}

// recordingAIRun transcribes and summarises one recording now (also when automatic AI is off).
func (s *Server) recordingAIRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.Spool.RecordingPath(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !s.sttBusy.TryLock() {
		writeErr(w, http.StatusConflict, "ai_busy")
		return
	}
	defer s.sttBusy.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	res := s.analyseRecording(ctx, name, p)
	s.audit(r, "recording.ai", name)
	writeJSON(w, http.StatusOK, res)
}

// analyseRecording makes the transcript and summary of a recording and stores the result.
func (s *Server) analyseRecording(ctx context.Context, name, path string) *aiResult {
	sst := s.speechSettings(ctx)
	res := &aiResult{Status: "failed"}
	text, err := s.speechEngine().Transcribe(ctx, sst.STT.Model, sst.STT.Language, path)
	if err != nil {
		res.Error = "transcription: " + err.Error()
		if errors.Is(err, speech.ErrNotReady) || ctx.Err() != nil {
			// Not the recording's fault: try again later.
			res.Error = "speech_not_ready"
			return res
		}
		s.aiStore("rec-"+name, res)
		return res
	}
	res.Transcript = text
	if strings.TrimSpace(text) == "" {
		res.Status, res.Error = "done", "no_speech"
		s.aiStore("rec-"+name, res)
		return res
	}
	sum, engine, err := s.summarizer(ctx).Summarize(ctx, s.aiSettings(ctx), ai.KindCall, text)
	res.Engine = engine
	if err != nil {
		res.Error = "summary: " + err.Error()
		if errors.Is(err, ai.ErrNoKey) {
			res.Error = "ai_no_key"
		} else if errors.Is(err, speech.ErrNotReady) {
			res.Error = "ai_not_ready"
		}
		// The transcript alone is still worth keeping.
		res.Status = "done"
		s.aiStore("rec-"+name, res)
		return res
	}
	res.Status, res.Summary = "done", sum
	s.aiStore("rec-"+name, res)
	return res
}

// runAI handles new recordings (newest first) and voicemail summaries. It runs in the
// housekeeping turn, so only one transcription runs at a time.
func (s *Server) runAI(ctx context.Context) {
	st := s.aiSettings(ctx)
	if st.Calls {
		since := time.Now().Add(-7 * 24 * time.Hour)
		recs := s.Spool.Recordings(s.loc(ctx), nil)
		sort.Slice(recs, func(i, j int) bool { return recs[i].Name > recs[j].Name })
		for _, rec := range recs {
			if ctx.Err() != nil {
				return
			}
			p, err := s.Spool.RecordingPath(rec.Name)
			if err != nil {
				continue
			}
			info, err := os.Stat(p)
			// Skip old recordings and calls still being recorded (the file still grows).
			if err != nil || info.ModTime().Before(since) || time.Since(info.ModTime()) < 30*time.Second {
				continue
			}
			if s.aiLoad("rec-"+rec.Name) != nil {
				continue
			}
			if res := s.analyseRecording(ctx, rec.Name, p); res.Error == "speech_not_ready" {
				return
			}
		}
	}
	if st.Voicemail {
		s.summariseSpool(ctx)
	}
	if st.KeepDays > 0 {
		s.purgeAI(time.Now().Add(-time.Duration(st.KeepDays) * 24 * time.Hour))
	}
}

// purgeAI removes results older than the cut-off, and results of recordings that are gone.
func (s *Server) purgeAI(cut time.Time) {
	entries, _ := os.ReadDir(s.aiDir())
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "rec-") || !strings.HasSuffix(n, ".json") {
			continue
		}
		info, err := e.Info()
		_, gone := s.Spool.RecordingPath(strings.TrimSuffix(strings.TrimPrefix(n, "rec-"), ".json"))
		if err == nil && (info.ModTime().Before(cut) || gone != nil) {
			_ = os.Remove(filepath.Join(s.aiDir(), n))
		}
	}
}

// voicemailSummary summarises a voicemail message (cached by audio content).
func (s *Server) voicemailSummary(ctx context.Context, audio []byte, transcript string) *ai.Summary {
	key := "vm-" + sumHex(audio)
	if r := s.aiLoad(key); r != nil {
		return r.Summary
	}
	if strings.TrimSpace(transcript) == "" {
		return nil
	}
	sum, engine, err := s.summarizer(ctx).Summarize(ctx, s.aiSettings(ctx), ai.KindVoicemail, transcript)
	if err != nil {
		if !errors.Is(err, ai.ErrNoKey) && !errors.Is(err, speech.ErrNotReady) {
			s.Log.Warn("voicemail summary", "err", err)
			s.aiStore(key, &aiResult{Status: "failed", Engine: engine, Error: err.Error()})
		}
		return nil
	}
	s.aiStore(key, &aiResult{Status: "done", Summary: sum, Engine: engine})
	return sum
}

func (s *Server) voicemailSummaryCached(audio []byte) *ai.Summary {
	if r := s.aiLoad("vm-" + sumHex(audio)); r != nil {
		return r.Summary
	}
	return nil
}

// summariseSpool summarises voicemail messages that have a transcript but no summary yet.
func (s *Server) summariseSpool(ctx context.Context) {
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
			if t := s.transcript(b); t != "" && s.aiLoad("vm-"+sumHex(b)) == nil {
				sum := s.voicemailSummary(ctx, b, t)
				if sum == nil && s.aiLoad("vm-"+sumHex(b)) == nil {
					return // engine not ready: try again next turn
				}
				// A new message: show the summary on the mobile app too.
				if at, err := time.Parse(time.RFC3339, m.Date); sum != nil && m.Folder == "INBOX" && err == nil && time.Since(at) < time.Hour {
					go s.pushVoicemailSummary(e.Number, m.CallerID, sum.Summary)
				}
			}
		}
	}
}
