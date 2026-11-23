// Package logs reads the log files of the phone system for the log viewer and explains why a
// trunk does not register.
package logs

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
)

// Entry is one log line.
type Entry struct {
	At     string `json:"at"`    // "2026-11-23 08:40:01" as written in the file, "" when the line has none
	Level  string `json:"level"` // error, warning, notice, security, info, debug
	Module string `json:"module,omitempty"`
	Text   string `json:"text"`
}

// Query filters entries. Words in Q must all appear (case-insensitive); Level is the lowest
// level shown ("" shows everything).
type Query struct {
	Q     string
	Level string
	Limit int
}

// MaxLimit caps how many lines one request returns.
const MaxLimit = 2000

// tailBytes is how much of the end of a log file is read; older lines are in the download.
const tailBytes = 4 << 20

var rank = map[string]int{"debug": 0, "info": 1, "notice": 2, "security": 2, "warning": 3, "error": 4}

// Asterisk: "[2026-11-23 08:40:01] WARNING[1234][C-00000001] res_pjsip_outbound_registration.c: text"
var reAsterisk = regexp.MustCompile(`^\[([^\]]+)\]\s+([A-Z]+)\[\d+\](?:\[[^\]]*\])?\s*(?:([\w./-]+\.c):\s*)?(.*)$`)

// ParseAsterisk turns one Asterisk log line into an entry.
func ParseAsterisk(line string) Entry {
	m := reAsterisk.FindStringSubmatch(line)
	if m == nil {
		return Entry{Level: "info", Text: line}
	}
	lvl := strings.ToLower(m[2])
	switch lvl {
	case "verbose", "dtmf":
		lvl = "info"
	}
	if _, ok := rank[lvl]; !ok {
		lvl = "info"
	}
	return Entry{At: m[1], Level: lvl, Module: strings.TrimSuffix(m[3], ".c"), Text: m[4]}
}

// Journal (journalctl -o short-iso): "2026-11-23T08:40:01+01:00 host kiestoon[123]: time=... level=WARN msg=..."
var reJournal = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)\S*\s+\S+\s+([\w.@-]+)(?:\[\d+\])?:\s?(.*)$`)
var reSlogLevel = regexp.MustCompile(`\blevel=(\w+)`)

// ParseJournal turns one journal line into an entry.
func ParseJournal(line string) Entry {
	m := reJournal.FindStringSubmatch(line)
	if m == nil {
		return Entry{Level: "info", Text: line}
	}
	e := Entry{At: strings.Replace(m[1], "T", " ", 1), Level: "info", Module: m[2], Text: m[3]}
	if l := reSlogLevel.FindStringSubmatch(m[3]); l != nil {
		switch strings.ToUpper(l[1]) {
		case "ERROR":
			e.Level = "error"
		case "WARN", "WARNING":
			e.Level = "warning"
		case "DEBUG":
			e.Level = "debug"
		}
	} else {
		low := strings.ToLower(m[3])
		switch {
		case strings.Contains(low, "error") || strings.Contains(low, "failed"):
			e.Level = "error"
		case strings.Contains(low, "warning"):
			e.Level = "warning"
		}
	}
	return e
}

// TailLines returns the last lines of a file, reading at most tailBytes, and when the file
// is short also the end of its rotated copy (file.1), oldest first.
func TailLines(path string) ([]string, error) {
	cur, err := tail(path, tailBytes)
	if err != nil {
		return nil, err
	}
	if len(cur) < tailBytes/4 {
		if old, err := tail(path+".1", tailBytes-len(cur)); err == nil {
			cur = append(old, cur...)
		}
	}
	return splitLines(cur), nil
}

func tail(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	off := st.Size() - int64(n)
	cut := off > 0
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(n)))
	if err != nil {
		return nil, err
	}
	if cut {
		// Drop the partial first line.
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return b, nil
}

func splitLines(b []byte) []string {
	out := []string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if l := strings.TrimRight(sc.Text(), "\r"); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Filter parses lines (oldest first) and returns the matching entries newest first, at most q.Limit.
// A line without its own time stamp (a continuation) belongs to the entry before it.
func Filter(lines []string, parse func(string) Entry, q Query) []Entry {
	limit := q.Limit
	if limit <= 0 || limit > MaxLimit {
		limit = 500
	}
	words := strings.Fields(strings.ToLower(q.Q))
	minRank := rank[strings.ToLower(q.Level)]
	entries := make([]Entry, 0, len(lines))
	for _, l := range lines {
		e := parse(l)
		if e.At == "" && len(entries) > 0 {
			entries[len(entries)-1].Text += "\n" + e.Text
			continue
		}
		entries = append(entries, e)
	}
	out := []Entry{}
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if q.Level != "" && rank[e.Level] < minRank {
			continue
		}
		if len(words) > 0 {
			hay := strings.ToLower(e.Module + " " + e.Text)
			ok := true
			for _, w := range words {
				if !strings.Contains(hay, w) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}
