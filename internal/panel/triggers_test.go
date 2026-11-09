package panel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestWebhookQueuesASignedRequest(t *testing.T) {
	openTestDB(t)
	j := jobRow{ID: "hooked", App: "a", Team: "t", Name: "hooked", Command: "true", LastOk: 1, HookSecret: "s3cret"}
	if err := putJob(j); err != nil {
		t.Fatal(err)
	}
	if got, _ := getJob(j.ID); got.HookSecret != "s3cret" {
		t.Fatalf("the secret should read back: %q", got.HookSecret)
	}
	var raw string
	_ = sqlDB.QueryRow(`SELECT hook_secret FROM jobs WHERE id=?`, j.ID).Scan(&raw)
	if raw == "" || strings.Contains(raw, "s3cret") {
		t.Errorf("the secret should be stored sealed, not %q", raw)
	}

	p := &Panel{}
	send := func(header, sig, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/hooks/jobs/"+j.ID, strings.NewReader(body))
		req.SetPathValue("id", j.ID)
		if header != "" {
			req.Header.Set(header, sig)
		}
		w := httptest.NewRecorder()
		p.handleJobHook(w, req)
		return w.Code
	}
	if code := send("X-Skiff-Signature", sign("wrong", `{"a":1}`), `{"a":1}`); code != http.StatusUnauthorized {
		t.Errorf("a wrong signature: %d", code)
	}
	if code := send("", "", `{"a":1}`); code != http.StatusUnauthorized {
		t.Errorf("no signature: %d", code)
	}
	if code := send("X-Skiff-Signature", sign("s3cret", `{"a":1}`), `{"a":1}`); code != http.StatusAccepted {
		t.Errorf("a signed request: %d", code)
	}
	if code := send("X-Hub-Signature-256", sign("s3cret", `{"b":2}`), `{"b":2}`); code != http.StatusAccepted {
		t.Errorf("GitHub's header: %d", code)
	}
	runs := listRuns(j.ID, 10)
	if len(runs) != 2 || runs[0].Status != runQueued || runs[1].Trigger != triggerWebhook {
		t.Fatalf("queued runs: %+v", runs)
	}
	var input string
	_ = sqlDB.QueryRow(`SELECT input FROM job_runs WHERE id=?`, runs[1].ID).Scan(&input)
	if input != `{"a":1}` {
		t.Errorf("the body should wait with the run: %q", input)
	}

	for range maxQueuedRuns {
		_, _ = enqueueRun(j, triggerWebhook, "")
	}
	if code := send("X-Skiff-Signature", sign("s3cret", "x"), "x"); code != http.StatusTooManyRequests {
		t.Errorf("a full queue: %d", code)
	}
	if code := send("X-Skiff-Signature", sign("s3cret", strings.Repeat("x", webhookBodyLimit+1)), strings.Repeat("x", webhookBodyLimit+1)); code != http.StatusRequestEntityTooLarge {
		t.Errorf("a body over the limit: %d", code)
	}
	putJob(jobRow{ID: "plain", App: "a", Team: "t", Name: "plain", Schedule: "* * * * *", Command: "true"})
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/jobs/plain", strings.NewReader("x"))
	req.SetPathValue("id", "plain")
	w := httptest.NewRecorder()
	p.handleJobHook(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("a job without a webhook: %d", w.Code)
	}
}

func TestNewFilesInABucket(t *testing.T) {
	files := []bucketFile{{"in/c", 30}, {"in/a", 10}, {"in/b", 20}, {"in/b2", 20}, {"in/d", 30}}
	got := newFiles(files, 20, map[string]int64{"in/a": 10, "in/b": 20})
	var keys []string
	for _, f := range got {
		keys = append(keys, f.key)
	}
	if strings.Join(keys, ",") != "in/b2,in/c,in/d" {
		t.Errorf("new files = %v", keys)
	}
}

// A large upload is dated from when it began, so it can show up after newer files were handled; it still starts a run, once.
func TestALateUploadStillStartsARun(t *testing.T) {
	now := int64(1_800_000_000)
	handled := map[string]int64{"in/small": now}
	late := []bucketFile{{"in/small", now}, {"in/big", now - 600}, {"in/ancient", now - 2*bucketLookback}}
	got := newFiles(late, now, handled)
	if len(got) != 1 || got[0].key != "in/big" {
		t.Fatalf("new files = %+v", got)
	}
	handled["in/big"] = now - 600
	if got := newFiles(late, now, handled); len(got) != 0 {
		t.Errorf("handled twice: %+v", got)
	}
	if got := newFiles([]bucketFile{{"in/small", now + 5}}, now, handled); len(got) != 1 {
		t.Errorf("a file written again should run again: %+v", got)
	}
}

func TestHandledFilesReadsTheOldList(t *testing.T) {
	got := handledFiles(jobRow{Seen: 50, SeenKeys: `["in/a","in/b"]`})
	if got["in/a"] != 50 || got["in/b"] != 50 || len(got) != 2 {
		t.Errorf("old list read as %v", got)
	}
	if got := handledFiles(jobRow{Seen: 50, SeenKeys: `{"in/c":40}`}); got["in/c"] != 40 {
		t.Errorf("new form read as %v", got)
	}
}

func TestJobTriggerChecks(t *testing.T) {
	openTestDB(t)
	for _, c := range []struct {
		schedule, bucket, prefix string
		webhook                  bool
		ok                       bool
	}{
		{"0 3 * * *", "", "", false, true},
		{"", "", "", true, true},
		{"", "", "", false, false},
		{"not cron", "", "", true, false},
		{"", "nope", "", false, false},
		{"", "", "in/", true, false},
	} {
		if got := checkJobTriggers("t", c.schedule, c.webhook, c.bucket, c.prefix) == ""; got != c.ok {
			t.Errorf("%+v: accepted = %v", c, got)
		}
	}
	env := withTriggerEnv(map[string]string{"A": "1"}, jobRow{Bucket: "media"}, triggerBucket, "in/x.csv")
	if env["A"] != "1" || env["SKIFF_TRIGGER"] != "bucket" || env["SKIFF_BUCKET"] != "media" || env["SKIFF_OBJECT_KEY"] != "in/x.csv" {
		t.Errorf("env = %v", env)
	}
}
