package docker

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/config"
)

// What a container was granted reads back as the same permissions, so its jobs and runs can be started the same way.
func TestRuntimeReadsBackPermissions(t *testing.T) {
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("docker not available")
	}
	cases := []config.Permissions{
		{},
		{Capabilities: []string{"NET_ADMIN"}, Devices: []string{"/dev/null:/dev/quiet"}, Sysctls: []string{"net.ipv4.ip_unprivileged_port_start=0"}},
		{NewPrivileges: true},
	}
	e := Local()
	for _, want := range cases {
		name := fmt.Sprintf("skiff-inspect-test-%d", time.Now().UnixNano())
		args := append([]string{"run", "-d", "--name", name}, want.HardeningArgs()...)
		args = append(args, want.DockerArgs()...)
		args = append(args, VolumeTool, "sleep", "30")
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		rt, err := e.Runtime(name)
		_ = exec.Command("docker", "rm", "-f", name).Run()
		if err != nil {
			t.Fatal(err)
		}
		if !rt.Permissions.Equal(want) && !(want.IsZero() && rt.Permissions.IsZero()) {
			t.Errorf("granted %+v, read back %+v", want, rt.Permissions)
		}
		if rt.Runtime != "" {
			t.Errorf("runtime %q for a plain container", rt.Runtime)
		}
	}
}

func TestParseRuntimeReadsIsolation(t *testing.T) {
	rt, err := parseRuntime([]byte(`[{"Config":{"Image":"app"},"HostConfig":{"Runtime":"runsc","NetworkMode":"host","SecurityOpt":["no-new-privileges"]}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if rt.Runtime != "runsc" || !rt.Permissions.HostNetwork || rt.Permissions.NewPrivileges {
		t.Fatalf("read %q %+v", rt.Runtime, rt.Permissions)
	}
}
