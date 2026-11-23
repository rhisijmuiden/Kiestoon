package pgdump

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

var rePlainCopy = regexp.MustCompile(`^COPY\s+(?:"?([A-Za-z0-9_]+)"?\.)?"?([A-Za-z0-9_]+)"?\s*\(([^)]*)\)\s+FROM\s+stdin;`)

// ReadPlain reads the table data of a plain-text dump (pg_dump without -Fc): the COPY ... FROM
// stdin blocks. INSERT statements (pg_dump --inserts) are not read.
func ReadPlain(r io.Reader) (*Dump, error) {
	d := &Dump{Tables: map[string]*Table{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var cur *Table
	var size int
	for sc.Scan() {
		line := sc.Text()
		if cur == nil {
			m := rePlainCopy.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			schema := m[1]
			if schema == "" {
				schema = "public"
			}
			cur = &Table{Schema: schema, Name: m[2]}
			for _, c := range strings.Split(m[3], ",") {
				cur.Columns = append(cur.Columns, strings.Trim(strings.TrimSpace(c), `"`))
			}
			size = 0
			continue
		}
		if line == `\.` {
			d.Tables[cur.Schema+"."+cur.Name] = cur
			cur = nil
			continue
		}
		size += len(line)
		if size > maxTableBytes {
			continue
		}
		fields := strings.Split(line, "\t")
		row := make(map[string]string, len(cur.Columns))
		for i, c := range cur.Columns {
			if i >= len(fields) || fields[i] == `\N` {
				continue
			}
			row[c] = unescapeCopy(fields[i])
		}
		cur.Rows = append(cur.Rows, row)
	}
	return d, sc.Err()
}
