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

Data CLI: `gitvoyager data <command>` exposes the discovery database. Commands: `repos`, `watchlist`, `searches`, `rising`, `taste`, `velocity`, `stats`, `seen`, `local`, `hot-topics`. Each supports `--table` for human-readable output (default: JSON).

## Architecture

Bubble Tea (Elm-architecture) TUI that discovers GitHub repos via the `gh` CLI (not direct API).

**Dependency flow:** `main` &rarr; `config`, `github`, `store`, `tui`. The `tui` package imports all others; no other cross-package imports exist within `internal/`.

### Packages

- **`internal/tui/`** (~3k LOC) &mdash; Bubble Tea app with state machine (`stateSearchPrompt` &rarr; `stateBrowsing` &rarr; `stateWatchlist` &rarr; `stateReturnVisit`). Three focus panes (list, detail, search) plus peek overlay and exclusion manager. `app.go` is the central Update/View hub; `list.go`, `detail.go`, `statusbar.go` are sub-models composed inside it. Responsive single-pane layout activates on narrow terminals. Home screen uses a discovery-first zone layout: signal board &rarr; surprise pick &rarr; return visit &rarr; search &rarr; preset cards &rarr; expedition pills.
- **`internal/github/`** &mdash; Wraps `gh` CLI commands (`gh search repos`, `gh api graphql`, `gh repo clone`). All GitHub communication goes through shell-exec of `gh`. Requires an authenticated `gh` session. `topicheat.go` samples topic creation counts for acceleration analysis.
- **`internal/store/`** &mdash; SQLite (pure-Go `modernc.org/sqlite`, no CGO) with WAL mode. Tables: `repos`, `seen_log`, `watchlist`, `api_cache`, `search_history`, `exclusions`, `topic_heat_snapshots`. `exclusions.go` manages keyword/topic/owner block rules; `query.go` provides aggregate stats and paginated repo queries; `topicheat.go` persists topic heat snapshots and computes acceleration ratios. Migrations are idempotent (safe to re-run).
- **`internal/model/`** &mdash; `Repo` struct (includes `IntrinsicSignals`, `IntrinsicScore`, `ReadmeScore`, `TopicHeatBoost`), `SearchParams`, `Preset` definitions, scoring functions (`ComputeScores`, `ComputeIntrinsicScore`, `SortByScore`).
- **`internal/readme/`** &mdash; Markdown quality analysis via goldmark AST walking. Extracts structural signals (headings, code blocks, badges, demo assets, section detection) and produces a 0&ndash;10 quality score.
- **`internal/config/`** &mdash; TOML config at `$XDG_CONFIG_HOME/gitvoyager/config.toml`. Falls back to defaults if missing. Includes `[exclusions]` section for seeding keyword/topic/owner block lists.

### Data flow

1. Search query &rarr; `github.SearchRepos()` (shells out to `gh search repos`) &rarr; JSON parse &rarr; `[]model.Repo`
2. Results upserted into SQLite via `store.UpsertRepos()`
3. Topic heat boosts applied (`model.ApplyTopicHeatBoosts`), then scores computed client-side (`model.ComputeScores`)
4. Batch intrinsic probe fires (`github.BatchIntrinsicProbeWithTopics`, 15 repos/request) &rarr; quality grades appear in list
5. On repo select &rarr; `github.EnrichRepo()` fetches watchers/commits/topics/structure via GraphQL &rarr; `ComputeIntrinsicScore()` &rarr; persisted via `store.UpdateEnrichment()`
6. README fetched separately via `github.FetchReadme()` (REST) &rarr; analyzed by `readme.Analyze()` for quality score
7. Watchlist refresh uses `github.FetchRepoStats()` (batched GraphQL, up to 20 per request)
8. Topic heat sampled on startup (10s delay) via `github.SampleTopicCounts()` &rarr; stored in `topic_heat_snapshots` &rarr; acceleration ratios fuel Signal Board and Hot Space preset

### TUI styling

Kanagawa/Tokyo Night dual-accent palette (cyan primary + violet secondary) defined in `tui/styles.go`. `tui/gradient.go` provides ANSI true-color gradient text. All color constants and lipgloss styles live in `styles.go`. Micro-animations (focus pulse, shimmer hold, compare reveal) run via background tickers.

## Key conventions

- XDG paths for all persistent data: config (`$XDG_CONFIG_HOME`), database (`$XDG_DATA_HOME`), cache (`$XDG_CACHE_HOME`)
- Database at `$XDG_DATA_HOME/gitvoyager/gitvoyager.db`
- Store migrations use `addColumnIfNotExists` pattern for safe idempotent ALTER TABLE
- `gh` CLI is a hard runtime dependency (not vendored, not optional)
- Release builds use GoReleaser (`.goreleaser.yaml`), CGO disabled
