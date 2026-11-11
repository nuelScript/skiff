package docker

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// GPU is one of the server's NVIDIA GPUs, as nvidia-smi reports it.
type GPU struct {
	Index       int    `json:"index"`
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	MemoryTotal int64  `json:"memoryTotal"` // MiB
	MemoryUsed  int64  `json:"memoryUsed"`  // MiB
	Utilization int    `json:"utilization"` // percent
	Temperature int    `json:"temperature"` // °C
}

// gpuQuery is what nvidia-smi is asked for, in this order.
const gpuQuery = "index,uuid,name,memory.total,memory.used,utilization.gpu,temperature.gpu"

// GPUs lists the server's GPUs; none when it has no NVIDIA driver. A broken driver can leave nvidia-smi hanging, so it gets a few seconds, and an answer is reused briefly since a page and a deploy may ask at once.
func (e *Engine) GPUs() []GPU {
	if e.IsRemote() {
		return nil
	}
	gpuCache.Lock()
	defer gpuCache.Unlock()
	if time.Since(gpuCache.at) < gpuCacheFor {
		return slices.Clone(gpuCache.gpus)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu="+gpuQuery, "--format=csv,noheader,nounits").Output()
	gpuCache.gpus, gpuCache.at = nil, time.Now()
	if err == nil {
		gpuCache.gpus = parseGPUs(string(out))
	}
	return slices.Clone(gpuCache.gpus)
}

const gpuCacheFor = 2 * time.Second

var gpuCache struct {
	sync.Mutex
	gpus []GPU
	at   time.Time
}

func parseGPUs(out string) []GPU {
	var gpus []GPU
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, ",")
		if len(f) != 7 {
			continue
		}
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		num := func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n } // "[N/A]" reads as 0
		idx, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		gpus = append(gpus, GPU{Index: idx, UUID: f[1], Name: f[2], MemoryTotal: num(f[3]), MemoryUsed: num(f[4]),
			Utilization: int(num(f[5])), Temperature: int(num(f[6]))})
	}
	return gpus
}

// CanUseGPUs says whether containers here can be given GPUs: an NVIDIA driver, and Docker set up with NVIDIA's container toolkit.
func (e *Engine) CanUseGPUs() error {
	if len(e.GPUs()) == 0 {
		return fmt.Errorf("this server has no NVIDIA GPU, or its driver isn't installed (nvidia-smi doesn't answer)")
	}
	if !e.HasRuntime("nvidia") {
		return fmt.Errorf("Docker can't give containers GPUs yet: install NVIDIA's container toolkit and run nvidia-ctk runtime configure --runtime=docker")
	}
	return nil
}

// LabelGPUs is the container label naming the GPUs an app's containers were given.
const LabelGPUs = "skiff.gpus"

// gpuArgs gives a container these GPUs by UUID.
func gpuArgs(uuids []string) []string {
	if len(uuids) == 0 {
		return nil
	}
	// The quotes keep Docker from reading the comma as the start of another option.
	return []string{"--gpus", `"device=` + strings.Join(uuids, ",") + `"`, "--label", LabelGPUs + "=" + strings.Join(uuids, ",")}
}

// LabelGPUsApp names the app whose GPUs a job or one-off run is using; those containers don't carry skiff.app, which would make them routes.
const LabelGPUsApp = "skiff.gpus-app"

// gpuUnknownOwner stands for a container holding GPUs that names no app, so its GPUs still count as taken.
const gpuUnknownOwner = "another container"

// GPUsInUse maps each GPU a container was given to that container's app. Stopped containers count, so a sleeping app keeps its cards.
func (e *Engine) GPUsInUse() map[string]string {
	out, err := e.command("ps", "-a", "--filter", "label="+LabelGPUs, "--format", `{{.Label "skiff.app"}}|{{.Label "`+LabelGPUsApp+`"}}|{{.Label "`+LabelGPUs+`"}}`).Output()
	if err != nil {
		return nil
	}
	return parseGPUsInUse(string(out))
}

func parseGPUsInUse(out string) map[string]string {
	used := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		owner := cmp.Or(parts[0], parts[1], gpuUnknownOwner)
		for _, id := range strings.Split(parts[2], ",") {
			if id != "" {
				used[id] = owner
			}
		}
	}
	return used
}

// gpuClaim is a GPU picked for a container that hasn't started yet. Claims are kept in a file every Skiff process on the server shares, since deploys run apart from the panel.
type gpuClaim struct {
	App   string `json:"app"`
	GPU   string `json:"gpu"`
	Token string `json:"token"`
	Until int64  `json:"until"`
}

// ClaimGPUs picks n GPUs for app as PickGPUs does, and holds them for up to hold, or until release is called, so another deploy, job or run choosing at the same moment doesn't get the same cards before this one's container has them.
func (e *Engine) ClaimGPUs(app string, n int, hold time.Duration) (picked []string, release func(), err error) {
	release = func() {}
	if n <= 0 {
		return nil, release, nil
	}
	token := strconv.FormatInt(time.Now().UnixNano(), 36)
	var pickErr error
	err = updateGPUClaims(func(claims []gpuClaim) []gpuClaim {
		picked, pickErr = pickClaimed(app, n, e.GPUs(), e.GPUsInUse(), claims)
		if pickErr != nil {
			return claims
		}
		until := time.Now().Add(hold).Unix()
		for _, id := range picked {
			claims = append(claims, gpuClaim{App: app, GPU: id, Token: token, Until: until})
		}
		return claims
	})
	if err = cmp.Or(pickErr, err); err != nil {
		return nil, release, err
	}
	release = func() {
		_ = updateGPUClaims(func(claims []gpuClaim) []gpuClaim {
			return slices.DeleteFunc(claims, func(c gpuClaim) bool { return c.Token == token })
		})
	}
	return picked, release, nil
}

// PickFreeGPUs is PickGPUs with other apps' claims counted as in use, without claiming anything.
func (e *Engine) PickFreeGPUs(app string, n int) ([]string, error) {
	var picked []string
	var pickErr error
	err := updateGPUClaims(func(claims []gpuClaim) []gpuClaim {
		picked, pickErr = pickClaimed(app, n, e.GPUs(), e.GPUsInUse(), claims)
		return claims
	})
	return picked, cmp.Or(pickErr, err)
}

// pickClaimed is PickGPUs with other apps' claims counted as in use.
func pickClaimed(app string, n int, all []GPU, inUse map[string]string, claims []gpuClaim) ([]string, error) {
	used := maps.Clone(inUse)
	if used == nil {
		used = map[string]string{}
	}
	for _, c := range claims {
		if c.App != app && used[c.GPU] == "" {
			used[c.GPU] = c.App + " (starting)"
		}
	}
	return PickGPUs(app, n, all, used)
}

// updateGPUClaims changes the claims under a lock, dropping lapsed ones.
func updateGPUClaims(fn func([]gpuClaim) []gpuClaim) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".skiff", "gpu-claims.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	var claims []gpuClaim
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &claims)
	}
	now := time.Now().Unix()
	claims = slices.DeleteFunc(claims, func(c gpuClaim) bool { return c.Until <= now })
	b, err := json.Marshal(fn(claims))
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PickGPUs chooses n GPUs for app: the ones it already has when they're enough, so a new version takes over from the old on the same cards, and otherwise free ones. It explains when there aren't n free.
func PickGPUs(app string, n int, all []GPU, used map[string]string) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	if len(all) < n {
		return nil, fmt.Errorf("%s asks for %d GPUs and this server has %d", app, n, len(all))
	}
	var mine, free []string
	for _, g := range all {
		switch owner := used[g.UUID]; owner {
		case app:
			mine = append(mine, g.UUID)
		case "":
			free = append(free, g.UUID)
		}
	}
	picked := append(slices.Clone(mine), free...)
	if len(picked) < n {
		var busy []string
		for _, g := range all {
			if o := used[g.UUID]; o != "" && o != app {
				busy = append(busy, fmt.Sprintf("GPU %d by %s", g.Index, o))
			}
		}
		return nil, fmt.Errorf("%s asks for %d GPUs but only %d are free (%s in use)", app, n, len(picked), strings.Join(busy, ", "))
	}
	return picked[:n], nil
}
