package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/firmware"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Firmware watch: Kiestoon looks up the newest firmware for the phone models in use (Grandstream's
// public firmware page, and the Kiestoon firmware catalog for the other vendors), shows which
// phones run an older version, and downloads and rolls it out on request or by itself every night.

const (
	fwModeOff    = "off"
	fwModeNotify = "notify" // check every night, roll out after confirmation
	fwModeAuto   = "auto"   // check every night and roll out by itself
)

// fwReleases finds the newest firmware; tests replace it.
var fwReleases = func(ctx context.Context, catalogURL string) ([]firmware.Release, []string) {
	f := &firmware.Finder{CatalogURL: catalogURL}
	return f.Releases(ctx)
}

// fwCache keeps the last lookup, so opening the page does not ask the vendors each time.
var fwCache struct {
	sync.Mutex
	at       time.Time
	url      string
	releases []firmware.Release
	errors   []string
}

// fwInstall lets one firmware download run at a time.
var fwInstall sync.Mutex

const fwCacheFor = 6 * time.Hour

type fwWatchSettings struct {
	Mode       string `json:"mode"`
	CatalogURL string `json:"catalogUrl"`
}

func (s *Server) fwSettings(ctx context.Context) fwWatchSettings {
	mode, _ := store.GetSetting(ctx, s.DB, "firmware_watch_mode", fwModeNotify)
	u, _ := store.GetSetting(ctx, s.DB, "firmware_catalog_url", "")
	return fwWatchSettings{Mode: mode, CatalogURL: u}
}

func (s *Server) fwLookup(ctx context.Context, refresh bool) ([]firmware.Release, []string, time.Time) {
	cat := s.fwSettings(ctx).CatalogURL
	fwCache.Lock()
	defer fwCache.Unlock()
	if refresh || fwCache.at.IsZero() || time.Since(fwCache.at) > fwCacheFor || fwCache.url != cat {
		lctx, cancel := context.WithTimeout(ctx, time.Minute)
		rs, errs := fwReleases(lctx, cat)
		cancel()
		fwCache.at, fwCache.url, fwCache.releases, fwCache.errors = time.Now(), cat, rs, errs
		for _, e := range errs {
			s.Log.Warn("firmware check", "err", e)
		}
	}
	return fwCache.releases, fwCache.errors, fwCache.at
}

type fwVersionCount struct {
	Version string `json:"version"`
	Phones  int    `json:"phones"`
}

// fwModel is the firmware state of one phone model in use.
type fwModel struct {
	Vendor   string             `json:"vendor"`
	Model    string             `json:"model"`
	Name     string             `json:"name"`
	Phones   int                `json:"phones"`
	Behind   int                `json:"behind"`
	Versions []fwVersionCount   `json:"versions"`
	Latest   *firmware.Release  `json:"latest"`
	Active   *firmware.Firmware `json:"active"`
	// State: current, update (newer firmware available), rolling (rolled out, phones not updated
	// yet) or nosource (no source knows this model).
	State string `json:"state"`
}

// fwModels lists the phone models in use with their versions and the newest firmware.
func (s *Server) fwModels(ctx context.Context, rs []firmware.Release) ([]fwModel, error) {
	phones, err := s.PBX.Phones(ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*fwModel{}
	versions := map[string]map[string]int{}
	for _, p := range phones {
		if !slices.Contains(provision.FirmwareVendors, p.Vendor) || p.Model == "" {
			continue
		}
		k := p.Vendor + "/" + p.Model
		m := byKey[k]
		if m == nil {
			name := p.Model
			if pm := provision.FindModel(p.Vendor, p.Model); pm != nil {
				name = pm.Name
			}
			m = &fwModel{Vendor: p.Vendor, Model: p.Model, Name: name, Versions: []fwVersionCount{}}
			byKey[k] = m
			versions[k] = map[string]int{}
		}
		m.Phones++
		versions[k][firmware.VersionFromUserAgent(p.UserAgent)]++
	}
	out := []fwModel{}
	for k, m := range byKey {
		m.Latest = firmware.Latest(rs, m.Vendor, m.Model)
		m.Active = s.Firmware.ForModel(ctx, m.Vendor, m.Model)
		for v, n := range versions[k] {
			m.Versions = append(m.Versions, fwVersionCount{v, n})
			if v != "" && m.Latest != nil && firmware.CompareVersions(v, m.Latest.Version) < 0 {
				m.Behind += n
			}
		}
		sort.Slice(m.Versions, func(i, j int) bool { return firmware.CompareVersions(m.Versions[i].Version, m.Versions[j].Version) > 0 })
		switch {
		case m.Latest == nil:
			m.State = "nosource"
		case m.Active != nil && firmware.CompareVersions(m.Active.Version, m.Latest.Version) >= 0:
			m.State = "current"
			if m.Behind > 0 {
				m.State = "rolling"
			}
		case m.Behind > 0 || versions[k][""] == m.Phones:
			m.State = "update"
		default:
			m.State = "current"
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Vendor != out[j].Vendor {
			return out[i].Vendor < out[j].Vendor
		}
		return out[i].Model < out[j].Model
	})
	return out, nil
}

func (s *Server) firmwareLatest(w http.ResponseWriter, r *http.Request) {
	rs, errs, at := s.fwLookup(r.Context(), r.URL.Query().Get("refresh") == "1")
	models, err := s.fwModels(r.Context(), rs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	set := s.fwSettings(r.Context())
	if errs == nil {
		errs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode": set.Mode, "catalogUrl": set.CatalogURL, "defaultCatalogUrl": firmware.DefaultCatalogURL,
		"checkedAt": at.UTC().Format(time.RFC3339), "errors": errs, "models": models,
	})
}

func (s *Server) firmwareWatchSave(w http.ResponseWriter, r *http.Request) {
	var req fwWatchSettings
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !slices.Contains([]string{fwModeOff, fwModeNotify, fwModeAuto}, req.Mode) {
		fieldErr(w, "mode")
		return
	}
	req.CatalogURL = strings.TrimSpace(req.CatalogURL)
	if req.CatalogURL == firmware.DefaultCatalogURL {
		req.CatalogURL = ""
	}
	if req.CatalogURL != "" && !firmware.ValidURL(req.CatalogURL) {
		fieldErr(w, "catalogUrl")
		return
	}
	if err := store.SetSetting(r.Context(), s.DB, "firmware_watch_mode", req.Mode); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	_ = store.SetSetting(r.Context(), s.DB, "firmware_catalog_url", req.CatalogURL)
	s.audit(r, "firmware.watch_saved", "mode="+req.Mode)
	writeJSON(w, http.StatusOK, req)
}

// firmwareLatestInstall downloads the newest firmware for a model and rolls it out.
func (s *Server) firmwareLatestInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Vendor string `json:"vendor"`
		Model  string `json:"model"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	rs, _, _ := s.fwLookup(r.Context(), false)
	rel := firmware.Latest(rs, req.Vendor, req.Model)
	if rel == nil {
		writeErr(w, http.StatusNotFound, "no_firmware_source")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(31 * time.Minute))
	f, err := s.fwRollOut(ctx, rs, rel)
	if err != nil {
		s.Log.Warn("firmware install", "url", rel.URL, "err", err)
		code := "download_failed"
		if errors.Is(err, firmware.ErrChecksum) {
			code = "checksum_mismatch"
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": code, "message": err.Error()})
		return
	}
	s.audit(r, "firmware.rolled_out", fmt.Sprintf("%s %s %v", f.Vendor, f.Version, f.Models))
	writeJSON(w, http.StatusOK, f)
}

// fwRollOut downloads a release (unless it is already stored) and activates it for every phone
// model in use that it is the newest firmware for.
func (s *Server) fwRollOut(ctx context.Context, rs []firmware.Release, rel *firmware.Release) (*firmware.Firmware, error) {
	fwInstall.Lock()
	defer fwInstall.Unlock()
	f := s.Firmware.FindVersion(ctx, rel.Vendor, rel.Version)
	if f == nil {
		var err error
		if f, err = s.downloadFirmware(ctx, rel.Vendor, rel.URL, rel.SHA256); err != nil {
			return nil, err
		}
		f.Version = rel.Version
	}
	models, err := s.fwModels(ctx, rs)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		if m.Vendor == rel.Vendor && m.Latest != nil && m.Latest.URL == rel.URL && !slices.Contains(f.Models, m.Model) {
			f.Models = append(f.Models, m.Model)
		}
	}
	f.Active = true
	if err := s.Firmware.Update(ctx, f, func(vendor, model string) bool { return provision.FindModel(vendor, model) != nil }); err != nil {
		return nil, err
	}
	store.Audit(ctx, s.DB, 0, "firmware.rolled_out", fmt.Sprintf("%s %s %v", f.Vendor, f.Version, f.Models), "")
	return s.Firmware.Get(ctx, f.ID)
}

// downloadFirmware fetches a firmware file from the vendor. A sha256 from the catalog must match
// the downloaded file.
func (s *Server) downloadFirmware(ctx context.Context, vendor, rawURL, sum string) (*firmware.Firmware, error) {
	if !firmware.ValidURL(rawURL) {
		return nil, errors.New("invalid url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, downloadError{err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, downloadError{errors.New(resp.Status)}
	}
	name := firmware.CleanName(path.Base(resp.Request.URL.Path))
	if name == "" {
		name = firmware.CleanName(path.Base(req.URL.Path))
	}
	h := sha256.New()
	f, err := s.Firmware.Add(ctx, vendor, name, io.TeeReader(resp.Body, h))
	if err != nil {
		return nil, err
	}
	if sum != "" && hex.EncodeToString(h.Sum(nil)) != sum {
		_ = s.Firmware.Delete(ctx, f.ID)
		return nil, firmware.ErrChecksum
	}
	return f, nil
}

// downloadError is a failure to reach the vendor, as opposed to a file that cannot be stored.
type downloadError struct{ error }

func (e downloadError) Unwrap() error { return e.error }

func isStoreErr(err error) bool {
	var d downloadError
	return !errors.As(err, &d) && !errors.Is(err, firmware.ErrChecksum)
}

// runFirmwareWatch checks for new firmware every night at 04:00 and, depending on the setting,
// rolls it out or mails the administrator once per new version.
func (s *Server) runFirmwareWatch(ctx context.Context) {
	done := ""
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			now := time.Now().In(s.loc(ctx))
			day := now.Format("2006-01-02")
			if now.Hour() != 4 || done == day {
				continue
			}
			done = day
			s.firmwareWatchOnce(ctx)
		}
	}
}

// firmwareWatchOnce is one nightly check. It returns the models it rolled out or reported.
func (s *Server) firmwareWatchOnce(ctx context.Context) []string {
	set := s.fwSettings(ctx)
	if set.Mode == fwModeOff {
		return nil
	}
	rs, _, _ := s.fwLookup(ctx, true)
	models, err := s.fwModels(ctx, rs)
	if err != nil {
		return nil
	}
	notified := map[string]string{}
	if v, _ := store.GetSetting(ctx, s.DB, "firmware_watch_notified", "{}"); v != "" {
		_ = json.Unmarshal([]byte(v), &notified)
	}
	var lines, done []string
	rolled := map[string]bool{}
	for _, m := range models {
		if m.State != "update" {
			continue
		}
		key := m.Vendor + "/" + m.Model
		if set.Mode == fwModeAuto {
			if rolled[m.Latest.URL] {
				continue // one download covers several models
			}
			rolled[m.Latest.URL] = true
			dctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			f, err := s.fwRollOut(dctx, rs, m.Latest)
			cancel()
			if err != nil {
				s.Log.Warn("firmware auto update", "model", key, "err", err)
				lines = append(lines, fmt.Sprintf("  %s: %s could not be downloaded (%v)", m.Name, m.Latest.Version, err))
				continue
			}
			lines = append(lines, fmt.Sprintf("  %s: %s rolled out to %d phone(s)", m.Name, f.Version, m.Phones))
			done = append(done, key)
			continue
		}
		if notified[key] == m.Latest.Version {
			continue
		}
		notified[key] = m.Latest.Version
		lines = append(lines, fmt.Sprintf("  %s: %s available, %d of %d phone(s) run an older version", m.Name, m.Latest.Version, m.Behind, m.Phones))
		done = append(done, key)
	}
	if b, err := json.Marshal(notified); err == nil {
		_ = store.SetSetting(ctx, s.DB, "firmware_watch_notified", string(b))
	}
	if len(lines) > 0 {
		subject := "Kiestoon: new phone firmware"
		body := "Kiestoon found new firmware for your phones:\n\n" + strings.Join(lines, "\n") + "\n\n"
		if set.Mode == fwModeAuto {
			body += "Phones install it on their next provisioning check, usually at night or after a reboot.\n"
		} else {
			body += "Open Phones > Firmware in Kiestoon to roll it out.\n"
		}
		s.Mail.Notify(ctx, subject, body)
	}
	return done
}
