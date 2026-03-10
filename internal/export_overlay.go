package internal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) openExportOverlay() {
	m.exportReturnMode = m.mode
	m.exportReturnView = m.viewMode
	m.mode = ModeExport
	m.exportMatches = nil
	if m.exportSummary == nil {
		m.exportSummary = []string{}
	}
}

func (m *Model) closeExportOverlay() {
	m.mode = m.exportReturnMode
	m.viewMode = m.exportReturnView
	if m.mode == 0 {
		m.mode = ModeNavigate
	}
}

func (m Model) updateExport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc), IsKey(msg, KeyQ):
		m.closeExportOverlay()
		return m, nil

	case IsKey(msg, KeyTab):
		m.applyExportCompletion()
		return m, nil

	case IsKey(msg, KeyEnter):
		m.exportMatches = nil
		path := strings.TrimSpace(m.exportPath)
		result, err := ExportICSFile(path, m.store)
		if err != nil {
			m.exportSummary = []string{fmt.Sprintf("Export failed: %v", err)}
			return m, nil
		}
		m.exportSummary = formatExportSummary(result)
		m.mode = ModeNavigate
		m.viewMode = ViewWeek
		m.settingsSearchActive = false
		m.settingsSearchQuery = ""
		return m, m.setTimedStatus(exportStatusMessage(result), 4*time.Second)

	case msg.String() == "backspace":
		if len(m.exportPath) > 0 {
			m.exportPath = m.exportPath[:len(m.exportPath)-1]
		}
		m.exportMatches = nil
		return m, nil

	case msg.String() == "ctrl+w":
		m.exportPath = deletePreviousWord(m.exportPath)
		m.exportMatches = nil
		return m, nil
	}

	if len(msg.Runes) > 0 {
		m.exportPath += string(msg.Runes)
		m.exportMatches = nil
	}
	return m, nil
}

func (m *Model) applyExportCompletion() {
	completed, matches, err := completeICSPath(m.exportPath, true)
	if err != nil {
		m.exportMatches = nil
		m.exportSummary = []string{fmt.Sprintf("Path completion failed: %v", err)}
		return
	}
	if len(matches) == 0 {
		m.exportMatches = nil
		m.exportSummary = []string{"No matching folders or .ics files found."}
		return
	}
	m.exportPath = completed
	m.exportMatches = matches
	if len(matches) == 1 {
		m.exportSummary = []string{fmt.Sprintf("Completed path: %s", completed)}
		m.exportMatches = nil
		return
	}
	m.exportSummary = []string{fmt.Sprintf("%d matches. Press Tab again after typing more.", len(matches))}
}

func RenderExport(m *Model) string {
	accent := m.uiColor("accent", "39")
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("hint_fg", "243")))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.uiColor("help_border", accent))).Padding(1, 2)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)).Padding(0, 1)

	panelWidth := m.width - 10
	if panelWidth > 84 {
		panelWidth = 84
	}
	if panelWidth < 40 {
		panelWidth = 40
	}
	contentWidth := panelWidth - 6
	if contentWidth < 34 {
		contentWidth = 34
	}

	var body []string
	body = append(body, titleStyle.Render("Export Events"))
	body = append(body, mutedStyle.Render("Type a relative or absolute .ics path manually. Press Tab to complete folders and .ics files, then Enter to export."))
	body = append(body, "")
	body = append(body, sectionStyle.Render(" Export "))
	body = append(body, fmt.Sprintf("  Action   %s", valueStyle.Render("Export events")))
	body = append(body, fmt.Sprintf("  File     %s_", m.exportPath))
	body = append(body, "")
	if len(m.exportMatches) > 0 {
		body = append(body, sectionStyle.Render(" Matches "))
		limit := len(m.exportMatches)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			body = append(body, "  "+m.exportMatches[i])
		}
		if len(m.exportMatches) > limit {
			body = append(body, fmt.Sprintf("  ...and %d more", len(m.exportMatches)-limit))
		}
		body = append(body, "")
	}
	if len(m.exportSummary) > 0 {
		body = append(body, sectionStyle.Render(" Results "))
		body = append(body, m.exportSummary...)
		body = append(body, "")
	}
	body = append(body, mutedStyle.Render("Tab: complete path  Enter: export  Ctrl+w: delete word  Backspace: delete  Esc/q: close"))

	content := lipgloss.NewStyle().Width(contentWidth).Render(strings.Join(body, "\n"))
	box := boxStyle.Render(content)
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(box)
}

func formatExportSummary(result ExportResult) []string {
	return []string{
		fmt.Sprintf("File: %s", result.OutputPath),
		fmt.Sprintf("Exported %d event(s).", result.Exported),
	}
}

func exportStatusMessage(result ExportResult) string {
	return fmt.Sprintf("Exported %d event(s)", result.Exported)
}
