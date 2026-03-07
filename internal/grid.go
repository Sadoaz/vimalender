package internal

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// eventHit is an event matched to a display row.
type eventHit struct {
	idx    int
	ev     Event
	layout EventLayout
}

// colWidthForIndex returns the width of the given day column.
// The last column gets any remainder pixels so the grid fills the full terminal width.
func colWidthForIndex(col, dayCount, availWidth int) int {
	base := availWidth / dayCount
	if base < 8 {
		base = 8
	}
	if col == dayCount-1 {
		// Last column absorbs the remainder
		w := availWidth - base*(dayCount-1)
		if w < base {
			w = base
		}
		return w
	}
	return base
}

// RenderGrid renders the day grid with time gutter.
// Each row represents MinutesPerRow() minutes.
func RenderGrid(m *Model) string {
	gutterWidth := 6
	availWidth := m.width - gutterWidth

	vpHeight := m.viewportHeight()
	mpr := m.MinutesPerRow()

	// Column headers
	headers := renderHeaders(m, availWidth, gutterWidth)

	// Precompute layouts for all day columns
	layouts := make([]map[int]EventLayout, m.dayCount)
	dayEvents := make([][]Event, m.dayCount)
	for i := 0; i < m.dayCount; i++ {
		date := m.windowStart.AddDate(0, 0, i)
		pinnedID := ""
		pinnedCol := 0
		if m.mode == ModeAdjust && i == m.cursorCol {
			pinnedID = m.adjustEventID
			pinnedCol = m.adjustCol
		}
		layouts[i] = m.store.LayoutEvents(date, pinnedID, pinnedCol)
		dayEvents[i] = m.store.GetByDate(date)
	}

	// Precompute consistent column count for create preview
	// (max TotalCol across all events overlapping the create range + 1 for preview)
	createTotalCols := 0
	if m.isCreating() && m.cursorCol >= 0 && m.cursorCol < m.dayCount {
		maxExisting := 0
		col := m.cursorCol
		for i, ev := range dayEvents[col] {
			if ev.StartMin < m.createEnd && ev.EndMin > m.createStart {
				l := EventLayout{Col: 0, TotalCol: 1}
				if layouts[col] != nil {
					if ll, ok := layouts[col][i]; ok {
						l = ll
					}
				}
				if l.TotalCol > maxExisting {
					maxExisting = l.TotalCol
				}
			}
		}
		createTotalCols = maxExisting + 1
	}

	// Compute "now" info for current time line
	now := time.Now()
	nowMin := now.Hour()*60 + now.Minute()
	todayDate := DateKey(now)
	todayCol := -1 // which column (if any) is today
	for i := 0; i < m.dayCount; i++ {
		if DateKey(m.windowStart.AddDate(0, 0, i)).Equal(todayDate) {
			todayCol = i
			break
		}
	}

	// Time rows
	var rows []string
	for row := 0; row < vpHeight; row++ {
		rowStartMin := m.viewportOffset + row*mpr
		rowEndMin := rowStartMin + mpr
		if rowStartMin >= MinutesPerDay {
			break
		}
		if rowEndMin > MinutesPerDay {
			rowEndMin = MinutesPerDay
		}

		line := renderRow(m, rowStartMin, rowEndMin, availWidth, gutterWidth, layouts, dayEvents, createTotalCols, todayCol, nowMin)
		rows = append(rows, line)
	}

	return headers + "\n" + strings.Join(rows, "\n")
}

// renderHeaders renders the day column headers with ISO week number.
func renderHeaders(m *Model, availWidth, gutterWidth int) string {
	// Show week number in gutter area
	_, week := m.windowStart.ISOWeek()
	gutter := fmt.Sprintf("W%-*d", gutterWidth-1, week)
	gutter = TimeGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, gutter))
	today := DateKey(time.Now())
	var cols []string
	for i := 0; i < m.dayCount; i++ {
		cw := colWidthForIndex(i, m.dayCount, availWidth)
		date := m.windowStart.AddDate(0, 0, i)
		label := date.Format("Mon 02")
		padded := fmt.Sprintf("%-*s", cw, label)
		isToday := DateKey(date).Equal(today)
		isCursor := i == m.cursorCol
		switch {
		case isToday && isCursor:
			cols = append(cols, SelectedTodayColumnHeaderStyle.Render(padded))
		case isToday:
			cols = append(cols, TodayColumnHeaderStyle.Render(padded))
		case isCursor:
			cols = append(cols, SelectedColumnHeaderStyle.Render(padded))
		default:
			cols = append(cols, ColumnHeaderStyle.Render(padded))
		}
	}
	return gutter + strings.Join(cols, "")
}

// renderRow renders one display row (covering rowStartMin to rowEndMin) across day columns.
func renderRow(m *Model, rowStartMin, rowEndMin, availWidth, gutterWidth int,
	layouts []map[int]EventLayout, dayEvents [][]Event, createTotalCols int,
	todayCol, nowMin int) string {

	// Determine if the current time line falls in this row
	isNowRow := todayCol >= 0 && nowMin >= rowStartMin && nowMin < rowEndMin

	// Time gutter: show label on full-hour boundaries
	timeLabel := ""
	if isNowRow {
		timeLabel = MinToTime(nowMin)
	} else if rowStartMin%60 == 0 {
		timeLabel = MinToTime(rowStartMin)
	}
	if isNowRow {
		gutter := NowGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, timeLabel))
		var cols []string
		for col := 0; col < m.dayCount; col++ {
			cw := colWidthForIndex(col, m.dayCount, availWidth)
			// Check if any events or cursor overlap this row in this column
			hasEvent := false
			for _, ev := range dayEvents[col] {
				if ev.StartMin < rowEndMin && ev.EndMin > rowStartMin {
					hasEvent = true
					break
				}
			}
			isCursorHere := col == m.cursorCol && m.cursorMin >= rowStartMin && m.cursorMin < rowEndMin
			if hasEvent {
				// Event present: render normally, the red gutter time signals "now"
				cell := renderCell(m, col, rowStartMin, rowEndMin, cw, layouts[col], dayEvents[col], createTotalCols)
				cols = append(cols, cell)
			} else if isCursorHere {
				// Cursor on now-row: show cursor on red line background
				marker := " \u25ba"
				content := fmt.Sprintf("%-*s", cw, marker)
				style := lipgloss.NewStyle().
					Background(lipgloss.Color("#3a0000")).
					Foreground(lipgloss.Color("#ff0000"))
				cols = append(cols, style.Render(content))
			} else {
				// Empty cell: draw the red now-line
				cols = append(cols, nowLine(cw))
			}
		}
		return gutter + strings.Join(cols, "")
	}

	gutter := TimeGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, timeLabel))

	var cols []string
	for col := 0; col < m.dayCount; col++ {
		cw := colWidthForIndex(col, m.dayCount, availWidth)
		cell := renderCell(m, col, rowStartMin, rowEndMin, cw, layouts[col], dayEvents[col], createTotalCols)
		cols = append(cols, cell)
	}

	return gutter + strings.Join(cols, "")
}

// nowLine renders a red horizontal line of the given width for the current time indicator.
func nowLine(width int) string {
	return NowLineStyle.Render(strings.Repeat("─", width))
}

// isSearchMatch checks if the event at the given date and index is a search match.
// For recurring events, all occurrences (including virtual) are highlighted
// since search matches refer to the base event by ID.
func isSearchMatch(m *Model, col, idx int) bool {
	if !m.searchActive || len(m.searchMatches) == 0 {
		return false
	}
	date := m.windowStart.AddDate(0, 0, col)
	events := m.store.GetByDate(date)
	if idx < 0 || idx >= len(events) {
		return false
	}
	evID := events[idx].ID
	for _, match := range m.searchMatches {
		if match.EventID == evID {
			return true
		}
	}
	return false
}

// isCurrentSearchMatch checks if the event is the currently selected search match.
// For the "current" match, we only highlight the base event on its original date.
func isCurrentSearchMatch(m *Model, col, idx int) bool {
	if !m.searchActive || len(m.searchMatches) == 0 {
		return false
	}
	if m.searchIndex < 0 || m.searchIndex >= len(m.searchMatches) {
		return false
	}
	match := m.searchMatches[m.searchIndex]
	date := m.windowStart.AddDate(0, 0, col)
	events := m.store.GetByDate(date)
	if idx < 0 || idx >= len(events) {
		return false
	}
	return match.EventID == events[idx].ID && DateKey(match.Date).Equal(DateKey(date))
}

// renderCell renders a single cell for one day-column at a given row time range.
func renderCell(m *Model, col, rowStartMin, rowEndMin, colWidth int,
	layout map[int]EventLayout, events []Event, createTotalCols int) string {

	isCursorRow := col == m.cursorCol && m.cursorMin >= rowStartMin && m.cursorMin < rowEndMin

	// Check create preview
	inCreatePreview := false
	if m.isCreating() && col == m.cursorCol {
		// Preview overlaps this row if ranges intersect
		if m.createStart < rowEndMin && m.createEnd > rowStartMin {
			inCreatePreview = true
		}
	}

	// Find all events overlapping this row's time range
	var hits []eventHit
	mpr := m.MinutesPerRow()
	for i, ev := range events {
		// Leave a 1-row visual gap at the bottom of events for spacing.
		// Events must span at least 2 rows (>mpr minutes) to get a gap,
		// otherwise the entire event would disappear.
		visualEnd := ev.EndMin
		rows := (ev.EndMin - ev.StartMin + mpr - 1) / mpr
		if rows >= 2 {
			visualEnd -= mpr
		}
		if ev.StartMin < rowEndMin && visualEnd > rowStartMin {
			l := EventLayout{Col: 0, TotalCol: 1}
			if layout != nil {
				if ll, ok := layout[i]; ok {
					l = ll
				}
			}
			hits = append(hits, eventHit{idx: i, ev: ev, layout: l})
		}
	}

	if inCreatePreview && len(hits) == 0 {
		label := ""
		if rowStartMin <= m.createStart && m.createStart < rowEndMin {
			if m.mode == ModeInput && m.inputBuffer != "" {
				label = m.inputBuffer
			} else {
				label = fmt.Sprintf("%s-%s", MinToTime(m.createStart), MinToTime(m.createEnd))
			}
		}
		if createTotalCols > 1 {
			// Maintain consistent column layout even in rows with no events
			subWidth := colWidth / createTotalCols
			if subWidth < 3 {
				subWidth = 3
			}
			lastSubWidth := colWidth - subWidth*(createTotalCols-1)
			if lastSubWidth < subWidth {
				lastSubWidth = subWidth
			}
			// Empty space for existing event columns
			prefix := fmt.Sprintf("%-*s", colWidth-lastSubWidth, "")
			// Preview in last column
			label = truncLabel(label, lastSubWidth-1)
			return prefix + renderCreatePreviewContent(m, label, lastSubWidth)
		}
		label = truncLabel(label, colWidth-1)
		return renderCreatePreviewContent(m, label, colWidth)
	}

	if inCreatePreview && len(hits) > 0 {
		// Create preview alongside existing events: split column
		return renderCreateWithEvents(m, col, hits, rowStartMin, rowEndMin, colWidth, createTotalCols)
	}

	if len(hits) == 0 {
		// Empty cell
		content := fmt.Sprintf("%-*s", colWidth, "")
		if isCursorRow {
			marker := " \u25ba"
			content = fmt.Sprintf("%-*s", colWidth, marker)
			return CursorStyle.Render(content)
		}
		return content
	}

	// When creating an event on this column, use consistent column widths
	// for events that overlap the create range, so their columns don't jump
	// between normal TotalCol and createTotalCols widths across rows.
	overrideCols := 0
	if m.isCreating() && col == m.cursorCol && createTotalCols > 1 {
		// Check if any hit event overlaps the create time range
		for _, h := range hits {
			if h.ev.StartMin < m.createEnd && h.ev.EndMin > m.createStart {
				overrideCols = createTotalCols
				break
			}
		}
	}

	if len(hits) == 1 {
		return renderSingleEvent(m, col, hits[0].idx, hits[0].ev, hits[0].layout,
			rowStartMin, rowEndMin, colWidth, isCursorRow, overrideCols)
	}

	// Multiple overlapping events: render side-by-side in sub-columns
	return renderMultiEvents(m, col, hits, rowStartMin, rowEndMin, colWidth, isCursorRow, overrideCols)
}

// renderMultiEvents renders multiple overlapping events side-by-side in sub-columns.
func renderMultiEvents(m *Model, col int, hits []eventHit,
	rowStartMin, rowEndMin, colWidth int, isCursorRow bool, overrideTotalCols int) string {
	totalCols := 1
	for _, h := range hits {
		if h.layout.TotalCol > totalCols {
			totalCols = h.layout.TotalCol
		}
	}
	if overrideTotalCols > totalCols {
		totalCols = overrideTotalCols
	}

	subWidth := colWidth / totalCols
	if subWidth < 3 {
		subWidth = 3
	}
	lastSubWidth := colWidth - subWidth*(totalCols-1)
	if lastSubWidth < subWidth {
		lastSubWidth = subWidth
	}

	borderChars := 1

	// Check for adjust mode
	isAdjusting := false
	for _, h := range hits {
		if m.mode == ModeAdjust && col == m.cursorCol && h.idx == m.adjustIndex {
			isAdjusting = true
		}
	}

	if isAdjusting {
		adjCols := make([]string, totalCols)
		for i := range adjCols {
			w := subWidth
			if i == totalCols-1 {
				w = lastSubWidth
			}
			adjCols[i] = fmt.Sprintf("%-*s", w, "")
		}
		for _, h := range hits {
			c := h.layout.Col
			if c < 0 || c >= totalCols {
				continue
			}
			w := subWidth
			if c == totalCols-1 {
				w = lastSubWidth
			}
			label := eventLabel(m, h.ev, rowStartMin, rowEndMin)
			pos := getEventRowPos(m, h.ev, rowStartMin, rowEndMin)
			if h.idx == m.adjustIndex {
				adjCols[c] = renderAdjustContent(m, truncLabel(label, w-borderChars), w, pos)
			} else {
				style := eventColorStyle(m, h.idx)
				adjCols[c] = renderEventContent(m, h.idx, truncLabel(label, w-borderChars), w, style, true, pos)
			}
		}
		return strings.Join(adjCols, "")
	}

	// Sort hits by layout column
	sort.Slice(hits, func(a, b int) bool {
		return hits[a].layout.Col < hits[b].layout.Col
	})

	// Get selected event for cursor highlighting
	selectedIdx := -1
	if isCursorRow {
		selectedIdx = m.selectedEventIndex()
	}

	// Build sub-columns
	subCols := make([]string, totalCols)
	for i := range subCols {
		w := subWidth
		if i == totalCols-1 {
			w = lastSubWidth
		}
		subCols[i] = fmt.Sprintf("%-*s", w, "")
	}

	for _, h := range hits {
		c := h.layout.Col
		if c < 0 || c >= totalCols {
			continue
		}
		w := subWidth
		if c == totalCols-1 {
			w = lastSubWidth
		}
		label := eventLabel(m, h.ev, rowStartMin, rowEndMin)
		pos := getEventRowPos(m, h.ev, rowStartMin, rowEndMin)

		isSearchHit := isSearchMatch(m, col, h.idx)
		isCurrentMatch := isCurrentSearchMatch(m, col, h.idx)

		if isCurrentMatch {
			subCols[c] = renderSearchSelectedContent(m, h.idx, truncLabel(label, w-borderChars), w, pos)
		} else if isCursorRow && h.idx == selectedIdx {
			subCols[c] = renderCursorContent(m, h.idx, truncLabel(label, w-borderChars), w, pos)
		} else if isSearchHit {
			subCols[c] = renderSearchContent(m, h.idx, truncLabel(label, w-borderChars), w, pos)
		} else {
			style := eventColorStyle(m, h.idx)
			subCols[c] = renderEventContent(m, h.idx, truncLabel(label, w-borderChars), w, style, true, pos)
		}
	}

	return strings.Join(subCols, "")
}

// renderCreateWithEvents renders create preview alongside existing events.
func renderCreateWithEvents(m *Model, col int, hits []eventHit,
	rowStartMin, rowEndMin, colWidth, createTotalCols int) string {
	// Use the precomputed consistent total columns count
	totalCols := createTotalCols
	if totalCols < 2 {
		totalCols = 2 // at least 1 event column + 1 preview column
	}
	existingCols := totalCols - 1

	subWidth := colWidth / totalCols
	if subWidth < 3 {
		subWidth = 3
	}
	lastSubWidth := colWidth - subWidth*(totalCols-1)
	if lastSubWidth < subWidth {
		lastSubWidth = subWidth
	}

	// Sort hits by layout column
	sort.Slice(hits, func(a, b int) bool {
		return hits[a].layout.Col < hits[b].layout.Col
	})

	// Build sub-columns (initialize to empty)
	subCols := make([]string, totalCols)
	for i := range subCols {
		w := subWidth
		if i == totalCols-1 {
			w = lastSubWidth
		}
		subCols[i] = fmt.Sprintf("%-*s", w, "")
	}

	// Render existing events in their sub-columns
	for _, h := range hits {
		c := h.layout.Col
		if c < 0 || c >= existingCols {
			continue
		}
		w := subWidth
		if c == totalCols-1 {
			w = lastSubWidth
		}
		label := eventLabel(m, h.ev, rowStartMin, rowEndMin)
		style := eventColorStyle(m, h.idx)
		pos := getEventRowPos(m, h.ev, rowStartMin, rowEndMin)
		subCols[c] = renderEventContent(m, h.idx, truncLabel(label, w-1), w, style, true, pos)
	}

	// Create preview in the last sub-column
	w := lastSubWidth
	previewLabel := ""
	if rowStartMin <= m.createStart && m.createStart < rowEndMin {
		if m.mode == ModeInput && m.inputBuffer != "" {
			previewLabel = m.inputBuffer
		} else {
			previewLabel = fmt.Sprintf("%s-%s", MinToTime(m.createStart), MinToTime(m.createEnd))
		}
		previewLabel = truncLabel(previewLabel, w-1)
	}
	subCols[totalCols-1] = renderCreatePreviewContent(m, previewLabel, w)

	return strings.Join(subCols, "")
}

// eventColorStyle returns a style for an event using the first color in the palette.
func eventColorStyle(m *Model, idx int) lipgloss.Style {
	colors := m.settings.EventColors
	if len(colors) == 0 {
		return EventBlockStyle
	}
	bg := colors[0]
	return lipgloss.NewStyle().
		Background(lipgloss.Color(bg)).
		Foreground(lipgloss.Color("#ffffff"))
}

// eventColor returns the hex color for an event.
func eventColor(m *Model, idx int) string {
	colors := m.settings.EventColors
	if len(colors) == 0 {
		return "#005fd7"
	}
	return colors[0]
}

// EventRowPos indicates where a row falls within an event's visual span.
type EventRowPos int

const (
	EventRowMiddle EventRowPos = iota
	EventRowTop
	EventRowBottom
	EventRowSingle // event is only 1 row tall
)

// getEventRowPos determines the position of a row within an event.
func getEventRowPos(m *Model, ev Event, rowStartMin, rowEndMin int) EventRowPos {
	mpr := m.MinutesPerRow()
	isFirst := ev.StartMin >= rowStartMin && ev.StartMin < rowEndMin
	// Last visual row: the row before the gap
	visualEnd := ev.EndMin
	rows := (ev.EndMin - ev.StartMin + mpr - 1) / mpr
	if rows >= 2 {
		visualEnd -= mpr
	}
	isLast := visualEnd > rowStartMin && visualEnd <= rowEndMin
	switch {
	case isFirst && isLast:
		return EventRowSingle
	case isFirst:
		return EventRowTop
	case isLast:
		return EventRowBottom
	default:
		return EventRowMiddle
	}
}

// borderBarChar returns the left border character based on position and rounded setting.
func borderBarChar(m *Model, pos EventRowPos) string {
	if !m.settings.RoundBorders {
		return "\u258e" // ▎
	}
	switch pos {
	case EventRowTop, EventRowSingle:
		return "╭"
	case EventRowBottom:
		return "╰"
	default:
		return "│"
	}
}

// renderBorderedContent renders content with left bar and body text.
// For bottom rows with round borders, fills with horizontal lines for visual separation.
func renderBorderedContent(m *Model, idx int, text string, width int, bgColor, fgColor string, pos EventRowPos) string {
	color := eventColor(m, idx)
	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Background(lipgloss.Color(bgColor))
	bodyStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(fgColor))

	bar := barStyle.Render(borderBarChar(m, pos))
	bodyWidth := width - 1
	if bodyWidth < 0 {
		bodyWidth = 0
	}

	body := bodyStyle.Render(padRight(text, bodyWidth))
	return bar + body
}

// renderEventContent renders event text with optional left color bar.
// When borders are enabled, renders bordered content with dim background.
// When disabled, renders with full event color background.
func renderEventContent(m *Model, idx int, text string, width int, style lipgloss.Style, useBorder bool, pos EventRowPos) string {
	if !useBorder || !m.settings.ShowBorders || width < 2 {
		return style.Render(padRight(text, width))
	}
	return renderBorderedContent(m, idx, text, width, "#1c1c2e", "#e0e0e0", pos)
}

// renderCursorContent renders an event on the cursor row with a subtle grey highlight.
func renderCursorContent(m *Model, idx int, text string, width int, pos EventRowPos) string {
	if m.settings.ShowBorders && width >= 2 {
		return renderBorderedContent(m, idx, text, width, "#2a2a3e", "#ffffff", pos)
	}
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#2a2a3e")).
		Foreground(lipgloss.Color("#ffffff"))
	return style.Render(padRight(text, width))
}

// renderSearchContent renders a search-matched event with a bright white border bar.
func renderSearchContent(m *Model, idx int, text string, width int, pos EventRowPos) string {
	if m.settings.ShowBorders && width >= 2 {
		barStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#1c1c2e")).
			Bold(true)
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#1c1c2e")).
			Foreground(lipgloss.Color("#e0e0e0")).
			Bold(true)
		bar := barStyle.Render(borderBarChar(m, pos))
		bodyWidth := width - 1
		body := bodyStyle.Render(padRight(text, bodyWidth))
		return bar + body
	}
	style := eventColorStyle(m, idx).Bold(true).Underline(true)
	return style.Render(padRight(text, width))
}

// renderSearchSelectedContent renders the currently selected search result.
func renderSearchSelectedContent(m *Model, idx int, text string, width int, pos EventRowPos) string {
	if m.settings.ShowBorders && width >= 2 {
		barStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffd700")).
			Background(lipgloss.Color("#3a3520")).
			Bold(true)
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#3a3520")).
			Foreground(lipgloss.Color("#ffd700")).
			Bold(true)
		bar := barStyle.Render(borderBarChar(m, pos))
		bodyWidth := width - 1
		body := bodyStyle.Render(padRight(text, bodyWidth))
		return bar + body
	}
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#3a3520")).
		Foreground(lipgloss.Color("#ffd700")).
		Bold(true)
	return style.Render(padRight(text, width))
}

// renderAdjustContent renders an event in adjust mode with orange left bar and dim bg.
func renderAdjustContent(m *Model, text string, width int, pos EventRowPos) string {
	adjustColor := "#ff5f00"
	if m.settings.ShowBorders && width >= 2 {
		barStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(adjustColor)).
			Background(lipgloss.Color("#2a1a0e"))
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#2a1a0e")).
			Foreground(lipgloss.Color("#f0d0a0")).
			Bold(true)
		bar := barStyle.Render(borderBarChar(m, pos))
		bodyWidth := width - 1
		body := bodyStyle.Render(padRight(text, bodyWidth))
		return bar + body
	}
	return AdjustEventStyle.Render(padRight(text, width))
}

// renderCreatePreviewContent renders create preview with the same left color bar style as events.
// Uses a distinct blue color (#5f87ff) to distinguish from existing events.
func renderCreatePreviewContent(m *Model, text string, width int) string {
	createColor := "#5f87ff"
	if m.settings.ShowBorders && width >= 2 {
		barStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(createColor)).
			Background(lipgloss.Color("#1c1c2e"))
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#1c1c2e")).
			Foreground(lipgloss.Color("#e0e0e0"))
		bar := barStyle.Render("\u258e")
		bodyWidth := width - 1
		body := bodyStyle.Render(padRight(text, bodyWidth))
		return bar + body
	}
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(createColor)).
		Foreground(lipgloss.Color("#ffffff"))
	return style.Render(padRight(text, width))
}

func renderSingleEvent(m *Model, col, idx int, ev Event, layout EventLayout,
	rowStartMin, rowEndMin, colWidth int, isCursorRow bool, overrideTotalCols int) string {

	totalCols := layout.TotalCol
	if totalCols < 1 {
		totalCols = 1
	}
	if overrideTotalCols > totalCols {
		totalCols = overrideTotalCols
	}

	subWidth := colWidth
	if totalCols > 1 {
		subWidth = colWidth / totalCols
		if subWidth < 3 {
			subWidth = 3
		}
	}
	// Last sub-column gets remaining width
	lastSubWidth := colWidth - subWidth*(totalCols-1)
	if lastSubWidth < subWidth {
		lastSubWidth = subWidth
	}
	w := subWidth
	if layout.Col == totalCols-1 {
		w = lastSubWidth
	}

	label := eventLabel(m, ev, rowStartMin, rowEndMin)
	pos := getEventRowPos(m, ev, rowStartMin, rowEndMin)

	// Determine the style for this event
	isAdjusting := m.mode == ModeAdjust && col == m.cursorCol && idx == m.adjustIndex
	isSearchHit := isSearchMatch(m, col, idx)
	isCurrentMatch := isCurrentSearchMatch(m, col, idx)
	var style lipgloss.Style
	if !isAdjusting && !isCursorRow && !isSearchHit {
		style = eventColorStyle(m, idx)
	}

	// Calculate max label width accounting for left border
	borderChars := 1

	if totalCols <= 1 {
		// Single column: render the whole cell
		if isAdjusting {
			return renderAdjustContent(m, truncLabel(label, colWidth-borderChars), colWidth, pos)
		}
		if isCurrentMatch {
			return renderSearchSelectedContent(m, idx, truncLabel(label, colWidth-borderChars), colWidth, pos)
		}
		if isCursorRow {
			return renderCursorContent(m, idx, truncLabel(label, colWidth-borderChars), colWidth, pos)
		}
		if isSearchHit {
			return renderSearchContent(m, idx, truncLabel(label, colWidth-borderChars), colWidth, pos)
		}
		return renderEventContent(m, idx, truncLabel(label, colWidth-borderChars), colWidth, style, true, pos)
	}

	// Multi sub-columns: only style the event content, leave prefix/suffix unstyled
	offset := layout.Col * subWidth
	prefix := fmt.Sprintf("%-*s", offset, "")
	var styledContent string
	if isAdjusting {
		styledContent = renderAdjustContent(m, truncLabel(label, w-borderChars), w, pos)
	} else if isCurrentMatch {
		styledContent = renderSearchSelectedContent(m, idx, truncLabel(label, w-borderChars), w, pos)
	} else if isCursorRow {
		styledContent = renderCursorContent(m, idx, truncLabel(label, w-borderChars), w, pos)
	} else if isSearchHit {
		styledContent = renderSearchContent(m, idx, truncLabel(label, w-borderChars), w, pos)
	} else {
		styledContent = renderEventContent(m, idx, truncLabel(label, w-borderChars), w, style, true, pos)
	}
	remaining := colWidth - offset - w
	suffix := ""
	if remaining > 0 {
		suffix = fmt.Sprintf("%-*s", remaining, "")
	}
	return prefix + styledContent + suffix
}

// eventLabel returns the label to display for an event in a given row.
// Shows title on the event's first row, time range on the row after, description on the row after that (if enabled).
func eventLabel(m *Model, ev Event, rowStartMin, rowEndMin int) string {
	mpr := m.MinutesPerRow()
	titleRowStart := (ev.StartMin / mpr) * mpr
	titleRowEnd := titleRowStart + mpr

	if ev.StartMin >= rowStartMin && ev.StartMin < rowEndMin {
		// First row: show title
		return displayTitle(ev)
	}
	if rowStartMin == titleRowEnd {
		// Second row: show time range
		return MinToTime(ev.StartMin) + " – " + MinToTime(ev.EndMin)
	}
	if m.settings.ShowDescs && ev.Desc != "" {
		descRowStart := titleRowEnd + mpr
		if rowStartMin == descRowStart {
			return ev.Desc
		}
	}
	return ""
}

// displayTitle returns the event title with a recurrence prefix if applicable.
func displayTitle(ev Event) string {
	if ev.IsRecurring() {
		return "↻ " + ev.Title
	}
	return ev.Title
}

// truncLabel truncates a label to fit in maxLen display columns, adding "." if truncated.
func truncLabel(label string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(label)
	if len(runes) <= maxLen {
		return label
	}
	if maxLen <= 1 {
		return "."
	}
	return string(runes[:maxLen-1]) + "."
}

// padRight pads a string with spaces to the given display width (rune-aware).
func padRight(s string, width int) string {
	runeLen := len([]rune(s))
	if runeLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-runeLen)
}
