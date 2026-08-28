//go:build !windows

package main

import "errors"

type globalShortcutListener struct{}

func newGlobalShortcutListener(string, func()) (*globalShortcutListener, error) {
	return nil, errors.New("全局快捷键当前仅支持 Windows")
}

func (s *globalShortcutListener) Stop() {}
