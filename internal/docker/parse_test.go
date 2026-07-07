package docker

import "testing"

func TestParsePercent(t *testing.T) {
	cases := map[string]float64{
		"0.00%": 0, "12.34%": 12.34, "100.09%": 100.09, "  5.5% ": 5.5, "--": 0, "": 0,
	}
	for in, want := range cases {
		if got := parsePercent(in); got != want {
			t.Errorf("parsePercent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	unit := func(v float64, shift uint) int64 { return int64(v * float64(int64(1)<<shift)) }
	cases := []struct {
		in   string
		want int64
	}{
		{"0B", 0},
		{"512B", 512},
		{"15.87MiB", unit(15.87, 20)},
		{"7.63GiB", unit(7.63, 30)},
		{"1.5KiB", unit(1.5, 10)},
		{"2GB", 2 << 30},
		{"", 0},
	}
	for _, c := range cases {
		if got := parseSize(c.in); got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	// Trailing space (from the "used / limit" split) must be trimmed.
	if got := parseSize("15.87MiB "); got != unit(15.87, 20) {
		t.Errorf("trailing space not trimmed: %d", got)
	}
}
