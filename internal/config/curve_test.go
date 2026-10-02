package config

import "testing"

func TestBrightnessForLux(t *testing.T) {
	curve := []Point{
		{Lux: 0, Brightness: 10},
		{Lux: 100, Brightness: 50},
		{Lux: 200, Brightness: 90},
	}
	cases := []struct {
		lux  uint32
		want int
	}{
		{0, 10},
		{50, 30},
		{100, 50},
		{150, 70},
		{200, 90},
		{500, 90},
	}
	for _, tc := range cases {
		got := BrightnessForLux(curve, tc.lux)
		if got != tc.want {
			t.Fatalf("lux=%d: got %d want %d", tc.lux, got, tc.want)
		}
	}
}

func TestValidateCurve(t *testing.T) {
	if err := ValidateCurve([]Point{{0, 10}}); err == nil {
		t.Fatal("expected error for single point")
	}
	if err := ValidateCurve([]Point{{0, 10}, {0, 20}}); err == nil {
		t.Fatal("expected error for non-increasing lux")
	}
	if err := ValidateCurve([]Point{{0, 10}, {50, 101}}); err == nil {
		t.Fatal("expected error for brightness > 100")
	}
	if err := ValidateCurve(DefaultCurve()); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeCurveSortsAndClamps(t *testing.T) {
	got := NormalizeCurve([]Point{
		{Lux: 200, Brightness: 150},
		{Lux: 0, Brightness: 10},
	})
	want := []Point{{Lux: 0, Brightness: 10}, {Lux: 200, Brightness: 100}}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// A graphical client derives lux from a pixel position, so two control points
// can round to the same lux. Normalizing must leave a curve ValidateCurve
// accepts rather than one that is rejected for non-increasing lux.
func TestNormalizeCurveCollapsesDuplicateLux(t *testing.T) {
	got := NormalizeCurve([]Point{
		{Lux: 0, Brightness: 5},
		{Lux: 4, Brightness: 20},
		{Lux: 4, Brightness: 30},
		{Lux: 4, Brightness: 40},
		{Lux: 50, Brightness: 60},
	})
	want := []Point{
		{Lux: 0, Brightness: 5},
		{Lux: 4, Brightness: 40}, // last of the run wins
		{Lux: 50, Brightness: 60},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if err := ValidateCurve(got); err != nil {
		t.Fatalf("normalized curve must validate: %v", err)
	}
}
