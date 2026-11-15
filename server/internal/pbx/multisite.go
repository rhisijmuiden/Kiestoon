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
	"time"

	"github.com/rhisijmuiden/kiestoon/server/internal/multisite"
	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Destination types of phase 5.
const (
	// DestRemote is a number on a federated Kiestoon: target "<peer id>:<number>".
	DestRemote = "remote"
	// DestFollowSun is a follow-the-sun route: target is its ID.
	DestFollowSun = "followsun"
)

// HubKey is the settings row with the WireGuard hub (HubSettings as JSON).
const HubKey = "wireguard_hub"

// HubSettings is the WireGuard interface kt0 that edges and federated sites connect to.
type HubSettings struct {
	Enabled    bool   `json:"enabled"`
	Subnet     string `json:"subnet"`
	PrivateKey string `json:"privateKey,omitempty"`
	PublicKey  string `json:"publicKey"`
	ListenPort int    `json:"listenPort"`
	// Host is the public host name or IP address edges and other sites reach this system on.
	Host string `json:"host"`
}

// Endpoint is host:port for the other side.
func (h HubSettings) Endpoint() string {
	host := h.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s:%d", host, h.ListenPort)
}

// LoadHub reads the hub settings.
func LoadHub(ctx context.Context, db *sql.DB) (HubSettings, error) {
	h := HubSettings{ListenPort: 51820}
	err := store.GetJSON(ctx, db, HubKey, &h)
	return h, err
}

// EdgeSite is a branch office with a Kiestoon Edge.
type EdgeSite struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Paired      bool              `json:"paired"`
	PairExpires string            `json:"pairExpires"`
	PublicKey   string            `json:"publicKey"`
	TunnelIP    string            `json:"tunnelIp"`
	Hostname    string            `json:"hostname"`
	Version     string            `json:"version"`
	LANAddress  string            `json:"lanAddress"`
	LANNet      string            `json:"lanNet"`
	Services    map[string]string `json:"services"`
	LastSeen    string            `json:"lastSeen"`
	PairedAt    string            `json:"pairedAt"`
	CreatedAt   string            `json:"createdAt"`
}

const edgeCols = `id, name, token_hash, pair_expires, public_key, tunnel_ip, hostname, version, lan_address, lan_net, services, last_seen, paired_at, created_at`

func scanEdge(row interface{ Scan(...any) error }) (*EdgeSite, error) {
	var e EdgeSite
	var tok, svc string
	if err := row.Scan(&e.ID, &e.Name, &tok, &e.PairExpires, &e.PublicKey, &e.TunnelIP, &e.Hostname, &e.Version, &e.LANAddress,
		&e.LANNet, &svc, &e.LastSeen, &e.PairedAt, &e.CreatedAt); err != nil {
		return nil, err
	}
	e.Paired = tok != ""
	_ = json.Unmarshal([]byte(svc), &e.Services)
	if e.Services == nil {
		e.Services = map[string]string{}
	}
	return &e, nil
}

// EdgeSites lists the edges.
func (r *Repo) EdgeSites(ctx context.Context) ([]EdgeSite, error) {
	return queryAll(ctx, r.DB, `SELECT `+edgeCols+` FROM edge_sites ORDER BY name COLLATE NOCASE`, scanEdge)
}

// EdgeSite returns one edge.
func (r *Repo) EdgeSite(ctx context.Context, id int64) (*EdgeSite, error) {
	e, err := scanEdge(r.DB.QueryRowContext(ctx, `SELECT `+edgeCols+` FROM edge_sites WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

// EdgeByToken finds the edge an API token belongs to.
func (r *Repo) EdgeByToken(ctx context.Context, token string) (*EdgeSite, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	e, err := scanEdge(r.DB.QueryRowContext(ctx, `SELECT `+edgeCols+` FROM edge_sites WHERE token_hash = ?`, multisite.Hash(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

// CreateEdgeSite adds a branch office and returns its pairing secret (valid for 24 hours).
func (r *Repo) CreateEdgeSite(ctx context.Context, name string) (*EdgeSite, string, error) {
	name = cleanText(name, 60)
	if name == "" {
		return nil, "", invalid("required", "name")
	}
	secret := multisite.NewSecret()
	res, err := r.DB.ExecContext(ctx, `INSERT INTO edge_sites(name, pair_hash, pair_expires) VALUES (?,?,?)`,
		name, multisite.Hash(secret), time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339))
	if err != nil {
		return nil, "", uniqueErr(err, "name")
	}
	id, _ := res.LastInsertId()
	e, err := r.EdgeSite(ctx, id)
	return e, secret, err
}

// RenewEdgePairing makes a new pairing secret for an edge. The old tunnel stays until the new
// pairing is done, so an edge can be replaced without a gap in the configuration.
func (r *Repo) RenewEdgePairing(ctx context.Context, id int64) (string, error) {
	secret := multisite.NewSecret()
	res, err := r.DB.ExecContext(ctx, `UPDATE edge_sites SET pair_hash = ?, pair_expires = ? WHERE id = ?`,
		multisite.Hash(secret), time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339), id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return secret, nil
}

// RenameEdgeSite changes the name of an edge.
func (r *Repo) RenameEdgeSite(ctx context.Context, id int64, name string) error {
	name = cleanText(name, 60)
	if name == "" {
		return invalid("required", "name")
	}
	return updateRow(ctx, r.DB, `UPDATE edge_sites SET name = ? WHERE id = ?`, []any{name, id}, "name")
}

// DeleteEdgeSite removes an edge; its tunnel is closed on the next apply.
func (r *Repo) DeleteEdgeSite(ctx context.Context, id int64) error {
	return updateRow(ctx, r.DB, `DELETE FROM edge_sites WHERE id = ?`, []any{id}, "")
}

// ErrPairing means the pairing secret is unknown or expired.
var ErrPairing = errors.New("invalid_pairing_code")

// PairEdge completes the pairing of an edge: it stores the key and tunnel address and returns the token.
func (r *Repo) PairEdge(ctx context.Context, secret, publicKey, tunnelIP, hostname, version, lanAddr, lanNet string) (*EdgeSite, string, error) {
	var id int64
	var expires string
	err := r.DB.QueryRowContext(ctx, `SELECT id, pair_expires FROM edge_sites WHERE pair_hash = ?`, multisite.Hash(secret)).Scan(&id, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrPairing
	}
	if err != nil {
		return nil, "", err
	}
	if t, err := time.Parse(time.RFC3339, expires); err != nil || time.Now().After(t) {
		return nil, "", ErrPairing
	}
	token := multisite.NewToken()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.DB.ExecContext(ctx, `UPDATE edge_sites SET pair_hash = '', pair_expires = '', public_key = ?, tunnel_ip = ?,
		token_hash = ?, hostname = ?, version = ?, lan_address = ?, lan_net = ?, paired_at = ?, last_seen = ? WHERE id = ?`,
		publicKey, tunnelIP, multisite.Hash(token), cleanText(hostname, 64), cleanText(version, 40), lanAddr, lanNet, now, now, id); err != nil {
		return nil, "", err
	}
	e, err := r.EdgeSite(ctx, id)
	return e, token, err
}

// EdgeHeartbeat stores what an edge reported.
func (r *Repo) EdgeHeartbeat(ctx context.Context, id int64, hb multisite.EdgeHeartbeat) error {
	svc, _ := json.Marshal(hb.Services)
	_, err := r.DB.ExecContext(ctx, `UPDATE edge_sites SET version = ?, hostname = ?, lan_address = ?, lan_net = ?, services = ?, last_seen = ? WHERE id = ?`,
		cleanText(hb.Version, 40), cleanText(hb.Hostname, 64), cleanText(hb.LANAddress, 64), cleanText(hb.LANNet, 64), string(svc),
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ---- federation ---------------------------------------------------------------

// FedPeer is another Kiestoon system this one is linked with.
type FedPeer struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	// Role: hosted (the other system joined our hub) or joined (we joined theirs).
	Role          string              `json:"role"`
	State         string              `json:"state"`
	InviteExpires string              `json:"inviteExpires"`
	PublicKey     string              `json:"publicKey"`
	TunnelIP      string              `json:"tunnelIp"`
	LocalIP       string              `json:"localIp"`
	PrivateKey    string              `json:"-"`
	Endpoint      string              `json:"endpoint"`
	TokenOut      string              `json:"-"`
	SIPPort       int                 `json:"sipPort"`
	PortalPort    int                 `json:"portalPort"`
	DirectDial    bool                `json:"directDial"`
	Directory     multisite.Directory `json:"directory"`
	Version       string              `json:"version"`
	LastSeen      string              `json:"lastSeen"`
	CreatedAt     string              `json:"createdAt"`
}

// SIPEndpoint is the Asterisk endpoint of the link.
func (p FedPeer) SIPEndpoint() string { return fmt.Sprintf("fed-%d", p.ID) }

// Interface is the WireGuard interface of a joined link (hosted peers use the hub).
func (p FedPeer) Interface() string { return fmt.Sprintf("kt-f%d", p.ID) }

// DeviceState is the custom device state that mirrors the BLF of a remote number.
func (p FedPeer) DeviceState(number string) string {
	return fmt.Sprintf("Custom:ktfed%d-%s", p.ID, number)
}

const fedCols = `id, name, prefix, role, state, invite_expires, public_key, tunnel_ip, local_ip, private_key, endpoint, token_out,
	sip_port, portal_port, direct_dial, directory, version, last_seen, created_at`

func scanPeer(row interface{ Scan(...any) error }) (*FedPeer, error) {
	var p FedPeer
	var dd int
	var dir string
	if err := row.Scan(&p.ID, &p.Name, &p.Prefix, &p.Role, &p.State, &p.InviteExpires, &p.PublicKey, &p.TunnelIP, &p.LocalIP,
		&p.PrivateKey, &p.Endpoint, &p.TokenOut, &p.SIPPort, &p.PortalPort, &dd, &dir, &p.Version, &p.LastSeen, &p.CreatedAt); err != nil {
		return nil, err
	}
	p.DirectDial = dd == 1
	_ = json.Unmarshal([]byte(dir), &p.Directory)
	if p.Directory.Entries == nil {
		p.Directory.Entries = []multisite.DirectoryEntry{}
	}
	return &p, nil
}

// FedPeers lists the linked systems.
func (r *Repo) FedPeers(ctx context.Context) ([]FedPeer, error) {
	return queryAll(ctx, r.DB, `SELECT `+fedCols+` FROM federation_peers ORDER BY name COLLATE NOCASE, id`, scanPeer)
}

// FedPeer returns one linked system.
func (r *Repo) FedPeer(ctx context.Context, id int64) (*FedPeer, error) {
	p, err := scanPeer(r.DB.QueryRowContext(ctx, `SELECT `+fedCols+` FROM federation_peers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// FedPeerByToken finds the active link an incoming API token belongs to.
func (r *Repo) FedPeerByToken(ctx context.Context, token string) (*FedPeer, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	p, err := scanPeer(r.DB.QueryRowContext(ctx, `SELECT `+fedCols+` FROM federation_peers WHERE token_in_hash = ? AND state = 'active'`, multisite.Hash(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

var rePrefix = regexp.MustCompile(`^[1-9][0-9]{0,3}$`)

// LocalNumbers are all internal numbers that callers can dial on this system.
func (r *Repo) LocalNumbers(ctx context.Context) ([]string, error) {
	out := []string{}
	for _, q := range []string{`SELECT number FROM extensions`, `SELECT number FROM ring_groups`, `SELECT number FROM queues`, `SELECT number FROM conferences`} {
		rows, err := r.DB.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				out = append(out, n)
			}
		}
		rows.Close()
	}
	if f, err := LoadFeatureSettings(ctx, r.DB); err == nil && f.ParkingSlots > 0 {
		out = append(out, "70")
		for i := 1; i <= f.ParkingSlots; i++ {
			out = append(out, ParkSlot(i))
		}
	}
	return append(out, Reserved...), nil
}

func overlaps(a, b string) bool { return strings.HasPrefix(a, b) || strings.HasPrefix(b, a) }

// ValidatePrefix checks a dial prefix for a linked system: digits, not starting with 0 (the outside
// line), and no clash with local numbers or the prefixes of other links.
func (r *Repo) ValidatePrefix(ctx context.Context, prefix string, peerID int64) error {
	if !rePrefix.MatchString(prefix) {
		return invalid("invalid", "prefix")
	}
	locals, err := r.LocalNumbers(ctx)
	if err != nil {
		return err
	}
	for _, n := range locals {
		if strings.HasPrefix(n, prefix) || (len(n) < len(prefix) && strings.HasPrefix(prefix, n)) {
			return invalid("prefix_conflict", "prefix")
		}
	}
	peers, err := r.FedPeers(ctx)
	if err != nil {
		return err
	}
	for _, p := range peers {
		if p.ID != peerID && p.Prefix != "" && overlaps(p.Prefix, prefix) {
			return invalid("prefix_conflict", "prefix")
		}
	}
	return nil
}

// CreateInvite adds a link this system hosts and returns the one-time secret (valid for 24 hours).
func (r *Repo) CreateInvite(ctx context.Context, name, prefix string) (*FedPeer, string, error) {
	name = cleanText(name, 60)
	if name == "" {
		return nil, "", invalid("required", "name")
	}
	if err := r.ValidatePrefix(ctx, prefix, 0); err != nil {
		return nil, "", err
	}
	secret := multisite.NewSecret()
	res, err := r.DB.ExecContext(ctx, `INSERT INTO federation_peers(name, prefix, role, state, invite_hash, invite_expires) VALUES (?,?,?,?,?,?)`,
		name, prefix, "hosted", "invited", multisite.Hash(secret), time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339))
	if err != nil {
		return nil, "", err
	}
	id, _ := res.LastInsertId()
	p, err := r.FedPeer(ctx, id)
	return p, secret, err
}

// AcceptInvite completes a hosted link when the other system joins.
func (r *Repo) AcceptInvite(ctx context.Context, secret string, in multisite.FedAcceptRequest, tunnelIP string) (*FedPeer, string, error) {
	var id int64
	var expires string
	err := r.DB.QueryRowContext(ctx, `SELECT id, invite_expires FROM federation_peers WHERE invite_hash = ? AND state = 'invited'`,
		multisite.Hash(secret)).Scan(&id, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrPairing
	}
	if err != nil {
		return nil, "", err
	}
	if t, err := time.Parse(time.RFC3339, expires); err != nil || time.Now().After(t) {
		return nil, "", ErrPairing
	}
	tokenIn := multisite.NewToken()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.DB.ExecContext(ctx, `UPDATE federation_peers SET state = 'active', invite_hash = '', invite_expires = '', public_key = ?,
		tunnel_ip = ?, token_out = ?, token_in_hash = ?, sip_port = ?, portal_port = ?, version = ?, last_seen = ? WHERE id = ?`,
		in.PublicKey, tunnelIP, in.Token, multisite.Hash(tokenIn), in.SIPPort, in.PortalPort, cleanText(in.Version, 40), now, id); err != nil {
		return nil, "", err
	}
	p, err := r.FedPeer(ctx, id)
	return p, tokenIn, err
}

// JoinedPeer is the data of a link this system joined.
type JoinedPeer struct {
	Name, Prefix, PublicKey, TunnelIP, LocalIP, PrivateKey, Endpoint, TokenOut, TokenIn, Version string
	SIPPort, PortalPort                                                                          int
}

// AddJoinedPeer stores a link to another hub.
func (r *Repo) AddJoinedPeer(ctx context.Context, j JoinedPeer) (*FedPeer, error) {
	res, err := r.DB.ExecContext(ctx, `INSERT INTO federation_peers(name, prefix, role, state, public_key, tunnel_ip, local_ip, private_key,
		endpoint, token_out, token_in_hash, sip_port, portal_port, version, last_seen) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		cleanText(j.Name, 60), j.Prefix, "joined", "active", j.PublicKey, j.TunnelIP, j.LocalIP, j.PrivateKey, j.Endpoint, j.TokenOut,
		multisite.Hash(j.TokenIn), j.SIPPort, j.PortalPort, cleanText(j.Version, 40), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return r.FedPeer(ctx, id)
}

// SaveFedPeer changes the editable fields of a link.
func (r *Repo) SaveFedPeer(ctx context.Context, id int64, name, prefix string, directDial bool) error {
	name = cleanText(name, 60)
	if name == "" {
		return invalid("required", "name")
	}
	if err := r.ValidatePrefix(ctx, prefix, id); err != nil {
		return err
	}
	return updateRow(ctx, r.DB, `UPDATE federation_peers SET name = ?, prefix = ?, direct_dial = ? WHERE id = ?`,
		[]any{name, prefix, b2i(directDial), id}, "")
}

// SetPeerDirectory stores the directory a linked system published. It reports whether it changed.
func (r *Repo) SetPeerDirectory(ctx context.Context, id int64, d multisite.Directory) (bool, error) {
	b, _ := json.Marshal(d)
	var old string
	_ = r.DB.QueryRowContext(ctx, `SELECT directory FROM federation_peers WHERE id = ?`, id).Scan(&old)
	_, err := r.DB.ExecContext(ctx, `UPDATE federation_peers SET directory = ?, version = ?, last_seen = ? WHERE id = ?`,
		string(b), cleanText(d.Version, 40), time.Now().UTC().Format(time.RFC3339), id)
	return old != string(b), err
}

// TouchPeer records that a linked system was reachable.
func (r *Repo) TouchPeer(ctx context.Context, id int64) {
	_, _ = r.DB.ExecContext(ctx, `UPDATE federation_peers SET last_seen = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)
}

// DeleteFedPeer removes a link and points destinations at it to "hang up".
func (r *Repo) DeleteFedPeer(ctx context.Context, id int64) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	pfx := strconv.FormatInt(id, 10) + ":"
	if err := replaceDestinations(ctx, tx, func(d *Destination) bool {
		if d.Type == DestRemote && strings.HasPrefix(d.Target, pfx) {
			*d = Destination{Type: DestHangup}
			return true
		}
		return false
	}); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM federation_peers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

var reRemoteNumber = regexp.MustCompile(`^[0-9]{2,10}$`)

// ParseRemote splits a remote destination target.
func ParseRemote(target string) (int64, string, bool) {
	id, num, ok := strings.Cut(target, ":")
	if !ok || !reRemoteNumber.MatchString(num) {
		return 0, "", false
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return n, num, err == nil && n > 0
}

func (r *Repo) validateRemote(ctx context.Context, d *Destination, field string) error {
	id, _, ok := ParseRemote(d.Target)
	if !ok {
		return invalid("invalid", field)
	}
	p, err := r.FedPeer(ctx, id)
	if err != nil || p.State != "active" {
		return invalid("unknown_destination", field)
	}
	return nil
}

// RemoteMember is an agent on a linked system that takes calls of a local queue.
type RemoteMember struct {
	PeerID  int64  `json:"peerId"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Site    string `json:"site"`
	Penalty int    `json:"penalty"`
}

func (r *Repo) remoteMembers(ctx context.Context) (map[int64][]RemoteMember, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT m.queue_id, m.peer_id, m.number, m.penalty, p.name, p.directory FROM queue_remote_members m
		JOIN federation_peers p ON p.id = m.peer_id ORDER BY m.penalty, m.number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]RemoteMember{}
	for rows.Next() {
		var qid int64
		var m RemoteMember
		var dir string
		if err := rows.Scan(&qid, &m.PeerID, &m.Number, &m.Penalty, &m.Site, &dir); err != nil {
			return nil, err
		}
		var d multisite.Directory
		_ = json.Unmarshal([]byte(dir), &d)
		for _, e := range d.Entries {
			if e.Number == m.Number {
				m.Name = e.Name
			}
		}
		out[qid] = append(out[qid], m)
	}
	return out, rows.Err()
}

func validateRemoteMembers(ctx context.Context, r *Repo, ms []RemoteMember) error {
	seen := map[string]bool{}
	for _, m := range ms {
		key := fmt.Sprintf("%d:%s", m.PeerID, m.Number)
		if seen[key] || !reRemoteNumber.MatchString(m.Number) || m.Penalty < 0 || m.Penalty > 10 {
			return invalid("invalid", "remoteMembers")
		}
		seen[key] = true
		p, err := r.FedPeer(ctx, m.PeerID)
		if err != nil || p.State != "active" {
			return invalid("unknown_member", "remoteMembers")
		}
	}
	return nil
}

// ---- follow the sun -------------------------------------------------------------

// SunRule sends calls to a destination while it is working time in a time zone.
type SunRule struct {
	Label       string      `json:"label"`
	Timezone    string      `json:"timezone"`
	Days        []int       `json:"days"` // 1 = Monday … 7 = Sunday
	From        string      `json:"from"`
	To          string      `json:"to"`
	Destination Destination `json:"destination"`
}

// SunRoute tries its rules in order; the first one whose time zone is in working hours wins.
type SunRoute struct {
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	Rules    []SunRule   `json:"rules"`
	Fallback Destination `json:"fallback"`
	Enabled  bool        `json:"enabled"`
}

func scanSun(row interface{ Scan(...any) error }) (*SunRoute, error) {
	var s SunRoute
	var rules string
	var en int
	if err := row.Scan(&s.ID, &s.Name, &rules, &s.Fallback.Type, &s.Fallback.Target, &en); err != nil {
		return nil, err
	}
	s.Enabled = en == 1
	_ = json.Unmarshal([]byte(rules), &s.Rules)
	if s.Rules == nil {
		s.Rules = []SunRule{}
	}
	return &s, nil
}

const sunCols = `id, name, rules, fallback_type, fallback, enabled`

// SunRoutes lists the follow-the-sun routes.
func (r *Repo) SunRoutes(ctx context.Context) ([]SunRoute, error) {
	return queryAll(ctx, r.DB, `SELECT `+sunCols+` FROM sun_routes ORDER BY name COLLATE NOCASE`, scanSun)
}

var reClock = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$|^24:00$`)

// SaveSunRoute validates and stores a follow-the-sun route.
func (r *Repo) SaveSunRoute(ctx context.Context, s *SunRoute) error {
	s.Name = cleanText(s.Name, 60)
	if s.Name == "" {
		return invalid("required", "name")
	}
	if len(s.Rules) == 0 || len(s.Rules) > 12 {
		return invalid("required", "rules")
	}
	for i := range s.Rules {
		rule := &s.Rules[i]
		rule.Label = cleanText(rule.Label, 40)
		if _, err := time.LoadLocation(rule.Timezone); err != nil || rule.Timezone == "" || strings.ContainsAny(rule.Timezone, ",?|") {
			return invalid("invalid_timezone", "rules")
		}
		if !reClock.MatchString(rule.From) || !reClock.MatchString(rule.To) || rule.From >= rule.To {
			return invalid("invalid_time", "rules")
		}
		if len(rule.Days) == 0 {
			return invalid("required", "rules")
		}
		slices.Sort(rule.Days)
		rule.Days = slices.Compact(rule.Days)
		for _, d := range rule.Days {
			if d < 1 || d > 7 {
				return invalid("invalid", "rules")
			}
		}
		if rule.Destination.Type == DestFollowSun {
			return invalid("loop", "rules")
		}
		if err := r.ValidateDestination(ctx, &rule.Destination, "rules"); err != nil {
			return err
		}
	}
	if s.Fallback.Type == DestFollowSun {
		return invalid("loop", "fallback")
	}
	if err := r.ValidateDestination(ctx, &s.Fallback, "fallback"); err != nil {
		return err
	}
	rules, _ := json.Marshal(s.Rules)
	if s.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO sun_routes(name, rules, fallback_type, fallback, enabled) VALUES (?,?,?,?,?)`,
			s.Name, string(rules), s.Fallback.Type, s.Fallback.Target, b2i(s.Enabled))
		if err != nil {
			return err
		}
		s.ID, _ = res.LastInsertId()
		return nil
	}
	return updateRow(ctx, r.DB, `UPDATE sun_routes SET name = ?, rules = ?, fallback_type = ?, fallback = ?, enabled = ? WHERE id = ?`,
		[]any{s.Name, string(rules), s.Fallback.Type, s.Fallback.Target, b2i(s.Enabled), s.ID}, "")
}

// DeleteSunRoute removes a follow-the-sun route.
func (r *Repo) DeleteSunRoute(ctx context.Context, id int64) error {
	return r.deleteWithRefs(ctx, "sun_routes", DestFollowSun, id)
}
