package pbx

import (
	"context"
	"encoding/json"
)

// KeyTemplate is a saved key layout for one phone model, with its expansion modules, that can be
// applied to several phones at once.
type KeyTemplate struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Vendor  string   `json:"vendor"`
	Model   string   `json:"model"`
	Keys    []Key    `json:"keys"`
	Modules []Module `json:"modules"`
}

// KeyTemplates lists all templates.
func (r *Repo) KeyTemplates(ctx context.Context) ([]KeyTemplate, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name, vendor, model, keys, modules FROM key_templates ORDER BY vendor, model, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KeyTemplate{}
	for rows.Next() {
		t, err := scanKeyTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// KeyTemplate returns one template.
func (r *Repo) KeyTemplate(ctx context.Context, id int64) (*KeyTemplate, error) {
	t, err := scanKeyTemplate(r.DB.QueryRowContext(ctx, `SELECT id, name, vendor, model, keys, modules FROM key_templates WHERE id = ?`, id))
	if err != nil {
		return nil, ErrNotFound
	}
	return t, nil
}

func scanKeyTemplate(row interface{ Scan(...any) error }) (*KeyTemplate, error) {
	var t KeyTemplate
	var keys, mods string
	if err := row.Scan(&t.ID, &t.Name, &t.Vendor, &t.Model, &keys, &mods); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(keys), &t.Keys)
	_ = json.Unmarshal([]byte(mods), &t.Modules)
	if t.Keys == nil {
		t.Keys = []Key{}
	}
	if t.Modules == nil {
		t.Modules = []Module{}
	}
	return &t, nil
}

// SaveKeyTemplate validates and stores a template against the limits of its model.
func (r *Repo) SaveKeyTemplate(ctx context.Context, t *KeyTemplate, lim PhoneLimits) error {
	t.Name = cleanText(t.Name, 60)
	if t.Name == "" {
		return invalid("required", "name")
	}
	if lim.Lines < 1 {
		lim.Lines = 1
	}
	var err error
	if t.Keys, err = CleanKeys(t.Keys, lim.Keys, lim.Lines); err != nil {
		return err
	}
	if t.Modules, err = CleanModules(t.Modules, lim); err != nil {
		return err
	}
	keys, _ := json.Marshal(t.Keys)
	mods, _ := json.Marshal(t.Modules)
	if t.ID == 0 {
		res, err := r.DB.ExecContext(ctx, `INSERT INTO key_templates(name, vendor, model, keys, modules) VALUES (?,?,?,?,?)`, t.Name, t.Vendor, t.Model, string(keys), string(mods))
		if err != nil {
			return err
		}
		t.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE key_templates SET name = ?, keys = ?, modules = ? WHERE id = ?`, t.Name, string(keys), string(mods), t.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteKeyTemplate removes a template.
func (r *Repo) DeleteKeyTemplate(ctx context.Context, id int64) error {
	return deleteRow(ctx, r.DB, "key_templates", id)
}
