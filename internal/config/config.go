// Package config reads agentyard.toml: whose word counts, which repositories to watch and
// which agent works on them.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// Agent names the runner knows.
const (
	Claude   = "claude"
	Codex    = "codex"
	OpenCode = "opencode"
)

type Config struct {
	// Owner is the GitHub login whose labels and comments the runner obeys.
	Owner    string        `toml:"owner"`
	Label    string        `toml:"label"`
	Interval time.Duration `toml:"interval"`
	Timeout  time.Duration `toml:"timeout"`
	// LimitPause is how long an agent rests after its plan ran out, when the provider gave
	// no time of its own.
	LimitPause time.Duration     `toml:"limit_pause"`
	WorkDir    string            `toml:"work_dir"`
	Agent      string            `toml:"agent"`
	Models     map[string]string `toml:"models"`
	Repos      []Repo            `toml:"repos"`
}

type Repo struct {
	Name  string `toml:"name"`
	Base  string `toml:"base"`
	Agent string `toml:"agent"`
}

func Load(path string) (Config, error) {
	cfg := Config{
		Label:      "agent",
		Interval:   time.Minute,
		Timeout:    time.Hour,
		LimitPause: time.Hour,
		WorkDir:    "/work",
		Agent:      OpenCode,
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	return cfg, cfg.check()
}

func (c Config) check() error {
	if c.Owner == "" {
		return errors.New("owner is empty")
	}
	if len(c.Repos) == 0 {
		return errors.New("no repos")
	}
	if err := checkAgent(c.Agent); err != nil {
		return err
	}
	for _, r := range c.Repos {
		if r.Name == "" || r.Base == "" {
			return fmt.Errorf("repo %q: name and base are required", r.Name)
		}
		if r.Agent != "" {
			if err := checkAgent(r.Agent); err != nil {
				return fmt.Errorf("repo %s: %w", r.Name, err)
			}
		}
	}
	return nil
}

func checkAgent(name string) error {
	switch name {
	case Claude, Codex, OpenCode:
		return nil
	}
	return fmt.Errorf("unknown agent %q: want claude, codex or opencode", name)
}
