package panel

import (
	"sync"
	"time"
)

// Env for an app that has no source yet — staged from the deploy dialog before
// the first deploy — isn't owned by any team in the database. Without a guard,
// any member of any team could stage vars under a name and have a *different*
// team inherit them on first deploy (a cross-tenant secret-injection vector).
// envStage records which team is staging each pending name so a second team can
// neither read nor overwrite that staging, and so a deploy of a name someone
// else staged starts clean instead of inheriting it. Entries are in-memory with
// a short TTL and are released once the app has a real, team-owned source.
type envStageTable struct {
	mu   sync.Mutex
	team map[string]envStageEntry
}

type envStageEntry struct {
	team  string
	until time.Time
}

const envStageTTL = 30 * time.Minute

var envStage = &envStageTable{team: map[string]envStageEntry{}}

// reserve claims a pending app name for team, refreshing the TTL. It returns
// false when a different team currently holds the name.
func (e *envStageTable) reserve(app, team string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cur, ok := e.team[app]; ok && now.Before(cur.until) && cur.team != team {
		return false
	}
	e.team[app] = envStageEntry{team: team, until: now.Add(envStageTTL)}
	return true
}

// heldByOther reports whether a different team is currently staging this name.
func (e *envStageTable) heldByOther(app, team string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	cur, ok := e.team[app]
	return ok && now.Before(cur.until) && cur.team != team
}

// release drops any reservation for app, once it has a real source.
func (e *envStageTable) release(app string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.team, app)
}
