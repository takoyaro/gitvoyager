package local

import "time"

// ProjectFingerprint describes a local project's tech stack.
type ProjectFingerprint struct {
	Path         string       `json:"path"`
	Name         string       `json:"name"`
	Language     string       `json:"language"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
	ScannedAt    time.Time    `json:"scanned_at"`
}

// Dependency represents a single package dependency.
type Dependency struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Source   string `json:"source,omitempty"`
	RepoName string `json:"github_repo,omitempty"`
}
