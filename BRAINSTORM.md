# GitVoyager: GitHub Repo Discovery Terminal App -- Brainstorm

Research conducted 2026-04-07 using `gh` CLI v2.x against the live GitHub API.

---

## 1. Core `gh` CLI Commands for Repo Discovery

### 1.1 `gh search repos` -- The Primary Workhorse

The single most useful command. Supports both structured flags and raw GitHub
search syntax passed as the query argument. Key capabilities:

**Structured flags (all tested and verified working):**

| Flag                     | Description                                   | Example                          |
|--------------------------|-----------------------------------------------|----------------------------------|
| `--topic`                | Filter by one or more topics                  | `--topic=llm,mcp`               |
| `--language`             | Primary coding language                       | `--language=python`              |
| `--stars`                | Star count (supports ranges)                  | `--stars="10..500"`              |
| `--forks`                | Fork count (supports ranges)                  | `--forks=">=5"`                  |
| `--created`              | Creation date filter                          | `--created=">2026-01-01"`        |
| `--updated`              | Last updated date filter                      | `--updated=">2026-03-01"`        |
| `--sort`                 | Sort by: stars, forks, updated, help-wanted-issues, best-match | `--sort=stars`     |
| `--order`                | asc or desc (default desc)                    | `--order=desc`                   |
| `--good-first-issues`    | Repos with N "good first issue" labels        | `--good-first-issues=">=5"`      |
| `--help-wanted-issues`   | Repos with N "help wanted" labels             | `--help-wanted-issues=">=3"`     |
| `--match`                | Restrict search to name, description, readme  | `--match=description`            |
| `--owner`                | Filter by owner/org                           | `--owner=microsoft`              |
| `--license`              | Filter by license type                        | `--license=mit`                  |
| `--archived`             | Include/exclude archived repos                | `--archived=false`               |
| `--include-forks`        | Include forked repos                          | `--include-forks=false`          |
| `--number-topics`        | Filter by number of topics applied            | `--number-topics=">=3"`          |
| `--followers`            | Number of followers/watchers                  | `--followers=">=10"`             |
| `--size`                 | Repo size in KB                               | `--size="<50000"`                |
| `--visibility`           | public, private, internal                     | `--visibility=public`            |
| `-L / --limit`           | Max results (default 30, max 1000)            | `--limit=100`                    |

**Raw query syntax (passed as positional argument):**

You can also pass raw GitHub search syntax directly:

```
gh search repos "mcp server stars:50..500 created:>2025-06-01 pushed:>2026-03-01"
```

This allows combining free-text keywords with qualifiers not exposed as flags
(like `pushed:` which differs from `updated:`).

**JSON output fields available:**

```
createdAt, defaultBranch, description, forksCount, fullName, hasDownloads,
hasIssues, hasPages, hasProjects, hasWiki, homepage, id, isArchived,
isDisabled, isFork, isPrivate, language, license, name, openIssuesCount,
owner, pushedAt, size, stargazersCount, updatedAt, url, visibility,
watchersCount
```

### 1.2 `gh repo view` -- Deep Dive on Individual Repos

Once a repo is identified, `gh repo view` provides rich detail via `--json`:

```
archivedAt, assignableUsers, codeOfConduct, contactLinks, createdAt,
defaultBranchRef, description, diskUsage, forkCount, fundingLinks,
hasDiscussionsEnabled, hasIssuesEnabled, hasProjectsEnabled, hasWikiEnabled,
homepageUrl, isArchived, isFork, isInOrganization, isTemplate, issues,
labels, languages, latestRelease, licenseInfo, mentionableUsers,
mergeCommitAllowed, milestones, name, nameWithOwner, owner, parent,
primaryLanguage, projects, projectsV2, pullRequests, pushedAt,
repositoryTopics, stargazerCount, updatedAt, url, viewerHasStarred,
visibility, watchers
```

Notable: `mentionableUsers` gives contributor count, `repositoryTopics` gives
full topic list, `latestRelease` gives release info, `languages` gives language
breakdown, `pullRequests` + `issues` give activity signals.

### 1.3 `gh repo list [owner]` -- Browse an Org/User's Repos

Useful for exploring all repos within a specific org/user. Supports filtering
by `--topic`, `--language`, `--visibility`, `--fork`/`--source`, `--archived`.

### 1.4 `gh api` -- Direct REST and GraphQL Access

The escape hatch for anything `gh search` cannot do. Key patterns:

- `gh api search/repositories?q=...` -- REST search with full query syntax
- `gh api graphql -f query='...'` -- GraphQL for complex nested queries
- `gh api repos/{owner}/{repo}/...` -- Specific repo endpoints
- Supports `--paginate`, `--slurp`, `--jq`, `--cache`, `--template`

### 1.5 Other `gh search` Subcommands

- `gh search commits` -- Find repos by commit messages/authors
- `gh search issues` -- Find active repos via their issue activity
- `gh search prs` -- Find repos with active PR activity
- `gh search code` -- Find repos containing specific code patterns (legacy engine)

---

## 2. GitHub Search API Qualifiers (Full Inventory)

Tested against both `gh search repos` flags and the REST API
(`search/repositories`). GitHub's search syntax supports these qualifiers for
repositories:

### 2.1 Quantitative Filters (all support ranges via `N..M`, `>N`, `>=N`, `<N`, `<=N`)

| Qualifier              | Description                                         |
|------------------------|-----------------------------------------------------|
| `stars:`               | Number of stars                                     |
| `forks:`               | Number of forks                                     |
| `followers:`           | Number of watchers/followers                        |
| `size:`                | Repo size in KB                                     |
| `good-first-issues:`   | Count of issues with "good first issue" label       |
| `help-wanted-issues:`  | Count of issues with "help wanted" label            |
| `topics:`              | Number of topics applied to the repo                |

### 2.2 Date Filters (support `>`, `>=`, `<`, `<=`, `YYYY-MM-DD..YYYY-MM-DD`)

| Qualifier    | Description                                              |
|--------------|----------------------------------------------------------|
| `created:`   | When the repo was created                                |
| `pushed:`    | When the repo last received a push (strongest freshness signal) |

Note: `updated:` is available via CLI flag but in the raw search syntax,
`pushed:` is the more reliable activity indicator.

### 2.3 Categorical Filters

| Qualifier      | Description                                            |
|----------------|--------------------------------------------------------|
| `language:`    | Primary programming language                           |
| `topic:`       | Repository topic tag                                   |
| `license:`     | License type (mit, apache-2.0, gpl-3.0, etc.)         |
| `user:` / `org:` | Owner filter                                        |
| `in:`          | Where to search: `name`, `description`, `topics`, `readme` |
| `is:`          | Repo state: `public`, `private`, `internal`            |
| `mirror:`      | Whether repo is a mirror: `true` / `false`             |
| `archived:`    | Whether repo is archived: `true` / `false`             |
| `fork:`        | Whether repo is a fork: `true` / `false` / `only`     |
| `template:`    | Whether repo is a template                             |
| `has:`         | Has discussions, pages, projects, etc. (undocumented)  |

### 2.4 Text Search

- Free text is matched against repo name, description, and README
- Quotes for exact phrase matching: `"mcp server"`
- Negation with `-`: `-topic:linux`
- Boolean: implicit AND between terms

### 2.5 Sorting Options

| Sort value           | Description                    |
|----------------------|--------------------------------|
| `best-match`         | Default relevance ranking      |
| `stars`              | Star count                     |
| `forks`              | Fork count                     |
| `help-wanted-issues` | Help wanted issue count        |
| `updated`            | Last updated time              |

---

## 3. Strategies for Finding "Underdog" Repos

These are repos that are low on stars but show strong signals of quality or
growth potential.

### 3.1 High Activity, Low Stars

```
gh search repos --stars="5..100" --updated=">2026-04-01" --forks=">=3" --sort=updated
```

Rationale: Repos that have been recently updated, have some forks (people using
it), but have not yet been "discovered" by the broader community.

### 3.2 Good Maintainership, Low Visibility

```
gh search repos --good-first-issues=">=5" --stars="10..500" --sort=updated
```

Rationale: Repos that have curated "good first issue" labels show intentional,
welcoming maintainership -- a quality signal independent of popularity.

### 3.3 New But Already Forked

```
gh search repos --created=">2026-01-01" --forks=">=5" --stars="<200" --sort=forks
```

Rationale: Forks indicate real usage/interest. A new repo with many forks
relative to its stars may be practically useful but under-marketed.

### 3.4 Active Issues, Low Stars (People Are Using It)

```
gh api 'search/repositories?q=stars:5..100+pushed:>2026-03-15+created:>2025-06-01&sort=updated&per_page=30'
```

Then filter results where `open_issues_count > 10` -- high issue count relative
to star count means real users are filing real bugs.

### 3.5 Multi-Topic Niche Repos

```
gh search repos --topic=mcp,typescript --stars="1..200" --created=">2025-06-01" --sort=stars
```

Repos at the intersection of multiple emerging topics that haven't yet broken
out.

### 3.6 Composite Scoring (Requires Post-Processing)

Fetch repos and compute a custom "underdog score":

```
underdog_score = (forks * 3 + open_issues * 2) / (stars + 1)
```

High score = lots of activity relative to fame. This requires fetching data and
scoring in-app since GitHub has no native composite scoring.

---

## 4. Strategies for Finding Trending Repos

### 4.1 Born-Trending: Recently Created + High Stars

```
gh search repos --created=">2026-03-01" --stars=">100" --sort=stars
```

This surfaces repos that accumulated significant stars in a very short period.
Tested and confirmed working -- found repos like claw-code (174K stars in ~1 week),
autoresearch (67K stars in ~1 month).

### 4.2 Star Velocity Calculation (Requires REST API)

GitHub does NOT provide a "stars gained this week" metric. But you can estimate it:

**Method A: Stargazer timestamps**

```
gh api 'repos/{owner}/{repo}/stargazers?per_page=100' \
  -H 'Accept: application/vnd.github.star+json' \
  --jq '[.[] | select(.starred_at > "2026-04-01")] | length'
```

This returns individual star events with timestamps. To calculate velocity,
count recent stars. CAVEAT: paginating through all stargazers for popular repos
is expensive (100 per page, 167K stars = 1,670 pages = 1,670 API calls).

**Method B: Snapshot comparison**

Store star counts periodically and compute deltas. This is the most practical
approach for a terminal app -- save state locally and compare over time.

**Method C: Age-based estimation**

```
stars_per_day = stars / days_since_creation
```

Crude but zero-cost. Useful for comparing repos of similar age.

### 4.3 Recent Push Activity on Popular Repos

```
gh search repos --stars=">1000" --updated=">2026-04-01" --sort=updated
```

Active popular repos -- not truly "trending" but still valuable for discovery.

### 4.4 Commit Velocity (Per-Repo Deep Dive)

```
gh api graphql -f query='{ repository(owner:"X", name:"Y") {
  defaultBranchRef { target { ... on Commit {
    history(since:"2026-03-01T00:00:00Z") { totalCount }
  }}}
}}'
```

Returns total commits since a date. Tested: ollama had 141 commits since March 1.
Great for "is this repo actively developed?" signal.

### 4.5 Participation Stats (Per-Repo)

```
gh api repos/{owner}/{repo}/stats/participation
```

Returns 52 weeks of commit data split by "all" vs "owner" contributors. Useful
for detecting if a repo has growing community contributions vs. being solo-maintained.

NOTE: This endpoint returns HTTP 202 on first request (GitHub computes stats in
background). Must retry after a short delay.

---

## 5. Rate Limits and How to Work Within Them

### 5.1 Observed Rate Limits (Authenticated User)

| Resource       | Limit       | Window     | Notes                               |
|----------------|-------------|------------|-------------------------------------|
| **search**     | **30/min**  | 1 minute   | THE critical constraint              |
| core (REST)    | 5,000/hr    | 1 hour     | Generous for per-repo detail calls   |
| graphql        | 5,000/hr    | 1 hour     | Points-based (complex queries cost more) |
| code_search    | 10/min      | 1 minute   | Very restrictive                     |

### 5.2 Practical Implications

- **Search is the bottleneck.** At 30 requests/minute, a user browsing
  different search queries will burn through the budget fast. Each `gh search
  repos` call = 1 search API request.
- **Pagination multiplies cost.** Getting 100 results at 30 per page = 4
  search requests (the `--limit` flag handles this transparently but still
  consumes the budget). The REST API allows `per_page=100`, which is more
  efficient.
- **Detail lookups are cheap.** Once you have repo names from search, calling
  `gh repo view` or `gh api repos/...` uses the core limit (5,000/hr), so
  enriching results with detail is affordable.
- **GraphQL is efficient for bulk enrichment.** A single GraphQL query can
  fetch details for multiple repos, reducing call count.

### 5.3 Rate Limit Mitigation Strategies

1. **Cache aggressively.** `gh api --cache 3600s` caches responses for 1 hour.
   Search results for the same query should be cached locally.
2. **Use GraphQL for batch enrichment.** Instead of N REST calls for N repos,
   use one GraphQL query with aliases to fetch details for up to ~20 repos at once.
3. **Show rate limit status in the UI.** Poll `gh api rate_limit` to display
   remaining budget.
4. **Debounce searches.** If building an interactive UI, debounce keystroke-
   triggered searches to avoid burning queries on partial input.
5. **Prefer `per_page=100` via REST API** over `gh search repos --limit=100`
   (which may use the default page size internally).
6. **Local state for trending.** Store snapshots locally to compute trends
   without repeatedly querying the same repos.

---

## 6. Gaps Where `gh` CLI Falls Short (Need REST/GraphQL Directly)

### 6.1 No Star Velocity / Trending Endpoint

GitHub has NO official "trending" API. The github.com/trending page is
server-rendered, not API-backed. To compute trending, you must either:

- Scrape github.com/trending (fragile, against ToS)
- Use the stargazers API with timestamps (expensive for popular repos)
- Build your own snapshot-based tracking

### 6.2 No Composite/Weighted Search

You cannot search for "repos where stars/age > X" or "repos where
forks/stars > Y". All search qualifiers are absolute values. Composite
scoring must be done client-side after fetching results.

### 6.3 Limited Sorting Options

`gh search repos --sort` only supports: stars, forks, updated,
help-wanted-issues, best-match. You cannot sort by:

- Created date (can filter but not sort by)
- Number of contributors
- Commit frequency
- Issue response time
- Star growth rate

### 6.4 No Contributor Count in Search Results

Search results do not include contributor count. Must use either:

- `gh api repos/{owner}/{repo}/contributors?per_page=1` and check the `Link`
  header for total count (hack but efficient)
- GraphQL `mentionableUsers { totalCount }` (verified working)

### 6.5 No Release Frequency in Search

Cannot search for "repos with a release in the last 30 days". Must fetch
individually per repo via:

```
gh api repos/{owner}/{repo}/releases?per_page=1 --jq '.[0].published_at'
```

### 6.6 Stats Endpoints Return 202 (Async Computation)

The `/stats/` endpoints (commit_activity, participation, contributors,
code_frequency, punch_card) return HTTP 202 on first access while GitHub
computes the data. Callers must implement retry logic. `gh api` does not
handle this automatically.

### 6.7 Traffic Data Requires Push Access

`/traffic/views`, `/traffic/clones`, `/traffic/popular/referrers` all require
push access to the repo. Useless for discovering repos you don't own.

### 6.8 Search Results Capped at 1,000

The GitHub Search API returns a maximum of 1,000 results per query, regardless
of total_count. For broad searches ("all Python repos with >10 stars"), you
must partition the search space (e.g., by creation date ranges) to get
complete coverage.

### 6.9 Code Search Is Legacy

`gh search code` uses GitHub's legacy code search engine. Results may not
match what github.com shows. Regex search is not available via the API.
Rate limit is only 10/minute.

---

## 7. Creative Search Strategies (Hidden Gems)

### 7.1 The "Quiet Contributor" Strategy

Find repos maintained by prolific open-source contributors but not yet famous:

1. Identify a well-known repo (e.g., ollama)
2. `gh api repos/ollama/ollama/contributors?per_page=10` to get top contributors
3. `gh repo list {username} --sort=updated --source` to find their personal projects

### 7.2 The "Topic Intersection" Strategy

Look for repos at the intersection of two hot but different domains:

```
gh search repos --topic=rust,llm --stars="10..1000" --sort=stars
gh search repos --topic=mcp,security --created=">2025-06-01" --sort=stars
gh search repos "drone reinforcement-learning" --stars=">5" --sort=stars
```

### 7.3 The "Help Wanted" Strategy

Repos actively seeking contributors are often high-quality but under-resourced:

```
gh search repos --help-wanted-issues=">=10" --stars="50..1000" --sort=help-wanted-issues
```

### 7.4 The "README Keyword" Strategy

Search specifically in READMEs for use-case-specific language:

```
gh search repos "production ready" --match=readme --stars=">50" --sort=stars
gh search repos "drop-in replacement" --match=readme --language=go --sort=stars
```

### 7.5 The "Fork-to-Star Ratio" Strategy (Post-Processing)

After fetching results, compute `fork_ratio = forks / stars`. High ratios
(> 0.3) suggest the repo is being actively used/extended, not just starred
for bookmarking. Libraries and tools tend to have higher fork ratios than
"awesome list" repos.

### 7.6 The "Issue Velocity" Strategy

Use `gh search issues` to find repos with high recent issue activity:

```
gh search issues --created=">2026-04-01" --label="bug" --sort=created
```

Group results by repo to find repos with the most active user bases.

### 7.7 The "Born Yesterday, Star Today" Strategy

Most effective trending detection without historical data:

```
gh search repos --created=">2026-04-01" --stars=">50" --sort=stars
```

A repo created this week with 50+ stars is definitionally trending. Adjusting
the date window and star threshold lets you calibrate signal strength.

### 7.8 The "Emerging Ecosystem" Strategy

Find repos in a new ecosystem/framework by topic density:

```
gh search repos --topic=mcp --created=">2025-01-01" --sort=stars --limit=100
```

Then analyze: what languages dominate? What secondary topics appear? This maps
an emerging ecosystem.

### 7.9 The "Active but Unstarred" Strategy

Find repos with lots of recent commits but few stars -- pure hidden gems:

1. Search for low-star, recently-pushed repos in a topic:
   ```
   gh search repos --topic=mcp --stars="1..20" --updated=">2026-03-15" --sort=updated
   ```
2. For each result, check commit velocity via GraphQL
3. Surface repos with high commit counts relative to their star count

### 7.10 The "Multi-Signal Score" Strategy

Fetch repos and compute a weighted discovery score:

```
discovery_score = (
    log(stars + 1) * 1.0 +
    log(forks + 1) * 2.0 +
    log(open_issues + 1) * 1.5 +
    recency_bonus(pushed_at) * 3.0 +
    has_good_first_issues * 2.0 +
    has_license * 1.0 +
    has_description * 0.5
) / log(age_in_days + 1)
```

Dividing by age normalizes for older repos, surfacing fast-growing newcomers.

---

## 8. Recommended Architecture Notes

### 8.1 Data Flow

```
[User Query] --> [Search API (30/min)] --> [Result List]
                                               |
                                               v
                                    [GraphQL Batch Enrichment (5000/hr)]
                                               |
                                               v
                                    [Client-Side Scoring & Ranking]
                                               |
                                               v
                                    [TUI Display with Details Panel]
```

### 8.2 Key Technical Decisions

- **Use `gh api` not `gh search repos`** for maximum control. The raw API
  gives access to `per_page`, pagination headers, and full query syntax. The
  CLI wrapper is convenient but less flexible.
- **GraphQL for enrichment, REST for search.** The search API is REST-only.
  But once you have repo names, GraphQL is far more efficient for fetching
  rich details (contributors, commit counts, release info, topics, languages)
  in batch.
- **Local SQLite for state.** Store star counts, discovery timestamps, and
  user bookmarks locally. This enables trending computation (delta between
  snapshots) and avoids re-fetching.
- **Respect the 30/min search limit.** This is the hard constraint. Design
  the UI so users explore results rather than firing off rapid searches.

### 8.3 GraphQL Batch Query Pattern (Verified Working)

A single GraphQL call can fetch rich data for a repo that would take 5+ REST calls:

```graphql
{
  repository(owner: "X", name: "Y") {
    stargazerCount
    forkCount
    watchers { totalCount }
    issues(states: OPEN) { totalCount }
    pullRequests(states: OPEN) { totalCount }
    releases(last: 3) { nodes { tagName publishedAt } }
    repositoryTopics(first: 10) { nodes { topic { name } } }
    languages(first: 5, orderBy: {field: SIZE, direction: DESC}) {
      edges { node { name } size }
    }
    mentionableUsers { totalCount }
    defaultBranchRef {
      target {
        ... on Commit {
          history(since: "2026-03-01T00:00:00Z") { totalCount }
        }
      }
    }
  }
}
```

Can use GraphQL aliases to query multiple repos in one request:

```graphql
{
  repo1: repository(owner: "A", name: "B") { ...RepoFields }
  repo2: repository(owner: "C", name: "D") { ...RepoFields }
  # ... up to ~20 repos per query before hitting complexity limits
}
```

---

## 9. Summary of Key Findings

1. **`gh search repos` is surprisingly powerful** -- it exposes nearly all GitHub
   search qualifiers as flags, supports JSON output, and handles pagination.

2. **The 30/min search rate limit is the primary constraint.** All architecture
   decisions should minimize search API calls.

3. **There is no trending API.** This is the single biggest gap. Building trending
   detection requires local state (snapshots) or expensive stargazer pagination.

4. **GraphQL is essential for enrichment.** Search results are shallow. GraphQL
   provides commit counts, contributor counts, release history, language breakdown,
   and topic lists in a single call.

5. **Composite scoring must be client-side.** GitHub cannot sort by "stars per day"
   or "fork-to-star ratio". The app must fetch, score, and re-rank locally.

6. **The best "hidden gem" strategies combine multiple signals**: low stars +
   high forks, recent creation + rapid growth, many good-first-issues + active
   pushes, high open issues + low stars.

7. **The `--cache` flag on `gh api` is a free win** for reducing redundant API
   calls during a session.

8. **Stats endpoints (participation, commit_activity) are async** -- they return
   202 on first call and require retry logic, which complicates real-time display.
