// Package speech runs local text-to-speech (Piper) and speech-to-text (whisper.cpp). The programs
// come with the kiestoon-speech package in /opt/kiestoon-speech; voices and models are downloaded on
// request into <data>/speech with pinned checksums. Nothing is sent to a cloud service.
package speech

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// SettingsKey is the settings row with Settings as JSON.
const SettingsKey = "speech"

// Settings choose the voices and what is transcribed.
type Settings struct {
	TTS struct {
		Enabled bool `json:"enabled"`
		// Voices per language (en, nl): an ID from Voices.
		VoiceEN string `json:"voiceEn"`
		VoiceNL string `json:"voiceNl"`
	} `json:"tts"`
	STT struct {
		Enabled  bool   `json:"enabled"`
		Model    string `json:"model"`    // an ID from Models
		Language string `json:"language"` // auto | nl | en
		// Voicemail adds the transcript to voicemail mails and to the voicemail list.
		Voicemail bool `json:"voicemail"`
	} `json:"stt"`
}

// Defaults are used until an admin saves the speech settings.
func Defaults() Settings {
	var s Settings
	s.TTS.VoiceEN, s.TTS.VoiceNL = "en_GB-cori-medium", "nl_NL-pim-medium"
	s.STT.Model, s.STT.Language, s.STT.Voicemail = "small-q5_1", "auto", true
	return s
}

// Validate checks the settings.
func (s *Settings) Validate() error {
	if v := FindVoice(s.TTS.VoiceEN); v == nil || v.Language != "en" {
		return errors.New("voiceEn")
	}
	if v := FindVoice(s.TTS.VoiceNL); v == nil || v.Language != "nl" {
		return errors.New("voiceNl")
	}
	if FindModel(s.STT.Model) == nil {
		return errors.New("model")
	}
	switch s.STT.Language {
	case "auto", "nl", "en":
	default:
		return errors.New("language")
	}
	return nil
}

// File is a download with a pinned checksum.
type File struct {
	Name   string `json:"name"`
	URL    string `json:"-"`
	SHA256 string `json:"-"`
	Size   int64  `json:"size"`
}

// Voice is a Piper voice.
type Voice struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Name     string `json:"name"`
	Licence  string `json:"licence"`
	Files    []File `json:"-"`
	Size     int64  `json:"size"`
}

// Model is a Whisper model.
type Model struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	File  File   `json:"-"`
	Size  int64  `json:"size"`
	Speed string `json:"speed"` // fast | medium | slow
}

const piperBase = "https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/"

func voice(id, lang, name, lic, path, onnxSHA string, onnxSize int64, jsonSHA string) Voice {
	return Voice{ID: id, Language: lang, Name: name, Licence: lic, Size: onnxSize, Files: []File{
		{Name: id + ".onnx", URL: piperBase + path + id + ".onnx", SHA256: onnxSHA, Size: onnxSize},
		{Name: id + ".onnx.json", URL: piperBase + path + id + ".onnx.json", SHA256: jsonSHA},
	}}
}

// Voices are the Piper voices offered in the portal (open licences only).
var Voices = []Voice{
	voice("nl_NL-pim-medium", "nl", "Pim (Nederlands)", "CC0 1.0", "nl/nl_NL/pim/medium/",
		"403e58c3675c394f505c2428117bf34cc56e9542dcf6eadbdd3a84706c12e048", 63516050, "08b58456ca00cf77123826b1712758f99d5fd19ddfb7ec7da8e1a715b047f642"),
	voice("nl_NL-mls-medium", "nl", "MLS (Nederlands)", "CC BY 4.0 (Multilingual LibriSpeech)", "nl/nl_NL/mls/medium/",
		"88312e0fbf505b87caf2373d94c1384892e86b1bf2ee482cf65dc8ba179cc7d3", 76584246, "6ddb215d38f1392ab935ad45441b82ada1eeae0452a2d6849ed71ea4f2e0aa63"),
	voice("en_GB-cori-medium", "en", "Cori (British English)", "Public domain (LibriVox)", "en/en_GB/cori/medium/",
		"1899f98e5fb8310154f3c2973f4b8a929ba7245e722b3d3a85680b833d95f10d", 63531379, "e262c16d7f192f69d4edd6b4ef8a5915379e67495fcc402f1ab15eeb33da3d36"),
	voice("en_US-ljspeech-medium", "en", "LJ Speech (American English)", "Public domain", "en/en_US/ljspeech/medium/",
		"6f52a751e2349abe7a76735eb09dc1875298c77ea2342ffd2fef79ff81b87f22", 63531379, "141d612cc0a95ed7efc1ca936b845c2364967f2e9217c5dbfcf69fc4d6c65860"),
}

const whisperBase = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

// Models are the Whisper models (MIT licence) offered in the portal.
var Models = []Model{
	{ID: "base", Name: "Whisper base", Speed: "fast", Size: 147951465, File: File{Name: "ggml-base.bin", URL: whisperBase + "ggml-base.bin",
		SHA256: "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe", Size: 147951465}},
	{ID: "small-q5_1", Name: "Whisper small", Speed: "medium", Size: 190085487, File: File{Name: "ggml-small-q5_1.bin", URL: whisperBase + "ggml-small-q5_1.bin",
		SHA256: "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb", Size: 190085487}},
	{ID: "medium-q5_0", Name: "Whisper medium", Speed: "slow", Size: 539212467, File: File{Name: "ggml-medium-q5_0.bin", URL: whisperBase + "ggml-medium-q5_0.bin",
		SHA256: "19fea4b380c3a618ec4723c3eef2eb785ffba0d0538cf43f8f235e7b3b34220f", Size: 539212467}},
}

// FindVoice returns the voice, or nil.
func FindVoice(id string) *Voice {
	for i := range Voices {
		if Voices[i].ID == id {
			return &Voices[i]
		}
	}
	return nil
}

// FindModel returns the model, or nil.
func FindModel(id string) *Model {
	for i := range Models {
		if Models[i].ID == id {
			return &Models[i]
		}
	}
	return nil
}

// Engine runs the programs.
type Engine struct {
	// Home is /opt/kiestoon-speech; Dir holds the downloads.
	Home string
	Dir  string
	HTTP *http.Client
	// mu allows one synthesis or transcription at a time: both use all CPU cores.
	mu sync.Mutex
	dl sync.Map // id -> *Download
}

// Download is the progress of a voice or model download.
type Download struct {
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error,omitempty"`
	mu    sync.Mutex
}

// Snapshot returns a copy safe to encode.
func (d *Download) Snapshot() Download {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Download{Done: d.Done, Total: d.Total, Error: d.Error}
}

func (e *Engine) piper() string   { return filepath.Join(e.Home, "bin", "piper") }
func (e *Engine) whisper() string { return filepath.Join(e.Home, "bin", "whisper-cli") }

// Installed reports whether the kiestoon-speech package is there.
func (e *Engine) Installed() (tts, stt bool) {
	_, err1 := os.Stat(e.piper())
	_, err2 := os.Stat(e.whisper())
	return err1 == nil, err2 == nil
}

func (e *Engine) has(f File) bool {
	st, err := os.Stat(filepath.Join(e.Dir, f.Name))
	return err == nil && (f.Size == 0 || st.Size() == f.Size)
}

// HasVoice reports whether all files of a voice are downloaded.
func (e *Engine) HasVoice(v *Voice) bool {
	for _, f := range v.Files {
		if !e.has(f) {
			return false
		}
	}
	return true
}

// HasModel reports whether a model is downloaded.
func (e *Engine) HasModel(m *Model) bool { return e.has(m.File) }

// Progress returns the running or last download of a voice or model.
func (e *Engine) Progress(id string) *Download {
	if v, ok := e.dl.Load(id); ok {
		d := v.(*Download).Snapshot()
		return &d
	}
	return nil
}

// Start downloads a voice or model in the background.
func (e *Engine) Start(id string) error {
	var files []File
	if v := FindVoice(id); v != nil {
		files = v.Files
	} else if m := FindModel(id); m != nil {
		files = []File{m.File}
	} else if l := FindLLM(id); l != nil {
		files = []File{l.File}
	} else {
		return errors.New("unknown")
	}
	d := &Download{}
	for _, f := range files {
		d.Total += f.Size
	}
	if old, loaded := e.dl.LoadOrStore(id, d); loaded {
		o := old.(*Download).Snapshot()
		if o.Error == "" && o.Done < o.Total {
			return nil // still running
		}
		e.dl.Store(id, d)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		for _, f := range files {
			if e.has(f) {
				d.mu.Lock()
				d.Done += f.Size
				d.mu.Unlock()
				continue
			}
			if err := e.fetch(ctx, f, d); err != nil {
				d.mu.Lock()
				d.Error = err.Error()
				d.mu.Unlock()
				return
			}
		}
		d.mu.Lock()
		d.Done = d.Total
		d.mu.Unlock()
	}()
	return nil
}

type counter struct {
	d *Download
}

func (c counter) Write(p []byte) (int, error) {
	c.d.mu.Lock()
	c.d.Done += int64(len(p))
	c.d.mu.Unlock()
	return len(p), nil
}

func (e *Engine) fetch(ctx context.Context, f File, d *Download) error {
	if err := os.MkdirAll(e.Dir, 0o750); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	c := e.HTTP
	if c == nil {
		c = &http.Client{Timeout: time.Hour}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", f.Name, resp.Status)
	}
	tmp := filepath.Join(e.Dir, "."+f.Name+".part")
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha256.New()
	limit := f.Size + 1
	if f.Size == 0 {
		limit = 1 << 20
	}
	_, err = io.Copy(io.MultiWriter(out, h, counter{d}), io.LimitReader(resp.Body, limit))
	out.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != f.SHA256 {
		_ = os.Remove(tmp)
		return fmt.Errorf("download %s: checksum mismatch", f.Name)
	}
	return os.Rename(tmp, filepath.Join(e.Dir, f.Name))
}

// Remove deletes a downloaded voice or model.
func (e *Engine) Remove(id string) {
	var files []File
	if v := FindVoice(id); v != nil {
		files = v.Files
	} else if m := FindModel(id); m != nil {
		files = []File{m.File}
	} else if l := FindLLM(id); l != nil {
		files = []File{l.File}
	}
	for _, f := range files {
		_ = os.Remove(filepath.Join(e.Dir, f.Name))
	}
	e.dl.Delete(id)
}

// MaxText is the longest text that can be spoken at once.
const MaxText = 2000

// ErrNotReady means the program or the voice/model is missing.
var ErrNotReady = errors.New("speech_not_ready")

func threads() string { return strconv.Itoa(max(1, min(runtime.NumCPU(), 8))) }

// Speak turns text into telephone audio: 8 kHz, mono, 16-bit WAV.
func (e *Engine) Speak(ctx context.Context, voiceID, text string) ([]byte, error) {
	v := FindVoice(voiceID)
	if tts, _ := e.Installed(); !tts || v == nil || !e.HasVoice(v) {
		return nil, ErrNotReady
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r", ""))
	if text == "" || utf8.RuneCountInString(text) > MaxText || !utf8.ValidString(text) {
		return nil, errors.New("text")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	tmp, err := os.MkdirTemp("", "kt-tts-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	raw, tel := filepath.Join(tmp, "raw.wav"), filepath.Join(tmp, "tel.wav")
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, e.piper(), "-m", filepath.Join(e.Dir, v.Files[0].Name), "-f", raw)
	cmd.Stdin = strings.NewReader(text)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + tmp, "OMP_NUM_THREADS=" + threads()}
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("piper: %v: %s", err, lastLine(out))
	}
	// The same telephone treatment as the Dutch system prompts (packaging/sounds-nl/build.sh).
	sox := exec.CommandContext(cctx, "sox", "-q", raw, "-r", "8000", "-c", "1", "-b", "16", tel,
		"gain", "-3", "highpass", "120", "lowpass", "3400", "compand", "0.02,0.2", "-60,-60,-30,-15,0,-6", "-6", "0", "0.05", "pad", "0.1", "0.3", "norm", "-1")
	if out, err := sox.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("sox: %v: %s", err, lastLine(out))
	}
	return os.ReadFile(tel)
}

// Transcribe returns the text spoken in an audio file (any format sox reads).
func (e *Engine) Transcribe(ctx context.Context, modelID, language, audio string) (string, error) {
	m := FindModel(modelID)
	if _, stt := e.Installed(); !stt || m == nil || !e.HasModel(m) {
		return "", ErrNotReady
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.transcribe(ctx, m, language, audio)
}

// TranscribeNow transcribes a short recording right away, next to a running background job (a
// caller is waiting for it in a voice menu).
func (e *Engine) TranscribeNow(ctx context.Context, modelID, language, audio string) (string, error) {
	m := FindModel(modelID)
	if _, stt := e.Installed(); !stt || m == nil || !e.HasModel(m) {
		return "", ErrNotReady
	}
	return e.transcribe(ctx, m, language, audio)
}

func (e *Engine) transcribe(ctx context.Context, m *Model, language, audio string) (string, error) {
	tmp, err := os.MkdirTemp("", "kt-stt-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	in := filepath.Join(tmp, "in.wav")
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(cctx, "sox", "-q", audio, "-r", "16000", "-c", "1", "-b", "16", in).CombinedOutput(); err != nil {
		return "", fmt.Errorf("sox: %v: %s", err, lastLine(out))
	}
	if language == "" {
		language = "auto"
	}
	cmd := exec.CommandContext(cctx, e.whisper(), "-m", filepath.Join(e.Dir, m.File.Name), "-f", in, "-l", language,
		"-t", threads(), "-nt", "-np")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + tmp}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("whisper: %v: %s", err, lastLine(stderr.Bytes()))
	}
	return Clean(string(out)), nil
}

// Clean joins the lines whisper prints and drops its markers for silence and music.
func Clean(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		switch strings.ToLower(l) {
		case "", "[blank_audio]", "[ silence ]", "[silence]", "[music]", "(music)", "[ music ]":
			continue
		}
		parts = append(parts, l)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func lastLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
