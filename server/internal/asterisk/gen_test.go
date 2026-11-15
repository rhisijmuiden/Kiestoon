package asterisk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func sample() *pbx.Snapshot {
	st := pbx.DefaultSettings()
	st.EmergencyCallerID = "0201234567"
	st.ExternalAddress = "203.0.113.10"
	return &pbx.Snapshot{
		Settings: st,
		Extensions: []pbx.Extension{
			{ID: 1, Number: "201", Name: "Anna de Vries", Email: "anna@example.com", SIPSecret: "s3cretS3cretS3cret", Transport: "udp",
				MaxContacts: 3, RingSeconds: 25, Voicemail: true, VoicemailPIN: "1234", VoicemailEmail: true, Enabled: true},
			{ID: 2, Number: "202", Name: "Bas", SIPSecret: "an0therS3cretXYZ", Transport: "tls", SRTP: true, MaxContacts: 1,
				RingSeconds: 20, OutboundCID: "0207654321", Enabled: true},
		},
		Trunks: []pbx.Trunk{
			{ID: 1, Name: "Provider A", AuthMode: "register", Host: "sip.example.net", Port: 5060, Transport: "udp", Username: "31201234567",
				Secret: "trunkSecret;x", DIDSource: "to", Codecs: "alaw,g722", MaxChannels: 4, CallerID: "0201234567", Enabled: true},
			{ID: 2, Name: "Provider B", AuthMode: "ip", Host: "192.0.2.20", Port: 5060, Transport: "udp", MatchIPs: "192.0.2.0/28",
				DIDSource: "ruri", Codecs: "alaw", Enabled: true},
		},
		Inbound: []pbx.InboundRoute{
			{ID: 1, Name: "Main", DID: "0201234567", DestinationType: "extension", Destination: "201", Enabled: true},
			{ID: 2, Name: "Rest", DID: "", DestinationType: "voicemail", Destination: "201", Enabled: true},
		},
		Outbound: []pbx.OutboundRoute{
			{ID: 1, Name: "NL", Patterns: []pbx.DialPattern{{Match: "0[1-9]XXXXXXXX"}, {Prefix: "9", Match: "0[1-9]XXXXXXXX"}}, TrunkIDs: []int64{1, 2}, Enabled: true},
			{ID: 2, Name: "International", Patterns: []pbx.DialPattern{{Match: "00Z."}}, TrunkIDs: []int64{2}, Enabled: true},
		},
	}
}

func TestGenerate(t *testing.T) {
	files := Generate(sample(), Options{AMIUser: "kiestoon", AMISecret: "amisecret", TLSCert: "/tmp/c.crt", TLSKey: "/tmp/c.key",
		MailFrom: "pbx@example.com", MailCmd: "/usr/bin/kiestoon sendmail -t"})
	pj := files["pjsip.conf"]
	for _, want := range []string{"[201]\ntype=endpoint", "media_encryption=sdes", `password=trunkSecret\;x`, "match=192.0.2.0/28",
		"external_media_address=203.0.113.10", "[transport-tls]", "set_var=KT_OUTCID=0207654321"} {
		if !strings.Contains(pj, want) {
			t.Errorf("pjsip.conf misses %q", want)
		}
	}
	dp := files["extensions.conf"]
	for _, want := range []string{"exten => 112,1", "Set(CALLERID(num)=0201234567)", "Dial(PJSIP/112@trunk-1,,T)",
		"include => kiestoon-out-1\ninclude => kiestoon-out-2", "exten => _90[1-9]XXXXXXXX,1,Set(KT_NUM=${EXTEN:1})",
		"exten => +31201234567,1,Goto(kiestoon-in-1,s,1)", "exten => 31201234567,1,Goto(kiestoon-in-1,s,1)",
		"exten => _X.,1,Goto(kiestoon-in-2,s,1)", "GROUP_COUNT(trunk-1@kt_trunks)} >= 4", "VoiceMail(201@default"} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf misses %q", want)
		}
	}
	if strings.Index(dp, "include => kiestoon-emergency") > strings.Index(dp, "include => kiestoon-outbound") {
		t.Error("emergency numbers must come before outbound routes")
	}
	vm := files["voicemail.conf"]
	if !strings.Contains(vm, "201 => 1234,Anna de Vries,anna@example.com,,") || strings.Contains(vm, "202 =>") {
		t.Errorf("voicemail.conf:\n%s", vm)
	}
	if out := os.Getenv("KT_GEN_OUT"); out != "" {
		for n, c := range files {
			_ = os.WriteFile(filepath.Join(out, n), []byte(c), 0o644)
		}
	}
}

func TestChangedModules(t *testing.T) {
	got := ChangedModules([]string{"pjsip.conf", "extensions.conf", "extensions.conf"})
	if len(got) != 2 || got[0] != "dialplan reload" {
		t.Fatalf("got %v", got)
	}
}
