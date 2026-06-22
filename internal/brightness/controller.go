package brightness

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/legion/display/internal/ddcutil"
)

const debounceDelay = 200 * time.Millisecond

// Startup retry delays when monitors may not be ready yet after boot.
var startupDetectDelays = []time.Duration{
	0,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
}

// ChangeHandler is called after brightness is applied to all displays.
type ChangeHandler func(percent int)

// Controller manages display discovery and debounced brightness changes.
type Controller struct {
	client   *ddcutil.Client
	mu       sync.Mutex
	displays []ddcutil.Display
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

// RefreshDisplays re-detects DDC/CI displays and warms the max-brightness cache.
func (c *Controller) RefreshDisplays(ctx context.Context) ([]int, error) {
	displays, err := c.client.DetectDisplays(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.displays = append([]ddcutil.Display(nil), displays...)
	c.maxCache = make(map[int]int)
	c.mu.Unlock()

	numbers := displayNumbers(displays)
	if len(numbers) > 0 {
		if c.verbose {
			log.Printf("detected displays: %v", displays)
		}
		c.WarmMaxCache(ctx)
	}
	return numbers, nil
}

// DiscoverAtStartup retries display detection until monitors are available.
func (c *Controller) DiscoverAtStartup(ctx context.Context) {
	for i, delay := range startupDetectDelays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}

		numbers, err := c.RefreshDisplays(ctx)
		if err != nil {
			log.Printf("display detect attempt %d/%d: %v", i+1, len(startupDetectDelays), err)
			continue
		}
		if len(numbers) > 0 {
			log.Printf("detected %d display(s): %v", len(numbers), numbers)
			return
		}
		log.Printf("display detect attempt %d/%d: no displays", i+1, len(startupDetectDelays))
	}
	log.Printf("no displays after startup retries; will retry on next request")
}

// WarmMaxCache reads max brightness for all displays in parallel.
func (c *Controller) WarmMaxCache(ctx context.Context) {
	displays := c.getDisplays()
	var wg sync.WaitGroup

	for _, display := range displays {
		wg.Go(func() {
			b, err := c.client.GetBrightness(ctx, display.Bus)
			if err != nil {
				log.Printf("warm max cache display %d (bus %d): %v", display.Number, display.Bus, err)
				return
			}
			c.mu.Lock()
			c.maxCache[display.Bus] = b.Max
			c.mu.Unlock()
		})
	}
	wg.Wait()
}

// GetDisplays returns cached display numbers.
func (c *Controller) GetDisplays() []int {
	return displayNumbers(c.getDisplays())
}

func (c *Controller) getDisplays() []ddcutil.Display {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ddcutil.Display(nil), c.displays...)
}

// GetBrightness returns the average brightness percent across displays.
func (c *Controller) GetBrightness(ctx context.Context) (int, error) {
	displays := c.ensureDisplays(ctx)
	if len(displays) == 0 {
		return 0, fmt.Errorf("no displays detected")
	}

	var wg sync.WaitGroup
	var sumMu sync.Mutex
	var sum, ok int

	for _, display := range displays {
		wg.Go(func() {
			b, err := c.client.GetBrightness(ctx, display.Bus)
			if err != nil {
				log.Printf("get brightness display %d (bus %d): %v", display.Number, display.Bus, err)
				return
			}
			sumMu.Lock()
			sum += b.Percent()
			ok++
			sumMu.Unlock()
		})
	}
	wg.Wait()

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
	onChange := c.onChange
	c.mu.Unlock()

	displays := c.ensureDisplays(context.Background())
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

	displays := c.ensureDisplays(ctx)
	if len(displays) == 0 {
		return fmt.Errorf("no displays detected")
	}

	var wg sync.WaitGroup
	var failed atomic.Int32

	for _, display := range displays {
		wg.Go(func() {
			if err := c.setDisplayPercent(ctx, display, percent); err != nil {
				log.Printf("set brightness display %d (bus %d) to %d%%: %v", display.Number, display.Bus, percent, err)
				failed.Add(1)
			}
		})
	}
	wg.Wait()

	if int(failed.Load()) == len(displays) {
		return fmt.Errorf("failed to set brightness on all displays")
	}
	return nil
}

func (c *Controller) setDisplayPercent(ctx context.Context, display ddcutil.Display, percent int) error {
	max := c.cachedMax(display.Bus)
	if max <= 0 {
		b, err := c.client.GetBrightness(ctx, display.Bus)
		if err != nil {
			return err
		}
		max = b.Max
		c.mu.Lock()
		c.maxCache[display.Bus] = max
		c.mu.Unlock()
	}
	return c.client.SetBrightnessAbsolute(ctx, display.Bus, percent, max)
}

func (c *Controller) cachedMax(bus int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxCache[bus]
}

func displayNumbers(displays []ddcutil.Display) []int {
	numbers := make([]int, len(displays))
	for i, d := range displays {
		numbers[i] = d.Number
	}
	return numbers
}

func (c *Controller) ensureDisplays(ctx context.Context) []ddcutil.Display {
	displays := c.getDisplays()
	if len(displays) > 0 {
		return displays
	}

	numbers, err := c.RefreshDisplays(ctx)
	if err != nil {
		log.Printf("on-demand display detect: %v", err)
		return nil
	}
	if len(numbers) == 0 {
		return nil
	}
	log.Printf("detected %d display(s) on demand: %v", len(numbers), numbers)
	return c.getDisplays()
}
