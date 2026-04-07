package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/config"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
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
	peek peekModel

	// State
	repos      []model.Repo
	focus      pane
	state      appState
	loading    bool
	detailTag  int
	width      int
	height     int
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

	// Local sets
	watchSet   map[string]bool
	excludeSet map[string]bool // session-ephemeral exclusions

	// Session tracking
	sessionViewed  int
	sessionWatched int
	sessionCloned  int

	// Return visit
	returnVisitRepos []model.Repo

	// Dependencies
	cfg    *config.Config
	store  *store.Store
	github *github.Client
}

func NewApp(cfg *config.Config, st *store.Store, gh *github.Client, initialQuery string) *appModel {
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
		excludeSet:   make(map[string]bool),
		cfg:          cfg,
		store:        st,
		github:       gh,
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
		rateLimitTickCmd(),
		checkReturnVisitCmd(m.store),
	}

	if m.searchParams.Query != "" {
		m.loading = true
		m.list.SetLoading(true)
		m.statusBar.SetLoading(true)
		cmds = append(cmds, searchReposCmd(m.github, m.store, m.searchParams), spinnerTickCmd())
	} else if m.store != nil {
		// Auto-trending on first launch: if no search history exists, pre-fire trending
		recent, _ := m.store.RecentSearches(1)
		if len(recent) == 0 {
			presets := model.GetPresets()
			if len(presets) > 0 {
				m.searchParams = presets[0].Params // trending preset
				m.loading = true
				m.list.SetLoading(true)
				m.statusBar.SetLoading(true)
				cmds = append(cmds, searchReposCmd(m.github, m.store, m.searchParams), spinnerTickCmd())
			}
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
		for i := range m.repos {
			m.repos[i].Watchlisted = m.watchSet[m.repos[i].FullName]
		}
		m.loading = false
		m.list.SetLoading(false)
		m.statusBar.SetLoading(false)
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
		if sel := m.list.Selected(); sel != nil {
			m.detailTag++
			cmds = append(cmds, debounceDetailCmd(m.detailTag, 100*time.Millisecond))
		}
		cmds = append(cmds, loadStarDeltasCmd(m.store, m.repos))
		return m, tea.Batch(cmds...)

	case starDeltasLoadedMsg:
		if len(msg.FirstSeenStars) > 0 {
			for i := range m.repos {
				if first, ok := msg.FirstSeenStars[m.repos[i].FullName]; ok {
					m.repos[i].StarDelta = m.repos[i].Stars - first
				}
			}
			m.list.SetRepos(m.repos)
			m.list.SetWatched(m.watchSet)
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
		return m, nil

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
		return m, clearStatusAfter(3 * time.Second)

	case watchlistReposMsg:
		m.watchlistRepos = msg.Repos
		m.watchlistLoading = false
		if len(m.watchlistRepos) > 0 {
			model.ComputeScores(m.watchlistRepos)
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
			return m, spinnerTickCmd()
		}
		return m, nil

	case rateLimitTickMsg:
		return m, rateLimitTickCmd()

	case returnVisitMsg:
		if len(msg.Repos) > 0 && m.state == stateSearchPrompt && len(m.repos) == 0 {
			m.returnVisitRepos = msg.Repos
			m.state = stateReturnVisit
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
		if m.state == stateReturnVisit {
			return m, tea.Quit
		}
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

	// Return visit screen: enter=watchlist, /=search, any other key=search prompt
	if m.state == stateReturnVisit {
		switch {
		case key.Matches(msg, keys.Enter):
			return m.toggleWatchlistState()
		case key.Matches(msg, keys.Search):
			m.state = stateSearchPrompt
			m.focus = paneSearch
			m.searchBar.Focus()
			return m, m.searchBar.input.Focus()
		default:
			m.state = stateSearchPrompt
			m.focus = paneSearch
			m.searchBar.Focus()
			return m, m.searchBar.input.Focus()
		}
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
		m.state = stateBrowsing
		m.focus = paneList
		return m, nil
	}
	m.state = stateWatchlist
	m.watchlistLoading = true
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
	m.searchParams = presets[idx].Params
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
		case "1":
			return m.launchPreset(0)
		case "2":
			return m.launchPreset(1)
		case "3":
			return m.launchPreset(2)
		case "4":
			return m.launchPreset(3)
		case "a":
			m.formMode = true
			return m, m.searchForm.Focus()
		}
	}

	switch {
	case key.Matches(msg, keys.Escape):
		m.searchBar.Blur()
		m.searchBar.input.SetValue("") // clear any partial input
		m.historyCursor = -1
		m.focus = paneList
		if len(m.repos) > 0 {
			m.state = stateBrowsing
		}
		return m, nil

	case msg.String() == "enter":
		query := m.searchBar.Value()
		if query == "" {
			if m.historyCursor >= 0 && m.historyCursor < len(m.searchHistory) {
				query = m.searchHistory[m.historyCursor].Query
			}
			if query == "" {
				return m, nil
			}
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
		// Cascade back: list → home (clears results)
		if m.state == stateWatchlist {
			return m.toggleWatchlistState()
		}
		m.repos = nil
		m.list.SetRepos(nil)
		m.excludeSet = make(map[string]bool)
		m.state = stateSearchPrompt
		m.focus = paneSearch
		m.searchBar.Reset()
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
			m.excludeSet[sel.FullName] = true
			m.applyExclusions()
			m.statusBar.SetMessage("excluded "+sel.FullName, false)
			m.statusBar.SetCounts(len(m.repos)-len(m.excludeSet), m.list.Len())
			return m, clearStatusAfter(2 * time.Second)
		}

	case key.Matches(msg, keys.Tab):
		m.focus = paneDetail
		return m, nil
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

	// Detail tab switching: 1/2 for Overview/README
	switch msg.String() {
	case "1":
		m.detail.SetTab(tabOverview)
		return m, nil
	case "2":
		m.detail.SetTab(tabReadme)
		return m, nil
	}

	switch {
	// Tab always switches panels (list ↔ detail)
	case key.Matches(msg, keys.Tab):
		m.focus = paneList
		return m, nil

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

	if m.searchParams.Sort == model.SortScore {
		if len(m.repos) > 0 {
			model.SortByScore(m.repos)
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

func (m *appModel) recalcLayout() {
	if m.width < 70 || m.height < 10 {
		return
	}

	listPct := 35
	if m.cfg != nil && m.cfg.Display.ListWidthPercent > 0 {
		listPct = m.cfg.Display.ListWidthPercent
	}

	listW := m.width * listPct / 100
	detailW := m.width - listW - 1

	contentH := m.height - 3
	if contentH < 1 {
		contentH = 1
	}

	listContentH := contentH - 2 // -1 for filter bar, -1 for panel header
	if listContentH < 1 {
		listContentH = 1
	}

	m.list.SetSize(listW, listContentH)
	m.watchlistList.SetSize(listW, listContentH)
	m.detail.SetSize(detailW, contentH)
	m.searchBar.SetWidth(m.width)
	m.filterInput.SetWidth(listW - 12)
	m.statusBar.SetWidth(m.width)
	m.helpView.SetSize(m.width, m.height)
	m.peek.SetSize(m.width, m.height)
}

func (m *appModel) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.WindowTitle = "GitVoyager"

	if m.width < 70 || m.height < 10 {
		v.Content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleError.Render(fmt.Sprintf("Terminal too small (need 70x10, have %dx%d)", m.width, m.height)),
		)
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

	if m.state == stateReturnVisit {
		v.Content = m.renderReturnVisit()
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
		tabName := "OVERVIEW"
		if m.detail.tab == tabReadme {
			tabName = "README"
		}
		m.statusBar.SetFocusLabel(" " + tabName + " ")
	case paneSearch:
		m.statusBar.SetFocusLabel(" SEARCH ")
	}

	header := m.searchBar.View()
	listHeader := m.renderListHeader()
	filterBar := m.renderFilterBar()
	listView := lipgloss.JoinVertical(lipgloss.Left, listHeader, filterBar, m.list.View())

	// Focus-aware divider
	divColor := colorBorder
	if m.focus == paneList {
		divColor = colorAccentViolet
	} else if m.focus == paneDetail {
		divColor = colorAccentViolet
	}
	border := lipgloss.NewStyle().
		Foreground(divColor).
		Render(lipgloss.NewStyle().Height(m.height - 3).Render("│"))

	detailView := m.detail.View()
	content := lipgloss.JoinHorizontal(lipgloss.Top, listView, border, detailView)

	status := m.statusBar.View()

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
		dot = lipgloss.NewStyle().Foreground(colorAccentViolet).Render("●")
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
		logo.WriteString(GradientText(line, "#818CF8", "#22D3EE"))
		logo.WriteByte('\n')
	}
	logoStr := logo.String()
	voyager := GradientText("  VOYAGER", "#C084FC", "#22D3EE")
	logoStr += voyager

	subtitle := styleMuted.Render("  discover underdogs ") +
		styleAccent.Render("·") +
		styleMuted.Render(" surface hidden gems ") +
		styleAccent.Render("·") +
		styleMuted.Render(" explore the void")

	// ── Search input ──
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentViolet).
		Padding(0, 1).
		Width(60).
		Render(styleCyan.Render("◎ ") + m.searchBar.input.View())

	examples := styleMuted.Render("  mcp server, language:go, topic:cli, stars:>1000   ") +
		styleAccent.Render("a") + styleMuted.Render(": advanced search")

	// ── Preset cards (2×2 grid) ──
	presets := model.GetPresets()
	icons := []string{"✦", "◈", "⚡", "◎"}
	cardW := 26

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

		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFgGhost).
			Padding(0, 1).
			Width(cardW).
			Render(inner)
	}

	var row1Cards, row2Cards string
	if len(presets) >= 2 {
		row1Cards = lipgloss.JoinHorizontal(lipgloss.Top,
			renderCard(0, presets[0]), "  ", renderCard(1, presets[1]))
	}
	if len(presets) >= 4 {
		row2Cards = lipgloss.JoinHorizontal(lipgloss.Top,
			renderCard(2, presets[2]), "  ", renderCard(3, presets[3]))
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
		"  " + inputBox,
		"  " + examples,
		"",
		"  " + row1Cards,
	}
	if row2Cards != "" {
		parts = append(parts, "  "+row2Cards)
	}
	if historyBlock != "" {
		parts = append(parts, "", historyBlock)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
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
		tabName := "OVERVIEW"
		if m.detail.tab == tabReadme {
			tabName = "README"
		}
		m.statusBar.SetFocusLabel(" " + tabName + " ")
	}

	header := m.renderWatchlistHeader()

	listHeader := m.renderListHeader()
	hint := styleSubtle.Render("  w: unwatch  o: open  c: clone  W/q: back")
	listView := lipgloss.JoinVertical(lipgloss.Left, listHeader, hint, m.watchlistList.View())

	divColor := colorBorder
	if m.focus == paneList || m.focus == paneDetail {
		divColor = colorAccentViolet
	}
	border := lipgloss.NewStyle().
		Foreground(divColor).
		Render(lipgloss.NewStyle().Height(m.height - 3).Render("│"))

	detailView := m.detail.View()
	content := lipgloss.JoinHorizontal(lipgloss.Top, listView, border, detailView)

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
		count = styleSubtle.Render(fmt.Sprintf("  %d repos", len(m.watchlistRepos)))
	} else {
		count = styleSubtle.Render("  empty — press w on a repo to watch it")
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, title, count)
}

func (m *appModel) renderReturnVisit() string {
	var lines []string

	lines = append(lines, "")
	lines = append(lines, GradientText("  GitVoyager", "#818CF8", "#22D3EE"))
	lines = append(lines, "")
	lines = append(lines, styleSubtle.Render("  since you were here"))
	lines = append(lines, "")

	for _, r := range m.returnVisitRepos {
		delta := styleSuccess.Render(fmt.Sprintf("  ▲+%-5d", r.StarDelta))
		name := stylePrimary.Render(r.FullName)
		stars := styleMuted.Render(fmt.Sprintf(" (%s ★)", formatStars(r.Stars)))
		lines = append(lines, delta+name+stars)
	}

	if len(m.returnVisitRepos) == 0 {
		lines = append(lines, styleMuted.Render("  your watched repos are holding steady"))
	}

	lines = append(lines, "")
	lines = append(lines, styleAccent.Render("  enter")+styleMuted.Render(" view watchlist   ")+
		styleAccent.Render("/")+styleMuted.Render(" search   ")+
		styleAccent.Render("q")+styleMuted.Render(" quit"))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
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
