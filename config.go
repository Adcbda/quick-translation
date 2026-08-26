package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultLocalModel = "Helsinki-NLP/opus-mt-en-zh"
	defaultNLLBModel  = "facebook/nllb-200-distilled-600M"
)

type Config struct {
	Provider        string `json:"provider"`
	LocalModel      string `json:"localModel"`
	LLMBaseURL      string `json:"llmBaseUrl"`
	LLMModel        string `json:"llmModel"`
	LLMAPIKey       string `json:"llmApiKey"`
	ShortcutEnabled bool   `json:"shortcutEnabled"`
	AlwaysOnTop     bool   `json:"alwaysOnTop"`
	MinimalMode     bool   `json:"minimalMode"`
	SourceLanguage  string `json:"sourceLanguage"`
	TargetLanguage  string `json:"targetLanguage"`
	DoubleTapMS     int    `json:"doubleTapMs"`
}

func defaultConfig() Config {
	return Config{
		Provider:        "local",
		LocalModel:      defaultLocalModel,
		ShortcutEnabled: true,
		SourceLanguage:  "en",
		TargetLanguage:  "zh",
		DoubleTapMS:     420,
	}
}

type configStore struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func newConfigStore() (*configStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "QuickTranslation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &configStore{path: filepath.Join(dir, "config.json"), cfg: defaultConfig()}
	data, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(data, &s.cfg); err != nil {
			return nil, fmt.Errorf("读取配置失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	s.cfg.normalize()
	return s, nil
}

func (c *Config) normalize() {
	if c.Provider != "llm" {
		c.Provider = "local"
	}
	if strings.TrimSpace(c.LocalModel) == "" {
		c.LocalModel = defaultLocalModel
	}
	if c.SourceLanguage == "" {
		c.SourceLanguage = "en"
	}
	if c.TargetLanguage == "" {
		c.TargetLanguage = "zh"
	}
	if c.DoubleTapMS < 250 || c.DoubleTapMS > 900 {
		c.DoubleTapMS = 420
	}
	if c.LLMBaseURL != "" {
		c.LLMBaseURL = strings.TrimRight(strings.TrimSpace(c.LLMBaseURL), "/")
	}
	c.LLMModel = strings.TrimSpace(c.LLMModel)
}

func (s *configStore) get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *configStore) save(cfg Config) error {
	cfg.normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Write directly because os.Rename cannot replace an existing file on Windows.
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}
