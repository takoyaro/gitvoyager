package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"
)

type Config struct {
	Search  SearchConfig  `toml:"search"`
	Cache   CacheConfig   `toml:"cache"`
	Display DisplayConfig `toml:"display"`
	Clone   CloneConfig   `toml:"clone"`
	Claude  ClaudeConfig  `toml:"claude"`
	Local   LocalConfig   `toml:"local"`
}

type LocalConfig struct {
	Enabled   bool     `toml:"enabled"`
	ScanPaths []string `toml:"scan_paths"`
	AutoScan  bool     `toml:"auto_scan"`
}

type ClaudeConfig struct {
	Enabled        bool   `toml:"enabled"`
	Model          string `toml:"model"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
}

type SearchConfig struct {
	DefaultLimit int    `toml:"default_limit"`
	DefaultSort  string `toml:"default_sort"`
	DebounceMs   int    `toml:"debounce_ms"`
}

type CacheConfig struct {
	SearchTTLMinutes int `toml:"search_ttl_minutes"`
	ReadmeTTLMinutes int `toml:"readme_ttl_minutes"`
}

type DisplayConfig struct {
	ListWidthPercent int `toml:"list_width_percent"`
}

type CloneConfig struct {
	DefaultDirectory string `toml:"default_directory"`
	Protocol         string `toml:"protocol"`
}

func Default() *Config {
	return &Config{
		Search: SearchConfig{
			DefaultLimit: 30,
			DefaultSort:  "stars",
			DebounceMs:   300,
		},
		Cache: CacheConfig{
			SearchTTLMinutes: 15,
			ReadmeTTLMinutes: 1440,
		},
		Display: DisplayConfig{
			ListWidthPercent: 35,
		},
		Clone: CloneConfig{
			Protocol: "ssh",
		},
		Claude: ClaudeConfig{
			Enabled:        true,
			Model:          "haiku",
			TimeoutSeconds: 30,
		},
		Local: LocalConfig{
			Enabled:   false, // opt-in
			ScanPaths: []string{"~/Projects"},
			AutoScan:  false,
		},
	}
}

func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, "gitvoyager")
}

func DataDir() string {
	return filepath.Join(xdg.DataHome, "gitvoyager")
}

func CacheDir() string {
	return filepath.Join(xdg.CacheHome, "gitvoyager")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

func DBPath() string {
	return filepath.Join(DataDir(), "gitvoyager.db")
}

func Load() (*Config, error) {
	cfg := Default()

	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
