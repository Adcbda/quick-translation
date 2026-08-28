//go:build !windows

package main

import "errors"

type systemTray struct{}

func newSystemTray(func(), func()) (*systemTray, error) {
	return nil, errors.New("关闭到系统托盘当前仅支持 Windows")
}

func (s *systemTray) Stop() {}
