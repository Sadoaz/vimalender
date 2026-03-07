package internal

import tea "github.com/charmbracelet/bubbletea"

// Key constants for all modes.
const (
	KeyH      = "h"
	KeyJ      = "j"
	KeyK      = "k"
	KeyL      = "l"
	KeyShiftJ = "J"
	KeyShiftK = "K"
	KeyShiftH = "H"
	KeyShiftL = "L"
	KeyQ      = "q"
	KeyA      = "a"
	KeyD      = "d"
	KeyE      = "e"
	KeyX      = "x"
	KeyY      = "y"
	KeyN      = "n"
	KeyShiftN = "N"
	KeyM      = "m"
	KeyShiftM = "M"
	KeyShiftY = "Y" // year view
	KeyShiftS = "S" // open settings menu
	KeyC      = "c" // center on current time (now-line)
	KeyG      = "g"
	KeyShiftG = "G" // jump to day of month with count prefix
	KeySlash  = "/"
	KeyEnter  = "enter"
	KeyEsc    = "esc"
	KeyPlus   = "+" // zoom in
	KeyMinus  = "-" // zoom out
	KeySpace  = " "
	KeyTab    = "tab"    // cycle overlapping events
	KeyS      = "s"      // edit menu in adjust mode
	KeyU      = "u"      // undo
	KeyCtrlD  = "ctrl+d" // half page down
	KeyCtrlU  = "ctrl+u" // half page up
	KeyCtrlP  = "ctrl+p" // previous search match
	KeyCtrlN  = "ctrl+n" // next search match
	KeyCtrlR  = "ctrl+r" // redo
)

// IsKey checks if a key message matches the given key string.
func IsKey(msg tea.KeyMsg, key string) bool {
	return msg.String() == key
}
