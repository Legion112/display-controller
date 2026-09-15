package config

import (
	"fmt"
	"sort"
)

// Point is a lux → brightness control point on the auto-brightness curve.
type Point struct {
	Lux        uint32 `json:"lux"`
	Brightness uint32 `json:"brightness"` // 0–100
}

// DefaultCurve is a reasonable indoor starting curve.
func DefaultCurve() []Point {
	return []Point{
		{Lux: 0, Brightness: 10},
		{Lux: 50, Brightness: 40},
		{Lux: 200, Brightness: 70},
		{Lux: 1000, Brightness: 100},
	}
}

// ValidateCurve checks points are usable for interpolation.
func ValidateCurve(points []Point) error {
	if len(points) < 2 {
		return fmt.Errorf("curve needs at least 2 points")
	}
	for i, p := range points {
		if p.Brightness > 100 {
			return fmt.Errorf("point %d: brightness %d out of range", i, p.Brightness)
		}
		if i > 0 && p.Lux <= points[i-1].Lux {
			return fmt.Errorf("point %d: lux must be strictly increasing", i)
		}
	}
	return nil
}

// NormalizeCurve sorts by lux and clamps brightness.
func NormalizeCurve(points []Point) []Point {
	out := append([]Point(nil), points...)
	for i := range out {
		if out[i].Brightness > 100 {
			out[i].Brightness = 100
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lux < out[j].Lux })
	return out
}

// BrightnessForLux maps lux through a piecewise-linear curve (clamped at ends).
func BrightnessForLux(points []Point, lux uint32) int {
	if len(points) == 0 {
		points = DefaultCurve()
	}
	if lux <= points[0].Lux {
		return int(points[0].Brightness)
	}
	last := points[len(points)-1]
	if lux >= last.Lux {
		return int(last.Brightness)
	}
	for i := 0; i < len(points)-1; i++ {
		a, b := points[i], points[i+1]
		if lux > b.Lux {
			continue
		}
		if b.Lux == a.Lux {
			return int(a.Brightness)
		}
		span := float64(b.Lux - a.Lux)
		t := float64(lux-a.Lux) / span
		v := float64(a.Brightness) + t*float64(int(b.Brightness)-int(a.Brightness))
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return int(v + 0.5)
	}
	return int(last.Brightness)
}
