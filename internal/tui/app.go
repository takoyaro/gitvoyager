package tui

import (
	"fmt"
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
)

type appModel struct {
	// Components
	list        listModel
	detail      detailModel
	searchBar   searchBarModel
	statusBar   statusBarModel
	helpView    helpModel
	filterInput textinput.Model

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

	// Search state
	searchParams  model.SearchParams
	searchHistory []searchHistoryItem
	historyCursor int // cursor into searchHistory on prompt screen

	// Local sets
	watchSet map[string]bool

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
		searchParams: model.DefaultSearchParams(),
		watchSet:     make(map[string]bool),
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
	}

	if m.searchParams.Query != "" {
		m.loading = true
		cmds = append(cmds, searchReposCmd(m.github, m.store, m.searchParams))
	}

	// Load seen repos from store
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
		// Apply watchlist state
		for i := range m.repos {
			m.repos[i].Watchlisted = m.watchSet[m.repos[i].FullName]
		}
		m.loading = false
		m.list.SetRepos(m.repos)
		m.list.SetWatched(m.watchSet)
		m.searchBar.SetQuery(msg.Query)
		m.searchBar.SetCount(len(m.repos))
		m.statusBar.SetCounts(len(m.repos), m.list.Len())
		m.state = stateBrowsing
		m.focus = paneList
		// Save to search history
		if m.store != nil && msg.Query != "" {
			_ = m.store.SaveSearch(msg.Query, string(m.searchParams.Sort), m.searchParams.Language, len(m.repos))
		}
		// Trigger detail fetch for first item
		if sel := m.list.Selected(); sel != nil {
			m.detailTag++
			return m, debounceDetailCmd(m.detailTag, 100*time.Millisecond)
		}
		return m, nil

	case searchErrorMsg:
		m.loading = false
		m.statusBar.SetMessage("Search failed: "+msg.Err.Error(), true)
		return m, clearStatusAfter(5 * time.Second)

	case detailDebounceMsg:
		if msg.Tag != m.detailTag {
			return m, nil // stale debounce
		}
		sel := m.list.Selected()
		if sel == nil {
			return m, nil
		}
		m.detail.SetRepo(sel)
		// Mark as seen
		if m.store != nil {
			_ = m.store.MarkSeen(sel.FullName)
			m.list.seenSet[sel.FullName] = true
		}
		return m, tea.Batch(
			enrichRepoCmd(m.github, *sel),
			fetchReadmeCmd(m.github, sel.Owner, sel.Name),
		)

	case repoDetailMsg:
		// Update the repo in our slice without resetting README/scroll
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
		return m, nil

	case readmeMsg:
		if sel := m.list.Selected(); sel != nil && sel.FullName == msg.FullName {
			// Update repo's readme content
			for i, r := range m.repos {
				if r.FullName == msg.FullName {
					m.repos[i].ReadmeContent = msg.Content
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
		// Prepend bookmarked searches (deduped)
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
		m.historyCursor = -1 // no selection
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
		// Update the repo slice
		for i := range m.repos {
			if m.repos[i].FullName == msg.FullName {
				m.repos[i].Watchlisted = msg.Watched
				break
			}
		}
		m.list.SetWatched(m.watchSet)
		action := "Watching"
		if !msg.Watched {
			action = "Unwatched"
		}
		m.statusBar.SetMessage(action+" "+msg.FullName, false)
		return m, clearStatusAfter(3 * time.Second)

	case cloneFinishedMsg:
		if msg.Err != nil {
			m.statusBar.SetMessage("Clone failed: "+msg.Err.Error(), true)
		} else {
			m.statusBar.SetMessage("Cloned "+msg.FullName, false)
		}
		return m, clearStatusAfter(5 * time.Second)

	case statusMsg:
		m.statusBar.SetMessage(msg.Text, msg.IsError)
		return m, clearStatusAfter(3 * time.Second)

	case clearStatusMsg:
		m.statusBar.ClearMessage()
		return m, nil
	}

	return m, nil
}

func (m *appModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Global keys always handled
	if key.Matches(msg, keys.Quit) && m.focus != paneSearch {
		return m, tea.Quit
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Help overlay captures all keys
	if m.helpView.IsActive() {
		m.helpView.Hide()
		return m, nil
	}

	if key.Matches(msg, keys.Help) {
		m.helpView.Toggle()
		return m, nil
	}

	// Route by focus
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
	// Preset shortcuts when the input is empty
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
		}
	}

	switch {
	case key.Matches(msg, keys.Escape):
		if m.state == stateBrowsing {
			m.focus = paneList
			m.searchBar.Blur()
		}
		m.historyCursor = -1
		return m, nil

	case msg.String() == "enter":
		query := m.searchBar.Value()
		if query == "" {
			// If a history item is selected, use that
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
		// Navigate history
		if len(m.searchHistory) > 0 {
			if m.historyCursor < len(m.searchHistory)-1 {
				m.historyCursor++
			}
			m.searchBar.input.SetValue(m.searchHistory[m.historyCursor].Query)
		}
		return m, nil

	case msg.String() == "down":
		// Navigate history
		if m.historyCursor > 0 {
			m.historyCursor--
			m.searchBar.input.SetValue(m.searchHistory[m.historyCursor].Query)
		} else if m.historyCursor == 0 {
			m.historyCursor = -1
			m.searchBar.input.SetValue("")
		}
		return m, nil

	case msg.String() == "ctrl+b":
		// Bookmark/unbookmark the selected history item
		if m.historyCursor >= 0 && m.historyCursor < len(m.searchHistory) && m.store != nil {
			item := &m.searchHistory[m.historyCursor]
			_ = m.store.ToggleSearchBookmark(item.ID)
			item.Bookmarked = !item.Bookmarked
		}
		return m, nil

	default:
		// Forward to textinput
		m.historyCursor = -1
		var cmd tea.Cmd
		m.searchBar.input, cmd = m.searchBar.input.Update(msg)
		return m, cmd
	}
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
	if m.filterMode {
		return m.handleFilterKey(msg)
	}

	prevCursor := m.list.cursor

	switch {
	case key.Matches(msg, keys.Up):
		m.list.MoveUp()
	case key.Matches(msg, keys.Down):
		m.list.MoveDown()
	case key.Matches(msg, keys.GoTop):
		m.list.GoTop()
	case key.Matches(msg, keys.GoEnd):
		m.list.GoBottom()
	case key.Matches(msg, keys.PageUp):
		m.list.PageUp()
	case key.Matches(msg, keys.PageDn):
		m.list.PageDown()

	case key.Matches(msg, keys.Enter):
		m.focus = paneDetail
		return m, nil

	case key.Matches(msg, keys.Search):
		m.focus = paneSearch
		m.searchBar.Focus()
		return m, m.searchBar.input.Focus()

	case key.Matches(msg, keys.Sort):
		return m, m.cycleSort()

	case key.Matches(msg, keys.Open):
		if sel := m.list.Selected(); sel != nil {
			return m, openBrowserCmd(m.github, sel.FullName)
		}
	case key.Matches(msg, keys.Clone):
		if sel := m.list.Selected(); sel != nil {
			m.statusBar.SetMessage("Cloning "+sel.FullName+"...", false)
			return m, cloneRepoCmd(m.github, sel.FullName, m.cfg.Clone.DefaultDirectory)
		}
	case key.Matches(msg, keys.Watch):
		if sel := m.list.Selected(); sel != nil {
			return m, toggleWatchCmd(m.store, sel.FullName)
		}
	case key.Matches(msg, keys.Filter):
		m.filterMode = true
		m.filterInput.Focus()
		return m, m.filterInput.Focus()

	case key.Matches(msg, keys.Tab):
		m.focus = paneDetail
		return m, nil
	}

	// If cursor moved, debounce detail fetch
	if m.list.cursor != prevCursor {
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
	switch {
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
		if sel := m.list.Selected(); sel != nil {
			return m, openBrowserCmd(m.github, sel.FullName)
		}
	case key.Matches(msg, keys.Clone):
		if sel := m.list.Selected(); sel != nil {
			m.statusBar.SetMessage("Cloning "+sel.FullName+"...", false)
			return m, cloneRepoCmd(m.github, sel.FullName, m.cfg.Clone.DefaultDirectory)
		}
	case key.Matches(msg, keys.Watch):
		if sel := m.list.Selected(); sel != nil {
			return m, toggleWatchCmd(m.store, sel.FullName)
		}

	case key.Matches(msg, keys.Tab):
		m.focus = paneList
		return m, nil

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

	// Score sort is applied client-side — no API re-fetch needed
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
		return // too small
	}

	listPct := 35
	if m.cfg != nil && m.cfg.Display.ListWidthPercent > 0 {
		listPct = m.cfg.Display.ListWidthPercent
	}

	listW := m.width * listPct / 100
	detailW := m.width - listW - 1 // 1 for border

	contentH := m.height - 3 // header(1) + statusbar(1) + border(1)
	if contentH < 1 {
		contentH = 1
	}

	// Reserve 1 row for the filter bar (always present in list pane)
	listContentH := contentH - 1
	if listContentH < 1 {
		listContentH = 1
	}

	m.list.SetSize(listW, listContentH)
	m.detail.SetSize(detailW, contentH)
	m.searchBar.SetWidth(m.width)
	m.filterInput.SetWidth(listW - 12)
	m.statusBar.SetWidth(m.width)
	m.helpView.SetSize(m.width, m.height)
}

func (m *appModel) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if m.width < 70 || m.height < 10 {
		v.Content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleError.Render(fmt.Sprintf("Terminal too small (need 70x10, have %dx%d)", m.width, m.height)),
		)
		return v
	}

	// Help overlay
	if m.helpView.IsActive() {
		v.Content = m.helpView.View()
		return v
	}

	// Search prompt state
	if m.state == stateSearchPrompt && len(m.repos) == 0 {
		v.Content = m.renderSearchPrompt()
		return v
	}

	// Main browsing layout
	header := m.searchBar.View()

	filterBar := m.renderFilterBar()
	listView := lipgloss.JoinVertical(lipgloss.Left, filterBar, m.list.View())
	border := lipgloss.NewStyle().
		Foreground(colorBorder).
		Render(lipgloss.NewStyle().Height(m.height - 3).Render("│"))
	detailView := m.detail.View()

	content := lipgloss.JoinHorizontal(lipgloss.Top, listView, border, detailView)

	if m.loading {
		m.statusBar.SetMessage("Searching...", false)
	}
	status := m.statusBar.View()

	v.Content = lipgloss.JoinVertical(lipgloss.Left, header, content, status)
	return v
}

func (m *appModel) renderFilterBar() string {
	if m.filterMode {
		return styleAccent.Render("  ") + m.filterInput.View()
	}
	if f := m.filterInput.Value(); f != "" {
		return styleSubtle.Render(fmt.Sprintf("  filter: %s  (esc to clear)", f))
	}
	return styleSubtle.Render("  f: filter")
}

func (m *appModel) renderSearchPrompt() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent).
		MarginBottom(1).
		Render("  GitVoyager")

	subtitle := styleSubtle.Render("  Discover GitHub repos from your terminal")

	searchInput := m.searchBar.input.View()

	examples := styleSubtle.Render(
		"  Examples: mcp server, language:go, topic:cli, stars:>1000",
	)

	parts := []string{
		"",
		title,
		subtitle,
		"",
		searchInput,
		"",
		examples,
		"",
		styleSubtle.Render("  Quick Discovery"),
	}

	for _, p := range model.GetPresets() {
		key := styleAccent.Render("  [" + p.Key + "] ")
		name := lipgloss.NewStyle().Bold(true).Render(p.Name)
		desc := styleSubtle.Render("  " + p.Description)
		parts = append(parts, key+name)
		parts = append(parts, desc)
	}

	// Show search history
	if len(m.searchHistory) > 0 {
		parts = append(parts, "")

		hasBookmarks := false
		for _, s := range m.searchHistory {
			if s.Bookmarked {
				hasBookmarks = true
				break
			}
		}

		if hasBookmarks {
			parts = append(parts, styleAccent.Render("  Bookmarked Searches"))
			for i, s := range m.searchHistory {
				if !s.Bookmarked {
					continue
				}
				parts = append(parts, m.renderHistoryItem(i, s))
			}
			parts = append(parts, "")
		}

		parts = append(parts, styleSubtle.Render("  Recent Searches  (up/down to select, ctrl+b to bookmark)"))
		for i, s := range m.searchHistory {
			if s.Bookmarked {
				continue
			}
			parts = append(parts, m.renderHistoryItem(i, s))
		}
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)

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
