package docker

import (
	"strings"
	"testing"
)

func TestPostgresTuning(t *testing.T) {
	const mb, gb = int64(1 << 20), int64(1 << 30)
	for limit, want := range map[int64]string{
		// A 4 GB server's safety cap is 3 GB.
		3 * gb:   "shared_buffers=768MB effective_cache_size=2304MB maintenance_work_mem=192MB work_mem=12MB",
		256 * mb: "shared_buffers=128MB effective_cache_size=512MB maintenance_work_mem=64MB work_mem=4MB",
		96 * gb:  "shared_buffers=8192MB effective_cache_size=73728MB maintenance_work_mem=2048MB work_mem=64MB",
	} {
		flags, shm := PostgresTuning(limit)
		var got []string
		for i := 1; i < len(flags); i += 2 {
			got = append(got, flags[i])
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%d MB: %s", limit/mb, strings.Join(got, " "))
		}
		if shm < 64*mb || shm > gb {
			t.Errorf("%d MB: shm %d", limit/mb, shm)
		}
	}
	if flags, _ := PostgresTuning(0); flags != nil {
		t.Error("an unknown limit was tuned for")
	}
}

func TestMemoryBytes(t *testing.T) {
	e := &Engine{}
	for in, want := range map[string]int64{"512m": 512 << 20, "2g": 2 << 30, "2G": 2 << 30, "1048576": 1 << 20} {
		if got := e.memoryBytes(in); got != want {
			t.Errorf("%s: %d", in, got)
		}
	}
}
