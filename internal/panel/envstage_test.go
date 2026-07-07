package panel

import (
	"testing"
	"time"
)

func TestEnvStageReservation(t *testing.T) {
	e := &envStageTable{team: map[string]envStageEntry{}}
	now := time.Unix(1_700_000_000, 0)

	// First team to stage a pending name reserves it.
	if !e.reserve("api", "teamA", now) {
		t.Fatal("teamA could not reserve a free name")
	}
	// A second team can neither re-reserve nor is it treated as the holder.
	if e.reserve("api", "teamB", now) {
		t.Fatal("teamB reserved a name teamA already holds")
	}
	if !e.heldByOther("api", "teamB", now) {
		t.Fatal("name should read as held by another team for teamB")
	}
	if e.heldByOther("api", "teamA", now) {
		t.Fatal("the holding team must not see its own name as foreign")
	}

	// The reservation expires after the TTL.
	if e.heldByOther("api", "teamB", now.Add(envStageTTL+time.Second)) {
		t.Fatal("reservation should have expired")
	}

	// Releasing frees the name immediately for anyone.
	e.reserve("web", "teamA", now)
	e.release("web")
	if !e.reserve("web", "teamB", now) {
		t.Fatal("teamB could not reserve after release")
	}
}
