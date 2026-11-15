package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const releasesJSON = `[
 {"tag_name":"v0.2.0-rc.1","prerelease":true},
 {"tag_name":"v0.1.1","prerelease":false},
 {"tag_name":"v0.1.0","prerelease":false},
 {"tag_name":"v0.3.0","draft":true},
 {"tag_name":"nightly","prerelease":true}
]`

func testChecker(t *testing.T) *Checker {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(releasesJSON)) }))
	t.Cleanup(srv.Close)
	c := NewChecker("o/r", t.TempDir())
	c.BaseURL = srv.URL
	return c
}

func TestChannels(t *testing.T) {
	c := testChecker(t)
	ctx := context.Background()
	prod, err := c.Check(ctx, "v0.1.0", ChannelProduction, true)
	if err != nil {
		t.Fatal(err)
	}
	if prod.Latest.Version != "v0.1.1" || !prod.UpdateAvailable {
		t.Fatalf("production: got %+v", prod)
	}
	test, _ := c.Check(ctx, "v0.1.1", ChannelTesting, false)
	if test.Latest.Version != "v0.2.0-rc.1" || !test.UpdateAvailable {
		t.Fatalf("testing: got %+v", test.Latest)
	}
	back, _ := c.Check(ctx, "v0.2.0-rc.1", ChannelProduction, false)
	if !back.Downgrade || back.UpdateAvailable {
		t.Fatalf("expected downgrade, got %+v", back)
	}
	if _, err := c.Find(ctx, "v0.2.0-rc.1", ChannelProduction); err == nil {
		t.Fatal("rc must not be installable on production")
	}
	if _, err := c.Find(ctx, "v0.3.0", ChannelTesting); err == nil {
		t.Fatal("draft must not be installable")
	}
}

func TestRequestUpdate(t *testing.T) {
	c := testChecker(t)
	if c.Status().State != "idle" {
		t.Fatal("expected idle")
	}
	if err := c.RequestUpdate(Request{Version: "v0.1.1"}); err != nil {
		t.Fatal(err)
	}
	if st := c.Status(); st.State != "queued" || st.Version != "v0.1.1" {
		t.Fatalf("got %+v", st)
	}
	if err := c.RequestUpdate(Request{Version: "v0.1.1"}); err != ErrBusy {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
}
