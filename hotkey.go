package main

import (
	"fmt"
	"strconv"
	"strings"
)

const defaultQuickOpenShortcut = "DoubleCtrl"

const (
	shortcutModifierCtrl uint8 = 1 << iota
	shortcutModifierShift
	shortcutModifierAlt
)

// Virtual-key values are part of the Windows API and are kept here so parsing
// and validation can be tested on every platform.
const (
	vkBackspace = 0x08
	vkTab       = 0x09
	vkEnter     = 0x0D
	vkShift     = 0x10
	vkControl   = 0x11
	vkAlt       = 0x12
	vkEscape    = 0x1B
	vkSpace     = 0x20
	vkPageUp    = 0x21
	vkPageDown  = 0x22
	vkEnd       = 0x23
	vkHome      = 0x24
	vkLeft      = 0x25
	vkUp        = 0x26
	vkRight     = 0x27
	vkDown      = 0x28
	vkInsert    = 0x2D
	vkDelete    = 0x2E
	vkF1        = 0x70
)

type globalShortcutBinding struct {
	canonical      string
	doubleModifier uint32
	modifiers      uint8
	key            uint32
}

var namedShortcutKeys = map[string]struct {
	name string
	vk   uint32
}{
	"backspace": {"Backspace", vkBackspace},
	"tab":       {"Tab", vkTab},
	"enter":     {"Enter", vkEnter},
	"return":    {"Enter", vkEnter},
	"escape":    {"Escape", vkEscape},
	"esc":       {"Escape", vkEscape},
	"space":     {"Space", vkSpace},
	"pageup":    {"PageUp", vkPageUp},
	"pagedown":  {"PageDown", vkPageDown},
	"end":       {"End", vkEnd},
	"home":      {"Home", vkHome},
	"left":      {"Left", vkLeft},
	"up":        {"Up", vkUp},
	"right":     {"Right", vkRight},
	"down":      {"Down", vkDown},
	"insert":    {"Insert", vkInsert},
	"delete":    {"Delete", vkDelete},
}

func parseGlobalShortcut(value string) (globalShortcutBinding, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return globalShortcutBinding{}, fmt.Errorf("快捷键不能为空")
	}
	lower := strings.ToLower(strings.ReplaceAll(value, " ", ""))
	if strings.HasPrefix(lower, "double") {
		switch strings.TrimPrefix(lower, "double") {
		case "ctrl", "control":
			return globalShortcutBinding{canonical: "DoubleCtrl", doubleModifier: vkControl}, nil
		case "shift":
			return globalShortcutBinding{canonical: "DoubleShift", doubleModifier: vkShift}, nil
		case "alt":
			return globalShortcutBinding{canonical: "DoubleAlt", doubleModifier: vkAlt}, nil
		default:
			return globalShortcutBinding{}, fmt.Errorf("双击快捷键仅支持 Ctrl、Shift 或 Alt")
		}
	}

	parts := strings.Split(lower, "+")
	if len(parts) < 2 {
		return globalShortcutBinding{}, fmt.Errorf("组合快捷键需要至少一个修饰键")
	}
	var binding globalShortcutBinding
	var keyName string
	for _, part := range parts {
		if part == "" {
			return globalShortcutBinding{}, fmt.Errorf("快捷键格式不正确")
		}
		switch part {
		case "ctrl", "control":
			if binding.modifiers&shortcutModifierCtrl != 0 {
				return globalShortcutBinding{}, fmt.Errorf("快捷键包含重复按键")
			}
			binding.modifiers |= shortcutModifierCtrl
		case "shift":
			if binding.modifiers&shortcutModifierShift != 0 {
				return globalShortcutBinding{}, fmt.Errorf("快捷键包含重复按键")
			}
			binding.modifiers |= shortcutModifierShift
		case "alt", "option":
			if binding.modifiers&shortcutModifierAlt != 0 {
				return globalShortcutBinding{}, fmt.Errorf("快捷键包含重复按键")
			}
			binding.modifiers |= shortcutModifierAlt
		case "win", "windows", "meta", "super":
			return globalShortcutBinding{}, fmt.Errorf("暂不支持使用 Windows 键作为快捷键")
		default:
			if binding.key != 0 {
				return globalShortcutBinding{}, fmt.Errorf("组合快捷键只能包含一个普通按键")
			}
			name, vk, ok := parseShortcutKey(part)
			if !ok {
				return globalShortcutBinding{}, fmt.Errorf("不支持快捷键按键 %q", part)
			}
			keyName, binding.key = name, vk
		}
	}
	if binding.modifiers == 0 || binding.key == 0 {
		return globalShortcutBinding{}, fmt.Errorf("组合快捷键需要修饰键和一个普通按键")
	}

	canonical := make([]string, 0, 4)
	if binding.modifiers&shortcutModifierCtrl != 0 {
		canonical = append(canonical, "Ctrl")
	}
	if binding.modifiers&shortcutModifierShift != 0 {
		canonical = append(canonical, "Shift")
	}
	if binding.modifiers&shortcutModifierAlt != 0 {
		canonical = append(canonical, "Alt")
	}
	canonical = append(canonical, keyName)
	binding.canonical = strings.Join(canonical, "+")
	if binding.canonical == "Ctrl+Alt+Delete" || binding.canonical == "Alt+F4" ||
		binding.canonical == "Alt+Tab" || binding.canonical == "Alt+Escape" || binding.canonical == "Ctrl+Escape" {
		return globalShortcutBinding{}, fmt.Errorf("%s 是系统保留快捷键，请选择其他组合", binding.canonical)
	}
	return binding, nil
}

func parseShortcutKey(value string) (string, uint32, bool) {
	if len(value) == 1 {
		ch := value[0]
		if ch >= 'a' && ch <= 'z' {
			return strings.ToUpper(value), uint32(ch - 'a' + 'A'), true
		}
		if ch >= '0' && ch <= '9' {
			return value, uint32(ch), true
		}
	}
	if len(value) >= 2 && value[0] == 'f' {
		if number, err := strconv.Atoi(value[1:]); err == nil && number >= 1 && number <= 12 {
			return fmt.Sprintf("F%d", number), vkF1 + uint32(number-1), true
		}
	}
	key, ok := namedShortcutKeys[value]
	return key.name, key.vk, ok
}

func normalizeQuickOpenShortcut(value string) string {
	binding, err := parseGlobalShortcut(value)
	if err != nil {
		return defaultQuickOpenShortcut
	}
	return binding.canonical
}
