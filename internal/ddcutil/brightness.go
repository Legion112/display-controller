package ddcutil

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// Brightness holds current and max VCP brightness for one display.
type Brightness struct {
	Current int
	Max     int
}

var briefBrightnessRE = regexp.MustCompile(`(?m)^VCP\s+10\s+C\s+(\d+)\s+(\d+)\s*$`)
var verboseBrightnessRE = regexp.MustCompile(`current value\s*=\s*(\d+),\s*max value\s*=\s*(\d+)`)

func parseBrightness(output string) (Brightness, error) {
	if m := briefBrightnessRE.FindStringSubmatch(output); len(m) == 3 {
		current, err1 := strconv.Atoi(m[1])
		max, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil || max <= 0 {
			return Brightness{}, fmt.Errorf("parse brightness from %q", output)
		}
		return Brightness{Current: current, Max: max}, nil
	}
	if m := verboseBrightnessRE.FindStringSubmatch(output); len(m) == 3 {
		current, err1 := strconv.Atoi(m[1])
		max, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil || max <= 0 {
			return Brightness{}, fmt.Errorf("parse brightness from %q", output)
		}
		return Brightness{Current: current, Max: max}, nil
	}
	return Brightness{}, fmt.Errorf("parse brightness from %q", output)
}

// GetBrightness reads VCP 0x10 for a monitor on the given I2C bus.
func (c *Client) GetBrightness(ctx context.Context, bus int) (Brightness, error) {
	out, err := c.run(ctx,
		"--bus", strconv.Itoa(bus),
		"getvcp", "10",
		"--brief",
	)
	if err != nil {
		return Brightness{}, err
	}
	return parseBrightness(out)
}

// SetBrightnessAbsolute sets VCP 0x10 using a known max value on the given I2C bus.
func (c *Client) SetBrightnessAbsolute(ctx context.Context, bus int, percent int, max int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if max <= 0 {
		return fmt.Errorf("invalid max brightness %d for bus %d", max, bus)
	}

	value := (percent*max + 50) / 100
	_, err := c.run(ctx,
		"--bus", strconv.Itoa(bus),
		"setvcp", "10", strconv.Itoa(value),
		"--noverify",
	)
	return err
}

// SetBrightnessPercent sets VCP 0x10 using a 0-100 percentage of the display max.
func (c *Client) SetBrightnessPercent(ctx context.Context, bus int, percent int) error {
	b, err := c.GetBrightness(ctx, bus)
	if err != nil {
		return err
	}
	return c.SetBrightnessAbsolute(ctx, bus, percent, b.Max)
}

// Percent returns brightness as 0-100 based on current/max.
func (b Brightness) Percent() int {
	if b.Max <= 0 {
		return 0
	}
	return (b.Current*100 + b.Max/2) / b.Max
}
