package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// LLM is a local language model (GGUF) for summaries, run with llama.cpp's llama-server.
type LLM struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Licence string `json:"licence"`
	Speed   string `json:"speed"`
	// RAM is roughly what the model needs while it runs (bytes).
	RAM  int64 `json:"ram"`
	File File  `json:"-"`
	Size int64 `json:"size"`
}

// LLMs are the local models offered in the portal (Apache 2.0, good in Dutch and English).
var LLMs = []LLM{
	{ID: "qwen3-4b-instruct", Name: "Qwen3 4B Instruct", Licence: "Apache 2.0", Speed: "medium", RAM: 4 << 30, Size: 2497281120,
		File: File{Name: "Qwen3-4B-Instruct-2507-Q4_K_M.gguf", Size: 2497281120,
			URL:    "https://huggingface.co/unsloth/Qwen3-4B-Instruct-2507-GGUF/resolve/main/Qwen3-4B-Instruct-2507-Q4_K_M.gguf",
			SHA256: "3605803b982cb64aead44f6c1b2ae36e3acdb41d8e46c8a94c6533bc4c67e597"}},
	{ID: "qwen3-1.7b", Name: "Qwen3 1.7B", Licence: "Apache 2.0", Speed: "fast", RAM: 2 << 30, Size: 1107409472,
		File: File{Name: "Qwen3-1.7B-Q4_K_M.gguf", Size: 1107409472,
			URL:    "https://huggingface.co/unsloth/Qwen3-1.7B-GGUF/resolve/main/Qwen3-1.7B-Q4_K_M.gguf",
			SHA256: "b139949c5bd74937ad8ed8c8cf3d9ffb1e99c866c823204dc42c0d91fa181897"}},
}

// FindLLM returns the local model, or nil.
func FindLLM(id string) *LLM {
	for i := range LLMs {
		if LLMs[i].ID == id {
			return &LLMs[i]
		}
	}
	return nil
}

// HasLLM reports whether a local model is downloaded.
func (e *Engine) HasLLM(m *LLM) bool { return e.has(m.File) }

func (e *Engine) llamaServer() string { return filepath.Join(e.Home, "llama", "bin", "llama-server") }

// LLMInstalled reports whether llama-server is in the kiestoon-speech package.
func (e *Engine) LLMInstalled() bool {
	_, err := os.Stat(e.llamaServer())
	return err == nil
}

// llamaProc is a running llama-server with one model loaded. It is started on the first request
// and stopped after a few idle minutes, so the memory is only used while summaries are made.
type llamaProc struct {
	mu     sync.Mutex
	model  string
	cmd    *exec.Cmd
	port   int
	timer  *time.Timer
	exited chan struct{}
}

var llama llamaProc

// IdleStop is how long llama-server stays loaded after the last request.
var IdleStop = 5 * time.Minute

// StopLLM stops a running llama-server (on shutdown, or when a model is removed).
func StopLLM() {
	llama.mu.Lock()
	defer llama.mu.Unlock()
	llama.stopLocked()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (p *llamaProc) stopLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		<-p.exited
	}
	p.cmd, p.model = nil, ""
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

// ensure starts llama-server with the model (or keeps the running one) and returns its port.
func (e *Engine) ensureLlama(ctx context.Context, m *LLM) (int, error) {
	p := &llama
	if p.cmd != nil && p.model == m.ID {
		select {
		case <-p.exited:
			p.cmd = nil
		default:
			return p.port, nil
		}
	}
	p.stopLocked()
	port, err := freePort()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(e.llamaServer(), "-m", filepath.Join(e.Dir, m.File.Name), "--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"-c", "16384", "-t", threads(), "--jinja", "--no-webui", "-np", "1")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + os.TempDir()}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	p.cmd, p.model, p.port, p.exited = cmd, m.ID, port, exited
	// Loading a model takes a few seconds to a minute on slow disks.
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			p.cmd = nil
			return 0, fmt.Errorf("llama-server stopped: %s", lastLine(stderr.Bytes()))
		case <-ctx.Done():
			p.stopLocked()
			return 0, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return port, nil
			}
		}
	}
	p.stopLocked()
	return 0, errors.New("llama-server did not start in time")
}

// Chat asks a local model. system and user are the two messages; with jsonOut the answer is
// constrained to a JSON object. Requests are serialised: one summary at a time.
func (e *Engine) Chat(ctx context.Context, llmID, system, user string, jsonOut bool) (string, error) {
	m := FindLLM(llmID)
	if m == nil || !e.LLMInstalled() || !e.HasLLM(m) {
		return "", ErrNotReady
	}
	p := &llama
	p.mu.Lock()
	defer p.mu.Unlock()
	port, err := e.ensureLlama(ctx, m)
	if err != nil {
		return "", err
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	defer func() {
		p.timer = time.AfterFunc(IdleStop, func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.stopLocked()
		})
	}()
	body := map[string]any{
		"messages":    []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"temperature": 0.2, "max_tokens": 1200, "stream": false,
	}
	if jsonOut {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	b, _ := json.Marshal(body)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama-server: %s: %s", resp.Status, lastLine(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", errors.New("llama-server: no answer")
	}
	return out.Choices[0].Message.Content, nil
}
