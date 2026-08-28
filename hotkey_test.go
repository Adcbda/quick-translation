package main

import "testing"

func TestParseGlobalShortcut(t *testing.T) {
	tests := map[string]string{
		" double control ": "DoubleCtrl",
		"DOUBLEshift":      "DoubleShift",
		"shift+ctrl+q":     "Ctrl+Shift+Q",
		"Alt+Space":        "Alt+Space",
		"ctrl+f12":         "Ctrl+F12",
	}
	for input, want := range tests {
		binding, err := parseGlobalShortcut(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if binding.canonical != want {
			t.Fatalf("parse %q = %q, want %q", input, binding.canonical, want)
		}
	}
}

func TestParseGlobalShortcutRejectsInvalidBindings(t *testing.T) {
	for _, input := range []string{"", "Q", "DoubleQ", "Ctrl+Alt+Delete", "Alt+F4", "Win+Q", "Ctrl+Q+W"} {
		if _, err := parseGlobalShortcut(input); err == nil {
			t.Fatalf("parse %q unexpectedly succeeded", input)
		}
	}
}
