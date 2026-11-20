package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
)

// Phone admin passwords: provisioning replaces the factory password of a phone's own web interface
// (admin / admin). By default every phone gets its own generated password; the provisioning
// settings can switch to one shared password for all phones. Admins look a password up in the
// phone list; every look is written to the audit log.

const adminPasswordLen = 14

func (s *Server) adminPasswordRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/phones/{id}/admin-password", admin(s.phoneAdminPasswordGet))
	mux.Handle("POST /api/phones/{id}/admin-password", admin(s.phoneAdminPasswordNew))
}

// phoneAdminPassword returns the phone's own password, making one the first time.
func (s *Server) phoneAdminPassword(ctx context.Context, id int64) (string, string, error) {
	pw, at, err := s.PBX.AdminPassword(ctx, id)
	if err != nil || pw != "" {
		return pw, at, err
	}
	pw = pbx.GenerateSecret(adminPasswordLen)
	at, err = s.PBX.SetAdminPassword(ctx, id, pw)
	return pw, at, err
}

// adminPasswordFor is the password a phone gets in its configuration.
func (s *Server) adminPasswordFor(ctx context.Context, set provision.Settings, id int64) (string, error) {
	if set.AdminPasswordMode == provision.AdminShared {
		return set.AdminPassword, nil
	}
	pw, _, err := s.phoneAdminPassword(ctx, id)
	return pw, err
}

type adminPasswordView struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Mode     string `json:"mode"`
	// Applied: the configuration the phone fetched last held this password.
	Applied   bool   `json:"applied"`
	AppliedAt string `json:"appliedAt"`
	LastIP    string `json:"lastIp"`
}

func (s *Server) adminPasswordView(ctx context.Context, p *pbx.Phone) (adminPasswordView, error) {
	set, err := provision.Load(ctx, s.DB)
	if err != nil {
		return adminPasswordView{}, err
	}
	pw, err := s.adminPasswordFor(ctx, set, p.ID)
	if err != nil {
		return adminPasswordView{}, err
	}
	v := adminPasswordView{User: "admin", Password: pw, Mode: set.AdminPasswordMode, LastIP: p.LastIP}
	if s.PBX.AdminPasswordReceived(ctx, p.ID, pw) {
		v.Applied, v.AppliedAt = true, p.LastSeenAt
	}
	return v, nil
}

func (s *Server) phoneAdminPasswordGet(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	v, err := s.adminPasswordView(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "phone.admin_password_viewed", fmt.Sprintf("%s %s", p.MAC, p.Label))
	writeJSON(w, http.StatusOK, v)
}

// phoneAdminPasswordNew gives a phone a new password (only when every phone has its own).
func (s *Server) phoneAdminPasswordNew(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	set, err := provision.Load(r.Context(), s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if set.AdminPasswordMode == provision.AdminShared {
		writeErr(w, http.StatusBadRequest, "admin_password_shared")
		return
	}
	if _, err := s.PBX.SetAdminPassword(r.Context(), p.ID, pbx.GenerateSecret(adminPasswordLen)); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "phone.admin_password_changed", fmt.Sprintf("%s %s", p.MAC, p.Label))
	v, err := s.adminPasswordView(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, v)
}
