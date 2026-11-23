package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

// Extension groups: a caller ID, enforced settings and key templates for a set of extensions,
// and outbound routes that only some groups may use.

func (s *Server) extGroupRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/pbx/extension-groups", admin(s.listExtGroups))
	mux.Handle("POST /api/pbx/extension-groups", admin(s.saveExtGroup))
	mux.Handle("PUT /api/pbx/extension-groups/{id}", admin(s.saveExtGroup))
	mux.Handle("DELETE /api/pbx/extension-groups/{id}", admin(s.deleteExtGroup))
	mux.Handle("POST /api/pbx/extension-groups/{id}/sync-phones", admin(s.syncExtGroupPhones))
}

func (s *Server) listExtGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.ExtensionGroups(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveExtGroup(w http.ResponseWriter, r *http.Request) {
	var g pbx.ExtensionGroup
	if err := decode(r, &g); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	g.ID = pathID(r)
	if err := s.PBX.SaveExtensionGroup(r.Context(), &g); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "extension_group.saved", g.Name)
	s.Ast.Changed()
	applied, notified := s.applyGroupTemplates(r.Context(), &g)
	writeJSON(w, http.StatusOK, struct {
		pbx.ExtensionGroup
		PhonesUpdated  int `json:"phonesUpdated"`
		PhonesNotified int `json:"phonesNotified"`
	}{g, applied, notified})
}

func (s *Server) deleteExtGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteExtensionGroup(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "extension_group.deleted", r.PathValue("id"))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// syncExtGroupPhones copies the group's key templates to its members' phones again.
func (s *Server) syncExtGroupPhones(w http.ResponseWriter, r *http.Request) {
	g, err := s.PBX.ExtensionGroup(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	applied, notified := s.applyGroupTemplates(r.Context(), g)
	s.audit(r, "extension_group.phones_synced", fmt.Sprintf("%s: %d phones", g.Name, applied))
	writeJSON(w, http.StatusOK, map[string]int{"phonesUpdated": applied, "phonesNotified": notified})
}

// applyGroupTemplates gives every phone of a member the group's template for that phone's model
// and asks the phone to fetch it. It returns how many phones changed and how many were told.
func (s *Server) applyGroupTemplates(ctx context.Context, g *pbx.ExtensionGroup) (int, int) {
	if len(g.KeyTemplateIDs) == 0 || len(g.Members) == 0 {
		return 0, 0
	}
	byModel := map[string]*pbx.KeyTemplate{}
	for _, id := range g.KeyTemplateIDs {
		if t, err := s.PBX.KeyTemplate(ctx, id); err == nil {
			byModel[t.Vendor+"/"+t.Model] = t
		}
	}
	members := map[int64]bool{}
	for _, id := range g.Members {
		members[id] = true
	}
	phones, err := s.PBX.Phones(ctx)
	if err != nil {
		return 0, 0
	}
	applied, sent := 0, 0
	for i := range phones {
		p := &phones[i]
		if p.ExtensionID == nil || !members[*p.ExtensionID] {
			continue
		}
		t := byModel[p.Vendor+"/"+p.Model]
		if t == nil {
			continue
		}
		lim, _ := provision.Limits(t.Vendor, t.Model)
		copyTemplate(t, p)
		if err := s.PBX.SavePhone(ctx, p, lim); err != nil {
			s.Log.Warn("group template not applied", "phone", p.MAC, "err", err)
			continue
		}
		applied++
		if st, _ := s.notifyPhone(ctx, p, s.registeredAs(ctx, p)); st == resyncSent {
			sent++
		}
	}
	return applied, sent
}

// groupTemplateFor returns the key template of the extension's group for a phone model, or nil.
func (s *Server) groupTemplateFor(ctx context.Context, extID int64, vendor, model string) *pbx.KeyTemplate {
	e, err := s.PBX.Extension(ctx, extID)
	if err != nil || e.GroupID == nil || *e.GroupID == 0 {
		return nil
	}
	g, err := s.PBX.ExtensionGroup(ctx, *e.GroupID)
	if err != nil {
		return nil
	}
	for _, id := range g.KeyTemplateIDs {
		if t, err := s.PBX.KeyTemplate(ctx, id); err == nil && t.Vendor == vendor && t.Model == model {
			return t
		}
	}
	return nil
}

func copyTemplate(t *pbx.KeyTemplate, p *pbx.Phone) {
	p.Keys = append([]pbx.Key{}, t.Keys...)
	p.Modules = []pbx.Module{}
	for _, m := range t.Modules {
		p.Modules = append(p.Modules, pbx.Module{Type: m.Type, Keys: append([]pbx.Key{}, m.Keys...)})
	}
}
