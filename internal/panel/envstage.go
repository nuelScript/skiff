package panel

import (
	"sync"
	"time"
)

// envStageTable reserves a pending app name to the team staging its env, so another team can't inherit or overwrite those vars on first deploy — a cross-tenant secret-injection guard. In-memory, short TTL.
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

func (e *envStageTable) reserve(app, team string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cur, ok := e.team[app]; ok && now.Before(cur.until) && cur.team != team {
		return false
	}
	e.team[app] = envStageEntry{team: team, until: now.Add(envStageTTL)}
	return true
}

func (e *envStageTable) heldByOther(app, team string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	cur, ok := e.team[app]
	return ok && now.Before(cur.until) && cur.team != team
}

func (e *envStageTable) release(app string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.team, app)
}
