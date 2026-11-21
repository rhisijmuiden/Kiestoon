package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// ivrLiveDest is what the IVR canvas shows live on a destination block.
type ivrLiveDest struct {
	State   string `json:"state,omitempty"`   // extension: idle, inuse, ringing, unavailable, ...
	Members int    `json:"members,omitempty"` // ring group or queue: configured members
	Online  *int   `json:"online,omitempty"`  // queue: agents signed in and reachable (only when the PBX is connected)
	Waiting int    `json:"waiting,omitempty"` // queue: callers waiting now
}

// ivrLive answers GET /api/pbx/ivrs/live: whether each time condition is open now and the live state
// of extensions, ring groups and queues, keyed "type:target" like an IVR destination.
func (s *Server) ivrLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := func(n int64) string { return strconv.FormatInt(n, 10) }

	open := map[string]bool{}
	if tcs, err := s.PBX.TimeConditions(ctx); err == nil {
		now := time.Now().In(s.loc(ctx))
		h, _ := pbx.LoadHolidays(ctx, s.DB)
		for _, tc := range tcs {
			open[id(tc.ID)] = tc.OpenAt(now, h.ForYear(now.Year()))
		}
	}

	dests := map[string]ivrLiveDest{}
	for _, e := range s.blfList(ctx) {
		dests["extension:"+e.Number] = ivrLiveDest{State: e.State}
	}
	if rgs, err := s.PBX.RingGroups(ctx); err == nil {
		for _, g := range rgs {
			dests["ringgroup:"+id(g.ID)] = ivrLiveDest{Members: len(g.Members)}
		}
	}
	live := map[int64]ivrLiveDest{}
	if s.AMI.Connected() {
		for _, st := range s.States.Queues(ctx) {
			n := 0
			for _, m := range st.Members {
				if m.Status != "unavailable" && m.Status != "invalid" && !m.Paused {
					n++
				}
			}
			d := ivrLiveDest{Waiting: st.Calls, Online: &n}
			live[st.ID] = d
		}
	}
	if qs, err := s.PBX.Queues(ctx); err == nil {
		for _, q := range qs {
			d := live[q.ID]
			d.Members = len(q.Members)
			dests["queue:"+id(q.ID)] = d
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"open": open, "dests": dests})
}
