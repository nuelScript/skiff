package panel

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nuelScript/skiff/internal/config"
	"github.com/nuelScript/skiff/internal/seal"
)

// getRegistryCreds is an app's registry login and the registry it's for; a login saved before the registry was kept with it has no host.
func getRegistryCreds(app string) (host, user, pass string, ok bool) {
	err := sqlDB.QueryRow(`SELECT host,username,password FROM registry_creds WHERE app=?`, app).Scan(&host, &user, &pass)
	if err != nil {
		return "", "", "", false
	}
	_ = seal.OpenAll(&pass)
	return host, user, pass, true
}

func setRegistryCreds(app, host, user, pass string) error {
	pass, err := seal.Seal(pass)
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(`INSERT INTO registry_creds(app,host,username,password) VALUES(?,?,?,?)
		ON CONFLICT(app) DO UPDATE SET host=excluded.host, username=excluded.username, password=excluded.password`, app, host, user, pass)
	return err
}

// backfillRegistryHosts gives a login saved before hosts were kept the registry of the image its app runs now.
func backfillRegistryHosts() {
	rows, err := sqlDB.Query(`SELECT c.app, s.image FROM registry_creds c JOIN sources s ON s.app=c.app WHERE c.host='' AND s.image<>''`)
	if err != nil {
		return
	}
	type pair struct{ app, image string }
	var todo []pair
	for rows.Next() {
		var x pair
		if rows.Scan(&x.app, &x.image) == nil {
			todo = append(todo, x)
		}
	}
	rows.Close()
	for _, x := range todo {
		_, _ = sqlDB.Exec(`UPDATE registry_creds SET host=? WHERE app=? AND host=''`, config.RegistryHost(x.image), x.app)
	}
}

// pullLogin is the login to pull image with: only the one saved for that image's registry, so changing the image can't send it somewhere else.
func pullLogin(app, image string) (user, pass string, ok bool) {
	host, user, pass, ok := getRegistryCreds(app)
	if !ok || host == "" {
		return "", "", false
	}
	return user, pass, host == config.RegistryHost(image)
}

func deleteRegistryCreds(app string) {
	_, _ = sqlDB.Exec(`DELETE FROM registry_creds WHERE app=?`, app)
}

// validPort accepts an empty port (use the one the image exposes) or 1-65535.
func validPort(s string) bool {
	if s == "" {
		return true
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1 && n <= 65535
}

// handleImageProject creates a project that runs a prebuilt image and starts its first deploy.
func (p *Panel) handleImageProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name, Image, Port, Username, Password string
		Agent                                 bool
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body)
	name := sanitizeName(body.Name)
	img := strings.TrimSpace(body.Image)
	port := strings.TrimSpace(body.Port)
	user := strings.TrimSpace(body.Username)
	switch {
	case name == "" || name != strings.TrimSpace(body.Name):
		http.Error(w, "name must be lowercase letters, digits or dashes", http.StatusBadRequest)
		return
	case config.ValidateImage(img) != nil:
		http.Error(w, config.ValidateImage(img).Error(), http.StatusBadRequest)
		return
	case !validPort(port):
		http.Error(w, "port must be a number from 1 to 65535", http.StatusBadRequest)
		return
	case user != "" && body.Password == "":
		http.Error(w, "enter the registry password or token", http.StatusBadRequest)
		return
	}
	team := p.teamID(r)
	if _, exists := getSource(name); exists {
		http.Error(w, "a project with that name already exists", http.StatusConflict)
		return
	}
	if nameReserved(name, team) {
		http.Error(w, reservedNameMsg, http.StatusBadRequest)
		return
	}
	if envStage.heldByOther(name, team, time.Now()) {
		_ = setEnv(name, nil)
	}
	src := Source{App: name, Team: team, Port: port, Image: img}
	if body.Agent {
		applyAgentPreset(&src)
	}
	if err := putSource(src); err != nil {
		http.Error(w, "couldn't create the project", http.StatusInternalServerError)
		return
	}
	if body.Agent {
		_ = setupAgent(src)
	}
	envStage.release(name)
	if user != "" {
		_ = setRegistryCreds(name, config.RegistryHost(img), user, body.Password)
	}
	id := newDeployID()
	p.audit(r, "deploy", name, "new app from image")
	src.actor = p.deployActor(r)
	go p.runDeploy(src, "", "", "", "manual", id)
	writeJSON(w, map[string]string{"app": name, "id": id})
}

// handleRegistry manages the credentials an image project pulls with; the password is never sent back.
func (p *Panel) handleRegistry(w http.ResponseWriter, r *http.Request) {
	app := sanitizeName(r.URL.Query().Get("app"))
	if !p.canAccess(r, app) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	src, ok := getSource(app)
	if !ok || src.Image == "" || src.Parent != "" {
		http.Error(w, "only projects that run an image have registry credentials", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		saved, user, _, set := getRegistryCreds(app)
		host := config.RegistryHost(src.Image)
		// A login for another registry isn't used for this image, so it doesn't count as set.
		if saved != "" && saved != host {
			user, set = "", false
		}
		writeJSON(w, map[string]any{"username": user, "set": set, "host": host})
	case http.MethodPut:
		var body struct{ Username, Password string }
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body)
		user := strings.TrimSpace(body.Username)
		if user == "" || body.Password == "" {
			http.Error(w, "enter both the username and the password or token", http.StatusBadRequest)
			return
		}
		if err := setRegistryCreds(app, config.RegistryHost(src.Image), user, body.Password); err != nil {
			http.Error(w, "couldn't save the credentials", http.StatusInternalServerError)
			return
		}
		p.audit(r, "registry.update", app, "")
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		deleteRegistryCreds(app)
		p.audit(r, "registry.delete", app, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
