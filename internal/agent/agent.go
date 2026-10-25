// Package agent runs a coding agent CLI headless in a worktree: Claude Code, Codex or
// OpenCode. A run takes a prompt and an optional session to resume and returns the session
// and the agent's final message.
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/daniil4545/agentyard/internal/config"
)

type Request struct {
	Dir     string
	Prompt  string
	Session string
	Model   string
}

type Result struct {
	Session string
	Text    string
}

type CLI struct {
	// Commands overrides the binary of an agent, for tests.
	Commands map[string]string
}

func (c CLI) Run(ctx context.Context, name string, req Request) (Result, error) {
	switch name {
	case config.Claude:
		return c.claude(ctx, req)
	case config.Codex:
		return c.codex(ctx, req)
	case config.OpenCode:
		return c.opencode(ctx, req)
	}
	return Result{}, fmt.Errorf("unknown agent %q", name)
}

// run starts the CLI in the worktree with the runner's own secrets removed from the
// environment: the agent needs its CLI login, not the GitHub App key.
func (c CLI) run(ctx context.Context, name, dir, stdin string, args ...string) ([]byte, error) {
	binary := name
	if c.Commands[name] != "" {
		binary = c.Commands[name]
	}
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // a known agent CLI, arguments built by the runner
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = agentEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return stdout.Bytes(), fmt.Errorf("%s: %w", name, ctx.Err())
	}
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("%s: %w: %s", name, err, tail(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func agentEnv(env []string) []string {
	var kept []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, "AGENTYARD_") {
			kept = append(kept, kv)
		}
	}
	return kept
}

// tail keeps the end of an error text, where CLIs print the cause.
func tail(s string) string {
	const limit = 500
	s = strings.TrimSpace(s)
	if len(s) > limit {
		return "..." + s[len(s)-limit:]
	}
	return s
}

var errNoText = errors.New("agent returned no final message")
