package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigNormalize(t *testing.T) {
	cfg := Config{Provider: "unknown", DoubleTapMS: 100}
	cfg.normalize()
	if cfg.Provider != "local" {
		t.Fatalf("provider = %q, want local", cfg.Provider)
	}
	if cfg.LocalModel != defaultLocalModel {
		t.Fatalf("local model = %q", cfg.LocalModel)
	}
	if cfg.SourceLanguage != "en" || cfg.TargetLanguage != "zh" {
		t.Fatalf("languages = %s -> %s", cfg.SourceLanguage, cfg.TargetLanguage)
	}
	if cfg.DoubleTapMS != 420 {
		t.Fatalf("double tap = %d", cfg.DoubleTapMS)
	}
}

func TestSafeModelName(t *testing.T) {
	got := safeModelName("facebook/nllb-200:demo")
	if got != "facebook--nllb-200-demo" {
		t.Fatalf("safeModelName = %q", got)
	}
}

func TestConfigStorePersistsMinimalMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := &configStore{path: path, cfg: defaultConfig()}
	cfg := defaultConfig()
	cfg.MinimalMode = true

	if err := store.save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if !saved.MinimalMode {
		t.Fatal("minimal mode was not persisted")
	}
}
