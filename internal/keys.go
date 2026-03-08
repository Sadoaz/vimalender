package internal

import tea "github.com/charmbracelet/bubbletea"

// Key constants for all modes.
const (
	KeyH        = "h"
	KeyJ        = "j"
	KeyK        = "k"
	KeyL        = "l"
	KeyR        = "r"
	KeyShiftJ   = "J"
	KeyShiftK   = "K"
	KeyShiftH   = "H"
	KeyShiftL   = "L"
	KeyShiftR   = "R"
	KeyQ        = "q"
	KeyA        = "a"
	KeyD        = "d"
	KeyE        = "e"
	KeyX        = "x"
	KeyY        = "y"
	KeyN        = "n"
	KeyShiftN   = "N"
	KeyM        = "m"
	KeyShiftM   = "M"
	KeyP        = "p"
	KeyShiftV   = "V"
	KeyShiftD   = "D"
	KeyShiftY   = "Y" // year view
	KeyShiftS   = "S" // open settings menu
	KeyC        = "c" // center on current time (now-line)
	KeyG        = "g"
	KeyShiftG   = "G" // jump to day of month with count prefix
	KeySlash    = "/"
	KeyEnter    = "enter"
	KeyEsc      = "esc"
	KeyPlus     = "+" // zoom in
	KeyMinus    = "-" // zoom out
	KeyEquals   = "=" // reset to default view
	Key0        = "0" // reset to default view
	KeySpace    = " "
	KeyTab      = "tab"    // cycle overlapping events
	KeyS        = "s"      // edit menu in adjust mode
	KeyU        = "u"      // undo
	KeyCtrlD    = "ctrl+d" // half page down
	KeyCtrlU    = "ctrl+u" // half page up
	KeyCtrlP    = "ctrl+p" // previous search match
	KeyCtrlN    = "ctrl+n" // next search match
	KeyCtrlR    = "ctrl+r" // redo
	KeyQuestion = "?"
)

var keyBindingOverrides = map[string]string{}

func DefaultKeybindings() map[string]string {
	keys := []string{
		Key0,
		KeyA,
		KeyC,
		KeyCtrlD,
		KeyCtrlN,
		KeyCtrlP,
		KeyCtrlR,
		KeyCtrlU,
		KeyD,
		KeyE,
		KeyEnter,
		KeyEquals,
		KeyEsc,
		KeyG,
		KeyH,
		KeyJ,
		KeyK,
		KeyL,
		KeyM,
		KeyMinus,
		KeyN,
		KeyP,
		KeyPlus,
		KeyQ,
		KeyQuestion,
		KeyR,
		KeyS,
		KeyShiftG,
		KeyShiftH,
		KeyShiftJ,
		KeyShiftK,
		KeyShiftL,
		KeyShiftM,
		KeyShiftN,
		KeyShiftR,
		KeyShiftS,
		KeyShiftV,
		KeyShiftY,
		KeySlash,
		KeySpace,
		KeyTab,
		KeyU,
		KeyX,
		KeyY,
	}
	b := make(map[string]string, len(keys))
	for _, key := range keys {
		b[key] = key
	}
	return b
}

func SetKeyBindingOverrides(overrides map[string]string) {
	keyBindingOverrides = map[string]string{}
	for k, v := range overrides {
		if v != "" {
			keyBindingOverrides[k] = v
		}
	}
}

func DisplayKey(key string) string {
	if v, ok := keyBindingOverrides[key]; ok && v != "" {
		return v
	}
	return key
}

// IsKey checks if a key message matches the given key string.
func IsKey(msg tea.KeyMsg, key string) bool {
	return msg.String() == DisplayKey(key)
}
