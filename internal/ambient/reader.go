package ambient

import (
	"fmt"
	"sync"
	"time"

	"go.bug.st/serial"
)

// Reader keeps a serial port open for repeated lux polls without USB reset.
type Reader struct {
	mu      sync.Mutex
	port    serial.Port
	portName string
	baud    int
	timeout time.Duration
}

// NewReader creates a closed reader. Call Open before Read.
func NewReader(portName string, baud int, timeout time.Duration) *Reader {
	if baud <= 0 {
		baud = defaultBaud
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Reader{portName: portName, baud: baud, timeout: timeout}
}

// Open opens the port, optionally pulses DTR for a clean READY, then waits for READY.
func (r *Reader) Open(reset bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.port != nil {
		_ = r.port.Close()
		r.port = nil
	}

	port, err := serial.Open(r.portName, &serial.Mode{BaudRate: r.baud})
	if err != nil {
		return fmt.Errorf("open %s: %w", r.portName, err)
	}
	if err := port.SetReadTimeout(200 * time.Millisecond); err != nil {
		_ = port.Close()
		return fmt.Errorf("set read timeout: %w", err)
	}

	if reset {
		_ = port.SetDTR(false)
		time.Sleep(50 * time.Millisecond)
		_ = port.SetDTR(true)
		time.Sleep(50 * time.Millisecond)
	}
	_ = port.ResetInputBuffer()

	if err := waitForReady(port, r.timeout); err != nil {
		_ = port.Close()
		return err
	}
	_ = port.ResetInputBuffer()
	r.port = port
	return nil
}

// Close releases the serial port.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.port == nil {
		return nil
	}
	err := r.port.Close()
	r.port = nil
	return err
}

// Read sends R and returns lux. Does not reset the Nano.
func (r *Reader) Read() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.port == nil {
		return 0, fmt.Errorf("ambient reader not open")
	}
	_ = r.port.ResetInputBuffer()
	if _, err := r.port.Write([]byte("R")); err != nil {
		return 0, fmt.Errorf("write request: %w", err)
	}
	return readLuxReply(r.port, r.timeout)
}

// EnsureOpen opens with reset if not already open.
func (r *Reader) EnsureOpen() error {
	r.mu.Lock()
	open := r.port != nil
	r.mu.Unlock()
	if open {
		return nil
	}
	return r.Open(true)
}

// Reopen closes and opens with reset (after errors).
func (r *Reader) Reopen() error {
	_ = r.Close()
	return r.Open(true)
}
