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

func TestGuessModel(t *testing.T) {
	cases := []struct {
		file, mac, ua, vendor, model string
	}{
		{"001565abcdef.cfg", "001565abcdef", "Yealink SIP-T54W 96.86.0.70 00:15:65:ab:cd:ef", "yealink", "T54W"},
		{"001565abcdef.cfg", "001565abcdef", "", "", ""},
		{"805ec0abcdef.cfg", "805ec0abcdef", "Yealink SIP-T42S 66.86.0.15 80:5e:c0:ab:cd:ef", "yealink", "T42S"},
		{"805ec0abcdef.cfg", "805ec0abcdef", "Yealink SIP-T42U 108.86.0.60 80:5e:c0:ab:cd:ef", "yealink", "T42U"},
		{"001565abcdef.cfg", "001565abcdef", "Yealink SIP-T42G 29.83.0.120 00:15:65:ab:cd:ef", "yealink", "T42G"},
		{"snom/000413abcdef.xml", "000413abcdef", "Mozilla/4.0 (compatible; snomD785-SIP 10.1.73.16)", "snom", "D785"},
		{"cfg000b82abcdef.xml", "000b82abcdef", "Grandstream Model HW GRP2614 SW 1.0.5.33 DevId 000b82abcdef", "grandstream", "GRP2614"},
		{"cfg000b82abcdef.xml", "000b82abcdef", "", "", ""},
		{"cfg001fc1abcdef.xml", "001fc1abcdef", "Htek UC926U 2.42.6.5.1", "htek", "UC926U"},
		{"0004f2abcdef.cfg", "0004f2abcdef", "FileTransport PolycomVVX-VVX_450-UA/6.4.3.5018", "poly", "VVX450"},
		{"cisco/00562babcdef.xml", "00562babcdef", "Cisco-CP-8851-3PCC/11.3.7", "cisco", "8851"},
		{"7c2f80abcdef.xml", "7c2f80abcdef", "Gigaset N870E IP PRO/83.V2.40.0", "gigaset", "N870E"},
		{"0c383eabcdef.cfg", "0c383eabcdef", "Fanvil X7 2.4.11", "fanvil", "X7"},
	}
	for _, c := range cases {
		req, ok := Parse(c.file)
		if !ok {
			t.Fatalf("parse %s", c.file)
		}
		v, m := GuessModel(req, c.mac, c.ua)
		if v != c.vendor || m != c.model {
			t.Errorf("%q: got %s/%s, want %s/%s", c.ua, v, m, c.vendor, c.model)
		}
	}
}

func TestKeysLinesAndModules(t *testing.T) {
	anna := &pbx.Extension{Number: "201", Name: "Anna", SIPSecret: "s1", Transport: "udp"}
	sales := &pbx.Extension{Number: "250", Name: "Sales", SIPSecret: "s2", Transport: "tls"}
	keys := []pbx.Key{
		{Type: "line", Label: "Anna"}, {Type: "line", Value: "2", Label: "Sales"}, {Type: "blf", Value: "202", Label: "Bob"},
		{Type: "park", Label: "Park"}, {Type: "parkslot", Value: "71", Label: "P1"}, {Type: "intercom", Value: "203", Label: "Ic"},
		{Type: "dnd", Label: "DND"}, {Type: "voicemail", Label: "VM"},
	}
	mods := []pbx.Module{{Type: "EXP50", Keys: []pbx.Key{{Type: "blf", Value: "210", Label: "Kees"}}}}
	in := Input{Settings: Settings{AdminPassword: "adminpw12", NTPServer: "pool.ntp.org", Language: "en"},
		PBX: pbx.Settings{SIPPort: 5060, TLSPort: 5061}, Ext: anna, Lines: []*pbx.Extension{sales}, Server: "pbx.local",
		Phone: pbx.Phone{Vendor: "yealink", Model: "T54W", Keys: keys, Modules: mods, Options: pbx.PhoneOptions{Intercom: pbx.IntercomOptions{Allow: true, Mute: true}}}}
	y := Yealink(in)
	for _, want := range []string{
		"account.2.user_name = 250", "account.2.sip_server.1.port = 5061", "account.2.sip_server.1.transport_type = 2",
		"linekey.2.type = 15\nlinekey.2.line = 2", "linekey.3.type = 16\nlinekey.3.line = 1\nlinekey.3.value = 202\nlinekey.3.extension = **",
		"linekey.4.type = 10\nlinekey.4.line = 1\nlinekey.4.value = 70", "linekey.5.value = 71\nlinekey.5.extension = \n",
		"linekey.6.type = 13\nlinekey.6.line = 1\nlinekey.6.value = *80203", "linekey.7.value = *76", "linekey.8.value = *97",
		"expansion_module.1.key.1.type = 16", "expansion_module.1.key.1.value = 210", "expansion_module.1.key.1.label = Kees",
		"features.intercom.allow = 1", "features.intercom.mute = 1", "features.intercom.tone = 0",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("yealink: missing %q", want)
		}
	}
	in.Phone.Vendor, in.Phone.Model = "snom", "D785"
	in.Phone.Modules = []pbx.Module{{Type: "D7", Keys: []pbx.Key{{Type: "blf", Value: "210", Label: "Kees"}}}}
	sn := Snom(in)
	for _, want := range []string{
		`<user_name idx="2" perm="R">250</user_name>`, `<fkey idx="1" context="2" label="Sales" perm="R">line</fkey>`,
		`<fkey idx="3" context="1" label="Park" perm="R">transfer 70</fkey>`, `<fkey idx="4" context="1" label="P1" perm="R">blf &lt;sip:71@pbx.local&gt;</fkey>`,
		`<fkey idx="24" context="1" label="Kees" perm="R">blf &lt;sip:210@pbx.local&gt;|**</fkey>`, `<answer_after_policy perm="R">idle</answer_after_policy>`,
	} {
		if !strings.Contains(sn, want) {
			t.Errorf("snom: missing %q", want)
		}
	}
	in.Phone.Vendor, in.Phone.Model = "grandstream", "GXP2170"
	in.Phone.Modules = []pbx.Module{{Type: "GBX20", Keys: []pbx.Key{{Type: "blf", Value: "210", Label: "Kees"}}}, {Type: "GBX20", Keys: []pbx.Key{{Type: "speed", Value: "0612345678", Label: "Mob"}}}}
	g := Grandstream(in)
	for _, want := range []string{"<P404>250</P404>", "<P406>s2</P406>", "<P326>9</P326>", "<P6001>1</P6001>", "<P6601>210</P6601>", "<P6041>0</P6041>", "<P6641>0612345678</P6641>", "<P298>1</P298>"} {
		if !strings.Contains(g, want) {
			t.Errorf("grandstream: missing %q", want)
		}
	}
	lim, ok := Limits("yealink", "T54W")
	if !ok || lim.Lines != 16 || lim.Modules["EXP50"].Max != 3 || lim.Modules["EXP40"].Keys != 0 {
		t.Errorf("limits: %+v", lim)
	}
	for model, lines := range map[string]int{"T42G": 3, "T42S": 12, "T42U": 12} {
		lim, ok := Limits("yealink", model)
		if !ok || lim.Keys != 15 || lim.Lines != lines || len(lim.Modules) != 0 {
			t.Errorf("limits %s: %+v", model, lim)
		}
	}
	in.Phone.Model, in.Phone.Modules = "T42U", nil
	in.Phone.Keys = append(make([]pbx.Key, 14), pbx.Key{Type: "blf", Value: "215", Label: "Last"})
	if y := Yealink(in); !strings.Contains(y, "linekey.15.type = 16") || !strings.Contains(y, "linekey.15.value = 215") {
		t.Error("yealink T42U: line key 15 missing")
	}
}

// Intercom auto-answer is pushed to every brand that supports it, and switched off per phone.
func TestIntercomAutoAnswerPerVendor(t *testing.T) {
	ext := &pbx.Extension{Number: "301", Name: "Loading bay", SIPSecret: "s1", Transport: "udp"}
	in := Input{Settings: Settings{AdminPassword: "adminpw12", NTPServer: "pool.ntp.org", Language: "en"},
		PBX: pbx.Settings{SIPPort: 5060}, Ext: ext, Server: "pbx.local"}
	cases := []struct {
		vendor, model string
		render        func(Input) string
		on, off       []string
	}{
		{"snom", "D785", Snom, []string{`<answer_after_policy perm="R">always</answer_after_policy>`}, []string{`<answer_after_policy perm="R">off</answer_after_policy>`}},
		{"fanvil", "X6U", Fanvil, []string{"Enable Intercom    :1", "Enable Intercom Barge:1"}, []string{"Enable Intercom    :0"}},
		{"poly", "VVX450", Poly, []string{`voIpProt.SIP.alertInfo.1.value="info=alert-autoanswer"`, `voIpProt.SIP.alertInfo.1.class="autoAnswer"`}, []string{`voIpProt.SIP.alertInfo.1.class="default"`}},
		{"cisco", "8851", Cisco, []string{`<Auto_Answer_Page ua="na">Yes</Auto_Answer_Page>`}, []string{`<Auto_Answer_Page ua="na">No</Auto_Answer_Page>`}},
		{"htek", "UC926", Htek, []string{"<P298>1</P298>"}, []string{"<P298>0</P298>"}},
		{"akuvox", "R20A", Akuvox, []string{"Config.Account1.AUTO_ANSWER.Enable = 1"}, []string{"Config.Account1.AUTO_ANSWER.Enable = 0"}},
	}
	for _, c := range cases {
		in.Phone = pbx.Phone{Vendor: c.vendor, Model: c.model, Options: pbx.PhoneOptions{Intercom: pbx.IntercomOptions{Allow: true, Barge: true}}}
		on := c.render(in)
		in.Phone.Options.Intercom.Allow = false
		off := c.render(in)
		for _, want := range c.on {
			if !strings.Contains(on, want) {
				t.Errorf("%s on: missing %q", c.vendor, want)
			}
		}
		for _, want := range c.off {
			if !strings.Contains(off, want) {
				t.Errorf("%s off: missing %q", c.vendor, want)
			}
		}
	}
}
