package docker

import (
	"fmt"
	"strconv"
	"strings"
)

// sharedDatabases is how many Postgres servers one without a memory limit of its own expects to share the server with: a primary, its replicas, previews' copies and restore checks all run at once, and each sized for the whole server would together ask for more than it has.
const sharedDatabases = 4

// tuningMemory is what a Postgres container is tuned for: its own limit when it has one, otherwise its share of the server's cap.
func tuningMemory(own string, limit int64) int64 {
	if own != "" || limit <= 0 {
		return limit
	}
	return max(limit/sharedDatabases, 512<<20)
}

// PostgresTuning sizes Postgres's memory settings to what its container may use, instead of the defaults that assume a tiny machine. The values follow common practice for a server Postgres shares with other things.
func PostgresTuning(limit int64) (flags []string, shm int64) {
	if limit <= 0 {
		return nil, 0
	}
	const mb = 1 << 20
	clamp := func(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }
	shared := clamp(limit/4, 128*mb, 8192*mb)
	cache := clamp(limit*3/4, 512*mb, 1<<50)
	maint := clamp(limit/16, 64*mb, 2048*mb)
	work := clamp(limit/256, 4*mb, 64*mb)
	shm = clamp(limit/8, 64*mb, 1024*mb)
	for _, s := range []struct {
		name string
		v    int64
	}{{"shared_buffers", shared}, {"effective_cache_size", cache}, {"maintenance_work_mem", maint}, {"work_mem", work}} {
		flags = append(flags, "-c", fmt.Sprintf("%s=%dMB", s.name, s.v/mb))
	}
	return flags, shm
}

// memoryBytes is the memory a container gets: its own limit, like 512m or 2g, or the server's safety cap.
func (e *Engine) memoryBytes(own string) int64 {
	v := strings.ToLower(strings.TrimSpace(e.memoryOr(own)))
	mult := int64(1)
	switch {
	case strings.HasSuffix(v, "g"):
		mult, v = 1<<30, strings.TrimSuffix(v, "g")
	case strings.HasSuffix(v, "m"):
		mult, v = 1<<20, strings.TrimSuffix(v, "m")
	case strings.HasSuffix(v, "k"):
		mult, v = 1<<10, strings.TrimSuffix(v, "k")
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n * mult
}
