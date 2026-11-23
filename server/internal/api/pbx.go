package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/quality"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) pbxRoutes(mux *http.ServeMux, admin, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/pbx/status", admin(s.pbxStatus))
	mux.Handle("POST /api/pbx/restart", admin(s.pbxRestart))
	mux.Handle("GET /api/pbx/settings", admin(s.pbxSettings))
	mux.Handle("PUT /api/pbx/settings", admin(s.pbxSaveSettings))
	mux.Handle("GET /api/pbx/numberplan", admin(s.numberPlan))
	mux.Handle("POST /api/pbx/numberplan/preview", admin(s.numberPlanPreview))
	mux.Handle("POST /api/pbx/numberplan/apply", admin(s.numberPlanApply))

	mux.Handle("GET /api/pbx/extensions", admin(s.listExtensions))
	mux.Handle("GET /api/pbx/extensions/next", admin(s.nextExtension))
	mux.Handle("POST /api/pbx/extensions", admin(s.saveExtension))
	mux.Handle("GET /api/pbx/extensions/{id}", admin(s.getExtension))
	mux.Handle("PUT /api/pbx/extensions/{id}", admin(s.saveExtension))
	mux.Handle("DELETE /api/pbx/extensions/{id}", admin(s.deleteExtension))
	mux.Handle("POST /api/pbx/extensions/{id}/secret", admin(s.resetExtensionSecret))

	mux.Handle("GET /api/pbx/trunks", admin(s.listTrunks))
	mux.Handle("GET /api/pbx/trunks/presets", admin(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, pbx.TrunkPresets) }))
	mux.Handle("POST /api/pbx/trunks", admin(s.saveTrunk))
	mux.Handle("PUT /api/pbx/trunks/{id}", admin(s.saveTrunk))
	mux.Handle("DELETE /api/pbx/trunks/{id}", admin(s.deleteTrunk))
	mux.Handle("POST /api/pbx/trunks/{id}/register", admin(s.reregisterTrunk))
	mux.Handle("GET /api/pbx/numbers", admin(s.listNumbers))

	mux.Handle("GET /api/pbx/inbound", admin(s.listInbound))
	mux.Handle("POST /api/pbx/inbound", admin(s.saveInbound))
	mux.Handle("PUT /api/pbx/inbound/{id}", admin(s.saveInbound))
	mux.Handle("DELETE /api/pbx/inbound/{id}", admin(s.deleteInbound))

	mux.Handle("GET /api/pbx/outbound", admin(s.listOutbound))
	mux.Handle("POST /api/pbx/outbound", admin(s.saveOutbound))
	mux.Handle("PUT /api/pbx/outbound/order", admin(s.reorderOutbound))
	mux.Handle("PUT /api/pbx/outbound/{id}", admin(s.saveOutbound))
	mux.Handle("DELETE /api/pbx/outbound/{id}", admin(s.deleteOutbound))

	mux.Handle("GET /api/live", recep(s.live))
	mux.Handle("GET /api/cdr", recep(s.cdrSearch))
	mux.Handle("GET /api/cdr/export", recep(s.cdrExport))
	mux.Handle("GET /api/cdr/journey/{id}", recep(s.cdrJourney))
}

// ---- helpers ----------------------------------------------------------------

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// saveErr maps repository errors to responses the UI can translate.
func (s *Server) saveErr(w http.ResponseWriter, err error) {
	var ve *pbx.ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ve.Code, "field": ve.Field})
	case errors.Is(err, pbx.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found")
	default:
		s.Log.Error("save failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal")
	}
}

func fieldErr(w http.ResponseWriter, field string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": field})
}

func (s *Server) audit(r *http.Request, action, detail string) {
	uid := int64(0)
	if sess := sessionFrom(r); sess != nil {
		uid = sess.User.ID
	}
	store.Audit(r.Context(), s.DB, uid, action, detail, clientIP(r))
}

func (s *Server) plan(ctx context.Context) pbx.NumberPlan {
	st, _ := pbx.LoadSettings(ctx, s.DB)
	return pbx.PlanFor(st.Digits)
}

func (s *Server) asteriskOptions(ctx context.Context) asterisk.Options {
	o := asterisk.Options{AMIUser: "kiestoon", AMISecret: s.AMI.Secret, MailCmd: "/usr/bin/kiestoon sendmail -t", PromptDir: s.promptDir(), FaxDir: s.faxSpool(),
		SpeechDir: s.speechSpool(), AGIAddr: s.agiAddr()}
	if cert, key, public, err := certs.Current(s.Cfg.PBXDir()); err == nil {
		o.TLSCert, o.TLSKey = cert, key
		if public {
			o.PublicCert, o.PublicKey = cert, key
		}
	}
	if m, err := mail.Load(ctx, s.DB); err == nil {
		o.MailFrom = m.FromAddress
	}
	o.Organization, _ = store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	return o
}

// ---- status and settings ----------------------------------------------------

func (s *Server) pbxStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Ast.Status(r.Context()))
}

func (s *Server) pbxRestart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Now bool `json:"now"`
	}
	_ = decode(r, &req)
	err := s.Ast.Restart(r.Context(), req.Now)
	if err != nil {
		// Asterisk not reachable over AMI: ask the helper to restart the service.
		err = s.Helper.Call(r.Context(), "asterisk.restart", nil, nil)
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		return
	}
	s.audit(r, "pbx.restart", fmt.Sprint(req.Now))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) pbxSettings(w http.ResponseWriter, r *http.Request) {
	st, err := pbx.LoadSettings(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) pbxSaveSettings(w http.ResponseWriter, r *http.Request) {
	var st pbx.Settings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	old, _ := pbx.LoadSettings(r.Context(), s.DB)
	// The number length only changes through the number plan migration.
	st.Digits = old.Digits
	if st.EmergencyTrunkIDs == nil {
		st.EmergencyTrunkIDs = []int64{}
	}
	if err := st.Validate(); err != nil {
		s.saveErr(w, err)
		return
	}
	if err := store.SetJSON(r.Context(), s.DB, pbx.SettingsKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "pbx.settings", "")
	s.Ast.Changed()
	s.firewallChanged(r.Context())
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) numberPlan(w http.ResponseWriter, r *http.Request) {
	st, _ := pbx.LoadSettings(r.Context(), s.DB)
	plans := map[int]pbx.NumberPlan{}
	for d := 3; d <= 5; d++ {
		plans[d] = pbx.PlanFor(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": pbx.PlanFor(st.Digits), "options": plans})
}

func (s *Server) numberPlanPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Digits int `json:"digits"`
	}
	if err := decode(r, &req); err != nil || req.Digits < 3 || req.Digits > 5 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	changes, conflicts, err := s.PBX.PlanMigrationAll(r.Context(), req.Digits)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "conflicts": conflicts, "plan": pbx.PlanFor(req.Digits)})
}

func (s *Server) numberPlanApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Digits int `json:"digits"`
	}
	if err := decode(r, &req); err != nil || req.Digits < 3 || req.Digits > 5 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	st, _ := pbx.LoadSettings(r.Context(), s.DB)
	changes, err := s.PBX.ApplyPlanMigration(r.Context(), st, req.Digits)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "pbx.numberplan", fmt.Sprintf("%d digits, %d renumbered", req.Digits, len(changes)))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes})
}

// ---- extensions -------------------------------------------------------------

func (s *Server) listExtensions(w http.ResponseWriter, r *http.Request) {
	exts, err := s.PBX.Extensions(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	for i := range exts {
		exts[i].SIPSecret, exts[i].VoicemailPIN = "", ""
	}
	writeJSON(w, http.StatusOK, exts)
}

func (s *Server) nextExtension(w http.ResponseWriter, r *http.Request) {
	exts, _ := s.PBX.Extensions(r.Context())
	used := make([]string, len(exts))
	for i, e := range exts {
		used[i] = e.Number
	}
	writeJSON(w, http.StatusOK, map[string]string{"number": s.plan(r.Context()).NextFree(used)})
}

func (s *Server) getExtension(w http.ResponseWriter, r *http.Request) {
	e, err := s.PBX.Extension(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) saveExtension(w http.ResponseWriter, r *http.Request) {
	var e pbx.Extension
	if err := decode(r, &e); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	e.ID = pathID(r)
	created := e.ID == 0
	if e.ID != 0 {
		old, err := s.PBX.Extension(r.Context(), e.ID)
		if err != nil {
			s.saveErr(w, err)
			return
		}
		if e.SIPSecret == "" {
			e.SIPSecret = old.SIPSecret
		}
		if e.VoicemailPIN == "" {
			e.VoicemailPIN = old.VoicemailPIN
		}
	}
	if err := s.PBX.SaveExtension(r.Context(), s.plan(r.Context()), &e); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "extension.saved", e.Number)
	s.Ast.Changed()
	// A new extension with an e-mail address gets a portal account and a welcome e-mail when the
	// administrator asks for it (the form does by default).
	if created && e.Email != "" && r.URL.Query().Get("welcome") == "1" {
		res := s.welcomeForExtension(r, &e, r.URL.Query().Get("lang"))
		writeJSON(w, http.StatusOK, struct {
			pbx.Extension
			Welcome welcomeResult `json:"welcome"`
		}{e, res})
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deleteExtension(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteExtension(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "extension.deleted", r.PathValue("id"))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) resetExtensionSecret(w http.ResponseWriter, r *http.Request) {
	e, err := s.PBX.Extension(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	e.SIPSecret = pbx.GenerateSecret(24)
	if err := s.PBX.SaveExtension(r.Context(), s.plan(r.Context()), e); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "extension.secret_reset", e.Number)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, e)
}

// ---- trunks -----------------------------------------------------------------

func (s *Server) listTrunks(w http.ResponseWriter, r *http.Request) {
	ts, err := s.PBX.Trunks(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	for i := range ts {
		ts[i].Secret = ""
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) saveTrunk(w http.ResponseWriter, r *http.Request) {
	var t pbx.Trunk
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t.ID = pathID(r)
	if err := s.PBX.SaveTrunk(r.Context(), &t); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "trunk.saved", t.Name)
	s.Ast.Changed()
	s.firewallChanged(r.Context())
	t.Secret = ""
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTrunk(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteTrunk(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "trunk.deleted", r.PathValue("id"))
	s.Ast.Changed()
	s.firewallChanged(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// reregisterTrunk sends a fresh REGISTER for a trunk and waits a few seconds for the provider's
// answer, so the button can show the outcome right away.
func (s *Server) reregisterTrunk(w http.ResponseWriter, r *http.Request) {
	t, err := s.PBX.Trunk(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if t.AuthMode != "register" {
		writeErr(w, http.StatusBadRequest, "trunk_not_register")
		return
	}
	if !t.Enabled {
		writeErr(w, http.StatusConflict, "trunk_disabled")
		return
	}
	if !s.AMI.Connected() {
		writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		return
	}
	reg := t.Endpoint()
	before, _ := s.AMI.RegistrationStatus(r.Context(), reg)
	if err := s.AMI.Reregister(r.Context(), reg); err != nil {
		var ae *asterisk.ActionError
		if errors.As(err, &ae) {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "reregister_failed", "detail": ae.Msg})
			return
		}
		writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		return
	}
	s.audit(r, "trunk.reregistered", t.Name)
	status := waitRegistration(r.Context(), func(ctx context.Context) (string, error) {
		return s.AMI.RegistrationStatus(ctx, reg)
	}, reregisterWait, 400*time.Millisecond)
	s.Live.Invalidate()
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.ToLower(status), "before": strings.ToLower(before)})
}

// reregisterWait is how long the refresh waits for the provider before it answers "pending".
var reregisterWait = 6 * time.Second

// waitRegistration polls the registration status until the provider answered (Registered or
// Rejected) or the time is up; it then returns the last status seen, or "pending".
func waitRegistration(ctx context.Context, get func(context.Context) (string, error), wait, every time.Duration) string {
	deadline := time.Now().Add(wait)
	last := ""
	for {
		st, err := get(ctx)
		if err == nil && st != "" {
			last = st
			if strings.EqualFold(st, "Registered") || strings.EqualFold(st, "Rejected") {
				return st
			}
		}
		if time.Now().Add(every).After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return "pending"
		case <-time.After(every):
		}
	}
	if last == "" || strings.EqualFold(last, "Unregistered") {
		return "pending"
	}
	return last
}

func (s *Server) listNumbers(w http.ResponseWriter, r *http.Request) {
	ns, err := s.PBX.Numbers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, ns)
}

// ---- routes -----------------------------------------------------------------

func (s *Server) listInbound(w http.ResponseWriter, r *http.Request) {
	rs, err := s.PBX.InboundRoutes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) saveInbound(w http.ResponseWriter, r *http.Request) {
	var x pbx.InboundRoute
	if err := decode(r, &x); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	x.ID = pathID(r)
	if err := s.PBX.SaveInboundRoute(r.Context(), &x); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "route.inbound.saved", x.Name)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) deleteInbound(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteInboundRoute(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "route.inbound.deleted", r.PathValue("id"))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listOutbound(w http.ResponseWriter, r *http.Request) {
	rs, err := s.PBX.OutboundRoutes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) saveOutbound(w http.ResponseWriter, r *http.Request) {
	var x pbx.OutboundRoute
	if err := decode(r, &x); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	x.ID = pathID(r)
	if err := s.PBX.SaveOutboundRoute(r.Context(), &x); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "route.outbound.saved", x.Name)
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) reorderOutbound(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.PBX.ReorderOutboundRoutes(r.Context(), req.IDs); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "route.outbound.reordered", "")
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteOutbound(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteOutboundRoute(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "route.outbound.deleted", r.PathValue("id"))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- live and history -------------------------------------------------------

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	l := s.Live.Get(r.Context())
	exts, _ := s.PBX.Extensions(r.Context())
	trunks, _ := s.PBX.Trunks(r.Context())
	type trunkState struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Mode   string `json:"mode"`
		Status string `json:"status"`
	}
	ts := []trunkState{}
	for _, t := range trunks {
		if !t.Enabled {
			continue
		}
		st := trunkState{ID: t.ID, Name: t.Name, Mode: t.AuthMode, Status: "unknown"}
		for _, reg := range l.Registrations {
			if reg.Endpoint == t.Endpoint() {
				st.Status = strings.ToLower(reg.Status)
			}
		}
		if t.AuthMode == "ip" {
			for _, c := range l.Contacts {
				if c.Endpoint == t.Endpoint() {
					st.Status = strings.ToLower(c.Status)
				}
			}
		}
		ts = append(ts, st)
	}
	registered := 0
	online := map[string]bool{}
	for _, c := range l.Contacts {
		if !strings.HasPrefix(c.Endpoint, "trunk-") && (c.Status == "Reachable" || c.Status == "NonQualified" || c.Status == "Unknown") {
			online[c.Endpoint] = true
		}
	}
	registered = len(online)
	writeJSON(w, http.StatusOK, map[string]any{
		"connected": l.Connected, "calls": l.Calls, "contacts": l.Contacts, "trunks": ts,
		"extensions": len(exts), "registered": registered, "stats": s.CDR.TodayStats(r.Context()), "at": l.At,
		"quality": s.qualityLiveSummary(r.Context(), l),
	})
}

func cdrFilter(r *http.Request) cdr.Filter {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return cdr.Filter{From: q.Get("from"), To: q.Get("to"), Query: q.Get("q"), Direction: q.Get("direction"),
		Disposition: q.Get("disposition"), Limit: limit, Offset: offset}
}

func (s *Server) cdrSearch(w http.ResponseWriter, r *http.Request) {
	recs, total, err := s.CDR.Search(r.Context(), cdrFilter(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	ids := make([]string, 0, len(recs))
	for _, rec := range recs {
		ids = append(ids, rec.LinkedID)
	}
	mos, _ := (&quality.Store{DB: s.DB}).CallMOS(r.Context(), ids)
	writeJSON(w, http.StatusOK, map[string]any{"records": recs, "total": total, "quality": mos,
		"qualityLimits": qualityLimits(quality.Load(r.Context(), s.DB))})
}

func (s *Server) cdrExport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="kiestoon-calls-%s.csv"`, time.Now().Format("20060102")))
	s.audit(r, "cdr.export", "")
	if err := s.CDR.ExportCSV(r.Context(), cdrFilter(r), w); err != nil {
		s.Log.Warn("cdr export", "err", err)
	}
}
