// Package store opens the SQLite database and applies schema migrations.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// 4: phase 2, business features. Rebuilds users (new role "reception", link to an extension,
	// SSO identity) and inbound_routes (more destination types). Runs with foreign keys off.
	fkOff + `CREATE TABLE users_new (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		username       TEXT NOT NULL UNIQUE COLLATE NOCASE,
		display_name   TEXT NOT NULL,
		password_hash  TEXT NOT NULL,
		role           TEXT NOT NULL CHECK (role IN ('superadmin','admin','reception','user')),
		totp_secret    TEXT NOT NULL DEFAULT '',
		totp_enabled   INTEGER NOT NULL DEFAULT 0,
		language       TEXT NOT NULL DEFAULT 'en',
		created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		totp_last_step INTEGER NOT NULL DEFAULT 0,
		email          TEXT NOT NULL DEFAULT '',
		extension_id   INTEGER REFERENCES extensions(id) ON DELETE SET NULL,
		sso_provider   INTEGER NOT NULL DEFAULT 0,
		sso_subject    TEXT NOT NULL DEFAULT '',
		enabled        INTEGER NOT NULL DEFAULT 1,
		last_login_at  TEXT NOT NULL DEFAULT ''
	);
	INSERT INTO users_new(id, username, display_name, password_hash, role, totp_secret, totp_enabled, language, created_at, totp_last_step)
		SELECT id, username, display_name, password_hash, role, totp_secret, totp_enabled, language, created_at, totp_last_step FROM users;
	DROP TABLE users;
	ALTER TABLE users_new RENAME TO users;
	CREATE INDEX users_sso ON users(sso_provider, sso_subject);
	ALTER TABLE sessions ADD COLUMN method TEXT NOT NULL DEFAULT 'password';

	CREATE TABLE inbound_routes_new (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		name             TEXT NOT NULL,
		did              TEXT NOT NULL UNIQUE,
		destination_type TEXT NOT NULL,
		destination      TEXT NOT NULL DEFAULT '',
		cid_prefix       TEXT NOT NULL DEFAULT '',
		enabled          INTEGER NOT NULL DEFAULT 1
	);
	INSERT INTO inbound_routes_new SELECT id, name, did, destination_type, destination, cid_prefix, enabled FROM inbound_routes;
	DROP TABLE inbound_routes;
	ALTER TABLE inbound_routes_new RENAME TO inbound_routes;

	ALTER TABLE extensions ADD COLUMN features TEXT NOT NULL DEFAULT '{}';
	ALTER TABLE extensions ADD COLUMN record_mode TEXT NOT NULL DEFAULT 'ondemand';
	ALTER TABLE extensions ADD COLUMN pickup_group TEXT NOT NULL DEFAULT '1';
	ALTER TABLE extensions ADD COLUMN outbound_class TEXT NOT NULL DEFAULT 'national';
	ALTER TABLE extensions ADD COLUMN allow_premium INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE extensions ADD COLUMN webrtc INTEGER NOT NULL DEFAULT 1;
	ALTER TABLE extensions ADD COLUMN webrtc_secret TEXT NOT NULL DEFAULT '';
	-- Existing extensions keep the international calling they had before.
	UPDATE extensions SET outbound_class = 'international';

	CREATE TABLE ring_groups (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		number       TEXT NOT NULL UNIQUE,
		name         TEXT NOT NULL,
		strategy     TEXT NOT NULL DEFAULT 'simultaneous',
		ring_seconds INTEGER NOT NULL DEFAULT 20,
		members      TEXT NOT NULL DEFAULT '[]',
		cid_prefix   TEXT NOT NULL DEFAULT '',
		skip_busy    INTEGER NOT NULL DEFAULT 1,
		failover_type TEXT NOT NULL DEFAULT 'hangup',
		failover     TEXT NOT NULL DEFAULT '',
		enabled      INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE queues (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		number             TEXT NOT NULL UNIQUE,
		name               TEXT NOT NULL,
		strategy           TEXT NOT NULL DEFAULT 'rrmemory',
		music              TEXT NOT NULL DEFAULT 'default',
		announce_position  INTEGER NOT NULL DEFAULT 1,
		announce_frequency INTEGER NOT NULL DEFAULT 60,
		agent_timeout      INTEGER NOT NULL DEFAULT 20,
		retry              INTEGER NOT NULL DEFAULT 5,
		wrapup             INTEGER NOT NULL DEFAULT 10,
		max_wait           INTEGER NOT NULL DEFAULT 300,
		max_callers        INTEGER NOT NULL DEFAULT 0,
		join_empty         INTEGER NOT NULL DEFAULT 0,
		record             INTEGER NOT NULL DEFAULT 0,
		cid_prefix         TEXT NOT NULL DEFAULT '',
		greeting_prompt    INTEGER NOT NULL DEFAULT 0,
		overflow_type      TEXT NOT NULL DEFAULT 'hangup',
		overflow           TEXT NOT NULL DEFAULT '',
		enabled            INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE queue_members (
		queue_id     INTEGER NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
		extension_id INTEGER NOT NULL REFERENCES extensions(id) ON DELETE CASCADE,
		penalty      INTEGER NOT NULL DEFAULT 0,
		dynamic      INTEGER NOT NULL DEFAULT 1,
		logged_in    INTEGER NOT NULL DEFAULT 1,
		paused       INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (queue_id, extension_id)
	);
	CREATE TABLE ivrs (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT NOT NULL,
		prompt_id       INTEGER NOT NULL DEFAULT 0,
		timeout         INTEGER NOT NULL DEFAULT 5,
		retries         INTEGER NOT NULL DEFAULT 2,
		direct_dial     INTEGER NOT NULL DEFAULT 1,
		options         TEXT NOT NULL DEFAULT '[]',
		timeout_type    TEXT NOT NULL DEFAULT 'hangup',
		timeout_target  TEXT NOT NULL DEFAULT '',
		layout          TEXT NOT NULL DEFAULT '{}'
	);
	CREATE TABLE time_conditions (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		name           TEXT NOT NULL,
		schedule       TEXT NOT NULL DEFAULT '[]',
		use_holidays   INTEGER NOT NULL DEFAULT 1,
		mode           TEXT NOT NULL DEFAULT 'auto',
		open_type      TEXT NOT NULL DEFAULT 'hangup',
		open_target    TEXT NOT NULL DEFAULT '',
		closed_type    TEXT NOT NULL DEFAULT 'hangup',
		closed_target  TEXT NOT NULL DEFAULT '',
		holiday_type   TEXT NOT NULL DEFAULT '',
		holiday_target TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE conferences (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		number      TEXT NOT NULL UNIQUE,
		name        TEXT NOT NULL,
		pin         TEXT NOT NULL DEFAULT '',
		admin_pin   TEXT NOT NULL DEFAULT '',
		max_members INTEGER NOT NULL DEFAULT 20,
		record      INTEGER NOT NULL DEFAULT 0,
		wait_admin  INTEGER NOT NULL DEFAULT 0,
		enabled     INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE prompts (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE sso_providers (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		kind          TEXT NOT NULL DEFAULT 'generic',
		issuer        TEXT NOT NULL,
		client_id     TEXT NOT NULL,
		client_secret TEXT NOT NULL DEFAULT '',
		scopes        TEXT NOT NULL DEFAULT 'openid profile email',
		groups_claim  TEXT NOT NULL DEFAULT 'groups',
		role_map      TEXT NOT NULL DEFAULT '[]',
		default_role  TEXT NOT NULL DEFAULT 'user',
		auto_create   INTEGER NOT NULL DEFAULT 1,
		trust_mfa     INTEGER NOT NULL DEFAULT 1,
		enabled       INTEGER NOT NULL DEFAULT 1
	);`,
	// 5: phase 3, integrations. Rebuilds phones (more vendors, extra DECT handsets) and
	// backup_targets (OneDrive). Runs with foreign keys off.
	fkOff + `CREATE TABLE phones_new (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		mac          TEXT NOT NULL UNIQUE,
		vendor       TEXT NOT NULL CHECK (vendor IN ('yealink','snom','grandstream','fanvil','gigaset','poly')),
		model        TEXT NOT NULL,
		extension_id INTEGER REFERENCES extensions(id) ON DELETE SET NULL,
		label        TEXT NOT NULL DEFAULT '',
		keys         TEXT NOT NULL DEFAULT '[]',
		last_seen_at TEXT NOT NULL DEFAULT '',
		last_ip      TEXT NOT NULL DEFAULT '',
		user_agent   TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		lines        TEXT NOT NULL DEFAULT '[]'
	);
	INSERT INTO phones_new(id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, created_at)
		SELECT id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, created_at FROM phones;
	DROP TABLE phones;
	ALTER TABLE phones_new RENAME TO phones;

	CREATE TABLE backup_targets_new (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		kind          TEXT NOT NULL CHECK (kind IN ('ftp','ftps','sftp','scp','onedrive')),
		host          TEXT NOT NULL,
		port          INTEGER NOT NULL DEFAULT 0,
		username      TEXT NOT NULL DEFAULT '',
		password      TEXT NOT NULL DEFAULT '',
		private_key   TEXT NOT NULL DEFAULT '',
		path          TEXT NOT NULL DEFAULT '',
		implicit_tls  INTEGER NOT NULL DEFAULT 0,
		repo_password TEXT NOT NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		onedrive      TEXT NOT NULL DEFAULT '{}'
	);
	INSERT INTO backup_targets_new(id, name, kind, host, port, username, password, private_key, path, implicit_tls, repo_password, enabled, created_at)
		SELECT id, name, kind, host, port, username, password, private_key, path, implicit_tls, repo_password, enabled, created_at FROM backup_targets;
	DROP TABLE backup_targets;
	ALTER TABLE backup_targets_new RENAME TO backup_targets;

	ALTER TABLE trunks ADD COLUMN t38 INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE sso_providers ADD COLUMN protocol TEXT NOT NULL DEFAULT 'oidc';
	ALTER TABLE sso_providers ADD COLUMN saml_metadata TEXT NOT NULL DEFAULT '';

	CREATE TABLE fax_boxes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		emails     TEXT NOT NULL DEFAULT '',
		keep_days  INTEGER NOT NULL DEFAULT 90,
		ecm        INTEGER NOT NULL DEFAULT 1,
		enabled    INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE faxes (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		box_id         INTEGER REFERENCES fax_boxes(id) ON DELETE SET NULL,
		received_at    TEXT NOT NULL,
		caller         TEXT NOT NULL DEFAULT '',
		caller_name    TEXT NOT NULL DEFAULT '',
		did            TEXT NOT NULL DEFAULT '',
		remote_station TEXT NOT NULL DEFAULT '',
		pages          INTEGER NOT NULL DEFAULT 0,
		status         TEXT NOT NULL DEFAULT '',
		error          TEXT NOT NULL DEFAULT '',
		file           TEXT NOT NULL DEFAULT '',
		size           INTEGER NOT NULL DEFAULT 0,
		mailed         INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX faxes_box ON faxes(box_id, received_at);
	CREATE TABLE contacts (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		company    TEXT NOT NULL DEFAULT '',
		work       TEXT NOT NULL DEFAULT '',
		mobile     TEXT NOT NULL DEFAULT '',
		other      TEXT NOT NULL DEFAULT '',
		email      TEXT NOT NULL DEFAULT '',
		notes      TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE api_tokens (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT NOT NULL,
		token_hash   TEXT NOT NULL UNIQUE,
		prefix       TEXT NOT NULL,
		user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role         TEXT NOT NULL CHECK (role IN ('admin','reception','user')),
		read_only    INTEGER NOT NULL DEFAULT 1,
		expires_at   TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT '',
		last_ip      TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE webhooks (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		url        TEXT NOT NULL,
		events     TEXT NOT NULL DEFAULT '[]',
		secret     TEXT NOT NULL,
		enabled    INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE webhook_deliveries (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		webhook_id  INTEGER NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
		at          TEXT NOT NULL,
		event       TEXT NOT NULL,
		attempt     INTEGER NOT NULL DEFAULT 1,
		status      INTEGER NOT NULL DEFAULT 0,
		error       TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX webhook_deliveries_hook ON webhook_deliveries(webhook_id, id);
	CREATE TABLE passkeys (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name          TEXT NOT NULL,
		credential_id TEXT NOT NULL UNIQUE,
		data          TEXT NOT NULL,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		last_used_at  TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX passkeys_user ON passkeys(user_id);`,
	// 6: phase 4, extensions. Rebuilds phones (vendor list checked in Go from now on) and
	// backup_targets (S3, WebDAV, SMB); adds firmware, ZTP state and fax sending.
	fkOff + `CREATE TABLE phones_new (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		mac          TEXT NOT NULL UNIQUE,
		vendor       TEXT NOT NULL,
		model        TEXT NOT NULL,
		extension_id INTEGER REFERENCES extensions(id) ON DELETE SET NULL,
		label        TEXT NOT NULL DEFAULT '',
		keys         TEXT NOT NULL DEFAULT '[]',
		last_seen_at TEXT NOT NULL DEFAULT '',
		last_ip      TEXT NOT NULL DEFAULT '',
		user_agent   TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		lines        TEXT NOT NULL DEFAULT '[]',
		ztp_state    TEXT NOT NULL DEFAULT '',
		ztp_error    TEXT NOT NULL DEFAULT '',
		ztp_at       TEXT NOT NULL DEFAULT ''
	);
	INSERT INTO phones_new(id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, created_at, lines)
		SELECT id, mac, vendor, model, extension_id, label, keys, last_seen_at, last_ip, user_agent, created_at, lines FROM phones;
	DROP TABLE phones;
	ALTER TABLE phones_new RENAME TO phones;

	CREATE TABLE backup_targets_new (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		kind          TEXT NOT NULL,
		host          TEXT NOT NULL,
		port          INTEGER NOT NULL DEFAULT 0,
		username      TEXT NOT NULL DEFAULT '',
		password      TEXT NOT NULL DEFAULT '',
		private_key   TEXT NOT NULL DEFAULT '',
		path          TEXT NOT NULL DEFAULT '',
		implicit_tls  INTEGER NOT NULL DEFAULT 0,
		repo_password TEXT NOT NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		onedrive      TEXT NOT NULL DEFAULT '{}',
		extra         TEXT NOT NULL DEFAULT '{}'
	);
	INSERT INTO backup_targets_new(id, name, kind, host, port, username, password, private_key, path, implicit_tls, repo_password, enabled, created_at, onedrive)
		SELECT id, name, kind, host, port, username, password, private_key, path, implicit_tls, repo_password, enabled, created_at, onedrive FROM backup_targets;
	DROP TABLE backup_targets;
	ALTER TABLE backup_targets_new RENAME TO backup_targets;

	CREATE TABLE firmware (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		vendor     TEXT NOT NULL,
		models     TEXT NOT NULL DEFAULT '[]',
		version    TEXT NOT NULL DEFAULT '',
		filename   TEXT NOT NULL,
		token      TEXT NOT NULL UNIQUE,
		size       INTEGER NOT NULL DEFAULT 0,
		sha256     TEXT NOT NULL DEFAULT '',
		active     INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);

	ALTER TABLE fax_boxes ADD COLUMN number TEXT NOT NULL DEFAULT '';
	ALTER TABLE fax_boxes ADD COLUMN send_enabled INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE fax_boxes ADD COLUMN senders TEXT NOT NULL DEFAULT '';
	ALTER TABLE fax_boxes ADD COLUMN send_pin TEXT NOT NULL DEFAULT '';
	CREATE TABLE fax_jobs (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		box_id       INTEGER REFERENCES fax_boxes(id) ON DELETE SET NULL,
		user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
		source       TEXT NOT NULL DEFAULT 'portal',
		sender       TEXT NOT NULL DEFAULT '',
		number       TEXT NOT NULL,
		document     TEXT NOT NULL DEFAULT '',
		file         TEXT NOT NULL DEFAULT '',
		pages        INTEGER NOT NULL DEFAULT 0,
		status       TEXT NOT NULL DEFAULT 'queued',
		attempts     INTEGER NOT NULL DEFAULT 0,
		error        TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		next_at      TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX fax_jobs_status ON fax_jobs(status, next_at);`,
	// 7: phase 5, multi-site. Kiestoon Edge satellites, federated Kiestoon systems (with the agents of
	// their queues) and follow-the-sun routes. The WireGuard hub is a JSON setting.
	`CREATE TABLE edge_sites (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT NOT NULL UNIQUE COLLATE NOCASE,
		pair_hash    TEXT NOT NULL DEFAULT '',
		pair_expires TEXT NOT NULL DEFAULT '',
		public_key   TEXT NOT NULL DEFAULT '',
		tunnel_ip    TEXT NOT NULL DEFAULT '',
		token_hash   TEXT NOT NULL DEFAULT '',
		hostname     TEXT NOT NULL DEFAULT '',
		version      TEXT NOT NULL DEFAULT '',
		lan_address  TEXT NOT NULL DEFAULT '',
		lan_net      TEXT NOT NULL DEFAULT '',
		services     TEXT NOT NULL DEFAULT '{}',
		last_seen    TEXT NOT NULL DEFAULT '',
		paired_at    TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE federation_peers (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		name           TEXT NOT NULL,
		prefix         TEXT NOT NULL DEFAULT '',
		role           TEXT NOT NULL CHECK (role IN ('hosted','joined')),
		state          TEXT NOT NULL DEFAULT 'invited' CHECK (state IN ('invited','active')),
		invite_hash    TEXT NOT NULL DEFAULT '',
		invite_expires TEXT NOT NULL DEFAULT '',
		public_key     TEXT NOT NULL DEFAULT '',
		tunnel_ip      TEXT NOT NULL DEFAULT '',
		local_ip       TEXT NOT NULL DEFAULT '',
		private_key    TEXT NOT NULL DEFAULT '',
		endpoint       TEXT NOT NULL DEFAULT '',
		token_out      TEXT NOT NULL DEFAULT '',
		token_in_hash  TEXT NOT NULL DEFAULT '',
		sip_port       INTEGER NOT NULL DEFAULT 5060,
		portal_port    INTEGER NOT NULL DEFAULT 8080,
		direct_dial    INTEGER NOT NULL DEFAULT 0,
		directory      TEXT NOT NULL DEFAULT '{}',
		version        TEXT NOT NULL DEFAULT '',
		last_seen      TEXT NOT NULL DEFAULT '',
		created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE TABLE queue_remote_members (
		queue_id INTEGER NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
		peer_id  INTEGER NOT NULL REFERENCES federation_peers(id) ON DELETE CASCADE,
		number   TEXT NOT NULL,
		penalty  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (queue_id, peer_id, number)
	);
	CREATE TABLE sun_routes (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		rules         TEXT NOT NULL DEFAULT '[]',
		fallback_type TEXT NOT NULL DEFAULT 'hangup',
		fallback      TEXT NOT NULL DEFAULT '',
		enabled       INTEGER NOT NULL DEFAULT 1
	);`,
	`ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'system';`,
	`CREATE TABLE password_tokens (
		token_hash TEXT PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE INDEX password_tokens_user ON password_tokens(user_id);`,
	// Phones that asked for their configuration but are not known yet, so an admin can adopt them.
	`CREATE TABLE phone_discoveries (
		mac        TEXT PRIMARY KEY,
		vendor     TEXT NOT NULL DEFAULT '',
		model      TEXT NOT NULL DEFAULT '',
		ip         TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		file       TEXT NOT NULL DEFAULT '',
		hits       INTEGER NOT NULL DEFAULT 1,
		first_seen TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		last_seen  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);`,
	// Expansion modules and intercom options per phone, and key layouts to reuse per model.
	`ALTER TABLE phones ADD COLUMN modules TEXT NOT NULL DEFAULT '[]';
	ALTER TABLE phones ADD COLUMN options TEXT NOT NULL DEFAULT '{}';
	CREATE TABLE key_templates (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		vendor     TEXT NOT NULL,
		model      TEXT NOT NULL,
		keys       TEXT NOT NULL DEFAULT '[]',
		modules    TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);`,
	// Call journeys: the CEL, dial and queue events of every call, by linkedid, to show in the
	// call history how a call went. Removed after the retention set under Call features.
	`CREATE TABLE call_events (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		linkedid TEXT NOT NULL,
		at       TEXT NOT NULL,
		kind     TEXT NOT NULL,
		uniqueid TEXT NOT NULL DEFAULT '',
		channel  TEXT NOT NULL DEFAULT '',
		data     TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX call_events_linked ON call_events(linkedid, at);
	CREATE INDEX call_events_at ON call_events(at);`,
	// Phase 6: queue options (supervision, callback) and the queue callback list.
	`ALTER TABLE queues ADD COLUMN options TEXT NOT NULL DEFAULT '{}';
	CREATE TABLE queue_callbacks (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		queue_id   INTEGER NOT NULL REFERENCES queues(id) ON DELETE CASCADE,
		number     TEXT NOT NULL,
		name       TEXT NOT NULL DEFAULT '',
		position   INTEGER NOT NULL DEFAULT 0,
		status     TEXT NOT NULL DEFAULT 'scheduled',
		attempts   INTEGER NOT NULL DEFAULT 0,
		agent      TEXT NOT NULL DEFAULT '',
		linkedid   TEXT NOT NULL DEFAULT '',
		note       TEXT NOT NULL DEFAULT '',
		next_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	);
	CREATE INDEX queue_callbacks_status ON queue_callbacks(status, next_at);`,
	// Phase 6: wallboard tv links and the statistics of queue calls (wallboard, reports).
	`CREATE TABLE wallboard_screens (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL UNIQUE,
		key        TEXT NOT NULL UNIQUE,
		queues     TEXT NOT NULL DEFAULT '[]',
		layouts    TEXT NOT NULL DEFAULT '["overview"]',
		rotate     INTEGER NOT NULL DEFAULT 30,
		theme      TEXT NOT NULL DEFAULT 'dark',
		hide_names INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		last_seen  TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE queue_calls (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		queue_id    INTEGER NOT NULL,
		uniqueid    TEXT NOT NULL,
		linkedid    TEXT NOT NULL DEFAULT '',
		caller      TEXT NOT NULL DEFAULT '',
		callback    INTEGER NOT NULL DEFAULT 0,
		joined_at   TEXT NOT NULL,
		result      TEXT NOT NULL DEFAULT '',
		hold        INTEGER NOT NULL DEFAULT 0,
		talk        INTEGER NOT NULL DEFAULT 0,
		agent       TEXT NOT NULL DEFAULT '',
		agent_name  TEXT NOT NULL DEFAULT '',
		ended_at    TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX queue_calls_joined ON queue_calls(joined_at);
	CREATE INDEX queue_calls_open ON queue_calls(uniqueid, result);`,
	// Phase 6: scheduled queue reports and the log of SLA alerts they count.
	`CREATE TABLE wallboard_reports (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL UNIQUE,
		period     TEXT NOT NULL,
		time       TEXT NOT NULL,
		weekday    INTEGER NOT NULL DEFAULT 1,
		workdays   INTEGER NOT NULL DEFAULT 0,
		queues     TEXT NOT NULL DEFAULT '[]',
		sections   TEXT NOT NULL DEFAULT '["queues"]',
		recipients TEXT NOT NULL DEFAULT '[]',
		language   TEXT NOT NULL DEFAULT 'en',
		enabled    INTEGER NOT NULL DEFAULT 1,
		last_sent  TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE sla_alerts (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		queue_id INTEGER NOT NULL,
		kind     TEXT NOT NULL,
		at       TEXT NOT NULL
	);
	CREATE INDEX sla_alerts_at ON sla_alerts(at);`,
	// Phase 6: phones with the mobile app. handle is the opaque push handle of the relay (the push
	// token itself is never stored); secret encrypts the content of notifications for this phone.
	`CREATE TABLE mobile_devices (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		app_id     TEXT NOT NULL,
		name       TEXT NOT NULL DEFAULT '',
		platform   TEXT NOT NULL,
		handle     TEXT NOT NULL,
		secret     TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_seen  TEXT NOT NULL DEFAULT '',
		last_push  TEXT NOT NULL DEFAULT '',
		UNIQUE(user_id, app_id)
	);`,
	// An iPhone has two push tokens: PushKit (calls) and the one for visible notifications.
	`ALTER TABLE mobile_devices ADD COLUMN alert_handle TEXT NOT NULL DEFAULT '';`,
	// Hot desks: the extension logged in on a phone and since when.
	`ALTER TABLE phones ADD COLUMN hotdesk_ext INTEGER REFERENCES extensions(id) ON DELETE SET NULL;
	ALTER TABLE phones ADD COLUMN hotdesk_at TEXT NOT NULL DEFAULT '';`,
}

// fkOff marks a migration that rebuilds tables: it runs with foreign key enforcement off, so
// dropping the old table does not cascade into the rows that point at it.
const fkOff = "-- foreign_keys=off\n"

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
		noFK := strings.HasPrefix(migrations[i], fkOff)
		if noFK {
			// PRAGMA foreign_keys cannot change inside a transaction. Open uses one connection, so this sticks.
			if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
				return err
			}
		}
		if err := runMigration(ctx, db, i, noFK); err != nil {
			return err
		}
		if noFK {
			if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
				return err
			}
		}
	}
	return nil
}

func runMigration(ctx context.Context, db *sql.DB, i int, checkFK bool) error {
	{
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
		if checkFK {
			var bad int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&bad); err != nil {
				tx.Rollback()
				return err
			}
			if bad > 0 {
				tx.Rollback()
				return fmt.Errorf("migration %d: %d foreign key violations", i+1, bad)
			}
		}
		return tx.Commit()
	}
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
