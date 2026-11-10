package compose

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/nuelScript/skiff/internal/config"
)

type Kind string

const (
	KindApp      Kind = "app"
	KindWorker   Kind = "worker"
	KindDatabase Kind = "database"
)

// Item is one thing Skiff would create for a Compose service.
type Item struct {
	Service string
	Name    string
	Kind    Kind
	Image   string
	Build   *Build
	Command []string
	Env     map[string]string
	Port    int
	Public  bool
	Volumes []PlanVolume
	Files   []PlanFile
	Options config.RunOptions
	// Wants are machine permissions the file asks for, which the server's admin grants or not.
	Wants    config.Permissions
	Replicas int
	Memory   string
	CPUs     string
	After    []string
	// HealthPath is the HTTP path from the service's healthcheck, when it has one.
	HealthPath string
	// WorkerOf names the service whose image and settings a worker shares.
	WorkerOf string
	// Database fields.
	Engine                 string
	DBUser, DBPassword, DB string
}

// PlanVolume is a volume a service mounts. Owner is the service holding one shared across services; empty means this service holds it.
type PlanVolume struct {
	Name, Path, Owner string
	// Also lists more paths the service mounts the same volume at.
	Also []string
}

func (v PlanVolume) equal(o PlanVolume) bool {
	return v.Name == o.Name && v.Path == o.Path && v.Owner == o.Owner && slices.Equal(v.Also, o.Also)
}

// PlanFile is a file or folder from the repository that a service mounts, built into its image.
type PlanFile struct {
	Source, Target string
	// Content is written to Target instead of copying Source, for a config written in the Compose file.
	Content *string
}

// PathKind is what a path in a Compose file's repository turned out to be.
type PathKind int

const (
	PathMissing PathKind = iota
	PathFile
	PathDir
)

type Plan struct {
	Stack string
	Items []Item
	// Problems must be fixed before this file can be used.
	Problems []string
	// Notes are things that will work differently, or be ignored.
	Notes []string
	// Unset lists variables the file uses that have no value yet.
	Unset []string
}

var managedEngines = map[string]string{"postgres": "postgres", "mysql": "mysql", "mongo": "mongodb", "redis": "redis"}

// plainTCPPorts are well-known ports for databases, queues and mail, which a web address can't serve.
var plainTCPPorts = map[int]bool{
	25: true, 465: true, 587: true, 1025: true, 1883: true, 3306: true, 4222: true, 5432: true, 5672: true,
	6379: true, 6432: true, 9042: true, 11211: true, 25565: true, 27017: true,
}

var unsupportedKeys = map[string]string{}

// ignoredWhy explains the service keys Skiff leaves out that files use most.
var ignoredWhy = map[string]string{
	"restart":        "Skiff restarts a service whenever it stops",
	"container_name": "Skiff names each service's container itself",
	"hostname":       "other services reach it by its service name",
	"networks":       "a stack's services share one private network",
	"logging":        "Skiff keeps each service's logs itself",
	"labels":         "Skiff sets its own labels",
	"platform":       "Skiff builds and runs for the server's own platform",
	"pull_policy":    "Skiff pulls the image on every deploy",
	"stdin_open":     "no terminal is attached to a service",
	"tty":            "no terminal is attached to a service",
}

// loopback says whether a published address keeps a port on the machine.
func loopback(ip string) bool {
	return ip == "127.0.0.1" || ip == "localhost" || ip == "::1" || ip == "[::1]"
}

var nameUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

func safeName(s string) string {
	return strings.Trim(nameUnsafe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// ParseEnvFile reads KEY=value lines.
func ParseEnvFile(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}

// BuildPlan works out what a parsed file becomes. readFile loads env_file entries, relative to the Compose file. repoPath says what a path is in the file's repository, and is nil for a pasted file, which has no repository.
func BuildPlan(stack string, f *File, readFile func(string) ([]byte, error), repoPath func(string) PathKind) *Plan {
	p := &Plan{Stack: safeName(stack)}
	if p.Stack == "" {
		p.Problems = append(p.Problems, "the stack needs a name made of letters, digits or dashes")
	}
	p.Unset = dedupe(f.Unset)
	p.Problems = append(p.Problems, f.Problems...)
	p.Notes = append(p.Notes, f.Notes...)
	for _, name := range f.Profiled {
		p.Notes = append(p.Notes, name+": only runs with a Compose profile, so Skiff leaves it out")
	}
	for _, v := range p.Unset {
		p.Problems = append(p.Problems, fmt.Sprintf("%s needs a value. For one that can stay empty, write ${%s:-} in the file", v, v))
	}

	resolved := map[string]Service{}
	files := map[string][]PlanFile{}
	// Decided once env files are read and before mounts are resolved, which can drop the setup scripts that rule a database out.
	engines, ordinary := map[string]string{}, map[string]string{}
	for _, name := range f.Order {
		s := f.Services[name]
		env := map[string]string{}
		for _, file := range s.EnvFiles {
			data, err := readFile(file)
			if err != nil {
				p.Problems = append(p.Problems, fmt.Sprintf("%s: can't read env_file %s", name, file))
				continue
			}
			for k, v := range ParseEnvFile(data) {
				env[k] = v
			}
		}
		for k, v := range s.Environment {
			env[k] = v
		}
		s.Environment = env
		engines[name] = managedEngine(s)
		if stockEngine(s) != "" {
			ordinary[name] = unmanageable(s)
		}
		if engines[name] == "" {
			s.Volumes, files[name] = resolveMounts(name, s.Volumes, repoPath, p)
		}
		resolved[name] = s
	}

	// A named volume can only be shared between an app and its workers.
	volumeUsers := map[string][]string{}
	for _, name := range f.Order {
		for _, m := range resolved[name].Volumes {
			if m.Source != "" && !m.Bind {
				volumeUsers[m.Source] = append(volumeUsers[m.Source], name)
			}
		}
	}

	byName := map[string]*Item{}
	for _, name := range f.Order {
		s := resolved[name]
		it := Item{
			Service: name, Name: p.Stack + "-" + safeName(name), Kind: KindApp, Image: s.Image, Build: s.Build,
			Command: s.Command, Env: s.Environment, Replicas: s.Replicas, Memory: s.Memory, CPUs: s.CPUs, After: s.DependsOn,
		}
		if safeName(name) == "" {
			p.Problems = append(p.Problems, fmt.Sprintf("service %q needs a name made of letters, digits or dashes", name))
		}
		if s.Image == "" && s.Build == nil {
			p.Problems = append(p.Problems, name+": needs an image or a build")
		}
		for _, key := range f.Extra[name] {
			if why, bad := unsupportedKeys[key]; bad {
				p.Problems = append(p.Problems, fmt.Sprintf("%s: %s %s", name, key, why))
			} else if why := ignoredWhy[key]; why != "" {
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s is ignored, because %s", name, key, why))
			} else {
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s is ignored", name, key))
			}
		}
		if s.Build != nil && s.Image != "" {
			p.Notes = append(p.Notes, name+": builds from source, so image is ignored")
			it.Image = ""
		}
		if s.Build != nil {
			for _, a := range s.Build.Args {
				if k, _, _ := strings.Cut(a, "="); secretName.MatchString(k) {
					p.Notes = append(p.Notes, fmt.Sprintf("%s: build arg %s stays readable in the built image's history. Pass a secret as an environment variable instead", name, k))
				}
			}
		}
		for _, d := range s.DependsOn {
			if _, ok := f.Services[d]; !ok {
				if slices.Contains(f.Profiled, d) {
					continue
				}
				p.Problems = append(p.Problems, fmt.Sprintf("%s: depends on %s, which isn't in this file", name, d))
			}
		}

		if engine := engines[name]; engine != "" {
			applyDatabase(&it, engine, s, p, readFile)
			byName[name] = &it
			continue
		}
		if why := ordinary[name]; why != "" {
			p.Notes = append(p.Notes, name+": "+why+", so it runs as an ordinary private app instead of a managed database. Its data is covered by volume backups")
		}

		var ports []Port
		for _, pt := range s.Ports {
			if pt.Protocol != "" && pt.Protocol != "tcp" {
				p.Notes = append(p.Notes, fmt.Sprintf("%s: port %d/%s isn't reachable, because Skiff only forwards TCP", name, pt.Target, pt.Protocol))
				continue
			}
			ports = append(ports, pt)
		}
		s.Ports = ports
		web := slices.IndexFunc(s.Ports, func(pt Port) bool { return !plainTCPPorts[pt.Target] })
		switch {
		case web >= 0 && loopback(s.Ports[web].HostIP):
			it.Port = s.Ports[web].Target
			p.Notes = append(p.Notes, fmt.Sprintf("%s: port %d is published only on %s, so it stays private. Turn on public access in its settings if it should be a website", name, it.Port, s.Ports[web].HostIP))
		case web >= 0:
			it.Port, it.Public = s.Ports[web].Target, true
			if len(s.Ports) > 1 {
				p.Notes = append(p.Notes, fmt.Sprintf("%s: only port %d is served, because Skiff serves one port per app", name, it.Port))
			}
		case len(s.Ports) > 0:
			it.Port = s.Ports[0].Target
			p.Notes = append(p.Notes, fmt.Sprintf("%s: port %d doesn't serve websites, so it stays inside the stack; open a public TCP port in its settings to reach it from outside", name, it.Port))
		case len(s.Expose) > 0:
			it.Port = s.Expose[0]
		}
		it.Files = files[name]
		it.Options = s.Options
		if err := s.Options.Validate(); err != nil {
			p.Problems = append(p.Problems, name+": "+err.Error())
		}
		if !s.Wants.IsZero() {
			if err := s.Wants.Validate(); err != nil {
				p.Problems = append(p.Problems, name+": "+err.Error())
			} else {
				it.Wants = s.Wants
				p.Notes = append(p.Notes, fmt.Sprintf("%s: asks for %s, which the server's admin can grant in its settings. Until then it runs without", name, describeWants(s.Wants)))
			}
		}
		sources := map[string]string{}
		for _, m := range s.Volumes {
			vol, source := safeName(m.Source), m.Source
			if vol == "" {
				vol, source = safeName(m.Target), m.Target
			}
			if first, ok := sources[vol]; ok && first != source {
				p.Problems = append(p.Problems, fmt.Sprintf("%s: volumes %s and %s would both be named %s; rename one of them", name, first, source, vol))
				continue
			}
			sources[vol] = source
			if i := slices.IndexFunc(it.Volumes, func(v PlanVolume) bool { return v.Name == vol }); i >= 0 {
				it.Volumes[i].Also = append(it.Volumes[i].Also, m.Target)
				continue
			}
			it.Volumes = append(it.Volumes, PlanVolume{Name: vol, Path: m.Target})
		}
		if len(it.Volumes) > 5 {
			p.Problems = append(p.Problems, name+": Skiff allows 5 volumes per app")
		}
		it.HealthPath = healthPath(s.Healthcheck)
		if len(s.Healthcheck) > 0 && it.HealthPath == "" {
			p.Notes = append(p.Notes, name+": its healthcheck isn't an HTTP request, so Skiff checks that the port answers instead")
		}
		byName[name] = &it
	}

	// A service built like another one, differing only by command, is that service's worker.
	for _, name := range f.Order {
		it := byName[name]
		// A worker runs through a shell, which a custom entrypoint would replace.
		if it.Kind != KindApp || len(it.Command) == 0 || it.Port != 0 || len(it.Options.Entrypoint) > 0 {
			continue
		}
		for _, other := range f.Order {
			o := byName[other]
			if other == name || o.Kind != KindApp || o.WorkerOf != "" || !sameBuild(it, o) || o.Port == 0 {
				continue
			}
			if !sameEnv(it.Env, o.Env) || !slices.EqualFunc(it.Volumes, o.Volumes, PlanVolume.equal) {
				break
			}
			it.Kind, it.WorkerOf = KindWorker, other
			break
		}
	}

	for vol, users := range volumeUsers {
		users = dedupe(users)
		if len(users) < 2 {
			continue
		}
		owner := ""
		shared := false
		for _, u := range users {
			it := byName[u]
			if it == nil || it.Kind == KindDatabase {
				p.Problems = append(p.Problems, fmt.Sprintf("volume %s is shared with %s, a managed database, which keeps its data to itself", vol, u))
				owner = ""
				break
			}
			root := u
			if it.WorkerOf != "" {
				root = it.WorkerOf
			}
			if owner == "" {
				owner = root
			}
			shared = shared || root != owner
		}
		if owner == "" || !shared {
			continue
		}
		name := safeName(vol)
		for _, u := range users {
			it := byName[u]
			if it.WorkerOf != "" || u == owner {
				continue
			}
			for i := range it.Volumes {
				if it.Volumes[i].Name == name {
					it.Volumes[i].Owner = owner
				}
			}
		}
		p.Notes = append(p.Notes, fmt.Sprintf("volume %s is shared by %s and %s; %s holds it and its backups", name, strings.Join(users[:len(users)-1], ", "), users[len(users)-1], owner))
	}

	order, cycle := startOrder(f.Order, resolved)
	if cycle != "" {
		p.Problems = append(p.Problems, "depends_on goes in a circle through "+cycle)
	}
	for _, name := range order {
		p.Items = append(p.Items, *byName[name])
	}
	if !slices.ContainsFunc(p.Items, func(i Item) bool { return i.Public }) {
		p.Notes = append(p.Notes, "no service publishes a port, so nothing will be reachable from outside")
	}
	slices.Sort(p.Problems)
	slices.Sort(p.Notes)
	return p
}

// systemPaths are places on the server that an app has no business reading or changing.
var systemPaths = []string{"/bin", "/boot", "/dev", "/etc", "/lib", "/lib64", "/proc", "/root", "/run", "/sbin", "/sys", "/usr", "/var/lib/docker", "/var/run"}

func isSystemPath(path string) bool {
	if path == "/" {
		return true
	}
	return slices.ContainsFunc(systemPaths, func(root string) bool {
		return path == root || strings.HasPrefix(path, root+"/")
	})
}

// secretName matches names that usually hold a secret.
var secretName = regexp.MustCompile(`(?i)secret|token|passw|api_?key|private_?key|credential`)

var fileLike = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)

// resolveMounts decides what each path a service mounts becomes, since an app can't reach into the server's disk. Anything that keeps being a mount comes back as a named volume.
func resolveMounts(service string, mounts []Mount, repoPath func(string) PathKind, p *Plan) ([]Mount, []PlanFile) {
	out := make([]Mount, 0, len(mounts))
	var files []PlanFile
	asVolume := func(m Mount) {
		m.Source, m.Bind = safeName(m.Source), false
		out = append(out, m)
	}
	for _, m := range mounts {
		if m.Content != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("%s: %s is built into its image, so changes reach it on the next deploy", service, m.Need))
			files = append(files, PlanFile{Target: m.Target, Content: m.Content})
			continue
		}
		if m.Need != "" {
			path := strings.TrimRight(m.Source, "/")
			switch {
			case strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~"):
				p.Problems = append(p.Problems, fmt.Sprintf("%s: %s comes from %s on the server, so give it a file in the repository", service, m.Need, m.Source))
			case repoPath == nil:
				p.Problems = append(p.Problems, fmt.Sprintf("%s: %s comes from %s, which only works from a repository, so deploy this stack from one instead of pasting the file", service, m.Need, m.Source))
			case repoPath(path) == PathMissing:
				p.Problems = append(p.Problems, fmt.Sprintf("%s: %s comes from %s, which isn't in the repository", service, m.Need, m.Source))
			default:
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s is built into its image from the repository, so changes reach it on the next deploy", service, m.Need))
				files = append(files, PlanFile{Source: path, Target: m.Target})
			}
			continue
		}
		if !m.Bind {
			out = append(out, m)
			continue
		}
		path := strings.TrimRight(m.Source, "/")
		switch {
		case path == "/etc/localtime" || path == "/etc/timezone":
			p.Notes = append(p.Notes, fmt.Sprintf("%s: uses UTC, because the server's %s isn't shared. Set TZ to change it", service, path))
		case path == "/var/run/docker.sock" || path == "/run/docker.sock":
			p.Problems = append(p.Problems, fmt.Sprintf("%s: mounts the Docker socket, which would give it control of the whole server, so Skiff doesn't allow it", service))
		case strings.HasPrefix(path, "/") || m.Source == "/":
			if isSystemPath(cmp.Or(path, "/")) {
				p.Problems = append(p.Problems, fmt.Sprintf("%s: mounts %s, a system folder on the server, which Skiff doesn't share with apps", service, m.Source))
				continue
			}
			p.Notes = append(p.Notes, fmt.Sprintf("%s: %s becomes a volume that starts empty, since apps can't reach the server's own folders", service, m.Source))
			asVolume(m)
		case strings.HasPrefix(path, "~"):
			p.Notes = append(p.Notes, fmt.Sprintf("%s: %s becomes a volume that starts empty, since apps can't reach the server's own folders", service, m.Source))
			asVolume(m)
		case repoPath == nil:
			if m.ReadOnly || fileLike.MatchString(path) {
				p.Problems = append(p.Problems, fmt.Sprintf("%s: mounts %s, which only works from a repository, so deploy this stack from one instead of pasting the file", service, m.Source))
				continue
			}
			p.Notes = append(p.Notes, fmt.Sprintf("%s: %s becomes a volume that starts empty", service, m.Source))
			asVolume(m)
		default:
			switch kind := repoPath(path); {
			case kind == PathMissing:
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s isn't in the repository, so it becomes a volume that starts empty", service, m.Source))
				asVolume(m)
			case kind == PathFile && !m.ReadOnly:
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s is built into its image from the repository, so changes the app makes to it are lost on each deploy. To keep them, mount the folder it's in instead", service, m.Source))
				files = append(files, PlanFile{Source: path, Target: m.Target})
			case kind == PathFile || m.ReadOnly:
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s is built into its image from the repository, so changes reach it on the next deploy", service, m.Source))
				files = append(files, PlanFile{Source: path, Target: m.Target})
			default:
				p.Notes = append(p.Notes, fmt.Sprintf("%s: %s becomes a volume that starts with the repository's copy and keeps its own changes", service, m.Source))
				files = append(files, PlanFile{Source: path, Target: m.Target})
				asVolume(m)
			}
		}
	}
	return out, files
}

// withFileValues fills a database's KEY from KEY_FILE when that file is one of its secrets or configs, as the official images do at start.
func withFileValues(s Service, readFile func(string) ([]byte, error)) map[string]string {
	env := maps.Clone(s.Environment)
	for k, v := range s.Environment {
		key, ok := strings.CutSuffix(k, "_FILE")
		if !ok || env[key] != "" {
			continue
		}
		for _, m := range s.Volumes {
			if m.Need == "" || m.Target != v {
				continue
			}
			if m.Content != nil {
				env[key] = *m.Content
			} else if data, err := readFile(m.Source); err == nil {
				env[key] = strings.TrimRight(string(data), "\r\n")
			}
		}
	}
	return env
}

// managedEngine is the engine a stock database image becomes, unless the file asks for something a managed database can't keep.
func managedEngine(s Service) string {
	if unmanageable(s) != "" {
		return ""
	}
	return stockEngine(s)
}

// unmanageable says why a database must run as the file wrote it: its own setup scripts only run on the image's first start, and one left without a password has apps that connect without one, which a managed database's generated password would lock out.
func unmanageable(s Service) string {
	if slices.ContainsFunc(s.Volumes, func(m Mount) bool {
		return m.Target == initScriptsDir || strings.HasPrefix(m.Target, initScriptsDir+"/")
	}) {
		return "runs its own setup scripts"
	}
	env := s.Environment
	switch stockEngine(s) {
	case "postgres":
		if env["POSTGRES_HOST_AUTH_METHOD"] == "trust" {
			return "lets apps in without a password"
		}
	case "mysql":
		if env["MYSQL_ALLOW_EMPTY_PASSWORD"] != "" && env["MYSQL_PASSWORD"] == "" && env["MYSQL_ROOT_PASSWORD"] == "" {
			return "lets apps in without a password"
		}
	case "mongodb":
		if env["MONGO_INITDB_ROOT_USERNAME"] == "" && env["MONGO_INITDB_ROOT_USERNAME_FILE"] == "" {
			return "lets apps in without a password"
		}
	}
	return ""
}

const initScriptsDir = "/docker-entrypoint-initdb.d"

func stockEngine(s Service) string {
	if s.Build != nil || s.Image == "" {
		return ""
	}
	repo, _, _ := strings.Cut(s.Image, "@")
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	repo = strings.TrimPrefix(strings.TrimPrefix(repo, "docker.io/"), "library/")
	return managedEngines[repo]
}

func applyDatabase(it *Item, engine string, s Service, p *Plan, readFile func(string) ([]byte, error)) {
	it.Kind, it.Engine = KindDatabase, engine
	env := withFileValues(s, readFile)
	switch engine {
	case "postgres":
		it.DBUser, it.DBPassword, it.DB = env["POSTGRES_USER"], env["POSTGRES_PASSWORD"], env["POSTGRES_DB"]
		if it.DBUser == "" {
			it.DBUser = "postgres"
		}
		if it.DB == "" {
			it.DB = it.DBUser
		}
	case "mysql":
		it.DBUser, it.DBPassword, it.DB = env["MYSQL_USER"], env["MYSQL_PASSWORD"], env["MYSQL_DATABASE"]
		if it.DBUser == "" {
			it.DBUser, it.DBPassword = "root", env["MYSQL_ROOT_PASSWORD"]
		} else if root := env["MYSQL_ROOT_PASSWORD"]; root != "" && root != it.DBPassword {
			p.Notes = append(p.Notes, it.Service+": its root password becomes the same as "+it.DBUser+"'s, since Skiff keeps one password per database")
		}
	case "mongodb":
		it.DBUser, it.DBPassword, it.DB = env["MONGO_INITDB_ROOT_USERNAME"], env["MONGO_INITDB_ROOT_PASSWORD"], env["MONGO_INITDB_DATABASE"]
	case "redis":
		for i, a := range s.Command {
			if a == "--requirepass" && i+1 < len(s.Command) {
				it.DBPassword = s.Command[i+1]
			}
		}
	}
	if it.DBPassword == "" && engine == "redis" {
		p.Notes = append(p.Notes, it.Service+": has no password, as in your file. Only this stack's services can reach it")
	} else if it.DBPassword == "" && len(p.Unset) == 0 {
		p.Notes = append(p.Notes, it.Service+": has no password, so Skiff generates one. Apps that connect with a hardcoded URL need updating")
	}
	if !s.Options.IsZero() || !s.Wants.IsZero() {
		p.Notes = append(p.Notes, it.Service+": is a managed database, so Skiff chooses how it starts and ignores settings like user, entrypoint and cap_add")
	}
	if len(s.Ports) > 0 {
		p.Notes = append(p.Notes, it.Service+": stays private, so its published port is ignored. Turn on public access in the dashboard if you need it")
	}
	if ignored := ignoredFlags(s.Command, it.DBPassword); len(ignored) > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%s: is a managed database, so its command's %s isn't used; Skiff starts it with its own settings", it.Service, strings.Join(ignored, " ")))
	}
	for _, m := range s.Volumes {
		switch {
		case m.Bind && fileLike.MatchString(strings.TrimRight(m.Source, "/")):
			p.Notes = append(p.Notes, fmt.Sprintf("%s: is a managed database, so the file %s mounted at %s isn't used; Skiff starts it with its own settings", it.Service, m.Source, m.Target))
		case m.Bind:
			p.Notes = append(p.Notes, fmt.Sprintf("%s: starts empty, so the data in %s isn't brought over. Load it with a restore", it.Service, m.Source))
		}
	}
}

// ignoredFlags is what a database's command sets beyond starting the server and its password, which Skiff takes over.
func ignoredFlags(command []string, password string) []string {
	var out []string
	for i, a := range command {
		switch {
		case i == 0 && slices.Contains([]string{"postgres", "mysqld", "redis-server", "mongod", "clickhouse-server"}, a):
		case a == "--requirepass", password != "" && a == password:
		default:
			out = append(out, a)
		}
	}
	return out
}

var healthURL = regexp.MustCompile(`https?://[^/\s'"]+(/[^\s'"]*)?`)

func healthPath(test []string) string {
	m := healthURL.FindStringSubmatch(strings.Join(test, " "))
	if m == nil {
		return ""
	}
	if m[1] == "" {
		return "/"
	}
	return m[1]
}

func sameBuild(a, b *Item) bool {
	if !slices.Equal(a.Files, b.Files) || !a.Options.Equal(b.Options) {
		return false
	}
	if a.Build != nil && b.Build != nil {
		return a.Build.equal(*b.Build)
	}
	return a.Build == nil && b.Build == nil && a.Image == b.Image
}

func sameEnv(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// startOrder lists services after the ones they depend on, keeping file order otherwise.
func startOrder(names []string, services map[string]Service) ([]string, string) {
	state := map[string]int{}
	var out []string
	cycle := ""
	var visit func(string)
	visit = func(n string) {
		switch state[n] {
		case 1:
			if cycle == "" {
				cycle = n
			}
			return
		case 2:
			return
		}
		state[n] = 1
		for _, d := range services[n].DependsOn {
			if _, ok := services[d]; ok {
				visit(d)
			}
		}
		state[n] = 2
		out = append(out, n)
	}
	for _, n := range names {
		visit(n)
	}
	return out, cycle
}

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// describeWants names what a service asks for, in one phrase.
func describeWants(w config.Permissions) string {
	var parts []string
	if w.Privileged {
		parts = append(parts, "privileged mode")
	}
	if w.HostNetwork {
		parts = append(parts, "the server's own network")
	}
	if len(w.Capabilities) > 0 {
		parts = append(parts, "the capabilities "+strings.Join(w.Capabilities, ", "))
	}
	if len(w.Devices) > 0 {
		parts = append(parts, "the devices "+strings.Join(w.Devices, ", "))
	}
	if len(w.Sysctls) > 0 {
		parts = append(parts, "the sysctls "+strings.Join(w.Sysctls, ", "))
	}
	return strings.Join(parts, " and ")
}
