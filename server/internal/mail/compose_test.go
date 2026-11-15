package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestComposeHTMLWithAttachment(t *testing.T) {
	s := Settings{FromName: "Kiestoon", FromAddress: "pbx@example.com"}
	raw := ComposeHTML(s, "a@example.com", "Weekly", "text", "<p>html</p>", Attachment{Name: "r.csv", ContentType: "text/csv", Data: []byte("a,b\n")})
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if mt != "multipart/mixed" {
		t.Fatalf("type %s", mt)
	}
	r := multipart.NewReader(m.Body, params["boundary"])
	alt, _ := r.NextPart()
	if !strings.HasPrefix(alt.Header.Get("Content-Type"), "multipart/alternative") {
		t.Fatalf("first part %s", alt.Header.Get("Content-Type"))
	}
	_, _ = io.ReadAll(alt)
	att, _ := r.NextPart()
	if att.FileName() != "r.csv" {
		t.Fatalf("attachment %q", att.FileName())
	}
	// Without attachments it stays a plain alternative message.
	m2, _ := mail.ReadMessage(bytes.NewReader(ComposeHTML(s, "a@example.com", "x", "t", "h")))
	if mt, _, _ := mime.ParseMediaType(m2.Header.Get("Content-Type")); mt != "multipart/alternative" {
		t.Fatalf("plain: %s", mt)
	}
}
