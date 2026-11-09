package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuelScript/skiff/internal/config"
)

// The API refuses to delete a volume another app borrows, as the dashboard does.
func TestAPIKeepsABorrowedVolume(t *testing.T) {
	openTestDB(t)
	putSource(Source{App: "shop", Team: "t1", Port: "3000"})
	_ = addVolume("shop", config.Volume{Name: "files", Path: "/data"})
	_ = addVolume("worker", config.Volume{Name: "files", Path: "/data", Owner: "shop"})
	putSource(Source{App: "worker", Team: "t1", Port: "3000"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/shop/volumes/files", nil)
	req.SetPathValue("name", "shop")
	req.SetPathValue("volume", "files")
	req = req.WithContext(context.WithValue(req.Context(), ctxTeam, "t1"))
	w := httptest.NewRecorder()
	(&Panel{}).apiRemoveVolume(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("deleting a borrowed volume: %d %s", w.Code, w.Body)
	}
	if len(listVolumes("shop")) != 1 {
		t.Fatal("the volume was deleted")
	}
}

// A borrower deploying before the holder makes the volume on the holder's disk.
func TestBorrowedVolumeIsOnItsHoldersDisk(t *testing.T) {
	openTestDB(t)
	_, _ = sqlDB.Exec(`INSERT INTO disks(id, name, path, created) VALUES('d1', 'data-1', '/mnt/data', 1)`)
	if err := addVolumeOn("shop", config.Volume{Name: "files", Path: "/data"}, "d1"); err != nil {
		t.Fatal(err)
	}
	_ = addVolume("worker", config.Volume{Name: "files", Path: "/data", Owner: "shop"})
	if v := listVolumes("worker"); len(v) != 1 || v[0].Disk != "/mnt/data" {
		t.Fatalf("borrowed volume: %+v", v)
	}
}
