package internal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// settingsItem describes a single setting in the menu.
type settingsItem struct {
	label   string
	display func(m *Model) string // current value display
	action  func(m *Model)        // Enter/Space action (toggle/cycle)
	actionL func(m *Model)        // h key action (decrease/previous)
	actionR func(m *Model)        // l key action (increase/next)
}

// jumpPercentOptions are the available jump percent values.
var jumpPercentOptions = []int{2, 3, 5, 8, 10, 15, 20, 25}

// settingsItems returns the list of settings menu items.
func settingsItems() []settingsItem {
	return []settingsItem{
		{
			label: "Zoom level",
			display: func(m *Model) string {
				if m.zoomLevel == ZoomAuto {
					return "auto"
				}
				return fmt.Sprintf("%d min/row", m.zoomLevel)
			},
			action: func(m *Model) {
				// Toggle: reset to auto
				m.zoomLevel = ZoomAuto
				m.viewportOffset = m.dayStartMin()
				m.saveSettings()
			},
			actionL: func(m *Model) {
				m.zoomOut()
				m.saveSettings()
			},
			actionR: func(m *Model) {
				m.zoomIn()
				m.saveSettings()
			},
		},
		{
			label: "Show keybinding hints",
			display: func(m *Model) string {
				if m.settings.ShowHints {
					return "on"
				}
				return "off"
			},
			action: func(m *Model) {
				m.settings.ShowHints = !m.settings.ShowHints
				m.saveSettings()
			},
		},
		{
			label: "Jump step (% of view)",
			display: func(m *Model) string {
				return fmt.Sprintf("%d%%", m.settings.JumpPercent)
			},
			action: func(m *Model) {
				// Cycle forward through options
				cycleJumpPercent(m, 1)
			},
			actionL: func(m *Model) {
				cycleJumpPercent(m, -1)
			},
			actionR: func(m *Model) {
				cycleJumpPercent(m, 1)
			},
		},
		{
			label: "Event colors",
			display: func(m *Model) string {
				return fmt.Sprintf("%d colors", len(m.settings.EventColors))
			},
			action: func(m *Model) {
				// Reset to defaults
				m.settings.EventColors = DefaultEventColors
				m.saveSettings()
			},
		},
		{
			label: "Show event borders",
			display: func(m *Model) string {
				if m.settings.ShowBorders {
					return "on"
				}
				return "off"
			},
			action: func(m *Model) {
				m.settings.ShowBorders = !m.settings.ShowBorders
				m.saveSettings()
			},
		},
		{
			label: "Show descriptions",
			display: func(m *Model) string {
				if m.settings.ShowDescs {
					return "on"
				}
				return "off"
			},
			action: func(m *Model) {
				m.settings.ShowDescs = !m.settings.ShowDescs
				m.saveSettings()
			},
		},
		{
			label: "Quick create (skip recurrence)",
			display: func(m *Model) string {
				if m.settings.QuickCreate {
					return "on"
				}
				return "off"
			},
			action: func(m *Model) {
				m.settings.QuickCreate = !m.settings.QuickCreate
				m.saveSettings()
			},
		},
		{
			label: "Skip description",
			display: func(m *Model) string {
				if m.settings.SkipDesc {
					return "on"
				}
				return "off"
			},
			action: func(m *Model) {
				m.settings.SkipDesc = !m.settings.SkipDesc
				m.saveSettings()
			},
		},
		{
			label: "Day start hour",
			display: func(m *Model) string {
				return fmt.Sprintf("%02d:00", m.settings.DayStartHour)
			},
			action: func(m *Model) {
				// Reset to default (08:00)
				m.settings.DayStartHour = 8
				m.ensureCursorVisible()
				m.saveSettings()
			},
			actionL: func(m *Model) {
				if m.settings.DayStartHour > 0 {
					m.settings.DayStartHour--
					m.ensureCursorVisible()
					m.saveSettings()
				}
			},
			actionR: func(m *Model) {
				if m.settings.DayStartHour < 23 {
					m.settings.DayStartHour++
					m.ensureCursorVisible()
					m.saveSettings()
				}
			},
		},
	}
}

// cycleJumpPercent cycles the jump percent through predefined options.
func cycleJumpPercent(m *Model, dir int) {
	current := m.settings.JumpPercent
	idx := 0
	for i, v := range jumpPercentOptions {
		if v == current {
			idx = i
			break
		}
		if v > current {
			idx = i
			break
		}
	}
	idx += dir
	if idx < 0 {
		idx = 0
	}
	if idx >= len(jumpPercentOptions) {
		idx = len(jumpPercentOptions) - 1
	}
	m.settings.JumpPercent = jumpPercentOptions[idx]
	m.saveSettings()
}

// updateSettings handles key events in the settings menu.
func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := settingsItems()
	itemCount := len(items)

	switch {
	case IsKey(msg, KeyJ):
		m.settingsCursor++
		if m.settingsCursor >= itemCount {
			m.settingsCursor = itemCount - 1
		}

	case IsKey(msg, KeyK):
		m.settingsCursor--
		if m.settingsCursor < 0 {
			m.settingsCursor = 0
		}

	case IsKey(msg, KeyH):
		if m.settingsCursor >= 0 && m.settingsCursor < itemCount {
			item := items[m.settingsCursor]
			if item.actionL != nil {
				item.actionL(&m)
			} else if item.action != nil {
				item.action(&m)
			}
		}

	case IsKey(msg, KeyL):
		if m.settingsCursor >= 0 && m.settingsCursor < itemCount {
			item := items[m.settingsCursor]
			if item.actionR != nil {
				item.actionR(&m)
			} else if item.action != nil {
				item.action(&m)
			}
		}

	case IsKey(msg, KeyEnter), IsKey(msg, KeySpace):
		if m.settingsCursor >= 0 && m.settingsCursor < itemCount {
			items[m.settingsCursor].action(&m)
		}

	case IsKey(msg, KeyEsc), IsKey(msg, KeyQ):
		m.mode = ModeNavigate
	}

	return m, nil
}

// Settings menu styles (local to this file).
var (
	settingsTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				MarginBottom(1)

	settingsItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	settingsSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("236"))

	settingsValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true)
)

// RenderSettings renders the fullscreen settings menu.
func RenderSettings(m *Model) string {
	items := settingsItems()

	var b strings.Builder
	b.WriteString(settingsTitleStyle.Render("Settings"))
	b.WriteString("\n\n")

	for i, item := range items {
		cursor := "  "
		style := settingsItemStyle
		if i == m.settingsCursor {
			cursor = "> "
			style = settingsSelectedStyle
		}

		val := settingsValueStyle.Render(item.display(m))
		line := fmt.Sprintf("%s%-30s  %s", cursor, item.label, val)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	// Show color swatches for event colors
	b.WriteString("\n")
	b.WriteString(settingsTitleStyle.Render("Color Palette"))
	b.WriteString("\n")
	for i, hex := range m.settings.EventColors {
		swatch := lipgloss.NewStyle().
			Background(lipgloss.Color(hex)).
			Render("  ")
		label := fmt.Sprintf(" %d: %s", i+1, hex)
		b.WriteString(fmt.Sprintf("  %s%s\n", swatch, StatusHintStyle.Render(label)))
	}

	b.WriteString("\n")
	b.WriteString(StatusHintStyle.Render("j/k: navigate  h/l: adjust  Enter/Space: toggle/cycle  Esc/q: close"))
	b.WriteString("\n")
	b.WriteString(StatusHintStyle.Render("Edit event_colors in ~/.local/share/vimalender/settings.json for custom hex colors"))

	// Center vertically
	content := b.String()
	contentLines := strings.Count(content, "\n") + 1
	topPad := (m.height - contentLines - 2) / 3
	if topPad < 0 {
		topPad = 0
	}

	return strings.Repeat("\n", topPad) + content
}
