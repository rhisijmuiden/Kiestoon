// Package pgdump reads PostgreSQL custom-format dumps (pg_dump -Fc) without PostgreSQL.
package pgdump

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// A PostgreSQL "custom format" dump (pg_dump -Fc) is what UniFi Talk puts in its backup. This is a
// small reader for it: the table of contents and the COPY data of each table, without a PostgreSQL
// installation. Layout (src/bin/pg_dump/pg_backup_archiver.c and pg_backup_custom.c):
//
//	"PGDMP", version major/minor/revision, int size, offset size, format (1 = custom)
//	compression (1.15+: algorithm byte; 1.4-1.14: level as int), creation time (7 ints),
//	database name, server version, pg_dump version (strings), TOC count, TOC entries,
//	then data blocks: block type, dump id, chunks (length + bytes) until a zero length.
//
// An int is a sign byte followed by intSize little-endian bytes; a string is an int length (-1
// for NULL) followed by the bytes. Table data is the text form of COPY, compressed as one zlib
// stream over the chunks when the dump is compressed.

// Table is the data of one table.
type Table struct {
	Schema  string
	Name    string
	Columns []string
	// Rows map a column to its value; a NULL value is absent.
	Rows []map[string]string
}

// Dump is a read PostgreSQL dump.
type Dump struct {
	Database string
	Tables   map[string]*Table // "schema.name"
}

// Table returns the table, or an empty one when the dump has none by that name.
func (d *Dump) Table(schema, name string) *Table {
	if t, ok := d.Tables[schema+"."+name]; ok {
		return t
	}
	return &Table{Schema: schema, Name: name}
}

const (
	blkData  = 1
	blkBlobs = 3

	compressNone = 0
	compressGzip = 1
)

// maxTableBytes limits the uncompressed data of one table.
const maxTableBytes = 256 << 20

var errDump = errors.New("not a PostgreSQL custom-format dump")

type dumpReader struct {
	r                *bufio.Reader
	vmaj, vmin       int
	intSize, offSize int
	compression      int
}

func (d *dumpReader) byte() (int, error) {
	b, err := d.r.ReadByte()
	return int(b), err
}

func (d *dumpReader) int() (int, error) {
	sign, err := d.byte()
	if err != nil {
		return 0, err
	}
	v := 0
	for i := 0; i < d.intSize; i++ {
		b, err := d.byte()
		if err != nil {
			return 0, err
		}
		if i < 8 {
			v |= b << (8 * i)
		}
	}
	if sign != 0 {
		v = -v
	}
	return v, nil
}

// str returns the string and whether it was not NULL.
func (d *dumpReader) str() (string, bool, error) {
	n, err := d.int()
	if err != nil || n < 0 {
		return "", false, err
	}
	if n > 64<<20 {
		return "", false, errDump
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(d.r, b); err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

func (d *dumpReader) after(maj, min int) bool {
	return d.vmaj > maj || (d.vmaj == maj && d.vmin >= min)
}

type tocEntry struct {
	id                    int
	desc, tag, ns, copyIn string
}

// Read reads a custom-format dump. Only table data is kept.
func Read(r io.Reader) (*Dump, error) {
	d := &dumpReader{r: bufio.NewReaderSize(r, 64<<10)}
	magic := make([]byte, 5)
	if _, err := io.ReadFull(d.r, magic); err != nil || string(magic) != "PGDMP" {
		return nil, errDump
	}
	var hdr [6]int
	for i := range hdr {
		b, err := d.byte()
		if err != nil {
			return nil, errDump
		}
		hdr[i] = b
	}
	d.vmaj, d.vmin, d.intSize, d.offSize = hdr[0], hdr[1], hdr[3], hdr[4]
	if d.vmaj != 1 || d.vmin < 10 || d.intSize < 1 || d.intSize > 8 || d.offSize < 1 || d.offSize > 8 || hdr[5] != 1 {
		return nil, fmt.Errorf("%w (version %d.%d, format %d)", errDump, d.vmaj, d.vmin, hdr[5])
	}
	if d.after(1, 15) {
		c, err := d.byte()
		if err != nil {
			return nil, errDump
		}
		d.compression = c
	} else {
		level, err := d.int()
		if err != nil {
			return nil, errDump
		}
		if level != 0 {
			d.compression = compressGzip
		}
	}
	if d.compression != compressNone && d.compression != compressGzip {
		return nil, fmt.Errorf("unsupported compression %d in PostgreSQL dump", d.compression)
	}
	for range 7 { // creation time
		if _, err := d.int(); err != nil {
			return nil, errDump
		}
	}
	dump := &Dump{Tables: map[string]*Table{}}
	var err error
	if dump.Database, _, err = d.str(); err != nil {
		return nil, errDump
	}
	for range 2 { // server and pg_dump versions
		if _, _, err := d.str(); err != nil {
			return nil, errDump
		}
	}
	count, err := d.int()
	if err != nil || count < 0 || count > 1_000_000 {
		return nil, errDump
	}
	toc := map[int]tocEntry{}
	for range count {
		e, err := d.tocEntry()
		if err != nil {
			return nil, fmt.Errorf("%w: table of contents: %v", errDump, err)
		}
		toc[e.id] = e
	}
	// Data blocks follow the table of contents in dump order.
	for {
		typ, err := d.byte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		id, err := d.int()
		if err != nil {
			return nil, errDump
		}
		switch typ {
		case blkData:
			data, err := d.chunks()
			if err != nil {
				return nil, fmt.Errorf("%w: data of entry %d: %v", errDump, id, err)
			}
			e, ok := toc[id]
			if ok && e.desc == "TABLE DATA" {
				t, err := parseCopy(e, data)
				if err != nil {
					return nil, err
				}
				dump.Tables[t.Schema+"."+t.Name] = t
			}
		case blkBlobs:
			for {
				oid, err := d.int()
				if err != nil {
					return nil, errDump
				}
				if oid == 0 {
					break
				}
				if _, err := d.chunks(); err != nil {
					return nil, errDump
				}
			}
		default:
			return nil, fmt.Errorf("%w: unknown block type %d", errDump, typ)
		}
	}
	return dump, nil
}

func (d *dumpReader) tocEntry() (tocEntry, error) {
	var e tocEntry
	var err error
	if e.id, err = d.int(); err != nil {
		return e, err
	}
	if _, err = d.int(); err != nil { // had dumper
		return e, err
	}
	skip := func() error { _, _, err := d.str(); return err }
	read := func(s *string) error { var err error; *s, _, err = d.str(); return err }
	if d.after(1, 8) {
		if err := skip(); err != nil { // table oid
			return e, err
		}
		if err := skip(); err != nil { // oid
			return e, err
		}
	}
	if err := read(&e.tag); err != nil {
		return e, err
	}
	if err := read(&e.desc); err != nil {
		return e, err
	}
	if d.after(1, 11) {
		if _, err := d.int(); err != nil { // section
			return e, err
		}
	}
	if err := skip(); err != nil { // definition
		return e, err
	}
	if err := skip(); err != nil { // drop statement
		return e, err
	}
	if d.after(1, 3) {
		if err := read(&e.copyIn); err != nil {
			return e, err
		}
	}
	if d.after(1, 6) {
		if err := read(&e.ns); err != nil {
			return e, err
		}
	}
	if d.after(1, 10) {
		if err := skip(); err != nil { // tablespace
			return e, err
		}
	}
	if d.after(1, 14) {
		if err := skip(); err != nil { // table access method
			return e, err
		}
	}
	if d.after(1, 16) {
		if _, err := d.int(); err != nil { // relkind
			return e, err
		}
	}
	if err := skip(); err != nil { // owner
		return e, err
	}
	if d.after(1, 9) {
		if err := skip(); err != nil { // "with oids"
			return e, err
		}
	}
	if d.after(1, 5) {
		for { // dependencies, ended by NULL
			_, ok, err := d.str()
			if err != nil {
				return e, err
			}
			if !ok {
				break
			}
		}
	}
	// Custom format: where the data block is (flag byte + offset).
	if _, err := d.byte(); err != nil {
		return e, err
	}
	for range d.offSize {
		if _, err := d.byte(); err != nil {
			return e, err
		}
	}
	return e, nil
}

// chunks reads the chunks of one data block and returns them uncompressed.
func (d *dumpReader) chunks() ([]byte, error) {
	var raw bytes.Buffer
	for {
		n, err := d.int()
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		if n < 0 || raw.Len()+n > maxTableBytes {
			return nil, errDump
		}
		if _, err := io.CopyN(&raw, d.r, int64(n)); err != nil {
			return nil, err
		}
	}
	if d.compression == compressNone || raw.Len() == 0 {
		return raw.Bytes(), nil
	}
	zr, err := zlib.NewReader(&raw)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, io.LimitReader(zr, maxTableBytes+1)); err != nil {
		return nil, err
	}
	if out.Len() > maxTableBytes {
		return nil, errDump
	}
	return out.Bytes(), nil
}

var reCopy = regexp.MustCompile(`(?s)^COPY\s+(\S+)\s*\((.*?)\)\s+FROM\s+stdin`)

func parseCopy(e tocEntry, data []byte) (*Table, error) {
	t := &Table{Schema: e.ns, Name: e.tag}
	m := reCopy.FindStringSubmatch(e.copyIn)
	if m == nil {
		return t, nil
	}
	for _, c := range strings.Split(m[2], ",") {
		t.Columns = append(t.Columns, strings.Trim(strings.TrimSpace(c), `"`))
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || line == `\.` {
			continue
		}
		fields := strings.Split(line, "\t")
		row := make(map[string]string, len(t.Columns))
		for i, c := range t.Columns {
			if i >= len(fields) || fields[i] == `\N` {
				continue
			}
			row[c] = unescapeCopy(fields[i])
		}
		t.Rows = append(t.Rows, row)
	}
	return t, nil
}

// unescapeCopy undoes the backslash escapes of the COPY text format.
func unescapeCopy(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 == len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch n := s[i]; n {
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case 'x':
			v, j := 0, i+1
			for ; j < len(s) && j < i+3 && isHex(s[j]); j++ {
				v = v*16 + hexVal(s[j])
			}
			if j == i+1 {
				b.WriteByte('x')
				continue
			}
			b.WriteByte(byte(v))
			i = j - 1
		default:
			if n >= '0' && n <= '7' {
				v, j := 0, i
				for ; j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7'; j++ {
					v = v*8 + int(s[j]-'0')
				}
				b.WriteByte(byte(v))
				i = j - 1
				continue
			}
			b.WriteByte(n)
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexVal(c byte) int {
	switch {
	case c >= 'a':
		return int(c-'a') + 10
	case c >= 'A':
		return int(c-'A') + 10
	}
	return int(c - '0')
}
