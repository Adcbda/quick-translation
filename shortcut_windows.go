//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL   = 13
	wmKeyDown      = 0x0100
	wmKeyUp        = 0x0101
	wmSysKeyDown   = 0x0104
	wmSysKeyUp     = 0x0105
	wmQuit         = 0x0012
	vkControl      = 0x11
	vkLControl     = 0xA2
	vkRControl     = 0xA3
	vkC            = 0x43
	keyeventfKeyUp = 0x0002
	llkhfInjected  = 0x00000010
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowsHookExW  = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx     = user32.NewProc("CallNextHookEx")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	procKeybdEvent         = user32.NewProc("keybd_event")
	procOpenClipboard      = user32.NewProc("OpenClipboard")
	procCloseClipboard     = user32.NewProc("CloseClipboard")
	procEmptyClipboard     = user32.NewProc("EmptyClipboard")
	procGetClipboardData   = user32.NewProc("GetClipboardData")
	procSetClipboardData   = user32.NewProc("SetClipboardData")
	procIsClipboardFormat  = user32.NewProc("IsClipboardFormatAvailable")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
	procGlobalAlloc        = kernel32.NewProc("GlobalAlloc")
	procGlobalLock         = kernel32.NewProc("GlobalLock")
	procGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	procGlobalFree         = kernel32.NewProc("GlobalFree")
)

type keyboardHookData struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type winMessage struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   struct{ X, Y int32 }
	Private uint32
}

type shortcutListener struct {
	doubleTap time.Duration
	onTrigger func()
	stopOnce  sync.Once
	done      chan struct{}
	threadID  uint32
	hook      uintptr
	callback  uintptr
	ctrlDown  bool
	downAt    time.Time
	lastTap   time.Time
}

func newShortcutListener(doubleTapMS int, onTrigger func()) (*shortcutListener, error) {
	listener := &shortcutListener{
		doubleTap: time.Duration(doubleTapMS) * time.Millisecond,
		onTrigger: onTrigger,
		done:      make(chan struct{}),
	}
	ready := make(chan error, 1)
	go listener.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return listener, nil
}

func (s *shortcutListener) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)

	threadID, _, _ := procGetCurrentThreadID.Call()
	s.threadID = uint32(threadID)
	s.callback = syscall.NewCallback(func(nCode int, wParam uintptr, lParam uintptr) uintptr {
		if nCode >= 0 && lParam != 0 {
			data := (*keyboardHookData)(unsafe.Pointer(lParam))
			if data.Flags&llkhfInjected == 0 && (data.VKCode == vkControl || data.VKCode == vkLControl || data.VKCode == vkRControl) {
				now := time.Now()
				switch wParam {
				case wmKeyDown, wmSysKeyDown:
					if !s.ctrlDown {
						s.ctrlDown = true
						s.downAt = now
					}
				case wmKeyUp, wmSysKeyUp:
					if s.ctrlDown {
						s.ctrlDown = false
						if now.Sub(s.downAt) <= 360*time.Millisecond {
							if !s.lastTap.IsZero() && now.Sub(s.lastTap) <= s.doubleTap {
								s.lastTap = time.Time{}
								go s.onTrigger()
							} else {
								s.lastTap = now
							}
						}
					}
				}
			}
		}
		result, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
		return result
	})
	hook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, s.callback, 0, 0)
	if hook == 0 {
		ready <- fmt.Errorf("注册双击 Ctrl 监听失败: %v", callErr)
		return
	}
	s.hook = hook
	ready <- nil
	defer procUnhookWindowsHook.Call(hook)

	var msg winMessage
	for {
		result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (s *shortcutListener) Stop() {
	s.stopOnce.Do(func() {
		if s.threadID != 0 {
			procPostThreadMessageW.Call(uintptr(s.threadID), wmQuit, 0, 0)
		}
		select {
		case <-s.done:
		case <-time.After(1500 * time.Millisecond):
		}
	})
}

func captureSelectedText() (string, error) {
	previous, _ := readClipboardText()
	marker := fmt.Sprintf("quick-translation-%d", time.Now().UnixNano())
	if err := writeClipboardText(marker); err != nil {
		return "", err
	}
	defer func() { _ = writeClipboardText(previous) }()

	procKeybdEvent.Call(vkControl, 0, 0, 0)
	procKeybdEvent.Call(vkC, 0, 0, 0)
	procKeybdEvent.Call(vkC, 0, keyeventfKeyUp, 0)
	procKeybdEvent.Call(vkControl, 0, keyeventfKeyUp, 0)

	deadline := time.Now().Add(650 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(35 * time.Millisecond)
		value, err := readClipboardText()
		if err == nil && value != marker {
			return value, nil
		}
	}
	return "", errors.New("未能读取选中文本")
}

func openClipboard() error {
	for i := 0; i < 12; i++ {
		ok, _, err := procOpenClipboard.Call(0)
		if ok != 0 {
			return nil
		}
		if i == 11 {
			return fmt.Errorf("无法打开剪贴板: %v", err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	return errors.New("无法打开剪贴板")
}

func readClipboardText() (string, error) {
	available, _, _ := procIsClipboardFormat.Call(cfUnicodeText)
	if available == 0 {
		return "", nil
	}
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()
	handle, _, err := procGetClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", err
	}
	ptr, _, err := procGlobalLock.Call(handle)
	if ptr == 0 {
		return "", err
	}
	defer procGlobalUnlock.Call(handle)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr))), nil
}

func writeClipboardText(value string) error {
	utf16, err := windows.UTF16FromString(value)
	if err != nil {
		return err
	}
	size := uintptr(len(utf16) * 2)
	handle, _, allocErr := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return allocErr
	}
	owned := false
	defer func() {
		if !owned {
			procGlobalFree.Call(handle)
		}
	}()
	ptr, _, lockErr := procGlobalLock.Call(handle)
	if ptr == 0 {
		return lockErr
	}
	destination := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(destination, utf16)
	procGlobalUnlock.Call(handle)

	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
		return err
	}
	if result, _, err := procSetClipboardData.Call(cfUnicodeText, handle); result == 0 {
		return err
	}
	owned = true
	return nil
}
