package local

import "time"

// ProjectFingerprint describes a local project's tech stack.
type ProjectFingerprint struct {
	Path         string
	Name         string // directory name
	Language     string // primary language inferred from manifest
	Dependencies []Dependency
	ScannedAt    time.Time
}

// Dependency represents a single package dependency.
type Dependency struct {
	Name     string // e.g., "github.com/charmbracelet/bubbletea" or "express"
	Version  string
	Source   string // manifest file: "go.mod", "package.json", etc.
	RepoName string // mapped GitHub full_name, e.g., "charmbracelet/bubbletea"
}
