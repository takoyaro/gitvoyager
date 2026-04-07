package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
)

func searchReposCmd(client *github.Client, st *store.Store, params model.SearchParams) tea.Cmd {
	return func() tea.Msg {
		repos, err := client.SearchRepos(context.Background(), params)
		if err != nil {
			return searchErrorMsg{Err: err}
		}
		if st != nil {
			_ = st.UpsertRepos(repos)
		}
		return searchResultsMsg{Repos: repos, Query: params.Query}
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
		return time.Duration(m).Truncate(time.Minute).String() + " ago"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return time.Duration(h).Truncate(time.Hour).String() + " ago"
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
