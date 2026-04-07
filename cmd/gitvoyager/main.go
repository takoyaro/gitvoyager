package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/config"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/store"
	"github.com/takoyaro/gitvoyager/internal/tui"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		query      string
		versionFlg bool
	)

	flag.StringVar(&query, "query", "", "initial search query")
	flag.StringVar(&query, "q", "", "initial search query (shorthand)")
	flag.BoolVar(&versionFlg, "version", false, "print version")
	flag.BoolVar(&versionFlg, "v", false, "print version (shorthand)")
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

	st, err := store.New(config.DBPath())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	gh := github.NewClient()

	app := tui.NewApp(cfg, st, gh, query)

	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
