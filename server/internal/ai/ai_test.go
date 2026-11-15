package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	s, err := parse("Here you go:\n```json\n{\"summary\":\" Jan belt over offerte. \",\"actions\":[\"Offerte sturen\",\"  \"],\"topic\":\"Offerte\",\"sentiment\":\"happy\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if s.Summary != "Jan belt over offerte." || len(s.Actions) != 1 || s.Sentiment != "neutral" || s.Topic != "Offerte" {
		t.Fatalf("%+v", s)
	}
	if _, err := parse("no json"); err == nil {
		t.Fatal("want error")
	}
	if _, err := parse(`{"summary":""}`); err == nil {
		t.Fatal("want error for empty summary")
	}
}

func TestValidate(t *testing.T) {
	st := Defaults()
	if err := st.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []func(*Settings){
		func(s *Settings) { s.Engine = "x" },
		func(s *Settings) { s.LLM = "x" },
		func(s *Settings) { s.CloudModel = "gpt" },
		func(s *Settings) { s.Language = "de" },
		func(s *Settings) { s.KeepDays = -1 },
	} {
		s := Defaults()
		bad(&s)
		if s.Validate() == nil {
			t.Fatalf("want error for %+v", s)
		}
	}
}

func TestCloud(t *testing.T) {
	var got map[string]any
	var beta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		beta = r.Header.Get("anthropic-beta")
		if r.Header.Get("x-api-key") != "sk-ant-test" && r.Header.Get("Authorization") != "Bearer sk-ant-test" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"end_turn",
"content":[{"type":"text","text":"{\"summary\":\"Caller asks for a quote.\",\"actions\":[\"Send quote\"],\"topic\":\"Quote\",\"sentiment\":\"positive\"}"}],
"usage":{"input_tokens":10,"output_tokens":20}}`)
	}))
	defer srv.Close()
	st := Defaults()
	st.Engine = EngineCloud
	z := &Summarizer{APIKey: "sk-ant-test", BaseURL: srv.URL}
	sum, engine, err := z.Summarize(context.Background(), st, KindCall, "Hello, I would like a quote.")
	if err != nil {
		t.Fatal(err)
	}
	if engine != "claude-opus-5-5" || sum.Summary != "Caller asks for a quote." || sum.Sentiment != "positive" {
		t.Fatalf("%s %+v", engine, sum)
	}
	if got["model"] != "claude-opus-5-5" || !strings.Contains(beta, "fallback") {
		t.Fatalf("request %v beta %q", got["model"], beta)
	}
	if _, ok := got["fallbacks"]; !ok {
		t.Fatal("no fallbacks in request")
	}
	z.APIKey = ""
	if _, _, err := z.Summarize(context.Background(), st, KindCall, "x"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
	z.APIKey = "sk-ant-bad"
	if _, _, err := z.Summarize(context.Background(), st, KindCall, "x"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401, got %v", err)
	}
}

func TestDetect(t *testing.T) {
	for text, want := range map[string]string{
		"Goedemorgen, met Mark. Kunt u mij terugbellen? Dank u wel.": "nl",
		"Hi, this is Sarah. Could you send me the invoice? Thanks.":  "en",
		"Okay.": "auto",
	} {
		if got := Detect(text); got != want {
			t.Errorf("%q: %s, want %s", text, got, want)
		}
	}
}
