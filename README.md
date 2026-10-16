<h1 align="center">
  a-gent
</h1>

<p align="center">
  <a href="https://github.com/hexaquarks/a-gent/actions/workflows/tests.yml"><img alt="Tests" src="https://img.shields.io/github/actions/workflow/status/hexaquarks/a-gent/tests.yml?branch=main&amp;label=tests" /></a>
  <a href="https://app.codecov.io/gh/hexaquarks/a-gent"><img alt="Coverage" src="https://img.shields.io/codecov/c/github/hexaquarks/a-gent/main" /></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/hexaquarks/a-gent?logo=go" /></a>
</p>

a-gent is a small terminal dashboard for keeping track of your favorite coding
agents across projects. See which agents are working or waiting for you, preview
their activity and file edits, and jump to their tmux panes.

<p align="center">
  <img src="assets/dashboard.png" width="1000" alt="a-gent List view with subagents, attention filters, activity, and the latest edit" />
</p>

<details>
<summary>Other layouts</summary>

**Board**

<p align="center">
  <img src="assets/dashboard-board.png" width="1000" alt="a-gent Board view with needs you, running, and idle columns, activity and subagent summaries, and completion hints" />
</p>

</details>

## Supported adapters

<p align="center">
  <a href="https://github.com/openai/codex"><img alt="Codex" src="https://img.shields.io/badge/Codex-74AA9C" /></a>
  <a href="https://github.com/anthropics/claude-code"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-D97757" /></a>
  <a href="https://github.com/anomalyco/opencode"><img alt="OpenCode" src="https://img.shields.io/badge/OpenCode-4B5563" /></a>
  <a href="https://github.com/badlogic/pi-mono/tree/main/packages/coding-agent"><img alt="Pi" src="https://img.shields.io/badge/Pi-8B5CF6" /></a>
  <a href="https://github.com/NousResearch/hermes-agent"><img alt="Hermes" src="https://img.shields.io/badge/Hermes-F59E0B" /></a>
</p>

Install the agents you use through the links above and make their commands
available on your `PATH`:

| Adapter | Command | Notes |
| --- | --- | --- |
| Codex | `codex` | Sessions launched with `--no-daemon` are not shown. |
| Claude Code | `claude` | Must support `claude agents --json`. |
| OpenCode | `opencode` | — |
| Pi | `pi` | — |
| Hermes | `hermes` | — |

Sessions are discovered automatically, including those started from a shell.
All five adapters support activity and edit previews, and launching a new
interactive instance in a tmux pane or window from the dashboard.

> [!IMPORTANT]
> a-gent automatically installs the OpenCode and Hermes plugins and the Pi
> extension. After your first launch, restart existing OpenCode and Hermes
> sessions; in Pi, run `/reload` or restart.

> [!NOTE]
> Pi shows running or idle; it cannot identify approval prompts from extensions.

## Install

Requires Go 1.24 or newer and Make:

```sh
git clone https://github.com/hexaquarks/a-gent.git
cd a-gent
make install
```

> [!TIP]
> The binary is installed to `~/.local/bin`. Add that directory to your `PATH`,
> or choose another destination with `make install BIN_DIR=/path/to/bin`.

## Usage

```sh
a-gent
```

Inside tmux, the dashboard opens in a popup. Select a session and press Enter to
jump to its matching tmux pane. Outside tmux, it runs directly in your terminal.

- `Tab` changes focus; arrows navigate; `Enter` opens a session or applies a filter.
- `m` switches layout; `v` expands the preview; `Esc` returns.
- `n` / `N` launches an agent in a pane / window.
- `q` closes the current view or quits; `?` shows all shortcuts.

## Local development

Run directly from the checkout:

```sh
make run
```

## Tests

Run formatting checks, unit tests, and static analysis:

```sh
make check
```

Run the terminal integration tests with tmux installed:

```sh
make test-integration
```
