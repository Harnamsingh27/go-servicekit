package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harnamsingh/go-servicekit/config"
)

// testConfig is a flat config struct used across most tests.
type testConfig struct {
	Host     string        `yaml:"host"     env:"TC_HOST"    default:"localhost"`
	Port     int           `yaml:"port"     env:"TC_PORT"    default:"8080"`
	Debug    bool          `yaml:"debug"    env:"TC_DEBUG"   default:"false"`
	Timeout  time.Duration `yaml:"timeout"  env:"TC_TIMEOUT" default:"30s"`
	Secret   string        `yaml:"secret"   env:"TC_SECRET"  validate:"required"`
	Score    float64       `yaml:"score"    env:"TC_SCORE"   default:"1.5"`
}

// requiredConfig has a required field with no default.
type requiredConfig struct {
	Token string `env:"TC_TOKEN" validate:"required"`
}

// validatedConfig implements Validator.
type validatedConfig struct {
	Port int `yaml:"port" env:"TC_VAL_PORT" default:"0"`
}

func (v *validatedConfig) Validate() error {
	if v.Port < 1 || v.Port > 65535 {
		return fmt.Errorf("port %d out of range", v.Port)
	}
	return nil
}

// nestedConfig tests struct nesting.
type nestedConfig struct {
	Server struct {
		Host string `yaml:"host" env:"TC_N_HOST" default:"0.0.0.0"`
		Port int    `yaml:"port" env:"TC_N_PORT" default:"9090"`
	} `yaml:"server"`
}

func writeTemp(t *testing.T, ext, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "cfg*"+ext)
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	f.Close()
	return f.Name()
}

// unsetEnv removes the env var and restores it after the test.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	os.Setenv(key, val)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, old)
		}
	})
}

// ---- Tests ---------------------------------------------------------------

func TestLoad_Defaults(t *testing.T) {
	unsetEnv(t, "TC_HOST")
	unsetEnv(t, "TC_PORT")
	unsetEnv(t, "TC_SECRET")

	cfg, err := config.Load[testConfig](
		config.WithOverrides(map[string]string{"TC_SECRET": "s3cr3t"}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.Score != 1.5 {
		t.Errorf("Score = %f, want 1.5", cfg.Score)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	setEnv(t, "TC_HOST", "prod.example.com")
	setEnv(t, "TC_PORT", "443")
	setEnv(t, "TC_SECRET", "topsecret")

	cfg, err := config.Load[testConfig]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "prod.example.com" {
		t.Errorf("Host = %q, want prod.example.com", cfg.Host)
	}
	if cfg.Port != 443 {
		t.Errorf("Port = %d, want 443", cfg.Port)
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	unsetEnv(t, "TC_HOST")
	unsetEnv(t, "TC_PORT")
	unsetEnv(t, "TC_SECRET")

	yaml := "host: yaml.host\nport: 9999\nsecret: yamlsecret\n"
	path := writeTemp(t, ".yaml", yaml)

	cfg, err := config.Load[testConfig](config.WithYAMLFile(path))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "yaml.host" {
		t.Errorf("Host = %q, want yaml.host", cfg.Host)
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want 9999", cfg.Port)
	}
	if cfg.Secret != "yamlsecret" {
		t.Errorf("Secret = %q, want yamlsecret", cfg.Secret)
	}
}

func TestLoad_EnvFileParsing(t *testing.T) {
	unsetEnv(t, "TC_HOST")
	unsetEnv(t, "TC_SECRET")

	envFile := "# comment\nTC_HOST=envfile.host\nTC_SECRET=envsecret\n"
	path := writeTemp(t, ".env", envFile)

	cfg, err := config.Load[testConfig](config.WithEnvFile(path))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "envfile.host" {
		t.Errorf("Host = %q, want envfile.host", cfg.Host)
	}
}

func TestLoad_EnvOverridesEnvFile(t *testing.T) {
	setEnv(t, "TC_HOST", "sysenv.host")
	setEnv(t, "TC_SECRET", "s3cr3t")

	envFile := "TC_HOST=envfile.host\n"
	path := writeTemp(t, ".env", envFile)

	cfg, err := config.Load[testConfig](config.WithEnvFile(path))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// System env var wins over .env file.
	if cfg.Host != "sysenv.host" {
		t.Errorf("Host = %q, want sysenv.host", cfg.Host)
	}
}

func TestLoad_OverridesWinOverEverything(t *testing.T) {
	setEnv(t, "TC_HOST", "sysenv.host")
	setEnv(t, "TC_SECRET", "s3cr3t")

	cfg, err := config.Load[testConfig](
		config.WithOverrides(map[string]string{"TC_HOST": "override.host"}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "override.host" {
		t.Errorf("Host = %q, want override.host", cfg.Host)
	}
}

func TestLoad_MissingRequiredField(t *testing.T) {
	unsetEnv(t, "TC_TOKEN")

	_, err := config.Load[requiredConfig]()
	if err == nil {
		t.Fatal("expected error for missing required field, got nil")
	}
}

func TestLoad_InvalidType(t *testing.T) {
	setEnv(t, "TC_PORT", "notanumber")
	setEnv(t, "TC_SECRET", "s3cr3t")

	_, err := config.Load[testConfig]()
	if err == nil {
		t.Fatal("expected error for invalid int, got nil")
	}
}

func TestLoad_YAMLFileMissing(t *testing.T) {
	_, err := config.Load[testConfig](
		config.WithYAMLFile(filepath.Join(t.TempDir(), "nonexistent.yaml")),
	)
	if err == nil {
		t.Fatal("expected error for missing YAML file, got nil")
	}
}

func TestLoad_EnvFileMissing(t *testing.T) {
	_, err := config.Load[testConfig](
		config.WithEnvFile(filepath.Join(t.TempDir(), "nonexistent.env")),
	)
	if err == nil {
		t.Fatal("expected error for missing .env file, got nil")
	}
}

func TestLoad_EnvFileMalformed(t *testing.T) {
	path := writeTemp(t, ".env", "NOEQUALSSIGN\n")
	_, err := config.Load[testConfig](config.WithEnvFile(path))
	if err == nil {
		t.Fatal("expected error for malformed .env file, got nil")
	}
}

func TestLoad_ValidatorHook(t *testing.T) {
	unsetEnv(t, "TC_VAL_PORT")

	_, err := config.Load[validatedConfig]()
	if err == nil {
		t.Fatal("expected Validate() error for port=0, got nil")
	}

	setEnv(t, "TC_VAL_PORT", "8080")
	cfg, err := config.Load[validatedConfig]()
	if err != nil {
		t.Fatalf("expected no error for port=8080, got %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
}

func TestLoad_NestedStruct(t *testing.T) {
	unsetEnv(t, "TC_N_HOST")
	unsetEnv(t, "TC_N_PORT")

	cfg, err := config.Load[nestedConfig]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoad_BoolField(t *testing.T) {
	setEnv(t, "TC_DEBUG", "true")
	setEnv(t, "TC_SECRET", "s3cr3t")

	cfg, err := config.Load[testConfig]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Debug {
		t.Error("Debug should be true")
	}
}

func TestLoad_InvalidBool(t *testing.T) {
	setEnv(t, "TC_DEBUG", "notabool")
	setEnv(t, "TC_SECRET", "s3cr3t")

	_, err := config.Load[testConfig]()
	if err == nil {
		t.Fatal("expected error for invalid bool, got nil")
	}
}
