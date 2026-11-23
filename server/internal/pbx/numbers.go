package pbx

import (
	"context"
	"regexp"
)

// MaxTrunkNumbers caps the numbers on one trunk, so a typo in a number block (a count of
// 1000000) cannot fill the database.
const MaxTrunkNumbers = 10000

var numberRe = regexp.MustCompile(`^\+?[0-9]{3,20}$`)

// TrunkNumber is one phone number a trunk carries.
type TrunkNumber struct {
	Number  string `json:"number"`
	TrunkID int64  `json:"trunkId"`
}

// NumberInfo is a trunk number with the trunk and the inbound route that answers it, for the
// number pickers in the UI.
type NumberInfo struct {
	Number          string `json:"number"`
	TrunkID         int64  `json:"trunkId"`
	TrunkName       string `json:"trunkName"`
	RouteID         int64  `json:"routeId,omitempty"`
	RouteName       string `json:"routeName,omitempty"`
	DestinationType string `json:"destinationType,omitempty"`
	Destination     string `json:"destination,omitempty"`
}

func (r *Repo) trunkNumbers(ctx context.Context) ([]TrunkNumber, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT number, trunk_id FROM trunk_numbers ORDER BY trunk_id, position, number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrunkNumber{}
	for rows.Next() {
		var n TrunkNumber
		if err := rows.Scan(&n.Number, &n.TrunkID); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CleanNumbers normalizes, checks and de-duplicates a list of phone numbers, keeping the order.
func CleanNumbers(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		n := NormalizeDID(raw)
		if n == "" || seen[n] {
			continue
		}
		if !numberRe.MatchString(n) {
			return nil, invalid("invalid", "numbers")
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) > MaxTrunkNumbers {
		return nil, invalid("too_large", "numbers")
	}
	return out, nil
}

// saveTrunkNumbers replaces the numbers of t. A nil list keeps the stored numbers. A number that
// already belongs to another trunk is refused.
func (r *Repo) saveTrunkNumbers(ctx context.Context, t *Trunk) error {
	if t.Numbers == nil {
		return nil
	}
	nums, err := CleanNumbers(t.Numbers)
	if err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM trunk_numbers WHERE trunk_id = ?`, t.ID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO trunk_numbers(number, trunk_id, position) VALUES (?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, n := range nums {
		if _, err := stmt.ExecContext(ctx, n, t.ID, i); err != nil {
			return uniqueErr(err, "numbers")
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	t.Numbers = nums
	return nil
}

// Numbers returns every trunk number with its trunk and the inbound route that answers it.
func (r *Repo) Numbers(ctx context.Context) ([]NumberInfo, error) {
	nums, err := r.trunkNumbers(ctx)
	if err != nil {
		return nil, err
	}
	trunks := map[int64]string{}
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name FROM trunks`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		trunks[id] = name
	}
	rows.Close()
	routes, err := r.InboundRoutes(ctx)
	if err != nil {
		return nil, err
	}
	byDID := map[string]InboundRoute{}
	for _, rt := range routes {
		if rt.DID != "" {
			byDID[rt.DID] = rt
		}
	}
	out := make([]NumberInfo, 0, len(nums))
	for _, n := range nums {
		ni := NumberInfo{Number: n.Number, TrunkID: n.TrunkID, TrunkName: trunks[n.TrunkID]}
		if rt, ok := byDID[n.Number]; ok {
			ni.RouteID, ni.RouteName, ni.DestinationType, ni.Destination = rt.ID, rt.Name, string(rt.DestinationType), rt.Destination
		}
		out = append(out, ni)
	}
	return out, nil
}
