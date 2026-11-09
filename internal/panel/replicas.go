package panel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
)

const (
	maxReadReplicas = 3
	replicaEnvVar   = "DATABASE_READ_URL"
	// replicaWALKeep caps the history a database holds back for a replica that stopped reading it, so a replica left off can't fill the disk; past it the replica has to be added again.
	replicaWALKeep = "2GB"
)

// replicaClone copies the database into an empty volume and leaves it following the original, then starts Postgres as usual. The copy lands in a side folder and PG_VERSION moves last, so an interrupted copy starts over. A database returned to an earlier time keeps the settings that took it there, which would make the copy stop and break away at that time, so they're dropped.
const replicaClone = `set -e
if [ ! -s "$PGDATA/PG_VERSION" ]; then
  until pg_isready -q -h %[1]s -p %[2]d; do sleep 2; done
  rm -rf "$PGDATA/.clone"
  pg_basebackup -h %[1]s -p %[2]d -U %[3]s -D "$PGDATA/.clone" -R -X stream -S %[4]s -c fast
  sed -i -e '/^restore_command/d' -e '/^recovery_target/d' "$PGDATA/.clone/postgresql.auto.conf"
  for f in "$PGDATA"/.clone/*; do [ "${f##*/}" = PG_VERSION ] || mv "$f" "$PGDATA"/; done
  mv "$PGDATA/.clone/PG_VERSION" "$PGDATA"/ && rmdir "$PGDATA/.clone"
fi
exec docker-entrypoint.sh "$@"`

type replicaRow struct {
	ID, DBID, Container string
	Created             int64
}

func (r replicaRow) volume() string { return r.Container + "-data" }

func (r replicaRow) slot() string { return "skiff_" + r.ID }

// readHost is the name every replica of a database answers to; Docker spreads lookups across the ones running.
func readHost(d dbRow) string { return d.Container + "-read" }

// replicasSupported is Skiff's own Postgres, outside stacks and previews.
func replicasSupported(d dbRow) bool {
	return tunesPostgres(d) && d.Stack == "" && d.PreviewApp == ""
}

func listReplicas(dbID string) []replicaRow {
	rows, err := sqlDB.Query(`SELECT id,db_id,container,created FROM db_replicas WHERE db_id=? ORDER BY created`, dbID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []replicaRow
	for rows.Next() {
		var r replicaRow
		if rows.Scan(&r.ID, &r.DBID, &r.Container, &r.Created) == nil {
			out = append(out, r)
		}
	}
	return out
}

func (p *Panel) readURL(d dbRow) string {
	return dbEngines[d.Engine].url(readHost(d), d.Port, d.Username, d.Password, d.DBName, false)
}

// Replica is a read-only copy as the dashboard shows it.
type Replica struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Created int64  `json:"created"`
}

func (p *Panel) replicaViews(d dbRow) []Replica {
	out := []Replica{}
	for _, r := range listReplicas(d.ID) {
		out = append(out, Replica{ID: r.ID, Name: r.Container, State: p.eng.State(r.Container), Created: r.Created})
	}
	return out
}

// ReplicaStatus is how far a replica has got, read from the database it follows. Phase is copying, catching-up, streaming, lost or stopped. While it copies, pg_basebackup holds the slot under its own name, so only a connection named after the replica counts.
type ReplicaStatus struct {
	ID         string  `json:"id"`
	Phase      string  `json:"phase"`
	LagBytes   int64   `json:"lagBytes"`
	LagSeconds float64 `json:"lagSeconds"`
}

// allowReplication lets replicas sign in to the database for its history and caps what it keeps for them. The settings live in its data folder, so they last across restarts.
func (p *Panel) allowReplication(ctx context.Context, d dbRow) error {
	psql := fmt.Sprintf("psql -U %s -d %s -v ON_ERROR_STOP=1 -tAc", d.Username, d.DBName)
	script := `f="$PGDATA/pg_hba.conf"
grep -q '^host replication all all' "$f" || echo 'host replication all all scram-sha-256' >> "$f"
[ "$(` + psql + ` 'SHOW max_slot_wal_keep_size')" != "-1" ] || ` + psql + ` "ALTER SYSTEM SET max_slot_wal_keep_size = '` + replicaWALKeep + `'"
` + psql + ` 'SELECT pg_reload_conf()' >/dev/null`
	var out bytes.Buffer
	if err := p.eng.Exec(ctx, d.Container, []string{"sh", "-c", script}, nil, &out); err != nil {
		return fmt.Errorf("couldn't let replicas connect: %v %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

func (p *Panel) primarySQL(ctx context.Context, d dbRow, sql string) (string, error) {
	var out bytes.Buffer
	err := p.eng.Exec(ctx, d.Container, []string{"psql", "-U", d.Username, "-d", d.DBName, "-v", "ON_ERROR_STOP=1", "-tAc", sql}, nil, &out)
	return strings.TrimSpace(out.String()), err
}

// startReplica keeps the database's history for the replica, then starts it on the database's networks. An empty volume is filled with a fresh copy first.
func (p *Panel) startReplica(d dbRow, r replicaRow) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := p.waitDatabaseReady(ctx, d); err != nil {
		return err
	}
	if err := p.allowReplication(ctx, d); err != nil {
		return err
	}
	slot := r.slot()
	if _, err := p.primarySQL(ctx, d, "SELECT pg_create_physical_replication_slot('"+slot+"', true) WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = '"+slot+"')"); err != nil {
		return fmt.Errorf("couldn't keep history for the replica: %w", err)
	}
	e := dbEngines[d.Engine]
	if _, err := p.eng.RunDatabase(docker.DBRunSpec{
		Name: r.Container, Image: d.image(), Network: d.network(),
		Volume: r.volume(), MountAt: e.mountAt,
		Env:        map[string]string{"PGDATA": e.mountAt, "PGPASSWORD": d.Password},
		Entrypoint: "sh",
		Cmd: []string{"-c", fmt.Sprintf(replicaClone, d.Host, d.Port, d.Username, slot), "skiff",
			"postgres", "-c", "cluster_name=" + r.ID},
		TunePostgres: true,
		Labels:       map[string]string{"skiff.kind": "replica", "skiff.db": d.ID, "skiff.replica": r.ID, "skiff.team": d.Team},
	}); err != nil {
		return err
	}
	go guard("publishReplica", func() { p.publishReplica(d, r) })
	return nil
}

// replicaReadyWait bounds how long a replica may take to copy the database before it's left out of the read name; a big database can take hours.
var (
	replicaReadyWait  = 24 * time.Hour
	replicaReadyEvery = 5 * time.Second
)

// publishReplica puts a replica under the shared read name once it accepts connections. Until then a client could be sent to a replica still copying, which refuses it. The first replica to be ready gives attached apps their read address.
func (p *Panel) publishReplica(d dbRow, r replicaRow) {
	deadline := time.Now().Add(replicaReadyWait)
	for !p.replicaAccepts(r) {
		if time.Now().After(deadline) || p.eng.State(r.Container) == "missing" || !replicaListed(d.ID, r.ID) {
			return
		}
		time.Sleep(replicaReadyEvery)
	}
	home := d.network()
	if !slices.Contains(p.eng.NetworkAliases(r.Container, home), readHost(d)) {
		// Docker only adds a name when a container joins a network, so it rejoins; streaming picks up again on its own.
		_ = p.eng.DisconnectNetwork(home, r.Container)
		if err := p.eng.ConnectNetwork(home, r.Container, readHost(d)); err != nil {
			log.Printf("skiff: naming replica %s: %v", r.Container, err)
			return
		}
	}
	p.linkReplica(d, r)
	for _, app := range dbAttachments(d.ID) {
		if !slices.ContainsFunc(getEnv(app), func(e EnvVar) bool { return e.Key == replicaEnvVar }) {
			_ = upsertEnvVar(app, replicaEnvVar, p.readURL(d), false)
		}
	}
}

// readyReplicas are the replicas already under the read name; one still copying joins it, and the networks of attached apps, when it's ready.
func (p *Panel) readyReplicas(d dbRow) []replicaRow {
	var out []replicaRow
	for _, r := range listReplicas(d.ID) {
		if slices.Contains(p.eng.NetworkAliases(r.Container, d.network()), readHost(d)) {
			out = append(out, r)
		}
	}
	return out
}

func (p *Panel) replicaAccepts(r replicaRow) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.eng.Exec(ctx, r.Container, []string{"pg_isready", "-q", "-h", "127.0.0.1"}, nil, io.Discard) == nil
}

func replicaListed(dbID, id string) bool {
	for _, r := range listReplicas(dbID) {
		if r.ID == id {
			return true
		}
	}
	return false
}

// linkReplica puts a replica on every network the database is reachable from, under the shared read name.
func (p *Panel) linkReplica(d dbRow, r replicaRow) {
	home := d.network()
	seen := map[string]bool{home: true}
	nets := rejoinNetworks(d, nil)
	for _, app := range dbAttachments(d.ID) {
		if src, ok := getSource(app); ok {
			nets = append(nets, appNetwork(src))
		}
	}
	for _, n := range nets {
		if seen[n] {
			continue
		}
		seen[n] = true
		if err := p.eng.EnsureNetwork(n); err != nil {
			continue
		}
		if err := p.eng.ConnectNetwork(n, r.Container, readHost(d)); err != nil && !strings.Contains(err.Error(), "already exists") {
			log.Printf("skiff: joining replica %s to %s: %v", r.Container, n, err)
		}
	}
}

// stopReplica removes the replica's container, and its copy and the history kept for it when forget is set.
func (p *Panel) stopReplica(d dbRow, r replicaRow, forget bool) {
	_ = p.eng.Remove(r.Container)
	if !forget {
		return
	}
	_ = p.eng.RemoveVolume(r.volume())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	slot := r.slot()
	// The sender can outlive the replica by a moment, and a slot in use can't be dropped.
	for range 10 {
		_, err := p.primarySQL(ctx, d, "SELECT pg_terminate_backend(active_pid) FROM pg_replication_slots WHERE slot_name = '"+slot+"' AND active_pid IS NOT NULL; SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = '"+slot+"'")
		if err == nil || p.eng.State(d.Container) != "running" {
			return
		}
		time.Sleep(time.Second)
	}
}

func (p *Panel) addReplica(d dbRow) (replicaRow, error) {
	if !replicasSupported(d) {
		return replicaRow{}, errors.New("read replicas are for Postgres databases Skiff created")
	}
	existing := listReplicas(d.ID)
	if len(existing) >= maxReadReplicas {
		return replicaRow{}, fmt.Errorf("a database can have up to %d read replicas", maxReadReplicas)
	}
	if p.eng.State(d.Container) != "running" {
		return replicaRow{}, errors.New("the database isn't running")
	}
	if err := p.roomForReplica(d); err != nil {
		return replicaRow{}, err
	}
	r := replicaRow{ID: randToken()[:8], DBID: d.ID, Created: time.Now().Unix()}
	r.Container = d.Container + "-replica-" + r.ID
	// Listed before it starts, so the watcher that waits for it to be ready finds it.
	if _, err := sqlDB.Exec(`INSERT INTO db_replicas(id,db_id,container,created) VALUES(?,?,?,?)`, r.ID, r.DBID, r.Container, r.Created); err != nil {
		return replicaRow{}, err
	}
	if err := p.startReplica(d, r); err != nil {
		p.removeReplica(d, r)
		return replicaRow{}, err
	}
	return r, nil
}

// roomForReplica checks the server has space for another full copy of the database, with room to spare.
func (p *Panel) roomForReplica(d dbRow) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := p.primarySQL(ctx, d, "SELECT sum(pg_database_size(datname)) FROM pg_database")
	if err != nil {
		return fmt.Errorf("couldn't tell how big the database is: %w", err)
	}
	size, _ := strconv.ParseInt(out, 10, 64)
	return hasRoomFor(skiffDir(), 2*size)
}

func (p *Panel) removeReplica(d dbRow, r replicaRow) {
	p.stopReplica(d, r, true)
	_, _ = sqlDB.Exec(`DELETE FROM db_replicas WHERE id=?`, r.ID)
	if len(listReplicas(d.ID)) == 0 {
		for _, app := range dbAttachments(d.ID) {
			deleteEnvVar(app, replicaEnvVar)
		}
	}
}

// resyncReplicas copies the database afresh into each replica, for when its data was replaced and the old copies follow a history that's gone.
func (p *Panel) resyncReplicas(d dbRow) {
	for _, r := range listReplicas(d.ID) {
		p.stopReplica(d, r, true)
		if err := p.startReplica(d, r); err != nil {
			log.Printf("skiff: recopying replica %s of %s: %v", r.ID, d.Name, err)
		}
	}
}

// recoverReplicas starts any replica whose container is gone; one whose volume went too is copied again. A replica still copying when Skiff stopped is watched again, to join the read name once it's ready.
func (p *Panel) recoverReplicas(d dbRow) {
	for _, r := range listReplicas(d.ID) {
		if p.eng.State(r.Container) != "missing" {
			if !slices.Contains(p.readyReplicas(d), r) {
				go guard("publishReplica", func() { p.publishReplica(d, r) })
			}
			continue
		}
		if !p.eng.VolumeExists(r.volume()) {
			p.stopReplica(d, r, true)
		}
		if err := p.startReplica(d, r); err != nil {
			log.Printf("skiff: restarting replica %s of %s: %v", r.ID, d.Name, err)
		}
	}
}

func (p *Panel) replicaStatus(d dbRow) []ReplicaStatus {
	out := []ReplicaStatus{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type seen struct {
		state, walStatus string
		bytes            int64
		secs             float64
	}
	byID := map[string]seen{}
	if p.eng.State(d.Container) == "running" {
		raw, err := p.primarySQL(ctx, d, `SELECT substr(s.slot_name, 7), CASE WHEN r.application_name = substr(s.slot_name, 7) THEN r.state ELSE '' END, COALESCE(s.wal_status, ''),
  COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), r.replay_lsn), 0)::bigint, COALESCE(EXTRACT(EPOCH FROM r.replay_lag), 0)
FROM pg_replication_slots s LEFT JOIN pg_stat_replication r ON r.pid = s.active_pid WHERE s.slot_name LIKE 'skiff\_%'`)
		if err == nil {
			for _, line := range strings.Split(raw, "\n") {
				f := strings.Split(line, "|")
				if len(f) != 5 {
					continue
				}
				b, _ := strconv.ParseInt(f[3], 10, 64)
				s, _ := strconv.ParseFloat(f[4], 64)
				byID[f[0]] = seen{state: f[1], walStatus: f[2], bytes: b, secs: s}
			}
		}
	}
	for _, r := range listReplicas(d.ID) {
		st := ReplicaStatus{ID: r.ID, Phase: "copying"}
		s, known := byID[r.ID]
		switch {
		case p.eng.State(r.Container) != "running":
			st.Phase = "stopped"
		case known && s.walStatus == "lost":
			st.Phase = "lost"
		case known && s.state == "streaming":
			st.Phase, st.LagBytes, st.LagSeconds = "streaming", s.bytes, s.secs
		case known && s.state != "":
			st.Phase = "catching-up"
		}
		out = append(out, st)
	}
	return out
}

func (p *Panel) handleReplicas(w http.ResponseWriter, r *http.Request) {
	d, ok := p.canAccessDB(r, sanitizeID(r.URL.Query().Get("id")))
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, p.replicaStatus(d))
	case http.MethodPost:
		rep, err := p.addReplica(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		p.audit(r, "database.replica.add", d.Name, rep.ID)
		writeJSON(w, p.toDatabase(d))
	case http.MethodDelete:
		id := sanitizeID(r.URL.Query().Get("replica"))
		for _, rep := range listReplicas(d.ID) {
			if rep.ID == id {
				p.removeReplica(d, rep)
				p.audit(r, "database.replica.remove", d.Name, rep.ID)
				writeJSON(w, p.toDatabase(d))
				return
			}
		}
		http.Error(w, "no such replica", http.StatusNotFound)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
