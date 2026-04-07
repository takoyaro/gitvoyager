package claude

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/takoyaro/gitvoyager/internal/store"
)

const (
	DefaultModel   = "haiku"
	DefaultTimeout = 30 * time.Second
	ModelEnvVar    = "GITVOYAGER_CLAUDE_MODEL"
)

// modelIDs maps short names to full model identifiers.
var modelIDs = map[string]string{
	"haiku":  "claude-haiku-4-5-20251001",
	"sonnet": "claude-sonnet-4-6",
	"opus":   "claude-opus-4-6",
}

// Client wraps the `claude` CLI for AI-powered features.
type Client struct {
	model   string
	timeout time.Duration
	store   *store.Store
	binary  string // resolved path to claude binary
}

// Config holds Claude integration settings.
type Config struct {
	Enabled        bool   `toml:"enabled"`
	Model          string `toml:"model"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
}

// DefaultConfig returns safe defaults — Haiku model, 30s timeout.
func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		Model:          DefaultModel,
		TimeoutSeconds: 30,
	}
}

// New creates a Claude client. Returns nil if disabled or binary not found.
func New(cfg Config, st *store.Store) *Client {
	if !cfg.Enabled {
		return nil
	}

	binary, err := exec.LookPath("claude")
	if err != nil {
		return nil
	}

	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	// Environment variable override
	if envModel := os.Getenv(ModelEnvVar); envModel != "" {
		model = envModel
	}

	timeout := DefaultTimeout
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	return &Client{
		model:   model,
		timeout: timeout,
		store:   st,
		binary:  binary,
	}
}

// Available returns true if the client is ready to use.
func (c *Client) Available() bool {
	return c != nil && c.binary != ""
}

// run executes `claude -p "<prompt>" --model <model>` and returns stdout.
func (c *Client) run(ctx context.Context, prompt string) (string, error) {
	modelID, ok := modelIDs[c.model]
	if !ok {
		modelID = c.model // allow raw model IDs
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binary, "-p", prompt, "--model", modelID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// getCached checks the store cache for a previous result.
func (c *Client) getCached(key string) (string, bool) {
	if c.store == nil {
		return "", false
	}
	data, ok := c.store.GetCached(key)
	if !ok {
		return "", false
	}
	return string(data), true
}

// setCache stores a result in the cache with the given TTL.
func (c *Client) setCache(key, value string, ttl time.Duration) {
	if c.store == nil {
		return
	}
	_ = c.store.SetCached(key, []byte(value), ttl, "")
}
