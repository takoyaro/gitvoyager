package tui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/claude"
	"github.com/takoyaro/gitvoyager/internal/config"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/local"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
	"github.com/takoyaro/gitvoyager/internal/taste"
)

type pane int

const (
	paneList pane = iota
	paneDetail
	paneSearch
)

type appState int

const (
	stateSearchPrompt appState = iota
	stateBrowsing
	stateWatchlist
	stateReturnVisit
	stateQuitting
)

type appModel struct {
	// Components
	list        listModel
	detail      detailModel
	searchBar   searchBarModel
	statusBar   statusBarModel
	helpView    helpModel
	filterInput textinput.Model
	searchForm  searchFormModel

	// Overlays
	peek    peekModel
	compare compareModel

	// State
	repos      []model.Repo
	focus      pane
	state      appState
	loading    bool
	detailTag  int
	width      int
	height     int
	singlePane bool // true when terminal too narrow for dual-pane
	authOk     bool
	filterMode bool
	formMode   bool

	// Search state
	searchParams  model.SearchParams
	searchHistory []searchHistoryItem
	historyCursor int

	// Watchlist screen
	watchlistRepos  []model.Repo
	watchlistList   listModel
	watchlistLoading bool

	// First-launch animation
	firstLaunch      bool
	firstLaunchFrame int

	// Micro-animation state
	focusAnimFrame int // counts down from 3 to 0 on Tab press (3 frames at 80ms = 240ms)

	// Skeleton crossfade
	shimmerHold bool

	// Local sets
	watchSet     map[string]bool
	excludeSet   map[string]bool      // session-ephemeral per-repo exclusions
	exclusionSet *store.ExclusionSet  // persistent global exclusions (from SQLite)

	// Exclude picker (inline status bar mode)
	excludePickerMode  bool
	excludePickerItems []excludePickerItem
	excludePickerKW    textinput.Model // keyword input when "keyword..." selected

	// Exclusion manager overlay
	exclusionMgr exclusionManagerModel

	// Session tracking
	sessionViewed  int
	sessionWatched int
	sessionCloned  int

	// Return visit
	returnVisitRepos []model.Repo

	// Taste profile
	tasteEngine      *taste.Engine
	tasteProfile     taste.Profile
	surprisePick     *model.Repo
	langCycleOptions []string // ["", "Python", "Go", ...] — "" = All
	langCycleIdx     int

	// Local intelligence
	localScanner *local.Scanner

	// Claude AI
	claude           *claude.Client
	claudeSummaries  map[string]string // fullName -> summary (in-memory cache)
	claudeAnalyses   map[string]string // fullName -> analysis
	nlSearchMode     bool
	summarizing      bool
	suggestedTopics  []string // topic drift suggestions

	// Dependencies
	cfg    *config.Config
	store  *store.Store
	github *github.Client
}

func NewApp(cfg *config.Config, st *store.Store, gh *github.Client, te *taste.Engine, cl *claude.Client, ls *local.Scanner, initialQuery string) *appModel {
	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.Prompt = "  filter: "

	m := &appModel{
		list:         newListModel(),
		detail:       newDetailModel(),
		searchBar:    newSearchBar(),
		statusBar:    newStatusBar(),
		helpView:     newHelpModel(),
		filterInput:  fi,
		searchForm:   newSearchForm(),
		watchlistList: newListModel(),
		searchParams: model.DefaultSearchParams(),
		watchSet:     make(map[string]bool),
		excludeSet:      make(map[string]bool),
		exclusionMgr:    newExclusionManager(),
		tasteEngine:     te,
		localScanner:    ls,
		claude:          cl,
		claudeSummaries: make(map[string]string),
		claudeAnalyses:  make(map[string]string),
		cfg:             cfg,
		store:           st,
		github:          gh,
	}

	if cfg != nil {
		m.searchParams.Limit = cfg.Search.DefaultLimit
		m.searchParams.Sort = model.SortField(cfg.Search.DefaultSort)
	}

	if initialQuery != "" {
		m.searchParams.Query = initialQuery
		m.state = stateBrowsing
	} else {
		m.state = stateSearchPrompt
		m.focus = paneSearch
		m.searchBar.Focus()
	}

	return m
}

func (m *appModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		func() tea.Msg { return tea.RequestWindowSize() },
		checkGhAuthCmd(m.github),
		fetchRateLimitCmd(m.github),
		m.searchBar.input.Focus(),
		loadSearchHistoryCmd(m.store),
		loadWatchlistCmd(m.store),
		loadExclusionsCmd(m.store),
		rateLimitTickCmd(),
		checkReturnVisitCmd(m.store),
		computeTasteCmd(m.tasteEngine),
	}

	// Auto-scan local projects if configured
	if m.localScanner != nil && m.cfg != nil && m.cfg.Local.Enabled && m.cfg.Local.AutoScan {
		cmds = append(cmds, localScanCmd(m.localScanner, m.store))
	}

	// Sync starred repos (debounced, max once per 24h)
	cmds = append(cmds, syncStarredReposCmd(m.github, m.store))

	if m.searchParams.Query != "" {
		m.loading = true
		m.list.SetLoading(true)
		m.statusBar.SetLoading(true)
		cmds = append(cmds, searchReposCmd(m.github, m.store, m.searchParams), spinnerTickCmd())
	} else if m.store != nil {
		// First launch: if no search history, show animated carousel
		recent, _ := m.store.RecentSearches(1)
		if len(recent) == 0 {
			m.firstLaunch = true
			cmds = append(cmds, firstLaunchTickCmd())
		}
	}

	if m.store != nil {
		if seen, err := m.store.GetSeenRepos(); err == nil {
			seenSet := make(map[string]bool)
			for k := range seen {
				seenSet[k] = true
			}
			m.list.SetSeen(seenSet)
		}
	}

	return tea.Batch(cmds...)
}

func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case tea.KeyPressMsg:
		m.firstLaunch = false // stop carousel on any keypress
		return m.handleKey(msg)

	case ghAuthCheckedMsg:
		if msg.Err != nil {
			m.statusBar.SetMessage("gh auth failed - run: gh auth login", true)
		} else {
			m.authOk = true
		}
		return m, nil

	case searchResultsMsg:
		m.repos = msg.Repos
		model.ComputeScores(m.repos)

		// Mark new discoveries (repos not in DB before this search)
		if msg.KnownRepos != nil {
			for i := range m.repos {
				m.repos[i].NewDiscovery = !msg.KnownRepos[m.repos[i].FullName]
			}
		}

		for i := range m.repos {
			m.repos[i].Watchlisted = m.watchSet[m.repos[i].FullName]
		}

		// Apply discovery sort as default view
		if m.searchParams.Sort == model.SortDiscovery {
			model.SortByDiscovery(m.repos, m.list.seenSet)
		}

		m.loading = false
		m.statusBar.SetLoading(false)
		// Hold skeleton visible briefly for crossfade effect
		m.shimmerHold = true
		m.list.shimmerHold = true
		m.list.SetRepos(m.repos)
		m.list.SetWatched(m.watchSet)
		m.searchBar.SetQuery(msg.Query)
		m.searchBar.SetCount(len(m.repos))
		m.statusBar.SetCounts(len(m.repos), m.list.Len())
		m.state = stateBrowsing
		m.focus = paneList
		if m.store != nil && msg.Query != "" {
			_ = m.store.SaveSearch(msg.Query, string(m.searchParams.Sort), m.searchParams.Language, len(m.repos))
		}
		var cmds []tea.Cmd
		cmds = append(cmds, shimmerHoldCmd())
		if sel := m.list.Selected(); sel != nil {
			m.detailTag++
			cmds = append(cmds, debounceDetailCmd(m.detailTag, 100*time.Millisecond))
		}
		cmds = append(cmds, loadStarDeltasCmd(m.store, m.repos))
		// Clear topic drift for new search
		m.suggestedTopics = nil
		return m, tea.Batch(cmds...)

	case starDeltasLoadedMsg:
		changed := false
		if len(msg.FirstSeenStars) > 0 {
			for i := range m.repos {
				if first, ok := msg.FirstSeenStars[m.repos[i].FullName]; ok {
					m.repos[i].StarDelta = m.repos[i].Stars - first
					changed = true
				}
			}
		}
		if len(msg.Acceleration) > 0 {
			for i := range m.repos {
				if accel, ok := msg.Acceleration[m.repos[i].FullName]; ok {
					m.repos[i].StarAccel = accel
					changed = true
				}
			}
		}
		if changed {
			// Re-sort with acceleration data now available
			if m.searchParams.Sort == model.SortDiscovery {
				// Preserve selected repo across re-sort
				var selectedName string
				if sel := m.list.Selected(); sel != nil {
					selectedName = sel.FullName
				}
				model.SortByDiscovery(m.repos, m.list.seenSet)
				m.list.SetRepos(m.repos)
				m.list.SetWatched(m.watchSet)
				// Restore cursor to previously selected repo
				if selectedName != "" {
					for i, idx := range m.list.filtered {
						if m.list.repos[idx].FullName == selectedName {
							m.list.cursor = i
							break
						}
					}
				}
			} else {
				m.list.SetRepos(m.repos)
				m.list.SetWatched(m.watchSet)
			}
		}
		return m, nil

	case searchErrorMsg:
		m.loading = false
		m.statusBar.SetMessage("Search failed: "+msg.Err.Error(), true)
		return m, clearStatusAfter(5 * time.Second)

	case detailDebounceMsg:
		if msg.Tag != m.detailTag {
			return m, nil
		}
		sel := m.activeList().Selected()
		if sel == nil {
			return m, nil
		}
		m.detail.SetRepo(sel)
		// Restore cached AI content for this repo
		if summary, ok := m.claudeSummaries[sel.FullName]; ok {
			m.detail.SetAISummary(summary)
		}
		if analysis, ok := m.claudeAnalyses[sel.FullName]; ok {
			m.detail.SetAIAnalysis(analysis)
		}
		m.sessionViewed++
		if m.store != nil {
			_ = m.store.MarkSeen(sel.FullName)
			m.list.seenSet[sel.FullName] = true
		}
		return m, tea.Batch(
			enrichRepoCmd(m.github, *sel),
			fetchReadmeCmd(m.github, sel.Owner, sel.Name),
		)

	case repoDetailMsg:
		var cmds []tea.Cmd
		for i, r := range m.repos {
			if r.FullName == msg.Repo.FullName {
				m.repos[i].Topics = msg.Repo.Topics
				m.repos[i].Enriched = msg.Repo.Enriched
				if sel := m.list.Selected(); sel != nil && sel.FullName == msg.Repo.FullName {
					m.detail.UpdateRepo(&m.repos[i])
				}
				break
			}
		}
		for i, r := range m.watchlistRepos {
			if r.FullName == msg.Repo.FullName {
				m.watchlistRepos[i].Topics = msg.Repo.Topics
				m.watchlistRepos[i].Enriched = msg.Repo.Enriched
				if sel := m.watchlistList.Selected(); sel != nil && sel.FullName == msg.Repo.FullName {
					m.detail.UpdateRepo(&m.watchlistRepos[i])
				}
				break
			}
		}
		// Trigger topic drift once per search session after first enrichment with topics
		if len(m.suggestedTopics) == 0 && len(msg.Repo.Topics) > 0 && m.claude != nil && m.claude.Available() {
			var allTopics []string
			for _, r := range m.repos {
				allTopics = append(allTopics, r.Topics...)
			}
			if len(allTopics) >= 3 {
				cmds = append(cmds, topicDriftCmd(m.claude, m.searchParams.Query, allTopics))
			}
		}
		return m, tea.Batch(cmds...)

	case readmeMsg:
		if sel := m.activeList().Selected(); sel != nil && sel.FullName == msg.FullName {
			for i, r := range m.repos {
				if r.FullName == msg.FullName {
					m.repos[i].ReadmeContent = msg.Content
					break
				}
			}
			for i, r := range m.watchlistRepos {
				if r.FullName == msg.FullName {
					m.watchlistRepos[i].ReadmeContent = msg.Content
					break
				}
			}
			m.detail.SetReadme(msg.Content)

			// Restore cached summary if available (no auto-call)
			if m.claude != nil && m.claude.Available() && msg.Content != "" {
				if summary, ok := m.claudeSummaries[msg.FullName]; ok {
					m.detail.SetAISummary(summary)
				}
			}
		}
		return m, nil

	case readmeErrorMsg:
		m.detail.SetLoading(false)
		return m, nil

	case rateLimitMsg:
		m.statusBar.SetRateLimit(msg.RateLimit)
		return m, nil

	case searchHistoryMsg:
		m.searchHistory = msg.Recent
		if len(msg.Bookmarked) > 0 {
			seen := make(map[string]bool)
			merged := make([]searchHistoryItem, 0, len(msg.Bookmarked)+len(msg.Recent))
			for _, s := range msg.Bookmarked {
				merged = append(merged, s)
				seen[s.Query] = true
			}
			for _, s := range msg.Recent {
				if !seen[s.Query] {
					merged = append(merged, s)
				}
			}
			m.searchHistory = merged
		}
		m.historyCursor = -1
		return m, nil

	case exclusionsLoadedMsg:
		m.exclusionSet = msg.Set
		m.applyGlobalExclusions()
		m.statusBar.SetExclusionCount(m.exclusionSet.Count())
		return m, nil

	case exclusionUpdatedMsg:
		m.exclusionSet = msg.Set
		m.applyGlobalExclusions()
		m.statusBar.SetExclusionCount(m.exclusionSet.Count())
		if m.exclusionMgr.IsActive() {
			m.exclusionMgr.rebuildItems(m.exclusionSet)
		}
		if msg.Added {
			hidden := m.refilterExclusions()
			m.list.SetRepos(m.repos)
			m.list.SetWatched(m.watchSet)
			label := msg.Kind + " \"" + msg.Value + "\""
			if hidden > 0 {
				m.statusBar.SetMessage(fmt.Sprintf("excluded %s (%d repos hidden)", label, hidden), false)
			} else {
				m.statusBar.SetMessage("excluded "+label, false)
			}
		} else {
			m.statusBar.SetMessage(fmt.Sprintf("removed %s \"%s\"", msg.Kind, msg.Value), false)
		}
		return m, clearStatusAfter(3 * time.Second)

	case watchlistLoadedMsg:
		m.watchSet = msg.WatchSet
		if m.watchSet == nil {
			m.watchSet = make(map[string]bool)
		}
		m.list.SetWatched(m.watchSet)
		return m, nil

	case watchToggledMsg:
		m.watchSet[msg.FullName] = msg.Watched
		if !msg.Watched {
			delete(m.watchSet, msg.FullName)
		}
		for i := range m.repos {
			if m.repos[i].FullName == msg.FullName {
				m.repos[i].Watchlisted = msg.Watched
				break
			}
		}
		// Remove from watchlist view immediately if unwatched while on watchlist screen.
		if m.state == stateWatchlist && !msg.Watched {
			newRepos := m.watchlistRepos[:0]
			for _, r := range m.watchlistRepos {
				if r.FullName != msg.FullName {
					newRepos = append(newRepos, r)
				}
			}
			m.watchlistRepos = newRepos
			m.watchlistList.SetRepos(m.watchlistRepos)
		}
		m.list.SetWatched(m.watchSet)
		m.watchlistList.SetWatched(m.watchSet)
		action := "Watching"
		if msg.Watched {
			m.sessionWatched++
		}
		if !msg.Watched {
			action = "Unwatched"
		}
		m.statusBar.SetMessage(action+" "+msg.FullName, false)
		// Watch pulse animation
		m.list.watchPulseRepo = msg.FullName
		m.list.watchPulseFrame = 4
		return m, tea.Batch(clearStatusAfter(3*time.Second), uiAnimTickCmd())

	case watchlistReposMsg:
		m.watchlistRepos = msg.Repos
		m.watchlistLoading = false
		if len(m.watchlistRepos) > 0 {
			model.ComputeScores(m.watchlistRepos)
			// Sort by star delta descending (most changed first)
			sort.Slice(m.watchlistRepos, func(i, j int) bool {
				return m.watchlistRepos[i].StarDelta > m.watchlistRepos[j].StarDelta
			})
			m.watchlistList.SetRepos(m.watchlistRepos)
			m.watchlistList.SetWatched(m.watchSet)
			names := make([]string, len(m.watchlistRepos))
			for i, r := range m.watchlistRepos {
				names[i] = r.FullName
			}
			return m, refreshWatchlistCmd(m.github, names)
		}
		return m, nil

	case watchlistRefreshedMsg:
		statsMap := make(map[string]model.Repo, len(msg.Stats))
		for _, s := range msg.Stats {
			statsMap[s.FullName] = s
		}
		for i := range m.watchlistRepos {
			fresh, ok := statsMap[m.watchlistRepos[i].FullName]
			if !ok {
				continue
			}
			firstSeen := m.watchlistRepos[i].Stars - m.watchlistRepos[i].StarDelta
			m.watchlistRepos[i].Stars = fresh.Stars
			m.watchlistRepos[i].Forks = fresh.Forks
			m.watchlistRepos[i].OpenIssues = fresh.OpenIssues
			if !fresh.PushedAt.IsZero() {
				m.watchlistRepos[i].PushedAt = fresh.PushedAt
			}
			m.watchlistRepos[i].StarDelta = fresh.Stars - firstSeen
		}
		m.watchlistList.SetRepos(m.watchlistRepos)
		m.watchlistList.SetWatched(m.watchSet)
		if m.store != nil {
			_ = m.store.UpsertRepos(m.watchlistRepos)
		}
		return m, nil

	case cloneFinishedMsg:
		if msg.Err != nil {
			m.statusBar.SetMessage("Clone failed: "+msg.Err.Error(), true)
		} else {
			m.sessionCloned++
			m.statusBar.SetMessage("Cloned "+msg.FullName, false)
		}
		return m, clearStatusAfter(5 * time.Second)

	case statusMsg:
		m.statusBar.SetMessage(msg.Text, msg.IsError)
		return m, clearStatusAfter(3 * time.Second)

	case clearStatusMsg:
		m.statusBar.ClearMessage()
		return m, nil

	case spinnerTickMsg:
		if m.loading {
			m.statusBar.Tick()
			m.list.Tick()
			return m, spinnerTickCmd()
		}
		// Keep ticking for summarize dots animation
		if m.detail.summarizing {
			m.detail.summarizeDots = (m.detail.summarizeDots + 1) % 3
			return m, spinnerTickCmd()
		}
		return m, nil

	case rateLimitTickMsg:
		return m, rateLimitTickCmd()

	case peekRevealTickMsg:
		if m.peek.IsActive() && m.peek.Tick() {
			return m, peekRevealTickCmd()
		}
		return m, nil

	case firstLaunchTickMsg:
		if m.firstLaunch && m.state == stateSearchPrompt {
			m.firstLaunchFrame++
			return m, firstLaunchTickCmd()
		}
		return m, nil

	case uiAnimTickMsg:
		anyActive := false

		// Compare reveal
		if m.compare.IsActive() && m.compare.revealPhase < 3 {
			m.compare.revealPhase++
			anyActive = true
		}

		// Focus pulse countdown
		if m.focusAnimFrame > 0 {
			m.focusAnimFrame--
			if m.focusAnimFrame == 0 {
				m.detail.focusPulseActive = false
			}
			anyActive = true
		}

		// Watch pulse countdown
		if m.list.watchPulseFrame > 0 {
			m.list.watchPulseFrame--
			anyActive = true
		}

		if anyActive {
			return m, uiAnimTickCmd()
		}
		return m, nil

	case shimmerHoldMsg:
		m.shimmerHold = false
		m.list.shimmerHold = false
		m.list.SetLoading(false)
		return m, nil

	case tasteProfileMsg:
		m.tasteProfile = msg.Profile
		// Build language cycle: All + top languages from taste profile
		m.langCycleOptions = []string{""}
		for _, lang := range msg.Profile.TopLanguages {
			m.langCycleOptions = append(m.langCycleOptions, lang)
		}
		m.langCycleIdx = 0
		if !msg.Profile.Empty() {
			return m, surprisePickCmd(m.tasteEngine, msg.Profile)
		}
		return m, nil

	case surprisePickMsg:
		m.surprisePick = msg.Repo
		return m, nil

	case localScanMsg:
		if msg.Err == nil && msg.ProjectCount > 0 {
			m.statusBar.SetMessage(fmt.Sprintf("Scanned %d projects, %d dependencies", msg.ProjectCount, msg.DepCount), false)
			// Recompute taste profile with local data
			return m, tea.Batch(
				computeTasteCmd(m.tasteEngine),
				clearStatusAfter(3*time.Second),
			)
		}
		return m, nil

	case claudeSummaryMsg:
		m.summarizing = false
		if msg.Err == nil && msg.Summary != "" {
			m.claudeSummaries[msg.FullName] = msg.Summary
			if m.detail.repo != nil && m.detail.repo.FullName == msg.FullName {
				m.detail.SetAISummary(msg.Summary)
			}
		} else if m.detail.repo != nil && m.detail.repo.FullName == msg.FullName {
			m.detail.SetSummarizing(false)
		}
		return m, nil

	case claudeNLSearchMsg:
		if msg.Err != nil {
			m.statusBar.SetMessage("AI search failed: "+msg.Err.Error(), true)
			return m, clearStatusAfter(3 * time.Second)
		}
		m.nlSearchMode = false
		m.searchParams = msg.Params
		m.applyGlobalExclusions()
		m.searchBar.SetQuery(msg.Display)
		m.statusBar.SetMessage("AI translated: "+msg.Display, false)
		m.loading = true
		m.list.SetLoading(true)
		m.statusBar.SetLoading(true)
		return m, tea.Batch(
			searchReposCmd(m.github, m.store, m.searchParams),
			fetchRateLimitCmd(m.github),
			spinnerTickCmd(),
			clearStatusAfter(5*time.Second),
		)

	case claudeAnalysisMsg:
		if msg.Err == nil && msg.Analysis != "" {
			m.claudeAnalyses[msg.FullName] = msg.Analysis
			if m.detail.repo != nil && m.detail.repo.FullName == msg.FullName {
				m.detail.SetAIAnalysis(msg.Analysis)
			}
			m.statusBar.SetMessage("AI analysis ready", false)
		} else if msg.Err != nil {
			m.statusBar.SetMessage("AI analysis failed", true)
		}
		return m, clearStatusAfter(3 * time.Second)

	case starredReposSyncedMsg:
		if msg.Err == nil && msg.Count > 0 {
			// Recompute taste profile with starred repos data
			return m, computeTasteCmd(m.tasteEngine)
		}
		return m, nil

	case topicDriftMsg:
		if msg.Err == nil && len(msg.Topics) > 0 {
			m.suggestedTopics = msg.Topics
		}
		return m, nil

	case returnVisitMsg:
		if len(msg.Repos) > 0 && m.state == stateSearchPrompt && len(m.repos) == 0 {
			m.returnVisitRepos = msg.Repos
			// Renders as inline banner in searchPrompt — no state change
		}
		return m, nil

	case quitTimerMsg:
		if m.state == stateQuitting {
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m *appModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// ctrl+c always quits
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// q: session summary on first press, quit on second (or from quit state)
	if key.Matches(msg, keys.Quit) && !m.formMode {
		if m.state == stateWatchlist {
			m.state = stateBrowsing
			m.focus = paneList
			return m, nil
		}
		if m.state == stateQuitting {
			return m, tea.Quit
		}
		// First q: show session summary
		if m.sessionViewed > 0 || m.sessionWatched > 0 || m.sessionCloned > 0 {
			m.state = stateQuitting
			summary := fmt.Sprintf("session: %d explored · %d watched · %d cloned  [q to exit]",
				m.sessionViewed, m.sessionWatched, m.sessionCloned)
			m.statusBar.SetMessage(summary, false)
			return m, quitTimerCmd()
		}
		return m, tea.Quit
	}

	// Compare overlay captures all keys when active
	if m.compare.IsActive() {
		switch {
		case key.Matches(msg, keys.Escape), msg.String() == "q", msg.String() == "C":
			m.compare.Hide()
		}
		return m, nil
	}

	// Peek overlay captures all keys when active
	if m.peek.IsActive() {
		return m.handlePeekKey(msg)
	}

	// Help overlay captures all keys
	if m.helpView.IsActive() {
		m.helpView.Hide()
		return m, nil
	}
	if key.Matches(msg, keys.Help) && !m.formMode {
		m.helpView.Toggle()
		return m, nil
	}

	// Exclusion manager overlay captures keys when active
	if m.exclusionMgr.IsActive() {
		return m.handleExclusionMgrKey(msg)
	}

	// Exclude picker captures keys when active
	if m.excludePickerMode {
		return m.handleExcludePickerKey(msg)
	}


	// Watchlist toggle from list or detail (not while in search or form)
	if key.Matches(msg, keys.Watchlist) && m.focus != paneSearch && !m.formMode {
		return m.toggleWatchlistState()
	}

	switch m.focus {
	case paneSearch:
		return m.handleSearchKey(msg)
	case paneList:
		return m.handleListKey(msg)
	case paneDetail:
		return m.handleDetailKey(msg)
	}

	return m, nil
}

func (m *appModel) toggleWatchlistState() (tea.Model, tea.Cmd) {
	if m.state == stateWatchlist {
		// Stamp last viewed when leaving watchlist
		if m.store != nil {
			_ = m.store.UpdateWatchlistViewedAt()
		}
		m.state = stateBrowsing
		m.list.isWatchlist = false
		m.focus = paneList
		return m, nil
	}
	m.state = stateWatchlist
	m.watchlistLoading = true
	m.list.isWatchlist = true
	m.focus = paneList
	return m, loadWatchlistReposCmd(m.store)
}

func (m *appModel) applyExclusions() {
	if len(m.excludeSet) == 0 {
		return
	}
	var filtered []model.Repo
	for _, r := range m.repos {
		if !m.excludeSet[r.FullName] {
			filtered = append(filtered, r)
		}
	}
	m.list.SetRepos(filtered)
	m.list.SetWatched(m.watchSet)
}

func (m *appModel) activeList() *listModel {
	if m.state == stateWatchlist {
		return &m.watchlistList
	}
	return &m.list
}

func (m *appModel) launchPreset(idx int) (tea.Model, tea.Cmd) {
	presets := model.GetPresets()
	if idx < 0 || idx >= len(presets) {
		return m, nil
	}
	p := presets[idx]
	// Apply user-selected language filter from cycling
	if m.langCycleIdx > 0 && m.langCycleIdx < len(m.langCycleOptions) && p.Params.Language == "" {
		p.Params.Language = m.langCycleOptions[m.langCycleIdx]
	}
	m.searchParams = p.Params
	m.applyGlobalExclusions()
	m.loading = true
	m.searchBar.Blur()
	m.focus = paneList
	m.historyCursor = -1
	return m, tea.Batch(
		searchReposCmd(m.github, m.store, m.searchParams),
		fetchRateLimitCmd(m.github),
	)
}

func (m *appModel) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Route to form handler when form mode is active
	if m.formMode {
		return m.handleFormKey(msg)
	}

	// Preset shortcuts and form toggle when input is empty
	if m.searchBar.Value() == "" {
		switch msg.String() {
		case "h":
			// "h" (home) on empty search bar is a no-op — user is already home
			return m, nil
		case "1":
			return m.launchPreset(0)
		case "2":
			return m.launchPreset(1)
		case "3":
			return m.launchPreset(2)
		case "4":
			return m.launchPreset(3)
		case "5":
			return m.launchPreset(4)
		case "a":
			m.formMode = true
			return m, m.searchForm.Focus()
		case "S":
			if m.tasteEngine != nil && !m.tasteProfile.Empty() {
				return m, surprisePickCmd(m.tasteEngine, m.tasteProfile)
			}
		case "n":
			if m.claude != nil && m.claude.Available() {
				m.nlSearchMode = !m.nlSearchMode
				if m.nlSearchMode {
					m.searchBar.input.Placeholder = "describe what you're looking for..."
					m.statusBar.SetMessage("AI search mode — describe in plain English", false)
				} else {
					m.searchBar.input.Placeholder = "Search repos (e.g. mcp server, language:go, topic:cli)"
					m.statusBar.ClearMessage()
				}
				return m, nil
			}
		case "left":
			if len(m.langCycleOptions) > 1 {
				m.langCycleIdx = (m.langCycleIdx - 1 + len(m.langCycleOptions)) % len(m.langCycleOptions)
				return m, nil
			}
		case "right":
			if len(m.langCycleOptions) > 1 {
				m.langCycleIdx = (m.langCycleIdx + 1) % len(m.langCycleOptions)
				return m, nil
			}
		}
	}

	switch {
	case key.Matches(msg, keys.Escape):
		m.searchBar.Blur()
		m.searchBar.input.SetValue("") // clear any partial input
		m.nlSearchMode = false
		m.searchBar.input.Placeholder = "Search repos (e.g. mcp server, language:go, topic:cli)"
		m.historyCursor = -1
		if len(m.repos) > 0 {
			// Return to browsing results
			m.focus = paneList
			m.state = stateBrowsing
		} else {
			// No results — stay on search prompt, re-focus input
			m.state = stateSearchPrompt
			m.focus = paneSearch
			m.searchBar.Focus()
			return m, m.searchBar.input.Focus()
		}
		return m, nil

	case msg.String() == "enter":
		query := m.searchBar.Value()
		if query == "" {
			if m.historyCursor >= 0 && m.historyCursor < len(m.searchHistory) {
				query = m.searchHistory[m.historyCursor].Query
			}
			if query == "" {
				// Enter with empty input: jump to watchlist if return visit data exists
				if len(m.returnVisitRepos) > 0 {
					return m.toggleWatchlistState()
				}
				return m, nil
			}
		}

		// "alt <dep>" — find alternatives to a dependency
		if strings.HasPrefix(query, "alt ") {
			depName := strings.TrimPrefix(query, "alt ")
			altQuery := m.buildAltSearch(depName)
			if altQuery != "" {
				m.searchParams.Query = altQuery
				m.loading = true
				m.list.SetLoading(true)
				m.statusBar.SetLoading(true)
				m.searchBar.Blur()
				m.focus = paneList
				m.historyCursor = -1
				m.statusBar.SetMessage("Finding alternatives to "+depName, false)
				return m, tea.Batch(
					searchReposCmd(m.github, m.store, m.searchParams),
					fetchRateLimitCmd(m.github),
					spinnerTickCmd(),
				)
			}
		}

		// "like" — seed search from local project fingerprint
		if strings.HasPrefix(query, "like ") || query == "like" {
			path := strings.TrimPrefix(query, "like ")
			if path == "" || path == "like" {
				path = "."
			}
			likeQuery := m.buildLikeSearch(path)
			if likeQuery != "" {
				m.searchParams.Query = likeQuery
				m.loading = true
				m.list.SetLoading(true)
				m.statusBar.SetLoading(true)
				m.searchBar.Blur()
				m.focus = paneList
				m.historyCursor = -1
				m.statusBar.SetMessage("Finding repos like "+path, false)
				return m, tea.Batch(
					searchReposCmd(m.github, m.store, m.searchParams),
					fetchRateLimitCmd(m.github),
					spinnerTickCmd(),
				)
			}
		}

		// NL search mode: route through Claude for translation
		if m.nlSearchMode && m.claude != nil && m.claude.Available() {
			m.nlSearchMode = false
			m.searchBar.input.Placeholder = "Search repos (e.g. mcp server, language:go, topic:cli)"
			m.statusBar.SetMessage("Translating with Claude...", false)
			m.loading = true
			m.list.SetLoading(true)
			m.statusBar.SetLoading(true)
			m.searchBar.Blur()
			m.focus = paneList
			m.historyCursor = -1
			return m, tea.Batch(
				claudeNLSearchCmd(m.claude, query),
				spinnerTickCmd(),
			)
		}

		m.searchParams.Query = query
		m.loading = true
		m.searchBar.Blur()
		m.focus = paneList
		m.historyCursor = -1
		return m, tea.Batch(
			searchReposCmd(m.github, m.store, m.searchParams),
			fetchRateLimitCmd(m.github),
		)

	case msg.String() == "up":
		if len(m.searchHistory) > 0 {
			if m.historyCursor < len(m.searchHistory)-1 {
				m.historyCursor++
			}
			m.searchBar.input.SetValue(m.searchHistory[m.historyCursor].Query)
		}
		return m, nil

	case msg.String() == "down":
		if m.historyCursor > 0 {
			m.historyCursor--
			m.searchBar.input.SetValue(m.searchHistory[m.historyCursor].Query)
		} else if m.historyCursor == 0 {
			m.historyCursor = -1
			m.searchBar.input.SetValue("")
		}
		return m, nil

	case msg.String() == "ctrl+b":
		if m.historyCursor >= 0 && m.historyCursor < len(m.searchHistory) && m.store != nil {
			item := &m.searchHistory[m.historyCursor]
			_ = m.store.ToggleSearchBookmark(item.ID)
			item.Bookmarked = !item.Bookmarked
		}
		return m, nil

	default:
		m.historyCursor = -1
		var cmd tea.Cmd
		m.searchBar.input, cmd = m.searchBar.input.Update(msg)
		return m, cmd
	}
}

func (m *appModel) handleFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.formMode = false
		m.searchForm.Blur()
		// Return to browsing if we have results, otherwise stay on prompt.
		if len(m.repos) > 0 {
			m.state = stateBrowsing
			m.focus = paneList
			m.searchBar.Blur()
		}
		return m, nil

	case msg.String() == "enter":
		params := m.searchForm.BuildParams()
		if params.Query == "" && params.Language == "" && params.Stars == "" && len(params.Topics) == 0 {
			return m, nil
		}
		m.searchParams = params
		m.applyGlobalExclusions()
		m.loading = true
		m.formMode = false
		m.searchForm.Blur()
		m.searchBar.Blur()
		m.focus = paneList
		m.state = stateBrowsing
		return m, tea.Batch(
			searchReposCmd(m.github, m.store, m.searchParams),
			fetchRateLimitCmd(m.github),
		)

	case msg.String() == "tab":
		return m, m.searchForm.NextField()

	case msg.String() == "shift+tab":
		return m, m.searchForm.PrevField()

	case msg.String() == "left" || msg.String() == "right":
		if m.searchForm.OnSortField() {
			if msg.String() == "right" {
				m.searchForm.CycleSortNext()
			} else {
				m.searchForm.CycleSortPrev()
			}
			return m, nil
		}
		return m, m.searchForm.UpdateActiveField(msg)

	default:
		return m, m.searchForm.UpdateActiveField(msg)
	}
}

func (m *appModel) handlePeekKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	al := m.activeList()
	switch {
	case key.Matches(msg, keys.Peek), key.Matches(msg, keys.Escape), msg.String() == "q":
		m.peek.Hide()
		return m, nil
	case key.Matches(msg, keys.Enter):
		m.peek.Hide()
		m.focus = paneDetail
		return m, nil
	case key.Matches(msg, keys.Watch):
		if sel := al.Selected(); sel != nil {
			return m, toggleWatchCmd(m.store, sel.FullName)
		}
		return m, nil
	case key.Matches(msg, keys.Open):
		if sel := al.Selected(); sel != nil {
			return m, openBrowserCmd(m.github, sel.FullName)
		}
		return m, nil
	case key.Matches(msg, keys.Clone):
		if sel := al.Selected(); sel != nil {
			m.peek.Hide()
			m.statusBar.SetMessage("Cloning "+sel.FullName+"...", false)
			return m, cloneRepoCmd(m.github, sel.FullName, m.cfg.Clone.DefaultDirectory)
		}
		return m, nil
	}
	return m, nil
}

func (m *appModel) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.filterMode = false
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.list.SetFilter("")
		m.statusBar.SetCounts(len(m.repos), m.list.Len())
		return m, nil
	case msg.String() == "enter":
		m.filterMode = false
		m.filterInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.list.SetFilter(m.filterInput.Value())
		m.statusBar.SetCounts(len(m.repos), m.list.Len())
		return m, cmd
	}
}

func (m *appModel) handleListKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Filter mode only applies to the main browsing list.
	if m.filterMode && m.state != stateWatchlist {
		return m.handleFilterKey(msg)
	}

	al := m.activeList()
	prevCursor := al.cursor

	switch {
	case key.Matches(msg, keys.Up):
		al.MoveUp()
	case key.Matches(msg, keys.Down):
		al.MoveDown()
	case key.Matches(msg, keys.GoTop):
		al.GoTop()
	case key.Matches(msg, keys.GoEnd):
		al.GoBottom()
	case key.Matches(msg, keys.PageUp):
		al.PageUp()
	case key.Matches(msg, keys.PageDn):
		al.PageDown()

	case key.Matches(msg, keys.Back), key.Matches(msg, keys.Escape):
		if m.state == stateWatchlist {
			return m.toggleWatchlistState()
		}
		// Return to home — clear results so View renders the landing screen
		m.repos = nil
		m.list.SetRepos(nil)
		m.detail.SetRepo(nil)
		m.searchBar.Reset()
		m.searchBar.SetQuery("")
		m.searchBar.SetCount(0)
		m.state = stateSearchPrompt
		m.focus = paneSearch
		m.searchBar.Focus()
		return m, m.searchBar.input.Focus()

	case key.Matches(msg, keys.Enter):
		m.focus = paneDetail
		return m, nil

	case key.Matches(msg, keys.Search):
		m.focus = paneSearch
		m.searchBar.Focus()
		return m, m.searchBar.input.Focus()

	case key.Matches(msg, keys.Sort):
		if m.state != stateWatchlist {
			return m, m.cycleSort()
		}

	case key.Matches(msg, keys.Open):
		if sel := al.Selected(); sel != nil {
			return m, openBrowserCmd(m.github, sel.FullName)
		}

	case key.Matches(msg, keys.Clone):
		if sel := al.Selected(); sel != nil {
			m.statusBar.SetMessage("Cloning "+sel.FullName+"...", false)
			return m, cloneRepoCmd(m.github, sel.FullName, m.cfg.Clone.DefaultDirectory)
		}

	case key.Matches(msg, keys.Watch):
		if sel := al.Selected(); sel != nil {
			return m, toggleWatchCmd(m.store, sel.FullName)
		}

	case key.Matches(msg, keys.Filter):
		if m.state != stateWatchlist {
			m.filterMode = true
			m.filterInput.Focus()
			return m, m.filterInput.Focus()
		}

	case key.Matches(msg, keys.Peek):
		if sel := al.Selected(); sel != nil {
			m.peek.Show(sel)
			return m, peekRevealTickCmd()
		}
		return m, nil

	case key.Matches(msg, keys.Yank):
		if sel := al.Selected(); sel != nil {
			return m, yankToClipboard("https://github.com/" + sel.FullName)
		}

	case key.Matches(msg, keys.YankClone):
		if sel := al.Selected(); sel != nil {
			return m, yankToClipboard("git clone https://github.com/" + sel.FullName + ".git")
		}

	case key.Matches(msg, keys.Exclude):
		if sel := al.Selected(); sel != nil && m.state != stateWatchlist {
			m.excludePickerItems = buildPickerItems(sel)
			m.excludePickerMode = true
			return m, nil
		}

	case key.Matches(msg, keys.ExcludeManager):
		if m.exclusionSet != nil {
			m.exclusionMgr.Show(m.exclusionSet)
			return m, nil
		}

	case key.Matches(msg, keys.AISummarize):
		if sel := al.Selected(); sel != nil && m.claude != nil && m.claude.Available() {
			if summary, ok := m.claudeSummaries[sel.FullName]; ok {
				m.detail.SetAISummary(summary)
				return m, nil
			}
			if m.detail.readme != "" {
				m.detail.SetSummarizing(true)
				m.detail.summarizeDots = 0
				m.statusBar.SetMessage("Asking Claude...", false)
				return m, tea.Batch(claudeSummarizeCmd(m.claude, sel.FullName, m.detail.readme), spinnerTickCmd())
			}
			m.statusBar.SetMessage("README not loaded yet", true)
			return m, clearStatusAfter(2 * time.Second)
		}

	case key.Matches(msg, keys.WhyTrending):
		if sel := al.Selected(); sel != nil && m.claude != nil && m.claude.Available() {
			if analysis, ok := m.claudeAnalyses[sel.FullName]; ok {
				m.detail.SetAIAnalysis(analysis)
				return m, nil
			}
			m.statusBar.SetMessage("Analyzing trend...", false)
			return m, claudeWhyTrendingCmd(m.claude, *sel, m.detail.readme)
		}

	case key.Matches(msg, keys.Compare):
		if sel := al.Selected(); sel != nil {
			if !m.compare.HasLeft() {
				m.compare.MarkLeft(sel)
				m.statusBar.SetMessage("Compare: "+sel.FullName+" marked — select second repo and press C", false)
				return m, clearStatusAfter(5 * time.Second)
			}
			// Second press: open comparison with entrance animation
			if sel.FullName != m.compare.left.FullName {
				m.compare.Show(sel)
				return m, uiAnimTickCmd()
			}
			m.statusBar.SetMessage("Select a different repo to compare", true)
			return m, clearStatusAfter(2 * time.Second)
		}

	case key.Matches(msg, keys.Tab):
		m.focus = paneDetail
		m.focusAnimFrame = 3
		m.detail.focusPulseActive = true
		return m, uiAnimTickCmd()
	}

	if al.cursor != prevCursor {
		m.detailTag++
		debounceMs := 300
		if m.cfg != nil && m.cfg.Search.DebounceMs > 0 {
			debounceMs = m.cfg.Search.DebounceMs
		}
		return m, debounceDetailCmd(m.detailTag, time.Duration(debounceMs)*time.Millisecond)
	}

	return m, nil
}

func (m *appModel) handleDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	al := m.activeList()

	switch {
	// Tab always switches panels (list ↔ detail)
	case key.Matches(msg, keys.Tab):
		m.focus = paneList
		m.focusAnimFrame = 3
		m.detail.focusPulseActive = false
		return m, uiAnimTickCmd()

	case key.Matches(msg, keys.Back), key.Matches(msg, keys.Escape):
		m.focus = paneList
		return m, nil

	case key.Matches(msg, keys.Up):
		m.detail.viewport.ScrollUp(1)
	case key.Matches(msg, keys.Down):
		m.detail.viewport.ScrollDown(1)
	case key.Matches(msg, keys.PageUp):
		m.detail.viewport.ScrollUp(m.detail.viewport.Height() / 2)
	case key.Matches(msg, keys.PageDn):
		m.detail.viewport.ScrollDown(m.detail.viewport.Height() / 2)
	case key.Matches(msg, keys.GoTop):
		m.detail.viewport.GotoTop()
	case key.Matches(msg, keys.GoEnd):
		m.detail.viewport.GotoBottom()

	case key.Matches(msg, keys.Open):
		if sel := al.Selected(); sel != nil {
			return m, openBrowserCmd(m.github, sel.FullName)
		}
	case key.Matches(msg, keys.Clone):
		if sel := al.Selected(); sel != nil {
			m.statusBar.SetMessage("Cloning "+sel.FullName+"...", false)
			return m, cloneRepoCmd(m.github, sel.FullName, m.cfg.Clone.DefaultDirectory)
		}
	case key.Matches(msg, keys.Watch):
		if sel := al.Selected(); sel != nil {
			return m, toggleWatchCmd(m.store, sel.FullName)
		}

	case key.Matches(msg, keys.AISummarize):
		if sel := al.Selected(); sel != nil && m.claude != nil && m.claude.Available() {
			if summary, ok := m.claudeSummaries[sel.FullName]; ok {
				m.detail.SetAISummary(summary)
				return m, nil
			}
			if m.detail.readme != "" {
				m.detail.SetSummarizing(true)
				m.detail.summarizeDots = 0
				m.statusBar.SetMessage("Asking Claude...", false)
				return m, tea.Batch(claudeSummarizeCmd(m.claude, sel.FullName, m.detail.readme), spinnerTickCmd())
			}
		}

	case key.Matches(msg, keys.WhyTrending):
		if sel := al.Selected(); sel != nil && m.claude != nil && m.claude.Available() {
			if analysis, ok := m.claudeAnalyses[sel.FullName]; ok {
				m.detail.SetAIAnalysis(analysis)
				return m, nil
			}
			m.statusBar.SetMessage("Analyzing trend...", false)
			return m, claudeWhyTrendingCmd(m.claude, *sel, m.detail.readme)
		}

	case key.Matches(msg, keys.Exclude):
		if sel := al.Selected(); sel != nil {
			m.excludePickerItems = buildPickerItems(sel)
			m.excludePickerMode = true
			return m, nil
		}

	case key.Matches(msg, keys.ExcludeManager):
		if m.exclusionSet != nil {
			m.exclusionMgr.Show(m.exclusionSet)
			return m, nil
		}

	case key.Matches(msg, keys.Search):
		m.focus = paneSearch
		m.searchBar.Focus()
		return m, m.searchBar.input.Focus()
	}

	return m, nil
}

func (m *appModel) cycleSort() tea.Cmd {
	current := m.searchParams.Sort
	for i, s := range model.SortCycle {
		if s == current {
			next := model.SortCycle[(i+1)%len(model.SortCycle)]
			m.searchParams.Sort = next
			m.searchBar.SetSort(string(next))
			break
		}
	}

	// Client-side sorts (no re-fetch needed)
	switch m.searchParams.Sort {
	case model.SortScore:
		if len(m.repos) > 0 {
			model.SortByScore(m.repos)
			m.list.SetRepos(m.repos)
			m.list.SetWatched(m.watchSet)
		}
		return nil
	case model.SortDiscovery:
		if len(m.repos) > 0 {
			model.SortByDiscovery(m.repos, m.list.seenSet)
			m.list.SetRepos(m.repos)
			m.list.SetWatched(m.watchSet)
		}
		return nil
	}

	if m.searchParams.Query != "" {
		m.loading = true
		return tea.Batch(
			searchReposCmd(m.github, m.store, m.searchParams),
			fetchRateLimitCmd(m.github),
		)
	}
	return nil
}

// applyGlobalExclusions injects persistent exclusions from SQLite into the current search params.
// Called after any full replacement of m.searchParams (presets, form, NL search) and after exclusion updates.
func (m *appModel) applyGlobalExclusions() {
	if m.exclusionSet == nil {
		return
	}
	m.searchParams.ExcludeTopics = m.exclusionSet.Topics
	m.searchParams.ExcludeOwners = m.exclusionSet.Owners
	m.searchParams.ExcludeKeywords = m.exclusionSet.Keywords
}

// refilterExclusions removes repos from the current result set that match persistent exclusions.
// Used for immediate feedback after adding an exclusion mid-session.
func (m *appModel) refilterExclusions() int {
	if m.exclusionSet == nil || len(m.repos) == 0 {
		return 0
	}
	ownerSet := make(map[string]bool, len(m.exclusionSet.Owners))
	for _, o := range m.exclusionSet.Owners {
		ownerSet[strings.ToLower(o)] = true
	}
	topicSet := make(map[string]bool, len(m.exclusionSet.Topics))
	for _, t := range m.exclusionSet.Topics {
		topicSet[strings.ToLower(t)] = true
	}

	before := len(m.repos)
	filtered := m.repos[:0]
	for _, r := range m.repos {
		if ownerSet[strings.ToLower(r.Owner)] {
			continue
		}
		// Check topics (only available if enriched)
		topicHit := false
		for _, t := range r.Topics {
			if topicSet[strings.ToLower(t)] {
				topicHit = true
				break
			}
		}
		if topicHit {
			continue
		}
		// Check keywords in name/description
		haystack := strings.ToLower(r.FullName + " " + r.Description)
		kwHit := false
		for _, kw := range m.exclusionSet.Keywords {
			if strings.Contains(haystack, strings.ToLower(kw)) {
				kwHit = true
				break
			}
		}
		if kwHit {
			continue
		}
		filtered = append(filtered, r)
	}
	m.repos = filtered
	return before - len(filtered)
}

func (m *appModel) handleExclusionMgrKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.exclusionMgr.addMode {
		switch {
		case key.Matches(msg, keys.Escape):
			m.exclusionMgr.addMode = false
			m.exclusionMgr.addInput.Blur()
			return m, nil
		case msg.String() == "tab":
			m.exclusionMgr.addType = (m.exclusionMgr.addType + 1) % len(addTypeLabels)
			m.exclusionMgr.addInput.Prompt = "  " + addTypeLabels[m.exclusionMgr.addType] + ": "
			return m, nil
		case msg.String() == "enter":
			val := strings.TrimSpace(m.exclusionMgr.addInput.Value())
			kind := addTypeLabels[m.exclusionMgr.addType]
			m.exclusionMgr.addMode = false
			m.exclusionMgr.addInput.Blur()
			m.exclusionMgr.addInput.SetValue("")
			if val == "" {
				return m, nil
			}
			return m, addExclusionCmd(m.store, kind, val)
		}
		var cmd tea.Cmd
		m.exclusionMgr.addInput, cmd = m.exclusionMgr.addInput.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, keys.Escape), msg.String() == "X":
		m.exclusionMgr.Hide()
		return m, nil
	case msg.String() == "j", msg.String() == "down":
		if m.exclusionMgr.cursor < len(m.exclusionMgr.items)-1 {
			m.exclusionMgr.cursor++
		}
		return m, nil
	case msg.String() == "k", msg.String() == "up":
		if m.exclusionMgr.cursor > 0 {
			m.exclusionMgr.cursor--
		}
		return m, nil
	case msg.String() == "d":
		if item := m.exclusionMgr.selectedItem(); item != nil {
			kind, value := item.kind, item.value
			return m, removeExclusionCmd(m.store, kind, value)
		}
		return m, nil
	case msg.String() == "a":
		m.exclusionMgr.addMode = true
		m.exclusionMgr.addType = 0
		m.exclusionMgr.addInput.Prompt = "  " + addTypeLabels[0] + ": "
		m.exclusionMgr.addInput.SetValue("")
		return m, m.exclusionMgr.addInput.Focus()
	}

	return m, nil
}

func (m *appModel) handleExcludePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Keyword text input mode
	if m.excludePickerKW.Focused() {
		switch {
		case key.Matches(msg, keys.Escape):
			m.excludePickerKW.Blur()
			m.excludePickerMode = false
			return m, nil
		case msg.String() == "enter":
			kw := strings.TrimSpace(m.excludePickerKW.Value())
			m.excludePickerKW.Blur()
			m.excludePickerMode = false
			if kw == "" {
				return m, nil
			}
			return m, addExclusionCmd(m.store, "keyword", kw)
		}
		var cmd tea.Cmd
		m.excludePickerKW, cmd = m.excludePickerKW.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, keys.Escape):
		m.excludePickerMode = false
		return m, nil
	}

	// Number keys 1-9 select picker items
	k := msg.String()
	if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
		idx := int(k[0]-'1')
		if idx < len(m.excludePickerItems) {
			item := m.excludePickerItems[idx]
			m.excludePickerMode = false
			if item.kind == "keyword" && item.value == "" {
				// Open keyword text input
				m.excludePickerKW = textinput.New()
				m.excludePickerKW.Placeholder = "keyword to exclude…"
				m.excludePickerKW.Prompt = "  exclude keyword: "
				m.excludePickerKW.CharLimit = 80
				m.excludePickerMode = true
				return m, m.excludePickerKW.Focus()
			}
			return m, addExclusionCmd(m.store, item.kind, item.value)
		}
	}

	return m, nil
}

func (m *appModel) recalcLayout() {
	if m.width < 50 || m.height < 8 {
		return
	}

	m.singlePane = m.width < 70

	contentH := m.height - 3
	if contentH < 1 {
		contentH = 1
	}

	listContentH := contentH - 2 // -1 for filter bar, -1 for panel header
	if listContentH < 1 {
		listContentH = 1
	}

	if m.singlePane {
		// Single-pane: active pane gets full width
		fullW := m.width
		m.list.SetSize(fullW, listContentH)
		m.watchlistList.SetSize(fullW, listContentH)
		m.detail.SetSize(fullW, contentH)
		m.filterInput.SetWidth(max(1, fullW-12))
	} else {
		// Dual-pane: proportional split
		listPct := 35
		if m.cfg != nil && m.cfg.Display.ListWidthPercent > 0 {
			listPct = m.cfg.Display.ListWidthPercent
		}
		listW := m.width * listPct / 100
		detailW := m.width - listW - 1

		m.list.SetSize(listW, listContentH)
		m.watchlistList.SetSize(listW, listContentH)
		m.detail.SetSize(detailW, contentH)
		m.filterInput.SetWidth(max(1, listW-12))
	}

	m.searchBar.SetWidth(m.width)
	m.statusBar.SetWidth(m.width)
	m.statusBar.singlePane = m.singlePane
	m.helpView.SetSize(m.width, m.height)
	m.peek.SetSize(m.width, m.height)
	m.compare.SetSize(m.width, m.height)
	m.exclusionMgr.SetSize(m.width, m.height)
}

func (m *appModel) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.WindowTitle = "GitVoyager"

	if m.width < 50 || m.height < 8 {
		v.Content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleError.Render(fmt.Sprintf("Terminal too small (need 50x8, have %dx%d)", m.width, m.height)),
		)
		return v
	}

	if m.compare.IsActive() {
		v.Content = m.compare.View()
		return v
	}

	if m.exclusionMgr.IsActive() {
		v.Content = m.exclusionMgr.View()
		return v
	}

	if m.peek.IsActive() {
		v.Content = m.peek.View()
		return v
	}

	if m.helpView.IsActive() {
		v.Content = m.helpView.View()
		return v
	}

	if m.state == stateWatchlist {
		v.WindowTitle = fmt.Sprintf("GitVoyager — Watchlist (%d)", len(m.watchlistRepos))
		v.Content = m.renderWatchlistView()
		return v
	}

	if (m.state == stateSearchPrompt || m.state == stateQuitting) && len(m.repos) == 0 {
		v.Content = m.renderSearchPrompt()
		return v
	}

	// Set focus state on sub-models
	m.list.SetFocused(m.focus == paneList)
	m.detail.SetFocused(m.focus == paneDetail)

	// Set focus breadcrumb
	switch m.focus {
	case paneList:
		m.statusBar.SetFocusLabel(" LIST ")
	case paneDetail:
		m.statusBar.SetFocusLabel(" DETAIL ")
	case paneSearch:
		m.statusBar.SetFocusLabel(" SEARCH ")
	}

	header := m.searchBar.View()

	var content string
	if m.singlePane {
		// Single-pane mode: show only the focused pane
		if m.focus == paneDetail {
			content = m.detail.View()
		} else {
			listHeader := m.renderListHeader()
			filterBar := m.renderFilterBar()
			content = lipgloss.NewStyle().Width(m.list.width).MaxWidth(m.list.width).
				Render(lipgloss.JoinVertical(lipgloss.Left, listHeader, filterBar, m.list.View()))
		}
	} else {
		// Dual-pane mode
		listHeader := m.renderListHeader()
		filterBar := m.renderFilterBar()
		listView := lipgloss.NewStyle().Width(m.list.width).MaxWidth(m.list.width).
			Render(lipgloss.JoinVertical(lipgloss.Left, listHeader, filterBar, m.list.View()))

		divColor := colorBorder
		if m.focus == paneList || m.focus == paneDetail {
			if m.focusAnimFrame > 0 {
				divColor = colorAccentPulse
			} else {
				divColor = colorAccentCyan
			}
		}
		border := lipgloss.NewStyle().
			Foreground(divColor).
			Render(lipgloss.NewStyle().Height(m.height - 3).Render("│"))

		detailView := m.detail.View()
		content = lipgloss.JoinHorizontal(lipgloss.Top, listView, border, detailView)
	}

	var status string
	if m.excludePickerMode && !m.excludePickerKW.Focused() {
		status = renderPicker(m.excludePickerItems, m.width)
	} else if m.excludePickerMode && m.excludePickerKW.Focused() {
		status = lipgloss.NewStyle().PaddingLeft(1).Width(m.width).
			Render(m.excludePickerKW.View())
	} else {
		status = m.statusBar.View()
	}

	// Window title
	v.WindowTitle = "GitVoyager"
	if m.searchBar.query != "" {
		v.WindowTitle = "GitVoyager — " + m.searchBar.query
	}

	v.Content = lipgloss.JoinVertical(lipgloss.Left, header, content, status)
	return v
}

func (m *appModel) renderListHeader() string {
	al := m.activeList()
	label := "Repos"
	if m.state == stateWatchlist {
		label = "Watchlist"
	}

	dot := stylePanelTitleDim.Render("○")
	titleStyle := stylePanelTitleDim
	if m.focus == paneList {
		dot = lipgloss.NewStyle().Foreground(colorAccentCyan).Render("●")
		titleStyle = stylePanelTitle
	}

	count := styleMuted.Render(fmt.Sprintf(" %d", al.Len()))
	return " " + dot + titleStyle.Render(" "+label) + count
}

func (m *appModel) renderFilterBar() string {
	if m.filterMode {
		return styleAccent.Render("  ") + m.filterInput.View()
	}
	if f := m.filterInput.Value(); f != "" {
		return styleSubtle.Render(fmt.Sprintf("  filter: %s  (esc to clear)", f))
	}
	// Show topic drift suggestions if available
	if len(m.suggestedTopics) > 0 {
		hint := styleMuted.Render("  also try: ")
		var pills []string
		for _, t := range m.suggestedTopics {
			pills = append(pills, styleTopicInline.Render("["+t+"]"))
		}
		return hint + strings.Join(pills, " ")
	}
	return styleSubtle.Render("  f: filter  a: advanced search")
}

func (m *appModel) renderSearchPrompt() string {
	if m.formMode {
		return m.renderFormView()
	}

	// ── Gradient logo ──
	logoLines := []string{
		" ██████╗ ██╗████████╗",
		"██╔════╝ ██║╚══██╔══╝",
		"██║  ███╗██║   ██║   ",
		"██║   ██║██║   ██║   ",
		"╚██████╔╝██║   ██║   ",
		" ╚═════╝ ╚═╝   ╚═╝   ",
	}
	var logo strings.Builder
	for _, line := range logoLines {
		logo.WriteString(GradientText(line, "#22D3EE", "#818CF8"))
		logo.WriteByte('\n')
	}
	logoStr := logo.String()
	voyager := GradientText("  VOYAGER", "#22D3EE", "#C084FC")
	logoStr += voyager

	subtitle := styleMuted.Render("  discover underdogs ") +
		styleAccent.Render("·") +
		styleMuted.Render(" surface hidden gems ") +
		styleAccent.Render("·") +
		styleMuted.Render(" explore the void")

	// ── Search input ──
	searchBoxW := min(60, m.width-6)
	if searchBoxW < 30 {
		searchBoxW = 30
	}
	iconStr := styleCyan.Render("◎ ")
	iconW := lipgloss.Width(iconStr)
	promptW := lipgloss.Width(m.searchBar.input.Prompt)
	m.searchBar.input.SetWidth(max(1, searchBoxW-2-2-iconW-promptW-1))
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentCyan).
		Padding(0, 1).
		Width(searchBoxW).
		Render(iconStr + m.searchBar.input.View())

	examples := styleMuted.Render("  mcp server, language:go, topic:cli, stars:>1000   ") +
		styleAccent.Render("a") + styleMuted.Render(": advanced search")
	if m.claude != nil && m.claude.Available() {
		examples += "  " + styleCyan.Render("n") + styleMuted.Render(": AI search")
	}
	if m.localScanner != nil {
		examples += "\n  " + stylePulse.Render("alt <dep>") + styleMuted.Render(": find alternatives") +
			"  " + stylePulse.Render("like .") + styleMuted.Render(": repos like your project")
	}

	// ── Language filter indicator ──
	var langIndicator string
	if len(m.langCycleOptions) > 1 {
		var pills []string
		for i, lang := range m.langCycleOptions {
			label := lang
			if label == "" {
				label = "All"
			}
			if i == m.langCycleIdx {
				c := langColor(lang)
				if lang == "" {
					c = colorAccentCyan
				}
				pill := lipgloss.NewStyle().
					Foreground(colorFgPrimary).
					Background(c).
					Bold(true).
					PaddingLeft(1).PaddingRight(1).
					Render(label)
				pills = append(pills, pill)
			} else {
				pills = append(pills, styleGhost.Render(label))
			}
		}
		langIndicator = "  " + styleGhost.Render("◀ ") + strings.Join(pills, styleMuted.Render(" · ")) + styleGhost.Render(" ▶")
	}

	// ── Preset cards (responsive grid) ──
	presets := model.GetPresets()
	icons := []string{"✦", "◈", "⚡", "◎"}
	singleColCards := m.width < 65
	cardW := 26
	if singleColCards {
		cardW = min(26, m.width-6)
	} else {
		cardW = min(26, (m.width-8)/2)
	}
	if cardW < 16 {
		cardW = 16
	}

	// Animated border colors for first-launch carousel
	carouselColors := []color.Color{colorAccentPulse, colorAccentViolet, colorAccentCyan, colorGoldStar}

	renderCard := func(idx int, p model.Preset) string {
		icon := styleCyan.Render(icons[idx])
		name := lipgloss.NewStyle().Bold(true).Foreground(colorFgPrimary).Render(p.Name)
		desc := styleMuted.Render(p.Description)
		if len(p.Description) > cardW-4 {
			desc = styleMuted.Render(p.Description[:cardW-7] + "...")
		}
		key := styleMuted.Render("[" + p.Key + "]")

		inner := lipgloss.JoinVertical(lipgloss.Left,
			icon+" "+name,
			desc,
			lipgloss.PlaceHorizontal(cardW-4, lipgloss.Right, key),
		)

		// Animated border on first launch — each card cycles through accent colors
		var borderColor color.Color = colorFgGhost
		if m.firstLaunch {
			colorIdx := (m.firstLaunchFrame + idx) % len(carouselColors)
			borderColor = carouselColors[colorIdx]
		}

		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1).
			Width(cardW).
			Render(inner)
	}

	var cardRows []string
	if singleColCards {
		// Vertical stack
		for i, p := range presets {
			cardRows = append(cardRows, renderCard(i, p))
		}
	} else {
		// 2×2 grid
		if len(presets) >= 2 {
			cardRows = append(cardRows, lipgloss.JoinHorizontal(lipgloss.Top,
				renderCard(0, presets[0]), "  ", renderCard(1, presets[1])))
		}
		if len(presets) >= 4 {
			cardRows = append(cardRows, lipgloss.JoinHorizontal(lipgloss.Top,
				renderCard(2, presets[2]), "  ", renderCard(3, presets[3])))
		}
	}

	// ── History ──
	var historyBlock string
	if len(m.searchHistory) > 0 {
		var hLines []string

		hasBookmarks := false
		for _, s := range m.searchHistory {
			if s.Bookmarked {
				hasBookmarks = true
				break
			}
		}

		if hasBookmarks {
			hLines = append(hLines, styleAccent.Render("  ★ Bookmarked"))
			for i, s := range m.searchHistory {
				if s.Bookmarked {
					hLines = append(hLines, m.renderHistoryItem(i, s))
				}
			}
			hLines = append(hLines, "")
		}

		hLines = append(hLines, styleMuted.Render("  ↩ Recent  ")+
			styleGhost.Render("up/down: select  ctrl+b: bookmark"))
		for i, s := range m.searchHistory {
			if !s.Bookmarked {
				hLines = append(hLines, m.renderHistoryItem(i, s))
			}
		}
		historyBlock = lipgloss.JoinVertical(lipgloss.Left, hLines...)
	}

	// ── Compose ──
	parts := []string{
		logoStr,
		subtitle,
		"",
		inputBox,
		"  " + examples,
	}
	if langIndicator != "" {
		parts = append(parts, langIndicator)
	} else {
		parts = append(parts, "")
	}
	if len(m.returnVisitRepos) > 0 {
		parts = append(parts, "", m.renderReturnVisitBanner())
	}
	for _, row := range cardRows {
		parts = append(parts, row)
	}
	if m.surprisePick != nil {
		parts = append(parts, "", m.renderSurprisePickCard())
	}
	if historyBlock != "" {
		parts = append(parts, "", historyBlock)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}

func (m *appModel) renderSurprisePickCard() string {
	r := m.surprisePick
	label := stylePulse.Render("  ⚡ Surprise Pick")
	name := lipgloss.NewStyle().Bold(true).Foreground(colorFgPrimary).Render("  " + r.FullName)
	lang := ""
	if r.Language != "" {
		lang = lipgloss.NewStyle().Foreground(langColor(r.Language)).Render("● " + r.Language)
	}
	stars := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
	desc := ""
	if r.Description != "" {
		d := r.Description
		if len(d) > 60 {
			d = d[:57] + "..."
		}
		desc = "\n" + styleMuted.Render("  "+d)
	}
	hint := styleMuted.Render("  enter: view  S: new pick")
	return lipgloss.JoinVertical(lipgloss.Left, label, name+"  "+lang+"  "+stars+desc, hint)
}

// buildAltSearch generates a search query to find alternatives to a dependency.
func (m *appModel) buildAltSearch(depName string) string {
	// Check if we know about this dependency from local scanning
	if m.store != nil {
		deps, _ := m.store.GetLocalDependencies()
		for _, d := range deps {
			// Match by name or last segment of the module path
			nameMatch := d.Name == depName || strings.HasSuffix(d.Name, "/"+depName)
			if nameMatch && d.RepoName != "" {
				// Use the dependency's language context
				lang := ""
				switch d.Source {
				case "go.mod":
					lang = "language:go"
				case "package.json":
					lang = "language:javascript"
				case "Cargo.toml":
					lang = "language:rust"
				case "requirements.txt", "pyproject.toml":
					lang = "language:python"
				}
				return fmt.Sprintf("%s %s stars:>10", depName, lang)
			}
		}
	}
	// Fallback: just search for the dep name
	return depName + " stars:>10"
}

// buildLikeSearch generates a search query from a local project's fingerprint.
func (m *appModel) buildLikeSearch(path string) string {
	if m.localScanner == nil {
		return ""
	}
	fp, err := m.localScanner.ScanDirectory(path)
	if err != nil || fp == nil {
		return ""
	}

	// Save the scan result
	if m.store != nil {
		_ = m.store.SaveProjectFingerprint(*fp)
	}

	parts := []string{}
	if fp.Language != "" {
		parts = append(parts, "language:"+strings.ToLower(fp.Language))
	}
	parts = append(parts, "stars:>10")

	return strings.Join(parts, " ")
}

func (m *appModel) renderFormView() string {
	formContent := m.searchForm.View(m.width)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Center, formContent)
}

func (m *appModel) renderWatchlistView() string {
	// Set focus state on sub-models
	m.watchlistList.SetFocused(m.focus == paneList)
	m.detail.SetFocused(m.focus == paneDetail)

	// Set focus breadcrumb
	switch m.focus {
	case paneList:
		m.statusBar.SetFocusLabel(" WATCHLIST ")
	case paneDetail:
		m.statusBar.SetFocusLabel(" DETAIL ")
	}

	header := m.renderWatchlistHeader()

	var content string
	if m.singlePane {
		if m.focus == paneDetail {
			content = m.detail.View()
		} else {
			listHeader := m.renderListHeader()
			hint := styleSubtle.Render("  w: unwatch  o: open  c: clone  W/q: back")
			content = lipgloss.JoinVertical(lipgloss.Left, listHeader, hint, m.watchlistList.View())
		}
	} else {
		listHeader := m.renderListHeader()
		hint := styleSubtle.Render("  w: unwatch  o: open  c: clone  W/q: back")
		listView := lipgloss.JoinVertical(lipgloss.Left, listHeader, hint, m.watchlistList.View())

		divColor := colorBorder
		if m.focus == paneList || m.focus == paneDetail {
			if m.focusAnimFrame > 0 {
				divColor = colorAccentPulse
			} else {
				divColor = colorAccentCyan
			}
		}
		border := lipgloss.NewStyle().
			Foreground(divColor).
			Render(lipgloss.NewStyle().Height(m.height - 3).Render("│"))

		detailView := m.detail.View()
		content = lipgloss.JoinHorizontal(lipgloss.Top, listView, border, detailView)
	}

	status := m.statusBar.View()

	v := lipgloss.JoinVertical(lipgloss.Left, header, content, status)
	return v
}

func (m *appModel) renderWatchlistHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).PaddingLeft(1).Render("♥ Watchlist")
	count := ""
	if m.watchlistLoading {
		count = styleSubtle.Render("  refreshing stats…")
	} else if len(m.watchlistRepos) > 0 {
		// Show "since you last looked" context
		sinceText := ""
		if m.store != nil {
			if lastViewed, err := m.store.GetWatchlistLastViewed(); err == nil && !lastViewed.IsZero() {
				sinceText = styleMuted.Render("  since " + relativeTime(lastViewed))
			}
		}
		count = styleSubtle.Render(fmt.Sprintf("  %d repos", len(m.watchlistRepos))) + sinceText
	} else {
		count = styleMuted.Render("  ♡ empty — press ") + styleAccent.Render("w") + styleMuted.Render(" on a repo to watch it")
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, title, count)
}

func (m *appModel) renderReturnVisit() string {
	return "" // deprecated: return visit now renders inline via renderReturnVisitBanner
}

func (m *appModel) renderReturnVisitBanner() string {
	// Determine width to match search box
	bannerW := min(60, m.width-6)
	if bannerW < 30 {
		bannerW = 30
	}
	innerW := bannerW - 4 // border padding

	var lines []string

	// Header
	title := styleMuted.Render("since you were here")
	lines = append(lines, title)
	lines = append(lines, "")

	// Delta repos (cap at 5)
	shown := m.returnVisitRepos
	if len(shown) > 5 {
		shown = shown[:5]
	}
	for _, r := range shown {
		delta := styleSuccess.Render(fmt.Sprintf("▲+%-4d", r.StarDelta))
		name := stylePrimary.Render(r.FullName)
		stars := styleMuted.Render(fmt.Sprintf("(%s ★)", formatStars(r.Stars)))

		nameW := lipgloss.Width(delta) + lipgloss.Width(name)
		starsW := lipgloss.Width(stars)
		gap := innerW - nameW - starsW
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, delta+name+strings.Repeat(" ", gap)+stars)
	}

	// Hint
	lines = append(lines, "")
	hint := styleAccent.Render("enter") + styleMuted.Render(" watchlist  ·  start typing to search")
	lines = append(lines, hint)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentViolet).
		Padding(0, 1).
		Width(bannerW).
		Render(content)
}

func (m *appModel) renderHistoryItem(idx int, s searchHistoryItem) string {
	cursor := "  "
	if idx == m.historyCursor {
		cursor = styleAccent.Render("> ")
	}

	bookmark := "  "
	if s.Bookmarked {
		bookmark = styleStars.Render("★ ")
	}

	query := s.Query
	meta := styleSubtle.Render(fmt.Sprintf(" (%d results, %s)", s.ResultCount, s.SearchedAt))

	if idx == m.historyCursor {
		query = styleAccent.Render(query)
	} else {
		query = lipgloss.NewStyle().Render(query)
	}

	return fmt.Sprintf("  %s%s%s%s", cursor, bookmark, query, meta)
}
