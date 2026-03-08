package internal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var editMenuFields = [7]string{"Title", "Desc", "Date", "Start", "End", "Repeat", "Until"}

func editFieldHelp(field int) string {
	switch field {
	case 0:
		return "Title: free text"
	case 1:
		return "Desc: free text"
	case 2:
		return "Date: YYYY-MM-DD"
	case 3:
		return "Start: HH:MM"
	case 4:
		return "End: HH:MM (must be after start)"
	case 5:
		return "Repeat: None, Daily, Weekdays, Weekly, Biweekly, Monthly, Yearly"
	case 6:
		return "Until: YYYY-MM-DD or blank"
	default:
		return ""
	}
}

func renderRepeatHelp(current string) string {
	var b strings.Builder
	b.WriteString("Repeat options:")
	for _, opt := range RecurrenceOptions {
		label := RecurrenceLabel(opt)
		line := "# " + label
		if opt == current {
			line = label
		}
		b.WriteString("\n")
		b.WriteString("  " + line)
	}
	return StatusHintStyle.Render(b.String())
}

func renderFieldFormatHelp() string {
	lines := []string{
		"Field formats:",
		"  Title: free text",
		"  Desc: free text",
		"  Date: YYYY-MM-DD",
		"  Start: HH:MM",
		"  End: HH:MM",
		"  Until: YYYY-MM-DD or blank",
	}
	return StatusHintStyle.Render(strings.Join(lines, "\n"))
}

// updateEditMenu handles key events in the inline edit menu.
func (m Model) updateEditMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editMenuActive {
		// Currently editing a field — capture typed input
		switch {
		case IsKey(msg, KeyEsc):
			// Cancel field edit, restore previous value
			m.editMenuActive = false
			m.editMenuBuf = ""

		case IsKey(msg, KeyEnter):
			// Confirm field edit
			m.editMenuValues[m.editMenuField] = m.editMenuBuf
			m.editMenuActive = false
			m.editMenuBuf = ""

		case msg.String() == "backspace":
			if len(m.editMenuBuf) > 0 {
				m.editMenuBuf = m.editMenuBuf[:len(m.editMenuBuf)-1]
			}

		default:
			s := msg.String()
			if len(s) == 1 || s == " " {
				m.editMenuBuf += s
			}
		}
		return m, nil
	}

	// Not editing — navigate fields
	switch {
	case IsKey(msg, KeyJ):
		if m.editMenuField < len(editMenuFields)-1 {
			m.editMenuField++
		}

	case IsKey(msg, KeyK):
		if m.editMenuField > 0 {
			m.editMenuField--
		}

	case IsKey(msg, KeyEnter):
		if m.editMenuField == 5 {
			// Repeat field: cycle through recurrence options instead of text edit
			cur := 0
			for i, opt := range RecurrenceOptions {
				if opt == m.editMenuValues[5] {
					cur = i
					break
				}
			}
			cur = (cur + 1) % len(RecurrenceOptions)
			m.editMenuValues[5] = RecurrenceOptions[cur]
		} else {
			// Start editing the selected field
			m.editMenuActive = true
			m.editMenuBuf = m.editMenuValues[m.editMenuField]
		}

	case IsKey(msg, KeyEsc), IsKey(msg, KeyQ):
		// Save changes and go back to adjust mode
		err := m.applyEditMenu()
		if err != nil {
			m.statusMsg = err.Error()
			// Stay in edit menu on error
			return m, nil
		}
		m.mode = ModeAdjust
	}
	return m, nil
}

// applyEditMenu applies the edited values to the event.
func (m *Model) applyEditMenu() error {
	date := m.SelectedDate()
	events := m.store.GetByDate(date)
	if m.editMenuIndex < 0 || m.editMenuIndex >= len(events) {
		return fmt.Errorf("invalid event")
	}

	ev := events[m.editMenuIndex]

	title := strings.TrimSpace(m.editMenuValues[0])
	if title == "" {
		title = "Untitled"
	}

	desc := m.editMenuValues[1]

	// Parse date
	newDate, err := time.Parse("2006-01-02", strings.TrimSpace(m.editMenuValues[2]))
	if err != nil {
		return fmt.Errorf("invalid date (use YYYY-MM-DD)")
	}

	// Parse start time
	startMin, err := parseTime(m.editMenuValues[3])
	if err != nil {
		return fmt.Errorf("invalid start time: %v", err)
	}

	// Parse end time
	endMin, err := parseTime(m.editMenuValues[4])
	if err != nil {
		return fmt.Errorf("invalid end time: %v", err)
	}

	if endMin <= startMin {
		return fmt.Errorf("end must be after start")
	}

	// Parse recurrence
	recurrence := m.editMenuValues[5]
	recurUntil := strings.TrimSpace(m.editMenuValues[6])
	if recurUntil != "" {
		if _, err := time.Parse("2006-01-02", recurUntil); err != nil {
			return fmt.Errorf("invalid until date (use YYYY-MM-DD or leave empty)")
		}
	}

	// Find the base event by ID for safe mutation
	baseDate, baseIdx := m.store.FindEventByID(ev.ID)
	if baseIdx < 0 {
		return fmt.Errorf("event not found")
	}

	newDateKey := DateKey(newDate)
	oldDateKey := DateKey(baseDate)

	m.pushUndo()

	if newDateKey.Equal(oldDateKey) {
		// Same date — update in place using the real storage index
		m.store.events[oldDateKey][baseIdx].Title = title
		m.store.events[oldDateKey][baseIdx].Desc = desc
		m.store.events[oldDateKey][baseIdx].StartMin = startMin
		m.store.events[oldDateKey][baseIdx].EndMin = endMin
		m.store.events[oldDateKey][baseIdx].Recurrence = recurrence
		m.store.events[oldDateKey][baseIdx].RecurUntilStr = recurUntil
		// Re-sync adjustIndex from GetByDate (order may have changed)
		events = m.store.GetByDate(date)
		for i, e := range events {
			if e.ID == ev.ID {
				m.adjustIndex = i
				break
			}
		}
	} else {
		// Different date — delete by ID and re-add
		baseEv := m.store.events[oldDateKey][baseIdx]
		baseEv.Title = title
		baseEv.Desc = desc
		baseEv.StartMin = startMin
		baseEv.EndMin = endMin
		baseEv.Date = newDateKey
		baseEv.Recurrence = recurrence
		baseEv.RecurUntilStr = recurUntil

		m.store.DeleteByID(ev.ID)
		m.store.Add(baseEv)

		// Move cursor to new date
		curDateKey := DateKey(date)
		diff := int(newDateKey.Sub(curDateKey).Hours() / 24)
		newCol := m.cursorCol + diff
		if newCol >= 0 && newCol < m.dayCount {
			m.cursorCol = newCol
		} else {
			// Shift window to show new date
			halfWay := m.dayCount / 2
			m.windowStart = newDateKey.AddDate(0, 0, -halfWay)
			m.cursorCol = halfWay
		}
		// Find new event index
		newEvents := m.store.GetByDate(newDateKey)
		m.adjustIndex = len(newEvents) - 1
	}

	m.cursorMin = startMin
	m.saveEvents()
	m.ensureCursorVisible()
	return nil
}

// RenderEditMenu renders the inline event editor.
func RenderEditMenu(m *Model) string {
	accent := m.uiColor("accent", "39")
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(accent)).
		MarginBottom(1)

	fieldStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255"))

	selectedFieldStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("236"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	editingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("24"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit Event"))
	b.WriteString("\n\n")

	for i, label := range editMenuFields {
		cursor := "  "
		style := fieldStyle
		if i == m.editMenuField {
			cursor = "> "
			style = selectedFieldStyle
		}

		var val string
		displayVal := m.editMenuValues[i]
		// Show human-readable label for recurrence field
		if i == 5 {
			displayVal = RecurrenceLabel(m.editMenuValues[i])
		}

		if m.editMenuActive && i == m.editMenuField {
			// Show the edit buffer with a cursor
			val = editingStyle.Render(m.editMenuBuf + "_")
		} else {
			val = valueStyle.Render(displayVal)
		}

		hint := ""
		if i == 5 && i == m.editMenuField {
			hint = "  (Enter: cycle)"
		}

		line := fmt.Sprintf("%s%-12s %s%s", cursor, label+":", val, hint)
		b.WriteString(style.Render(line))
		b.WriteString("\n")

		if i == m.editMenuField {
			b.WriteString(StatusHintStyle.Render("    " + editFieldHelp(i)))
			b.WriteString("\n")
			if i == 5 {
				b.WriteString(renderRepeatHelp(m.editMenuValues[5]))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(StatusHintStyle.Render("j/k: field  Enter: edit  Esc/q: save & back"))
	b.WriteString("\n")
	b.WriteString(renderFieldFormatHelp())

	return b.String()
}
