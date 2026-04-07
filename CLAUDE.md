# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

This project uses [Task](https://taskfile.dev) as its build tool. Go 1.25+.

```bash
task build          # Build to bin/gitvoyager (injects version/commit/date via ldflags)
task run -- -q "mcp server"  # Build and run with args
task install        # Install to $GOPATH/bin
task test           # go test ./... -v -race (no tests exist yet)
task clean          # Remove build artifacts
```

Binary entry point: `cmd/gitvoyager/main.go`. Flags: `-q`/`-query` (initial search), `-v`/`-version`.

## Architecture

Bubble Tea (Elm-architecture) TUI that discovers GitHub repos via the `gh` CLI (not direct API).

**Dependency flow:** `main` &rarr; `config`, `github`, `store`, `tui`. The `tui` package imports all others; no other cross-package imports exist within `internal/`.

### Packages

- **`internal/tui/`** (~3k LOC) &mdash; Bubble Tea app with state machine (`stateSearchPrompt` &rarr; `stateBrowsing` &rarr; `stateWatchlist` &rarr; `stateReturnVisit`). Three focus panes (list, detail, search) plus peek overlay. `app.go` is the central Update/View hub; `list.go`, `detail.go`, `statusbar.go` are sub-models composed inside it.
- **`internal/github/`** &mdash; Wraps `gh` CLI commands (`gh search repos`, `gh api graphql`, `gh repo clone`). All GitHub communication goes through shell-exec of `gh`. Requires an authenticated `gh` session.
- **`internal/store/`** &mdash; SQLite (pure-Go `modernc.org/sqlite`, no CGO) with WAL mode. Tables: `repos`, `seen_log`, `watchlist`, `api_cache`, `search_history`. Migrations are idempotent (safe to re-run).
- **`internal/model/`** &mdash; `Repo` struct, `SearchParams`, `Preset` definitions, scoring functions (`ComputeScores`, `SortByScore`).
- **`internal/config/`** &mdash; TOML config at `$XDG_CONFIG_HOME/gitvoyager/config.toml`. Falls back to defaults if missing.

### Data flow

1. Search query &rarr; `github.SearchRepos()` (shells out to `gh search repos`) &rarr; JSON parse &rarr; `[]model.Repo`
2. Results upserted into SQLite via `store.UpsertRepos()`
3. Scores computed client-side (`model.ComputeScores`)
4. On repo select &rarr; `github.EnrichRepo()` fetches watchers/commits/topics via GraphQL
5. README fetched separately via `github.FetchReadme()` (REST)
6. Watchlist refresh uses `github.FetchRepoStats()` (batched GraphQL, up to 20 per request)

### TUI styling

Kanagawa/Tokyo Night dual-accent palette (violet + cyan) defined in `tui/styles.go`. `tui/gradient.go` provides ANSI true-color gradient text. All color constants and lipgloss styles live in `styles.go`.

## Key conventions

- XDG paths for all persistent data: config (`$XDG_CONFIG_HOME`), database (`$XDG_DATA_HOME`), cache (`$XDG_CACHE_HOME`)
- Database at `$XDG_DATA_HOME/gitvoyager/gitvoyager.db`
- Store migrations use `addColumnIfNotExists` pattern for safe idempotent ALTER TABLE
- `gh` CLI is a hard runtime dependency (not vendored, not optional)
- Release builds use GoReleaser (`.goreleaser.yaml`), CGO disabled
