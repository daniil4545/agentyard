// Package workspace keeps one clone per repository and one git worktree per issue under the
// work directory, and pushes branches with the App token.
package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// TokenFunc returns a push and fetch token for the repository.
type TokenFunc func(ctx context.Context, repo string) (string, error)

type Workspace struct {
	root   string
	remote string // prefix of clone URLs, https://github.com/ in production
	token  TokenFunc
	name   string
	email  string
}

func New(root, remote string, token TokenFunc, name, email string) *Workspace {
	return &Workspace{root: root, remote: remote, token: token, name: name, email: email}
}

// Prepare returns the worktree of the issue on its branch. A new branch starts from the
// fresh base; an existing worktree or remote branch is reused.
func (w *Workspace) Prepare(ctx context.Context, repo, base, branch string, issue int) (string, error) {
	clone := filepath.Join(w.root, "repos", repo)
	if _, err := os.Stat(filepath.Join(clone, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(clone), 0o750); err != nil {
			return "", fmt.Errorf("clone dir: %w", err)
		}
		if err := w.authGit(ctx, repo, "", "clone", "--filter=blob:none", w.remote+repo+".git", clone); err != nil {
			return "", err
		}
	}
	if err := w.authGit(ctx, repo, clone, "fetch", "--prune", "origin"); err != nil {
		return "", err
	}
	for key, value := range map[string]string{"user.name": w.name, "user.email": w.email} {
		if _, err := git(ctx, clone, nil, "config", key, value); err != nil {
			return "", err
		}
	}
	tree := w.treePath(repo, issue)
	if _, err := os.Stat(tree); err == nil {
		return tree, nil
	}
	start := "origin/" + base
	if _, err := git(ctx, clone, nil, "rev-parse", "--verify", "--quiet", "origin/"+branch); err == nil {
		start = "origin/" + branch
	}
	if _, err := git(ctx, clone, nil, "worktree", "add", "-B", branch, tree, start); err != nil {
		return "", err
	}
	return tree, nil
}

func (w *Workspace) Head(ctx context.Context, dir string) (string, error) {
	return git(ctx, dir, nil, "rev-parse", "HEAD")
}

// CommitLeftovers commits files the agent changed but did not commit; free models often skip
// the commit, and losing their work would waste the run.
func (w *Workspace) CommitLeftovers(ctx context.Context, dir, message string) (bool, error) {
	status, err := git(ctx, dir, nil, "status", "--porcelain")
	if err != nil || status == "" {
		return false, err
	}
	if _, err := git(ctx, dir, nil, "add", "-A"); err != nil {
		return false, err
	}
	if _, err := git(ctx, dir, nil, "commit", "-q", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// Ahead counts commits of the worktree branch that the base does not have.
func (w *Workspace) Ahead(ctx context.Context, dir, base string) (int, error) {
	out, err := git(ctx, dir, nil, "rev-list", "--count", "origin/"+base+"..HEAD")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("count commits: %w", err)
	}
	return n, nil
}

// Push sends the branch without force: a rejected push means someone else wrote to it.
func (w *Workspace) Push(ctx context.Context, repo, dir, branch string) error {
	return w.authGit(ctx, repo, dir, "push", "origin", "HEAD:refs/heads/"+branch)
}

// Remove drops the worktree and the local branch; the remote branch stays with the PR.
func (w *Workspace) Remove(ctx context.Context, repo, branch string, issue int) error {
	clone := filepath.Join(w.root, "repos", repo)
	tree := w.treePath(repo, issue)
	if _, err := os.Stat(tree); err == nil {
		if _, err := git(ctx, clone, nil, "worktree", "remove", "--force", tree); err != nil {
			return err
		}
	}
	_, err := git(ctx, clone, nil, "branch", "-D", branch)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

func (w *Workspace) treePath(repo string, issue int) string {
	return filepath.Join(w.root, "trees", strings.ReplaceAll(repo, "/", "-")+"-"+strconv.Itoa(issue))
}

// authGit runs git with the token in an HTTP header passed through the environment, so it
// is neither in the process arguments nor in the saved remote URL.
func (w *Workspace) authGit(ctx context.Context, repo, dir string, args ...string) error {
	tok, err := w.token(ctx, repo)
	if err != nil {
		return fmt.Errorf("git token: %w", err)
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + tok))
	env := []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic " + basic,
		"GIT_TERMINAL_PROMPT=0",
	}
	_, err = git(ctx, dir, env, args...)
	return err
}

func git(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // fixed binary, arguments built by the runner
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
