//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDagsterGuide follows the Dagster guide: a repository with the webserver as the app, the daemon as a worker and a managed Postgres for storage. A job on a schedule runs by itself, and its run is recorded.
func TestDagsterGuide(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	const name = "e2e-dagster"
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	t.Cleanup(func() {
		_ = exec.Command("sh", "-c", "docker rm -f $(docker ps -aq --filter label=skiff.app="+name+") ; docker rmi $(docker images -q skiff-"+name+")").Run()
	})
	repo := gitRepoFrom(t, root, "testdata/dagster")

	protectExisting(t, home)
	env := []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup", "SKIFF_ALLOW_PRIVATE_ENDPOINTS=1"}
	panelAddr, routerAddr := freeAddr(t), freeAddr(t)
	start(t, env, bin, "panel", "--addr", panelAddr, "--domain", "localhost")
	base := "http://" + panelAddr
	waitHTTP(t, base+"/api/me")
	start(t, env, bin, "router", "--domain", "localhost", "--http-addr", routerAddr, "--panel", panelAddr)
	c := client(t)
	post(t, c, base+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	var db struct{ ID, URL string }
	postJSON(t, c, base+"/api/databases", `{"engine":"postgres","name":"dagster"}`, &db)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "skiff-db-"+db.ID).Run()
		_ = exec.Command("docker", "volume", "rm", "-f", "skiff-db-"+db.ID+"-data").Run()
	})
	vars, _ := json.Marshal(map[string]any{"vars": []map[string]string{{"key": "DATABASE_URL", "value": db.URL}}})
	req, _ := http.NewRequest(http.MethodPut, base+"/api/env?app="+name, strings.NewReader(string(vars)))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := c.Do(req); err != nil || resp.StatusCode >= 300 {
		t.Fatalf("setting DATABASE_URL: %v %v", err, resp)
	}

	stream(t, c, base+"/api/deploy?name="+name+"&port=3000&git="+repo)
	post(t, c, base+"/api/workers?app="+name, `{"name":"daemon","command":"dagster-daemon run -m defs","replicas":1}`)

	graphql := func(query string, into any) error {
		body, _ := json.Marshal(map[string]string{"query": query})
		req, _ := http.NewRequest(http.MethodPost, "http://"+routerAddr+"/graphql", strings.NewReader(string(body)))
		req.Host = name + ".localhost"
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%d %s", resp.StatusCode, b)
		}
		return json.Unmarshal(b, into)
	}
	var last string
	ran := func() bool {
		var out struct {
			Data struct {
				RunsOrError struct {
					Results []struct{ RunID, Status string }
				}
			}
		}
		err := graphql(`{ runsOrError { ... on Runs { results { runId status } } } }`, &out)
		last = fmt.Sprintf("%v %+v", err, out.Data.RunsOrError.Results)
		for _, r := range out.Data.RunsOrError.Results {
			if r.Status == "SUCCESS" {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(6 * time.Minute)
	for !ran() {
		if time.Now().After(deadline) {
			t.Fatalf("no scheduled run succeeded; last: %s", last)
		}
		time.Sleep(10 * time.Second)
	}
	// The run history is in the managed Postgres, so it survives redeploys.
	out, err := exec.Command("docker", "exec", "skiff-db-"+db.ID, "psql", "-U", "skiff", "-d", "dagster", "-qtAc", "select count(*) from runs where status = 'SUCCESS'").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "0" {
		t.Errorf("runs in Postgres: %v %s", err, out)
	}
}

// gitRepoFrom serves a copy of dir as a git repository over HTTP.
func gitRepoFrom(t *testing.T, root, dir string) string {
	t.Helper()
	work := filepath.Join(root, "work")
	bare := filepath.Join(root, "srv", "app.git")
	if err := os.CopyFS(work, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	gitEnv := []string{"GIT_AUTHOR_NAME=e2e", "GIT_AUTHOR_EMAIL=e2e@example.com", "GIT_COMMITTER_NAME=e2e", "GIT_COMMITTER_EMAIL=e2e@example.com"}
	run(t, work, nil, "git", "init", "-q", "-b", "main")
	run(t, "", nil, "git", "init", "-q", "--bare", "-b", "main", bare)
	run(t, work, gitEnv, "git", "add", "-A")
	run(t, work, gitEnv, "git", "commit", "-q", "-m", "app")
	run(t, work, gitEnv, "git", "push", "-q", bare, "main")
	backend, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(&cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(string(backend)), "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + filepath.Join(root, "srv"), "GIT_HTTP_EXPORT_ALL=1"},
	})
	t.Cleanup(srv.Close)
	return srv.URL + "/app.git"
}
