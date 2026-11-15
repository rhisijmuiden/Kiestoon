package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

// The user portal: every signed-in account that is linked to an extension manages its own phone here.

func (s *Server) portalRoutes(mux *http.ServeMux, user func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/portal/me", user(s.portalMe))
	mux.Handle("PUT /api/portal/features", user(s.portalFeatures))
	mux.Handle("PUT /api/portal/voicemail", user(s.portalVoicemailSettings))
	mux.Handle("GET /api/portal/voicemail", user(s.portalMessages))
	mux.Handle("GET /api/portal/voicemail/{folder}/{id}/audio", user(s.portalMessageAudio))
	mux.Handle("POST /api/portal/voicemail/{folder}/{id}/read", user(s.portalMessageRead))
	mux.Handle("DELETE /api/portal/voicemail/{folder}/{id}", user(s.portalMessageDelete))
	mux.Handle("GET /api/portal/greetings", user(s.portalGreetings))
	mux.Handle("GET /api/portal/greetings/{kind}", user(s.portalGreetingAudio))
	mux.Handle("PUT /api/portal/greetings/{kind}", user(s.portalSaveGreeting))
	mux.Handle("DELETE /api/portal/greetings/{kind}", user(s.portalDeleteGreeting))
	mux.Handle("GET /api/portal/webrtc", user(s.portalWebRTC))
	mux.Handle("GET /api/portal/sip", user(s.portalSIP))
	mux.Handle("GET /api/portal/queues", user(s.portalQueues))
	mux.Handle("POST /api/portal/queues/{id}", user(s.portalQueueAction))
	mux.Handle("GET /api/portal/groups", user(s.portalGroups))
	mux.Handle("GET /api/portal/colleagues", user(s.portalColleagues))
	mux.Handle("POST /api/portal/call", user(s.portalCall))
	mux.Handle("GET /api/portal/history", user(s.portalHistory))
	mux.Handle("GET /api/portal/history/journey/{id}", user(s.portalJourney))
	mux.Handle("GET /api/portal/recordings", user(s.portalRecordings))
	mux.Handle("GET /api/portal/recordings/{name}", user(s.portalRecordingAudio))
	mux.Handle("GET /api/portal/phones", user(s.portalPhones))
	mux.Handle("PUT /api/portal/phones/{id}/keys", user(s.portalPhoneKeys))
	// The WebSocket of the browser softphone, proxied to Asterisk's HTTP server on 127.0.0.1.
	mux.Handle("GET /api/webrtc/ws", user(s.webrtcProxy))
}

// myExtension returns the extension of the signed-in user, or writes an error.
func (s *Server) myExtension(w http.ResponseWriter, r *http.Request) *pbx.Extension {
	u := sessionFrom(r).User
	if u.ExtensionID == nil {
		writeErr(w, http.StatusConflict, "no_extension")
		return nil
	}
	e, err := s.PBX.Extension(r.Context(), *u.ExtensionID)
	if err != nil {
		writeErr(w, http.StatusConflict, "no_extension")
		return nil
	}
	return e
}

func (s *Server) portalMe(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	tcs, _ := s.PBX.TimeConditions(r.Context())
	type sched struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	schedules := []sched{}
	for _, t := range tcs {
		schedules = append(schedules, sched{t.ID, t.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": e.ID, "number": e.Number, "name": e.Name, "email": e.Email, "features": e.Features, "voicemail": e.Voicemail,
		"voicemailEmail": e.VoicemailEmail, "webrtc": e.WebRTC, "recordMode": e.RecordMode, "outboundClass": e.OutboundClass,
		"schedules": schedules, "codes": pbx.FeatureCodes,
	})
}

func (s *Server) portalFeatures(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var f pbx.Features
	if err := decode(r, &f); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.PBX.SaveFeatures(r.Context(), e.ID, f); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "portal.features", e.Number)
	s.Ast.Changed()
	s.States.Invalidate()
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) portalVoicemailSettings(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var req struct {
		PIN   string `json:"pin"`
		Email bool   `json:"email"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.PIN != "" {
		e.VoicemailPIN = req.PIN
	}
	e.VoicemailEmail = req.Email
	if err := s.PBX.SaveExtension(r.Context(), s.plan(r.Context()), e); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "portal.voicemail", e.Number)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalMessages(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	msgs, err := s.Spool.Messages(e.Number)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if st := s.speechSettings(r.Context()); st.STT.Enabled && st.STT.Voicemail {
		for i := range msgs {
			if p, err := s.Spool.MessageAudio(e.Number, msgs[i].Folder, msgs[i].ID); err == nil {
				if b, err := os.ReadFile(p); err == nil {
					msgs[i].Transcript = s.transcript(b)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func msgID(r *http.Request) (string, int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	folder := r.PathValue("folder")
	return folder, id, err == nil && slices.Contains(media.Folders, folder)
}

func (s *Server) portalMessageAudio(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	folder, id, ok := msgID(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	p, err := s.Spool.MessageAudio(e.Number, folder, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	serveWAV(w, r, p)
}

func (s *Server) portalMessageRead(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	folder, id, ok := msgID(r)
	if !ok || folder != "INBOX" {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.Spool.MarkRead(e.Number, id); err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.AMI.VoicemailRefresh(r.Context(), e.Number)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalMessageDelete(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	folder, id, ok := msgID(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.Spool.Delete(e.Number, folder, id); err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	s.AMI.VoicemailRefresh(r.Context(), e.Number)
	s.audit(r, "portal.voicemail.deleted", e.Number)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalGreetings(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.Spool.Greetings(e.Number))
}

func (s *Server) portalGreetingAudio(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	p, err := s.Spool.GreetingPath(e.Number, r.PathValue("kind"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	serveWAV(w, r, p)
}

func (s *Server) portalSaveGreeting(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var req struct {
		Audio string `json:"audio"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	audio, _, err := decodeWAV(req.Audio)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_audio")
		return
	}
	if err := s.Spool.SaveGreeting(e.Number, r.PathValue("kind"), audio); err != nil {
		if errors.Is(err, media.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		s.Log.Error("save greeting", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "portal.greeting", e.Number+" "+r.PathValue("kind"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalDeleteGreeting(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	s.Spool.DeleteGreeting(e.Number, r.PathValue("kind"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// portalWebRTC gives the browser softphone its account. The password never leaves the
// signed-in session and only works over the proxied WebSocket.
func (s *Server) portalWebRTC(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	if !e.WebRTC {
		writeErr(w, http.StatusConflict, "webrtc_disabled")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wsPath": "/api/webrtc/ws", "username": e.WebRTCEndpoint(), "password": e.WebRTCSecret, "number": e.Number,
		"name": e.Name, "domain": "kiestoon.local", "secureContext": s.secure(r),
	})
}

func (s *Server) portalSIP(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	st, _ := pbx.LoadSettings(r.Context(), s.DB)
	host := s.sipHost(r)
	port := st.SIPPort
	if e.Transport == "tls" {
		port = st.TLSPort
	}
	s.audit(r, "portal.sip_details", e.Number)
	writeJSON(w, http.StatusOK, map[string]any{"server": host, "port": port, "transport": e.Transport, "username": e.Number,
		"password": e.SIPSecret, "srtp": e.SRTP})
}

func splitHost(hostport string) (string, string, error) {
	u, err := url.Parse("//" + hostport)
	if err != nil {
		return "", "", err
	}
	return u.Hostname(), u.Port(), nil
}

// ---- queues and groups ------------------------------------------------------

func (s *Server) portalQueues(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	views := s.queueViews(r.Context(), func(q pbx.Queue) bool {
		for _, m := range q.Members {
			if m.ExtensionID == e.ID {
				return true
			}
		}
		return false
	})
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) portalQueueAction(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var req struct {
		Action string `json:"action"` // login, logout, pause, unpause
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	qid := pathID(r)
	if qid != 0 {
		q, err := s.PBX.Queue(r.Context(), qid)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		member := false
		for _, m := range q.Members {
			if m.ExtensionID == e.ID && (m.Dynamic || req.Action == "pause" || req.Action == "unpause") {
				member = true
			}
		}
		if !member {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	if err := s.agentAction(r.Context(), qid, e.ID, req.Action); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.audit(r, "portal.queue."+req.Action, fmt.Sprintf("%s queue %d", e.Number, qid))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalGroups(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	rgs, _ := s.PBX.RingGroups(r.Context())
	states := map[string]string{}
	if s.AMI.Connected() {
		states = s.States.Extensions(r.Context())
	}
	exts, _ := s.PBX.Extensions(r.Context())
	names := map[string]string{}
	for _, x := range exts {
		names[x.Number] = x.Name
	}
	type member struct {
		Number string `json:"number"`
		Name   string `json:"name"`
		State  string `json:"state"`
	}
	type group struct {
		ID       int64    `json:"id"`
		Number   string   `json:"number"`
		Name     string   `json:"name"`
		Strategy string   `json:"strategy"`
		Members  []member `json:"members"`
	}
	out := []group{}
	for _, g := range rgs {
		if !slices.Contains(g.Members, e.Number) {
			continue
		}
		gr := group{ID: g.ID, Number: g.Number, Name: g.Name, Strategy: g.Strategy, Members: []member{}}
		for _, m := range g.Members {
			st := states[m]
			if names[m] == "" {
				st = "external"
			}
			gr.Members = append(gr.Members, member{m, names[m], st})
		}
		out = append(out, gr)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) portalColleagues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.blfList(r.Context()))
}

var reDial = regexp.MustCompile(`^\+?[0-9*#]{2,20}$`)

func (s *Server) portalCall(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var req struct {
		Number string `json:"number"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	n := pbx.NormalizeDID(req.Number)
	if !reDial.MatchString(n) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "number"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.AMI.Originate(ctx, e.Number, e.Name, n); err != nil {
		writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		return
	}
	s.audit(r, "portal.call", e.Number+" -> "+n)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalHistory(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	f := cdrFilter(r)
	f.Ext = e.Number
	recs, total, err := s.CDR.Search(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": recs, "total": total})
}

func (s *Server) mine(e *pbx.Extension) func(media.Recording) bool {
	return func(rec media.Recording) bool { return rec.From == e.Number || rec.To == e.Number }
}

func (s *Server) portalRecordings(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.Spool.Recordings(s.loc(r.Context()), s.mine(e)))
}

func (s *Server) portalRecordingAudio(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	name := r.PathValue("name")
	rec, ok := media.ParseRecording(name, 0, time.UTC)
	if !ok || !s.mine(e)(rec) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	p, err := s.Spool.RecordingPath(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	serveWAV(w, r, p)
}

// ---- own phones -------------------------------------------------------------

func (s *Server) portalPhones(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	phones, _ := s.PBX.Phones(r.Context())
	type view struct {
		pbx.Phone
		MaxKeys     int                    `json:"maxKeys"`
		Lines       int                    `json:"maxLines"`
		ModuleTypes []provision.ModuleType `json:"moduleTypes"`
	}
	out := []view{}
	for _, p := range phones {
		if p.ExtensionID != nil && *p.ExtensionID == e.ID {
			max, lines := 0, 1
			if m := provision.FindModel(p.Vendor, p.Model); m != nil {
				max, lines = m.Keys, m.MaxLines()
			}
			mts := []provision.ModuleType{}
			for _, mt := range provision.ModuleTypes {
				for _, m := range p.Modules {
					if mt.Vendor == p.Vendor && mt.ID == m.Type {
						mts = append(mts, mt)
						break
					}
				}
			}
			out = append(out, view{p, max, lines, mts})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) portalPhoneKeys(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil || p.ExtensionID == nil || *p.ExtensionID != e.ID {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var req struct {
		Keys    []pbx.Key    `json:"keys"`
		Modules []pbx.Module `json:"modules"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	lim, _ := provision.Limits(p.Vendor, p.Model)
	p.Keys = req.Keys
	// Users fill in the keys of the modules the admin set up; they cannot add or change modules.
	if req.Modules != nil {
		if len(req.Modules) != len(p.Modules) {
			fieldErr(w, "modules")
			return
		}
		for i := range p.Modules {
			if req.Modules[i].Type != p.Modules[i].Type {
				fieldErr(w, "modules")
				return
			}
			p.Modules[i].Keys = req.Modules[i].Keys
		}
	}
	if err := s.PBX.SavePhone(r.Context(), p, lim); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "portal.phone_keys", p.MAC)
	state, _ := s.notifyPhone(r.Context(), p, s.registeredAs(r.Context(), p))
	writeJSON(w, http.StatusOK, struct {
		*pbx.Phone
		Resync string `json:"resync"`
	}{p, state})
}

// ---- WebRTC -----------------------------------------------------------------

func (s *Server) webrtcProxy(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	// Only the browser of this portal may open the socket.
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != r.Host {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	// A call can last hours: lift the server's read and write timeouts for this connection.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	target, _ := url.Parse("http://" + s.Cfg.AsteriskHTTP)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = "/ws"
			pr.Out.Host = target.Host
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Origin")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.Log.Warn("webrtc proxy", "err", err)
			writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		},
	}
	proxy.ServeHTTP(w, r)
}
