package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/takoyaro/gitvoyager/internal/config"
	"github.com/takoyaro/gitvoyager/internal/store"
)

const dataUsage = `GitVoyager Data CLI — query your discovery database

Usage:
  gitvoyager data <command> [flags]

Commands:
  repos      List all discovered repos
  watchlist  List watchlisted repos
  searches   Show search history
  rising     Fastest-rising repos by star velocity
  taste      Language and topic preferences
  velocity   Star velocity for a specific repo
  stats      Aggregate database statistics
  seen       Repos you've viewed with timestamps
  local      Local project dependencies

Per-command flags:
  --limit N      Max results (default varies)
  --language L   Filter by programming language
  --table        Human-readable table output

Output is JSON by default. Pipe to jq for formatting.

Examples:
  gitvoyager data repos --limit 20 --language Go
  gitvoyager data watchlist --table
  gitvoyager data velocity charmbracelet/bubbletea --days 14
  gitvoyager data stats | jq .
  gitvoyager data searches --bookmarked
`

func runDataCLI(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(dataUsage)
		return
	}

	st, err := store.New(config.DBPath(), nil)
	if err != nil {
		fatalf("open database: %v", err)
	}
	defer st.Close()

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "repos":
		cmdRepos(st, subArgs)
	case "watchlist":
		cmdWatchlist(st, subArgs)
	case "searches":
		cmdSearches(st, subArgs)
	case "rising":
		cmdRising(st, subArgs)
	case "taste":
		cmdTaste(st, subArgs)
	case "velocity":
		cmdVelocity(st, subArgs)
	case "stats":
		cmdStats(st)
	case "seen":
		cmdSeen(st, subArgs)
	case "local":
		cmdLocal(st, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown data command: %s\n\n", subcmd)
		fmt.Print(dataUsage)
		os.Exit(1)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatalf("encode json: %v", err)
	}
}

// --- repos ---

func cmdRepos(st *store.Store, args []string) {
	fs := flag.NewFlagSet("repos", flag.ExitOnError)
	limit := fs.Int("limit", 100, "max results")
	language := fs.String("language", "", "filter by language")
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	repos, err := st.GetRepos(*limit, *language)
	if err != nil {
		fatalf("query repos: %v", err)
	}

	if *table {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "REPO\tLANG\tSTARS\tFORKS\tΔ\tDESCRIPTION")
		for _, r := range repos {
			desc := truncate(r.Description, 55)
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%+d\t%s\n",
				r.FullName, r.Language, r.Stars, r.Forks, r.StarDelta, desc)
		}
		w.Flush()
	} else {
		printJSON(repos)
	}
}

// --- watchlist ---

func cmdWatchlist(st *store.Store, args []string) {
	fs := flag.NewFlagSet("watchlist", flag.ExitOnError)
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	repos, err := st.GetWatchlistRepos()
	if err != nil {
		fatalf("query watchlist: %v", err)
	}

	if *table {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "REPO\tLANG\tSTARS\tΔSTARS\tDESCRIPTION")
		for _, r := range repos {
			desc := truncate(r.Description, 50)
			fmt.Fprintf(w, "%s\t%s\t%d\t%+d\t%s\n",
				r.FullName, r.Language, r.Stars, r.StarDelta, desc)
		}
		w.Flush()
	} else {
		printJSON(repos)
	}
}

// --- searches ---

func cmdSearches(st *store.Store, args []string) {
	fs := flag.NewFlagSet("searches", flag.ExitOnError)
	limit := fs.Int("limit", 50, "max results")
	bookmarked := fs.Bool("bookmarked", false, "only bookmarked searches")
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	var searches []store.SavedSearch
	var err error
	if *bookmarked {
		searches, err = st.BookmarkedSearches()
	} else {
		searches, err = st.RecentSearches(*limit)
	}
	if err != nil {
		fatalf("query searches: %v", err)
	}

	if *table {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tQUERY\tSORT\tLANG\tRESULTS\tDATE\tBM")
		for _, s := range searches {
			bm := ""
			if s.Bookmarked {
				bm = "*"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\t%s\t%s\n",
				s.ID, truncate(s.Query, 40), s.SortField, s.Language, s.ResultCount,
				s.SearchedAt.Format("2006-01-02 15:04"), bm)
		}
		w.Flush()
	} else {
		printJSON(searches)
	}
}

// --- rising ---

func cmdRising(st *store.Store, args []string) {
	fs := flag.NewFlagSet("rising", flag.ExitOnError)
	limit := fs.Int("limit", 20, "max results")
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	repos, err := st.GetFastestRising(*limit)
	if err != nil {
		fatalf("query rising: %v", err)
	}

	if *table {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "REPO\tLANG\tSTARS\tΔSTARS\tDESCRIPTION")
		for _, r := range repos {
			desc := truncate(r.Description, 50)
			fmt.Fprintf(w, "%s\t%s\t%d\t%+d\t%s\n",
				r.FullName, r.Language, r.Stars, r.StarDelta, desc)
		}
		w.Flush()
	} else {
		printJSON(repos)
	}
}

// --- taste ---

func cmdTaste(st *store.Store, args []string) {
	fs := flag.NewFlagSet("taste", flag.ExitOnError)
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	langs, err := st.GetLanguageDistribution()
	if err != nil {
		fatalf("query languages: %v", err)
	}
	topics, err := st.GetTopicDistribution()
	if err != nil {
		fatalf("query topics: %v", err)
	}

	// Include cached taste profile if available
	var profile any
	if snap, ts, err := st.GetLatestTasteSnapshot(); err == nil && snap != "" {
		var parsed any
		if json.Unmarshal([]byte(snap), &parsed) == nil {
			profile = map[string]any{
				"data":        parsed,
				"computed_at": ts,
			}
		}
	}

	if *table {
		fmt.Println("Languages (weighted):")
		printSortedMap(langs)
		fmt.Println("\nTopics (weighted):")
		printSortedMap(topics)
	} else {
		result := map[string]any{
			"languages": langs,
			"topics":    topics,
		}
		if profile != nil {
			result["taste_profile"] = profile
		}
		printJSON(result)
	}
}

// --- velocity ---

func cmdVelocity(st *store.Store, args []string) {
	fs := flag.NewFlagSet("velocity", flag.ExitOnError)
	days := fs.Int("days", 7, "window in days")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fatalf("usage: gitvoyager data velocity <owner/repo> [--days N]")
	}
	repo := fs.Arg(0)

	vel, err := st.GetStarVelocity(repo, *days)
	if err != nil {
		fatalf("no velocity data for %s: %v", repo, err)
	}

	printJSON(map[string]any{
		"repo":          repo,
		"window_days":   *days,
		"stars_per_day": vel,
	})
}

// --- stats ---

func cmdStats(st *store.Store) {
	stats, err := st.GetStats()
	if err != nil {
		fatalf("query stats: %v", err)
	}

	langs, _ := st.GetLanguageDistribution()
	topics, _ := st.GetTopicDistribution()

	result := map[string]any{
		"counts":         stats,
		"top_languages":  topN(langs, 15),
		"top_topics":     topN(topics, 15),
	}
	printJSON(result)
}

// --- seen ---

func cmdSeen(st *store.Store, args []string) {
	fs := flag.NewFlagSet("seen", flag.ExitOnError)
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	seen, err := st.GetSeenRepos()
	if err != nil {
		fatalf("query seen: %v", err)
	}

	type seenEntry struct {
		Repo   string    `json:"repo"`
		SeenAt time.Time `json:"seen_at"`
	}

	entries := make([]seenEntry, 0, len(seen))
	for name, t := range seen {
		entries = append(entries, seenEntry{name, t})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SeenAt.After(entries[j].SeenAt)
	})

	if *table {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "REPO\tLAST SEEN")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\n", e.Repo, e.SeenAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
	} else {
		printJSON(entries)
	}
}

// --- local ---

func cmdLocal(st *store.Store, args []string) {
	fs := flag.NewFlagSet("local", flag.ExitOnError)
	table := fs.Bool("table", false, "table output")
	fs.Parse(args)

	deps, err := st.GetLocalDependencies()
	if err != nil {
		fatalf("query local deps: %v", err)
	}
	projLangs, _ := st.GetProjectLanguages()

	if *table {
		fmt.Printf("Local projects: %d\n\n", st.GetLocalProjectCount())
		if len(projLangs) > 0 {
			fmt.Println("Project languages:")
			printSortedMap(projLangs)
			fmt.Println()
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DEPENDENCY\tVERSION\tSOURCE\tGITHUB")
		for _, d := range deps {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", d.Name, d.Version, d.Source, d.RepoName)
		}
		w.Flush()
	} else {
		printJSON(map[string]any{
			"project_count":     st.GetLocalProjectCount(),
			"project_languages": projLangs,
			"dependencies":      deps,
		})
	}
}

// --- helpers ---

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

type kv struct {
	Key   string
	Value int
}

func printSortedMap(m map[string]int) {
	sorted := make([]kv, 0, len(m))
	for k, v := range m {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })
	for _, e := range sorted {
		fmt.Printf("  %-20s %d\n", e.Key, e.Value)
	}
}

func topN(m map[string]int, n int) map[string]int {
	if len(m) <= n {
		return m
	}
	sorted := make([]kv, 0, len(m))
	for k, v := range m {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })
	result := make(map[string]int, n)
	for i := 0; i < n && i < len(sorted); i++ {
		result[sorted[i].Key] = sorted[i].Value
	}
	return result
}
