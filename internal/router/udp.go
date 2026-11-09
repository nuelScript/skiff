package router

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	// udpIdle is how long a client may send nothing before its session ends; the next packet starts a new one, on whichever version is current.
	udpIdle           = 2 * time.Minute
	udpMaxSessions    = 10000
	udpMaxPerIP       = 200
	udpMaxPacket      = 64 << 10
	udpRoutesSuffix   = "/udp"
	udpSessionReapGap = 10 * time.Second
)

// udpProxy forwards public UDP ports to apps' containers. Each client gets a session pinned to one container, so a deploy moves new clients to the new version while existing ones finish where they are.
type udpProxy struct {
	rt     *Router
	mu     sync.Mutex
	active map[int]*udpListener
	addr   string
}

type udpListener struct {
	app      string
	pc       net.PacketConn
	mu       sync.Mutex
	sessions map[string]*udpSession
	perIP    map[string]int
	closed   bool
}

type udpSession struct {
	up     *net.UDPConn
	last   time.Time
	done   func()
	ipKey  string
	client net.Addr
}

func (rt *Router) startUDP() {
	if rt.EdgeFile == "" {
		return
	}
	rt.udpOnce.Do(func() {
		rt.udp = &udpProxy{rt: rt, active: map[int]*udpListener{}}
		go func() {
			for {
				rt.udp.sync()
				time.Sleep(tcpSyncEvery)
			}
		}()
	})
}

// sync opens and closes listeners to match the rules.
func (u *udpProxy) sync() {
	want := map[int]string{}
	for app, rule := range u.rt.edgeRules() {
		if rule.UDPPort > 0 {
			want[rule.UDPPort] = app
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for port, l := range u.active {
		if want[port] != l.app {
			l.close()
			delete(u.active, port)
		}
	}
	for port, app := range want {
		if _, open := u.active[port]; open {
			continue
		}
		pc, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", u.addr, port))
		if err != nil {
			log.Printf("skiff: can't open UDP port %d for %s: %v", port, app, err)
			continue
		}
		l := &udpListener{app: app, pc: pc, sessions: map[string]*udpSession{}, perIP: map[string]int{}}
		u.active[port] = l
		go u.serve(l)
		go l.reap()
	}
}

func (l *udpListener) close() {
	l.mu.Lock()
	l.closed = true
	for key, s := range l.sessions {
		l.end(key, s)
	}
	l.mu.Unlock()
	_ = l.pc.Close()
}

// end closes a session; the caller holds l.mu.
func (l *udpListener) end(key string, s *udpSession) {
	_ = s.up.Close()
	s.done()
	delete(l.sessions, key)
	if l.perIP[s.ipKey]--; l.perIP[s.ipKey] <= 0 {
		delete(l.perIP, s.ipKey)
	}
}

// reap ends sessions that have gone quiet.
func (l *udpListener) reap() {
	for {
		time.Sleep(udpSessionReapGap)
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			return
		}
		for key, s := range l.sessions {
			if time.Since(s.last) > udpIdle {
				l.end(key, s)
			}
		}
		l.mu.Unlock()
	}
}

func (u *udpProxy) serve(l *udpListener) {
	buf := make([]byte, udpMaxPacket)
	for {
		n, client, err := l.pc.ReadFrom(buf)
		if err != nil {
			return
		}
		s := u.session(l, client)
		if s == nil {
			continue
		}
		_, _ = s.up.Write(buf[:n])
	}
}

// session finds the client's session or starts one on the next container in turn.
func (u *udpProxy) session(l *udpListener, client net.Addr) *udpSession {
	key := client.String()
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := l.sessions[key]; ok {
		s.last = time.Now()
		return s
	}
	host, _, _ := net.SplitHostPort(key)
	ipKey := limitKeyFor(host)
	if len(l.sessions) >= udpMaxSessions || l.perIP[ipKey] >= udpMaxPerIP {
		return nil
	}
	ports := u.rt.portsFor(l.app + udpRoutesSuffix)
	if len(ports) == 0 {
		return nil
	}
	port := ports[u.rt.nextReplica(l.app+udpRoutesSuffix, len(ports))]
	up, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return nil
	}
	s := &udpSession{up: up, last: time.Now(), done: u.rt.trackOpen(port), ipKey: ipKey, client: client}
	l.sessions[key] = s
	l.perIP[ipKey]++
	go u.replies(l, key, s)
	return s
}

// replies sends what the container answers back to the client, until the session ends.
func (u *udpProxy) replies(l *udpListener, key string, s *udpSession) {
	buf := make([]byte, udpMaxPacket)
	for {
		n, err := s.up.Read(buf)
		if err != nil {
			// A closed port on the container side (it restarted, or a deploy removed it) ends the session, so the next packet finds a live one.
			l.mu.Lock()
			if cur, ok := l.sessions[key]; ok && cur == s {
				l.end(key, s)
			}
			l.mu.Unlock()
			return
		}
		l.mu.Lock()
		s.last = time.Now()
		l.mu.Unlock()
		_, _ = l.pc.WriteTo(buf[:n], s.client)
	}
}
