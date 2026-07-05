package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

const dbNetwork = "skiff"

// dbEngine describes a supported database image and how to provision, address,
// and open a shell into it.
type dbEngine struct {
	image   string
	port    int
	mountAt string
	envVar  string // env var injected into an attached app
	label   string
	// container builds the run-time env + command for a fresh instance.
	container func(pass, dbname string) (map[string]string, []string)
	url       func(host string, port int, user, pass, dbname string) string
	shell     func(pass, dbname string) []string
}

var dbEngines = map[string]dbEngine{
	"postgres": {
		image: "postgres:16-alpine", port: 5432, mountAt: "/var/lib/postgresql/data",
		envVar: "DATABASE_URL", label: "PostgreSQL",
		container: func(pass, dbname string) (map[string]string, []string) {
			return map[string]string{"POSTGRES_USER": "skiff", "POSTGRES_PASSWORD": pass, "POSTGRES_DB": dbname}, nil
		},
		url: func(host string, port int, user, pass, dbname string) string {
			// sslmode=disable: it's a private-network hop, and some drivers (Go's
			// lib/pq) otherwise default to requiring TLS the container doesn't serve.
			return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, pass, host, port, dbname)
		},
		shell: func(pass, dbname string) []string {
			return []string{"sh", "-c", fmt.Sprintf("PGPASSWORD=%s exec psql -U skiff -d %s", pass, dbname)}
		},
	},
	"redis": {
		image: "redis:7-alpine", port: 6379, mountAt: "/data",
		envVar: "REDIS_URL", label: "Redis",
		container: func(pass, _ string) (map[string]string, []string) {
			return nil, []string{"redis-server", "--requirepass", pass, "--appendonly", "yes"}
		},
		url: func(host string, port int, _, pass, _ string) string {
			// The "default" ACL user is what requirepass sets the password for; the
			// empty-username form (redis://:pass@) is rejected as WRONGPASS on Redis 7.
			return fmt.Sprintf("redis://default:%s@%s:%d", pass, host, port)
		},
		shell: func(pass, _ string) []string {
			return []string{"sh", "-c", fmt.Sprintf("exec redis-cli -a %s", pass)}
		},
	},
}

// Database is a managed data store, plus computed fields the dashboard needs.
type Database struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Engine   string   `json:"engine"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Created  int64    `json:"created"`
	State    string   `json:"state"`    // computed: running / exited / missing
	URL      string   `json:"url"`      // computed: connection string
	Attached []string `json:"attached"` // computed: apps it's wired into
}

type dbRow struct {
	ID, Team, Name, Engine, Container, Host string
	Port                                    int
	Username, Password, DBName              string
	Created                                 int64
}

const dbCols = `id,team,name,engine,container,host,port,username,password,dbname,created`

func scanDB(row interface{ Scan(...any) error }) (dbRow, bool) {
	var d dbRow
	if row.Scan(&d.ID, &d.Team, &d.Name, &d.Engine, &d.Container, &d.Host, &d.Port, &d.Username, &d.Password, &d.DBName, &d.Created) != nil {
		return dbRow{}, false
	}
	return d, true
}

func getDB(id string) (dbRow, bool) {
	return scanDB(sqlDB.QueryRow(`SELECT `+dbCols+` FROM databases WHERE id=?`, id))
}

func listDBs(team string) []dbRow {
	rows, err := sqlDB.Query(`SELECT `+dbCols+` FROM databases WHERE team=? ORDER BY created DESC`, team)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []dbRow
	for rows.Next() {
		if d, ok := scanDB(rows); ok {
			out = append(out, d)
		}
	}
	return out
}

func putDB(d dbRow) error {
	_, err := sqlDB.Exec(`INSERT INTO databases(`+dbCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Team, d.Name, d.Engine, d.Container, d.Host, d.Port, d.Username, d.Password, d.DBName, d.Created)
	return err
}

func deleteDBRow(id string) {
	_, _ = sqlDB.Exec(`DELETE FROM databases WHERE id=?`, id)
	_, _ = sqlDB.Exec(`DELETE FROM db_attachments WHERE db_id=?`, id)
}

func dbAttachments(id string) []string {
	rows, err := sqlDB.Query(`SELECT app FROM db_attachments WHERE db_id=? ORDER BY app`, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var app string
		if rows.Scan(&app) == nil {
			out = append(out, app)
		}
	}
	return out
}

func attachDB(id, app, varName string) error {
	_, err := sqlDB.Exec(`INSERT INTO db_attachments(db_id,app,var) VALUES(?,?,?)
		ON CONFLICT(db_id,app) DO UPDATE SET var=excluded.var`, id, app, varName)
	return err
}

func detachDB(id, app string) (string, bool) {
	var v string
	if sqlDB.QueryRow(`SELECT var FROM db_attachments WHERE db_id=? AND app=?`, id, app).Scan(&v) != nil {
		return "", false
	}
	_, _ = sqlDB.Exec(`DELETE FROM db_attachments WHERE db_id=? AND app=?`, id, app)
	return v, true
}

func upsertEnvVar(app, key, value string, build bool) error {
	_, err := sqlDB.Exec(`INSERT INTO env_vars(app,key,value,build) VALUES(?,?,?,?)
		ON CONFLICT(app,key) DO UPDATE SET value=excluded.value, build=excluded.build`, app, key, value, b2i(build))
	return err
}

func deleteEnvVar(app, key string) {
	_, _ = sqlDB.Exec(`DELETE FROM env_vars WHERE app=? AND key=?`, app, key)
}

func (p *Panel) toDatabase(d dbRow) Database {
	e := dbEngines[d.Engine]
	return Database{
		ID: d.ID, Name: d.Name, Engine: d.Engine, Host: d.Host, Port: d.Port,
		Created:  d.Created,
		State:    p.eng.State(d.Container),
		URL:      e.url(d.Host, d.Port, d.Username, d.Password, d.DBName),
		Attached: dbAttachments(d.ID),
	}
}

func (p *Panel) canAccessDB(r *http.Request, id string) (dbRow, bool) {
	d, ok := getDB(id)
	if !ok || d.Team == "" {
		return dbRow{}, false
	}
	s, ok := p.session(r)
	if !ok {
		return dbRow{}, false
	}
	if _, member := p.auth.Role(s.userID, d.Team); !member {
		return dbRow{}, false
	}
	return d, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (p *Panel) handleDatabases(w http.ResponseWriter, r *http.Request) {
	team := p.teamID(r)
	if team == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		out := []Database{}
		for _, d := range listDBs(team) {
			out = append(out, p.toDatabase(d))
		}
		writeJSON(w, out)

	case http.MethodPost:
		var body struct{ Engine, Name string }
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		e, ok := dbEngines[body.Engine]
		if !ok {
			http.Error(w, "unknown engine", http.StatusBadRequest)
			return
		}
		name := sanitizeName(body.Name)
		if name == "" {
			http.Error(w, "invalid name", http.StatusBadRequest)
			return
		}
		id := randToken()[:12]
		container := "skiff-db-" + id
		pass := randToken()
		env, cmd := e.container(pass, name)
		if err := p.eng.EnsureNetwork(dbNetwork); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := p.eng.RunDatabase(docker.DBRunSpec{
			Name: container, Image: e.image, Network: dbNetwork,
			Volume: container + "-data", MountAt: e.mountAt, Env: env, Cmd: cmd,
			Labels: map[string]string{"skiff.kind": "database", "skiff.db": id, "skiff.team": team},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user, dbname := "", ""
		if body.Engine == "postgres" {
			user, dbname = "skiff", name
		}
		d := dbRow{
			ID: id, Team: team, Name: name, Engine: body.Engine, Container: container,
			Host: container, Port: e.port, Username: user, Password: pass, DBName: dbname,
			Created: time.Now().Unix(),
		}
		if err := putDB(d); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, p.toDatabase(d))

	case http.MethodDelete:
		id := sanitizeID(r.URL.Query().Get("id"))
		d, ok := p.canAccessDB(r, id)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		for _, app := range dbAttachments(id) {
			if v, ok := detachDB(id, app); ok {
				deleteEnvVar(app, v)
			}
		}
		_ = p.eng.Stop(d.Container)
		_ = p.eng.Remove(d.Container)
		_ = p.eng.RemoveVolume(d.Container + "-data")
		deleteDBRow(id)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDatabaseAttach wires a database into an app (POST) or unwires it (DELETE)
// by injecting/removing the connection URL in the app's environment. The app
// picks it up on its next deploy.
func (p *Panel) handleDatabaseAttach(w http.ResponseWriter, r *http.Request) {
	id := sanitizeID(r.URL.Query().Get("id"))
	d, ok := p.canAccessDB(r, id)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || src.Team != d.Team {
		http.Error(w, "unknown app", http.StatusNotFound)
		return
	}
	e := dbEngines[d.Engine]
	switch r.Method {
	case http.MethodPost:
		url := e.url(d.Host, d.Port, d.Username, d.Password, d.DBName)
		if err := upsertEnvVar(app, e.envVar, url, false); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = attachDB(id, app, e.envVar)
		writeJSON(w, p.toDatabase(d))
	case http.MethodDelete:
		if v, ok := detachDB(id, app); ok {
			deleteEnvVar(app, v)
		}
		writeJSON(w, p.toDatabase(d))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleDBShell(w http.ResponseWriter, r *http.Request) {
	id := sanitizeID(r.URL.Query().Get("db"))
	d, ok := p.canAccessDB(r, id)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	e := dbEngines[d.Engine]
	p.serveContainerShell(w, r, d.Container, e.shell(d.Password, d.DBName))
}
