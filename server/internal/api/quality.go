package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/quality"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Call quality: MOS, jitter, packet loss and round trip per call leg from the RTCP reports of
// Asterisk, per trunk and per phone, with alerts by mail.

func (s *Server) qualityRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/quality", admin(s.qualityOverview))
	mux.Handle("GET /api/quality/settings", admin(s.qualitySettingsGet))
	mux.Handle("PUT /api/quality/settings", admin(s.qualitySettingsSave))
	mux.Handle("POST /api/quality/test-mail", admin(s.qualityTestMail))
}

type qualityState struct {
	tracker *quality.Tracker
	mu      sync.Mutex
	// mailedCalls are the calls mailed by the "each poor call" rule, so a call with two poor legs
	// gives one mail.
	mailedCalls map[string]time.Time
}

func (q *qualityState) init() *quality.Tracker {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tracker == nil {
		q.tracker = quality.NewTracker()
		q.mailedCalls = map[string]time.Time{}
	}
	return q.tracker
}

// qualityEvent takes the RTCP reports and hangups of Asterisk.
func (s *Server) qualityEvent(m asterisk.Message) {
	tr := s.quality.init()
	done, isNew := tr.Event(m.Get, time.Now())
	if isNew {
		go s.qualityCodec(m.Get("Channel"), m.Get("Uniqueid"))
	}
	if done != nil {
		go s.qualityFinished(*done)
	}
}

// qualityCodec asks Asterisk for the codec of a channel: RTCP jitter is in units of its clock.
func (s *Server) qualityCodec(channel, uid string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := s.AMI.Action(ctx, asterisk.Message{{Key: "Action", Value: "Getvar"}, {Key: "Channel", Value: channel},
		{Key: "Variable", Value: "CHANNEL(audionativeformat)"}})
	if err != nil {
		return
	}
	if v := strings.Trim(resp.Get("Value"), "()"); v != "" {
		if i := strings.IndexAny(v, "|,"); i > 0 {
			v = v[:i]
		}
		s.quality.tracker.SetCodec(uid, v)
	}
}

// qualityFinished stores a leg that ended and mails it when every poor call is to be mailed.
func (s *Server) qualityFinished(l quality.Leg) {
	if l.RX.Reports+l.TX.Reports == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := (&quality.Store{DB: s.DB}).Save(ctx, l); err != nil {
		s.Log.Warn("store call quality", "err", err)
		return
	}
	set := quality.Load(ctx, s.DB)
	if !set.EachCall || !set.Email || len(set.Recipients) == 0 || !l.Poor(set.Poor) {
		return
	}
	now := time.Now()
	s.quality.mu.Lock()
	for id, at := range s.quality.mailedCalls {
		if now.Sub(at) > time.Hour {
			delete(s.quality.mailedCalls, id)
		}
	}
	_, seen := s.quality.mailedCalls[l.LinkedID]
	s.quality.mailedCalls[l.LinkedID] = now
	s.quality.mu.Unlock()
	if seen {
		return
	}
	names := s.qualityNames(ctx)
	lang := s.defaultLanguage(ctx)
	subject, body := qualityCallMail(lang, l, names, set, s.qualityURL(ctx), now.In(s.loc(ctx)))
	for _, to := range set.Recipients {
		if err := s.Mail.SendText(ctx, "quality", to, subject, body); err != nil {
			s.Log.Warn("call quality mail", "to", to, "err", err)
		}
	}
}

// runQuality checks the alert rules every minute and stores legs whose hangup was missed.
func (s *Server) runQuality(ctx context.Context) {
	tr := s.quality.init()
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			now := time.Now()
			for _, l := range tr.Stale(now, 5*time.Minute) {
				s.qualityFinished(l)
			}
			s.checkQuality(ctx, now)
		}
	}
}

type qualityNames struct {
	trunks map[int64]string
	exts   map[string]string
}

func (n qualityNames) of(kind, ref string) string {
	switch kind {
	case "trunk":
		id, _ := strconv.ParseInt(ref, 10, 64)
		if name := n.trunks[id]; name != "" {
			return name
		}
		return "trunk " + ref
	case "phone":
		if name := n.exts[ref]; name != "" {
			return ref + " " + name
		}
	}
	return ref
}

func (s *Server) qualityNames(ctx context.Context) qualityNames {
	n := qualityNames{trunks: map[int64]string{}, exts: map[string]string{}}
	if ts, err := s.PBX.Trunks(ctx); err == nil {
		for _, t := range ts {
			n.trunks[t.ID] = t.Name
		}
	}
	if es, err := s.PBX.Extensions(ctx); err == nil {
		for _, e := range es {
			n.exts[e.Number] = e.Name
		}
	}
	return n
}

// qualityRecent returns the legs of the longest rule window plus the legs going on.
func (s *Server) qualityRecent(ctx context.Context, set quality.Settings, now time.Time) ([]quality.Leg, error) {
	window := max(set.Trunk.Minutes, set.Phone.Minutes, 15)
	legs, err := (&quality.Store{DB: s.DB}).Between(ctx, now.Add(-time.Duration(window)*time.Minute), now.Add(time.Minute))
	if err != nil {
		return nil, err
	}
	return append(legs, s.quality.init().Active(now)...), nil
}

func (s *Server) checkQuality(ctx context.Context, now time.Time) {
	set := quality.Load(ctx, s.DB)
	legs, err := s.qualityRecent(ctx, set, now)
	if err != nil {
		s.Log.Warn("call quality alerts", "err", err)
		return
	}
	st := &quality.Store{DB: s.DB}
	active, err := st.ActiveAlerts(ctx)
	if err != nil {
		return
	}
	names := s.qualityNames(ctx)
	current := map[string]quality.Alert{}
	for _, a := range quality.Breaches(legs, set, now) {
		a.Name = names.of(a.Kind, a.Ref)
		current[a.Key] = a
	}
	type job struct {
		a         quality.Alert
		recovered bool
	}
	jobs := []job{}
	open := map[string]bool{}
	for _, a := range active {
		open[a.Key] = true
		if cur, ok := current[a.Key]; ok {
			cur.ID = a.ID
			_ = st.Update(ctx, cur)
			continue
		}
		_ = st.Close(ctx, a.ID, now)
		if set.Recovered && a.Mailed != "" {
			jobs = append(jobs, job{a: a, recovered: true})
		}
	}
	for key, a := range current {
		if open[key] {
			continue
		}
		last := st.LastMailed(ctx, key)
		id, err := st.Open(ctx, a, now)
		if err != nil {
			s.Log.Warn("call quality alert", "err", err)
			continue
		}
		a.ID = id
		store.Audit(ctx, s.DB, 0, "quality.alert", fmt.Sprintf("%s %s: MOS %.2f", a.Kind, a.Name, a.MOS), "")
		if last.IsZero() || now.Sub(last) >= time.Duration(set.Quiet)*time.Minute {
			jobs = append(jobs, job{a: a})
		}
	}
	if len(jobs) == 0 || !set.Email || len(set.Recipients) == 0 {
		return
	}
	lang := s.defaultLanguage(ctx)
	url := s.qualityURL(ctx)
	local := now.In(s.loc(ctx))
	for _, j := range jobs {
		if !j.recovered {
			_ = st.Mailed(ctx, j.a.ID, now)
		}
		subject, body := qualityAlertMail(lang, j.a, set, j.recovered, url, local)
		for _, to := range set.Recipients {
			if err := s.Mail.SendText(ctx, "quality", to, subject, body); err != nil {
				s.Log.Warn("call quality alert mail", "to", to, "err", err)
			}
		}
	}
}

// qualityURL is the link to the call quality page in mails, "" when the address is not known.
func (s *Server) qualityURL(ctx context.Context) string {
	if base, _ := store.GetSetting(ctx, s.DB, "public_url", ""); base != "" {
		return strings.TrimRight(base, "/") + "/quality"
	}
	return ""
}

// ---- API ----------------------------------------------------------------------

type qualityLive struct {
	ID        string  `json:"id"`
	From      string  `json:"from"`
	FromName  string  `json:"fromName"`
	To        string  `json:"to"`
	Direction string  `json:"direction"`
	Route     string  `json:"route"`
	Codec     string  `json:"codec"`
	Duration  int     `json:"duration"`
	MOS       float64 `json:"mos"`
	Jitter    float64 `json:"jitter"`
	Loss      float64 `json:"loss"`
	RTT       float64 `json:"rtt"`
}

// qualityLiveCalls joins the legs going on with the calls Asterisk reports.
func (s *Server) qualityLiveCalls(ctx context.Context, names qualityNames, now time.Time) []qualityLive {
	legs := s.quality.init().Active(now)
	byCall := map[string][]quality.Leg{}
	for _, l := range legs {
		byCall[l.LinkedID] = append(byCall[l.LinkedID], l)
	}
	out := []qualityLive{}
	if len(byCall) == 0 {
		return out
	}
	l := s.Live.Get(ctx)
	for _, c := range l.Calls {
		ls := byCall[c.ID]
		if len(ls) == 0 {
			continue
		}
		row := qualityLive{ID: c.ID, From: c.From, FromName: c.FromName, To: c.To, Direction: c.Direction, Duration: c.Duration}
		for _, leg := range ls {
			m := leg.Current
			if m == 0 {
				m = leg.MOS
			}
			if row.MOS == 0 || (m > 0 && m < row.MOS) {
				row.MOS = m
			}
			row.Jitter = max(row.Jitter, leg.RX.Jitter, leg.TX.Jitter)
			row.Loss = max(row.Loss, leg.RX.Loss, leg.TX.Loss)
			row.RTT = max(row.RTT, leg.RTT)
			if row.Codec == "" {
				row.Codec = leg.Codec
			}
			if leg.Kind == quality.KindTrunk {
				row.Route = names.of("trunk", strconv.FormatInt(leg.TrunkID, 10))
			}
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MOS < out[j].MOS })
	return out
}

func (s *Server) qualityOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	loc := s.loc(ctx)
	var from time.Time
	var bucket time.Duration
	var n int
	switch r.URL.Query().Get("range") {
	case "7d":
		// 6-hour buckets from local midnight six days ago.
		y, m, d := now.In(loc).AddDate(0, 0, -6).Date()
		from, bucket, n = time.Date(y, m, d, 0, 0, 0, 0, loc), 6*time.Hour, 28
	case "30d":
		y, m, d := now.In(loc).AddDate(0, 0, -29).Date()
		from, bucket, n = time.Date(y, m, d, 0, 0, 0, 0, loc), 24*time.Hour, 30
	default:
		from, bucket, n = now.Truncate(time.Hour).Add(-23*time.Hour), time.Hour, 24
	}
	set := quality.Load(ctx, s.DB)
	st := &quality.Store{DB: s.DB}
	legs, err := st.Between(ctx, from, now.Add(time.Minute))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	o := quality.Summarize(legs, from, bucket, n, set)
	names := s.qualityNames(ctx)
	for _, g := range o.Trunks {
		g.Name = names.of("trunk", g.Key)
	}
	for _, g := range o.Phones {
		g.Name = names.exts[g.Key]
	}
	if len(o.Phones) > 10 {
		o.Phones = o.Phones[:10]
	}
	alerts, _ := st.ActiveAlerts(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"overview": o, "live": s.qualityLiveCalls(ctx, names, now), "alerts": alerts, "lastCleared": st.LastEnded(ctx),
		"limits": qualityLimits(set), "connected": s.AMI.Connected(),
	})
}

// qualityLiveSummary is what the dashboard shows: the current MOS of each call going on and the
// active alerts.
func (s *Server) qualityLiveSummary(ctx context.Context, l *asterisk.Live) map[string]any {
	calls := map[string]float64{}
	for _, leg := range s.quality.init().Active(time.Now()) {
		m := leg.Current
		if m == 0 {
			m = leg.MOS
		}
		if cur, ok := calls[leg.LinkedID]; m > 0 && (!ok || m < cur) {
			calls[leg.LinkedID] = m
		}
	}
	alerts, err := (&quality.Store{DB: s.DB}).ActiveAlerts(ctx)
	if err != nil {
		alerts = []quality.Alert{}
	}
	set := quality.Load(ctx, s.DB)
	return map[string]any{"calls": calls, "alerts": alerts, "limits": qualityLimits(set)}
}

// qualityLimitsJSON are the limits the screens colour values with.
type qualityLimitsJSON struct {
	Good   float64 `json:"good"`
	Poor   float64 `json:"poor"`
	Jitter float64 `json:"jitter"`
	Loss   float64 `json:"loss"`
	RTT    float64 `json:"rtt"`
}

func qualityLimits(s quality.Settings) qualityLimitsJSON {
	return qualityLimitsJSON{Good: s.Good, Poor: s.Poor, Jitter: s.Jitter, Loss: s.Loss, RTT: s.RTT}
}

func (s *Server) qualitySettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, quality.Load(r.Context(), s.DB))
}

func (s *Server) qualitySettingsSave(w http.ResponseWriter, r *http.Request) {
	var set quality.Settings
	if err := decode(r, &set); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := quality.Save(r.Context(), s.DB, &set); err != nil {
		var fe *quality.FieldError
		if errors.As(err, &fe) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fe.Code, "field": fe.Field})
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	s.audit(r, "quality.settings.saved", "")
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) qualityTestMail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Recipients []string `json:"recipients"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	set := quality.Defaults()
	set.Recipients, set.Email = req.Recipients, true
	if err := set.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": "recipients"})
		return
	}
	ctx := r.Context()
	cur := quality.Load(ctx, s.DB)
	a := quality.Alert{Kind: "trunk", Ref: "0", Name: "Test", Calls: 6, Poor: 4, MOS: 3.42, Jitter: 12, Loss: 4.8, RTT: 44, Cause: quality.CauseLossFrom}
	subject, body := qualityAlertMail(s.defaultLanguage(ctx), a, cur, false, s.qualityURL(ctx), time.Now().In(s.loc(ctx)))
	subject = "[TEST] " + subject
	for _, to := range set.Recipients {
		if err := s.Mail.SendText(ctx, "quality", to, subject, body); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "mail_failed", "detail": err.Error()})
			return
		}
	}
	s.audit(r, "quality.testmail", strings.Join(set.Recipients, ", "))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// qualityLegView is a leg of a call with the name of the trunk or phone, for the call history.
type qualityLegView struct {
	quality.Leg
	Name   string `json:"name"`
	Rating string `json:"rating"`
}

// qualityCallView is the quality of one call: its worst leg and all legs.
type qualityCallView struct {
	MOS    float64           `json:"mos"`
	Rating string            `json:"rating"`
	Cause  string            `json:"cause"`
	Where  string            `json:"where"` // name of the worst leg
	Legs   []qualityLegView  `json:"legs"`
	Limits qualityLimitsJSON `json:"limits"`
}

// qualityForCall returns the quality of a call, nil when nothing was measured.
func (s *Server) qualityForCall(ctx context.Context, linkedID string) *qualityCallView {
	legs, err := (&quality.Store{DB: s.DB}).Call(ctx, linkedID)
	if err != nil {
		return nil
	}
	if len(legs) == 0 {
		// A call that is still going on.
		for _, l := range s.quality.init().Active(time.Now()) {
			if l.LinkedID == linkedID {
				legs = append(legs, l)
			}
		}
		sort.Slice(legs, func(i, j int) bool { return legs[i].Start.Before(legs[j].Start) })
	}
	set := quality.Load(ctx, s.DB)
	names := s.qualityNames(ctx)
	v := &qualityCallView{Legs: []qualityLegView{}, Limits: qualityLimits(set)}
	var worstLeg *quality.Leg
	for i, l := range legs {
		if l.MOS == 0 {
			continue
		}
		lv := qualityLegView{Leg: l, Rating: quality.Rating(l.MOS, set)}
		switch l.Kind {
		case quality.KindTrunk:
			lv.Name = names.of("trunk", strconv.FormatInt(l.TrunkID, 10))
		case quality.KindExtension:
			lv.Name = names.of("phone", l.Ext)
		default:
			lv.Name = l.Endpoint
		}
		v.Legs = append(v.Legs, lv)
		if worstLeg == nil || l.MOS < worstLeg.MOS {
			worstLeg = &legs[i]
			v.Where = lv.Name
		}
	}
	if worstLeg == nil {
		return nil
	}
	v.MOS = worstLeg.MOS
	v.Rating = quality.Rating(v.MOS, set)
	v.Cause = quality.Cause(worstLeg.MOS, worstLeg.RX.Jitter, worstLeg.TX.Jitter, worstLeg.RX.Loss, worstLeg.TX.Loss, worstLeg.RTT, set)
	return v
}

// ---- mails ----------------------------------------------------------------------

var qualityCauseText = map[string]map[string]string{
	"en": {
		quality.CauseLossTo:     "packet loss on the audio towards %s",
		quality.CauseLossFrom:   "packet loss on the audio from %s",
		quality.CauseJitterTo:   "jitter on the audio towards %s",
		quality.CauseJitterFrom: "jitter on the audio from %s",
		quality.CauseRTT:        "a long round trip to %s",
		quality.CauseMixed:      "a mix of jitter, loss and delay with %s",
		quality.CauseOK:         "no single figure over its limit with %s",
	},
	"nl": {
		quality.CauseLossTo:     "pakketverlies op de audio richting %s",
		quality.CauseLossFrom:   "pakketverlies op de audio van %s",
		quality.CauseJitterTo:   "jitter op de audio richting %s",
		quality.CauseJitterFrom: "jitter op de audio van %s",
		quality.CauseRTT:        "een lange round trip naar %s",
		quality.CauseMixed:      "een mix van jitter, verlies en vertraging met %s",
		quality.CauseOK:         "geen enkele waarde boven de drempel met %s",
	},
}

func qualityLang(lang string) string {
	if lang == "nl" {
		return "nl"
	}
	return "en"
}

func qualityFigures(lang string, mos, jitter, loss, rtt float64) string {
	if lang == "nl" {
		return fmt.Sprintf("MOS %s, jitter %.0f ms, pakketverlies %s%%, round trip %.0f ms", quality.FormatMOS(mos, lang), jitter,
			strings.Replace(fmt.Sprintf("%.1f", loss), ".", ",", 1), rtt)
	}
	return fmt.Sprintf("MOS %.2f, jitter %.0f ms, packet loss %.1f%%, round trip %.0f ms", mos, jitter, loss, rtt)
}

func qualityAlertMail(lang string, a quality.Alert, set quality.Settings, recovered bool, url string, now time.Time) (string, string) {
	lang = qualityLang(lang)
	at := now.Format("15:04")
	open := ""
	if url != "" {
		open = map[string]string{"en": "\nOpen: ", "nl": "\nOpenen: "}[lang] + url + "\n"
	}
	side := map[string]map[string]string{"en": {"trunk": "the provider", "phone": "the phone"}, "nl": {"trunk": "de provider", "phone": "het toestel"}}[lang][a.Kind]
	cause := fmt.Sprintf(qualityCauseText[lang][a.Cause], side)
	figures := qualityFigures(lang, a.MOS, a.Jitter, a.Loss, a.RTT)
	if lang == "nl" {
		what := map[string]string{"trunk": "trunk", "phone": "toestel"}[a.Kind]
		if recovered {
			return fmt.Sprintf("Gesprekskwaliteit weer in orde: %s %s", what, a.Name),
				fmt.Sprintf("De gesprekskwaliteit van %s %s zit sinds %s weer binnen de drempels.\n%s-- \nKiestoon\n", what, a.Name, at, open)
		}
		rule := fmt.Sprintf("Gemiddeld %s over de laatste %d minuten (%d gesprekken), de drempel is MOS %s.", figures, set.Trunk.Minutes, a.Calls, quality.FormatMOS(set.Poor, lang))
		if a.Kind == "phone" {
			rule = fmt.Sprintf("%d van de %d gesprekken in de laatste %d minuten lagen onder MOS %s. Gemiddeld %s.", a.Poor, a.Calls, set.Phone.Minutes, quality.FormatMOS(set.Poor, lang), figures)
		}
		return fmt.Sprintf("Melding gesprekskwaliteit: %s %s", what, a.Name),
			fmt.Sprintf("%s %s heeft om %s een slechte gesprekskwaliteit.\n%s\n\nWaarschijnlijke oorzaak: %s.\n\nJe krijgt voor deze %s pas weer een mail na de rustperiode van %d minuten.\n%s-- \nKiestoon\n",
				strings.ToUpper(what[:1])+what[1:], a.Name, at, rule, cause, what, set.Quiet, open)
	}
	what := map[string]string{"trunk": "trunk", "phone": "phone"}[a.Kind]
	if recovered {
		return fmt.Sprintf("Call quality back to normal: %s %s", what, a.Name),
			fmt.Sprintf("The call quality of %s %s is back within the limits since %s.\n%s-- \nKiestoon\n", what, a.Name, at, open)
	}
	rule := fmt.Sprintf("Average %s over the last %d minutes (%d calls), the limit is MOS %.2f.", figures, set.Trunk.Minutes, a.Calls, set.Poor)
	if a.Kind == "phone" {
		rule = fmt.Sprintf("%d of %d calls in the last %d minutes were under MOS %.2f. Average %s.", a.Poor, a.Calls, set.Phone.Minutes, set.Poor, figures)
	}
	return fmt.Sprintf("Call quality alert: %s %s", what, a.Name),
		fmt.Sprintf("%s %s has poor call quality at %s.\n%s\n\nMost likely cause: %s.\n\nYou will not get another mail for this %s until the quiet period of %d minutes has passed.\n%s-- \nKiestoon\n",
			strings.ToUpper(what[:1])+what[1:], a.Name, at, rule, cause, what, set.Quiet, open)
}

func qualityCallMail(lang string, l quality.Leg, names qualityNames, set quality.Settings, url string, now time.Time) (string, string) {
	lang = qualityLang(lang)
	kind, ref, side := "phone", l.Ext, map[string]string{"en": "the phone", "nl": "het toestel"}[lang]
	if l.Kind == quality.KindTrunk {
		kind, ref, side = "trunk", strconv.FormatInt(l.TrunkID, 10), map[string]string{"en": "the provider", "nl": "de provider"}[lang]
	}
	name := names.of(kind, ref)
	if l.Kind == quality.KindOther {
		name = l.Endpoint
	}
	cause := fmt.Sprintf(qualityCauseText[lang][quality.Cause(l.MOS, l.RX.Jitter, l.TX.Jitter, l.RX.Loss, l.TX.Loss, l.RTT, set)], side)
	figures := qualityFigures(lang, l.MOS, max(l.RX.Jitter, l.TX.Jitter), max(l.RX.Loss, l.TX.Loss), l.RTT)
	open := ""
	if url != "" {
		open = map[string]string{"en": "\nOpen: ", "nl": "\nOpenen: "}[lang] + strings.TrimSuffix(url, "/quality") + "/history?call=" + l.LinkedID + "\n"
	}
	at := l.End.In(now.Location()).Format("15:04")
	if lang == "nl" {
		return fmt.Sprintf("Slecht gesprek: %s", name),
			fmt.Sprintf("Een gesprek via %s dat om %s eindigde had een slechte kwaliteit: %s.\n\nWaarschijnlijke oorzaak: %s.\n%s-- \nKiestoon\n", name, at, figures, cause, open)
	}
	return fmt.Sprintf("Poor call: %s", name),
		fmt.Sprintf("A call via %s that ended at %s had poor quality: %s.\n\nMost likely cause: %s.\n%s-- \nKiestoon\n", name, at, figures, cause, open)
}

// purgeQuality removes measurements older than the retention.
func (s *Server) purgeQuality(ctx context.Context) {
	set := quality.Load(ctx, s.DB)
	if n, err := (&quality.Store{DB: s.DB}).Purge(ctx, set.KeepDays, time.Now()); err != nil {
		s.Log.Warn("purge call quality", "err", err)
	} else if n > 0 {
		s.Log.Info("old call quality measurements removed", "count", n)
	}
}

// qualityMetrics adds the call quality of the last 15 minutes to the Prometheus metrics.
func (s *Server) qualityMetrics(ctx context.Context, p *promWriter) {
	now := time.Now()
	from := now.Add(-15 * time.Minute)
	legs, err := (&quality.Store{DB: s.DB}).Between(ctx, from, now.Add(time.Minute))
	if err != nil {
		return
	}
	active := s.quality.init().Active(now)
	set := quality.Load(ctx, s.DB)
	o := quality.Summarize(append(legs, active...), from, 16*time.Minute, 1, set)
	if o.Total.Legs > 0 {
		p.metric("kiestoon_call_mos", "gauge", "Average MOS of the call legs of the last 15 minutes.", o.Total.MOS)
	}
	p.metric("kiestoon_call_poor_total", "gauge", "Calls under the poor MOS limit in the last 15 minutes.", float64(o.Total.Poor))
	names := s.qualityNames(ctx)
	for _, g := range o.Trunks {
		name := names.of("trunk", g.Key)
		p.metric("kiestoon_trunk_mos", "gauge", "Average MOS of a trunk over the last 15 minutes.", g.MOS, "trunk", name)
		p.metric("kiestoon_trunk_jitter_ms", "gauge", "Average jitter of a trunk over the last 15 minutes (worst direction).", g.Jitter, "trunk", name)
		p.metric("kiestoon_trunk_packet_loss_percent", "gauge", "Average packet loss of a trunk over the last 15 minutes (worst direction).", g.Loss, "trunk", name)
		p.metric("kiestoon_trunk_rtt_ms", "gauge", "Average round trip of a trunk over the last 15 minutes.", g.RTT, "trunk", name)
	}
	worstActive := 0.0
	for _, l := range active {
		m := l.Current
		if m == 0 {
			m = l.MOS
		}
		if m > 0 && (worstActive == 0 || m < worstActive) {
			worstActive = m
		}
	}
	if worstActive > 0 {
		p.metric("kiestoon_active_call_mos_min", "gauge", "Lowest current MOS of the calls going on.", worstActive)
	}
	if alerts, err := (&quality.Store{DB: s.DB}).ActiveAlerts(ctx); err == nil {
		p.metric("kiestoon_quality_alerts_active", "gauge", "Call quality alerts that are active.", float64(len(alerts)))
	}
}
