package local

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// parseGoMod extracts dependencies from a go.mod file.
func parseGoMod(path string) ([]Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var deps []Dependency
	inRequire := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "require (") || line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}

		// Single-line require
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				deps = append(deps, Dependency{
					Name:    parts[1],
					Version: parts[2],
					Source:  "go.mod",
				})
			}
			continue
		}

		if inRequire {
			// Skip comments and indirect
			if strings.HasPrefix(line, "//") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := parts[0]
				version := parts[1]
				// Skip indirect dependencies
				if strings.Contains(line, "// indirect") {
					continue
				}
				deps = append(deps, Dependency{
					Name:    name,
					Version: version,
					Source:  "go.mod",
				})
			}
		}
	}

	return deps, scanner.Err()
}

// parsePackageJSON extracts dependencies from a package.json file.
func parsePackageJSON(path string) ([]Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, version := range pkg.Dependencies {
		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "package.json",
		})
	}
	// Include dev dependencies too — they reveal tooling preferences
	for name, version := range pkg.DevDependencies {
		deps = append(deps, Dependency{
			Name:    name,
			Version: version,
			Source:  "package.json",
		})
	}

	return deps, nil
}

// parseCargoToml extracts dependencies from a Cargo.toml file.
func parseCargoToml(path string) ([]Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var deps []Dependency
	inDeps := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Detect [dependencies] or [dev-dependencies] section
		if strings.HasPrefix(line, "[") {
			lower := strings.ToLower(line)
			inDeps = lower == "[dependencies]" || lower == "[dev-dependencies]"
			continue
		}

		if inDeps && line != "" && !strings.HasPrefix(line, "#") {
			// Parse "name = version" or "name = { version = "..." }"
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			name := strings.TrimSpace(parts[0])
			versionPart := strings.TrimSpace(parts[1])

			version := ""
			// Simple version string: "1.0"
			if strings.HasPrefix(versionPart, "\"") {
				version = strings.Trim(versionPart, "\"")
			}
			// Table syntax: { version = "1.0", ... }
			if strings.HasPrefix(versionPart, "{") {
				if idx := strings.Index(versionPart, "version"); idx >= 0 {
					rest := versionPart[idx:]
					if eqIdx := strings.Index(rest, "="); eqIdx >= 0 {
						verStr := strings.TrimSpace(rest[eqIdx+1:])
						// Extract quoted string
						if len(verStr) > 0 && verStr[0] == '"' {
							end := strings.Index(verStr[1:], "\"")
							if end >= 0 {
								version = verStr[1 : end+1]
							}
						}
					}
				}
			}

			deps = append(deps, Dependency{
				Name:    name,
				Version: version,
				Source:  "Cargo.toml",
			})
		}
	}

	return deps, scanner.Err()
}

// parseRequirementsTxt extracts dependencies from a requirements.txt file.
func parseRequirementsTxt(path string) ([]Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var deps []Dependency
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}

		// Parse "package==version", "package>=version", "package"
		name := line
		version := ""
		for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<"} {
			if idx := strings.Index(line, sep); idx >= 0 {
				name = strings.TrimSpace(line[:idx])
				version = strings.TrimSpace(line[idx+len(sep):])
				break
			}
		}
		// Strip extras like [security]
		if idx := strings.Index(name, "["); idx >= 0 {
			name = name[:idx]
		}

		if name != "" {
			deps = append(deps, Dependency{
				Name:    name,
				Version: version,
				Source:  "requirements.txt",
			})
		}
	}

	return deps, scanner.Err()
}

// parsePyprojectToml extracts dependencies from a pyproject.toml file.
func parsePyprojectToml(path string) ([]Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var deps []Dependency
	inDeps := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "dependencies = [" || line == "dependencies= [" {
			inDeps = true
			continue
		}
		if inDeps && line == "]" {
			inDeps = false
			continue
		}
		if inDeps {
			// Parse "package>=version", "package"
			dep := strings.Trim(line, "\",")
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			name := dep
			version := ""
			for _, sep := range []string{">=", "<=", "==", "~=", "!=", ">", "<"} {
				if idx := strings.Index(dep, sep); idx >= 0 {
					name = strings.TrimSpace(dep[:idx])
					version = strings.TrimSpace(dep[idx+len(sep):])
					break
				}
			}
			if name != "" {
				deps = append(deps, Dependency{
					Name:    name,
					Version: version,
					Source:  "pyproject.toml",
				})
			}
		}
	}

	return deps, scanner.Err()
}
