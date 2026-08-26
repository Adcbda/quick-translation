//go:build !windows

package main

import "errors"

type clipboardListener struct{}

func newClipboardListener(func(string)) (*clipboardListener, error) {
	return nil, errors.New("剪贴板自动翻译当前仅支持 Windows")
}

func (s *clipboardListener) SetText(string) error {
	return errors.New("剪贴板监听当前仅支持 Windows")
}

func (s *clipboardListener) Stop() {}
