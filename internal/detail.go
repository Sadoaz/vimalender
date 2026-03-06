package internal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// updateDetail handles keys in detail view mode.
func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc), IsKey(msg, KeyQ):
		m.mode = ModeNavigate
		m.detailIndex = -1

	case IsKey(msg, KeyE):
		// Open editor from detail view
		date := m.SelectedDate()
		idx := m.detailIndex
		if idx >= 0 {
			m.mode = ModeNavigate
			m.detailIndex = -1
			return m, m.openEditor(date, idx)
		}
	}
	return m, nil
}

// RenderDetail renders the fullscreen event detail view.
func RenderDetail(m *Model) string {
	date := m.SelectedDate()
	events := m.store.GetByDate(date)
	if m.detailIndex < 0 || m.detailIndex >= len(events) {
		return "Event not found"
	}

	ev := events[m.detailIndex]
	maxWidth := m.width - 4
	if maxWidth < 20 {
		maxWidth = 20
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(DetailTitleStyle.Render(fmt.Sprintf("  %s  ", ev.Title)))
	sb.WriteString("\n\n")
	if ev.Desc != "" {
		sb.WriteString(fmt.Sprintf("  %s\n\n", ev.Desc))
	}
	sb.WriteString(fmt.Sprintf("  Date:     %s\n", ev.Date.Format("Monday, January 02, 2006")))
	sb.WriteString(fmt.Sprintf("  Time:     %s - %s\n", MinToTime(ev.StartMin), MinToTime(ev.EndMin)))

	duration := ev.EndMin - ev.StartMin
	hours := duration / 60
	mins := duration % 60
	if hours > 0 {
		sb.WriteString(fmt.Sprintf("  Duration: %dh %dm\n", hours, mins))
	} else {
		sb.WriteString(fmt.Sprintf("  Duration: %dm\n", mins))
	}

	if ev.Notes != "" {
		sb.WriteString("\n  Notes:\n")
		// Word wrap notes
		for _, line := range strings.Split(ev.Notes, "\n") {
			wrapped := wordWrap(line, maxWidth-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				sb.WriteString("    " + wl + "\n")
			}
		}
	}

	sb.WriteString("\n")
	sb.WriteString(StatusHintStyle.Render("  e: edit  Esc/q: back"))
	sb.WriteString("\n")

	return sb.String()
}

// wordWrap wraps text at the given width.
func wordWrap(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	var lines []string
	for len(text) > width {
		// Find last space before width
		idx := strings.LastIndex(text[:width], " ")
		if idx <= 0 {
			idx = width
		}
		lines = append(lines, text[:idx])
		text = strings.TrimLeft(text[idx:], " ")
	}
	if text != "" {
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}
