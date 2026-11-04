package panel

import "sync"

// recentDeliveries remembers the last webhook delivery ids, so a captured request that's sent again does nothing. Bounded: the oldest id is forgotten first.
type recentDeliveries struct {
	mu   sync.Mutex
	seen map[string]bool
	ring []string
	next int
}

func newRecentDeliveries(n int) *recentDeliveries {
	return &recentDeliveries{seen: make(map[string]bool, n), ring: make([]string, n)}
}

// firstSeen records id and reports whether it's new. An empty id is never a repeat.
func (d *recentDeliveries) firstSeen(id string) bool {
	if id == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[id] {
		return false
	}
	if old := d.ring[d.next]; old != "" {
		delete(d.seen, old)
	}
	d.ring[d.next] = id
	d.seen[id] = true
	d.next = (d.next + 1) % len(d.ring)
	return true
}

var hookDeliveries = newRecentDeliveries(2048)
