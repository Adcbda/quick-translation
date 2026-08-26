//go:build windows

package main

import "testing"

func TestPythonCommandIsUTF8AndHidden(t *testing.T) {
	cmd := pythonCommand("python")
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow is false")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("CreationFlags = %#x; CREATE_NO_WINDOW is missing", cmd.SysProcAttr.CreationFlags)
	}

	want := map[string]bool{
		"PYTHONUTF8=1":           false,
		"PYTHONIOENCODING=utf-8": false,
	}
	for _, value := range cmd.Env {
		if _, ok := want[value]; ok {
			want[value] = true
		}
	}
	for value, found := range want {
		if !found {
			t.Errorf("command environment is missing %q", value)
		}
	}
}
