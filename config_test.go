package main

import "testing"

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
