package ambient

import "testing"

func TestParseLuxLine(t *testing.T) {
	tests := []struct {
		in  string
		lux int
		ok  bool
	}{
		{"LUX 42", 42, true},
		{"  LUX 100  ", 100, true},
		{"ERR sensor", 0, false},
		{"LUX abc", 0, false},
	}

	for _, tc := range tests {
		lux, ok := parseLuxLine(tc.in)
		if ok != tc.ok || (tc.ok && lux != tc.lux) {
			t.Fatalf("parseLuxLine(%q) = (%d, %v), want (%d, %v)", tc.in, lux, ok, tc.lux, tc.ok)
		}
	}
}
