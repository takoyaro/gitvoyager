package local

import "strings"

// MapToGitHub attempts to resolve a dependency to a GitHub owner/repo.
func MapToGitHub(dep Dependency) string {
	switch dep.Source {
	case "go.mod":
		return mapGoModule(dep.Name)
	case "package.json":
		return mapNPMPackage(dep.Name)
	case "Cargo.toml":
		return mapCrate(dep.Name)
	default:
		return ""
	}
}

// mapGoModule maps a Go module path to GitHub owner/repo.
// Go modules often use github.com/owner/repo directly.
func mapGoModule(modulePath string) string {
	if strings.HasPrefix(modulePath, "github.com/") {
		parts := strings.SplitN(modulePath, "/", 4)
		if len(parts) >= 3 {
			return parts[1] + "/" + parts[2]
		}
	}
	// Other known hosting: golang.org/x/foo -> golang/foo
	if strings.HasPrefix(modulePath, "golang.org/x/") {
		name := strings.TrimPrefix(modulePath, "golang.org/x/")
		return "golang/" + name
	}
	// charm.land/X -> charmbracelet/X
	if strings.HasPrefix(modulePath, "charm.land/") {
		parts := strings.SplitN(strings.TrimPrefix(modulePath, "charm.land/"), "/", 2)
		if len(parts) >= 1 {
			name := parts[0]
			return "charmbracelet/" + name
		}
	}
	return ""
}

// mapNPMPackage maps an npm package name to a GitHub repo.
// Uses common patterns; not exhaustive.
func mapNPMPackage(name string) string {
	// Scoped packages: @scope/name -> scope/name
	if strings.HasPrefix(name, "@") {
		cleaned := strings.TrimPrefix(name, "@")
		parts := strings.SplitN(cleaned, "/", 2)
		if len(parts) == 2 {
			return parts[0] + "/" + parts[1]
		}
	}
	return ""
}

// mapCrate maps a Rust crate name to a GitHub repo.
// Crate names don't embed repo URLs, so mapping is limited.
func mapCrate(name string) string {
	// Well-known crates
	known := map[string]string{
		"tokio":    "tokio-rs/tokio",
		"serde":    "serde-rs/serde",
		"reqwest":  "seanmonstar/reqwest",
		"clap":     "clap-rs/clap",
		"axum":     "tokio-rs/axum",
		"warp":     "seanmonstar/warp",
		"actix-web": "actix/actix-web",
		"rocket":   "rwf2/Rocket",
	}
	if repo, ok := known[name]; ok {
		return repo
	}
	return ""
}
