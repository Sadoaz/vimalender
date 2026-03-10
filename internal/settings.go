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
			label: "Default zoom",
			display: func(m *Model) string {
				return fmt.Sprintf("%d min/row", m.settings.ZoomLevel)
			},
			action: func(m *Model) {
				cycleSettingsZoom(m, 1)
			},
			actionL: func(m *Model) {
				cycleSettingsZoom(m, -1)
			},
			actionR: func(m *Model) {
				cycleSettingsZoom(m, 1)
			},
		},
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
			label: "Consecutive event color",
			display: func(m *Model) string {
				return m.settings.UIColors["consecutive_color"]
			},
			editKey: "consecutive_color",
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
		{
			label: "Import events",
			display: func(m *Model) string {
				return "open"
			},
			action: func(m *Model) {
				m.openImportOverlay()
			},
		},
		{
			label: "Export events",
			display: func(m *Model) string {
				return "open"
			},
			action: func(m *Model) {
				m.openExportOverlay()
			},
		},
	}
}

func settingsVisibleIndices(items []settingsItem, m *Model, query string) []int {
	if strings.TrimSpace(query) == "" {
		indices := make([]int, len(items))
		for i := range items {
			indices[i] = i
		}
		return indices
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var indices []int
	for i, item := range items {
		label := strings.ToLower(item.label)
		value := strings.ToLower(item.display(m))
		if strings.Contains(label, query) || strings.Contains(value, query) {
			indices = append(indices, i)
		}
	}
	return indices
}

func moveSettingsCursor(m *Model, delta int, items []settingsItem) {
	visible := settingsVisibleIndices(items, m, m.settingsSearchQuery)
	if len(visible) == 0 {
		m.settingsCursor = 0
		return
	}
	pos := 0
	for i, idx := range visible {
		if idx == m.settingsCursor {
			pos = i
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(visible) {
		pos = len(visible) - 1
	}
	m.settingsCursor = visible[pos]
}

func clampSettingsCursorToVisible(m *Model, items []settingsItem) {
	visible := settingsVisibleIndices(items, m, m.settingsSearchQuery)
	if len(visible) == 0 {
		m.settingsCursor = 0
		return
	}
	for _, idx := range visible {
		if idx == m.settingsCursor {
			return
		}
	}
	m.settingsCursor = visible[0]
}

func cycleSettingsZoom(m *Model, dir int) {
	idx := 0
	for i, level := range ZoomLevels {
		if level == m.settings.ZoomLevel {
			idx = i
			break
		}
	}
	idx += dir
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ZoomLevels) {
		idx = len(ZoomLevels) - 1
	}
	m.settings.ZoomLevel = ZoomLevels[idx]
	m.applyZoomLevel(m.settings.ZoomLevel)
	m.saveSettings()
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
			case "consecutive_color":
				if m.settings.UIColors == nil {
					m.settings.UIColors = map[string]string{}
				}
				m.settings.UIColors["consecutive_color"] = value
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

	if m.settingsSearchActive {
		switch msg.String() {
		case "esc":
			m.settingsSearchActive = false
			m.settingsSearchQuery = ""
			m.statusMsg = ""
			clampSettingsCursorToVisible(&m, items)
			return m, nil
		case "enter":
			m.settingsSearchActive = false
			clampSettingsCursorToVisible(&m, items)
			return m, nil
		case "backspace":
			if len(m.settingsSearchQuery) > 0 {
				m.settingsSearchQuery = m.settingsSearchQuery[:len(m.settingsSearchQuery)-1]
			}
			clampSettingsCursorToVisible(&m, items)
			return m, nil
		case "ctrl+w":
			m.settingsSearchQuery = deletePreviousWord(m.settingsSearchQuery)
			clampSettingsCursorToVisible(&m, items)
			return m, nil
		default:
			s := msg.String()
			if len([]rune(s)) == 1 || s == " " {
				m.settingsSearchQuery += s
				clampSettingsCursorToVisible(&m, items)
			}
			return m, nil
		}
	}

	switch {
	case IsKey(msg, KeySlash):
		m.settingsSearchActive = true
		m.settingsSearchQuery = ""
		return m, nil

	case IsKey(msg, KeyJ):
		moveSettingsCursor(&m, 1, items)

	case IsKey(msg, KeyK):
		moveSettingsCursor(&m, -1, items)

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
	case "consecutive_color":
		return m.settings.UIColors["consecutive_color"]
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
	case "consecutive_color":
		if m.settings.UIColors == nil {
			m.settings.UIColors = map[string]string{}
		}
		m.settings.UIColors["consecutive_color"] = DefaultUIColors()["consecutive_color"]
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
	visible := settingsVisibleIndices(items, m, m.settingsSearchQuery)
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
	b.WriteString(mutedStyle.Render("Adjust preferences or open event import from here."))
	b.WriteString("\n\n")
	b.WriteString(sectionStyle.Render(" Options "))
	b.WriteString("\n")

	for _, i := range visible {
		item := items[i]
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
	if len(visible) == 0 {
		b.WriteString(mutedStyle.Render("  No matching settings"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.settingsEditActive {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Hex color: %s_", m.settingsEditBuffer)))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Enter: save  Backspace: delete  Ctrl+r: reset default  Esc: cancel  Example: #1a5fb4"))
	} else if m.settingsSearchActive {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("/%s_", m.settingsSearchQuery)))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Enter: keep filter  Backspace: delete  Ctrl+w: delete word  Esc: clear"))
	} else if m.settings.ShowHints {
		b.WriteString(mutedStyle.Render("Enter on a color row to edit hex directly  /: search settings"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.settings.ShowHints {
		b.WriteString(mutedStyle.Render("j/k: navigate  h/l: adjust  Enter/Space: toggle or open  /: search  Esc/q: close"))
	}

	content := lipgloss.NewStyle().Width(contentWidth).Render(b.String())
	box := boxStyle.Render(content)
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(box)
}
