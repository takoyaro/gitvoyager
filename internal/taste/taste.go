package taste

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
)

// Profile represents the user's inferred preferences based on search history,
// watchlist, and viewed repos.
type Profile struct {
	Languages    map[string]float64 `json:"languages"`     // language -> affinity 0.0-1.0
	Topics       map[string]float64 `json:"topics"`        // topic -> affinity 0.0-1.0
	TopLanguages []string           `json:"top_languages"` // top 3
	TopTopics    []string           `json:"top_topics"`    // top 5
	TotalSearches int              `json:"total_searches"`
	ComputedAt   time.Time          `json:"computed_at"`
}

// Empty returns true if the profile has no meaningful data.
func (p Profile) Empty() bool {
	return len(p.Languages) == 0 && len(p.Topics) == 0
}

// Engine computes taste profiles from stored data.
type Engine struct {
	store *store.Store
}

// New creates a new taste engine.
func New(st *store.Store) *Engine {
	return &Engine{store: st}
}

// ComputeProfile mines search_history, watchlist, repos, and seen_log to build
// a taste profile. It weights watchlisted repos 3x and seen repos 1x.
func (e *Engine) ComputeProfile() (Profile, error) {
	p := Profile{
		Languages:  make(map[string]float64),
		Topics:     make(map[string]float64),
		ComputedAt: time.Now(),
	}

	// 1. Language distribution from repos (weighted by interaction)
	langDist, err := e.store.GetLanguageDistribution()
	if err != nil {
		return p, err
	}

	// 2. Topic distribution from repos
	topicDist, err := e.store.GetTopicDistribution()
	if err != nil {
		return p, err
	}

	// 3. Search count
	recent, _ := e.store.RecentSearches(100)
	p.TotalSearches = len(recent)

	// Also mine language mentions from search history
	for _, s := range recent {
		if s.Language != "" {
			langDist[s.Language] += 2 // explicit language filter = strong signal
		}
	}

	// 4. Local project languages (2x boost — you actually write in these)
	localLangs, _ := e.store.GetProjectLanguages()
	for lang, count := range localLangs {
		langDist[lang] += count * 5 // strong signal: local projects
	}

	// Normalize languages
	p.Languages = normalizeWeights(langDist)
	p.TopLanguages = topN(p.Languages, 3)

	// Normalize topics
	p.Topics = normalizeWeights(topicDist)
	p.TopTopics = topN(p.Topics, 5)

	// Cache the snapshot
	if data, err := json.Marshal(p); err == nil {
		_ = e.store.SaveTasteSnapshot(string(data))
	}

	return p, nil
}

// LoadCachedProfile returns the most recent cached profile, if fresh enough.
// Returns an empty profile if no cache exists or it's stale.
func (e *Engine) LoadCachedProfile() (Profile, bool) {
	data, computedAt, err := e.store.GetLatestTasteSnapshot()
	if err != nil || data == "" {
		return Profile{}, false
	}

	// Consider stale if older than 6 hours
	if time.Since(computedAt) > 6*time.Hour {
		return Profile{}, false
	}

	var p Profile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return Profile{}, false
	}
	return p, true
}

// PersonalizePreset injects taste profile data into a preset's search params.
// If the preset already specifies a language, it's left alone.
func (e *Engine) PersonalizePreset(preset model.Preset, profile Profile) model.Preset {
	if profile.Empty() {
		return preset
	}

	p := preset
	params := p.Params

	// Inject top language if preset doesn't already specify one
	if params.Language == "" && len(profile.TopLanguages) > 0 {
		params.Language = profile.TopLanguages[0]
		p.Name = preset.Name + " " + profile.TopLanguages[0]
	}

	p.Params = params
	return p
}

// SurprisePick returns a weighted-random repo from the database that matches
// the user's taste profile. Returns nil if no suitable repo is found.
func (e *Engine) SurprisePick(profile Profile) (*model.Repo, error) {
	if profile.Empty() {
		return nil, nil
	}

	repos, err := e.store.GetUnseenReposByAffinity(profile.TopLanguages, profile.TopTopics, 50)
	if err != nil || len(repos) == 0 {
		return nil, err
	}

	// Weight by discovery score with randomness
	model.ComputeScores(repos)
	type weighted struct {
		repo   model.Repo
		weight float64
	}
	candidates := make([]weighted, len(repos))
	for i, r := range repos {
		w := r.DiscoveryScore * (0.7 + rand.Float64()*0.6) // 0.7-1.3 random factor
		candidates[i] = weighted{repo: r, weight: w}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].weight > candidates[j].weight
	})

	pick := candidates[0].repo
	return &pick, nil
}

// normalizeWeights converts raw counts to 0.0-1.0 weights.
func normalizeWeights(counts map[string]int) map[string]float64 {
	if len(counts) == 0 {
		return nil
	}

	maxVal := 0
	for _, v := range counts {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return nil
	}

	result := make(map[string]float64, len(counts))
	for k, v := range counts {
		result[k] = math.Round(float64(v)/float64(maxVal)*100) / 100
	}
	return result
}

// topN returns the top N keys sorted by weight descending.
func topN(weights map[string]float64, n int) []string {
	type kv struct {
		key    string
		weight float64
	}
	sorted := make([]kv, 0, len(weights))
	for k, v := range weights {
		if k != "" {
			sorted = append(sorted, kv{k, v})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].weight > sorted[j].weight
	})

	result := make([]string, 0, n)
	for i := 0; i < n && i < len(sorted); i++ {
		result = append(result, sorted[i].key)
	}
	return result
}
