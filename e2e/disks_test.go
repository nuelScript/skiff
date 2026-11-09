//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// addedDisk gives the test a folder on a disk other than the system's: /mnt where that's separate, or else a small file system made in an image and mounted through a loop device.
func addedDisk(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("needs Linux")
	}
	sudo := func(script string) error { return exec.Command("sudo", "-n", "sh", "-c", script).Run() }
	dir := fmt.Sprintf("/mnt/skiff-e2e-%d", time.Now().UnixNano())
	if sudo(fmt.Sprintf("mkdir -p %s", dir)) != nil {
		t.Skip("can't make a folder under /mnt")
	}
	t.Cleanup(func() { _ = sudo("rm -rf " + dir) })
	if !separateDevice(dir) {
		img := dir + ".img"
		if err := sudo(fmt.Sprintf("truncate -s 1G %s && mkfs.ext4 -q -F %s && mount -o loop %s %s", img, img, img, dir)); err != nil {
			t.Fatalf("making a disk to test with: %v", err)
		}
		t.Cleanup(func() { _ = sudo(fmt.Sprintf("umount %s; rm -f %s", dir, img)) })
	}
	if err := sudo(fmt.Sprintf("chown %d %s", os.Getuid(), dir)); err != nil {
		t.Fatal(err)
	}
	if !separateDevice(dir) {
		t.Fatal("the test disk is on the system disk")
	}
	return dir
}

func separateDevice(dir string) bool {
	var a, b syscall.Stat_t
	return syscall.Stat(dir, &a) == nil && syscall.Stat("/", &b) == nil && a.Dev != b.Dev
}

// TestDatabaseMovesBetweenDisks creates a database on an added disk, moves it to Docker's storage and back, and checks its rows each time.
func TestDatabaseMovesBetweenDisks(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	diskPath := addedDisk(t)
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

	var dk struct{ ID string }
	postJSON(t, c, base+"/api/disks", fmt.Sprintf(`{"name":"data-1","path":%q}`, diskPath), &dk)
	var db struct{ ID string }
	postJSON(t, c, base+"/api/databases", fmt.Sprintf(`{"engine":"postgres","name":"moved","disk":%q}`, dk.ID), &db)
	ctr := "skiff-db-" + db.ID
	vol := ctr + "-data"
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", ctr).Run()
		_ = exec.Command("docker", "volume", "rm", vol).Run()
	})
	folder := func() string {
		out, _ := exec.Command("docker", "volume", "inspect", "--format", "{{index .Options \"device\"}}", vol).Output()
		return strings.TrimSpace(string(out))
	}
	if got := folder(); got != diskPath+"/skiff-volumes/"+vol {
		t.Fatalf("created at %q, not on the disk", got)
	}
	sql := func(stmt string) (string, error) {
		out, err := exec.Command("docker", "exec", ctr, "psql", "-U", "skiff", "-d", "moved", "-v", "ON_ERROR_STOP=1", "-qtAc", stmt).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	eventually(t, 3*time.Minute, func() bool {
		_, err := sql("create table if not exists notes(body text); insert into notes select 'note ' || g from generate_series(1, 500) g")
		return err == nil
	})
	want, _ := sql("select count(*) || ' ' || md5(string_agg(body, ',' order by body)) from notes")

	move := func(to string) {
		t.Helper()
		post(t, c, base+"/api/move", fmt.Sprintf(`{"database":%q,"disk":%q}`, db.ID, to))
		eventually(t, 5*time.Minute, func() bool {
			var list []struct {
				ID   string
				Disk string
				Move struct {
					Moving bool
					Error  string
				}
			}
			getJSON(t, c, base+"/api/databases", &list)
			for _, d := range list {
				if d.ID == db.ID {
					if d.Move.Error != "" {
						t.Fatalf("the move failed: %s", d.Move.Error)
					}
					return !d.Move.Moving && d.Disk == to
				}
			}
			return false
		})
		var got string
		eventually(t, 2*time.Minute, func() bool {
			var err error
			got, err = sql("select count(*) || ' ' || md5(string_agg(body, ',' order by body)) from notes")
			return err == nil
		})
		if got != want {
			t.Fatalf("after moving to %q: %q, want %q", to, got, want)
		}
	}

	move("")
	if got := folder(); got != "" {
		t.Errorf("still in a folder after moving to Docker's storage: %q", got)
	}
	if _, err := os.Stat(diskPath + "/skiff-volumes/" + vol); err == nil {
		t.Error("the old folder on the disk was left behind")
	}
	move(dk.ID)
	if got := folder(); got != diskPath+"/skiff-volumes/"+vol {
		t.Errorf("not back on the disk: %q", got)
	}

	// Docker leaves a folder volume's files behind, where a new volume of the same name would find them.
	req, _ := http.NewRequest(http.MethodDelete, base+"/api/databases?id="+db.ID, nil)
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode >= 300 {
		t.Fatalf("deleting the database: %v %v", resp, err)
	}
	resp.Body.Close()
	if _, err := os.Stat(diskPath + "/skiff-volumes/" + vol); err == nil {
		t.Error("the database's files were left on the disk")
	}
}
