package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// onAMIEvent stores call detail records and handles the feature codes the dialplan reports.
func (s *Server) onAMIEvent(m asterisk.Message) {
	switch m.Get("Event") {
	case "CEL", "DialBegin", "DialEnd", "QueueCallerJoin", "AgentCalled", "AgentRingNoAnswer", "AgentConnect", "AgentComplete":
		s.journeyEvent(m)
	}
	switch m.Get("Event") {
	case "QueueCallerJoin", "AgentConnect", "AgentComplete", "QueueCallerAbandon", "QueueCallerLeave":
		s.queueStatEvent(m)
	}
	switch m.Get("Event") {
	case "Cdr":
		rec := cdr.FromEvent(m.Get, time.Local)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.CDR.Insert(ctx, rec); err != nil {
			s.Log.Warn("store cdr", "err", err)
		}
		if rec.Direction == "outbound" {
			go s.checkFraud(rec)
		}
		s.cdrEvent(rec)
	case "UserEvent":
		switch m.Get("UserEvent") {
		case "Kiestoon":
			go s.featureCode(m.Get("Action"), m.Get("Ext"), m.Get("Value"), m.Get("Queue"), m.Get("Id"))
		case "KiestoonFax":
			go s.onFax(m)
		case "KiestoonFaxOut":
			go s.onFaxOut(m)
		case "KiestoonCallback":
			go s.onCallbackEvent(m)
		case "KiestoonWake":
			go s.onMobileWake(m)
		case "KiestoonWakeEnd":
			go s.onMobileWakeEnd(m)
		}
	case "AgentConnect":
		go s.onCallbackConnect(m)
	case "OriginateResponse":
		go s.onOriginateResponse(m)
	case "DialBegin", "DialEnd", "MessageWaiting", "QueueCallerAbandon":
		if m.Get("Event") == "MessageWaiting" {
			go s.onMobileMWI(m)
		}
		if m.Get("Event") == "QueueCallerAbandon" {
			s.journeyEvent(m)
		}
		s.callEvent(m)
	case "ExtensionStatus":
		// BLF changes of local extensions go to linked Kiestoon systems.
		if m.Get("Context") == "kiestoon-extensions" {
			s.fedLocalState(m.Get("Exten"), asterisk.ExtStatus(m.Get("Status")))
		}
	}
}

// featureCode applies a star code dialled on a phone (*76, *72, *45, *28).
func (s *Server) featureCode(action, ext, value, queue, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e, err := s.PBX.ExtensionByNumber(ctx, ext)
	if err != nil && action != "tcmode" {
		return
	}
	switch action {
	case "dnd":
		e.Features.DND = value == "on"
		err = s.PBX.SaveFeatures(ctx, e.ID, e.Features)
	case "forward":
		e.Features.ForwardAlways = value
		err = s.PBX.SaveFeatures(ctx, e.ID, e.Features)
	case "queue":
		qid, _ := strconv.ParseInt(queue, 10, 64)
		_, err = s.PBX.SetQueueLogin(ctx, qid, e.ID, value == "in")
		s.States.Invalidate()
	case "tcmode":
		tid, _ := strconv.ParseInt(id, 10, 64)
		err = s.PBX.SetTimeConditionMode(ctx, tid, value)
	default:
		return
	}
	if err != nil {
		s.Log.Warn("feature code", "action", action, "ext", ext, "err", err)
		return
	}
	store.Audit(ctx, s.DB, 0, "featurecode."+action, strings.TrimSpace(ext+" "+queue+id+" "+value), "phone")
	s.Ast.Changed()
}

// checkFraud looks at the outbound calls of the last hour and alerts (and blocks) on unusual traffic.
func (s *Server) checkFraud(rec cdr.Record) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f, err := pbx.LoadFeatureSettings(ctx, s.DB)
	if err != nil {
		return
	}
	fr := f.Fraud
	if fr.AlertOutboundPerHour == 0 && fr.AlertForeignPerHour == 0 {
		return
	}
	st, _ := pbx.LoadSettings(ctx, s.DB)
	since := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	var total, foreign int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN (dst LIKE '00%' AND dst NOT LIKE ?) OR (dst LIKE '+%' AND dst NOT LIKE ?)
		THEN 1 ELSE 0 END), 0) FROM cdr WHERE direction = 'outbound' AND start_at >= ?`,
		"00"+st.CountryCode+"%", "+"+st.CountryCode+"%", since).Scan(&total, &foreign)
	over := (fr.AlertOutboundPerHour > 0 && total >= fr.AlertOutboundPerHour) || (fr.AlertForeignPerHour > 0 && foreign >= fr.AlertForeignPerHour)
	if !over {
		return
	}
	last, _ := store.GetSetting(ctx, s.DB, "fraud_alert_at", "")
	if t, err := time.Parse(time.RFC3339, last); err == nil && time.Since(t) < time.Hour {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_ = store.SetSetting(ctx, s.DB, "fraud_alert_at", now)
	blocked := ""
	if fr.BlockOnAlert {
		_ = store.SetSetting(ctx, s.DB, pbx.FraudLockKey, now)
		s.Ast.Changed()
		blocked = "\nInternational and premium-rate calls are now BLOCKED until an administrator unblocks them under Security > Fraud protection.\n"
	}
	store.Audit(ctx, s.DB, 0, "fraud.alert", fmt.Sprintf("%d outbound, %d international in the last hour", total, foreign), "")
	s.Log.Warn("fraud alert", "outbound", total, "international", foreign)
	org, _ := store.GetSetting(ctx, s.DB, "organization", "Kiestoon")
	subject := "Kiestoon fraud alert: unusual outbound calls (" + org + ")"
	body := fmt.Sprintf("Kiestoon noticed unusual outbound traffic in the last hour:\n\n  outbound calls: %d\n  international calls: %d\n\nLast call: %s to %s.\n%s\nCheck the call history in the portal.\n",
		total, foreign, rec.Src, rec.Dst, blocked)
	if fr.AlertEmail != "" {
		_ = s.Mail.SendText(ctx, "fraud", fr.AlertEmail, subject, body)
	} else {
		s.Mail.Notify(ctx, subject, body)
	}
}

// RunMaintenance regenerates the dialplan after midnight (holidays move every year), removes old
// recordings and keeps the day/night BLF lamps right.
func (s *Server) RunMaintenance(ctx context.Context) {
	go s.runCallbacks(ctx)
	go s.runAGI(ctx)
	go s.runSLA(ctx)
	go s.runReports(ctx)
	day := ""
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		loc := s.loc(ctx)
		today := time.Now().In(loc).Format("2006-01-02")
		if today != day {
			if day != "" {
				s.Ast.Changed()
				// The wallboard counts "today": start the queue statistics over at midnight.
				if s.AMI.Connected() {
					if _, err := s.AMI.Action(ctx, asterisk.Message{{Key: "Action", Value: "QueueReset"}}); err != nil {
						s.Log.Warn("queue reset", "err", err)
					}
				}
			}
			f, _ := pbx.LoadFeatureSettings(ctx, s.DB)
			if n := s.Spool.PurgeRecordings(f.RecordingDays, time.Now()); n > 0 {
				s.Log.Info("old recordings removed", "count", n)
			}
			s.purgeJourneys(ctx)
			s.purgeQueueStats(ctx)
			day = today
		}
		s.syncDeviceStates(ctx)
		s.refreshJourneySetting(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// syncDeviceStates lights the BLF of *28<id> while a time condition is switched by hand.
func (s *Server) syncDeviceStates(ctx context.Context) {
	if !s.AMI.Connected() {
		return
	}
	tcs, err := s.PBX.TimeConditions(ctx)
	if err != nil {
		return
	}
	for _, t := range tcs {
		state := "NOT_INUSE"
		if t.Mode != "auto" {
			state = "INUSE"
		}
		s.AMI.SetDeviceState(ctx, fmt.Sprintf("Custom:kt-tc-%d", t.ID), state)
	}
}
