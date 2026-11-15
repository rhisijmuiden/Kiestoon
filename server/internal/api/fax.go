package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

func (s *Server) faxRoutes(mux *http.ServeMux, admin, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/pbx/faxboxes", admin(list(s, s.PBX.FaxBoxes)))
	mux.Handle("POST /api/pbx/faxboxes", admin(s.saveFaxBox))
	mux.Handle("PUT /api/pbx/faxboxes/{id}", admin(s.saveFaxBox))
	mux.Handle("DELETE /api/pbx/faxboxes/{id}", admin(s.deleter("faxbox", s.PBX.DeleteFaxBox)))
	mux.Handle("GET /api/faxes", recep(s.faxList))
	mux.Handle("GET /api/faxes/{id}/pdf", recep(s.faxPDF))
	mux.Handle("DELETE /api/faxes/{id}", admin(s.faxDelete))
}

// faxSpool is where Asterisk writes received faxes (TIFF).
func (s *Server) faxSpool() string { return filepath.Join(s.Cfg.SpoolDir(), "fax") }

// faxStore keeps the converted PDFs.
func (s *Server) faxStore() string { return filepath.Join(s.Cfg.DataDir, "fax") }

func (s *Server) faxInUse(ctx context.Context) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM fax_boxes WHERE enabled = 1`).Scan(&n)
	return n > 0
}

func (s *Server) saveFaxBox(w http.ResponseWriter, r *http.Request) {
	var f pbx.FaxBox
	if err := decode(r, &f); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	f.ID = pathID(r)
	had := s.faxInUse(r.Context())
	if err := s.PBX.SaveFaxBox(r.Context(), &f); err != nil {
		s.saveErr(w, err)
		return
	}
	if had != s.faxInUse(r.Context()) {
		s.firewallChanged(r.Context()) // the T.38 ports open or close
	}
	_ = os.MkdirAll(s.faxSpool(), 0o2770)
	s.saved(w, r, "faxbox", f.Name, f)
}

func (s *Server) faxList(w http.ResponseWriter, r *http.Request) {
	fs, err := s.PBX.Faxes(r.Context(), 500)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

func (s *Server) faxPDF(w http.ResponseWriter, r *http.Request) {
	f, err := s.PBX.Fax(r.Context(), pathID(r))
	if err != nil || f.File == "" || strings.ContainsAny(f.File, "/\\") {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	path := filepath.Join(s.faxStore(), f.File)
	fh, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	defer fh.Close()
	ct := "application/pdf"
	if strings.HasSuffix(f.File, ".tif") {
		ct = "image/tiff"
	}
	s.audit(r, "fax.downloaded", strconv.FormatInt(f.ID, 10))
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="fax-%d%s"`, f.ID, filepath.Ext(f.File)))
	st, _ := fh.Stat()
	http.ServeContent(w, r, f.File, st.ModTime(), fh)
}

func (s *Server) faxDelete(w http.ResponseWriter, r *http.Request) {
	file, err := s.PBX.DeleteFax(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if file != "" && !strings.ContainsAny(file, "/\\") {
		_ = os.Remove(filepath.Join(s.faxStore(), file))
	}
	s.audit(r, "fax.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// onFax handles the KiestoonFax event the dialplan sends when a fax call ends.
func (s *Server) onFax(m asterisk.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	boxID, _ := strconv.ParseInt(m.Get("Box"), 10, 64)
	pages, _ := strconv.Atoi(m.Get("Pages"))
	tif := m.Get("File")
	// Only files in our own spool directory, named by Asterisk's unique ID.
	if filepath.Dir(tif) != s.faxSpool() || !strings.HasSuffix(tif, ".tif") || strings.Contains(filepath.Base(tif), "..") {
		s.Log.Warn("fax event with unexpected file", "file", tif)
		return
	}
	f := &pbx.Fax{ReceivedAt: time.Now().UTC().Format(time.RFC3339), Caller: m.Get("Caller"), DID: m.Get("DID"),
		RemoteStation: strings.TrimSpace(m.Get("Remote")), Pages: pages, Status: "failed", Error: m.Get("Error")}
	if boxID > 0 {
		f.BoxID = &boxID
	}
	box, boxErr := s.PBX.FaxBox(ctx, boxID)
	st, statErr := os.Stat(tif)
	if strings.EqualFold(m.Get("Status"), "SUCCESS") && statErr == nil && st.Size() > 0 {
		f.Status, f.Error = "success", ""
	}
	if statErr == nil && st.Size() > 0 {
		if err := os.MkdirAll(s.faxStore(), 0o750); err != nil {
			s.Log.Warn("fax store", "err", err)
		}
		base := strings.TrimSuffix(filepath.Base(tif), ".tif")
		pdf := filepath.Join(s.faxStore(), base+".pdf")
		if err := tiffToPDF(ctx, tif, pdf); err != nil {
			// Keep the TIFF when it cannot be converted, so nothing is lost.
			s.Log.Warn("fax conversion failed, keeping TIFF", "err", err)
			if b, rerr := os.ReadFile(tif); rerr == nil && os.WriteFile(filepath.Join(s.faxStore(), base+".tif"), b, 0o640) == nil {
				f.File = base + ".tif"
			}
		} else {
			f.File = base + ".pdf"
		}
		if f.File != "" {
			if fi, err := os.Stat(filepath.Join(s.faxStore(), f.File)); err == nil {
				f.Size = fi.Size()
			}
			_ = os.Remove(tif)
		}
	}
	if err := s.PBX.InsertFax(ctx, f); err != nil {
		s.Log.Warn("store fax", "err", err)
		return
	}
	s.Log.Info("fax received", "box", boxID, "status", f.Status, "pages", f.Pages, "caller", f.Caller)
	store.Audit(ctx, s.DB, 0, "fax.received", fmt.Sprintf("%s %d pages from %s", f.Status, f.Pages, f.Caller), "")
	s.emit("fax.received", map[string]any{"id": f.ID, "box": boxID, "status": f.Status, "pages": f.Pages, "caller": f.Caller, "did": f.DID})
	if boxErr != nil || f.File == "" || f.Status != "success" {
		return
	}
	if err := s.mailFax(ctx, box, f); err != nil {
		s.Log.Warn("fax mail", "err", err)
		return
	}
	s.PBX.SetFaxMailed(ctx, f.ID)
}

// tiffToPDF converts a fax with tiff2pdf (libtiff-tools), which keeps the CCITT compression.
func tiffToPDF(ctx context.Context, in, out string) error {
	cctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	tmp := out + ".tmp"
	b, err := exec.CommandContext(cctx, "tiff2pdf", "-p", "A4", "-F", "-o", tmp, in).CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("tiff2pdf: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return os.Rename(tmp, out)
}

func (s *Server) mailFax(ctx context.Context, box *pbx.FaxBox, f *pbx.Fax) error {
	rcpts := box.Recipients()
	if len(rcpts) == 0 {
		return errors.New("fax box has no e-mail address")
	}
	b, err := os.ReadFile(filepath.Join(s.faxStore(), f.File))
	if err != nil {
		return err
	}
	set, err := mail.Load(ctx, s.DB)
	if err != nil {
		return err
	}
	from := f.Caller
	if f.RemoteStation != "" {
		from = strings.TrimSpace(f.RemoteStation + " (" + f.Caller + ")")
	}
	if from == "" {
		from = "unknown"
	}
	at := time.Now().In(s.loc(ctx)).Format("02-01-2006 15:04")
	subject := fmt.Sprintf("Fax from %s (%d pages)", from, f.Pages)
	body := fmt.Sprintf("A fax arrived on %s for %s.\n\nFrom: %s\nTo number: %s\nPages: %d\nReceived: %s\n\nThe fax is attached as a PDF.\n",
		at, box.Name, from, f.DID, f.Pages, at)
	if s.defaultLanguage(ctx) == "nl" {
		subject = fmt.Sprintf("Fax van %s (%d pagina's)", from, f.Pages)
		body = fmt.Sprintf("Er is een fax binnengekomen voor %s.\n\nVan: %s\nNaar nummer: %s\nPagina's: %d\nOntvangen: %s\n\nDe fax zit als PDF in de bijlage.\n",
			box.Name, from, f.DID, f.Pages, at)
	}
	ct := "application/pdf"
	if strings.HasSuffix(f.File, ".tif") {
		ct = "image/tiff"
	}
	name := fmt.Sprintf("fax-%s-%d%s", time.Now().In(s.loc(ctx)).Format("20060102-1504"), f.ID, filepath.Ext(f.File))
	msg := mail.Compose(set, strings.Join(rcpts, ", "), subject, body, mail.Attachment{Name: name, ContentType: ct, Data: b})
	// The mail queue retries when the mail server is unreachable.
	return mail.Enqueue(mail.QueueDir(s.Cfg.DataDir), msg)
}

// defaultLanguage is the language of the phone system (prompts), used for system mails.
func (s *Server) defaultLanguage(ctx context.Context) string {
	st, _ := pbx.LoadSettings(ctx, s.DB)
	return st.Language
}

// purgeFaxes removes faxes older than the retention of their fax box.
func (s *Server) purgeFaxes(ctx context.Context) {
	fs, err := s.PBX.ExpiredFaxes(ctx)
	if err != nil {
		return
	}
	for _, f := range fs {
		if _, err := s.PBX.DeleteFax(ctx, f.ID); err == nil && f.File != "" && !strings.ContainsAny(f.File, "/\\") {
			_ = os.Remove(filepath.Join(s.faxStore(), f.File))
		}
	}
	// Leftover TIFFs of calls that never reported back.
	entries, _ := os.ReadDir(s.faxSpool())
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour && strings.HasSuffix(e.Name(), ".tif") {
			_ = os.Remove(filepath.Join(s.faxSpool(), e.Name()))
		}
	}
}
