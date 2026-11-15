package api

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/firmware"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/provision"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
	"github.com/rhisijmuiden/kiestoon/server/internal/ztp"
)

// ---- firmware ---------------------------------------------------------------

// firmwareBase is the download address of firmware files, on the host the phone used.
func (s *Server) firmwareBase(scheme, host string) string {
	return fmt.Sprintf("%s://%s/prov/fw/", scheme, host)
}

// serveFirmware answers /prov/fw/<token>/<file>. No credentials: phones fetch firmware with other
// HTTP clients than their configuration, and not all of them can send a password.
func (s *Server) serveFirmware(w http.ResponseWriter, r *http.Request, ip string) {
	tok, name, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/prov/fw/"), "/")
	p := s.Firmware.Path(tok, name)
	if p == "" {
		s.provLimit.Fail(ip)
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Large files: allow the download more time than the usual write timeout.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Del("Cache-Control")
	s.Log.Info("firmware download", "file", name, "ip", ip)
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// snomFirmware is the firmware pointer a Snom fetches through its firmware_status setting.
func (s *Server) snomFirmware(w http.ResponseWriter, r *http.Request, model string) {
	for _, m := range provision.Models {
		if m.Vendor == "snom" && strings.EqualFold(m.ID, model) {
			if fw := s.Firmware.ForModel(r.Context(), "snom", m.ID); fw != nil {
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				_, _ = w.Write([]byte(provision.SnomFirmware(s.firmwareBase(s.scheme(r), r.Host) + fw.Token + "/" + fw.Filename)))
				return
			}
		}
	}
	http.NotFound(w, r)
}

func (s *Server) firmwareList(w http.ResponseWriter, r *http.Request) {
	list, err := s.Firmware.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"firmware": list, "vendors": provision.FirmwareVendors})
}

func (s *Server) firmwareStored(w http.ResponseWriter, r *http.Request, f *firmware.Firmware, err error) {
	if err != nil {
		switch err.Error() {
		case "filename", "zip", "empty", "too_large":
			fieldErr(w, "file")
		default:
			if strings.HasPrefix(err.Error(), "zip") {
				fieldErr(w, "file")
				return
			}
			s.Log.Warn("firmware upload", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal")
		}
		return
	}
	s.audit(r, "firmware.added", f.Vendor+" "+f.Filename)
	writeJSON(w, http.StatusOK, f)
}

// firmwareUpload takes the file as the request body: POST /api/firmware/upload?vendor=…&name=…
func (s *Server) firmwareUpload(w http.ResponseWriter, r *http.Request) {
	vendor, name := r.URL.Query().Get("vendor"), r.URL.Query().Get("name")
	if !slices.Contains(provision.FirmwareVendors, vendor) {
		fieldErr(w, "vendor")
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(30 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, firmware.MaxSize)
	f, err := s.Firmware.Add(r.Context(), vendor, name, r.Body)
	s.firmwareStored(w, r, f, err)
}

// firmwareFetch downloads a firmware file from the vendor's website.
func (s *Server) firmwareFetch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Vendor string `json:"vendor"`
		URL    string `json:"url"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !slices.Contains(provision.FirmwareVendors, req.Vendor) {
		fieldErr(w, "vendor")
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		fieldErr(w, "url")
		return
	}
	name := path.Base(u.Path)
	if !firmware.SafeName(name) {
		fieldErr(w, "url")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(31 * time.Minute))
	hreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "download_failed", "message": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "download_failed", "message": resp.Status})
		return
	}
	f, err := s.Firmware.Add(ctx, req.Vendor, name, resp.Body)
	s.firmwareStored(w, r, f, err)
}

func (s *Server) firmwareSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string   `json:"version"`
		Models  []string `json:"models"`
		Active  bool     `json:"active"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	f, err := s.Firmware.Get(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	f.Version, f.Models, f.Active = req.Version, req.Models, req.Active
	if err := s.Firmware.Update(r.Context(), f, func(vendor, model string) bool { return provision.FindModel(vendor, model) != nil }); err != nil {
		fieldErr(w, err.Error())
		return
	}
	s.audit(r, "firmware.saved", fmt.Sprintf("%s %s %v active=%v", f.Vendor, f.Filename, f.Models, f.Active))
	f, _ = s.Firmware.Get(r.Context(), f.ID)
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) firmwareDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Firmware.Delete(r.Context(), pathID(r)); err != nil {
		if errors.Is(err, firmware.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "firmware.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- zero-touch provisioning ------------------------------------------------

func (s *Server) ztpSettings(ctx context.Context) ztp.Settings {
	var st ztp.Settings
	_ = store.GetJSON(ctx, s.DB, ztp.SettingsKey, &st)
	if st.Yealink.API == "" {
		st.Yealink.API = ztp.DefaultYealinkAPI
	}
	return st
}

// ztpServer is the provisioning address handed to the redirection services, without credentials.
func (s *Server) ztpServer(ctx context.Context, fallback string) string {
	if st := s.ztpSettings(ctx); st.ServerURL != "" {
		return st.ServerURL
	}
	if base, _ := store.GetSetting(ctx, s.DB, "public_url", ""); base != "" {
		return strings.TrimRight(base, "/") + "/prov/"
	}
	return fallback
}

func (s *Server) ztpGet(w http.ResponseWriter, r *http.Request) {
	st := s.ztpSettings(r.Context())
	st.Yealink.SecretSet, st.Yealink.Secret = st.Yealink.Secret != "", ""
	prov, _ := provision.Load(r.Context(), s.DB)
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": st, "guides": ztp.Guides,
		"effectiveUrl": s.ztpServer(r.Context(), s.publicBase(r)+"/prov/"),
		"username":     prov.Username, "password": prov.Password,
	})
}

func (s *Server) ztpSave(w http.ResponseWriter, r *http.Request) {
	var st ztp.Settings
	if err := decode(r, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	old := s.ztpSettings(r.Context())
	if st.Yealink.Secret == "" && st.Yealink.AccessKey == old.Yealink.AccessKey {
		st.Yealink.Secret = old.Yealink.Secret
	}
	if err := st.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	st.Yealink.SecretSet = false
	if err := store.SetJSON(r.Context(), s.DB, ztp.SettingsKey, st); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "ztp.saved", fmt.Sprintf("yealink=%v", st.Yealink.Enabled))
	s.ztpGet(w, r)
}

// ztpRegister registers one phone at its vendor's redirection service and stores the outcome.
func (s *Server) ztpRegister(ctx context.Context, p *pbx.Phone, fallback string) error {
	st := s.ztpSettings(ctx)
	if p.Vendor != "yealink" || !st.Yealink.Enabled {
		return errors.New("ztp_not_available")
	}
	prov, err := provision.Load(ctx, s.DB)
	if err != nil {
		return err
	}
	label := p.Label
	if label == "" {
		label = "Kiestoon " + p.Model
	}
	err = ztp.NewClient(st.Yealink).Register(ctx, p.MAC, s.ztpServer(ctx, fallback), prov.Username, prov.Password, label)
	switch {
	case errors.Is(err, ztp.ErrOtherOwner):
		s.PBX.SetPhoneZTP(ctx, p.ID, "elsewhere", "")
	case err != nil:
		s.PBX.SetPhoneZTP(ctx, p.ID, "error", err.Error())
	default:
		s.PBX.SetPhoneZTP(ctx, p.ID, "registered", "")
	}
	return err
}

// ztpAfterSave registers a new or changed Yealink phone in the background.
func (s *Server) ztpAfterSave(r *http.Request, p *pbx.Phone, old *pbx.Phone) {
	st := s.ztpSettings(r.Context())
	if !st.Yealink.Enabled {
		return
	}
	fallback := s.publicBase(r) + "/prov/"
	if old != nil && old.Vendor == "yealink" && (old.MAC != p.MAC || p.Vendor != "yealink") {
		s.ztpRemove(old)
	}
	if p.Vendor != "yealink" || (old != nil && old.MAC == p.MAC && old.Vendor == p.Vendor && old.ZTPState == "registered") {
		return
	}
	phone := *p
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := s.ztpRegister(ctx, &phone, fallback); err != nil {
			s.Log.Warn("yealink rps registration", "mac", phone.MAC, "err", err)
		}
	}()
}

// ztpRemove takes a deleted phone out of the RPS account, so it can be used elsewhere.
func (s *Server) ztpRemove(p *pbx.Phone) {
	st := s.ztpSettings(context.Background())
	if p.Vendor != "yealink" || !st.Yealink.Enabled || p.ZTPState != "registered" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := ztp.NewClient(st.Yealink).Remove(ctx, p.MAC); err != nil {
			s.Log.Warn("yealink rps removal", "mac", p.MAC, "err", err)
		}
	}()
}

func (s *Server) ztpPhone(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	if err := s.ztpRegister(ctx, p, s.publicBase(r)+"/prov/"); err != nil {
		if err.Error() == "ztp_not_available" {
			writeErr(w, http.StatusConflict, "ztp_not_available")
			return
		}
		if errors.Is(err, ztp.ErrOtherOwner) {
			writeErr(w, http.StatusConflict, "ztp_elsewhere")
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ztp_failed", "message": err.Error()})
		return
	}
	s.audit(r, "ztp.registered", p.MAC)
	p, _ = s.PBX.Phone(r.Context(), p.ID)
	writeJSON(w, http.StatusOK, p)
}

// ztpExport lists the MAC addresses of one vendor as CSV for bulk import in a vendor portal.
func (s *Server) ztpExport(w http.ResponseWriter, r *http.Request) {
	vendor := r.URL.Query().Get("vendor")
	if !slices.Contains(pbx.Vendors, vendor) {
		fieldErr(w, "vendor")
		return
	}
	ps, err := s.PBX.Phones(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="kiestoon-%s-macs.csv"`, vendor))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"mac", "model", "label"})
	for _, p := range ps {
		if p.Vendor == vendor {
			label := p.Label
			if label != "" && strings.ContainsRune("=+-@", rune(label[0])) {
				label = "'" + label
			}
			_ = cw.Write([]string{strings.ToUpper(p.MAC), p.Model, label})
		}
	}
	cw.Flush()
}
