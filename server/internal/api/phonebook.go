package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/certs"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/phonebook"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) phonebookRoutes(mux *http.ServeMux, admin, recep, full func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/phonebook", full(s.phonebookList))
	mux.Handle("POST /api/phonebook", recep(s.saveContact))
	mux.Handle("PUT /api/phonebook/{id}", recep(s.saveContact))
	mux.Handle("DELETE /api/phonebook/{id}", recep(s.deleter("contact", s.PBX.DeleteContact)))
	mux.Handle("POST /api/phonebook/import", admin(s.phonebookImport))
	mux.Handle("GET /api/phonebook/export", admin(s.phonebookExport))
	mux.Handle("GET /api/phonebook/settings", admin(s.phonebookSettingsGet))
	mux.Handle("PUT /api/phonebook/settings", admin(s.phonebookSettingsSave))
}

func (s *Server) ldapSettings(ctx context.Context) pbx.PhonebookSettings {
	st, _ := pbx.LoadPhonebookSettings(ctx, s.DB)
	return st
}

func (s *Server) phonebookList(w http.ResponseWriter, r *http.Request) {
	cs, err := s.PBX.Contacts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) saveContact(w http.ResponseWriter, r *http.Request) {
	var c pbx.Contact
	if err := decode(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	c.ID = pathID(r)
	if err := s.PBX.SaveContact(r.Context(), &c); err != nil {
		s.saveErr(w, err)
		return
	}
	s.saved(w, r, "contact", c.Name, c)
}

func (s *Server) phonebookImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CSV     string `json:"csv"`
		Replace bool   `json:"replace"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	cs, err := phonebook.ReadCSV(req.CSV)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "csv", "message": err.Error()})
		return
	}
	added, skipped, err := s.PBX.ImportContacts(r.Context(), cs, req.Replace)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "phonebook.imported", strconv.Itoa(added))
	s.Ast.Changed()
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "skipped": skipped})
}

func (s *Server) phonebookExport(w http.ResponseWriter, r *http.Request) {
	cs, err := s.PBX.Contacts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	var b bytes.Buffer
	b.WriteString("\ufeff") // spreadsheet programs then read UTF-8
	_ = phonebook.WriteCSV(&b, cs)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="kiestoon-phonebook.csv"`)
	_, _ = w.Write(b.Bytes())
}

func (s *Server) phonebookSettingsGet(w http.ResponseWriter, r *http.Request) {
	st := s.ldapSettings(r.Context())
	prov, _ := provision.Load(r.Context(), s.DB)
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": st, "baseDn": phonebook.BaseDN, "bindDn": "cn=" + prov.Username + "," + phonebook.BaseDN,
		"server": host, "ldapRunning": s.ldapRunning(), "xmlBase": s.publicBase(r) + "/prov/phonebook/"})
}

func (s *Server) phonebookSettingsSave(w http.ResponseWriter, r *http.Request) {
	var st pbx.PhonebookSettings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := st.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	old := s.ldapSettings(r.Context())
	if err := store.SetJSON(r.Context(), s.DB, pbx.PhonebookKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "phonebook.settings", "")
	s.Ast.Changed()
	if old.LDAPEnabled != st.LDAPEnabled || old.LDAPPort != st.LDAPPort || old.LDAPSPort != st.LDAPSPort {
		s.firewallChanged(r.Context())
		s.restartLDAP(r.Context())
	}
	s.phonebookSettingsGet(w, r)
}

// ---- LDAP directory ---------------------------------------------------------

type ldapSource struct{ s *Server }

func (l ldapSource) Entries(ctx context.Context) ([]pbx.Entry, error) {
	st := l.s.ldapSettings(ctx)
	return l.s.PBX.PhonebookEntries(ctx, st.IncludeExtensions)
}

func (l ldapSource) Credentials(ctx context.Context) (string, string, bool) {
	st := l.s.ldapSettings(ctx)
	prov, _ := provision.Load(ctx, l.s.DB)
	return prov.Username, prov.Password, st.LDAPAuth
}

func (s *Server) ldapRunning() bool {
	s.ldapMu.Lock()
	defer s.ldapMu.Unlock()
	return s.ldapOn
}

// restartLDAP starts, restarts or stops the directory after a settings change.
func (s *Server) restartLDAP(ctx context.Context) {
	s.ldapMu.Lock()
	defer s.ldapMu.Unlock()
	if s.ldap == nil {
		s.ldap = &phonebook.Directory{Source: ldapSource{s}, Log: s.Log}
	}
	s.ldap.Stop()
	s.ldapOn = false
	st := s.ldapSettings(ctx)
	if !st.LDAPEnabled {
		return
	}
	tlsAddr := ""
	var cfg *tls.Config
	if st.LDAPSPort > 0 {
		loader := &certs.Loader{Dir: s.Cfg.PBXDir()}
		tlsAddr = ":" + strconv.Itoa(st.LDAPSPort)
		cfg = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: loader.GetCertificate}
	}
	if err := s.ldap.Start(":"+strconv.Itoa(st.LDAPPort), tlsAddr, cfg); err != nil {
		s.Log.Warn("ldap directory not started", "port", st.LDAPPort, "err", err)
		return
	}
	s.ldapOn = true
	s.Log.Info("ldap directory listening", "port", st.LDAPPort)
}

// RunLDAP starts the directory when it is switched on and stops it at shutdown.
func (s *Server) RunLDAP(ctx context.Context) {
	s.restartLDAP(ctx)
	<-ctx.Done()
	s.ldapMu.Lock()
	if s.ldap != nil {
		s.ldap.Stop()
	}
	s.ldapMu.Unlock()
}

// phonebookXML serves the XML phone book to phones (behind the provisioning password).
func (s *Server) phonebookXML(w http.ResponseWriter, r *http.Request, vendor string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	st := s.ldapSettings(ctx)
	if !st.XMLEnabled {
		http.NotFound(w, r)
		return
	}
	es, err := s.PBX.PhonebookEntries(ctx, st.IncludeExtensions)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body, ok := phonebook.XML(vendor, es)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(body))
}
