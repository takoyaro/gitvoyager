package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/claude"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/local"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/readme"
	"github.com/takoyaro/gitvoyager/internal/store"
	"github.com/takoyaro/gitvoyager/internal/taste"
)

func searchReposCmd(client *github.Client, st *store.Store, params model.SearchParams) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		repos, err := client.SearchRepos(ctx, params)
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

func enrichRepoCmd(client *github.Client, st *store.Store, repo model.Repo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		enriched, err := client.EnrichRepo(ctx, repo)
		if err != nil {
			return repoDetailMsg{Repo: repo} // return unenriched on error
		}
		model.ComputeIntrinsicScore(&enriched)

		// Persist enrichment signals to DB for future sessions.
		if st != nil && enriched.Intrinsic != nil {
			if data, err := json.Marshal(enriched.Intrinsic); err == nil {
				_ = st.UpdateEnrichment(enriched.FullName, data)
			}
		}

		return repoDetailMsg{Repo: enriched}
	}
}

func analyzeReadmeCmd(fullName, content string) tea.Cmd {
	return func() tea.Msg {
		sigs := readme.Analyze(content)
		return readmeAnalyzedMsg{FullName: fullName, Score: readme.Score(sigs)}
	}
}

func batchIntrinsicProbeCmd(client *github.Client, repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		signals, topics, err := client.BatchIntrinsicProbeWithTopics(ctx, repos)
		return batchIntrinsicMsg{Signals: signals, Topics: topics, Err: err}
	}
}

func fetchReadmeCmd(client *github.Client, owner, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		content, err := client.FetchReadme(ctx, owner, name)
		if err != nil {
			return readmeErrorMsg{Err: err}
		}
		return readmeMsg{FullName: owner + "/" + name, Content: content}
	}
}

func openBrowserCmd(client *github.Client, repoFullName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.OpenInBrowser(ctx, repoFullName)
		if err != nil {
			return statusMsg{Text: "Failed to open browser: " + err.Error(), IsError: true}
		}
		return statusMsg{Text: "Opened " + repoFullName + " in browser"}
	}
}

func cloneRepoCmd(client *github.Client, repoFullName, dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		err := client.CloneRepo(ctx, repoFullName, dir)
		return cloneFinishedMsg{FullName: repoFullName, Err: err}
	}
}

func fetchRateLimitCmd(client *github.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rl, err := client.FetchRateLimit(ctx)
		if err != nil {
			return rateLimitMsg{} // silent failure
		}
		return rateLimitMsg{RateLimit: rl}
	}
}

func checkGhAuthCmd(client *github.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.CheckAuth(ctx)
		return ghAuthCheckedMsg{Err: err}
	}
}

// --- Fire-and-forget DB write commands (off the main thread) ---

func saveSearchCmd(st *store.Store, query, sort, lang string, count int) tea.Cmd {
	return func() tea.Msg {
		_ = st.SaveSearch(query, sort, lang, count)
		return nil
	}
}

func markSeenCmd(st *store.Store, fullName string) tea.Cmd {
	return func() tea.Msg {
		_ = st.MarkSeen(fullName)
		return nil
	}
}

func asyncUpsertReposCmd(st *store.Store, repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		_ = st.UpsertRepos(repos)
		return nil
	}
}

func updateWatchlistViewedAtCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		_ = st.UpdateWatchlistViewedAt()
		return nil
	}
}

func toggleSearchBookmarkCmd(st *store.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		_ = st.ToggleSearchBookmark(id)
		return nil
	}
}

// likeSearchCmd scans a local project directory off the main thread,
// then returns the constructed search query as a message.
func likeSearchCmd(scanner *local.Scanner, st *store.Store, path string) tea.Cmd {
	return func() tea.Msg {
		if scanner == nil {
			return likeSearchReadyMsg{}
		}
		fp, err := scanner.ScanDirectory(path)
		if err != nil || fp == nil {
			return likeSearchReadyMsg{}
		}
		if st != nil {
			_ = st.SaveProjectFingerprint(*fp)
		}
		var parts []string
		if fp.Language != "" {
			parts = append(parts, "language:"+strings.ToLower(fp.Language))
		}
		parts = append(parts, "stars:>10")
		return likeSearchReadyMsg{Query: strings.Join(parts, " "), Path: path}
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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		stats, err := client.FetchRepoStats(ctx, fullNames)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		summary, err := client.SummarizeReadme(ctx, fullName, readme)
		return claudeSummaryMsg{FullName: fullName, Summary: summary, Err: err}
	}
}

func claudeNLSearchCmd(client *claude.Client, query string) tea.Cmd {
	return func() tea.Msg {
		if client == nil || !client.Available() {
			return claudeNLSearchMsg{Err: fmt.Errorf("claude not available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		params, display, err := client.TranslateToSearch(ctx, query)
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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		repos, err := gh.FetchStarredRepos(ctx, 200)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		suggested, err := client.SuggestAdjacentTopics(ctx, query, topics)
		return topicDriftMsg{Topics: suggested, Err: err}
	}
}

func claudeWhyTrendingCmd(client *claude.Client, repo model.Repo, readme string) tea.Cmd {
	return func() tea.Msg {
		if client == nil || !client.Available() {
			return claudeAnalysisMsg{FullName: repo.FullName, Err: fmt.Errorf("claude not available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		analysis, err := client.WhyTrending(ctx, repo, readme)
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

// Default topics to sample when no user data is available.
var defaultHeatTopics = []string{
	"llm", "mcp", "ai-agent", "claude-code", "anthropic", "openai",
	"coding-agent", "rag", "prompt-engineering", "langchain",
	"mcp-server", "vibe-coding", "rust", "go", "typescript",
	"python", "zig", "wasm", "webgpu", "kubernetes", "docker",
	"game-engine", "security", "cli", "tui", "devtools",
}

// delayedTopicHeatCmd returns existing data immediately or schedules sampling
// after a 10-second delay (via tea.Tick, not time.Sleep) so it never blocks a goroutine.
func delayedTopicHeatCmd(st *store.Store) tea.Cmd {
	if st == nil {
		return func() tea.Msg { return topicHeatSampledMsg{} }
	}
	if !st.NeedsTopicHeatRefresh() {
		return func() tea.Msg {
			hm, _ := st.GetTopicHeatMap()
			return topicHeatSampledMsg{HeatMap: hm}
		}
	}
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return topicHeatDelayMsg{}
	})
}

func sampleTopicHeatCmd(client *github.Client, st *store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return topicHeatSampledMsg{}
		}

		// Gather topics: from user's discovered repos + defaults.
		topics, _ := st.GetTrackedTopics(10)
		seen := make(map[string]bool, len(topics))
		for _, t := range topics {
			seen[t] = true
		}
		for _, t := range defaultHeatTopics {
			if !seen[t] && len(topics) < 25 {
				topics = append(topics, t)
				seen[t] = true
			}
		}

		now := time.Now()
		currentStart := now.AddDate(0, 0, -30).Format("2006-01-02")
		priorStart := now.AddDate(0, 0, -60).Format("2006-01-02")
		priorEnd := now.AddDate(0, 0, -30).Format("2006-01-02")

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		// Sample current 30-day window.
		currentSamples, err := client.SampleTopicCounts(ctx, topics, 30)
		if err != nil {
			return topicHeatSampledMsg{Err: err}
		}

		for _, s := range currentSamples {
			_ = st.RecordTopicHeat(s.Topic, s.Count, currentStart, now.Format("2006-01-02"))
		}

		// Sample prior 30-day window (for comparison).
		priorSamples, err := client.SampleTopicCounts(ctx, topics, 60)
		if err != nil {
			// Still return what we have.
			hm, _ := st.GetTopicHeatMap()
			return topicHeatSampledMsg{HeatMap: hm}
		}

		// Prior window count = 60-day count - 30-day count.
		currentMap := make(map[string]int)
		for _, s := range currentSamples {
			currentMap[s.Topic] = s.Count
		}
		for _, s := range priorSamples {
			priorCount := s.Count - currentMap[s.Topic]
			if priorCount < 0 {
				priorCount = 0
			}
			_ = st.RecordTopicHeat(s.Topic, priorCount, priorStart, priorEnd)
		}

		hm, _ := st.GetTopicHeatMap()
		return topicHeatSampledMsg{HeatMap: hm}
	}
}
