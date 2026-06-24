package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads the YAML file at path and unmarshals it into target.
func Load(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("config: parse %q: %w", path, err)
	}
	return nil
}

// MustLoad is like Load but panics on error.
func MustLoad(path string, target any) {
	if err := Load(path, target); err != nil {
		panic(err)
	}
}

// Getenv returns the value of the named environment variable, or defaultVal
// when the variable is unset or empty.
func Getenv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
