package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"html"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/mail"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/qstats"
)

// Scheduled reports: an HTML mail with the queue figures of a day, week or month, compared with
// the period before, and the figures per day and agent as a CSV attachment.

func (s *Server) reportRoutes(mux *http.ServeMux, admin func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/wallboard/reports", admin(s.reportList))
	mux.Handle("POST /api/wallboard/reports", admin(s.reportSave))
	mux.Handle("PUT /api/wallboard/reports/{id}", admin(s.reportSave))
	mux.Handle("DELETE /api/wallboard/reports/{id}", admin(s.reportDelete))
	mux.Handle("GET /api/wallboard/reports/{id}/preview", admin(s.reportPreview))
	mux.Handle("POST /api/wallboard/reports/{id}/send", admin(s.reportSendMe))
}

func (s *Server) reportList(w http.ResponseWriter, r *http.Request) {
	list, err := s.PBX.Reports(r.Context())
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if list == nil {
		list = []pbx.Report{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) reportSave(w http.ResponseWriter, r *http.Request) {
	var rep pbx.Report
	if err := decode(r, &rep); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	rep.ID = pathID(r)
	if r.Method == http.MethodPut && rep.ID == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if r.Method == http.MethodPut {
		// A changed schedule must not send the report again for a period already sent.
		if old, err := s.PBX.Report(r.Context(), rep.ID); err == nil {
			rep.LastSent = old.LastSent
		}
	}
	if err := s.PBX.SaveReport(r.Context(), &rep); err != nil {
		s.saveErr(w, err)
		return
	}
	if r.Method == http.MethodPost {
		// Do not send a new report straight away for a moment that has already passed today.
		s.PBX.ReportSent(r.Context(), rep.ID, time.Now())
	}
	s.audit(r, "report.saved", rep.Name)
	saved, _ := s.PBX.Report(r.Context(), rep.ID)
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) reportDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.PBX.DeleteReport(r.Context(), pathID(r)); err != nil {
		s.saveErr(w, err)
		return
	}
	s.audit(r, "report.deleted", strconv.FormatInt(pathID(r), 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) reportPreview(w http.ResponseWriter, r *http.Request) {
	rep, err := s.PBX.Report(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	out, err := s.buildReport(r.Context(), rep, time.Now().In(s.loc(r.Context())))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"subject": out.subject, "html": out.html, "filename": out.filename})
}

// reportSendMe sends a report now to the signed-in administrator.
func (s *Server) reportSendMe(w http.ResponseWriter, r *http.Request) {
	rep, err := s.PBX.Report(r.Context(), pathID(r))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	sess := sessionFrom(r)
	if sess == nil || sess.User.Email == "" {
		writeErr(w, http.StatusConflict, "no_email")
		return
	}
	out, err := s.buildReport(r.Context(), rep, time.Now().In(s.loc(r.Context())))
	if err != nil {
		s.saveErr(w, err)
		return
	}
	if err := s.Mail.SendHTML(r.Context(), "report", sess.User.Email, out.subject, out.text, out.html, out.csv); err != nil {
		writeErr(w, http.StatusBadGateway, "mail_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"to": sess.User.Email})
}

// runReports sends the reports that are due, checked every minute.
func (s *Server) runReports(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.sendDueReports(ctx, time.Now())
		}
	}
}

func (s *Server) sendDueReports(ctx context.Context, now time.Time) {
	list, err := s.PBX.Reports(ctx)
	if err != nil {
		return
	}
	local := now.In(s.loc(ctx))
	for _, rep := range list {
		if !rep.Due(local) {
			continue
		}
		// Mark first: a mail server that is down must not make the report go out every minute.
		s.PBX.ReportSent(ctx, rep.ID, now)
		out, err := s.buildReport(ctx, &rep, local)
		if err != nil {
			s.Log.Warn("report", "name", rep.Name, "err", err)
			continue
		}
		for _, to := range rep.Recipients {
			if err := s.Mail.SendHTML(ctx, "report", to, out.subject, out.text, out.html, out.csv); err != nil {
				s.Log.Warn("report mail", "name", rep.Name, "to", to, "err", err)
			}
		}
	}
}

// logSLAAlert remembers an alert for the reports.
func (s *Server) logSLAAlert(ctx context.Context, a slaAlert, at time.Time) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO sla_alerts(queue_id, kind, at) VALUES (?,?,?)`, a.QueueID, a.Kind, at.UTC().Format(time.RFC3339))
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sla_alerts WHERE at < ?`, at.AddDate(0, 0, -400).UTC().Format(time.RFC3339))
}

// ---- building a report ------------------------------------------------------

type reportOut struct {
	subject, text, html, filename string
	csv                           mail.Attachment
}

var reportWords = map[string]map[string]string{
	"en": {
		"calls": "Calls", "level": "Service level", "avgWait": "Avg. wait", "abandoned": "Hung up", "vs": "vs. %s",
		"queues": "Per queue", "queue": "Queue", "offered": "Offered", "answered": "Answered", "longest": "Longest wait", "callbacks": "Called back",
		"agents": "Per agent", "agent": "Agent", "avgTalk": "Avg. call", "hours": "Per hour", "hour": "Hour",
		"busiest": "Busiest hour: %s (%d calls).", "alerts": "%d SLA alerts in this period.", "noAlerts": "No SLA alerts in this period.",
		"footer": "Report \"%s\" · change it under Reception › Wallboard › Settings", "allQueues": "All queues",
		"daily": "Daily summary", "weekly": "Weekly summary", "monthly": "Monthly summary", "week": "week %d", "pts": "pts", "none": "No calls in this period.",
		"prevDay": "a week earlier", "prevWeek": "the week before", "prevMonth": "the month before", "csv": "The figures per day and per agent are attached as CSV.",
	},
	"nl": {
		"calls": "Gesprekken", "level": "Servicelevel", "avgWait": "Gem. wachttijd", "abandoned": "Opgehangen", "vs": "t.o.v. %s",
		"queues": "Per wachtrij", "queue": "Wachtrij", "offered": "Aangeboden", "answered": "Beantwoord", "longest": "Langste wacht", "callbacks": "Teruggebeld",
		"agents": "Per medewerker", "agent": "Medewerker", "avgTalk": "Gem. gesprek", "hours": "Per uur", "hour": "Uur",
		"busiest": "Drukste uur: %s (%d gesprekken).", "alerts": "%d SLA-meldingen in deze periode.", "noAlerts": "Geen SLA-meldingen in deze periode.",
		"footer": "Rapportage \"%s\" · aanpassen onder Receptie › Wallboard › Instellingen", "allQueues": "Alle wachtrijen",
		"daily": "Dagoverzicht", "weekly": "Weekoverzicht", "monthly": "Maandoverzicht", "week": "week %d", "pts": "punten", "none": "Geen gesprekken in deze periode.",
		"prevDay": "een week eerder", "prevWeek": "de week ervoor", "prevMonth": "de maand ervoor", "csv": "De cijfers per dag en per medewerker zitten als CSV in de bijlage.",
	},
}

var (
	monthsNL = []string{"januari", "februari", "maart", "april", "mei", "juni", "juli", "augustus", "september", "oktober", "november", "december"}
	daysNL   = []string{"zondag", "maandag", "dinsdag", "woensdag", "donderdag", "vrijdag", "zaterdag"}
)

func fmtDate(t time.Time, lang string, year bool) string {
	if lang == "nl" {
		s := fmt.Sprintf("%d %s", t.Day(), monthsNL[t.Month()-1])
		if year {
			s += fmt.Sprintf(" %d", t.Year())
		}
		return s
	}
	if year {
		return t.Format("2 January 2006")
	}
	return t.Format("2 January")
}

// reportPeriod describes the period of a report, for example "week 46 (9 to 15 November)".
func reportPeriod(rep *pbx.Report, from, to time.Time) string {
	lang := rep.Language
	switch rep.Period {
	case pbx.ReportWeekly:
		_, wk := from.ISOWeek()
		sep := " – "
		return fmt.Sprintf(reportWords[lang]["week"], wk) + " (" + fmtDate(from, lang, false) + sep + fmtDate(to.AddDate(0, 0, -1), lang, true) + ")"
	case pbx.ReportMonthly:
		if lang == "nl" {
			return fmt.Sprintf("%s %d", monthsNL[from.Month()-1], from.Year())
		}
		return from.Format("January 2006")
	}
	if lang == "nl" {
		return daysNL[from.Weekday()] + " " + fmtDate(from, lang, true)
	}
	return from.Format("Monday ") + fmtDate(from, lang, true)
}

func secs(n int) string { return fmt.Sprintf("%d:%02d", n/60, n%60) }

func pct(v float64) string {
	if v < 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", v)
}

func (s *Server) buildReport(ctx context.Context, rep *pbx.Report, now time.Time) (*reportOut, error) {
	loc := now.Location()
	from, to, pFrom, pTo := rep.Range(now)
	qs, err := s.PBX.Queues(ctx)
	if err != nil {
		return nil, err
	}
	target := map[int64]int{}
	names := map[int64]string{}
	for _, q := range qs {
		target[q.ID], names[q.ID] = q.Options.SLA.Target, q.Name
	}
	tf := func(id int64) int { return target[id] }
	keep := func(cs []qstats.Call) []qstats.Call {
		out := []qstats.Call{}
		for _, c := range cs {
			if len(rep.Queues) == 0 || slices.Contains(rep.Queues, c.QueueID) {
				out = append(out, c)
			}
		}
		return out
	}
	st := &qstats.Store{DB: s.DB}
	calls, err := st.Calls(ctx, from, to)
	if err != nil {
		return nil, err
	}
	prev, err := st.Calls(ctx, pFrom, pTo)
	if err != nil {
		return nil, err
	}
	calls, prev = keep(calls), keep(prev)
	cur, before := qstats.Total(calls, tf), qstats.Total(prev, tf)
	sum := qstats.Summarize(calls, tf, loc)

	alerts := 0
	{
		q := `SELECT queue_id FROM sla_alerts WHERE at >= ? AND at < ?`
		rows, err := s.DB.QueryContext(ctx, q, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
		if err == nil {
			for rows.Next() {
				var id int64
				if rows.Scan(&id) == nil && (len(rep.Queues) == 0 || slices.Contains(rep.Queues, id)) {
					alerts++
				}
			}
			rows.Close()
		}
	}

	wd := reportWords[rep.Language]
	period := reportPeriod(rep, from, to)
	title := wd[rep.Period]
	subject := fmt.Sprintf("%s · %s", rep.Name, period)
	queueLabel := wd["allQueues"]
	if len(rep.Queues) > 0 {
		ns := []string{}
		for _, id := range rep.Queues {
			if n := names[id]; n != "" {
				ns = append(ns, n)
			}
		}
		queueLabel = strings.Join(ns, ", ")
	}
	prevName := map[string]string{pbx.ReportDaily: wd["prevDay"], pbx.ReportWeekly: wd["prevWeek"], pbx.ReportMonthly: wd["prevMonth"]}[rep.Period]

	var h, t strings.Builder
	esc := html.EscapeString
	h.WriteString(`<!doctype html><html><body style="margin:0;padding:24px;background:#F4F7FB;font-family:Inter,Segoe UI,Arial,sans-serif;color:#0F172A">`)
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:720px;margin:0 auto;background:#FFFFFF;border:1px solid #E2E8F0;border-radius:14px"><tr><td style="padding:28px 32px">`)
	fmt.Fprintf(&h, `<div style="font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:#0891B2">Kiestoon · %s</div>`, esc(title))
	fmt.Fprintf(&h, `<h1 style="margin:6px 0 2px;font-size:24px">%s</h1><div style="color:#526072;font-size:14px">%s · %s</div>`, esc(rep.Name), esc(queueLabel), esc(period))
	fmt.Fprintf(&t, "%s\n%s · %s\n\n", rep.Name, queueLabel, period)

	// Headline figures with the change against the period before.
	delta := func(cur, prev float64, unit string, lowerBetter bool) (string, string) {
		if prev < 0 || cur < 0 || before.Offered == 0 {
			return "", "#64748B"
		}
		d := cur - prev
		if unit != "%calls" {
			d = math.Round(d)
		}
		good := (d >= 0) != lowerBetter
		col := "#059669"
		if !good {
			col = "#DC2626"
		}
		if d == 0 {
			col = "#64748B"
		}
		switch unit {
		case "%calls":
			if prev == 0 {
				return "", "#64748B"
			}
			if p := math.Round(d * 100 / prev); p != 0 {
				return fmt.Sprintf("%+.0f%%", p), "#526072"
			}
			return "±0%", "#526072"
		case "pts":
			if d == 0 {
				return "±0 " + wd["pts"], col
			}
			return fmt.Sprintf("%+.0f %s", d, wd["pts"]), col
		case "s":
			sign := "+"
			if d == 0 {
				sign = "±"
			} else if d < 0 {
				sign, d = "−", -d
			}
			return sign + secs(int(d)), col
		}
		return "", col
	}
	kpis := []struct {
		label, value string
		d, col       string
	}{}
	add := func(label, value string, cur, prev float64, unit string, lower bool) {
		d, col := delta(cur, prev, unit, lower)
		kpis = append(kpis, struct{ label, value, d, col string }{label, value, d, col})
	}
	add(wd["calls"], strconv.Itoa(cur.Offered), float64(cur.Offered), float64(before.Offered), "%calls", false)
	add(wd["level"], pct(cur.ServiceLevel), cur.ServiceLevel, before.ServiceLevel, "pts", false)
	add(wd["avgWait"], secs(cur.AvgWait), float64(cur.AvgWait), float64(before.AvgWait), "s", true)
	add(wd["abandoned"], pct(cur.AbandonedPct), cur.AbandonedPct, before.AbandonedPct, "pts", true)
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:22px 0 8px"><tr>`)
	for _, k := range kpis {
		fmt.Fprintf(&h, `<td style="padding:12px;border:1px solid #E2E8F0;border-radius:10px;width:25%%"><div style="font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:#64748B">%s</div><div style="font-size:26px;font-family:Consolas,monospace;margin-top:4px">%s</div>`, esc(k.label), esc(k.value))
		if k.d != "" && slices.Contains(rep.Sections, "trend") {
			fmt.Fprintf(&h, `<div style="font-size:12px;color:%s">%s</div>`, k.col, esc(fmt.Sprintf("%s "+wd["vs"], k.d, prevName)))
		}
		h.WriteString(`</td>`)
		fmt.Fprintf(&t, "%s: %s", k.label, k.value)
		if k.d != "" && slices.Contains(rep.Sections, "trend") {
			fmt.Fprintf(&t, " ("+wd["vs"]+": %s)", prevName, k.d)
		}
		t.WriteString("\n")
	}
	h.WriteString(`</tr></table>`)
	if cur.Offered == 0 {
		fmt.Fprintf(&h, `<p style="color:#526072">%s</p>`, esc(wd["none"]))
		t.WriteString("\n" + wd["none"] + "\n")
	}

	table := func(title string, head []string, rows [][]string) {
		fmt.Fprintf(&h, `<h2 style="font-size:16px;margin:26px 0 8px">%s</h2><table width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;font-size:13px">`, esc(title))
		h.WriteString(`<tr>`)
		for i, c := range head {
			align := "right"
			if i == 0 {
				align = "left"
			}
			fmt.Fprintf(&h, `<th style="text-align:%s;padding:6px 8px;border-bottom:1px solid #CBD5E1;font-size:11px;letter-spacing:.06em;text-transform:uppercase;color:#64748B">%s</th>`, align, esc(c))
		}
		h.WriteString(`</tr>`)
		fmt.Fprintf(&t, "\n%s\n%s\n", title, strings.Join(head, " | "))
		for _, r := range rows {
			h.WriteString(`<tr>`)
			for i, c := range r {
				align, font := "right", "font-family:Consolas,monospace;"
				if i == 0 {
					align, font = "left", ""
				}
				fmt.Fprintf(&h, `<td style="text-align:%s;padding:6px 8px;border-bottom:1px solid #EEF2F7;%s">%s</td>`, align, font, esc(c))
			}
			h.WriteString(`</tr>`)
			t.WriteString(strings.Join(r, " | ") + "\n")
		}
		h.WriteString(`</table>`)
	}

	ids := []int64{}
	for id := range sum.Queues {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return names[ids[i]] < names[ids[j]] })
	if slices.Contains(rep.Sections, "queues") && len(ids) > 0 {
		rows := [][]string{}
		for _, id := range ids {
			q := sum.Queues[id]
			n := names[id]
			if n == "" {
				n = fmt.Sprintf("#%d", id)
			}
			rows = append(rows, []string{n, strconv.Itoa(q.Offered), strconv.Itoa(q.Answered), pct(q.ServiceLevel), secs(q.AvgWait), secs(q.LongestWait), strconv.Itoa(q.Callbacks)})
		}
		table(wd["queues"], []string{wd["queue"], wd["offered"], wd["answered"], wd["level"], wd["avgWait"], wd["longest"], wd["callbacks"]}, rows)
	}
	agents := []*qstats.Agent{}
	for _, a := range sum.Agents {
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Calls > agents[j].Calls })
	if slices.Contains(rep.Sections, "agents") && len(agents) > 0 {
		rows := [][]string{}
		for _, a := range agents {
			n := a.Name
			if n == "" {
				n = a.Number
			}
			rows = append(rows, []string{n, strconv.Itoa(a.Calls), secs(a.AvgTalk)})
		}
		table(wd["agents"], []string{wd["agent"], wd["answered"], wd["avgTalk"]}, rows)
	}
	var hours [24]qstats.Hour
	for _, hs := range sum.Hours {
		for i := range hs {
			hours[i].Answered += hs[i].Answered
			hours[i].Abandoned += hs[i].Abandoned
		}
	}
	busy, busyN := -1, 0
	for i, hr := range hours {
		if n := hr.Answered + hr.Abandoned; n > busyN {
			busy, busyN = i, n
		}
	}
	if slices.Contains(rep.Sections, "hours") && busy >= 0 {
		rows := [][]string{}
		for i, hr := range hours {
			if hr.Answered+hr.Abandoned == 0 {
				continue
			}
			rows = append(rows, []string{fmt.Sprintf("%02d:00", i), strconv.Itoa(hr.Answered), strconv.Itoa(hr.Abandoned)})
		}
		table(wd["hours"], []string{wd["hour"], wd["answered"], wd["abandoned"]}, rows)
	}
	notes := []string{}
	if busy >= 0 {
		notes = append(notes, fmt.Sprintf(wd["busiest"], fmt.Sprintf("%02d:00–%02d:00", busy, busy+1), busyN))
	}
	if alerts > 0 {
		notes = append(notes, fmt.Sprintf(wd["alerts"], alerts))
	} else {
		notes = append(notes, wd["noAlerts"])
	}
	notes = append(notes, wd["csv"])
	fmt.Fprintf(&h, `<p style="margin:22px 0 0;font-size:13px;color:#334155">%s</p>`, esc(strings.Join(notes, " ")))
	t.WriteString("\n" + strings.Join(notes, " ") + "\n")
	footer := fmt.Sprintf(wd["footer"], rep.Name)
	fmt.Fprintf(&h, `<p style="margin:22px 0 0;padding-top:14px;border-top:1px solid #E2E8F0;font-size:12px;color:#64748B">%s · Powered by RHIS</p>`, esc(footer))
	h.WriteString(`</td></tr></table></body></html>`)
	t.WriteString("\n-- \n" + footer + "\n")

	// CSV: one row per day and queue, then one per agent.
	var cb bytes.Buffer
	cw := csv.NewWriter(&cb)
	_ = cw.Write([]string{"type", "date", "queue", "agent", "offered", "answered", "abandoned", "service_level", "avg_wait", "avg_talk", "longest_wait", "callbacks"})
	byDay := map[string][]qstats.Call{}
	days := []string{}
	for _, c := range calls {
		d := c.Joined.In(loc).Format("2006-01-02")
		if _, ok := byDay[d]; !ok {
			days = append(days, d)
		}
		byDay[d] = append(byDay[d], c)
	}
	sort.Strings(days)
	f := func(v float64) string {
		if v < 0 {
			return ""
		}
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	for _, d := range days {
		ds := qstats.Summarize(byDay[d], tf, loc)
		for _, id := range ids {
			q := ds.Queues[id]
			if q == nil {
				continue
			}
			_ = cw.Write([]string{"queue", d, names[id], "", strconv.Itoa(q.Offered), strconv.Itoa(q.Answered), strconv.Itoa(q.Abandoned - q.ShortAbandoned),
				f(q.ServiceLevel), strconv.Itoa(q.AvgWait), strconv.Itoa(q.AvgTalk), strconv.Itoa(q.LongestWait), strconv.Itoa(q.Callbacks)})
		}
	}
	for _, a := range agents {
		_ = cw.Write([]string{"agent", "", "", strings.TrimSpace(a.Number + " " + a.Name), "", strconv.Itoa(a.Calls), "", "", "", strconv.Itoa(a.AvgTalk), "", ""})
	}
	cw.Flush()
	name := fmt.Sprintf("kiestoon-%s-%s.csv", rep.Period, from.Format("2006-01-02"))
	return &reportOut{subject: subject, text: t.String(), html: h.String(), filename: name,
		csv: mail.Attachment{Name: name, ContentType: "text/csv", Data: cb.Bytes()}}, nil
}
