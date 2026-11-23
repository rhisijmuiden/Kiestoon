package pgdump

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
)

// Write writes tables as a compressed custom-format dump (format 1.15). It is meant for tests and
// fixtures of the importers: real dumps come from pg_dump.
func Write(out io.Writer, database string, tables []*Table) error {
	var b bytes.Buffer
	num := func(v int) {
		sign := byte(0)
		if v < 0 {
			sign, v = 1, -v
		}
		b.WriteByte(sign)
		for i := 0; i < 4; i++ {
			b.WriteByte(byte(v >> (8 * i)))
		}
	}
	str := func(s string) { num(len(s)); b.WriteString(s) }
	b.WriteString("PGDMP")
	b.Write([]byte{1, 15, 0, 4, 8, 1, compressGzip})
	for range 7 {
		num(0)
	}
	str(database)
	str("16")
	str("16")
	num(len(tables))
	for i, t := range tables {
		cols := make([]string, len(t.Columns))
		for j, c := range t.Columns {
			cols[j] = `"` + c + `"`
		}
		num(i + 1)
		num(1)
		str("0")
		str("0")
		str(t.Name)
		str("TABLE DATA")
		num(3)
		str("")
		str("")
		str(fmt.Sprintf("COPY %s.%q (%s) FROM stdin;\n", t.Schema, t.Name, strings.Join(cols, ", ")))
		str(t.Schema)
		num(-1)
		num(-1)
		str("postgres")
		str("false")
		num(-1)
		b.WriteByte(1)
		b.Write(make([]byte, 8))
	}
	esc := strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`)
	for i, t := range tables {
		var data bytes.Buffer
		for _, r := range t.Rows {
			for j, c := range t.Columns {
				if j > 0 {
					data.WriteByte('\t')
				}
				if v, ok := r[c]; ok {
					data.WriteString(esc.Replace(v))
				} else {
					data.WriteString(`\N`)
				}
			}
			data.WriteByte('\n')
		}
		data.WriteString("\\.\n\n")
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(data.Bytes())
		zw.Close()
		b.WriteByte(blkData)
		num(i + 1)
		num(z.Len())
		b.Write(z.Bytes())
		num(0)
	}
	_, err := out.Write(b.Bytes())
	return err
}
