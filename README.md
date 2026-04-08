<div align="center">

# gitvoyager

**Discover GitHub's hidden gems without leaving your terminal.**

A keyboard-driven TUI that finds underdog repos, tracks rising stars,
and learns what you like — powered by `gh` CLI and local SQLite.

[Install](#install) · [Usage](#usage) · [Keys](#keys) · [Config](#config)

</div>

---

## Why

GitHub search is noisy. Trending lists reward fame over signal.
GitVoyager flips the model: multi-signal scoring surfaces repos that are
*active, young, and under-appreciated* — the projects you'd find if you had
infinite time to browse.

- **5 discovery presets** — Trending, Underdogs, Fresh Signal, Hidden Gems, Rising Stars
- **Watchlist with star velocity** — track repos over time, see who's growing
- **AI-powered summaries** — optional Claude integration for README digests and trend analysis
- **Taste profile** — learns your preferred languages and topics from usage
- **Zero browser required** — search, read READMEs, clone, compare — all in the terminal

## Install

Requires **Go 1.25+** and an authenticated **[`gh` CLI](https://cli.github.com)**.
Optional: **[`claude` CLI](https://docs.anthropic.com/en/docs/claude-code)** for AI features (auto-detected on startup).

```bash
git clone https://github.com/takoyaro/gitvoyager.git
cd gitvoyager
make install
```

Binary lands in `$GOPATH/bin` — make sure that's on your `PATH`.

## Usage

```bash
gitvoyager                    # interactive search prompt
gitvoyager -q "mcp server"   # jump straight into results
gitvoyager rust web framework # positional args work too
gitvoyager data stats         # aggregate discovery stats (JSON)
gitvoyager data repos --table # browse stored repos in a table
```

On first launch you'll land on the search prompt. Type a query or press
<kbd>1</kbd>–<kbd>5</kbd> to fire a preset. Press <kbd>S</kbd> for a surprise
pick based on your taste profile.

## Keys

### Browsing

| Key | Action | | Key | Action |
|-----|--------|-|-----|--------|
| <kbd>j</kbd> / <kbd>k</kbd> | Navigate | | <kbd>o</kbd> | Open in browser |
| <kbd>Enter</kbd> / <kbd>l</kbd> | Select | | <kbd>c</kbd> | Clone repo |
| <kbd>h</kbd> / <kbd>Esc</kbd> | Back | | <kbd>w</kbd> | Watch / unwatch |
| <kbd>Tab</kbd> | Switch pane | | <kbd>W</kbd> | Open watchlist |
| <kbd>Space</kbd> | Peek overlay | | <kbd>s</kbd> | Cycle sort mode |
| <kbd>/</kbd> | Search | | <kbd>f</kbd> | Filter results |
| <kbd>a</kbd> | Advanced search | | <kbd>x</kbd> | Exclude repo |
| <kbd>y</kbd> / <kbd>Y</kbd> | Yank URL / clone cmd | | <kbd>C</kbd> | Compare repos |
| <kbd>X</kbd> | Exclusion manager | | <kbd>&larr;</kbd> / <kbd>&rarr;</kbd> | Cycle language filter |
| <kbd>?</kbd> | Help | | <kbd>q</kbd> | Quit |

### AI (requires [`claude` CLI](https://docs.anthropic.com/en/docs/claude-code))

| Key | Action |
|-----|--------|
| <kbd>A</kbd> | Summarize README |
| <kbd>n</kbd> | Natural language search |
| <kbd>t</kbd> | "Why is this trending?" |

## Scoring

Every result gets three scores computed client-side:

| Score | What it rewards |
|-------|-----------------|
| **Discovery** | Stars + forks + issues + recency, normalized by repo age — young active repos rank higher |
| **Underdog** | Fork-to-star and issue-to-star ratio — finds repos with disproportionate community engagement |
| **Freshness** | Quality signals (description, license, language) + push recency + youth — gates the Fresh Signal preset |

## Config

Optional. Defaults are sane. Lives at `$XDG_CONFIG_HOME/gitvoyager/config.toml`.

```toml
[search]
default_limit = 30
default_sort  = "stars"

[clone]
default_directory = ""       # empty = cwd
protocol          = "ssh"    # or "https"

[claude]
enabled = true
model   = "haiku"            # haiku | sonnet | opus

[exclusions]
keywords = ["awesome-list"]  # hide repos matching name/description
topics   = ["hacktoberfest"] # -topic: qualifiers in GitHub search
owners   = []                # -user: qualifiers in GitHub search

[local]
enabled    = false
scan_paths = ["~/Projects"]  # detect deps → map to GitHub repos
```

See [`internal/config/config.go`](internal/config/config.go) for all options.

## Data

All data follows XDG conventions:

| Path | Contents |
|------|----------|
| `$XDG_CONFIG_HOME/gitvoyager/` | `config.toml` |
| `$XDG_DATA_HOME/gitvoyager/` | `gitvoyager.db` (SQLite, WAL mode) |

No telemetry. Network calls go through `gh` CLI and, when AI features are enabled, the `claude` CLI.

## Building

```bash
make build      # → bin/gitvoyager
make test       # go test ./... -v -race
make clean      # rm -rf bin/
```

Or with [Task](https://taskfile.dev): `task build`, `task test`, etc.

## License

[MIT](LICENSE)
