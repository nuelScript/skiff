package panel

import (
	"fmt"
	"testing"
)

func TestRecentDeliveries(t *testing.T) {
	d := newRecentDeliveries(3)
	if !d.firstSeen("a") || d.firstSeen("a") {
		t.Fatal("a repeated id must be reported as seen")
	}
	if !d.firstSeen("") || !d.firstSeen("") {
		t.Fatal("a request without an id is never treated as a repeat")
	}
	d.firstSeen("b")
	d.firstSeen("c")
	d.firstSeen("d") // pushes "a" out
	if !d.firstSeen("a") {
		t.Fatal("the oldest id should have been forgotten")
	}
	for i := 0; i < 100; i++ {
		d.firstSeen(fmt.Sprint("x", i))
	}
	if len(d.seen) > 3 {
		t.Fatalf("set grew to %d", len(d.seen))
	}
}
