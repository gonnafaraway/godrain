package allowlist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gonnafaraway/godrain/internal/profile"
)

// Config lists patterns that suppress leak suspects (intentional allocators).
type Config struct {
	Patterns []string `yaml:"patterns"`
}

// Load reads an allowlist YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse allowlist: %w", err)
	}
	return &c, nil
}

// Match reports whether any frame in the stack matches an allowlist pattern.
// Patterns support simple '*' wildcards (glob-style on the function or file path).
func (c *Config) Match(frames []profile.Frame) bool {
	if c == nil {
		return false
	}
	for _, pat := range c.Patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		for _, f := range frames {
			if matchOne(pat, f.Function) || matchOne(pat, f.File) {
				return true
			}
		}
	}
	return false
}

func matchOne(pattern, value string) bool {
	if pattern == value {
		return true
	}
	// substring convenience: "cache" matches ".../cache/..."
	if !strings.ContainsAny(pattern, "*?") {
		return strings.Contains(value, pattern)
	}
	ok, err := filepath.Match(pattern, value)
	if err != nil {
		return false
	}
	if ok {
		return true
	}
	// Also try matching against path segments with ** simplified to *
	simplified := strings.ReplaceAll(pattern, "**", "*")
	ok, err = filepath.Match(simplified, value)
	return err == nil && ok
}
