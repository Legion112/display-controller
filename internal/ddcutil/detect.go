package ddcutil

import (
	"context"
	"regexp"
	"strconv"
)

// Display identifies a monitor by ddcutil display number and I2C bus.
type Display struct {
	Number int
	Bus    int
}

var displayBlockRE = regexp.MustCompile(`(?ms)^Display\s+(\d+)\s*\n\s*I2C bus:\s+/dev/i2c-(\d+)`)

func parseDetectOutput(out string) []Display {
	matches := displayBlockRE.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return nil
	}

	displays := make([]Display, 0, len(matches))
	for _, m := range matches {
		number, err1 := strconv.Atoi(m[1])
		bus, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		displays = append(displays, Display{Number: number, Bus: bus})
	}
	return displays
}

// DetectDisplays returns monitors from `ddcutil detect --brief` with I2C bus numbers.
func (c *Client) DetectDisplays(ctx context.Context) ([]Display, error) {
	out, err := c.run(ctx, "detect", "--brief")
	if err != nil {
		return nil, err
	}

	displays := parseDetectOutput(out)
	if len(displays) == 0 {
		return nil, nil
	}
	return displays, nil
}
