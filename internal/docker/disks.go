package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// A volume can keep its files in a folder of the server's choosing, such as one on an added disk, through the local driver's bind options. It keeps its name, so everything that mounts it carries on as before.

// VolumeFolder is the folder a volume keeps its files in when it was created on one, or "" for Docker's own storage.
func (e *Engine) VolumeFolder(name string) string {
	out, err := e.command("volume", "inspect", "--format", "{{json .Options}}", name).Output()
	if err != nil {
		return ""
	}
	var opts map[string]string
	if json.Unmarshal(out, &opts) != nil || opts["o"] != "bind" {
		return ""
	}
	return opts["device"]
}

// CreateVolume makes a volume with these labels, in folder when it's set (which must exist) or in Docker's own storage.
func (e *Engine) CreateVolume(name, folder string, labels map[string]string) error {
	args := []string{"volume", "create"}
	for _, k := range sortedKeys(labels) {
		args = append(args, "--label", k+"="+labels[k])
	}
	if folder != "" {
		args = append(args, "--driver", "local", "--opt", "type=none", "--opt", "o=bind", "--opt", "device="+folder)
	}
	out, err := e.command(append(args, name)...).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

// VolumeLabels are a volume's labels, so a volume made again keeps them.
func (e *Engine) VolumeLabels(name string) map[string]string {
	out, err := e.command("volume", "inspect", "--format", "{{json .Labels}}", name).Output()
	labels := map[string]string{}
	if err == nil {
		_ = json.Unmarshal(out, &labels)
	}
	return labels
}

// VolumeUsers are the containers, running or stopped, that mount a volume.
func (e *Engine) VolumeUsers(name string) []string {
	out, err := e.command("ps", "-aq", "--filter", "volume="+name).Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// CopyTally is what a copy counted on each side: entries and bytes in files.
type CopyTally struct{ Entries, Bytes int64 }

// copyScript copies /from into /to, then counts each side's entries and the bytes in its files, so the copy can be checked before its source goes.
const copyScript = `set -e
tally() { cd "$1" && printf '%s %s\n' "$(find . | wc -l)" "$(find . -type f -exec stat -c %s {} + | awk '{s+=$1} END {print s+0}')"; }
cp -a /from/. /to/
tally /from
tally /to`

// CopyVolume copies everything from one place to another, each a volume name or an absolute folder, keeping owners and permissions, and checks the copy has the same entries and bytes.
func (e *Engine) CopyVolume(ctx context.Context, from, to string) (CopyTally, error) {
	out, err := e.contextCommand(ctx, "run", "--rm", "-v", from+":/from:ro", "-v", to+":/to", VolumeTool, "sh", "-c", copyScript).CombinedOutput()
	if err != nil {
		return CopyTally{}, fmt.Errorf("copying: %s", lastLine(out))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return CopyTally{}, fmt.Errorf("copying: couldn't count what was copied")
	}
	src, err1 := parseTally(lines[len(lines)-2])
	dst, err2 := parseTally(lines[len(lines)-1])
	if err1 != nil || err2 != nil {
		return CopyTally{}, fmt.Errorf("copying: couldn't count what was copied")
	}
	if src != dst {
		return src, fmt.Errorf("the copy doesn't match: %d entries and %d bytes, but %d and %d copied", src.Entries, src.Bytes, dst.Entries, dst.Bytes)
	}
	return src, nil
}

func parseTally(line string) (CopyTally, error) {
	f := strings.Fields(line)
	if len(f) != 2 {
		return CopyTally{}, fmt.Errorf("bad tally %q", line)
	}
	n, err1 := strconv.ParseInt(f[0], 10, 64)
	b, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return CopyTally{}, fmt.Errorf("bad tally %q", line)
	}
	return CopyTally{Entries: n, Bytes: b}, nil
}

// ClearFolder empties a folder through a container, as its files can belong to users the panel isn't.
func (e *Engine) ClearFolder(folder string) error {
	out, err := e.command("run", "--rm", "-v", folder+":/f", VolumeTool, "sh", "-c", "find /f -mindepth 1 -delete").CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}
