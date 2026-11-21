package crm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
)

func TestParseNumber(t *testing.T) {
	for _, raw := range []string{"0612345678", "+31612345678", "0031612345678", "31612345678", "+31 6 1234 5678"} {
		n, ok := ParseNumber(raw, "31")
		if !ok || n.Key != "612345678" || n.E164 != "+31612345678" {
			t.Errorf("%s: %+v", raw, n)
		}
		for _, stored := range []string{"06-12345678", "+31 (0)6 12 34 56 78", "0031612345678"} {
			if !n.Matches(stored) {
				t.Errorf("%s should match %s", raw, stored)
			}
		}
		if n.Matches("0612345679") || n.Matches("") {
			t.Errorf("%s matched a wrong number", raw)
		}
	}
	if _, ok := ParseNumber("201", "31"); ok {
		t.Error("an extension is not looked up")
	}
}

func TestSummarize(t *testing.T) {
	in := []cdr.Record{
		{UniqueID: "1", LinkedID: "1", Start: "2026-11-21T10:00:00Z", Src: "0612345678", Dst: "0201234567", SrcChannel: "PJSIP/trunk-1-0001", DstChannel: "Local/600@kiestoon-q-1", Disposition: "ANSWERED", BillSec: 90},
		{UniqueID: "2", LinkedID: "1", Start: "2026-11-21T10:00:05Z", SrcChannel: "Local/201@kiestoon-ring-1;2", DstChannel: "PJSIP/202-0002", Disposition: "NO ANSWER"},
		{UniqueID: "3", LinkedID: "1", Start: "2026-11-21T10:00:05Z", SrcChannel: "Local/201@kiestoon-ring-1;2", DstChannel: "PJSIP/201-web-0003", Disposition: "ANSWERED", BillSec: 80},
	}
	s, ok := Summarize(in)
	if !ok || s.Direction != "inbound" || s.Number != "0612345678" || s.Line != "0201234567" || s.Ext != "201" || !s.Answered || s.TalkSec != 80 {
		t.Fatalf("inbound: %+v", s)
	}
	s, ok = Summarize(in[:2])
	if !ok || s.Answered || s.Ext != "" {
		t.Fatalf("missed: %+v", s)
	}
	out := []cdr.Record{{UniqueID: "9", LinkedID: "9", Start: "2026-11-21T10:00:00Z", Src: "201", Dst: "0612345678", SrcChannel: "PJSIP/201-0009", DstChannel: "PJSIP/trunk-1-000a", BillSec: 30, Disposition: "ANSWERED"}}
	s, ok = Summarize(out)
	if !ok || s.Direction != "outbound" || s.Number != "0612345678" || s.Ext != "201" || s.TalkSec != 30 {
		t.Fatalf("outbound: %+v", s)
	}
	if _, ok := Summarize([]cdr.Record{{LinkedID: "5", SrcChannel: "PJSIP/201-1", DstChannel: "PJSIP/202-2"}}); ok {
		t.Fatal("internal calls are not logged")
	}
	if EndpointExt("PJSIP/trunk-2-0001") != "" || EndpointExt("PJSIP/201-mobile-00a") != "201" || EndpointExt("PJSIP/abc-1") != "" {
		t.Fatal("EndpointExt")
	}
}

// fakeCRM records requests and answers from a table of path prefixes.
type fakeCRM struct {
	mu   sync.Mutex
	reqs []string
	body map[string]string
	ans  func(method, path string, body []byte) (int, string)
}

func (f *fakeCRM) server(t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		key := r.Method + " " + r.URL.Path
		f.reqs = append(f.reqs, key+"?"+r.URL.RawQuery)
		if f.body == nil {
			f.body = map[string]string{}
		}
		f.body[key] = string(b)
		f.mu.Unlock()
		code, out := f.ans(r.Method, r.URL.Path+"?"+r.URL.RawQuery, b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, out)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (f *fakeCRM) sent(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.body[key]
}

func TestHubSpot(t *testing.T) {
	f := &fakeCRM{ans: func(method, path string, body []byte) (int, string) {
		switch {
		case strings.HasPrefix(path, "/account-info/v3/details"):
			return 200, `{"portalId":24871093,"uiDomain":"app-eu1.hubspot.com"}`
		case strings.HasPrefix(path, "/crm/v3/objects/contacts/search"):
			return 200, `{"results":[{"id":"51","properties":{"firstname":"Jan","lastname":"de Vries","company":"Bakkerij De Vries","phone":"+31 6 12345678","hubspot_owner_id":"7","notes_last_contacted":"2026-11-18T10:00:00Z"}},
				{"id":"52","properties":{"firstname":"Fout","phone":"+31 6 99999999"}}]}`
		case strings.HasPrefix(path, "/crm/v3/objects/deals/search"):
			return 200, `{"results":[{"id":"900","properties":{"dealname":"Jaaronderhoud 2027","amount":"4800","deal_currency_code":"EUR"}}]}`
		case strings.HasPrefix(path, "/crm/v3/objects/tickets/search"):
			return 200, `{"results":[{"id":"4821","properties":{"subject":"Toestel herstart steeds"}}]}`
		case strings.HasPrefix(path, "/crm/v3/owners"):
			return 200, `{"results":[{"id":"7","email":"reinder@example.com","firstName":"Reinder","lastName":"H"}]}`
		case path == "/crm/v3/objects/calls?" && method == "POST":
			return 201, `{"id":"call-1"}`
		case strings.HasPrefix(path, "/crm/v3/objects/calls/call-1") && method == "PATCH":
			return 200, `{"id":"call-1"}`
		case strings.HasPrefix(path, "/crm/v3/objects/tasks"):
			return 201, `{"id":"task-1"}`
		}
		return 404, `{}`
	}}
	ts := f.server(t)
	old := HubSpotBase
	HubSpotBase = ts.URL
	t.Cleanup(func() { HubSpotBase = old })

	svc := NewService()
	st := Defaults()
	st.Connectors[HubSpot].Enabled = true
	svc.Reload(st, map[string]Secret{HubSpot: {Token: "pat-1"}})
	if !svc.Active() {
		t.Fatal("not active")
	}
	n, _ := ParseNumber("0612345678", "31")
	res, err := svc.Lookup(context.Background(), n)
	if err != nil || !res.Found() || len(res.Contacts) != 1 {
		t.Fatalf("lookup: %v %+v", err, res)
	}
	c := res.First()
	if c.Name != "Jan de Vries" || c.Company != "Bakkerij De Vries" || c.Owner != "Reinder H" || len(c.Deals) != 1 || c.Deals[0].Value != "EUR 4800" ||
		len(c.Tickets) != 1 || c.Last == nil || !strings.Contains(c.URL, "app-eu1.hubspot.com/contacts/24871093/record/0-1/51") {
		t.Fatalf("contact: %+v", c)
	}
	if c.DisplayName() != "Jan de Vries - Bakkerij De Vries" {
		t.Fatal(c.DisplayName())
	}
	if !strings.Contains(f.sent("POST /crm/v3/objects/contacts/search"), `"hs_searchable_calculated_phone_number","operator":"EQ","value":"612345678"`) {
		t.Fatal("search filter:", f.sent("POST /crm/v3/objects/contacts/search"))
	}
	// Second look-up comes from the cache.
	before := len(f.reqs)
	if _, err := svc.Lookup(context.Background(), n); err != nil || len(f.reqs) != before {
		t.Fatal("not cached")
	}
	if got := svc.OwnerFor(context.Background(), HubSpot, "3", "REINDER@example.com"); got != "7" {
		t.Fatal("owner by e-mail:", got)
	}
	conn, _ := svc.Connector(HubSpot)
	id, err := conn.LogCall(context.Background(), Call{Contact: c, Direction: "inbound", Answered: true, Number: "0612345678", Line: "0201234567",
		Start: time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC), TalkSec: 402, OwnerID: "7", Body: "Answered by Sanne"})
	if err != nil || id != "call-1" {
		t.Fatal(id, err)
	}
	var sent struct {
		Properties   map[string]string `json:"properties"`
		Associations []hsAssoc         `json:"associations"`
	}
	_ = json.Unmarshal([]byte(f.sent("POST /crm/v3/objects/calls")), &sent)
	if sent.Properties["hs_call_duration"] != "402000" || sent.Properties["hs_call_direction"] != "INBOUND" || sent.Properties["hubspot_owner_id"] != "7" ||
		len(sent.Associations) != 1 || sent.Associations[0].Types[0].TypeID != hsCallToContact {
		t.Fatalf("call: %+v", sent)
	}
	if err := conn.AddNote(context.Background(), "call-1", Call{Body: "Answered by Sanne"}, "Firmware <86>"); err != nil {
		t.Fatal(err)
	}
	var patch struct {
		Properties map[string]string `json:"properties"`
	}
	_ = json.Unmarshal([]byte(f.sent("PATCH /crm/v3/objects/calls/call-1")), &patch)
	if patch.Properties["hs_call_body"] != "Answered by Sanne<br><br>Firmware &lt;86&gt;" {
		t.Fatal("note:", patch.Properties)
	}
}

func TestLookupOrderAndExpiredSignIn(t *testing.T) {
	var saved []Secret
	hs := &fakeCRM{ans: func(method, path string, body []byte) (int, string) { return 401, `{"message":"expired"}` }}
	hts := hs.server(t)
	pd := &fakeCRM{ans: func(method, path string, body []byte) (int, string) {
		if strings.HasPrefix(path, "/api/v1/persons/search") {
			return 200, `{"data":{"items":[{"item":{"id":12,"name":"Pieter Bos","phones":["06 12 34 56 78"],"organization":{"id":3,"name":"Bos Installatie"},"owner":{"id":5}}}]}}`
		}
		if strings.HasPrefix(path, "/api/v1/persons/12/deals") {
			return 200, `{"data":[{"id":77,"title":"Nieuwe centrale","value":1200,"currency":"EUR"}]}`
		}
		if strings.HasPrefix(path, "/api/v1/users") {
			return 200, `{"data":[{"id":5,"name":"Lisa","email":"lisa@example.com","active_flag":true}]}`
		}
		return 200, `{"data":{}}`
	}}
	pts := pd.server(t)
	old := HubSpotBase
	HubSpotBase = hts.URL
	t.Cleanup(func() { HubSpotBase = old })

	svc := NewService()
	svc.SaveSecret = func(kind string, s Secret) { saved = append(saved, s) }
	st := Defaults()
	st.Order = []string{HubSpot, Pipedrive}
	st.Connectors[HubSpot].Enabled = true
	st.Connectors[Pipedrive].Enabled = true
	st.Connectors[Pipedrive].Domain = "acme"
	svc.Reload(st, map[string]Secret{HubSpot: {Token: "x"}, Pipedrive: {Token: "y"}})
	// Point Pipedrive at the fake server.
	svc.mu.Lock()
	svc.conns[Pipedrive].(*pipedrive).base = pts.URL
	svc.mu.Unlock()

	n, _ := ParseNumber("+31612345678", "31")
	res, err := svc.Lookup(context.Background(), n)
	if err != nil || res.CRM != Pipedrive || res.First().Name != "Pieter Bos" || res.First().Company != "Bos Installatie" || res.First().Owner != "Lisa" || len(res.First().Deals) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if len(saved) != 1 || !saved[0].LoginExpired {
		t.Fatalf("HubSpot sign-in should be marked expired: %+v", saved)
	}
	// Expired HubSpot is skipped from now on.
	svc.Forget(n)
	before := len(hs.reqs)
	_, _ = svc.Lookup(context.Background(), n)
	if len(hs.reqs) != before {
		t.Fatal("expired CRM was asked again")
	}
}

func TestSalesforceTokenRenewal(t *testing.T) {
	tokens := 0
	var ts *httptest.Server
	f := &fakeCRM{ans: func(method, path string, body []byte) (int, string) {
		switch {
		case strings.HasPrefix(path, "/services/oauth2/token"):
			tokens++
			if !strings.Contains(string(body), "grant_type=client_credentials") {
				return 400, `{"error":"unsupported_grant_type"}`
			}
			return 200, `{"access_token":"tok` + string(rune('0'+tokens)) + `","instance_url":"` + ts.URL + `"}`
		case strings.HasPrefix(path, "/services/data/"+sfVersion+"/search/"):
			if tokens < 2 {
				return 401, `[{"errorCode":"INVALID_SESSION_ID"}]`
			}
			return 200, `{"searchRecords":[{"attributes":{"type":"Contact"},"Id":"0031x00000AbCdEAAV","Name":"Gerda Smit","Phone":"+31 6 1234 5678","AccountId":"0011x00000AbCdEAAV","Account":{"Name":"Smit Transport"},"Owner":{"Name":"Reinder"}}]}`
		case strings.HasPrefix(path, "/services/data/"+sfVersion+"/query"):
			return 200, `{"records":[]}`
		case strings.HasPrefix(path, "/services/data/"+sfVersion+"/sobjects/Task"):
			return 201, `{"id":"00T1x00000AbCdEAAV","success":true}`
		}
		return 404, `{}`
	}}
	ts = f.server(t)
	cfg := DefaultConfig(Salesforce)
	cfg.Domain, cfg.ClientID = ts.URL, "key"
	var saved Secret
	c, _ := New(Salesforce, cfg, Secret{ClientSecret: "s"}, func(s Secret) { saved = s })
	n, _ := ParseNumber("0612345678", "31")
	got, err := c.Lookup(context.Background(), n, cfg)
	if err != nil || len(got) != 1 || got[0].Company != "Smit Transport" || got[0].Owner != "Reinder" || !strings.HasSuffix(got[0].URL, "/lightning/r/Contact/0031x00000AbCdEAAV/view") {
		t.Fatalf("%v %+v", err, got)
	}
	if tokens != 2 || saved.AccessToken != "tok2" {
		t.Fatalf("token renewal: %d %+v", tokens, saved)
	}
	id, err := c.LogCall(context.Background(), Call{Contact: got[0], Direction: "outbound", Answered: true, Number: "0612345678", Start: time.Now(), TalkSec: 30})
	if err != nil || id != "00T1x00000AbCdEAAV" {
		t.Fatal(id, err)
	}
	body := f.sent("POST /services/data/" + sfVersion + "/sobjects/Task")
	if !strings.Contains(body, `"WhoId":"0031x00000AbCdEAAV"`) || !strings.Contains(body, `"CallType":"Outbound"`) {
		t.Fatal(body)
	}
}

func TestExactIndexAndRefresh(t *testing.T) {
	refreshes := 0
	var ts *httptest.Server
	f := &fakeCRM{ans: func(method, path string, body []byte) (int, string) {
		switch {
		case strings.HasPrefix(path, "/api/oauth2/token"):
			refreshes++
			if !strings.Contains(string(body), "refresh_token=r") {
				return 400, `{"error":"invalid_grant"}`
			}
			return 200, `{"access_token":"a","refresh_token":"r` + strings.Repeat("x", refreshes) + `","expires_in":"600"}`
		case strings.HasPrefix(path, "/api/v1/current/Me"):
			return 200, `{"d":{"results":[{"CurrentDivision":1234,"FullName":"Reinder"}]}}`
		case strings.HasPrefix(path, "/api/v1/1234/bulk/CRM/Contacts"):
			if strings.Contains(path, "page2") {
				return 200, `{"d":{"results":[{"ID":"11111111-2222-3333-4444-555555555555","FullName":"Martine Kok","Mobile":"06-12345678","Account":"aaaaaaaa-2222-3333-4444-555555555555","AccountName":"Kok & Zn"}]}}`
			}
			return 200, `{"d":{"results":[{"ID":"x","FullName":"Ander","Phone":"020 7654321"}],"__next":"` + ts.URL + `/api/v1/1234/bulk/CRM/Contacts?page2"}}`
		case strings.HasPrefix(path, "/api/v1/1234/bulk/CRM/Accounts"):
			return 200, `{"d":{"results":[{"ID":"aaaaaaaa-2222-3333-4444-555555555555","Name":"Kok & Zn","Phone":"+31 20 1234567"}]}}`
		case strings.HasPrefix(path, "/api/v1/1234/activities/CommunicationNotes"):
			return 201, `{"d":{"ID":"bbbbbbbb-2222-3333-4444-555555555555"}}`
		}
		return 404, `{}`
	}}
	ts = f.server(t)
	exactHostOverride = ts.URL
	t.Cleanup(func() { exactHostOverride = "" })
	cfg := DefaultConfig(Exact)
	cfg.ClientID = "cid"
	var saved Secret
	c, _ := New(Exact, cfg, Secret{ClientSecret: "s", RefreshToken: "r"}, func(s Secret) { saved = s })
	n, _ := ParseNumber("+31612345678", "31")
	got, err := c.Lookup(context.Background(), n, cfg)
	if err != nil || len(got) != 1 || got[0].Name != "Martine Kok" || got[0].Company != "Kok & Zn" {
		t.Fatalf("%v %+v", err, got)
	}
	if refreshes != 1 || saved.RefreshToken != "rx" {
		t.Fatalf("refresh token not saved: %+v", saved)
	}
	// The company number finds the account.
	n2, _ := ParseNumber("0201234567", "31")
	got, _ = c.Lookup(context.Background(), n2, cfg)
	if len(got) != 1 || !got[0].IsCompany {
		t.Fatalf("%+v", got)
	}
	id, err := c.LogCall(context.Background(), Call{Contact: got[0], Direction: "inbound", Number: "0201234567", Start: time.Now()})
	if err != nil || id != "bbbbbbbb-2222-3333-4444-555555555555" {
		t.Fatal(id, err)
	}
	if !strings.Contains(f.sent("POST /api/v1/1234/activities/CommunicationNotes"), `"Account":"aaaaaaaa-2222-3333-4444-555555555555"`) {
		t.Fatal(f.sent("POST /api/v1/1234/activities/CommunicationNotes"))
	}
}

func TestSettingsValidate(t *testing.T) {
	st := Defaults()
	st.Connectors[Pipedrive].Enabled = true
	st.Connectors[Pipedrive].Domain = "https://acme.pipedrive.com/"
	if err := st.Validate(); err != nil || st.Connectors[Pipedrive].Domain != "acme" {
		t.Fatal(err, st.Connectors[Pipedrive].Domain)
	}
	st.Connectors[Salesforce].Enabled = true
	st.Connectors[Salesforce].Domain = "evil.example.com"
	if err := st.Validate(); err == nil || err.Error() != "salesforce.domain" {
		t.Fatal(err)
	}
	st.Connectors[Salesforce].Domain = "acme.my.salesforce.com"
	st.Connectors[Salesforce].ClientID = "k"
	if err := st.Validate(); err != nil || st.Connectors[Salesforce].Domain != "https://acme.my.salesforce.com" {
		t.Fatal(err)
	}
	st.Connectors[HubSpot].LogWhich = "sometimes"
	if err := st.Validate(); err == nil || err.Error() != "hubspot.logWhich" {
		t.Fatal(err)
	}
}
