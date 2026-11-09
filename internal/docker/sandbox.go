package docker

import (
	"archive/tar"
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	SandboxLabel   = "skiff.sandbox"
	sandboxWorkdir = "/work"
	// SandboxMaxFile and SandboxMaxFiles bound what comes back from a sandbox, so a runaway can't fill the panel's memory.
	SandboxMaxFile   = 8 << 20
	SandboxMaxFiles  = 16 << 20
	sandboxMaxOutput = 256 << 10
	// sandboxPullTimeout is how long a sandbox may wait for its image the first time, apart from its own timeout.
	sandboxPullTimeout = 10 * time.Minute
	// SandboxMaxDisk bounds what a sandbox writes, in its working directory and the rest of its file system. Docker has no size limit that works everywhere, so it's measured while the command runs and a sandbox past it is stopped.
	SandboxMaxDisk = 2 << 30
)

// SandboxNetwork is where sandboxes with the internet run. Its containers can't reach one another, so one team's sandbox never reaches another's, and as one of Skiff's networks the outbound limits on a sandbox's label apply.
const SandboxNetwork = "skiff-sandboxes"

// EnsureSandboxNetwork makes SandboxNetwork if it isn't there yet.
func (e *Engine) EnsureSandboxNetwork() error {
	if e.command("network", "inspect", SandboxNetwork).Run() == nil {
		return nil
	}
	if out, err := e.command("network", "create", "--opt", "com.docker.network.bridge.enable_icc=false", SandboxNetwork).CombinedOutput(); err != nil {
		return fmt.Errorf("network create failed: %s", firstLine(out))
	}
	return nil
}

// ErrSandboxDisk is returned for a sandbox stopped for writing more than SandboxMaxDisk.
var ErrSandboxDisk = fmt.Errorf("the sandbox wrote more than %d GB, so it was stopped", SandboxMaxDisk>>30)

var sandboxDiskEvery = 2 * time.Second

// SandboxSpec is a throwaway container: files go into its working directory, the command runs, and what's left there comes back.
type SandboxSpec struct {
	App      string // who asked, as a label, so they can all be stopped at once
	Image    string
	Command  string
	Files    map[string][]byte
	Internet bool // otherwise it has no network at all
	// Network is where a sandbox with the internet runs, and Labels go on its container, so outbound limits reach it; empty is Docker's default network.
	Network string
	Labels  map[string]string
	Memory  string
	CPU     string
	ShmSize string // shared memory, which browsers need more of than Docker's 64 MB
	Runtime string // another OCI runtime, such as gVisor's
}

type SandboxResult struct {
	ExitCode  int
	Output    string
	Truncated bool
	Files     map[string][]byte
	Skipped   []string // files left out for size
}

// SandboxPath checks a path the caller gave for inside the working directory.
func SandboxPath(p string) (string, bool) {
	clean := path.Clean(strings.TrimPrefix(p, "./"))
	if p == "" || path.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// Sandbox runs spec in a new container with no privileges and removes it afterwards, whatever happens.
func (e *Engine) Sandbox(ctx context.Context, spec SandboxSpec) (SandboxResult, error) {
	var id [6]byte
	_, _ = rand.Read(id[:])
	name := "skiff-sbx-" + hex.EncodeToString(id[:])
	network := "none"
	if spec.Internet {
		network = cmp.Or(spec.Network, "bridge")
	}
	// The working directory is a volume of its own: under gVisor, writes elsewhere stay in the sandbox's memory, out of reach of copying them back.
	work := name + "-work"
	args := []string{"create", "--name", name, "--label", SandboxLabel + "=" + spec.App,
		"--network", network, "--workdir", sandboxWorkdir, "--mount", "type=volume,src=" + work + ",dst=" + sandboxWorkdir,
		"--memory", spec.Memory, "--cpus", spec.CPU, "--pids-limit", "256",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--log-driver", "none"}
	for _, k := range sortedKeys(spec.Labels) {
		args = append(args, "--label", k+"="+spec.Labels[k])
	}
	if spec.ShmSize != "" {
		args = append(args, "--shm-size", spec.ShmSize)
	}
	if spec.Runtime != "" {
		args = append(args, "--runtime", spec.Runtime)
	}
	args = append(args, spec.Image, "sh", "-c", spec.Command)
	defer func() {
		_ = e.command("rm", "-f", name).Run()
		_ = e.command("volume", "rm", "-f", work).Run()
	}()
	if out, err := e.contextCommand(ctx, "volume", "create", "--label", SandboxLabel+"="+spec.App, work).CombinedOutput(); err != nil {
		return SandboxResult{}, fmt.Errorf("couldn't create the sandbox's working directory: %s", firstLine(out))
	}
	// A first pull of a large image shouldn't use up the run's own time.
	if !e.ImageExists(spec.Image) {
		pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxPullTimeout)
		err := e.Pull(pullCtx, spec.Image, nil, io.Discard)
		cancel()
		if err != nil {
			return SandboxResult{}, fmt.Errorf("couldn't pull %s: %v", spec.Image, err)
		}
	}
	if out, err := e.contextCommand(ctx, args...).CombinedOutput(); err != nil {
		return SandboxResult{}, fmt.Errorf("couldn't create the sandbox: %s", firstLine(out))
	}
	if len(spec.Files) > 0 {
		var in bytes.Buffer
		tw := tar.NewWriter(&in)
		dirs := map[string]bool{}
		for p := range spec.Files {
			for d := path.Dir(p); d != "." && !dirs[d]; d = path.Dir(d) {
				dirs[d] = true
			}
		}
		for _, d := range slices.Sorted(maps.Keys(dirs)) {
			_ = tw.WriteHeader(&tar.Header{Name: d + "/", Typeflag: tar.TypeDir, Mode: 0o755})
		}
		for p, content := range spec.Files {
			if err := tw.WriteHeader(&tar.Header{Name: p, Mode: 0o644, Size: int64(len(content))}); err != nil {
				return SandboxResult{}, err
			}
			_, _ = tw.Write(content)
		}
		_ = tw.Close()
		cp := e.contextCommand(ctx, "cp", "-", name+":"+sandboxWorkdir)
		cp.Stdin = &in
		if out, err := cp.CombinedOutput(); err != nil {
			return SandboxResult{}, fmt.Errorf("couldn't copy the files in: %s", firstLine(out))
		}
	}

	var res SandboxResult
	out := &cappedBuffer{max: sandboxMaxOutput}
	runCtx, stopRun := context.WithCancelCause(ctx)
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		e.watchSandboxDisk(runCtx, name, work, stopRun)
	}()
	start := e.contextCommand(runCtx, "start", "-a", name)
	start.Stdout, start.Stderr = out, out
	err := start.Run()
	// A measurement still going holds the volume, which then couldn't be removed.
	stopRun(nil)
	<-watched
	res.Output, res.Truncated = out.String(), out.cut
	if errors.Is(context.Cause(runCtx), ErrSandboxDisk) {
		return res, ErrSandboxDisk
	}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		res.ExitCode = exit.ExitCode()
	} else if err != nil {
		return res, err
	}

	// Copying out needs a context of its own: the run may have used up most of the caller's.
	cp := e.command("cp", name+":"+sandboxWorkdir+"/.", "-")
	tarOut, err := cp.StdoutPipe()
	if err != nil {
		return res, err
	}
	if err := cp.Start(); err != nil {
		return res, err
	}
	res.Files, res.Skipped = readSandboxFiles(tarOut)
	_, _ = io.Copy(io.Discard, tarOut)
	_ = cp.Wait()
	return res, nil
}

// watchSandboxDisk stops the sandbox once it has written more than SandboxMaxDisk. Both measures come from outside the sandbox, so its code can't hide what it wrote: Docker's size of the container's own layer, and the blocks its working volume uses on the server.
func (e *Engine) watchSandboxDisk(ctx context.Context, name, work string, stop context.CancelCauseFunc) {
	folder := ""
	if out, err := e.command("volume", "inspect", "--format", "{{.Mountpoint}}", work).Output(); err == nil {
		folder = strings.TrimSpace(string(out))
	}
	t := time.NewTicker(sandboxDiskEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		used := e.volumeUsed(work, folder)
		if out, err := e.command("inspect", "--size", "--format", "{{.SizeRw}}", name).Output(); err == nil {
			n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			used += n
		}
		if used > SandboxMaxDisk {
			stop(ErrSandboxDisk)
			_ = e.command("kill", name).Run()
			return
		}
	}
}

// volumeUsed is the space a volume's files take up: read straight from its folder when the panel can, as it can running as root on a server, or else through a container of Skiff's own tool image.
func (e *Engine) volumeUsed(volume, folder string) int64 {
	if n, ok := diskUsed(folder); ok {
		return n
	}
	out, err := e.command("run", "--rm", "--network", "none", "-v", volume+":/v:ro", VolumeTool, "du", "-sk", "/v").Output()
	if err != nil {
		return 0
	}
	kb, _ := strconv.ParseInt(strings.Fields(string(out) + " 0")[0], 10, 64)
	return kb << 10
}

// diskUsed is the space the files under folder take up, counting allocated blocks so a sparse file isn't taken at its apparent size. It's false when the folder can't be read.
func diskUsed(folder string) (int64, bool) {
	if folder == "" {
		return 0, false
	}
	if _, err := os.ReadDir(folder); err != nil {
		return 0, false
	}
	var total int64
	_ = filepath.WalkDir(folder, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				total += st.Blocks * 512
			}
		}
		return nil
	})
	return total, true
}

// readSandboxFiles keeps the regular files in a tar stream, within the size limits.
func readSandboxFiles(r io.Reader) (map[string][]byte, []string) {
	files := map[string][]byte{}
	var skipped []string
	total := 0
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		p, ok := SandboxPath(h.Name)
		if !ok {
			continue
		}
		if h.Size > SandboxMaxFile || total+int(h.Size) > SandboxMaxFiles {
			skipped = append(skipped, p)
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, SandboxMaxFile))
		if err != nil {
			break
		}
		files[p] = b
		total += len(b)
	}
	return files, skipped
}

// RemoveSandboxes removes every sandbox of an app, or every sandbox at all when app is "".
func (e *Engine) RemoveSandboxes(app string) {
	filter := "label=" + SandboxLabel
	if app != "" {
		filter += "=" + app
	}
	out, err := e.command("ps", "-aq", "--filter", filter).Output()
	if err != nil {
		return
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		_ = e.command(append([]string{"rm", "-f"}, ids...)...).Run()
	}
	if vols, err := e.command("volume", "ls", "-q", "--filter", filter).Output(); err == nil {
		if names := strings.Fields(string(vols)); len(names) > 0 {
			_ = e.command(append([]string{"volume", "rm", "-f"}, names...)...).Run()
		}
	}
}

type cappedBuffer struct {
	bytes.Buffer
	max int
	cut bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room < len(p) {
		b.cut = true
		if room > 0 {
			b.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

// BrowserImage is the sandbox image for browsing: Microsoft's Playwright image has the browsers and their libraries, and the Python package of the same version is added here once per server.
const (
	browserBase       = "mcr.microsoft.com/playwright/python:v1.64.1-noble"
	browserPlaywright = "1.64.1"
	BrowserImage      = "skiff-sandbox-browser:" + browserPlaywright
)

// EnsureBrowserImage builds BrowserImage if this server doesn't have it yet.
func (e *Engine) EnsureBrowserImage(ctx context.Context) error {
	if e.ImageExists(BrowserImage) {
		return nil
	}
	dir, err := os.MkdirTemp("", "skiff-browser-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var out bytes.Buffer
	dockerfile := "FROM " + browserBase + "\nRUN pip install --no-cache-dir playwright==" + browserPlaywright + "\n"
	if err := e.BuildFromDockerfile(ctx, BrowserImage, dockerfile, dir, false, BuildOptions{}, &out); err != nil {
		return fmt.Errorf("couldn't prepare the browser image: %s", lastLine(out.Bytes()))
	}
	return nil
}
