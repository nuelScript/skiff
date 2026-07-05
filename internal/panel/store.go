package panel

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"
)

// Source is a deployable app's git origin (per team), so a webhook or redeploy
// can rebuild it without the user re-entering anything.
type Source struct {
	App      string `json:"app"`
	Team     string `json:"team"`
	Repo     string `json:"repo"` // owner/name
	Branch   string `json:"branch"`
	RootDir  string `json:"rootDir"` // subdirectory to build (monorepos)
	Port     string `json:"port"`
	CloneURL string `json:"cloneUrl"`
	Auto     bool   `json:"auto"`
}

// Deploy is one build/release, with a persisted log the dashboard can replay.
type Deploy struct {
	ID      string `json:"id"`
	App     string `json:"app"`
	Commit  string `json:"commit"`
	Message string `json:"message"`
	Trigger string `json:"trigger"`
	Status  string `json:"status"`
	Started int64  `json:"started"`
	// Rollbackable is computed (not persisted): the build's image is still
	// retained and it isn't the version currently serving.
	Rollbackable bool `json:"rollbackable,omitempty"`
}

// EnvVar is a project environment variable. Build vars land in the image build
// ([env]); non-build vars are runtime-only secrets ([secrets]).
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Build bool   `json:"build"`
}

// sqlDB is the panel's shared handle to the SQLite database (set in New).
var sqlDB *sql.DB

func skiffDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".skiff")
}

func logPath(app, id string) string {
	return filepath.Join(skiffDir(), "deploys", sanitizeName(app), sanitizeID(id)+".log")
}

const sourceCols = `app,team,repo,branch,root_dir,port,clone_url,auto`

func putSource(s Source) error {
	_, err := sqlDB.Exec(`
		INSERT INTO sources(app,team,repo,branch,root_dir,port,clone_url,auto)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(app) DO UPDATE SET
			team=excluded.team, repo=excluded.repo, branch=excluded.branch,
			root_dir=excluded.root_dir, port=excluded.port,
			clone_url=excluded.clone_url, auto=excluded.auto`,
		s.App, s.Team, s.Repo, s.Branch, s.RootDir, s.Port, s.CloneURL, b2i(s.Auto))
	return err
}

func scanSource(row interface{ Scan(...any) error }) (Source, bool) {
	var s Source
	var auto int
	if row.Scan(&s.App, &s.Team, &s.Repo, &s.Branch, &s.RootDir, &s.Port, &s.CloneURL, &auto) != nil {
		return Source{}, false
	}
	s.Auto = auto != 0
	return s, true
}

func getSource(app string) (Source, bool) {
	return scanSource(sqlDB.QueryRow(`SELECT `+sourceCols+` FROM sources WHERE app=?`, app))
}

func deleteSource(app string) {
	_, _ = sqlDB.Exec(`DELETE FROM sources WHERE app=?`, app)
	_, _ = sqlDB.Exec(`DELETE FROM deploys WHERE app=?`, app)
	_, _ = sqlDB.Exec(`DELETE FROM env_vars WHERE app=?`, app)
}

// sourcesForRepo returns auto-deploy sources matching a pushed repo + branch.
func sourcesForRepo(repo, branch string) []Source {
	rows, err := sqlDB.Query(`SELECT `+sourceCols+` FROM sources
		WHERE auto=1 AND repo=? AND (branch='' OR branch=?)`, repo, branch)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		if s, ok := scanSource(rows); ok {
			out = append(out, s)
		}
	}
	return out
}

func addDeploy(d Deploy) {
	_, _ = sqlDB.Exec(
		`INSERT INTO deploys(id,app,commit_sha,message,trigger,status,started) VALUES(?,?,?,?,?,?,?)`,
		d.ID, d.App, d.Commit, d.Message, d.Trigger, d.Status, d.Started)
}

func setDeployStatus(app, id, status string) {
	_, _ = sqlDB.Exec(`UPDATE deploys SET status=? WHERE id=? AND app=?`, status, id, app)
}

func appDeploys(app string) []Deploy {
	rows, err := sqlDB.Query(
		`SELECT id,app,commit_sha,message,trigger,status,started FROM deploys WHERE app=? ORDER BY started DESC LIMIT 20`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Deploy{}
	for rows.Next() {
		var d Deploy
		if rows.Scan(&d.ID, &d.App, &d.Commit, &d.Message, &d.Trigger, &d.Status, &d.Started) == nil {
			out = append(out, d)
		}
	}
	return out
}

// allDeploys is the global build feed across every app, newest first.
func allDeploys() []Deploy {
	rows, err := sqlDB.Query(
		`SELECT id,app,commit_sha,message,trigger,status,started FROM deploys ORDER BY started DESC LIMIT 100`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Deploy{}
	for rows.Next() {
		var d Deploy
		if rows.Scan(&d.ID, &d.App, &d.Commit, &d.Message, &d.Trigger, &d.Status, &d.Started) == nil {
			out = append(out, d)
		}
	}
	return out
}

func deployStatus(app, id string) string {
	var st string
	_ = sqlDB.QueryRow(`SELECT status FROM deploys WHERE id=? AND app=?`, id, app).Scan(&st)
	return st
}

func getEnv(app string) []EnvVar {
	rows, err := sqlDB.Query(`SELECT key,value,build FROM env_vars WHERE app=? ORDER BY key`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []EnvVar{}
	for rows.Next() {
		var e EnvVar
		var build int
		if rows.Scan(&e.Key, &e.Value, &build) == nil {
			e.Build = build != 0
			out = append(out, e)
		}
	}
	return out
}

func setEnv(app string, vars []EnvVar) error {
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM env_vars WHERE app=?`, app); err != nil {
		return err
	}
	for _, e := range vars {
		key := sanitizeEnvKey(e.Key)
		if key == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO env_vars(app,key,value,build) VALUES(?,?,?,?)`,
			app, key, e.Value, b2i(e.Build)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// sanitizeID keeps deploy ids filesystem-safe (used for log file paths).
func sanitizeID(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out = append(out, byte(r))
		}
	}
	return string(out)
}

// sanitizeEnvKey keeps env keys to valid TOML bare keys.
func sanitizeEnvKey(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out = append(out, byte(r))
		}
	}
	return string(out)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func putSession(token, userID, teamID string) {
	_, _ = sqlDB.Exec(
		`INSERT INTO sessions(token,user_id,team_id,created) VALUES(?,?,?,?)`,
		token, userID, teamID, time.Now().Unix())
}

func getSession(token string) (sess, bool) {
	var s sess
	if sqlDB.QueryRow(`SELECT user_id,team_id FROM sessions WHERE token=?`, token).
		Scan(&s.userID, &s.teamID) != nil {
		return sess{}, false
	}
	return s, true
}

func deleteSession(token string) {
	_, _ = sqlDB.Exec(`DELETE FROM sessions WHERE token=?`, token)
}

func setSessionTeam(token, teamID string) {
	_, _ = sqlDB.Exec(`UPDATE sessions SET team_id=? WHERE token=?`, teamID, token)
}
