// Package crm links Kiestoon to CRM systems (HubSpot, Salesforce, Pipedrive, Exact Online): it
// looks callers up by phone number for the call pop-up and the desk phone display, and writes every
// call back to the CRM as an activity. Only the phone number goes out for a look-up; recordings
// never leave the server (the CRM gets a link).
package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Kinds of CRM.
const (
	HubSpot    = "hubspot"
	Salesforce = "salesforce"
	Pipedrive  = "pipedrive"
	Exact      = "exact"
)

// Kinds lists every CRM in the default look-up order.
var Kinds = []string{HubSpot, Salesforce, Pipedrive, Exact}

// Names are the product names, for logs and the UI.
var Names = map[string]string{HubSpot: "HubSpot", Salesforce: "Salesforce", Pipedrive: "Pipedrive", Exact: "Exact Online"}

// SettingsKey is the settings row with Settings as JSON. Secrets live apart (SecretKey), so they
// are never sent back to the browser.
const SettingsKey = "crm"

// SecretKey is the settings row with the Secret of a CRM.
func SecretKey(kind string) string { return "crm_secret_" + kind }

// DeskNameKey is set to "1" when CRM names go to desk phone displays (read by the dialplan generator).
const DeskNameKey = "crm_desk_name"

// Fields that can show in the pop-up.
const (
	ShowDeals     = "deals"
	ShowTickets   = "tickets"
	ShowLast      = "last"
	ShowOwner     = "owner"
	ShowLifecycle = "lifecycle"
)

// Which calls are logged.
const (
	LogAll      = "all"
	LogInbound  = "inbound"
	LogAnswered = "answered"
)

// Settings are the CRM settings of the installation.
type Settings struct {
	// Order is the look-up order: the first CRM that knows the number wins.
	Order []string `json:"order"`
	// DeskName puts the CRM name on the display of desk phones (the call waits at most 1.5 s).
	DeskName bool `json:"deskName"`
	// PopupAll shows the pop-up to everyone whose phone rings (queues, ring groups), not only the
	// one who answers.
	PopupAll bool `json:"popupAll"`
	// Connectors per kind.
	Connectors map[string]*Config `json:"connectors"`
}

// Config is the non-secret part of one CRM link.
type Config struct {
	Enabled bool `json:"enabled"`
	// Domain is the Pipedrive company domain (acme for acme.pipedrive.com) or the Salesforce My
	// Domain URL (https://acme.my.salesforce.com).
	Domain string `json:"domain,omitempty"`
	// ClientID is the Salesforce consumer key or the Exact Online client ID.
	ClientID string `json:"clientId,omitempty"`
	// Region is the Exact Online country site (nl, be, de, uk, fr, es, us).
	Region string `json:"region,omitempty"`
	// Division is the Exact Online administration (0 = the user's current one).
	Division int `json:"division,omitempty"`

	LookupContacts  bool     `json:"lookupContacts"`
	LookupCompanies bool     `json:"lookupCompanies"`
	Show            []string `json:"show"`
	LogCalls        bool     `json:"logCalls"`
	LogWhich        string   `json:"logWhich"`
	MissedTask      bool     `json:"missedTask"`
	RecordingLink   bool     `json:"recordingLink"`
	AISummary       bool     `json:"aiSummary"`
	CreateUnknown   bool     `json:"createUnknown"`
	// Owners maps a Kiestoon user id to a CRM owner id, for users whose e-mail does not match.
	Owners map[string]string `json:"owners,omitempty"`
}

// DefaultConfig is the configuration of a CRM that was never saved.
func DefaultConfig(kind string) *Config {
	c := &Config{LookupContacts: true, LookupCompanies: true, Show: []string{ShowDeals, ShowTickets, ShowLast, ShowOwner},
		LogCalls: true, LogWhich: LogAll, MissedTask: true, AISummary: true}
	if kind == Exact {
		c.Region = "nl"
	}
	return c
}

// Defaults are the settings until an admin saves them.
func Defaults() Settings {
	s := Settings{Order: slices.Clone(Kinds), DeskName: true, PopupAll: true, Connectors: map[string]*Config{}}
	for _, k := range Kinds {
		s.Connectors[k] = DefaultConfig(k)
	}
	return s
}

// Normalize fills in what is missing (new kinds, the order) so older saved settings keep working.
func (s *Settings) Normalize() {
	if s.Connectors == nil {
		s.Connectors = map[string]*Config{}
	}
	for _, k := range Kinds {
		if s.Connectors[k] == nil {
			s.Connectors[k] = DefaultConfig(k)
		}
		if !slices.Contains(s.Order, k) {
			s.Order = append(s.Order, k)
		}
	}
	s.Order = slices.DeleteFunc(s.Order, func(k string) bool { return Names[k] == "" })
	for k := range s.Connectors {
		if Names[k] == "" {
			delete(s.Connectors, k)
		}
	}
}

var exactRegions = map[string]string{"nl": "start.exactonline.nl", "be": "start.exactonline.be", "de": "start.exactonline.de",
	"uk": "start.exactonline.co.uk", "fr": "start.exactonline.fr", "es": "start.exactonline.es", "us": "start.exactonline.com"}

// Validate checks the settings. The error is the name of the field that is wrong.
func (s *Settings) Validate() error {
	s.Normalize()
	for k, c := range s.Connectors {
		c.Domain = strings.TrimSpace(c.Domain)
		c.ClientID = strings.TrimSpace(c.ClientID)
		if !slices.Contains([]string{LogAll, LogInbound, LogAnswered}, c.LogWhich) {
			return errors.New(k + ".logWhich")
		}
		for _, f := range c.Show {
			if !slices.Contains([]string{ShowDeals, ShowTickets, ShowLast, ShowOwner, ShowLifecycle}, f) {
				return errors.New(k + ".show")
			}
		}
		if len(c.Owners) > 500 {
			return errors.New(k + ".owners")
		}
		switch k {
		case Pipedrive:
			c.Domain = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(c.Domain, "https://"), "http://"), "/")
			c.Domain = strings.TrimSuffix(c.Domain, ".pipedrive.com")
			if c.Enabled && !validLabel(c.Domain) {
				return errors.New(k + ".domain")
			}
		case Salesforce:
			c.Domain = strings.TrimSuffix(c.Domain, "/")
			if c.Domain != "" && !strings.Contains(c.Domain, "://") {
				c.Domain = "https://" + c.Domain
			}
			if c.Enabled {
				u, err := url.Parse(c.Domain)
				if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".salesforce.com") || u.Path != "" {
					return errors.New(k + ".domain")
				}
				if c.ClientID == "" || len(c.ClientID) > 300 {
					return errors.New(k + ".clientId")
				}
			}
		case Exact:
			if exactRegions[c.Region] == "" {
				return errors.New(k + ".region")
			}
			if c.Enabled && (c.ClientID == "" || len(c.ClientID) > 100) {
				return errors.New(k + ".clientId")
			}
		}
	}
	return nil
}

func validLabel(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// Secret holds the credentials of one CRM. It is stored apart and never sent to the browser.
type Secret struct {
	// Token is the HubSpot private app token or the Pipedrive API token.
	Token string `json:"token,omitempty"`
	// ClientSecret is the Salesforce consumer secret or the Exact Online client secret.
	ClientSecret string `json:"clientSecret,omitempty"`
	// RefreshToken, AccessToken and Expiry are the OAuth tokens (Exact Online; Salesforce keeps
	// only the access token).
	RefreshToken string    `json:"refreshToken,omitempty"`
	AccessToken  string    `json:"accessToken,omitempty"`
	Expiry       time.Time `json:"expiry,omitzero"`
	// Account describes the connected account, for the UI (portal id, company name...).
	Account string `json:"account,omitempty"`
	// LoginExpired is set when the CRM refused the stored sign-in (an admin must sign in again).
	LoginExpired bool `json:"loginExpired,omitempty"`
}

// Configured tells whether the secret has what the kind needs to connect.
func (s Secret) Configured(kind string) bool {
	switch kind {
	case HubSpot, Pipedrive:
		return s.Token != ""
	case Salesforce:
		return s.ClientSecret != ""
	case Exact:
		return s.RefreshToken != ""
	}
	return false
}

// Item is a deal, ticket or activity shown in the pop-up.
type Item struct {
	Title string `json:"title"`
	Value string `json:"value,omitempty"`
	When  string `json:"when,omitempty"` // RFC 3339
	URL   string `json:"url,omitempty"`
}

// Contact is a person or company the CRM knows for a phone number.
type Contact struct {
	CRM       string `json:"crm"`
	ID        string `json:"id"`
	CompanyID string `json:"companyId,omitempty"`
	// IsCompany is set when the number belongs to a company record, not a person.
	IsCompany bool   `json:"isCompany,omitempty"`
	Name      string `json:"name"`
	Company   string `json:"company,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	OwnerID   string `json:"ownerId,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Lifecycle string `json:"lifecycle,omitempty"`
	URL       string `json:"url,omitempty"`
	Deals     []Item `json:"deals,omitempty"`
	Tickets   []Item `json:"tickets,omitempty"`
	Last      *Item  `json:"last,omitempty"`
}

// DisplayName is what goes on a desk phone: "Jan de Vries - Bakkerij De Vries".
func (c Contact) DisplayName() string {
	if c.Company != "" && c.Company != c.Name {
		return c.Name + " - " + c.Company
	}
	return c.Name
}

// Owner is a CRM user that can own contacts, calls and tasks.
type Owner struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Call is a finished call to write to the CRM.
type Call struct {
	Contact   Contact
	Direction string // inbound, outbound
	Answered  bool
	Number    string // the customer's number
	Line      string // our number or extension
	Start     time.Time
	TalkSec   int
	OwnerID   string
	Agent     string // display name of the colleague
	Body      string // note, recording link, summary (plain text)
}

// Title is the subject of the activity.
func (c Call) Title() string {
	switch {
	case c.Direction == "outbound":
		return "Outbound call " + c.Number
	case !c.Answered:
		return "Missed call " + c.Number
	}
	return "Inbound call " + c.Number
}

// Task is a follow-up task (call back).
type Task struct {
	Contact Contact
	OwnerID string
	Subject string
	Body    string
	Due     time.Time
}

// Connector talks to one CRM.
type Connector interface {
	// Test signs in and returns a description of the account.
	Test(ctx context.Context) (string, error)
	// Lookup finds the contacts and companies with this phone number.
	Lookup(ctx context.Context, n Number, cfg *Config) ([]Contact, error)
	// Details adds deals, tickets and the last activity to a contact found by Lookup.
	Details(ctx context.Context, c *Contact, cfg *Config) error
	// LogCall writes a call activity and returns its id.
	LogCall(ctx context.Context, c Call) (string, error)
	// AddNote adds text to an activity that LogCall wrote.
	AddNote(ctx context.Context, activityID string, c Call, text string) error
	// CreateTask creates a follow-up task.
	CreateTask(ctx context.Context, t Task) (string, error)
	// CreateContact creates a contact for an unknown number.
	CreateContact(ctx context.Context, name, company, phone string) (Contact, error)
	// Owners lists the CRM users that can own records.
	Owners(ctx context.Context) ([]Owner, error)
}

// ErrAuth means the CRM refused the credentials: an admin must sign in again.
var ErrAuth = errors.New("crm: sign-in refused")

// HTTPError is an unexpected answer from a CRM.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	b := e.Body
	if len(b) > 200 {
		b = b[:200]
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, strings.TrimSpace(b))
}

// client is the HTTP client of all connectors. Tests replace it.
var client = &http.Client{Timeout: 15 * time.Second}

// doJSON sends a request with an optional JSON body and decodes a JSON answer into out (when not nil).
func doJSON(ctx context.Context, method, u string, hdr http.Header, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "Kiestoon-CRM")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return ErrAuth
	}
	if res.StatusCode >= 300 {
		return &HTTPError{Status: res.StatusCode, Body: string(raw)}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// postForm sends an OAuth token request.
func postForm(ctx context.Context, u string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized {
		// invalid_grant, invalid_client: the stored sign-in is no longer valid.
		return fmt.Errorf("%w: %s", ErrAuth, strings.TrimSpace(string(raw)))
	}
	if res.StatusCode >= 300 {
		return &HTTPError{Status: res.StatusCode, Body: string(raw)}
	}
	return json.Unmarshal(raw, out)
}

// SaveSecret stores a changed secret (an OAuth token that was refreshed).
type SaveSecret func(Secret)

// New makes the connector of a kind.
func New(kind string, cfg *Config, sec Secret, save SaveSecret) (Connector, error) {
	if save == nil {
		save = func(Secret) {}
	}
	switch kind {
	case HubSpot:
		return &hubspot{base: HubSpotBase, token: sec.Token, account: sec.Account}, nil
	case Pipedrive:
		return &pipedrive{base: "https://" + cfg.Domain + ".pipedrive.com", token: sec.Token}, nil
	case Salesforce:
		return &salesforce{domain: cfg.Domain, clientID: cfg.ClientID, sec: sec, save: save}, nil
	case Exact:
		return newExact(cfg, sec, save), nil
	}
	return nil, fmt.Errorf("crm: unknown kind %q", kind)
}
