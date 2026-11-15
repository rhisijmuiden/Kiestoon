package media

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Spool gives access to the Asterisk spool directory (voicemail and recordings). The portal runs
// in the asterisk group, and Asterisk writes with umask 0007, so the portal can manage the files.
type Spool struct {
	Dir string // <DataDir>/asterisk/spool
	mu  sync.Mutex
}

// ErrNotFound is returned for a message or recording that does not exist.
var ErrNotFound = errors.New("not found")

var (
	reBox    = regexp.MustCompile(`^[0-9]{3,5}$`)
	reMsg    = regexp.MustCompile(`^msg([0-9]{4})\.txt$`)
	Folders  = []string{"INBOX", "Old", "Work", "Family", "Friends", "Urgent"}
	Greeting = []string{"unavail", "busy", "greet", "temp"}
)

func (s *Spool) box(mailbox string) (string, error) {
	if !reBox.MatchString(mailbox) {
		return "", ErrNotFound
	}
	return filepath.Join(s.Dir, "voicemail", "default", mailbox), nil
}

// Message is one voicemail message.
type Message struct {
	Folder   string `json:"folder"`
	ID       int    `json:"id"`
	CallerID string `json:"callerId"`
	Date     string `json:"date"`
	Duration int    `json:"duration"`
	// Transcript is the text of the message when speech-to-text is on.
	Transcript string `json:"transcript,omitempty"`
}

func readInfo(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

// Messages lists the messages of a mailbox in INBOX and Old, newest first.
func (s *Spool) Messages(mailbox string) ([]Message, error) {
	dir, err := s.box(mailbox)
	if err != nil {
		return nil, err
	}
	out := []Message{}
	for _, folder := range []string{"INBOX", "Old"} {
		entries, _ := os.ReadDir(filepath.Join(dir, folder))
		for _, e := range entries {
			m := reMsg.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			id, _ := strconv.Atoi(m[1])
			info := readInfo(filepath.Join(dir, folder, e.Name()))
			msg := Message{Folder: folder, ID: id, CallerID: info["callerid"]}
			if t, err := strconv.ParseInt(info["origtime"], 10, 64); err == nil {
				msg.Date = time.Unix(t, 0).UTC().Format(time.RFC3339)
			}
			msg.Duration, _ = strconv.Atoi(info["duration"])
			out = append(out, msg)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

func (s *Spool) msgBase(mailbox, folder string, id int) (string, error) {
	dir, err := s.box(mailbox)
	if err != nil || !slices.Contains(Folders, folder) || id < 0 || id > 9999 {
		return "", ErrNotFound
	}
	base := filepath.Join(dir, folder, fmt.Sprintf("msg%04d", id))
	if _, err := os.Stat(base + ".txt"); err != nil {
		return "", ErrNotFound
	}
	return base, nil
}

// MessageAudio returns the path of the WAV file of a message.
func (s *Spool) MessageAudio(mailbox, folder string, id int) (string, error) {
	base, err := s.msgBase(mailbox, folder, id)
	if err != nil {
		return "", err
	}
	for _, ext := range []string{".wav", ".WAV", ".gsm"} {
		if _, err := os.Stat(base + ext); err == nil {
			return base + ext, nil
		}
	}
	return "", ErrNotFound
}

func msgFiles(base string) []string {
	m, _ := filepath.Glob(base + ".*")
	return m
}

// resequence renumbers a folder to msg0000, msg0001 ... as app_voicemail expects.
func resequence(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	ids := []int{}
	for _, e := range entries {
		if m := reMsg.FindStringSubmatch(e.Name()); m != nil {
			id, _ := strconv.Atoi(m[1])
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	for want, id := range ids {
		if id == want {
			continue
		}
		from := filepath.Join(dir, fmt.Sprintf("msg%04d", id))
		to := filepath.Join(dir, fmt.Sprintf("msg%04d", want))
		for _, f := range msgFiles(from) {
			if err := os.Rename(f, to+filepath.Ext(f)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Delete removes a message.
func (s *Spool) Delete(mailbox, folder string, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.msgBase(mailbox, folder, id)
	if err != nil {
		return err
	}
	for _, f := range msgFiles(base) {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return resequence(filepath.Dir(base))
}

// MarkRead moves a message from INBOX to Old.
func (s *Spool) MarkRead(mailbox string, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.msgBase(mailbox, "INBOX", id)
	if err != nil {
		return err
	}
	dir, _ := s.box(mailbox)
	old := filepath.Join(dir, "Old")
	if err := os.MkdirAll(old, 0o2770); err != nil {
		return err
	}
	next := 0
	entries, _ := os.ReadDir(old)
	for _, e := range entries {
		if m := reMsg.FindStringSubmatch(e.Name()); m != nil {
			if n, _ := strconv.Atoi(m[1]); n >= next {
				next = n + 1
			}
		}
	}
	to := filepath.Join(old, fmt.Sprintf("msg%04d", next))
	for _, f := range msgFiles(base) {
		if err := os.Rename(f, to+filepath.Ext(f)); err != nil {
			return err
		}
	}
	return resequence(filepath.Dir(base))
}

// GreetingPath returns the WAV path of a greeting (unavail, busy, greet or temp).
func (s *Spool) GreetingPath(mailbox, kind string) (string, error) {
	dir, err := s.box(mailbox)
	if err != nil || !slices.Contains(Greeting, kind) {
		return "", ErrNotFound
	}
	return filepath.Join(dir, kind+".wav"), nil
}

// SaveGreeting stores a greeting. Other formats of the same greeting are removed so Asterisk plays the new one.
func (s *Spool) SaveGreeting(mailbox, kind string, wav []byte) error {
	p, err := s.GreetingPath(mailbox, kind)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o2770); err != nil {
		return err
	}
	s.DeleteGreeting(mailbox, kind)
	return os.WriteFile(p, wav, 0o660)
}

// DeleteGreeting removes a greeting in every format.
func (s *Spool) DeleteGreeting(mailbox, kind string) {
	p, err := s.GreetingPath(mailbox, kind)
	if err != nil {
		return
	}
	m, _ := filepath.Glob(strings.TrimSuffix(p, ".wav") + ".*")
	for _, f := range m {
		_ = os.Remove(f)
	}
}

// Greetings reports which greetings exist.
func (s *Spool) Greetings(mailbox string) map[string]bool {
	out := map[string]bool{}
	for _, k := range Greeting {
		p, err := s.GreetingPath(mailbox, k)
		if err != nil {
			continue
		}
		m, _ := filepath.Glob(strings.TrimSuffix(p, ".wav") + ".*")
		out[k] = len(m) > 0
	}
	return out
}

// ---- recordings -------------------------------------------------------------

// Recording is a recorded call (kt-<date>-<time>-<uniqueid>-<from>-<to>.wav) or conference
// (kt-conf<number>-<date>-<time>.wav).
type Recording struct {
	Name     string `json:"name"`
	Date     string `json:"date"`
	UniqueID string `json:"uniqueId"`
	From     string `json:"from"`
	To       string `json:"to"`
	Size     int64  `json:"size"`
	Duration int64  `json:"duration"` // seconds, from the file size
}

var reRec = regexp.MustCompile(`^kt-(conf[0-9]+-)?([0-9]{8})-([0-9]{6})(?:-([0-9.]+)-([0-9+]*)-([0-9+*#q]*))?\.wav$`)

func (s *Spool) monitor() string { return filepath.Join(s.Dir, "monitor") }

// ParseRecording reads the details from a recording's file name.
func ParseRecording(name string, size int64, loc *time.Location) (Recording, bool) {
	m := reRec.FindStringSubmatch(name)
	if m == nil {
		return Recording{}, false
	}
	r := Recording{Name: name, Size: size, UniqueID: m[4], From: m[5], To: m[6], Duration: max(0, (size-44)/16000)}
	if m[1] != "" {
		r.To = "conf" + strings.TrimSuffix(strings.TrimPrefix(m[1], "conf"), "-")
	}
	if t, err := time.ParseInLocation("20060102150405", m[2]+m[3], loc); err == nil {
		r.Date = t.UTC().Format(time.RFC3339)
	}
	return r, true
}

// Recordings lists recordings, newest first. keep filters them (nil: all).
func (s *Spool) Recordings(loc *time.Location, keep func(Recording) bool) []Recording {
	entries, _ := os.ReadDir(s.monitor())
	out := []Recording{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		r, ok := ParseRecording(e.Name(), info.Size(), loc)
		if !ok || (keep != nil && !keep(r)) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out
}

// RecordingPath returns the path of a recording by name.
func (s *Spool) RecordingPath(name string) (string, error) {
	if !reRec.MatchString(name) {
		return "", ErrNotFound
	}
	p := filepath.Join(s.monitor(), name)
	if _, err := os.Stat(p); err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

// DeleteRecording removes a recording.
func (s *Spool) DeleteRecording(name string) error {
	p, err := s.RecordingPath(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// PurgeRecordings removes recordings older than days and returns how many were removed.
func (s *Spool) PurgeRecordings(days int, now time.Time) int {
	if days <= 0 {
		return 0
	}
	cut := now.AddDate(0, 0, -days)
	entries, _ := os.ReadDir(s.monitor())
	n := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || !strings.HasPrefix(e.Name(), "kt-") {
			continue
		}
		if info.ModTime().Before(cut) && os.Remove(filepath.Join(s.monitor(), e.Name())) == nil {
			n++
		}
	}
	return n
}
