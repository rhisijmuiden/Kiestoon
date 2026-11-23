// Package elevenlabs talks to the ElevenLabs text-to-speech API. Only typed prompt text is sent;
// the audio that comes back is converted to telephone quality and stored on the server, so calls
// never depend on ElevenLabs being reachable.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBase is the public API.
const DefaultBase = "https://api.elevenlabs.io"

// SettingsKey and KeyKey are the settings rows: Settings as JSON and the sealed API key.
const (
	SettingsKey = "elevenlabs"
	KeyKey      = "elevenlabs_api_key"
	// AlertKey holds the reset time of the period the 80% usage mail was sent for.
	AlertKey = "elevenlabs_usage_alert"
)

// Model is a text-to-speech model offered in the portal.
type Model struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MaxChars int    `json:"maxChars"`
	// Language: the model accepts language_code.
	Language bool `json:"-"`
}

// Models in order of preference. Multilingual v2 sounds the most natural in Dutch on the phone.
var Models = []Model{
	{ID: "eleven_multilingual_v2", Name: "Multilingual v2", MaxChars: 10000},
	{ID: "eleven_flash_v2_5", Name: "Flash v2.5", MaxChars: 40000, Language: true},
	{ID: "eleven_v3", Name: "Eleven v3", MaxChars: 5000, Language: true},
}

// FindModel returns a model by ID.
func FindModel(id string) *Model {
	for i := range Models {
		if Models[i].ID == id {
			return &Models[i]
		}
	}
	return nil
}

// VoiceRef is a chosen voice: its ID and the name shown in the portal.
type VoiceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings of the ElevenLabs engine.
type Settings struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
	// DefaultEngine for new prompts: piper or elevenlabs.
	DefaultEngine string   `json:"defaultEngine"`
	VoiceNL       VoiceRef `json:"voiceNl"`
	VoiceEN       VoiceRef `json:"voiceEn"`
	// Stability 0-100 and Speed 70-120 (percent of normal).
	Stability int `json:"stability"`
	Speed     int `json:"speed"`
	// Similarity 0-100 (how close to the original voice), Style 0-100 (exaggeration) and
	// SpeakerBoost (extra clarity).
	Similarity   int  `json:"similarity"`
	Style        int  `json:"style"`
	SpeakerBoost bool `json:"speakerBoost"`
	// Locked fixes the voice profile: every generated prompt uses ElevenLabs with the profile's
	// voice for its language and these settings, so all prompts sound the same.
	Locked bool `json:"locked"`
	// Fallback uses the local Piper voice when ElevenLabs cannot be reached.
	Fallback bool `json:"fallback"`
}

// Defaults are used until an admin saves the settings.
func Defaults() Settings {
	return Settings{Model: Models[0].ID, DefaultEngine: "piper", Stability: 55, Speed: 95, Similarity: 75, SpeakerBoost: true, Fallback: true}
}

// Validate checks the settings.
func (s *Settings) Validate() error {
	if FindModel(s.Model) == nil {
		return errors.New("model")
	}
	if s.DefaultEngine != "piper" && s.DefaultEngine != "elevenlabs" {
		return errors.New("defaultEngine")
	}
	if s.Stability < 0 || s.Stability > 100 {
		return errors.New("stability")
	}
	if s.Speed < 70 || s.Speed > 120 {
		return errors.New("speed")
	}
	if s.Similarity < 0 || s.Similarity > 100 {
		return errors.New("similarity")
	}
	if s.Style < 0 || s.Style > 100 {
		return errors.New("style")
	}
	for f, v := range map[string]*VoiceRef{"voiceNl": &s.VoiceNL, "voiceEn": &s.VoiceEN} {
		v.Name = strings.TrimSpace(v.Name)
		if len(v.Name) > 80 {
			v.Name = v.Name[:80]
		}
		if v.ID != "" && !ValidID(v.ID) {
			return errors.New(f)
		}
	}
	if s.Locked && s.VoiceNL.ID == "" && s.VoiceEN.ID == "" {
		return errors.New("locked")
	}
	return nil
}

// Voice returns the default voice for a language (nl or en).
func (s *Settings) Voice(lang string) VoiceRef {
	if lang == "nl" {
		return s.VoiceNL
	}
	return s.VoiceEN
}

// ValidID reports whether id looks like an ElevenLabs ID (letters and digits). IDs end up in URL
// paths, so nothing else is allowed.
func ValidID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Client calls the API with one key.
type Client struct {
	Key  string
	Base string // DefaultBase when empty
	HTTP *http.Client
}

// Error is an error answer of the API.
type Error struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("elevenlabs %d %s: %s", e.Status, e.Code, e.Message)
}

// Unauthorized reports a missing or wrong API key.
func (e *Error) Unauthorized() bool { return e.Status == http.StatusUnauthorized }

// ErrNoKey: no API key is stored.
var ErrNoKey = errors.New("elevenlabs_no_key")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, max int64) ([]byte, string, error) {
	if c.Key == "" {
		return nil, "", ErrNoKey
	}
	base := c.Base
	if base == "" {
		base = DefaultBase
	}
	u := strings.TrimRight(base, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("xi-api-key", c.Key)
	req.Header.Set("User-Agent", "Kiestoon")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode/100 != 2 {
		return nil, "", parseError(res.StatusCode, data)
	}
	if int64(len(data)) > max {
		return nil, "", errors.New("elevenlabs: answer too large")
	}
	return data, res.Header.Get("Content-Type"), nil
}

// parseError reads {"detail":{"status":..,"message":..}} or {"detail":"..."}.
func parseError(status int, data []byte) error {
	e := &Error{Status: status, Code: "error"}
	var v struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(data, &v) == nil && len(v.Detail) > 0 {
		var d struct {
			Status  string `json:"status"`
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		var s string
		if json.Unmarshal(v.Detail, &d) == nil {
			e.Code, e.Message = firstOf(d.Status, d.Code, "error"), d.Message
		} else if json.Unmarshal(v.Detail, &s) == nil {
			e.Message = s
		}
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	if len(e.Message) > 300 {
		e.Message = e.Message[:300]
	}
	return e
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// Subscription is the plan and the character use of the current period.
type Subscription struct {
	Tier           string `json:"tier"`
	Status         string `json:"status"`
	CharacterCount int64  `json:"characterCount"`
	CharacterLimit int64  `json:"characterLimit"`
	ResetUnix      int64  `json:"resetUnix"`
}

// Subscription returns the plan of the key's account.
func (c *Client) Subscription(ctx context.Context) (*Subscription, error) {
	data, _, err := c.do(ctx, http.MethodGet, "/v1/user/subscription", nil, nil, 1<<20)
	if err != nil {
		return nil, err
	}
	var v struct {
		Tier           string `json:"tier"`
		Status         string `json:"status"`
		CharacterCount int64  `json:"character_count"`
		CharacterLimit int64  `json:"character_limit"`
		Reset          *int64 `json:"next_character_count_reset_unix"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	s := &Subscription{Tier: v.Tier, Status: v.Status, CharacterCount: v.CharacterCount, CharacterLimit: v.CharacterLimit}
	if v.Reset != nil {
		s.ResetUnix = *v.Reset
	}
	return s, nil
}

// Voice is a voice from the account ("my voices") or from the public voice library.
type Voice struct {
	ID          string   `json:"id"`
	OwnerID     string   `json:"ownerId,omitempty"` // library voices: the owner, needed to add it
	Name        string   `json:"name"`
	Language    string   `json:"language,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	Gender      string   `json:"gender,omitempty"`
	Age         string   `json:"age,omitempty"`
	Accent      string   `json:"accent,omitempty"`
	UseCase     string   `json:"useCase,omitempty"`
	Descriptive string   `json:"descriptive,omitempty"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Mine        bool     `json:"mine"`
	PreviewURL  string   `json:"-"`
	// Previews per language (verified languages of a voice), served as ?lang=.
	Previews map[string]string `json:"-"`
}

type verified struct {
	Language   string `json:"language"`
	Accent     string `json:"accent"`
	PreviewURL string `json:"preview_url"`
}

type accountVoice struct {
	VoiceID     string            `json:"voice_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	PreviewURL  string            `json:"preview_url"`
	Verified    []verified        `json:"verified_languages"`
}

func (a *accountVoice) voice() Voice {
	v := Voice{ID: a.VoiceID, Name: a.Name, Category: a.Category, Description: a.Description, Mine: true, PreviewURL: a.PreviewURL,
		Gender: a.Labels["gender"], Age: a.Labels["age"], Accent: a.Labels["accent"], UseCase: firstOf(a.Labels["use_case"], a.Labels["use case"]),
		Descriptive: firstOf(a.Labels["descriptive"], a.Labels["description"]), Language: a.Labels["language"]}
	for _, l := range a.Verified {
		if l.Language == "" {
			continue
		}
		if v.Previews == nil {
			v.Previews = map[string]string{}
		}
		if _, ok := v.Previews[l.Language]; !ok {
			v.Languages = append(v.Languages, l.Language)
			if l.PreviewURL != "" {
				v.Previews[l.Language] = l.PreviewURL
			}
		}
		if v.Accent == "" && l.Language == v.Language {
			v.Accent = l.Accent
		}
	}
	if v.Language == "" && len(v.Languages) > 0 {
		v.Language = v.Languages[0]
	}
	return v
}

// MyVoices returns the voices in the account (premade, added from the library and cloned).
func (c *Client) MyVoices(ctx context.Context, search string) ([]Voice, error) {
	out := []Voice{}
	token := ""
	for range 5 {
		q := url.Values{"page_size": {"100"}, "include_total_count": {"false"}}
		if search != "" {
			q.Set("search", search)
		}
		if token != "" {
			q.Set("next_page_token", token)
		}
		data, _, err := c.do(ctx, http.MethodGet, "/v2/voices", q, nil, 8<<20)
		if err != nil {
			return nil, err
		}
		var v struct {
			Voices  []accountVoice `json:"voices"`
			HasMore bool           `json:"has_more"`
			Next    string         `json:"next_page_token"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		for i := range v.Voices {
			out = append(out, v.Voices[i].voice())
		}
		if !v.HasMore || v.Next == "" {
			break
		}
		token = v.Next
	}
	return out, nil
}

// GetVoice returns one voice of the account.
func (c *Client) GetVoice(ctx context.Context, id string) (*Voice, error) {
	if !ValidID(id) {
		return nil, &Error{Status: http.StatusNotFound, Code: "voice_not_found", Message: "unknown voice"}
	}
	data, _, err := c.do(ctx, http.MethodGet, "/v1/voices/"+id, nil, nil, 1<<20)
	if err != nil {
		return nil, err
	}
	var a accountVoice
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	v := a.voice()
	return &v, nil
}

// LibraryQuery filters the public voice library.
type LibraryQuery struct {
	Language string // ISO 639-1, e.g. nl
	Gender   string // male | female | neutral
	UseCase  string
	Accent   string
	Search   string
	Page     int
}

// Library searches the public voice library. Voices from the library do not use voice slots.
func (c *Client) Library(ctx context.Context, f LibraryQuery) ([]Voice, bool, error) {
	q := url.Values{"page_size": {"30"}, "page": {strconv.Itoa(max(0, f.Page))}, "sort": {"usage_character_count_1y"}}
	for k, v := range map[string]string{"language": f.Language, "gender": f.Gender, "use_cases": f.UseCase, "accent": f.Accent, "search": f.Search} {
		if v != "" {
			q.Set(k, v)
		}
	}
	data, _, err := c.do(ctx, http.MethodGet, "/v1/shared-voices", q, nil, 8<<20)
	if err != nil {
		return nil, false, err
	}
	var v struct {
		Voices []struct {
			OwnerID     string     `json:"public_owner_id"`
			VoiceID     string     `json:"voice_id"`
			Name        string     `json:"name"`
			Accent      string     `json:"accent"`
			Gender      string     `json:"gender"`
			Age         string     `json:"age"`
			Descriptive string     `json:"descriptive"`
			UseCase     string     `json:"use_case"`
			Category    string     `json:"category"`
			Language    string     `json:"language"`
			Description string     `json:"description"`
			PreviewURL  string     `json:"preview_url"`
			Added       bool       `json:"is_added_by_user"`
			Verified    []verified `json:"verified_languages"`
		} `json:"voices"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, false, err
	}
	out := make([]Voice, 0, len(v.Voices))
	for _, l := range v.Voices {
		vc := Voice{ID: l.VoiceID, OwnerID: l.OwnerID, Name: l.Name, Language: l.Language, Gender: l.Gender, Age: l.Age, Accent: l.Accent,
			UseCase: l.UseCase, Descriptive: l.Descriptive, Description: l.Description, Category: l.Category, Mine: l.Added, PreviewURL: l.PreviewURL}
		// A multilingual voice has a preview per language; prefer the one in the language searched for.
		for _, x := range l.Verified {
			if f.Language != "" && x.Language == f.Language && x.PreviewURL != "" {
				vc.PreviewURL = x.PreviewURL
				if x.Accent != "" {
					vc.Accent = x.Accent
				}
				break
			}
		}
		out = append(out, vc)
	}
	return out, v.HasMore, nil
}

// AddVoice adds a library voice to the account and returns its ID there.
func (c *Client) AddVoice(ctx context.Context, ownerID, voiceID, name string) (string, error) {
	if !ValidID(ownerID) || !ValidID(voiceID) {
		return "", &Error{Status: http.StatusNotFound, Code: "voice_not_found", Message: "unknown voice"}
	}
	data, _, err := c.do(ctx, http.MethodPost, "/v1/voices/add/"+ownerID+"/"+voiceID, nil, map[string]any{"new_name": name}, 1<<20)
	if err != nil {
		return "", err
	}
	var v struct {
		VoiceID string `json:"voice_id"`
	}
	if err := json.Unmarshal(data, &v); err != nil || !ValidID(v.VoiceID) {
		return "", errors.New("elevenlabs: no voice id in answer")
	}
	return v.VoiceID, nil
}

// SpeakRequest is one text to speak.
type SpeakRequest struct {
	Voice     string
	Model     string
	Text      string
	Language  string // nl | en, sent to models that take it
	Stability int    // 0-100
	Speed     int    // 70-120
	// Similarity and Style 0-100, SpeakerBoost on or off.
	Similarity   int
	Style        int
	SpeakerBoost bool
}

// SampleRate of the PCM that Speak returns.
const SampleRate = 16000

// Speak returns the text as raw PCM: 16 kHz, mono, signed 16-bit little endian.
func (c *Client) Speak(ctx context.Context, r SpeakRequest) ([]byte, error) {
	if !ValidID(r.Voice) {
		return nil, &Error{Status: http.StatusNotFound, Code: "voice_not_found", Message: "choose a voice"}
	}
	m := FindModel(r.Model)
	if m == nil {
		m = &Models[0]
	}
	body := map[string]any{
		"text":     r.Text,
		"model_id": m.ID,
		"voice_settings": map[string]any{
			"stability":         float64(r.Stability) / 100,
			"similarity_boost":  float64(r.Similarity) / 100,
			"style":             float64(r.Style) / 100,
			"use_speaker_boost": r.SpeakerBoost,
			"speed":             float64(r.Speed) / 100,
		},
	}
	if m.Language && (r.Language == "nl" || r.Language == "en") {
		body["language_code"] = r.Language
	}
	q := url.Values{"output_format": {"pcm_16000"}}
	data, _, err := c.do(ctx, http.MethodPost, "/v1/text-to-speech/"+r.Voice, q, body, 64<<20)
	if err != nil {
		return nil, err
	}
	if len(data) < 320 {
		return nil, errors.New("elevenlabs: empty audio")
	}
	return data, nil
}

// Preview downloads a voice sample (MP3). Only https URLs are fetched (plain http only for tests
// against a local API); the caller takes them from an API answer, never from the browser.
func Preview(ctx context.Context, hc *http.Client, raw string, allowHTTP bool) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || !(u.Scheme == "https" || allowHTTP && u.Scheme == "http") || u.Host == "" {
		return nil, "", errors.New("elevenlabs: bad preview url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("elevenlabs preview: %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 5<<20+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > 5<<20 {
		return nil, "", errors.New("elevenlabs preview too large")
	}
	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "audio/") {
		ct = "audio/mpeg"
	}
	return data, ct, nil
}
