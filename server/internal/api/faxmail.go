package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	netmail "net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// FaxMailKey is the settings row with FaxMail as JSON.
const FaxMailKey = "fax_mail"

// FaxMail is the mailbox Kiestoon reads for e-mail to fax. Every unread message from an allowed
// sender with a PDF attachment is faxed to the number in the subject (or in fax+<number>@ of the
// address it was sent to).
type FaxMail struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"` // tls | starttls
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	PasswordSet bool   `json:"passwordSet"`
	Folder      string `json:"folder"`
	// Delete removes handled messages instead of only marking them read.
	Delete bool `json:"delete"`
	// LastCheck and LastError show the last poll in the UI.
	LastCheck string `json:"lastCheck,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

var reHostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// Validate checks the settings.
func (f *FaxMail) Validate() error {
	f.Host = strings.TrimSpace(f.Host)
	f.Username = strings.TrimSpace(f.Username)
	if f.Folder == "" {
		f.Folder = "INBOX"
	}
	if f.Security != "tls" && f.Security != "starttls" {
		return errors.New("security")
	}
	if f.Port == 0 {
		f.Port = 993
		if f.Security == "starttls" {
			f.Port = 143
		}
	}
	if f.Port < 1 || f.Port > 65535 {
		return errors.New("port")
	}
	if strings.ContainsAny(f.Folder, "\r\n\"") || len(f.Folder) > 100 {
		return errors.New("folder")
	}
	if !f.Enabled {
		return nil
	}
	if net.ParseIP(f.Host) == nil && !reHostName.MatchString(f.Host) {
		return errors.New("host")
	}
	if f.Username == "" || len(f.Username) > 200 || strings.ContainsAny(f.Username, "\r\n") {
		return errors.New("username")
	}
	if f.Password == "" {
		return errors.New("password")
	}
	return nil
}

func (s *Server) faxMailSettings(ctx context.Context) FaxMail {
	f := FaxMail{Security: "tls", Port: 993, Folder: "INBOX"}
	_ = store.GetJSON(ctx, s.DB, FaxMailKey, &f)
	return f
}

func (s *Server) faxMailGet(w http.ResponseWriter, r *http.Request) {
	f := s.faxMailSettings(r.Context())
	f.PasswordSet, f.Password = f.Password != "", ""
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) faxMailSave(w http.ResponseWriter, r *http.Request) {
	var f FaxMail
	if err := decode(r, &f); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	old := s.faxMailSettings(r.Context())
	if f.Password == "" {
		f.Password = old.Password
	}
	if err := f.Validate(); err != nil {
		fieldErr(w, err.Error())
		return
	}
	f.PasswordSet, f.LastCheck, f.LastError = false, "", ""
	if f.Enabled {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		c, err := imapLogin(ctx, f)
		cancel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "imap_failed", "message": err.Error()})
			return
		}
		_, err = c.Select(f.Folder, true)
		_ = c.Logout()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "imap_failed", "message": err.Error()})
			return
		}
	}
	if err := store.SetJSON(r.Context(), s.DB, FaxMailKey, f); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "fax.mail.saved", f.Username+" enabled="+boolString(f.Enabled))
	s.faxMailGet(w, r)
}

func imapLogin(ctx context.Context, f FaxMail) (*imapclient.Client, error) {
	addr := net.JoinHostPort(f.Host, strconv.Itoa(f.Port))
	d := &net.Dialer{Timeout: 15 * time.Second}
	conf := &tls.Config{ServerName: f.Host, MinVersion: tls.VersionTLS12}
	var c *imapclient.Client
	var err error
	if f.Security == "tls" {
		c, err = imapclient.DialWithDialerTLS(d, addr, conf)
	} else {
		c, err = imapclient.DialWithDialer(d, addr)
		if err == nil {
			if err = c.StartTLS(conf); err != nil {
				_ = c.Logout()
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		c.Timeout = time.Until(dl)
	} else {
		c.Timeout = time.Minute
	}
	if err := c.Login(f.Username, f.Password); err != nil {
		_ = c.Logout()
		return nil, err
	}
	return c, nil
}

// pollFaxMail reads new messages and queues them as faxes.
func (s *Server) pollFaxMail(ctx context.Context) {
	f := s.faxMailSettings(ctx)
	if !f.Enabled {
		return
	}
	err := s.pollFaxMailOnce(ctx, f)
	f.LastCheck = time.Now().UTC().Format(time.RFC3339)
	f.LastError = ""
	if err != nil {
		f.LastError = err.Error()
		s.Log.Warn("e-mail to fax", "err", err)
	}
	// Store only the status; the settings may have changed meanwhile.
	cur := s.faxMailSettings(ctx)
	cur.LastCheck, cur.LastError = f.LastCheck, f.LastError
	_ = store.SetJSON(ctx, s.DB, FaxMailKey, cur)
}

func (s *Server) pollFaxMailOnce(ctx context.Context, f FaxMail) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	c, err := imapLogin(ctx, f)
	if err != nil {
		return err
	}
	defer func() { _ = c.Logout() }()
	if _, err := c.Select(f.Folder, false); err != nil {
		return err
	}
	crit := imap.NewSearchCriteria()
	crit.WithoutFlags = []string{imap.SeenFlag, imap.DeletedFlag}
	uids, err := c.UidSearch(crit)
	if err != nil || len(uids) == 0 {
		return err
	}
	if len(uids) > 20 {
		uids = uids[:20]
	}
	set := new(imap.SeqSet)
	set.AddNum(uids...)
	section := &imap.BodySectionName{Peek: true}
	msgs := make(chan *imap.Message, len(uids))
	if err := c.UidFetch(set, []imap.FetchItem{imap.FetchUid, section.FetchItem()}, msgs); err != nil {
		return err
	}
	done := new(imap.SeqSet)
	boxes, _ := s.PBX.FaxBoxes(ctx)
	for m := range msgs {
		body := m.GetBody(section)
		if body == nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(body, 30<<20))
		if err != nil {
			continue
		}
		s.mailToFax(ctx, boxes, raw)
		done.AddNum(m.Uid)
	}
	if done.Empty() {
		return nil
	}
	flags := []any{imap.SeenFlag}
	if f.Delete {
		flags = append(flags, imap.DeletedFlag)
	}
	if err := c.UidStore(done, imap.FormatFlagsOp(imap.AddFlags, true), flags, nil); err != nil {
		return err
	}
	if f.Delete {
		return c.Expunge(nil)
	}
	return nil
}

var reFaxPlus = regexp.MustCompile(`^[^@+]+\+(\+?[0-9]{3,20})@`)

// mailToFax checks one message and queues it, or answers the sender why not.
func (s *Server) mailToFax(ctx context.Context, boxes []pbx.FaxBox, raw []byte) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return
	}
	from, err := netmail.ParseAddress(msg.Header.Get("From"))
	if err != nil {
		return
	}
	sender := strings.ToLower(from.Address)
	// Never answer automatic mail (bounces, out of office, our own reports): no mail loops.
	if msg.Header.Get("Auto-Submitted") != "" && !strings.EqualFold(msg.Header.Get("Auto-Submitted"), "no") ||
		msg.Header.Get("X-Kiestoon-Prepared") != "" || strings.HasPrefix(sender, "mailer-daemon@") || strings.HasPrefix(sender, "postmaster@") {
		return
	}
	var box *pbx.FaxBox
	for i := range boxes {
		if boxes[i].Enabled && boxes[i].SendEnabled && slices.Contains(boxes[i].SenderList(), sender) {
			box = &boxes[i]
			break
		}
	}
	if box == nil {
		s.Log.Warn("e-mail to fax from an unknown sender", "from", sender)
		store.Audit(ctx, s.DB, 0, "fax.mail.refused", "unknown sender "+sender, "")
		return // no answer: do not confirm the address exists
	}
	// A forged sender fails SPF/DKIM: the receiving mail server records that.
	ar := strings.ToLower(strings.Join(msg.Header["Authentication-Results"], " "))
	if strings.Contains(ar, "dmarc=fail") || strings.Contains(ar, "spf=fail") && !strings.Contains(ar, "dkim=pass") {
		s.Log.Warn("e-mail to fax failed sender authentication", "from", sender)
		store.Audit(ctx, s.DB, 0, "fax.mail.refused", "authentication failed "+sender, "")
		return
	}
	dec := new(mime.WordDecoder)
	subject, _ := dec.DecodeHeader(msg.Header.Get("Subject"))
	number, pinOK := "", box.SendPIN == ""
	for _, tok := range strings.Fields(subject) {
		if box.SendPIN != "" && subtle.ConstantTimeCompare([]byte(strings.Trim(tok, "#:[]()")), []byte(box.SendPIN)) == 1 {
			pinOK = true
			continue
		}
		if n := pbx.FaxDialNumber(tok); n != "" && number == "" {
			number = n
		}
	}
	if number == "" {
		if list, err := msg.Header.AddressList("To"); err == nil {
			for _, a := range list {
				if m := reFaxPlus.FindStringSubmatch(a.Address); m != nil {
					number = pbx.FaxDialNumber(m[1])
				}
			}
		}
	}
	nl := s.defaultLanguage(ctx) == "nl"
	refuse := func(en, nlText string) {
		text := en
		if nl {
			text = nlText
		}
		s.faxMailAnswer(ctx, sender, subject, text)
	}
	if !pinOK {
		store.Audit(ctx, s.DB, 0, "fax.mail.refused", "wrong PIN from "+sender, "")
		refuse("The fax was not sent: the PIN is missing or wrong.", "De fax is niet verzonden: de pincode ontbreekt of klopt niet.")
		return
	}
	if number == "" {
		refuse("The fax was not sent: put the fax number in the subject.", "De fax is niet verzonden: zet het faxnummer in het onderwerp.")
		return
	}
	docs, names := pdfAttachments(msg)
	if len(docs) == 0 {
		refuse("The fax was not sent: attach the document as a PDF.", "De fax is niet verzonden: voeg het document toe als PDF.")
		return
	}
	total := 0
	for _, d := range docs {
		total += len(d)
	}
	if total > MaxFaxDocument {
		refuse("The fax was not sent: the attachments are larger than 10 MB.", "De fax is niet verzonden: de bijlagen zijn groter dan 10 MB.")
		return
	}
	j, err := s.queueFax(ctx, box, number, cleanDocName(strings.Join(names, ", ")), "email", sender, nil, docs)
	if err != nil {
		s.Log.Warn("e-mail to fax", "err", err)
		return
	}
	store.Audit(ctx, s.DB, 0, "fax.queued", fmt.Sprintf("job %d to %s by e-mail from %s", j.ID, number, sender), "")
}

func (s *Server) faxMailAnswer(ctx context.Context, to, subject, text string) {
	_ = s.Mail.SendText(ctx, "fax", to, "Re: "+subject, text+"\n\n-- \nKiestoon\n")
}

// pdfAttachments walks the MIME tree and returns the PDF attachments.
func pdfAttachments(msg *netmail.Message) ([][]byte, []string) {
	var docs [][]byte
	var names []string
	var walk func(ct, cte, disp string, body io.Reader, depth int)
	walk = func(ct, cte, disp string, body io.Reader, depth int) {
		mt, params, err := mime.ParseMediaType(ct)
		if err != nil || depth > 5 {
			return
		}
		if strings.HasPrefix(mt, "multipart/") {
			mr := multipart.NewReader(body, params["boundary"])
			for {
				p, err := mr.NextRawPart()
				if err != nil {
					return
				}
				walk(p.Header.Get("Content-Type"), p.Header.Get("Content-Transfer-Encoding"), p.Header.Get("Content-Disposition"), p, depth+1)
			}
		}
		name := params["name"]
		if _, dp, err := mime.ParseMediaType(disp); err == nil && dp["filename"] != "" {
			name = dp["filename"]
		}
		isPDF := mt == "application/pdf" || (mt == "application/octet-stream" && strings.HasSuffix(strings.ToLower(name), ".pdf"))
		if !isPDF || len(docs) >= 10 {
			return
		}
		var r io.Reader = body
		switch strings.ToLower(strings.TrimSpace(cte)) {
		case "base64":
			r = base64.NewDecoder(base64.StdEncoding, stripSpace{body})
		case "quoted-printable":
			r = quotedprintable.NewReader(body)
		}
		b, err := io.ReadAll(io.LimitReader(r, MaxFaxDocument+1))
		if err != nil || !bytes.HasPrefix(b, []byte("%PDF-")) {
			return
		}
		docs = append(docs, b)
		if name == "" {
			name = "document.pdf"
		}
		names = append(names, name)
	}
	ct := msg.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	walk(ct, msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Disposition"), msg.Body, 0)
	return docs, names
}

// stripSpace drops the line breaks of base64 bodies.
type stripSpace struct{ r io.Reader }

func (s stripSpace) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	j := 0
	for i := 0; i < n; i++ {
		if c := p[i]; c != '\r' && c != '\n' && c != ' ' && c != '\t' {
			p[j] = c
			j++
		}
	}
	return j, err
}
