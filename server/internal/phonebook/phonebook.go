// Package phonebook serves the central phone book to phones: as the XML phone book formats of the
// vendors, as CSV for import and export, and as a read-only LDAP directory.
package phonebook

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/pbx"
)

func x(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s)))
	return b.String()
}

func label(e pbx.Entry) string {
	if e.Company != "" && !e.Extension {
		return e.Name + " (" + e.Company + ")"
	}
	return e.Name
}

// XML renders the phone book in the format of a vendor.
func XML(vendor string, entries []pbx.Entry) (string, bool) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	switch vendor {
	case "yealink", "snom", "fanvil":
		root := map[string]string{"yealink": "YealinkIPPhoneDirectory", "snom": "SnomIPPhoneDirectory", "fanvil": "PhoneBook"}[vendor]
		fmt.Fprintf(&b, "<%s>\n", root)
		if vendor == "snom" {
			b.WriteString("  <Title>Kiestoon</Title>\n")
		}
		for _, e := range entries {
			if vendor == "fanvil" {
				fmt.Fprintf(&b, "  <DirectoryEntry>\n    <Name>%s</Name>\n", x(label(e)))
				for _, n := range [][2]string{{"Telephone", e.Work}, {"Mobile", e.Mobile}, {"Other", e.Other}} {
					if n[1] != "" {
						fmt.Fprintf(&b, "    <%s>%s</%s>\n", n[0], x(n[1]), n[0])
					}
				}
				b.WriteString("  </DirectoryEntry>\n")
				continue
			}
			for _, n := range []string{e.Work, e.Mobile, e.Other} {
				if n != "" {
					// One entry per number: the phones show the name next to each.
					fmt.Fprintf(&b, "  <DirectoryEntry>\n    <Name>%s</Name>\n    <Telephone>%s</Telephone>\n  </DirectoryEntry>\n", x(label(e)), x(n))
				}
			}
		}
		fmt.Fprintf(&b, "</%s>\n", root)
	case "grandstream":
		b.WriteString("<AddressBook>\n")
		for _, e := range entries {
			first, last := e.Name, ""
			if i := strings.LastIndex(e.Name, " "); i > 0 {
				first, last = e.Name[:i], e.Name[i+1:]
			}
			fmt.Fprintf(&b, "  <Contact>\n    <FirstName>%s</FirstName>\n    <LastName>%s</LastName>\n", x(first), x(last))
			if e.Company != "" {
				fmt.Fprintf(&b, "    <Company>%s</Company>\n", x(e.Company))
			}
			for _, n := range [][2]string{{"Work", e.Work}, {"Cell", e.Mobile}, {"Home", e.Other}} {
				if n[1] != "" {
					fmt.Fprintf(&b, "    <Phone type=\"%s\">\n      <phonenumber>%s</phonenumber>\n      <accountindex>1</accountindex>\n    </Phone>\n", n[0], x(n[1]))
				}
			}
			b.WriteString("  </Contact>\n")
		}
		b.WriteString("</AddressBook>\n")
	default:
		return "", false
	}
	return b.String(), true
}

// CSVHeader is the column order of export and import.
var CSVHeader = []string{"name", "company", "work", "mobile", "other", "email", "notes"}

// WriteCSV exports contacts.
func WriteCSV(w io.Writer, cs []pbx.Contact) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(CSVHeader); err != nil {
		return err
	}
	for _, c := range cs {
		row := []string{c.Name, c.Company, c.Work, c.Mobile, c.Other, c.Email, c.Notes}
		for i, v := range row {
			// Spreadsheet programs run cells that start with these as formulas.
			if v != "" && strings.ContainsRune("=+-@", rune(v[0])) && i != 2 && i != 3 && i != 4 {
				row[i] = "'" + v
			}
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadCSV imports contacts. The first row may be a header (names as in CSVHeader, in any order,
// also Dutch: naam, bedrijf, werk, mobiel, overig); without one the CSVHeader order is assumed.
// Both comma and semicolon separators are accepted.
func ReadCSV(text string) ([]pbx.Contact, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	first, _, _ := strings.Cut(text, "\n")
	cr := csv.NewReader(strings.NewReader(text))
	if strings.Count(first, ";") > strings.Count(first, ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []pbx.Contact{}, nil
	}
	alias := map[string]string{"naam": "name", "bedrijf": "company", "werk": "work", "telefoon": "work", "phone": "work",
		"mobiel": "mobile", "overig": "other", "e-mail": "email", "mail": "email", "notities": "notes", "opmerkingen": "notes"}
	col := map[string]int{}
	for i, h := range rows[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		if a, ok := alias[h]; ok {
			h = a
		}
		for _, k := range CSVHeader {
			if h == k {
				col[k] = i
			}
		}
	}
	start := 1
	if _, ok := col["name"]; !ok {
		start = 0
		for i, k := range CSVHeader {
			col[k] = i
		}
	}
	out := []pbx.Contact{}
	for _, r := range rows[start:] {
		get := func(k string) string {
			if i, ok := col[k]; ok && i < len(r) {
				return strings.TrimPrefix(strings.TrimSpace(r[i]), "'")
			}
			return ""
		}
		out = append(out, pbx.Contact{Name: get("name"), Company: get("company"), Work: get("work"), Mobile: get("mobile"),
			Other: get("other"), Email: get("email"), Notes: get("notes")})
	}
	return out, nil
}
