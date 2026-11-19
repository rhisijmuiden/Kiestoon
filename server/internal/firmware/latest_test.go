package firmware

import "testing"

const gsPage = `<a href="https://firmware.grandstream.com/Release_GRP261x_1.0.13.100.zip">old</a>
<a href="https://firmware.grandstream.com/Release_GRP261x_1.0.13.127.zip">GRP261x</a>
<a href="http://firmware.grandstream.com/Release_GRP26xx_1.0.15.23.zip">GRP26xx</a>
<a href="https://firmware.grandstream.com/Release_GXP2170_1.0.11.106.zip">GXP2170</a>
<a href="https://firmware.grandstream.com/Release_WP810_WP822_WP825_1.0.11.83.zip">WP</a>
<a href="http://firmware.grandstream.com/Release_Note_GRP261x_1.0.13.127.pdf">notes</a>`

func TestParseGrandstream(t *testing.T) {
	rs := ParseGrandstream(gsPage)
	if len(rs) != 4 {
		t.Fatalf("releases: %+v", rs)
	}
	if r := Latest(rs, "grandstream", "GRP2612"); r == nil || r.Version != "1.0.13.127" || r.URL != "https://firmware.grandstream.com/Release_GRP261x_1.0.13.127.zip" {
		t.Fatalf("GRP2612: %+v", r)
	}
	if r := Latest(rs, "grandstream", "GRP2670"); r == nil || r.Version != "1.0.15.23" || r.URL[:8] != "https://" {
		t.Fatalf("GRP2670: %+v", r)
	}
	if r := Latest(rs, "grandstream", "WP822"); r == nil || r.Version != "1.0.11.83" {
		t.Fatalf("WP822: %+v", r)
	}
	if r := Latest(rs, "grandstream", "GXP2160"); r != nil {
		t.Fatalf("GXP2160 should have no match: %+v", r)
	}
	if r := Latest(rs, "yealink", "GXP2170"); r != nil {
		t.Fatal("vendor ignored")
	}
}

func TestMatchModelAndVersions(t *testing.T) {
	for _, c := range []struct {
		p, m string
		want int
	}{{"T42U", "T42U", 0}, {"t42u", "T42U", 0}, {"GRP261x", "GRP2612", 1}, {"GRP26xx", "GRP2612", 2}, {"GRP261x", "GRP2622", -1}, {"T4x", "T42U", -1}, {"T4xU", "T4AU", -1}} {
		if got := MatchModel(c.p, c.m); got != c.want {
			t.Errorf("MatchModel(%s, %s) = %d, want %d", c.p, c.m, got, c.want)
		}
	}
	for ua, want := range map[string]string{
		"Yealink SIP-T42U 108.86.0.45 00:15:65:aa:bb:cc": "108.86.0.45",
		"Grandstream GRP2612 1.0.13.127":                 "1.0.13.127",
		"snomD717/10.1.198.18":                           "10.1.198.18",
		"PolycomVVX-VVX_450-UA/6.4.6.2546":               "6.4.6.2546",
		"Mozilla/5.0":                                    "",
	} {
		if got := VersionFromUserAgent(ua); got != want {
			t.Errorf("VersionFromUserAgent(%q) = %q, want %q", ua, got, want)
		}
	}
	for _, c := range []struct {
		a, b string
		want int
	}{{"108.86.0.45", "108.86.0.60", -1}, {"1.0.13.127", "1.0.13.99", 1}, {"1.0", "1.0.0", 0}, {"96.86.0.70", "108.86.0.45", -1}} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s, %s) = %d", c.a, c.b, got)
		}
	}
}

func TestParseCatalog(t *testing.T) {
	rs, err := ParseCatalog([]byte(`{"firmware":[
		{"vendor":"Yealink","models":["T42U","T43U"],"version":"108.86.0.60","url":"https://example.com/T4xU-108.86.0.60.rom","sha256":""},
		{"vendor":"yealink","models":["T54W"],"version":"96.86.0.70","url":"ftp://example.com/x.rom"},
		{"vendor":"yealink","models":[],"version":"1.0.0","url":"https://example.com/x.rom"},
		{"vendor":"yealink","models":["T46U"],"version":"108.86.0.60","url":"https://example.com/y.rom","sha256":"nope"}
	]}`))
	if err != nil || len(rs) != 1 || rs[0].Vendor != "yealink" || rs[0].Source != "catalog" {
		t.Fatalf("catalog: %v %+v", err, rs)
	}
	if _, err := ParseCatalog([]byte("<html>")); err == nil {
		t.Fatal("bad json accepted")
	}
}
