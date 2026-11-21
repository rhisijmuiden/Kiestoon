package pbx

import (
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// A wallboard design is what an admin composes in the wallboard designer: pages of tiles on a
// grid, and the house style of the screen. The browser draws it; the server only checks it.

// Grid sizes per screen format.
const (
	FormatLandscape = "landscape"
	FormatPortrait  = "portrait"
)

// GridSize returns the columns and rows of the grid of a screen format.
func GridSize(format string) (cols, rows int) {
	if format == FormatPortrait {
		return 6, 12
	}
	return 12, 8
}

// Tile types of the wallboard designer.
var wallTileTypes = []string{
	"kpi", "gauge", "trend", "queue", "callers", "compare", "callbacks",
	"agents", "agentTiles", "leaderboard", "hourly", "donut",
	"clock", "text", "ticker", "image", "alerts",
}

// Metrics a figure tile can show. Live metrics are "now"; the others are counted over a period.
var wallMetrics = []string{
	"waiting", "longestWait", "callbacks", "free", "busy", "paused",
	"offered", "answered", "abandoned", "abandonedPct", "serviceLevel", "avgWait", "avgTalk", "maxWait",
}

var wallPeriods = []string{"today", "hour", "yesterday", "week"}

// WallTile is one tile on a page of the designer.
type WallTile struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	// Title replaces the default title of the tile; empty = default.
	Title  string  `json:"title"`
	Metric string  `json:"metric"`
	Queues []int64 `json:"queues"`
	Period string  `json:"period"`
	// Warn and Alarm are the thresholds of a figure (0 = not set); FromSLA takes them from the queue SLA.
	Warn    float64 `json:"warn"`
	Alarm   float64 `json:"alarm"`
	FromSLA bool    `json:"fromSla"`
	// Display is big, trend or gauge for a figure tile.
	Display   string `json:"display"`
	Blink     bool   `json:"blink"`
	HideLabel bool   `json:"hideLabel"`
	Compare   bool   `json:"compare"`
	// Text is the text of a text or ticker tile.
	Text string `json:"text"`
	// Image is a data: URL for an image tile.
	Image string `json:"image"`
}

// WallPage is one page of a screen; the pages rotate.
type WallPage struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Seconds int        `json:"seconds"`
	Tiles   []WallTile `json:"tiles"`
}

// WallStyle is the house style of one screen.
type WallStyle struct {
	// Theme is dark, light or auto (light from 07:00 to 19:00).
	Theme  string `json:"theme"`
	Accent string `json:"accent"`
	// Logo is kiestoon, own or none; LogoImage is the own logo as a data: URL.
	Logo       string `json:"logo"`
	LogoImage  string `json:"logoImage"`
	Background string `json:"background"`
	Tiles      string `json:"tiles"`
	Scale      int    `json:"scale"`
	Density    string `json:"density"`
	Clock      bool   `json:"clock"`
	Date       bool   `json:"date"`
	Name       bool   `json:"name"`
	Dots       bool   `json:"dots"`
	// Language is nl, en or browser.
	Language    string `json:"language"`
	AlarmBanner bool   `json:"alarmBanner"`
	AlarmSound  bool   `json:"alarmSound"`
	AlarmJump   bool   `json:"alarmJump"`
}

// WallDesign is the full design of a screen.
type WallDesign struct {
	Format string     `json:"format"`
	Pages  []WallPage `json:"pages"`
	Style  WallStyle  `json:"style"`
}

var (
	wallIDRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,24}$`)
	accentRe     = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	slugRe       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])$`)
	imagePrefixs = []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/webp;base64,", "data:image/svg+xml;base64,"}
)

// maxWallImage is the largest image (as a data: URL) a design may hold.
const maxWallImage = 400 << 10

func validImage(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > maxWallImage {
		return false
	}
	for _, p := range imagePrefixs {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func oneOf(v string, list []string, def string) string {
	if slices.Contains(list, v) {
		return v
	}
	return def
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(hi, v))
}

// normalize checks a design and fixes what can be fixed (sizes that fall off the grid).
func (d *WallDesign) normalize() error {
	d.Format = oneOf(d.Format, []string{FormatLandscape, FormatPortrait}, FormatLandscape)
	cols, rows := GridSize(d.Format)
	if len(d.Pages) == 0 {
		return invalid("required", "pages")
	}
	if len(d.Pages) > 12 {
		return invalid("too_many", "pages")
	}
	ids := map[string]bool{}
	for pi := range d.Pages {
		p := &d.Pages[pi]
		if !wallIDRe.MatchString(p.ID) || ids[p.ID] {
			return invalid("invalid", "pages")
		}
		ids[p.ID] = true
		p.Name = strings.TrimSpace(p.Name)
		if len([]rune(p.Name)) > 60 {
			return invalid("too_long", "pageName")
		}
		if p.Seconds == 0 {
			p.Seconds = 30
		}
		p.Seconds = clampInt(p.Seconds, 5, 600)
		if p.Tiles == nil {
			p.Tiles = []WallTile{}
		}
		if len(p.Tiles) > 48 {
			return invalid("too_many", "tiles")
		}
		for ti := range p.Tiles {
			t := &p.Tiles[ti]
			if !wallIDRe.MatchString(t.ID) || ids[t.ID] {
				return invalid("invalid", "tiles")
			}
			ids[t.ID] = true
			if !slices.Contains(wallTileTypes, t.Type) {
				return invalid("invalid", "tileType")
			}
			t.W = clampInt(t.W, 1, cols)
			t.H = clampInt(t.H, 1, rows)
			t.X = clampInt(t.X, 0, cols-t.W)
			t.Y = clampInt(t.Y, 0, rows-t.H)
			t.Title = strings.TrimSpace(t.Title)
			if len([]rune(t.Title)) > 60 {
				return invalid("too_long", "tileTitle")
			}
			t.Metric = oneOf(t.Metric, wallMetrics, "")
			t.Period = oneOf(t.Period, wallPeriods, "today")
			t.Display = oneOf(t.Display, []string{"big", "trend", "gauge"}, "big")
			if t.Queues == nil {
				t.Queues = []int64{}
			}
			if len(t.Queues) > 50 {
				return invalid("invalid", "queues")
			}
			if t.Warn < 0 || t.Alarm < 0 || t.Warn > 1e6 || t.Alarm > 1e6 {
				return invalid("invalid", "thresholds")
			}
			if len([]rune(t.Text)) > 1000 {
				return invalid("too_long", "text")
			}
			if !validImage(t.Image) {
				return invalid("invalid", "image")
			}
		}
	}
	s := &d.Style
	s.Theme = oneOf(s.Theme, []string{"dark", "light", "auto"}, "dark")
	if s.Accent != "" && !accentRe.MatchString(s.Accent) {
		return invalid("invalid", "accent")
	}
	s.Accent = strings.ToUpper(s.Accent)
	s.Logo = oneOf(s.Logo, []string{"kiestoon", "own", "none"}, "kiestoon")
	if !validImage(s.LogoImage) {
		return invalid("invalid", "logoImage")
	}
	s.Background = oneOf(s.Background, []string{"glow", "grid", "plain"}, "glow")
	s.Tiles = oneOf(s.Tiles, []string{"glass", "solid", "outlined"}, "glass")
	if s.Scale == 0 {
		s.Scale = 100
	}
	s.Scale = clampInt(s.Scale, 60, 180)
	s.Density = oneOf(s.Density, []string{"roomy", "normal", "compact"}, "normal")
	s.Language = oneOf(s.Language, []string{"nl", "en", "browser"}, "browser")
	return nil
}

// Access to a screen by its own address /wall/<slug>.
const (
	// AccessLink: only the secret link /wall/<key> works.
	AccessLink = "link"
	// AccessOpen: anyone who can reach the address sees the screen (limit it with networks).
	AccessOpen = "open"
	// AccessPin: the screen asks for a pin code once; the tv then remembers it.
	AccessPin = "pin"
)

// Slugify turns a screen name into an address: lower case letters, digits and dashes.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if len(s) < 2 {
		s = "wallboard"
	}
	return s
}

// ParseNetworks checks a list of networks (1.2.3.0/24 or a single address).
func ParseNetworks(list []string) ([]netip.Prefix, error) {
	out := []netip.Prefix{}
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if p, err := netip.ParsePrefix(v); err == nil {
			out = append(out, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(v); err == nil {
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
			continue
		}
		return nil, invalid("invalid", "networks")
	}
	if len(out) > 20 {
		return nil, invalid("too_many", "networks")
	}
	return out, nil
}

// Allows reports whether a client address may see the screen.
func (w *WallScreen) Allows(ip string) bool {
	if len(w.Networks) == 0 {
		return true
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	nets, _ := ParseNetworks(w.Networks)
	for _, p := range nets {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
