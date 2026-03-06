package internal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateMonth handles keys in month view mode.
func (m Model) updateMonth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyQ):
		return m, tea.Quit

	case IsKey(msg, KeyH):
		// Move cursor one day left
		m.monthCursor = m.monthCursor.AddDate(0, 0, -1)

	case IsKey(msg, KeyL):
		// Move cursor one day right
		m.monthCursor = m.monthCursor.AddDate(0, 0, 1)

	case IsKey(msg, KeyJ):
		// Move cursor one week down
		m.monthCursor = m.monthCursor.AddDate(0, 0, 7)

	case IsKey(msg, KeyK):
		// Move cursor one week up
		m.monthCursor = m.monthCursor.AddDate(0, 0, -7)

	case IsKey(msg, KeyShiftH):
		// Move one month back
		m.monthCursor = m.monthCursor.AddDate(0, -1, 0)

	case IsKey(msg, KeyShiftL):
		// Move one month forward
		m.monthCursor = m.monthCursor.AddDate(0, 1, 0)

	case IsKey(msg, KeyC):
		// Jump to today
		now := time.Now()
		m.monthCursor = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	case IsKey(msg, KeyShiftY):
		// Switch to year view
		m.viewMode = ViewYear
		m.mode = ModeYear
		m.yearCursor = m.monthCursor

	case IsKey(msg, KeyEnter):
		// Switch to week view with selected day on the far left
		m.viewMode = ViewWeek
		m.mode = ModeNavigate
		m.windowStart = m.monthCursor
		m.cursorCol = 0

	case IsKey(msg, KeyShiftM):
		// Toggle back to week view, preserving cursor date
		m.viewMode = ViewWeek
		m.mode = ModeNavigate
		m.windowStart = m.monthCursor
		m.cursorCol = 0

	case IsKey(msg, KeyEsc):
		// Also go back to week view
		m.viewMode = ViewWeek
		m.mode = ModeNavigate
		m.windowStart = m.monthCursor
		m.cursorCol = 0
	}
	return m, nil
}

// RenderMonth renders a full-screen month calendar grid.
// Each day cell fills available space proportionally.
func RenderMonth(m *Model) string {
	cursor := m.monthCursor
	year, month, _ := cursor.Date()
	loc := cursor.Location()

	// First day of the month
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	// Last day of the month
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	daysInMonth := lastOfMonth.Day()

	// Weekday of first day (Monday=0 .. Sunday=6)
	wd := int(firstOfMonth.Weekday())
	wd = (wd + 6) % 7

	today := DateKey(time.Now())
	selectedDay := DateKey(cursor)

	// Calculate number of rows needed (weeks in the grid)
	totalGridSlots := wd + daysInMonth
	numWeeks := (totalGridSlots + 6) / 7

	// Layout: header(1) + daynames(1) + gap(1) + grid rows
	availHeight := m.height - 2 // reserve for status bar
	headerHeight := 3           // month title + day names + gap
	gridHeight := availHeight - headerHeight
	if gridHeight < numWeeks {
		gridHeight = numWeeks
	}

	// Row height for each week
	rowHeight := gridHeight / numWeeks
	if rowHeight < 1 {
		rowHeight = 1
	}

	// Column width for each day (leave space for week number gutter)
	weekNumWidth := 4
	contentWidth := m.width - weekNumWidth
	cellWidth := contentWidth / 7
	if cellWidth < 6 {
		cellWidth = 6
	}

	// Styles
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dayNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	todayNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#1a3a5c")).Foreground(lipgloss.Color("255"))
	eventDotStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	// Subtle highlight for the cursor's week row
	weekHighlightBg := lipgloss.NewStyle().Background(lipgloss.Color("234"))
	weekHighlightDim := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Background(lipgloss.Color("234"))
	weekHighlightDay := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("234"))
	weekHighlightToday := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Background(lipgloss.Color("234"))
	weekHighlightDot := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Background(lipgloss.Color("234"))

	// Determine which week row the cursor is in
	cursorWeekRow := (cursor.Day() - 1 + wd) / 7

	var sb strings.Builder

	// Month/year header
	header := fmt.Sprintf("%s %d", month.String(), year)
	sb.WriteString(MonthHeaderStyle.Render(fmt.Sprintf("%-*s", m.width, centerText(header, m.width))))
	sb.WriteString("\n")

	// Day-of-week header row (with week number gutter)
	sb.WriteString(fmt.Sprintf("%-*s", weekNumWidth, ""))
	dayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	for _, dn := range dayNames {
		sb.WriteString(MonthHeaderStyle.Render(fmt.Sprintf("%-*s", cellWidth, centerText(dn, cellWidth))))
	}
	sb.WriteString("\n")

	// Separator line
	sb.WriteString(dimStyle.Render(strings.Repeat("─", m.width)))
	sb.WriteString("\n")

	// Render grid: iterate through weeks
	day := 1 - wd // start before the 1st to fill leading blanks
	for week := 0; week < numWeeks; week++ {
		isCursorWeek := week == cursorWeekRow
		// Each week produces rowHeight lines
		for line := 0; line < rowHeight; line++ {
			// Week number gutter (only on first line of each week)
			if line == 0 {
				// Find a valid day in this week to get the ISO week number
				weekDay := day + week*7
				if weekDay < 1 {
					weekDay = 1
				}
				if weekDay > daysInMonth {
					weekDay = daysInMonth
				}
				date := time.Date(year, month, weekDay, 0, 0, 0, 0, loc)
				_, isoWeek := date.ISOWeek()
				gutterStyle := dimStyle
				if isCursorWeek {
					gutterStyle = weekHighlightDim
				}
				sb.WriteString(gutterStyle.Render(fmt.Sprintf("W%-*d", weekNumWidth-1, isoWeek)))
			} else {
				if isCursorWeek {
					sb.WriteString(weekHighlightBg.Render(fmt.Sprintf("%-*s", weekNumWidth, "")))
				} else {
					sb.WriteString(fmt.Sprintf("%-*s", weekNumWidth, ""))
				}
			}
			for col := 0; col < 7; col++ {
				currentDay := day + week*7 + col

				if currentDay < 1 || currentDay > daysInMonth {
					// Empty cell (before/after month)
					if isCursorWeek {
						sb.WriteString(weekHighlightBg.Render(fmt.Sprintf("%-*s", cellWidth, "")))
					} else {
						sb.WriteString(fmt.Sprintf("%-*s", cellWidth, ""))
					}
					continue
				}

				date := time.Date(year, month, currentDay, 0, 0, 0, 0, loc)
				dateKey := DateKey(date)
				isSelected := dateKey.Equal(selectedDay)
				isToday := dateKey.Equal(today)
				eventCount := m.store.EventCount(date)

				var cellContent string

				if line == 0 {
					// First line: day number
					numStr := fmt.Sprintf(" %2d", currentDay)
					if eventCount > 0 {
						dots := ""
						for i := 0; i < eventCount && i < 3; i++ {
							dots += "●"
						}
						if eventCount > 3 {
							dots += "+"
						}
						numStr = fmt.Sprintf(" %2d %s", currentDay, dots)
					}
					if len(numStr) > cellWidth {
						numStr = numStr[:cellWidth]
					}
					cellContent = fmt.Sprintf("%-*s", cellWidth, numStr)

					if isSelected {
						cellContent = selectedBg.Render(cellContent)
					} else if isToday {
						if isCursorWeek {
							cellContent = weekHighlightToday.Render(cellContent)
						} else {
							cellContent = todayNumStyle.Render(cellContent)
						}
					} else if eventCount > 0 {
						// Day number in normal, dots in color
						plain := fmt.Sprintf(" %2d ", currentDay)
						dots := ""
						for i := 0; i < eventCount && i < 3; i++ {
							dots += "●"
						}
						if eventCount > 3 {
							dots += "+"
						}
						remaining := cellWidth - len(plain) - len([]rune(dots))
						if remaining < 0 {
							remaining = 0
						}
						if isCursorWeek {
							cellContent = weekHighlightDay.Render(plain) + weekHighlightDot.Render(dots) + weekHighlightBg.Render(fmt.Sprintf("%-*s", remaining, ""))
						} else {
							cellContent = dayNumStyle.Render(plain) + eventDotStyle.Render(dots) + fmt.Sprintf("%-*s", remaining, "")
						}
					} else {
						if isCursorWeek {
							cellContent = weekHighlightDay.Render(cellContent)
						} else {
							cellContent = dayNumStyle.Render(cellContent)
						}
					}
				} else if line == 1 && eventCount > 0 {
					// Second line: show first event title (truncated)
					events := m.store.GetByDate(date)
					title := events[0].Title
					maxLen := cellWidth - 2
					if maxLen < 1 {
						maxLen = 1
					}
					if len(title) > maxLen {
						title = title[:maxLen-1] + "."
					}
					cellContent = fmt.Sprintf(" %-*s", cellWidth-1, title)
					if len([]rune(cellContent)) > cellWidth {
						cellContent = string([]rune(cellContent)[:cellWidth])
					}
					if isSelected {
						cellContent = selectedBg.Render(cellContent)
					} else if isCursorWeek {
						cellContent = weekHighlightDim.Render(cellContent)
					} else {
						cellContent = dimStyle.Render(cellContent)
					}
				} else {
					// Empty lines in this cell
					cellContent = fmt.Sprintf("%-*s", cellWidth, "")
					if isSelected {
						cellContent = selectedBg.Render(cellContent)
					} else if isCursorWeek {
						cellContent = weekHighlightBg.Render(cellContent)
					}
				}

				sb.WriteString(cellContent)
			}
			sb.WriteString("\n")
		}
	}

	// Fill remaining vertical space
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if contentLines < availHeight {
		content += strings.Repeat("\n", availHeight-contentLines)
	}

	return content
}

// centerText centers text within a given width.
func centerText(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	pad := (width - len(s)) / 2
	return fmt.Sprintf("%*s%s", pad, "", s)
}
