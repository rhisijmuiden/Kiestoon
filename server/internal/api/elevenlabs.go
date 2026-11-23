package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/elevenlabs"
	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/secrets"
	"github.com/rhisijmuiden/kiestoon/server/internal/speech"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// ElevenLabs: optional cloud voices next to the local Piper voices. The API key is sealed with the
// server's secrets key. Generated audio is converted to the same 8 kHz WAV as every prompt and
// cached by text + voice + settings, so speaking the same text again costs no credits.

type elState struct {
	once     sync.Once
	box      *secrets.Box
	previews sync.Map // voice ID -> preview URL (from API answers only)
	alertMu  sync.Mutex
}

func (s *Server) elevenRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/speech/elevenlabs", admin(s.elGet))
	mux.Handle("PUT /api/speech/elevenlabs", admin(s.elSave))
	mux.Handle("PUT /api/speech/elevenlabs/key", admin(s.elSetKey))
	mux.Handle("GET /api/speech/elevenlabs/usage", admin(s.elUsage))
	mux.Handle("GET /api/speech/elevenlabs/voices", admin(s.elVoices))
	mux.Handle("POST /api/speech/elevenlabs/voices", admin(s.elAddVoice))
	mux.Handle("GET /api/speech/elevenlabs/voices/{id}/preview", admin(s.elPreview))
	mux.Handle("POST /api/pbx/prompts/regenerate", admin(s.promptsRegenerate))
}

func (s *Server) secretBox() *secrets.Box {
	s.el.once.Do(func() {
		s.el.box = &secrets.Box{Path: filepath.Join(s.Cfg.DataDir, "secrets", "secrets.key")}
	})
	return s.el.box
}

func (s *Server) elSettings(ctx context.Context) elevenlabs.Settings {
	st := elevenlabs.Defaults()
	_ = store.GetJSON(ctx, s.DB, elevenlabs.SettingsKey, &st)
	return st
}

func (s *Server) elKey(ctx context.Context) string {
	v, _ := store.GetSetting(ctx, s.DB, elevenlabs.KeyKey, "")
	k, err := s.secretBox().Open(elevenlabs.KeyKey, v)
	if err != nil {
		s.Log.Warn("elevenlabs key cannot be read", "err", err)
		return ""
	}
	return k
}

func (s *Server) elClient(ctx context.Context) *elevenlabs.Client {
	return &elevenlabs.Client{Key: s.elKey(ctx), Base: os.Getenv("KIESTOON_ELEVENLABS_BASE_URL")}
}

// elErr writes an ElevenLabs failure as an API error the portal can translate.
func (s *Server) elErr(w http.ResponseWriter, err error) {
	var e *elevenlabs.Error
	switch {
	case errors.Is(err, elevenlabs.ErrNoKey):
		writeErr(w, http.StatusConflict, "elevenlabs_no_key")
	case errors.As(err, &e) && e.Unauthorized():
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "elevenlabs_key", "message": e.Message})
	case errors.As(err, &e) && (e.Code == "quota_exceeded" || e.Status == http.StatusPaymentRequired):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "elevenlabs_quota", "message": e.Message})
	case errors.As(err, &e):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "elevenlabs_failed", "message": e.Message})
	default:
		s.Log.Warn("elevenlabs", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "elevenlabs_unreachable", "message": err.Error()})
	}
}

func (s *Server) elGet(w http.ResponseWriter, r *http.Request) {
	key := s.elKey(r.Context())
	hint := ""
	if len(key) > 8 {
		hint = "…" + key[len(key)-4:]
	}
	st, _ := pbx.LoadSettings(r.Context(), s.DB)
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.elSettings(r.Context()), "keySet": key != "", "keyHint": hint,
		"models": elevenlabs.Models, "pbxLanguage": st.Language, "maxText": speech.MaxText})
}

func (s *Server) elSave(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their current value, so older clients do not reset the voice profile.
	st := s.elSettings(r.Context())
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := st.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	if !st.Enabled {
		st.Locked = false
	}
	if st.Locked {
		st.DefaultEngine = "elevenlabs"
	}
	if st.DefaultEngine == "elevenlabs" && !st.Enabled {
		st.DefaultEngine = "piper"
	}
	if err := store.SetJSON(r.Context(), s.DB, elevenlabs.SettingsKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "speech.elevenlabs.saved", fmt.Sprintf("enabled=%v locked=%v", st.Enabled, st.Locked))
	s.elGet(w, r)
}

// elSetKey checks a new key against the API and stores it sealed. An empty key removes it.
func (s *Server) elSetKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	key := strings.TrimSpace(req.Key)
	if len(key) > 200 || strings.ContainsAny(key, " \t\r\n") {
		fieldErr(w, "key")
		return
	}
	if key != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		c := &elevenlabs.Client{Key: key, Base: os.Getenv("KIESTOON_ELEVENLABS_BASE_URL")}
		if _, err := c.Subscription(ctx); err != nil {
			var e *elevenlabs.Error
			// A key limited to text-to-speech may not read the subscription; the voices list proves it works.
			if errors.As(err, &e) && e.Status == http.StatusForbidden || errors.As(err, &e) && e.Code == "missing_permissions" {
				_, err = c.MyVoices(ctx, "")
			}
			if err != nil {
				s.elErr(w, err)
				return
			}
		}
	}
	sealed, err := s.secretBox().Seal(elevenlabs.KeyKey, key)
	if err != nil {
		s.Log.Error("seal elevenlabs key", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := store.SetSetting(r.Context(), s.DB, elevenlabs.KeyKey, sealed); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if key == "" {
		s.audit(r, "speech.elevenlabs.key_removed", "")
	} else {
		s.audit(r, "speech.elevenlabs.key_set", "")
	}
	s.elGet(w, r)
}

func (s *Server) elUsage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	sub, err := s.elClient(ctx).Subscription(ctx)
	if err != nil {
		s.elErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

// elVoices lists voices: source=mine (the account) or library (public voice library, filtered).
func (s *Server) elVoices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	c := s.elClient(ctx)
	lang := q.Get("language")
	if len(lang) > 5 {
		lang = ""
	}
	search := q.Get("search")
	if len(search) > 80 {
		search = search[:80]
	}
	var (
		voices  []elevenlabs.Voice
		hasMore bool
		err     error
	)
	if q.Get("source") == "mine" {
		voices, err = c.MyVoices(ctx, search)
		if err == nil {
			voices = filterMine(voices, lang, q.Get("gender"))
		}
	} else {
		page, _ := strconv.Atoi(q.Get("page"))
		voices, hasMore, err = c.Library(ctx, elevenlabs.LibraryQuery{Language: lang, Gender: q.Get("gender"), UseCase: q.Get("useCase"),
			Accent: q.Get("accent"), Search: search, Page: min(page, 50)})
	}
	if err != nil {
		s.elErr(w, err)
		return
	}
	for _, v := range voices {
		if v.PreviewURL != "" {
			s.el.previews.Store(v.ID, v.PreviewURL)
		}
		for l, u := range v.Previews {
			s.el.previews.Store(v.ID+"/"+l, u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"voices": voices, "hasMore": hasMore})
}

// filterMine keeps account voices in the language (voices without a known language always stay:
// cloned voices often have none) and of the gender.
func filterMine(vs []elevenlabs.Voice, lang, gender string) []elevenlabs.Voice {
	out := vs[:0]
	for _, v := range vs {
		if gender != "" && v.Gender != "" && v.Gender != gender {
			continue
		}
		if lang != "" && (v.Language != "" || len(v.Languages) > 0) && v.Language != lang && !slices.Contains(v.Languages, lang) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// elAddVoice adds a library voice to the account, so it can be used for prompts.
func (s *Server) elAddVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OwnerID string `json:"ownerId"`
		VoiceID string `json:"voiceId"`
		Name    string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 80 {
		fieldErr(w, "name")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	id, err := s.elClient(ctx).AddVoice(ctx, req.OwnerID, req.VoiceID, name)
	if err != nil {
		s.elErr(w, err)
		return
	}
	if u, ok := s.el.previews.Load(req.VoiceID); ok {
		s.el.previews.Store(id, u)
	}
	s.audit(r, "speech.elevenlabs.voice_added", name)
	writeJSON(w, http.StatusOK, elevenlabs.VoiceRef{ID: id, Name: name})
}

// elPreview plays the free sample of a voice through Kiestoon (the portal's CSP allows no other
// media hosts). The URL comes from an earlier API answer, or is looked up for account voices.
func (s *Server) elPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !elevenlabs.ValidID(id) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	key := id
	if l := r.URL.Query().Get("language"); l != "" && len(l) <= 5 {
		if _, ok := s.el.previews.Load(id + "/" + l); ok {
			key = id + "/" + l
		}
	}
	u, ok := s.el.previews.Load(key)
	if !ok {
		v, err := s.elClient(ctx).GetVoice(ctx, id)
		if err != nil {
			s.elErr(w, err)
			return
		}
		if v.PreviewURL == "" {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		s.el.previews.Store(id, v.PreviewURL)
		u = v.PreviewURL
	}
	data, ct, err := elevenlabs.Preview(ctx, nil, u.(string), strings.HasPrefix(os.Getenv("KIESTOON_ELEVENLABS_BASE_URL"), "http://"))
	if err != nil {
		s.Log.Warn("elevenlabs preview", "err", err)
		writeErr(w, http.StatusBadGateway, "elevenlabs_unreachable")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

// ---- speaking ----------------------------------------------------------------

// ttsRequest is text to speak with an engine and voice (empty: the defaults).
type ttsRequest struct {
	Text      string `json:"text"`
	Language  string `json:"language"`
	Engine    string `json:"engine"`
	Voice     string `json:"voice"`
	VoiceName string `json:"voiceName"`
}

// ttsResult is the spoken audio and how it was made.
type ttsResult struct {
	Audio    []byte
	Source   pbx.PromptSource
	Cached   bool // from the cache: no credits used
	FellBack bool // ElevenLabs failed, Piper spoke it
}

var errTTSOff = errors.New("speech_not_ready")

func (s *Server) elCacheDir() string { return filepath.Join(s.Cfg.DataDir, "speech", "elevenlabs") }

// speak turns text into an 8 kHz prompt WAV with the requested or default engine.
func (s *Server) speak(ctx context.Context, req ttsRequest) (*ttsResult, error) {
	text, err := speech.CheckText(req.Text)
	if err != nil {
		return nil, err
	}
	lang := req.Language
	if lang != "nl" {
		lang = "en"
	}
	el := s.elSettings(ctx)
	engine := req.Engine
	if engine == "" {
		engine = el.DefaultEngine
	}
	if el.Enabled && el.Locked {
		// The voice profile is locked: every prompt gets the profile's voice for its language.
		engine, req.Voice, req.VoiceName = "elevenlabs", "", ""
		if el.Voice(lang).ID == "" {
			if other := map[string]string{"nl": "en", "en": "nl"}[lang]; el.Voice(other).ID != "" {
				lang = other
			}
		}
	}
	if engine == "elevenlabs" && el.Enabled {
		res, err := s.speakEleven(ctx, el, text, lang, req.Voice, req.VoiceName)
		if err == nil || !el.Fallback || errors.Is(err, elevenlabs.ErrNoKey) {
			return res, err
		}
		s.Log.Warn("elevenlabs failed, falling back to piper", "err", err)
		pr, perr := s.speakPiper(ctx, text, lang, "")
		if perr != nil {
			return nil, err // the ElevenLabs error says more
		}
		pr.FellBack = true
		return pr, nil
	}
	if engine == "elevenlabs" {
		return nil, errTTSOff
	}
	return s.speakPiper(ctx, text, lang, req.Voice)
}

func (s *Server) speakPiper(ctx context.Context, text, lang, voice string) (*ttsResult, error) {
	st := s.speechSettings(ctx)
	if !st.TTS.Enabled {
		return nil, errTTSOff
	}
	if voice == "" || speech.FindVoice(voice) == nil {
		voice = st.TTS.VoiceEN
		if lang == "nl" {
			voice = st.TTS.VoiceNL
		}
	}
	audio, err := s.speechEngine().Speak(ctx, voice, text)
	if err != nil {
		return nil, err
	}
	name := voice
	if v := speech.FindVoice(voice); v != nil {
		name = v.Name
	}
	return &ttsResult{Audio: audio, Source: pbx.PromptSource{Engine: "piper", Voice: voice, VoiceName: name, Language: lang, Text: text}}, nil
}

func (s *Server) speakEleven(ctx context.Context, el elevenlabs.Settings, text, lang, voice, voiceName string) (*ttsResult, error) {
	if voice == "" {
		def := el.Voice(lang)
		voice, voiceName = def.ID, def.Name
	}
	if !elevenlabs.ValidID(voice) {
		return nil, &elevenlabs.Error{Status: http.StatusNotFound, Code: "voice_not_found", Message: "choose a voice"}
	}
	if m := elevenlabs.FindModel(el.Model); m != nil && len(text) > m.MaxChars {
		return nil, errors.New("text")
	}
	src := pbx.PromptSource{Engine: "elevenlabs", Voice: voice, VoiceName: strings.TrimSpace(voiceName), Model: el.Model, Language: lang, Text: text}
	if len(src.VoiceName) > 80 {
		src.VoiceName = src.VoiceName[:80]
	}
	h := sha256.Sum256([]byte(strings.Join([]string{src.Model, voice, lang, strconv.Itoa(el.Stability), strconv.Itoa(el.Speed),
		strconv.Itoa(el.Similarity), strconv.Itoa(el.Style), strconv.FormatBool(el.SpeakerBoost), text}, "\x00")))
	cache := filepath.Join(s.elCacheDir(), hex.EncodeToString(h[:])+".wav")
	if b, err := os.ReadFile(cache); err == nil {
		if _, err := media.CheckWAV(b); err == nil {
			_ = os.Chtimes(cache, time.Now(), time.Now())
			return &ttsResult{Audio: b, Source: src, Cached: true}, nil
		}
	}
	c := s.elClient(ctx)
	sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pcm, err := c.Speak(sctx, elevenlabs.SpeakRequest{Voice: voice, Model: el.Model, Text: text, Language: lang, Stability: el.Stability, Speed: el.Speed,
		Similarity: el.Similarity, Style: el.Style, SpeakerBoost: el.SpeakerBoost})
	if err != nil {
		return nil, err
	}
	wav, err := speech.TelephonePCM(ctx, pcm, elevenlabs.SampleRate)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.elCacheDir(), 0o750); err == nil {
		_ = writeFileAtomic(cache, wav, 0o640)
		go s.elPruneCache()
	}
	go s.elCheckUsage(c)
	return &ttsResult{Audio: wav, Source: src}, nil
}

// elPruneCache keeps the newest 500 cached texts.
func (s *Server) elPruneCache() {
	es, err := os.ReadDir(s.elCacheDir())
	if err != nil || len(es) <= 500 {
		return
	}
	type f struct {
		name string
		mod  time.Time
	}
	fs := make([]f, 0, len(es))
	for _, e := range es {
		if i, err := e.Info(); err == nil && e.Type().IsRegular() {
			fs = append(fs, f{e.Name(), i.ModTime()})
		}
	}
	for len(fs) > 500 {
		oldest := 0
		for i := range fs {
			if fs[i].mod.Before(fs[oldest].mod) {
				oldest = i
			}
		}
		_ = os.Remove(filepath.Join(s.elCacheDir(), fs[oldest].name))
		fs = append(fs[:oldest], fs[oldest+1:]...)
	}
}

// elCheckUsage mails the admins once per period when 80% of the monthly characters are used.
func (s *Server) elCheckUsage(c *elevenlabs.Client) {
	s.el.alertMu.Lock()
	defer s.el.alertMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sub, err := c.Subscription(ctx)
	if err != nil || sub.CharacterLimit <= 0 || sub.CharacterCount*100 < sub.CharacterLimit*80 {
		return
	}
	period := strconv.FormatInt(sub.ResetUnix, 10)
	if done, _ := store.GetSetting(ctx, s.DB, elevenlabs.AlertKey, ""); done == period {
		return
	}
	_ = store.SetSetting(ctx, s.DB, elevenlabs.AlertKey, period)
	reset := "the next billing period"
	if sub.ResetUnix > 0 {
		reset = time.Unix(sub.ResetUnix, 0).Format("2 January 2006")
	}
	body := fmt.Sprintf("Kiestoon has used %d of the %d ElevenLabs characters of this period (%d%%).\n\n"+
		"The counter starts again on %s. When the characters run out, new prompts are spoken by the local Piper voice "+
		"if the fallback is on; existing prompts keep playing.\n\nSystem > Speech in Kiestoon shows the usage.\n",
		sub.CharacterCount, sub.CharacterLimit, sub.CharacterCount*100/sub.CharacterLimit, reset)
	s.Mail.Notify(ctx, "Kiestoon: 80% of the ElevenLabs characters used", body)
}

// ---- prompts: speak again with another voice ----------------------------------

func (s *Server) promptsRegenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs       []int64 `json:"ids"`
		Engine    string  `json:"engine"`
		Voice     string  `json:"voice"`
		VoiceName string  `json:"voiceName"`
	}
	if err := decode(r, &req); err != nil || len(req.IDs) == 0 || len(req.IDs) > 200 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.Engine != "piper" && req.Engine != "elevenlabs" {
		fieldErr(w, "engine")
		return
	}
	type failure struct {
		ID    int64  `json:"id"`
		Error string `json:"error"`
	}
	done, failed := []int64{}, []failure{}
	for _, id := range req.IDs {
		p, err := s.PBX.Prompt(r.Context(), id)
		if err != nil || p.Source == nil {
			failed = append(failed, failure{id, "no_text"})
			continue
		}
		res, err := s.speak(r.Context(), ttsRequest{Text: p.Source.Text, Language: p.Source.Language, Engine: req.Engine, Voice: req.Voice, VoiceName: req.VoiceName})
		if err != nil {
			failed = append(failed, failure{id, ttsErrCode(err)})
			if errors.Is(err, elevenlabs.ErrNoKey) || errors.Is(err, errTTSOff) {
				break
			}
			continue
		}
		ms, err := media.CheckWAV(res.Audio)
		if err != nil {
			failed = append(failed, failure{id, "invalid_audio"})
			continue
		}
		if err := writeFileAtomic(s.promptFile(id), res.Audio, 0o640); err != nil {
			failed = append(failed, failure{id, "internal"})
			continue
		}
		src := res.Source
		p.DurationMS, p.Source = ms, &src
		if err := s.PBX.SavePrompt(r.Context(), p); err != nil {
			failed = append(failed, failure{id, "internal"})
			continue
		}
		done = append(done, id)
	}
	s.audit(r, "prompt.regenerated", fmt.Sprintf("%d prompts, %s %s", len(done), req.Engine, req.VoiceName))
	writeJSON(w, http.StatusOK, map[string]any{"done": done, "failed": failed})
}

// ttsErrCode is the error code of a failed text-to-speech call.
func ttsErrCode(err error) string {
	var e *elevenlabs.Error
	switch {
	case errors.Is(err, errTTSOff), errors.Is(err, speech.ErrNotReady):
		return "speech_not_ready"
	case errors.Is(err, elevenlabs.ErrNoKey):
		return "elevenlabs_no_key"
	case errors.As(err, &e) && e.Unauthorized():
		return "elevenlabs_key"
	case errors.As(err, &e) && (e.Code == "quota_exceeded" || e.Status == http.StatusPaymentRequired):
		return "elevenlabs_quota"
	case errors.As(err, &e):
		return "elevenlabs_failed"
	case err.Error() == "text":
		return "text"
	}
	return "speech_failed"
}

// encodeAudio is the JSON answer of a spoken text.
func encodeAudio(res *ttsResult) map[string]any {
	return map[string]any{"audio": base64.StdEncoding.EncodeToString(res.Audio), "source": res.Source, "cached": res.Cached,
		"fellBack": res.FellBack, "characters": len([]rune(res.Source.Text))}
}
