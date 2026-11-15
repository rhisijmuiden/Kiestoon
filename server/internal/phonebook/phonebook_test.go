package phonebook

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

type src struct{ required bool }

func (src) Entries(context.Context) ([]pbx.Entry, error) {
	return []pbx.Entry{
		{ID: "ext-201", Name: "Anna de Vries", Work: "201", Extension: true},
		{ID: "c1", Name: "Jan Jansen", Company: "Bakkerij", Work: "0201234567", Mobile: "06-12345678"},
		{ID: "c2", Name: "Piet Pieters", Work: "+31301234567"},
	}, nil
}

func (s src) Credentials(context.Context) (string, string, bool) {
	return "phone", "secretsecret", s.required
}

func freePort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestLDAP(t *testing.T) {
	addr := freePort(t)
	d := &Directory{Source: src{required: true}, Log: slog.Default()}
	if err := d.Start(addr, "", nil); err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	c, err := ldap.DialURL("ldap://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	search := func(filter string) []string {
		res, err := c.Search(ldap.NewSearchRequest(BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, filter,
			[]string{"cn", "telephoneNumber", "mobile"}, nil))
		if err != nil {
			t.Fatalf("%s: %v", filter, err)
		}
		out := []string{}
		for _, e := range res.Entries {
			out = append(out, e.GetAttributeValue("cn"))
		}
		return out
	}
	if _, err := c.Search(ldap.NewSearchRequest(BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, "(cn=*)", nil, nil)); err == nil {
		t.Error("search without bind allowed")
	}
	if err := c.Bind("cn=phone,"+BaseDN, "wrong"); err == nil {
		t.Error("wrong password accepted")
	}
	if err := c.Bind("cn=phone,"+BaseDN, "secretsecret"); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"(cn=*jan*)":                             "Jan Jansen",
		"(|(cn=*piet*)(telephoneNumber=*piet*))": "Piet Pieters",
		"(mobile=0612345678)":                    "Jan Jansen",
		"(&(objectClass=person)(cn=Anna*))":      "Anna de Vries",
		"(!(cn=*a*))":                            "Piet Pieters",
		"(objectClass=*)":                        "Anna de Vries,Jan Jansen,Piet Pieters",
	}
	for f, want := range cases {
		if got := strings.Join(search(f), ","); got != want {
			t.Errorf("%s: got %q, want %q", f, got, want)
		}
	}
}

func TestXMLAndCSV(t *testing.T) {
	es, _ := src{}.Entries(context.Background())
	for _, v := range []string{"yealink", "snom", "grandstream", "fanvil"} {
		body, ok := XML(v, es)
		if !ok || !strings.Contains(body, "Jan") || !strings.Contains(body, "0201234567") {
			t.Errorf("%s: %s", v, body)
		}
	}
	if _, ok := XML("poly", es); ok {
		t.Error("poly has no XML phone book")
	}
	cs, err := ReadCSV("naam;bedrijf;werk;mobiel\nJan;Acme;020 123 4567;\n")
	if err != nil || len(cs) != 1 || cs[0].Name != "Jan" || cs[0].Work != "020 123 4567" {
		t.Errorf("%+v %v", cs, err)
	}
	var b strings.Builder
	_ = WriteCSV(&b, []pbx.Contact{{Name: "=cmd", Work: "+31201234567"}})
	if !strings.Contains(b.String(), "'=cmd") || !strings.Contains(b.String(), "+31201234567") {
		t.Error(b.String())
	}
}
