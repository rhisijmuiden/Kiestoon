package crm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Exact Online with OAuth (an app registered in the Exact App Center, with Kiestoon's callback URL
// as redirect URI). Access tokens last 10 minutes; every refresh hands out a new refresh token, which
// is saved at once. Exact has no phone search, so Kiestoon keeps an index of the phone numbers of
// all contacts and accounts (rebuilt every hour through the bulk API).
type exact struct {
	host, clientID string
	division       int

	mu   sync.Mutex
	sec  Secret
	save SaveSecret

	idxMu    sync.Mutex
	idx      map[string][]exactEntry
	idxAt    time.Time
	idxBusy  bool
	idxError error
}

type exactEntry struct {
	contact Contact
}

// exactIndexTTL is how long the phone index is used before it is rebuilt.
var exactIndexTTL = time.Hour

func newExact(cfg *Config, sec Secret, save SaveSecret) *exact {
	return &exact{host: ExactHost(cfg.Region), clientID: cfg.ClientID, division: cfg.Division, sec: sec, save: save}
}

// exactHostOverride lets tests talk to a local server.
var exactHostOverride string

// ExactHost is the Exact Online site of a region.
func ExactHost(region string) string {
	if exactHostOverride != "" {
		return exactHostOverride
	}
	h := exactRegions[region]
	if h == "" {
		h = exactRegions["nl"]
	}
	return "https://" + h
}

// ExactAuthURL is where an admin signs in to give Kiestoon access.
func ExactAuthURL(cfg *Config, redirect, state string) string {
	q := url.Values{"client_id": {cfg.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "state": {state}, "force_login": {"0"}}
	return ExactHost(cfg.Region) + "/api/oauth2/auth?" + q.Encode()
}

type exactToken struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    json.RawMessage `json:"expires_in"` // Exact sends a string, others a number
}

func (t exactToken) expiry() time.Time {
	s := strings.Trim(string(t.ExpiresIn), `"`)
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		n = 600
	}
	return time.Now().Add(time.Duration(n) * time.Second)
}

// ExactExchange turns the code from the sign-in into tokens.
func ExactExchange(ctx context.Context, cfg *Config, sec Secret, code, redirect string) (Secret, error) {
	var t exactToken
	err := postForm(ctx, ExactHost(cfg.Region)+"/api/oauth2/token", url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {redirect}, "client_id": {cfg.ClientID}, "client_secret": {sec.ClientSecret}}, &t)
	if err != nil {
		return sec, err
	}
	if t.RefreshToken == "" {
		return sec, errors.New("crm: Exact sent no refresh token")
	}
	sec.AccessToken, sec.RefreshToken, sec.Expiry, sec.LoginExpired = t.AccessToken, t.RefreshToken, t.expiry(), false
	return sec, nil
}

func (e *exact) token(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sec.AccessToken != "" && time.Until(e.sec.Expiry) > 30*time.Second {
		return e.sec.AccessToken, nil
	}
	if e.sec.RefreshToken == "" {
		return "", ErrAuth
	}
	var t exactToken
	err := postForm(ctx, e.host+"/api/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {e.sec.RefreshToken},
		"client_id": {e.clientID}, "client_secret": {e.sec.ClientSecret}}, &t)
	if err != nil {
		return "", err
	}
	e.sec.AccessToken, e.sec.Expiry = t.AccessToken, t.expiry()
	if t.RefreshToken != "" {
		e.sec.RefreshToken = t.RefreshToken
	}
	e.save(e.sec)
	return e.sec.AccessToken, nil
}

func (e *exact) doURL(ctx context.Context, method, u string, in, out any) error {
	tok, err := e.token(ctx)
	if err != nil {
		return err
	}
	return doJSON(ctx, method, u, http.Header{"Authorization": {"Bearer " + tok}, "Prefer": {"return=representation"}}, in, out)
}

func (e *exact) div(ctx context.Context) (int, error) {
	e.mu.Lock()
	d := e.division
	e.mu.Unlock()
	if d != 0 {
		return d, nil
	}
	var res struct {
		D struct {
			Results []struct {
				CurrentDivision int    `json:"CurrentDivision"`
				FullName        string `json:"FullName"`
			} `json:"results"`
		} `json:"d"`
	}
	if err := e.doURL(ctx, http.MethodGet, e.host+"/api/v1/current/Me?$select=CurrentDivision,FullName", nil, &res); err != nil {
		return 0, err
	}
	if len(res.D.Results) == 0 || res.D.Results[0].CurrentDivision == 0 {
		return 0, errors.New("crm: Exact has no current division")
	}
	e.mu.Lock()
	e.division = res.D.Results[0].CurrentDivision
	e.mu.Unlock()
	return res.D.Results[0].CurrentDivision, nil
}

func (e *exact) api(ctx context.Context, method, path string, in, out any) error {
	d, err := e.div(ctx)
	if err != nil {
		return err
	}
	return e.doURL(ctx, method, fmt.Sprintf("%s/api/v1/%d/%s", e.host, d, path), in, out)
}

func (e *exact) Test(ctx context.Context) (string, error) {
	d, err := e.div(ctx)
	if err != nil {
		return "", err
	}
	var res struct {
		D struct {
			Results []struct {
				Description string `json:"Description"`
			} `json:"results"`
		} `json:"d"`
	}
	if err := e.api(ctx, http.MethodGet, fmt.Sprintf("system/Divisions?$filter=Code+eq+%d&$select=Description", d), nil, &res); err == nil && len(res.D.Results) > 0 {
		return fmt.Sprintf("%s (%d)", res.D.Results[0].Description, d), nil
	}
	return "Division " + strconv.Itoa(d), nil
}

var reGUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (e *exact) docURL(kind, id string) string {
	d := e.division
	switch kind {
	case "account":
		return fmt.Sprintf("%s/docs/CRMAccountCard.aspx?_Division_=%d&AccountID=%%7B%s%%7D", e.host, d, id)
	case "contact":
		return fmt.Sprintf("%s/docs/CRMContact.aspx?_Division_=%d&ContactID=%%7B%s%%7D", e.host, d, id)
	}
	return ""
}

// pages reads every page of a bulk or list endpoint.
func (e *exact) pages(ctx context.Context, path string, each func(json.RawMessage)) error {
	d, err := e.div(ctx)
	if err != nil {
		return err
	}
	next := fmt.Sprintf("%s/api/v1/%d/%s", e.host, d, path)
	for range 200 {
		var res struct {
			D struct {
				Results []json.RawMessage `json:"results"`
				Next    string            `json:"__next"`
			} `json:"d"`
		}
		if err := e.doURL(ctx, http.MethodGet, next, nil, &res); err != nil {
			return err
		}
		for _, r := range res.D.Results {
			each(r)
		}
		if res.D.Next == "" || !strings.HasPrefix(res.D.Next, e.host+"/") {
			return nil
		}
		next = res.D.Next
	}
	return nil
}

func (e *exact) buildIndex(ctx context.Context) (map[string][]exactEntry, error) {
	idx := map[string][]exactEntry{}
	add := func(num string, c Contact) {
		k := KeyOf(num)
		if len(k) < 7 {
			return
		}
		for _, x := range idx[k] {
			if x.contact.ID == c.ID {
				return
			}
		}
		c.Phone = num
		idx[k] = append(idx[k], exactEntry{contact: c})
	}
	err := e.pages(ctx, "bulk/CRM/Contacts?$select=ID,FullName,Phone,Mobile,BusinessPhone,BusinessMobile,Email,Account,AccountName", func(raw json.RawMessage) {
		var r struct {
			ID, FullName, Phone, Mobile, BusinessPhone, BusinessMobile, Email, Account, AccountName string
		}
		if json.Unmarshal(raw, &r) != nil {
			return
		}
		c := Contact{CRM: Exact, ID: r.ID, Name: r.FullName, Email: r.Email, Company: r.AccountName, CompanyID: r.Account, URL: e.docURL("contact", r.ID)}
		for _, n := range []string{r.Phone, r.Mobile, r.BusinessPhone, r.BusinessMobile} {
			add(n, c)
		}
	})
	if err != nil {
		return nil, err
	}
	err = e.pages(ctx, "bulk/CRM/Accounts?$select=ID,Name,Phone,Email", func(raw json.RawMessage) {
		var r struct{ ID, Name, Phone, Email string }
		if json.Unmarshal(raw, &r) != nil {
			return
		}
		add(r.Phone, Contact{CRM: Exact, ID: r.ID, IsCompany: true, CompanyID: r.ID, Name: r.Name, Company: r.Name, Email: r.Email, URL: e.docURL("account", r.ID)})
	})
	return idx, err
}

// index returns the phone index, rebuilding it when it is old. While a rebuild runs the old index
// is used, so a call never waits for it (only the very first look-up does).
func (e *exact) index(ctx context.Context) (map[string][]exactEntry, error) {
	e.idxMu.Lock()
	idx, at, busy := e.idx, e.idxAt, e.idxBusy
	if idx != nil && time.Since(at) < exactIndexTTL || busy && idx != nil {
		e.idxMu.Unlock()
		return idx, nil
	}
	e.idxBusy = true
	e.idxMu.Unlock()
	rebuild := func(ctx context.Context) (map[string][]exactEntry, error) {
		n, err := e.buildIndex(ctx)
		e.idxMu.Lock()
		defer e.idxMu.Unlock()
		e.idxBusy, e.idxError = false, err
		if err == nil {
			e.idx, e.idxAt = n, time.Now()
		}
		return e.idx, err
	}
	if idx != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = rebuild(ctx)
		}()
		return idx, nil
	}
	return rebuild(ctx)
}

func (e *exact) Lookup(ctx context.Context, n Number, cfg *Config) ([]Contact, error) {
	idx, err := e.index(ctx)
	if err != nil {
		return nil, err
	}
	var people, companies []Contact
	for _, x := range idx[n.Key] {
		c := x.contact
		if !n.Matches(c.Phone) {
			continue
		}
		if c.IsCompany {
			if cfg.LookupCompanies {
				companies = append(companies, c)
			}
		} else if cfg.LookupContacts {
			people = append(people, c)
		}
	}
	if len(people) > 0 {
		return people, nil
	}
	return companies, nil
}

func odataDate(s string) string {
	// /Date(1700000000000)/
	s = strings.TrimSuffix(strings.TrimPrefix(s, "/Date("), ")/")
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	return ""
}

func (e *exact) Details(ctx context.Context, c *Contact, cfg *Config) error {
	if !hasShow(cfg, ShowDeals) || !reGUID.MatchString(c.CompanyID) {
		return nil
	}
	var res struct {
		D struct {
			Results []struct {
				ID         string  `json:"ID"`
				Name       string  `json:"Name"`
				AmountDC   float64 `json:"AmountDC"`
				ActionDate string  `json:"ActionDate"`
			} `json:"results"`
		} `json:"d"`
	}
	q := "crm/Opportunities?$select=ID,Name,AmountDC,ActionDate&$top=5&$filter=" + url.QueryEscape("Account eq guid'"+c.CompanyID+"' and OpportunityStatus eq 1")
	if err := e.api(ctx, http.MethodGet, q, nil, &res); err != nil {
		return err
	}
	for _, o := range res.D.Results {
		v := ""
		if o.AmountDC != 0 {
			v = "€ " + strconv.FormatFloat(o.AmountDC, 'f', -1, 64)
		}
		c.Deals = append(c.Deals, Item{Title: o.Name, Value: v, When: odataDate(o.ActionDate)})
	}
	return nil
}

func exactLinks(c Contact, body map[string]any) {
	if reGUID.MatchString(c.CompanyID) {
		body["Account"] = c.CompanyID
	}
	if !c.IsCompany && reGUID.MatchString(c.ID) {
		body["Contact"] = c.ID
	}
}

type exactCreated struct {
	D struct {
		ID string `json:"ID"`
	} `json:"d"`
}

// LogCall writes a communication note: Exact has no call activity.
func (e *exact) LogCall(ctx context.Context, c Call) (string, error) {
	body := map[string]any{"Subject": c.Title(), "Notes": c.Body}
	exactLinks(c.Contact, body)
	var res exactCreated
	err := e.api(ctx, http.MethodPost, "activities/CommunicationNotes", body, &res)
	return res.D.ID, err
}

func (e *exact) AddNote(ctx context.Context, id string, c Call, text string) error {
	if !reGUID.MatchString(id) {
		return errors.New("crm: bad activity id")
	}
	return e.api(ctx, http.MethodPut, "activities/CommunicationNotes(guid'"+id+"')", map[string]any{"Notes": strings.TrimSpace(c.Body + "\n\n" + text)}, nil)
}

func (e *exact) CreateTask(ctx context.Context, t Task) (string, error) {
	body := map[string]any{"Description": t.Subject, "Notes": t.Body, "ActionDate": t.Due.UTC().Format("2006-01-02T15:04:05")}
	exactLinks(t.Contact, body)
	if reGUID.MatchString(t.OwnerID) {
		body["User"] = t.OwnerID
	}
	var res exactCreated
	err := e.api(ctx, http.MethodPost, "activities/Tasks", body, &res)
	return res.D.ID, err
}

// CreateContact creates an account (the company, or the person when there is none) with a contact:
// in Exact every contact belongs to an account.
func (e *exact) CreateContact(ctx context.Context, name, company, phone string) (Contact, error) {
	accName := company
	if accName == "" {
		accName = name
	}
	var acc exactCreated
	if err := e.api(ctx, http.MethodPost, "crm/Accounts", map[string]any{"Name": accName, "Phone": phone, "Status": "C"}, &acc); err != nil {
		return Contact{}, err
	}
	first, last := splitName(name)
	if last == "" {
		first, last = "", first
	}
	var con exactCreated
	if err := e.api(ctx, http.MethodPost, "crm/Contacts", map[string]any{"Account": acc.D.ID, "FirstName": first, "LastName": last, "Phone": phone}, &con); err != nil {
		return Contact{}, err
	}
	c := Contact{CRM: Exact, ID: con.D.ID, Name: name, Company: company, CompanyID: acc.D.ID, Phone: phone, URL: e.docURL("contact", con.D.ID)}
	// The next look-up must find the new number.
	e.idxMu.Lock()
	if e.idx != nil {
		e.idx[KeyOf(phone)] = append(e.idx[KeyOf(phone)], exactEntry{contact: c})
	}
	e.idxMu.Unlock()
	return c, nil
}

func (e *exact) Owners(ctx context.Context) ([]Owner, error) {
	var out []Owner
	err := e.pages(ctx, "users/Users?$select=UserID,FullName,Email", func(raw json.RawMessage) {
		var u struct{ UserID, FullName, Email string }
		if json.Unmarshal(raw, &u) == nil {
			out = append(out, Owner{ID: u.UserID, Name: u.FullName, Email: u.Email})
		}
	})
	return out, err
}
