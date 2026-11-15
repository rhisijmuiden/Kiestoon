// Package updates checks GitHub releases for new versions and hands update requests to the
// root-owned updater service.
//
// The portal runs as the unprivileged "kiestoon" user and cannot install packages itself. Instead it
// writes update-request.json into the data directory. A systemd path unit (kiestoon-updater.path)
// notices the file and runs /usr/lib/kiestoon/kiestoon-updater as root, which makes a backup,
// installs the .deb from the release and writes update-status.json.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

// Channels a system can follow.
const (
	ChannelProduction = "production"
	ChannelTesting    = "testing"
)

// ValidChannel reports whether c is a known channel.
func ValidChannel(c string) bool { return c == ChannelProduction || c == ChannelTesting }

// Release is the part of a GitHub release the portal needs.
type Release struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"publishedAt"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
}

// CheckResult is what the Info dialog shows after "Check for updates".
type CheckResult struct {
	Channel         string   `json:"channel"`
	Current         string   `json:"current"`
	Latest          *Release `json:"latest"`
	UpdateAvailable bool     `json:"updateAvailable"`
	// Downgrade is true when the newest release on the channel is older than the running version,
	// which happens after switching from testing back to production.
	Downgrade bool      `json:"downgrade"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Request is written for the updater service.
type Request struct {
	Version     string    `json:"version"`
	Channel     string    `json:"channel"`
	RequestedAt time.Time `json:"requestedAt"`
	RequestedBy string    `json:"requestedBy"`
}

// Status is written by the updater service while and after it runs.
type Status struct {
	State     string    `json:"state"` // idle, queued, running, success, failed
	Version   string    `json:"version,omitempty"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// Checker fetches releases from GitHub, with a short cache so the API rate limit is never hit.
type Checker struct {
	Repo    string // owner/name
	DataDir string
	Client  *http.Client
	BaseURL string // https://api.github.com, overridable in tests

	mu       sync.Mutex
	cached   []Release
	cachedAt time.Time
}

// NewChecker returns a checker for the given repository.
func NewChecker(repo, dataDir string) *Checker {
	return &Checker{Repo: repo, DataDir: dataDir, Client: &http.Client{Timeout: 15 * time.Second}, BaseURL: "https://api.github.com"}
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
}

func (c *Checker) releases(ctx context.Context, force bool) ([]Release, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.cached != nil && time.Since(c.cachedAt) < 5*time.Minute {
		return c.cached, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases?per_page=50", c.BaseURL, c.Repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kiestoon-updater")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contact GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var raw []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("read releases: %w", err)
	}
	out := make([]Release, 0, len(raw))
	for _, r := range raw {
		if r.Draft || !semver.IsValid(r.TagName) {
			continue
		}
		out = append(out, Release{Version: r.TagName, Name: r.Name, Prerelease: r.Prerelease, PublishedAt: r.PublishedAt, Notes: r.Body, URL: r.HTMLURL})
	}
	c.cached, c.cachedAt = out, time.Now()
	return out, nil
}

// Latest returns the newest release on a channel: production only sees stable releases,
// testing sees release candidates too.
func Latest(rels []Release, channel string) *Release {
	var best *Release
	for i := range rels {
		r := &rels[i]
		if channel == ChannelProduction && (r.Prerelease || semver.Prerelease(r.Version) != "") {
			continue
		}
		if best == nil || semver.Compare(r.Version, best.Version) > 0 {
			best = r
		}
	}
	return best
}

// Check compares the running version with the newest release on the channel.
func (c *Checker) Check(ctx context.Context, current, channel string, force bool) (*CheckResult, error) {
	rels, err := c.releases(ctx, force)
	if err != nil {
		return nil, err
	}
	res := &CheckResult{Channel: channel, Current: current, Latest: Latest(rels, channel), CheckedAt: time.Now().UTC()}
	if res.Latest != nil {
		cmp := 1
		if semver.IsValid(current) {
			cmp = semver.Compare(res.Latest.Version, current)
		}
		res.UpdateAvailable = cmp > 0
		res.Downgrade = cmp < 0
	}
	return res, nil
}

// Find returns the release with the given version on the channel, so only real releases can be installed.
func (c *Checker) Find(ctx context.Context, version, channel string) (*Release, error) {
	rels, err := c.releases(ctx, false)
	if err != nil {
		return nil, err
	}
	for i := range rels {
		r := rels[i]
		if r.Version != version {
			continue
		}
		if channel == ChannelProduction && (r.Prerelease || semver.Prerelease(r.Version) != "") {
			return nil, errors.New("this version is not available on the production channel")
		}
		return &r, nil
	}
	return nil, errors.New("unknown version")
}

func (c *Checker) requestPath() string { return filepath.Join(c.DataDir, "update-request.json") }
func (c *Checker) statusPath() string  { return filepath.Join(c.DataDir, "update-status.json") }

// ErrBusy means an update is already queued or running.
var ErrBusy = errors.New("an update is already in progress")

// RequestUpdate queues an update for the updater service.
func (c *Checker) RequestUpdate(r Request) error {
	if st := c.Status(); st.State == "queued" || st.State == "running" {
		if time.Since(st.UpdatedAt) < 30*time.Minute {
			return ErrBusy
		}
	}
	if err := writeJSON(c.statusPath(), Status{State: "queued", Version: r.Version, UpdatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return writeJSON(c.requestPath(), r)
}

// Status returns the last status the updater wrote.
func (c *Checker) Status() Status {
	b, err := os.ReadFile(c.statusPath())
	if err != nil {
		return Status{State: "idle"}
	}
	var st Status
	if json.Unmarshal(b, &st) != nil || st.State == "" {
		return Status{State: "idle"}
	}
	return st
}

// writeJSON writes atomically, so the path unit never sees a half-written file.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
