package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestApprovalsWaitForAPerson(t *testing.T) {
	openTestDB(t)
	a := newTestAgent(t, "bot")
	call := func(method, path, body string) (int, approval) {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+a.token)
		w := httptest.NewRecorder()
		(&Panel{}).aiGateway(w, r)
		var out approval
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	code, req := call(http.MethodPost, "/_skiff/approvals", `{"action":"drop the orders table","detail":"it's been migrated"}`)
	if code != http.StatusCreated || req.Status != approvalPending || req.ID == 0 {
		t.Fatalf("ask: %d %+v", code, req)
	}
	path := "/_skiff/approvals/" + strconv.FormatInt(req.ID, 10)
	if _, got := call(http.MethodGet, path, ""); got.Status != approvalPending {
		t.Errorf("before an answer: %+v", got)
	}

	// The agent waits; a person answers meanwhile.
	go func() {
		time.Sleep(1200 * time.Millisecond)
		_ = answerApproval("bot", req.ID, true, "go ahead", "Ada", time.Now())
	}()
	start := time.Now()
	_, got := call(http.MethodGet, path+"?wait=10", "")
	if got.Status != approvalApproved || got.Note != "go ahead" || got.By != "Ada" || time.Since(start) > 5*time.Second {
		t.Errorf("after waiting: %+v in %s", got, time.Since(start))
	}
	if err := answerApproval("bot", req.ID, false, "", "Bob", time.Now()); err == nil {
		t.Error("an answered request was answered again")
	}

	// Nobody answering for a day denies it as expired.
	_, old := call(http.MethodPost, "/_skiff/approvals", `{"action":"email every customer"}`)
	if a, _ := getApproval("bot", old.ID, time.Now().Add(approvalExpire+time.Minute)); a.Status != approvalExpired {
		t.Errorf("after a day: %+v", a)
	}

	// Stopping the agent denies what's waiting.
	_, waiting := call(http.MethodPost, "/_skiff/approvals", `{"action":"restart the database"}`)
	denyPendingApprovals("bot", "The agent was stopped.", time.Now())
	if a, _ := getApproval("bot", waiting.ID, time.Now()); a.Status != approvalDenied || a.Note != "The agent was stopped." {
		t.Errorf("after stopping: %+v", a)
	}

	for _, bad := range []string{`{}`, `{"action":""}`, `{"action":"` + strings.Repeat("x", 201) + `"}`, `not json`} {
		if code, _ := call(http.MethodPost, "/_skiff/approvals", bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
}
