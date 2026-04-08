package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/claude"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/local"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
	"github.com/takoyaro/gitvoyager/internal/taste"
)

func searchReposCmd(client *github.Client, st *store.Store, params model.SearchParams) tea.Cmd {
	return func() tea.Msg {
		repos, err := client.SearchRepos(context.Background(), params)
		if err != nil {
			return searchErrorMsg{Err: err}
		}
		var knownRepos map[string]bool
		if st != nil {
			// Detect new discoveries BEFORE upsert (critical ordering)
			names := make([]string, len(repos))
			for i, r := range repos {
				names[i] = r.FullName
			}
			knownRepos, _ = st.CheckExistingRepos(names)

			_ = st.UpsertRepos(repos)
			_ = st.RecordStarSnapshots(repos)
		}
		return searchResultsMsg{Repos: repos, Query: params.Query, KnownRepos: knownRepos}
	}
}

func enrichRepoCmd(client *github.Client, repo model.Repo) tea.Cmd {
	return func() tea.Msg {
		enriched, err := client.EnrichRepo(context.Background(), repo)
		if err != nil {
			return repoDetailMsg{Repo: repo} // return unenriched on error
		}
		return repoDetailMsg{Repo: enriched}
	}
}

func fetchReadmeCmd(client *github.Client, owner, name string) tea.Cmd {
	return func() tea.Msg {
		content, err := client.FetchReadme(context.Background(), owner, name)
		if err != nil {
			return readmeErrorMsg{Err: err}
		}
		return readmeMsg{FullName: owner + "/" + name, Content: content}
	}
}

func openBrowserCmd(client *github.Client, repoFullName string) tea.Cmd {
	return func() tea.Msg {
		err := client.OpenInBrowser(context.Background(), repoFullName)
		if err != nil {
			return statusMsg{Text: "Failed to open browser: " + err.Error(), IsError: true}
		}
		return statusMsg{Text: "Opened " + repoFullName + " in browser"}
	}
}

func cloneRepoCmd(client *github.Client, repoFullName, dir string) tea.Cmd {
	return func() tea.Msg {
		err := client.CloneRepo(context.Background(), repoFullName, dir)
		return cloneFinishedMsg{FullName: repoFullName, Err: err}
	}
}

func fetchRateLimitCmd(client *github.Client) tea.Cmd {
	return func() tea.Msg {
		rl, err := client.FetchRateLimit(context.Background())
		if err != nil {
			return rateLimitMsg{} // silent failure
		}
		return rateLimitMsg{RateLimit: rl}
	}
}

func checkGhAuthCmd(client *github.Client) tea.Cmd {
	return func() tea.Msg {
		err := client.CheckAuth(context.Background())
		return ghAuthCheckedMsg{Err: err}
	}
}

func debounceDetailCmd(tag int, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return detailDebounceMsg{Tag: tag}
	})
}

func loadWatchlistCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return watchlistLoadedMsg{}
		}
		watched, _ := st.GetWatchlist()
		if watched == nil {
			watched = make(map[string]bool)
		}
		return watchlistLoadedMsg{WatchSet: watched}
	}
}

func toggleWatchCmd(st *store.Store, fullName string) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return watchToggledMsg{FullName: fullName, Watched: false}
		}
		watched, err := st.ToggleWatch(fullName)
		if err != nil {
			return statusMsg{Text: "Watch failed: " + err.Error(), IsError: true}
		}
		return watchToggledMsg{FullName: fullName, Watched: watched}
	}
}

func loadWatchlistReposCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return watchlistReposMsg{}
		}
		repos, err := st.GetWatchlistRepos()
		if err != nil {
			return watchlistReposMsg{}
		}
		return watchlistReposMsg{Repos: repos}
	}
}

func refreshWatchlistCmd(client *github.Client, fullNames []string) tea.Cmd {
	return func() tea.Msg {
		stats, err := client.FetchRepoStats(context.Background(), fullNames)
		if err != nil {
			return statusMsg{Text: "Watchlist refresh failed: " + err.Error(), IsError: true}
		}
		return watchlistRefreshedMsg{Stats: stats}
	}
}

func loadStarDeltasCmd(st *store.Store, repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		if st == nil || len(repos) == 0 {
			return starDeltasLoadedMsg{}
		}
		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = r.FullName
		}
		firstSeen, err := st.GetFirstSeenStars(names)
		if err != nil {
			return starDeltasLoadedMsg{}
		}
		// Batch-load acceleration in the same round-trip
		accel, _ := st.GetBatchStarAcceleration(names)
		return starDeltasLoadedMsg{FirstSeenStars: firstSeen, Acceleration: accel}
	}
}

func yankToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		// Try clipboard commands in order: wl-copy (Wayland), xclip (X11), xsel
		cmds := []struct {
			name string
			args []string
		}{
			{"wl-copy", nil},
			{"xclip", []string{"-selection", "clipboard"}},
			{"xsel", []string{"--clipboard", "--input"}},
		}
		for _, c := range cmds {
			path, err := exec.LookPath(c.name)
			if err != nil {
				continue
			}
			cmd := exec.Command(path, c.args...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return statusMsg{Text: "yanked: " + text}
			}
		}
		return statusMsg{Text: "no clipboard tool found (install wl-copy or xclip)", IsError: true}
	}
}

func checkReturnVisitCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return returnVisitMsg{}
		}
		repos, err := st.GetWatchlistRepos()
		if err != nil || len(repos) == 0 {
			return returnVisitMsg{}
		}
		// Filter to repos with positive star delta
		var growing []model.Repo
		for _, r := range repos {
			if r.StarDelta > 0 {
				growing = append(growing, r)
			}
		}
		return returnVisitMsg{Repos: growing}
	}
}

func quitTimerCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return quitTimerMsg{}
	})
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func rateLimitTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return rateLimitTickMsg{}
	})
}

func loadSearchHistoryCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return searchHistoryMsg{}
		}
		recent, _ := st.RecentSearches(10)
		bookmarked, _ := st.BookmarkedSearches()

		toItems := func(searches []store.SavedSearch) []searchHistoryItem {
			items := make([]searchHistoryItem, len(searches))
			for i, s := range searches {
				items[i] = searchHistoryItem{
					ID:          s.ID,
					Query:       s.Query,
					SortField:   s.SortField,
					ResultCount: s.ResultCount,
					Bookmarked:  s.Bookmarked,
					SearchedAt:  relativeTime(s.SearchedAt),
				}
			}
			return items
		}

		return searchHistoryMsg{
			Recent:     toItems(recent),
			Bookmarked: toItems(bookmarked),
		}
	}
}

func computeTasteCmd(engine *taste.Engine) tea.Cmd {
	return func() tea.Msg {
		if engine == nil {
			return tasteProfileMsg{}
		}
		// Try cached first
		if cached, ok := engine.LoadCachedProfile(); ok {
			return tasteProfileMsg{Profile: cached}
		}
		profile, _ := engine.ComputeProfile()
		return tasteProfileMsg{Profile: profile}
	}
}

func surprisePickCmd(engine *taste.Engine, profile taste.Profile) tea.Cmd {
	return func() tea.Msg {
		if engine == nil || profile.Empty() {
			return surprisePickMsg{}
		}
		repo, _ := engine.SurprisePick(profile)
		return surprisePickMsg{Repo: repo}
	}
}

func localScanCmd(scanner *local.Scanner, st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if scanner == nil {
			return localScanMsg{Err: fmt.Errorf("local scanning not configured")}
		}
		projects, err := scanner.Scan()
		if err != nil {
			return localScanMsg{Err: err}
		}
		depCount := 0
		for _, fp := range projects {
			if st != nil {
				_ = st.SaveProjectFingerprint(fp)
			}
			depCount += len(fp.Dependencies)
		}
		return localScanMsg{ProjectCount: len(projects), DepCount: depCount}
	}
}

func claudeSummarizeCmd(client *claude.Client, fullName, readme string) tea.Cmd {
	return func() tea.Msg {
		if client == nil || !client.Available() {
			return claudeSummaryMsg{FullName: fullName, Err: fmt.Errorf("claude not available")}
		}
		summary, err := client.SummarizeReadme(context.Background(), fullName, readme)
		return claudeSummaryMsg{FullName: fullName, Summary: summary, Err: err}
	}
}

func claudeNLSearchCmd(client *claude.Client, query string) tea.Cmd {
	return func() tea.Msg {
		if client == nil || !client.Available() {
			return claudeNLSearchMsg{Err: fmt.Errorf("claude not available")}
		}
		params, display, err := client.TranslateToSearch(context.Background(), query)
		return claudeNLSearchMsg{Params: params, Display: display, Err: err}
	}
}

func syncStarredReposCmd(gh *github.Client, st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if gh == nil || st == nil {
			return starredReposSyncedMsg{}
		}
		// Check if we synced recently (within 24h)
		if _, ok := st.GetCached("starred:last_sync"); ok {
			return starredReposSyncedMsg{}
		}
		repos, err := gh.FetchStarredRepos(context.Background(), 200)
		if err != nil {
			return starredReposSyncedMsg{Err: err}
		}
		// Mark sync timestamp (24h TTL)
		_ = st.SetCached("starred:last_sync", []byte("1"), 24*time.Hour, "")
		return starredReposSyncedMsg{Count: len(repos)}
	}
}

func topicDriftCmd(client *claude.Client, query string, topics []string) tea.Cmd {
	return func() tea.Msg {
		if client == nil || !client.Available() {
			return topicDriftMsg{}
		}
		suggested, err := client.SuggestAdjacentTopics(context.Background(), query, topics)
		return topicDriftMsg{Topics: suggested, Err: err}
	}
}

func claudeWhyTrendingCmd(client *claude.Client, repo model.Repo, readme string) tea.Cmd {
	return func() tea.Msg {
		if client == nil || !client.Available() {
			return claudeAnalysisMsg{FullName: repo.FullName, Err: fmt.Errorf("claude not available")}
		}
		analysis, err := client.WhyTrending(context.Background(), repo, readme)
		return claudeAnalysisMsg{FullName: repo.FullName, Analysis: analysis, Err: err}
	}
}

func loadExclusionsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return exclusionsLoadedMsg{}
		}
		set, _ := st.GetAllExclusions()
		return exclusionsLoadedMsg{Set: set}
	}
}

func addExclusionCmd(st *store.Store, kind, value string) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return exclusionUpdatedMsg{}
		}
		_ = st.AddExclusion(kind, value)
		set, _ := st.GetAllExclusions()
		return exclusionUpdatedMsg{Set: set, Kind: kind, Value: value, Added: true}
	}
}

func removeExclusionCmd(st *store.Store, kind, value string) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return exclusionUpdatedMsg{}
		}
		_ = st.RemoveExclusion(kind, value)
		set, _ := st.GetAllExclusions()
		return exclusionUpdatedMsg{Set: set, Kind: kind, Value: value, Added: false}
	}
}

func firstLaunchTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return firstLaunchTickMsg{}
	})
}

func peekRevealTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return peekRevealTickMsg{}
	})
}

func uiAnimTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return uiAnimTickMsg{}
	})
}

func shimmerHoldCmd() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg {
		return shimmerHoldMsg{}
	})
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
