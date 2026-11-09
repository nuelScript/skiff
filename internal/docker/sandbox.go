package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
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
)

// SandboxSpec is a throwaway container: files go into its working directory, the command runs, and what's left there comes back.
type SandboxSpec struct {
	App      string // who asked, as a label, so they can all be stopped at once
	Image    string
	Command  string
	Files    map[string][]byte
	Internet bool // otherwise it has no network at all
	Memory   string
	CPU      string
	ShmSize  string // shared memory, which browsers need more of than Docker's 64 MB
	Runtime  string // another OCI runtime, such as gVisor's
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
		network = "bridge"
	}
	// The working directory is a volume of its own: under gVisor, writes elsewhere stay in the sandbox's memory, out of reach of copying them back.
	work := name + "-work"
	args := []string{"create", "--name", name, "--label", SandboxLabel + "=" + spec.App,
		"--network", network, "--workdir", sandboxWorkdir, "--mount", "type=volume,src=" + work + ",dst=" + sandboxWorkdir,
		"--memory", spec.Memory, "--cpus", spec.CPU, "--pids-limit", "256",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--log-driver", "none"}
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
	start := e.contextCommand(ctx, "start", "-a", name)
	start.Stdout, start.Stderr = out, out
	err := start.Run()
	res.Output, res.Truncated = out.String(), out.cut
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
