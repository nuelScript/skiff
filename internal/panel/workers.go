package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
)

type Worker struct {
	ID       string `json:"id"`
	App      string `json:"app"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Replicas int    `json:"replicas"`
	Running  int    `json:"running"`
	Created  int64  `json:"created"`
	// CrashLoop says the worker keeps exiting and being restarted.
	CrashLoop *crashLoop `json:"crashLoop,omitempty"`
}

type workerRow struct {
	ID, App, Team, Name, Command string
	Replicas                     int
	Created                      int64
}

const workerCols = `id,app,team,name,command,replicas,created`

func scanWorker(row interface{ Scan(...any) error }) (workerRow, bool) {
	var w workerRow
	if row.Scan(&w.ID, &w.App, &w.Team, &w.Name, &w.Command, &w.Replicas, &w.Created) != nil {
		return workerRow{}, false
	}
	return w, true
}

func getWorker(id string) (workerRow, bool) {
	return scanWorker(sqlDB.QueryRow(`SELECT `+workerCols+` FROM workers WHERE id=?`, id))
}

func listWorkers(app string) []workerRow {
	rows, err := sqlDB.Query(`SELECT `+workerCols+` FROM workers WHERE app=? ORDER BY name`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []workerRow
	for rows.Next() {
		if w, ok := scanWorker(rows); ok {
			out = append(out, w)
		}
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

func putWorker(w workerRow) error {
	_, err := sqlDB.Exec(`INSERT INTO workers(`+workerCols+`) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(app,name) DO UPDATE SET command=excluded.command, replicas=excluded.replicas`,
		w.ID, w.App, w.Team, w.Name, w.Command, w.Replicas, w.Created)
	return err
}

func toWorker(w workerRow, containers []string) Worker {
	prefix := docker.WorkerPrefix(w.App, w.Name)
	running := 0
	for _, c := range containers {
		if replicaOfWorker(c, prefix) {
			running++
		}
	}
	out := Worker{
		ID: w.ID, App: w.App, Name: w.Name, Command: w.Command,
		Replicas: w.Replicas, Running: running, Created: w.Created,
	}
	if loop, ok := workerCrashLoops(w.App)[w.Name]; ok {
		out.CrashLoop = &loop
	}
	return out
}

var workerLocks sync.Map // app → *sync.Mutex

// reconcileWorkers brings the app's worker containers in line with its rows. A container started from the same image, command and settings as its worker asks for now is left running, so changing one worker doesn't restart the others. The rest stop gracefully before their replacements start. A deploy and a change to a worker can both ask at once, so one runs at a time per app.
func (p *Panel) reconcileWorkers(app string) {
	mu, _ := workerLocks.LoadOrStore(app, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	src, ok := getSource(app)
	if !ok {
		return
	}
	existing, err := p.eng.WorkerSpecs(app)
	if err != nil {
		log.Printf("skiff: workers of %s left as they are: %v", app, err)
		return
	}
	image := "skiff-" + app + ":latest"
	imageID := p.eng.ImageID(image)
	if imageID == "" {
		p.stopWorkers(slices.Collect(maps.Keys(existing)))
		return
	}
	settings, err := p.appSettings(src)
	if err != nil {
		log.Printf("skiff: workers of %s left as they are: %v", app, err)
		return
	}
	keep := map[string]bool{}
	type start struct{ name, command, spec string }
	var starts []start
	for _, w := range listWorkers(app) {
		spec := workerFingerprint(imageID, w.Command, settings)
		prefix := docker.WorkerPrefix(app, w.Name)
		reps := max(w.Replicas, 1)
		for _, name := range slices.Sorted(maps.Keys(existing)) {
			if reps > 0 && !keep[name] && replicaOfWorker(name, prefix) && existing[name] == spec {
				keep[name] = true
				reps--
			}
		}
		for range reps {
			starts = append(starts, start{prefix + replicaSuffix(), w.Command, spec})
		}
	}
	var stale []string
	for name := range existing {
		if !keep[name] {
			stale = append(stale, name)
		}
	}
	p.stopWorkers(stale)
	for _, s := range starts {
		st := settings
		st.Labels = maps.Clone(settings.Labels)
		if st.Labels == nil {
			st.Labels = map[string]string{}
		}
		st.Labels[docker.LabelWorkerSpec] = s.spec
		if err := p.eng.RunWorker(docker.WorkerSpec{Settings: st, Name: s.name, App: app, Image: image, Command: s.command}); err != nil {
			log.Printf("skiff: starting worker %s: %v", s.name, err)
		}
	}
}

// workerFingerprint is what a worker container is started with, in short.
func workerFingerprint(imageID, command string, s docker.Settings) string {
	sum := sha256.Sum256([]byte(imageID + "\x00" + command + "\x00" + fmt.Sprint(s)))
	return hex.EncodeToString(sum[:8])
}

// replicaOfWorker says whether a container belongs to the worker with this name prefix: its prefix followed by a replica suffix alone, so a worker named queue doesn't claim queue-high's containers.
func replicaOfWorker(container, prefix string) bool {
	rest, ok := strings.CutPrefix(container, prefix)
	return ok && rest != "" && !strings.Contains(rest, "-")
}

// stopWorkers gives each worker the usual grace period to finish what it's doing, all at once, then removes it.
func (p *Panel) stopWorkers(names []string) {
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Go(func() {
			_ = p.eng.Stop(name)
			_ = p.eng.Remove(name)
		})
	}
	wg.Wait()
}

func (p *Panel) handleWorkers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app := sanitizeName(r.URL.Query().Get("app"))
		if !p.canAccess(r, app) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		containers := p.eng.WorkerContainers(app)
		out := []Worker{}
		for _, wk := range listWorkers(app) {
			out = append(out, toWorker(wk, containers))
		}
		writeJSON(w, out)

	case http.MethodPost:
		app := sanitizeName(r.URL.Query().Get("app"))
		if !p.canAccess(r, app) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if workLocked(app) {
			http.Error(w, tomlOwned, http.StatusBadRequest)
			return
		}
		var body struct {
			Name, Command string
			Replicas      int
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		wk, problem := newWorker(app, body.Name, body.Command, body.Replicas)
		if problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		if err := putWorker(wk); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.audit(r, "worker.set", app, wk.Name)
		go p.reconcileWorkers(app)
		// re-read the stored row: on a conflict update the id differs from the one we submitted.
		writeJSON(w, toWorker(mustWorker(app, wk.Name, wk), p.eng.WorkerContainers(app)))

	case http.MethodDelete:
		wk, ok := getWorker(sanitizeID(r.URL.Query().Get("id")))
		if !ok || !p.canAccess(r, wk.App) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if workLocked(wk.App) {
			http.Error(w, tomlOwned, http.StatusBadRequest)
			return
		}
		_, _ = sqlDB.Exec(`DELETE FROM workers WHERE id=?`, wk.ID)
		p.audit(r, "worker.delete", wk.App, wk.Name)
		go p.reconcileWorkers(wk.App)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// newWorker checks a worker's settings, the same for the dashboard and the API; replicas are kept between 1 and 10.
func newWorker(app, name, command string, replicas int) (workerRow, string) {
	name, command = sanitizeName(name), strings.TrimSpace(command)
	if name == "" {
		return workerRow{}, "a worker name is required"
	}
	if command == "" {
		return workerRow{}, "a command is required"
	}
	src, _ := getSource(app)
	return workerRow{
		ID: randToken()[:12], App: app, Team: src.Team, Name: name,
		Command: command, Replicas: min(max(replicas, 1), 10), Created: time.Now().Unix(),
	}, ""
}

func mustWorker(app, name string, fallback workerRow) workerRow {
	if w, ok := scanWorker(sqlDB.QueryRow(`SELECT `+workerCols+` FROM workers WHERE app=? AND name=?`, app, name)); ok {
		return w
	}
	return fallback
}

// tomlOwned explains why a CLI app's workers and jobs can't be changed here: its skiff.toml lists them, and the next deploy would put them back.
const tomlOwned = "this app's workers and jobs are set by its skiff.toml, so change them there and run `skiff deploy`"

// workLocked reports whether the app's skiff.toml decides its workers and jobs.
func workLocked(app string) bool {
	src, ok := getSource(app)
	return ok && src.Origin == originCLI
}

// syncTomlWork copies the workers and jobs a CLI app's skiff.toml lists, read off its running container, so the dashboard shows them and the jobs run on schedule. An app deployed by an older CLI has no such labels and keeps what it has.
func (p *Panel) syncTomlWork(src Source, labels map[string]string) {
	if raw, ok := labels["skiff.workers"]; ok {
		var want []config.Worker
		if json.Unmarshal([]byte(raw), &want) == nil {
			setTomlWorkers(src, want)
		}
	}
	if raw, ok := labels["skiff.jobs"]; ok {
		var want []config.Job
		if json.Unmarshal([]byte(raw), &want) == nil {
			p.setTomlJobs(src, want)
		}
	}
}

func setTomlWorkers(src Source, want []config.Worker) {
	keep := map[string]bool{}
	for _, w := range want {
		keep[w.Name] = true
		_, _ = sqlDB.Exec(`INSERT INTO workers(`+workerCols+`) VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(app,name) DO UPDATE SET command=excluded.command, replicas=excluded.replicas`,
			randToken()[:12], src.App, src.Team, w.Name, w.Command, max(w.Replicas, 1), time.Now().Unix())
	}
	for _, w := range listWorkers(src.App) {
		if !keep[w.Name] {
			_, _ = sqlDB.Exec(`DELETE FROM workers WHERE id=?`, w.ID)
		}
	}
}

// setTomlJobs keeps each job's id, and so when it last ran, while its schedule or command changes.
func (p *Panel) setTomlJobs(src Source, want []config.Job) {
	have := map[string]jobRow{}
	for _, j := range listJobRows(src.App) {
		have[j.Name] = j
	}
	for _, j := range want {
		if old, ok := have[j.Name]; ok {
			delete(have, j.Name)
			if old.Schedule != j.Schedule || old.Command != j.Command || old.Timeout != j.TimeoutSeconds() || old.Retries != j.Retries || old.CatchUp != j.CatchUp ||
				encodeSteps(old.Steps) != encodeSteps(j.Steps) {
				_, _ = sqlDB.Exec(`UPDATE jobs SET schedule=?, command=?, timeout=?, retries=?, catch_up=?, steps=? WHERE id=?`,
					j.Schedule, j.Command, j.TimeoutSeconds(), j.Retries, j.CatchUp, encodeSteps(j.Steps), old.ID)
			}
			setTomlTriggers(old, j)
			continue
		}
		row := jobRow{ID: randToken()[:12], App: src.App, Team: src.Team, Name: j.Name, Schedule: j.Schedule, Command: j.Command, LastOk: 1, Created: time.Now().Unix(),
			Timeout: j.TimeoutSeconds(), Retries: j.Retries, CatchUp: j.CatchUp, Bucket: j.Bucket, Prefix: j.Prefix, Steps: j.Steps}
		if j.Webhook {
			row.HookSecret = randToken()
		}
		_ = putJob(row)
	}
	for _, j := range have {
		p.deleteJob(j.ID)
	}
}

// setTomlTriggers keeps a webhook's secret while skiff.toml keeps the webhook, and starts a bucket watch afresh when its bucket or prefix changes.
func setTomlTriggers(old jobRow, j config.Job) {
	switch {
	case j.Webhook && old.HookSecret == "":
		if secret, err := sealHookSecret(randToken()); err == nil {
			_, _ = sqlDB.Exec(`UPDATE jobs SET hook_secret=? WHERE id=?`, secret, old.ID)
		}
	case !j.Webhook && old.HookSecret != "":
		_, _ = sqlDB.Exec(`UPDATE jobs SET hook_secret='' WHERE id=?`, old.ID)
	}
	if old.Bucket != j.Bucket || old.Prefix != j.Prefix {
		_, _ = sqlDB.Exec(`UPDATE jobs SET bucket=?, bucket_prefix=?, bucket_seen=0, bucket_seen_keys='' WHERE id=?`, j.Bucket, j.Prefix, old.ID)
	}
}
