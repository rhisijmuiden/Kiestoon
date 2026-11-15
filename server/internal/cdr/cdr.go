// Package cdr stores call detail records that Asterisk sends over AMI (cdr_manager) and searches them.
package cdr

import (
	"context"
	"database/sql"
	"encoding/csv"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Record is one call detail record.
type Record struct {
	ID          int64  `json:"id"`
	UniqueID    string `json:"uniqueId"`
	LinkedID    string `json:"linkedId"`
	Start       string `json:"start"`
	Answer      string `json:"answer"`
	End         string `json:"end"`
	Duration    int    `json:"duration"`
	BillSec     int    `json:"billsec"`
	Src         string `json:"src"`
	Dst         string `json:"dst"`
	CLID        string `json:"clid"`
	SrcChannel  string `json:"srcChannel"`
	DstChannel  string `json:"dstChannel"`
	DContext    string `json:"dcontext"`
	LastApp     string `json:"lastApp"`
	Disposition string `json:"disposition"`
	Direction   string `json:"direction"`
	Trunk       string `json:"trunk"`
}

// Store reads and writes CDRs.
type Store struct{ DB *sql.DB }

var reTrunk = regexp.MustCompile(`^PJSIP/trunk-\d+-`)

func parseTime(s string, loc *time.Location) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// FromEvent turns a cdr_manager "Cdr" event into a Record.
func FromEvent(get func(string) string, loc *time.Location) Record {
	r := Record{
		UniqueID: get("UniqueID"), LinkedID: get("LinkedID"), Src: get("Source"), Dst: get("Destination"),
		CLID: get("CallerID"), SrcChannel: get("Channel"), DstChannel: get("DestinationChannel"),
		DContext: get("DestinationContext"), LastApp: get("LastApplication"), Disposition: get("Disposition"),
		Trunk: get("Trunk"),
	}
	r.Start = parseTime(get("StartTime"), loc)
	if r.Start == "" {
		r.Start = time.Now().UTC().Format(time.RFC3339)
	}
	r.Answer = parseTime(get("AnswerTime"), loc)
	r.End = parseTime(get("EndTime"), loc)
	r.Duration, _ = strconv.Atoi(get("Duration"))
	r.BillSec, _ = strconv.Atoi(get("BillableSeconds"))
	switch {
	case reTrunk.MatchString(r.SrcChannel):
		r.Direction = "inbound"
	case reTrunk.MatchString(r.DstChannel) || r.Trunk != "":
		r.Direction = "outbound"
	default:
		r.Direction = "internal"
	}
	return r
}

// Insert stores a record.
func (s *Store) Insert(ctx context.Context, r Record) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cdr(uniqueid, linkedid, start_at, answer_at, end_at, duration, billsec, src, dst,
		clid, src_channel, dst_channel, dcontext, last_app, disposition, direction, trunk)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.UniqueID, r.LinkedID, r.Start, r.Answer, r.End, r.Duration, r.BillSec, r.Src, r.Dst, r.CLID, r.SrcChannel,
		r.DstChannel, r.DContext, r.LastApp, r.Disposition, r.Direction, r.Trunk)
	return err
}

// Filter narrows a search.
type Filter struct {
	From, To    string // RFC3339 or YYYY-MM-DD
	Query       string // number or name
	Direction   string
	Disposition string
	// Ext limits the records to calls to or from one extension (user portal).
	Ext string
	// LinkedID limits the records to one call (all its legs).
	LinkedID string
	Limit    int
	Offset   int
}

func (f Filter) where() (string, []any) {
	conds := []string{"1=1"}
	args := []any{}
	if f.From != "" {
		conds = append(conds, "start_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		to := f.To
		if len(to) == 10 {
			to += "T23:59:59Z"
		}
		conds = append(conds, "start_at <= ?")
		args = append(args, to)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + strings.NewReplacer("%", "", "_", "").Replace(q) + "%"
		conds = append(conds, "(src LIKE ? OR dst LIKE ? OR clid LIKE ?)")
		args = append(args, like, like, like)
	}
	if f.Direction != "" {
		conds = append(conds, "direction = ?")
		args = append(args, f.Direction)
	}
	if f.Disposition != "" {
		conds = append(conds, "disposition = ?")
		args = append(args, f.Disposition)
	}
	if f.LinkedID != "" {
		conds = append(conds, "(linkedid = ? OR (linkedid = '' AND uniqueid = ?))")
		args = append(args, f.LinkedID, f.LinkedID)
	}
	if f.Ext != "" {
		conds = append(conds, "(src = ? OR dst = ? OR src_channel LIKE ? OR dst_channel LIKE ? OR src_channel LIKE ? OR dst_channel LIKE ?)")
		ch, web := "PJSIP/"+f.Ext+"-%", "PJSIP/"+f.Ext+"-web-%"
		args = append(args, f.Ext, f.Ext, ch, ch, web, web)
	}
	return strings.Join(conds, " AND "), args
}

const cols = `id, uniqueid, linkedid, start_at, answer_at, end_at, duration, billsec, src, dst, clid, src_channel,
	dst_channel, dcontext, last_app, disposition, direction, trunk`

// Search returns matching records, newest first, and the total count.
func (s *Store) Search(ctx context.Context, f Filter) ([]Record, int, error) {
	where, args := f.where()
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM cdr WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+cols+` FROM cdr WHERE `+where+` ORDER BY start_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.UniqueID, &r.LinkedID, &r.Start, &r.Answer, &r.End, &r.Duration, &r.BillSec, &r.Src,
			&r.Dst, &r.CLID, &r.SrcChannel, &r.DstChannel, &r.DContext, &r.LastApp, &r.Disposition, &r.Direction, &r.Trunk); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// csvSafe stops spreadsheet programs from running a cell as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ExportCSV writes every matching record (up to 100 000) as CSV.
func (s *Store) ExportCSV(ctx context.Context, f Filter, w io.Writer) error {
	where, args := f.where()
	rows, err := s.DB.QueryContext(ctx, `SELECT `+cols+` FROM cdr WHERE `+where+` ORDER BY start_at DESC LIMIT 100000`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"start", "answer", "end", "duration", "billsec", "direction", "source", "destination", "caller_id",
		"disposition", "trunk", "source_channel", "destination_channel", "unique_id", "linked_id"})
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.UniqueID, &r.LinkedID, &r.Start, &r.Answer, &r.End, &r.Duration, &r.BillSec, &r.Src,
			&r.Dst, &r.CLID, &r.SrcChannel, &r.DstChannel, &r.DContext, &r.LastApp, &r.Disposition, &r.Direction, &r.Trunk); err != nil {
			return err
		}
		rec := []string{r.Start, r.Answer, r.End, strconv.Itoa(r.Duration), strconv.Itoa(r.BillSec), r.Direction, r.Src, r.Dst,
			r.CLID, r.Disposition, r.Trunk, r.SrcChannel, r.DstChannel, r.UniqueID, r.LinkedID}
		for i := range rec {
			rec[i] = csvSafe(rec[i])
		}
		_ = cw.Write(rec)
	}
	cw.Flush()
	return cw.Error()
}

// Stats is shown on the dashboard.
type Stats struct {
	Today    int `json:"today"`
	Answered int `json:"answered"`
	Missed   int `json:"missed"`
	Inbound  int `json:"inbound"`
	Outbound int `json:"outbound"`
}

// TodayStats counts today's calls (in the server's time zone).
func (s *Store) TodayStats(ctx context.Context) Stats {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)
	var st Stats
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT COALESCE(NULLIF(linkedid,''), uniqueid)),
		COUNT(DISTINCT CASE WHEN disposition = 'ANSWERED' THEN COALESCE(NULLIF(linkedid,''), uniqueid) END),
		COUNT(DISTINCT CASE WHEN direction = 'inbound' THEN COALESCE(NULLIF(linkedid,''), uniqueid) END),
		COUNT(DISTINCT CASE WHEN direction = 'outbound' THEN COALESCE(NULLIF(linkedid,''), uniqueid) END)
		FROM cdr WHERE start_at >= ?`, start).Scan(&st.Today, &st.Answered, &st.Inbound, &st.Outbound)
	st.Missed = st.Today - st.Answered
	return st
}
