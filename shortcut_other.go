//go:build !windows

package main

import "errors"

type shortcutListener struct{}

func newShortcutListener(int, func()) (*shortcutListener, error) {
	return nil, errors.New("双击 Ctrl 快捷取词当前仅支持 Windows")
}

func (s *shortcutListener) Stop() {}

func captureSelectedText() (string, error) {
	return "", errors.New("快捷取词当前仅支持 Windows")
}
