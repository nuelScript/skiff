package panel

import (
	"testing"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/egress"
	"github.com/nuelScript/skiff/internal/registry"
)

func TestAddedReplicasKeepTheAppsLimits(t *testing.T) {
	openTestDB(t)
	src := Source{App: "api", Team: "t1", Port: "8080", Isolation: "strong",
		Egress: egress.Policy{Mode: egress.Private}.Encode(), Permissions: `{"capabilities":["NET_ADMIN"]}`}
	putSource(src)
	p := &Panel{eng: docker.Local()}
	spec, err := p.replicaSpec(registry.App{Name: "api", Port: 8080}, src)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Settings.Runtime != docker.SandboxRuntime {
		t.Errorf("runtime %q, want gVisor", spec.Settings.Runtime)
	}
	if egress.Decode(spec.Settings.Labels[egress.Label]).Mode != egress.Private {
		t.Errorf("labels %v don't carry the outbound limit", spec.Settings.Labels)
	}
	if len(spec.Settings.Permissions.Capabilities) != 1 {
		t.Errorf("permissions %+v", spec.Settings.Permissions)
	}
	if spec.Image != "skiff-api:latest" || spec.ContainerPort != 8080 {
		t.Errorf("spec %+v", spec)
	}
}
