package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Frequent reads go to the Docker socket directly: forking the docker CLI costs tens of milliseconds and, for anything long-lived, about 30 MB per process.

var (
	socketOnce   sync.Once
	socketClient *http.Client
)

func dockerSocket() string {
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		if p, ok := strings.CutPrefix(h, "unix://"); ok {
			return p
		}
		return ""
	}
	candidates := []string{"/var/run/docker.sock"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".docker", "run", "docker.sock"))
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return p
		}
	}
	return ""
}

// api is the socket client, or nil when this engine is remote or no socket is reachable, in which case callers use the CLI.
func (e *Engine) api() *http.Client {
	if e.host != "" {
		return nil
	}
	socketOnce.Do(func() {
		path := dockerSocket()
		if path == "" {
			return
		}
		socketClient = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", path)
			},
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     30 * time.Second,
		}}
	})
	return socketClient
}

func apiGet(ctx context.Context, c *http.Client, path string, query url.Values) (*http.Response, error) {
	u := "http://docker" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("docker api %s: %s", path, resp.Status)
	}
	return resp, nil
}

type apiContainer struct {
	Names  []string
	State  string
	Labels map[string]string
	Ports  []struct {
		PrivatePort int
		PublicPort  int
		Type        string
	}
}

func apiList(c *http.Client, all bool, label string) ([]apiContainer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q := url.Values{}
	if all {
		q.Set("all", "1")
	}
	if label != "" {
		f, _ := json.Marshal(map[string][]string{"label": {label}})
		q.Set("filters", string(f))
	}
	resp, err := apiGet(ctx, c, "/containers/json", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []apiContainer
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// routesFromList reads each app's published port for the container port it declared.
func routesFromList(list []apiContainer) []Route {
	routes := make([]Route, 0, len(list))
	for _, c := range list {
		app, want := c.Labels["skiff.app"], c.Labels["skiff.port"]
		cport, err := strconv.Atoi(want)
		if app == "" || err != nil {
			continue
		}
		udp, _ := strconv.Atoi(c.Labels["skiff.udp"])
		route := Route{App: app}
		if len(c.Names) > 0 {
			route.Container = strings.TrimPrefix(c.Names[0], "/")
		}
		// On the server's own network, the app's ports are the server's.
		if c.Labels["skiff.hostnet"] == "1" {
			route.HostPort, route.UDPHostPort = cport, udp
			routes = append(routes, route)
			continue
		}
		for _, p := range c.Ports {
			switch {
			case p.PrivatePort == cport && p.PublicPort > 0 && (p.Type == "" || p.Type == "tcp"):
				route.HostPort = p.PublicPort
			case udp > 0 && p.PrivatePort == udp && p.PublicPort > 0 && p.Type == "udp":
				route.UDPHostPort = p.PublicPort
			}
		}
		if route.HostPort > 0 {
			routes = append(routes, route)
		}
	}
	return routes
}

const stateTTL = time.Second

// stateCache answers State for every container from one listing, so a page showing fifty apps makes one request.
var stateCache struct {
	sync.Mutex
	states map[string]string
	at     time.Time
}

func invalidateStates() {
	stateCache.Lock()
	stateCache.states = nil
	stateCache.Unlock()
}

func apiState(c *http.Client, name string) (string, bool) {
	stateCache.Lock()
	if stateCache.states == nil || time.Since(stateCache.at) > stateTTL {
		list, err := apiList(c, true, "")
		if err != nil {
			stateCache.Unlock()
			return "", false
		}
		m := make(map[string]string, len(list))
		for _, ct := range list {
			for _, n := range ct.Names {
				m[strings.TrimPrefix(n, "/")] = ct.State
			}
		}
		stateCache.states, stateCache.at = m, time.Now()
	}
	state, ok := stateCache.states[name]
	stateCache.Unlock()
	if ok {
		return state, true
	}
	// Not in the listing: created a moment ago, or truly gone. Ask about this one.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := apiGet(ctx, c, "/containers/"+url.PathEscape(name)+"/json", nil)
	if err != nil {
		return "missing", true
	}
	defer resp.Body.Close()
	var one struct{ State struct{ Status string } }
	if json.NewDecoder(resp.Body).Decode(&one) != nil || one.State.Status == "" {
		return "missing", true
	}
	return one.State.Status, true
}

// apiFollowLogs streams a container's output in this process; it can't outlive the panel the way a child `docker logs` can.
func apiFollowLogs(ctx context.Context, c *http.Client, container string, since time.Time, line func(ts time.Time, stream, text string)) error {
	resp, err := apiGet(ctx, c, "/containers/"+url.PathEscape(container)+"/json", nil)
	if err != nil {
		return err
	}
	var info struct{ Config struct{ Tty bool } }
	err = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if err != nil {
		return err
	}
	q := url.Values{"follow": {"1"}, "stdout": {"1"}, "stderr": {"1"}, "timestamps": {"1"}}
	if !since.IsZero() {
		q.Set("since", fmt.Sprintf("%d.%09d", since.Unix(), since.Nanosecond()))
	}
	resp, err = apiGet(ctx, c, "/containers/"+url.PathEscape(container)+"/logs", q)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if info.Config.Tty {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			emitLogLine(sc.Text(), "out", line)
		}
		return sc.Err()
	}
	return demuxLogs(resp.Body, line)
}

// WatchOOM calls fn with the app whenever one of its containers is killed for exceeding its memory limit, until ctx ends. It reports an error when events can't be read, and needs the local Docker socket.
func (e *Engine) WatchOOM(ctx context.Context, fn func(app, container string)) error {
	return e.watchApps(ctx, "oom", func(app, container string, _ bool, _ map[string]string) { fn(app, container) })
}

// WatchDeaths calls fn whenever one of an app's or worker's containers exits, with its exit code, until ctx ends.
func (e *Engine) WatchDeaths(ctx context.Context, fn func(app, container string, worker bool, exitCode int)) error {
	return e.watchApps(ctx, "die", func(app, container string, worker bool, attrs map[string]string) {
		code, _ := strconv.Atoi(attrs["exitCode"])
		fn(app, container, worker, code)
	})
}

// watchApps follows one kind of container event for Skiff's apps and workers.
func (e *Engine) watchApps(ctx context.Context, event string, fn func(app, container string, worker bool, attrs map[string]string)) error {
	c := e.api()
	if c == nil {
		return errors.New("no docker socket")
	}
	f, _ := json.Marshal(map[string][]string{"type": {"container"}, "event": {event}})
	resp, err := apiGet(ctx, c, "/events", url.Values{"filters": {string(f)}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	for {
		var ev struct {
			Actor struct{ Attributes map[string]string }
		}
		if err := dec.Decode(&ev); err != nil {
			return err
		}
		attrs := ev.Actor.Attributes
		app, worker := attrs["skiff.app"], false
		if app == "" {
			app, worker = attrs["skiff.worker.app"], true
		}
		if app != "" {
			fn(app, attrs["name"], worker, attrs)
		}
	}
}

const maxLogLine = 1 << 20

// demuxLogs splits Docker's framed stream: each frame is an 8-byte header (stream type, then a big-endian length at offset 4) and its payload. One line can span frames, and a frame can hold several lines.
func demuxLogs(r io.Reader, line func(ts time.Time, stream, text string)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var header [8]byte
	var pending [3][]byte
	names := [3]string{"out", "out", "err"}
	for {
		if _, err := io.ReadFull(br, header[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		kind := int(header[0])
		if kind > 2 {
			kind = 1
		}
		n := binary.BigEndian.Uint32(header[4:])
		payload := make([]byte, n)
		if _, err := io.ReadFull(br, payload); err != nil {
			return nil
		}
		buf := append(pending[kind], payload...)
		for {
			i := strings.IndexByte(string(buf), '\n')
			if i < 0 {
				break
			}
			emitLogLine(string(buf[:i]), names[kind], line)
			buf = buf[i+1:]
		}
		if len(buf) > maxLogLine {
			emitLogLine(string(buf[:maxLogLine]), names[kind], line)
			buf = nil
		}
		pending[kind] = append([]byte(nil), buf...)
	}
}

func emitLogLine(raw, stream string, line func(ts time.Time, stream, text string)) {
	raw = strings.TrimSuffix(raw, "\r")
	stamp, text, _ := strings.Cut(raw, " ")
	ts, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		ts, text = time.Now(), raw
	}
	line(ts, stream, text)
}
