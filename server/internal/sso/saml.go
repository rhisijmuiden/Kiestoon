package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// ParseIDPMetadata checks and parses the metadata XML of a SAML identity provider.
func ParseIDPMetadata(xml string) (*saml.EntityDescriptor, error) {
	if strings.TrimSpace(xml) == "" || len(xml) > 512<<10 {
		return nil, errors.New("no metadata")
	}
	ed, err := samlsp.ParseMetadata([]byte(xml))
	if err != nil {
		return nil, err
	}
	if len(ed.IDPSSODescriptors) == 0 {
		return nil, errors.New("metadata has no IDPSSODescriptor")
	}
	return ed, nil
}

// FetchIDPMetadata downloads the metadata of an identity provider.
func FetchIDPMetadata(ctx context.Context, metadataURL string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return "", err
	}
	if _, err := ParseIDPMetadata(string(b)); err != nil {
		return "", err
	}
	return string(b), nil
}

const spKeySetting = "saml_sp_key"

// SPCredentials returns the key and certificate Kiestoon signs SAML requests with (and decrypts
// encrypted assertions with). They are created on first use and kept in the database.
func SPCredentials(ctx context.Context, db *sql.DB) (*rsa.PrivateKey, *x509.Certificate, error) {
	raw, err := store.GetSetting(ctx, db, spKeySetting, "")
	if err != nil {
		return nil, nil, err
	}
	if raw != "" {
		var key *rsa.PrivateKey
		var cert *x509.Certificate
		rest := []byte(raw)
		for {
			var blk *pem.Block
			blk, rest = pem.Decode(rest)
			if blk == nil {
				break
			}
			switch blk.Type {
			case "RSA PRIVATE KEY":
				key, _ = x509.ParsePKCS1PrivateKey(blk.Bytes)
			case "CERTIFICATE":
				cert, _ = x509.ParseCertificate(blk.Bytes)
			}
		}
		if key != nil && cert != nil && time.Until(cert.NotAfter) > 30*24*time.Hour {
			return key, cert, nil
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Kiestoon SAML", Organization: []string{"Kiestoon"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, _ := x509.ParseCertificate(der)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})) +
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err := store.SetSetting(ctx, db, spKeySetting, pemText); err != nil {
		return nil, nil, err
	}
	return key, cert, nil
}

// SAMLEndpoints are the addresses of Kiestoon as a SAML service provider.
type SAMLEndpoints struct {
	EntityID string // the metadata URL
	ACS      string
}

// Endpoints derives the SP addresses from the public base URL of the portal.
func Endpoints(base string) SAMLEndpoints {
	base = strings.TrimRight(base, "/")
	return SAMLEndpoints{EntityID: base + "/api/auth/saml/metadata", ACS: base + "/api/auth/saml/acs"}
}

// ServiceProvider builds the SP for one identity provider (idp may be nil for the metadata only).
func ServiceProvider(ctx context.Context, db *sql.DB, ep SAMLEndpoints, idp *Provider) (*saml.ServiceProvider, error) {
	key, cert, err := SPCredentials(ctx, db)
	if err != nil {
		return nil, err
	}
	mu, err := url.Parse(ep.EntityID)
	if err != nil {
		return nil, err
	}
	au, err := url.Parse(ep.ACS)
	if err != nil {
		return nil, err
	}
	sp := &saml.ServiceProvider{
		EntityID:          ep.EntityID,
		Key:               key,
		Certificate:       cert,
		MetadataURL:       *mu,
		AcsURL:            *au,
		AuthnNameIDFormat: saml.UnspecifiedNameIDFormat,
		SignatureMethod:   "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256",
		AllowIDPInitiated: false,
	}
	if idp != nil {
		ed, err := ParseIDPMetadata(idp.SAMLMetadata)
		if err != nil {
			return nil, err
		}
		sp.IDPMetadata = ed
	}
	return sp, nil
}

type samlPending struct {
	provider  int64
	requestID string
	expires   time.Time
}

// SAMLFlow keeps the SAML sign-ins in progress. The browser comes back with a cross-site POST, which
// carries no SameSite cookie, so the RelayState and the request ID are checked against this list.
type SAMLFlow struct {
	mu      sync.Mutex
	pending map[string]samlPending
}

// NewSAMLFlow returns an empty flow.
func NewSAMLFlow() *SAMLFlow { return &SAMLFlow{pending: map[string]samlPending{}} }

// Start returns the URL that sends the browser to the identity provider.
func (f *SAMLFlow) Start(sp *saml.ServiceProvider, p *Provider) (string, error) {
	loc := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)
	if loc == "" {
		return "", errors.New("identity provider has no HTTP-Redirect sign-in endpoint")
	}
	req, err := sp.MakeAuthenticationRequest(loc, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		return "", err
	}
	state := random()
	f.mu.Lock()
	for k, v := range f.pending {
		if time.Now().After(v.expires) {
			delete(f.pending, k)
		}
	}
	if len(f.pending) > 1000 {
		f.mu.Unlock()
		return "", errors.New("too many sign-ins in progress")
	}
	f.pending[state] = samlPending{provider: p.ID, requestID: req.ID, expires: time.Now().Add(10 * time.Minute)}
	f.mu.Unlock()
	u, err := req.Redirect(state, sp)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Take returns and removes the sign-in of a RelayState.
func (f *SAMLFlow) Take(state string) (provider int64, requestID string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pd, found := f.pending[state]
	delete(f.pending, state)
	if !found || time.Now().After(pd.expires) {
		return 0, "", false
	}
	return pd.provider, pd.requestID, true
}

// Common attribute names of Entra ID, AD FS, Okta, Keycloak and others.
var (
	attrEmail = []string{"email", "mail", "emailaddress", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress", "urn:oid:0.9.2342.19200300.100.1.3"}
	attrName  = []string{"displayName", "name", "http://schemas.microsoft.com/identity/claims/displayname", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name", "urn:oid:2.16.840.1.113730.3.1.241", "cn"}
	attrUser  = []string{"preferred_username", "uid", "username", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/upn", "urn:oid:0.9.2342.19200300.100.1.1"}
	attrGroup = []string{"groups", "memberOf", "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups", "http://schemas.microsoft.com/ws/2008/06/identity/claims/role", "Role"}
)

// SAMLIdentity turns a verified assertion into an Identity.
func SAMLIdentity(p *Provider, a *saml.Assertion) (*Identity, error) {
	if a.Subject == nil || a.Subject.NameID == nil || strings.TrimSpace(a.Subject.NameID.Value) == "" {
		return nil, errors.New("assertion without NameID")
	}
	values := map[string][]string{}
	for _, st := range a.AttributeStatements {
		for _, at := range st.Attributes {
			for _, v := range at.Values {
				if v.Value == "" {
					continue
				}
				for _, k := range []string{at.Name, at.FriendlyName} {
					if k != "" {
						values[strings.ToLower(k)] = append(values[strings.ToLower(k)], v.Value)
					}
				}
			}
		}
	}
	first := func(names []string) string {
		for _, n := range names {
			if v := values[strings.ToLower(n)]; len(v) > 0 {
				return v[0]
			}
		}
		return ""
	}
	id := &Identity{ProviderID: p.ID, Subject: a.Subject.NameID.Value}
	id.Email = first(attrEmail)
	id.Name = first(attrName)
	id.Username = first(attrUser)
	if id.Email == "" && strings.Contains(id.Subject, "@") {
		id.Email = id.Subject
	}
	if id.Username == "" {
		id.Username = id.Email
	}
	names := attrGroup
	if p.GroupsClaim != "" && p.GroupsClaim != "groups" {
		names = append([]string{p.GroupsClaim}, attrGroup...)
	}
	seen := map[string]bool{}
	for _, n := range names {
		for _, g := range values[strings.ToLower(n)] {
			if !seen[g] {
				seen[g] = true
				id.Groups = append(id.Groups, g)
			}
		}
	}
	return id, nil
}
