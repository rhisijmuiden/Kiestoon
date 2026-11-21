package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rhisijmuiden/kiestoon/server/internal/elevenlabs"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// fakeEleven is a small ElevenLabs API: subscription, voices, library, add voice, text-to-speech.
func fakeEleven(t *testing.T, speaks *atomic.Int32) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/p/") { // previews are public files
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "ID3"+r.URL.Path)
			return
		}
		if r.Header.Get("xi-api-key") != "sk_good_key_3f9a" {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"detail":{"status":"invalid_api_key","message":"Invalid API key"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/user/subscription":
			_, _ = io.WriteString(w, `{"tier":"creator","status":"active","character_count":85000,"character_limit":100000,"next_character_count_reset_unix":1800000000}`)
		case r.URL.Path == "/v2/voices":
			_, _ = io.WriteString(w, `{"voices":[
				{"voice_id":"ruth1","name":"Ruth","category":"professional","labels":{"gender":"female","accent":"dutch"},"preview_url":"`+srv.URL+`/p/ruth.mp3","verified_languages":[{"language":"nl","accent":"standard","preview_url":"`+srv.URL+`/p/ruth-nl.mp3"}]},
				{"voice_id":"grace1","name":"Grace","category":"premade","labels":{"gender":"female","language":"en"},"preview_url":"`+srv.URL+`/p/grace.mp3"},
				{"voice_id":"clone1","name":"My clone","category":"cloned","labels":{}}],"has_more":false}`)
		case r.URL.Path == "/v1/shared-voices":
			if r.URL.Query().Get("language") != "nl" {
				t.Errorf("library language: %q", r.URL.Query().Get("language"))
			}
			_, _ = io.WriteString(w, `{"voices":[{"public_owner_id":"own1","voice_id":"daan1","name":"Daan","gender":"male","accent":"dutch","use_case":"conversational","language":"nl","preview_url":"`+srv.URL+`/p/daan.mp3"}],"has_more":true}`)
		case r.URL.Path == "/v1/voices/add/own1/daan1":
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["new_name"] != "Daan" {
				t.Errorf("add name: %v", b)
			}
			_, _ = io.WriteString(w, `{"voice_id":"daanMine"}`)
		case strings.HasPrefix(r.URL.Path, "/v1/text-to-speech/"):
			speaks.Add(1)
			if r.URL.Query().Get("output_format") != "pcm_16000" {
				t.Errorf("format: %s", r.URL.RawQuery)
			}
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["model_id"] != "eleven_multilingual_v2" || b["text"] == "" {
				t.Errorf("tts body: %v", b)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(make([]byte, 16000)) // half a second of silence
		default:
			w.WriteHeader(404)
		}
	}))
	return srv
}

func TestElevenLabs(t *testing.T) {
	c, s := adminClient(t)
	var speaks atomic.Int32
	api := fakeEleven(t, &speaks)
	defer api.Close()
	t.Setenv("KIESTOON_ELEVENLABS_BASE_URL", api.URL)

	body := mustDo(t, c, "GET", "/api/speech/elevenlabs", nil, 200)
	st := body["settings"].(map[string]any)
	if body["keySet"] != false || st["enabled"] != false || st["model"] != "eleven_multilingual_v2" || body["pbxLanguage"] != "en" {
		t.Fatalf("get: %v", body)
	}
	// Without a key nothing is called.
	if code, b := c.do("GET", "/api/speech/elevenlabs/usage", nil); code != 409 || b["error"] != "elevenlabs_no_key" {
		t.Fatalf("no key: %d %v", code, b)
	}
	// A wrong key is refused by ElevenLabs and not stored.
	if code, b := c.do("PUT", "/api/speech/elevenlabs/key", map[string]string{"key": "sk_wrong"}); code != 502 || b["error"] != "elevenlabs_key" {
		t.Fatalf("wrong key: %d %v", code, b)
	}
	body = mustDo(t, c, "PUT", "/api/speech/elevenlabs/key", map[string]string{"key": "sk_good_key_3f9a"}, 200)
	if body["keySet"] != true || body["keyHint"] != "…3f9a" {
		t.Fatalf("key: %v", body)
	}
	// Sealed in the database, never sent back.
	raw, _ := store.GetSetting(t.Context(), s.DB, elevenlabs.KeyKey, "")
	if raw == "" || strings.Contains(raw, "good_key") {
		t.Fatalf("key not sealed: %q", raw)
	}
	if b, _ := json.Marshal(mustDo(t, c, "GET", "/api/speech/elevenlabs", nil, 200)); strings.Contains(string(b), "good_key") {
		t.Fatal("key leaked")
	}
	u := mustDo(t, c, "GET", "/api/speech/elevenlabs/usage", nil, 200)
	if u["tier"] != "creator" || u["characterLimit"] != float64(100000) {
		t.Fatalf("usage: %v", u)
	}

	// My voices in Dutch: Ruth (verified nl) and the clone without a language, not Grace.
	v := mustDo(t, c, "GET", "/api/speech/elevenlabs/voices?source=mine&language=nl", nil, 200)
	names := []string{}
	for _, x := range v["voices"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "Ruth,My clone" {
		t.Fatalf("mine: %v", names)
	}
	lib := mustDo(t, c, "GET", "/api/speech/elevenlabs/voices?language=nl", nil, 200)
	if lib["hasMore"] != true || lib["voices"].([]any)[0].(map[string]any)["ownerId"] != "own1" {
		t.Fatalf("library: %v", lib)
	}
	if b, _ := json.Marshal(lib); strings.Contains(string(b), "/p/daan") {
		t.Fatal("preview url sent to the browser")
	}
	// Previews play through Kiestoon; the Dutch sample of a multilingual voice when asked.
	res, err := c.hc.Get(c.url + "/api/speech/elevenlabs/voices/ruth1/preview?language=nl")
	if err != nil {
		t.Fatal(err)
	}
	pb, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(pb) != "ID3/p/ruth-nl.mp3" || res.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("preview: %d %q", res.StatusCode, pb)
	}
	added := mustDo(t, c, "POST", "/api/speech/elevenlabs/voices", map[string]string{"ownerId": "own1", "voiceId": "daan1", "name": "Daan"}, 200)
	if added["id"] != "daanMine" {
		t.Fatalf("add: %v", added)
	}

	// Speaking needs the engine switched on.
	tts := map[string]any{"text": "Welkom bij RHIS.", "language": "nl", "engine": "elevenlabs"}
	if code, b := c.do("POST", "/api/speech/tts", tts); code != 409 || b["error"] != "speech_not_ready" {
		t.Fatalf("off: %d %v", code, b)
	}
	st["enabled"], st["defaultEngine"] = true, "elevenlabs"
	st["voiceNl"] = map[string]string{"id": "ruth1", "name": "Ruth"}
	st["speed"] = 200
	if code, b := c.do("PUT", "/api/speech/elevenlabs", st); code != 400 || b["field"] != "speed" {
		t.Fatalf("bad speed: %d %v", code, b)
	}
	st["speed"] = 95
	mustDo(t, c, "PUT", "/api/speech/elevenlabs", st, 200)
	if _, err := exec.LookPath("sox"); err != nil {
		t.Skip("sox not installed")
	}
	out := mustDo(t, c, "POST", "/api/speech/tts", tts, 200)
	src := out["source"].(map[string]any)
	if src["engine"] != "elevenlabs" || src["voice"] != "ruth1" || src["voiceName"] != "Ruth" || out["cached"] != false || out["characters"] != float64(16) {
		t.Fatalf("tts: %v", out)
	}
	wav, _ := base64.StdEncoding.DecodeString(out["audio"].(string))
	if string(wav[:4]) != "RIFF" {
		t.Fatal("not a wav")
	}
	// The same text and voice again: from the cache, no credits.
	if out := mustDo(t, c, "POST", "/api/speech/tts", tts, 200); out["cached"] != true || speaks.Load() != 1 {
		t.Fatalf("cache: %v %d", out["cached"], speaks.Load())
	}

	// A prompt keeps its source and can be spoken again with another voice.
	p := mustDo(t, c, "POST", "/api/pbx/prompts", map[string]any{"name": "Welkom", "audio": out["audio"], "source": out["source"]}, 200)
	id := p["id"].(float64)
	if b := rawGet(t, c, "/api/pbx/prompts"); !strings.Contains(b, `"voiceName":"Ruth"`) {
		t.Fatalf("source not stored: %s", b)
	}
	mustDo(t, c, "PUT", "/api/pbx/prompts/"+jsonNum(id), map[string]any{"name": "Welkom 2"}, 200)
	r := mustDo(t, c, "POST", "/api/pbx/prompts/regenerate", map[string]any{"ids": []float64{id}, "engine": "elevenlabs", "voice": "daanMine", "voiceName": "Daan"}, 200)
	if len(r["done"].([]any)) != 1 || speaks.Load() != 2 {
		t.Fatalf("regenerate: %v", r)
	}
	if b := rawGet(t, c, "/api/pbx/prompts"); !strings.Contains(b, `"voiceName":"Daan"`) || !strings.Contains(b, "Welkom 2") {
		t.Fatalf("after regenerate: %s", b)
	}
}

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }

func rawGet(t *testing.T, c *client, path string) string {
	t.Helper()
	res, err := c.hc.Get(c.url + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}
