// Package config loads the modhak YAML config file (ADR 0008).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPort           = 3306
	DefaultConnectTimeout = 5 * time.Second
	DefaultQueryTimeout   = 10 * time.Second
)

// Config is the top-level config file.
type Config struct {
	Targets []Target `yaml:"targets"`
}

// Target is one database modhak watches. Capabilities, collection and
// findings are tracked per target.
type Target struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
	// Password is a literal password. PasswordEnv names an environment
	// variable to read instead; set at most one. Secrets handling is an open
	// question (ARCHITECTURE.md #11); this is the minimum that avoids
	// committing passwords.
	Password    string `yaml:"password"`
	PasswordEnv string `yaml:"password_env"`
	// TLS is passed to the driver: "", "false", "true", "skip-verify" or
	// "preferred".
	TLS            string        `yaml:"tls"`
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	QueryTimeout   time.Duration `yaml:"query_timeout"`
}

// Addr returns host:port.
func (t Target) Addr() string { return fmt.Sprintf("%s:%d", t.Host, t.Port) }

// ResolvedPassword returns the literal password or the value of PasswordEnv.
func (t Target) ResolvedPassword(getenv func(string) string) (string, error) {
	if t.PasswordEnv == "" {
		return t.Password, nil
	}
	v := getenv(t.PasswordEnv)
	if v == "" {
		return "", fmt.Errorf("target %q: environment variable %s is empty or unset", t.Name, t.PasswordEnv)
	}
	return v, nil
}

// Target returns the named target.
func (c *Config) Target(name string) (Target, bool) {
	for _, t := range c.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}

// Load reads and validates the config file at path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	c, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes and validates config from r. Unknown fields are errors so
// typos do not silently fall back to defaults.
func Parse(r io.Reader) (*Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config is empty")
		}
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Target names appear in URLs (/t/{target}/...), so keep them URL-safe.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var validTLS = map[string]bool{"": true, "false": true, "true": true, "skip-verify": true, "preferred": true}

func (c *Config) normalize() error {
	if len(c.Targets) == 0 {
		return errors.New("config has no targets")
	}
	seen := map[string]bool{}
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Name == "" {
			return fmt.Errorf("targets[%d]: name is required", i)
		}
		if !nameRE.MatchString(t.Name) {
			return fmt.Errorf("target %q: name must match %s", t.Name, nameRE)
		}
		if seen[t.Name] {
			return fmt.Errorf("target %q: duplicate name", t.Name)
		}
		seen[t.Name] = true
		if t.Host == "" {
			return fmt.Errorf("target %q: host is required", t.Name)
		}
		if t.User == "" {
			return fmt.Errorf("target %q: user is required", t.Name)
		}
		if t.Port == 0 {
			t.Port = DefaultPort
		}
		if t.Port < 1 || t.Port > 65535 {
			return fmt.Errorf("target %q: port %d out of range", t.Name, t.Port)
		}
		if t.Password != "" && t.PasswordEnv != "" {
			return fmt.Errorf("target %q: set only one of password and password_env", t.Name)
		}
		if !validTLS[strings.ToLower(t.TLS)] {
			return fmt.Errorf("target %q: tls %q must be one of false, true, skip-verify, preferred", t.Name, t.TLS)
		}
		t.TLS = strings.ToLower(t.TLS)
		if t.ConnectTimeout < 0 || t.QueryTimeout < 0 {
			return fmt.Errorf("target %q: timeouts must not be negative", t.Name)
		}
		if t.ConnectTimeout == 0 {
			t.ConnectTimeout = DefaultConnectTimeout
		}
		if t.QueryTimeout == 0 {
			t.QueryTimeout = DefaultQueryTimeout
		}
	}
	return nil
}
