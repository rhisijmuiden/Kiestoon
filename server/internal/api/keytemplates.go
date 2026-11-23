package api

import (
	"fmt"
	"net/http"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

// Key templates: a key layout (with expansion modules) for one phone model, applied to many phones.

func (s *Server) keyTemplateRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/key-templates", admin(s.keyTemplates))
	mux.Handle("POST /api/key-templates", admin(s.saveKeyTemplate))
	mux.Handle("PUT /api/key-templates/{id}", admin(s.saveKeyTemplate))
	mux.Handle("DELETE /api/key-templates/{id}", admin(s.deleteKeyTemplate))
	mux.Handle("POST /api/key-templates/{id}/apply", admin(s.applyKeyTemplate))
}

func (s *Server) keyTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.KeyTemplates(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveKeyTemplate(w http.ResponseWriter, r *http.Request) {
	var t pbx.KeyTemplate
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t.ID = pathID(r)
	if t.ID != 0 {
		// The model of a template is fixed: its keys only make sense on that model.
		old, err := s.PBX.KeyTemplate(r.Context(), t.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		t.Vendor, t.Model = old.Vendor, old.Model
	}
	lim, ok := provision.Limits(t.Vendor, t.Model)
	if !ok {
		fieldErr(w, "model")
		return
	}
	if err := s.PBX.SaveKeyTemplate(r.Context(), &t, lim); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "key_template.saved", fmt.Sprintf("%s (%s %s)", t.Name, t.Vendor, t.Model))
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteKeyTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteKeyTemplate(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "key_template.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// applyKeyTemplate copies the layout to phones of the same model and asks them to fetch it.
func (s *Server) applyKeyTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PhoneIDs []int64 `json:"phoneIds"`
		Resync   *bool   `json:"resync"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t, err := s.PBX.KeyTemplate(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	lim, _ := provision.Limits(t.Vendor, t.Model)
	applied, sent := 0, 0
	for _, id := range req.PhoneIDs {
		p, err := s.PBX.Phone(r.Context(), id)
		if err != nil || p.Vendor != t.Vendor || p.Model != t.Model {
			fieldErr(w, "phoneIds")
			return
		}
		copyTemplate(t, p)
		if err := s.PBX.SavePhone(r.Context(), p, lim); err != nil {
			s.saveErr(w, err)
			return
		}
		applied++
		if req.Resync == nil || *req.Resync {
			if st, _ := s.notifyPhone(r.Context(), p, s.registeredAs(r.Context(), p)); st == resyncSent {
				sent++
			}
		}
	}
	s.audit(r, "key_template.applied", fmt.Sprintf("%s to %d phones", t.Name, applied))
	writeJSON(w, http.StatusOK, map[string]int{"applied": applied, "notified": sent})
}
