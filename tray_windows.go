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
	trayCallbackMessage = 0x8000 + 41
	trayStopMessage     = 0x8000 + 42

	trayIconID = 1

	trayNIMAdd     = 0x00000000
	trayNIMDelete  = 0x00000002
	trayNIFMessage = 0x00000001
	trayNIFIcon    = 0x00000002
	trayNIFTip     = 0x00000004

	trayWMLButtonDoubleClick = 0x0203
	trayWMRButtonUp          = 0x0205
	trayWMContextMenu        = 0x007B
	trayWMNull               = 0x0000

	trayMFString    = 0x00000000
	trayMFSeparator = 0x00000800
	trayTPMRight    = 0x0002
	trayTPMReturn   = 0x0100

	trayMenuOpen = 1
	trayMenuExit = 2

	trayAppIconResource = 3
	trayDefaultAppIcon  = 32512
)

var (
	trayShell32 = windows.NewLazySystemDLL("shell32.dll")

	trayProcRegisterClassExW    = user32.NewProc("RegisterClassExW")
	trayProcCreateWindowExW     = user32.NewProc("CreateWindowExW")
	trayProcDefWindowProcW      = user32.NewProc("DefWindowProcW")
	trayProcDestroyWindow       = user32.NewProc("DestroyWindow")
	trayProcLoadIconW           = user32.NewProc("LoadIconW")
	trayProcGetMessageW         = user32.NewProc("GetMessageW")
	trayProcTranslateMessage    = user32.NewProc("TranslateMessage")
	trayProcDispatchMessageW    = user32.NewProc("DispatchMessageW")
	trayProcPostMessageW        = user32.NewProc("PostMessageW")
	trayProcPostQuitMessage     = user32.NewProc("PostQuitMessage")
	trayProcCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	trayProcAppendMenuW         = user32.NewProc("AppendMenuW")
	trayProcTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	trayProcDestroyMenu         = user32.NewProc("DestroyMenu")
	trayProcGetCursorPos        = user32.NewProc("GetCursorPos")
	trayProcSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	trayProcShellNotifyIconW    = trayShell32.NewProc("Shell_NotifyIconW")
	trayProcGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")

	trayWindowProcCallback = windows.NewCallback(systemTrayWindowProc)
	trayWindowClassName, _ = windows.UTF16PtrFromString("QuickTranslationSystemTrayWindow")
	trayWindowClassOnce    sync.Once
	trayWindowClassErr     error
	trayWindows            sync.Map
)

type trayWindowClass struct {
	CbSize     uint32
	Style      uint32
	WindowProc uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	SmallIcon  uintptr
}

type trayNotifyIconData struct {
	CbSize          uint32
	Window          uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            windows.GUID
	BalloonIcon     uintptr
}

type trayPoint struct {
	X int32
	Y int32
}

type trayMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   trayPoint
	Private uint32
}

type systemTray struct {
	onOpen   func()
	onExit   func()
	window   uintptr
	iconData trayNotifyIconData
	stopOnce sync.Once
	done     chan struct{}
}

func newSystemTray(onOpen, onExit func()) (*systemTray, error) {
	tray := &systemTray{onOpen: onOpen, onExit: onExit, done: make(chan struct{})}
	ready := make(chan error, 1)
	go tray.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return tray, nil
}

func (s *systemTray) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)

	if err := registerTrayWindowClass(); err != nil {
		ready <- err
		return
	}
	instance, _, _ := trayProcGetModuleHandleW.Call(0)
	window, _, callErr := trayProcCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(trayWindowClassName)),
		0,
		0,
		0, 0, 0, 0,
		0, 0, instance, 0,
	)
	if window == 0 {
		ready <- fmt.Errorf("无法创建系统托盘窗口: %v", callErr)
		return
	}
	s.window = window
	trayWindows.Store(window, s)
	defer trayWindows.Delete(window)
	defer trayProcDestroyWindow.Call(window)

	icon, _, _ := trayProcLoadIconW.Call(instance, trayAppIconResource)
	if icon == 0 {
		icon, _, _ = trayProcLoadIconW.Call(0, trayDefaultAppIcon)
	}
	if icon == 0 {
		ready <- fmt.Errorf("无法加载系统托盘图标")
		return
	}
	s.iconData = trayNotifyIconData{
		CbSize:          uint32(unsafe.Sizeof(trayNotifyIconData{})),
		Window:          window,
		ID:              trayIconID,
		Flags:           trayNIFMessage | trayNIFIcon | trayNIFTip,
		CallbackMessage: trayCallbackMessage,
		Icon:            icon,
	}
	copy(s.iconData.Tip[:], windows.StringToUTF16("Quick Translation"))
	result, _, addErr := trayProcShellNotifyIconW.Call(trayNIMAdd, uintptr(unsafe.Pointer(&s.iconData)))
	if result == 0 {
		ready <- fmt.Errorf("无法添加系统托盘图标: %v", addErr)
		return
	}
	defer trayProcShellNotifyIconW.Call(trayNIMDelete, uintptr(unsafe.Pointer(&s.iconData)))
	ready <- nil

	var message trayMessage
	for {
		result, _, _ := trayProcGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		trayProcTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		trayProcDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func registerTrayWindowClass() error {
	trayWindowClassOnce.Do(func() {
		instance, _, _ := trayProcGetModuleHandleW.Call(0)
		class := trayWindowClass{
			CbSize:     uint32(unsafe.Sizeof(trayWindowClass{})),
			WindowProc: trayWindowProcCallback,
			Instance:   instance,
			ClassName:  trayWindowClassName,
		}
		result, _, err := trayProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
		if result == 0 {
			trayWindowClassErr = fmt.Errorf("无法注册系统托盘窗口: %v", err)
		}
	})
	return trayWindowClassErr
}

func systemTrayWindowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := trayWindows.Load(window)
	if found {
		tray := value.(*systemTray)
		switch message {
		case trayCallbackMessage:
			switch uint32(lParam) {
			case trayWMLButtonDoubleClick:
				go tray.onOpen()
			case trayWMRButtonUp, trayWMContextMenu:
				tray.showMenu()
			}
			return 0
		case trayStopMessage:
			trayProcPostQuitMessage.Call(0)
			return 0
		}
	}
	result, _, _ := trayProcDefWindowProcW.Call(window, uintptr(message), wParam, lParam)
	return result
}

func (s *systemTray) showMenu() {
	menu, _, _ := trayProcCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer trayProcDestroyMenu.Call(menu)
	openText, _ := windows.UTF16PtrFromString("打开翻译窗口")
	exitText, _ := windows.UTF16PtrFromString("退出")
	trayProcAppendMenuW.Call(menu, trayMFString, trayMenuOpen, uintptr(unsafe.Pointer(openText)))
	trayProcAppendMenuW.Call(menu, trayMFSeparator, 0, 0)
	trayProcAppendMenuW.Call(menu, trayMFString, trayMenuExit, uintptr(unsafe.Pointer(exitText)))

	var point trayPoint
	trayProcGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	trayProcSetForegroundWindow.Call(s.window)
	command, _, _ := trayProcTrackPopupMenu.Call(
		menu,
		trayTPMRight|trayTPMReturn,
		uintptr(point.X),
		uintptr(point.Y),
		0,
		s.window,
		0,
	)
	trayProcPostMessageW.Call(s.window, trayWMNull, 0, 0)
	switch command {
	case trayMenuOpen:
		go s.onOpen()
	case trayMenuExit:
		go s.onExit()
	}
}

func (s *systemTray) Stop() {
	s.stopOnce.Do(func() {
		if s.window != 0 {
			trayProcPostMessageW.Call(s.window, trayStopMessage, 0, 0)
		}
		select {
		case <-s.done:
		case <-time.After(1500 * time.Millisecond):
		}
	})
}
