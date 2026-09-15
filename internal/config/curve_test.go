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
