package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) businessRoutes(mux *http.ServeMux, admin, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/pbx/ringgroups", admin(list(s, s.PBX.RingGroups)))
	mux.Handle("POST /api/pbx/ringgroups", admin(s.saveRingGroup))
	mux.Handle("PUT /api/pbx/ringgroups/{id}", admin(s.saveRingGroup))
	mux.Handle("DELETE /api/pbx/ringgroups/{id}", admin(s.deleter("ringgroup", s.PBX.DeleteRingGroup)))

	mux.Handle("GET /api/pbx/queues", admin(list(s, s.PBX.Queues)))
	mux.Handle("POST /api/pbx/queues", admin(s.saveQueue))
	mux.Handle("PUT /api/pbx/queues/{id}", admin(s.saveQueue))
	mux.Handle("DELETE /api/pbx/queues/{id}", admin(s.deleter("queue", s.PBX.DeleteQueue)))

	mux.Handle("GET /api/pbx/ivrs", admin(list(s, s.PBX.IVRs)))
	mux.Handle("POST /api/pbx/ivrs", admin(s.saveIVR))
	mux.Handle("PUT /api/pbx/ivrs/{id}", admin(s.saveIVR))
	mux.Handle("DELETE /api/pbx/ivrs/{id}", admin(s.deleter("ivr", s.PBX.DeleteIVR)))

	mux.Handle("GET /api/pbx/timeconditions", recep(list(s, s.PBX.TimeConditions)))
	mux.Handle("POST /api/pbx/timeconditions", admin(s.saveTimeCondition))
	mux.Handle("PUT /api/pbx/timeconditions/{id}", admin(s.saveTimeCondition))
	mux.Handle("PUT /api/pbx/timeconditions/{id}/mode", recep(s.setTimeConditionMode))
	mux.Handle("DELETE /api/pbx/timeconditions/{id}", admin(s.deleter("timecondition", s.PBX.DeleteTimeCondition)))
	mux.Handle("GET /api/pbx/holidays", recep(s.holidays))
	mux.Handle("PUT /api/pbx/holidays", admin(s.saveHolidays))

	mux.Handle("GET /api/pbx/conferences", admin(list(s, s.PBX.Conferences)))
	mux.Handle("POST /api/pbx/conferences", admin(s.saveConference))
	mux.Handle("PUT /api/pbx/conferences/{id}", admin(s.saveConference))
	mux.Handle("DELETE /api/pbx/conferences/{id}", admin(s.deleter("conference", s.PBX.DeleteConference)))

	mux.Handle("GET /api/pbx/prompts", admin(list(s, s.PBX.Prompts)))
	mux.Handle("POST /api/pbx/prompts", admin(s.savePrompt))
	mux.Handle("PUT /api/pbx/prompts/{id}", admin(s.savePrompt))
	mux.Handle("GET /api/pbx/prompts/{id}/audio", admin(s.promptAudio))
	mux.Handle("DELETE /api/pbx/prompts/{id}", admin(s.deletePrompt))

	mux.Handle("GET /api/pbx/features", admin(s.featureSettings))
	mux.Handle("PUT /api/pbx/features", admin(s.saveFeatureSettings))
	mux.Handle("POST /api/pbx/fraud/unlock", admin(s.fraudUnlock))
	mux.Handle("GET /api/pbx/featurecodes", recep(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, pbx.FeatureCodes) }))
	mux.Handle("GET /api/pbx/destinations", admin(s.destinations))

	mux.Handle("GET /api/recordings", recep(s.recordings))
	mux.Handle("GET /api/recordings/{name}", recep(s.recordingAudio))
	mux.Handle("DELETE /api/recordings/{name}", admin(s.deleteRecording))

	mux.Handle("GET /api/reception/queues", recep(s.receptionQueues))
	mux.Handle("POST /api/reception/queues/{id}/agents/{ext}", recep(s.receptionAgent))
	mux.Handle("GET /api/reception/blf", recep(s.blf))
}

// list serves a repository list function.
func list[T any](s *Server, fn func(context.Context) ([]T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := fn(r.Context())
		if err != nil {
			s.saveErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (s *Server) deleter(what string, fn func(context.Context, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(r.Context(), pathID(r)); err != nil {
			s.saveErr(w, err)
			return
		}
		s.audit(r, what+".deleted", r.PathValue("id"))
		s.Ast.Changed()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// saved finishes a save: audit, regenerate, answer.
func (s *Server) saved(w http.ResponseWriter, r *http.Request, what, detail string, v any) {
	s.audit(r, what+".saved", detail)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) saveRingGroup(w http.ResponseWriter, r *http.Request) {
	var g pbx.RingGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	g.ID = pathID(r)
	if err := s.PBX.SaveRingGroup(r.Context(), s.plan(r.Context()), &g); err != nil {
		s.saveErr(w, err)
		return
	}
	s.saved(w, r, "ringgroup", g.Number, g)
}

func (s *Server) saveQueue(w http.ResponseWriter, r *http.Request) {
	var q pbx.Queue
	if err := decode(r, &q); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	q.ID = pathID(r)
	if err := s.PBX.SaveQueue(r.Context(), s.plan(r.Context()), &q); err != nil {
		s.saveErr(w, err)
		return
	}
	saved, _ := s.PBX.Queue(r.Context(), q.ID)
	s.saved(w, r, "queue", q.Number, saved)
}

func (s *Server) saveIVR(w http.ResponseWriter, r *http.Request) {
	var v pbx.IVR
	if err := decode(r, &v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	v.ID = pathID(r)
	if err := s.PBX.SaveIVR(r.Context(), &v); err != nil {
		s.saveErr(w, err)
		return
	}
	s.saved(w, r, "ivr", v.Name, v)
}

func (s *Server) saveTimeCondition(w http.ResponseWriter, r *http.Request) {
	var t pbx.TimeCondition
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t.ID = pathID(r)
	if err := s.PBX.SaveTimeCondition(r.Context(), &t); err != nil {
		s.saveErr(w, err)
		return
	}
	s.saved(w, r, "timecondition", t.Name, t)
}

func (s *Server) setTimeConditionMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.PBX.SetTimeConditionMode(r.Context(), pathID(r), req.Mode); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "timecondition.mode", fmt.Sprintf("%s %s", r.PathValue("id"), req.Mode))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]string{"mode": req.Mode})
}

func (s *Server) holidays(w http.ResponseWriter, r *http.Request) {
	h, err := pbx.LoadHolidays(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	if year < 2000 || year > 2100 {
		year = time.Now().Year()
	}
	writeJSON(w, http.StatusOK, map[string]any{"calendar": h, "year": year, "days": h.ForYear(year), "presets": pbx.DutchHolidays})
}

func (s *Server) saveHolidays(w http.ResponseWriter, r *http.Request) {
	var h pbx.Holidays
	if err := decode(r, &h); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := h.Validate(); err != nil {
		s.saveErr(w, err)
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, pbx.HolidaysKey, h); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.saved(w, r, "holidays", "", h)
}

func (s *Server) saveConference(w http.ResponseWriter, r *http.Request) {
	var c pbx.Conference
	if err := decode(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	c.ID = pathID(r)
	if err := s.PBX.SaveConference(r.Context(), s.plan(r.Context()), &c); err != nil {
		s.saveErr(w, err)
		return
	}
	s.saved(w, r, "conference", c.Number, c)
}

// ---- prompts ----------------------------------------------------------------

func (s *Server) promptDir() string { return filepath.Join(s.Cfg.PBXDir(), "prompts") }

func (s *Server) promptFile(id int64) string {
	return filepath.Join(s.promptDir(), strconv.FormatInt(id, 10)+".wav")
}

// decodeWAV reads base64 audio from a JSON request and checks the format.
func decodeWAV(b64 string) ([]byte, int64, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) > media.MaxPromptBytes+44 {
		return nil, 0, media.ErrFormat
	}
	ms, err := media.CheckWAV(raw)
	return raw, ms, err
}

func (s *Server) savePrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Audio string `json:"audio"` // base64 WAV, 8 kHz mono 16 bit; empty on rename
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	p := pbx.Prompt{ID: pathID(r), Name: req.Name}
	var audio []byte
	if req.Audio != "" {
		var err error
		if audio, p.DurationMS, err = decodeWAV(req.Audio); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_audio")
			return
		}
	} else if p.ID == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_audio")
		return
	} else {
		old, err := s.PBX.Prompt(r.Context(), p.ID)
		if err != nil {
			s.saveErr(w, err)
			return
		}
		p.DurationMS = old.DurationMS
	}
	if err := s.PBX.SavePrompt(r.Context(), &p); err != nil {
		s.saveErr(w, err)
		return
	}
	if audio != nil {
		if err := os.MkdirAll(s.promptDir(), 0o2750); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
		if err := writeFileAtomic(s.promptFile(p.ID), audio, 0o640); err != nil {
			s.Log.Error("write prompt", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal")
			return
		}
	}
	s.audit(r, "prompt.saved", p.Name)
	writeJSON(w, http.StatusOK, p)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Server) promptAudio(w http.ResponseWriter, r *http.Request) {
	serveWAV(w, r, s.promptFile(pathID(r)))
}

func serveWAV(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func (s *Server) deletePrompt(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if used, err := s.PBX.PromptInUse(r.Context(), id); err != nil || used {
		writeErr(w, http.StatusConflict, "prompt_in_use")
		return
	}
	if err := s.PBX.DeletePrompt(r.Context(), id); err != nil {
		s.saveErr(w, err)
		return
	}
	_ = os.Remove(s.promptFile(id))
	s.audit(r, "prompt.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- feature settings -------------------------------------------------------

func (s *Server) featureSettings(w http.ResponseWriter, r *http.Request) {
	f, err := pbx.LoadFeatureSettings(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	locked, _ := store.GetSetting(r.Context(), s.DB, pbx.FraudLockKey, "")
	writeJSON(w, http.StatusOK, map[string]any{"settings": f, "fraudLockedAt": locked, "codes": pbx.FeatureCodes})
}

func (s *Server) saveFeatureSettings(w http.ResponseWriter, r *http.Request) {
	var f pbx.FeatureSettings
	if err := decode(r, &f); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := f.Validate(); err != nil {
		s.saveErr(w, err)
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, pbx.FeaturesKey, f); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.saved(w, r, "features", "", f)
}

func (s *Server) fraudUnlock(w http.ResponseWriter, r *http.Request) {
	_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM settings WHERE key = ?`, pbx.FraudLockKey)
	s.audit(r, "fraud.unlocked", "")
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// destinations lists everything a call can be sent to, for the destination pickers in the UI.
func (s *Server) destinations(w http.ResponseWriter, r *http.Request) {
	type opt struct {
		Type   string `json:"type"`
		Target string `json:"target"`
		Label  string `json:"label"`
	}
	out := []opt{}
	ctx := r.Context()
	exts, _ := s.PBX.Extensions(ctx)
	for _, e := range exts {
		out = append(out, opt{pbx.DestExtension, e.Number, e.Number + " " + e.Name})
		if e.Voicemail {
			out = append(out, opt{pbx.DestVoicemail, e.Number, e.Number + " " + e.Name})
		}
	}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	rgs, _ := s.PBX.RingGroups(ctx)
	for _, g := range rgs {
		out = append(out, opt{pbx.DestRingGroup, id(g.ID), g.Number + " " + g.Name})
	}
	qs, _ := s.PBX.Queues(ctx)
	for _, q := range qs {
		out = append(out, opt{pbx.DestQueue, id(q.ID), q.Number + " " + q.Name})
	}
	ivrs, _ := s.PBX.IVRs(ctx)
	for _, v := range ivrs {
		out = append(out, opt{pbx.DestIVR, id(v.ID), v.Name})
	}
	tcs, _ := s.PBX.TimeConditions(ctx)
	for _, t := range tcs {
		out = append(out, opt{pbx.DestTimeCondition, id(t.ID), t.Name})
	}
	cs, _ := s.PBX.Conferences(ctx)
	for _, c := range cs {
		out = append(out, opt{pbx.DestConference, id(c.ID), c.Number + " " + c.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": pbx.DestinationTypes, "options": out})
}

// ---- recordings -------------------------------------------------------------

func (s *Server) loc(ctx context.Context) *time.Location {
	st, _ := pbx.LoadSettings(ctx, s.DB)
	if l, err := time.LoadLocation(st.Timezone); err == nil {
		return l
	}
	return time.Local
}

func (s *Server) recordings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	recs := s.Spool.Recordings(s.loc(r.Context()), func(rec media.Recording) bool {
		return q == "" || rec.From == q || rec.To == q || rec.To == "q"+q
	})
	if len(recs) > 500 {
		recs = recs[:500]
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *Server) recordingAudio(w http.ResponseWriter, r *http.Request) {
	p, err := s.Spool.RecordingPath(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "recording.played", r.PathValue("name"))
	serveWAV(w, r, p)
}

func (s *Server) deleteRecording(w http.ResponseWriter, r *http.Request) {
	if err := s.Spool.DeleteRecording(r.PathValue("name")); err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.audit(r, "recording.deleted", r.PathValue("name"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- reception --------------------------------------------------------------

// queueView combines the configuration of a queue with its live state.
type queueView struct {
	pbx.Queue
	Live *liveQueue `json:"live"`
}

type liveQueue struct {
	Calls       int     `json:"calls"`
	LongestWait int     `json:"longestWait"`
	Holdtime    int     `json:"holdtime"`
	Completed   int     `json:"completed"`
	Abandoned   int     `json:"abandoned"`
	ServiceLvl  float64 `json:"serviceLevel"`
	Callers     any     `json:"callers"`
	Agents      any     `json:"agents"`
}

func (s *Server) queueViews(ctx context.Context, keep func(pbx.Queue) bool) []queueView {
	qs, _ := s.PBX.Queues(ctx)
	live := map[int64]*liveQueue{}
	if s.AMI.Connected() {
		for _, st := range s.States.Queues(ctx) {
			live[st.ID] = &liveQueue{Calls: st.Calls, LongestWait: st.LongestWait, Holdtime: st.Holdtime, Completed: st.Completed,
				Abandoned: st.Abandoned, ServiceLvl: st.ServiceLvl, Callers: st.Callers, Agents: st.Members}
		}
	}
	out := []queueView{}
	for _, q := range qs {
		if keep != nil && !keep(q) {
			continue
		}
		out = append(out, queueView{Queue: q, Live: live[q.ID]})
	}
	return out
}

func (s *Server) receptionQueues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.queueViews(r.Context(), nil))
}

// agentAction signs an agent in or out, or pauses them.
func (s *Server) agentAction(ctx context.Context, queueID, extID int64, action string) error {
	switch action {
	case "login", "logout":
		if _, err := s.PBX.SetQueueLogin(ctx, queueID, extID, action == "login"); err != nil {
			return err
		}
		s.Ast.Changed()
	case "pause", "unpause":
		if err := s.PBX.SetQueuePause(ctx, queueID, extID, action == "pause"); err != nil {
			return err
		}
		e, err := s.PBX.Extension(ctx, extID)
		if err != nil {
			return err
		}
		qs, _ := s.PBX.Queues(ctx)
		for _, q := range qs {
			if queueID == 0 || q.ID == queueID {
				_ = s.AMI.QueuePause(ctx, fmt.Sprintf("kiestoon-q-%d", q.ID), e.Number, action == "pause")
			}
		}
	default:
		return errors.New("bad action")
	}
	s.States.Invalidate()
	return nil
}

func (s *Server) receptionAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ext, _ := strconv.ParseInt(r.PathValue("ext"), 10, 64)
	if err := s.agentAction(r.Context(), pathID(r), ext, req.Action); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.audit(r, "queue.agent."+req.Action, fmt.Sprintf("queue %s ext %d", r.PathValue("id"), ext))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// blf returns every extension with its live state (busy lamp field).
func (s *Server) blf(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.blfList(r.Context()))
}

type blfEntry struct {
	Number string `json:"number"`
	Name   string `json:"name"`
	State  string `json:"state"`
	DND    bool   `json:"dnd"`
}

func (s *Server) blfList(ctx context.Context) []blfEntry {
	exts, _ := s.PBX.Extensions(ctx)
	states := map[string]string{}
	if s.AMI.Connected() {
		states = s.States.Extensions(ctx)
	}
	out := []blfEntry{}
	for _, e := range exts {
		if !e.Enabled {
			continue
		}
		st := states[e.Number]
		if st == "" {
			st = "unknown"
		}
		out = append(out, blfEntry{Number: e.Number, Name: e.Name, State: st, DND: e.Features.DND})
	}
	return out
}
