package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/qstats"
)

// Wallboard v2: the figures of today per queue, the agents, SLA thresholds with alerts, and tv
// links that show the wallboard without signing in.

func (s *Server) wallboardRoutes(mux *http.ServeMux, admin, recep func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/reception/wallboard", recep(s.wallboardData))
	mux.HandleFunc("GET /api/wall/{key}", s.wallScreenData)
	mux.HandleFunc("POST /api/wall/{key}/pin", s.wallScreenPin)
	mux.Handle("GET /api/wallboard/settings", admin(s.wallSettings))
	mux.Handle("POST /api/wallboard/screens", admin(s.wallScreenSave))
	mux.Handle("PUT /api/wallboard/screens/{id}", admin(s.wallScreenSave))
	mux.Handle("DELETE /api/wallboard/screens/{id}", admin(s.wallScreenDelete))
	mux.Handle("POST /api/wallboard/screens/{id}/key", admin(s.wallScreenKey))
	mux.Handle("POST /api/wallboard/screens/{id}/publish", admin(s.wallScreenPublish))
	mux.Handle("PUT /api/wallboard/alerts", admin(s.wallAlertsSave))
	mux.Handle("PUT /api/wallboard/sla", admin(s.wallSLASave))
}

// wallState is what the wallboard keeps in memory: since when each agent is in their state, the
// queue call each agent is in, the last figures and the SLA alerts that are active.
type wallState struct {
	mu      sync.Mutex
	agents  map[string]agentTrack
	calls   map[string]queueCallOf
	cache   *wallData
	cacheAt time.Time
	// past are the calls of yesterday and the 6 days before today, read again every minute.
	past   []qstats.Call
	pastAt time.Time
	pastOf time.Time
	active map[string]*slaAlert
	mailed map[string]time.Time
}

type agentTrack struct {
	state string
	since time.Time
}

type queueCallOf struct {
	queue  int64
	number string
	name   string
	since  time.Time
}

func (w *wallState) init() {
	if w.agents == nil {
		w.agents, w.calls = map[string]agentTrack{}, map[string]queueCallOf{}
		w.active, w.mailed = map[string]*slaAlert{}, map[string]time.Time{}
	}
}

type wallCaller struct {
	Position int    `json:"position"`
	Number   string `json:"number"`
	Name     string `json:"name"`
	Wait     int    `json:"wait"`
	Callback bool   `json:"callback"`
}

type wallQueue struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Number      string          `json:"number"`
	SLA         pbx.SLAOptions  `json:"sla"`
	Supervision bool            `json:"supervision"`
	Live        bool            `json:"live"`
	Waiting     int             `json:"waiting"`
	Callbacks   int             `json:"callbacks"`
	LongestWait int             `json:"longestWait"`
	Callers     []wallCaller    `json:"callers"`
	Free        int             `json:"free"`
	Busy        int             `json:"busy"`
	Paused      int             `json:"paused"`
	Today       qstats.Totals   `json:"today"`
	Hours       [24]qstats.Hour `json:"hours"`
	// LastHour, Yesterday and Week (the last 7 days, today included) are for the designer's tiles.
	LastHour  qstats.Totals `json:"lastHour"`
	Yesterday qstats.Totals `json:"yesterday"`
	Week      qstats.Totals `json:"week"`
}

// Agent states on the wallboard.
const (
	agentFree     = "free"
	agentRinging  = "ringing"
	agentCall     = "call"
	agentInternal = "internal"
	agentPaused   = "paused"
	agentOffline  = "offline"
)

type wallAgent struct {
	Number string  `json:"number"`
	Name   string  `json:"name"`
	State  string  `json:"state"`
	Since  int64   `json:"since"` // unix seconds
	Queues []int64 `json:"queues"`
	// Queue and Caller are set while the agent is in a queue call.
	Queue      int64  `json:"queue,omitempty"`
	Caller     string `json:"caller,omitempty"`
	CallsToday int    `json:"callsToday"`
}

type slaAlert struct {
	QueueID int64  `json:"queueId"`
	Queue   string `json:"queue"`
	// Kind is waiting, wait, level or abandoned.
	Kind  string  `json:"kind"`
	Value float64 `json:"value"`
	Limit float64 `json:"limit"`
	Since string  `json:"since"`

	since time.Time
}

type wallScreenInfo struct {
	Name    string   `json:"name"`
	Layouts []string `json:"layouts"`
	Rotate  int      `json:"rotate"`
	Theme   string   `json:"theme"`
	// Design is the published design of the designer; nil = the classic layouts.
	Design *pbx.WallDesign `json:"design,omitempty"`
}

type wallData struct {
	Now    string          `json:"now"`
	Live   bool            `json:"live"`
	Queues []wallQueue     `json:"queues"`
	Agents []wallAgent     `json:"agents"`
	Alerts []slaAlert      `json:"alerts"`
	Screen *wallScreenInfo `json:"screen,omitempty"`
}

// queueStatEvent keeps the statistics and the agents' queue calls up to date.
func (s *Server) queueStatEvent(m asterisk.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now()
	if err := (&qstats.Store{DB: s.DB}).Event(ctx, m.Get, now); err != nil {
		s.Log.Warn("queue statistics", "err", err)
	}
	ev := m.Get("Event")
	if ev != "AgentConnect" && ev != "AgentComplete" {
		return
	}
	agent := qstats.AgentNumber(m.Get("Interface"))
	s.wall.mu.Lock()
	defer s.wall.mu.Unlock()
	s.wall.init()
	if ev == "AgentConnect" {
		s.wall.calls[agent] = queueCallOf{queue: qstats.QueueID(m.Get("Queue")), number: m.Get("CallerIDNum"), name: m.Get("CallerIDName"), since: now}
	} else {
		delete(s.wall.calls, agent)
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// pastCalls returns the queue calls of the 7 days before today (cached for a minute).
func (s *Server) pastCalls(ctx context.Context, today time.Time) []qstats.Call {
	s.wall.mu.Lock()
	if s.wall.pastOf.Equal(today) && time.Since(s.wall.pastAt) < time.Minute {
		p := s.wall.past
		s.wall.mu.Unlock()
		return p
	}
	s.wall.mu.Unlock()
	calls, err := (&qstats.Store{DB: s.DB}).Calls(ctx, today.AddDate(0, 0, -6), today)
	if err != nil {
		s.Log.Warn("queue statistics", "err", err)
	}
	s.wall.mu.Lock()
	s.wall.past, s.wall.pastAt, s.wall.pastOf = calls, time.Now(), today
	s.wall.mu.Unlock()
	return calls
}

// wallBuild returns the wallboard figures of all queues, at most two seconds old.
func (s *Server) wallBuild(ctx context.Context) *wallData {
	s.wall.mu.Lock()
	if s.wall.cache != nil && time.Since(s.wall.cacheAt) < 2*time.Second {
		d := s.wall.cache
		s.wall.mu.Unlock()
		return d
	}
	s.wall.mu.Unlock()

	loc := s.loc(ctx)
	now := time.Now().In(loc)
	qs, _ := s.PBX.Queues(ctx)
	target := map[int64]int{}
	for _, q := range qs {
		target[q.ID] = q.Options.SLA.Target
	}
	calls, err := (&qstats.Store{DB: s.DB}).Calls(ctx, startOfDay(now), now.Add(time.Minute))
	if err != nil {
		s.Log.Warn("queue statistics", "err", err)
	}
	tgt := func(id int64) int { return target[id] }
	sum := qstats.Summarize(calls, tgt, loc)
	var recent []qstats.Call
	for _, c := range calls {
		if !c.Joined.Before(now.Add(-time.Hour)) {
			recent = append(recent, c)
		}
	}
	hour := qstats.Summarize(recent, tgt, loc)
	past := s.pastCalls(ctx, startOfDay(now))
	var yest []qstats.Call
	for _, c := range past {
		if !c.Joined.Before(startOfDay(now).AddDate(0, 0, -1)) {
			yest = append(yest, c)
		}
	}
	yday := qstats.Summarize(yest, tgt, loc)
	week := qstats.Summarize(append(slices.Clip(past), calls...), tgt, loc)
	totalsOf := func(m map[int64]*qstats.Totals, id int64) qstats.Totals {
		if t := m[id]; t != nil {
			return *t
		}
		var t qstats.Totals
		t.Finish()
		return t
	}

	live := map[int64]asterisk.QueueState{}
	connected := s.AMI.Connected()
	if connected {
		for _, st := range s.States.Queues(ctx) {
			live[st.ID] = st
		}
	}
	cbs := map[int64]pbx.Callback{}
	if list, err := s.PBX.Callbacks(ctx); err == nil {
		for _, c := range list {
			cbs[c.ID] = c
		}
	}

	d := &wallData{Now: now.Format(time.RFC3339), Live: connected, Queues: []wallQueue{}, Agents: []wallAgent{}}
	type member struct {
		asterisk.QueueMemberState
		queues []int64
	}
	members := map[string]*member{}
	order := []string{}
	for _, q := range qs {
		if !q.Enabled {
			continue
		}
		wq := wallQueue{ID: q.ID, Name: q.Name, Number: q.Number, SLA: q.Options.SLA, Supervision: q.Options.Supervision, Callers: []wallCaller{}}
		if t := sum.Queues[q.ID]; t != nil {
			wq.Today, wq.Hours = *t, *sum.Hours[q.ID]
		} else {
			wq.Today.Finish()
		}
		wq.LastHour, wq.Yesterday, wq.Week = totalsOf(hour.Queues, q.ID), totalsOf(yday.Queues, q.ID), totalsOf(week.Queues, q.ID)
		if st, ok := live[q.ID]; ok {
			wq.Live = true
			for _, c := range st.Callers {
				wc := wallCaller{Position: c.Position, Number: c.CallerID, Name: c.Name, Wait: c.Wait}
				if m := placeholderRe.FindStringSubmatch(c.Channel); m != nil {
					wc.Callback = true
					wq.Callbacks++
					id, _ := strconv.ParseInt(m[1], 10, 64)
					if cb, ok := cbs[id]; ok {
						wc.Number, wc.Name = cb.Number, cb.Name
						if t, err := time.Parse("2006-01-02T15:04:05Z", cb.CreatedAt); err == nil {
							wc.Wait = int(time.Since(t).Seconds())
						}
					}
				} else if c.Wait > wq.LongestWait {
					wq.LongestWait = c.Wait
				}
				wq.Callers = append(wq.Callers, wc)
			}
			wq.Waiting = len(st.Callers)
			for _, m := range st.Members {
				switch {
				case m.Paused:
					wq.Paused++
				case m.InCall || m.Status == "inuse" || m.Status == "busy" || m.Status == "ringing" || m.Status == "ringinuse" || m.Status == "onhold":
					wq.Busy++
				case m.Status == "idle" || m.Status == "unknown" || m.Status == "":
					wq.Free++
				}
				if members[m.Number] == nil {
					members[m.Number] = &member{QueueMemberState: m}
					order = append(order, m.Number)
				}
				mm := members[m.Number]
				mm.queues = append(mm.queues, q.ID)
				mm.Paused = mm.Paused || m.Paused
				mm.InCall = mm.InCall || m.InCall
			}
		}
		d.Queues = append(d.Queues, wq)
	}

	s.wall.mu.Lock()
	defer s.wall.mu.Unlock()
	s.wall.init()
	seen := map[string]bool{}
	for _, num := range order {
		m := members[num]
		a := wallAgent{Number: num, Name: m.Name, Queues: m.queues}
		if st := sum.Agents[num]; st != nil {
			a.CallsToday = st.Calls
		}
		call, inQueueCall := s.wall.calls[num]
		switch {
		case m.Paused:
			a.State = agentPaused
		case m.Status == "unavailable" || m.Status == "invalid":
			a.State = agentOffline
		case inQueueCall && m.Status != "idle":
			a.State, a.Queue, a.Caller = agentCall, call.queue, call.number
		case m.Status == "ringing" || m.Status == "ringinuse":
			a.State = agentRinging
		case m.InCall:
			a.State = agentCall
		case m.Status == "inuse" || m.Status == "busy" || m.Status == "onhold":
			a.State = agentInternal
		default:
			a.State = agentFree
		}
		tr, ok := s.wall.agents[num]
		if !ok || tr.state != a.State {
			tr = agentTrack{state: a.State, since: now}
			switch {
			case a.State == agentCall && inQueueCall:
				tr.since = call.since
			case a.State == agentFree && !ok && m.LastCall > 0:
				tr.since = time.Unix(m.LastCall, 0)
			}
			s.wall.agents[num] = tr
		}
		a.Since = tr.since.Unix()
		seen[num] = true
		d.Agents = append(d.Agents, a)
	}
	for num := range s.wall.agents {
		if !seen[num] {
			delete(s.wall.agents, num)
		}
	}
	for num, c := range s.wall.calls {
		if time.Since(c.since) > 12*time.Hour {
			delete(s.wall.calls, num)
		}
	}
	rank := map[string]int{agentCall: 0, agentRinging: 1, agentInternal: 2, agentFree: 3, agentPaused: 4, agentOffline: 5}
	sort.SliceStable(d.Agents, func(i, j int) bool { return rank[d.Agents[i].State] < rank[d.Agents[j].State] })
	d.Alerts = s.activeAlertsLocked()
	s.wall.cache, s.wall.cacheAt = d, time.Now()
	return d
}

func (s *Server) activeAlertsLocked() []slaAlert {
	out := []slaAlert{}
	for _, a := range s.wall.active {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].since.Before(out[j].since) })
	return out
}

// forScreen limits the figures to the queues of a tv link and hides what a tv should not show.
func (d *wallData) forScreen(w *pbx.WallScreen, alertsOn bool) *wallData {
	out := &wallData{Now: d.Now, Live: d.Live, Queues: []wallQueue{}, Agents: []wallAgent{}, Alerts: []slaAlert{},
		Screen: &wallScreenInfo{Name: w.Name, Layouts: w.Layouts, Rotate: w.Rotate, Theme: w.Theme, Design: w.Published}}
	keep := func(id int64) bool { return len(w.Queues) == 0 || slices.Contains(w.Queues, id) }
	mask := func(n string) string {
		if !w.HideNames {
			return n
		}
		if len(n) <= 4 {
			return "••••"
		}
		return "••• " + n[len(n)-4:]
	}
	for _, q := range d.Queues {
		if !keep(q.ID) {
			continue
		}
		q.Supervision = false
		cs := make([]wallCaller, len(q.Callers))
		for i, c := range q.Callers {
			c.Number = mask(c.Number)
			if w.HideNames {
				c.Name = ""
			}
			cs[i] = c
		}
		q.Callers = cs
		out.Queues = append(out.Queues, q)
	}
	for _, a := range d.Agents {
		qs := []int64{}
		for _, id := range a.Queues {
			if keep(id) {
				qs = append(qs, id)
			}
		}
		if len(qs) == 0 {
			continue
		}
		a.Queues = qs
		a.Caller = mask(a.Caller)
		out.Agents = append(out.Agents, a)
	}
	if alertsOn {
		for _, a := range d.Alerts {
			if keep(a.QueueID) {
				out.Alerts = append(out.Alerts, a)
			}
		}
	}
	return out
}

func (s *Server) wallboardData(w http.ResponseWriter, r *http.Request) {
	d := *s.wallBuild(r.Context())
	if !pbx.LoadAlertSettings(r.Context(), s.DB).Screens {
		d.Alerts = []slaAlert{}
	}
	writeJSON(w, http.StatusOK, d)
}

// wallScreen finds the screen of a tv address: the secret link /wall/<key>, or the screen's own
// address /wall/<slug> when its access allows that. A tv that typed the pin code sends the key
// in X-Wall-Key. It writes the error itself and returns nil when the screen may not be shown.
func (s *Server) wallScreen(w http.ResponseWriter, r *http.Request, pinOK bool) *pbx.WallScreen {
	ip := clientIP(r)
	if s.provLimit.Blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return nil
	}
	ref := r.PathValue("key")
	sc, err := s.PBX.WallScreenByKey(r.Context(), ref)
	if err != nil {
		sc, err = s.PBX.WallScreenBySlug(r.Context(), ref)
		if err == nil && sc.Access == pbx.AccessLink {
			err = pbx.ErrNotFound
		}
		if err == nil && sc.Access == pbx.AccessPin && !pinOK {
			key := r.Header.Get("X-Wall-Key")
			if key == "" {
				writeErr(w, http.StatusUnauthorized, "pin_required")
				return nil
			}
			if subtle.ConstantTimeCompare([]byte(key), []byte(sc.Key)) != 1 {
				s.provLimit.Fail(ip)
				writeErr(w, http.StatusUnauthorized, "pin_required")
				return nil
			}
		}
	}
	if err != nil {
		s.provLimit.Fail(ip)
		writeErr(w, http.StatusNotFound, "not_found")
		return nil
	}
	if !sc.Allows(ip) {
		writeErr(w, http.StatusForbidden, "network_not_allowed")
		return nil
	}
	return sc
}

// wallScreenData serves a tv link. It needs no session: the key in the link, or the access of
// the screen's own address, decides.
func (s *Server) wallScreenData(w http.ResponseWriter, r *http.Request) {
	sc := s.wallScreen(w, r, false)
	if sc == nil {
		return
	}
	s.PBX.TouchWallScreen(r.Context(), sc.ID, time.Now())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.wallBuild(r.Context()).forScreen(sc, pbx.LoadAlertSettings(r.Context(), s.DB).Screens))
}

// wallScreenPin checks the pin code of a screen and gives the tv the key to remember.
func (s *Server) wallScreenPin(w http.ResponseWriter, r *http.Request) {
	sc := s.wallScreen(w, r, true)
	if sc == nil {
		return
	}
	var in struct {
		Pin string `json:"pin"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if sc.Access != pbx.AccessPin || !sc.CheckPin(in.Pin) {
		s.provLimit.Fail(clientIP(r))
		writeErr(w, http.StatusUnauthorized, "wrong_pin")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": sc.Key})
}

// ---- settings ---------------------------------------------------------------

func (s *Server) wallSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	screens, err := s.PBX.WallScreens(ctx)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if screens == nil {
		screens = []pbx.WallScreen{}
	}
	type queueSLA struct {
		ID     int64          `json:"id"`
		Name   string         `json:"name"`
		Number string         `json:"number"`
		SLA    pbx.SLAOptions `json:"sla"`
		Alerts []string       `json:"alerts"`
	}
	d := s.wallBuild(ctx)
	qs, _ := s.PBX.Queues(ctx)
	out := []queueSLA{}
	for _, q := range qs {
		v := queueSLA{ID: q.ID, Name: q.Name, Number: q.Number, SLA: q.Options.SLA, Alerts: []string{}}
		for _, a := range d.Alerts {
			if a.QueueID == q.ID {
				v.Alerts = append(v.Alerts, a.Kind)
			}
		}
		out = append(out, v)
	}
	tcs, _ := s.PBX.TimeConditions(ctx)
	type tcRef struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	refs := []tcRef{}
	for _, t := range tcs {
		refs = append(refs, tcRef{t.ID, t.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"screens": screens, "alerts": pbx.LoadAlertSettings(ctx, s.DB), "queues": out, "timeConditions": refs})
}

func (s *Server) wallScreenSave(w http.ResponseWriter, r *http.Request) {
	var sc pbx.WallScreen
	if err := decode(r, &sc); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sc.ID = pathID(r)
	if r.Method == http.MethodPut && sc.ID == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.PBX.SaveWallScreen(r.Context(), &sc); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "wallboard.screen.saved", sc.Name)
	list, _ := s.PBX.WallScreens(r.Context())
	for _, x := range list {
		if x.ID == sc.ID {
			writeJSON(w, http.StatusOK, x)
			return
		}
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) wallScreenPublish(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if err := s.PBX.PublishWallScreen(r.Context(), id, time.Now()); err != nil {
		s.saveErr(w, err)
		return
	}
	sc, err := s.PBX.WallScreen(r.Context(), id)
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "wallboard.screen.published", sc.Name)
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) wallScreenDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteWallScreen(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "wallboard.screen.revoked", strconv.FormatInt(pathID(r), 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) wallScreenKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.PBX.NewWallScreenKey(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "wallboard.screen.newkey", strconv.FormatInt(pathID(r), 10))
	writeJSON(w, http.StatusOK, map[string]string{"key": key})
}

func (s *Server) wallAlertsSave(w http.ResponseWriter, r *http.Request) {
	var a pbx.AlertSettings
	if err := decode(r, &a); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := pbx.SaveAlertSettings(r.Context(), s.PBX, &a); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "wallboard.alerts.saved", "")
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) wallSLASave(w http.ResponseWriter, r *http.Request) {
	var req []struct {
		QueueID int64          `json:"queueId"`
		SLA     pbx.SLAOptions `json:"sla"`
	}
	if err := decode(r, &req); err != nil || len(req) > 200 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	for _, q := range req {
		if err := s.PBX.SetQueueSLA(r.Context(), q.QueueID, q.SLA); err != nil {
			s.saveErr(w, err)
			return
		}
	}
	s.audit(r, "wallboard.sla.saved", fmt.Sprintf("%d queues", len(req)))
	// The service level target is in queues.conf.
	s.Ast.Changed()
	s.wall.mu.Lock()
	s.wall.cache = nil
	s.wall.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- SLA alerts -------------------------------------------------------------

// Minimum number of calls today before the service level and abandon rate are judged.
const (
	minLevelCalls   = 5
	minAbandonCalls = 10
)

// slaBreaches returns the thresholds a queue is over now.
func slaBreaches(q wallQueue) []slaAlert {
	out := []slaAlert{}
	o, t := q.SLA, q.Today
	add := func(kind string, v, limit float64) {
		out = append(out, slaAlert{QueueID: q.ID, Queue: q.Name, Kind: kind, Value: v, Limit: limit})
	}
	if !q.Live {
		return out
	}
	if o.MaxWaiting > 0 && q.Waiting > o.MaxWaiting {
		add("waiting", float64(q.Waiting), float64(o.MaxWaiting))
	}
	if o.MaxWait > 0 && q.LongestWait > o.MaxWait {
		add("wait", float64(q.LongestWait), float64(o.MaxWait))
	}
	if o.Norm > 0 && t.ServiceLevel >= 0 && t.Answered+t.Abandoned-t.ShortAbandoned >= minLevelCalls && t.ServiceLevel < float64(o.Norm) {
		add("level", t.ServiceLevel, float64(o.Norm))
	}
	if o.MaxAbandoned > 0 && t.AbandonedPct >= 0 && t.Offered-t.ShortAbandoned >= minAbandonCalls && t.AbandonedPct > float64(o.MaxAbandoned) {
		add("abandoned", t.AbandonedPct, float64(o.MaxAbandoned))
	}
	return out
}

// runSLA checks the thresholds every few seconds.
func (s *Server) runSLA(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if s.AMI.Connected() {
				s.checkSLA(ctx, time.Now())
			}
		}
	}
}

func (s *Server) checkSLA(ctx context.Context, now time.Time) {
	d := s.wallBuild(ctx)
	current := map[string]slaAlert{}
	for _, q := range d.Queues {
		for _, a := range slaBreaches(q) {
			current[fmt.Sprintf("%d/%s", a.QueueID, a.Kind)] = a
		}
	}
	set := pbx.LoadAlertSettings(ctx, s.DB)
	type mailJob struct {
		a         slaAlert
		recovered bool
	}
	jobs := []mailJob{}
	s.wall.mu.Lock()
	s.wall.init()
	for k, a := range current {
		if cur, ok := s.wall.active[k]; ok {
			cur.Value = a.Value
			continue
		}
		a.since = now
		a.Since = now.UTC().Format(time.RFC3339)
		s.wall.active[k] = &a
		go s.logSLAAlert(context.Background(), a, now)
		if last, ok := s.wall.mailed[k]; !ok || now.Sub(last) >= time.Duration(set.Quiet)*time.Minute {
			jobs = append(jobs, mailJob{a: a})
		}
	}
	for k, a := range s.wall.active {
		if _, ok := current[k]; ok {
			continue
		}
		delete(s.wall.active, k)
		if _, ok := s.wall.mailed[k]; ok && set.Recovered {
			jobs = append(jobs, mailJob{a: *a, recovered: true})
		}
	}
	if len(jobs) > 0 {
		s.wall.cache = nil
	}
	s.wall.mu.Unlock()
	if len(jobs) == 0 || !set.Email || len(set.Recipients) == 0 || !s.alertHours(ctx, set, now) {
		return
	}
	lang := s.defaultLanguage(ctx)
	for _, j := range jobs {
		if !j.recovered {
			s.wall.mu.Lock()
			s.wall.mailed[fmt.Sprintf("%d/%s", j.a.QueueID, j.a.Kind)] = now
			s.wall.mu.Unlock()
		}
		subject, body := slaMail(lang, j.a, j.recovered, now.In(s.loc(ctx)))
		go func() {
			mctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			for _, to := range set.Recipients {
				if err := s.Mail.SendText(mctx, "sla", to, subject, body); err != nil {
					s.Log.Warn("sla alert mail", "to", to, "err", err)
				}
			}
		}()
	}
}

// alertHours reports whether alerts may be mailed now (the opening hours of the chosen time condition).
func (s *Server) alertHours(ctx context.Context, set pbx.AlertSettings, now time.Time) bool {
	if set.TimeCondition == 0 {
		return true
	}
	tc, err := s.PBX.TimeCondition(ctx, set.TimeCondition)
	if err != nil {
		return true
	}
	local := now.In(s.loc(ctx))
	h, _ := pbx.LoadHolidays(ctx, s.DB)
	return tc.OpenAt(local, h.ForYear(local.Year()))
}

// slaValue formats the value of an alert: seconds as m:ss, percentages with a % sign.
func slaValue(kind string, v float64) string {
	switch kind {
	case "wait":
		n := int(v)
		return fmt.Sprintf("%d:%02d", n/60, n%60)
	case "level", "abandoned":
		return fmt.Sprintf("%.0f%%", v)
	}
	return strconv.Itoa(int(v))
}

func slaMail(lang string, a slaAlert, recovered bool, now time.Time) (string, string) {
	val, limit := slaValue(a.Kind, a.Value), slaValue(a.Kind, a.Limit)
	at := now.Format("15:04")
	if lang == "nl" {
		what := map[string]string{
			"waiting":   fmt.Sprintf("%s bellers wachten, de drempel is %s", val, limit),
			"wait":      fmt.Sprintf("de langste wachttijd is %s, de drempel is %s", val, limit),
			"level":     fmt.Sprintf("het servicelevel van vandaag is %s, de norm is %s", val, limit),
			"abandoned": fmt.Sprintf("%s van de bellers heeft vandaag opgehangen, de drempel is %s", val, limit),
		}[a.Kind]
		if recovered {
			return fmt.Sprintf("SLA weer in orde: %s", a.Queue),
				fmt.Sprintf("Wachtrij %s zit sinds %s weer binnen de drempel (%s).\n\n-- \nKiestoon wallboard\n", a.Queue, at, slaKindNL[a.Kind])
		}
		return fmt.Sprintf("SLA-melding: %s, %s", a.Queue, slaKindNL[a.Kind]),
			fmt.Sprintf("Wachtrij %s overschrijdt om %s een SLA-drempel: %s.\n\nJe krijgt voor deze drempel van deze wachtrij pas weer een mail na de rustperiode.\nDe drempels pas je aan onder Receptie › Wallboard › Instellingen.\n\n-- \nKiestoon wallboard\n", a.Queue, at, what)
	}
	what := map[string]string{
		"waiting":   fmt.Sprintf("%s callers are waiting, the limit is %s", val, limit),
		"wait":      fmt.Sprintf("the longest wait is %s, the limit is %s", val, limit),
		"level":     fmt.Sprintf("today's service level is %s, the norm is %s", val, limit),
		"abandoned": fmt.Sprintf("%s of today's callers hung up, the limit is %s", val, limit),
	}[a.Kind]
	if recovered {
		return fmt.Sprintf("SLA back to normal: %s", a.Queue),
			fmt.Sprintf("Queue %s is back within its threshold (%s) since %s.\n\n-- \nKiestoon wallboard\n", a.Queue, slaKindEN[a.Kind], at)
	}
	return fmt.Sprintf("SLA alert: %s, %s", a.Queue, slaKindEN[a.Kind]),
		fmt.Sprintf("Queue %s is over an SLA threshold at %s: %s.\n\nYou will not get another mail for this threshold of this queue until the quiet period has passed.\nChange the thresholds under Reception › Wallboard › Settings.\n\n-- \nKiestoon wallboard\n", a.Queue, at, what)
}

var (
	slaKindEN = map[string]string{"waiting": "callers waiting", "wait": "wait time", "level": "service level", "abandoned": "hung up"}
	slaKindNL = map[string]string{"waiting": "wachtende bellers", "wait": "wachttijd", "level": "servicelevel", "abandoned": "opgehangen"}
)

// purgeQueueStats keeps the queue statistics for reports for 400 days.
func (s *Server) purgeQueueStats(ctx context.Context) {
	if n, err := (&qstats.Store{DB: s.DB}).Purge(ctx, 400, time.Now()); err != nil {
		s.Log.Warn("purge queue statistics", "err", err)
	} else if n > 0 {
		s.Log.Info("old queue statistics removed", "count", n)
	}
}
