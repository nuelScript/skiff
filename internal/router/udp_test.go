package router

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/edge"
)

func udpEcho(t *testing.T, greeting string) (int, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo([]byte(greeting+" "+string(buf[:n])), from)
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port, func() { pc.Close() }
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

// udpClient keeps one source port, as a game client or VPN peer does.
func udpClient(t *testing.T, port int) func(msg string) (string, error) {
	t.Helper()
	c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return func(msg string) (string, error) {
		if _, err := c.Write([]byte(msg)); err != nil {
			return "", err
		}
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 2048)
		n, err := c.Read(buf)
		return string(buf[:n]), err
	}
}

func TestUDPProxy(t *testing.T) {
	first, stopFirst := udpEcho(t, "first")
	second, stopSecond := udpEcho(t, "second")
	defer stopSecond()
	file := filepath.Join(t.TempDir(), "edge.json")
	public := freeUDPPort(t)
	if err := edge.Save(file, edge.Rules{"game": {UDPPort: public}}); err != nil {
		t.Fatal(err)
	}
	rt := &Router{EdgeFile: file, Engine: docker.Local(), routeTable: map[string][]int{"game" + udpRoutesSuffix: {first}}, routeAt: time.Now().Add(time.Hour)}
	proxy := &udpProxy{rt: rt, active: map[int]*udpListener{}, addr: "127.0.0.1"}
	defer func() {
		for _, l := range proxy.active {
			l.close()
		}
	}()
	proxy.sync()

	old := udpClient(t, public)
	if got, err := old("hello"); err != nil || got != "first hello" {
		t.Fatalf("forwarding: %q %v", got, err)
	}
	if rt.openOn(first) != 1 {
		t.Errorf("a session should hold its container open while draining: %d", rt.openOn(first))
	}

	// A deploy: new clients reach the new container, and a client already talking stays where it is.
	rt.fetchRoutesForTest(map[string][]int{"game" + udpRoutesSuffix: {second}})
	if got, err := udpClient(t, public)("hi"); err != nil || got != "second hi" {
		t.Fatalf("a new client after a deploy: %q %v", got, err)
	}
	if got, err := old("again"); err != nil || got != "first again" {
		t.Fatalf("an existing client should stay on its container: %q %v", got, err)
	}

	// Once the old container is gone, the client's next packets reach the new one.
	stopFirst()
	_, _ = old("lost")
	var got string
	for range 5 {
		if got, _ = old("back"); got == "second back" {
			break
		}
	}
	if got != "second back" {
		t.Fatalf("after the old container went: %q", got)
	}

	// Removing the rule closes the port.
	if err := edge.Save(file, edge.Rules{}); err != nil {
		t.Fatal(err)
	}
	rt.edgeMu.Lock()
	rt.cachedEdgeAt = time.Time{}
	rt.edgeMu.Unlock()
	proxy.sync()
	if got, err := udpClient(t, public)("anyone"); err == nil {
		t.Fatalf("the port should be closed, got %q", got)
	}
}
