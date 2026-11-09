package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

// ContainerAddr is a running container's address on each network it's on.
type ContainerAddr struct {
	Name     string
	Labels   map[string]string
	Networks map[string]string // network name → IPv4 address
}

// ContainerAddrs lists every running container with its addresses, through the local Docker socket.
func (e *Engine) ContainerAddrs() ([]ContainerAddr, error) {
	c := e.api()
	if c == nil {
		return nil, errors.New("no docker socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := apiGet(ctx, c, "/containers/json", url.Values{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list []struct {
		Names           []string
		Labels          map[string]string
		NetworkSettings struct {
			Networks map[string]struct{ IPAddress string }
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	out := make([]ContainerAddr, 0, len(list))
	for _, c := range list {
		a := ContainerAddr{Labels: c.Labels, Networks: map[string]string{}}
		if len(c.Names) > 0 {
			a.Name = strings.TrimPrefix(c.Names[0], "/")
		}
		for name, n := range c.NetworkSettings.Networks {
			if n.IPAddress != "" {
				a.Networks[name] = n.IPAddress
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// NetworkSubnet is a network's IPv4 range, like 172.20.0.0/16.
func (e *Engine) NetworkSubnet(name string) string {
	out, err := e.command("network", "inspect", "-f", "{{range .IPAM.Config}}{{.Subnet}} {{end}}", name).Output()
	if err != nil {
		return ""
	}
	for _, s := range strings.Fields(string(out)) {
		if !strings.Contains(s, ":") {
			return s
		}
	}
	return ""
}

// WatchContainers calls fn whenever a container starts or stops, until ctx ends.
func (e *Engine) WatchContainers(ctx context.Context, fn func()) error {
	c := e.api()
	if c == nil {
		return errors.New("no docker socket")
	}
	f, _ := json.Marshal(map[string][]string{"type": {"container"}, "event": {"start", "die"}})
	resp, err := apiGet(ctx, c, "/events", url.Values{"filters": {string(f)}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	for {
		var ev struct{}
		if err := dec.Decode(&ev); err != nil {
			return err
		}
		fn()
	}
}
