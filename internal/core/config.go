package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultBaseURL is the xAI OpenAI-compatible chat completions root.
const DefaultBaseURL = "https://api.x.ai/v1"

// Config holds runtime settings for the agent. Secrets are never logged.
type Config struct {
	APIKey  string // xAI / Grok API key (from env or ~/.grok/config.json)
	BaseURL string // e.g. https://api.x.ai/v1
	// PollIntervalSec is Mode B mailbox poll cadence (protocol: 2–5s).
	PollIntervalSec int
	// HTTPTimeoutSec bounds a single HTTP round-trip.
	HTTPTimeoutSec int
}

// grokFileConfig matches the minimal ~/.grok/config.json shape we care about.
// Field names are flexible across Grok Build variants.
type grokFileConfig struct {
	APIKey    string `json:"api_key"`
	XAIAPIKey string `json:"xai_api_key"`
	GrokKey   string `json:"grok_api_key"`
	Key       string `json:"key"`
	BaseURL   string `json:"base_url"`
	BaseURL2  string `json:"baseUrl"`
}

// LoadConfig resolves API key and base URL from environment first, then
// config files under %USERPROFILE%\.grok\config.json and ~/.grok/config.json.
//
// Env (first non-empty wins for key):
//   GBR_API_KEY, XAI_API_KEY
//
// Env for base URL:
//   GBR_BASE_URL, XAI_BASE_URL
func LoadConfig() (*Config, error) {
	cfg := &Config{
		BaseURL:         DefaultBaseURL,
		PollIntervalSec: 3,
		HTTPTimeoutSec:  60,
	}

	if v := firstNonEmpty(os.Getenv("GBR_BASE_URL"), os.Getenv("XAI_BASE_URL")); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}

	if v := firstNonEmpty(os.Getenv("GBR_API_KEY"), os.Getenv("XAI_API_KEY")); v != "" {
		cfg.APIKey = strings.TrimSpace(v)
	}

	// File fallback when env did not supply a key (and optionally base URL).
	if cfg.APIKey == "" || cfg.BaseURL == DefaultBaseURL {
		if fc, path, err := loadGrokConfigFile(); err == nil {
			_ = path // reserved for future debug logging of source path only
			if cfg.APIKey == "" {
				cfg.APIKey = strings.TrimSpace(fc.pickKey())
			}
			if b := fc.pickBaseURL(); b != "" && cfg.BaseURL == DefaultBaseURL {
				// Only override default if env did not set a custom base URL.
				if os.Getenv("GBR_BASE_URL") == "" && os.Getenv("XAI_BASE_URL") == "" {
					cfg.BaseURL = strings.TrimRight(b, "/")
				}
			}
		}
	}

	if cfg.APIKey == "" {
		return nil, errors.New("api key not found: set GBR_API_KEY or XAI_API_KEY, or put api_key in %USERPROFILE%\\.grok\\config.json")
	}

	return cfg, nil
}

func (f *grokFileConfig) pickKey() string {
	return firstNonEmpty(f.APIKey, f.XAIAPIKey, f.GrokKey, f.Key)
}

func (f *grokFileConfig) pickBaseURL() string {
	return firstNonEmpty(f.BaseURL, f.BaseURL2)
}

// loadGrokConfigFile tries Windows USERPROFILE path then HOME (~).
func loadGrokConfigFile() (*grokFileConfig, string, error) {
	var candidates []string

	if up := os.Getenv("USERPROFILE"); up != "" {
		candidates = append(candidates, filepath.Join(up, ".grok", "config.json"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		p := filepath.Join(home, ".grok", "config.json")
		// Avoid duplicate if USERPROFILE == home
		if len(candidates) == 0 || candidates[0] != p {
			candidates = append(candidates, p)
		}
	}
	// Explicit ~ expansion is not needed: UserHomeDir covers Unix/macOS.

	var lastErr error
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		var fc grokFileConfig
		if err := json.Unmarshal(data, &fc); err != nil {
			return nil, p, fmt.Errorf("parse %s: %w", p, err)
		}
		return &fc, p, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no config path available")
	}
	return nil, "", lastErr
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// RedactKey returns a safe-to-log preview of an API key.
func RedactKey(key string) string {
	k := strings.TrimSpace(key)
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "…" + k[len(k)-4:]
}
