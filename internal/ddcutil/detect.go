package ddcutil

import (
	"context"
	"regexp"
	"strconv"
)

var displayLineRE = regexp.MustCompile(`(?m)^Display\s+(\d+)\s*$`)

// DetectDisplays returns ddcutil display numbers from `ddcutil detect --brief`.
func (c *Client) DetectDisplays(ctx context.Context) ([]int, error) {
	out, err := c.run(ctx, "detect", "--brief")
	if err != nil {
		return nil, err
	}

	matches := displayLineRE.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	displays := make([]int, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		displays = append(displays, n)
	}
	return displays, nil
}
