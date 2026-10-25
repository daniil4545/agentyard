package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniil4545/agentyard/internal/config"
)

// claude: `claude -p --output-format json` prints one JSON object with the result and the
// session id; the container is the sandbox, so permission prompts are off.
func (c CLI) claude(ctx context.Context, req Request) (Result, error) {
	args := []string{"-p", "--output-format", "json", "--dangerously-skip-permissions"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Session != "" {
		args = append(args, "--resume", req.Session)
	}
	out, err := c.run(ctx, config.Claude, req.Dir, req.Prompt, args...)
	if err != nil {
		return Result{}, err
	}
	return parseClaude(out)
}

func parseClaude(out []byte) (Result, error) {
	var res struct {
		Result  string `json:"result"`
		Session string `json:"session_id"`
		IsError bool   `json:"is_error"`
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(lastLine(out), &res); err != nil {
		return Result{}, fmt.Errorf("claude output: %w", err)
	}
	if res.IsError {
		return Result{Session: res.Session}, fmt.Errorf("claude: %s: %s", res.Subtype, tail(res.Result))
	}
	if strings.TrimSpace(res.Result) == "" {
		return Result{Session: res.Session}, errNoText
	}
	return Result{Session: res.Session, Text: res.Result}, nil
}

// codex: `codex exec --json` prints JSONL events, the first carries the thread id that
// `codex exec resume` takes; the final message goes to the -o file.
func (c CLI) codex(ctx context.Context, req Request) (Result, error) {
	tmp, err := os.MkdirTemp("", "codex-")
	if err != nil {
		return Result{}, fmt.Errorf("codex temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // a leftover temp file harms nothing
	last := filepath.Join(tmp, "last.txt")
	args := []string{"exec"}
	if req.Session != "" {
		args = append(args, "resume", req.Session)
	}
	args = append(args, "--json", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "-o", last)
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	args = append(args, "-")
	out, err := c.run(ctx, config.Codex, req.Dir, req.Prompt, args...)
	if err != nil {
		return Result{}, err
	}
	text, err := os.ReadFile(last) //nolint:gosec // our own temp file
	if err != nil {
		return Result{}, fmt.Errorf("codex last message: %w", err)
	}
	return parseCodex(out, string(text), req.Session)
}

func parseCodex(out []byte, last, session string) (Result, error) {
	var failure string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1<<20), 16<<20)
	for scanner.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Thread  string `json:"thread_id"`
			Message string `json:"message"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.Thread != "":
			session = ev.Thread
		case ev.Type == "turn.failed" && ev.Error != nil:
			failure = ev.Error.Message
		case ev.Type == "error":
			failure = ev.Message
		}
	}
	if failure != "" {
		return Result{Session: session}, fmt.Errorf("codex: %s", tail(failure))
	}
	if strings.TrimSpace(last) == "" {
		return Result{Session: session}, errNoText
	}
	return Result{Session: session, Text: strings.TrimSpace(last)}, nil
}

// opencode: `opencode run --format json` prints JSONL events with the session id; the final
// message is the text parts of the last assistant message.
func (c CLI) opencode(ctx context.Context, req Request) (Result, error) {
	args := []string{"run", "--format", "json"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Session != "" {
		args = append(args, "--session", req.Session)
	}
	args = append(args, req.Prompt)
	out, err := c.run(ctx, config.OpenCode, req.Dir, "", args...)
	if err != nil {
		return Result{}, err
	}
	return parseOpenCode(out)
}

func parseOpenCode(out []byte) (Result, error) {
	var session, failure, lastMessage string
	var texts []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1<<20), 16<<20)
	for scanner.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Session string `json:"sessionID"`
			Part    struct {
				Type    string `json:"type"`
				Message string `json:"messageID"`
				Text    string `json:"text"`
			} `json:"part"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &ev) != nil {
			continue
		}
		if ev.Session != "" {
			session = ev.Session
		}
		switch ev.Type {
		case "error":
			failure = ev.Error.Message
		case "text":
			if ev.Part.Message != lastMessage {
				lastMessage, texts = ev.Part.Message, nil
			}
			texts = append(texts, ev.Part.Text)
		}
	}
	if failure != "" {
		return Result{Session: session}, fmt.Errorf("opencode: %s", tail(failure))
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" {
		return Result{Session: session}, errNoText
	}
	return Result{Session: session, Text: text}, nil
}

func lastLine(out []byte) []byte {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	return lines[len(lines)-1]
}
