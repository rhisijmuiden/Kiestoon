package provision

import (
	"encoding/xml"
	"reflect"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func TestParse(t *testing.T) {
	cases := map[string]Request{
		"y000000000000.cfg":                   {Vendor: "yealink", Common: true},
		"001565ABCDEF.cfg":                    {MAC: "001565abcdef"},
		"000000000000.cfg":                    {Vendor: "poly", Common: true},
		"kiestoon-0004f2abcdef.cfg":           {Vendor: "poly", MAC: "0004f2abcdef", PolyConfig: true},
		"gigaset/7c2f80abcdef.xml":            {Vendor: "gigaset", MAC: "7c2f80abcdef"},
		"7C2F80ABCDEF.xml":                    {Vendor: "gigaset", MAC: "7c2f80abcdef"},
		"phonebook/yealink.xml":               {Vendor: "yealink", Phonebook: true},
		"phonebook/grandstream/phonebook.xml": {Vendor: "grandstream", Phonebook: true},
		"snom/000413abcdef.xml":               {Vendor: "snom", MAC: "000413abcdef"},
		"snomD785.htm":                        {Vendor: "snom", Model: "D785", Common: true},
		"snomD785-000413ABCDEF.htm":           {Vendor: "snom", Model: "D785", MAC: "000413abcdef"},
		"cfg000b82abcdef.xml":                 {Vendor: "grandstream", MAC: "000b82abcdef", Shared: []string{"grandstream", "htek"}},
		"cfg000b82abcdef":                     {Vendor: "grandstream", MAC: "000b82abcdef", Shared: []string{"grandstream", "htek"}},
		"cisco/00562babcdef.xml":              {Vendor: "cisco", MAC: "00562babcdef"},
		"CP-8841-3PCC.xml":                    {Vendor: "cisco", Common: true},
		"snom/firmware-d785.xml":              {Vendor: "snom", Model: "d785", Firmware: true},
	}
	for in, want := range cases {
		got, ok := Parse(in)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v %v", in, got, ok)
		}
	}
	for _, bad := range []string{"../etc/passwd", "001565abcde.cfg", "snom/../x.xml", ""} {
		if _, ok := Parse(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestVendors(t *testing.T) {
	ext := &pbx.Extension{Number: "201", Name: "Anna <&>", SIPSecret: "s3cret", Transport: "udp"}
	in := Input{Settings: Settings{AdminPassword: "adminpw12", NTPServer: "pool.ntp.org", Language: "nl"},
		PBX: pbx.Settings{SIPPort: 5060, TLSPort: 5061}, Ext: ext, Server: "pbx.example.com", Org: "Acme",
		Phone: pbx.Phone{MAC: "0c383eabcdef", Keys: []pbx.Key{{Type: "blf", Value: "202", Label: "Bob"}, {Type: "speed", Value: "0612345678", Label: "Mob"}}}}
	for vendor, want := range map[string][]string{
		"fanvil":  {"<<VOIP CONFIG FILE>>", "SIP1 Phone Number  :201", "Fkey1 Value        :202@1/ba", "<<END OF FILE>>"},
		"gigaset": {`<param name="SipAccount.0.AuthName" value="201"/>`, `<param name="SipAccount.1.UserName" value="203"/>`, "Anna &lt;&amp;&gt;"},
		"poly":    {`reg.1.address="201"`, `attendant.resourceList.1.address="202"`, `attendant.resourceList.2.type="automata"`, "Nederlands_Nederland"},
	} {
		in.Phone.Vendor = vendor
		in.Phone.Model = map[string]string{"fanvil": "X4U", "gigaset": "N870", "poly": "VVX450"}[vendor]
		in.Lines = []*pbx.Extension{{Number: "203", Name: "Handset 2", SIPSecret: "x"}}
		body, _ := Render(in)
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: missing %q in\n%s", vendor, w, body)
			}
		}
		if vendor != "fanvil" {
			d := xml.NewDecoder(strings.NewReader(body))
			for {
				if _, err := d.Token(); err != nil {
					if err.Error() != "EOF" {
						t.Errorf("%s: not well-formed XML: %v", vendor, err)
					}
					break
				}
			}
		}
	}
	if !strings.Contains(PolyMaster(""), "kiestoon-[PHONE_MAC_ADDRESS].cfg") {
		t.Error("poly master")
	}
	in.Phone.Vendor, in.PhonebookURL = "yealink", "https://u:p@pbx.example.com/prov/phonebook/yealink.xml"
	if body, _ := Render(in); !strings.Contains(body, "remote_phonebook.data.1.url = https://u:p@pbx.example.com/prov/phonebook/yealink.xml") {
		t.Error(body)
	}
}

func TestPhase4Vendors(t *testing.T) {
	ext := &pbx.Extension{Number: "201", Name: "Jan", SIPSecret: "s3cret-secret", Transport: "udp"}
	in := Input{Settings: Settings{AdminPassword: "adminpass", NTPServer: "pool.ntp.org", Language: "nl"},
		PBX: pbx.Settings{SIPPort: 5060, TLSPort: 5061}, Ext: ext, Server: "10.0.0.1", Org: "Acme",
		Phone:    pbx.Phone{MAC: "00562babcdef", Vendor: "cisco", Model: "8841", Keys: []pbx.Key{{Type: "blf", Value: "202", Label: "Piet"}}},
		Firmware: "http://10.0.0.1:8080/prov/fw/abc/sip88xx.loads", FirmwareDir: "http://10.0.0.1:8080/prov/fw/abc/",
		Base: "http://phone:pw@10.0.0.1:8080/prov/"}
	c := Cisco(in)
	for _, want := range []string{"<User_ID_1_ ua=\"na\">201</User_ID_1_>", "<Proxy_1_ ua=\"na\">10.0.0.1:5060</Proxy_1_>",
		"fnc=blf+sd+cp;sub=202@10.0.0.1", "<Upgrade_Rule ua=\"na\">http://10.0.0.1:8080/prov/fw/abc/sip88xx.loads</Upgrade_Rule>"} {
		if !strings.Contains(c, want) {
			t.Errorf("cisco: missing %s", want)
		}
	}
	cc := CiscoCommon(in.Base)
	if !strings.Contains(cc, "[--uid phone --pwd pw]http://10.0.0.1:8080/prov/cisco/$MA.xml") {
		t.Errorf("cisco common: %s", cc)
	}
	in.Phone.Vendor, in.Phone.Keys = "htek", nil
	h := Htek(in)
	if !strings.Contains(h, "<P47>10.0.0.1:5060</P47>") || !strings.Contains(h, "<P192>10.0.0.1:8080/prov/fw/abc</P192>") || !strings.Contains(h, "<P212>1</P212>") {
		t.Errorf("htek: %s", h)
	}
	in.Phone.Vendor = "akuvox"
	if a := Akuvox(in); !strings.Contains(a, "Config.Account1.SIP.Server = 10.0.0.1") || !strings.Contains(a, "Config.Account1.GENERAL.Pwd = s3cret-secret") {
		t.Errorf("akuvox: %s", a)
	}
	in.Phone.Vendor = "yealink"
	if y := Yealink(in); !strings.Contains(y, "static.firmware.url = http://10.0.0.1:8080/prov/fw/abc/sip88xx.loads") {
		t.Errorf("yealink firmware missing")
	}
	in.Phone.Vendor, in.Phone.Model = "snom", "D785"
	if s := Snom(in); !strings.Contains(s, "snom/firmware-d785.xml") {
		t.Errorf("snom firmware pointer missing")
	}
	r, _ := Parse("cfg000b82abcdef.xml")
	if !r.Matches("htek") || !r.Matches("grandstream") || r.Matches("yealink") {
		t.Error("shared cfg<mac>.xml")
	}
	r, _ = Parse("0c1105abcdef.cfg")
	if !r.Matches("akuvox") {
		t.Error("akuvox <mac>.cfg")
	}
}
