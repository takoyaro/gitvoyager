package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/claude"
	"github.com/takoyaro/gitvoyager/internal/config"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/local"
	"github.com/takoyaro/gitvoyager/internal/store"
	"github.com/takoyaro/gitvoyager/internal/taste"
	"github.com/takoyaro/gitvoyager/internal/tui"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Intercept subcommands before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "data" {
		runDataCLI(os.Args[2:])
		return
	}

	var (
		query      string
		versionFlg bool
	)

	flag.StringVar(&query, "query", "", "initial search query")
	flag.StringVar(&query, "q", "", "initial search query (shorthand)")
	flag.BoolVar(&versionFlg, "version", false, "print version")
	flag.BoolVar(&versionFlg, "v", false, "print version (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `GitVoyager — discover GitHub repos from your terminal

Usage:
  gitvoyager [flags] [query...]
  gitvoyager data <command> [flags]

Flags:
  -q, -query string   Initial search query
  -v, -version         Print version and exit

Subcommands:
  data    Query your local discovery database (repos, watchlist, searches, etc.)
          Run 'gitvoyager data --help' for details.

Examples:
  gitvoyager "mcp server"
  gitvoyager -q "bubble tea"
  gitvoyager data repos --table --language Go
  gitvoyager data stats | jq .
`)
	}

	flag.Parse()

	if versionFlg {
		fmt.Printf("gitvoyager %s (%s) built %s\n", version, commit, date)
		os.Exit(0)
	}

	// If positional args, treat as query
	if query == "" && flag.NArg() > 0 {
		for i, arg := range flag.Args() {
			if i > 0 {
				query += " "
			}
			query += arg
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	st, err := store.New(config.DBPath(), &cfg.Exclusions)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	gh := github.NewClient()
	te := taste.New(st)
	cl := claude.New(claude.Config(cfg.Claude), st)

	var ls *local.Scanner
	if cfg.Local.Enabled {
		ls = local.New(cfg.Local.ScanPaths)
	}

	app := tui.NewApp(cfg, st, gh, te, cl, ls, query)

	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
