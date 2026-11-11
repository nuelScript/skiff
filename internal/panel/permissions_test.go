package panel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefuseBlockDevices(t *testing.T) {
	if err := refuseBlockDevices([]string{"/dev/null", "/dev/net/tun", "/dev/nope"}); err != nil {
		t.Errorf("a char or missing device was refused: %v", err)
	}
	ents, _ := filepath.Glob("/dev/*")
	for _, e := range ents {
		if fi, err := os.Stat(e); err == nil && fi.Mode()&os.ModeDevice != 0 && fi.Mode()&os.ModeCharDevice == 0 {
			if refuseBlockDevices([]string{filepath.Dir(e) + "//./" + filepath.Base(e) + ":/dev/x"}) == nil {
				t.Errorf("block device %s was allowed", e)
			}
			return
		}
	}
	t.Log("no block device on this host to try")
}
