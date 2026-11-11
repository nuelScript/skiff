package docker

import "testing"

func TestWorkerSpecsReportsDockerErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if specs, err := Local().WorkerSpecs("app"); err == nil || specs != nil {
		t.Fatalf("got %v, %v; want an error, so no worker looks missing", specs, err)
	}
}
