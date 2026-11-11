package docker

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/config"
)

// Settings are what every container Skiff starts for an app shares, whatever it's for: what it can use, what it sees and how it starts.
type Settings struct {
	Memory string // optional, e.g. "512m"; otherwise the safety cap
	CPU    string // optional, e.g. "0.5"
	// GPUs are the UUIDs of the GPUs the container gets.
	GPUs    []string
	Env     map[string]string
	Network string // optional docker network to join (for reaching managed resources by name)
	Volumes []Mount
	Options config.RunOptions
	// OOMScore decides who the kernel stops first when memory runs out: higher goes first.
	OOMScore int
	// Runtime runs the container under another OCI runtime, such as runsc for gVisor's stronger sandbox.
	Runtime string
	// Labels go on every container these settings start, such as the app's outbound policy.
	Labels map[string]string
	// Permissions are machine permissions the server's admin granted.
	Permissions config.Permissions
}

// settingsArgs is the one place Settings become docker flags, so a limit or start setting means the same for an app, its workers, its jobs and its release command.
func (e *Engine) settingsArgs(s Settings) []string {
	args := e.limitArgs()
	args = append(args, "--memory", e.memoryOr(s.Memory))
	if s.CPU != "" {
		args = append(args, "--cpus", s.CPU)
	}
	switch {
	case s.Permissions.HostNetwork:
		args = append(args, "--network", "host")
	case s.Network != "":
		args = append(args, "--network", s.Network)
	}
	args = append(args, mountArgs(s.Volumes)...)
	args = append(args, gpuArgs(s.GPUs)...)
	if s.OOMScore != 0 {
		args = append(args, "--oom-score-adj", strconv.Itoa(s.OOMScore))
	}
	if s.Runtime != "" {
		args = append(args, "--runtime", s.Runtime)
	}
	for _, k := range sortedKeys(s.Labels) {
		if s.Labels[k] != "" {
			args = append(args, "--label", k+"="+s.Labels[k])
		}
	}
	for _, k := range sortedKeys(s.Env) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, s.Env[k]))
	}
	args = append(args, s.Permissions.HardeningArgs()...)
	args = append(args, s.Permissions.DockerArgs()...)
	return append(args, s.Options.DockerArgs()...)
}

type RunSpec struct {
	Settings
	Public        bool // on all interfaces, for a box with no router in front of its apps
	Name          string
	App           string // app name, for the skiff.app route label
	Image         string
	ContainerPort int
	Labels        map[string]string
	Aliases       []string // extra names on the network, such as a Compose service name
	Command       []string // replaces the image's default command
	// UDPPort is a UDP port the app listens on, published for the router to forward to; 0 means none.
	UDPPort int
}

type Route struct {
	App         string
	Container   string
	HostPort    int
	UDPHostPort int
}

// SafetyCapPercent is how much of the server's memory an app with no limit of its own may use: enough for nearly anything, while leaving room for the other apps and for Skiff to keep running if one runs away.
const SafetyCapPercent = 75

var (
	capMu    sync.Mutex
	capCache = map[string]int64{}
)

// memoryOr is the memory limit for a container: its own, or the safety cap.
func (e *Engine) memoryOr(own string) string {
	if own != "" {
		return own
	}
	capMu.Lock()
	defer capMu.Unlock()
	total, ok := capCache[e.host]
	if !ok {
		out, err := e.command("info", "--format", "{{.MemTotal}}").Output()
		if err != nil {
			return "0" // can't tell the size of the server; leave it unlimited
		}
		total, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		capCache[e.host] = total
	}
	return strconv.FormatInt(safetyCap(total), 10)
}

func safetyCap(total int64) int64 { return total / 100 * SafetyCapPercent }

const hostPortWait = 10 * time.Second

// limitArgs caps what one container can use besides memory: processes, and the log file docker keeps for it. A server shipping logs through another driver keeps its own setup.
func (e *Engine) limitArgs() []string {
	args := []string{"--pids-limit", "4096"}
	if e.usesJSONFileLogs() {
		args = append(args, "--log-opt", "max-size=50m", "--log-opt", "max-file=3")
	}
	return args
}

func (e *Engine) usesJSONFileLogs() bool {
	out, err := e.command("info", "--format", "{{.LoggingDriver}}").Output()
	return err == nil && strings.TrimSpace(string(out)) == "json-file"
}

func (e *Engine) Run(s RunSpec) (int, error) {
	defer invalidateStates()
	_ = e.command("rm", "-f", s.Name).Run()

	out, err := e.command(e.runArgs(s)...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("docker run failed: %s", firstLine(out))
	}
	if s.Permissions.HostNetwork {
		return s.ContainerPort, nil
	}
	return e.waitHostPort(s.Name, s.ContainerPort)
}

func (e *Engine) runArgs(s RunSpec) []string {
	bind := "127.0.0.1"
	if s.Public {
		bind = "0.0.0.0"
	}
	args := []string{"run", "-d",
		"--name", s.Name,
		"--restart", "unless-stopped",
		"--label", "skiff=1",
	}
	hostNet := s.Permissions.HostNetwork
	if hostNet {
		// On the server's own network the app's port is the server's port, so there's nothing to publish.
		args = append(args, "--label", "skiff.hostnet=1")
	} else {
		args = append(args, "-p", fmt.Sprintf("%s::%d", bind, s.ContainerPort))
	}
	if s.App != "" {
		args = append(args, "--label", "skiff.app="+s.App, "--label", fmt.Sprintf("skiff.port=%d", s.ContainerPort))
	}
	if s.UDPPort > 0 {
		if !hostNet {
			args = append(args, "-p", fmt.Sprintf("%s::%d/udp", bind, s.UDPPort))
		}
		args = append(args, "--label", fmt.Sprintf("skiff.udp=%d", s.UDPPort))
	}
	for _, k := range sortedKeys(s.Labels) {
		if s.Labels[k] != "" {
			args = append(args, "--label", k+"="+s.Labels[k])
		}
	}
	// A stable name for other services on the network; docker spreads lookups across replicas.
	if s.Network != "" && !hostNet {
		if s.App != "" {
			args = append(args, "--network-alias", s.App)
		}
		for _, a := range s.Aliases {
			args = append(args, "--network-alias", a)
		}
	}
	args = append(args, "-e", fmt.Sprintf("PORT=%d", s.ContainerPort))
	args = append(args, e.settingsArgs(s.Settings)...)
	args = append(args, s.Image)
	return append(args, s.Options.Command(s.Command)...)
}

// waitHostPort allows for an app that exits straight away: a restarting container has no published port until it's running again, and one that crashes once waiting for its database can still come up.
func (e *Engine) waitHostPort(name string, containerPort int) (int, error) {
	port, err := e.HostPort(name, containerPort)
	for deadline := time.Now().Add(hostPortWait); err != nil && time.Now().Before(deadline); {
		time.Sleep(250 * time.Millisecond)
		port, err = e.HostPort(name, containerPort)
	}
	return port, err
}

// WorkerPrefix starts the name of every container of one of an app's workers; the dashboard counts a worker's running copies by it.
func WorkerPrefix(app, worker string) string { return "skiff-" + app + "-" + worker + "-" }

// WorkerSpec runs a long-lived background process with no published port, routing labels or network aliases, so nothing routes to it.
type WorkerSpec struct {
	Settings
	Name    string
	App     string
	Image   string
	Command string // run via sh -c
}

func (e *Engine) RunWorker(s WorkerSpec) error {
	_ = e.command("rm", "-f", s.Name).Run()
	args := []string{"run", "-d", "--name", s.Name, "--restart", "unless-stopped",
		"--label", "skiff.kind=worker", "--label", "skiff.worker.app=" + s.App}
	args = append(args, e.settingsArgs(s.Settings)...)
	args = append(args, s.Image, "sh", "-c", s.Command)
	if out, err := e.command(args...).CombinedOutput(); err != nil {
		return cmdErr(out, err)
	}
	return nil
}

// LabelWorkerSpec fingerprints what a worker container was started with, so one that still matches is left running.
const LabelWorkerSpec = "skiff.worker.spec"

// WorkerSpecs maps each of an app's worker containers, running or not, to its fingerprint.
func (e *Engine) WorkerSpecs(app string) (map[string]string, error) {
	out, err := e.command("ps", "-a", "--filter", "label=skiff.worker.app="+app, "--format", `{{.Names}} {{.Label "`+LabelWorkerSpec+`"}}`).Output()
	if err != nil {
		return nil, err
	}
	specs := map[string]string{}
	for _, l := range splitLines(out) {
		name, spec, _ := strings.Cut(l, " ")
		specs[name] = spec
	}
	return specs, nil
}

// ImageID is the ID an image reference points at now, or "" when there's no such image.
func (e *Engine) ImageID(ref string) string {
	out, err := e.command("image", "inspect", "--format", "{{.Id}}", ref).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// WorkerContainers lists an app's worker containers (running or not); all of them when app is "".
func (e *Engine) WorkerContainers(app string) []string {
	filter := "label=skiff.kind=worker"
	if app != "" {
		filter = "label=skiff.worker.app=" + app
	}
	out, err := e.command("ps", "-a", "--filter", filter, "--format", "{{.Names}}").Output()
	if err != nil {
		return nil
	}
	return splitLines(out)
}

func (e *Engine) EnsureNetwork(name string) error {
	if e.command("network", "inspect", name).Run() == nil {
		return nil
	}
	if out, err := e.command("network", "create", name).CombinedOutput(); err != nil {
		return fmt.Errorf("network create failed: %s", firstLine(out))
	}
	return nil
}

// NetworkAliases are the names a container answers to on a network.
func (e *Engine) NetworkAliases(container, network string) []string {
	out, err := e.command("inspect", "--format", "{{json .NetworkSettings.Networks}}", container).Output()
	if err != nil {
		return nil
	}
	var nets map[string]struct{ Aliases []string }
	if json.Unmarshal(out, &nets) != nil {
		return nil
	}
	return nets[network].Aliases
}

// ConnectNetwork attaches a container to an additional network; an already-connected container returns a benign error.
// RemoveNetwork deletes a network nothing is using any more.
func (e *Engine) RemoveNetwork(name string) error {
	out, err := e.command("network", "rm", name).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

// ConnectNetwork joins a running container to a network, under extra names there when aliases are given.
func (e *Engine) ConnectNetwork(network, container string, aliases ...string) error {
	args := []string{"network", "connect"}
	for _, a := range aliases {
		args = append(args, "--alias", a)
	}
	out, err := e.command(append(args, network, container)...).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

func (e *Engine) DisconnectNetwork(network, container string) error {
	out, err := e.command("network", "disconnect", network, container).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

// BridgeGateway is this server's address on Docker's default bridge, which containers on any network can reach.
func (e *Engine) BridgeGateway() string {
	if e.IsRemote() {
		return ""
	}
	out, err := e.command("network", "inspect", "bridge", "-f", "{{range .IPAM.Config}}{{.Gateway}} {{end}}").Output()
	if err != nil {
		return ""
	}
	for _, ip := range strings.Fields(string(out)) {
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			return ip
		}
	}
	return ""
}

// SandboxRuntime is gVisor's runtime, which runs a container against its own kernel in user space, so code in it doesn't talk to the server's kernel directly.
const SandboxRuntime = "runsc"

// HasRuntime reports whether Docker on this server knows a runtime, such as gVisor's.
func (e *Engine) HasRuntime(name string) bool {
	out, err := e.command("info", "--format", "{{json .Runtimes}}").Output()
	if err != nil {
		return false
	}
	var runtimes map[string]json.RawMessage
	return json.Unmarshal(out, &runtimes) == nil && runtimes[name] != nil
}
