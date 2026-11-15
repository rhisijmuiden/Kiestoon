// Package store opens the SQLite database and applies schema migrations.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo needed
)

// migrations are applied in order; each runs once. Never edit an applied migration, add a new one.
var migrations = []string{
	`CREATE TABLE users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
		display_name  TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		role          TEXT NOT NULL CHECK (role IN ('superadmin','admin','user')),
		totp_secret   TEXT NOT NULL DEFAULT '',
		totp_enabled  INTEGER NOT NULL DEFAULT 0,
		language      TEXT NOT NULL DEFAULT 'en',
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE sessions (
		token_hash  TEXT PRIMARY KEY,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		mfa_passed  INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL,
		expires_at  TEXT NOT NULL
	);
	CREATE INDEX sessions_user ON sessions(user_id);
	CREATE TABLE recovery_codes (
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		code_hash  TEXT NOT NULL,
		used       INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE audit_log (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		user_id  INTEGER,
		action   TEXT NOT NULL,
		detail   TEXT NOT NULL DEFAULT '',
		ip       TEXT NOT NULL DEFAULT ''
	);`,
	// 2: remember the last accepted TOTP time step so a code cannot be used twice.
	`ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0;`,
	// 3: phase 1, the phone system. Settings such as the number plan, mail, firewall and backup
	// schedule are JSON documents in the settings table; everything with rows gets its own table.
	`CREATE TABLE extensions (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		number           TEXT NOT NULL UNIQUE,
		name             TEXT NOT NULL,
		email            TEXT NOT NULL DEFAULT '',
		sip_secret       TEXT NOT NULL,
		transport        TEXT NOT NULL DEFAULT 'udp' CHECK (transport IN ('udp','tcp','tls')),
		srtp             INTEGER NOT NULL DEFAULT 0,
		max_contacts     INTEGER NOT NULL DEFAULT 3,
		outbound_cid     TEXT NOT NULL DEFAULT '',
		ring_seconds     INTEGER NOT NULL DEFAULT 25,
		voicemail        INTEGER NOT NULL DEFAULT 1,
		voicemail_pin    TEXT NOT NULL DEFAULT '',
		voicemail_email  INTEGER NOT NULL DEFAULT 1,
		voicemail_delete INTEGER NOT NULL DEFAULT 0,
		enabled          INTEGER NOT NULL DEFAULT 1,
		created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE trunks (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL UNIQUE COLLATE NOCASE,
		provider      TEXT NOT NULL DEFAULT 'generic',
		auth_mode     TEXT NOT NULL DEFAULT 'register' CHECK (auth_mode IN ('register','ip')),
		host          TEXT NOT NULL,
		port          INTEGER NOT NULL DEFAULT 5060,
		transport     TEXT NOT NULL DEFAULT 'udp' CHECK (transport IN ('udp','tcp','tls')),
		username      TEXT NOT NULL DEFAULT '',
		auth_username TEXT NOT NULL DEFAULT '',
		secret        TEXT NOT NULL DEFAULT '',
		from_domain   TEXT NOT NULL DEFAULT '',
		from_user     TEXT NOT NULL DEFAULT '',
		match_ips     TEXT NOT NULL DEFAULT '',
		did_source    TEXT NOT NULL DEFAULT 'ruri' CHECK (did_source IN ('ruri','to')),
		codecs        TEXT NOT NULL DEFAULT 'alaw,ulaw,g722',
		max_channels  INTEGER NOT NULL DEFAULT 0,
		caller_id     TEXT NOT NULL DEFAULT '',
		srtp          INTEGER NOT NULL DEFAULT 0,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE inbound_routes (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		name             TEXT NOT NULL,
		did              TEXT NOT NULL UNIQUE,
		destination_type TEXT NOT NULL CHECK (destination_type IN ('extension','voicemail','hangup')),
		destination      TEXT NOT NULL DEFAULT '',
		cid_prefix       TEXT NOT NULL DEFAULT '',
		enabled          INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE outbound_routes (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		name      TEXT NOT NULL,
		position  INTEGER NOT NULL DEFAULT 0,
		patterns  TEXT NOT NULL DEFAULT '[]',
		trunk_ids TEXT NOT NULL DEFAULT '[]',
		caller_id TEXT NOT NULL DEFAULT '',
		enabled   INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE phones (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		mac          TEXT NOT NULL UNIQUE,
		vendor       TEXT NOT NULL CHECK (vendor IN ('yealink','snom','grandstream')),
		model        TEXT NOT NULL,
		extension_id INTEGER REFERENCES extensions(id) ON DELETE SET NULL,
		label        TEXT NOT NULL DEFAULT '',
		keys         TEXT NOT NULL DEFAULT '[]',
		last_seen_at TEXT NOT NULL DEFAULT '',
		last_ip      TEXT NOT NULL DEFAULT '',
		user_agent   TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE cdr (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		uniqueid     TEXT NOT NULL,
		linkedid     TEXT NOT NULL DEFAULT '',
		start_at     TEXT NOT NULL,
		answer_at    TEXT NOT NULL DEFAULT '',
		end_at       TEXT NOT NULL DEFAULT '',
		duration     INTEGER NOT NULL DEFAULT 0,
		billsec      INTEGER NOT NULL DEFAULT 0,
		src          TEXT NOT NULL DEFAULT '',
		dst          TEXT NOT NULL DEFAULT '',
		clid         TEXT NOT NULL DEFAULT '',
		src_channel  TEXT NOT NULL DEFAULT '',
		dst_channel  TEXT NOT NULL DEFAULT '',
		dcontext     TEXT NOT NULL DEFAULT '',
		last_app     TEXT NOT NULL DEFAULT '',
		disposition  TEXT NOT NULL DEFAULT '',
		direction    TEXT NOT NULL DEFAULT 'internal',
		trunk        TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX cdr_start ON cdr(start_at);
	CREATE INDEX cdr_linked ON cdr(linkedid);
	CREATE TABLE mail_log (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		recipient TEXT NOT NULL,
		subject   TEXT NOT NULL DEFAULT '',
		kind      TEXT NOT NULL DEFAULT '',
		ok        INTEGER NOT NULL,
		error     TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE backup_targets (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		kind          TEXT NOT NULL CHECK (kind IN ('ftp','ftps','sftp','scp')),
		host          TEXT NOT NULL,
		port          INTEGER NOT NULL DEFAULT 0,
		username      TEXT NOT NULL DEFAULT '',
		password      TEXT NOT NULL DEFAULT '',
		private_key   TEXT NOT NULL DEFAULT '',
		path          TEXT NOT NULL DEFAULT '',
		implicit_tls  INTEGER NOT NULL DEFAULT 0,
		repo_password TEXT NOT NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE backup_runs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		target_id   INTEGER REFERENCES backup_targets(id) ON DELETE CASCADE,
		kind        TEXT NOT NULL,
		trigger     TEXT NOT NULL DEFAULT 'manual',
		started_at  TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT '',
		state       TEXT NOT NULL DEFAULT 'running',
		message     TEXT NOT NULL DEFAULT '',
		snapshot_id TEXT NOT NULL DEFAULT '',
		bytes_added INTEGER NOT NULL DEFAULT 0,
		files       INTEGER NOT NULL DEFAULT 0
	);`,
}

// Open opens (and creates if needed) the database at path and migrates it to the latest schema.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer at a time; a single connection avoids "database is locked" errors.
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for i := current; i < len(migrations); i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// GetSetting returns a stored setting, or fallback when it is not set.
func GetSetting(ctx context.Context, db *sql.DB, key, fallback string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	return v, err
}

// SetSetting stores a setting.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// Audit writes an entry to the audit log. Errors are ignored on purpose: auditing must never block an action.
func Audit(ctx context.Context, db *sql.DB, userID int64, action, detail, ip string) {
	var uid any
	if userID > 0 {
		uid = userID
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO audit_log(user_id, action, detail, ip) VALUES(?, ?, ?, ?)`, uid, action, detail, ip)
}

// GetJSON loads a JSON setting into v. A missing setting leaves v unchanged, so callers pass defaults.
func GetJSON(ctx context.Context, db *sql.DB, key string, v any) error {
	raw, err := GetSetting(ctx, db, key, "")
	if err != nil || raw == "" {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

// SetJSON stores v as a JSON setting.
func SetJSON(ctx context.Context, db *sql.DB, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return SetSetting(ctx, db, key, string(b))
}
