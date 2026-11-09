package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/registry"
)

func listVolumes(app string) []config.Volume {
	rows, err := sqlDB.Query(`SELECT name, path, owner, also, disk FROM volumes WHERE app=? ORDER BY created, name`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []config.Volume
	var disks []string
	for rows.Next() {
		var v config.Volume
		var also, diskID string
		if rows.Scan(&v.Name, &v.Path, &v.Owner, &also, &diskID) == nil {
			_ = json.Unmarshal([]byte(also), &v.Also)
			out = append(out, v)
			disks = append(disks, diskID)
		}
	}
	rows.Close()
	// The disk's folder is looked up once the rows are read: the database has one connection.
	for i, id := range disks {
		if id != "" {
			out[i].Disk = diskPathFor(id)
		}
	}
	return out
}

func addVolume(app string, v config.Volume) error {
	return addVolumeOn(app, v, "")
}

// addVolumeOn records a volume whose first deploy makes it on a disk, or in Docker's storage with an empty diskID.
func addVolumeOn(app string, v config.Volume, diskID string) error {
	also := ""
	if len(v.Also) > 0 {
		b, _ := json.Marshal(v.Also)
		also = string(b)
	}
	_, err := sqlDB.Exec(`INSERT INTO volumes (app, name, path, created, owner, also, disk) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		app, v.Name, v.Path, time.Now().Unix(), v.Owner, also, diskID)
	return err
}

func deleteVolume(app, name string) bool {
	res, err := sqlDB.Exec(`DELETE FROM volumes WHERE app=? AND name=?`, app, name)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// volumeDefs is what an app should mount: a CLI app's come from its skiff.toml (read off the running container), a preview uses its project's set on volumes of its own.
func (p *Panel) volumeDefs(src Source) []config.Volume {
	if src.Origin == originCLI {
		return p.mountedVolumes(src.App)
	}
	if src.Parent == "" {
		return listVolumes(src.App)
	}
	vols := listVolumes(src.Parent)
	for i := range vols {
		vols[i].Owner = ""
	}
	return vols
}

// volumeBorrowers is every app outside app's stack that mounts the volume app holds under name, or any of its volumes with an empty name.
func volumeBorrowers(app, name string) []string {
	holder, _ := getSource(app)
	return slices.DeleteFunc(appsMounting(app, name), func(a string) bool {
		s, ok := getSource(a)
		return ok && holder.Stack != "" && s.Stack == holder.Stack
	})
}

// appsMounting is every other app that mounts the volume app holds under name, or any of its volumes with an empty name.
func appsMounting(app, name string) []string {
	rows, err := sqlDB.Query(`SELECT DISTINCT app FROM volumes WHERE owner=? AND (?='' OR name=?) ORDER BY app`, app, name, name)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var apps []string
	for rows.Next() {
		var a string
		if rows.Scan(&a) == nil {
			apps = append(apps, a)
		}
	}
	return apps
}

// shareable says whether a project can hold a volume other apps mount: stacks share among their own services, and previews and CLI apps don't keep theirs here.
func shareable(s Source) bool {
	return s.Stack == "" && volumeKind(s) == "project"
}

type sharedVolume struct {
	App  string `json:"app"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// handleSharedVolumes lists the volumes of the team's other projects that app could mount.
func (p *Panel) handleSharedVolumes(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	out := []sharedVolume{}
	for _, s := range allSources() {
		if s.App == app || s.Team != src.Team || !shareable(s) || !p.canAccess(r, s.App) {
			continue
		}
		for _, v := range listVolumes(s.App) {
			if v.Owner == "" {
				out = append(out, sharedVolume{App: s.App, Name: v.Name, Path: v.Path})
			}
		}
	}
	writeJSON(w, out)
}

// borrow checks that app may mount the volume holder keeps under name.
func (p *Panel) borrow(r *http.Request, src Source, holder, name string) string {
	h, ok := getSource(holder)
	switch {
	case !ok || h.Team != src.Team || !p.canAccess(r, holder):
		return "no such app"
	case holder == src.App:
		return "this volume is already this app's"
	case !shareable(src):
		return "a stack's services share volumes in its compose file"
	case !shareable(h):
		return "only a project's own volumes can be shared"
	case !slices.ContainsFunc(listVolumes(holder), func(v config.Volume) bool { return v.Name == name && v.Owner == "" }):
		return holder + " has no volume named " + name
	}
	return ""
}

// runtimeMounts is the volume mounts of the app's running container.
func (p *Panel) runtimeMounts(app string) []docker.Mount {
	apps, err := registry.Load()
	if err != nil {
		return nil
	}
	a, ok := apps[app]
	if !ok || a.Container == "" {
		return nil
	}
	rt, err := p.eng.Runtime(a.Container)
	if err != nil {
		return nil
	}
	return rt.Mounts
}

// mountedVolumes reads the volumes the app's running container has, as defined in skiff.toml.
func (p *Panel) mountedVolumes(app string) []config.Volume {
	prefix := config.VolumeName(app, "")
	var out []config.Volume
	for _, m := range p.runtimeMounts(app) {
		if name, ok := strings.CutPrefix(m.Name, prefix); ok {
			if i := slices.IndexFunc(out, func(v config.Volume) bool { return v.Name == name }); i >= 0 {
				out[i].Also = append(out[i].Also, m.Destination)
				continue
			}
			out = append(out, config.Volume{Name: name, Path: m.Destination})
		}
	}
	return out
}

// appMounts is the volume set for containers the panel starts itself (scale-ups, workers, jobs), so they see the same files as the app.
func (p *Panel) appMounts(src Source) []docker.Mount {
	var out []docker.Mount
	for _, v := range p.volumeDefs(src) {
		name := v.DockerName(src.App)
		if p.eng.EnsureAppVolume(name, v.Holder(src.App), v.Name) != nil {
			continue
		}
		out = append(out, docker.Mount{Name: name, Destination: v.Path})
	}
	return out
}

// pruneVolumes deletes volumes removed from a project once a deploy has moved its containers off them; CLI apps keep theirs, since their skiff.toml lives elsewhere.
func (p *Panel) pruneVolumes(src Source) {
	if src.Origin == originCLI {
		return
	}
	keep := map[string]bool{}
	for _, v := range p.volumeDefs(src) {
		keep[v.DockerName(src.App)] = true
	}
	for _, name := range p.eng.AppVolumes(src.App) {
		if !keep[name] {
			_ = p.eng.RemoveVolume(name)
			removeVolumeBackups(src.App, strings.TrimPrefix(name, config.VolumeName(src.App, "")))
		}
	}
}

func (p *Panel) removeAppVolumes(app string) {
	removeAppVolumeBackups(app)
	for _, name := range p.eng.AppVolumes(app) {
		_ = p.eng.RemoveVolume(name)
	}
}

var volSizes struct {
	sync.Mutex
	at    time.Time
	sizes map[string]int64
}

func (p *Panel) volumeSizes() map[string]int64 {
	volSizes.Lock()
	defer volSizes.Unlock()
	if volSizes.sizes == nil || time.Since(volSizes.at) > time.Minute {
		volSizes.sizes = p.eng.VolumeSizes()
		volSizes.at = time.Now()
	}
	return volSizes.sizes
}

type volumeView struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Attached bool   `json:"attached"`
	// Budget is the size, in bytes, past which the team hears about it; 0 means none.
	Budget int64 `json:"budget"`
	// Disk is the added disk it keeps its files on; empty means Docker's own storage.
	Disk string    `json:"disk,omitempty"`
	Move moveState `json:"move"`
	// Owner is the project whose volume this app mounts; empty means the app's own.
	Owner string `json:"owner,omitempty"`
	// UsedBy is the other projects that mount this volume.
	UsedBy []string `json:"usedBy,omitempty"`
}

// volumeKind says who sets an app's volumes: the dashboard for a project, skiff.toml for a CLI app, the parent project for a preview.
func volumeKind(src Source) string {
	switch {
	case src.Origin == originCLI:
		return "cli"
	case src.Parent != "":
		return "preview"
	}
	return "project"
}

// volumeLock explains why an app's volumes can't be changed here, or is empty when they can.
func volumeLock(src Source) string {
	if kind := volumeKind(src); kind != "project" {
		return "this app's volumes are set by " + map[string]string{"cli": "its skiff.toml", "preview": "its project"}[kind]
	}
	return ""
}

func (p *Panel) volumeViews(src Source) []volumeView {
	mounted := map[string]bool{}
	for _, m := range p.runtimeMounts(src.App) {
		mounted[m.Name+"\x00"+m.Destination] = true
	}
	sizes := p.volumeSizes()
	budgets := volumeBudgets(src.App)
	disks := listDisks()
	out := []volumeView{}
	for _, v := range p.volumeDefs(src) {
		name := v.DockerName(src.App)
		view := volumeView{
			Name: v.Name, Path: v.Path,
			Size:     sizes[name],
			Attached: mounted[name+"\x00"+v.Path] || (v.Owner != "" && src.Stack != ""),
			Budget:   budgets[v.Name],
			Move:     moveStatus(name),
		}
		if src.Stack == "" {
			view.Owner = v.Owner
			if v.Owner == "" {
				view.UsedBy = volumeBorrowers(src.App, v.Name)
			}
		}
		if folder := p.eng.VolumeFolder(name); folder != "" {
			view.Disk = diskOf(folder, disks)
		} else if !p.eng.VolumeExists(name) && v.Disk != "" {
			for _, d := range disks {
				if d.Path == v.Disk {
					view.Disk = d.ID
				}
			}
		}
		out = append(out, view)
	}
	return out
}

// newVolume checks a volume added by hand against the app's others, as this app's own.
func newVolume(app string, v config.Volume) (config.Volume, string) {
	v.Name, v.Path, v.Owner = strings.TrimSpace(v.Name), strings.TrimSpace(v.Path), ""
	if err := config.ValidateVolumes(append(listVolumes(app), v)); err != nil {
		return v, err.Error()
	}
	return v, ""
}

func (p *Panel) handleVolumes(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	src, ok := getSource(app)
	if !ok || !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if lock := volumeLock(src); r.Method != http.MethodGet && lock != "" {
		http.Error(w, lock, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"kind": volumeKind(src), "volumes": p.volumeViews(src)})

	case http.MethodPost:
		var v config.Volume
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&v); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		// From the dashboard, disk names a listed disk rather than a folder.
		diskID := v.Disk
		v.Disk = ""
		if diskID != "" {
			if _, ok := getDisk(diskID); !ok {
				http.Error(w, "no such disk", http.StatusBadRequest)
				return
			}
		}
		holder := sanitizeName(v.Owner)
		v, problem := newVolume(app, v)
		if problem == "" && holder != "" {
			if diskID != "" {
				problem = "a shared volume stays on " + holder + "'s disk"
			} else if problem = p.borrow(r, src, holder, v.Name); problem == "" {
				v.Owner = holder
			}
		}
		if problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		if err := addVolumeOn(app, v, diskID); err != nil {
			http.Error(w, "couldn't save the volume", http.StatusInternalServerError)
			return
		}
		detail := v.Name + " → " + v.Path
		if v.Owner != "" {
			detail = v.Owner + "'s " + detail
		}
		p.audit(r, "volume.add", app, detail)
		w.WriteHeader(http.StatusCreated)

	case http.MethodPut:
		var body struct {
			Name   string `json:"name"`
			Budget int64  `json:"budget"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || body.Budget < 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		res, err := sqlDB.Exec(`UPDATE volumes SET budget=? WHERE app=? AND name=?`, body.Budget, app, body.Name)
		if n, _ := res.RowsAffected(); err != nil || n == 0 {
			http.Error(w, "no such volume", http.StatusNotFound)
			return
		}
		p.audit(r, "volume.budget", app, fmt.Sprintf("%s: %d", body.Name, body.Budget))
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if users := volumeBorrowers(app, name); len(users) > 0 {
			http.Error(w, strings.Join(users, ", ")+" also use this volume. Remove it from them first.", http.StatusConflict)
			return
		}
		if !deleteVolume(app, name) {
			http.Error(w, "no such volume", http.StatusNotFound)
			return
		}
		p.audit(r, "volume.remove", app, name)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func volumeBudgets(app string) map[string]int64 {
	out := map[string]int64{}
	rows, err := sqlDB.Query(`SELECT name, budget FROM volumes WHERE app=? AND budget > 0`, app)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var b int64
		if rows.Scan(&name, &b) == nil {
			out[name] = b
		}
	}
	return out
}
