package api

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/asterisk"
	"github.com/rhisijmuiden/kiestoon/server/internal/cdr"
	"github.com/rhisijmuiden/kiestoon/server/internal/journey"
	"github.com/rhisijmuiden/kiestoon/server/internal/media"
	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// journeyState records call journeys: the events Asterisk reports for each call.
type journeyState struct {
	store *journey.Store
	rec   *journey.Recorder
	off   atomic.Bool // journeys switched off (keep 0 days)
}

func (s *Server) startJourneys() {
	s.journeys.store = &journey.Store{DB: s.DB}
	s.journeys.rec = journey.NewRecorder(context.Background(), s.journeys.store, s.Log, func() bool { return !s.journeys.off.Load() })
	s.refreshJourneySetting(context.Background())
}

func (s *Server) refreshJourneySetting(ctx context.Context) {
	f, _ := pbx.LoadFeatureSettings(ctx, s.DB)
	s.journeys.off.Store(f.JourneyDays == 0)
}

// journeyEvent keeps the AMI events that make up a call journey.
func (s *Server) journeyEvent(m asterisk.Message) {
	if s.journeys.rec == nil {
		return
	}
	if ev, ok := journey.FromAMI(m.Get, time.Local); ok {
		s.journeys.rec.Add(ev)
	}
}

// purgeJourneys removes the events older than the retention (all of them when switched off).
func (s *Server) purgeJourneys(ctx context.Context) {
	if s.journeys.store == nil {
		return
	}
	f, _ := pbx.LoadFeatureSettings(ctx, s.DB)
	if n, err := s.journeys.store.Purge(ctx, f.JourneyDays, time.Now()); err != nil {
		s.Log.Warn("purge call journeys", "err", err)
	} else if n > 0 {
		s.Log.Info("old call journey events removed", "count", n)
	}
}

// journeyNames resolves the ids in a journey to the names in the phone system.
func (s *Server) journeyNames(ctx context.Context) journey.Names {
	snap, err := s.PBX.Snapshot(ctx)
	if err != nil {
		return nil
	}
	m := map[string]string{}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	for _, e := range snap.Extensions {
		m["ext:"+e.Number] = e.Name
	}
	for _, t := range snap.Trunks {
		m["trunk:"+id(t.ID)] = t.Name
	}
	for _, r := range snap.Inbound {
		m["route:"+id(r.ID)] = r.Name
	}
	for _, r := range snap.Outbound {
		m["outroute:"+id(r.ID)] = r.Name
	}
	for _, g := range snap.RingGroups {
		m["rg:"+id(g.ID)] = g.Name
	}
	for _, q := range snap.Queues {
		m["queue:"+id(q.ID)] = q.Name
	}
	for _, v := range snap.IVRs {
		m["ivr:"+id(v.ID)] = v.Name
	}
	for _, t := range snap.TimeConditions {
		m["tc:"+id(t.ID)] = t.Name
	}
	for _, c := range snap.Conferences {
		m["conf:"+id(c.ID)] = c.Name
	}
	for _, f := range snap.FaxBoxes {
		m["fax:"+id(f.ID)] = f.Name
	}
	return func(kind, key string) string { return m[kind+":"+key] }
}

// buildJourney puts together the journey of one call from its events, or from its call detail
// records when there are none. keep filters the recordings (the portal shows only the user's own).
func (s *Server) buildJourney(ctx context.Context, linkedID string, keep func(media.Recording) bool) (*journey.Journey, error) {
	recs, _, err := s.CDR.Search(ctx, cdr.Filter{LinkedID: linkedID, Limit: 500})
	if err != nil {
		return nil, err
	}
	evs, err := s.journeys.store.Events(ctx, linkedID)
	if err != nil {
		return nil, err
	}
	if len(evs) == 0 && len(recs) == 0 {
		return nil, nil
	}
	names := s.journeyNames(ctx)
	var j *journey.Journey
	if len(evs) > 0 {
		j = journey.Build(linkedID, evs, names)
	} else {
		j = journey.FromCDR(linkedID, recs, names)
	}
	ids := journey.UniqueIDs(evs)
	for _, r := range recs {
		ids[r.UniqueID] = true
	}
	for _, rec := range s.Spool.Recordings(s.loc(ctx), keep) {
		if rec.UniqueID != "" && ids[rec.UniqueID] {
			j.Recordings = append(j.Recordings, journey.Recording{Name: rec.Name, DurationMs: rec.Duration * 1000, From: rec.From, To: rec.To})
		}
	}
	return j, nil
}

func (s *Server) cdrJourney(w http.ResponseWriter, r *http.Request) {
	j, err := s.buildJourney(r.Context(), r.PathValue("id"), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if j == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// portalJourney shows a user the journey of a call they took part in.
func (s *Server) portalJourney(w http.ResponseWriter, r *http.Request) {
	e := s.myExtension(w, r)
	if e == nil {
		return
	}
	id := r.PathValue("id")
	_, n, err := s.CDR.Search(r.Context(), cdr.Filter{LinkedID: id, Ext: e.Number, Limit: 1})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal")
		return
	}
	if n == 0 {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	j, err := s.buildJourney(r.Context(), id, s.mine(e))
	if err != nil || j == nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, j)
}
