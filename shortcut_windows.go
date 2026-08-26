//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procIsClipboardFormat          = user32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
)

// clipboardListener watches the Windows clipboard sequence number. Using the
// sequence number avoids repeatedly reading or translating an unchanged value.
type clipboardListener struct {
	onChange func(string)
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}

	mu           sync.Mutex
	lastSequence uint32
	ignoredText  string
}

func newClipboardListener(onChange func(string)) (*clipboardListener, error) {
	if err := procGetClipboardSequenceNumber.Find(); err != nil {
		return nil, fmt.Errorf("系统不支持剪贴板监听: %w", err)
	}
	listener := &clipboardListener{
		onChange:     onChange,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		lastSequence: clipboardSequenceNumber(),
	}
	go listener.run()
	return listener, nil
}

func clipboardSequenceNumber() uint32 {
	sequence, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(sequence)
}

func (s *clipboardListener) run() {
	defer close(s.done)
	ticker := time.NewTicker(180 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.checkForChange()
		case <-s.stop:
			return
		}
	}
}

func (s *clipboardListener) checkForChange() {
	s.mu.Lock()
	sequence := clipboardSequenceNumber()
	if sequence == 0 {
		s.mu.Unlock()
		return
	}
	if sequence == s.lastSequence {
		s.mu.Unlock()
		return
	}
	text, err := readClipboardText()
	if err != nil {
		// Do not advance lastSequence so a temporarily locked clipboard is retried.
		s.mu.Unlock()
		return
	}
	// If another application copied again while the clipboard was being read,
	// wait for the next tick and read the newest value instead.
	if clipboardSequenceNumber() != sequence {
		s.mu.Unlock()
		return
	}
	s.lastSequence = sequence
	if s.ignoredText != "" && text == s.ignoredText {
		s.ignoredText = ""
		s.mu.Unlock()
		return
	}
	s.ignoredText = ""
	s.mu.Unlock()

	if strings.TrimSpace(text) != "" {
		s.onChange(text)
	}
}

// SetText writes text without feeding that application-originated change back
// into automatic translation (used by the "copy result" button).
func (s *clipboardListener) SetText(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeClipboardText(value); err != nil {
		return err
	}
	sequence := clipboardSequenceNumber()
	if sequence != 0 {
		s.lastSequence = sequence
		s.ignoredText = ""
	} else {
		s.ignoredText = value
	}
	return nil
}

func (s *clipboardListener) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
		select {
		case <-s.done:
		case <-time.After(1500 * time.Millisecond):
		}
	})
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
