package crm

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HubSpotBase is the HubSpot API (tests point it at a local server).
var HubSpotBase = "https://api.hubapi.com"

// HubSpot with a private app token (Settings → Integrations → Private apps). Scopes:
// crm.objects.contacts.read/write, crm.objects.companies.read, crm.objects.deals.read, tickets,
// crm.objects.owners.read (calls and tasks are written with the contacts write scope).
type hubspot struct {
	base, token, account string

	mu       sync.Mutex
	portalID int64
	uiDomain string
}

// HubSpot association type ids (HUBSPOT_DEFINED).
const (
	hsCallToContact = 194
	hsCallToCompany = 182
	hsTaskToContact = 204
	hsTaskToCompany = 192
)

func (h *hubspot) hdr() http.Header {
	return http.Header{"Authorization": {"Bearer " + h.token}}
}

func (h *hubspot) do(ctx context.Context, method, path string, in, out any) error {
	return doJSON(ctx, method, h.base+path, h.hdr(), in, out)
}

func (h *hubspot) portal(ctx context.Context) (int64, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.portalID != 0 {
		return h.portalID, h.uiDomain, nil
	}
	var a struct {
		PortalID int64  `json:"portalId"`
		UIDomain string `json:"uiDomain"`
	}
	if err := h.do(ctx, http.MethodGet, "/account-info/v3/details", nil, &a); err != nil {
		return 0, "", err
	}
	if a.UIDomain == "" {
		a.UIDomain = "app.hubspot.com"
	}
	h.portalID, h.uiDomain = a.PortalID, a.UIDomain
	return h.portalID, h.uiDomain, nil
}

func (h *hubspot) recordURL(ctx context.Context, object, id string) string {
	p, ui, err := h.portal(ctx)
	if err != nil || p == 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/contacts/%d/record/%s/%s", ui, p, object, id)
}

func (h *hubspot) Test(ctx context.Context) (string, error) {
	p, _, err := h.portal(ctx)
	if err != nil {
		return "", err
	}
	// The account details work with any token: check a scope we need as well.
	if err := h.do(ctx, http.MethodGet, "/crm/v3/objects/contacts?limit=1", nil, nil); err != nil {
		return "", err
	}
	return "Portal " + strconv.FormatInt(p, 10), nil
}

type hsFilter struct {
	PropertyName string `json:"propertyName"`
	Operator     string `json:"operator"`
	Value        string `json:"value,omitempty"`
}

type hsGroup struct {
	Filters []hsFilter `json:"filters"`
}

type hsSearch struct {
	FilterGroups []hsGroup `json:"filterGroups"`
	Properties   []string  `json:"properties"`
	Sorts        []string  `json:"sorts,omitempty"`
	Limit        int       `json:"limit"`
}

type hsObject struct {
	ID         string            `json:"id"`
	Properties map[string]string `json:"properties"`
}

type hsResults struct {
	Results []hsObject `json:"results"`
}

func (h *hubspot) search(ctx context.Context, object string, q hsSearch) ([]hsObject, error) {
	var res hsResults
	err := h.do(ctx, http.MethodPost, "/crm/v3/objects/"+object+"/search", q, &res)
	return res.Results, err
}

func hsDate(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	return ""
}

func (h *hubspot) Lookup(ctx context.Context, n Number, cfg *Config) ([]Contact, error) {
	var out []Contact
	if cfg.LookupContacts {
		props := []string{"firstname", "lastname", "company", "phone", "mobilephone", "email", "hubspot_owner_id", "lifecyclestage", "notes_last_contacted", "associatedcompanyid"}
		res, err := h.search(ctx, "contacts", hsSearch{Properties: props, Limit: 10, FilterGroups: []hsGroup{
			{Filters: []hsFilter{{"hs_searchable_calculated_phone_number", "EQ", n.Key}}},
			{Filters: []hsFilter{{"hs_searchable_calculated_mobile_number", "EQ", n.Key}}},
			{Filters: []hsFilter{{"phone", "CONTAINS_TOKEN", "*" + n.Key}}},
		}})
		if err != nil {
			return nil, err
		}
		for _, o := range res {
			p := o.Properties
			if !n.Matches(p["phone"]) && !n.Matches(p["mobilephone"]) {
				continue
			}
			name := strings.TrimSpace(p["firstname"] + " " + p["lastname"])
			if name == "" {
				name = p["email"]
			}
			phone := p["phone"]
			if !n.Matches(phone) {
				phone = p["mobilephone"]
			}
			c := Contact{CRM: HubSpot, ID: o.ID, Name: name, Company: p["company"], CompanyID: p["associatedcompanyid"], Phone: phone,
				Email: p["email"], OwnerID: p["hubspot_owner_id"], Lifecycle: p["lifecyclestage"], URL: h.recordURL(ctx, "0-1", o.ID)}
			if d := hsDate(p["notes_last_contacted"]); d != "" {
				c.Last = &Item{Title: "Last contacted", When: d}
			}
			out = append(out, c)
		}
	}
	if cfg.LookupCompanies && len(out) == 0 {
		res, err := h.search(ctx, "companies", hsSearch{Limit: 10,
			Properties:   []string{"name", "phone", "hubspot_owner_id", "lifecyclestage", "notes_last_contacted"},
			FilterGroups: []hsGroup{{Filters: []hsFilter{{"phone", "CONTAINS_TOKEN", "*" + n.Key}}}}})
		if err != nil {
			return nil, err
		}
		for _, o := range res {
			p := o.Properties
			if !n.Matches(p["phone"]) {
				continue
			}
			c := Contact{CRM: HubSpot, ID: o.ID, IsCompany: true, CompanyID: o.ID, Name: p["name"], Company: p["name"], Phone: p["phone"],
				OwnerID: p["hubspot_owner_id"], Lifecycle: p["lifecyclestage"], URL: h.recordURL(ctx, "0-2", o.ID)}
			if d := hsDate(p["notes_last_contacted"]); d != "" {
				c.Last = &Item{Title: "Last contacted", When: d}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (h *hubspot) Details(ctx context.Context, c *Contact, cfg *Config) error {
	assoc := "associations.contact"
	if c.IsCompany {
		assoc = "associations.company"
	}
	if hasShow(cfg, ShowDeals) {
		res, err := h.search(ctx, "deals", hsSearch{Limit: 5, Properties: []string{"dealname", "amount", "closedate", "deal_currency_code"},
			FilterGroups: []hsGroup{{Filters: []hsFilter{{assoc, "EQ", c.ID}, {"hs_is_closed", "EQ", "false"}}}}})
		if err != nil {
			return err
		}
		for _, o := range res {
			v := o.Properties["amount"]
			if v != "" {
				v = strings.TrimSpace(o.Properties["deal_currency_code"] + " " + v)
			}
			c.Deals = append(c.Deals, Item{Title: o.Properties["dealname"], Value: v, When: hsDate(o.Properties["closedate"]), URL: h.recordURL(ctx, "0-3", o.ID)})
		}
	}
	if hasShow(cfg, ShowTickets) {
		res, err := h.search(ctx, "tickets", hsSearch{Limit: 5, Properties: []string{"subject", "createdate"},
			FilterGroups: []hsGroup{{Filters: []hsFilter{{assoc, "EQ", c.ID}, {PropertyName: "closed_date", Operator: "NOT_HAS_PROPERTY"}}}}})
		if err != nil {
			return err
		}
		for _, o := range res {
			c.Tickets = append(c.Tickets, Item{Title: "#" + o.ID + " · " + o.Properties["subject"], When: hsDate(o.Properties["createdate"]), URL: h.recordURL(ctx, "0-5", o.ID)})
		}
	}
	return nil
}

type hsAssoc struct {
	To    map[string]string `json:"to"`
	Types []hsAssocType     `json:"types"`
}

type hsAssocType struct {
	Category string `json:"associationCategory"`
	TypeID   int    `json:"associationTypeId"`
}

func hsAssocs(c Contact, toContact, toCompany int) []hsAssoc {
	var a []hsAssoc
	if c.ID != "" && !c.IsCompany {
		a = append(a, hsAssoc{To: map[string]string{"id": c.ID}, Types: []hsAssocType{{"HUBSPOT_DEFINED", toContact}}})
	}
	if c.CompanyID != "" {
		a = append(a, hsAssoc{To: map[string]string{"id": c.CompanyID}, Types: []hsAssocType{{"HUBSPOT_DEFINED", toCompany}}})
	}
	return a
}

func hsHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\n", "<br>")
	return r.Replace(s)
}

func (h *hubspot) LogCall(ctx context.Context, c Call) (string, error) {
	dir, status := "INBOUND", "COMPLETED"
	from, to := c.Number, c.Line
	if c.Direction == "outbound" {
		dir, from, to = "OUTBOUND", c.Line, c.Number
	}
	if !c.Answered {
		status = "NO_ANSWER"
	}
	props := map[string]string{
		"hs_timestamp": c.Start.UTC().Format(time.RFC3339), "hs_call_title": c.Title(), "hs_call_body": hsHTML(c.Body),
		"hs_call_duration": strconv.Itoa(c.TalkSec * 1000), "hs_call_direction": dir, "hs_call_status": status,
		"hs_call_from_number": from, "hs_call_to_number": to,
	}
	if c.OwnerID != "" {
		props["hubspot_owner_id"] = c.OwnerID
	}
	var res hsObject
	err := h.do(ctx, http.MethodPost, "/crm/v3/objects/calls", map[string]any{"properties": props, "associations": hsAssocs(c.Contact, hsCallToContact, hsCallToCompany)}, &res)
	return res.ID, err
}

func (h *hubspot) AddNote(ctx context.Context, id string, c Call, text string) error {
	body := strings.TrimSpace(c.Body + "\n\n" + text)
	return h.do(ctx, http.MethodPatch, "/crm/v3/objects/calls/"+id, map[string]any{"properties": map[string]string{"hs_call_body": hsHTML(body)}}, nil)
}

func (h *hubspot) CreateTask(ctx context.Context, t Task) (string, error) {
	props := map[string]string{"hs_timestamp": t.Due.UTC().Format(time.RFC3339), "hs_task_subject": t.Subject, "hs_task_body": hsHTML(t.Body),
		"hs_task_status": "NOT_STARTED", "hs_task_type": "CALL"}
	if t.OwnerID != "" {
		props["hubspot_owner_id"] = t.OwnerID
	}
	var res hsObject
	err := h.do(ctx, http.MethodPost, "/crm/v3/objects/tasks", map[string]any{"properties": props, "associations": hsAssocs(t.Contact, hsTaskToContact, hsTaskToCompany)}, &res)
	return res.ID, err
}

func (h *hubspot) CreateContact(ctx context.Context, name, company, phone string) (Contact, error) {
	first, last := splitName(name)
	var res hsObject
	err := h.do(ctx, http.MethodPost, "/crm/v3/objects/contacts", map[string]any{"properties": map[string]string{
		"firstname": first, "lastname": last, "company": company, "phone": phone}}, &res)
	if err != nil {
		return Contact{}, err
	}
	return Contact{CRM: HubSpot, ID: res.ID, Name: name, Company: company, Phone: phone, URL: h.recordURL(ctx, "0-1", res.ID)}, nil
}

func (h *hubspot) Owners(ctx context.Context) ([]Owner, error) {
	var res struct {
		Results []struct {
			ID        string `json:"id"`
			Email     string `json:"email"`
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"results"`
	}
	if err := h.do(ctx, http.MethodGet, "/crm/v3/owners?limit=500", nil, &res); err != nil {
		return nil, err
	}
	out := make([]Owner, 0, len(res.Results))
	for _, o := range res.Results {
		out = append(out, Owner{ID: o.ID, Email: o.Email, Name: strings.TrimSpace(o.FirstName + " " + o.LastName)})
	}
	return out, nil
}

func hasShow(cfg *Config, f string) bool {
	for _, s := range cfg.Show {
		if s == f {
			return true
		}
	}
	return false
}

// splitName splits "Jan de Vries" into "Jan" and "de Vries".
func splitName(name string) (string, string) {
	name = strings.TrimSpace(name)
	first, last, ok := strings.Cut(name, " ")
	if !ok {
		return name, ""
	}
	return first, strings.TrimSpace(last)
}
