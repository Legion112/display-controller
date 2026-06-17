package ddcutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const defaultTimeout = 5 * time.Second

var i2c0NoiseRE = regexp.MustCompile(`(?m)^Device /dev/i2c-0 is not readable and writable\.[^\n]*\n(?:Devices possibly used for DDC/CI communication cannot be opened: /dev/i2c-0\n)?(?:See https://www\.ddcutil\.com/i2c_permissions\n)?`)

// Client runs ddcutil commands.
type Client struct {
	Path    string
	Timeout time.Duration
	Verbose bool
	log     func(format string, args ...any)
}

// NewClient returns a ddcutil client using path or "ddcutil" from PATH.
func NewClient(path string, timeout time.Duration, verbose bool) *Client {
	if path == "" {
		path = "ddcutil"
	}
	if timeout == 0 {
		timeout = defaultTimeout
	}
	logFn := func(string, ...any) {}
	if verbose {
		logFn = func(format string, args ...any) {
			fmt.Printf("[ddcutil] "+format+"\n", args...)
		}
	}
	return &Client{
		Path:    path,
		Timeout: timeout,
		Verbose: verbose,
		log:     logFn,
	}
}

func stripKnownNoise(stderr string) string {
	cleaned := i2c0NoiseRE.ReplaceAllString(stderr, "")
	return strings.TrimSpace(cleaned)
}

// HasI2C0Noise reports whether stderr contains the harmless /dev/i2c-0 warning.
func HasI2C0Noise(stderr string) bool {
	return strings.Contains(stderr, "/dev/i2c-0") && strings.Contains(stderr, "EACCES")
}

// IsOnlyI2C0Noise reports whether stderr contains only the harmless i2c-0 warning.
func IsOnlyI2C0Noise(stderr string) bool {
	if !HasI2C0Noise(stderr) {
		return false
	}
	return stripKnownNoise(stderr) == ""
}

// ProbeI2C0Noise runs detect and returns true if stderr has only the i2c-0 warning.
func (c *Client) ProbeI2C0Noise(ctx context.Context) bool {
	_, stderr, err := c.runOnce(ctx, "detect", "--brief")
	if err != nil {
		return false
	}
	return IsOnlyI2C0Noise(stderr)
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		out, _, err := c.runOnce(ctx, args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (c *Client) runOnce(ctx context.Context, args ...string) (string, string, error) {
	cmdArgs := append([]string{c.Path}, args...)
	c.log("exec: %v", cmdArgs)

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		msg := stripKnownNoise(stderrStr)
		if msg == "" {
			msg = err.Error()
		}
		return "", stderrStr, fmt.Errorf("ddcutil %v: %s", args, msg)
	}
	return stdout.String(), stderr.String(), nil
}
