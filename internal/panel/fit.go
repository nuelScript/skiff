package panel

import (
	"fmt"
	"strconv"
	"strings"
)

// diskFloor is the free disk a deploy needs to build and unpack an image.
const diskFloor = 1 << 30

// fitCheck says, before a deploy, whether it fits what the server has free. Memory only warns, since the kernel may free some. Too little disk refuses a build, which would fail partway anyway, but only warns for an image, which may already be on the server, so a fix can still go out. A preview is checked against its project's limit, which it runs with.
func fitCheck(src Source, disk, mem resourceUsage, appUses int64) (warning, refusal string) {
	if disk.Total > 0 && disk.Total-disk.Used < diskFloor {
		if src.Image == "" {
			return "", fmt.Sprintf("the server has only %s of disk free, and a deploy needs room to build. Free up space on the Server page, then deploy again", humanBytes(disk.Total-disk.Used))
		}
		warning = fmt.Sprintf("the server has only %s of disk free, so pulling a new image may fail. Free up space on the Server page", humanBytes(disk.Total-disk.Used))
	}
	need := limitBytes(healthOf(src).Memory) * int64(max(src.Replicas, 1))
	if need == 0 || mem.Total == 0 {
		return warning, ""
	}
	// The version being replaced gives its memory back.
	free := mem.Total - mem.Used + appUses
	if need <= free {
		return warning, ""
	}
	copies := ""
	if src.Replicas > 1 {
		copies = fmt.Sprintf(" for %d copies", src.Replicas)
	}
	mw := fmt.Sprintf("this app may use up to %s%s, and the server has about %s free. If it fills that, the server stops it. Lower its memory limit, run fewer copies, or stop another app", humanBytes(need), copies, humanBytes(free))
	if warning != "" {
		mw = warning + ". " + mw
	}
	return mw, ""
}

// limitBytes reads a memory limit as Docker does, like 512m, 1g, 1gb or 1GiB; 0 when there is none.
func limitBytes(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(strings.TrimSuffix(s, "b"), "i")
	if s == "" || s == "0" {
		return 0
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k':
		mult = 1 << 10
	case 'm':
		mult = 1 << 20
	case 'g':
		mult = 1 << 30
	case 't':
		mult = 1 << 40
	}
	if mult > 1 {
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(v * float64(mult))
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%d MB", b>>20)
	}
	return fmt.Sprintf("%d KB", b>>10)
}
