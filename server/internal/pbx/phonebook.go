package pbx

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/rhisijmuiden/kiestoon/server/internal/store"
)

// Contact is an entry of the central phone book.
type Contact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Company string `json:"company"`
	Work    string `json:"work"`
	Mobile  string `json:"mobile"`
	Other   string `json:"other"`
	Email   string `json:"email"`
	Notes   string `json:"notes"`
}

// PhonebookKey is the settings row with PhonebookSettings as JSON.
const PhonebookKey = "phonebook"

// PhonebookSettings control how the phone book reaches phones.
type PhonebookSettings struct {
	// IncludeExtensions adds every extension to the phone book phones see.
	IncludeExtensions bool `json:"includeExtensions"`
	// CallerIDLookup shows the phone book name of inbound callers.
	CallerIDLookup bool `json:"callerIdLookup"`
	// XML phone book for phones, fetched from the provisioning URL.
	XMLEnabled bool `json:"xmlEnabled"`
	// LDAP directory for phones (read-only).
	LDAPEnabled bool `json:"ldapEnabled"`
	LDAPPort    int  `json:"ldapPort"`
	// LDAPSPort is LDAP over TLS (0: off).
	LDAPSPort int `json:"ldapsPort"`
	// LDAPAuth requires phones to bind with the provisioning user name and password.
	LDAPAuth bool `json:"ldapAuth"`
}

// DefaultPhonebook is the configuration of a fresh install.
func DefaultPhonebook() PhonebookSettings {
	return PhonebookSettings{IncludeExtensions: true, CallerIDLookup: true, XMLEnabled: true, LDAPPort: 389, LDAPSPort: 636, LDAPAuth: true}
}

// LoadPhonebookSettings reads the phone book settings.
func LoadPhonebookSettings(ctx context.Context, db *sql.DB) (PhonebookSettings, error) {
	s := DefaultPhonebook()
	err := store.GetJSON(ctx, db, PhonebookKey, &s)
	return s, err
}

// Validate checks the settings.
func (s *PhonebookSettings) Validate() error {
	if s.LDAPPort < 1 || s.LDAPPort > 65535 || s.LDAPSPort < 0 || s.LDAPSPort > 65535 || s.LDAPPort == s.LDAPSPort {
		return errors.New("ldapPort")
	}
	return nil
}

// Ports returns the LDAP ports to open.
func (s PhonebookSettings) Ports() []int {
	if !s.LDAPEnabled {
		return nil
	}
	p := []int{s.LDAPPort}
	if s.LDAPSPort > 0 {
		p = append(p, s.LDAPSPort)
	}
	return p
}

var reNumber = regexp.MustCompile(`^\+?[0-9]{2,20}$`)

// CleanNumber removes spaces, dashes, dots and brackets from a phone number.
func CleanNumber(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')', '/', ' ':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

func (c *Contact) validate() error {
	c.Name = cleanText(c.Name, 80)
	c.Company = cleanText(c.Company, 80)
	c.Email = cleanText(c.Email, 120)
	c.Notes = cleanText(c.Notes, 500)
	if c.Name == "" {
		return invalid("required", "name")
	}
	for f, p := range map[string]*string{"work": &c.Work, "mobile": &c.Mobile, "other": &c.Other} {
		*p = CleanNumber(*p)
		if *p != "" && !reNumber.MatchString(*p) {
			return invalid("invalid", f)
		}
	}
	if c.Work == "" && c.Mobile == "" && c.Other == "" {
		return invalid("required", "work")
	}
	if c.Email != "" && (!strings.Contains(c.Email, "@") || strings.ContainsAny(c.Email, " ,<>")) {
		return invalid("invalid", "email")
	}
	return nil
}

const contactCols = `id, name, company, work, mobile, other, email, notes`

func scanContact(row interface{ Scan(...any) error }) (*Contact, error) {
	var c Contact
	err := row.Scan(&c.ID, &c.Name, &c.Company, &c.Work, &c.Mobile, &c.Other, &c.Email, &c.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

// Contacts returns the phone book, sorted by name.
func (r *Repo) Contacts(ctx context.Context) ([]Contact, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+contactCols+` FROM contacts ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// SaveContact validates and stores a contact.
func (r *Repo) SaveContact(ctx context.Context, c *Contact) error {
	if err := c.validate(); err != nil {
		return err
	}
	args := []any{c.Name, c.Company, c.Work, c.Mobile, c.Other, c.Email, c.Notes}
	if c.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO contacts(name, company, work, mobile, other, email, notes) VALUES (?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE contacts SET name=?, company=?, work=?, mobile=?, other=?, email=?, notes=? WHERE id=?`, append(args, c.ID)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteContact removes a contact.
func (r *Repo) DeleteContact(ctx context.Context, id int64) error {
	return deleteRow(ctx, r.DB, "contacts", id)
}

// ImportContacts adds contacts in one transaction. replace removes the existing phone book first.
// It returns how many were added and the (1-based) rows that were skipped.
func (r *Repo) ImportContacts(ctx context.Context, in []Contact, replace bool) (int, []int, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	if replace {
		if _, err := tx.ExecContext(ctx, `DELETE FROM contacts`); err != nil {
			return 0, nil, err
		}
	}
	added, skipped := 0, []int{}
	for i := range in {
		c := in[i]
		if c.validate() != nil {
			skipped = append(skipped, i+1)
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO contacts(name, company, work, mobile, other, email, notes) VALUES (?,?,?,?,?,?,?)`,
			c.Name, c.Company, c.Work, c.Mobile, c.Other, c.Email, c.Notes); err != nil {
			return 0, nil, err
		}
		added++
	}
	return added, skipped, tx.Commit()
}

// Entry is one name with its numbers as phones see it (contacts and, optionally, extensions).
type Entry struct {
	ID        string
	Name      string
	Company   string
	Work      string
	Mobile    string
	Other     string
	Email     string
	Extension bool
}

// PhonebookEntries combines the contacts and (when switched on) the enabled extensions.
func (r *Repo) PhonebookEntries(ctx context.Context, includeExtensions bool) ([]Entry, error) {
	cs, err := r.Contacts(ctx)
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	if includeExtensions {
		exts, err := r.Extensions(ctx)
		if err != nil {
			return nil, err
		}
		org, _ := store.GetSetting(ctx, r.DB, "organization", "")
		for _, e := range exts {
			if e.Enabled {
				out = append(out, Entry{ID: "ext-" + e.Number, Name: e.Name, Company: org, Work: e.Number, Email: e.Email, Extension: true})
			}
		}
	}
	for _, c := range cs {
		out = append(out, Entry{ID: "c" + strconv.FormatInt(c.ID, 10), Name: c.Name, Company: c.Company, Work: c.Work, Mobile: c.Mobile, Other: c.Other, Email: c.Email})
	}
	return out, nil
}
