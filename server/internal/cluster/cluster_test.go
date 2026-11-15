package cluster

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func TestAllowed(t *testing.T) {
	for p, want := range map[string]bool{
		"asterisk/spool/voicemail/default/101/INBOX/msg0000.wav": true,
		"pbx/tls/kiestoon.key": true,
		"pbx/extensions.conf":  false,
		"kiestoon.db":          false,
		"../etc/passwd":        false,
		"fax/../kiestoon.db":   false,
		"/etc/shadow":          false,
		"firmware/x.rom.tmp":   false,
		"cluster/node.json":    false,
	} {
		if Allowed(p) != want {
			t.Errorf("Allowed(%q) = %v", p, !want)
		}
	}
}

func TestKeepalived(t *testing.T) {
	n := Node{Enabled: true, Mode: ModeVIP, Address: "192.168.1.10", VIP: "192.168.1.50/24", RouterID: 51, Priority: 150,
		Secret: "abcdefghijklmnop", Peer: &Peer{Address: "192.168.1.11"}}
	conf, err := Keepalived(n, "eth0")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"nopreempt", "unicast_peer {\n    192.168.1.11", "192.168.1.50/24 dev eth0", "auth_pass abcdefgh", "cluster-notify master"} {
		if !strings.Contains(conf, want) {
			t.Errorf("missing %q", want)
		}
	}
	if _, err := Keepalived(n, "eth0; rm -rf /"); err == nil {
		t.Error("bad interface accepted")
	}
}

func TestReplication(t *testing.T) {
	ctx := context.Background()
	prim, stby := t.TempDir(), t.TempDir()
	dbPath := filepath.Join(prim, "kiestoon.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.SetSetting(ctx, db, "organization", "Primary BV"); err != nil {
		t.Fatal(err)
	}
	vm := filepath.Join(prim, "asterisk/spool/voicemail/default/101/INBOX")
	os.MkdirAll(vm, 0o750)
	os.WriteFile(filepath.Join(vm, "msg0000.wav"), []byte("RIFFfake"), 0o640)
	os.MkdirAll(filepath.Join(prim, "pbx"), 0o750)
	os.WriteFile(filepath.Join(prim, "pbx/extensions.conf"), []byte("generated"), 0o640)
	// A file the primary no longer has must disappear on the standby.
	os.MkdirAll(filepath.Join(stby, "fax"), 0o750)
	os.WriteFile(filepath.Join(stby, "fax/old.pdf"), []byte("x"), 0o640)

	snap := &Snapshotter{DataDir: prim, DBPath: dbPath, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	m1, err := snap.Take(ctx, false)
	if err != nil || m1.Seq != 1 || m1.Epoch != 1 {
		t.Fatalf("%+v %v", m1, err)
	}
	if m, _ := snap.Take(ctx, false); m.Seq != 1 {
		t.Fatal("snapshot without a change")
	}
	var got Heartbeat
	h := &PrimaryHandler{DataDir: prim, Snap: snap, Secret: func() string { return "s3cret" },
		OnHeartbeat: func(hb Heartbeat) PrimaryInfo { got = hb; return PrimaryInfo{Name: "a", Snapshot: snap.Meta()} }}
	srv := httptest.NewServer(h)
	defer srv.Close()

	rep := &Replicator{DataDir: stby, BaseURL: srv.URL, Secret: "s3cret", Name: "b", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := rep.SyncNow(ctx, true); err != nil {
		t.Fatal(err)
	}
	if got.Name != "b" {
		t.Fatal("no heartbeat")
	}
	rdb, err := sql.Open("sqlite", "file:"+ReplicaPath(stby)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var org string
	if err := rdb.QueryRow(`SELECT value FROM settings WHERE key = 'organization'`).Scan(&org); err != nil || org != "Primary BV" {
		t.Fatal(org, err)
	}
	rdb.Close()
	if b, err := os.ReadFile(filepath.Join(stby, "asterisk/spool/voicemail/default/101/INBOX/msg0000.wav")); err != nil || string(b) != "RIFFfake" {
		t.Fatal("voicemail not copied", err)
	}
	if _, err := os.Stat(filepath.Join(stby, "pbx/extensions.conf")); err == nil {
		t.Fatal("generated config copied")
	}
	if _, err := os.Stat(filepath.Join(stby, "fax/old.pdf")); err == nil {
		t.Fatal("stale file kept")
	}

	// A change gives a new snapshot that the standby picks up.
	time.Sleep(10 * time.Millisecond)
	store.SetSetting(ctx, db, "organization", "Changed")
	m2, _ := snap.Take(ctx, false)
	if m2.Seq != 2 {
		t.Fatalf("%+v", m2)
	}
	if err := rep.SyncNow(ctx, false); err != nil {
		t.Fatal(err)
	}
	if rep.Status().Replica.Seq != 2 {
		t.Fatalf("%+v", rep.Status())
	}
	// Wrong secret.
	bad := &Replicator{DataDir: t.TempDir(), BaseURL: srv.URL, Secret: "nope", Log: rep.Log}
	if bad.SyncNow(ctx, true) == nil {
		t.Fatal("wrong secret accepted")
	}
}
