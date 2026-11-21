package crm

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sfVersion is the Salesforce REST API version.
const sfVersion = "v62.0"

// Salesforce with an External Client App that has the client credentials flow on (it runs as the
// "run as" user picked there) and the My Domain URL.
type salesforce struct {
	domain, clientID string

	mu       sync.Mutex
	sec      Secret
	save     SaveSecret
	instance string
}

var reSFID = regexp.MustCompile(`^[a-zA-Z0-9]{15,18}$`)

func (s *salesforce) token(ctx context.Context, fresh bool) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !fresh && s.sec.AccessToken != "" && s.instance != "" {
		return s.sec.AccessToken, s.instance, nil
	}
	var res struct {
		AccessToken string `json:"access_token"`
		InstanceURL string `json:"instance_url"`
	}
	err := postForm(ctx, s.domain+"/services/oauth2/token", url.Values{"grant_type": {"client_credentials"},
		"client_id": {s.clientID}, "client_secret": {s.sec.ClientSecret}}, &res)
	if err != nil {
		return "", "", err
	}
	if res.InstanceURL == "" {
		res.InstanceURL = s.domain
	}
	s.sec.AccessToken, s.instance = res.AccessToken, strings.TrimSuffix(res.InstanceURL, "/")
	s.save(s.sec)
	return s.sec.AccessToken, s.instance, nil
}

// do calls the REST API; an expired access token is renewed once.
func (s *salesforce) do(ctx context.Context, method, path string, in, out any) error {
	for attempt := 0; ; attempt++ {
		tok, inst, err := s.token(ctx, attempt > 0)
		if err != nil {
			return err
		}
		err = doJSON(ctx, method, inst+"/services/data/"+sfVersion+path, http.Header{"Authorization": {"Bearer " + tok}}, in, out)
		if errors.Is(err, ErrAuth) && attempt == 0 {
			continue
		}
		return err
	}
}

func (s *salesforce) query(ctx context.Context, soql string, out any) error {
	return s.do(ctx, http.MethodGet, "/query?q="+url.QueryEscape(soql), nil, out)
}

func (s *salesforce) recordURL(typ, id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.instance == "" {
		return ""
	}
	return s.instance + "/lightning/r/" + typ + "/" + id + "/view"
}

func (s *salesforce) Test(ctx context.Context) (string, error) {
	var res struct {
		Records []struct {
			Name string `json:"Name"`
		} `json:"records"`
	}
	if err := s.query(ctx, "SELECT Name FROM Organization LIMIT 1", &res); err != nil {
		return "", err
	}
	if len(res.Records) == 0 {
		return "Salesforce", nil
	}
	return res.Records[0].Name, nil
}

type sfRecord struct {
	Attributes struct {
		Type string `json:"type"`
	} `json:"attributes"`
	ID               string `json:"Id"`
	Name             string `json:"Name"`
	Phone            string `json:"Phone"`
	MobilePhone      string `json:"MobilePhone"`
	Email            string `json:"Email"`
	OwnerID          string `json:"OwnerId"`
	AccountID        string `json:"AccountId"`
	LastActivityDate string `json:"LastActivityDate"`
	Account          *struct {
		Name string `json:"Name"`
	} `json:"Account"`
	Owner *struct {
		Name string `json:"Name"`
	} `json:"Owner"`
}

// soslQuote escapes a term for a SOSL FIND clause.
func soslQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func (s *salesforce) Lookup(ctx context.Context, n Number, cfg *Config) ([]Contact, error) {
	var ret []string
	if cfg.LookupContacts {
		ret = append(ret, "Contact(Id, Name, Phone, MobilePhone, Email, OwnerId, Owner.Name, AccountId, Account.Name, LastActivityDate LIMIT 10)")
	}
	if cfg.LookupCompanies {
		ret = append(ret, "Account(Id, Name, Phone, OwnerId, Owner.Name, LastActivityDate LIMIT 10)")
	}
	if len(ret) == 0 {
		return nil, nil
	}
	terms := []string{soslQuote(n.Digits)}
	if n.Key != n.Digits {
		terms = append(terms, soslQuote(n.Key))
	}
	if n.E164 != "" {
		terms = append(terms, soslQuote(n.E164))
	}
	q := "FIND {" + strings.Join(terms, " OR ") + "} IN PHONE FIELDS RETURNING " + strings.Join(ret, ", ")
	var res struct {
		SearchRecords []sfRecord `json:"searchRecords"`
	}
	if err := s.do(ctx, http.MethodGet, "/search/?q="+url.QueryEscape(q), nil, &res); err != nil {
		return nil, err
	}
	var people, companies []Contact
	for _, r := range res.SearchRecords {
		phone := r.Phone
		if !n.Matches(phone) {
			phone = r.MobilePhone
		}
		if !n.Matches(phone) {
			continue
		}
		c := Contact{CRM: Salesforce, ID: r.ID, Name: r.Name, Phone: phone, Email: r.Email, OwnerID: r.OwnerID, URL: s.recordURL(r.Attributes.Type, r.ID)}
		if r.Owner != nil {
			c.Owner = r.Owner.Name
		}
		if r.LastActivityDate != "" {
			c.Last = &Item{Title: "Last activity", When: pdDate(r.LastActivityDate)}
		}
		if r.Attributes.Type == "Account" {
			c.IsCompany, c.CompanyID, c.Company = true, r.ID, r.Name
			companies = append(companies, c)
			continue
		}
		c.CompanyID = r.AccountID
		if r.Account != nil {
			c.Company = r.Account.Name
		}
		people = append(people, c)
	}
	if len(people) > 0 {
		return people, nil
	}
	return companies, nil
}

func (s *salesforce) Details(ctx context.Context, c *Contact, cfg *Config) error {
	if hasShow(cfg, ShowDeals) && reSFID.MatchString(c.CompanyID) {
		var res struct {
			Records []struct {
				ID        string  `json:"Id"`
				Name      string  `json:"Name"`
				Amount    float64 `json:"Amount"`
				CloseDate string  `json:"CloseDate"`
			} `json:"records"`
		}
		if err := s.query(ctx, "SELECT Id, Name, Amount, CloseDate FROM Opportunity WHERE AccountId = '"+c.CompanyID+"' AND IsClosed = false ORDER BY CloseDate LIMIT 5", &res); err != nil {
			return err
		}
		for _, o := range res.Records {
			v := ""
			if o.Amount != 0 {
				v = strconv.FormatFloat(o.Amount, 'f', -1, 64)
			}
			c.Deals = append(c.Deals, Item{Title: o.Name, Value: v, When: pdDate(o.CloseDate), URL: s.recordURL("Opportunity", o.ID)})
		}
	}
	if hasShow(cfg, ShowTickets) && reSFID.MatchString(c.ID) {
		field := "ContactId"
		if c.IsCompany {
			field = "AccountId"
		}
		var res struct {
			Records []struct {
				ID      string `json:"Id"`
				Number  string `json:"CaseNumber"`
				Subject string `json:"Subject"`
				Created string `json:"CreatedDate"`
			} `json:"records"`
		}
		if err := s.query(ctx, "SELECT Id, CaseNumber, Subject, CreatedDate FROM Case WHERE "+field+" = '"+c.ID+"' AND IsClosed = false ORDER BY CreatedDate DESC LIMIT 5", &res); err != nil {
			return err
		}
		for _, o := range res.Records {
			when := ""
			if t, err := time.Parse("2006-01-02T15:04:05.000-0700", o.Created); err == nil {
				when = t.UTC().Format(time.RFC3339)
			}
			c.Tickets = append(c.Tickets, Item{Title: "#" + o.Number + " · " + o.Subject, When: when, URL: s.recordURL("Case", o.ID)})
		}
	}
	return nil
}

func (s *salesforce) whoWhat(c Contact, body map[string]any) {
	if !c.IsCompany && reSFID.MatchString(c.ID) {
		body["WhoId"] = c.ID
	}
	if reSFID.MatchString(c.CompanyID) {
		body["WhatId"] = c.CompanyID
	}
}

func (s *salesforce) LogCall(ctx context.Context, c Call) (string, error) {
	typ, disp := "Inbound", "Connected"
	if c.Direction == "outbound" {
		typ = "Outbound"
	}
	if !c.Answered {
		disp = "No answer"
	}
	body := map[string]any{"Subject": c.Title(), "TaskSubtype": "Call", "CallType": typ, "CallDurationInSeconds": c.TalkSec,
		"CallDisposition": disp, "Status": "Completed", "ActivityDate": c.Start.Format("2006-01-02"), "Description": c.Body}
	s.whoWhat(c.Contact, body)
	if reSFID.MatchString(c.OwnerID) {
		body["OwnerId"] = c.OwnerID
	}
	var res struct {
		ID string `json:"id"`
	}
	err := s.do(ctx, http.MethodPost, "/sobjects/Task", body, &res)
	return res.ID, err
}

func (s *salesforce) AddNote(ctx context.Context, id string, c Call, text string) error {
	if !reSFID.MatchString(id) {
		return errors.New("crm: bad activity id")
	}
	return s.do(ctx, http.MethodPatch, "/sobjects/Task/"+id, map[string]any{"Description": strings.TrimSpace(c.Body + "\n\n" + text)}, nil)
}

func (s *salesforce) CreateTask(ctx context.Context, t Task) (string, error) {
	body := map[string]any{"Subject": t.Subject, "Status": "Not Started", "ActivityDate": t.Due.Format("2006-01-02"), "Description": t.Body, "TaskSubtype": "Task"}
	s.whoWhat(t.Contact, body)
	if reSFID.MatchString(t.OwnerID) {
		body["OwnerId"] = t.OwnerID
	}
	var res struct {
		ID string `json:"id"`
	}
	err := s.do(ctx, http.MethodPost, "/sobjects/Task", body, &res)
	return res.ID, err
}

func (s *salesforce) CreateContact(ctx context.Context, name, company, phone string) (Contact, error) {
	first, last := splitName(name)
	if last == "" {
		first, last = "", first
	}
	c := Contact{CRM: Salesforce, Name: name, Company: company, Phone: phone}
	body := map[string]any{"FirstName": first, "LastName": last, "Phone": phone}
	if company != "" {
		var acc struct {
			ID string `json:"id"`
		}
		if err := s.do(ctx, http.MethodPost, "/sobjects/Account", map[string]any{"Name": company, "Phone": phone}, &acc); err != nil {
			return Contact{}, err
		}
		body["AccountId"], c.CompanyID = acc.ID, acc.ID
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := s.do(ctx, http.MethodPost, "/sobjects/Contact", body, &res); err != nil {
		return Contact{}, err
	}
	c.ID, c.URL = res.ID, s.recordURL("Contact", res.ID)
	return c, nil
}

func (s *salesforce) Owners(ctx context.Context) ([]Owner, error) {
	var res struct {
		Records []struct {
			ID    string `json:"Id"`
			Name  string `json:"Name"`
			Email string `json:"Email"`
		} `json:"records"`
	}
	if err := s.query(ctx, "SELECT Id, Name, Email FROM User WHERE IsActive = true AND UserType = 'Standard' ORDER BY Name LIMIT 500", &res); err != nil {
		return nil, err
	}
	out := make([]Owner, 0, len(res.Records))
	for _, u := range res.Records {
		out = append(out, Owner{ID: u.ID, Name: u.Name, Email: u.Email})
	}
	return out, nil
}
