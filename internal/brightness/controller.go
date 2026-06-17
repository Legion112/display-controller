package brightness

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/legion/display/internal/ddcutil"
)

const debounceDelay = 200 * time.Millisecond
const interDisplayDelay = 150 * time.Millisecond

// ChangeHandler is called after brightness is applied to all displays.
type ChangeHandler func(percent int)

// Controller manages display discovery and debounced brightness changes.
type Controller struct {
	client   *ddcutil.Client
	mu       sync.Mutex
	displays []int
	maxCache map[int]int
	pending  int
	timer    *time.Timer
	onChange ChangeHandler
	verbose  bool
}

// NewController creates a brightness controller.
func NewController(client *ddcutil.Client, verbose bool) *Controller {
	return &Controller{
		client:   client,
		maxCache: make(map[int]int),
		verbose:  verbose,
	}
}

// SetChangeHandler registers a callback for successful brightness updates.
func (c *Controller) SetChangeHandler(fn ChangeHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChange = fn
}

// RefreshDisplays re-detects DDC/CI displays.
func (c *Controller) RefreshDisplays(ctx context.Context) ([]int, error) {
	displays, err := c.client.DetectDisplays(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.displays = append([]int(nil), displays...)
	c.maxCache = make(map[int]int)
	c.mu.Unlock()

	if c.verbose {
		log.Printf("detected displays: %v", displays)
	}
	return displays, nil
}

// GetDisplays returns cached display numbers.
func (c *Controller) GetDisplays() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.displays...)
}

// GetBrightness returns the average brightness percent across displays.
func (c *Controller) GetBrightness(ctx context.Context) (int, error) {
	displays := c.GetDisplays()
	if len(displays) == 0 {
		return 0, fmt.Errorf("no displays detected")
	}

	var sum int
	var ok int
	for _, display := range displays {
		b, err := c.client.GetBrightness(ctx, display)
		if err != nil {
			log.Printf("get brightness display %d: %v", display, err)
			continue
		}
		sum += b.Percent()
		ok++
	}
	if ok == 0 {
		return 0, fmt.Errorf("failed to read brightness from all displays")
	}
	return (sum + ok/2) / ok, nil
}

// SetBrightness schedules a debounced brightness update for all displays.
func (c *Controller) SetBrightness(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pending = percent
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(debounceDelay, func() {
		c.applyPending()
	})
}

func (c *Controller) applyPending() {
	c.mu.Lock()
	percent := c.pending
	displays := append([]int(nil), c.displays...)
	onChange := c.onChange
	c.mu.Unlock()

	if len(displays) == 0 {
		log.Printf("set brightness %d: no displays", percent)
		return
	}

	if err := c.applyToDisplays(context.Background(), percent); err != nil {
		log.Printf("set brightness %d%%: %v", percent, err)
		return
	}

	if onChange != nil {
		onChange(percent)
	}
}

// ApplyNow sets brightness immediately without debouncing.
func (c *Controller) ApplyNow(ctx context.Context, percent int) error {
	return c.applyToDisplays(ctx, percent)
}

func (c *Controller) applyToDisplays(ctx context.Context, percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	displays := c.GetDisplays()
	if len(displays) == 0 {
		return fmt.Errorf("no displays detected")
	}

	var failed int
	for i, display := range displays {
		if i > 0 {
			time.Sleep(interDisplayDelay)
		}
		if err := c.setDisplayPercent(ctx, display, percent); err != nil {
			log.Printf("set brightness display %d to %d%%: %v", display, percent, err)
			failed++
		}
	}
	if failed == len(displays) {
		return fmt.Errorf("failed to set brightness on all displays")
	}
	return nil
}

func (c *Controller) setDisplayPercent(ctx context.Context, display int, percent int) error {
	max := c.cachedMax(display)
	if max <= 0 {
		b, err := c.client.GetBrightness(ctx, display)
		if err != nil {
			return err
		}
		max = b.Max
		c.mu.Lock()
		c.maxCache[display] = max
		c.mu.Unlock()
	}
	return c.client.SetBrightnessAbsolute(ctx, display, percent, max)
}

func (c *Controller) cachedMax(display int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxCache[display]
}
