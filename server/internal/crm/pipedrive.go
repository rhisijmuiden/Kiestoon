package crm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Pipedrive with the API token of a user (Personal preferences → API) and the company domain.
// Persons are looked up by phone; Pipedrive organisations have no standard phone field.
type pipedrive struct {
	base, token string
}

func (p *pipedrive) do(ctx context.Context, method, path string, in, out any) error {
	return doJSON(ctx, method, p.base+path, http.Header{"X-Api-Token": {p.token}}, in, out)
}

// pdID reads an id that Pipedrive sends as a number or as an object with "value".
type pdID int64

func (i *pdID) UnmarshalJSON(b []byte) error {
	var n int64
	if json.Unmarshal(b, &n) == nil {
		*i = pdID(n)
		return nil
	}
	var o struct {
		Value int64 `json:"value"`
		ID    int64 `json:"id"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return nil
	}
	*i = pdID(max(o.Value, o.ID))
	return nil
}

func (i pdID) String() string {
	if i == 0 {
		return ""
	}
	return strconv.FormatInt(int64(i), 10)
}

func (p *pipedrive) Test(ctx context.Context) (string, error) {
	var res struct {
		Data struct {
			Name        string `json:"name"`
			CompanyName string `json:"company_name"`
		} `json:"data"`
	}
	if err := p.do(ctx, http.MethodGet, "/api/v1/users/me", nil, &res); err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Data.CompanyName + " · " + res.Data.Name), nil
}

func (p *pipedrive) Lookup(ctx context.Context, n Number, cfg *Config) ([]Contact, error) {
	if !cfg.LookupContacts {
		return nil, nil
	}
	var res struct {
		Data struct {
			Items []struct {
				Item struct {
					ID           int64    `json:"id"`
					Name         string   `json:"name"`
					Phones       []string `json:"phones"`
					Emails       []string `json:"emails"`
					Organization *struct {
						ID   int64  `json:"id"`
						Name string `json:"name"`
					} `json:"organization"`
					Owner struct {
						ID int64 `json:"id"`
					} `json:"owner"`
				} `json:"item"`
			} `json:"items"`
		} `json:"data"`
	}
	q := url.Values{"term": {n.Key}, "fields": {"phone"}, "exact_term": {"false"}, "limit": {"10"}}
	if err := p.do(ctx, http.MethodGet, "/api/v1/persons/search?"+q.Encode(), nil, &res); err != nil {
		return nil, err
	}
	var out []Contact
	for _, it := range res.Data.Items {
		x := it.Item
		phone := ""
		for _, ph := range x.Phones {
			if n.Matches(ph) {
				phone = ph
				break
			}
		}
		if phone == "" {
			continue
		}
		c := Contact{CRM: Pipedrive, ID: strconv.FormatInt(x.ID, 10), Name: x.Name, Phone: phone,
			URL: fmt.Sprintf("%s/person/%d", p.base, x.ID)}
		if x.Owner.ID != 0 {
			c.OwnerID = strconv.FormatInt(x.Owner.ID, 10)
		}
		if len(x.Emails) > 0 {
			c.Email = x.Emails[0]
		}
		if x.Organization != nil {
			c.Company, c.CompanyID = x.Organization.Name, strconv.FormatInt(x.Organization.ID, 10)
		}
		out = append(out, c)
	}
	return out, nil
}

func pdDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return ""
}

func (p *pipedrive) Details(ctx context.Context, c *Contact, cfg *Config) error {
	if hasShow(cfg, ShowDeals) {
		var res struct {
			Data []struct {
				ID       int64   `json:"id"`
				Title    string  `json:"title"`
				Value    float64 `json:"value"`
				Currency string  `json:"currency"`
				Close    string  `json:"expected_close_date"`
			} `json:"data"`
		}
		if err := p.do(ctx, http.MethodGet, "/api/v1/persons/"+url.PathEscape(c.ID)+"/deals?status=open&limit=5", nil, &res); err != nil {
			return err
		}
		for _, d := range res.Data {
			v := ""
			if d.Value != 0 {
				v = strings.TrimSpace(d.Currency + " " + strconv.FormatFloat(d.Value, 'f', -1, 64))
			}
			c.Deals = append(c.Deals, Item{Title: d.Title, Value: v, When: pdDate(d.Close), URL: fmt.Sprintf("%s/deal/%d", p.base, d.ID)})
		}
	}
	if hasShow(cfg, ShowLast) {
		var res struct {
			Data struct {
				Last string `json:"last_activity_date"`
			} `json:"data"`
		}
		if err := p.do(ctx, http.MethodGet, "/api/v1/persons/"+url.PathEscape(c.ID), nil, &res); err == nil && res.Data.Last != "" {
			c.Last = &Item{Title: "Last activity", When: pdDate(res.Data.Last)}
		}
	}
	return nil
}

func pdTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

func atoiOrZero(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func (p *pipedrive) LogCall(ctx context.Context, c Call) (string, error) {
	outcome := "connected"
	if !c.Answered {
		outcome = "no_answer"
	}
	from, to := c.Number, c.Line
	if c.Direction == "outbound" {
		from, to = c.Line, c.Number
	}
	body := map[string]any{
		"outcome": outcome, "from_phone_number": from, "to_phone_number": to, "subject": c.Title(), "note": hsHTML(c.Body),
		"start_time": pdTime(c.Start), "end_time": pdTime(c.Start.Add(time.Duration(c.TalkSec) * time.Second)), "duration": strconv.Itoa(c.TalkSec),
	}
	if id := atoiOrZero(c.Contact.ID); id != 0 && !c.Contact.IsCompany {
		body["person_id"] = id
	}
	if id := atoiOrZero(c.Contact.CompanyID); id != 0 {
		body["org_id"] = id
	}
	if id := atoiOrZero(c.OwnerID); id != 0 {
		body["user_id"] = id
	}
	var res struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	err := p.do(ctx, http.MethodPost, "/api/v1/callLogs", body, &res)
	return res.Data.ID, err
}

// AddNote adds a note to the person: Pipedrive call logs cannot be changed.
func (p *pipedrive) AddNote(ctx context.Context, _ string, c Call, text string) error {
	body := map[string]any{"content": hsHTML(text)}
	if id := atoiOrZero(c.Contact.ID); id != 0 && !c.Contact.IsCompany {
		body["person_id"] = id
	}
	if id := atoiOrZero(c.Contact.CompanyID); id != 0 {
		body["org_id"] = id
	}
	return p.do(ctx, http.MethodPost, "/api/v1/notes", body, nil)
}

func (p *pipedrive) CreateTask(ctx context.Context, t Task) (string, error) {
	body := map[string]any{"subject": t.Subject, "type": "call", "due_date": t.Due.Format("2006-01-02"), "note": hsHTML(t.Body)}
	if id := atoiOrZero(t.Contact.ID); id != 0 && !t.Contact.IsCompany {
		body["person_id"] = id
	}
	if id := atoiOrZero(t.Contact.CompanyID); id != 0 {
		body["org_id"] = id
	}
	if id := atoiOrZero(t.OwnerID); id != 0 {
		body["user_id"] = id
	}
	var res struct {
		Data struct {
			ID pdID `json:"id"`
		} `json:"data"`
	}
	err := p.do(ctx, http.MethodPost, "/api/v1/activities", body, &res)
	return res.Data.ID.String(), err
}

func (p *pipedrive) CreateContact(ctx context.Context, name, company, phone string) (Contact, error) {
	body := map[string]any{"name": name, "phone": []map[string]any{{"value": phone, "primary": true, "label": "work"}}}
	c := Contact{CRM: Pipedrive, Name: name, Company: company, Phone: phone}
	if company != "" {
		var org struct {
			Data struct {
				ID pdID `json:"id"`
			} `json:"data"`
		}
		if err := p.do(ctx, http.MethodPost, "/api/v1/organizations", map[string]any{"name": company}, &org); err != nil {
			return Contact{}, err
		}
		body["org_id"] = int64(org.Data.ID)
		c.CompanyID = org.Data.ID.String()
	}
	var res struct {
		Data struct {
			ID pdID `json:"id"`
		} `json:"data"`
	}
	if err := p.do(ctx, http.MethodPost, "/api/v1/persons", body, &res); err != nil {
		return Contact{}, err
	}
	c.ID = res.Data.ID.String()
	c.URL = p.base + "/person/" + c.ID
	return c, nil
}

func (p *pipedrive) Owners(ctx context.Context) ([]Owner, error) {
	var res struct {
		Data []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Email  string `json:"email"`
			Active bool   `json:"active_flag"`
		} `json:"data"`
	}
	if err := p.do(ctx, http.MethodGet, "/api/v1/users", nil, &res); err != nil {
		return nil, err
	}
	var out []Owner
	for _, u := range res.Data {
		if u.Active {
			out = append(out, Owner{ID: strconv.FormatInt(u.ID, 10), Name: u.Name, Email: u.Email})
		}
	}
	return out, nil
}
