package panel

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"
)

const (
	// A container that exits crashDeaths times within crashWindow is crash-looping. A deploy stops each old container once, so it never counts.
	crashWindow = 10 * time.Minute
	crashDeaths = 3
)

// crashLoop is an app or worker whose container keeps exiting and being restarted.
type crashLoop struct {
	Worker   string `json:"worker,omitempty"`
	Restarts int    `json:"restarts"`
	ExitCode int    `json:"exitCode"`
	Since    int64  `json:"since"`
	Last     int64  `json:"last"`
	team     string
	app      string
}

var crashes = struct {
	sync.Mutex
	deaths map[string][]time.Time // by container
	loops  map[string]*crashLoop  // by app, or app/worker
}{deaths: map[string][]time.Time{}, loops: map[string]*crashLoop{}}

func crashKey(app, worker string) string {
	if worker == "" {
		return app
	}
	return app + "/" + worker
}

// noteDeath records a container exiting and reports whether that starts a crash loop.
func noteDeath(team, app, worker, container string, code int, now time.Time) (*crashLoop, bool) {
	crashes.Lock()
	defer crashes.Unlock()
	recent := slices.DeleteFunc(crashes.deaths[container], func(t time.Time) bool { return now.Sub(t) > crashWindow })
	recent = append(recent, now)
	crashes.deaths[container] = recent
	key := crashKey(app, worker)
	if loop, ok := crashes.loops[key]; ok {
		loop.Restarts++
		loop.ExitCode, loop.Last = code, now.Unix()
		return nil, false
	}
	if len(recent) < crashDeaths {
		return nil, false
	}
	loop := &crashLoop{Worker: worker, Restarts: len(recent), ExitCode: code, Since: recent[0].Unix(), Last: now.Unix(), team: team, app: app}
	crashes.loops[key] = loop
	return loop, true
}

// settleCrashLoops ends the loops that have been quiet for a whole window, and returns them.
func settleCrashLoops(now time.Time) []crashLoop {
	crashes.Lock()
	defer crashes.Unlock()
	var over []crashLoop
	for key, loop := range crashes.loops {
		if now.Unix()-loop.Last > int64(crashWindow/time.Second) {
			over = append(over, *loop)
			delete(crashes.loops, key)
		}
	}
	for c, ts := range crashes.deaths {
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > crashWindow {
			delete(crashes.deaths, c)
		}
	}
	return over
}

func appCrashLoop(app string) *crashLoop {
	crashes.Lock()
	defer crashes.Unlock()
	if loop, ok := crashes.loops[app]; ok {
		c := *loop
		return &c
	}
	return nil
}

func workerCrashLoops(app string) map[string]crashLoop {
	crashes.Lock()
	defer crashes.Unlock()
	out := map[string]crashLoop{}
	for loop := range maps.Values(crashes.loops) {
		if loop.app == app && loop.Worker != "" {
			out[loop.Worker] = *loop
		}
	}
	return out
}

// exitMeaning explains the exit codes people see most, for alerts.
func exitMeaning(code int) string {
	switch code {
	case 137:
		return "it was killed, often for running out of memory"
	case 143:
		return "it was stopped by a signal"
	case 127:
		return "its command wasn't found"
	case 126:
		return "its command couldn't be run"
	case 0:
		return "it exited cleanly, but a service is meant to keep running"
	}
	return fmt.Sprintf("it exited with code %d", code)
}

func (p *Panel) crashLoopWatch() {
	for {
		err := p.eng.WatchDeaths(context.Background(), func(app, container string, worker bool, code int) {
			src, ok := getSource(app)
			if !ok {
				return
			}
			name := ""
			if worker {
				name = workerOf(app, container, workerNames(app))
			}
			if loop, started := noteDeath(src.Team, app, name, container, code, time.Now()); started {
				what := app
				if name != "" {
					what = "Worker " + name + " of " + app
				}
				go dispatchAlert(alertEvent{Team: src.Team, Kind: "app.crashloop", App: app,
					Title:  what + " keeps crashing",
					Detail: fmt.Sprintf("It restarted %d times in %d minutes; the last time %s. Its logs show why.", loop.Restarts, int(crashWindow/time.Minute), exitMeaning(code))})
			}
		})
		if err != nil {
			time.Sleep(oomRetryWait)
		}
	}
}

// alertCrashRecoveries tells the team when a crash loop has been quiet for a whole window.
func alertCrashRecoveries(now time.Time) {
	for _, loop := range settleCrashLoops(now) {
		what := loop.app
		if loop.Worker != "" {
			what = "Worker " + loop.Worker + " of " + loop.app
		}
		go dispatchAlert(alertEvent{Team: loop.team, Kind: "app.recovered", App: loop.app,
			Title: what + " stopped crashing", Detail: fmt.Sprintf("No restarts for %d minutes.", int(crashWindow/time.Minute))})
	}
}
