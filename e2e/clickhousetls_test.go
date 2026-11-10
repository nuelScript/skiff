//go:build e2e

package e2e

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPublicClickHouseIsEncrypted turns a ClickHouse database public: its address is HTTPS, and the published port answers only over TLS.
func TestPublicClickHouseIsEncrypted(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	protectExisting(t, home)
	panel := freeAddr(t)
	start(t, []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup"}, bin, "panel", "--addr", panel, "--domain", "localhost")
	base := "http://" + panel
	waitHTTP(t, base+"/api/me")
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	var db struct{ ID string }
	postJSON(t, c, base+"/api/databases", `{"engine":"clickhouse","name":"events"}`, &db)
	ctr := "skiff-db-" + db.ID
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f "+ctr+"; docker volume rm -f $(docker volume ls -q --filter name="+ctr+")").Run()
	})
	post(t, c, base+"/api/databases/public?id="+db.ID+"&on=1", "")

	var public string
	var u *url.URL
	var err error
	address := func() {
		var dbs []struct {
			ID        string
			PublicURL string `json:"publicUrl"`
		}
		getJSON(t, c, base+"/api/databases", &dbs)
		for _, d := range dbs {
			if d.ID == db.ID {
				public = d.PublicURL
			}
		}
		u, err = url.Parse(public)
	}
	eventually(t, 3*time.Minute, func() bool { address(); return public != "" })
	if err != nil || u.Scheme != "https" {
		t.Fatalf("public address %q isn't https", public)
	}
	pass, _ := u.User.Password()
	insecure := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	query := func(scheme string) (string, error) {
		req, _ := http.NewRequest(http.MethodPost, scheme+"://127.0.0.1:"+u.Port()+"/", strings.NewReader("SELECT 1"))
		req.SetBasicAuth(u.User.Username(), pass)
		resp, err := insecure.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return strings.TrimSpace(string(b)), nil
	}
	var got string
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		address()
		if got, err = query("https"); err == nil && got == "1" {
			break
		}
	}
	if got != "1" {
		logs, _ := exec.Command("sh", "-c", "docker port "+ctr+"; docker logs --tail 20 "+ctr+" 2>&1").CombinedOutput()
		t.Fatalf("over https at %s: %q %v\n%s", public, got, err, logs)
	}
	if got, err := query("http"); err == nil && got == "1" {
		t.Fatal("the public port answered in clear text")
	}
	// Apps on the server keep the plain address.
	if out, err := exec.Command("docker", "exec", ctr, "sh", "-c", `wget -qO- --header="X-ClickHouse-User: skiff" --header="X-ClickHouse-Key: $CLICKHOUSE_PASSWORD" "http://127.0.0.1:8123/?query=SELECT%202"`).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "2" {
		t.Errorf("the private HTTP port: %v %s", err, out)
	}
}
