package panel

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nuelScript/skiff/internal/config"
)

// Disks are folders where added block storage is mounted. The server's admin lists them; volumes and databases can then be created on one, or moved there later.
type disk struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Created int64  `json:"created"`
	Total   uint64 `json:"total"`
	Free    uint64 `json:"free"`
	Missing bool   `json:"missing,omitempty"`
}

// moveTimeout bounds a move, however much data it copies.
const moveTimeout = 12 * time.Hour

func listDisks() []disk {
	rows, err := sqlDB.Query(`SELECT id, name, path, created FROM disks ORDER BY created`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []disk{}
	for rows.Next() {
		var d disk
		if rows.Scan(&d.ID, &d.Name, &d.Path, &d.Created) == nil {
			out = append(out, d)
		}
	}
	return out
}

func getDisk(id string) (disk, bool) {
	for _, d := range listDisks() {
		if d.ID == id {
			return d, true
		}
	}
	return disk{}, false
}

// withSpace fills in a disk's size and free space, and notices when nothing is mounted there any more.
func withSpace(d disk) disk {
	var st syscall.Statfs_t
	if err := syscall.Statfs(d.Path, &st); err != nil {
		d.Missing = true
		return d
	}
	d.Total, d.Free = st.Blocks*uint64(st.Bsize), st.Bavail*uint64(st.Bsize)
	return d
}

// diskProblem checks a folder can be a disk: an existing folder on a different device from the system's, so it really is added storage.
func diskProblem(path string) string {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return "give the absolute folder the disk is mounted at, like /mnt/data"
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return path + " isn't a folder on this server; mount the disk first"
	}
	root, err := os.Stat("/")
	if err == nil && sameDevice(fi, root) {
		return path + " is on the same disk as the system; mount the added disk there first"
	}
	for _, d := range listDisks() {
		if d.Path == path || strings.HasPrefix(path+"/", d.Path+"/") || strings.HasPrefix(d.Path+"/", path+"/") {
			return "that folder overlaps " + d.Name
		}
	}
	return ""
}

func sameDevice(a, b os.FileInfo) bool {
	sa, ok1 := a.Sys().(*syscall.Stat_t)
	sb, ok2 := b.Sys().(*syscall.Stat_t)
	return ok1 && ok2 && sa.Dev == sb.Dev
}

// diskOf says which listed disk a folder is on, or "" for Docker's own storage.
func diskOf(folder string, disks []disk) string {
	for _, d := range disks {
		if strings.HasPrefix(folder, d.Path+"/") {
			return d.ID
		}
	}
	return ""
}

// diskPathFor is the disk's folder for a volume being created on it.
func diskPathFor(id string) string {
	if d, ok := getDisk(id); ok {
		return d.Path
	}
	return ""
}

// moves remembers the moves under way and the last outcome of each, for the dashboard to show.
var moves = struct {
	sync.Mutex
	m map[string]moveState
}{m: map[string]moveState{}}

type moveState struct {
	Moving bool   `json:"moving"`
	Error  string `json:"error,omitempty"`
	To     string `json:"to"`
	Since  int64  `json:"since"`
}

func moveStatus(volume string) moveState {
	moves.Lock()
	defer moves.Unlock()
	return moves.m[volume]
}

func startMove(volume, to string) bool {
	moves.Lock()
	defer moves.Unlock()
	if moves.m[volume].Moving {
		return false
	}
	moves.m[volume] = moveState{Moving: true, To: to, Since: time.Now().Unix()}
	return true
}

func endMove(volume string, err error) {
	moves.Lock()
	defer moves.Unlock()
	st := moves.m[volume]
	st.Moving = false
	if err != nil {
		st.Error = err.Error()
	} else {
		delete(moves.m, volume)
		return
	}
	moves.m[volume] = st
}

// moveVolume puts a volume's files on another disk, or back in Docker's storage when to is "". Nothing may be using the volume. The source is only removed once the copy is checked, and a failed move leaves the volume where it was.
func (p *Panel) moveVolume(ctx context.Context, volume, to string) error {
	if users := p.eng.VolumeUsers(volume); len(users) > 0 {
		return fmt.Errorf("%d containers still use the volume", len(users))
	}
	from := p.eng.VolumeFolder(volume)
	target := ""
	if to != "" {
		target = config.DiskFolder(to, volume)
	}
	if from == target {
		return nil
	}
	labels := p.eng.VolumeLabels(volume)
	if target != "" {
		if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
			return fmt.Errorf("%s already has files in it", target)
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("can't make the folder on the disk: %w", err)
		}
		if _, err := p.eng.CopyVolume(ctx, volume, target); err != nil {
			_ = p.eng.ClearFolder(target)
			return err
		}
		if err := p.eng.RemoveVolume(volume); err != nil {
			_ = p.eng.ClearFolder(target)
			return fmt.Errorf("couldn't let go of the old volume: %w", err)
		}
		if err := p.eng.CreateVolume(volume, target, labels); err != nil {
			return fmt.Errorf("the files are copied to %s but the volume couldn't be made there: %w", target, err)
		}
	} else {
		// The files stay in the folder when the volume is forgotten, and are copied back from there.
		if err := p.eng.ForgetVolume(volume); err != nil {
			return fmt.Errorf("couldn't let go of the old volume: %w", err)
		}
		if err := p.eng.CreateVolume(volume, "", labels); err != nil {
			_ = p.eng.CreateVolume(volume, from, labels)
			return err
		}
		if _, err := p.eng.CopyVolume(ctx, from, volume); err != nil {
			_ = p.eng.RemoveVolume(volume)
			_ = p.eng.CreateVolume(volume, from, labels)
			return err
		}
	}
	if from != "" {
		if err := p.eng.ClearFolder(from); err == nil {
			_ = os.Remove(from)
		}
	}
	return nil
}

// moveAppVolume stops what uses an app's volume, moves it, and deploys the app again so it comes back on the moved volume.
func (p *Panel) moveAppVolume(src Source, volume, to, actor string) {
	ctx, cancel := context.WithTimeout(context.Background(), moveTimeout)
	defer cancel()
	for _, c := range p.eng.VolumeUsers(volume) {
		_ = p.eng.Stop(c)
		_ = p.eng.Remove(c)
	}
	err := p.moveVolume(ctx, volume, to)
	endMove(volume, err)
	if err != nil {
		log.Printf("skiff: moving %s: %v", volume, err)
		go dispatchAlert(alertEvent{Team: src.Team, Kind: "deploy.failed", App: src.App, Title: "Moving a volume of " + src.App + " failed",
			Detail: err.Error() + " The volume is where it was; the app is being deployed again."})
	}
	src.actor = actor
	go p.runDeploy(src, "", "", "", "redeploy", newDeployID())
	for _, a := range appsMounting(src.App, strings.TrimPrefix(volume, config.VolumeName(src.App, ""))) {
		if b, ok := getSource(a); ok {
			b.actor = actor
			go p.runDeploy(b, "", "", "", "redeploy", newDeployID())
		}
	}
}

// moveDatabase stops a database, moves its volume, and starts it again with the same settings.
func (p *Panel) moveDatabase(d dbRow, to string) {
	ctx, cancel := context.WithTimeout(context.Background(), moveTimeout)
	defer cancel()
	_ = p.eng.Stop(d.Container)
	_ = p.eng.Remove(d.Container)
	err := p.moveVolume(ctx, d.volume(), to)
	endMove(d.volume(), err)
	if _, startErr := p.runDatabaseContainer(d, d.Public); startErr != nil && err == nil {
		err = startErr
	}
	if err != nil {
		log.Printf("skiff: moving database %s: %v", d.Name, err)
		go dispatchAlert(alertEvent{Team: d.Team, Kind: "deploy.failed", Title: "Moving database " + d.Name + " failed", Detail: err.Error()})
	}
}

func (p *Panel) handleDisks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && !p.isInstanceAdmin(r) {
		http.Error(w, "only the server's admin can change its disks", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		out := []disk{}
		for _, d := range listDisks() {
			out = append(out, withSpace(d))
		}
		writeJSON(w, map[string]any{"disks": out, "admin": p.isInstanceAdmin(r)})
	case http.MethodPost:
		var body struct{ Name, Path string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body.Name, body.Path = strings.TrimSpace(body.Name), strings.TrimSpace(body.Path)
		if body.Name == "" || len(body.Name) > 40 {
			http.Error(w, "name the disk, like data-1", http.StatusBadRequest)
			return
		}
		if problem := diskProblem(body.Path); problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		d := disk{ID: randHex(6), Name: body.Name, Path: body.Path, Created: time.Now().Unix()}
		if _, err := sqlDB.Exec(`INSERT INTO disks(id, name, path, created) VALUES(?,?,?,?)`, d.ID, d.Name, d.Path, d.Created); err != nil {
			http.Error(w, "couldn't save", http.StatusInternalServerError)
			return
		}
		p.audit(r, "disk.add", d.Name, d.Path)
		writeJSON(w, withSpace(d))
	case http.MethodDelete:
		d, ok := getDisk(r.URL.Query().Get("id"))
		if !ok {
			http.Error(w, "no such disk", http.StatusNotFound)
			return
		}
		if n := p.volumesOnDisk(d); n > 0 {
			http.Error(w, fmt.Sprintf("%d volumes and databases are still on %s; move them off first", n, d.Name), http.StatusConflict)
			return
		}
		_, _ = sqlDB.Exec(`DELETE FROM disks WHERE id=?`, d.ID)
		_, _ = sqlDB.Exec(`UPDATE volumes SET disk='' WHERE disk=?`, d.ID)
		p.audit(r, "disk.remove", d.Name, d.Path)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// volumesOnDisk counts the volumes keeping their files on a disk.
func (p *Panel) volumesOnDisk(d disk) int {
	entries, err := os.ReadDir(filepath.Join(d.Path, "skiff-volumes"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if p.eng.VolumeFolder(e.Name()) == config.DiskFolder(d.Path, e.Name()) {
			n++
		}
	}
	return n
}

var errMoveRunning = errors.New("a move of this volume is already under way")

// handleMove moves an app's volume or a database to a disk, or back to Docker's storage with an empty disk.
func (p *Panel) handleMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.isOwner(r) {
		http.Error(w, "only a team owner can move data", http.StatusForbidden)
		return
	}
	var body struct {
		App, Volume, Database, Disk string
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	to := ""
	if body.Disk != "" {
		d, ok := getDisk(body.Disk)
		if !ok || withSpace(d).Missing {
			http.Error(w, "that disk isn't mounted", http.StatusBadRequest)
			return
		}
		to = d.Path
	}
	switch {
	case body.Database != "":
		d, ok := p.canAccessDB(r, sanitizeID(body.Database))
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !startMove(d.volume(), to) {
			http.Error(w, errMoveRunning.Error(), http.StatusConflict)
			return
		}
		p.audit(r, "database.move", d.Name, cmp.Or(to, "Docker's storage"))
		go p.moveDatabase(d, to)
	case body.App != "":
		app := sanitizeName(body.App)
		src, ok := getSource(app)
		if !ok || !p.canAccess(r, app) || src.Origin == originCLI || src.Parent != "" {
			http.Error(w, "only a project's own volumes can be moved from here", http.StatusBadRequest)
			return
		}
		var vol config.Volume
		for _, v := range listVolumes(app) {
			if v.Name == body.Volume {
				vol = v
			}
		}
		if vol.Name == "" {
			http.Error(w, "no such volume", http.StatusNotFound)
			return
		}
		if vol.Owner != "" {
			http.Error(w, "this volume is "+vol.Owner+"'s; move it from there", http.StatusBadRequest)
			return
		}
		name := vol.DockerName(app)
		if !p.eng.VolumeExists(name) {
			// Not made yet: the next deploy makes it on the chosen disk.
			_, _ = sqlDB.Exec(`UPDATE volumes SET disk=? WHERE app=? AND name=?`, body.Disk, app, vol.Name)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !startMove(name, to) {
			http.Error(w, errMoveRunning.Error(), http.StatusConflict)
			return
		}
		_, _ = sqlDB.Exec(`UPDATE volumes SET disk=? WHERE app=? AND name=?`, body.Disk, app, vol.Name)
		p.audit(r, "volume.move", app, vol.Name+" → "+cmp.Or(to, "Docker's storage"))
		go p.moveAppVolume(src, name, to, p.deployActor(r))
	default:
		http.Error(w, "name a database, or an app and its volume", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
