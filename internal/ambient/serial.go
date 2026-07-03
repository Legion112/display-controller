package ambient

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.bug.st/serial"
)

const defaultBaud = 57600

var luxLineRE = regexp.MustCompile(`^LUX (\d+)$`)

// ReadLux opens the serial port, sends a read request, and parses the lux reply.
func ReadLux(portName string, baud int, timeout time.Duration) (int, error) {
	if baud <= 0 {
		baud = defaultBaud
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	port, err := serial.Open(portName, &serial.Mode{BaudRate: baud})
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", portName, err)
	}
	defer port.Close()

	if err := port.SetReadTimeout(200 * time.Millisecond); err != nil {
		return 0, fmt.Errorf("set read timeout: %w", err)
	}
	if err := port.ResetInputBuffer(); err != nil {
		return 0, fmt.Errorf("reset input buffer: %w", err)
	}

	if _, err := port.Write([]byte("R\n")); err != nil {
		return 0, fmt.Errorf("write request: %w", err)
	}

	deadline := time.Now().Add(timeout)
	buf := make([]byte, 128)
	var line strings.Builder

	for time.Now().Before(deadline) {
		n, err := port.Read(buf)
		if err != nil {
			if isSerialTimeout(err) {
				continue
			}
			return 0, fmt.Errorf("read serial: %w", err)
		}
		for _, b := range buf[:n] {
			if b == '\n' || b == '\r' {
				if lux, ok := parseLuxLine(line.String()); ok {
					return lux, nil
				}
				if strings.HasPrefix(strings.TrimSpace(line.String()), "ERR ") {
					return 0, fmt.Errorf("device error: %s", strings.TrimSpace(line.String()))
				}
				line.Reset()
				continue
			}
			line.WriteByte(b)
		}
	}

	if lux, ok := parseLuxLine(line.String()); ok {
		return lux, nil
	}

	return 0, fmt.Errorf("timeout waiting for LUX response from %s", portName)
}

func parseLuxLine(raw string) (int, bool) {
	line := strings.TrimSpace(raw)
	matches := luxLineRE.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}
	var lux int
	if _, err := fmt.Sscanf(matches[1], "%d", &lux); err != nil {
		return 0, false
	}
	return lux, true
}

func isSerialTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out")
}

// DetectPort returns a serial device path for the ambient sensor Arduino.
func DetectPort() (string, error) {
	if env := strings.TrimSpace(os.Getenv("LUX_PORT")); env != "" {
		return env, nil
	}

	candidates := []string{"/dev/ttyUSB0", "/dev/ttyACM0"}
	for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		candidates = append(candidates, matches...)
	}

	seen := make(map[string]struct{})
	var unique []string
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	sort.Strings(unique)

	for _, path := range unique {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no serial port found (set LUX_PORT or connect Arduino)")
}
