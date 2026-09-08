# rfx — Redfish Explorer

## Build & Commands

- **Compile code**: `task build` (binary at `bin/rfx`)
- **Run Tests**: `task test`
- **Run Tests for a Single Package**: `go test ./path/to/package`
- **Run Single Test**: `go test -run TestName ./path/to/package`
- **Run Linter**: `task lint`
- **Format code**: `task format`
- **Install tools**: `task install` (builds the pinned toolchain into `bin/`)
- **Install git hooks**: `task install-githooks`

## Architecture & Structure

- `cmd/rfx`: entry point — flags, credential resolution, fail-fast connect, TUI
  startup
- `internal/redfish`: Redfish client, raw fetch, curl rendering, link
  extraction, OEM detection
- `internal/cache`: in-memory TTL cache of visited endpoints
- `internal/tui`: Bubble Tea model, panes, key bindings, JSON rendering
- `.scratch/`: plans and experiments — not linted, not tested, not committed

`gofish` is used for connection setup, TLS and ServiceRoot validation only.
Exploration GETs go through `APIClient.HTTPClient` directly, because
`APIClient.Get` discards the response for any status outside 200/201/202/204 and
rfx must be able to show a 404 or 501 body.

## Code Style & Conventions

- **Formatting**: `gofumpt`, plus `newline-after-block`; `task format` applies
  both
- **Linting**: `golangci-lint` with `revive: enable-all-rules: true` — it is
  strict, and it shapes the code: tests live in `package foo_test`, no
  package-level `var`, no `init()`, no behaviour-switching bool parameters,
  two-value type assertions, named mutex fields, wrapped errors, exhaustive enum
  switches
- **Documentation**: self-documenting code, minimal inline comments
- **Go**: Version 1.26

## Tools & Dependencies

- **Task**: task runner (`Taskfile.yml`)
- **lefthook**: git hooks (`lefthook.yml`) — pre-commit, pre-push, commit-msg
- Dev tools are pinned as `go.mod` `tool` directives and built into `bin/` by
  `task install`
- `markdownlint-cli2` is an npm tool: `npm install -g markdownlint-cli2`

**Bubble Tea and Lip Gloss are v2** (`charm.land/bubbletea/v2`,
`charm.land/lipgloss/v2`), not the v1 `github.com/charmbracelet/...` packages.
`View()` returns `tea.View`, key presses arrive as `tea.KeyPressMsg`, alt-screen
is a `View` field, and `lipgloss.Color` is a function.
