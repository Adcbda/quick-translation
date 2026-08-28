package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigNormalize(t *testing.T) {
	cfg := Config{Provider: "unknown"}
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
}

func TestConfigMigratesLegacyShortcutSetting(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"shortcutEnabled":false}`), &cfg); err != nil {
		t.Fatalf("decode legacy config: %v", err)
	}
	if cfg.ClipboardEnabled {
		t.Fatal("legacy disabled shortcut should keep clipboard monitoring disabled")
	}
}

func TestSafeModelName(t *testing.T) {
	got := safeModelName("facebook/nllb-200:demo")
	if got != "facebook--nllb-200-demo" {
		t.Fatalf("safeModelName = %q", got)
	}
}

func TestConfigStorePersistsAppearanceSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := &configStore{path: path, cfg: defaultConfig()}
	cfg := defaultConfig()
	cfg.MinimalMode = true
	cfg.DarkMode = true
	cfg.CloseToTray = true

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
	if !saved.DarkMode {
		t.Fatal("dark mode was not persisted")
	}
	if !saved.CloseToTray {
		t.Fatal("close-to-tray setting was not persisted")
	}
}

func TestConfigDefaultsQuickOpenShortcut(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"provider":"local"}`), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if !cfg.QuickOpenEnabled {
		t.Fatal("quick open should be enabled by default")
	}
	if cfg.QuickOpenShortcut != defaultQuickOpenShortcut {
		t.Fatalf("quick open shortcut = %q", cfg.QuickOpenShortcut)
	}
}

func TestConfigStorePersistsQuickOpenSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := &configStore{path: path, cfg: defaultConfig()}
	cfg := defaultConfig()
	cfg.QuickOpenEnabled = false
	cfg.QuickOpenShortcut = "shift+ctrl+q"

	if err := store.save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var saved Config
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if saved.QuickOpenEnabled {
		t.Fatal("disabled quick open setting was not persisted")
	}
	if saved.QuickOpenShortcut != "Ctrl+Shift+Q" {
		t.Fatalf("quick open shortcut = %q", saved.QuickOpenShortcut)
	}
}
