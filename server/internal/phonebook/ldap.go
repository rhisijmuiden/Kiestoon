package phonebook

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
	"github.com/hashicorp/go-hclog"
	"github.com/jimlambrt/gldap"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// BaseDN is the root of the directory. Phones use it as their search base.
const BaseDN = "ou=phonebook,dc=kiestoon"

// Source gives the directory its data and credentials.
type Source interface {
	Entries(ctx context.Context) ([]pbx.Entry, error)
	// Credentials returns the user name and password phones bind with; ok is false when anonymous
	// access is allowed.
	Credentials(ctx context.Context) (user, password string, required bool)
}

// Directory is a small read-only LDAP v3 server.
type Directory struct {
	Source Source
	Log    *slog.Logger

	mu      sync.Mutex
	servers []*gldap.Server
	authed  map[int]bool
}

// Start listens on addr (plain LDAP) and, when tlsAddr is set, on tlsAddr with LDAP over TLS.
func (d *Directory) Start(addr, tlsAddr string, cfg *tls.Config) error {
	d.Stop()
	d.mu.Lock()
	d.authed = map[int]bool{}
	d.mu.Unlock()
	start := func(a string, c *tls.Config) error {
		quiet := hclog.New(&hclog.LoggerOptions{Output: io.Discard, Level: hclog.Off})
		s, err := gldap.NewServer(gldap.WithLogger(quiet), gldap.WithReadTimeout(time.Minute), gldap.WithWriteTimeout(time.Minute),
			gldap.WithOnClose(func(id int) { d.mu.Lock(); delete(d.authed, id); d.mu.Unlock() }))
		if err != nil {
			return err
		}
		mux, err := gldap.NewMux()
		if err != nil {
			return err
		}
		_ = mux.Bind(d.bind)
		_ = mux.Search(d.search)
		_ = mux.DefaultRoute(func(w *gldap.ResponseWriter, r *gldap.Request) {
			_ = w.Write(r.NewResponse(gldap.WithResponseCode(gldap.ResultUnwillingToPerform)))
		})
		if err := s.Router(mux); err != nil {
			return err
		}
		opts := []gldap.Option{}
		if c != nil {
			opts = append(opts, gldap.WithTLSConfig(c))
		}
		errc := make(chan error, 1)
		go func() { errc <- s.Run(a, opts...) }()
		// Run returns at once on a listen error (port in use, no permission).
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-waitReady(s):
		}
		d.mu.Lock()
		d.servers = append(d.servers, s)
		d.mu.Unlock()
		return nil
	}
	if err := start(addr, nil); err != nil {
		return err
	}
	if tlsAddr != "" && cfg != nil {
		if err := start(tlsAddr, cfg); err != nil {
			d.Log.Warn("ldaps", "err", err)
		}
	}
	return nil
}

func waitReady(s *gldap.Server) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for i := 0; i < 50 && !s.Ready(); i++ {
			time.Sleep(20 * time.Millisecond)
		}
		close(ch)
	}()
	return ch
}

// Stop closes all listeners.
func (d *Directory) Stop() {
	d.mu.Lock()
	ss := d.servers
	d.servers = nil
	d.mu.Unlock()
	for _, s := range ss {
		_ = s.Stop()
	}
}

func (d *Directory) bind(w *gldap.ResponseWriter, r *gldap.Request) {
	resp := r.NewBindResponse(gldap.WithResponseCode(gldap.ResultInvalidCredentials))
	defer func() { _ = w.Write(resp) }()
	m, err := r.GetSimpleBindMessage()
	if err != nil {
		resp.SetResultCode(gldap.ResultAuthMethodNotSupported)
		return
	}
	user, pass, required := d.Source.Credentials(context.Background())
	name := string(m.UserName)
	// Accept "phone", "cn=phone" and "cn=phone,ou=phonebook,dc=kiestoon".
	if i := strings.Index(name, ","); i > 0 {
		name = name[:i]
	}
	name = strings.TrimPrefix(strings.TrimPrefix(name, "cn="), "uid=")
	ok := subtle.ConstantTimeCompare([]byte(name), []byte(user)) == 1 && subtle.ConstantTimeCompare([]byte(m.Password), []byte(pass)) == 1
	if !ok && !required && m.UserName == "" && m.Password == "" {
		ok = true // anonymous bind
	}
	d.mu.Lock()
	d.authed[r.ConnectionID()] = ok
	d.mu.Unlock()
	if ok {
		resp.SetResultCode(gldap.ResultSuccess)
	} else if d.Log != nil {
		d.Log.Warn("ldap bind failed", "user", name)
	}
}

func (d *Directory) search(w *gldap.ResponseWriter, r *gldap.Request) {
	resp := r.NewSearchDoneResponse(gldap.WithResponseCode(gldap.ResultSuccess))
	defer func() { _ = w.Write(resp) }()
	ctx := context.Background()
	if _, _, required := d.Source.Credentials(ctx); required {
		d.mu.Lock()
		ok := d.authed[r.ConnectionID()]
		d.mu.Unlock()
		if !ok {
			resp.SetResultCode(gldap.ResultInsufficientAccessRights)
			return
		}
	}
	m, err := r.GetSearchMessage()
	if err != nil {
		resp.SetResultCode(gldap.ResultProtocolError)
		return
	}
	base := strings.ToLower(strings.ReplaceAll(m.BaseDN, " ", ""))
	if base != "" && base != "dc=kiestoon" && !strings.HasSuffix(base, BaseDN) {
		resp.SetResultCode(gldap.ResultNoSuchObject)
		return
	}
	match, err := compile(m.Filter)
	if err != nil {
		resp.SetResultCode(gldap.ResultProtocolError)
		return
	}
	entries, err := d.Source.Entries(ctx)
	if err != nil {
		resp.SetResultCode(gldap.ResultOperationsError)
		return
	}
	limit := int(m.SizeLimit)
	n := 0
	for _, e := range entries {
		attrs := Attributes(e)
		if !match(attrs) {
			continue
		}
		dn := "cn=" + escapeDN(e.Name+" "+e.ID) + "," + BaseDN
		_ = w.Write(r.NewSearchResponseEntry(dn, gldap.WithAttributes(attrs)))
		n++
		if limit > 0 && n >= limit {
			resp.SetResultCode(gldap.ResultSizeLimitExceeded)
			return
		}
	}
}

// Attributes are the LDAP attributes of an entry (inetOrgPerson).
func Attributes(e pbx.Entry) map[string][]string {
	a := map[string][]string{
		"objectClass": {"top", "person", "organizationalPerson", "inetOrgPerson"},
		"cn":          {e.Name},
		"displayName": {e.Name},
	}
	first, last := "", e.Name
	if i := strings.LastIndex(e.Name, " "); i > 0 {
		first, last = e.Name[:i], e.Name[i+1:]
	}
	a["sn"] = []string{last}
	if first != "" {
		a["givenName"] = []string{first}
	}
	add := func(k, v string) {
		if v != "" {
			a[k] = append(a[k], v)
		}
	}
	add("o", e.Company)
	add("telephoneNumber", e.Work)
	add("mobile", e.Mobile)
	add("homePhone", e.Other)
	add("otherTelephone", e.Other)
	add("mail", e.Email)
	return a
}

func escapeDN(s string) string {
	r := strings.NewReplacer(",", `\,`, "+", `\+`, `"`, `\"`, `\`, `\\`, "<", `\<`, ">", `\>`, ";", `\;`, "=", `\=`)
	return r.Replace(s)
}

// compile turns an LDAP filter string into a matcher on attributes (case-insensitive, like the
// caseIgnoreMatch rules of the inetOrgPerson attributes).
func compile(filter string) (func(map[string][]string) bool, error) {
	if strings.TrimSpace(filter) == "" {
		return func(map[string][]string) bool { return true }, nil
	}
	p, err := ldap.CompileFilter(filter)
	if err != nil {
		return nil, err
	}
	return matcher(p)
}

func get(attrs map[string][]string, name string) []string {
	for k, v := range attrs {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

// Phone numbers compare without spaces and dashes.
func norm(attr, v string) string {
	v = strings.ToLower(v)
	switch strings.ToLower(attr) {
	case "telephonenumber", "mobile", "homephone", "othertelephone":
		v = pbx.CleanNumber(v)
	}
	return v
}

func matcher(p *ber.Packet) (func(map[string][]string) bool, error) {
	switch p.Tag {
	case ldap.FilterAnd, ldap.FilterOr:
		subs := []func(map[string][]string) bool{}
		for _, c := range p.Children {
			m, err := matcher(c)
			if err != nil {
				return nil, err
			}
			subs = append(subs, m)
		}
		and := p.Tag == ldap.FilterAnd
		return func(a map[string][]string) bool {
			for _, m := range subs {
				if m(a) != and {
					return !and
				}
			}
			return and
		}, nil
	case ldap.FilterNot:
		if len(p.Children) != 1 {
			return nil, errors.New("bad not filter")
		}
		m, err := matcher(p.Children[0])
		if err != nil {
			return nil, err
		}
		return func(a map[string][]string) bool { return !m(a) }, nil
	case ldap.FilterPresent:
		name := string(p.Data.Bytes())
		return func(a map[string][]string) bool {
			return strings.EqualFold(name, "objectClass") || len(get(a, name)) > 0
		}, nil
	case ldap.FilterEqualityMatch, ldap.FilterApproxMatch, ldap.FilterGreaterOrEqual, ldap.FilterLessOrEqual:
		if len(p.Children) != 2 {
			return nil, errors.New("bad filter")
		}
		name, _ := p.Children[0].Value.(string)
		want, _ := p.Children[1].Value.(string)
		tag := p.Tag
		return func(a map[string][]string) bool {
			w := norm(name, want)
			for _, v := range get(a, name) {
				v = norm(name, v)
				switch tag {
				case ldap.FilterGreaterOrEqual:
					if v >= w {
						return true
					}
				case ldap.FilterLessOrEqual:
					if v <= w {
						return true
					}
				default:
					if v == w {
						return true
					}
				}
			}
			return false
		}, nil
	case ldap.FilterSubstrings:
		if len(p.Children) != 2 {
			return nil, errors.New("bad substring filter")
		}
		name, _ := p.Children[0].Value.(string)
		type part struct {
			kind ber.Tag
			s    string
		}
		parts := []part{}
		for _, c := range p.Children[1].Children {
			s, _ := c.Value.(string)
			if s == "" {
				s = string(c.Data.Bytes())
			}
			parts = append(parts, part{c.Tag, norm(name, s)})
		}
		return func(a map[string][]string) bool {
			for _, v := range get(a, name) {
				v = norm(name, v)
				ok, pos := true, 0
				for _, pt := range parts {
					switch pt.kind {
					case ldap.FilterSubstringsInitial:
						if !strings.HasPrefix(v, pt.s) {
							ok = false
						}
						pos = len(pt.s)
					case ldap.FilterSubstringsAny:
						i := strings.Index(v[min(pos, len(v)):], pt.s)
						if i < 0 {
							ok = false
						} else {
							pos += i + len(pt.s)
						}
					case ldap.FilterSubstringsFinal:
						if !strings.HasSuffix(v[min(pos, len(v)):], pt.s) {
							ok = false
						}
					}
					if !ok {
						break
					}
				}
				if ok {
					return true
				}
			}
			return false
		}, nil
	}
	return nil, errors.New("unsupported filter")
}
