package panel

import (
	"testing"

	"github.com/nuelScript/skiff/internal/config"
)

func TestDiskAlertsOncePerLevel(t *testing.T) {
	for _, c := range []struct {
		pct, alerted, level int
		raise               bool
	}{
		{70, 0, 0, false},
		{81, 0, 80, true},
		{84, 80, 80, false},
		{91, 80, 90, true},
		{95, 90, 90, false},
		{78, 90, 90, false}, // between the clear mark and the warning: stays quiet
		{74, 90, 0, false},
		{92, 0, 90, true},
	} {
		level, raise := diskAlert(c.pct, c.alerted)
		if level != c.level || raise != c.raise {
			t.Errorf("%d%% after alerting at %d: level %d raise %v, want %d %v", c.pct, c.alerted, level, raise, c.level, c.raise)
		}
	}
}

func TestVolumeBudgetsAreStored(t *testing.T) {
	openTestDB(t)
	if err := addVolume("shop", config.Volume{Name: "data", Path: "/data"}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`UPDATE volumes SET budget=? WHERE app=? AND name=?`, 10<<30, "shop", "data"); err != nil {
		t.Fatal(err)
	}
	if got := volumeBudgets("shop"); got["data"] != 10<<30 {
		t.Errorf("budgets = %v", got)
	}
}
