package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultPollMS       = 2000
	DefaultHysteresis   = 3 // brightness percent points
	configDirName       = "display-brightness"
	configFileName      = "config.json"
)

// Config is persisted daemon state for auto-brightness.
type Config struct {
	Auto    bool    `json:"auto"`
	Curve   []Point `json:"curve"`
	PollMS  int     `json:"poll_ms"`
	Port    string  `json:"port"`
	// Hysteresis is minimum brightness change before applying (not always in JSON).
	Hysteresis int `json:"hysteresis,omitempty"`
}

// Default returns a fresh default config (auto off until user enables).
func Default() Config {
	return Config{
		Auto:       false,
		Curve:      DefaultCurve(),
		PollMS:     DefaultPollMS,
		Port:       "",
		Hysteresis: DefaultHysteresis,
	}
}

// Path returns ~/.config/display-brightness/config.json (or $XDG_CONFIG_HOME).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configDirName, configFileName), nil
}

// Load reads config from disk, or returns Default if missing.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := ValidateCurve(cfg.Curve); err != nil {
		cfg.Curve = DefaultCurve()
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if len(c.Curve) == 0 {
		c.Curve = DefaultCurve()
	} else {
		c.Curve = NormalizeCurve(c.Curve)
	}
	if c.PollMS <= 0 {
		c.PollMS = DefaultPollMS
	}
	if c.Hysteresis <= 0 {
		c.Hysteresis = DefaultHysteresis
	}
}

// Save writes config atomically.
func Save(cfg Config) error {
	cfg.applyDefaults()
	if err := ValidateCurve(cfg.Curve); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
