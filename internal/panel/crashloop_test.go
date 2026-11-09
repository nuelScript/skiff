package panel

import (
	"testing"
	"time"
)

func TestCrashLoopNeedsRepeatedDeaths(t *testing.T) {
	crashes.Lock()
	crashes.deaths, crashes.loops = map[string][]time.Time{}, map[string]*crashLoop{}
	crashes.Unlock()
	start := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	// A deploy stops each old container once: never a loop.
	for i := range 5 {
		if _, started := noteDeath("t", "web", "", "web-old-"+string(rune('a'+i)), 143, start); started {
			t.Fatal("one death per container shouldn't count as a crash loop")
		}
	}
	// Deaths spread wider than the window don't either.
	for i := range 3 {
		if _, started := noteDeath("t", "slow", "", "slow-1", 1, start.Add(time.Duration(i)*6*time.Minute)); started {
			t.Fatal("deaths 6 minutes apart shouldn't count")
		}
	}
	noteDeath("t", "web", "", "web-1", 1, start)
	noteDeath("t", "web", "", "web-1", 1, start.Add(time.Second))
	loop, started := noteDeath("t", "web", "", "web-1", 2, start.Add(3*time.Second))
	if !started || loop.Restarts != 3 || loop.ExitCode != 2 {
		t.Fatalf("the third death should start a loop: %+v %v", loop, started)
	}
	if _, again := noteDeath("t", "web", "", "web-1", 2, start.Add(time.Minute)); again {
		t.Error("a loop already started shouldn't start again")
	}
	if got := appCrashLoop("web"); got == nil || got.Restarts != 4 {
		t.Errorf("the app's loop: %+v", got)
	}
	if item, ok := projectAttention("web", "crashing", true, nil); !ok || item.Kind != "app.crashloop" || item.Restarts != 4 {
		t.Errorf("attention: %+v", item)
	}

	for i := range 3 {
		noteDeath("t", "web", "queue", "skiff-web-worker-queue-x", 3, start.Add(time.Duration(i)*time.Second))
	}
	if w := workerCrashLoops("web"); w["queue"].ExitCode != 3 || appCrashLoop("web").Worker != "" {
		t.Errorf("worker loops: %+v", w)
	}

	if over := settleCrashLoops(start.Add(5 * time.Minute)); len(over) != 0 {
		t.Errorf("still within the window: %+v", over)
	}
	if over := settleCrashLoops(start.Add(20 * time.Minute)); len(over) != 2 {
		t.Errorf("quiet for a whole window, both should end: %+v", over)
	}
	if appCrashLoop("web") != nil {
		t.Error("the loop should be over")
	}
}
