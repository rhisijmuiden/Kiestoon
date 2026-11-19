package firmware

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"T54W-96.86.0.70.rom":             "T54W-96.86.0.70.rom",
		"T54W(T57W,T53W)-96.86.0.70.zip":  "T54W_T57W_T53W_-96.86.0.70.zip",
		"Release Notes.pdf":               "Release_Notes.pdf",
		`C:\Users\x\Downloads\fw 1.rom`:   "fw_1.rom",
		"../../etc/passwd":                "passwd",
		"...":                             "",
		"é.rom":                           "rom",
		strings.Repeat("a", 200) + ".rom": strings.Repeat("a", 117) + ".rom",
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A Yealink bundle: the firmware in a sub folder next to release notes with spaces in the name.
func TestAddVendorZip(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: db, Dir: t.TempDir()}
	rom := bytes.Repeat([]byte{7}, 3<<20)
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	f, _ := zw.Create("T54W(T57W,T53W)-96.86.0.70/T54W(T57W,T53W)-96.86.0.70.rom")
	f.Write(rom)
	g, _ := zw.Create("T54W(T57W,T53W)-96.86.0.70/Release Notes V86.pdf")
	g.Write([]byte("%PDF"))
	zw.Close()
	fw, err := s.Add(context.Background(), "yealink", "T54W(T57W,T53W)-96.86.0.70.zip", &zb)
	if err != nil {
		t.Fatal(err)
	}
	if fw.Filename != "T54W_T57W_T53W_-96.86.0.70.rom" || fw.Size != int64(len(rom)) {
		t.Fatalf("main file: %+v", fw)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, fw.Token, "Release_Notes_V86.pdf")); err != nil {
		t.Fatal("release notes not kept")
	}
}
