package internal

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Mode represents the current interaction mode.
type Mode int

const (
	ModeNavigate           Mode = iota
	ModeCreate                  // selecting event duration
	ModeInput                   // typing event title (new event only)
	ModeInputDesc               // typing event description
	ModeInputRecurrence         // choosing recurrence after title
	ModeAdjust                  // moving an event
	ModeConfirmDelete           // dd second-d pending
	ModeConfirmRecurDelete      // "Delete: (o)ne / (a)ll?" for recurring events
	ModeDetail                  // fullscreen event detail view
	ModeGoto                    // typing go-to time
	ModeGotoDay                 // typing go-to day of month
	ModeSearch                  // typing search query
	ModeMonth                   // month view navigation
	ModeYear                    // year view navigation
	ModeSettings                // settings menu
	ModeEditMenu                // inline event editor
)

// ViewMode represents the high-level view (week grid vs month overview).
type ViewMode int

const (
	ViewWeek ViewMode = iota
	ViewMonth
	ViewYear
)

// MinWidth and MinHeight are the minimum terminal dimensions.
const (
	MinWidth  = 80
	MinHeight = 24
)

// ZoomAuto is the sentinel value for auto-fit zoom.
const ZoomAuto = -1

// ZoomLevels are the zoom levels (minutes per row) — all "clean" divisors of 60.
var ZoomLevels = []int{1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60}

// Model is the single Bubbletea model for Vimalender.
type Model struct {
	mode           Mode
	viewMode       ViewMode  // week grid or month overview
	windowStart    time.Time // first date in the day window
	cursorCol      int       // 0..dayCount-1, which day column
	cursorMin      int       // 0-1439, which minute
	viewportOffset int       // first visible minute in viewport
	store          *EventStore
	width          int
	height         int
	statusMsg      string // transient message

	// Zoom
	zoomLevel int // minutes per row, or ZoomAuto

	// Day count
	dayCount int // number of day columns to display

	// Create mode state
	createStart      int
	createEnd        int
	createDesc       string // description for new event
	createRecurrence string // recurrence pattern for new event

	// Adjust mode state
	adjustIndex   int
	adjustEventID string // ID of event being adjusted
	adjustCol     int    // pinned visual column during adjust

	// Input mode state
	inputBuffer string
	descBuffer  string // description input buffer
	editIndex   int    // -1 for new event, >=0 for editing existing
	isEdit      bool   // true when editing existing event title

	// Detail view state
	detailIndex int // event index being viewed

	// Goto mode state
	gotoBuffer string

	// Search state
	searchQuery   string
	searchMatches []SearchMatch // all matches
	searchIndex   int           // current match index
	searchActive  bool          // true when search highlights are shown

	// Month view state
	monthCursor time.Time // selected date in month view

	// Year view state
	yearCursor time.Time // selected date in year view

	// Settings
	settings       Settings // persistent user preferences
	settingsCursor int      // selected option in settings menu

	// Edit menu state
	editMenuIndex  int       // event index being edited
	editMenuField  int       // 0=title, 1=date, 2=start, 3=end
	editMenuBuf    string    // current field edit buffer
	editMenuActive bool      // true when editing a field (typing)
	editMenuValues [7]string // current values: title, desc, date, start, end, recurrence, recur_until

	// Delete: dd (vim double-key)
	pendingD bool // true after first 'd' press

	// Recurring delete confirmation
	recurDeleteIdx int // index in GetByDate result for the event being deleted

	// Undo stack — snapshots of event store before each mutation
	undoStack []map[time.Time][]Event
	// Redo stack — snapshots pushed when undoing
	redoStack []map[time.Time][]Event

	// Overlap selection: tracks the event ID so selection is stable
	// even when GetByDate ordering changes.  "" = no selection.
	selectedOverlapEvt string
}

// NewModel creates a new model, loading persisted events and settings.
func NewModel() Model {
	today := DateKey(time.Now())
	store, errMsg := LoadEvents()
	settings, settingsErr := LoadSettings()
	if settingsErr != "" && errMsg == "" {
		errMsg = settingsErr
	}

	// Force DayStartHour to 0 (always show full day from midnight)
	settings.DayStartHour = 0
	settings.RoundBorders = false

	// Restore persisted position, or default to today
	windowStart := today
	cursorCol := 0
	cursorMin := settings.DayStartHour * 60
	viewportOffset := settings.DayStartHour * 60

	if settings.LastDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", settings.LastDate, time.Now().Location()); err == nil {
			windowStart = t
			cursorCol = settings.LastCursorCol
			cursorMin = settings.LastCursorMin
			viewportOffset = settings.LastViewport
		}
	}

	return Model{
		mode:               ModeNavigate,
		windowStart:        windowStart,
		cursorCol:          cursorCol,
		cursorMin:          cursorMin,
		viewportOffset:     viewportOffset,
		store:              store,
		zoomLevel:          ZoomAuto,
		dayCount:           settings.DayCount,
		settings:           settings,
		adjustIndex:        -1,
		editIndex:          -1,
		selectedOverlapEvt: "",
		statusMsg:          errMsg,
	}
}

// SelectedDate returns the date under the cursor.
func (m *Model) SelectedDate() time.Time {
	return m.windowStart.AddDate(0, 0, m.cursorCol)
}

// selectedEventIndex returns the event index under cursor, respecting overlap selection.
// Uses visual bounds (excluding gap row) so the gap between events is not selectable.
// Returns -1 if no event at cursor.
func (m *Model) selectedEventIndex() int {
	date := m.SelectedDate()
	events := m.store.GetByDate(date)
	indices := m.store.VisualEventsAtMinute(date, m.cursorMin, m.MinutesPerRow())
	if len(indices) == 0 {
		return -1
	}
	// If we have a specific event selected by ID, try to find it in the overlap list
	if m.selectedOverlapEvt != "" {
		for _, idx := range indices {
			if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
				return idx
			}
		}
	}
	// Fall back to first overlapping event
	return indices[0]
}

// resetOverlapIndex resets overlap selection (used when moving to a different day or deleting).
func (m *Model) resetOverlapIndex() {
	m.selectedOverlapEvt = ""
}

// autoSelectOverlapEvent picks the best event to highlight at the current
// cursor position after j/k movement. If an event starts on this row it gets
// priority (so scrolling "enters" new events naturally). Otherwise the
// currently selected event is kept if it still covers this row.
func (m *Model) autoSelectOverlapEvent() {
	date := m.SelectedDate()
	events := m.store.GetByDate(date)
	indices := m.store.VisualEventsAtMinute(date, m.cursorMin, m.MinutesPerRow())
	if len(indices) == 0 {
		return
	}

	mpr := m.MinutesPerRow()
	rowStart := (m.cursorMin / mpr) * mpr
	rowEnd := rowStart + mpr

	// Check if any event starts on this row — if so, prefer it
	for _, idx := range indices {
		ev := events[idx]
		if ev.StartMin >= rowStart && ev.StartMin < rowEnd {
			m.selectedOverlapEvt = ev.ID
			return
		}
	}

	// Keep current selection if it still covers this row
	if m.selectedOverlapEvt != "" {
		for _, idx := range indices {
			if events[idx].ID == m.selectedOverlapEvt {
				return
			}
		}
	}

	// Fall back to first event at this minute
	m.selectedOverlapEvt = events[indices[0]].ID
}

// sortedOverlapIndices returns all event indices in the same overlap group
// as the event under the cursor, sorted by their visual layout column
// so H/L navigation follows left-to-right order.
// This uses LayoutEvents' TotalCol: events sharing the same TotalCol > 1
// and belonging to the same connected overlap group are returned together,
// regardless of whether they all overlap the exact cursor minute.
func (m *Model) sortedOverlapIndices(date time.Time, minute int) []int {
	events := m.store.GetByDate(date)
	layout := m.store.LayoutEvents(date, "", 0)
	if layout == nil {
		return m.store.EventsAtMinute(date, minute)
	}

	// Find which event is currently selected or under cursor (visual bounds)
	cursorIndices := m.store.VisualEventsAtMinute(date, minute, m.MinutesPerRow())
	if len(cursorIndices) == 0 {
		return nil
	}

	// Pick the selected event (by ID) or fall back to first at cursor
	seedIdx := cursorIndices[0]
	if m.selectedOverlapEvt != "" {
		for _, idx := range cursorIndices {
			if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
				seedIdx = idx
				break
			}
		}
	}

	seedLayout, ok := layout[seedIdx]
	if !ok || seedLayout.TotalCol <= 1 {
		return cursorIndices
	}

	// Collect all events in the same overlap group.
	// Two events are in the same group if they share a TotalCol value AND
	// are transitively connected through overlapping time ranges.
	// We do a simple flood-fill: start from seedIdx, find all events that
	// overlap any event already in the group.
	inGroup := map[int]bool{seedIdx: true}
	changed := true
	for changed {
		changed = false
		for idx, l := range layout {
			if inGroup[idx] {
				continue
			}
			if l.TotalCol != seedLayout.TotalCol {
				continue
			}
			// Check if this event overlaps any event already in the group
			ev := events[idx]
			for gIdx := range inGroup {
				gev := events[gIdx]
				if ev.StartMin < gev.EndMin && ev.EndMin > gev.StartMin {
					inGroup[idx] = true
					changed = true
					break
				}
			}
		}
	}

	var indices []int
	for idx := range inGroup {
		indices = append(indices, idx)
	}

	// Sort by start time so navigation goes sequentially down the day
	sort.Slice(indices, func(a, b int) bool {
		ea, eb := events[indices[a]], events[indices[b]]
		if ea.StartMin != eb.StartMin {
			return ea.StartMin < eb.StartMin
		}
		return ea.ID < eb.ID
	})
	return indices
}

// saveEvents persists the event store to disk.
func (m *Model) saveEvents() {
	if err := SaveEvents(m.store); err != nil {
		m.statusMsg = fmt.Sprintf("Save error: %v", err)
	}
}

// pushUndo saves a snapshot of the current events for undo.
func (m *Model) pushUndo() {
	const maxUndo = 50
	m.undoStack = append(m.undoStack, m.store.Snapshot())
	if len(m.undoStack) > maxUndo {
		m.undoStack = m.undoStack[len(m.undoStack)-maxUndo:]
	}
	m.redoStack = nil // new action clears redo history
}

// popUndo restores the most recent undo snapshot. Returns true if successful.
func (m *Model) popUndo() bool {
	if len(m.undoStack) == 0 {
		return false
	}
	m.redoStack = append(m.redoStack, m.store.Snapshot())
	snap := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.store.Restore(snap)
	m.saveEvents()
	return true
}

// popRedo re-applies the most recently undone action. Returns true if successful.
func (m *Model) popRedo() bool {
	if len(m.redoStack) == 0 {
		return false
	}
	m.undoStack = append(m.undoStack, m.store.Snapshot())
	snap := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	m.store.Restore(snap)
	m.saveEvents()
	return true
}

// saveSettings persists user settings to disk.
func (m *Model) saveSettings() {
	m.settings.ZoomLevel = m.zoomLevel
	m.settings.DayCount = m.dayCount
	if err := SaveSettings(m.settings); err != nil {
		m.statusMsg = fmt.Sprintf("Settings save error: %v", err)
	}
}

// savePosition persists the current cursor position for restore on next startup.
func (m *Model) savePosition() {
	m.settings.LastDate = m.windowStart.Format("2006-01-02")
	m.settings.LastCursorCol = m.cursorCol
	m.settings.LastCursorMin = m.cursorMin
	m.settings.LastViewport = m.viewportOffset
	m.saveSettings()
}

// handleEditorResult processes the result from the external editor.
func (m Model) handleEditorResult(msg editorResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusMsg = fmt.Sprintf("Editor error: %v", msg.err)
		return m, nil
	}

	// Find the event by its position — the editor passes date + index from GetByDate
	events := m.store.GetByDate(msg.date)
	if msg.index < 0 || msg.index >= len(events) {
		m.statusMsg = "Event no longer exists"
		return m, nil
	}

	if msg.endMin <= msg.startMin {
		m.statusMsg = "End time must be after start time"
		return m, nil
	}

	ev := events[msg.index]
	baseDate, baseIdx := m.store.FindEventByID(ev.ID)
	if baseIdx < 0 {
		m.statusMsg = "Event no longer exists"
		return m, nil
	}

	key := DateKey(baseDate)
	m.pushUndo()
	m.store.events[key][baseIdx].Title = msg.title
	m.store.events[key][baseIdx].Desc = msg.desc
	m.store.events[key][baseIdx].StartMin = msg.startMin
	m.store.events[key][baseIdx].EndMin = msg.endMin
	m.store.events[key][baseIdx].Notes = msg.notes
	m.store.events[key][baseIdx].Recurrence = msg.recurrence
	m.store.events[key][baseIdx].RecurUntilStr = msg.recurUntil
	m.saveEvents()
	m.statusMsg = fmt.Sprintf("Updated %q", msg.title)
	return m, nil
}

// MinutesPerRow returns the fixed minutes-per-row value (30).
// This gives 2 rows per hour with clean time labels.
func (m *Model) MinutesPerRow() int {
	return 30
}

// dayStartMin returns the start-of-day minute offset from the DayStartHour setting.
func (m *Model) dayStartMin() int {
	return m.settings.DayStartHour * 60
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		firstResize := m.width == 0 && m.height == 0
		m.width = msg.Width
		m.height = msg.Height
		// If current zoom level is too coarse for the new size, switch to auto
		if m.zoomLevel != ZoomAuto {
			vpHeight := m.viewportHeight()
			visibleMinutes := MinutesPerDay - m.dayStartMin()
			maxMpr := visibleMinutes / vpHeight
			if maxMpr < 1 {
				maxMpr = 1
			}
			if m.zoomLevel > maxMpr {
				m.zoomLevel = ZoomAuto
			}
		}
		// On first resize (app startup), center viewport on current time
		if firstResize {
			now := time.Now()
			nowMin := now.Hour()*60 + now.Minute()
			mpr := m.MinutesPerRow()
			m.cursorMin = (nowMin / mpr) * mpr
			// Place today's column under cursor if visible
			todayDate := DateKey(now)
			for i := 0; i < m.dayCount; i++ {
				if DateKey(m.windowStart.AddDate(0, 0, i)).Equal(todayDate) {
					m.cursorCol = i
					break
				}
			}
			m.centerViewportOnCursor()
		}
		return m, nil

	case editorResultMsg:
		return m.handleEditorResult(msg)

	case tea.KeyMsg:
		// Ctrl+C always quits
		if msg.String() == "ctrl+c" {
			m.savePosition()
			return m, tea.Quit
		}

		// Clear transient status on any key (except during dd sequence)
		if !m.pendingD || !IsKey(msg, KeyD) {
			m.statusMsg = ""
		}

		switch m.mode {
		case ModeNavigate:
			return m.updateNavigate(msg)
		case ModeCreate:
			return m.updateCreate(msg)
		case ModeInput:
			return m.updateInput(msg)
		case ModeInputDesc:
			return m.updateInputDesc(msg)
		case ModeInputRecurrence:
			return m.updateInputRecurrence(msg)
		case ModeAdjust:
			return m.updateAdjust(msg)
		case ModeDetail:
			return m.updateDetail(msg)
		case ModeGoto:
			return m.updateGoto(msg)
		case ModeGotoDay:
			return m.updateGotoDay(msg)
		case ModeSearch:
			return m.updateSearch(msg)
		case ModeMonth:
			return m.updateMonth(msg)
		case ModeYear:
			return m.updateYear(msg)
		case ModeSettings:
			return m.updateSettings(msg)
		case ModeEditMenu:
			return m.updateEditMenu(msg)
		case ModeConfirmRecurDelete:
			return m.updateConfirmRecurDelete(msg)
		}
	}
	return m, nil
}

func (m *Model) viewportHeight() int {
	h := m.height - 3 // 1 header + 1 newline + 1 status bar
	if h < 1 {
		h = 1
	}
	return h
}

// centerViewportOnCursor adjusts viewport offset to center the cursor vertically.
func (m *Model) centerViewportOnCursor() {
	mpr := m.MinutesPerRow()
	vpHeight := m.viewportHeight()
	// Place cursor in the middle of the viewport
	m.viewportOffset = m.cursorMin - mpr*(vpHeight/2)
	if m.viewportOffset < 0 {
		m.viewportOffset = 0
	}
	// Clamp to max offset
	maxOffset := MinutesPerDay - mpr*vpHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.viewportOffset > maxOffset {
		m.viewportOffset = maxOffset
	}
}

// zoomIn decreases minutes-per-row (more detail).
func (m *Model) zoomIn() {
	currentMpr := m.MinutesPerRow()
	if m.zoomLevel == ZoomAuto {
		// Switch from auto to the largest predefined level that is finer than auto
		for i := len(ZoomLevels) - 1; i >= 0; i-- {
			if ZoomLevels[i] < currentMpr {
				m.zoomLevel = ZoomLevels[i]
				m.centerViewportOnCursor()
				return
			}
		}
		// Auto is already at finest possible — try level 1
		if currentMpr > 1 {
			m.zoomLevel = 1
			m.centerViewportOnCursor()
		}
		return
	}
	// Find current index and go one step finer
	for i := len(ZoomLevels) - 1; i >= 0; i-- {
		if ZoomLevels[i] < m.zoomLevel {
			m.zoomLevel = ZoomLevels[i]
			m.centerViewportOnCursor()
			return
		}
	}
	// Already at finest level
}

// zoomOut increases minutes-per-row (less detail).
func (m *Model) zoomOut() {
	if m.zoomLevel == ZoomAuto {
		return // already at maximum zoom out
	}
	autoMpr := m.autoMpr()

	// Find current index and go one step coarser
	for i := 0; i < len(ZoomLevels); i++ {
		if ZoomLevels[i] > m.zoomLevel {
			if ZoomLevels[i] >= autoMpr {
				// This level would match or exceed auto — jump to auto instead
				m.zoomLevel = ZoomAuto
				m.viewportOffset = m.dayStartMin()
				return
			}
			m.zoomLevel = ZoomLevels[i]
			m.centerViewportOnCursor()
			return
		}
	}
	// Past the coarsest predefined level — switch to auto
	m.zoomLevel = ZoomAuto
	m.viewportOffset = m.dayStartMin()
}

// autoMpr returns what MinutesPerRow would be in auto mode.
func (m *Model) autoMpr() int {
	vpHeight := m.viewportHeight()
	if vpHeight <= 0 {
		return 30
	}
	visibleMinutes := MinutesPerDay - m.dayStartMin()
	mpr := (visibleMinutes + vpHeight - 1) / vpHeight
	if mpr < 1 {
		mpr = 1
	}
	return mpr
}

// ensureCursorVisible adjusts viewport offset so the cursor is visible.
func (m *Model) ensureCursorVisible() {
	mpr := m.MinutesPerRow()
	vpHeight := m.viewportHeight()
	vpEnd := m.viewportOffset + mpr*vpHeight

	if m.cursorMin < m.viewportOffset {
		m.viewportOffset = m.cursorMin
	} else if m.cursorMin >= vpEnd {
		m.viewportOffset = m.cursorMin - mpr*(vpHeight-1)
	}

	// Clamp so viewport doesn't extend past end of day (no blank space at bottom)
	maxOffset := MinutesPerDay - mpr*vpHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.viewportOffset > maxOffset {
		m.viewportOffset = maxOffset
	}
	if m.viewportOffset < 0 {
		m.viewportOffset = 0
	}
}

// ensureCreateVisible adjusts viewport so the create preview end is visible.
func (m *Model) ensureCreateVisible() {
	mpr := m.MinutesPerRow()
	vpHeight := m.viewportHeight()
	vpEnd := m.viewportOffset + mpr*vpHeight

	if m.createEnd > vpEnd {
		m.viewportOffset = m.createEnd - mpr*vpHeight
	}
	if m.createStart < m.viewportOffset {
		m.viewportOffset = m.createStart
	}
	if m.viewportOffset < 0 {
		m.viewportOffset = 0
	}
}

// isCreating returns true if the model is in any create-related mode
// (selecting duration, typing title, description, or recurrence).
func (m *Model) isCreating() bool {
	return m.mode == ModeCreate || m.mode == ModeInput || m.mode == ModeInputDesc || m.mode == ModeInputRecurrence
}

// maxDayCount returns the maximum number of day columns that fit with minimum column width.
func (m *Model) maxDayCount() int {
	gutterWidth := 6
	availWidth := m.width - gutterWidth
	maxCols := availWidth / 8 // minimum 8 chars per column
	if maxCols < 1 {
		maxCols = 1
	}
	return maxCols
}

// jumpStep returns the number of minutes per j/k press (2% of viewport).
// The step is rounded to the nearest multiple of MinutesPerRow so the cursor
// always lands on a grid-line (clean time like :00, :05, :15, :30).
func (m *Model) jumpStep() int {
	mpr := m.MinutesPerRow()
	vpHeight := m.viewportHeight()
	totalVisible := mpr * vpHeight
	step := totalVisible * 2 / 100
	if step < mpr {
		step = mpr
	}
	// Round to nearest multiple of mpr
	step = (step / mpr) * mpr
	if step < mpr {
		step = mpr
	}
	return step
}

// setDayCount sets the number of day columns, capped to fit the terminal.
func (m *Model) setDayCount(n int) {
	max := m.maxDayCount()
	if n > max {
		n = max
	}
	if n < 1 {
		n = 1
	}
	// Center the window on the current cursor date
	curDate := m.SelectedDate()
	m.dayCount = n
	// Reposition: put cursor in the middle of the new window
	halfWay := n / 2
	m.windowStart = curDate.AddDate(0, 0, -halfWay)
	m.cursorCol = halfWay
}

// --- Navigate mode ---

func (m Model) updateNavigate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle dd sequence
	if m.pendingD {
		m.pendingD = false
		if IsKey(msg, KeyD) {
			// dd confirmed - delete event under cursor
			idx := m.selectedEventIndex()
			if idx != -1 {
				events := m.store.GetByDate(m.SelectedDate())
				ev := events[idx]
				if ev.IsRecurring() {
					// Enter recurring delete confirmation
					m.mode = ModeConfirmRecurDelete
					m.recurDeleteIdx = idx
					m.statusMsg = fmt.Sprintf("Delete %q: (o)ne / (a)ll?", ev.Title)
				} else {
					m.pushUndo()
					m.statusMsg = fmt.Sprintf("Deleted %q", ev.Title)
					m.store.DeleteByID(ev.ID)
					m.saveEvents()
					m.resetOverlapIndex()
				}
			}
		}
		return m, nil
	}

	switch {
	case IsKey(msg, KeyQ):
		m.savePosition()
		return m, tea.Quit

	case IsKey(msg, KeyH):
		// If overlapping events exist, navigate to previous event in group;
		// otherwise move to the previous day column.
		date := m.SelectedDate()
		indices := m.sortedOverlapIndices(date, m.cursorMin)
		if len(indices) > 1 {
			events := m.store.GetByDate(date)
			pos := 0
			for i, idx := range indices {
				if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
					pos = i
					break
				}
			}
			pos--
			if pos < 0 {
				// Already at first event in group → move to previous day
				if m.cursorCol > 0 {
					m.cursorCol--
				} else {
					m.windowStart = m.windowStart.AddDate(0, 0, -1)
				}
				m.resetOverlapIndex()
			} else {
				selIdx := indices[pos]
				if selIdx < len(events) {
					m.selectedOverlapEvt = events[selIdx].ID
					mpr := m.MinutesPerRow()
					m.cursorMin = (events[selIdx].StartMin / mpr) * mpr
					m.ensureCursorVisible()
					m.statusMsg = fmt.Sprintf("[%d/%d] %s", pos+1, len(indices), events[selIdx].Title)
				}
			}
		} else {
			if m.cursorCol > 0 {
				m.cursorCol--
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
			}
			m.resetOverlapIndex()
		}

	case IsKey(msg, KeyL):
		// If overlapping events exist, navigate to next event in group;
		// otherwise move to the next day column.
		date := m.SelectedDate()
		indices := m.sortedOverlapIndices(date, m.cursorMin)
		if len(indices) > 1 {
			events := m.store.GetByDate(date)
			pos := 0
			for i, idx := range indices {
				if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
					pos = i
					break
				}
			}
			pos++
			if pos >= len(indices) {
				// Already at last event in group → move to next day
				if m.cursorCol < m.dayCount-1 {
					m.cursorCol++
				} else {
					m.windowStart = m.windowStart.AddDate(0, 0, 1)
				}
				m.resetOverlapIndex()
			} else {
				selIdx := indices[pos]
				if selIdx < len(events) {
					m.selectedOverlapEvt = events[selIdx].ID
					mpr := m.MinutesPerRow()
					m.cursorMin = (events[selIdx].StartMin / mpr) * mpr
					m.ensureCursorVisible()
					m.statusMsg = fmt.Sprintf("[%d/%d] %s", pos+1, len(indices), events[selIdx].Title)
				}
			}
		} else {
			if m.cursorCol < m.dayCount-1 {
				m.cursorCol++
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
			}
			m.resetOverlapIndex()
		}

	case IsKey(msg, KeyJ):
		step := m.jumpStep()
		m.cursorMin += step
		if m.cursorMin >= MinutesPerDay {
			// Wrap to start of next day
			if m.cursorCol < m.dayCount-1 {
				m.cursorCol++
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
			}
			m.cursorMin = m.dayStartMin()
			m.viewportOffset = m.dayStartMin()
			m.resetOverlapIndex()
		} else {
			// Snap to grid
			mpr := m.MinutesPerRow()
			m.cursorMin = (m.cursorMin / mpr) * mpr
		}
		m.ensureCursorVisible()
		m.autoSelectOverlapEvent()

	case IsKey(msg, KeyK):
		step := m.jumpStep()
		m.cursorMin -= step
		if m.cursorMin < 0 {
			// Wrap to end of previous day
			if m.cursorCol > 0 {
				m.cursorCol--
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
			}
			// Snap to the last grid line of the day
			mpr := m.MinutesPerRow()
			m.cursorMin = ((MinutesPerDay - 1) / mpr) * mpr
			m.resetOverlapIndex()
		} else {
			// Snap to grid
			mpr := m.MinutesPerRow()
			m.cursorMin = (m.cursorMin / mpr) * mpr
		}
		m.ensureCursorVisible()
		m.autoSelectOverlapEvent()

	case IsKey(msg, KeyCtrlD):
		// Quarter page down, wrap to next day
		mpr := m.MinutesPerRow()
		quarterPage := mpr * (m.viewportHeight() / 4)
		m.cursorMin += quarterPage
		if m.cursorMin >= MinutesPerDay {
			if m.cursorCol < m.dayCount-1 {
				m.cursorCol++
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
			}
			m.cursorMin = m.dayStartMin()
			m.viewportOffset = m.dayStartMin()
			m.resetOverlapIndex()
		}
		m.ensureCursorVisible()
		m.autoSelectOverlapEvent()

	case IsKey(msg, KeyCtrlU):
		// Quarter page up, wrap to previous day
		mpr := m.MinutesPerRow()
		quarterPage := mpr * (m.viewportHeight() / 4)
		m.cursorMin -= quarterPage
		if m.cursorMin < 0 {
			if m.cursorCol > 0 {
				m.cursorCol--
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
			}
			m.cursorMin = ((MinutesPerDay - 1) / mpr) * mpr
			m.resetOverlapIndex()
		}
		m.ensureCursorVisible()
		m.autoSelectOverlapEvent()

	case IsKey(msg, KeyShiftJ):
		m.cursorMin++
		if m.cursorMin >= MinutesPerDay {
			// Wrap to start of next day
			if m.cursorCol < m.dayCount-1 {
				m.cursorCol++
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
			}
			m.cursorMin = m.dayStartMin()
			m.viewportOffset = m.dayStartMin()
			m.resetOverlapIndex()
		}
		m.ensureCursorVisible()

	case IsKey(msg, KeyShiftK):
		m.cursorMin--
		if m.cursorMin < 0 {
			// Wrap to end of previous day
			if m.cursorCol > 0 {
				m.cursorCol--
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
			}
			m.cursorMin = MinutesPerDay - 1
			m.resetOverlapIndex()
		}
		m.ensureCursorVisible()

	case IsKey(msg, KeyTab):
		// Cycle through overlapping events (forward), moving cursor to each
		date := m.SelectedDate()
		events := m.store.GetByDate(date)
		indices := m.sortedOverlapIndices(date, m.cursorMin)
		if len(indices) > 1 {
			pos := 0
			for i, idx := range indices {
				if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
					pos = i
					break
				}
			}
			pos = (pos + 1) % len(indices)
			selIdx := indices[pos]
			if selIdx < len(events) {
				m.selectedOverlapEvt = events[selIdx].ID
				mpr := m.MinutesPerRow()
				m.cursorMin = (events[selIdx].StartMin / mpr) * mpr
				m.ensureCursorVisible()
				m.statusMsg = fmt.Sprintf("[%d/%d] %s", pos+1, len(indices), events[selIdx].Title)
			}
		}

	case IsKey(msg, KeyShiftL):
		// Move to next overlapping event, moving cursor to it
		date := m.SelectedDate()
		events := m.store.GetByDate(date)
		indices := m.sortedOverlapIndices(date, m.cursorMin)
		if len(indices) > 1 {
			pos := 0
			for i, idx := range indices {
				if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
					pos = i
					break
				}
			}
			pos = (pos + 1) % len(indices)
			selIdx := indices[pos]
			if selIdx < len(events) {
				m.selectedOverlapEvt = events[selIdx].ID
				mpr := m.MinutesPerRow()
				m.cursorMin = (events[selIdx].StartMin / mpr) * mpr
				m.ensureCursorVisible()
				m.statusMsg = fmt.Sprintf("[%d/%d] %s", pos+1, len(indices), events[selIdx].Title)
			}
		}

	case IsKey(msg, KeyShiftH):
		// Move to previous overlapping event, moving cursor to it
		date := m.SelectedDate()
		events := m.store.GetByDate(date)
		indices := m.sortedOverlapIndices(date, m.cursorMin)
		if len(indices) > 1 {
			pos := 0
			for i, idx := range indices {
				if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
					pos = i
					break
				}
			}
			pos--
			if pos < 0 {
				pos = len(indices) - 1
			}
			selIdx := indices[pos]
			if selIdx < len(events) {
				m.selectedOverlapEvt = events[selIdx].ID
				mpr := m.MinutesPerRow()
				m.cursorMin = (events[selIdx].StartMin / mpr) * mpr
				m.ensureCursorVisible()
				m.statusMsg = fmt.Sprintf("[%d/%d] %s", pos+1, len(indices), events[selIdx].Title)
			}
		}

	case IsKey(msg, KeyA):
		// Enter create mode (overlaps allowed)
		m.mode = ModeCreate
		mpr := m.MinutesPerRow()
		m.createStart = (m.cursorMin / mpr) * mpr
		m.createEnd = m.createStart + m.jumpStep()
		if m.createEnd > MinutesPerDay {
			m.createEnd = MinutesPerDay
		}
		m.ensureCreateVisible()

	case IsKey(msg, KeyEnter):
		// If on event, open detail view
		idx := m.selectedEventIndex()
		if idx != -1 {
			m.mode = ModeDetail
			m.detailIndex = idx
		}

	case IsKey(msg, KeyM):
		// If on event, enter adjust mode (not for virtual occurrences)
		idx := m.selectedEventIndex()
		if idx != -1 {
			date := m.SelectedDate()
			events := m.store.GetByDate(date)
			ev := events[idx]
			if m.store.IsVirtualIndex(date, idx) {
				m.statusMsg = "Cannot adjust a virtual occurrence"
			} else {
				m.pushUndo()
				m.mode = ModeAdjust
				m.adjustIndex = idx
				m.adjustEventID = ev.ID
				// Remember current visual column for stability during adjust
				layout := m.store.LayoutEvents(date, "", 0)
				if l, ok := layout[idx]; ok {
					m.adjustCol = l.Col
				} else {
					m.adjustCol = 0
				}
			}
		}

	case IsKey(msg, KeyE):
		// Open external editor for event
		idx := m.selectedEventIndex()
		if idx != -1 {
			return m, m.openEditor(m.SelectedDate(), idx)
		}

	case IsKey(msg, KeyD):
		// First d press - wait for second d
		m.pendingD = true
		m.statusMsg = "d-"

	case IsKey(msg, KeyU):
		// Undo last action
		if m.popUndo() {
			m.statusMsg = "Undone"
			m.resetOverlapIndex()
		} else {
			m.statusMsg = "Nothing to undo"
		}

	case IsKey(msg, KeyCtrlR):
		// Redo last undone action
		if m.popRedo() {
			m.statusMsg = "Redone"
			m.resetOverlapIndex()
		} else {
			m.statusMsg = "Nothing to redo"
		}

	case IsKey(msg, KeyX):
		// Single-key delete — prompts for recurring events
		idx := m.selectedEventIndex()
		if idx != -1 {
			events := m.store.GetByDate(m.SelectedDate())
			ev := events[idx]
			if ev.IsRecurring() {
				m.mode = ModeConfirmRecurDelete
				m.recurDeleteIdx = idx
				m.statusMsg = fmt.Sprintf("Delete %q: (o)ne / (a)ll?", ev.Title)
			} else {
				m.pushUndo()
				m.statusMsg = fmt.Sprintf("Deleted %q", ev.Title)
				m.store.DeleteByID(ev.ID)
				m.saveEvents()
				m.resetOverlapIndex()
			}
		}

	case IsKey(msg, KeyPlus):
		// Zoom disabled — always show full day

	case IsKey(msg, KeyMinus):
		// Zoom disabled — always show full day

	case IsKey(msg, KeySlash):
		// Enter search mode
		m.mode = ModeSearch
		m.searchQuery = ""
		m.searchMatches = nil

	case IsKey(msg, KeyG):
		// Enter go-to time mode
		m.mode = ModeGoto
		m.gotoBuffer = ""

	case IsKey(msg, KeyShiftG):
		// Enter go-to day mode
		m.mode = ModeGotoDay
		m.gotoBuffer = ""

	case IsKey(msg, KeyN):
		// Next search match
		if m.searchActive {
			m.nextMatch()
		}

	case IsKey(msg, KeyShiftN):
		// Previous search match
		if m.searchActive {
			m.prevMatch()
		}

	case IsKey(msg, KeyCtrlP):
		// Previous search match (alternative binding)
		if m.searchActive {
			m.prevMatch()
		}

	case IsKey(msg, KeyCtrlN):
		// Next search match (alternative binding)
		if m.searchActive {
			m.nextMatch()
		}

	case IsKey(msg, KeyEsc):
		// Clear active search
		if m.searchActive {
			m.searchActive = false
			m.searchMatches = nil
			m.searchQuery = ""
		}

	case IsKey(msg, KeyC):
		// Jump to now: today on far left, cursor at current time, viewport centered
		now := time.Now()
		nowMin := now.Hour()*60 + now.Minute()
		m.windowStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		m.cursorCol = 0
		m.cursorMin = (nowMin / m.MinutesPerRow()) * m.MinutesPerRow()
		m.centerViewportOnCursor()
		m.resetOverlapIndex()

	case IsKey(msg, KeyShiftM):
		// Toggle to month view
		m.viewMode = ViewMonth
		m.mode = ModeMonth
		m.monthCursor = m.SelectedDate()

	case IsKey(msg, KeyShiftY):
		// Toggle to year view
		m.viewMode = ViewYear
		m.mode = ModeYear
		m.yearCursor = m.SelectedDate()

	case IsKey(msg, KeyShiftS):
		// Open settings menu
		m.mode = ModeSettings
		m.settingsCursor = 0

	default:
		// Number keys 1-9 to set day count
		s := msg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			requested := int(s[0] - '0')
			m.setDayCount(requested)
			m.saveSettings()
		}
	}
	return m, nil
}

// --- Create mode ---

func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = ModeNavigate

	case IsKey(msg, KeyJ):
		step := m.jumpStep()
		newEnd := m.createEnd + step
		if newEnd > MinutesPerDay {
			newEnd = MinutesPerDay
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyK):
		step := m.jumpStep()
		newEnd := m.createEnd - step
		mpr := m.MinutesPerRow()
		minEnd := m.createStart + mpr
		if minEnd > MinutesPerDay {
			minEnd = m.createStart + 1
		}
		if newEnd < minEnd {
			newEnd = minEnd
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyShiftJ):
		newEnd := m.createEnd + 1
		if newEnd > MinutesPerDay {
			newEnd = MinutesPerDay
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyShiftK):
		newEnd := m.createEnd - 1
		if newEnd <= m.createStart {
			newEnd = m.createStart + 1
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyEnter):
		m.mode = ModeInput
		m.isEdit = false
		m.editIndex = -1
		m.inputBuffer = ""
	}
	return m, nil
}

// --- Input mode (title entry for new or edit) ---

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = ModeNavigate
		m.inputBuffer = ""

	case IsKey(msg, KeyEnter):
		title := m.inputBuffer
		if title == "" {
			title = "Untitled"
		}
		m.inputBuffer = title
		m.createDesc = ""
		if m.settings.SkipDesc {
			// Skip description step
			m.createRecurrence = RecurNone
			if m.settings.QuickCreate {
				// Skip both desc and recurrence — create immediately
				m.pushUndo()
				err := m.store.Add(Event{
					Title:      m.inputBuffer,
					Desc:       "",
					Date:       m.SelectedDate(),
					StartMin:   m.createStart,
					EndMin:     m.createEnd,
					Recurrence: RecurNone,
				})
				if err != nil {
					m.statusMsg = err.Error()
				} else {
					m.saveEvents()
				}
				m.mode = ModeNavigate
				m.inputBuffer = ""
			} else {
				m.mode = ModeInputRecurrence
			}
		} else {
			m.mode = ModeInputDesc
		}

	case msg.String() == "backspace":
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}

	default:
		s := msg.String()
		if len(s) == 1 || s == " " {
			m.inputBuffer += s
		}
	}
	return m, nil
}

// --- Input description mode (description entry for new event) ---

func (m Model) updateInputDesc(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = ModeNavigate
		m.inputBuffer = ""
		m.createDesc = ""

	case IsKey(msg, KeyEnter):
		m.createDesc = m.descBuffer
		m.descBuffer = ""
		m.createRecurrence = RecurNone
		if m.settings.QuickCreate {
			// Skip recurrence picker — create event immediately
			m.pushUndo()
			err := m.store.Add(Event{
				Title:      m.inputBuffer,
				Desc:       m.createDesc,
				Date:       m.SelectedDate(),
				StartMin:   m.createStart,
				EndMin:     m.createEnd,
				Recurrence: RecurNone,
			})
			if err != nil {
				m.statusMsg = err.Error()
			} else {
				m.saveEvents()
			}
			m.mode = ModeNavigate
			m.inputBuffer = ""
			m.createDesc = ""
		} else {
			m.mode = ModeInputRecurrence
		}

	case msg.String() == "backspace":
		if len(m.descBuffer) > 0 {
			m.descBuffer = m.descBuffer[:len(m.descBuffer)-1]
		}

	default:
		s := msg.String()
		if len(s) == 1 || s == " " {
			m.descBuffer += s
		}
	}
	return m, nil
}

// --- Input recurrence mode (choose recurrence for new event) ---

func (m Model) updateInputRecurrence(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = ModeNavigate
		m.inputBuffer = ""
		m.createRecurrence = RecurNone

	case IsKey(msg, KeyEnter):
		// Create the event with selected recurrence
		title := m.inputBuffer
		m.pushUndo()
		err := m.store.Add(Event{
			Title:      title,
			Desc:       m.createDesc,
			Date:       m.SelectedDate(),
			StartMin:   m.createStart,
			EndMin:     m.createEnd,
			Recurrence: m.createRecurrence,
		})
		if err != nil {
			m.statusMsg = err.Error()
		} else {
			m.saveEvents()
		}
		m.mode = ModeNavigate
		m.inputBuffer = ""
		m.createDesc = ""
		m.createRecurrence = RecurNone

	case IsKey(msg, "r"):
		// Cycle through recurrence options
		cur := 0
		for i, opt := range RecurrenceOptions {
			if opt == m.createRecurrence {
				cur = i
				break
			}
		}
		cur = (cur + 1) % len(RecurrenceOptions)
		m.createRecurrence = RecurrenceOptions[cur]

	case IsKey(msg, "R"):
		// Cycle backwards through recurrence options
		cur := 0
		for i, opt := range RecurrenceOptions {
			if opt == m.createRecurrence {
				cur = i
				break
			}
		}
		cur--
		if cur < 0 {
			cur = len(RecurrenceOptions) - 1
		}
		m.createRecurrence = RecurrenceOptions[cur]
	}
	return m, nil
}

// --- Adjust mode ---

func (m Model) updateAdjust(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	date := m.SelectedDate()

	// Helper to move event and follow with cursor
	moveAndFollow := func(delta int) {
		err := m.store.MoveEventByID(m.adjustEventID, delta)
		if err != nil {
			m.statusMsg = err.Error()
		} else {
			m.saveEvents()
			// Move cursor to follow the event
			baseDate, baseIdx := m.store.FindEventByID(m.adjustEventID)
			if baseIdx >= 0 {
				ev := m.store.events[DateKey(baseDate)][baseIdx]
				m.cursorMin = ev.StartMin
			}
			// Update adjustIndex to match GetByDate position
			events := m.store.GetByDate(date)
			for i, ev := range events {
				if ev.ID == m.adjustEventID {
					m.adjustIndex = i
					break
				}
			}
			m.ensureCursorVisible()
		}
	}

	switch {
	case IsKey(msg, KeyJ):
		moveAndFollow(m.jumpStep())

	case IsKey(msg, KeyK):
		moveAndFollow(-m.jumpStep())

	case IsKey(msg, KeyShiftJ):
		moveAndFollow(1)

	case IsKey(msg, KeyShiftK):
		moveAndFollow(-1)

	case IsKey(msg, KeyH):
		// Move event to previous day
		targetDate := date.AddDate(0, 0, -1)
		_, err := m.store.MoveEventToDateByID(m.adjustEventID, targetDate)
		if err != nil {
			m.statusMsg = err.Error()
		} else {
			m.saveEvents()
			// Move cursor to follow
			if m.cursorCol > 0 {
				m.cursorCol--
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
			}
			// Update adjustIndex for the new date
			newDate := m.SelectedDate()
			events := m.store.GetByDate(newDate)
			for i, ev := range events {
				if ev.ID == m.adjustEventID {
					m.adjustIndex = i
					break
				}
			}
		}

	case IsKey(msg, KeyL):
		// Move event to next day
		targetDate := date.AddDate(0, 0, 1)
		_, err := m.store.MoveEventToDateByID(m.adjustEventID, targetDate)
		if err != nil {
			m.statusMsg = err.Error()
		} else {
			m.saveEvents()
			// Move cursor to follow
			if m.cursorCol < m.dayCount-1 {
				m.cursorCol++
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
			}
			// Update adjustIndex for the new date
			newDate := m.SelectedDate()
			events := m.store.GetByDate(newDate)
			for i, ev := range events {
				if ev.ID == m.adjustEventID {
					m.adjustIndex = i
					break
				}
			}
		}

	case IsKey(msg, KeyS):
		// Open inline edit menu for this event
		events := m.store.GetByDate(date)
		if m.adjustIndex >= 0 && m.adjustIndex < len(events) {
			ev := events[m.adjustIndex]
			m.mode = ModeEditMenu
			m.editMenuIndex = m.adjustIndex
			m.editMenuField = 0
			m.editMenuBuf = ""
			m.editMenuActive = false
			m.editMenuValues = [7]string{
				ev.Title,
				ev.Desc,
				date.Format("2006-01-02"),
				MinToTime(ev.StartMin),
				MinToTime(ev.EndMin),
				ev.Recurrence,
				ev.RecurUntilStr,
			}
		}

	case IsKey(msg, KeyEsc), IsKey(msg, KeyEnter):
		m.mode = ModeNavigate
		m.adjustIndex = -1
		m.adjustEventID = ""
	}
	return m, nil
}

// --- Confirm recurring delete mode ---

func (m Model) updateConfirmRecurDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	date := m.SelectedDate()
	events := m.store.GetByDate(date)

	if m.recurDeleteIdx < 0 || m.recurDeleteIdx >= len(events) {
		m.mode = ModeNavigate
		m.statusMsg = "Event no longer exists"
		return m, nil
	}

	ev := events[m.recurDeleteIdx]

	switch {
	case IsKey(msg, "o"):
		// Delete just this occurrence — add exception date
		m.pushUndo()
		if m.store.IsVirtualIndex(date, m.recurDeleteIdx) {
			// Virtual occurrence: add exception to the base event
			err := m.store.AddException(ev.ID, date)
			if err != nil {
				m.statusMsg = err.Error()
			} else {
				m.statusMsg = fmt.Sprintf("Skipped %q on %s", ev.Title, date.Format("Jan 02"))
				m.saveEvents()
			}
		} else {
			// Stored on this date (the base event's own date):
			// add exception and it won't generate an occurrence for itself
			// But since the base is stored here, we need to just add the exception
			err := m.store.AddException(ev.ID, date)
			if err != nil {
				m.statusMsg = err.Error()
			} else {
				m.statusMsg = fmt.Sprintf("Skipped %q on %s", ev.Title, date.Format("Jan 02"))
				m.saveEvents()
			}
		}
		m.resetOverlapIndex()
		m.mode = ModeNavigate

	case IsKey(msg, "a"):
		// Delete the entire recurring event (base + all occurrences)
		m.pushUndo()
		err := m.store.DeleteByID(ev.ID)
		if err != nil {
			m.statusMsg = err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("Deleted all occurrences of %q", ev.Title)
			m.saveEvents()
		}
		m.resetOverlapIndex()
		m.mode = ModeNavigate

	case IsKey(msg, KeyEsc):
		m.statusMsg = ""
		m.mode = ModeNavigate
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width < MinWidth || m.height < MinHeight {
		return SmallTermStyle.Render("Terminal too small (need 80x24)")
	}

	// Detail view is fullscreen
	if m.mode == ModeDetail {
		statusBar := m.renderStatusBar()
		return RenderDetail(&m) + "\n" + statusBar
	}

	// Settings view is fullscreen
	if m.mode == ModeSettings {
		statusBar := m.renderStatusBar()
		return RenderSettings(&m) + "\n" + statusBar
	}

	// Edit menu is fullscreen
	if m.mode == ModeEditMenu {
		statusBar := m.renderStatusBar()
		return RenderEditMenu(&m) + "\n" + statusBar
	}

	// Month view
	if m.viewMode == ViewMonth {
		monthGrid := RenderMonth(&m)
		statusBar := m.renderStatusBar()
		return monthGrid + "\n" + statusBar
	}

	// Year view
	if m.viewMode == ViewYear {
		yearGrid := RenderYear(&m)
		statusBar := m.renderStatusBar()
		return yearGrid + "\n" + statusBar
	}

	grid := RenderGrid(&m)
	statusBar := m.renderStatusBar()

	if m.mode == ModeInput {
		label := "Event title: "
		prompt := "\n" + InputPromptStyle.Render(label) + m.inputBuffer + "█"
		return grid + prompt + "\n" + statusBar
	}

	if m.mode == ModeInputDesc {
		label := "Description (Enter to skip): "
		prompt := "\n" + InputPromptStyle.Render(label) + m.descBuffer + "█"
		return grid + prompt + "\n" + statusBar
	}

	if m.mode == ModeInputRecurrence {
		recLabel := RecurrenceLabel(m.createRecurrence)
		prompt := "\n" + InputPromptStyle.Render("Repeat: ") + recLabel + "  " +
			StatusHintStyle.Render("r: cycle  Enter: confirm  Esc: cancel")
		return grid + prompt + "\n" + statusBar
	}

	if m.mode == ModeSearch {
		prompt := "\n" + InputPromptStyle.Render("/") + m.searchQuery + "█"
		matchInfo := ""
		if len(m.searchMatches) > 0 {
			matchInfo = fmt.Sprintf(" (%d matches)", len(m.searchMatches))
		}
		return grid + prompt + StatusHintStyle.Render(matchInfo) + "\n" + statusBar
	}

	if m.mode == ModeGoto {
		prompt := "\n" + InputPromptStyle.Render("Go to time: ") + m.gotoBuffer + "█"
		return grid + prompt + "\n" + statusBar
	}

	if m.mode == ModeGotoDay {
		prompt := "\n" + InputPromptStyle.Render("Go to day: ") + m.gotoBuffer + "█"
		return grid + prompt + "\n" + statusBar
	}

	return grid + "\n" + statusBar
}

// renderStatusBar renders the bottom status bar.
func (m Model) renderStatusBar() string {
	date := m.SelectedDate().Format("Mon Jan 02 2006")
	cursorTime := MinToTime(m.cursorMin)

	var mode, hints, extra string

	switch m.mode {
	case ModeNavigate:
		mode = StatusModeStyle.Render(" WEEK ")
		searchInfo := ""
		if m.searchActive {
			searchInfo = fmt.Sprintf(" /%s [%d/%d]", m.searchQuery, m.searchIndex+1, len(m.searchMatches))
		}
		overlapInfo := ""
		curDate := m.SelectedDate()
		overlaps := m.sortedOverlapIndices(curDate, m.cursorMin)
		if len(overlaps) > 1 {
			events := m.store.GetByDate(curDate)
			// Find position of selected event in overlap list by ID
			pos := 0
			for i, idx := range overlaps {
				if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
					pos = i
					break
				}
			}
			selected := overlaps[pos]
			if selected < len(events) {
				overlapInfo = fmt.Sprintf("  [%d/%d: %s]", pos+1, len(overlaps), events[selected].Title)
			}
		}
		info := fmt.Sprintf(" %s  %s%s%s", date, cursorTime, searchInfo, overlapInfo)
		if m.settings.ShowHints {
			hints = StatusHintStyle.Render(info +
				"  hjkl:nav a:add e:edit m:move dd:del /:search g:goto 1-9:cols c:now ^d/^u:jump M:month Y:year S:set q:quit")
		} else {
			hints = StatusHintStyle.Render(info)
		}
	case ModeCreate:
		mode = StatusCreateModeStyle.Render(" CREATE ")
		hints = StatusHintStyle.Render(
			fmt.Sprintf(" %s  %s-%s  j/k: jump  J/K: 1min  Enter: confirm  Esc: cancel",
				date, MinToTime(m.createStart), MinToTime(m.createEnd)))
	case ModeInput:
		mode = StatusCreateModeStyle.Render(" CREATE ")
		hints = StatusHintStyle.Render(" Type title, Enter: next, Esc: cancel")
	case ModeInputDesc:
		mode = StatusCreateModeStyle.Render(" CREATE ")
		hints = StatusHintStyle.Render(
			fmt.Sprintf(" %q  Type description (optional), Enter: next, Esc: cancel", m.inputBuffer))
	case ModeInputRecurrence:
		mode = StatusCreateModeStyle.Render(" CREATE ")
		hints = StatusHintStyle.Render(
			fmt.Sprintf(" %q  Repeat: %s  r/R: cycle  Enter: save  Esc: cancel",
				m.inputBuffer, RecurrenceLabel(m.createRecurrence)))
	case ModeAdjust:
		mode = StatusAdjustModeStyle.Render(" ADJUST ")
		hints = StatusHintStyle.Render(
			fmt.Sprintf(" %s  %s  j/k: jump  J/K: 1min  h/l: day  s: edit  Enter/Esc: done", date, cursorTime))
	case ModeDetail:
		mode = StatusDetailModeStyle.Render(" DETAIL ")
		hints = StatusHintStyle.Render(" e: edit  Esc/q: back")
	case ModeGoto:
		mode = StatusGotoModeStyle.Render(" GOTO ")
		hints = StatusHintStyle.Render(" Type time (12, 1200, 12:00), Enter: go, Esc: cancel")
	case ModeGotoDay:
		mode = StatusGotoModeStyle.Render(" GOTO DAY ")
		hints = StatusHintStyle.Render(" Type day of month (1-31), Enter: go, Esc: cancel")
	case ModeSearch:
		mode = StatusSearchModeStyle.Render(" SEARCH ")
		hints = StatusHintStyle.Render(fmt.Sprintf(" %d matches  Enter: confirm  Esc: cancel", len(m.searchMatches)))
	case ModeMonth:
		mode = StatusMonthModeStyle.Render(" MONTH ")
		monthDate := m.monthCursor.Format("January 2006")
		selectedDay := m.monthCursor.Format("Mon Jan 02")
		eventCount := m.store.EventCount(m.monthCursor)
		evInfo := ""
		if eventCount > 0 {
			evInfo = fmt.Sprintf("  %d events", eventCount)
		}
		hints = StatusHintStyle.Render(
			fmt.Sprintf(" %s  %s%s  h/l: day  j/k: week  H/L: month  Enter: open  c: today  Y: year  M: back  q: quit",
				monthDate, selectedDay, evInfo))
	case ModeYear:
		mode = StatusYearModeStyle.Render(" YEAR ")
		yearLabel := m.yearCursor.Format("2006")
		selectedDay := m.yearCursor.Format("Mon Jan 02")
		eventCount := m.store.EventCount(m.yearCursor)
		evInfo := ""
		if eventCount > 0 {
			evInfo = fmt.Sprintf("  %d events", eventCount)
		}
		hints = StatusHintStyle.Render(
			fmt.Sprintf(" %s  %s%s  h/l: day  j/k: week  H/L: month  J/K: year  Enter: open  c: today  M: month  Y/Esc: back  q: quit",
				yearLabel, selectedDay, evInfo))
	case ModeSettings:
		mode = StatusSettingsModeStyle.Render(" SETTINGS ")
		hints = StatusHintStyle.Render(" j/k: navigate  Enter/Space: toggle  Esc/q: close")
	case ModeEditMenu:
		mode = StatusAdjustModeStyle.Render(" EDIT ")
		if m.editMenuActive {
			hints = StatusHintStyle.Render(" Type value  Enter: confirm  Esc: cancel")
		} else {
			hints = StatusHintStyle.Render(" j/k: field  Enter: edit  Esc: back")
		}
	case ModeConfirmRecurDelete:
		mode = WarningStyle.Render(" DELETE ")
		hints = StatusHintStyle.Render(" (o): delete this occurrence  (a): delete all  Esc: cancel")
	}

	if m.statusMsg != "" {
		extra = "  " + WarningStyle.Render(m.statusMsg)
	}

	bar := mode + hints + extra
	return StatusBarStyle.Width(m.width).Render(bar)
}
