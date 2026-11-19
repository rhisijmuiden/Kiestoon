package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Hot desks (flexplekken): a phone with hot desking on carries its own (guest) extension until
// someone dials *55 and logs in with their extension number and PIN. The phone then fetches a
// configuration with that extension; *56 or the admin console logs out again, and phones with
// automatic logout are freed every night at 03:00.

// Results for the dialplan (KT_HD).
const (
	hdOK       = "ok"
	hdWrong    = "wrong"   // unknown extension or wrong PIN
	hdLocked   = "locked"  // too many wrong attempts on this phone
	hdNotDesk  = "notdesk" // the call does not come from a hot desk (or nobody is logged in there)
	hdFailed   = "failed"  // database error
	hdMaxFails = 5         // wrong attempts per phone before a lock
	hdLockFor  = 10 * time.Minute
)

// hotdeskFails counts wrong logins per phone.
var hotdeskFails = struct {
	sync.Mutex
	m map[int64][]time.Time
}{m: map[int64][]time.Time{}}

// hotdeskResyncDelay lets the dialplan finish its announcement before the phone reloads.
var hotdeskResyncDelay = 4 * time.Second

var reHotdeskDigits = regexp.MustCompile(`^[0-9]{1,12}$`)

func (s *Server) hotdeskRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("PUT /api/phones/{id}/hotdesk", admin(s.phoneHotdesk))
}

// hotdeskPhone finds the hot desk a call comes from: a phone with hot desking whose current
// extension is ext. When several fit (the same extension on more desks cannot be logged in, but
// a guest extension can be shared), the one at the caller's IP address wins.
func (s *Server) hotdeskPhone(ctx context.Context, ext, remote string) (*pbx.Phone, error) {
	phones, err := s.PBX.Phones(ctx)
	if err != nil {
		return nil, err
	}
	ip := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		ip = h
	}
	var found []pbx.Phone
	for _, p := range phones {
		if !p.Options.Hotdesk.Enabled {
			continue
		}
		if id := p.ActiveExtensionID(); id != nil && s.extNumber(ctx, id) == ext {
			found = append(found, p)
		}
	}
	if len(found) > 1 {
		var byIP []pbx.Phone
		for _, p := range found {
			if ip != "" && p.LastIP == ip {
				byIP = append(byIP, p)
			}
		}
		found = byIP
	}
	if len(found) != 1 {
		return nil, nil
	}
	return &found[0], nil
}

// hotdeskAGI handles *55 (login) and *56 (logout). caller is the extension the call comes from,
// remote the phone's address, number and pin what the user typed (login only).
func (s *Server) hotdeskAGI(ctx context.Context, action, caller, remote, number, pin string) string {
	p, err := s.hotdeskPhone(ctx, caller, remote)
	if err != nil {
		return hdFailed
	}
	if p == nil {
		return hdNotDesk
	}
	switch action {
	case "login":
		if hotdeskLocked(p.ID) {
			return hdLocked
		}
		if !reHotdeskDigits.MatchString(number) || !reHotdeskDigits.MatchString(pin) {
			hotdeskFail(p.ID)
			return hdWrong
		}
		e, err := s.PBX.ExtensionByNumber(ctx, number)
		if err != nil || !e.Enabled || e.VoicemailPIN == "" || subtle.ConstantTimeCompare([]byte(e.VoicemailPIN), []byte(pin)) != 1 {
			hotdeskFail(p.ID)
			store.Audit(ctx, s.DB, 0, "hotdesk.login_failed", fmt.Sprintf("%s ext=%s", p.MAC, number), "")
			return hdWrong
		}
		if p.ExtensionID != nil && *p.ExtensionID == e.ID {
			// Logging in with the desk's own extension is logging out.
			return s.hotdeskSet(ctx, p, nil, "hotdesk.logout", caller)
		}
		hotdeskFails.Lock()
		delete(hotdeskFails.m, p.ID)
		hotdeskFails.Unlock()
		return s.hotdeskSet(ctx, p, &e.ID, "hotdesk.login", caller)
	case "logout":
		if p.HotdeskExtensionID == nil {
			return hdNotDesk
		}
		return s.hotdeskSet(ctx, p, nil, "hotdesk.logout", caller)
	}
	return hdFailed
}

// hotdeskSet logs ext in on p (nil: out), and has p and any desk the extension leaves fetch their
// configuration. registeredAs is the extension the phone uses now.
func (s *Server) hotdeskSet(ctx context.Context, p *pbx.Phone, ext *int64, action, registeredAs string) string {
	others := []int64{}
	var err error
	if others, err = s.PBX.SetHotdesk(ctx, p.ID, ext); err != nil {
		return hdFailed
	}
	store.Audit(ctx, s.DB, 0, action, fmt.Sprintf("%s ext=%s", p.MAC, s.extNumber(ctx, ext)), "")
	type job struct {
		p        pbx.Phone
		endpoint string
	}
	jobs := []job{{*p, registeredAs}}
	for _, id := range others {
		// The other desk is registered with the extension that just moved here.
		if o, err := s.PBX.Phone(ctx, id); err == nil {
			jobs = append(jobs, job{*o, s.extNumber(ctx, ext)})
		}
	}
	go func() {
		time.Sleep(hotdeskResyncDelay)
		bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, j := range jobs {
			s.notifyPhone(bg, &j.p, j.endpoint)
		}
	}()
	return hdOK
}

func hotdeskLocked(id int64) bool {
	hotdeskFails.Lock()
	defer hotdeskFails.Unlock()
	recent := hotdeskFails.m[id][:0]
	for _, t := range hotdeskFails.m[id] {
		if time.Since(t) < hdLockFor {
			recent = append(recent, t)
		}
	}
	hotdeskFails.m[id] = recent
	return len(recent) >= hdMaxFails
}

func hotdeskFail(id int64) {
	hotdeskFails.Lock()
	hotdeskFails.m[id] = append(hotdeskFails.m[id], time.Now())
	hotdeskFails.Unlock()
}

// phoneHotdesk logs an extension in on a hot desk from the admin console, or out (extensionId null).
func (s *Server) phoneHotdesk(w http.ResponseWriter, r *http.Request) {
	p, err := s.PBX.Phone(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if !p.Options.Hotdesk.Enabled {
		writeErr(w, http.StatusBadRequest, "not_hotdesk")
		return
	}
	var req struct {
		ExtensionID *int64 `json:"extensionId"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.ExtensionID != nil {
		e, err := s.PBX.Extension(r.Context(), *req.ExtensionID)
		if err != nil || !e.Enabled {
			fieldErr(w, "extensionId")
			return
		}
		if p.ExtensionID != nil && *p.ExtensionID == e.ID {
			req.ExtensionID = nil
		}
	}
	before := s.registeredAs(r.Context(), p)
	action := "hotdesk.login"
	if req.ExtensionID == nil {
		action = "hotdesk.logout"
	}
	if s.hotdeskSet(r.Context(), p, req.ExtensionID, action, before) != hdOK {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, action, fmt.Sprintf("%s ext=%s (admin)", p.MAC, s.extNumber(r.Context(), req.ExtensionID)))
	p, _ = s.PBX.Phone(r.Context(), p.ID)
	writeJSON(w, http.StatusOK, p)
}

// runHotdeskAutoLogout frees hot desks with automatic logout every night at 03:00 (server time).
func (s *Server) runHotdeskAutoLogout(ctx context.Context) {
	done := ""
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			day := now.Format("2006-01-02")
			if now.Hour() != 3 || done == day {
				continue
			}
			done = day
			s.hotdeskLogoutAll(ctx)
		}
	}
}

// hotdeskLogoutAll logs out every hot desk that has automatic logout on.
func (s *Server) hotdeskLogoutAll(ctx context.Context) int {
	phones, err := s.PBX.Phones(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range phones {
		if p.Options.Hotdesk.Enabled && p.Options.Hotdesk.AutoLogout && p.HotdeskExtensionID != nil {
			if s.hotdeskSet(ctx, &p, nil, "hotdesk.auto_logout", s.registeredAs(ctx, &p)) == hdOK {
				n++
			}
		}
	}
	return n
}
