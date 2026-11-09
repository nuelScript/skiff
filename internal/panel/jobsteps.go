package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
)

// stepsAtOnce is how many steps of one run go side by side when nothing orders them.
const stepsAtOnce = 4

// stepResult is how one step of a run went.
type stepResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // ok, failed, timeout, or skipped when a step it comes after didn't succeed
	ExitCode int    `json:"exitCode"`
	Started  int64  `json:"started,omitempty"`
	Finished int64  `json:"finished,omitempty"`
}

const stepSkipped = "skipped"

func encodeSteps(steps []config.Step) string {
	if len(steps) == 0 {
		return ""
	}
	b, _ := json.Marshal(steps)
	return string(b)
}

func decodeSteps(s string) []config.Step {
	var out []config.Step
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

// runSteps runs a pipeline's steps wave by wave in the app's image. A failed step stops only the steps that come after it; the others carry on.
func (p *Panel) runSteps(ctx context.Context, image string, steps []config.Step, settings docker.Settings) (string, string, int, []stepResult) {
	waves, err := config.StepWaves(steps)
	if err != nil {
		return err.Error(), runFailed, 0, nil
	}
	results := map[string]*stepResult{}
	outs := map[string]string{}
	var mu sync.Mutex
	for _, wave := range waves {
		sem := make(chan struct{}, stepsAtOnce)
		var wg sync.WaitGroup
		for _, s := range wave {
			r := &stepResult{Name: s.Name}
			results[s.Name] = r
			for _, a := range s.After {
				if results[a].Status != runOK {
					r.Status = stepSkipped
				}
			}
			if r.Status == stepSkipped {
				continue
			}
			wg.Add(1)
			go func(s config.Step, r *stepResult) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				st := settings
				st.Env = withEnv(settings.Env, "SKIFF_STEP", s.Name)
				r.Started = time.Now().Unix()
				out, err := p.eng.RunOnce(ctx, image, s.Command, st)
				r.Finished = time.Now().Unix()
				switch {
				case errors.Is(ctx.Err(), context.DeadlineExceeded):
					r.Status = runTimeout
				case err != nil:
					r.Status = runFailed
					var exit *exec.ExitError
					if errors.As(err, &exit) {
						r.ExitCode = exit.ExitCode()
					}
				default:
					r.Status = runOK
				}
				mu.Lock()
				outs[s.Name] = out
				mu.Unlock()
			}(s, r)
		}
		wg.Wait()
	}

	var b strings.Builder
	status, code := runOK, 0
	ordered := make([]stepResult, 0, len(steps))
	for _, wave := range waves {
		for _, s := range wave {
			r := *results[s.Name]
			ordered = append(ordered, r)
			took := ""
			if r.Finished > 0 {
				took = fmt.Sprintf(", %ds", r.Finished-r.Started)
			}
			fmt.Fprintf(&b, "── %s (%s%s)\n", s.Name, r.Status, took)
			if out := outs[s.Name]; out != "" {
				b.WriteString(strings.TrimRight(out, "\n") + "\n")
			}
			if r.Status != runOK && status == runOK {
				status, code = cmpStatus(r.Status), r.ExitCode
			}
		}
	}
	return b.String(), status, code, ordered
}

// cmpStatus is a run's status from its first unsuccessful step: a timeout stays a timeout, anything else is a failure.
func cmpStatus(step string) string {
	if step == runTimeout {
		return runTimeout
	}
	return runFailed
}

func withEnv(env map[string]string, k, v string) map[string]string {
	out := make(map[string]string, len(env)+1)
	for key, val := range env {
		out[key] = val
	}
	out[k] = v
	return out
}
