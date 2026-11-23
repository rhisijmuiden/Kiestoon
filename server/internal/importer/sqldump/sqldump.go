// Package sqldump reads the rows of a MySQL/MariaDB dump (mysqldump output): the INSERT
// statements, with the column names from CREATE TABLE or the INSERT itself. Values are returned
// as strings; NULL becomes "".
package sqldump

import (
	"bufio"
	"io"
	"strings"
)

// Tables are rows by table name.
type Tables map[string][]map[string]string

// Read parses a dump. Statements it does not understand are skipped.
func Read(r io.Reader, keep func(table string) bool) (Tables, error) {
	out := Tables{}
	cols := map[string][]string{}
	br := bufio.NewReaderSize(r, 1<<20)
	var create string
	var createCols []string
	for {
		stmt, err := readStatement(br)
		if s := strings.TrimSpace(stmt); s != "" {
			upper := strings.ToUpper(s[:min(len(s), 32)])
			switch {
			case strings.HasPrefix(upper, "CREATE TABLE"):
				create, createCols = parseCreate(s)
				if create != "" {
					cols[create] = createCols
				}
			case strings.HasPrefix(upper, "INSERT INTO"), strings.HasPrefix(upper, "INSERT IGNORE INTO"), strings.HasPrefix(upper, "REPLACE INTO"):
				parseInsert(s, cols, keep, out)
			}
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

// readStatement reads up to the next ";" outside quotes and comments.
func readStatement(br *bufio.Reader) (string, error) {
	var b strings.Builder
	var quote byte
	lineStart := true
	for {
		c, err := br.ReadByte()
		if err != nil {
			return b.String(), err
		}
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' {
				n, err := br.ReadByte()
				if err != nil {
					return b.String(), err
				}
				b.WriteByte(n)
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if lineStart && (c == '-' || c == '#' || c == '/') {
			// "-- comment", "# comment" and "/*!40101 ... */;" lines
			peek, _ := br.Peek(1)
			if c == '#' || (c == '-' && len(peek) == 1 && peek[0] == '-') || (c == '/' && len(peek) == 1 && peek[0] == '*') {
				line, err := br.ReadString('\n')
				if c == '/' && !strings.Contains(line, "*/") {
					// multi-line comment
					for err == nil && !strings.Contains(line, "*/") {
						line, err = br.ReadString('\n')
					}
				}
				if err != nil {
					return b.String(), err
				}
				lineStart = true
				continue
			}
		}
		lineStart = c == '\n'
		switch c {
		case '\'', '"', '`':
			quote = c
		case ';':
			return b.String(), nil
		}
		b.WriteByte(c)
	}
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`\"")
	if i := strings.LastIndex(s, "`.`"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// parseCreate returns the table name and its column names.
func parseCreate(s string) (string, []string) {
	open := strings.IndexByte(s, '(')
	if open < 0 {
		return "", nil
	}
	head := strings.Fields(s[:open])
	if len(head) < 3 {
		return "", nil
	}
	name := unquoteIdent(head[len(head)-1])
	var cols []string
	for _, line := range strings.Split(s[open+1:], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "`") {
			if end := strings.Index(line[1:], "`"); end > 0 {
				cols = append(cols, line[1:end+1])
			}
		}
	}
	return name, cols
}

func parseInsert(s string, cols map[string][]string, keep func(string) bool, out Tables) {
	upper := strings.ToUpper(s)
	into := strings.Index(upper, "INTO")
	values := strings.Index(upper, "VALUES")
	if into < 0 || values < 0 || values < into {
		return
	}
	head := strings.TrimSpace(s[into+4 : values])
	var names []string
	if i := strings.IndexByte(head, '('); i >= 0 {
		for _, c := range strings.Split(strings.Trim(head[i:], "() "), ",") {
			names = append(names, unquoteIdent(c))
		}
		head = head[:i]
	}
	table := unquoteIdent(head)
	if keep != nil && !keep(table) {
		return
	}
	if names == nil {
		names = cols[table]
	}
	body := s[values+6:]
	for _, tuple := range splitTuples(body) {
		row := map[string]string{}
		for i, v := range tuple {
			if i < len(names) {
				row[names[i]] = v
			}
		}
		out[table] = append(out[table], row)
	}
}

// splitTuples parses "(1,'a',NULL),(2,'b\'c',3)".
func splitTuples(s string) [][]string {
	var out [][]string
	var cur []string
	var val strings.Builder
	in, quoted, wasQuoted := false, false, false
	flush := func() {
		v := val.String()
		if !wasQuoted {
			v = strings.TrimSpace(v)
			if strings.EqualFold(v, "NULL") {
				v = ""
			}
		}
		cur = append(cur, v)
		val.Reset()
		wasQuoted = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quoted {
			switch c {
			case '\\':
				if i+1 < len(s) {
					i++
					switch s[i] {
					case 'n':
						val.WriteByte('\n')
					case 'r':
						val.WriteByte('\r')
					case 't':
						val.WriteByte('\t')
					case '0':
						val.WriteByte(0)
					case 'Z':
						val.WriteByte(26)
					default:
						val.WriteByte(s[i])
					}
				}
			case '\'':
				if i+1 < len(s) && s[i+1] == '\'' {
					val.WriteByte('\'')
					i++
				} else {
					quoted = false
				}
			default:
				val.WriteByte(c)
			}
			continue
		}
		switch c {
		case '(':
			if !in {
				in, cur = true, nil
				continue
			}
			val.WriteByte(c)
		case ')':
			if in {
				flush()
				out = append(out, cur)
				in = false
				continue
			}
		case '\'':
			if in {
				quoted, wasQuoted = true, true
			}
		case ',':
			if in {
				flush()
			}
		default:
			if in {
				val.WriteByte(c)
			}
		}
	}
	return out
}
