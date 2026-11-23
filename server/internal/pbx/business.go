package pbx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// ---- destinations -----------------------------------------------------------

// Destination types: where a call can be sent by an inbound route, IVR option, time condition,
// queue overflow or ring group failover.
const (
	DestExtension     = "extension"
	DestVoicemail     = "voicemail"
	DestRingGroup     = "ringgroup"
	DestQueue         = "queue"
	DestIVR           = "ivr"
	DestTimeCondition = "timecondition"
	DestConference    = "conference"
	DestExternal      = "external"
	DestFax           = "fax"
	DestHangup        = "hangup"
)

// DestinationTypes lists every destination type, in the order the UI offers them.
var DestinationTypes = []string{DestExtension, DestVoicemail, DestRingGroup, DestQueue, DestIVR, DestTimeCondition,
	DestConference, DestOnCall, DestFax, DestFollowSun, DestRemote, DestExternal, DestHangup}

// Destination is a place a call goes to. Target is an extension number for extension and voicemail,
// a phone number for external, and a row ID for the other types.
type Destination struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

var reExternal = regexp.MustCompile(`^\+?[0-9]{3,20}$`)

// ValidateDestination checks that the destination exists.
func (r *Repo) ValidateDestination(ctx context.Context, d *Destination, field string) error {
	d.Target = strings.TrimSpace(d.Target)
	table := ""
	switch d.Type {
	case DestExtension, DestVoicemail:
		col := "1"
		if d.Type == DestVoicemail {
			col = "voicemail"
		}
		var n int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM extensions WHERE number = ? AND `+col+` = 1`, d.Target).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return invalid("unknown_destination", field)
		}
		return nil
	case DestExternal:
		d.Target = NormalizeDID(d.Target)
		if !reExternal.MatchString(d.Target) {
			return invalid("invalid", field)
		}
		return nil
	case DestHangup, "":
		d.Type, d.Target = DestHangup, ""
		return nil
	case DestRingGroup:
		table = "ring_groups"
	case DestQueue:
		table = "queues"
	case DestIVR:
		table = "ivrs"
	case DestTimeCondition:
		table = "time_conditions"
	case DestConference:
		table = "conferences"
	case DestFax:
		table = "fax_boxes"
	case DestOnCall:
		table = "oncall_teams"
	case DestFollowSun:
		table = "sun_routes"
	case DestRemote:
		return r.validateRemote(ctx, d, field)
	default:
		return invalid("invalid", field)
	}
	id, err := strconv.ParseInt(d.Target, 10, 64)
	if err != nil {
		return invalid("unknown_destination", field)
	}
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("unknown_destination", field)
	}
	return nil
}

type execQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// destColumns are the (type, target) column pairs that hold a destination.
var destColumns = []struct{ table, typ, target string }{
	{"inbound_routes", "destination_type", "destination"},
	{"ring_groups", "failover_type", "failover"},
	{"queues", "overflow_type", "overflow"},
	{"ivrs", "timeout_type", "timeout_target"},
	{"time_conditions", "open_type", "open_target"},
	{"time_conditions", "closed_type", "closed_target"},
	{"time_conditions", "holiday_type", "holiday_target"},
	{"sun_routes", "fallback_type", "fallback"},
}

// replaceDestinations calls fn for every stored destination and saves the ones fn changed. It is
// used when an extension, group or menu is deleted or renumbered.
func replaceDestinations(ctx context.Context, db execQuerier, fn func(*Destination) bool) error {
	for _, c := range destColumns {
		rows, err := db.QueryContext(ctx, `SELECT id, `+c.typ+`, `+c.target+` FROM `+c.table)
		if err != nil {
			return err
		}
		type upd struct {
			id int64
			d  Destination
		}
		var ups []upd
		for rows.Next() {
			var id int64
			var d Destination
			if err := rows.Scan(&id, &d.Type, &d.Target); err != nil {
				rows.Close()
				return err
			}
			if fn(&d) {
				ups = append(ups, upd{id, d})
			}
		}
		rows.Close()
		for _, u := range ups {
			if _, err := db.ExecContext(ctx, `UPDATE `+c.table+` SET `+c.typ+` = ?, `+c.target+` = ? WHERE id = ?`, u.d.Type, u.d.Target, u.id); err != nil {
				return err
			}
		}
	}
	// IVR options are a JSON list.
	// The flow drawn on the canvas sits in the layout column.
	rows, err := db.QueryContext(ctx, `SELECT id, options, layout FROM ivrs`)
	if err != nil {
		return err
	}
	type ivrUpd struct {
		id           int64
		opts, layout string
	}
	var ivrUps []ivrUpd
	for rows.Next() {
		var id int64
		var raw, layout string
		if err := rows.Scan(&id, &raw, &layout); err != nil {
			rows.Close()
			return err
		}
		var opts []IVROption
		_ = json.Unmarshal([]byte(raw), &opts)
		changed := false
		for i := range opts {
			if fn(&opts[i].Destination) {
				changed = true
			}
		}
		newLayout, flowChanged := replaceLayoutDestinations(layout, fn)
		if changed || flowChanged {
			b, _ := json.Marshal(opts)
			ivrUps = append(ivrUps, ivrUpd{id, string(b), newLayout})
		}
	}
	rows.Close()
	for _, u := range ivrUps {
		if _, err := db.ExecContext(ctx, `UPDATE ivrs SET options = ?, layout = ? WHERE id = ?`, u.opts, u.layout, u.id); err != nil {
			return err
		}
	}
	type upd struct {
		id  int64
		raw string
	}
	var ups []upd
	// Follow-the-sun rules are a JSON list too.
	rows, err = db.QueryContext(ctx, `SELECT id, rules FROM sun_routes`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var rules []SunRule
		_ = json.Unmarshal([]byte(raw), &rules)
		changed := false
		for i := range rules {
			if fn(&rules[i].Destination) {
				changed = true
			}
		}
		if changed {
			b, _ := json.Marshal(rules)
			ups = append(ups, upd{id, string(b)})
		}
	}
	rows.Close()
	for _, u := range ups {
		if _, err := db.ExecContext(ctx, `UPDATE sun_routes SET rules = ? WHERE id = ?`, u.raw, u.id); err != nil {
			return err
		}
	}
	return nil
}

// clearReferences points every destination that refers to (typ, id) at "hang up".
func clearReferences(ctx context.Context, db execQuerier, typ string, id int64) error {
	target := strconv.FormatInt(id, 10)
	return replaceDestinations(ctx, db, func(d *Destination) bool {
		if d.Type == typ && d.Target == target {
			*d = Destination{Type: DestHangup}
			return true
		}
		return false
	})
}

// ---- user features ----------------------------------------------------------

// FollowMe rings other numbers (usually a mobile) when the desk phone is not answered.
type FollowMe struct {
	Enabled bool     `json:"enabled"`
	Numbers []string `json:"numbers"`
	// Delay: seconds the desk phone rings alone before the follow-me numbers join. 0: all at once.
	Delay int `json:"delay"`
	// Confirm asks the person on the mobile to press 1, so a mobile voicemail does not take the call.
	Confirm bool `json:"confirm"`
}

// Features are the call settings a user sets in the user portal (and with feature codes).
type Features struct {
	DND             bool     `json:"dnd"`
	ForwardAlways   string   `json:"forwardAlways"`
	ForwardBusy     string   `json:"forwardBusy"`
	ForwardNoAnswer string   `json:"forwardNoAnswer"`
	FollowMe        FollowMe `json:"followMe"`
	// ScheduleID is a time condition: while it is closed, calls go to ClosedForward
	// (for example "outside office hours to my mobile").
	ScheduleID    int64  `json:"scheduleId"`
	ClosedForward string `json:"closedForward"`
}

func (f *Features) normalize() {
	if f.FollowMe.Numbers == nil {
		f.FollowMe.Numbers = []string{}
	}
}

var reForward = regexp.MustCompile(`^\+?[0-9*#]{2,20}$`)

// Validate checks the user features.
func (f *Features) Validate() error {
	f.normalize()
	for field, v := range map[string]*string{"forwardAlways": &f.ForwardAlways, "forwardBusy": &f.ForwardBusy,
		"forwardNoAnswer": &f.ForwardNoAnswer, "closedForward": &f.ClosedForward} {
		*v = NormalizeDID(*v)
		if *v != "" && !reForward.MatchString(*v) {
			return invalid("invalid", field)
		}
	}
	if len(f.FollowMe.Numbers) > 5 {
		return invalid("invalid", "followMe")
	}
	nums := []string{}
	for _, n := range f.FollowMe.Numbers {
		n = NormalizeDID(n)
		if n == "" {
			continue
		}
		if !reForward.MatchString(n) {
			return invalid("invalid", "followMe")
		}
		nums = append(nums, n)
	}
	f.FollowMe.Numbers = nums
	if f.FollowMe.Enabled && len(nums) == 0 {
		f.FollowMe.Enabled = false
	}
	if f.FollowMe.Delay < 0 || f.FollowMe.Delay > 120 {
		return invalid("invalid", "followMe")
	}
	if f.ScheduleID < 0 {
		return invalid("invalid", "scheduleId")
	}
	return nil
}

// SaveFeatures stores only the user features of an extension (used by the user portal and feature codes).
func (r *Repo) SaveFeatures(ctx context.Context, extID int64, f Features) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if f.ScheduleID != 0 {
		if err := r.ValidateDestination(ctx, &Destination{Type: DestTimeCondition, Target: strconv.FormatInt(f.ScheduleID, 10)}, "scheduleId"); err != nil {
			return err
		}
	}
	b, _ := json.Marshal(f)
	res, err := r.DB.ExecContext(ctx, `UPDATE extensions SET features = ? WHERE id = ?`, string(b), extID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ExtensionByNumber returns the extension with this number.
func (r *Repo) ExtensionByNumber(ctx context.Context, number string) (*Extension, error) {
	return scanExt(r.DB.QueryRowContext(ctx, `SELECT `+extCols+` FROM extensions WHERE number = ?`, number))
}

// ---- ring groups ------------------------------------------------------------

// RingGroup rings several phones for one number.
type RingGroup struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
	Name   string `json:"name"`
	// Strategy: simultaneous (all at once), sequential (in order), rotating (start after the last
	// one that rang) or paging (one-way announcement through the speakers).
	Strategy    string `json:"strategy"`
	RingSeconds int    `json:"ringSeconds"`
	// Members are extension numbers or external numbers (for example a mobile).
	Members   []string    `json:"members"`
	CIDPrefix string      `json:"cidPrefix"`
	SkipBusy  bool        `json:"skipBusy"`
	Failover  Destination `json:"failover"`
	Enabled   bool        `json:"enabled"`
}

const rgCols = `id, number, name, strategy, ring_seconds, members, cid_prefix, skip_busy, failover_type, failover, enabled`

func scanRG(row interface{ Scan(...any) error }) (*RingGroup, error) {
	var g RingGroup
	var mem string
	var skip, en int
	err := row.Scan(&g.ID, &g.Number, &g.Name, &g.Strategy, &g.RingSeconds, &mem, &g.CIDPrefix, &skip, &g.Failover.Type, &g.Failover.Target, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	_ = json.Unmarshal([]byte(mem), &g.Members)
	if g.Members == nil {
		g.Members = []string{}
	}
	g.SkipBusy, g.Enabled = skip == 1, en == 1
	return &g, err
}

// RingGroups returns all ring groups.
func (r *Repo) RingGroups(ctx context.Context) ([]RingGroup, error) {
	return queryAll(ctx, r.DB, `SELECT `+rgCols+` FROM ring_groups ORDER BY number`, scanRG)
}

// RingGroup returns one ring group.
func (r *Repo) RingGroup(ctx context.Context, id int64) (*RingGroup, error) {
	return scanRG(r.DB.QueryRowContext(ctx, `SELECT `+rgCols+` FROM ring_groups WHERE id = ?`, id))
}

// SaveRingGroup validates and stores a ring group.
func (r *Repo) SaveRingGroup(ctx context.Context, plan NumberPlan, g *RingGroup) error {
	g.Name = cleanText(g.Name, 60)
	g.CIDPrefix = cleanText(g.CIDPrefix, 20)
	if g.Name == "" {
		return invalid("required", "name")
	}
	if !plan.InRange("ringGroups", g.Number) {
		return invalid("number_out_of_range", "number")
	}
	switch g.Strategy {
	case "":
		g.Strategy = "simultaneous"
	case "simultaneous", "sequential", "rotating", "paging":
	default:
		return invalid("invalid", "strategy")
	}
	if g.RingSeconds == 0 {
		g.RingSeconds = 20
	}
	if g.RingSeconds < 5 || g.RingSeconds > 300 {
		return invalid("invalid", "ringSeconds")
	}
	if len(g.Members) == 0 || len(g.Members) > 50 {
		return invalid("required", "members")
	}
	seen := map[string]bool{}
	mem := []string{}
	for _, m := range g.Members {
		m = NormalizeDID(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		if len(m) == plan.Digits && reDigits.MatchString(m) {
			if _, err := r.ExtensionByNumber(ctx, m); err != nil {
				return invalid("unknown_member", "members")
			}
		} else if !reExternal.MatchString(m) || g.Strategy == "paging" {
			return invalid("invalid", "members")
		}
		mem = append(mem, m)
	}
	g.Members = mem
	if err := r.ValidateDestination(ctx, &g.Failover, "failover"); err != nil {
		return err
	}
	if g.Failover.Type == DestRingGroup && g.Failover.Target == strconv.FormatInt(g.ID, 10) {
		return invalid("loop", "failover")
	}
	b, _ := json.Marshal(g.Members)
	args := []any{g.Number, g.Name, g.Strategy, g.RingSeconds, string(b), g.CIDPrefix, b2i(g.SkipBusy), g.Failover.Type, g.Failover.Target, b2i(g.Enabled)}
	if g.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO ring_groups(number, name, strategy, ring_seconds, members, cid_prefix, skip_busy,
			failover_type, failover, enabled) VALUES (?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "number")
		}
		g.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE ring_groups SET number=?, name=?, strategy=?, ring_seconds=?, members=?, cid_prefix=?,
		skip_busy=?, failover_type=?, failover=?, enabled=? WHERE id=?`, append(args, g.ID), "number")
}

// DeleteRingGroup removes a ring group and points everything that used it at "hang up".
func (r *Repo) DeleteRingGroup(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "ring_groups", DestRingGroup, id)
}

func (r *Repo) deleteWithRefs(ctx context.Context, table, typ string, id int64) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := clearReferences(ctx, tx, typ, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func updateRow(ctx context.Context, db *sql.DB, q string, args []any, uniqueField string) error {
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return uniqueErr(err, uniqueField)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func queryAll[T any](ctx context.Context, db *sql.DB, q string, scan func(interface{ Scan(...any) error }) (*T, error), args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// ---- queues -----------------------------------------------------------------

// QueueMember is an agent of a queue.
type QueueMember struct {
	ExtensionID int64  `json:"extensionId"`
	Number      string `json:"number"`
	Name        string `json:"name"`
	Penalty     int    `json:"penalty"`
	// Dynamic members sign in and out themselves; static members are always signed in.
	Dynamic  bool `json:"dynamic"`
	LoggedIn bool `json:"loggedIn"`
	Paused   bool `json:"paused"`
}

// Queue is a call queue: callers wait with music until an agent is free.
type Queue struct {
	ID                int64       `json:"id"`
	Number            string      `json:"number"`
	Name              string      `json:"name"`
	Strategy          string      `json:"strategy"`
	Music             string      `json:"music"`
	AnnouncePosition  bool        `json:"announcePosition"`
	AnnounceFrequency int         `json:"announceFrequency"`
	AgentTimeout      int         `json:"agentTimeout"`
	Retry             int         `json:"retry"`
	Wrapup            int         `json:"wrapup"`
	MaxWait           int         `json:"maxWait"`
	MaxCallers        int         `json:"maxCallers"`
	JoinEmpty         bool        `json:"joinEmpty"`
	Record            bool        `json:"record"`
	CIDPrefix         string      `json:"cidPrefix"`
	GreetingPrompt    int64       `json:"greetingPrompt"`
	Overflow          Destination `json:"overflow"`
	// RemoteMembers are agents on linked Kiestoon systems.
	RemoteMembers []RemoteMember `json:"remoteMembers"`
	Enabled       bool           `json:"enabled"`
	Members       []QueueMember  `json:"members"`
	// Options holds the phase 6 settings: supervision, callback and service levels.
	Options QueueOptions `json:"options"`
}

// QueueOptions are the queue settings stored as JSON.
type QueueOptions struct {
	// Supervision lets supervisors (reception, admin) listen, whisper and barge on calls of this queue.
	Supervision bool            `json:"supervision"`
	Callback    CallbackOptions `json:"callback"`
	// SLA are the service level thresholds for the wallboard and the SLA alerts.
	SLA SLAOptions `json:"sla"`
	// External are contacts outside the PBX (a mobile, another office) that take calls of this
	// queue like an agent; the queue rings them over the trunk.
	External []ExternalMember `json:"external"`
}

// ExternalMember is an external number that is a member of a queue.
type ExternalMember struct {
	Name    string `json:"name"`
	Number  string `json:"number"`
	Penalty int    `json:"penalty"`
	// Active members get calls; an inactive one stays in the list but is not rung.
	Active bool `json:"active"`
}

// MaxExternalMembers limits the external numbers of one queue.
const MaxExternalMembers = 20

var reExternalNumber = regexp.MustCompile(`^\+?[0-9]{3,20}$`)

// CallbackOptions configure the queue callback: a waiting caller presses 1, hangs up and keeps their
// place; when an agent is free the agent is connected first and then the caller is called back.
type CallbackOptions struct {
	Enabled bool `json:"enabled"`
	// OfferEvery is how often (seconds) the offer is played to waiting callers.
	OfferEvery int `json:"offerEvery"`
	// Retries is how many more times an unanswered callback is tried.
	Retries int `json:"retries"`
	// RetryDelay is the time (seconds) between attempts.
	RetryDelay int `json:"retryDelay"`
	// Expire (minutes) gives up on a callback that has not reached an agent by then.
	Expire int `json:"expire"`
	// CallerID is the number shown to the caller when called back ("" = the route default).
	CallerID string `json:"callerId"`
}

func (o *QueueOptions) normalize() error {
	if err := o.SLA.normalize(); err != nil {
		return err
	}
	if o.External == nil {
		o.External = []ExternalMember{}
	}
	if len(o.External) > MaxExternalMembers {
		return invalid("invalid", "options.external")
	}
	seenNum := map[string]bool{}
	for i := range o.External {
		m := &o.External[i]
		m.Name = cleanText(m.Name, 40)
		m.Number = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "").Replace(strings.TrimSpace(m.Number))
		if !reExternalNumber.MatchString(m.Number) {
			return invalid("invalid", "options.external")
		}
		if seenNum[m.Number] {
			return invalid("duplicate", "options.external")
		}
		seenNum[m.Number] = true
		if m.Penalty < 0 || m.Penalty > 10 {
			return invalid("invalid", "options.external")
		}
	}
	c := &o.Callback
	def := func(v *int, d, min, max int, field string) error {
		if *v == 0 {
			*v = d
		}
		if *v < min || *v > max {
			return invalid("invalid", "options.callback."+field)
		}
		return nil
	}
	if err := def(&c.OfferEvery, 60, 15, 600, "offerEvery"); err != nil {
		return err
	}
	if c.Retries < 0 || c.Retries > 5 {
		return invalid("invalid", "options.callback.retries")
	}
	if err := def(&c.RetryDelay, 120, 30, 3600, "retryDelay"); err != nil {
		return err
	}
	if err := def(&c.Expire, 240, 10, 1440, "expire"); err != nil {
		return err
	}
	c.CallerID = strings.TrimSpace(c.CallerID)
	if c.CallerID != "" && !regexp.MustCompile(`^\+?[0-9]{3,20}$`).MatchString(c.CallerID) {
		return invalid("invalid", "options.callback.callerId")
	}
	return nil
}

// QueueStrategies are the ring strategies Asterisk supports.
var QueueStrategies = []string{"ringall", "rrmemory", "leastrecent", "fewestcalls", "random", "linear", "wrandom"}

const queueCols = `id, number, name, strategy, music, announce_position, announce_frequency, agent_timeout, retry, wrapup,
	max_wait, max_callers, join_empty, record, cid_prefix, greeting_prompt, overflow_type, overflow, enabled, options`

func scanQueue(row interface{ Scan(...any) error }) (*Queue, error) {
	var q Queue
	var ap, je, rec, en int
	var opts string
	err := row.Scan(&q.ID, &q.Number, &q.Name, &q.Strategy, &q.Music, &ap, &q.AnnounceFrequency, &q.AgentTimeout, &q.Retry,
		&q.Wrapup, &q.MaxWait, &q.MaxCallers, &je, &rec, &q.CIDPrefix, &q.GreetingPrompt, &q.Overflow.Type, &q.Overflow.Target, &en, &opts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	_ = json.Unmarshal([]byte(opts), &q.Options)
	if q.Options.External == nil {
		q.Options.External = []ExternalMember{}
	}
	_ = q.Options.normalize()
	q.AnnouncePosition, q.JoinEmpty, q.Record, q.Enabled = ap == 1, je == 1, rec == 1, en == 1
	q.Members = []QueueMember{}
	return &q, err
}

// Queues returns all queues with their members.
func (r *Repo) Queues(ctx context.Context) ([]Queue, error) {
	qs, err := queryAll(ctx, r.DB, `SELECT `+queueCols+` FROM queues ORDER BY number`, scanQueue)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT m.queue_id, m.extension_id, e.number, e.name, m.penalty, m.dynamic, m.logged_in, m.paused
		FROM queue_members m JOIN extensions e ON e.id = m.extension_id ORDER BY m.penalty, CAST(e.number AS INTEGER)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]*Queue{}
	for i := range qs {
		byID[qs[i].ID] = &qs[i]
	}
	for rows.Next() {
		var qid int64
		var m QueueMember
		var dyn, li, pa int
		if err := rows.Scan(&qid, &m.ExtensionID, &m.Number, &m.Name, &m.Penalty, &dyn, &li, &pa); err != nil {
			return nil, err
		}
		m.Dynamic, m.LoggedIn, m.Paused = dyn == 1, li == 1 || dyn == 0, pa == 1
		if q := byID[qid]; q != nil {
			q.Members = append(q.Members, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	remote, err := r.remoteMembers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range qs {
		qs[i].RemoteMembers = remote[qs[i].ID]
		if qs[i].RemoteMembers == nil {
			qs[i].RemoteMembers = []RemoteMember{}
		}
	}
	return qs, nil
}

// Queue returns one queue with its members.
func (r *Repo) Queue(ctx context.Context, id int64) (*Queue, error) {
	qs, err := r.Queues(ctx)
	if err != nil {
		return nil, err
	}
	for _, q := range qs {
		if q.ID == id {
			return &q, nil
		}
	}
	return nil, ErrNotFound
}

// SaveQueue validates and stores a queue and its members.
func (r *Repo) SaveQueue(ctx context.Context, plan NumberPlan, q *Queue) error {
	q.Name = cleanText(q.Name, 60)
	q.CIDPrefix = cleanText(q.CIDPrefix, 20)
	if q.Name == "" {
		return invalid("required", "name")
	}
	if !plan.InRange("queues", q.Number) {
		return invalid("number_out_of_range", "number")
	}
	if q.Strategy == "" {
		q.Strategy = "rrmemory"
	}
	if !slices.Contains(QueueStrategies, q.Strategy) {
		return invalid("invalid", "strategy")
	}
	if q.Music == "" {
		q.Music = "default"
	}
	if !regexp.MustCompile(`^[a-z0-9_-]{1,30}$`).MatchString(q.Music) {
		return invalid("invalid", "music")
	}
	checks := []struct {
		v        *int
		min, max int
		def      int
		field    string
	}{
		{&q.AnnounceFrequency, 0, 600, -1, "announceFrequency"},
		{&q.AgentTimeout, 5, 120, 20, "agentTimeout"},
		{&q.Retry, 1, 60, 5, "retry"},
		{&q.Wrapup, 0, 600, -1, "wrapup"},
		{&q.MaxWait, 0, 7200, -1, "maxWait"},
		{&q.MaxCallers, 0, 500, -1, "maxCallers"},
	}
	for _, c := range checks {
		if *c.v == 0 && c.def > 0 {
			*c.v = c.def
		}
		if *c.v < c.min || *c.v > c.max {
			return invalid("invalid", c.field)
		}
	}
	if q.GreetingPrompt != 0 {
		if _, err := r.Prompt(ctx, q.GreetingPrompt); err != nil {
			return invalid("unknown_prompt", "greetingPrompt")
		}
	}
	if err := r.ValidateDestination(ctx, &q.Overflow, "overflow"); err != nil {
		return err
	}
	if q.Overflow.Type == DestQueue && q.Overflow.Target == strconv.FormatInt(q.ID, 10) {
		return invalid("loop", "overflow")
	}
	seen := map[int64]bool{}
	for _, m := range q.Members {
		if seen[m.ExtensionID] {
			return invalid("duplicate", "members")
		}
		seen[m.ExtensionID] = true
		if _, err := r.Extension(ctx, m.ExtensionID); err != nil {
			return invalid("unknown_member", "members")
		}
		if m.Penalty < 0 || m.Penalty > 10 {
			return invalid("invalid", "members")
		}
	}
	if err := validateRemoteMembers(ctx, r, q.RemoteMembers); err != nil {
		return err
	}
	if err := q.Options.normalize(); err != nil {
		return err
	}
	optsJSON, _ := json.Marshal(q.Options)
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{q.Number, q.Name, q.Strategy, q.Music, b2i(q.AnnouncePosition), q.AnnounceFrequency, q.AgentTimeout, q.Retry,
		q.Wrapup, q.MaxWait, q.MaxCallers, b2i(q.JoinEmpty), b2i(q.Record), q.CIDPrefix, q.GreetingPrompt, q.Overflow.Type,
		q.Overflow.Target, b2i(q.Enabled), string(optsJSON)}
	if q.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO queues(number, name, strategy, music, announce_position, announce_frequency,
			agent_timeout, retry, wrapup, max_wait, max_callers, join_empty, record, cid_prefix, greeting_prompt, overflow_type,
			overflow, enabled, options) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "number")
		}
		q.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE queues SET number=?, name=?, strategy=?, music=?, announce_position=?,
			announce_frequency=?, agent_timeout=?, retry=?, wrapup=?, max_wait=?, max_callers=?, join_empty=?, record=?, cid_prefix=?,
			greeting_prompt=?, overflow_type=?, overflow=?, enabled=?, options=? WHERE id=?`, append(args, q.ID)...)
		if err != nil {
			return uniqueErr(err, "number")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	// Keep the sign-in state of members that stay.
	state := map[int64][2]int{}
	rows, err := tx.QueryContext(ctx, `SELECT extension_id, logged_in, paused FROM queue_members WHERE queue_id = ?`, q.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var li, pa int
		if err := rows.Scan(&id, &li, &pa); err != nil {
			rows.Close()
			return err
		}
		state[id] = [2]int{li, pa}
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM queue_members WHERE queue_id = ?`, q.ID); err != nil {
		return err
	}
	for _, m := range q.Members {
		li, pa := 1, 0
		if st, ok := state[m.ExtensionID]; ok {
			li, pa = st[0], st[1]
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO queue_members(queue_id, extension_id, penalty, dynamic, logged_in, paused)
			VALUES (?,?,?,?,?,?)`, q.ID, m.ExtensionID, m.Penalty, b2i(m.Dynamic), li, pa); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM queue_remote_members WHERE queue_id = ?`, q.ID); err != nil {
		return err
	}
	for _, m := range q.RemoteMembers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO queue_remote_members(queue_id, peer_id, number, penalty) VALUES (?,?,?,?)`,
			q.ID, m.PeerID, m.Number, m.Penalty); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteQueue removes a queue.
func (r *Repo) DeleteQueue(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "queues", DestQueue, id)
}

// SetQueueLogin signs an agent in or out of a queue (only for dynamic members).
// queueID 0 means every queue the extension is a dynamic member of. It returns the number of queues changed.
func (r *Repo) SetQueueLogin(ctx context.Context, queueID, extID int64, in bool) (int64, error) {
	q := `UPDATE queue_members SET logged_in = ?, paused = 0 WHERE extension_id = ? AND dynamic = 1`
	args := []any{b2i(in), extID}
	if queueID != 0 {
		q += ` AND queue_id = ?`
		args = append(args, queueID)
	}
	res, err := r.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetQueuePause pauses an agent (still signed in, but gets no calls).
func (r *Repo) SetQueuePause(ctx context.Context, queueID, extID int64, paused bool) error {
	q := `UPDATE queue_members SET paused = ? WHERE extension_id = ?`
	args := []any{b2i(paused), extID}
	if queueID != 0 {
		q += ` AND queue_id = ?`
		args = append(args, queueID)
	}
	_, err := r.DB.ExecContext(ctx, q, args...)
	return err
}

// ---- IVR --------------------------------------------------------------------

// IVROption is a key the caller can press.
type IVROption struct {
	Digit       string      `json:"digit"`
	Destination Destination `json:"destination"`
}

// IVR is a voice menu ("press 1 for sales").
type IVR struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	PromptID int64  `json:"promptId"`
	Timeout  int    `json:"timeout"`
	Retries  int    `json:"retries"`
	// DirectDial lets callers dial an extension number during the menu.
	DirectDial bool        `json:"directDial"`
	Options    []IVROption `json:"options"`
	// TimeoutDest is used when the caller presses nothing or a wrong key too often.
	TimeoutDest Destination `json:"timeoutDestination"`
	// Flow is the menu as drawn on the canvas; the fields above are derived from it on save.
	Flow *Flow `json:"flow"`
}

const ivrCols = `id, name, prompt_id, timeout, retries, direct_dial, options, timeout_type, timeout_target, layout`

func scanIVR(row interface{ Scan(...any) error }) (*IVR, error) {
	var v IVR
	var dd int
	var opts, layout string
	err := row.Scan(&v.ID, &v.Name, &v.PromptID, &v.Timeout, &v.Retries, &dd, &opts, &v.TimeoutDest.Type, &v.TimeoutDest.Target, &layout)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	v.DirectDial = dd == 1
	_ = json.Unmarshal([]byte(opts), &v.Options)
	if v.Options == nil {
		v.Options = []IVROption{}
	}
	if err != nil {
		return nil, err
	}
	var l ivrLayout
	_ = json.Unmarshal([]byte(layout), &l)
	v.Flow = l.Flow
	if v.Flow == nil {
		v.Flow = LegacyFlow(&v, l.Labels)
	}
	return &v, nil
}

// IVRs returns all voice menus.
func (r *Repo) IVRs(ctx context.Context) ([]IVR, error) {
	return queryAll(ctx, r.DB, `SELECT `+ivrCols+` FROM ivrs ORDER BY name`, scanIVR)
}

// IVR returns one voice menu.
func (r *Repo) IVR(ctx context.Context, id int64) (*IVR, error) {
	return scanIVR(r.DB.QueryRowContext(ctx, `SELECT `+ivrCols+` FROM ivrs WHERE id = ?`, id))
}

var reIVRDigit = regexp.MustCompile(`^[0-9*#]$`)

// SaveIVR validates and stores a voice menu.
func (r *Repo) SaveIVR(ctx context.Context, v *IVR) error {
	v.Name = cleanText(v.Name, 60)
	if v.Name == "" {
		return invalid("required", "name")
	}
	if v.Timeout == 0 {
		v.Timeout = 5
	}
	if v.Timeout < 1 || v.Timeout > 60 {
		return invalid("invalid", "timeout")
	}
	if v.Retries < 0 || v.Retries > 5 {
		return invalid("invalid", "retries")
	}
	if v.Flow == nil {
		// An API client that only sends options: check them and build the flow from them.
		if v.PromptID != 0 {
			if _, err := r.Prompt(ctx, v.PromptID); err != nil {
				return invalid("unknown_prompt", "promptId")
			}
		}
		seen := map[string]bool{}
		if v.Options == nil {
			v.Options = []IVROption{}
		}
		for i := range v.Options {
			o := &v.Options[i]
			if !reIVRDigit.MatchString(o.Digit) || seen[o.Digit] {
				return invalid("invalid", "options")
			}
			seen[o.Digit] = true
			if err := r.ValidateDestination(ctx, &o.Destination, "options"); err != nil {
				return err
			}
		}
		if err := r.ValidateDestination(ctx, &v.TimeoutDest, "timeoutDestination"); err != nil {
			return err
		}
		v.Flow = LegacyFlow(v, nil)
	}
	if err := r.validateFlow(ctx, v); err != nil {
		return err
	}
	layout, _ := json.Marshal(ivrLayout{Flow: v.Flow})
	if len(layout) > 512<<10 {
		return invalid("too_large", "flow")
	}
	opts, _ := json.Marshal(v.Options)
	args := []any{v.Name, v.PromptID, v.Timeout, v.Retries, b2i(v.DirectDial), string(opts), v.TimeoutDest.Type, v.TimeoutDest.Target, string(layout)}
	if v.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO ivrs(name, prompt_id, timeout, retries, direct_dial, options, timeout_type,
			timeout_target, layout) VALUES (?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return err
		}
		v.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE ivrs SET name=?, prompt_id=?, timeout=?, retries=?, direct_dial=?, options=?, timeout_type=?,
		timeout_target=?, layout=? WHERE id=?`, append(args, v.ID), "name")
}

// DeleteIVR removes a voice menu.
func (r *Repo) DeleteIVR(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "ivrs", DestIVR, id)
}

// ---- time conditions --------------------------------------------------------

// TimeRange is a block of opening hours on some days of the week (1 = Monday ... 7 = Sunday).
type TimeRange struct {
	Days []int  `json:"days"`
	From string `json:"from"` // HH:MM
	To   string `json:"to"`   // HH:MM
}

// TimeCondition sends calls one way during opening hours and another way outside them.
type TimeCondition struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	Schedule    []TimeRange `json:"schedule"`
	UseHolidays bool        `json:"useHolidays"`
	// Mode: auto (follow the schedule), open or closed (manual day/night switch).
	Mode    string      `json:"mode"`
	Open    Destination `json:"open"`
	Closed  Destination `json:"closed"`
	Holiday Destination `json:"holiday"` // type "" = same as closed
}

const tcCols = `id, name, schedule, use_holidays, mode, open_type, open_target, closed_type, closed_target, holiday_type, holiday_target`

func scanTC(row interface{ Scan(...any) error }) (*TimeCondition, error) {
	var t TimeCondition
	var sched string
	var uh int
	err := row.Scan(&t.ID, &t.Name, &sched, &uh, &t.Mode, &t.Open.Type, &t.Open.Target, &t.Closed.Type, &t.Closed.Target,
		&t.Holiday.Type, &t.Holiday.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	t.UseHolidays = uh == 1
	_ = json.Unmarshal([]byte(sched), &t.Schedule)
	if t.Schedule == nil {
		t.Schedule = []TimeRange{}
	}
	return &t, err
}

// TimeConditions returns all time conditions.
func (r *Repo) TimeConditions(ctx context.Context) ([]TimeCondition, error) {
	return queryAll(ctx, r.DB, `SELECT `+tcCols+` FROM time_conditions ORDER BY id`, scanTC)
}

// TimeCondition returns one time condition.
func (r *Repo) TimeCondition(ctx context.Context, id int64) (*TimeCondition, error) {
	return scanTC(r.DB.QueryRowContext(ctx, `SELECT `+tcCols+` FROM time_conditions WHERE id = ?`, id))
}

var reHHMM = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// SaveTimeCondition validates and stores a time condition.
func (r *Repo) SaveTimeCondition(ctx context.Context, t *TimeCondition) error {
	t.Name = cleanText(t.Name, 60)
	if t.Name == "" {
		return invalid("required", "name")
	}
	switch t.Mode {
	case "":
		t.Mode = "auto"
	case "auto", "open", "closed":
	default:
		return invalid("invalid", "mode")
	}
	if len(t.Schedule) > 30 {
		return invalid("invalid", "schedule")
	}
	if t.Schedule == nil {
		t.Schedule = []TimeRange{}
	}
	for i := range t.Schedule {
		s := &t.Schedule[i]
		if len(s.Days) == 0 || !reHHMM.MatchString(s.From) || !(reHHMM.MatchString(s.To) || s.To == "24:00") || s.From >= s.To {
			return invalid("invalid", "schedule")
		}
		slices.Sort(s.Days)
		s.Days = slices.Compact(s.Days)
		for _, d := range s.Days {
			if d < 1 || d > 7 {
				return invalid("invalid", "schedule")
			}
		}
	}
	if err := r.ValidateDestination(ctx, &t.Open, "open"); err != nil {
		return err
	}
	if err := r.ValidateDestination(ctx, &t.Closed, "closed"); err != nil {
		return err
	}
	if t.Holiday.Type != "" {
		if err := r.ValidateDestination(ctx, &t.Holiday, "holiday"); err != nil {
			return err
		}
	} else {
		t.Holiday.Target = ""
	}
	self := strconv.FormatInt(t.ID, 10)
	for _, d := range []Destination{t.Open, t.Closed, t.Holiday} {
		if t.ID != 0 && d.Type == DestTimeCondition && d.Target == self {
			return invalid("loop", "open")
		}
	}
	sched, _ := json.Marshal(t.Schedule)
	args := []any{t.Name, string(sched), b2i(t.UseHolidays), t.Mode, t.Open.Type, t.Open.Target, t.Closed.Type, t.Closed.Target,
		t.Holiday.Type, t.Holiday.Target}
	if t.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO time_conditions(name, schedule, use_holidays, mode, open_type, open_target,
			closed_type, closed_target, holiday_type, holiday_target) VALUES (?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return err
		}
		t.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE time_conditions SET name=?, schedule=?, use_holidays=?, mode=?, open_type=?, open_target=?,
		closed_type=?, closed_target=?, holiday_type=?, holiday_target=? WHERE id=?`, append(args, t.ID), "name")
}

// SetTimeConditionMode switches a time condition by hand (the day/night switch).
func (r *Repo) SetTimeConditionMode(ctx context.Context, id int64, mode string) error {
	if mode != "auto" && mode != "open" && mode != "closed" {
		return invalid("invalid", "mode")
	}
	return updateRow(ctx, r.DB, `UPDATE time_conditions SET mode = ? WHERE id = ?`, []any{mode, id}, "mode")
}

// DeleteTimeCondition removes a time condition.
func (r *Repo) DeleteTimeCondition(ctx context.Context, id int64) error {
	if _, err := r.DB.ExecContext(ctx, `UPDATE extensions SET features = json_set(features, '$.scheduleId', 0)
		WHERE json_extract(features, '$.scheduleId') = ?`, id); err != nil {
		return err
	}
	return r.deleteWithRefs(ctx, "time_conditions", DestTimeCondition, id)
}

// ---- conferences ------------------------------------------------------------

// Conference is a conference room callers join with an optional PIN.
type Conference struct {
	ID         int64  `json:"id"`
	Number     string `json:"number"`
	Name       string `json:"name"`
	PIN        string `json:"pin"`
	AdminPIN   string `json:"adminPin"`
	MaxMembers int    `json:"maxMembers"`
	Record     bool   `json:"record"`
	// WaitAdmin keeps participants on music until someone joins with the admin PIN.
	WaitAdmin bool `json:"waitAdmin"`
	Enabled   bool `json:"enabled"`
}

const confCols = `id, number, name, pin, admin_pin, max_members, record, wait_admin, enabled`

func scanConf(row interface{ Scan(...any) error }) (*Conference, error) {
	var c Conference
	var rec, wa, en int
	err := row.Scan(&c.ID, &c.Number, &c.Name, &c.PIN, &c.AdminPIN, &c.MaxMembers, &rec, &wa, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	c.Record, c.WaitAdmin, c.Enabled = rec == 1, wa == 1, en == 1
	return &c, err
}

// Conferences returns all conference rooms.
func (r *Repo) Conferences(ctx context.Context) ([]Conference, error) {
	return queryAll(ctx, r.DB, `SELECT `+confCols+` FROM conferences ORDER BY number`, scanConf)
}

// SaveConference validates and stores a conference room.
func (r *Repo) SaveConference(ctx context.Context, plan NumberPlan, c *Conference) error {
	c.Name = cleanText(c.Name, 60)
	if c.Name == "" {
		return invalid("required", "name")
	}
	if !plan.InRange("conferences", c.Number) {
		return invalid("number_out_of_range", "number")
	}
	for field, p := range map[string]string{"pin": c.PIN, "adminPin": c.AdminPIN} {
		if p != "" && (!reDigits.MatchString(p) || len(p) < 4 || len(p) > 10) {
			return invalid("invalid", field)
		}
	}
	if c.PIN != "" && c.PIN == c.AdminPIN {
		return invalid("invalid", "adminPin")
	}
	if c.WaitAdmin && c.AdminPIN == "" {
		return invalid("host_pin_needed", "adminPin")
	}
	if c.MaxMembers == 0 {
		c.MaxMembers = 20
	}
	if c.MaxMembers < 2 || c.MaxMembers > 100 {
		return invalid("invalid", "maxMembers")
	}
	args := []any{c.Number, c.Name, c.PIN, c.AdminPIN, c.MaxMembers, b2i(c.Record), b2i(c.WaitAdmin), b2i(c.Enabled)}
	if c.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO conferences(number, name, pin, admin_pin, max_members, record, wait_admin, enabled)
			VALUES (?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return uniqueErr(err, "number")
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE conferences SET number=?, name=?, pin=?, admin_pin=?, max_members=?, record=?, wait_admin=?,
		enabled=? WHERE id=?`, append(args, c.ID), "number")
}

// DeleteConference removes a conference room.
func (r *Repo) DeleteConference(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "conferences", DestConference, id)
}

// ---- prompts ----------------------------------------------------------------

// Prompt is an uploaded or recorded audio file, used by IVRs and queues.
type Prompt struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	DurationMS int64         `json:"durationMs"`
	CreatedAt  string        `json:"createdAt"`
	Source     *PromptSource `json:"source,omitempty"`
}

// PromptSource says how a prompt was spoken from text. Recorded and uploaded prompts have none.
type PromptSource struct {
	Engine    string `json:"engine"` // piper | elevenlabs
	Voice     string `json:"voice"`
	VoiceName string `json:"voiceName,omitempty"`
	Model     string `json:"model,omitempty"`
	Language  string `json:"language,omitempty"`
	Text      string `json:"text"`
}

// Valid reports whether the source can be stored.
func (s *PromptSource) Valid() bool {
	return (s.Engine == "piper" || s.Engine == "elevenlabs") && s.Voice != "" && len(s.Voice) <= 64 &&
		len(s.VoiceName) <= 80 && len(s.Model) <= 64 && len(s.Language) <= 8 && s.Text != "" && len(s.Text) <= 8000
}

func scanPrompt(row interface{ Scan(...any) error }) (*Prompt, error) {
	var p Prompt
	var src string
	err := row.Scan(&p.ID, &p.Name, &p.DurationMS, &p.CreatedAt, &src)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil && src != "" && src != "{}" {
		var s PromptSource
		if json.Unmarshal([]byte(src), &s) == nil && s.Text != "" {
			p.Source = &s
		}
	}
	return &p, err
}

func promptSourceJSON(s *PromptSource) string {
	if s == nil || !s.Valid() {
		return "{}"
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Prompts returns all prompts.
func (r *Repo) Prompts(ctx context.Context) ([]Prompt, error) {
	return queryAll(ctx, r.DB, `SELECT id, name, duration_ms, created_at, source FROM prompts ORDER BY name`, scanPrompt)
}

// Prompt returns one prompt.
func (r *Repo) Prompt(ctx context.Context, id int64) (*Prompt, error) {
	return scanPrompt(r.DB.QueryRowContext(ctx, `SELECT id, name, duration_ms, created_at, source FROM prompts WHERE id = ?`, id))
}

// SavePrompt stores the prompt metadata (the audio file is written by the caller).
func (r *Repo) SavePrompt(ctx context.Context, p *Prompt) error {
	p.Name = cleanText(p.Name, 60)
	if p.Name == "" {
		return invalid("required", "name")
	}
	if p.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO prompts(name, duration_ms, source) VALUES (?, ?, ?)`, p.Name, p.DurationMS, promptSourceJSON(p.Source))
		if err != nil {
			return err
		}
		p.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE prompts SET name = ?, duration_ms = ?, source = ? WHERE id = ?`, []any{p.Name, p.DurationMS, promptSourceJSON(p.Source), p.ID}, "name")
}

// PromptInUse reports whether an IVR or queue plays the prompt.
func (r *Repo) PromptInUse(ctx context.Context, id int64) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM ivrs WHERE prompt_id = ?) + (SELECT COUNT(*) FROM queues WHERE greeting_prompt = ?)`, id, id).Scan(&n)
	if err != nil || n > 0 {
		return n > 0, err
	}
	return r.promptUsedByFlows(ctx, id)
}

// DeletePrompt removes the prompt metadata.
func (r *Repo) DeletePrompt(ctx context.Context, id int64) error {
	return deleteRow(ctx, r.DB, "prompts", id)
}

// ---- system-wide feature settings -------------------------------------------

// FeaturesKey is the settings row that holds FeatureSettings.
const FeaturesKey = "pbx_features"

// FraudSettings protect against expensive outbound calls when an account is abused.
type FraudSettings struct {
	// PremiumPrefixes are national numbers with a high rate (0900, 0906, 0909 ...).
	PremiumPrefixes []string `json:"premiumPrefixes"`
	// BlockedCountries are country codes that are never allowed, even for international extensions.
	BlockedCountries []string `json:"blockedCountries"`
	// MaxOutbound is the maximum number of outbound calls at the same time. 0: no limit.
	MaxOutbound int `json:"maxOutbound"`
	// AlertEmail gets a message when the thresholds below are passed.
	AlertEmail           string `json:"alertEmail"`
	AlertOutboundPerHour int    `json:"alertOutboundPerHour"`
	AlertForeignPerHour  int    `json:"alertInternationalPerHour"`
	// BlockOnAlert blocks international and premium calls after an alert until an admin unblocks them.
	BlockOnAlert bool `json:"blockOnAlert"`
}

// FeatureSettings are system-wide options for parking, recording and fraud protection.
type FeatureSettings struct {
	ParkingSlots   int  `json:"parkingSlots"`
	ParkingTimeout int  `json:"parkingTimeout"`
	RecordingDays  int  `json:"recordingDays"`
	RecordingBeep  bool `json:"recordingBeep"`
	// JourneyDays is how long the call journey (the steps of each call) is kept; 0 records none.
	JourneyDays int           `json:"journeyDays"`
	Fraud       FraudSettings `json:"fraud"`
}

// DefaultFeatureSettings are used on a fresh install. The premium list fits the Netherlands.
func DefaultFeatureSettings() FeatureSettings {
	return FeatureSettings{
		ParkingSlots: 8, ParkingTimeout: 120, RecordingDays: 90, JourneyDays: 90,
		Fraud: FraudSettings{
			PremiumPrefixes:      []string{"0900", "0906", "0909", "084", "087", "18"},
			BlockedCountries:     []string{},
			MaxOutbound:          10,
			AlertOutboundPerHour: 60,
			AlertForeignPerHour:  10,
		},
	}
}

// LoadFeatureSettings reads the feature settings with defaults.
func LoadFeatureSettings(ctx context.Context, db *sql.DB) (FeatureSettings, error) {
	f := DefaultFeatureSettings()
	err := store.GetJSON(ctx, db, FeaturesKey, &f)
	if f.Fraud.PremiumPrefixes == nil {
		f.Fraud.PremiumPrefixes = []string{}
	}
	if f.Fraud.BlockedCountries == nil {
		f.Fraud.BlockedCountries = []string{}
	}
	return f, err
}

// Validate checks the feature settings.
func (f *FeatureSettings) Validate() error {
	if f.ParkingSlots < 0 || f.ParkingSlots > 9 {
		return invalid("invalid", "parkingSlots")
	}
	if f.ParkingTimeout < 30 || f.ParkingTimeout > 3600 {
		return invalid("invalid", "parkingTimeout")
	}
	if f.RecordingDays < 1 || f.RecordingDays > 3650 {
		return invalid("invalid", "recordingDays")
	}
	if f.JourneyDays < 0 || f.JourneyDays > 3650 {
		return invalid("invalid", "journeyDays")
	}
	clean := func(list []string, max int, field string) ([]string, error) {
		out := []string{}
		for _, p := range list {
			p = strings.TrimPrefix(NormalizeDID(p), "+")
			if p == "" {
				continue
			}
			if !reDigits.MatchString(p) || len(p) > max {
				return nil, invalid("invalid", field)
			}
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
		return out, nil
	}
	var err error
	if f.Fraud.PremiumPrefixes, err = clean(f.Fraud.PremiumPrefixes, 8, "premiumPrefixes"); err != nil {
		return err
	}
	if f.Fraud.BlockedCountries, err = clean(f.Fraud.BlockedCountries, 4, "blockedCountries"); err != nil {
		return err
	}
	if f.Fraud.MaxOutbound < 0 || f.Fraud.MaxOutbound > 500 {
		return invalid("invalid", "maxOutbound")
	}
	if f.Fraud.AlertOutboundPerHour < 0 || f.Fraud.AlertForeignPerHour < 0 {
		return invalid("invalid", "alertOutboundPerHour")
	}
	f.Fraud.AlertEmail = strings.TrimSpace(f.Fraud.AlertEmail)
	if f.Fraud.AlertEmail != "" && !strings.Contains(f.Fraud.AlertEmail, "@") {
		return invalid("invalid", "alertEmail")
	}
	return nil
}

// FeatureCode documents a code users dial on their phone. The UI shows this list.
type FeatureCode struct {
	Code string `json:"code"`
	Key  string `json:"key"` // translation key
}

// FeatureCodes are the star codes Kiestoon's dialplan understands.
var FeatureCodes = []FeatureCode{
	{"*97", "voicemail"}, {"*98", "voicemailAny"}, {"*43", "echo"},
	{"*72<number>", "forwardOn"}, {"*73", "forwardOff"}, {"*76", "dndToggle"},
	{"*45", "queueToggle"}, {"*45<queue>", "queueToggleOne"},
	{"*8", "groupPickup"}, {"**<extension>", "directedPickup"},
	{"70", "park"}, {"71 … 79", "parkSlots"},
	{"*80<extension>", "intercom"}, {"*1", "recordToggle"},
	{"*28<id>", "dayNight"}, {"*2", "attendedTransfer"}, {"##", "blindTransfer"},
}

// ParkSlot returns the number of parking slot i (1-based): 71, 72, ...
func ParkSlot(i int) string { return fmt.Sprintf("7%d", i) }
