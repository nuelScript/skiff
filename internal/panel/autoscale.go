package panel

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/registry"
)

// Autoscaling closes the loop on the metrics we record: a control loop compares
// each opted-in app's recent CPU against its target and adds or retires replicas
// to hold every replica near that target, bounded by min..max. Replicas are the
// same labeled containers a manual deploy runs, so the edge router — which
// discovers backends from container labels — load-balances to them the moment
// they're up, with no restart.

const (
	autoscaleEvery     = 30 * time.Second
	autoscaleSettle    = 45 * time.Second // wait for the first resource samples
	autoscaleWindow    = 3                // minutes of CPU to average on
	autoscaleUpCool    = 60               // seconds between scale-ups (react fast)
	autoscaleDownCool  = 300              // seconds between scale-downs (retreat slowly)
	autoscaleHealthTTL = 30 * time.Second
)

// autoscaleLast records the last scale action per app so a single control loop
// (the only writer) can enforce cooldowns without a lock.
var autoscaleLast = map[string]int64{}

// recentCPU is the average total CPU% (summed across replicas) for an app over
// the last mins minutes. ok is false when there's no data to act on.
func (s *resStore) recentCPU(app string, mins int) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buckets := s.apps[app]
	if buckets == nil {
		return 0, false
	}
	cutoff := time.Now().Unix() - int64(mins)*60
	var sum float64
	var n int
	for t, b := range buckets {
		if t >= cutoff && b.N > 0 {
			sum += b.CPUSum
			n += b.N
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

func (p *Panel) autoscaleLoop() {
	time.Sleep(autoscaleSettle)
	for range time.Tick(autoscaleEvery) {
		p.autoscaleTick()
	}
}

func (p *Panel) autoscaleTick() {
	apps, err := registry.Load()
	if err != nil {
		return
	}
	for name, app := range apps {
		src, ok := getSource(name)
		if !ok || !src.Autoscale {
			continue
		}
		min, max, target := scaleBounds(src)
		cpu, ok := resStats.recentCPU(name, autoscaleWindow)
		if !ok {
			continue // no signal yet
		}
		current := len(app.Replicas)
		if current < 1 {
			current = 1
		}
		desired := int(math.Ceil(cpu / target))
		if desired < min {
			desired = min
		}
		if desired > max {
			desired = max
		}
		if desired == current {
			continue
		}

		now := time.Now().Unix()
		if desired > current {
			if now-autoscaleLast[name] < autoscaleUpCool {
				continue
			}
			if added := p.scaleUp(app, src, desired-current); added > 0 {
				autoscaleLast[name] = now
				log.Printf("autoscale %s: %d→%d replicas (cpu %.0f%%, target %.0f%%)", name, current, current+added, cpu, target)
			}
		} else {
			if now-autoscaleLast[name] < autoscaleDownCool {
				continue
			}
			if removed := p.scaleDown(name, 1); removed > 0 {
				autoscaleLast[name] = now
				log.Printf("autoscale %s: %d→%d replicas (cpu %.0f%%, target %.0f%%)", name, current, current-removed, cpu, target)
			}
		}
	}
}

// scaleBounds normalizes an app's autoscale settings to sane values.
func scaleBounds(src Source) (min, max int, target float64) {
	min, max = src.ScaleMin, src.ScaleMax
	if min < 1 {
		min = 1
	}
	if max < min {
		max = min
	}
	if max > 10 {
		max = 10
	}
	target = float64(src.ScaleCPU)
	if target < 1 {
		target = 70
	}
	return min, max, target
}

// scaleUp starts up to count new replicas from the app's current image, waits
// for each to answer, and registers it so the reaper keeps it. Returns how many
// were added; stops early on the first failure, leaving what succeeded.
func (p *Panel) scaleUp(app registry.App, src Source, count int) int {
	image := fmt.Sprintf("skiff-%s:latest", app.Name)
	if !p.eng.ImageExists(image) {
		return 0
	}
	env := map[string]string{}
	for _, e := range deployEnv(src) {
		env[e.Key] = e.Value
	}
	_ = p.eng.EnsureNetwork(dbNetwork)

	added := 0
	for i := 0; i < count; i++ {
		container := fmt.Sprintf("%s-%s", app.Name, replicaSuffix())
		hostPort, err := p.eng.Run(docker.RunSpec{
			Name:          container,
			App:           app.Name,
			Image:         image,
			ContainerPort: app.Port,
			Env:           env,
			Public:        p.eng.IsRemote(),
			Network:       dbNetwork,
		})
		if err != nil {
			break
		}
		if !healthPoll(hostPort, autoscaleHealthTTL) {
			_ = p.eng.Remove(container)
			break
		}
		// Re-load per replica so a concurrent deploy's write isn't clobbered.
		fresh, err := registry.Load()
		if err != nil {
			_ = p.eng.Remove(container)
			break
		}
		a, ok := fresh[app.Name]
		if !ok {
			_ = p.eng.Remove(container) // app was torn down mid-scale
			break
		}
		a.Replicas = append(a.Replicas, registry.Replica{Container: container, HostPort: hostPort})
		if a.Container == "" {
			a.Container, a.HostPort, a.Port = container, hostPort, app.Port
		}
		if registry.Put(a) != nil {
			_ = p.eng.Remove(container)
			break
		}
		added++
	}
	return added
}

// scaleDown retires up to count replicas (never the last one), draining each
// gracefully and dropping it from the registry.
func (p *Panel) scaleDown(app string, count int) int {
	fresh, err := registry.Load()
	if err != nil {
		return 0
	}
	a, ok := fresh[app]
	if !ok {
		return 0
	}
	removed := 0
	for i := 0; i < count && len(a.Replicas) > 1; i++ {
		last := a.Replicas[len(a.Replicas)-1]
		_ = p.eng.Stop(last.Container) // SIGTERM drains in-flight before removal
		_ = p.eng.Remove(last.Container)
		a.Replicas = a.Replicas[:len(a.Replicas)-1]
		removed++
	}
	if removed == 0 {
		return 0
	}
	a.Container, a.HostPort = a.Replicas[0].Container, a.Replicas[0].HostPort
	_ = registry.Put(a)
	return removed
}

// healthPoll returns true once the container answers HTTP on its host port (any
// status = it's listening), or false if it never does within the timeout.
func healthPoll(hostPort int, timeout time.Duration) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/", hostPort)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			_ = resp.Body.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func replicaSuffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
