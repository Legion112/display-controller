package autobrightness

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/legion/display/internal/ambient"
	"github.com/legion/display/internal/brightness"
	"github.com/legion/display/internal/config"
)

// LuxHandler is called when a new lux sample is available.
type LuxHandler func(lux uint32)

// AutoHandler is called when auto mode changes.
type AutoHandler func(enabled bool)

// Manager runs the lux → curve → brightness loop.
type Manager struct {
	ctrl *brightness.Controller

	mu         sync.Mutex
	cfg        config.Config
	reader     *ambient.Reader
	lastLux    uint32
	lastBright int
	stopCh     chan struct{}
	wg         sync.WaitGroup
	running    bool

	onLux  LuxHandler
	onAuto AutoHandler
}

// NewManager prepares a manager from loaded config (does not start).
func NewManager(ctrl *brightness.Controller, cfg config.Config) *Manager {
	return &Manager{
		ctrl:       ctrl,
		cfg:        cfg,
		lastBright: -1,
	}
}

// SetLuxHandler registers lux update callbacks.
func (m *Manager) SetLuxHandler(fn LuxHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onLux = fn
}

// SetAutoHandler registers auto-mode change callbacks.
func (m *Manager) SetAutoHandler(fn AutoHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAuto = fn
}

func (m *Manager) copyConfigLocked() config.Config {
	cfg := m.cfg
	cfg.Curve = append([]config.Point(nil), m.cfg.Curve...)
	return cfg
}

// AutoEnabled reports whether auto-brightness is on.
func (m *Manager) AutoEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Auto
}

// LastLux returns the last successful lux sample (0 if none).
func (m *Manager) LastLux() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastLux
}

// Curve returns a copy of curve points.
func (m *Manager) Curve() []config.Point {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]config.Point(nil), m.cfg.Curve...)
}

// SetAuto enables/disables auto mode, persists, and starts/stops the loop.
func (m *Manager) SetAuto(enabled bool) error {
	m.mu.Lock()
	if m.cfg.Auto == enabled {
		m.mu.Unlock()
		return nil
	}
	m.cfg.Auto = enabled
	cfg := m.copyConfigLocked()
	onAuto := m.onAuto
	m.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return err
	}
	if onAuto != nil {
		onAuto(enabled)
	}
	if enabled {
		m.Start()
	} else {
		m.Stop()
	}
	return nil
}

// SetCurve validates, stores, persists; if auto on, applies from last lux.
func (m *Manager) SetCurve(points []config.Point) error {
	points = config.NormalizeCurve(points)
	if err := config.ValidateCurve(points); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg.Curve = points
	m.lastBright = -1
	cfg := m.copyConfigLocked()
	lux := m.lastLux
	auto := m.cfg.Auto
	m.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return err
	}
	if auto {
		m.applyLux(lux)
	}
	return nil
}

// Start begins the poll loop if auto is enabled (idempotent).
func (m *Manager) Start() {
	m.mu.Lock()
	if !m.cfg.Auto || m.running {
		m.mu.Unlock()
		return
	}
	m.stopCh = make(chan struct{})
	m.running = true
	m.wg.Add(1)
	stop := m.stopCh
	m.mu.Unlock()
	go m.loop(stop)
}

// Stop ends the poll loop and closes the reader.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	close(m.stopCh)
	m.running = false
	m.mu.Unlock()
	m.wg.Wait()

	m.mu.Lock()
	if m.reader != nil {
		_ = m.reader.Close()
		m.reader = nil
	}
	m.mu.Unlock()
}

func (m *Manager) loop(stop <-chan struct{}) {
	defer m.wg.Done()
	backoff := time.Second

	for {
		m.mu.Lock()
		poll := time.Duration(m.cfg.PollMS) * time.Millisecond
		m.mu.Unlock()
		if poll <= 0 {
			poll = time.Duration(config.DefaultPollMS) * time.Millisecond
		}

		if err := m.ensureReader(); err != nil {
			log.Printf("auto-brightness: sensor: %v", err)
			select {
			case <-stop:
				return
			case <-time.After(backoff):
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
		}
		backoff = time.Second

		lux, err := m.readLux()
		if err != nil {
			log.Printf("auto-brightness: read lux: %v", err)
			m.dropReader()
			select {
			case <-stop:
				return
			case <-time.After(backoff):
				continue
			}
		}

		m.mu.Lock()
		m.lastLux = uint32(lux)
		onLux := m.onLux
		m.mu.Unlock()
		if onLux != nil {
			onLux(uint32(lux))
		}
		m.applyLux(uint32(lux))

		select {
		case <-stop:
			return
		case <-time.After(poll):
		}
	}
}

func (m *Manager) ensureReader() error {
	m.mu.Lock()
	if m.reader != nil {
		m.mu.Unlock()
		return nil
	}
	port := m.cfg.Port
	m.mu.Unlock()

	if port == "" {
		var err error
		port, err = ambient.DetectPort()
		if err != nil {
			return err
		}
	}
	r := ambient.NewReader(port, 0, 2*time.Second)
	if err := r.Open(true); err != nil {
		return err
	}

	m.mu.Lock()
	if m.reader != nil {
		// Another path opened first.
		m.mu.Unlock()
		_ = r.Close()
		return nil
	}
	m.reader = r
	m.mu.Unlock()
	log.Printf("auto-brightness: opened ambient sensor on %s", port)
	return nil
}

func (m *Manager) dropReader() {
	m.mu.Lock()
	r := m.reader
	m.reader = nil
	m.mu.Unlock()
	if r != nil {
		_ = r.Close()
	}
}

func (m *Manager) readLux() (int, error) {
	m.mu.Lock()
	r := m.reader
	m.mu.Unlock()
	if r == nil {
		return 0, fmt.Errorf("reader not open")
	}
	return r.Read()
}

func (m *Manager) applyLux(lux uint32) {
	m.mu.Lock()
	if !m.cfg.Auto {
		m.mu.Unlock()
		return
	}
	curve := append([]config.Point(nil), m.cfg.Curve...)
	hyst := m.cfg.Hysteresis
	last := m.lastBright
	m.mu.Unlock()

	target := config.BrightnessForLux(curve, lux)
	if last >= 0 {
		delta := target - last
		if delta < 0 {
			delta = -delta
		}
		if delta < hyst {
			return
		}
	}

	m.ctrl.SetBrightness(target)
	m.mu.Lock()
	m.lastBright = target
	m.mu.Unlock()
}
