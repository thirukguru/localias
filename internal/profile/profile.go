// Package profile provides localias.yaml profile parsing and service management.
// Profiles define named groups of services that can be started/stopped together.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/profile
package profile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level localias.yaml configuration.
type Config struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

// Profile represents a named group of services.
type Profile struct {
	Services []Service `yaml:"services"`
}

// Service represents a single service within a profile.
type Service struct {
	Name string `yaml:"name"`
	Cmd  string `yaml:"cmd"`
	Dir  string `yaml:"dir"`
	Env  map[string]string `yaml:"env,omitempty"`
}

// LoadConfig loads and parses a localias.yaml file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

// FindConfig searches for localias.yaml in the current directory and parent directories.
func FindConfig() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		path := dir + "/localias.yaml"
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		path = dir + "/localias.yml"
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		parent := dir[:max(0, len(dir)-len("/"+dir[lastSlash(dir)+1:]))]
		if parent == dir || parent == "" {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("localias.yaml not found in current or parent directories")
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GetProfile returns a profile by name, defaulting to "default".
func (c *Config) GetProfile(name string) (*Profile, error) {
	if name == "" {
		name = "default"
	}
	p, ok := c.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	return &p, nil
}

// ListProfiles returns all profile names.
func (c *Config) ListProfiles() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	return names
}
