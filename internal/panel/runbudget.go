package panel

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// A run is one piece of an agent's work: the trace its calls belong to, or what it names with X-Skiff-Run. A per-run budget stops one task that loops from using the whole month's.
const runUsageKeep = 7 * 24 * time.Hour

var runIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,100}$`)

// runOf names the run a model call belongs to, or "" when it doesn't say.
func runOf(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Skiff-Run")); runIDRe.MatchString(id) {
		return id
	}
	if m := traceparentRe.FindStringSubmatch(strings.ToLower(r.Header.Get("traceparent"))); m != nil {
		return m[1]
	}
	return ""
}

func runUsed(app, run string) int64 {
	var n int64
	_ = sqlDB.QueryRow(`SELECT tokens FROM ai_run_usage WHERE app=? AND run=?`, app, run).Scan(&n)
	return n
}

// recordRunUse adds a call's tokens to its run, and tells the team the first time a run reaches its budget.
func recordRunUse(a aiApp, run string, tokens int64, now time.Time) {
	_, _ = sqlDB.Exec(`INSERT INTO ai_run_usage(app, run, tokens, updated) VALUES(?,?,?,?)
		ON CONFLICT(app, run) DO UPDATE SET tokens=tokens+excluded.tokens, updated=excluded.updated`, a.App, run, tokens, now.Unix())
	_, _ = sqlDB.Exec(`DELETE FROM ai_run_usage WHERE app=? AND updated<?`, a.App, now.Add(-runUsageKeep).Unix())
	if a.RunBudget <= 0 || runUsed(a.App, run) < a.RunBudget {
		return
	}
	res, err := sqlDB.Exec(`UPDATE ai_run_usage SET alerted=1 WHERE app=? AND run=? AND alerted=0`, a.App, run)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n == 1 {
		go dispatchAlert(alertEvent{Team: a.Team, Kind: "ai.budget", App: a.App,
			Title:  "A run of " + a.App + " reached its budget",
			Detail: fmt.Sprintf("Run %s used %d tokens, its limit. Skiff refuses that run's model calls; other runs carry on.", run, runUsed(a.App, run))})
	}
}

func setRunBudget(app string, budget int64) error {
	_, err := sqlDB.Exec(`UPDATE ai_apps SET run_budget=? WHERE app=?`, budget, app)
	return err
}
