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

func TestListSortedAndOverwrite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Put(App{Name: "zebra", Container: "z", Port: 1}); err != nil {
		t.Fatal(err)
	}
	if err := Put(App{Name: "apple", Container: "a", Port: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "apple" || got[1].Name != "zebra" {
		t.Fatalf("List not name-sorted: %+v", got)
	}

	if err := Put(App{Name: "apple", Container: "a", Port: 99}); err != nil {
		t.Fatal(err)
	}
	got, _ = List()
	if len(got) != 2 {
		t.Fatalf("overwrite duplicated the app: %+v", got)
	}
	for _, a := range got {
		if a.Name == "apple" && a.Port != 99 {
			t.Fatalf("apple not updated in place: port=%d", a.Port)
		}
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
