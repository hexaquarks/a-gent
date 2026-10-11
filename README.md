<h1 align="center">
  a-gent
</h1>

<p align="center">
  <a href="https://github.com/hexaquarks/a-gent/actions/workflows/tests.yml"><img alt="Tests" src="https://img.shields.io/github/actions/workflow/status/hexaquarks/a-gent/tests.yml?branch=main&amp;label=tests" /></a>
  <a href="https://app.codecov.io/gh/hexaquarks/a-gent"><img alt="Coverage" src="https://img.shields.io/codecov/c/github/hexaquarks/a-gent/main" /></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/hexaquarks/a-gent?logo=go" /></a>
</p>

a-gent is a small terminal dashboard for keeping track of Codex, Claude Code,
OpenCode, Pi, and Hermes sessions across projects. See which agents are working or
waiting for you, preview their activity and file edits, and jump to their tmux panes.

<p align="center">
  <img src="assets/dashboard.png" width="700" alt="a-gent dashboard showing pinned projects, unread session updates, and a color-coded edit preview" />
</p>

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

| Adapter | Command |
| --- | --- |
| Codex | `codex` |
| Claude Code | `claude` |
| OpenCode | `opencode` |
| Pi | `pi` |
| Hermes | `hermes` |

Sessions are discovered automatically, including those started from a shell.
All five adapters support activity and edit previews, and launching a new
interactive instance in a tmux pane or window from the dashboard.

> [!IMPORTANT]
> For Claude Code, update your CLI if `claude agents --json` is unavailable.
>
> After the first a-gent launch, restart any already-running OpenCode and Hermes
> sessions. In Pi, run `/reload` or restart. New sessions need no extra setup.

> [!NOTE]
> Pi shows running or idle; it cannot identify approval prompts from extensions.

## Usage

```sh
a-gent
```

Inside tmux, the dashboard opens in a popup. Select a session and press Enter to
jump to its matching tmux pane. Outside tmux, it runs directly in your terminal.

- Press Tab to switch between the sidebar and sessions; use `j`/`k` or the arrow
  keys to browse.
- Use the sidebar to filter by project, provider, or session state. Press Enter
  to apply a filter.
- Press `/` to search projects and `p` to pin or unpin a project in the sidebar.
- Press `s` to change the sort column or `S` to reverse it.
- Press `v` to expand the preview and Escape to return; press `q` to quit.

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
