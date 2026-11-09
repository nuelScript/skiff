package docker

import (
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
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

// GPUs lists the server's GPUs; none when it has no NVIDIA driver.
func (e *Engine) GPUs() []GPU {
	if e.IsRemote() {
		return nil
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu="+gpuQuery, "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	return parseGPUs(string(out))
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

// GPUsInUse maps each GPU that a running container was given to that container's app.
func (e *Engine) GPUsInUse() map[string]string {
	out, err := e.command("ps", "--filter", "label="+LabelGPUs, "--format", `{{.Label "skiff.app"}} {{.Label "`+LabelGPUs+`"}}`).Output()
	if err != nil {
		return nil
	}
	used := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		app, ids, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		for _, id := range strings.Split(ids, ",") {
			if id != "" {
				used[id] = app
			}
		}
	}
	return used
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
