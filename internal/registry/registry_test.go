package registry

import (
	"fmt"
	"sync"
	"testing"
)

// TestUpdateConcurrent guards the registry lock: an unlocked read-modify-write
// would lose most of these concurrent writes.
func TestUpdateConcurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const n = 64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("app-%03d", i)
			if err := Put(App{Name: name, Container: name + "-c", Port: 3000}); err != nil {
				t.Errorf("Put: %v", err)
			}
		}(i)
	}
	wg.Wait()

	apps, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(apps) != n {
		t.Fatalf("lost concurrent updates: %d/%d survived", len(apps), n)
	}
}

func TestDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = Put(App{Name: "a", Container: "a-c"})
	if existed, _ := Delete("a"); !existed {
		t.Fatal("Delete reported a didn't exist")
	}
	if existed, _ := Delete("a"); existed {
		t.Fatal("Delete reported a still existed after removal")
	}
	apps, _ := Load()
	if _, ok := apps["a"]; ok {
		t.Fatal("app survived delete")
	}
}
