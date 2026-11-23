// Package asterisk imports a plain Asterisk system from its configuration files: pjsip.conf or
// sip.conf (phones and trunks), voicemail.conf, queues.conf, extensions.conf (dialplan: ring
// groups, voice menus, time conditions, inbound numbers) and the sounds the dialplan plays.
//
// A dialplan is a program, so only the common patterns are recognised: Dial to one or more phones,
// Queue, VoiceMail, Background with WaitExten and digit extensions (a voice menu), GotoIfTime (a
// time condition), Playback (an announcement) and Goto between them. What is not recognised is
// listed as a note.
package asterisk

import (
	"bufio"
	"bytes"
	"path"
	"slices"
	"strings"
)

// Section is one [section] of an Asterisk configuration file. Asterisk allows the same name more
// than once (pjsip.conf has an endpoint, an aor and an auth called "100"), so a file is a list.
type Section struct {
	Name      string
	Template  bool     // [name](!)
	Inherits  []string // [name](tpl1,tpl2)
	Keys      []KV
	File      string
	templates []*Section
}

// KV is one line "key = value" or "key => value".
type KV struct {
	Key   string
	Value string
}

// Get returns the last value of key (including inherited templates), "" when not set.
func (s *Section) Get(key string) string {
	v, _ := s.Lookup(key)
	return v
}

// Lookup returns the last value of key and whether it is set.
func (s *Section) Lookup(key string) (string, bool) {
	key = strings.ToLower(key)
	for i := len(s.Keys) - 1; i >= 0; i-- {
		if s.Keys[i].Key == key {
			return s.Keys[i].Value, true
		}
	}
	for i := len(s.templates) - 1; i >= 0; i-- {
		if v, ok := s.templates[i].Lookup(key); ok {
			return v, true
		}
	}
	return "", false
}

// All returns every value of key, templates first.
func (s *Section) All(key string) []string {
	key = strings.ToLower(key)
	var out []string
	for _, t := range s.templates {
		out = append(out, t.All(key)...)
	}
	for _, kv := range s.Keys {
		if kv.Key == key {
			out = append(out, kv.Value)
		}
	}
	return out
}

// Config is a parsed configuration file with its includes.
type Config struct {
	Sections []*Section
	// Global holds the lines before the first section.
	Global []KV
}

// Named returns the non-template sections called name.
func (c *Config) Named(name string) []*Section {
	var out []*Section
	for _, s := range c.Sections {
		if s.Name == name && !s.Template {
			out = append(out, s)
		}
	}
	return out
}

// Files are the configuration files by base name (for #include).
type Files map[string][]byte

// Parse reads a configuration file and the files it includes.
func Parse(name string, files Files) *Config {
	c := &Config{}
	var cur *Section
	seen := map[string]bool{}
	var read func(name string, depth int)
	read = func(name string, depth int) {
		base := path.Base(name)
		if depth > 8 || seen[base] {
			return
		}
		seen[base] = true
		data, ok := files[base]
		if !ok {
			return
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		inComment := false
		for sc.Scan() {
			line := sc.Text()
			// Block comments ;-- ... --;
			if inComment {
				if i := strings.Index(line, "--;"); i >= 0 {
					line, inComment = line[i+3:], false
				} else {
					continue
				}
			}
			if i := strings.Index(line, ";--"); i >= 0 {
				if j := strings.Index(line[i:], "--;"); j >= 0 {
					line = line[:i] + line[i+j+3:]
				} else {
					line, inComment = line[:i], true
				}
			}
			line = strings.TrimSpace(stripComment(line))
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "#include") || strings.HasPrefix(line, "#tryinclude") {
				f := strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "#tryinclude"), "#include")), `"<> `)
				if strings.Contains(f, "*") {
					// #include pjsip.*.conf: every matching file
					for _, cand := range files.glob(f) {
						read(cand, depth+1)
					}
					continue
				}
				read(f, depth+1)
				continue
			}
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "[") {
				end := strings.Index(line, "]")
				if end < 0 {
					continue
				}
				cur = &Section{Name: strings.TrimSpace(line[1:end]), File: base}
				rest := strings.TrimSpace(line[end+1:])
				if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
					for _, t := range strings.Split(rest[1:len(rest)-1], ",") {
						t = strings.TrimSpace(t)
						switch {
						case t == "!":
							cur.Template = true
						case t == "+":
							// [name](+) adds to an earlier section of the same name
							for i := len(c.Sections) - 1; i >= 0; i-- {
								if c.Sections[i].Name == cur.Name {
									cur = c.Sections[i]
									break
								}
							}
						case t != "":
							cur.Inherits = append(cur.Inherits, t)
						}
					}
				}
				if !containsSection(c.Sections, cur) {
					c.Sections = append(c.Sections, cur)
				}
				continue
			}
			kv, ok := splitKV(line)
			if !ok {
				continue
			}
			if cur == nil {
				c.Global = append(c.Global, kv)
			} else {
				cur.Keys = append(cur.Keys, kv)
			}
		}
	}
	read(name, 0)
	for _, s := range c.Sections {
		for _, t := range s.Inherits {
			for _, x := range c.Sections {
				if x.Name == t && x != s {
					s.templates = append(s.templates, x)
					break
				}
			}
		}
	}
	return c
}

// glob returns the files matching a wildcard include, sorted.
func (f Files) glob(pattern string) []string {
	var out []string
	pattern = path.Base(pattern)
	for n := range f {
		if ok, _ := path.Match(pattern, n); ok {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

func containsSection(list []*Section, s *Section) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// stripComment removes a ; comment; "\;" is a literal semicolon.
func stripComment(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == ';' && (i == 0 || line[i-1] != '\\') {
			return line[:i]
		}
	}
	return line
}

func splitKV(line string) (KV, bool) {
	i := strings.IndexByte(line, '=')
	if i <= 0 {
		return KV{}, false
	}
	k := strings.TrimSpace(line[:i])
	v := line[i+1:]
	if strings.HasPrefix(v, ">") {
		v = v[1:]
	}
	k = strings.TrimSuffix(k, "+") // key += value
	return KV{Key: strings.ToLower(strings.TrimSpace(k)), Value: strings.TrimSpace(strings.ReplaceAll(v, `\;`, ";"))}, true
}
