package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

// Linking phones to extensions: phones that fetched their configuration but are not known yet
// (discoveries), assigning a phone to an extension, and asking a phone to fetch its configuration
// again (SIP NOTIFY check-sync).

func (s *Server) phoneAssignRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/phones/discovered", admin(s.phoneDiscoveries))
	mux.Handle("POST /api/phones/discovered/{mac}/adopt", admin(s.phoneAdopt))
	mux.Handle("DELETE /api/phones/discovered/{mac}", admin(s.phoneDismiss))
	mux.Handle("PUT /api/phones/{id}/assign", admin(s.phoneAssign))
	mux.Handle("POST /api/phones/{id}/resync", admin(s.phoneResync))
}

// recordDiscovery is called by the provisioning handler for a MAC address it does not know.
func (s *Server) recordDiscovery(ctx context.Context, req provision.Request, file, ip, ua string) {
	vendor, model := provision.GuessModel(req, req.MAC, ua)
	s.PBX.RecordDiscovery(ctx, pbx.Discovery{MAC: req.MAC, Vendor: vendor, Model: model, IP: ip, UserAgent: ua, File: file})
}

func (s *Server) phoneDiscoveries(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.Discoveries(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// phoneAdopt turns a discovered phone into a phone, optionally assigned to an extension. The phone
// gets its configuration the next time it asks for it (it is not registered yet, so it cannot be
// told to): on its own provisioning schedule or after a reboot.
func (s *Server) phoneAdopt(w http.ResponseWriter, r *http.Request) {
	d, err := s.PBX.Discovery(r.Context(), r.PathValue("mac"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var req struct {
		Vendor      string `json:"vendor"`
		Model       string `json:"model"`
		ExtensionID *int64 `json:"extensionId"`
		Label       string `json:"label"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	m := provision.FindModel(req.Vendor, req.Model)
	if m == nil {
		fieldErr(w, "model")
		return
	}
	if req.ExtensionID != nil {
		if _, err := s.PBX.Extension(r.Context(), *req.ExtensionID); err != nil {
			fieldErr(w, "extensionId")
			return
		}
	}
	p := pbx.Phone{MAC: d.MAC, Vendor: req.Vendor, Model: req.Model, ExtensionID: req.ExtensionID, Label: req.Label}
	if m.Keys > 0 {
		p.Keys = []pbx.Key{{Type: "line"}}
	}
	if err := s.PBX.SavePhone(r.Context(), &p, m.Keys, m.MaxLines()); err != nil {
		s.saveErr(w, err)
		return
	}
	_ = s.PBX.ForgetDiscovery(r.Context(), d.MAC)
	s.audit(r, "phone.adopted", fmt.Sprintf("%s %s/%s ext=%s", p.MAC, p.Vendor, p.Model, s.extNumber(r.Context(), p.ExtensionID)))
	s.ztpAfterSave(r, &p, nil)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) phoneDismiss(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.ForgetDiscovery(r.Context(), r.PathValue("mac")); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "phone.dismissed", pbx.NormalizeMAC(r.PathValue("mac")))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) extNumber(ctx context.Context, id *int64) string {
	if id == nil {
		return "-"
	}
	if e, err := s.PBX.Extension(ctx, *id); err == nil {
		return e.Number
	}
	return "?"
}

// registeredAs is the extension number a phone is registered with now: its main extension, or
// the first extra line when it has none.
func (s *Server) registeredAs(ctx context.Context, p *pbx.Phone) string {
	ids := p.Lines
	if p.ExtensionID != nil {
		ids = append([]int64{*p.ExtensionID}, ids...)
	}
	for _, id := range ids {
		if e, err := s.PBX.Extension(ctx, id); err == nil && e.Enabled {
			return e.Number
		}
	}
	return ""
}

// Resync outcomes, translated by the UI.
const (
	resyncSent          = "sent"           // the phone was told to fetch its configuration
	resyncNotRegistered = "not_registered" // no account on the phone yet, or it is offline
	resyncFailed        = "failed"         // Asterisk did not accept the NOTIFY
	resyncSkipped       = "skipped"        // the admin chose not to notify
)

// notifyPhone sends the vendor's check-sync NOTIFY to the account the phone is registered with.
func (s *Server) notifyPhone(ctx context.Context, p *pbx.Phone, endpoint string) (string, string) {
	if endpoint == "" {
		return resyncNotRegistered, ""
	}
	if s.AMI == nil || !s.AMI.Connected() {
		return resyncFailed, "asterisk_offline"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Asterisk accepts a NOTIFY for an endpoint without contacts and sends nothing: check first.
	online := false
	if s.Live != nil {
		if l := s.Live.Get(ctx); l != nil {
			for _, c := range l.Contacts {
				if c.Endpoint == endpoint && c.Status != "Unavailable" {
					online = true
				}
			}
		}
	}
	if !online {
		return resyncNotRegistered, ""
	}
	resp, err := s.AMI.Action(ctx, asterisk.Message{
		{Key: "Action", Value: "PJSIPNotify"}, {Key: "Endpoint", Value: endpoint},
		{Key: "Variable", Value: "Event=" + provision.ResyncEvent(p.Vendor)},
	})
	if err != nil {
		return resyncFailed, err.Error()
	}
	if strings.EqualFold(resp.Get("Response"), "Error") {
		msg := resp.Get("Message")
		// Asterisk answers with an error when the endpoint has no registered contact.
		if l := strings.ToLower(msg); strings.Contains(l, "contact") || strings.Contains(l, "unable to") {
			return resyncNotRegistered, msg
		}
		return resyncFailed, msg
	}
	return resyncSent, ""
}

func (s *Server) phoneAssign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExtensionID *int64 `json:"extensionId"`
		Resync      *bool  `json:"resync"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	old, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if req.ExtensionID != nil {
		if _, err := s.PBX.Extension(r.Context(), *req.ExtensionID); err != nil {
			fieldErr(w, "extensionId")
			return
		}
	}
	// The phone still registers with its old account: that is where the NOTIFY has to go.
	before := s.registeredAs(r.Context(), old)
	p, err := s.PBX.AssignPhone(r.Context(), old.ID, req.ExtensionID)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "phone.assigned", fmt.Sprintf("%s ext %s -> %s", p.MAC, s.extNumber(r.Context(), old.ExtensionID), s.extNumber(r.Context(), p.ExtensionID)))
	state, msg := resyncSkipped, ""
	if req.Resync == nil || *req.Resync {
		state, msg = s.notifyPhone(r.Context(), p, before)
	}
	if state == resyncFailed {
		s.Log.Warn("phone resync", "mac", p.MAC, "err", msg)
	}
	writeJSON(w, http.StatusOK, map[string]any{"phone": p, "resync": state, "message": msg})
}

func (s *Server) phoneResync(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	state, msg := s.notifyPhone(r.Context(), p, s.registeredAs(r.Context(), p))
	if state == resyncSent {
		s.audit(r, "phone.resync", p.MAC)
	}
	writeJSON(w, http.StatusOK, map[string]any{"resync": state, "message": msg})
}
