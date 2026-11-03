//go:build e2e

package e2e

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const stackFile = `
services:
  api:
    image: python:3.12-alpine
    command: python -m http.server 8000
    ports: ["8080:8000"]
    environment:
      DATABASE_URL: postgres://app:${DB_PASSWORD}@db:5432/shop
    depends_on: [db, cache]
  worker:
    image: python:3.12-alpine
    command: python -c "import time; time.sleep(100000)"
    environment:
      DATABASE_URL: postgres://app:${DB_PASSWORD}@db:5432/shop
    depends_on: [db]
  probe:
    image: alpine:3.20
    command: sleep 100000
    volumes: [files:/files]
    environment: {MARK: "%s"}
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: shop
    volumes: [pg:/var/lib/postgresql/data]
  cache:
    image: redis:7-alpine
volumes: {files: {}, pg: {}}
`

type stackMember struct{ Service, App, Kind, Engine, State string }

// TestComposeStack creates real stacks from a Compose file and checks services find each other the way the file expects.
func TestComposeStack(t *testing.T) {
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
	waitHTTP(t, "http://"+panel+"/api/me")
	c := client(t)
	post(t, c, "http://"+panel+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	base := "http://" + panel + "/api/stacks"
	request := func(name, mark string, vars bool) string {
		v := `{}`
		if vars {
			v = `{"DB_PASSWORD":"s3cret-` + name + `"}`
		}
		body := fmt.Sprintf(stackFile, mark)
		quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(body)
		return fmt.Sprintf(`{"name":%q,"compose":"%s","vars":%s}`, name, quoted, v)
	}
	cleanup := func(name string) {
		req, _ := http.NewRequest(http.MethodDelete, base+"?name="+name, nil)
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	t.Cleanup(func() { cleanup("e2e-a"); cleanup("e2e-b") })

	// The plan asks for the missing variable and never shows values.
	var plan struct {
		Items []struct{ Service, Kind, WorkerOf, Engine string }
		Unset []string
	}
	postJSON(t, c, base+"/plan", request("e2e-a", "a", false), &plan)
	if len(plan.Unset) != 1 || plan.Unset[0] != "DB_PASSWORD" {
		t.Fatalf("unset: %+v", plan.Unset)
	}
	resp, err := c.Post(base+"/plan", "application/json", strings.NewReader(request("e2e-a", "a", true)))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(raw), "s3cret") {
		t.Fatalf("the plan leaks a secret: %s", raw)
	}

	for _, s := range []struct{ name, mark string }{{"e2e-a", "a"}, {"e2e-b", "b"}} {
		resp, err := c.Post(base, "application/json", strings.NewReader(request(s.name, s.mark, true)))
		if err != nil {
			t.Fatal(err)
		}
		msg, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("creating %s: %d %s", s.name, resp.StatusCode, msg)
		}
	}

	members := func(name string) []stackMember {
		var list []struct {
			Name    string
			Members []stackMember
		}
		getJSON(t, c, base, &list)
		for _, s := range list {
			if s.Name == name {
				return s.Members
			}
		}
		return nil
	}
	for _, name := range []string{"e2e-a", "e2e-b"} {
		eventually(t, 4*time.Minute, func() bool {
			m := members(name)
			if len(m) != 5 {
				return false
			}
			for _, x := range m {
				if x.State != "running" {
					return false
				}
			}
			return true
		})
	}

	inProbe := func(stack, script string) string {
		t.Helper()
		out, err := exec.Command("docker", "ps", "-q", "--filter", "label=skiff.app="+stack+"-probe").Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			t.Fatalf("no probe container for %s: %v", stack, err)
		}
		return dockerExec(t, strings.Fields(string(out))[0], script)
	}
	dbContainer := func(stack, service string) string {
		t.Helper()
		out, err := exec.Command("docker", "ps", "--filter", "label=skiff.stack="+stack, "--filter", "label=skiff.kind=database", "--format", "{{.Names}}").Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range strings.Fields(string(out)) {
			img, _ := exec.Command("docker", "inspect", "-f", "{{.Config.Image}}", n).Output()
			if strings.HasPrefix(string(img), map[string]string{"db": "postgres", "cache": "redis"}[service]) {
				return n
			}
		}
		t.Fatalf("no %s for %s in %s", service, stack, out)
		return ""
	}

	for _, stack := range []string{"e2e-a", "e2e-b"} {
		// Compose service names resolve, on the file's own ports.
		for _, target := range []string{"db 5432", "cache 6379", "api 8000"} {
			inProbe(stack, "nc -z -w 5 "+target)
		}
		if got := inProbe(stack, "wget -qO- http://api:8000/ | head -c 200"); !strings.Contains(strings.ToLower(got), "directory listing") {
			t.Fatalf("%s: api answered %q", stack, got)
		}
		// The file's credentials work, over the service name.
		db := dbContainer(stack, "db")
		if got := dockerExec(t, db, "PGPASSWORD=s3cret-"+stack+" psql -h db -U app -d shop -tAc 'select 1'"); got != "1" {
			t.Fatalf("%s: psql over the service name: %q", stack, got)
		}
		// Redis has no password in the file, so clients without one must work.
		if got := dockerExec(t, dbContainer(stack, "cache"), "redis-cli -h cache ping"); got != "PONG" {
			t.Fatalf("%s: redis: %q", stack, got)
		}
		// The worker shares the API's image and env, and runs its own command.
		out, _ := exec.Command("docker", "ps", "-q", "--filter", "label=skiff.worker.app="+stack+"-api").Output()
		ids := strings.Fields(string(out))
		if len(ids) != 1 {
			t.Fatalf("%s: workers: %q", stack, ids)
		}
		if got := dockerExec(t, ids[0], "echo $DATABASE_URL"); got != "postgres://app:s3cret-"+stack+"@db:5432/shop" {
			t.Fatalf("%s: worker env: %q", stack, got)
		}
	}

	// The same service names in two stacks stay apart.
	marker := func(stack, value string) {
		t.Helper()
		dockerExec(t, dbContainer(stack, "db"), "PGPASSWORD=s3cret-"+stack+" psql -h db -U app -d shop -qc \"create table who(v text); insert into who values ('"+value+"')\"")
	}
	marker("e2e-a", "first")
	marker("e2e-b", "second")
	got := dockerExec(t, dbContainer("e2e-b", "db"), "PGPASSWORD=s3cret-e2e-b psql -h db -U app -d shop -tAc 'select v from who'")
	if got != "second" {
		t.Fatalf("stack b reached the wrong database: %q", got)
	}
	netA, _ := exec.Command("docker", "inspect", "-f", "{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}", dbContainer("e2e-a", "db")).Output()
	if strings.TrimSpace(string(netA)) != "skiff-s-e2e-a" {
		t.Fatalf("database networks: %q", netA)
	}

	// Backups and restore work on a stack's database.
	var dbs []struct{ ID, Name string }
	getJSON(t, c, "http://"+panel+"/api/databases", &dbs)
	id := ""
	for _, d := range dbs {
		if d.Name == "e2e-a-db" {
			id = d.ID
		}
	}
	if id == "" {
		t.Fatalf("stack database isn't listed: %+v", dbs)
	}
	var b struct{ ID string }
	postJSON(t, c, "http://"+panel+"/api/backups?db="+id, "", &b)
	a := dbContainer("e2e-a", "db")
	dockerExec(t, a, "PGPASSWORD=s3cret-e2e-a psql -h db -U app -d shop -qc \"update who set v='changed'\"")
	post(t, c, "http://"+panel+"/api/backups/restore?id="+b.ID, "")
	if got := dockerExec(t, a, "PGPASSWORD=s3cret-e2e-a psql -h db -U app -d shop -tAc 'select v from who'"); got != "first" {
		t.Fatalf("restore on a stack database: %q", got)
	}

	// A taken name is refused.
	resp, err = c.Post(base, "application/json", strings.NewReader(request("e2e-a", "a", true)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("a duplicate stack: %d", resp.StatusCode)
	}

	// Deleting leaves nothing behind, and doesn't touch the other stack.
	cleanup("e2e-a")
	eventually(t, time.Minute, func() bool {
		out, _ := exec.Command("sh", "-c", "docker ps -aq --filter label=skiff.stack=e2e-a; docker ps -aq --filter label=skiff.app=e2e-a-api; docker ps -aq --filter label=skiff.app=e2e-a-probe; docker ps -aq --filter label=skiff.worker.app=e2e-a-api; docker network ls -q --filter name=skiff-s-e2e-a; docker volume ls -q --filter name=skiff-e2e-a-probe").Output()
		return strings.TrimSpace(string(out)) == ""
	})
	if m := members("e2e-b"); len(m) != 5 {
		t.Fatalf("deleting one stack changed another: %+v", m)
	}
	inProbe("e2e-b", "nc -z -w 5 db 5432")
}

// TestComposeStackFromRepo builds a stack from a repository, with the Compose file in a subfolder and a custom Dockerfile.
func TestComposeStackFromRepo(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "skiff")
	run(t, "", nil, "go", "build", "-o", bin, "..")
	repo, push := gitServer(t, root)
	work := filepath.Join(root, "work")
	for path, content := range map[string]string{
		"api/Dockerfile.custom": "FROM busybox:1.36\nRUN echo built-custom > /marker && mkdir /www && echo hello-from-stack > /www/index.html\nCMD [\"httpd\", \"-f\", \"-p\", \"3000\", \"-h\", \"/www\"]\n",
		"deploy/compose.yml": `services:
  web:
    build: {context: ../api, dockerfile: Dockerfile.custom}
    ports: ["3000"]
    env_file: app.env
    environment: {FROM_DOTENV: "${FROM_DOTENV}", FROM_REQUEST: "${FROM_REQUEST}"}
  jobs:
    build: {context: ../api, dockerfile: Dockerfile.custom}
    command: sleep 100000
    env_file: app.env
    environment: {FROM_DOTENV: "${FROM_DOTENV}", FROM_REQUEST: "${FROM_REQUEST}"}
`,
		"deploy/.env":    "FROM_DOTENV=dotenv-value\nFROM_REQUEST=dotenv-loses\n",
		"deploy/app.env": "FROM_ENV_FILE=file-value\n",
	} {
		full := filepath.Join(work, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	push("v1")

	protectExisting(t, home)
	panel := freeAddr(t)
	start(t, []string{"HOME=" + home, "SKIFF_PANEL_PASSWORD=e2e-setup", "SKIFF_ALLOW_PRIVATE_ENDPOINTS=1"}, bin, "panel", "--addr", panel, "--domain", "localhost")
	waitHTTP(t, "http://"+panel+"/api/me")
	c := client(t)
	post(t, c, "http://"+panel+"/api/auth/setup", `{"secret":"e2e-setup","email":"e2e@example.com","name":"E2E","password":"e2e-password-1"}`)

	base := "http://" + panel + "/api/stacks"
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, base+"?name=e2e-repo", nil)
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	body := fmt.Sprintf(`{"name":"e2e-repo","git":%q,"path":"deploy/compose.yml","vars":{"FROM_REQUEST":"request-value"}}`, repo)
	resp, err := c.Post(base, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("creating the stack: %d %s", resp.StatusCode, msg)
	}

	var ctr string
	eventually(t, 3*time.Minute, func() bool {
		out, _ := exec.Command("docker", "ps", "-q", "--filter", "label=skiff.app=e2e-repo-web").Output()
		ids := strings.Fields(string(out))
		if len(ids) != 1 {
			return false
		}
		ctr = ids[0]
		out, _ = exec.Command("docker", "ps", "-q", "--filter", "label=skiff.worker.app=e2e-repo-web").Output()
		return len(strings.Fields(string(out))) == 1
	})
	if got := dockerExec(t, ctr, "cat /marker"); got != "built-custom" {
		t.Fatalf("the custom Dockerfile wasn't used: %q", got)
	}
	for k, want := range map[string]string{"FROM_DOTENV": "dotenv-value", "FROM_REQUEST": "request-value", "FROM_ENV_FILE": "file-value"} {
		if got := dockerExec(t, ctr, "printenv "+k); got != want {
			t.Fatalf("%s = %q, want %q", k, got, want)
		}
	}
	if got := dockerExec(t, ctr, "wget -qO- http://web:3000/"); got != "hello-from-stack" {
		t.Fatalf("over its own service name: %q", got)
	}
	// Nothing secret reaches the image.
	if out, _ := exec.Command("docker", "exec", ctr, "ls", "/app/skiff.toml").CombinedOutput(); !strings.Contains(string(out), "No such file") && !strings.Contains(string(out), "can't access") {
		t.Fatalf("skiff.toml in the image: %s", out)
	}
}
