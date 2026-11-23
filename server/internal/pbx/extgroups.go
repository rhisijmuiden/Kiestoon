package pbx

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ExtensionGroup is a set of extensions that call out the same way: they share a caller ID, and
// outbound routes can be limited to some groups (so a group can get its own trunk). An extension
// is in at most one group.
//
// The caller ID of an outside call is chosen in this order: the extension's own outbound caller
// ID, then its group's, then the outbound route's, then the trunk's.
//
// Settings that the group sets are enforced on every member; the ones it leaves unset stay per
// extension. Key templates (one per phone model) are copied to the members' phones.
type ExtensionGroup struct {
	ID             int64         `json:"id"`
	Name           string        `json:"name"`
	CallerID       string        `json:"callerId"`
	Settings       GroupSettings `json:"settings"`
	KeyTemplateIDs []int64       `json:"keyTemplateIds"`
	Members        []int64       `json:"members"` // extension ids
}

// GroupSettings are extension settings a group can enforce. nil: each extension decides.
type GroupSettings struct {
	Voicemail       *bool   `json:"voicemail,omitempty"`
	VoicemailEmail  *bool   `json:"voicemailEmail,omitempty"`
	VoicemailDelete *bool   `json:"voicemailDelete,omitempty"`
	RingSeconds     *int    `json:"ringSeconds,omitempty"`
	RecordMode      *string `json:"recordMode,omitempty"`
	PickupGroup     *string `json:"pickupGroup,omitempty"`
	OutboundClass   *string `json:"outboundClass,omitempty"`
	AllowPremium    *bool   `json:"allowPremium,omitempty"`
	WebRTC          *bool   `json:"webrtc,omitempty"`
	MaxContacts     *int    `json:"maxContacts,omitempty"`
}

// Apply writes the enforced settings into an extension.
func (g GroupSettings) Apply(e *Extension) {
	if g.Voicemail != nil {
		e.Voicemail = *g.Voicemail
	}
	if g.VoicemailEmail != nil {
		// Mailing voicemail needs an e-mail address; validateExtension turns it off without one.
		e.VoicemailEmail = *g.VoicemailEmail
	}
	if g.VoicemailDelete != nil {
		e.VoicemailDelete = *g.VoicemailDelete
	}
	if g.RingSeconds != nil {
		e.RingSeconds = *g.RingSeconds
	}
	if g.RecordMode != nil {
		e.RecordMode = *g.RecordMode
	}
	if g.PickupGroup != nil {
		e.PickupGroup = *g.PickupGroup
	}
	if g.OutboundClass != nil {
		e.OutboundClass = *g.OutboundClass
	}
	if g.AllowPremium != nil {
		e.AllowPremium = *g.AllowPremium
	}
	if g.WebRTC != nil {
		e.WebRTC = *g.WebRTC
	}
	if g.MaxContacts != nil {
		e.MaxContacts = *g.MaxContacts
	}
}

func (g *GroupSettings) validate() error {
	if g.RingSeconds != nil && (*g.RingSeconds < 5 || *g.RingSeconds > 300) {
		return invalid("invalid", "settings.ringSeconds")
	}
	if g.MaxContacts != nil && (*g.MaxContacts < 1 || *g.MaxContacts > 10) {
		return invalid("invalid", "settings.maxContacts")
	}
	if g.RecordMode != nil && *g.RecordMode != "off" && *g.RecordMode != "ondemand" && *g.RecordMode != "always" {
		return invalid("invalid", "settings.recordMode")
	}
	if g.OutboundClass != nil && *g.OutboundClass != "internal" && *g.OutboundClass != "national" && *g.OutboundClass != "international" {
		return invalid("invalid", "settings.outboundClass")
	}
	if g.PickupGroup != nil {
		v := strings.TrimSpace(*g.PickupGroup)
		g.PickupGroup = &v
		if v != "" && !rePickup.MatchString(v) {
			return invalid("invalid", "settings.pickupGroup")
		}
	}
	return nil
}

var rePickup = regexp.MustCompile(`^[0-9]{1,2}$`)

// ExtensionGroup returns one group.
func (r *Repo) ExtensionGroup(ctx context.Context, id int64) (*ExtensionGroup, error) {
	all, err := r.ExtensionGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, ErrNotFound
}

// ExtensionGroups returns all extension groups by name, with their members by extension number.
func (r *Repo) ExtensionGroups(ctx context.Context) ([]ExtensionGroup, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name, caller_id, settings, key_template_ids FROM extension_groups ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	out := []ExtensionGroup{}
	idx := map[int64]int{}
	for rows.Next() {
		g := ExtensionGroup{Members: []int64{}}
		var set, kt string
		if err := rows.Scan(&g.ID, &g.Name, &g.CallerID, &set, &kt); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(set), &g.Settings)
		g.KeyTemplateIDs = scanGroupIDs(kt)
		idx[g.ID] = len(out)
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mrows, err := r.DB.QueryContext(ctx, `SELECT m.group_id, m.extension_id FROM extension_group_members m
		JOIN extensions e ON e.id = m.extension_id ORDER BY e.number`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var gid, eid int64
		if err := mrows.Scan(&gid, &eid); err != nil {
			return nil, err
		}
		if i, ok := idx[gid]; ok {
			out[i].Members = append(out[i].Members, eid)
		}
	}
	return out, mrows.Err()
}

// SaveExtensionGroup validates and stores a group with its members. An extension that was in
// another group moves to this one.
func (r *Repo) SaveExtensionGroup(ctx context.Context, g *ExtensionGroup) error {
	g.Name = cleanText(g.Name, 60)
	if g.Name == "" {
		return invalid("required", "name")
	}
	g.CallerID = NormalizeDID(strings.TrimSpace(g.CallerID))
	if g.CallerID != "" && !reCallerID.MatchString(g.CallerID) {
		return invalid("invalid", "callerId")
	}
	seen := map[int64]bool{}
	members := []int64{}
	for _, id := range g.Members {
		if seen[id] {
			continue
		}
		seen[id] = true
		var n int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM extensions WHERE id = ?`, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return invalid("unknown_member", "members")
		}
		members = append(members, id)
	}
	g.Members = members
	if err := g.Settings.validate(); err != nil {
		return err
	}
	// At most one template per phone model, and only templates that exist.
	models := map[string]bool{}
	kts := []int64{}
	for _, id := range g.KeyTemplateIDs {
		t, err := r.KeyTemplate(ctx, id)
		if err != nil {
			return invalid("unknown_template", "keyTemplateIds")
		}
		if models[t.Vendor+"/"+t.Model] {
			return invalid("template_model_twice", "keyTemplateIds")
		}
		models[t.Vendor+"/"+t.Model] = true
		kts = append(kts, id)
	}
	g.KeyTemplateIDs = kts
	set, _ := json.Marshal(g.Settings)
	ktj, _ := json.Marshal(g.KeyTemplateIDs)

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if g.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO extension_groups(name, caller_id, settings, key_template_ids, created_at) VALUES (?,?,?,?,?)`,
			g.Name, g.CallerID, string(set), string(ktj), time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return uniqueErr(err, "name")
		}
		g.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE extension_groups SET name = ?, caller_id = ?, settings = ?, key_template_ids = ? WHERE id = ?`,
			g.Name, g.CallerID, string(set), string(ktj), g.ID)
		if err != nil {
			return uniqueErr(err, "name")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM extension_group_members WHERE group_id = ?`, g.ID); err != nil {
		return err
	}
	for _, id := range g.Members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO extension_group_members(extension_id, group_id) VALUES (?,?)
			ON CONFLICT(extension_id) DO UPDATE SET group_id = excluded.group_id`, id, g.ID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return r.applyGroupSettings(ctx, g)
}

// applyGroupSettings writes the group's enforced settings into its members.
func (r *Repo) applyGroupSettings(ctx context.Context, g *ExtensionGroup) error {
	for _, id := range g.Members {
		e, err := r.Extension(ctx, id)
		if err != nil {
			continue
		}
		g.Settings.Apply(e)
		if e.VoicemailEmail && e.Email == "" {
			e.VoicemailEmail = false
		}
		if err := r.saveExtensionRow(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// DeleteExtensionGroup removes a group. A group that an outbound route is limited to cannot be
// deleted: the route would otherwise silently open up to everyone.
func (r *Repo) DeleteExtensionGroup(ctx context.Context, id int64) error {
	routes, err := r.OutboundRoutes(ctx)
	if err != nil {
		return err
	}
	for _, rt := range routes {
		for _, gid := range rt.GroupIDs {
			if gid == id {
				return invalid("group_in_use", "id")
			}
		}
	}
	return deleteRow(ctx, r.DB, "extension_groups", id)
}

// SetExtensionGroup puts one extension in a group (0 takes it out of its group).
func (r *Repo) SetExtensionGroup(ctx context.Context, extID, groupID int64) error {
	if groupID == 0 {
		_, err := r.DB.ExecContext(ctx, `DELETE FROM extension_group_members WHERE extension_id = ?`, extID)
		return err
	}
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM extension_groups WHERE id = ?`, groupID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("unknown_group", "groupId")
	}
	if _, err := r.DB.ExecContext(ctx, `INSERT INTO extension_group_members(extension_id, group_id) VALUES (?,?)
		ON CONFLICT(extension_id) DO UPDATE SET group_id = excluded.group_id`, extID, groupID); err != nil {
		return err
	}
	g, err := r.ExtensionGroup(ctx, groupID)
	if err != nil {
		return err
	}
	return r.applyGroupSettings(ctx, &ExtensionGroup{Settings: g.Settings, Members: []int64{extID}})
}

func (r *Repo) validateGroupIDs(ctx context.Context, ids []int64) ([]int64, error) {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		var n int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM extension_groups WHERE id = ?`, id).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, invalid("unknown_group", "groupIds")
		}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func scanGroupIDs(s string) []int64 {
	out := []int64{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []int64{}
	}
	return out
}

// GroupOf returns the group an extension is in, or nil.
func (s *Snapshot) GroupOf(extID int64) *ExtensionGroup {
	for i := range s.ExtGroups {
		for _, m := range s.ExtGroups[i].Members {
			if m == extID {
				return &s.ExtGroups[i]
			}
		}
	}
	return nil
}

// OutboundCallerID is the caller ID an extension sends on outside calls before the route and
// trunk defaults: its own, else its group's ("" lets the route or trunk decide).
func (s *Snapshot) OutboundCallerID(e Extension) string {
	if e.OutboundCID != "" {
		return e.OutboundCID
	}
	if g := s.GroupOf(e.ID); g != nil {
		return g.CallerID
	}
	return ""
}

// GroupID is the id of the extension's group, 0 when it has none.
func (s *Snapshot) GroupID(e Extension) int64 {
	if g := s.GroupOf(e.ID); g != nil {
		return g.ID
	}
	return 0
}
