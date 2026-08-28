//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL  = 13
	hcAction      = 0
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	wmQuit        = 0x0012
	pmNoRemove    = 0x0000
	llkhfInjected = 0x00000010

	vkLShift   = 0xA0
	vkRShift   = 0xA1
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkLAlt     = 0xA4
	vkRAlt     = 0xA5
)

var (
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPeekMessageW        = user32.NewProc("PeekMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	kernel32Hotkey          = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentThreadID  = kernel32Hotkey.NewProc("GetCurrentThreadId")
)

type keyboardHookData struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type messagePoint struct {
	X int32
	Y int32
}

type threadMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   messagePoint
	Private uint32
}

type globalShortcutListener struct {
	binding   globalShortcutBinding
	onTrigger func()
	stopOnce  sync.Once
	done      chan struct{}
	threadID  uint32

	pressed         map[uint32]bool
	doublePressed   bool
	doubleChorded   bool
	doublePressedAt time.Time
	lastTap         time.Time
	lastTriggered   time.Time
}

func newGlobalShortcutListener(shortcut string, onTrigger func()) (*globalShortcutListener, error) {
	binding, err := parseGlobalShortcut(shortcut)
	if err != nil {
		return nil, err
	}
	listener := &globalShortcutListener{
		binding:   binding,
		onTrigger: onTrigger,
		done:      make(chan struct{}),
		pressed:   make(map[uint32]bool),
	}
	ready := make(chan error, 1)
	go listener.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return listener, nil
}

func (s *globalShortcutListener) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)

	threadID, _, _ := procGetCurrentThreadID.Call()
	s.threadID = uint32(threadID)
	var message threadMessage
	// Force Windows to create this thread's message queue before Stop can post
	// WM_QUIT to it.
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, pmNoRemove)
	callback := windows.NewCallback(s.keyboardProc)
	hook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, callback, 0, 0)
	if hook == 0 {
		ready <- fmt.Errorf("无法注册全局快捷键监听: %v", callErr)
		return
	}
	defer procUnhookWindowsHookEx.Call(hook)
	ready <- nil

	for {
		result, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			_ = err
			return
		}
		if result == 0 {
			return
		}
	}
}

func (s *globalShortcutListener) keyboardProc(code int, wParam uintptr, data *keyboardHookData) uintptr {
	if code == hcAction {
		if data.Flags&llkhfInjected == 0 {
			isDown := wParam == wmKeyDown || wParam == wmSysKeyDown
			isUp := wParam == wmKeyUp || wParam == wmSysKeyUp
			if (isDown || isUp) && s.handleKey(data.VKCode, isDown) {
				return 1
			}
		}
	}
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, uintptr(unsafe.Pointer(data)))
	return result
}

// handleKey returns true only for the ordinary key-down that completes a
// combination. Swallowing that one event prevents the shortcut from also
// acting in the foreground application. Double-modifier taps always pass
// through unchanged.
func (s *globalShortcutListener) handleKey(vk uint32, isDown bool) bool {
	wasDown := s.pressed[vk]
	if isDown {
		s.pressed[vk] = true
	} else {
		delete(s.pressed, vk)
	}

	if s.binding.doubleModifier != 0 {
		s.handleDoubleModifier(vk, isDown, wasDown)
		return false
	}
	if !isDown || vk != s.binding.key {
		return false
	}
	if s.currentModifiers() != s.binding.modifiers {
		return false
	}
	if !wasDown {
		s.trigger()
	}
	return true
}

func (s *globalShortcutListener) handleDoubleModifier(vk uint32, isDown bool, wasDown bool) {
	if !sameModifier(vk, s.binding.doubleModifier) {
		if isDown && !wasDown {
			s.lastTap = time.Time{}
			if s.doublePressed {
				s.doubleChorded = true
			}
		}
		return
	}
	if isDown {
		if !wasDown && !s.modifierIsPressed(s.binding.doubleModifier, vk) {
			s.doublePressed = true
			s.doubleChorded = false
			s.doublePressedAt = time.Now()
		}
		return
	}
	if s.modifierIsPressed(s.binding.doubleModifier, 0) {
		return
	}
	if !s.doublePressed {
		return
	}
	s.doublePressed = false
	if s.doubleChorded {
		s.doubleChorded = false
		s.lastTap = time.Time{}
		return
	}
	now := time.Now()
	if now.Sub(s.doublePressedAt) > 500*time.Millisecond {
		s.lastTap = time.Time{}
		return
	}
	if !s.lastTap.IsZero() && now.Sub(s.lastTap) >= 40*time.Millisecond && now.Sub(s.lastTap) <= 500*time.Millisecond {
		s.lastTap = time.Time{}
		s.trigger()
		return
	}
	s.lastTap = now
}

func (s *globalShortcutListener) modifierIsPressed(modifier uint32, except uint32) bool {
	for vk := range s.pressed {
		if vk != except && sameModifier(vk, modifier) {
			return true
		}
	}
	return false
}

func sameModifier(vk uint32, modifier uint32) bool {
	switch modifier {
	case vkControl:
		return vk == vkControl || vk == vkLControl || vk == vkRControl
	case vkShift:
		return vk == vkShift || vk == vkLShift || vk == vkRShift
	case vkAlt:
		return vk == vkAlt || vk == vkLAlt || vk == vkRAlt
	default:
		return false
	}
}

func (s *globalShortcutListener) currentModifiers() uint8 {
	var modifiers uint8
	if asyncKeyDown(vkControl) {
		modifiers |= shortcutModifierCtrl
	}
	if asyncKeyDown(vkShift) {
		modifiers |= shortcutModifierShift
	}
	if asyncKeyDown(vkAlt) {
		modifiers |= shortcutModifierAlt
	}
	return modifiers
}

func asyncKeyDown(vk uint32) bool {
	state, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return uint16(state)&0x8000 != 0
}

func (s *globalShortcutListener) trigger() {
	now := time.Now()
	if now.Sub(s.lastTriggered) < 250*time.Millisecond {
		return
	}
	s.lastTriggered = now
	go s.onTrigger()
}

func (s *globalShortcutListener) Stop() {
	s.stopOnce.Do(func() {
		procPostThreadMessageW.Call(uintptr(s.threadID), wmQuit, 0, 0)
		select {
		case <-s.done:
		case <-time.After(1500 * time.Millisecond):
		}
	})
}
