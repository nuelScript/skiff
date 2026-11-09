package docker

import (
	"reflect"
	"strings"
	"testing"
)

// What nvidia-smi prints for --query-gpu=index,uuid,name,memory.total,memory.used,utilization.gpu,temperature.gpu --format=csv,noheader,nounits on a two-card server; some cards answer [N/A] for a reading.
const smiOutput = `0, GPU-4b1e1a2c-6f3d-7e4a-8b9c-0d1e2f3a4b5c, NVIDIA L4, 23034, 1203, 37, 45
1, GPU-9c8b7a6d-5e4f-3a2b-1c0d-ef9876543210, NVIDIA A100-SXM4-80GB, 81920, 0, [N/A], 31
`

func TestParseGPUs(t *testing.T) {
	gpus := parseGPUs(smiOutput)
	if len(gpus) != 2 {
		t.Fatalf("%+v", gpus)
	}
	if g := gpus[0]; g.Index != 0 || g.Name != "NVIDIA L4" || g.MemoryTotal != 23034 || g.MemoryUsed != 1203 || g.Utilization != 37 || g.Temperature != 45 || !strings.HasPrefix(g.UUID, "GPU-4b1e") {
		t.Errorf("first: %+v", g)
	}
	if g := gpus[1]; g.Utilization != 0 || g.Name != "NVIDIA A100-SXM4-80GB" {
		t.Errorf("an [N/A] reading: %+v", g)
	}
	if got := parseGPUs("NVIDIA-SMI has failed because it couldn't communicate with the NVIDIA driver."); len(got) != 0 {
		t.Errorf("an error message read as GPUs: %+v", got)
	}
}

func TestPickGPUs(t *testing.T) {
	all := []GPU{{Index: 0, UUID: "a"}, {Index: 1, UUID: "b"}, {Index: 2, UUID: "c"}}
	used := map[string]string{"a": "llm", "b": "other"}
	if got, err := PickGPUs("llm", 1, all, used); err != nil || !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("an app keeps its own GPU: %v %v", got, err)
	}
	if got, err := PickGPUs("llm", 2, all, used); err != nil || !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("its own, then a free one: %v %v", got, err)
	}
	if _, err := PickGPUs("new", 2, all, used); err == nil || !strings.Contains(err.Error(), "only 1 are free") || !strings.Contains(err.Error(), "GPU 1 by other") {
		t.Errorf("not enough free: %v", err)
	}
	if _, err := PickGPUs("big", 4, all, nil); err == nil || !strings.Contains(err.Error(), "has 3") {
		t.Errorf("more than the server has: %v", err)
	}
	if got, err := PickGPUs("cpu", 0, all, used); got != nil || err != nil {
		t.Errorf("no GPUs asked for: %v %v", got, err)
	}
}

func TestGPUArgs(t *testing.T) {
	if got := gpuArgs([]string{"a", "c"}); strings.Join(got, " ") != `--gpus "device=a,c" --label skiff.gpus=a,c` {
		t.Errorf("%v", got)
	}
	if gpuArgs(nil) != nil {
		t.Error("args without GPUs")
	}
}
