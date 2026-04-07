package local

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scanner walks directories to discover local projects and their dependencies.
type Scanner struct {
	paths    []string
	maxDepth int
}

// New creates a scanner for the given directories.
func New(paths []string) *Scanner {
	return &Scanner{
		paths:    paths,
		maxDepth: 3,
	}
}

// Scan walks all configured paths and returns discovered projects.
func (s *Scanner) Scan() ([]ProjectFingerprint, error) {
	var projects []ProjectFingerprint
	seen := make(map[string]bool)

	for _, root := range s.paths {
		expanded := expandHome(root)
		if _, err := os.Stat(expanded); err != nil {
			continue
		}
		found, err := s.scanDir(expanded, 0, seen)
		if err != nil {
			continue
		}
		projects = append(projects, found...)
	}
	return projects, nil
}

// ScanDirectory scans a single directory for project manifests.
func (s *Scanner) ScanDirectory(path string) (*ProjectFingerprint, error) {
	expanded := expandHome(path)
	return detectProject(expanded)
}

func (s *Scanner) scanDir(dir string, depth int, seen map[string]bool) ([]ProjectFingerprint, error) {
	if depth > s.maxDepth {
		return nil, nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if seen[absDir] {
		return nil, nil
	}
	seen[absDir] = true

	var projects []ProjectFingerprint

	// Check if this directory is itself a project
	if fp, err := detectProject(absDir); err == nil && fp != nil {
		projects = append(projects, *fp)
		// Don't recurse into detected projects (they have their own structure)
		return projects, nil
	}

	// Recurse into subdirectories
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden dirs, vendor dirs, node_modules, etc.
		if strings.HasPrefix(name, ".") || isSkipDir(name) {
			continue
		}
		subProjects, err := s.scanDir(filepath.Join(absDir, name), depth+1, seen)
		if err != nil {
			continue
		}
		projects = append(projects, subProjects...)
	}

	return projects, nil
}

// detectProject checks if a directory contains a recognizable project manifest.
func detectProject(dir string) (*ProjectFingerprint, error) {
	// Order matters: check most specific first
	manifests := []struct {
		file     string
		language string
		parser   func(string) ([]Dependency, error)
	}{
		{"go.mod", "Go", parseGoMod},
		{"Cargo.toml", "Rust", parseCargoToml},
		{"package.json", "JavaScript", parsePackageJSON},
		{"pyproject.toml", "Python", parsePyprojectToml},
		{"requirements.txt", "Python", parseRequirementsTxt},
	}

	for _, m := range manifests {
		manifestPath := filepath.Join(dir, m.file)
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}

		deps, err := m.parser(manifestPath)
		if err != nil {
			deps = nil // still register the project even if parsing fails
		}

		// Map dependencies to GitHub repos
		for i := range deps {
			deps[i].RepoName = MapToGitHub(deps[i])
		}

		return &ProjectFingerprint{
			Path:         dir,
			Name:         filepath.Base(dir),
			Language:     m.language,
			Dependencies: deps,
			ScannedAt:    time.Now(),
		}, nil
	}

	return nil, nil
}

func isSkipDir(name string) bool {
	skip := map[string]bool{
		"node_modules": true, "vendor": true, "target": true,
		"dist": true, "build": true, "__pycache__": true,
		"venv": true, ".venv": true, "env": true,
	}
	return skip[name]
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
