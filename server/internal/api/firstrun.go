package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/auth"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// InitialAdminPath holds the generated first administrator until the first-run wizard in the portal
// has replaced its password. The installer, the console and `kiestoon initial-admin` show it, so only
// someone with access to the server can sign in for the first time.
func InitialAdminPath(dataDir string) string { return filepath.Join(dataDir, "initial-admin") }

// InitialAdmin is the generated first administrator.
type InitialAdmin struct {
	ID       int64
	Username string
	Password string
}

// LoadInitialAdmin reads the generated first administrator, if the first-run wizard is still pending.
func LoadInitialAdmin(dataDir string) (*InitialAdmin, bool) {
	b, err := os.ReadFile(InitialAdminPath(dataDir))
	if err != nil {
		return nil, false
	}
	a := &InitialAdmin{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch k {
		case "id":
			a.ID, _ = strconv.ParseInt(v, 10, 64)
		case "username":
			a.Username = v
		case "password":
			a.Password = v
		}
	}
	return a, a.Username != "" && a.Password != ""
}

func writeInitialAdmin(dataDir string, a *InitialAdmin) error {
	body := fmt.Sprintf("id=%d\nusername=%s\npassword=%s\n", a.ID, a.Username, a.Password)
	tmp := InitialAdminPath(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, InitialAdminPath(dataDir))
}

// newInitialPassword makes a readable random password such as "k7Rm-Xq4T-9fwP-Lh2c" (95 bits).
// It leaves out characters that look alike (0/O, 1/l/I).
func newInitialPassword() string {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var b strings.Builder
	for i := 0; i < 16; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic(err)
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String()
}

// EnsureInitialAdmin creates the first administrator ("admin" with a random password) on a fresh
// installation, so the installer can show a user name and password instead of a setup wizard.
func (s *Server) EnsureInitialAdmin(ctx context.Context) error {
	n, err := s.Auth.UserCount(ctx)
	if err != nil || n > 0 {
		s.updateConsoleIssue(ctx)
		return err
	}
	// The password is saved before the account exists, so a restart in between reuses it.
	a, ok := LoadInitialAdmin(s.Cfg.DataDir)
	if !ok {
		a = &InitialAdmin{Username: "admin", Password: newInitialPassword()}
		if err := writeInitialAdmin(s.Cfg.DataDir, a); err != nil {
			return fmt.Errorf("save initial administrator: %w", err)
		}
	}
	u, err := s.Auth.CreateUser(ctx, a.Username, "Administrator", a.Password, "superadmin", "en")
	if err != nil {
		return fmt.Errorf("create initial administrator: %w", err)
	}
	a.ID = u.ID
	if err := writeInitialAdmin(s.Cfg.DataDir, a); err != nil {
		return err
	}
	_ = os.Remove(SetupTokenPath(s.Cfg.DataDir))
	store.Audit(ctx, s.DB, u.ID, "setup.initial_admin", u.Username, "")
	s.Log.Info("first administrator created; sign in with the user name and password from `kiestoon initial-admin`", "username", u.Username)
	s.updateConsoleIssue(ctx)
	return nil
}

// firstRunPending tells whether u is the generated administrator that still has to finish the
// first-run wizard (new password, organization). Until then nothing else can be configured.
func (s *Server) firstRunPending(u *auth.User) bool {
	if u == nil {
		return false
	}
	a, ok := LoadInitialAdmin(s.Cfg.DataDir)
	return ok && a.ID == u.ID
}

// updateConsoleIssue lets the root helper show the portal address (and, while the first run is
// pending, the user name and password) on the console login screen.
func (s *Server) updateConsoleIssue(ctx context.Context) {
	if s.Helper == nil {
		return
	}
	go func() { _ = s.Helper.Call(context.WithoutCancel(ctx), "console.issue", nil, nil) }()
}

type firstRunReq struct {
	Organization string `json:"organization"`
	DisplayName  string `json:"displayName"`
	Password     string `json:"password"`
	Language     string `json:"language"`
}

// firstRun finishes the first-run wizard: the generated password is replaced and the organization set.
func (s *Server) firstRun(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	a, ok := LoadInitialAdmin(s.Cfg.DataDir)
	if !ok || a.ID != sess.User.ID {
		writeErr(w, http.StatusConflict, "setup_done")
		return
	}
	var req firstRunReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	org := strings.TrimSpace(req.Organization)
	if org == "" {
		fieldErr(w, "organization")
		return
	}
	if req.Password == a.Password {
		fieldErr(w, "password")
		return
	}
	ctx := r.Context()
	if err := s.Auth.ChangePassword(ctx, sess.User, a.Password, req.Password); err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			writeErr(w, http.StatusBadRequest, "weak_password")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if name := strings.TrimSpace(req.DisplayName); name != "" {
		_ = s.Auth.SetDisplayName(ctx, sess.User.ID, name)
	}
	_ = s.Auth.SetLanguage(ctx, sess.User.ID, req.Language)
	// The language chosen at setup is also the first caller language (prompts, phones, e-mails).
	if req.Language == "nl" || req.Language == "en" {
		if st, err := pbx.LoadSettings(ctx, s.DB); err == nil && st.Language != req.Language {
			st.Language = req.Language
			if store.SetJSON(ctx, s.DB, pbx.SettingsKey, st) == nil && s.Ast != nil {
				s.Ast.Changed()
			}
		}
	}
	_ = store.SetSetting(ctx, s.DB, "organization", org)
	_ = os.Remove(InitialAdminPath(s.Cfg.DataDir))
	s.updateConsoleIssue(ctx)
	store.Audit(ctx, s.DB, sess.User.ID, "setup.completed", sess.User.Username, clientIP(r))
	u, err := s.Auth.UserByID(ctx, sess.User.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, s.meBody(r, u, sess.MFAPassed))
}

// firstRunClusterJoin makes this fresh installation the standby of an existing Kiestoon. Signing in
// with the generated password proves access to the console, like the setup token did.
func (s *Server) firstRunClusterJoin(w http.ResponseWriter, r *http.Request) {
	if !s.firstRunPending(sessionFrom(r).User) {
		writeErr(w, http.StatusConflict, "setup_done")
		return
	}
	var req struct {
		Code    string `json:"code"`
		Address string `json:"address"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if s.joinAsStandby(w, r, req.Code, req.Address) {
		_ = os.Remove(InitialAdminPath(s.Cfg.DataDir))
		s.updateConsoleIssue(r.Context())
	}
}
