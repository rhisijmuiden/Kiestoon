package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Transferring calls from the portal: an employee drags an active call onto a colleague, a ring
// group or a queue. Blind sends the call straight on; attended (consult) first rings the target
// while the caller waits on hold, and the employee completes or cancels the transfer. The
// switchboard moves any call with a redirect. Everything goes over AMI, so it works for desk
// phones and for the browser softphone alike.

// xferContext is where transfer targets are dialled: extensions, ring groups and queues.
const xferContext = "kiestoon-local"

type consult struct {
	Target     string
	TargetName string
	Peer       string
	PeerName   string
	Started    time.Time
}

type transfers struct {
	mu sync.Mutex
	m  map[string]consult
}

func (t *transfers) set(ch string, c consult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]consult{}
	}
	t.m[ch] = c
}

func (t *transfers) get(ch string) (consult, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.m[ch]
	return c, ok
}

func (t *transfers) del(ch string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, ch)
}

// prune forgets transfers whose channel is gone.
func (t *transfers) prune(alive func(string) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ch := range t.m {
		if !alive(ch) {
			delete(t.m, ch)
		}
	}
}

func (s *Server) transferRoutes(mux *http.ServeMux, user, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/portal/active", user(s.portalActive))
	mux.Handle("GET /api/portal/targets", user(s.transferTargets))
	mux.Handle("POST /api/portal/transfer", user(s.portalTransfer))
	mux.Handle("POST /api/portal/transfer/complete", user(s.portalTransferComplete))
	mux.Handle("POST /api/portal/transfer/cancel", user(s.portalTransferCancel))
	mux.Handle("GET /api/reception/targets", recep(s.transferTargets))
	mux.Handle("POST /api/reception/transfer", recep(s.receptionTransfer))
}

// TransferTarget is something a call can be dropped on.
type TransferTarget struct {
	Kind    string `json:"kind"` // extension, ringgroup, queue
	Number  string `json:"number"`
	Name    string `json:"name"`
	State   string `json:"state,omitempty"`
	DND     bool   `json:"dnd,omitempty"`
	Members int    `json:"members,omitempty"`
	Waiting int    `json:"waiting,omitempty"`
}

func (s *Server) targets(ctx context.Context) []TransferTarget {
	out := []TransferTarget{}
	for _, e := range s.blfList(ctx) {
		out = append(out, TransferTarget{Kind: "extension", Number: e.Number, Name: e.Name, State: e.State, DND: e.DND})
	}
	rgs, _ := s.PBX.RingGroups(ctx)
	for _, g := range rgs {
		if g.Enabled {
			out = append(out, TransferTarget{Kind: "ringgroup", Number: g.Number, Name: g.Name, Members: len(g.Members)})
		}
	}
	qs, _ := s.PBX.Queues(ctx)
	waiting := map[int64]int{}
	if s.AMI.Connected() {
		for _, st := range s.States.Queues(ctx) {
			waiting[st.ID] = st.Calls
		}
	}
	for _, q := range qs {
		if q.Enabled {
			out = append(out, TransferTarget{Kind: "queue", Number: q.Number, Name: q.Name, Members: len(q.Members), Waiting: waiting[q.ID]})
		}
	}
	return out
}

func (s *Server) transferTargets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.targets(r.Context()))
}

func findTarget(list []TransferTarget, number string) (TransferTarget, bool) {
	for _, t := range list {
		if t.Number == number {
			return t, true
		}
	}
	return TransferTarget{}, false
}

// ActiveCall is one call of the signed-in user, as the transfer card shows it.
type ActiveCall struct {
	Channel  string        `json:"channel"`
	Peer     string        `json:"peer"`
	PeerName string        `json:"peerName"`
	State    string        `json:"state"` // ringing, talking
	Duration int           `json:"duration"`
	Device   string        `json:"device"` // phone, web
	Consult  *ConsultState `json:"consult,omitempty"`
}

// ConsultState is an attended transfer in progress.
type ConsultState struct {
	Target     string `json:"target"`
	TargetName string `json:"targetName"`
	Answered   bool   `json:"answered"`
}

func knownNum(n string) string {
	if n == "<unknown>" || n == "s" {
		return ""
	}
	return n
}

// activeCalls lists the device legs of an extension with the party on the other side.
func activeCalls(l *asterisk.Live, e *pbx.Extension, x *transfers) []ActiveCall {
	out := []ActiveCall{}
	for _, leg := range l.LegsOf(e.Number, e.WebRTCEndpoint(), e.AppEndpoint()) {
		ac := ActiveCall{Channel: leg.Channel, Peer: knownNum(leg.Connected), PeerName: knownNum(leg.ConnName), Duration: leg.Duration, Device: "phone"}
		switch leg.Endpoint {
		case e.WebRTCEndpoint():
			ac.Device = "web"
		case e.AppEndpoint():
			ac.Device = "app"
		}
		ac.State = "ringing"
		if leg.State == "Up" {
			ac.State = "talking"
		}
		if c, ok := x.get(leg.Channel); ok {
			// During the consultation the connected line is the target: show the waiting caller.
			ac.Peer, ac.PeerName = c.Peer, c.PeerName
			cs := &ConsultState{Target: c.Target, TargetName: c.TargetName}
			for _, p := range l.ConsultLegs(leg, xferContext, c.Target) {
				if p.State == "Up" && p.BridgeID != "" && p.BridgeID == leg.BridgeID {
					cs.Answered = true
				}
			}
			ac.Consult = cs
		}
		out = append(out, ac)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Duration > out[j].Duration })
	return out
}

func (s *Server) portalActive(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	l := s.Live.Get(r.Context())
	if l.Connected {
		s.xfers.prune(func(ch string) bool { _, ok := l.Channel(ch); return ok })
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": l.Connected, "calls": activeCalls(l, e, &s.xfers)})
}

// myLeg returns the live leg of the signed-in user with this channel name, or writes an error.
func (s *Server) myLeg(w http.ResponseWriter, r *http.Request, e *pbx.Extension, channel string) (*asterisk.Live, asterisk.Channel, bool) {
	l := s.Live.Get(r.Context())
	if !l.Connected {
		writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		return nil, asterisk.Channel{}, false
	}
	for _, leg := range l.LegsOf(e.Number, e.WebRTCEndpoint(), e.AppEndpoint()) {
		if leg.Channel == channel {
			return l, leg, true
		}
	}
	writeErr(w, http.StatusNotFound, "call_gone")
	return nil, asterisk.Channel{}, false
}

func (s *Server) amiErr(w http.ResponseWriter, err error) {
	var ae *asterisk.ActionError
	if errors.As(err, &ae) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "transfer_failed", "message": ae.Msg})
		return
	}
	writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
}

func (s *Server) portalTransfer(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var req struct {
		Channel string `json:"channel"`
		Target  string `json:"target"`
		Mode    string `json:"mode"`
	}
	if err := decode(r, &req); err != nil || (req.Mode != "blind" && req.Mode != "attended") {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t, ok := findTarget(s.targets(r.Context()), req.Target)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "target"})
		return
	}
	if t.Number == e.Number {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "self_transfer", "field": "target"})
		return
	}
	_, leg, ok := s.myLeg(w, r, e, req.Channel)
	if !ok {
		return
	}
	if leg.State != "Up" {
		writeErr(w, http.StatusConflict, "not_answered")
		return
	}
	if _, busy := s.xfers.get(leg.Channel); busy {
		writeErr(w, http.StatusConflict, "transfer_in_progress")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var err error
	if req.Mode == "blind" {
		err = s.AMI.BlindTransfer(ctx, leg.Channel, xferContext, t.Number)
	} else {
		err = s.AMI.AttendedTransfer(ctx, leg.Channel, xferContext, t.Number)
	}
	if err != nil {
		s.amiErr(w, err)
		return
	}
	if req.Mode == "attended" {
		s.xfers.set(leg.Channel, consult{Target: t.Number, TargetName: t.Name, Peer: knownNum(leg.Connected), PeerName: knownNum(leg.ConnName), Started: time.Now()})
	}
	s.Live.Invalidate()
	s.audit(r, "portal.transfer."+req.Mode, e.Number+" -> "+t.Number)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) transferStep(w http.ResponseWriter, r *http.Request, complete bool) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	var req struct {
		Channel string `json:"channel"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	l, leg, ok := s.myLeg(w, r, e, req.Channel)
	if !ok {
		return
	}
	c, ok := s.xfers.get(leg.Channel)
	if !ok {
		writeErr(w, http.StatusConflict, "no_transfer")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if complete {
		// The transferer hangs up: Asterisk connects the waiting caller with the target.
		if err := s.AMI.Hangup(ctx, leg.Channel); err != nil {
			s.amiErr(w, err)
			return
		}
	} else {
		// The consultation ends: the transferer is back with the waiting caller.
		for _, p := range l.ConsultLegs(leg, xferContext, c.Target) {
			if err := s.AMI.Hangup(ctx, p.Channel); err != nil {
				s.amiErr(w, err)
				return
			}
		}
	}
	s.xfers.del(leg.Channel)
	s.Live.Invalidate()
	action := "portal.transfer.cancel"
	if complete {
		action = "portal.transfer.complete"
	}
	s.audit(r, action, e.Number+" -> "+c.Target)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) portalTransferComplete(w http.ResponseWriter, r *http.Request) {
	s.transferStep(w, r, true)
}
func (s *Server) portalTransferCancel(w http.ResponseWriter, r *http.Request) {
	s.transferStep(w, r, false)
}

// receptionTransfer moves one leg of any live call (usually the caller) to a target.
func (s *Server) receptionTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Target  string `json:"target"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	t, ok := findTarget(s.targets(r.Context()), req.Target)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "target"})
		return
	}
	l := s.Live.Get(r.Context())
	if !l.Connected {
		writeErr(w, http.StatusBadGateway, "asterisk_unavailable")
		return
	}
	leg, ok := l.Channel(req.Channel)
	if !ok || !strings.HasPrefix(leg.Channel, "PJSIP/") {
		writeErr(w, http.StatusNotFound, "call_gone")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.AMI.Redirect(ctx, leg.Channel, xferContext, t.Number); err != nil {
		s.amiErr(w, err)
		return
	}
	s.Live.Invalidate()
	who := leg.CallerNum
	if who == "" {
		who = leg.Endpoint
	}
	s.audit(r, "reception.transfer", who+" -> "+t.Number)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
