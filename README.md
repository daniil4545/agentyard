# agentyard

Label a GitHub issue `agent`, get a pull request from a coding agent, and review it like a
colleague's: your comments go back to the same agent session. Runs locally in Docker with
OpenCode, Claude Code or Codex on your own subscriptions.

## Stage

MVP for one owner. Polling, the three agents, questions on the issue, the review loop and a
GitHub App bot work; the owner merges by hand. Design and scenarios: `docs/spec.md`.

## How it works

1. You open an issue and add the label `agent` (or `agent:claude`, `agent:codex` next to it).
2. Within a minute the runner checks out `agent/<N>-<slug>` and starts the agent.
3. The agent commits; the runner pushes and opens a PR that `Closes #N`.
4. You comment or review the PR. The runner passes your notes to the same session; the agent
   pushes fixes and replies.
5. If the issue lacks something, the agent asks on the issue (`agent:blocked`) and continues
   after your answer.

Issue labels show the state: `agent:working`, `agent:pr`, `agent:blocked`, `agent:failed`,
`agent:done`. Remove `agent:failed` to retry.

## Quick start

Requirements: Docker, a GitHub App, `envkey` or another way to pass environment variables.

1. Create a GitHub App: permissions Contents, Issues and Pull requests read and write,
   Metadata read; webhook off. Generate a private key and install the App on the
   repositories to watch.
2. Put its id and key into `.env`:
   `envkey set --public AGENTYARD_APP_ID <id>` and
   `base64 -i <key>.pem | envkey set AGENTYARD_APP_KEY_B64`.
3. `cp agentyard.example.toml agentyard.toml` and list your repositories.
4. `envkey run -- docker compose up -d --build`
5. `docker compose logs -f` shows `agentyard started` and a pass every minute.

Optional logins:

- Claude Code: `claude setup-token`, then `envkey set CLAUDE_CODE_OAUTH_TOKEN`.
- Codex: `docker compose exec agentyard codex login --device-auth` once; the login stays in
  the `home` volume.
- OpenCode free models need no login.

Tests and linters: `make check` (needs Go, git and golangci-lint).

## Configuration

| Variable | Required | Meaning |
|---|---|---|
| `AGENTYARD_APP_ID` | yes | GitHub App id |
| `AGENTYARD_APP_KEY_B64` | yes | base64 of the App private key PEM |
| `CLAUDE_CODE_OAUTH_TOKEN` | for Claude | subscription token from `claude setup-token` |

Everything else is in `agentyard.toml`, see `agentyard.example.toml`.

## Documents

| Question | Where |
|---|---|
| model, states, scenarios, decisions | `docs/spec.md` |
| rules every agent run starts with | `rules/AGENTS.md` |
| rules for agents developing agentyard | `AGENTS.md` |
| what changed | `git log` |
