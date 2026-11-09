package panel

import "testing"

func TestImageFlavorsAreNotAdopted(t *testing.T) {
	cases := []struct {
		d       dbRow
		adopted bool
	}{
		{dbRow{Engine: "postgres"}, false},
		{dbRow{Engine: "postgres", Image: pgvectorImage}, false},
		{dbRow{Engine: "postgres", Image: timescaleImage}, false},
		{dbRow{Engine: "postgres", Image: "postgis/postgis:16-3.4"}, true},
		{dbRow{Engine: "mysql", Image: "mysql:8.4"}, true},
	}
	for _, c := range cases {
		if got := c.d.adopted(); got != c.adopted {
			t.Errorf("%s adopted = %v", c.d.Image, got)
		}
		if c.d.Engine == "postgres" && tunesPostgres(c.d) == c.adopted {
			t.Errorf("%s tuned = %v", c.d.Image, !c.adopted)
		}
	}
}
