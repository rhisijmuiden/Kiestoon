package pgdump

import (
	"bytes"
	"compress/zlib"
	"testing"
)

// writer builds a small custom-format dump (version 1.15, like pg_dump 16).
type writer struct {
	b bytes.Buffer
}

func (w *writer) int(v int) {
	sign := byte(0)
	if v < 0 {
		sign, v = 1, -v
	}
	w.b.WriteByte(sign)
	for i := 0; i < 4; i++ {
		w.b.WriteByte(byte(v >> (8 * i)))
	}
}

func (w *writer) str(s string) { w.int(len(s)); w.b.WriteString(s) }
func (w *writer) null()        { w.int(-1) }

type entry struct {
	id            int
	desc, tag, ns string
	copyIn, data  string
}

func build(t *testing.T, compress bool, entries []entry) []byte {
	t.Helper()
	w := &writer{}
	w.b.WriteString("PGDMP")
	c := byte(0)
	if compress {
		c = 1
	}
	w.b.Write([]byte{1, 15, 0, 4, 8, 1, c})
	for _, v := range []int{1, 2, 3, 4, 5, 124, 0} {
		w.int(v)
	}
	w.str("unifi-talk")
	w.str("14.24")
	w.str("16.15")
	w.int(len(entries))
	for _, e := range entries {
		w.int(e.id)
		w.int(1)
		w.str("0")
		w.str("0")
		w.str(e.tag)
		w.str(e.desc)
		w.int(3) // section
		w.str("")
		w.str("")
		w.str(e.copyIn)
		w.str(e.ns)
		w.null() // tablespace
		w.null() // table access method
		w.str("talk")
		w.str("false")
		w.str("12") // one dependency
		w.null()
		w.b.WriteByte(1) // offset not set
		w.b.Write(make([]byte, 8))
	}
	for _, e := range entries {
		if e.desc != "TABLE DATA" {
			continue
		}
		w.b.WriteByte(1)
		w.int(e.id)
		data := []byte(e.data)
		if compress {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			zw.Write(data)
			zw.Close()
			data = z.Bytes()
		}
		// Two chunks, like pg_dump writes for larger tables.
		half := len(data) / 2
		for _, ch := range [][]byte{data[:half], data[half:]} {
			if len(ch) == 0 {
				continue
			}
			w.int(len(ch))
			w.b.Write(ch)
		}
		w.int(0)
	}
	return w.b.Bytes()
}

func TestRead(t *testing.T) {
	for _, compress := range []bool{false, true} {
		dump := build(t, compress, []entry{
			{id: 1, desc: "TABLE", tag: "user", ns: "public"},
			{id: 2, desc: "TABLE DATA", tag: "user", ns: "public", copyIn: `COPY public."user" (id, ext, "name", data) FROM stdin;` + "\n",
				data: "1\t0001\tReinder\t{\"a\": \"x\\\\ny\"}\n2\t0002\t\\N\tline\\tone\\nnext \\101\\x42\n\\.\n\n"},
			{id: 3, desc: "TABLE DATA", tag: "tiers", ns: "fsw", copyIn: "COPY fsw.tiers (queue, agent) FROM stdin;\n", data: "1\t1_abc\n"},
		})
		d, err := Read(bytes.NewReader(dump))
		if err != nil {
			t.Fatalf("compress=%v: %v", compress, err)
		}
		if d.Database != "unifi-talk" {
			t.Errorf("database %q", d.Database)
		}
		u := d.Table("public", "user")
		if len(u.Rows) != 2 || len(u.Columns) != 4 || u.Columns[2] != "name" {
			t.Fatalf("user table: %+v", u)
		}
		if u.Rows[0]["data"] != `{"a": "x\ny"}` {
			t.Errorf("json unescape: %q", u.Rows[0]["data"])
		}
		if _, ok := u.Rows[1]["name"]; ok {
			t.Error("NULL should be absent")
		}
		if u.Rows[1]["data"] != "line\tone\nnext AB" {
			t.Errorf("escapes: %q", u.Rows[1]["data"])
		}
		if r := d.Table("fsw", "tiers").Rows; len(r) != 1 || r[0]["agent"] != "1_abc" {
			t.Errorf("tiers: %v", r)
		}
		if len(d.Table("public", "missing").Rows) != 0 {
			t.Error("missing table should be empty")
		}
	}
	if _, err := Read(bytes.NewReader([]byte("not a dump"))); err == nil {
		t.Error("garbage accepted")
	}
}

func TestWriteRoundTrip(t *testing.T) {
	in := []*Table{{Schema: "public", Name: "swb_item", Columns: []string{"id", "parent", "data"},
		Rows: []map[string]string{{"id": "1", "data": "{\"t\": \"a\\tb\"}"}, {"id": "2", "parent": "1", "data": "x\ny\\z"}}}}
	var b bytes.Buffer
	if err := Write(&b, "db", in); err != nil {
		t.Fatal(err)
	}
	d, err := Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	rows := d.Table("public", "swb_item").Rows
	if len(rows) != 2 || rows[0]["data"] != in[0].Rows[0]["data"] || rows[1]["data"] != "x\ny\\z" || rows[1]["parent"] != "1" {
		t.Fatalf("round trip: %v", rows)
	}
	if _, ok := rows[0]["parent"]; ok {
		t.Error("NULL lost")
	}
}
