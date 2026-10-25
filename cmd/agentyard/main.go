// Command agentyard polls GitHub issues labeled for agents, runs a coding agent on each and
// opens pull requests as a GitHub App.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/daniil4545/agentyard/internal/agent"
	"github.com/daniil4545/agentyard/internal/config"
	"github.com/daniil4545/agentyard/internal/github"
	"github.com/daniil4545/agentyard/internal/state"
	"github.com/daniil4545/agentyard/internal/workspace"
	"github.com/daniil4545/agentyard/internal/yard"
)

func main() {
	if err := run(); err != nil {
		slog.Error("agentyard stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "agentyard.toml", "path to the config file")
	once := flag.Bool("once", false, "make one pass and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	appID := os.Getenv("AGENTYARD_APP_ID")
	if appID == "" {
		return errors.New("AGENTYARD_APP_ID is not set")
	}
	key, err := github.ParseKey(os.Getenv("AGENTYARD_APP_KEY_B64"))
	if err != nil {
		return fmt.Errorf("AGENTYARD_APP_KEY_B64: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gh := github.NewClient("https://api.github.com", appID, key)
	name, email, err := gh.Identity(ctx)
	if err != nil {
		return fmt.Errorf("bot identity: %w", err)
	}
	store, err := state.Load(filepath.Join(cfg.WorkDir, "state.json"))
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	y := &yard.Yard{
		Config: cfg,
		GitHub: gh,
		Work:   workspace.New(cfg.WorkDir, "https://github.com/", gh.Token, name, email),
		Agents: agent.CLI{},
		State:  store,
		Log:    log,
	}
	log.Info("agentyard started", "bot", name, "owner", cfg.Owner, "repos", len(cfg.Repos), "interval", cfg.Interval)
	if *once {
		y.Pass(ctx)
		return nil
	}
	y.Run(ctx)
	return nil
}
