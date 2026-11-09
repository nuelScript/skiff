package panel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	triggerWebhook = "webhook"
	triggerBucket  = "bucket"
	runQueued      = "queued"
	// maxQueuedRuns bounds how many events can wait for one job.
	maxQueuedRuns = 50
	// webhookBodyLimit keeps a webhook's body small enough to hand to the run as an environment variable.
	webhookBodyLimit = 64 << 10
	// bucketWatchLimit is how many files under a prefix a watch reads each pass, and bucketBatch how many new ones it starts at once.
	bucketWatchLimit = 5000
	bucketBatch      = 20
)

// queueWake nudges the dispatcher when a run is queued, so it starts without waiting for its next pass.
var queueWake = make(chan struct{}, 1)

func wakeQueue() {
	select {
	case queueWake <- struct{}{}:
	default:
	}
}

// checkJobTriggers checks when a job runs: on a schedule, when its webhook is called, when a file lands in a bucket, or a mix.
func checkJobTriggers(team, schedule string, webhook bool, bucket, prefix string) string {
	if schedule == "" && !webhook && bucket == "" {
		return "a job needs a schedule, a webhook or a bucket to watch"
	}
	if schedule != "" {
		if _, err := cron.ParseStandard(schedule); err != nil {
			return "invalid schedule — use cron syntax like \"0 3 * * *\""
		}
	}
	if bucket != "" {
		if _, ok := teamBucket(team, bucket); !ok {
			return "your team has no bucket named " + bucket
		}
	}
	if prefix != "" && (bucket == "" || !validObjectPrefix(prefix)) {
		return "a prefix is a folder in the bucket, like incoming/"
	}
	return ""
}

func teamBucket(team, name string) (bucketRow, bool) {
	for _, b := range allBuckets() {
		if b.Team == team && b.Name == name {
			return b, true
		}
	}
	return bucketRow{}, false
}

// withTriggerEnv tells a run what started it.
func withTriggerEnv(env map[string]string, j jobRow, trigger, input string) map[string]string {
	out := maps.Clone(env)
	if out == nil {
		out = map[string]string{}
	}
	out["SKIFF_TRIGGER"] = trigger
	switch trigger {
	case triggerWebhook:
		out["SKIFF_WEBHOOK_BODY"] = input
	case triggerBucket:
		out["SKIFF_BUCKET"] = j.Bucket
		out["SKIFF_OBJECT_KEY"] = input
	}
	return out
}

func queuedRuns(job string) int {
	var n int
	_ = sqlDB.QueryRow(`SELECT COUNT(*) FROM job_runs WHERE job=? AND status=?`, job, runQueued).Scan(&n)
	return n
}

var errQueueFull = errors.New("too many runs are waiting for this job")

// enqueueRun records a run to start when the job is free. Queued runs survive a restart.
func enqueueRun(j jobRow, trigger, input string) (int64, error) {
	if queuedRuns(j.ID) >= maxQueuedRuns {
		return 0, errQueueFull
	}
	res, err := sqlDB.Exec(`INSERT INTO job_runs(job, app, trigger, attempt, started, status, input) VALUES(?,?,?,1,?,?,?)`,
		j.ID, j.App, trigger, time.Now().Unix(), runQueued, input)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	wakeQueue()
	return id, nil
}

// queueLoop starts queued runs, one at a time per job, in the order they arrived.
func (p *Panel) queueLoop() {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		guard("queueLoop", p.dispatchQueued)
		select {
		case <-queueWake:
		case <-tick.C:
		}
	}
}

func (p *Panel) dispatchQueued() {
	rows, err := sqlDB.Query(`SELECT DISTINCT job FROM job_runs WHERE status=?`, runQueued)
	if err != nil {
		return
	}
	var jobs []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			jobs = append(jobs, id)
		}
	}
	rows.Close()
	for _, id := range jobs {
		j, ok := getJob(id)
		if !ok {
			_, _ = sqlDB.Exec(`DELETE FROM job_runs WHERE job=? AND status=?`, id, runQueued)
			continue
		}
		if !claimJob(j.ID) {
			continue
		}
		go func(j jobRow) {
			defer releaseJob(j.ID)
			for {
				var run int64
				var trigger, input string
				if sqlDB.QueryRow(`SELECT id, trigger, input FROM job_runs WHERE job=? AND status=? ORDER BY id LIMIT 1`, j.ID, runQueued).Scan(&run, &trigger, &input) != nil {
					return
				}
				guard("job:"+j.ID, func() { p.execJob(j, trigger, run, input) })
				if fresh, ok := getJob(j.ID); ok {
					j = fresh
				} else {
					return
				}
			}
		}(j)
	}
}

// signedBy checks a webhook's signature: an HMAC-SHA256 of the body as "sha256=<hex>", in Skiff's header or GitHub's.
func signedBy(r *http.Request, body []byte, secret string) bool {
	sig := r.Header.Get("X-Skiff-Signature")
	if sig == "" {
		sig = r.Header.Get("X-Hub-Signature-256")
	}
	got, ok := strings.CutPrefix(sig, "sha256=")
	if !ok || secret == "" {
		return false
	}
	want := hmac.New(sha256.New, []byte(secret))
	want.Write(body)
	raw, err := hex.DecodeString(got)
	return err == nil && hmac.Equal(raw, want.Sum(nil))
}

// handleJobHook queues a job when a correctly signed request arrives. It answers before the job runs.
func (p *Panel) handleJobHook(w http.ResponseWriter, r *http.Request) {
	j, ok := getJob(sanitizeID(r.PathValue("id")))
	if !ok || j.HookSecret == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyLimit))
	if err != nil {
		http.Error(w, "the body is over 64 KB", http.StatusRequestEntityTooLarge)
		return
	}
	if !signedBy(r, body, j.HookSecret) {
		http.Error(w, "the signature doesn't match", http.StatusUnauthorized)
		return
	}
	// GitHub checks a new webhook with a ping, which shouldn't run the job.
	if r.Header.Get("X-GitHub-Event") == "ping" {
		writeJSON(w, map[string]string{"ok": "pong"})
		return
	}
	id, err := enqueueRun(j, triggerWebhook, string(body))
	if errors.Is(err, errQueueFull) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, "couldn't queue the run", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]int64{"run": id})
}

// bucketTriggerLoop looks for new files in the buckets jobs watch.
func (p *Panel) bucketTriggerLoop() {
	for {
		guard("bucketTriggers", func() {
			for _, j := range allJobRows() {
				if j.Bucket != "" {
					p.watchBucket(j)
				}
			}
		})
		time.Sleep(30 * time.Second)
	}
}

// watchBucket queues a run for each file newer than the job last saw. The first pass only notes what's already there.
func (p *Panel) watchBucket(j jobRow) {
	b, ok := teamBucket(j.Team, j.Bucket)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := p.bucketClient(b)
	var files []bucketFile
	token := ""
	for len(files) < bucketWatchLimit {
		page, err := c.ListAll(ctx, j.Prefix, token, 1000)
		if err != nil {
			return
		}
		for _, o := range page.Objects {
			if !strings.HasSuffix(o.Key, "/") {
				files = append(files, bucketFile{o.Key, o.Modified.Unix()})
			}
		}
		if token = page.NextToken; token == "" {
			break
		}
	}
	var seenKeys []string
	_ = json.Unmarshal([]byte(j.SeenKeys), &seenKeys)
	if j.Seen == 0 {
		latest := time.Now().Unix()
		for _, f := range files {
			latest = max(latest, f.when)
		}
		saveBucketSeen(j.ID, latest, nil)
		return
	}
	fresh := newFiles(files, j.Seen, seenKeys)
	if len(fresh) == 0 {
		return
	}
	seen, keys := j.Seen, seenKeys
	for i, f := range fresh {
		if i == bucketBatch {
			break
		}
		if _, err := enqueueRun(j, triggerBucket, f.key); err != nil {
			break
		}
		if f.when != seen {
			seen, keys = f.when, nil
		}
		keys = append(keys, f.key)
	}
	saveBucketSeen(j.ID, seen, keys)
}

type bucketFile struct {
	key  string
	when int64
}

// newFiles is what arrived after the last file handled, oldest first. Listings give times to the second, so files from that same second are told apart by key.
func newFiles(files []bucketFile, seen int64, seenKeys []string) []bucketFile {
	var fresh []bucketFile
	for _, f := range files {
		if f.when > seen || f.when == seen && !slices.Contains(seenKeys, f.key) {
			fresh = append(fresh, f)
		}
	}
	slices.SortStableFunc(fresh, func(a, b bucketFile) int { return int(a.when - b.when) })
	return fresh
}

func saveBucketSeen(job string, seen int64, keys []string) {
	raw, _ := json.Marshal(keys)
	if keys == nil {
		raw = nil
	}
	_, _ = sqlDB.Exec(`UPDATE jobs SET bucket_seen=?, bucket_seen_keys=? WHERE id=?`, seen, string(raw), job)
}
