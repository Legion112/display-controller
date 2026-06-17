package ddcutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

const defaultTimeout = 5 * time.Second

// Client runs ddcutil commands.
type Client struct {
	Path    string
	Timeout time.Duration
	Verbose bool
	mu      sync.Mutex
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

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		out, err := c.runOnce(ctx, args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (c *Client) runOnce(ctx context.Context, args ...string) (string, error) {
	cmdArgs := append([]string{c.Path}, args...)
	c.log("exec: %v", cmdArgs)

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("ddcutil %v: %s", args, msg)
	}
	return stdout.String(), nil
}
