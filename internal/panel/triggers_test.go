package panel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/hooksig"
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
	post := func(h http.Header, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/hooks/jobs/"+j.ID, strings.NewReader(body))
		req.SetPathValue("id", j.ID)
		maps.Copy(req.Header, h)
		w := httptest.NewRecorder()
		p.handleJobHook(w, req)
		return w.Code
	}
	signed := func(secret, body string) http.Header {
		h := http.Header{}
		hooksig.Set(h, secret, []byte(body))
		return h
	}
	send := func(secret, body string) int { return post(signed(secret, body), body) }
	if code := send("wrong", `{"a":1}`); code != http.StatusUnauthorized {
		t.Errorf("a wrong signature: %d", code)
	}
	if code := post(nil, `{"a":1}`); code != http.StatusUnauthorized {
		t.Errorf("no signature: %d", code)
	}
	first := signed("s3cret", `{"a":1}`)
	if code := post(first, `{"a":1}`); code != http.StatusAccepted {
		t.Errorf("a signed request: %d", code)
	}
	if code := post(first, `{"a":1}`); code != http.StatusOK {
		t.Errorf("the same request again should be taken as received and not run: %d", code)
	}
	old := http.Header{}
	ts := time.Now().Add(-10 * time.Minute).Unix()
	old.Set(hooksig.TimestampHeader, strconv.FormatInt(ts, 10))
	old.Set(hooksig.SignatureHeader, hooksig.Sign("s3cret", ts, []byte(`{"c":3}`)))
	if code := post(old, `{"c":3}`); code != http.StatusUnauthorized {
		t.Errorf("a request signed ten minutes ago: %d", code)
	}
	if code := post(http.Header{"X-Skiff-Signature": {sign("s3cret", `{"c":3}`)}}, `{"c":3}`); code != http.StatusUnauthorized {
		t.Errorf("a signature of the body alone, without a time: %d", code)
	}
	gh := http.Header{"X-Hub-Signature-256": {sign("s3cret", `{"b":2}`)}, "X-Github-Delivery": {"d-1"}}
	if code := post(gh, `{"b":2}`); code != http.StatusAccepted {
		t.Errorf("GitHub's header: %d", code)
	}
	if code := post(gh, `{"b":2}`); code != http.StatusOK {
		t.Errorf("the same GitHub delivery again: %d", code)
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
	if code := send("s3cret", "x"); code != http.StatusTooManyRequests {
		t.Errorf("a full queue: %d", code)
	}
	if code := send("s3cret", strings.Repeat("x", webhookBodyLimit+1)); code != http.StatusRequestEntityTooLarge {
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
	got := newFiles(files, 20, []string{"in/b"})
	var keys []string
	for _, f := range got {
		keys = append(keys, f.key)
	}
	if strings.Join(keys, ",") != "in/b2,in/c,in/d" {
		t.Errorf("new files = %v", keys)
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
