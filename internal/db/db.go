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
	return d, nil
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
  clone_url TEXT NOT NULL DEFAULT '',
  auto      INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS deploys (
  id         TEXT PRIMARY KEY,
  app        TEXT NOT NULL,
  commit_sha TEXT NOT NULL DEFAULT '',
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
CREATE TABLE IF NOT EXISTS sessions (
  token   TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  team_id TEXT NOT NULL DEFAULT '',
  created INTEGER NOT NULL
);
`
