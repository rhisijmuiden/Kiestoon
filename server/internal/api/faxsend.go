package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
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

// MaxFaxDocument is the largest PDF that can be faxed.
const MaxFaxDocument = 10 << 20

// maxFaxLines is how many faxes are sent at the same time.
const maxFaxLines = 2

func (s *Server) faxOutStore() string { return filepath.Join(s.Cfg.DataDir, "fax", "out") }
func (s *Server) faxOutSpool() string { return filepath.Join(s.Cfg.SpoolDir(), "fax-out") }

// publicFaxBox hides the e-mail PIN.
func publicFaxBox(b pbx.FaxBox) pbx.FaxBox {
	b.SendPINSet, b.SendPIN = b.SendPIN != "", ""
	return b
}

func (s *Server) faxBoxesList(w http.ResponseWriter, r *http.Request) {
	bs, err := s.PBX.FaxBoxes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	for i := range bs {
		bs[i] = publicFaxBox(bs[i])
	}
	writeJSON(w, http.StatusOK, bs)
}

// faxSendBoxes are the boxes the reception can send from.
func (s *Server) faxSendBoxes(w http.ResponseWriter, r *http.Request) {
	bs, err := s.PBX.FaxBoxes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	type box struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Number string `json:"number"`
	}
	out := []box{}
	for _, b := range bs {
		if b.Enabled && b.SendEnabled {
			out = append(out, box{b.ID, b.Name, b.Number})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) faxJobs(w http.ResponseWriter, r *http.Request) {
	js, err := s.PBX.FaxJobs(r.Context(), nil, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, js)
}

// faxSend queues a PDF from the portal: {boxId, number, name, pdf (base64)}.
func (s *Server) faxSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BoxID  int64  `json:"boxId"`
		Number string `json:"number"`
		Name   string `json:"name"`
		PDF    string `json:"pdf"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	box, err := s.PBX.FaxBox(r.Context(), req.BoxID)
	if err != nil || !box.Enabled || !box.SendEnabled {
		fieldErr(w, "boxId")
		return
	}
	num := pbx.FaxDialNumber(req.Number)
	if num == "" {
		fieldErr(w, "number")
		return
	}
	doc, err := base64.StdEncoding.DecodeString(req.PDF)
	if err != nil || len(doc) == 0 || len(doc) > MaxFaxDocument || !bytes.HasPrefix(doc, []byte("%PDF-")) {
		fieldErr(w, "pdf")
		return
	}
	sess := sessionFrom(r)
	uid := sess.User.ID
	j, err := s.queueFax(r.Context(), box, num, cleanDocName(req.Name), "portal", sess.User.Username, &uid, [][]byte{doc})
	if err != nil {
		s.Log.Warn("fax queue", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "fax.queued", fmt.Sprintf("job %d to %s", j.ID, num))
	writeJSON(w, http.StatusOK, j)
}

func cleanDocName(n string) string {
	n = filepath.Base(strings.TrimSpace(n))
	n = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '/' || r == '\\' || r == '"' {
			return -1
		}
		return r
	}, n)
	if n == "" || n == "." {
		n = "document.pdf"
	}
	if len(n) > 100 {
		n = n[:100]
	}
	return n
}

// queueFax stores the document(s) as one PDF and queues the job.
func (s *Server) queueFax(ctx context.Context, box *pbx.FaxBox, number, name, source, sender string, uid *int64, docs [][]byte) (*pbx.FaxJob, error) {
	if err := os.MkdirAll(s.faxOutStore(), 0o750); err != nil {
		return nil, err
	}
	j := &pbx.FaxJob{BoxID: &box.ID, UserID: uid, Source: source, Sender: sender, Number: number, Document: name}
	if err := s.PBX.InsertFaxJob(ctx, j); err != nil {
		return nil, err
	}
	var paths []string
	for i, d := range docs {
		p := filepath.Join(s.faxOutStore(), fmt.Sprintf("job%d-%d.pdf", j.ID, i))
		if err := os.WriteFile(p, d, 0o640); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	j.File = fmt.Sprintf("job%d.pdf", j.ID)
	out := filepath.Join(s.faxOutStore(), j.File)
	if len(paths) == 1 {
		if err := os.Rename(paths[0], out); err != nil {
			return nil, err
		}
	} else {
		err := mergePDF(ctx, out, paths)
		for _, p := range paths {
			_ = os.Remove(p)
		}
		if err != nil {
			j.Status, j.Error = "failed", "pdf: "+err.Error()
			_ = s.PBX.UpdateFaxJob(ctx, j)
			return j, nil
		}
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE fax_jobs SET file = ? WHERE id = ?`, j.File, j.ID); err != nil {
		return nil, err
	}
	s.emit("fax.queued", map[string]any{"id": j.ID, "box": box.ID, "number": number, "source": source})
	return s.PBX.FaxJob(ctx, j.ID)
}

func gs(ctx context.Context, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	base := []string{"-q", "-dNOPAUSE", "-dBATCH", "-dSAFER", "-dNOINTERPOLATE"}
	b, err := exec.CommandContext(cctx, "gs", append(base, args...)...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 200 {
			msg = msg[len(msg)-200:]
		}
		return fmt.Errorf("ghostscript: %v %s", err, msg)
	}
	return nil
}

func mergePDF(ctx context.Context, out string, in []string) error {
	return gs(ctx, append([]string{"-sDEVICE=pdfwrite", "-sOutputFile=" + out}, in...)...)
}

// pdfToFax renders a PDF as a fax: TIFF class F (T.4), 204×196 dpi, A4.
func pdfToFax(ctx context.Context, pdf, tif string) error {
	return gs(ctx, "-sDEVICE=tiffg3", "-r204x196", "-sPAPERSIZE=a4", "-dFIXEDMEDIA", "-dPDFFitPage", "-sOutputFile="+tif, pdf)
}

// tiffPages counts the pages (image directories) of a TIFF file.
func tiffPages(path string) int {
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(b[4:8]))
	n := 0
	for off > 0 && off+2 <= len(b) && n < 1000 {
		entries := int(bo.Uint16(b[off : off+2]))
		next := off + 2 + entries*12
		if next+4 > len(b) {
			break
		}
		n++
		off = int(bo.Uint32(b[next : next+4]))
	}
	return n
}

// runFaxQueue starts due jobs; called every few seconds.
func (s *Server) runFaxQueue(ctx context.Context) {
	s.faxMu.Lock()
	defer s.faxMu.Unlock()
	sending, err := s.PBX.SendingFaxJobs(ctx)
	if err != nil {
		return
	}
	busy := 0
	for _, j := range sending {
		// A call that never reported back (Asterisk restarted): try again.
		if t, err := time.Parse(time.RFC3339, j.UpdatedAt); err == nil && time.Since(t) > 20*time.Minute {
			s.faxRetry(ctx, &j, "no result from the phone system")
			continue
		}
		busy++
	}
	if busy >= maxFaxLines || !s.AMI.Connected() {
		return
	}
	due, err := s.PBX.DueFaxJobs(ctx, time.Now(), maxFaxLines-busy)
	if err != nil {
		return
	}
	for _, j := range due {
		s.startFax(ctx, &j)
	}
}

func (s *Server) startFax(ctx context.Context, j *pbx.FaxJob) {
	if j.BoxID == nil {
		s.faxDone(ctx, j, "failed", "fax box removed")
		return
	}
	box, err := s.PBX.FaxBox(ctx, *j.BoxID)
	if err != nil || !box.Enabled || !box.SendEnabled {
		s.faxDone(ctx, j, "failed", "fax box removed or sending switched off")
		return
	}
	if err := os.MkdirAll(s.faxOutSpool(), 0o2770); err != nil {
		s.faxRetry(ctx, j, err.Error())
		return
	}
	tif := filepath.Join(s.faxOutSpool(), fmt.Sprintf("job%d.tif", j.ID))
	if _, err := os.Stat(tif); err != nil {
		if err := pdfToFax(ctx, filepath.Join(s.faxOutStore(), j.File), tif); err != nil {
			s.faxDone(ctx, j, "failed", err.Error())
			return
		}
		_ = os.Chmod(tif, 0o660)
	}
	j.Pages = tiffPages(tif)
	j.Status, j.Attempts, j.Error = "sending", j.Attempts+1, ""
	if err := s.PBX.UpdateFaxJob(ctx, j); err != nil {
		return
	}
	ecm := "yes"
	if !box.ECM {
		ecm = "no"
	}
	cid := box.Number
	err = s.AMI.OriginateApp(ctx, "Local/"+j.Number+"@kiestoon-faxout", "kiestoon-faxsend", strconv.FormatInt(j.ID, 10),
		`"`+strings.ReplaceAll(box.Name, `"`, "")+`" <`+cid+`>`, time.Minute,
		"KT_FAXJOB="+strconv.FormatInt(j.ID, 10), "KT_FAXFILE="+tif, "KT_FAXCID="+cid, "KT_FAXECM="+ecm,
		"KT_FAXHEADER="+asteriskSafe(box.Name))
	if err != nil {
		s.faxRetry(ctx, j, "originate: "+err.Error())
		return
	}
	s.Log.Info("fax sending", "job", j.ID, "number", j.Number, "pages", j.Pages, "attempt", j.Attempts)
}

func asteriskSafe(v string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ',', '(', ')', '$', '[', ']', '{', '}', '"', '\\', ';', '|', '\n', '\r':
			return ' '
		}
		return r
	}, v)
}

// faxRetry puts a job back in the queue, or fails it after the last attempt.
func (s *Server) faxRetry(ctx context.Context, j *pbx.FaxJob, reason string) {
	if j.Attempts >= pbx.MaxFaxAttempts {
		s.faxDone(ctx, j, "failed", reason)
		return
	}
	j.Status, j.Error = "queued", reason
	j.NextAt = time.Now().Add(time.Duration(j.Attempts) * 5 * time.Minute).UTC().Format(time.RFC3339)
	_ = s.PBX.UpdateFaxJob(ctx, j)
}

func (s *Server) faxDone(ctx context.Context, j *pbx.FaxJob, status, reason string) {
	j.Status, j.Error = status, reason
	_ = s.PBX.UpdateFaxJob(ctx, j)
	_ = os.Remove(filepath.Join(s.faxOutSpool(), fmt.Sprintf("job%d.tif", j.ID)))
	s.Log.Info("fax finished", "job", j.ID, "status", status, "reason", reason)
	store.Audit(ctx, s.DB, 0, "fax.sent", fmt.Sprintf("job %d to %s: %s %s", j.ID, j.Number, status, reason), "")
	s.emit("fax.sent", map[string]any{"id": j.ID, "number": j.Number, "status": status, "pages": j.Pages})
	s.faxReport(ctx, j)
}

// faxReport mails the result to the sender of an e-mail fax, or to the portal user's address.
func (s *Server) faxReport(ctx context.Context, j *pbx.FaxJob) {
	to := ""
	switch {
	case j.Source == "email":
		to = j.Sender
	case j.UserID != nil:
		if u, err := s.Auth.UserByID(ctx, *j.UserID); err == nil {
			to = u.Email
		}
	}
	if to == "" {
		return
	}
	set, err := mail.Load(ctx, s.DB)
	if err != nil || !set.Enabled {
		return
	}
	nl := s.defaultLanguage(ctx) == "nl"
	var subject, body string
	switch {
	case j.Status == "success" && nl:
		subject = fmt.Sprintf("Fax verzonden naar %s (%d pagina's)", j.Number, j.Pages)
		body = fmt.Sprintf("Je fax \"%s\" is verzonden naar %s.\n\nPagina's: %d\nPogingen: %d\n\n-- \nKiestoon\n", j.Document, j.Number, j.Pages, j.Attempts)
	case j.Status == "success":
		subject = fmt.Sprintf("Fax sent to %s (%d pages)", j.Number, j.Pages)
		body = fmt.Sprintf("Your fax \"%s\" was sent to %s.\n\nPages: %d\nAttempts: %d\n\n-- \nKiestoon\n", j.Document, j.Number, j.Pages, j.Attempts)
	case nl:
		subject = fmt.Sprintf("Fax naar %s niet verzonden", j.Number)
		body = fmt.Sprintf("Je fax \"%s\" kon niet worden verzonden naar %s.\n\nReden: %s\nPogingen: %d\n\n-- \nKiestoon\n", j.Document, j.Number, j.Error, j.Attempts)
	default:
		subject = fmt.Sprintf("Fax to %s not sent", j.Number)
		body = fmt.Sprintf("Your fax \"%s\" could not be sent to %s.\n\nReason: %s\nAttempts: %d\n\n-- \nKiestoon\n", j.Document, j.Number, j.Error, j.Attempts)
	}
	_ = mail.Enqueue(mail.QueueDir(s.Cfg.DataDir), mail.Compose(set, to, subject, body))
}

// onFaxOut handles the KiestoonFaxOut event at the end of a fax call.
func (s *Server) onFaxOut(m asterisk.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, _ := strconv.ParseInt(m.Get("Job"), 10, 64)
	j, err := s.PBX.FaxJob(ctx, id)
	if err != nil || j.Status != "sending" {
		return
	}
	if p, err := strconv.Atoi(m.Get("Pages")); err == nil && p > 0 {
		j.Pages = p
	}
	if strings.EqualFold(m.Get("Status"), "SUCCESS") {
		s.faxDone(ctx, j, "success", "")
		return
	}
	reason := strings.TrimSpace(m.Get("Error"))
	if reason == "" {
		reason = "transmission failed"
	}
	s.faxRetry(ctx, j, reason)
}

// onOriginateResponse handles a fax call that was not answered (busy, no answer, no route).
func (s *Server) onOriginateResponse(m asterisk.Message) {
	if m.Get("Context") != "kiestoon-faxsend" || strings.EqualFold(m.Get("Response"), "Success") {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, _ := strconv.ParseInt(m.Get("Exten"), 10, 64)
	j, err := s.PBX.FaxJob(ctx, id)
	if err != nil || j.Status != "sending" {
		return
	}
	reason := map[string]string{"1": "no answer", "3": "no answer", "5": "busy", "8": "congestion", "0": "no route"}[m.Get("Reason")]
	if reason == "" {
		reason = "call failed (" + m.Get("Reason") + ")"
	}
	s.faxRetry(ctx, j, reason)
}

func (s *Server) faxJobPDF(w http.ResponseWriter, r *http.Request) {
	j, err := s.PBX.FaxJob(r.Context(), pathID(r))
	if err != nil || j.File == "" || strings.ContainsAny(j.File, "/\\") {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="fax-out-%d.pdf"`, j.ID))
	http.ServeFile(w, r, filepath.Join(s.faxOutStore(), j.File))
}

func (s *Server) faxJobDelete(w http.ResponseWriter, r *http.Request) {
	j, err := s.PBX.FaxJob(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if j.Status == "sending" {
		writeErr(w, http.StatusConflict, "fax_sending")
		return
	}
	if _, err := s.PBX.DeleteFaxJob(r.Context(), j.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if j.File != "" && !strings.ContainsAny(j.File, "/\\") {
		_ = os.Remove(filepath.Join(s.faxOutStore(), j.File))
	}
	_ = os.Remove(filepath.Join(s.faxOutSpool(), fmt.Sprintf("job%d.tif", j.ID)))
	s.audit(r, "fax.job.deleted", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// faxJobRetry sends a failed fax again.
func (s *Server) faxJobRetry(w http.ResponseWriter, r *http.Request) {
	j, err := s.PBX.FaxJob(r.Context(), pathID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if j.Status != "failed" {
		writeErr(w, http.StatusConflict, "fax_not_failed")
		return
	}
	j.Status, j.Attempts, j.Error, j.NextAt = "queued", 0, "", ""
	_ = s.PBX.UpdateFaxJob(r.Context(), j)
	s.audit(r, "fax.job.retry", r.PathValue("id"))
	writeJSON(w, http.StatusOK, j)
}

// purgeFaxJobs removes sent faxes after the retention of their box.
func (s *Server) purgeFaxJobs(ctx context.Context) {
	js, err := s.PBX.ExpiredFaxJobs(ctx)
	if err != nil {
		return
	}
	for _, j := range js {
		if _, err := s.PBX.DeleteFaxJob(ctx, j.ID); err == nil && j.File != "" && !strings.ContainsAny(j.File, "/\\") {
			_ = os.Remove(filepath.Join(s.faxOutStore(), j.File))
		}
	}
}
