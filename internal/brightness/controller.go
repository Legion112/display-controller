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
// These are cumulative waits before each attempt (full window ~32s).
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
	client       *ddcutil.Client
	mu           sync.Mutex
	displays     []ddcutil.Display
	maxCache     map[int]int
	pending      int
	timer        *time.Timer
	applying     bool // single-flight: at most one apply loop running
	onChange     ChangeHandler
	verbose      bool
	needsRefresh bool      // set after hard apply failure; next ensureDisplays re-detects
	lastDetect   time.Time // last successful DetectDisplays that updated or confirmed the cache
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
// A successful detect with fewer displays than already cached is ignored so a
// transient partial scan cannot shrink the working set.
func (c *Controller) RefreshDisplays(ctx context.Context) ([]int, error) {
	displays, err := c.client.DetectDisplays(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if len(displays) < len(c.displays) {
		kept := displayNumbers(c.displays)
		c.needsRefresh = false
		c.lastDetect = time.Now()
		c.mu.Unlock()
		log.Printf("detect returned %d display(s), keeping cached %d: %v", len(displays), len(kept), kept)
		return kept, nil
	}
	grew := len(displays) > len(c.displays) && len(c.displays) > 0
	oldCache := c.maxCache
	c.displays = append([]ddcutil.Display(nil), displays...)
	c.maxCache = mergeMaxCache(oldCache, displays)
	c.needsRefresh = false
	c.lastDetect = time.Now()
	c.mu.Unlock()

	numbers := displayNumbers(displays)
	if grew {
		log.Printf("display set grew to %d: %v", len(numbers), numbers)
	}
	if len(numbers) > 0 {
		if c.verbose {
			log.Printf("detected displays: %v", displays)
		}
		c.WarmMaxCache(ctx)
	}
	return numbers, nil
}

// DiscoverAtStartup retries display detection across the full startup window.
// Early after boot, ddcutil may report only a subset of monitors; stopping on the
// first non-empty result leaves later monitors out of the cache permanently.
func (c *Controller) DiscoverAtStartup(ctx context.Context) {
	for i, delay := range startupDetectDelays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				numbers := c.GetDisplays()
				if len(numbers) > 0 {
					log.Printf("startup cancelled; keeping %d display(s): %v", len(numbers), numbers)
				}
				return
			case <-time.After(delay):
			}
		}

		numbers, err := c.RefreshDisplays(ctx)
		if err != nil {
			log.Printf("display detect attempt %d/%d: %v", i+1, len(startupDetectDelays), err)
			continue
		}
		if len(numbers) == 0 {
			log.Printf("display detect attempt %d/%d: no displays", i+1, len(startupDetectDelays))
			continue
		}
		log.Printf("display detect attempt %d/%d: %d display(s): %v", i+1, len(startupDetectDelays), len(numbers), numbers)
	}

	numbers := c.GetDisplays()
	if len(numbers) > 0 {
		log.Printf("detected %d display(s) after startup retries: %v", len(numbers), numbers)
		return
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
	if c.applying {
		c.mu.Unlock()
		return
	}
	c.applying = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.applying = false
		c.mu.Unlock()
	}()

	ctx := context.Background()
	for {
		c.mu.Lock()
		percent := c.pending
		onChange := c.onChange
		c.mu.Unlock()

		displays := c.ensureDisplays(ctx)
		if len(displays) == 0 {
			log.Printf("set brightness %d: no displays", percent)
			return
		}

		if err := c.applyToDisplays(ctx, percent); err != nil {
			log.Printf("set brightness %d%%: %v", percent, err)
			c.mu.Lock()
			stale := c.pending != percent
			c.mu.Unlock()
			if stale {
				continue
			}
			return
		}

		c.mu.Lock()
		stale := c.pending != percent
		c.mu.Unlock()
		if stale {
			continue
		}

		if onChange != nil {
			onChange(percent)
		}
		return
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

	failed := c.setAllDisplays(ctx, displays, percent)
	if failed == 0 {
		return nil
	}

	log.Printf("set brightness: %d/%d displays failed; retrying", failed, len(displays))
	failed = c.setAllDisplays(ctx, displays, percent)
	if failed == 0 {
		return nil
	}

	if failed == len(displays) {
		c.mu.Lock()
		c.needsRefresh = true
		c.mu.Unlock()

		displays = c.ensureDisplays(ctx)
		if len(displays) == 0 {
			return fmt.Errorf("failed to set brightness on all displays")
		}
		failed = c.setAllDisplays(ctx, displays, percent)
		if failed == len(displays) {
			return fmt.Errorf("failed to set brightness on all displays")
		}
	}

	if failed > 0 {
		c.mu.Lock()
		c.needsRefresh = true
		c.mu.Unlock()
		log.Printf("set brightness: %d/%d displays still failing after retry", failed, len(displays))
	}
	return nil
}

func (c *Controller) setAllDisplays(ctx context.Context, displays []ddcutil.Display, percent int) int {
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
	return int(failed.Load())
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

func mergeMaxCache(old map[int]int, displays []ddcutil.Display) map[int]int {
	merged := make(map[int]int, len(displays))
	for _, d := range displays {
		if v, ok := old[d.Bus]; ok {
			merged[d.Bus] = v
		}
	}
	return merged
}

func (c *Controller) ensureDisplays(ctx context.Context) []ddcutil.Display {
	c.mu.Lock()
	needRefresh := c.needsRefresh || len(c.displays) == 0
	c.mu.Unlock()

	if !needRefresh {
		return c.getDisplays()
	}

	numbers, err := c.RefreshDisplays(ctx)
	if err != nil {
		log.Printf("on-demand display detect: %v", err)
		return c.getDisplays()
	}
	if len(numbers) == 0 {
		return nil
	}
	log.Printf("detected %d display(s) on demand: %v", len(numbers), numbers)
	return c.getDisplays()
}
