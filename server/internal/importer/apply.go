package importer

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

// Choice is what the administrator ticked, with the numbers they chose.
type Choice struct {
	Extensions     []string          `json:"extensions"`
	RingGroups     []string          `json:"ringGroups"`
	Queues         []string          `json:"queues"`
	Trunks         []string          `json:"trunks"`
	TimeConditions []string          `json:"timeConditions"`
	Numbers        []string          `json:"numbers"`
	Menus          []string          `json:"menus"`
	NumberOf       map[string]string `json:"numberOf"` // key -> internal number
	Prompts        map[string]int64  `json:"prompts"`  // audio file -> prompt id
}

// Created is one thing the import made or reused.
type Created struct {
	Kind   string `json:"kind"` // extension | ringgroup | queue | trunk | timecondition | route | ivr
	Key    string `json:"key"`
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Number string `json:"number,omitempty"`
	Reused bool   `json:"reused,omitempty"`
}

// Result is what the import did.
type Result struct {
	Created  []Created `json:"created"`
	Warnings []Note    `json:"warnings"`
}

// FieldError is a validation error of one planned item.
type FieldError struct {
	Key   string
	Field string
	Code  string
}

func (e *FieldError) Error() string { return e.Key + ": " + e.Code + " " + e.Field }

func wrap(key string, err error) error {
	var ve *pbx.ValidationError
	if errors.As(err, &ve) {
		return &FieldError{Key: key, Field: ve.Field, Code: ve.Code}
	}
	return err
}

type applier struct {
	ctx    context.Context
	r      *pbx.Repo
	np     pbx.NumberPlan
	b      *Bundle
	p      *Plan
	c      Choice
	res    *Result
	ext    map[string]pbx.Extension // user key -> extension
	groups map[string]int64
	queues map[string]int64
	menus  map[string]int64
	times  map[string]int64
	name   string
}

func (ap *applier) warn(code string, args ...string) {
	ap.res.Warnings = append(ap.res.Warnings, NewNote(code, args...))
}

func (ap *applier) number(key, planned string) string {
	if n, ok := ap.c.NumberOf[key]; ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	return planned
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

// dest turns a plan target key into a Kiestoon destination.
func (ap *applier) dest(key string) pbx.Destination {
	kind, ref, _ := strings.Cut(key, ":")
	switch kind {
	case "u":
		if e, ok := ap.ext[key]; ok {
			return pbx.Destination{Type: pbx.DestExtension, Target: e.Number}
		}
	case "v":
		if e, ok := ap.ext[keyUser(ref)]; ok {
			if e.Voicemail {
				return pbx.Destination{Type: pbx.DestVoicemail, Target: e.Number}
			}
			return pbx.Destination{Type: pbx.DestExtension, Target: e.Number}
		}
	case "g":
		if n, ok := ap.groups[key]; ok {
			return pbx.Destination{Type: pbx.DestRingGroup, Target: id(n)}
		}
	case "q":
		if n, ok := ap.queues[key]; ok {
			return pbx.Destination{Type: pbx.DestQueue, Target: id(n)}
		}
	case "m":
		if n, ok := ap.menus[key]; ok {
			return pbx.Destination{Type: pbx.DestIVR, Target: id(n)}
		}
	case "c":
		if n, ok := ap.times[key]; ok {
			return pbx.Destination{Type: pbx.DestTimeCondition, Target: id(n)}
		}
	case "x":
		if d := pbx.NormalizeDID(ref); d != "" {
			return pbx.Destination{Type: pbx.DestExternal, Target: d}
		}
	case "":
		return pbx.Destination{Type: pbx.DestHangup}
	}
	ap.warn("target_missing", "target", key)
	return pbx.Destination{Type: pbx.DestHangup}
}

// Apply creates what was chosen. It stops at the first item Kiestoon does not accept; what was
// created before stays (it can be removed in the normal pages, or kept: a second import reuses it).
func Apply(ctx context.Context, r *pbx.Repo, np pbx.NumberPlan, b *Bundle, p *Plan, c Choice) (*Result, error) {
	ap := &applier{ctx: ctx, r: r, np: np, b: b, p: p, c: c, res: &Result{Created: []Created{}, Warnings: []Note{}},
		ext: map[string]pbx.Extension{}, groups: map[string]int64{}, queues: map[string]int64{}, menus: map[string]int64{},
		times: map[string]int64{}, name: b.shortName()}
	// Everything is created first with "hang up" where it points elsewhere, so things can point at
	// each other in any direction; the second pass fills in the real destinations.
	for _, step := range []func() error{ap.extensions, ap.ringGroups, ap.queuesStep, ap.trunks, ap.timesStep, ap.menusStep,
		ap.linkGroups, ap.linkQueues, ap.linkTimes, ap.routes} {
		if err := step(); err != nil {
			return ap.res, err
		}
	}
	return ap.res, nil
}

var rePIN = regexp.MustCompile(`^[0-9]{4,10}$`)

func (ap *applier) extensions() error {
	for _, pe := range ap.p.Extensions {
		if pe.ExistingID != 0 {
			// Reused whether ticked or not, so the groups and menus can ring it.
			e, err := ap.r.Extension(ap.ctx, pe.ExistingID)
			if err == nil {
				ap.ext[pe.Key] = *e
				if slices.Contains(ap.c.Extensions, pe.Key) {
					ap.res.Created = append(ap.res.Created, Created{Kind: "extension", Key: pe.Key, ID: e.ID, Name: e.Name, Number: e.Number, Reused: true})
				}
			}
			continue
		}
		if !slices.Contains(ap.c.Extensions, pe.Key) {
			continue
		}
		u := ap.b.User(strings.TrimPrefix(pe.Key, "u:"))
		if u == nil {
			continue
		}
		e := pbx.Extension{Number: ap.number(pe.Key, pe.Number), Name: pe.Name, Voicemail: pe.Voicemail, Enabled: !u.Disabled,
			OutboundClass: "national", RecordMode: "off", Transport: "udp", MaxContacts: 3, RingSeconds: 25}
		if strings.Contains(u.Email, "@") && len(u.Email) <= 254 {
			e.Email = strings.TrimSpace(u.Email)
			e.VoicemailEmail = u.VoicemailEmail && e.Voicemail
		}
		if e.Voicemail && rePIN.MatchString(u.VoicemailPIN) {
			e.VoicemailPIN = u.VoicemailPIN
		}
		if d := pbx.NormalizeDID(u.OutboundCID); d != "" {
			e.OutboundCID = d
		}
		if pe.KeepPassword {
			e.SIPSecret = u.SIPPassword
		}
		err := ap.r.SaveExtension(ap.ctx, ap.np, &e)
		var ve *pbx.ValidationError
		if errors.As(err, &ve) && (ve.Field == "email" || ve.Field == "outboundCallerId" || ve.Field == "voicemailPin") {
			// Details that Kiestoon does not accept are left out rather than stopping the import.
			ap.warn("field_dropped", "name", e.Name, "field", ve.Field)
			switch ve.Field {
			case "email":
				e.Email, e.VoicemailEmail = "", false
			case "outboundCallerId":
				e.OutboundCID = ""
			default:
				e.VoicemailPIN = ""
			}
			err = ap.r.SaveExtension(ap.ctx, ap.np, &e)
		}
		if err != nil {
			return wrap(pe.Key, err)
		}
		ap.ext[pe.Key] = e
		ap.res.Created = append(ap.res.Created, Created{Kind: "extension", Key: pe.Key, ID: e.ID, Name: e.Name, Number: e.Number})
	}
	return nil
}

func (ap *applier) members(keys []string, owner string) []string {
	var out []string
	for _, k := range keys {
		if e, ok := ap.ext[k]; ok {
			out = append(out, e.Number)
		} else if strings.HasPrefix(k, "x:") {
			out = append(out, strings.TrimPrefix(k, "x:"))
		} else {
			ap.warn("member_missing", "member", k, "owner", owner)
		}
	}
	return out
}

func (ap *applier) ringGroups() error {
	for _, pg := range ap.p.RingGroups {
		if pg.ExistingID != 0 {
			ap.groups[pg.Key] = pg.ExistingID
			if slices.Contains(ap.c.RingGroups, pg.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "ringgroup", Key: pg.Key, ID: pg.ExistingID, Name: pg.Name, Number: pg.Number, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.RingGroups, pg.Key) {
			continue
		}
		g := pbx.RingGroup{Number: ap.number(pg.Key, pg.Number), Name: pg.Name, Strategy: pg.Strategy, Members: ap.members(pg.Members, pg.Key),
			Enabled: true, Failover: pbx.Destination{Type: pbx.DestHangup}, RingSeconds: 20}
		if src := ap.b.group(strings.TrimPrefix(pg.Key, "g:")); src != nil && src.RingSeconds > 0 {
			g.RingSeconds = min(max(src.RingSeconds, 5), 300)
		}
		if !slices.Contains([]string{"simultaneous", "sequential", "rotating", "paging"}, g.Strategy) {
			g.Strategy = "simultaneous"
		}
		if len(g.Members) == 0 {
			ap.warn("group_empty", "name", pg.Name)
			continue
		}
		if err := ap.r.SaveRingGroup(ap.ctx, ap.np, &g); err != nil {
			return wrap(pg.Key, err)
		}
		ap.groups[pg.Key] = g.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "ringgroup", Key: pg.Key, ID: g.ID, Name: g.Name, Number: g.Number})
	}
	return nil
}

// created reports whether the import made the thing (rather than reusing it).
func (ap *applier) created(key string) bool {
	return slices.ContainsFunc(ap.res.Created, func(c Created) bool { return c.Key == key && !c.Reused })
}

func (ap *applier) linkGroups() error {
	for _, pg := range ap.p.RingGroups {
		if pg.Failover == "" || !ap.created(pg.Key) {
			continue
		}
		g, err := ap.r.RingGroup(ap.ctx, ap.groups[pg.Key])
		if err != nil {
			continue
		}
		g.Failover = ap.dest(pg.Failover)
		if err := ap.r.SaveRingGroup(ap.ctx, ap.np, g); err != nil {
			return wrap(pg.Key, err)
		}
	}
	return nil
}

func (ap *applier) queuesStep() error {
	for _, pq := range ap.p.Queues {
		if pq.ExistingID != 0 {
			ap.queues[pq.Key] = pq.ExistingID
			if slices.Contains(ap.c.Queues, pq.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "queue", Key: pq.Key, ID: pq.ExistingID, Name: pq.Name, Number: pq.Number, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.Queues, pq.Key) {
			continue
		}
		src := ap.b.queue(strings.TrimPrefix(pq.Key, "q:"))
		q := pbx.Queue{Number: ap.number(pq.Key, pq.Number), Name: pq.Name, Strategy: pq.Strategy, Enabled: true, Members: []pbx.QueueMember{},
			MaxWait: min(max(pq.MaxWait, 0), 7200), Overflow: pbx.Destination{Type: pbx.DestHangup}, AgentTimeout: 15, Retry: 5, Music: "default"}
		if src != nil {
			if src.RingTime > 0 {
				q.AgentTimeout = min(max(src.RingTime, 5), 120)
			}
			q.Wrapup = min(max(src.WrapUp, 0), 600)
			q.MaxCallers = min(max(src.MaxSize, 0), 500)
			q.Enabled = !src.Disabled
			if src.Greeting != "" {
				q.GreetingPrompt = ap.c.Prompts[src.Greeting]
			}
		}
		for _, k := range pq.Agents {
			if e, ok := ap.ext[k]; ok {
				q.Members = append(q.Members, pbx.QueueMember{ExtensionID: e.ID})
			} else {
				ap.warn("member_missing", "member", k, "owner", pq.Key)
			}
		}
		if err := ap.r.SaveQueue(ap.ctx, ap.np, &q); err != nil {
			return wrap(pq.Key, err)
		}
		ap.queues[pq.Key] = q.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "queue", Key: pq.Key, ID: q.ID, Name: q.Name, Number: q.Number})
	}
	return nil
}

func (ap *applier) linkQueues() error {
	for _, pq := range ap.p.Queues {
		if pq.Overflow == "" || !ap.created(pq.Key) {
			continue
		}
		q, err := ap.r.Queue(ap.ctx, ap.queues[pq.Key])
		if err != nil {
			continue
		}
		q.Overflow = ap.dest(pq.Overflow)
		if err := ap.r.SaveQueue(ap.ctx, ap.np, q); err != nil {
			return wrap(pq.Key, err)
		}
	}
	return nil
}

func (ap *applier) trunks() error {
	for _, pt := range ap.p.Trunks {
		if !slices.Contains(ap.c.Trunks, pt.Key) {
			continue
		}
		if pt.ExistingID != 0 {
			ap.res.Created = append(ap.res.Created, Created{Kind: "trunk", Key: pt.Key, ID: pt.ExistingID, Name: pt.Name, Reused: true})
			continue
		}
		tt := ap.b.trunk(strings.TrimPrefix(pt.Key, "t:"))
		if tt == nil {
			continue
		}
		tr := pbx.Trunk{Name: pt.Name, Provider: pt.Provider, AuthMode: "register", Host: tt.Host, Port: tt.Port, Transport: tt.Transport,
			Username: tt.Username, AuthUsername: tt.AuthUsername, Secret: tt.Password, FromDomain: tt.FromDomain, FromUser: tt.FromUser,
			DIDSource: "to", Codecs: "alaw,ulaw,g722", Enabled: true, MatchIPs: strings.Join(tt.Networks, ","), Numbers: pt.Numbers}
		if d := pbx.NormalizeDID(tt.CallerID); d != "" {
			tr.CallerID = d
		}
		if !tt.Register {
			tr.AuthMode, tr.DIDSource = "ip", "ruri"
		}
		if tt.Codecs != "" {
			tr.Codecs = tt.Codecs
		}
		for _, preset := range pbx.TrunkPresets {
			if preset.ID == pt.Provider && preset.Host != "" {
				tr.Host, tr.Port, tr.Transport, tr.DIDSource, tr.Codecs = preset.Host, preset.Port, preset.Transport, preset.DIDSource, preset.Codecs
				if preset.MatchIPs != "" {
					tr.MatchIPs = preset.MatchIPs
				}
			}
		}
		if tr.Transport != "tcp" && tr.Transport != "tls" {
			tr.Transport = "udp"
		}
		err := ap.r.SaveTrunk(ap.ctx, &tr)
		var ve *pbx.ValidationError
		if errors.As(err, &ve) && ve.Field == "matchIps" {
			ap.warn("field_dropped", "name", tr.Name, "field", ve.Field)
			tr.MatchIPs = ""
			err = ap.r.SaveTrunk(ap.ctx, &tr)
		}
		if err != nil {
			return wrap(pt.Key, err)
		}
		ap.res.Created = append(ap.res.Created, Created{Kind: "trunk", Key: pt.Key, ID: tr.ID, Name: tr.Name})
	}
	return nil
}

func (ap *applier) timesStep() error {
	for _, pt := range ap.p.TimeConditions {
		if pt.ExistingID != 0 {
			ap.times[pt.Key] = pt.ExistingID
			if slices.Contains(ap.c.TimeConditions, pt.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "timecondition", Key: pt.Key, ID: pt.ExistingID, Name: pt.Name, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.TimeConditions, pt.Key) {
			continue
		}
		t := pbx.TimeCondition{Name: pt.Name, Schedule: pt.Schedule, Mode: "auto", Open: pbx.Destination{Type: pbx.DestHangup},
			Closed: pbx.Destination{Type: pbx.DestHangup}}
		err := ap.r.SaveTimeCondition(ap.ctx, &t)
		var ve *pbx.ValidationError
		if errors.As(err, &ve) && ve.Field == "schedule" {
			ap.warn("schedule_dropped", "name", t.Name)
			t.Schedule = nil
			err = ap.r.SaveTimeCondition(ap.ctx, &t)
		}
		if err != nil {
			return wrap(pt.Key, err)
		}
		ap.times[pt.Key] = t.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "timecondition", Key: pt.Key, ID: t.ID, Name: t.Name})
	}
	return nil
}

func (ap *applier) linkTimes() error {
	for _, pt := range ap.p.TimeConditions {
		if !ap.created(pt.Key) {
			continue
		}
		t, err := ap.r.TimeCondition(ap.ctx, ap.times[pt.Key])
		if err != nil {
			continue
		}
		t.Open, t.Closed = ap.dest(pt.Open), ap.dest(pt.Closed)
		if err := ap.r.SaveTimeCondition(ap.ctx, t); err != nil {
			return wrap(pt.Key, err)
		}
	}
	return nil
}

func (ap *applier) menusStep() error {
	// First every menu, empty, so menus can send calls to each other (and to themselves).
	for _, pm := range ap.p.Menus {
		if pm.ExistingID != 0 {
			ap.menus[pm.Key] = pm.ExistingID
			if slices.Contains(ap.c.Menus, pm.Key) {
				ap.res.Created = append(ap.res.Created, Created{Kind: "ivr", Key: pm.Key, ID: pm.ExistingID, Name: pm.Name, Reused: true})
			}
			continue
		}
		if !slices.Contains(ap.c.Menus, pm.Key) {
			continue
		}
		v := pbx.IVR{Name: pm.Name, Timeout: 5, Retries: 2, TimeoutDest: pbx.Destination{Type: pbx.DestHangup}}
		if err := ap.r.SaveIVR(ap.ctx, &v); err != nil {
			return wrap(pm.Key, err)
		}
		ap.menus[pm.Key] = v.ID
		ap.res.Created = append(ap.res.Created, Created{Kind: "ivr", Key: pm.Key, ID: v.ID, Name: v.Name})
	}
	return nil
}

// linkMenus draws the flow of every new menu, once everything it can point at exists.
func (ap *applier) linkMenus() error {
	for _, pm := range ap.p.Menus {
		if !ap.created(pm.Key) {
			continue
		}
		v, err := ap.r.IVR(ap.ctx, ap.menus[pm.Key])
		if err != nil {
			return err
		}
		v.Flow = ap.flow(pm)
		if err := ap.r.SaveIVR(ap.ctx, v); err != nil {
			return wrap(pm.Key, err)
		}
	}
	return nil
}

func (ap *applier) routes() error {
	if err := ap.linkMenus(); err != nil {
		return err
	}
	for _, pn := range ap.p.Numbers {
		if pn.Exists || !slices.Contains(ap.c.Numbers, pn.Key) {
			continue
		}
		d := ap.dest(pn.Target)
		x := pbx.InboundRoute{Name: PromptName(ap.name, pn.Number), DID: pn.Number, DestinationType: d.Type, Destination: d.Target, Enabled: true}
		if err := ap.r.SaveInboundRoute(ap.ctx, &x); err != nil {
			return wrap(pn.Key, err)
		}
		ap.res.Created = append(ap.res.Created, Created{Kind: "route", Key: pn.Key, ID: x.ID, Name: x.Name, Number: x.DID})
	}
	return nil
}
