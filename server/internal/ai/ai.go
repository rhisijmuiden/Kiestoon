// Package ai makes summaries of call and voicemail transcripts. Transcripts always come from the
// local Whisper (package speech). Summaries are made by a local model (llama.cpp, nothing leaves
// the server) or, when an administrator adds an API key, by Claude in the cloud (text only).
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/rhisijmuiden/kiestoon/server/internal/speech"
)

// SettingsKey is the settings row with Settings as JSON; KeyKey holds the cloud API key apart, so
// it is never sent back to the browser.
const (
	SettingsKey = "ai"
	KeyKey      = "ai_api_key"
)

// Engines.
const (
	EngineLocal = "local"
	EngineCloud = "cloud"
)

// CloudModels are the Claude models offered. The first is the default.
var CloudModels = []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"}

// Settings choose what is transcribed and summarised, and how.
type Settings struct {
	// Calls transcribes and summarises recorded calls (only calls that are recorded anyway).
	Calls bool `json:"calls"`
	// Voicemail summarises voicemail messages (the transcript itself is a speech setting).
	Voicemail bool `json:"voicemail"`
	// Engine is local or cloud.
	Engine string `json:"engine"`
	// LLM is the local model (speech.LLMs).
	LLM string `json:"llm"`
	// CloudModel is the Claude model.
	CloudModel string `json:"cloudModel"`
	// Language of the summaries: auto (the language of the call), nl or en.
	Language string `json:"language"`
	// KeepDays removes AI results of calls after this many days (0 = keep as long as the recording).
	KeepDays int `json:"keepDays"`
}

// Defaults are used until an admin saves the settings.
func Defaults() Settings {
	return Settings{Engine: EngineLocal, LLM: "qwen3-4b-instruct", CloudModel: CloudModels[0], Language: "auto"}
}

// Validate checks the settings.
func (s *Settings) Validate() error {
	switch s.Engine {
	case EngineLocal, EngineCloud:
	default:
		return errors.New("engine")
	}
	if speech.FindLLM(s.LLM) == nil {
		return errors.New("llm")
	}
	ok := false
	for _, m := range CloudModels {
		ok = ok || m == s.CloudModel
	}
	if !ok {
		return errors.New("cloudModel")
	}
	switch s.Language {
	case "auto", "nl", "en":
	default:
		return errors.New("language")
	}
	if s.KeepDays < 0 || s.KeepDays > 3650 {
		return errors.New("keepDays")
	}
	return nil
}

// Summary is what the model returns.
type Summary struct {
	// Summary is a few sentences on what the call was about and how it ended.
	Summary string `json:"summary"`
	// Actions are follow-ups somebody promised or should do.
	Actions []string `json:"actions"`
	// Topic is a short label (a few words), for lists.
	Topic string `json:"topic"`
	// Sentiment of the caller: positive, neutral or negative.
	Sentiment string `json:"sentiment"`
}

// Kind of text summarised.
const (
	KindCall      = "call"
	KindVoicemail = "voicemail"
)

// MaxTranscript bounds the text sent to a model (about an hour of talking).
const MaxTranscript = 60000

// Summarizer makes summaries.
type Summarizer struct {
	Speech *speech.Engine
	// APIKey is the Claude API key (only for the cloud engine).
	APIKey string
	// BaseURL overrides the API address (tests).
	BaseURL string
}

// ErrNoKey means the cloud engine is chosen but no API key was added.
var ErrNoKey = errors.New("ai_no_key")

func prompt(kind, language string) string {
	what := "a recorded business phone call (one mixed audio track, so speakers are not labelled)"
	if kind == KindVoicemail {
		what = "a voicemail message left for an employee"
	}
	lang := "Write in the language that is spoken in the transcript."
	switch language {
	case "nl":
		lang = "Write every field (summary, actions and topic) in Dutch."
	case "en":
		lang = "Write every field (summary, actions and topic) in English."
	}
	return "You summarise " + what + " for the staff of a company. The transcript was made by speech recognition " +
		"and can contain errors; do not invent names, numbers or facts that are not in it. " + lang + "\n" +
		"Answer with one JSON object and nothing else, with these fields:\n" +
		`- "summary": 1 to 4 short sentences: who called (if said), what it was about and what was agreed or asked.` + "\n" +
		`- "actions": a list of follow-ups that someone promised or that the caller asked for (call back, send a quote...), each one short; empty when there are none.` + "\n" +
		`- "topic": a label of 2 to 5 words.` + "\n" +
		`- "sentiment": "positive", "neutral" or "negative" for the caller's mood.`
}

var (
	wordRe  = regexp.MustCompile(`[\p{L}']+`)
	nlWords = words("de het een en van is niet ik u je met voor dat op wij we kunt mijn onze bel belt graag dank bedankt goedemorgen goedemiddag hallo terugbellen")
	enWords = words("the a and of is not i you with for that on we could my our call please thanks thank hello hi morning back")
)

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// Detect guesses whether a transcript is Dutch or English (the languages of the portal). It
// returns "auto" when it cannot tell.
func Detect(text string) string {
	nl, en := 0, 0
	for _, w := range wordRe.FindAllString(strings.ToLower(text), 2000) {
		if nlWords[w] {
			nl++
		}
		if enWords[w] {
			en++
		}
	}
	switch {
	case nl > en && nl >= 2:
		return "nl"
	case en > nl && en >= 2:
		return "en"
	}
	return "auto"
}

var fence = regexp.MustCompile("(?s)^```(?:json)?\\s*(.*?)\\s*```$")

// parse reads the JSON object a model returned, tolerating code fences and text around it.
func parse(s string) (*Summary, error) {
	s = strings.TrimSpace(s)
	if m := fence.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	var out Summary
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("model answer is not JSON: %w", err)
	}
	out.Summary = strings.TrimSpace(out.Summary)
	if out.Summary == "" {
		return nil, errors.New("model answer has no summary")
	}
	actions := []string{}
	for _, a := range out.Actions {
		if a = strings.TrimSpace(a); a != "" && len(actions) < 10 {
			actions = append(actions, a)
		}
	}
	out.Actions = actions
	switch out.Sentiment {
	case "positive", "neutral", "negative":
	default:
		out.Sentiment = "neutral"
	}
	out.Topic = strings.TrimSpace(out.Topic)
	if utf8.RuneCountInString(out.Topic) > 60 {
		out.Topic = string([]rune(out.Topic)[:60])
	}
	return &out, nil
}

// Summarize summarises a transcript with the engine in st. engine is a label for the UI.
func (z *Summarizer) Summarize(ctx context.Context, st Settings, kind, transcript string) (sum *Summary, engine string, err error) {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return nil, "", errors.New("empty transcript")
	}
	if len(transcript) > MaxTranscript {
		transcript = transcript[:MaxTranscript]
	}
	lang := st.Language
	if lang == "auto" {
		// Small local models do not follow "the language of the transcript" well: name it.
		lang = Detect(transcript)
	}
	system := prompt(kind, lang)
	user := "Transcript:\n\n" + transcript
	var answer string
	if st.Engine == EngineCloud {
		if z.APIKey == "" {
			return nil, "", ErrNoKey
		}
		answer, err = z.claude(ctx, st.CloudModel, system, user)
		engine = st.CloudModel
	} else {
		answer, err = z.Speech.Chat(ctx, st.LLM, system, user, true)
		engine = st.LLM
	}
	if err != nil {
		return nil, engine, err
	}
	sum, err = parse(answer)
	return sum, engine, err
}

// claude asks Claude through the Messages API. Opus and Sonnet get server-side fallbacks, so a
// refused request is answered by another model instead of failing.
func (z *Summarizer) claude(ctx context.Context, model, system, user string) (string, error) {
	// Only the key the administrator stored counts, never ANTHROPIC_* variables of the host.
	opts := []option.RequestOption{option.WithoutEnvironmentDefaults(), option.WithAPIKey(z.APIKey), option.WithMaxRetries(2), option.WithRequestTimeout(3 * time.Minute)}
	if z.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(z.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 4000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))},
	}
	if model != "claude-haiku-5-5" {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}
	resp, err := client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return "", fmt.Errorf("claude: HTTP %d", apiErr.StatusCode)
		}
		return "", fmt.Errorf("claude: %w", err)
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return "", errors.New("claude declined to summarise this call")
	}
	var b strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String(), nil
}

// Classify picks which of the options (from 1) fits what a caller said in a voice menu, or 0 when
// none fits. It is the fallback of a speech step when no keyword matched.
func (z *Summarizer) Classify(ctx context.Context, st Settings, said string, options []string) (int, error) {
	said = strings.TrimSpace(said)
	if said == "" || len(options) == 0 {
		return 0, nil
	}
	if len(said) > 2000 {
		said = said[:2000]
	}
	var b strings.Builder
	for i, o := range options {
		fmt.Fprintf(&b, "%d. %s\n", i+1, o)
	}
	system := "You route callers in the voice menu of a company. The caller was asked what they are calling about; " +
		"their answer was transcribed by speech recognition and can contain errors. Choose the option that fits the " +
		"answer best. Answer with one JSON object and nothing else: {\"option\": n} with the number of the option, " +
		"or {\"option\": 0} when no option fits or the answer is unclear."
	user := "Options:\n" + b.String() + "\nThe caller said: " + said
	var answer string
	var err error
	if st.Engine == EngineCloud {
		if z.APIKey == "" {
			return 0, ErrNoKey
		}
		answer, err = z.claude(ctx, st.CloudModel, system, user)
	} else {
		answer, err = z.Speech.Chat(ctx, st.LLM, system, user, true)
	}
	if err != nil {
		return 0, err
	}
	answer = strings.TrimSpace(answer)
	if i, j := strings.Index(answer, "{"), strings.LastIndex(answer, "}"); i >= 0 && j > i {
		answer = answer[i : j+1]
	}
	var out struct {
		Option int `json:"option"`
	}
	if err := json.Unmarshal([]byte(answer), &out); err != nil {
		return 0, fmt.Errorf("model answer is not JSON: %w", err)
	}
	if out.Option < 0 || out.Option > len(options) {
		return 0, nil
	}
	return out.Option, nil
}
