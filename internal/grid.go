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

type createPreviewLayout struct {
	layoutMap  map[int]EventLayout
	previewIdx int
	previewCol int
	totalCols  int
	hasPreview bool
}

type allDaySpan struct {
	key       string
	ev        Event
	startCol  int
	endCol    int
	lane      int
	showStart bool
	showEnd   bool
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

	// Precompute layouts for all day columns
	layouts := make([]map[int]EventLayout, m.dayCount)
	rawDayEvents := make([][]Event, m.dayCount)
	for i := 0; i < m.dayCount; i++ {
		date := m.windowStart.AddDate(0, 0, i)
		rawDayEvents[i] = m.applyRecurringAdjustPreview(date, m.store.GetByDate(date))
	}
	allDaySpans, dayEvents := collectAllDaySpans(m, rawDayEvents)
	for i := 0; i < m.dayCount; i++ {
		pinnedID := ""
		pinnedCol := 0
		if m.mode == ModeAdjust && i == m.cursorCol {
			pinnedID = m.adjustEventID
			pinnedCol = m.adjustCol
		}
		layouts[i] = layoutEventsList(dayEvents[i], pinnedID, pinnedCol)
	}

	// Column headers
	headers := renderHeaders(m, availWidth, gutterWidth)
	allDayRows := renderAllDayRows(m, allDaySpans, availWidth, gutterWidth)
	vpHeight -= len(allDayRows)
	if vpHeight < 0 {
		vpHeight = 0
	}

	createLayouts := make([]createPreviewLayout, m.dayCount)
	if m.isCreating() {
		for col := 0; col < m.dayCount; col++ {
			startMin, endMin, ok := createRangeForCol(m, col)
			if !ok {
				continue
			}
			preview := Event{ID: "__create_preview__", Date: m.windowStart.AddDate(0, 0, col), StartMin: startMin, EndMin: endMin}
			eventsWithPreview := append(append([]Event{}, dayEvents[col]...), preview)
			layoutMap := layoutEventsList(eventsWithPreview, "", 0)
			previewIdx := len(eventsWithPreview) - 1
			info := createPreviewLayout{layoutMap: layoutMap, previewIdx: previewIdx, hasPreview: true}
			if l, ok := layoutMap[previewIdx]; ok {
				info.previewCol = l.Col
				info.totalCols = l.TotalCol
			} else {
				info.totalCols = 1
			}
			createLayouts[col] = info
		}
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
	actualRows := (MinutesPerDay - m.viewportOffset + mpr - 1) / mpr
	if actualRows < 0 {
		actualRows = 0
	}
	if actualRows > vpHeight {
		actualRows = vpHeight
	}
	extraBeforeByRow := make([]int, actualRows)
	if actualRows > 1 && vpHeight > actualRows {
		extra := vpHeight - actualRows
		gaps := actualRows - 1
		inserted := 0
		for i := 0; i < actualRows; i++ {
			extraBeforeByRow[i] = inserted
			if i == actualRows-1 {
				continue
			}
			before := (i * extra) / gaps
			after := ((i + 1) * extra) / gaps
			inserted += after - before
		}
	}
	rows := spreadRowsWithSpacing(m, actualRows, vpHeight, availWidth, gutterWidth, layouts, dayEvents, createLayouts, todayCol, nowMin, extraBeforeByRow)

	parts := []string{headers}
	parts = append(parts, allDayRows...)
	parts = append(parts, strings.Join(rows, "\n"))
	return strings.Join(parts, "\n")
}

func spreadRowsWithSpacing(m *Model, rowCount, targetHeight, availWidth, gutterWidth int,
	layouts []map[int]EventLayout, dayEvents [][]Event, createLayouts []createPreviewLayout,
	todayCol, nowMin int, extraBeforeByRow []int) []string {
	if rowCount >= targetHeight || rowCount <= 1 {
		rows := make([]string, 0, targetHeight)
		for row := 0; row < rowCount; row++ {
			rowStartMin := m.viewportOffset + row*m.MinutesPerRow()
			rowEndMin := rowStartMin + m.MinutesPerRow()
			if rowEndMin > MinutesPerDay {
				rowEndMin = MinutesPerDay
			}
			rows = append(rows, renderRow(m, rowStartMin, rowEndMin, availWidth, gutterWidth, layouts, dayEvents, createLayouts, todayCol, nowMin, false, row, extraBeforeByRow))
		}
		for len(rows) < targetHeight {
			rows = append(rows, renderBlankGridRow(m, availWidth, gutterWidth))
		}
		return rows
	}
	extra := targetHeight - rowCount
	gaps := rowCount - 1
	spread := make([]string, 0, targetHeight)
	for i := 0; i < rowCount; i++ {
		mpr := m.MinutesPerRow()
		rowStartMin := m.viewportOffset + i*mpr
		rowEndMin := rowStartMin + mpr
		if rowEndMin > MinutesPerDay {
			rowEndMin = MinutesPerDay
		}
		spread = append(spread, renderRow(m, rowStartMin, rowEndMin, availWidth, gutterWidth, layouts, dayEvents, createLayouts, todayCol, nowMin, false, i+extraBeforeByRow[i], extraBeforeByRow))
		if i == rowCount-1 {
			continue
		}
		before := (i * extra) / gaps
		after := ((i + 1) * extra) / gaps
		for j := 0; j < after-before; j++ {
			spread = append(spread, renderRow(m, rowStartMin, rowEndMin, availWidth, gutterWidth, layouts, dayEvents, createLayouts, todayCol, nowMin, true, i+extraBeforeByRow[i]+j+1, extraBeforeByRow))
		}
	}
	for len(spread) < targetHeight {
		spread = append(spread, renderBlankGridRow(m, availWidth, gutterWidth))
	}
	return spread
}

func renderBlankGridRow(m *Model, availWidth, gutterWidth int) string {
	_ = m
	gutter := TimeGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, ""))
	return gutter + strings.Repeat(" ", availWidth)
}

func collectAllDaySpans(m *Model, dayEvents [][]Event) ([]allDaySpan, [][]Event) {
	filtered := make([][]Event, len(dayEvents))
	seen := map[string]bool{}
	var spans []allDaySpan
	windowStart := DateKey(m.windowStart)
	windowEnd := windowStart.AddDate(0, 0, m.dayCount-1)
	for col, events := range dayEvents {
		for _, ev := range events {
			segments := occurrenceSegments(m, ev)
			duration := 0
			for _, seg := range segments {
				duration += seg.EndMin - seg.StartMin
			}
			if len(segments) <= 1 || duration < MinutesPerDay {
				filtered[col] = append(filtered[col], ev)
				continue
			}
			key := m.selectionKeyForEvent(ev)
			if seen[key] {
				continue
			}
			seen[key] = true
			startDate := DateKey(segments[0].Date)
			endDate := DateKey(segments[len(segments)-1].Date)
			if endDate.Before(windowStart) || startDate.After(windowEnd) {
				continue
			}
			startCol := max(0, int(startDate.Sub(windowStart).Hours()/24))
			endCol := min(m.dayCount-1, int(endDate.Sub(windowStart).Hours()/24))
			spans = append(spans, allDaySpan{
				key:       key,
				ev:        ev,
				startCol:  startCol,
				endCol:    endCol,
				showStart: !startDate.Before(windowStart),
				showEnd:   !endDate.After(windowEnd),
			})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].startCol != spans[j].startCol {
			return spans[i].startCol < spans[j].startCol
		}
		if spans[i].endCol != spans[j].endCol {
			return spans[i].endCol < spans[j].endCol
		}
		return spans[i].key < spans[j].key
	})
	laneEnds := []int{}
	for i := range spans {
		lane := 0
		for ; lane < len(laneEnds); lane++ {
			if spans[i].startCol > laneEnds[lane] {
				break
			}
		}
		if lane == len(laneEnds) {
			laneEnds = append(laneEnds, spans[i].endCol)
		} else {
			laneEnds[lane] = spans[i].endCol
		}
		spans[i].lane = lane
	}
	return spans, filtered
}

func renderAllDayRows(m *Model, spans []allDaySpan, availWidth, gutterWidth int) []string {
	if len(spans) == 0 {
		return nil
	}
	hasAllDayOnCursorCol := false
	for _, span := range spans {
		if m.cursorMin == m.dayStartMin() && m.cursorCol >= span.startCol && m.cursorCol <= span.endCol {
			hasAllDayOnCursorCol = true
			break
		}
	}
	selectedKey := ""
	if idx := m.selectedEventIndex(); idx >= 0 {
		events := m.store.GetByDate(m.SelectedDate())
		if idx < len(events) {
			selectedKey = m.selectionKeyForEvent(events[idx])
		}
	}
	maxLane := 0
	for _, span := range spans {
		if span.lane > maxLane {
			maxLane = span.lane
		}
	}
	rows := make([]string, 0, maxLane+1)
	for lane := 0; lane <= maxLane; lane++ {
		cells := make([]string, m.dayCount)
		for col := 0; col < m.dayCount; col++ {
			cells[col] = strings.Repeat(" ", colWidthForIndex(col, m.dayCount, availWidth))
		}
		laneHasCursor := false
		for _, span := range spans {
			if span.lane != lane {
				continue
			}
			isSelected := m.cursorMin == m.dayStartMin() && m.cursorCol >= span.startCol && m.cursorCol <= span.endCol && selectedKey == span.key
			if isSelected {
				laneHasCursor = true
			}
			for col := span.startCol; col <= span.endCol; col++ {
				cw := colWidthForIndex(col, m.dayCount, availWidth)
				left := ""
				right := ""
				if col == span.startCol {
					left = displayTitle(span.ev)
					if span.showStart {
						left += ", " + MinToTime(span.ev.StartMin)
					}
				}
				if col == span.endCol && span.showEnd {
					right = MinToTime(span.ev.EndMin)
				}
				cells[col] = renderAllDaySegmentCell(m, cw, left, right, col == span.startCol, isSelected)
			}
		}
		gutterLabel := ""
		if hasAllDayOnCursorCol && laneHasCursor {
			gutterLabel = " ►"
		}
		gutter := TimeGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, gutterLabel))
		rows = append(rows, gutter+strings.Join(cells, ""))
	}
	return rows
}

func renderAllDaySegmentCell(m *Model, width int, left, right string, isStart, selected bool) string {
	if width <= 0 {
		return ""
	}
	bg := eventBGColor(m)
	fg := "#e0e0e0"
	if selected {
		bg = "#2a2a3e"
		fg = "#ffffff"
	}
	barColor := eventColor(m, 0)
	if selected {
		barColor = m.uiColor("accent", barColor)
	}
	bar := " "
	bodyWidth := width
	if isStart && width >= 1 {
		bar = lipgloss.NewStyle().
			Foreground(lipgloss.Color(barColor)).
			Background(lipgloss.Color(bg)).
			Render("▎")
		bodyWidth = width - 1
	}
	if bodyWidth < 0 {
		bodyWidth = 0
	}
	content := ""
	if left != "" && right != "" {
		space := bodyWidth - len([]rune(left)) - len([]rune(right))
		if space < 1 {
			content = truncLabel(left, bodyWidth)
		} else {
			content = left + strings.Repeat(" ", space) + right
		}
	} else if left != "" {
		content = truncLabel(left, bodyWidth)
	} else if right != "" {
		r := []rune(right)
		if len(r) > bodyWidth {
			content = truncLabel(right, bodyWidth)
		} else {
			content = strings.Repeat(" ", bodyWidth-len(r)) + right
		}
	} else {
		content = strings.Repeat(" ", bodyWidth)
	}
	body := lipgloss.NewStyle().
		Background(lipgloss.Color(bg)).
		Foreground(lipgloss.Color(fg)).
		Render(fmt.Sprintf("%-*s", bodyWidth, content))
	if isStart {
		return bar + body
	}
	return body
}

// renderHeaders renders the day column headers with ISO week number.
func renderHeaders(m *Model, availWidth, gutterWidth int) string {
	// Show week number in gutter area
	_, week := m.windowStart.ISOWeek()
	gutter := fmt.Sprintf("W%-*d", gutterWidth-1, week)
	gutter = TimeGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, gutter))
	accent := m.uiColor("header_accent", m.uiColor("accent", "39"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)).Align(lipgloss.Center)
	todayStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)).Underline(true).Align(lipgloss.Center)
	selectedTodayStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)).Underline(true).Align(lipgloss.Center)
	defaultStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Align(lipgloss.Center)
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
			cols = append(cols, selectedTodayStyle.Render(padded))
		case isToday:
			cols = append(cols, todayStyle.Render(padded))
		case isCursor:
			cols = append(cols, selectedStyle.Render(padded))
		default:
			cols = append(cols, defaultStyle.Render(padded))
		}
	}
	return gutter + strings.Join(cols, "")
}

// renderRow renders one display row (covering rowStartMin to rowEndMin) across day columns.
func renderRow(m *Model, rowStartMin, rowEndMin, availWidth, gutterWidth int,
	layouts []map[int]EventLayout, dayEvents [][]Event, createLayouts []createPreviewLayout,
	todayCol, nowMin int, suppressGutter bool, visualRowIndex int, extraBeforeByRow []int) string {

	// Determine if the current time line falls in this row
	isNowRow := !suppressGutter && todayCol >= 0 && nowMin >= rowStartMin && nowMin < rowEndMin

	// Time gutter: show every row when zoomed in, but reduce density when zoomed out.
	timeLabel := ""
	mpr := m.MinutesPerRow()
	gutterInterval := mpr
	switch {
	case mpr <= 5:
		gutterInterval = mpr
	case mpr <= 15:
		gutterInterval = 15
	default:
		gutterInterval = 60
	}
	if isNowRow {
		timeLabel = MinToTime(nowMin)
	} else if !suppressGutter && rowStartMin%gutterInterval == 0 {
		timeLabel = MinToTime(rowStartMin)
	}
	if isNowRow {
		gutter := NowGutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, timeLabel))
		var cols []string
		for col := 0; col < m.dayCount; col++ {
			cw := colWidthForIndex(col, m.dayCount, availWidth)
			// Check if any events or cursor overlap this row in this column
			hasEvent := false
			mpr := m.MinutesPerRow()
			for _, ev := range dayEvents[col] {
				if eventOverlapsVisualRow(ev, rowStartMin, rowEndMin, mpr) {
					hasEvent = true
					break
				}
			}
			isCursorHere := col == m.cursorCol && m.cursorMin >= rowStartMin && m.cursorMin < rowEndMin
			if hasEvent {
				// Event present: render the row with the now-line over the event cell
				cell := renderCell(m, col, rowStartMin, rowEndMin, cw, layouts[col], dayEvents[col], createLayouts[col], true, suppressGutter, visualRowIndex, extraBeforeByRow)
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

	gutterStyle := TimeGutterStyle
	if !isNowRow && rowStartMin%60 != 0 && timeLabel != "" {
		gutterStyle = gutterStyle.Foreground(lipgloss.Color("#666666"))
	}
	_ = mpr
	gutter := gutterStyle.Render(fmt.Sprintf("%-*s", gutterWidth, timeLabel))

	var cols []string
	for col := 0; col < m.dayCount; col++ {
		cw := colWidthForIndex(col, m.dayCount, availWidth)
		cell := renderCell(m, col, rowStartMin, rowEndMin, cw, layouts[col], dayEvents[col], createLayouts[col], false, suppressGutter, visualRowIndex, extraBeforeByRow)
		cols = append(cols, cell)
	}

	return gutter + strings.Join(cols, "")
}

// nowLine renders a red horizontal line of the given width for the current time indicator.
func nowLine(width int) string {
	return NowLineStyle.Render(strings.Repeat("─", width))
}

func createRangeForCol(m *Model, col int) (int, int, bool) {
	if !m.isCreating() {
		return 0, 0, false
	}
	dayOffset := col - m.cursorCol
	if dayOffset < 0 {
		return 0, 0, false
	}
	segmentStart := 0
	if dayOffset == 0 {
		segmentStart = m.createStart
	}
	segmentEnd := m.createEnd - dayOffset*MinutesPerDay
	if segmentEnd <= segmentStart {
		return 0, 0, false
	}
	if segmentEnd > MinutesPerDay {
		segmentEnd = MinutesPerDay
	}
	return segmentStart, segmentEnd, true
}

func fillEmpty(width int, showNowLine bool) string {
	if width <= 0 {
		return ""
	}
	if showNowLine {
		return nowLine(width)
	}
	return fmt.Sprintf("%-*s", width, "")
}

func fillEmptySelected(width int, showNowLine, selected bool) string {
	if selected {
		if showNowLine {
			return nowLine(width)
		}
		return CursorStyle.Render(fmt.Sprintf("%-*s", width, ""))
	}
	return fillEmpty(width, showNowLine)
}

func isVisualCellSelected(m *Model, col, rowStartMin, rowEndMin int) bool {
	if m.mode != ModeVisual {
		return false
	}
	startDate, endDate, minMin, maxMin, ok := m.visualSelectionBounds()
	if !ok {
		return false
	}
	date := DateKey(m.windowStart.AddDate(0, 0, col))
	if date.Before(startDate) || date.After(endDate) {
		return false
	}
	return rowStartMin < maxMin && rowEndMin > minMin
}

func visualEventEndMin(startMin, endMin, minutesPerRow int) int {
	// No visual gap — events render their full time range.
	// Separation between adjacent events is handled by bottom-row styling.
	return endMin
}

func eventOverlapsVisualRow(ev Event, rowStartMin, rowEndMin, minutesPerRow int) bool {
	// Visual height should match the actual event time range exactly.
	visualEnd := visualEventEndMin(ev.StartMin, ev.EndMin, minutesPerRow)
	return ev.StartMin < rowEndMin && visualEnd > rowStartMin
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

func isVisualSelected(m *Model, col, idx int) bool {
	if m.mode != ModeVisual {
		return false
	}
	date := m.windowStart.AddDate(0, 0, col)
	events := m.store.GetByDate(date)
	if idx < 0 || idx >= len(events) {
		return false
	}
	return m.visualSelectedKeys()[m.selectionKeyForEvent(events[idx])]
}

func isAdjustSelected(m *Model, col, idx int) bool {
	if m.mode != ModeAdjust {
		return false
	}
	date := m.windowStart.AddDate(0, 0, col)
	events := m.store.GetByDate(date)
	if idx < 0 || idx >= len(events) {
		return false
	}
	if len(m.adjustEventIDs) == 0 {
		return events[idx].ID == m.adjustEventID
	}
	id := events[idx].ID
	for _, selectedID := range m.adjustEventIDs {
		if selectedID == id {
			return true
		}
	}
	return false
}

func isAdjustEventSelected(m *Model, ev Event) bool {
	if m.mode != ModeAdjust {
		return false
	}
	if m.adjustRecurringSelection {
		return strings.HasPrefix(ev.ID, "__selection_preview__")
	}
	if m.adjustRecurring {
		return ev.GroupID != "" && ev.GroupID == m.adjustPreviewGroupID
	}
	if len(m.adjustEventIDs) > 0 {
		for _, selectedID := range m.adjustEventIDs {
			if selectedID == ev.ID {
				return true
			}
		}
		return false
	}
	return ev.ID == m.adjustEventID
}

// renderCell renders a single cell for one day-column at a given row time range.
func renderCell(m *Model, col, rowStartMin, rowEndMin, colWidth int,
	layout map[int]EventLayout, events []Event, createLayout createPreviewLayout, showNowLine bool, suppressDecorations bool, visualRowIndex int, extraBeforeByRow []int) string {

	isCursorRow := !suppressDecorations && col == m.cursorCol && m.cursorMin >= rowStartMin && m.cursorMin < rowEndMin
	isVisualCell := isVisualCellSelected(m, col, rowStartMin, rowEndMin)

	// Check create preview
	inCreatePreview := false
	createStartMin, createEndMin, ok := createRangeForCol(m, col)
	if ok && createStartMin < rowEndMin && createEndMin > rowStartMin {
		inCreatePreview = true
	}

	// Find all events overlapping this row's time range
	var hits []eventHit
	mpr := m.MinutesPerRow()
	for i, ev := range events {
		if eventOverlapsVisualRow(ev, rowStartMin, rowEndMin, mpr) {
			l := EventLayout{Col: 0, TotalCol: 1}
			if createLayout.layoutMap != nil {
				if ll, ok := createLayout.layoutMap[i]; ok {
					l = ll
				}
			} else if layout != nil {
				if ll, ok := layout[i]; ok {
					l = ll
				}
			}
			hits = append(hits, eventHit{idx: i, ev: ev, layout: l})
		}
	}

	if inCreatePreview && len(hits) == 0 {
		label := createPreviewLabel(m, col, rowStartMin, rowEndMin, visualRowIndex, extraBeforeByRow)
		previewFill := rowFillFull
		if createLayout.totalCols > 1 {
			// Maintain consistent column layout even in rows with no events
			subWidth := colWidth / createLayout.totalCols
			if subWidth < 3 {
				subWidth = 3
			}
			lastSubWidth := colWidth - subWidth*(createLayout.totalCols-1)
			if lastSubWidth < subWidth {
				lastSubWidth = subWidth
			}
			previewWidth := subWidth
			if createLayout.previewCol == createLayout.totalCols-1 {
				previewWidth = lastSubWidth
			}
			prefix := fillEmptySelected(createLayout.previewCol*subWidth, showNowLine, isVisualCell)
			label = truncLabel(label, previewWidth-1)
			if showNowLine {
				content := renderNowLineCreatePreviewContent(m, previewWidth)
				suffix := fillEmptySelected(colWidth-createLayout.previewCol*subWidth-previewWidth, showNowLine, isVisualCell)
				return prefix + content + suffix
			}
			content := renderCreatePreviewContent(m, label, previewWidth, previewFill)
			suffix := fillEmptySelected(colWidth-createLayout.previewCol*subWidth-previewWidth, showNowLine, isVisualCell)
			return prefix + content + suffix
		}
		label = truncLabel(label, colWidth-1)
		if showNowLine {
			return renderNowLineCreatePreviewContent(m, colWidth)
		}
		return renderCreatePreviewContent(m, label, colWidth, previewFill)
	}

	if inCreatePreview && len(hits) > 0 {
		// Create preview alongside existing events: split column
		return renderCreateWithEvents(m, col, hits, rowStartMin, rowEndMin, colWidth, createLayout, showNowLine, suppressDecorations, visualRowIndex, extraBeforeByRow)
	}

	if len(hits) == 0 {
		// Empty cell
		content := fillEmptySelected(colWidth, showNowLine, isVisualCell)
		suppressCursorRow := rowStartMin == m.dayStartMin() && m.cursorMin == m.dayStartMin() && col == m.cursorCol && m.selectedAllDayEventIndex(m.SelectedDate()) >= 0
		if isCursorRow && !suppressCursorRow {
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
	if inCreatePreview && createLayout.totalCols > 1 {
		// Check if any hit event overlaps the create time range
		for _, h := range hits {
			if h.ev.StartMin < createEndMin && h.ev.EndMin > createStartMin {
				overrideCols = createLayout.totalCols
				break
			}
		}
	}

	if len(hits) == 1 {
		return renderSingleEvent(m, col, hits[0].idx, hits[0].ev, hits[0].layout,
			rowStartMin, rowEndMin, colWidth, isCursorRow, overrideCols, showNowLine, events, layout, suppressDecorations, visualRowIndex, extraBeforeByRow)
	}

	// Multiple overlapping events: render side-by-side in sub-columns
	return renderMultiEvents(m, col, hits, rowStartMin, rowEndMin, colWidth, isCursorRow, overrideCols, showNowLine, events, layout, suppressDecorations, visualRowIndex, extraBeforeByRow)
}

// renderMultiEvents renders multiple overlapping events side-by-side in sub-columns.
func renderMultiEvents(m *Model, col int, hits []eventHit,
	rowStartMin, rowEndMin, colWidth int, isCursorRow bool, overrideTotalCols int, showNowLine bool, events []Event, layouts map[int]EventLayout, suppressDecorations bool, visualRowIndex int, extraBeforeByRow []int) string {
	isVisualCell := isVisualCellSelected(m, col, rowStartMin, rowEndMin)
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
		if isAdjustEventSelected(m, h.ev) {
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
			adjCols[i] = fillEmptySelected(w, showNowLine, isVisualCell)
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
			label := eventLabel(m, h.ev, rowStartMin, rowEndMin, w-borderChars, false, visualRowIndex, extraBeforeByRow)
			pos := getEventRowPos(m, h.ev, rowStartMin, rowEndMin)
			fill := rowFillFull
			adjAbove := consecutiveChainDepth(h.idx, h.ev, h.layout, events, layouts)
			adjBelow := hasAdjacentEventBelow(h.idx, h.ev, h.layout, events, layouts)
			if isAdjustEventSelected(m, h.ev) {
				if showNowLine {
					adjCols[c] = renderNowLineAdjustContent(m, w, pos)
				} else {
					adjCols[c] = renderAdjustContent(m, label, w, pos, rowStartMin, fill)
				}
			} else {
				style := eventColorStyle(m, h.idx)
				if showNowLine {
					adjCols[c] = renderNowLineEventContent(m, h.idx, w, pos)
				} else {
					adjCols[c] = renderEventContent(m, h.idx, label, w, style, true, pos, rowStartMin, fill, adjAbove, adjBelow)
				}
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
		subCols[i] = fillEmptySelected(w, showNowLine, isVisualCell)
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
		label := eventLabel(m, h.ev, rowStartMin, rowEndMin, w-borderChars, false, visualRowIndex, extraBeforeByRow)
		pos := getEventRowPos(m, h.ev, rowStartMin, rowEndMin)
		fill := rowFillFull
		adjAbove := consecutiveChainDepth(h.idx, h.ev, h.layout, events, layouts)
		adjBelow := hasAdjacentEventBelow(h.idx, h.ev, h.layout, events, layouts)

		isSearchHit := isSearchMatch(m, col, h.idx)
		isCurrentMatch := isCurrentSearchMatch(m, col, h.idx)
		isVisualHit := isVisualSelected(m, col, h.idx)

		if isCurrentMatch {
			if showNowLine {
				subCols[c] = renderNowLineSearchSelectedContent(m, h.idx, w, pos)
			} else {
				subCols[c] = renderSearchSelectedContent(m, h.idx, label, w, pos)
			}
		} else if isVisualHit {
			if showNowLine {
				subCols[c] = renderNowLineCursorContent(m, h.idx, w, pos)
			} else {
				subCols[c] = renderCursorContent(m, h.idx, label, w, pos, rowStartMin, fill, adjAbove, adjBelow)
			}
		} else if isCursorRow && h.idx == selectedIdx {
			if showNowLine {
				subCols[c] = renderNowLineCursorContent(m, h.idx, w, pos)
			} else {
				subCols[c] = renderCursorContent(m, h.idx, label, w, pos, rowStartMin, fill, adjAbove, adjBelow)
			}
		} else if isSearchHit {
			if showNowLine {
				subCols[c] = renderNowLineSearchContent(m, h.idx, w, pos)
			} else {
				subCols[c] = renderSearchContent(m, h.idx, label, w, pos)
			}
		} else {
			style := eventColorStyle(m, h.idx)
			if showNowLine {
				subCols[c] = renderNowLineEventContent(m, h.idx, w, pos)
			} else {
				subCols[c] = renderEventContent(m, h.idx, label, w, style, true, pos, rowStartMin, fill, adjAbove, adjBelow)
			}
		}
	}

	return strings.Join(subCols, "")
}

// renderCreateWithEvents renders create preview alongside existing events.
func renderCreateWithEvents(m *Model, col int, hits []eventHit,
	rowStartMin, rowEndMin, colWidth int, createLayout createPreviewLayout, showNowLine bool, suppressDecorations bool, visualRowIndex int, extraBeforeByRow []int) string {
	isVisualCell := isVisualCellSelected(m, col, rowStartMin, rowEndMin)
	// Use the precomputed consistent total columns count
	totalCols := createLayout.totalCols
	if totalCols < 2 {
		totalCols = 2 // at least 1 event column + 1 preview column
	}
	previewCol := createLayout.previewCol

	subWidth := colWidth / totalCols
	if subWidth < 3 {
		subWidth = 3
	}
	lastSubWidth := colWidth - subWidth*(totalCols-1)
	if lastSubWidth < subWidth {
		lastSubWidth = subWidth
	}

	// Build sub-columns (initialize to empty)
	subCols := make([]string, totalCols)
	for i := range subCols {
		w := subWidth
		if i == totalCols-1 {
			w = lastSubWidth
		}
		subCols[i] = fillEmptySelected(w, showNowLine, isVisualCell)
	}

	// Render existing events in their preview-layout sub-columns.
	for _, h := range hits {
		mapped, ok := createLayout.layoutMap[h.idx]
		if !ok {
			continue
		}
		c := mapped.Col
		if c < 0 || c >= totalCols || c == previewCol {
			continue
		}
		w := subWidth
		if c == totalCols-1 {
			w = lastSubWidth
		}
		label := eventLabel(m, h.ev, rowStartMin, rowEndMin, w-1, false, visualRowIndex, extraBeforeByRow)
		pos := getEventRowPos(m, h.ev, rowStartMin, rowEndMin)
		fill := rowFillFull
		if showNowLine {
			subCols[c] = renderNowLineEventContent(m, h.idx, w, pos)
		} else {
			style := eventColorStyle(m, h.idx)
			subCols[c] = renderEventContent(m, h.idx, label, w, style, true, pos, rowStartMin, fill, 0, false)
		}
	}

	// Create preview in the last sub-column
	w := subWidth
	if previewCol == totalCols-1 {
		w = lastSubWidth
	}
	previewLabel := truncLabel(createPreviewLabel(m, col, rowStartMin, rowEndMin, visualRowIndex, extraBeforeByRow), w-1)
	previewFill := rowFillFull
	if showNowLine {
		subCols[previewCol] = renderNowLineCreatePreviewContent(m, w)
	} else {
		subCols[previewCol] = renderCreatePreviewContent(m, previewLabel, w, previewFill)
	}

	return strings.Join(subCols, "")
}

// eventColorStyle returns a style for an event using the first color in the palette.
func eventColorStyle(m *Model, idx int) lipgloss.Style {
	_ = idx
	bg := m.settings.EventColor
	if bg == "" {
		bg = DefaultEventColor
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(bg)).
		Foreground(lipgloss.Color("#ffffff"))
}

// eventColor returns the hex color for an event.
func eventColor(m *Model, idx int) string {
	_ = idx
	if m.settings.EventColor != "" {
		return m.settings.EventColor
	}
	return DefaultEventColor
}

// EventRowPos indicates where a row falls within an event's visual span.
type EventRowPos int

const (
	EventRowMiddle EventRowPos = iota
	EventRowTop
	EventRowBottom
	EventRowSingle // event is only 1 row tall
)

type rowFill int

const rowFillFull rowFill = 0

// getEventRowPos determines the position of a row within an event.
func getEventRowPos(m *Model, ev Event, rowStartMin, rowEndMin int) EventRowPos {
	mpr := m.MinutesPerRow()
	isFirst := ev.StartMin >= rowStartMin && ev.StartMin < rowEndMin
	// Last visual row: the row before the gap
	visualEnd := visualEventEndMin(ev.StartMin, ev.EndMin, mpr)
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

func renderBorderedContent(m *Model, idx int, text string, width int, bgColor, fgColor string, pos EventRowPos, rowStartMin int, fill rowFill, chainDepth int, adjacentBelow bool) string {
	color := eventColor(m, idx)
	if pos == EventRowSingle && bgColor == eventBGColor(m) {
		mpr := m.MinutesPerRow()
		if mpr > 0 && ((rowStartMin/mpr)%2 == 1) {
			bgColor = "#2a2a42"
		} else {
			bgColor = "#202038"
		}
	}
	// Alternate event color / consecutive_color for chains of back-to-back events
	if chainDepth > 0 && chainDepth%2 == 1 {
		color = m.uiColor("consecutive_color", "#26a269")
	}

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

func layoutsOverlap(a, b EventLayout) bool {
	return a.Col*b.TotalCol < (b.Col+1)*a.TotalCol && b.Col*a.TotalCol < (a.Col+1)*b.TotalCol
}

// consecutiveChainDepth returns how deep this event is in a chain of
// back-to-back events. 0 means no event above, 1 means one above, etc.
func consecutiveChainDepth(idx int, ev Event, layout EventLayout, events []Event, layouts map[int]EventLayout) int {
	depth := 0
	cur := ev
	curIdx := idx
	curLayout := layout
	for {
		found := false
		for otherIdx, other := range events {
			otherLayout, ok := layouts[otherIdx]
			if !ok || otherIdx == curIdx {
				continue
			}
			if layoutsOverlap(curLayout, otherLayout) && other.EndMin == cur.StartMin {
				depth++
				cur = other
				curIdx = otherIdx
				curLayout = otherLayout
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return depth
}

// hasAdjacentEventBelow checks if another event that overlaps this event
// visually starts exactly when this event ends.
func hasAdjacentEventBelow(idx int, ev Event, layout EventLayout, events []Event, layouts map[int]EventLayout) bool {
	for otherIdx, other := range events {
		otherLayout, ok := layouts[otherIdx]
		if !ok || otherIdx == idx {
			continue
		}
		if layoutsOverlap(layout, otherLayout) && other.StartMin == ev.EndMin {
			return true
		}
	}
	return false
}

func eventBGColor(m *Model) string {
	return m.uiColor("event_bg", "#1c1c2e")
}

// renderEventContent renders event text with optional left color bar.
// When borders are enabled, renders bordered content with dim background.
// When disabled, renders with full event color background.

func renderEventContent(m *Model, idx int, text string, width int, style lipgloss.Style, useBorder bool, pos EventRowPos, rowStartMin int, fill rowFill, chainDepth int, adjacentBelow bool) string {
	if !useBorder || !m.settings.ShowBorders || width < 2 {
		return style.Render(padRight(text, width))
	}
	return renderBorderedContent(m, idx, text, width, eventBGColor(m), "#e0e0e0", pos, rowStartMin, fill, chainDepth, adjacentBelow)
}

func renderNowLineBox(m *Model, barColor, bgColor string, width int, barChar string) string {
	_ = m
	_ = barColor
	_ = bgColor
	_ = barChar
	if width <= 0 {
		return ""
	}
	return nowLine(width)
}

func renderNowLineEventContent(m *Model, idx int, width int, pos EventRowPos) string {
	return renderNowLineBox(m, eventColor(m, idx), eventBGColor(m), width, borderBarChar(m, pos))
}

func renderNowLineCursorContent(m *Model, idx int, width int, pos EventRowPos) string {
	return renderNowLineBox(m, eventColor(m, idx), "#2a2a3e", width, borderBarChar(m, pos))
}

func renderNowLineSearchContent(m *Model, _ int, width int, pos EventRowPos) string {
	return renderNowLineBox(m, "#ffffff", eventBGColor(m), width, borderBarChar(m, pos))
}

func renderNowLineSearchSelectedContent(m *Model, _ int, width int, pos EventRowPos) string {
	return renderNowLineBox(m, "#ffd700", "#3a3520", width, borderBarChar(m, pos))
}

func renderNowLineAdjustContent(m *Model, width int, pos EventRowPos) string {
	return renderNowLineBox(m, m.uiColor("accent", eventColor(m, 0)), eventBGColor(m), width, borderBarChar(m, pos))
}

func renderNowLineCreatePreviewContent(m *Model, width int) string {
	return renderNowLineBox(m, m.uiColor("create_preview", m.uiColor("accent", "#00a8ff")), eventBGColor(m), width, "▎")
}

// renderCursorContent renders an event on the cursor row with a subtle grey highlight.
func renderCursorContent(m *Model, idx int, text string, width int, pos EventRowPos, rowStartMin int, fill rowFill, chainDepth int, adjacentBelow bool) string {
	if m.settings.ShowBorders && width >= 2 {
		return renderBorderedContent(m, idx, text, width, "#2a2a3e", "#ffffff", pos, rowStartMin, fill, chainDepth, adjacentBelow)
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
			Background(lipgloss.Color(eventBGColor(m))).
			Bold(true)
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(eventBGColor(m))).
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

// renderAdjustContent renders an event in move mode with themed accent styling.
func renderAdjustContent(m *Model, text string, width int, pos EventRowPos, rowStartMin int, fill rowFill) string {
	adjustColor := m.uiColor("accent", eventColor(m, 0))
	bgColor := eventBGColor(m)
	if pos == EventRowSingle {
		mpr := m.MinutesPerRow()
		if mpr > 0 && ((rowStartMin/mpr)%2 == 1) {
			bgColor = "#2a2a42"
		} else {
			bgColor = "#202038"
		}
	}
	if m.settings.ShowBorders && width >= 2 {
		barStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(adjustColor)).
			Background(lipgloss.Color(bgColor))
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(bgColor)).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true)
		bar := barStyle.Render(borderBarChar(m, pos))
		bodyWidth := width - 1
		body := bodyStyle.Render(padRight(text, bodyWidth))
		return bar + body
	}
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)
	return style.Render(padRight(text, width))
}

// renderCreatePreviewContent renders create preview with the current accent color.
func renderCreatePreviewContent(m *Model, text string, width int, fill rowFill) string {
	createColor := m.uiColor("create_preview", m.uiColor("accent", "#00a8ff"))
	if m.settings.ShowBorders && width >= 2 {
		barStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(createColor)).
			Background(lipgloss.Color(eventBGColor(m)))
		bodyStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(eventBGColor(m))).
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

func createPreviewLabel(m *Model, col, rowStartMin, rowEndMin, visualRowIndex int, extraBeforeByRow []int) string {
	createStartMin, createEndMin, ok := createRangeForCol(m, col)
	if !ok {
		return ""
	}
	timeLabel := formatCreateTimeRange(createStartMin, createEndMin)
	mpr := m.MinutesPerRow()
	startBaseRow := (createStartMin - m.viewportOffset) / mpr
	if startBaseRow < 0 {
		startBaseRow = 0
	}
	startVisualIndex := startBaseRow
	if startBaseRow >= 0 && startBaseRow < len(extraBeforeByRow) {
		startVisualIndex += extraBeforeByRow[startBaseRow]
	}
	lineIndex := visualRowIndex - startVisualIndex
	if rowStartMin <= createStartMin && createStartMin < rowEndMin && lineIndex < 0 {
		lineIndex = 0
	}

	switch {
	case rowStartMin <= createStartMin && createStartMin < rowEndMin && lineIndex == 0:
		if m.inputBuffer != "" {
			return m.inputBuffer
		}
		return timeLabel
	case rowStartMin <= createStartMin && createStartMin < rowEndMin && lineIndex == 1 && m.inputBuffer != "":
		return timeLabel
	default:
		return ""
	}
}

func renderSingleEvent(m *Model, col, idx int, ev Event, layout EventLayout,
	rowStartMin, rowEndMin, colWidth int, isCursorRow bool, overrideTotalCols int, showNowLine bool, events []Event, layouts map[int]EventLayout, suppressDecorations bool, visualRowIndex int, extraBeforeByRow []int) string {

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
	borderChars := 1

	label := eventLabel(m, ev, rowStartMin, rowEndMin, w-borderChars, totalCols <= 1, visualRowIndex, extraBeforeByRow)
	pos := getEventRowPos(m, ev, rowStartMin, rowEndMin)
	fill := rowFillFull
	adjAbove := consecutiveChainDepth(idx, ev, layout, events, layouts)
	adjBelow := hasAdjacentEventBelow(idx, ev, layout, events, layouts)
	isVisualHit := isVisualSelected(m, col, idx)

	// Determine the style for this event
	isAdjusting := isAdjustEventSelected(m, ev)
	isSearchHit := isSearchMatch(m, col, idx)
	isCurrentMatch := isCurrentSearchMatch(m, col, idx)
	var style lipgloss.Style
	if !isAdjusting && !isCursorRow && !isSearchHit && !isVisualHit {
		style = eventColorStyle(m, idx)
	}

	if totalCols <= 1 {
		// Single column: render the whole cell
		if isAdjusting {
			if showNowLine {
				return renderNowLineAdjustContent(m, colWidth, pos)
			}
			return renderAdjustContent(m, label, colWidth, pos, rowStartMin, fill)
		}
		if isCurrentMatch {
			if showNowLine {
				return renderNowLineSearchSelectedContent(m, idx, colWidth, pos)
			}
			return renderSearchSelectedContent(m, idx, label, colWidth, pos)
		}
		if isVisualHit {
			if showNowLine {
				return renderNowLineCursorContent(m, idx, colWidth, pos)
			}
			return renderCursorContent(m, idx, label, colWidth, pos, rowStartMin, fill, adjAbove, adjBelow)
		}
		if isCursorRow {
			if showNowLine {
				return renderNowLineCursorContent(m, idx, colWidth, pos)
			}
			return renderCursorContent(m, idx, label, colWidth, pos, rowStartMin, fill, adjAbove, adjBelow)
		}
		if isSearchHit {
			if showNowLine {
				return renderNowLineSearchContent(m, idx, colWidth, pos)
			}
			return renderSearchContent(m, idx, label, colWidth, pos)
		}
		if showNowLine {
			return renderNowLineEventContent(m, idx, colWidth, pos)
		}
		return renderEventContent(m, idx, label, colWidth, style, true, pos, rowStartMin, fill, adjAbove, adjBelow)
	}

	// Multi sub-columns: only style the event content, leave prefix/suffix unstyled
	offset := layout.Col * subWidth
	prefix := fillEmptySelected(offset, showNowLine, isVisualHit)
	var styledContent string
	if isAdjusting {
		if showNowLine {
			styledContent = renderNowLineAdjustContent(m, w, pos)
		} else {
			styledContent = renderAdjustContent(m, label, w, pos, rowStartMin, fill)
		}
	} else if isCurrentMatch {
		if showNowLine {
			styledContent = renderNowLineSearchSelectedContent(m, idx, w, pos)
		} else {
			styledContent = renderSearchSelectedContent(m, idx, label, w, pos)
		}
	} else if isVisualHit {
		if showNowLine {
			styledContent = renderNowLineCursorContent(m, idx, w, pos)
		} else {
			styledContent = renderCursorContent(m, idx, label, w, pos, rowStartMin, fill, adjAbove, adjBelow)
		}
	} else if isCursorRow {
		if showNowLine {
			styledContent = renderNowLineCursorContent(m, idx, w, pos)
		} else {
			styledContent = renderCursorContent(m, idx, label, w, pos, rowStartMin, fill, adjAbove, adjBelow)
		}
	} else if isSearchHit {
		if showNowLine {
			styledContent = renderNowLineSearchContent(m, idx, w, pos)
		} else {
			styledContent = renderSearchContent(m, idx, label, w, pos)
		}
	} else {
		if showNowLine {
			styledContent = renderNowLineEventContent(m, idx, w, pos)
		} else {
			styledContent = renderEventContent(m, idx, label, w, style, true, pos, rowStartMin, fill, adjAbove, adjBelow)
		}
	}
	remaining := colWidth - offset - w
	suffix := ""
	if remaining > 0 {
		suffix = fillEmptySelected(remaining, showNowLine, isVisualHit)
	}
	return prefix + styledContent + suffix
}

// eventLabel returns the label to display for an event in a given row.
// Titles and times stay on single rows and truncate when narrow.
func eventLabel(m *Model, ev Event, rowStartMin, rowEndMin, maxWidth int, allowWrap bool, visualRowIndex int, extraBeforeByRow []int) string {
	showLabel, showDesc := segmentDisplayPolicy(m, ev)
	if !showLabel {
		return ""
	}
	mpr := m.MinutesPerRow()
	rows := (ev.EndMin - ev.StartMin + mpr - 1) / mpr
	if rows <= 2 {
		allowWrap = false
	}
	titleRowStart := (ev.StartMin / mpr) * mpr
	if rows <= 1 {
		if rowStartMin == titleRowStart {
			compact := displayTitle(ev) + " " + MinToTime(ev.StartMin)
			return truncLabel(compact, maxWidth)
		}
		return ""
	}
	titleLines := wrapLabel(displayTitle(ev), maxWidth)
	if !allowWrap && len(titleLines) > 1 {
		titleLines = titleLines[:1]
	}
	if len(titleLines) == 0 {
		titleLines = []string{""}
	}
	timeInline := eventDisplayTimeRange(m, ev)

	startBaseRow := (titleRowStart - m.viewportOffset) / mpr
	if startBaseRow < 0 {
		startBaseRow = 0
	}
	startVisualIndex := startBaseRow
	if startBaseRow >= 0 && startBaseRow < len(extraBeforeByRow) {
		startVisualIndex += extraBeforeByRow[startBaseRow]
	}
	lineIndex := visualRowIndex - startVisualIndex
	if rowStartMin <= ev.StartMin && ev.StartMin < rowEndMin && lineIndex < 0 {
		lineIndex = 0
	}
	if lineIndex < 0 {
		return ""
	}

	if lineIndex < len(titleLines) {
		return titleLines[lineIndex]
	}
	timeIndex := len(titleLines)
	if lineIndex == timeIndex {
		return truncLabel(timeInline, maxWidth)
	}
	if showDesc && m.settings.ShowDescs && ev.Desc != "" {
		descStartIndex := timeIndex + 1
		descLines := wrapLabel(ev.Desc, maxWidth)
		if !allowWrap && len(descLines) > 1 {
			descLines = descLines[:1]
		}
		descIndex := lineIndex - descStartIndex
		if descIndex >= 0 && descIndex < len(descLines) {
			return descLines[descIndex]
		}
	}
	return ""
}

func wrapLabel(label string, maxWidth int) []string {
	if maxWidth <= 0 {
		return nil
	}
	words := strings.Fields(label)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len([]rune(candidate)) <= maxWidth {
			current = candidate
			continue
		}
		lines = append(lines, truncLabel(current, maxWidth))
		current = word
	}
	lines = append(lines, truncLabel(current, maxWidth))
	return lines
}

func eventDisplayTimeRange(m *Model, ev Event) string {
	if ev.GroupID == "" {
		return MinToTime(ev.StartMin) + " - " + MinToTime(ev.EndMin)
	}
	grouped := occurrenceSegments(m, ev)
	if len(grouped) == 0 {
		return MinToTime(ev.StartMin) + " - " + MinToTime(ev.EndMin)
	}
	start := grouped[0].StartMin
	end := grouped[len(grouped)-1].EndMin
	return MinToTime(start) + " - " + MinToTime(end)
}

func occurrenceSegments(m *Model, ev Event) []Event {
	if ev.GroupID == "" {
		return []Event{ev}
	}
	if (m.mode == ModeAdjust || m.mode == ModeConfirmRecurMove) && m.adjustPreviewGroupID != "" && ev.GroupID == m.adjustPreviewGroupID {
		segments := m.recurringAdjustPreviewSegments()
		if len(segments) > 0 {
			return segments
		}
	}
	if (m.mode == ModeAdjust || m.mode == ModeConfirmRecurMove) && m.adjustRecurringSelection {
		previews := m.recurringSelectionPreviewEvents()
		if strings.HasPrefix(ev.ID, "__selection_preview__") {
			if ev.GroupID == "" {
				for _, p := range previews {
					if p.ID == ev.ID {
						return []Event{p}
					}
				}
			}
			if ev.GroupID != "" {
				var grouped []Event
				for _, p := range previews {
					if p.GroupID == ev.GroupID {
						grouped = append(grouped, p)
					}
				}
				if len(grouped) > 0 {
					return grouped
				}
			}
		}
	}
	baseDate, baseIdx, baseSeg := m.store.findEventRecordByID(ev.ID)
	if baseIdx < 0 {
		return []Event{ev}
	}
	baseSegments := m.store.groupedEvents(baseSeg)
	if len(baseSegments) == 0 {
		return []Event{ev}
	}
	if !ev.IsRecurring() {
		return baseSegments
	}
	dayShift := int(DateKey(ev.Date).Sub(DateKey(baseDate)).Hours() / 24)
	segments := make([]Event, len(baseSegments))
	for i, seg := range baseSegments {
		shifted := seg
		shifted.Date = DateKey(seg.Date).AddDate(0, 0, dayShift)
		shifted.DateStr = shifted.Date.Format("2006-01-02")
		segments[i] = shifted
	}
	return segments
}

func segmentDisplayPolicy(m *Model, ev Event) (showLabel bool, showDesc bool) {
	if ev.GroupID == "" {
		return true, true
	}
	segments := occurrenceSegments(m, ev)
	if len(segments) == 0 {
		return true, true
	}
	hasPrev := DateKey(segments[0].Date) != DateKey(ev.Date)
	hasNext := DateKey(segments[len(segments)-1].Date) != DateKey(ev.Date)
	if hasPrev && hasNext {
		return false, false
	}
	return true, !hasPrev
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
