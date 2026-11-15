package asterisk

import (
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestFaxAndCallerID(t *testing.T) {
	s := sample()
	s.Trunks[0].T38 = true
	s.FaxBoxes = []pbx.FaxBox{{ID: 3, Name: "Fax, office", Emails: "fax@example.com", ECM: true, Enabled: true}}
	s.Inbound = append(s.Inbound, pbx.InboundRoute{ID: 9, Name: "Fax", DID: "0201234599", DestinationType: "fax", Destination: "3", Enabled: true})
	s.Contacts = []pbx.Contact{{ID: 1, Name: "Jan (Jansen)", Company: "Acme", Mobile: "0612345678"}}
	s.Phonebook = pbx.DefaultPhonebook()
	files := Generate(s, Options{FaxDir: "/var/lib/kiestoon/asterisk/spool/fax"})
	pj, dp := files["pjsip.conf"], files["extensions.conf"]
	for _, want := range []string{"t38_udptl=yes", "t38_udptl_ec=redundancy"} {
		if !strings.Contains(pj, want) {
			t.Errorf("pjsip.conf misses %q", want)
		}
	}
	for _, want := range []string{"[kiestoon-fax-3]", "ReceiveFAX(${KT_FAXFILE},f)", "Set(FAXOPT(ecm)=yes)", "UserEvent(KiestoonFax,Box: 3,",
		"Goto(kiestoon-fax-3,s,1)", "[kiestoon-cidname]", "exten => +31612345678,1,Set(CALLERID(name)=Jan Jansen - Acme)",
		"exten => 0612345678,1,Set(CALLERID(name)=", "DIALPLAN_EXISTS(kiestoon-cidname"} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	s.Phonebook.CallerIDLookup = false
	if dp := Generate(s, Options{})["extensions.conf"]; strings.Contains(dp, "kiestoon-cidname") {
		t.Error("caller ID lookup while switched off")
	}
}
