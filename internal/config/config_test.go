package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, body string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "famulus.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadDefaultsAndDurations(t *testing.T) {
	cfg, err := load(t, `
owner = "me"
timeout = "30m"
[models]
opencode = "opencode/glm-5.3-flash-free"
[[repos]]
name = "me/tix"
base = "main"
agent = "claude"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != time.Minute || cfg.Timeout != 30*time.Minute || cfg.Label != "agent" || cfg.Agent != OpenCode {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Models[OpenCode] != "opencode/glm-5.3-flash-free" || cfg.Repos[0].Agent != Claude {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]string{
		"owner is empty": `[[repos]]
name = "a/b"
base = "main"`,
		"no repos":              `owner = "me"`,
		"name and base":         "owner = \"me\"\n[[repos]]\nname = \"a/b\"",
		`unknown agent "aider"`: "owner = \"me\"\nagent = \"aider\"\n[[repos]]\nname = \"a/b\"\nbase = \"main\"",
		"interval":              "owner = \"me\"\ninterval = \"soon\"",
		`effort "fast"`:         "owner = \"me\"\n[effort]\nclaude = \"fast\"\n[[repos]]\nname = \"a/b\"\nbase = \"main\"",
		"effort is for claude":  "owner = \"me\"\n[effort]\ncodex = \"high\"\n[[repos]]\nname = \"a/b\"\nbase = \"main\"",
	}
	for want, body := range cases {
		if _, err := load(t, body); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error %q, got %v", want, err)
		}
	}
}
