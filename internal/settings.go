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
	editKey string
}

// settingsItems returns the list of settings menu items.
func settingsItems() []settingsItem {
	return []settingsItem{
		{
			label: "Event color",
			display: func(m *Model) string {
				return m.settings.EventColor
			},
			editKey: "event_color",
		},
		{
			label: "Accent color",
			display: func(m *Model) string {
				return m.settings.UIColors["accent"]
			},
			editKey: "accent",
		},
		{
			label: "Event background",
			display: func(m *Model) string {
				return m.settings.UIColors["event_bg"]
			},
			editKey: "event_bg",
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
	}
}

// updateSettings handles key events in the settings menu.
func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := settingsItems()
	itemCount := len(items)
	if m.settingsEditActive {
		switch msg.String() {
		case "esc":
			m.settingsEditActive = false
			m.settingsEditKey = ""
			m.settingsEditBuffer = ""
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.settingsEditBuffer)
			if !isHexColor(value) {
				m.statusMsg = "Use hex like #1a5fb4"
				return m, nil
			}
			switch m.settingsEditKey {
			case "event_color":
				m.settings.EventColor = value
			case "accent":
				applyAccentColor(&m, value)
			case "event_bg":
				if m.settings.UIColors == nil {
					m.settings.UIColors = map[string]string{}
				}
				m.settings.UIColors["event_bg"] = value
			}
			m.saveSettings()
			m.settingsEditActive = false
			m.settingsEditKey = ""
			m.settingsEditBuffer = ""
			return m, nil
		case "backspace":
			if len(m.settingsEditBuffer) > 0 {
				m.settingsEditBuffer = m.settingsEditBuffer[:len(m.settingsEditBuffer)-1]
			}
			return m, nil
		case "ctrl+r":
			resetSettingsEditValue(&m)
			m.saveSettings()
			m.settingsEditActive = false
			m.settingsEditKey = ""
			m.settingsEditBuffer = ""
			return m, nil
		default:
			s := msg.String()
			if pasted := sanitizeHexInput(s); pasted != "" {
				if strings.HasPrefix(pasted, "#") {
					m.settingsEditBuffer = pasted
				} else {
					m.settingsEditBuffer += pasted
				}
			}
			return m, nil
		}
	}

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
			item := items[m.settingsCursor]
			if item.editKey != "" {
				m.settingsEditActive = true
				m.settingsEditKey = item.editKey
				m.settingsEditBuffer = item.display(&m)
			} else {
				items[m.settingsCursor].action(&m)
			}
		}

	case IsKey(msg, KeyEsc), IsKey(msg, KeyQ):
		m.mode = ModeNavigate
	}

	return m, nil
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func sanitizeHexInput(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range s {
		if r == '#' {
			if i == 0 && b.Len() == 0 {
				b.WriteRune(r)
			}
			continue
		}
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func currentSettingsEditValue(m *Model) string {
	switch m.settingsEditKey {
	case "event_color":
		return m.settings.EventColor
	case "accent":
		return m.settings.UIColors["accent"]
	case "event_bg":
		return m.settings.UIColors["event_bg"]
	default:
		return ""
	}
}

func resetSettingsEditValue(m *Model) {
	switch m.settingsEditKey {
	case "event_color":
		m.settings.EventColor = DefaultEventColor
	case "accent":
		applyAccentColor(m, DefaultUIColors()["accent"])
	case "event_bg":
		if m.settings.UIColors == nil {
			m.settings.UIColors = map[string]string{}
		}
		m.settings.UIColors["event_bg"] = DefaultUIColors()["event_bg"]
	}
}

func applyAccentColor(m *Model, value string) {
	if m.settings.UIColors == nil {
		m.settings.UIColors = map[string]string{}
	}
	m.settings.UIColors["accent"] = value
	m.settings.UIColors["header_accent"] = value
	m.settings.UIColors["help_border"] = value
	m.settings.UIColors["help_section"] = value
	m.settings.UIColors["prompt_fg"] = value
	m.settings.UIColors["create_preview"] = value
}

// RenderSettings renders the fullscreen settings menu.
func RenderSettings(m *Model) string {
	items := settingsItems()
	accent := m.uiColor("accent", "39")
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Padding(0, 1)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color(m.uiColor("help_selected_bg", "236"))).Padding(0, 1)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("hint_fg", "243")))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)).Padding(0, 1)
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.uiColor("help_border", accent))).Padding(1, 2)
	panelWidth := m.width - 10
	if panelWidth > 78 {
		panelWidth = 78
	}
	if panelWidth < 40 {
		panelWidth = 40
	}
	contentWidth := panelWidth - 6
	if contentWidth < 34 {
		contentWidth = 34
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Toggle the small always-visible shortcut hints."))
	b.WriteString("\n\n")
	b.WriteString(sectionStyle.Render(" Options "))
	b.WriteString("\n")

	for i, item := range items {
		cursor := "  "
		style := itemStyle
		if i == m.settingsCursor {
			cursor = "> "
			style = selectedStyle
		}

		val := valueStyle.Render(item.display(m))
		line := fmt.Sprintf("%s%-30s  %s", cursor, item.label, val)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.settingsEditActive {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Hex color: %s_", m.settingsEditBuffer)))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Enter: save  Backspace: delete  Ctrl+r: reset default  Esc: cancel  Example: #1a5fb4"))
	} else if m.settings.ShowHints {
		b.WriteString(mutedStyle.Render("Enter on a color row to edit hex directly"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.settings.ShowHints {
		b.WriteString(mutedStyle.Render("j/k: navigate  h/l: adjust  Enter/Space: toggle  Esc/q: close"))
	}

	content := lipgloss.NewStyle().Width(contentWidth).Render(b.String())
	box := boxStyle.Render(content)
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Top, box)
}
