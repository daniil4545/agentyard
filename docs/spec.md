# agentyard spec

Stage: MVP. The owner accepts pull requests by hand. Acceptance gates, scores and a journal
come later (admitd vision, stage 2 "own farm").

## Contents

1. What it is
2. Flow
3. States
4. Parts
5. Agents
6. Boundaries
7. Scenarios
8. Decisions
9. Out of scope

## 1. What it is

A Docker container on the owner's machine. One process polls the watched GitHub
repositories every minute. An issue with the label `agent` goes to a coding agent: OpenCode,
Claude Code or Codex. The agent commits on a branch. The runner pushes the branch and opens
a pull request as a GitHub App bot. Later notes of the owner on the pull request or the issue
go back to the same agent session. When the agent lacks something, it asks in a comment.

Not: a GitHub Action, a hosted service, an auto-merger, a queue for several users.

Prior art checked on 2026-10-25: anthropics/claude-code-action (no PR of its own, a workflow
and a self-hosted runner per repository, Claude only), openai/codex-action (API key only),
OrchestratorInc/agent-orchestrator (desktop app, tasks started by a person), marcus/nightshift
(maintenance tasks it picks itself, no issues, no review loop), mraza007/baton (polls issues,
no review loop). None covers polling + subscription CLIs + review loop + headless container.

## 2. Flow

```mermaid
flowchart LR
  O["Owner"] -->|"issue + label agent"| I["Issue"]
  P["Runner pass, every 60 s"] -->|"REST as App"| I
  P --> W["Worktree agent/N-slug"]
  W --> A["opencode run / claude -p / codex exec"]
  A -->|"commits"| W
  P -->|"push, open PR: Closes #N"| R["Pull request"]
  A -->|"no commits: questions"| I
  O -->|"comment, review, line comment"| R
  P -->|"new owner notes"| A2["resume the session"]
  A2 -->|"commits, reply"| R
```

One pass, per repository:

1. Every task in state `pr`: a merged or closed PR finishes the task (`done`, worktree
   removed). New owner notes on the PR or the issue resume the session.
2. Open issues with the label `agent`. A new one starts: label `agent:working`, worktree from
   the fresh base branch, first prompt = agent rules + issue + owner comments.
3. A `blocked` issue resumes when the owner wrote after the agent's question.
4. After a run: changes the agent left uncommitted are committed by the runner; new commits
   are pushed; the final message becomes the PR description (first PR), a reply on the PR,
   or a comment on the issue when there are no commits (state `blocked`).

## 3. States

The issue label `agent:<state>` mirrors the task state, so the owner sees it on GitHub.

```mermaid
stateDiagram-v2
  [*] --> working: owner sets agent
  working --> pr: commits, PR opened
  working --> blocked: no commits, question on the issue
  blocked --> working: owner answered
  pr --> working: owner notes on PR or issue
  working --> failed: CLI error or timeout
  failed --> working: owner removed agent:failed
  pr --> done: PR merged or closed
```

`state.json` on the work volume keeps what GitHub does not: agent, session, branch, PR
number and `since`, the creation time of the newest owner note already given to the agent.

## 4. Parts

| Part | Role |
|---|---|
| `internal/config` | `agentyard.toml`: owner, label, interval, timeout, agents, repositories |
| `internal/github` | REST client as GitHub App: JWT, installation tokens, issues, labels, notes, PRs |
| `internal/workspace` | one clone per repository, one worktree per issue, push without force |
| `internal/agent` | the three CLIs behind one call: prompt and session in, session and final message out |
| `internal/state` | `state.json`, written through a temporary file |
| `internal/yard` | the pass, state changes, prompts |
| `rules/AGENTS.md` | rules every first prompt starts with |

## 5. Agents

| Agent | New run | Resume | Login |
|---|---|---|---|
| OpenCode | `opencode run --format json --model M` | `--session ID` | free Zen models need none |
| Claude Code | `claude -p --output-format json --dangerously-skip-permissions` | `--resume ID` | `CLAUDE_CODE_OAUTH_TOKEN` from `claude setup-token` |
| Codex | `codex exec --json --dangerously-bypass-approvals-and-sandbox -o FILE -` | `codex exec resume ID` | `codex login --device-auth` once, inside the container |

The container is the sandbox, so the permission bypass flags are allowed inside it. The
agent's environment has no `AGENTYARD_*` variables: it never sees the App key and cannot
push. A resume that fails starts a new session with the rules and a summary of the task.

## 6. Boundaries

What the runner does not do on any input:

- R1. Takes only issues with the label `agent`; on GitHub only people with triage rights
  can set labels.
- R2. Passes to the agent only notes written by `owner`. A stranger's comment never reaches a
  prompt.
- R3. Never merges, never pushes to the base branch, never force-pushes.
- R4. One agent at a time; a run is cut at `timeout`.
- R5. Gives the agent no GitHub token and no App key.

## 7. Scenarios

| ID | Given | Expect |
|---|---|---|
| S1 | issue labeled `agent`, agent commits | branch pushed, PR `Closes #N`, label `agent:pr` |
| S2 | agent makes no commits | its message is an issue comment, label `agent:blocked` |
| S3 | blocked issue, owner answers | same session resumed with the answer |
| S4 | owner review or line comment on the PR | same session resumed, new commits pushed, reply on the PR |
| S5 | stranger comments | nothing reaches the agent (R2) |
| S6 | PR merged or closed | task `done`, worktree removed |
| S7 | agent CLI fails | comment with the cause, label `agent:failed`; removing the label retries |
| S8 | resume fails (session lost) | new session with rules and task summary |
| S9 | agent leaves changes uncommitted | runner commits them, they reach the PR |
| S10 | container restarts mid-run | task restarts from labels and `state.json` |

## 8. Decisions

- Own poller instead of claude-code-action: polling needs no public URL, one runner serves
  all repositories, any CLI with a subscription works.
- GitHub App bot instead of the owner's token: the owner can review the bot's PRs formally,
  and the bot's comments are told apart by author.
- The runner, not the agent, pushes and opens the PR: deterministic, and the agent holds
  no credentials.
- Rules travel in the prompt, not in files in the image: the same text for all three CLIs.
- simplified: one agent at a time and the first 100 issues and notes per call; revisit with
  parallel agents.

## 9. Out of scope

Acceptance gates and scores of admitd, parallel agents, tasks from tix, notifications, a web
panel, Linux builds of macOS-only projects (tarn's Swift part does not build in the container).
