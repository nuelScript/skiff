// Package db opens Skiff's embedded SQLite database (pure-Go, no CGO, so it
// cross-compiles) and applies the schema. It lives as a single file on the box,
// keeping the one-binary self-hosted story intact.
package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open() (*sql.DB, error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".skiff")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dir, "skiff.db") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer; serialize to avoid lock churn on a low-traffic panel.
	d.SetMaxOpenConns(1)
	if _, err := d.Exec(schema); err != nil {
		return nil, err
	}
	// Idempotent migrations for databases created before a column existed.
	// (CREATE TABLE IF NOT EXISTS won't add columns to a table that's already there.)
	for _, m := range migrations {
		_, _ = d.Exec(m)
	}
	return d, nil
}

// migrations are additive and safe to re-run; each fails harmlessly once applied.
var migrations = []string{
	`ALTER TABLE deploys ADD COLUMN message TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE sources ADD COLUMN parent TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE sources ADD COLUMN preview_auto INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE databases ADD COLUMN public INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE databases ADD COLUMN public_port INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE sources ADD COLUMN replicas INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE sources ADD COLUMN release TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE sources ADD COLUMN autoscale INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE sources ADD COLUMN scale_min INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE sources ADD COLUMN scale_max INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE sources ADD COLUMN scale_cpu INTEGER NOT NULL DEFAULT 0`,
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,
  email         TEXT UNIQUE NOT NULL,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  created       INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS teams (
  id      TEXT PRIMARY KEY,
  name    TEXT NOT NULL,
  slug    TEXT NOT NULL,
  created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS memberships (
  user_id TEXT NOT NULL,
  team_id TEXT NOT NULL,
  role    TEXT NOT NULL,
  PRIMARY KEY (user_id, team_id)
);
CREATE TABLE IF NOT EXISTS invites (
  token   TEXT PRIMARY KEY,
  email   TEXT NOT NULL,
  team_id TEXT NOT NULL,
  role    TEXT NOT NULL,
  created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sources (
  app       TEXT PRIMARY KEY,
  team      TEXT NOT NULL DEFAULT '',
  repo      TEXT NOT NULL DEFAULT '',
  branch    TEXT NOT NULL DEFAULT '',
  root_dir  TEXT NOT NULL DEFAULT '',
  port      TEXT NOT NULL DEFAULT '3000',
  clone_url    TEXT NOT NULL DEFAULT '',
  auto         INTEGER NOT NULL DEFAULT 0,
  parent       TEXT NOT NULL DEFAULT '',
  preview_auto INTEGER NOT NULL DEFAULT 0,
  replicas     INTEGER NOT NULL DEFAULT 1,
  release      TEXT NOT NULL DEFAULT '',
  autoscale    INTEGER NOT NULL DEFAULT 0,
  scale_min    INTEGER NOT NULL DEFAULT 1,
  scale_max    INTEGER NOT NULL DEFAULT 1,
  scale_cpu    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS deploys (
  id         TEXT PRIMARY KEY,
  app        TEXT NOT NULL,
  commit_sha TEXT NOT NULL DEFAULT '',
  message    TEXT NOT NULL DEFAULT '',
  trigger    TEXT NOT NULL DEFAULT 'manual',
  status     TEXT NOT NULL DEFAULT 'building',
  started    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_deploys_app ON deploys(app, started DESC);
CREATE TABLE IF NOT EXISTS env_vars (
  app   TEXT NOT NULL,
  key   TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  build INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (app, key)
);
CREATE TABLE IF NOT EXISTS shared_env (
  team  TEXT NOT NULL,
  key   TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  build INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (team, key)
);
CREATE TABLE IF NOT EXISTS domains (
  host    TEXT PRIMARY KEY,
  app     TEXT NOT NULL,
  team    TEXT NOT NULL DEFAULT '',
  created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token   TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  team_id TEXT NOT NULL DEFAULT '',
  created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS databases (
  id        TEXT PRIMARY KEY,
  team      TEXT NOT NULL,
  name      TEXT NOT NULL,
  engine    TEXT NOT NULL,
  container TEXT NOT NULL,
  host      TEXT NOT NULL,
  port      INTEGER NOT NULL,
  username  TEXT NOT NULL DEFAULT '',
  password  TEXT NOT NULL DEFAULT '',
  dbname    TEXT NOT NULL DEFAULT '',
  created   INTEGER NOT NULL,
  public      INTEGER NOT NULL DEFAULT 0,
  public_port INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS db_attachments (
  db_id TEXT NOT NULL,
  app   TEXT NOT NULL,
  var   TEXT NOT NULL,
  PRIMARY KEY (db_id, app)
);
CREATE TABLE IF NOT EXISTS backups (
  id      TEXT PRIMARY KEY,
  db_id   TEXT NOT NULL,
  team    TEXT NOT NULL,
  engine  TEXT NOT NULL,
  file    TEXT NOT NULL,
  size    INTEGER NOT NULL DEFAULT 0,
  trigger TEXT NOT NULL DEFAULT 'manual',
  created INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_backups_db ON backups(db_id, created DESC);
CREATE TABLE IF NOT EXISTS jobs (
  id       TEXT PRIMARY KEY,
  app      TEXT NOT NULL,
  team     TEXT NOT NULL,
  name     TEXT NOT NULL,
  schedule TEXT NOT NULL,
  command  TEXT NOT NULL,
  last_run INTEGER NOT NULL DEFAULT 0,
  last_ok  INTEGER NOT NULL DEFAULT 1,
  created  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_app ON jobs(app);
CREATE TABLE IF NOT EXISTS alerts (
  team        TEXT PRIMARY KEY,
  email       TEXT NOT NULL DEFAULT '',
  slack_url   TEXT NOT NULL DEFAULT '',
  webhook_url TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS audit (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  team    TEXT NOT NULL,
  actor   TEXT NOT NULL,
  action  TEXT NOT NULL,
  target  TEXT NOT NULL DEFAULT '',
  detail  TEXT NOT NULL DEFAULT '',
  created INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_team ON audit(team, id DESC);
CREATE TABLE IF NOT EXISTS api_tokens (
  id         TEXT PRIMARY KEY,
  team       TEXT NOT NULL,
  name       TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created    INTEGER NOT NULL,
  last_used  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_team ON api_tokens(team, created DESC);
`
