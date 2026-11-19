package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Queue callback: see asterisk/supervise.go for the dialplan side. The portal keeps the list,
// puts a placeholder call in the queue for every scheduled callback and, when an agent answers
// the placeholder, moves the agent's channel to the dialplan that calls the caller back.

func (s *Server) callbackRoutes(mux *http.ServeMux, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/reception/callbacks", recep(s.listCallbacks))
	mux.Handle("POST /api/reception/callbacks", recep(s.addCallback))
	mux.Handle("POST /api/reception/callbacks/{id}/cancel", recep(s.cancelCallback))
	mux.Handle("POST /api/reception/callbacks/{id}/retry", recep(s.retryCallback))
	mux.Handle("POST /api/reception/supervise", recep(s.supervise))
}

// cbMu serialises the status changes of callbacks between the AMI events and the scheduler.
var cbMu sync.Mutex

var placeholderRe = regexp.MustCompile(`^Local/([0-9]+)@kiestoon-qcbwait-`)

func (s *Server) listCallbacks(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.Callbacks(r.Context())
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if list == nil {
		list = []pbx.Callback{}
	}
	writeJSON(w, http.StatusOK, list)
}

// addCallback lets the reception put a callback in a queue by hand.
func (s *Server) addCallback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueID int64  `json:"queueId"`
		Number  string `json:"number"`
		Name    string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	q, err := s.PBX.Queue(r.Context(), req.QueueID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "unknown_queue")
		return
	}
	num := strings.TrimSpace(req.Number)
	if !regexp.MustCompile(`^\+?[0-9]{6,20}$`).MatchString(num) {
		writeErr(w, http.StatusBadRequest, "invalid_number")
		return
	}
	c := pbx.Callback{QueueID: q.ID, Number: num, Name: req.Name}
	if err := s.PBX.AddCallback(r.Context(), &c); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "callback.added", fmt.Sprintf("queue %s number %s", q.Number, num))
	go s.runDueCallbacks(context.Background())
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) cancelCallback(w http.ResponseWriter, r *http.Request) {
	cbMu.Lock()
	ok, err := s.PBX.UpdateCallback(r.Context(), pathID(r), []string{pbx.CallbackScheduled, pbx.CallbackWaiting},
		pbx.CallbackCancelled, 0, time.Time{}, "", "cancelled")
	cbMu.Unlock()
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusConflict, "not_open")
		return
	}
	s.hangupPlaceholder(r.Context(), pathID(r))
	s.audit(r, "callback.cancelled", strconv.FormatInt(pathID(r), 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// retryCallback schedules a failed or cancelled callback again.
func (s *Server) retryCallback(w http.ResponseWriter, r *http.Request) {
	cbMu.Lock()
	ok, err := s.PBX.UpdateCallback(r.Context(), pathID(r), []string{pbx.CallbackFailed, pbx.CallbackCancelled},
		pbx.CallbackScheduled, 0, time.Now(), "", "")
	cbMu.Unlock()
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusConflict, "not_closed")
		return
	}
	s.audit(r, "callback.retry", strconv.FormatInt(pathID(r), 10))
	go s.runDueCallbacks(context.Background())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) hangupPlaceholder(ctx context.Context, id int64) {
	if !s.AMI.Connected() {
		return
	}
	prefix := fmt.Sprintf("Local/%d@kiestoon-qcbwait-", id)
	s.Live.Invalidate()
	for _, call := range s.Live.Get(ctx).Calls {
		for _, c := range call.Legs {
			if strings.HasPrefix(c.Channel, prefix) {
				_ = s.AMI.Hangup(ctx, c.Channel)
			}
		}
	}
}

// onCallbackEvent handles the KiestoonCallback user events of the dialplan.
func (s *Server) onCallbackEvent(m asterisk.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch m.Get("Action") {
	case "request":
		qid, _ := strconv.ParseInt(m.Get("Queue"), 10, 64)
		pos, _ := strconv.Atoi(m.Get("Position"))
		c := pbx.Callback{QueueID: qid, Number: m.Get("Number"), Name: m.Get("Name"), Position: pos, Linkedid: m.Get("Linkedid")}
		if err := s.PBX.AddCallback(ctx, &c); err != nil {
			s.Log.Warn("queue callback", "err", err)
			return
		}
		s.Log.Info("queue callback requested", "id", c.ID, "queue", qid)
		s.runDueCallbacks(ctx)
	case "left":
		// The placeholder left the queue without an agent (timeout, queue full, no agents).
		id, _ := strconv.ParseInt(m.Get("Id"), 10, 64)
		s.callbackLater(ctx, id, []string{pbx.CallbackWaiting}, 0, "queue_left")
	case "done":
		id, _ := strconv.ParseInt(m.Get("Id"), 10, 64)
		if m.Get("Status") == "ANSWER" {
			cbMu.Lock()
			_, _ = s.PBX.UpdateCallback(ctx, id, []string{pbx.CallbackCalling}, pbx.CallbackDone, 0, time.Time{}, "", "answered")
			cbMu.Unlock()
			return
		}
		s.callbackLater(ctx, id, []string{pbx.CallbackCalling}, 1, "no_answer")
	}
}

// Callback notes are codes the portal translates: restarted, interrupted, cancelled, answered,
// queue_left, no_answer, queue_gone, redirect_failed, start_failed.

// callbackLater schedules the next attempt, or gives up after the retries or the expiry time.
func (s *Server) callbackLater(ctx context.Context, id int64, from []string, attempt int, note string) {
	cbMu.Lock()
	defer cbMu.Unlock()
	c, err := s.PBX.Callback(ctx, id)
	if err != nil {
		return
	}
	q, err := s.PBX.Queue(ctx, c.QueueID)
	if err != nil {
		_, _ = s.PBX.UpdateCallback(ctx, id, from, pbx.CallbackFailed, attempt, time.Time{}, "", "queue_gone")
		return
	}
	o := q.Options.Callback
	created, _ := time.Parse(time.RFC3339, c.CreatedAt)
	next := time.Now().Add(time.Duration(o.RetryDelay) * time.Second)
	if c.Attempts+attempt > o.Retries || next.After(created.Add(time.Duration(o.Expire)*time.Minute)) {
		_, _ = s.PBX.UpdateCallback(ctx, id, from, pbx.CallbackFailed, attempt, time.Time{}, "", note)
		return
	}
	_, _ = s.PBX.UpdateCallback(ctx, id, from, pbx.CallbackScheduled, attempt, next, "", note)
}

// onCallbackConnect moves an agent who answered a placeholder to the call back dialplan.
func (s *Server) onCallbackConnect(m asterisk.Message) {
	sm := placeholderRe.FindStringSubmatch(m.Get("Channel"))
	if sm == nil {
		return
	}
	id, _ := strconv.ParseInt(sm[1], 10, 64)
	agentChan := m.Get("DestChannel")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cbMu.Lock()
	ok, _ := s.PBX.UpdateCallback(ctx, id, []string{pbx.CallbackWaiting}, pbx.CallbackCalling, 0, time.Time{}, m.Get("MemberName"), "")
	cbMu.Unlock()
	c, err := s.PBX.Callback(ctx, id)
	if !ok || err != nil {
		_ = s.AMI.Hangup(ctx, m.Get("Channel"))
		return
	}
	cid := ""
	if q, err := s.PBX.Queue(ctx, c.QueueID); err == nil {
		cid = q.Options.Callback.CallerID
	}
	for _, v := range [][2]string{{"KT_CBID", sm[1]}, {"KT_CBCID", cid}} {
		_, _ = s.AMI.Action(ctx, asterisk.Message{{Key: "Action", Value: "Setvar"}, {Key: "Channel", Value: agentChan},
			{Key: "Variable", Value: v[0]}, {Key: "Value", Value: v[1]}})
	}
	if err := s.AMI.Redirect(ctx, agentChan, "kiestoon-qcbcall", c.Number); err != nil {
		s.Log.Warn("queue callback redirect", "id", id, "err", err)
		s.callbackLater(ctx, id, []string{pbx.CallbackCalling}, 0, "redirect_failed")
	}
}

// runCallbacks resets callbacks that lost their placeholder (restart) and starts due ones.
func (s *Server) runCallbacks(ctx context.Context) {
	reset := func() {
		open, _ := s.PBX.OpenCallbacks(ctx)
		cbMu.Lock()
		for _, c := range open {
			switch c.Status {
			case pbx.CallbackWaiting:
				_, _ = s.PBX.UpdateCallback(ctx, c.ID, []string{pbx.CallbackWaiting}, pbx.CallbackScheduled, 0, time.Now(), "", "restarted")
			case pbx.CallbackCalling:
				_, _ = s.PBX.UpdateCallback(ctx, c.ID, []string{pbx.CallbackCalling}, pbx.CallbackFailed, 0, time.Time{}, "", "interrupted")
			}
		}
		cbMu.Unlock()
	}
	reset()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	wasUp := s.AMI.Connected()
	for {
		up := s.AMI.Connected()
		if up && !wasUp {
			reset() // Asterisk restarted: the placeholders are gone.
		}
		wasUp = up
		if up {
			s.runDueCallbacks(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// runDueCallbacks puts a placeholder in the queue for every callback whose time has come.
func (s *Server) runDueCallbacks(ctx context.Context) {
	if !s.AMI.Connected() {
		return
	}
	due, err := s.PBX.DueCallbacks(ctx, time.Now())
	if err != nil {
		return
	}
	for _, c := range due {
		q, err := s.PBX.Queue(ctx, c.QueueID)
		if err != nil || !q.Enabled {
			cbMu.Lock()
			_, _ = s.PBX.UpdateCallback(ctx, c.ID, []string{pbx.CallbackScheduled}, pbx.CallbackFailed, 0, time.Time{}, "", "queue_gone")
			cbMu.Unlock()
			continue
		}
		cbMu.Lock()
		ok, _ := s.PBX.UpdateCallback(ctx, c.ID, []string{pbx.CallbackScheduled}, pbx.CallbackWaiting, 0, time.Time{}, "", "")
		cbMu.Unlock()
		if !ok {
			continue
		}
		pos := c.Position
		if c.Attempts > 0 {
			pos = 1 // a retry goes to the front
		}
		posVar := ""
		if pos > 0 {
			posVar = strconv.Itoa(pos)
		}
		name := c.Name
		if name == "" {
			name = c.Number
		}
		cidName := strings.ReplaceAll("CB "+name, `"`, "")
		err = s.AMI.OriginateApp(ctx, fmt.Sprintf("Local/%d@kiestoon-qcbwait/n", c.ID), "kiestoon-qcbhold", "s",
			`"`+cidName+`" <`+c.Number+`>`, 30*time.Second, fmt.Sprintf("KT_CBQ=%d", q.ID), "KT_CBPOS="+posVar)
		if err != nil {
			s.Log.Warn("queue callback placeholder", "id", c.ID, "err", err)
			s.callbackLater(ctx, c.ID, []string{pbx.CallbackWaiting}, 0, "start_failed")
		}
	}
}

// supervise rings the supervisor's own phone and lets them listen, whisper or barge in on the
// queue call of an agent.
func (s *Server) supervise(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Agent string `json:"agent"`
		Mode  string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	digit, ok := asterisk.SpyModes[req.Mode]
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid_mode")
		return
	}
	me := s.myExtension(w, r)
	if me == nil {
		return
	}
	snap, err := s.PBX.Snapshot(r.Context())
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if !asterisk.CanSupervise(snap, me.Number, req.Agent) {
		writeErr(w, http.StatusForbidden, "not_supervised")
		return
	}
	if !s.AMI.Connected() {
		writeErr(w, http.StatusServiceUnavailable, "pbx_offline")
		return
	}
	label := map[string]string{"listen": "Listen", "whisper": "Whisper", "barge": "Barge"}[req.Mode]
	err = s.AMI.OriginateApp(r.Context(), "Local/"+me.Number+"@kiestoon-ring/n", asterisk.SpyContext(me.Number), digit+req.Agent,
		fmt.Sprintf(`"%s %s" <%s>`, label, req.Agent, req.Agent), 30*time.Second)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "originate_failed")
		return
	}
	// Supervision is privacy sensitive: always in the audit log.
	s.audit(r, "supervise."+req.Mode, fmt.Sprintf("supervisor %s agent %s", me.Number, req.Agent))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
