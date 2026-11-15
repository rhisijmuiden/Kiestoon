package ztp

import (
	"net/url"
	"testing"
)

// The examples of "Yealink JSON API for RPS Management Platform" (string to sign, section 1.3.4).
func TestSignExamples(t *testing.T) {
	key, secret := "2df23f2d9c255e7138dc603b3847b58a", "d4a4be460a8d43609d8e8a5e7d0d4ad1"
	// The documented Content-MD5 of {"key":"TestServer","skip":0}.
	got := sign(secret, "POST", "KOK7YpXasjJC+MslP+MGWw==", key, "b681e77450a04d22aaffc914a3379561", "1544008291631", "/api/open/v1/server/list", nil)
	if got == "" {
		t.Fatal("empty signature")
	}
	a := sign(secret, "GET", "", key, "9e730a223b48433785494801fb016d39", "1544094691000", "/api/open/v1/device/checkMac", url.Values{"mac": {"001565123123"}})
	b := sign(secret, "GET", "", key, "9e730a223b48433785494801fb016d39", "1544094691000", "/api/open/v1/device/checkMac", url.Values{"mac": {"001565123124"}})
	if a == b {
		t.Fatal("query must be signed")
	}
}

func TestValidate(t *testing.T) {
	s := Settings{ServerURL: "https://pbx.example.com/prov", Yealink: Yealink{Enabled: true, AccessKey: "2df23f2d9c255e7138dc603b3847b58a", Secret: "x"}}
	if err := s.Validate(); err != nil || s.ServerURL != "https://pbx.example.com/prov/" || s.Yealink.API != DefaultYealinkAPI {
		t.Fatalf("%v %+v", err, s)
	}
	s.ServerURL = "https://u:p@pbx.example.com/prov/"
	if s.Validate() == nil {
		t.Fatal("credentials in the URL must be refused")
	}
}
