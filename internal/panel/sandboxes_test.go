package panel

import (
	"context"
	"testing"
	"time"
)

func TestSandboxSpecChecksTheRequest(t *testing.T) {
	spec, timeout, err := sandboxSpec("bot", sandboxRequest{Command: "python main.py",
		Files: map[string]string{"./src/main.py": "print(1)"}, BinaryFiles: map[string]string{"data/x.bin": "AAE="}, Timeout: 99999})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Image != sandboxDefaultImage || timeout != sandboxMaxTimeout || spec.Internet {
		t.Errorf("defaults: %s %s %v", spec.Image, timeout, spec.Internet)
	}
	if string(spec.Files["src/main.py"]) != "print(1)" || string(spec.Files["data/x.bin"]) != "\x00\x01" {
		t.Errorf("files: %q", spec.Files)
	}
	for _, bad := range []sandboxRequest{
		{},
		{Command: "ls", Image: "bad image"},
		{Command: "ls", Files: map[string]string{"../etc/passwd": "x"}},
		{Command: "ls", Files: map[string]string{"/etc/passwd": "x"}},
		{Command: "ls", BinaryFiles: map[string]string{"a": "not base64!"}},
	} {
		if _, _, err := sandboxSpec("bot", bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if _, timeout, _ := sandboxSpec("bot", sandboxRequest{Command: "ls"}); timeout != sandboxDefaultTimeout {
		t.Errorf("default timeout %s", timeout)
	}
	if _, timeout, _ := sandboxSpec("bot", sandboxRequest{Command: "ls", Timeout: 30}); timeout != 30*time.Second {
		t.Errorf("own timeout %s", timeout)
	}
}

func TestSandboxPlacesPerAgent(t *testing.T) {
	var releases []func()
	for i := range sandboxPerApp {
		release, ok := claimSandbox("bot", func() {})
		if !ok {
			t.Fatalf("place %d refused", i)
		}
		releases = append(releases, release)
	}
	if _, ok := claimSandbox("bot", func() {}); ok {
		t.Error("one over the limit was allowed")
	}
	if release, ok := claimSandbox("other", func() {}); !ok {
		t.Error("another agent was refused")
	} else {
		release()
	}
	releases[0]()
	if release, ok := claimSandbox("bot", func() {}); !ok {
		t.Error("a freed place wasn't reused")
	} else {
		release()
	}
	for _, r := range releases[1:] {
		r()
	}
	// Stopping cancels what's running.
	ctx, cancel := context.WithCancel(context.Background())
	release, _ := claimSandbox("bot", cancel)
	defer release()
	sandboxes.Lock()
	for _, c := range sandboxes.live["bot"] {
		c()
	}
	sandboxes.Unlock()
	if ctx.Err() == nil {
		t.Error("not cancelled")
	}
}
