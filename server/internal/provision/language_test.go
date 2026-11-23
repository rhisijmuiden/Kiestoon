package provision

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestPhonesFollowCallerLanguage(t *testing.T) {
	for _, c := range []struct{ set, system, want string }{
		{LangSystem, "nl", "nl"}, {LangSystem, "en", "en"}, {"", "nl", "nl"}, {"en", "nl", "en"}, {"nl", "en", "nl"},
	} {
		if got := (Settings{Language: c.set}).WithSystemLanguage(c.system).Lang(); got != c.want {
			t.Errorf("Lang(%q, system %q) = %q, want %q", c.set, c.system, got, c.want)
		}
	}
	ok := Settings{Username: "phone", Password: "longenough1234", AdminPassword: "adminpw12", NTPServer: "pool.ntp.org", Language: LangSystem}
	if err := ok.Validate(); err != nil {
		t.Fatalf("system language rejected: %v", err)
	}
	ext := &pbx.Extension{Number: "201", Name: "Anna", SIPSecret: "s3cret", Transport: "udp"}
	in := Input{Settings: ok.WithSystemLanguage("nl"), PBX: pbx.Settings{SIPPort: 5060, TLSPort: 5061}, Ext: ext,
		Server: "pbx.example.com", Org: "Acme", Phone: pbx.Phone{MAC: "0c383eabcdef", Vendor: "poly", Model: "VVX450"}}
	body, _ := Render(in)
	if !strings.Contains(body, "Nederlands_Nederland") {
		t.Errorf("phone does not follow the Dutch caller language:\n%s", body)
	}
}
