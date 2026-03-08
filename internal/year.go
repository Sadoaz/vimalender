package internal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateYear handles keys in year view mode.
func (m Model) updateYear(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyQ):
		return m, tea.Quit

	case IsKey(msg, KeyH):
		// Move cursor one day left
		m.yearCursor = m.yearCursor.AddDate(0, 0, -1)

	case IsKey(msg, KeyL):
		// Move cursor one day right
		m.yearCursor = m.yearCursor.AddDate(0, 0, 1)

	case IsKey(msg, KeyJ):
		// Move cursor one week down
		m.yearCursor = m.yearCursor.AddDate(0, 0, 7)

	case IsKey(msg, KeyK):
		// Move cursor one week up
		m.yearCursor = m.yearCursor.AddDate(0, 0, -7)

	case IsKey(msg, KeyShiftH):
		// Move one month back
		m.yearCursor = m.yearCursor.AddDate(0, -1, 0)

	case IsKey(msg, KeyShiftL):
		// Move one month forward
		m.yearCursor = m.yearCursor.AddDate(0, 1, 0)

	case IsKey(msg, KeyShiftJ):
		// Move one year forward
		m.yearCursor = m.yearCursor.AddDate(1, 0, 0)

	case IsKey(msg, KeyShiftK):
		// Move one year back
		m.yearCursor = m.yearCursor.AddDate(-1, 0, 0)

	case IsKey(msg, KeyC):
		// Jump to today
		now := time.Now()
		m.yearCursor = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	case IsKey(msg, KeyShiftM):
		// Switch to month view
		m.viewMode = ViewMonth
		m.mode = ModeMonth
		m.monthCursor = m.yearCursor

	case IsKey(msg, KeyEnter):
		// Switch to week view with selected day on the far left
		m.viewMode = ViewWeek
		m.mode = ModeNavigate
		m.windowStart = m.yearCursor
		m.cursorCol = 0

	case IsKey(msg, KeyEsc), IsKey(msg, KeyShiftY):
		// Go back to week view (Y toggles year view on/off)
		m.viewMode = ViewWeek
		m.mode = ModeNavigate
		m.windowStart = m.yearCursor
		m.cursorCol = 0
	}
	return m, nil
}

// padToWidth pads a styled string to the given visual width using spaces.
func padToWidth(s string, width int) string {
	vis := lipgloss.Width(s)
	if vis >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vis)
}

// RenderYear renders a full-screen year overview with all 12 months.
// Layout: 4 rows x 3 columns of mini month calendars.
func RenderYear(m *Model) string {
	cursor := m.yearCursor
	year := cursor.Year()
	loc := cursor.Location()
	today := DateKey(time.Now())
	selectedDay := DateKey(cursor)

	availHeight := m.height - 2 // reserve for status bar
	availWidth := m.width

	// 4 rows x 3 columns of months
	monthCols := 3
	monthRows := 4

	// Width per mini-month column — last column gets remainder
	baseColWidth := availWidth / monthCols
	if baseColWidth < 22 {
		baseColWidth = 22
	}
	colWidth := func(col int) int {
		if col == monthCols-1 {
			return availWidth - baseColWidth*(monthCols-1)
		}
		return baseColWidth
	}

	// Height: 1 line for year header, rest split across 4 rows
	bodyHeight := availHeight - 1
	baseRowHeight := bodyHeight / monthRows
	if baseRowHeight < 9 {
		baseRowHeight = 9
	}
	rowHeightRemainder := bodyHeight - baseRowHeight*monthRows
	rowHeight := func(row int) int {
		if row < rowHeightRemainder {
			return baseRowHeight + 1
		}
		return baseRowHeight
	}

	// Styles
	accent := m.uiColor("accent", "39")
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	monthNameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true)
	dayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	todayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true)
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255"))
	hasEventStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("70"))

	// Render each mini-month as a block of lines.
	// Each line is padded to the column's visual width.
	type miniMonth struct {
		lines []string
	}

	miniMonths := make([]miniMonth, 12)
	for mo := 0; mo < 12; mo++ {
		col := mo % monthCols
		row := mo / monthCols
		w := colWidth(col)
		h := rowHeight(row)

		month := time.Month(mo + 1)
		firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
		daysInMonth := lastOfMonth.Day()

		// Weekday of first day (Monday=0)
		wd := int(firstOfMonth.Weekday())
		wd = (wd + 6) % 7

		var lines []string

		// Month name header (centered)
		name := month.String()[:3]
		headerLine := centerText(name, w)
		lines = append(lines, monthNameStyle.Render(fmt.Sprintf("%-*s", w, headerLine)))

		// Day-of-week abbreviations (with week number column prefix)
		dowLine := "   Mo Tu We Th Fr Sa Su"
		centered := centerText(dowLine, w)
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%-*s", w, centered)))

		// Day grid
		day := 1
		for week := 0; week < 6; week++ {
			var weekBuf strings.Builder
			// Show ISO week number at start of each week row
			if day <= daysInMonth {
				// Find the date of the Monday of this row
				rowDate := time.Date(year, month, day, 0, 0, 0, 0, loc)
				_, isoWeek := rowDate.ISOWeek()
				weekBuf.WriteString(dimStyle.Render(fmt.Sprintf("%2d ", isoWeek)))
			} else {
				weekBuf.WriteString("   ")
			}
			for c := 0; c < 7; c++ {
				slot := week*7 + c
				if slot < wd || day > daysInMonth {
					weekBuf.WriteString("   ")
				} else {
					date := time.Date(year, month, day, 0, 0, 0, 0, loc)
					dateKey := DateKey(date)
					isSelected := dateKey.Equal(selectedDay)
					isToday := dateKey.Equal(today)
					hasEvents := m.store.EventCount(date) > 0

					numStr := fmt.Sprintf("%2d", day)
					if isSelected {
						weekBuf.WriteString(selectedStyle.Render(numStr))
					} else if isToday {
						weekBuf.WriteString(todayStyle.Render(numStr))
					} else if hasEvents {
						weekBuf.WriteString(hasEventStyle.Render(numStr))
					} else {
						weekBuf.WriteString(dayStyle.Render(numStr))
					}
					weekBuf.WriteString(" ")
					day++
				}
			}
			rawLine := weekBuf.String()
			// Center the day row within the column width
			visW := lipgloss.Width(rawLine)
			leftPad := (w - visW) / 2
			if leftPad < 0 {
				leftPad = 0
			}
			paddedLine := strings.Repeat(" ", leftPad) + rawLine
			lines = append(lines, padToWidth(paddedLine, w))
			if day > daysInMonth {
				break
			}
		}

		// Pad to row height with empty lines
		emptyLine := strings.Repeat(" ", w)
		for len(lines) < h {
			lines = append(lines, emptyLine)
		}

		miniMonths[mo] = miniMonth{lines: lines}
	}

	// Compose the grid: 4 rows of 3 months
	var sb strings.Builder

	// Year header
	yearHeader := fmt.Sprintf("%d", year)
	yearLine := centerText(yearHeader, availWidth)
	sb.WriteString(monthNameStyle.Render(fmt.Sprintf("%-*s", availWidth, yearLine)))
	sb.WriteString("\n")

	for row := 0; row < monthRows; row++ {
		h := rowHeight(row)
		for line := 0; line < h; line++ {
			for col := 0; col < monthCols; col++ {
				moIdx := row*monthCols + col
				if line < len(miniMonths[moIdx].lines) {
					sb.WriteString(miniMonths[moIdx].lines[line])
				} else {
					sb.WriteString(strings.Repeat(" ", colWidth(col)))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Fill any remaining space (shouldn't be needed but safety net)
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if contentLines < availHeight {
		content += strings.Repeat("\n", availHeight-contentLines)
	}

	return content
}
