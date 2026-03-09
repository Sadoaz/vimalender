package internal

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	term "github.com/charmbracelet/x/term"
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
	ModeConfirmRecurMove        // "Move: (o)ne / (a)ll?" for recurring events
	ModeDetail                  // fullscreen event detail view
	ModeGoto                    // typing go-to time
	ModeGotoDay                 // typing go-to day of month
	ModeSearch                  // typing search query
	ModeVisual                  // visual area selection
	ModeHelp                    // keybinding help
	ModeMonth                   // month view navigation
	ModeYear                    // year view navigation
	ModeSettings                // settings menu
	ModeEditMenu                // inline event editor
)

const MaxCreateSpanDays = 30

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

const DefaultZoomLevel = 30
const MaxPreciseZoomLevel = 30
const MinZoomLevel = 1

// ZoomLevels are the supported zoom levels (minutes per row).
// Keep this small and deliberate so zooming lands on useful calendar views.
var ZoomLevels = []int{1, 5, 15, 20, 30}

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
	adjustIndex               int
	adjustEventID             string // ID of event being adjusted
	adjustCol                 int    // pinned visual column during adjust
	adjustEventIDs            []string
	adjustRecurring           bool
	adjustRecurringSelection  bool
	adjustPreviewBase         Event
	adjustPreviewDuration     int
	adjustPreviewDelta        int
	adjustOccurrenceDate      time.Time
	adjustBasePartIDs         []string
	adjustPreviewGroupID      string
	adjustSelectedOccurrences []Event
	confirmVisualRecurring    bool

	// Input mode state
	inputBuffer string
	descBuffer  string // description input buffer
	editIndex   int    // -1 for new event, >=0 for editing existing
	isEdit      bool   // true when editing existing event title

	// Detail view state
	detailIndex int // event index being viewed

	// Goto mode state
	gotoBuffer     string
	gotoReturnMode Mode

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
	settings           Settings // persistent user preferences
	settingsCursor     int      // selected option in settings menu
	settingsEditActive bool
	settingsEditKey    string
	settingsEditBuffer string
	helpCursor         int
	helpScroll         int
	helpRebinding      bool
	helpRebindKey      string

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
	undoStack []undoSnapshot
	// Redo stack — snapshots pushed when undoing
	redoStack []undoSnapshot

	// Overlap selection: tracks the event ID so selection is stable
	// even when GetByDate ordering changes.  "" = no selection.
	selectedOverlapEvt string
	selectedLogicalEvt string
	clipboard          []ClipboardItem
	visualAnchorDate   time.Time
	visualAnchorMin    int
	pendingYank        bool
}

type undoSnapshot struct {
	events             map[time.Time][]Event
	windowStart        time.Time
	cursorCol          int
	cursorMin          int
	viewportOffset     int
	selectedOverlapEvt string
}

type ClipboardItem struct {
	Title       string
	Desc        string
	Notes       string
	Duration    int
	Recurrence  string
	StartOffset int
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
	SetKeyBindingOverrides(settings.Keybindings)

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

	zoomLevel := settings.ZoomLevel
	if zoomLevel <= 0 {
		zoomLevel = DefaultZoomLevel
	}

	// Snap restored position to grid boundaries
	cursorMin = (cursorMin / zoomLevel) * zoomLevel
	viewportOffset = (viewportOffset / zoomLevel) * zoomLevel

	width, height := initialTerminalSize()

	return Model{
		mode:               ModeNavigate,
		windowStart:        windowStart,
		cursorCol:          cursorCol,
		cursorMin:          cursorMin,
		viewportOffset:     viewportOffset,
		store:              store,
		width:              width,
		height:             height,
		zoomLevel:          zoomLevel,
		dayCount:           settings.DayCount,
		settings:           settings,
		adjustIndex:        -1,
		editIndex:          -1,
		selectedOverlapEvt: "",
		statusMsg:          errMsg,
	}
}

func initialTerminalSize() (int, int) {
	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 && h > 0 {
		return w, h
	}
	w, _ := strconv.Atoi(os.Getenv("COLUMNS"))
	h, _ := strconv.Atoi(os.Getenv("LINES"))
	return w, h
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
	if m.cursorMin == m.dayStartMin() {
		if idx := m.selectedAllDayEventIndex(date); idx >= 0 {
			return idx
		}
	}
	indices := m.store.VisualEventsAtMinute(date, m.cursorMin, m.MinutesPerRow())
	if len(indices) == 0 {
		return -1
	}
	if m.searchActive && m.searchIndex >= 0 && m.searchIndex < len(m.searchMatches) {
		match := m.searchMatches[m.searchIndex]
		if DateKey(match.Date).Equal(DateKey(date)) {
			for _, idx := range indices {
				if idx < len(events) && events[idx].ID == match.EventID {
					return idx
				}
			}
		}
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

func (m *Model) allDayEventIndices(date time.Time) []int {
	events := m.store.GetByDate(date)
	var indices []int
	seen := map[string]bool{}
	for i, ev := range events {
		segments := occurrenceSegments(m, ev)
		duration := 0
		for _, seg := range segments {
			duration += seg.EndMin - seg.StartMin
		}
		if len(segments) <= 1 || duration < MinutesPerDay {
			continue
		}
		key := m.selectionKeyForEvent(ev)
		if seen[key] {
			continue
		}
		seen[key] = true
		indices = append(indices, i)
	}
	return indices
}

func (m *Model) selectedAllDayEventIndex(date time.Time) int {
	events := m.store.GetByDate(date)
	indices := m.allDayEventIndices(date)
	if len(indices) == 0 {
		return -1
	}
	if m.selectedOverlapEvt != "" {
		for _, idx := range indices {
			if idx < len(events) && events[idx].ID == m.selectedOverlapEvt {
				return idx
			}
		}
	}
	return indices[0]
}

// resetOverlapIndex resets overlap selection (used when moving to a different day or deleting).
func (m *Model) resetOverlapIndex() {
	m.selectedOverlapEvt = ""
}

func (m *Model) selectedLogicalEventID() string {
	if m.selectedLogicalEvt != "" {
		return m.selectedLogicalEvt
	}
	idx := m.selectedEventIndex()
	if idx == -1 {
		return ""
	}
	events := m.store.GetByDate(m.SelectedDate())
	if idx < 0 || idx >= len(events) {
		return ""
	}
	return events[idx].ID
}

func (m *Model) clipboardItemForEvent(id string) (ClipboardItem, string, error) {
	start, duration, err := m.store.LogicalEventByID(id)
	if err != nil {
		return ClipboardItem{}, "", err
	}
	return ClipboardItem{
		Title:       start.Title,
		Desc:        start.Desc,
		Notes:       start.Notes,
		Duration:    duration,
		Recurrence:  start.Recurrence,
		StartOffset: 0,
	}, start.Title, nil
}

func (m *Model) visualSelectionBounds() (time.Time, time.Time, int, int, bool) {
	if m.mode != ModeVisual {
		return time.Time{}, time.Time{}, 0, 0, false
	}
	startDate := DateKey(m.visualAnchorDate)
	endDate := DateKey(m.SelectedDate())
	if endDate.Before(startDate) {
		startDate, endDate = endDate, startDate
	}
	minMin, maxMin := m.visualAnchorMin, m.cursorMin
	if maxMin < minMin {
		minMin, maxMin = maxMin, minMin
	}
	maxMin += m.MinutesPerRow()
	if maxMin > MinutesPerDay {
		maxMin = MinutesPerDay
	}
	return startDate, endDate, minMin, maxMin, true
}

func (m *Model) selectionKeyForEvent(ev Event) string {
	if !ev.IsRecurring() {
		return groupKey(ev)
	}
	if ev.GroupID == "" {
		return fmt.Sprintf("%s@%s", ev.ID, DateKey(ev.Date).Format("2006-01-02"))
	}
	baseDate, baseIdx, _ := m.store.findEventRecordByID(ev.ID)
	if baseIdx < 0 {
		return fmt.Sprintf("%s@%s", ev.ID, DateKey(ev.Date).Format("2006-01-02"))
	}
	baseGroup := m.store.groupedEventsByGroupID(ev.GroupID)
	if len(baseGroup) == 0 {
		return fmt.Sprintf("%s@%s", ev.ID, DateKey(ev.Date).Format("2006-01-02"))
	}
	anchorBaseDate := DateKey(baseGroup[0].Date)
	offset := int(DateKey(baseDate).Sub(anchorBaseDate).Hours() / 24)
	occurrenceDate := DateKey(ev.Date).AddDate(0, 0, -offset)
	return fmt.Sprintf("%s@%s", ev.GroupID, occurrenceDate.Format("2006-01-02"))
}

func (m *Model) visualSelectedKeys() map[string]bool {
	startDate, endDate, minMin, maxMin, ok := m.visualSelectionBounds()
	if !ok {
		return nil
	}
	selected := make(map[string]bool)
	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		for _, ev := range m.store.GetByDate(day) {
			if ev.StartMin < maxMin && ev.EndMin > minMin {
				selected[m.selectionKeyForEvent(ev)] = true
			}
		}
	}
	return selected
}

func (m *Model) visualSelectedOccurrences() []Event {
	startDate, endDate, minMin, maxMin, ok := m.visualSelectionBounds()
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	var picked []Event
	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		for _, ev := range m.store.GetByDate(day) {
			if ev.StartMin >= maxMin || ev.EndMin <= minMin {
				continue
			}
			key := m.selectionKeyForEvent(ev)
			if seen[key] {
				continue
			}
			seen[key] = true
			picked = append(picked, ev)
		}
	}
	sort.Slice(picked, func(i, j int) bool {
		if !DateKey(picked[i].Date).Equal(DateKey(picked[j].Date)) {
			return DateKey(picked[i].Date).Before(DateKey(picked[j].Date))
		}
		if picked[i].StartMin != picked[j].StartMin {
			return picked[i].StartMin < picked[j].StartMin
		}
		return picked[i].Title < picked[j].Title
	})
	return picked
}

func (m *Model) visualClipboardItems() []ClipboardItem {
	if m.mode != ModeVisual {
		return nil
	}
	anchorDate, _, anchorMin, _, ok := m.visualSelectionBounds()
	if !ok {
		return nil
	}
	var items []ClipboardItem
	for _, ev := range m.visualSelectedOccurrences() {
		start, duration, err := m.store.LogicalEventByID(ev.ID)
		if err != nil {
			continue
		}
		start.Date = DateKey(ev.Date)
		start.StartMin = ev.StartMin
		items = append(items, ClipboardItem{
			Title:       start.Title,
			Desc:        start.Desc,
			Notes:       start.Notes,
			Duration:    duration,
			Recurrence:  start.Recurrence,
			StartOffset: int(DateKey(start.Date).Sub(anchorDate).Hours()/24)*MinutesPerDay + (start.StartMin - anchorMin),
		})
	}
	return items
}

func (m *Model) visualSelectedEventIDs() []string {
	type selectedEvent struct {
		id    string
		date  time.Time
		start int
	}
	var picked []selectedEvent
	for _, ev := range m.visualSelectedOccurrences() {
		picked = append(picked, selectedEvent{id: ev.ID, date: DateKey(ev.Date), start: ev.StartMin})
	}
	sort.Slice(picked, func(i, j int) bool {
		if !picked[i].date.Equal(picked[j].date) {
			return picked[i].date.Before(picked[j].date)
		}
		if picked[i].start != picked[j].start {
			return picked[i].start < picked[j].start
		}
		return picked[i].id < picked[j].id
	})
	ids := make([]string, len(picked))
	for i, ev := range picked {
		ids[i] = ev.id
	}
	return ids
}

func (m *Model) deleteVisualSelection() (int, error) {
	selected := m.visualSelectedOccurrences()
	deleted := 0
	for _, ev := range selected {
		if ev.IsRecurring() {
			if err := m.store.AddException(ev.ID, ev.Date); err != nil {
				return deleted, err
			}
		} else {
			if err := m.store.DeleteByID(ev.ID); err != nil {
				return deleted, err
			}
		}
		deleted++
	}
	return deleted, nil
}

func (m *Model) clearVisualSelection() {
	m.pendingYank = false
	if m.mode == ModeVisual {
		m.mode = ModeNavigate
	}
}

func (m *Model) recurringAdjustPreviewSegments() []Event {
	if !m.adjustRecurring {
		return nil
	}
	startMin := m.adjustPreviewBase.StartMin + m.adjustPreviewDelta
	startDate := DateKey(m.adjustOccurrenceDate)
	for startMin < 0 {
		startMin += MinutesPerDay
		startDate = startDate.AddDate(0, 0, -1)
	}
	for startMin >= MinutesPerDay {
		startMin -= MinutesPerDay
		startDate = startDate.AddDate(0, 0, 1)
	}
	segments, err := buildSpanningSegments(m.adjustPreviewBase, startDate, startMin, m.adjustPreviewDuration, m.adjustPreviewGroupID)
	if err != nil {
		return nil
	}
	for i := range segments {
		segments[i].Recurrence = ""
		segments[i].RecurUntilStr = ""
		segments[i].ExceptionDates = nil
	}
	return segments
}

func (m *Model) recurringSelectionPreviewEvents() []Event {
	if !m.adjustRecurringSelection || len(m.adjustSelectedOccurrences) == 0 {
		return nil
	}
	var all []Event
	for i, occ := range m.adjustSelectedOccurrences {
		start, duration, err := m.store.LogicalEventByID(occ.ID)
		if err != nil {
			continue
		}
		startMin := occ.StartMin + m.adjustPreviewDelta
		startDate := DateKey(occ.Date)
		for startMin < 0 {
			startMin += MinutesPerDay
			startDate = startDate.AddDate(0, 0, -1)
		}
		for startMin >= MinutesPerDay {
			startMin -= MinutesPerDay
			startDate = startDate.AddDate(0, 0, 1)
		}
		template := start
		template.Recurrence = ""
		template.RecurUntilStr = ""
		template.ExceptionDates = nil
		template.ID = fmt.Sprintf("__selection_preview__%d", i)
		template.GroupID = ""
		segments, err := buildSpanningSegments(template, startDate, startMin, duration, "")
		if err != nil {
			continue
		}
		for j := range segments {
			segments[j].Recurrence = ""
			segments[j].RecurUntilStr = ""
			segments[j].ExceptionDates = nil
		}
		all = append(all, segments...)
	}
	return all
}

func (m *Model) applyRecurringAdjustPreview(date time.Time, events []Event) []Event {
	if !m.adjustRecurring && !m.adjustRecurringSelection {
		return events
	}
	if m.adjustRecurringSelection {
		key := DateKey(date)
		selected := make(map[string]bool)
		for _, ev := range m.adjustSelectedOccurrences {
			selected[m.selectionKeyForEvent(ev)] = true
		}
		filtered := make([]Event, 0, len(events))
		for _, ev := range events {
			if selected[m.selectionKeyForEvent(ev)] {
				continue
			}
			filtered = append(filtered, ev)
		}
		for _, seg := range m.recurringSelectionPreviewEvents() {
			if DateKey(seg.Date).Equal(key) {
				filtered = append(filtered, seg)
			}
		}
		return filtered
	}
	key := DateKey(date)
	anchorBaseDate := DateKey(m.adjustPreviewBase.Date)
	filtered := make([]Event, 0, len(events))
	for _, ev := range events {
		remove := false
		for _, id := range m.adjustBasePartIDs {
			if ev.ID != id {
				continue
			}
			baseDate, baseIdx := m.store.FindEventByID(id)
			if baseIdx < 0 {
				continue
			}
			offset := int(DateKey(baseDate).Sub(anchorBaseDate).Hours() / 24)
			occDate := DateKey(m.adjustOccurrenceDate).AddDate(0, 0, offset)
			if key.Equal(occDate) {
				remove = true
				break
			}
		}
		if !remove {
			filtered = append(filtered, ev)
		}
	}
	for _, seg := range m.recurringAdjustPreviewSegments() {
		if DateKey(seg.Date).Equal(key) {
			filtered = append(filtered, seg)
		}
	}
	return filtered
}

// autoSelectOverlapEvent picks the best event to highlight at the current
// cursor position after j/k movement. If an event starts on this row it gets
// priority (so scrolling "enters" new events naturally). Otherwise the
// currently selected event is kept if it still covers this row.
func (m *Model) autoSelectOverlapEvent() {
	date := m.SelectedDate()
	events := m.store.GetByDate(date)
	if m.cursorMin == m.dayStartMin() {
		if idx := m.selectedAllDayEventIndex(date); idx >= 0 {
			m.selectedOverlapEvt = events[idx].ID
			return
		}
	}
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
	if minute == m.dayStartMin() {
		if indices := m.allDayEventIndices(date); len(indices) > 0 {
			return indices
		}
	}
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
	if m.searchQuery != "" || m.searchActive {
		m.updateSearchMatches()
		if len(m.searchMatches) == 0 {
			m.searchActive = false
			m.searchIndex = 0
		} else if m.searchIndex >= len(m.searchMatches) {
			m.searchIndex = len(m.searchMatches) - 1
		}
	}
}

// pushUndo saves a snapshot of the current events for undo.
func (m *Model) pushUndo() {
	const maxUndo = 50
	m.undoStack = append(m.undoStack, undoSnapshot{
		events:             m.store.Snapshot(),
		windowStart:        m.windowStart,
		cursorCol:          m.cursorCol,
		cursorMin:          m.cursorMin,
		viewportOffset:     m.viewportOffset,
		selectedOverlapEvt: m.selectedOverlapEvt,
	})
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
	m.redoStack = append(m.redoStack, undoSnapshot{
		events:             m.store.Snapshot(),
		windowStart:        m.windowStart,
		cursorCol:          m.cursorCol,
		cursorMin:          m.cursorMin,
		viewportOffset:     m.viewportOffset,
		selectedOverlapEvt: m.selectedOverlapEvt,
	})
	snap := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.store.Restore(snap.events)
	m.windowStart = snap.windowStart
	m.cursorCol = snap.cursorCol
	m.cursorMin = snap.cursorMin
	m.viewportOffset = snap.viewportOffset
	m.selectedOverlapEvt = snap.selectedOverlapEvt
	m.saveEvents()
	return true
}

// popRedo re-applies the most recently undone action. Returns true if successful.
func (m *Model) popRedo() bool {
	if len(m.redoStack) == 0 {
		return false
	}
	m.undoStack = append(m.undoStack, undoSnapshot{
		events:             m.store.Snapshot(),
		windowStart:        m.windowStart,
		cursorCol:          m.cursorCol,
		cursorMin:          m.cursorMin,
		viewportOffset:     m.viewportOffset,
		selectedOverlapEvt: m.selectedOverlapEvt,
	})
	snap := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	m.store.Restore(snap.events)
	m.windowStart = snap.windowStart
	m.cursorCol = snap.cursorCol
	m.cursorMin = snap.cursorMin
	m.viewportOffset = snap.viewportOffset
	m.selectedOverlapEvt = snap.selectedOverlapEvt
	m.saveEvents()
	return true
}

// saveSettings persists user settings to disk.
func (m *Model) saveSettings() {
	m.settings.ZoomLevel = m.zoomLevel
	m.settings.DayCount = m.dayCount
	SetKeyBindingOverrides(m.settings.Keybindings)
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

func (m *Model) moveAdjustBy(delta int) {
	date := m.SelectedDate()
	if m.adjustRecurring {
		m.adjustPreviewDelta += delta
		preview := m.recurringAdjustPreviewSegments()
		if len(preview) > 0 {
			m.cursorMin = preview[0].StartMin
			previewDate := DateKey(preview[0].Date)
			dayDelta := int(previewDate.Sub(DateKey(date)).Hours() / 24)
			if dayDelta != 0 {
				newCursorCol := m.cursorCol + dayDelta
				for newCursorCol < 0 {
					m.windowStart = m.windowStart.AddDate(0, 0, -1)
					newCursorCol++
				}
				for newCursorCol >= m.dayCount {
					m.windowStart = m.windowStart.AddDate(0, 0, 1)
					newCursorCol--
				}
				m.cursorCol = newCursorCol
			}
			m.ensureCursorVisible()
		}
		return
	}
	if m.adjustRecurringSelection {
		m.adjustPreviewDelta += delta
		preview := m.recurringSelectionPreviewEvents()
		if len(preview) > 0 {
			m.cursorMin = preview[0].StartMin
			previewDate := DateKey(preview[0].Date)
			dayDelta := int(previewDate.Sub(DateKey(date)).Hours() / 24)
			if dayDelta != 0 {
				newCursorCol := m.cursorCol + dayDelta
				for newCursorCol < 0 {
					m.windowStart = m.windowStart.AddDate(0, 0, -1)
					newCursorCol++
				}
				for newCursorCol >= m.dayCount {
					m.windowStart = m.windowStart.AddDate(0, 0, 1)
					newCursorCol--
				}
				m.cursorCol = newCursorCol
			}
			m.ensureCursorVisible()
		}
		return
	}
	if len(m.adjustEventIDs) > 0 {
		newIDs := make([]string, 0, len(m.adjustEventIDs))
		anchorID := m.adjustEventID
		anchorDate := date
		for _, id := range m.adjustEventIDs {
			newDate, newID, err := m.store.ShiftEventByID(id, delta)
			if err != nil {
				m.statusMsg = err.Error()
				return
			}
			newIDs = append(newIDs, newID)
			if id == anchorID {
				anchorID = newID
				anchorDate = newDate
			}
		}
		m.adjustEventIDs = newIDs
		m.adjustEventID = anchorID
		m.saveEvents()
		baseDate, baseIdx := m.store.FindEventByID(anchorID)
		if baseIdx >= 0 {
			ev := m.store.events[DateKey(baseDate)][baseIdx]
			m.cursorMin = ev.StartMin
		}
		dayDelta := int(DateKey(anchorDate).Sub(DateKey(date)).Hours() / 24)
		if dayDelta != 0 {
			newCursorCol := m.cursorCol + dayDelta
			for newCursorCol < 0 {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
				newCursorCol++
			}
			for newCursorCol >= m.dayCount {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
				newCursorCol--
			}
			m.cursorCol = newCursorCol
		}
		events := m.store.GetByDate(anchorDate)
		for i, ev := range events {
			if ev.ID == anchorID {
				m.adjustIndex = i
				break
			}
		}
		m.ensureCursorVisible()
		return
	}

	newDate, newID, err := m.store.ShiftEventByID(m.adjustEventID, delta)
	if err != nil {
		m.statusMsg = err.Error()
		return
	}
	m.adjustEventID = newID
	m.saveEvents()
	baseDate, baseIdx := m.store.FindEventByID(newID)
	if baseIdx >= 0 {
		ev := m.store.events[DateKey(baseDate)][baseIdx]
		m.cursorMin = ev.StartMin
	}
	dayDelta := int(DateKey(newDate).Sub(DateKey(date)).Hours() / 24)
	if dayDelta != 0 {
		newCursorCol := m.cursorCol + dayDelta
		for newCursorCol < 0 {
			m.windowStart = m.windowStart.AddDate(0, 0, -1)
			newCursorCol++
		}
		for newCursorCol >= m.dayCount {
			m.windowStart = m.windowStart.AddDate(0, 0, 1)
			newCursorCol--
		}
		m.cursorCol = newCursorCol
	}
	events := m.store.GetByDate(newDate)
	for i, ev := range events {
		if ev.ID == newID {
			m.adjustIndex = i
			break
		}
	}
	m.ensureCursorVisible()
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

// MinutesPerRow returns the current minutes-per-row value.
func (m *Model) MinutesPerRow() int {
	if m.zoomLevel != ZoomAuto && m.zoomLevel > 0 {
		return m.zoomLevel
	}
	return m.autoMpr()
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
		} else {
			mpr := m.MinutesPerRow()
			visibleMinutes := mpr * m.viewportHeight()
			dayVisible := MinutesPerDay - m.dayStartMin()
			if visibleMinutes >= dayVisible {
				m.viewportOffset = m.dayStartMin()
			} else {
				m.ensureCursorVisible()
			}
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
		if IsKey(msg, KeyQuestion) && m.mode != ModeHelp && m.mode != ModeInput && m.mode != ModeInputDesc && m.mode != ModeInputRecurrence && m.mode != ModeSearch && m.mode != ModeGoto && m.mode != ModeGotoDay {
			m.mode = ModeHelp
			m.helpCursor = 0
			m.helpScroll = 0
			m.helpRebinding = false
			m.helpRebindKey = ""
			return m, nil
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
		case ModeVisual:
			return m.updateVisual(msg)
		case ModeHelp:
			return m.updateHelp(msg)
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
		case ModeConfirmRecurMove:
			return m.updateConfirmRecurMove(msg)
		}
	}
	return m, nil
}

func (m *Model) viewportHeight() int {
	// Reserve 1 line for the header and 1 for the bottom status bar.
	// Input/search/goto prompts need one extra line above the status bar.
	h := m.height - 2
	switch m.mode {
	case ModeInput, ModeInputDesc, ModeInputRecurrence, ModeSearch, ModeGoto, ModeGotoDay:
		h--
	}
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
	m.viewportOffset = (m.viewportOffset / mpr) * mpr
}

func (m *Model) zoomAnchorMinute() int {
	idx := m.selectedEventIndex()
	if idx >= 0 {
		events := m.store.GetByDate(m.SelectedDate())
		if idx < len(events) {
			return events[idx].StartMin
		}
	}
	return m.cursorMin
}

func (m *Model) applyZoomLevel(level int) {
	anchor := m.zoomAnchorMinute()
	m.zoomLevel = level
	mpr := m.MinutesPerRow()
	m.cursorMin = (anchor / mpr) * mpr
	m.centerViewportOnCursor()
}

// zoomIn decreases minutes-per-row (more detail).
func (m *Model) zoomIn() {
	currentMpr := m.MinutesPerRow()
	if m.zoomLevel == ZoomAuto {
		// Switch from auto to the largest predefined level that is finer than auto
		for i := len(ZoomLevels) - 1; i >= 0; i-- {
			if ZoomLevels[i] < currentMpr {
				m.applyZoomLevel(ZoomLevels[i])
				return
			}
		}
		// Auto is already at finest possible — try level 1
		if currentMpr > 1 {
			m.applyZoomLevel(1)
		}
		return
	}
	// Find current index and go one step finer
	for i := len(ZoomLevels) - 1; i >= 0; i-- {
		if ZoomLevels[i] < m.zoomLevel {
			m.applyZoomLevel(ZoomLevels[i])
			return
		}
	}
	// Already at finest level
}

// zoomOut increases minutes-per-row (less detail).
func (m *Model) zoomOut() {
	if m.zoomLevel == ZoomAuto {
		m.applyZoomLevel(DefaultZoomLevel)
		return
	}

	// Find current index and go one step coarser
	for i := 0; i < len(ZoomLevels); i++ {
		if ZoomLevels[i] > m.zoomLevel {
			m.applyZoomLevel(ZoomLevels[i])
			return
		}
	}
	// Already at coarsest predefined level
}

func (m *Model) resetDefaultView() {
	m.zoomLevel = DefaultZoomLevel
	m.cursorMin = m.dayStartMin()
	m.viewportOffset = m.dayStartMin()
}

// autoMpr returns what MinutesPerRow would be in auto mode.
func (m *Model) autoMpr() int {
	vpHeight := m.viewportHeight()
	if vpHeight <= 0 {
		return DefaultZoomLevel
	}
	visibleMinutes := MinutesPerDay - m.dayStartMin()
	target := (visibleMinutes + vpHeight - 1) / vpHeight
	if target < 1 {
		target = 1
	}
	if target > MaxPreciseZoomLevel {
		target = MaxPreciseZoomLevel
	}
	best := 1
	for _, level := range ZoomLevels {
		if level > target {
			break
		}
		best = level
	}
	return best
}

// ensureCursorVisible adjusts viewport offset so the cursor is visible.
func (m *Model) ensureCursorVisible() {
	mpr := m.MinutesPerRow()
	m.cursorMin = (m.cursorMin / mpr) * mpr
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
	m.viewportOffset = (m.viewportOffset / mpr) * mpr
}

// ensureCreateVisible adjusts viewport so the create preview end is visible.
func (m *Model) ensureCreateVisible() {
	if m.createEnd > MinutesPerDay {
		overflowCols := (m.createEnd - 1) / MinutesPerDay
		targetCol := m.cursorCol + overflowCols
		if targetCol >= m.dayCount {
			shift := targetCol - (m.dayCount - 1)
			m.windowStart = m.windowStart.AddDate(0, 0, shift)
			m.cursorCol -= shift
			if m.cursorCol < 0 {
				m.cursorCol = 0
			}
		}
		// When the preview continues into the next day, follow the overflow
		// from midnight so the continuation tail is visible immediately.
		m.viewportOffset = 0
		return
	}

	mpr := m.MinutesPerRow()
	vpHeight := m.viewportHeight()
	vpEnd := m.viewportOffset + mpr*vpHeight

	visibleEnd := m.createEnd
	if visibleEnd > MinutesPerDay {
		visibleEnd = MinutesPerDay
	}
	if visibleEnd > vpEnd {
		m.viewportOffset = visibleEnd - mpr*vpHeight
	}
	if m.createStart < m.viewportOffset {
		m.viewportOffset = m.createStart
	}
	if m.viewportOffset < 0 {
		m.viewportOffset = 0
	}
}

func formatCreateTimeRange(startMin, endMin int) string {
	endDaySuffix := ""
	if endMin > MinutesPerDay {
		endDaySuffix = fmt.Sprintf(" (+%dd)", (endMin-1)/MinutesPerDay)
	}
	endDisplay := endMin
	for endDisplay >= MinutesPerDay {
		endDisplay -= MinutesPerDay
	}
	return fmt.Sprintf("%s-%s%s", MinToTime(startMin), MinToTime(endDisplay), endDaySuffix)
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
	pct := m.settings.JumpPercent
	if pct < 1 {
		pct = 5
	}
	totalVisible := mpr * vpHeight
	step := totalVisible * pct / 100
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
	// Keep the current cursor date anchored at the left edge so resizing the
	// number of visible day columns feels stable while stepping through counts.
	curDate := m.SelectedDate()
	m.dayCount = n
	m.windowStart = curDate
	m.cursorCol = 0
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

	case IsKey(msg, KeyQuestion):
		m.mode = ModeHelp

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
		mpr := m.MinutesPerRow()
		m.cursorMin += mpr
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
		mpr := m.MinutesPerRow()
		m.cursorMin -= mpr
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
		m.createEnd = m.createStart + mpr
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
		// If on event, enter adjust mode
		idx := m.selectedEventIndex()
		if idx != -1 {
			date := m.SelectedDate()
			events := m.store.GetByDate(date)
			ev := events[idx]
			if ev.IsRecurring() {
				m.mode = ModeAdjust
				m.adjustIndex = idx
				m.adjustEventID = ev.ID
				m.adjustEventIDs = nil
				m.adjustRecurring = true
				m.adjustPreviewDelta = 0
				baseDate, baseIdx := m.store.FindEventByID(ev.ID)
				if baseIdx >= 0 {
					base := m.store.events[DateKey(baseDate)][baseIdx]
					grouped := m.store.groupedEvents(base)
					m.adjustPreviewBase = grouped[0]
					_, duration, _ := m.store.LogicalEventByID(grouped[0].ID)
					m.adjustPreviewDuration = duration
					m.adjustPreviewGroupID = "__adjust_preview__"
					m.adjustBasePartIDs = make([]string, 0, len(grouped))
					anchorBaseDate := DateKey(grouped[0].Date)
					selectedBaseDate := DateKey(baseDate)
					m.adjustOccurrenceDate = DateKey(date).AddDate(0, 0, -int(selectedBaseDate.Sub(anchorBaseDate).Hours()/24))
					for _, part := range grouped {
						m.adjustBasePartIDs = append(m.adjustBasePartIDs, part.ID)
					}
				}
			} else {
				m.pushUndo()
				m.mode = ModeAdjust
				m.adjustIndex = idx
				m.adjustEventID = ev.ID
				m.adjustEventIDs = nil
				m.adjustRecurring = false
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
		id := m.selectedLogicalEventID()
		if id != "" {
			item, title, err := m.clipboardItemForEvent(id)
			if err == nil {
				m.clipboard = []ClipboardItem{item}
				m.pushUndo()
				_ = m.store.DeleteByID(id)
				m.saveEvents()
				m.resetOverlapIndex()
				m.statusMsg = fmt.Sprintf("Cut %q", title)
			}
		}

	case IsKey(msg, KeyY):
		id := m.selectedLogicalEventID()
		if id != "" {
			item, title, err := m.clipboardItemForEvent(id)
			if err == nil {
				m.clipboard = []ClipboardItem{item}
				m.statusMsg = fmt.Sprintf("Copied %q", title)
			}
		}

	case IsKey(msg, KeyP):
		if len(m.clipboard) > 0 {
			anchorDate := m.SelectedDate()
			anchorMin := m.cursorMin
			m.pushUndo()
			for _, item := range m.clipboard {
				totalStart := anchorMin + item.StartOffset
				dayShift := 0
				for totalStart < 0 {
					totalStart += MinutesPerDay
					dayShift--
				}
				for totalStart >= MinutesPerDay {
					totalStart -= MinutesPerDay
					dayShift++
				}
				err := m.store.AddSpanningEvent(Event{
					Title:      item.Title,
					Desc:       item.Desc,
					Notes:      item.Notes,
					Date:       anchorDate.AddDate(0, 0, dayShift),
					StartMin:   totalStart,
					EndMin:     totalStart + item.Duration,
					Recurrence: item.Recurrence,
				})
				if err != nil {
					m.statusMsg = err.Error()
					return m, nil
				}
			}
			m.saveEvents()
			m.statusMsg = fmt.Sprintf("Pasted %d event(s)", len(m.clipboard))
		}

	case IsKey(msg, KeyShiftV):
		m.mode = ModeVisual
		m.visualAnchorDate = m.SelectedDate()
		m.visualAnchorMin = m.cursorMin
		m.pendingYank = false
		m.statusMsg = "Visual selection"

	case IsKey(msg, KeyPlus):
		m.zoomIn()
		m.saveSettings()
		m.statusMsg = fmt.Sprintf("Zoom %d min/row", m.MinutesPerRow())

	case IsKey(msg, KeyMinus):
		m.zoomOut()
		m.saveSettings()
		m.statusMsg = fmt.Sprintf("Zoom %d min/row", m.MinutesPerRow())

	case IsKey(msg, KeyEquals):
		m.statusMsg = "Use 0 for default view"

	case IsKey(msg, Key0):
		m.resetDefaultView()
		m.saveSettings()
		m.statusMsg = fmt.Sprintf("Default view (%d min/row)", m.MinutesPerRow())

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
		mpr := m.MinutesPerRow()
		newEnd := m.createEnd + mpr
		if newEnd > MaxCreateSpanDays*MinutesPerDay {
			newEnd = MaxCreateSpanDays * MinutesPerDay
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyK):
		mpr := m.MinutesPerRow()
		newEnd := m.createEnd - mpr
		minEnd := m.createStart + mpr
		if minEnd > MinutesPerDay {
			minEnd = m.createStart + 1
		}
		if newEnd < minEnd {
			newEnd = minEnd
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyCtrlD):
		step := m.createFastStep()
		newEnd := m.createEnd + step
		if newEnd > MaxCreateSpanDays*MinutesPerDay {
			newEnd = MaxCreateSpanDays * MinutesPerDay
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyCtrlU):
		step := m.createFastStep()
		newEnd := m.createEnd - step
		minEnd := m.createStart + 1
		if newEnd < minEnd {
			newEnd = minEnd
		}
		m.createEnd = newEnd
		m.ensureCreateVisible()

	case IsKey(msg, KeyShiftJ):
		newEnd := m.createEnd + 1
		if newEnd > MaxCreateSpanDays*MinutesPerDay {
			newEnd = MaxCreateSpanDays * MinutesPerDay
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
		// Snap create times to grid boundaries before entering title input
		mpr := m.MinutesPerRow()
		m.createStart = (m.createStart / mpr) * mpr
		m.createEnd = ((m.createEnd + mpr - 1) / mpr) * mpr
		if m.createEnd <= m.createStart {
			m.createEnd = m.createStart + mpr
		}
		if m.createEnd > MaxCreateSpanDays*MinutesPerDay {
			m.createEnd = MaxCreateSpanDays * MinutesPerDay
		}
		m.mode = ModeInput
		m.isEdit = false
		m.editIndex = -1
		m.inputBuffer = ""
	}
	return m, nil
}

func (m Model) updateVisual(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc), IsKey(msg, KeyShiftV):
		m.clearVisualSelection()
		m.statusMsg = ""
		return m, nil

	case IsKey(msg, KeyY):
		if m.pendingYank {
			m.clipboard = m.visualClipboardItems()
			m.clearVisualSelection()
			m.statusMsg = fmt.Sprintf("Copied %d event(s)", len(m.clipboard))
			return m, nil
		}
		m.pendingYank = true
		m.statusMsg = "y-"
		return m, nil

	case IsKey(msg, KeyX):
		m.clipboard = m.visualClipboardItems()
		m.pushUndo()
		deleted, err := m.deleteVisualSelection()
		if err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		m.saveEvents()
		m.clearVisualSelection()
		m.resetOverlapIndex()
		m.statusMsg = fmt.Sprintf("Cut %d event(s)", deleted)
		return m, nil

	case IsKey(msg, KeyD):
		m.pushUndo()
		deleted, err := m.deleteVisualSelection()
		if err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		m.saveEvents()
		m.clearVisualSelection()
		m.resetOverlapIndex()
		m.statusMsg = fmt.Sprintf("Deleted %d event(s)", deleted)
		return m, nil

	case IsKey(msg, KeyM):
		selectedOccs := m.visualSelectedOccurrences()
		allRecurring := len(selectedOccs) > 0
		for _, ev := range selectedOccs {
			if !ev.IsRecurring() {
				allRecurring = false
				break
			}
		}
		if allRecurring {
			m.adjustSelectedOccurrences = append([]Event{}, selectedOccs...)
			m.adjustRecurringSelection = true
			m.adjustRecurring = false
			m.adjustPreviewDelta = 0
			m.mode = ModeAdjust
			m.confirmVisualRecurring = false
			m.clearVisualSelection()
			if len(m.adjustSelectedOccurrences) > 0 {
				first := m.adjustSelectedOccurrences[0]
				m.cursorCol = int(DateKey(first.Date).Sub(DateKey(m.windowStart)).Hours() / 24)
				m.cursorMin = first.StartMin
				m.ensureCursorVisible()
			}
			return m, nil
		}
		ids := m.visualSelectedEventIDs()
		if len(ids) > 0 {
			m.pushUndo()
			m.adjustEventIDs = append([]string{}, ids...)
			m.adjustEventID = ids[0]
			m.mode = ModeAdjust
			m.clearVisualSelection()
			date, idx := m.store.FindEventByID(m.adjustEventID)
			if idx >= 0 {
				events := m.store.GetByDate(date)
				for i, ev := range events {
					if ev.ID == m.adjustEventID {
						m.adjustIndex = i
						break
					}
				}
			}
			m.statusMsg = fmt.Sprintf("Moving %d event(s)", len(ids))
			return m, nil
		}
	}

	if IsKey(msg, KeyH) || IsKey(msg, KeyJ) || IsKey(msg, KeyK) || IsKey(msg, KeyL) || IsKey(msg, KeyCtrlD) || IsKey(msg, KeyCtrlU) || IsKey(msg, KeyShiftJ) || IsKey(msg, KeyShiftK) || IsKey(msg, KeyShiftH) || IsKey(msg, KeyShiftL) {
		next, cmd := m.updateNavigate(msg)
		nm := next.(Model)
		nm.mode = ModeVisual
		nm.pendingYank = false
		return nm, cmd
	}

	m.pendingYank = false
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
				err := m.store.AddSpanningEvent(Event{
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

	case msg.String() == "ctrl+w":
		m.inputBuffer = deletePreviousWord(m.inputBuffer)

	default:
		s := msg.String()
		if len([]rune(s)) == 1 || s == " " {
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
			err := m.store.AddSpanningEvent(Event{
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

	case msg.String() == "ctrl+w":
		m.descBuffer = deletePreviousWord(m.descBuffer)

	default:
		s := msg.String()
		if len([]rune(s)) == 1 || s == " " {
			m.descBuffer += s
		}
	}
	return m, nil
}

func deletePreviousWord(s string) string {
	r := []rune(s)
	i := len(r)
	for i > 0 && unicode.IsSpace(r[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(r[i-1]) {
		i--
	}
	return string(r[:i])
}

func (m *Model) createFastStep() int {
	step := m.jumpStep() * 4
	if step < 60 {
		step = 60
	}
	return step
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
		err := m.store.AddSpanningEvent(Event{
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

	case IsKey(msg, KeyR):
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

	case IsKey(msg, KeyShiftR):
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

	switch {
	case IsKey(msg, KeyJ):
		m.moveAdjustBy(m.MinutesPerRow())

	case IsKey(msg, KeyK):
		m.moveAdjustBy(-m.MinutesPerRow())

	case IsKey(msg, KeyCtrlD):
		mpr := m.MinutesPerRow()
		quarterPage := mpr * (m.viewportHeight() / 4)
		if quarterPage < mpr {
			quarterPage = mpr
		}
		m.moveAdjustBy(quarterPage)

	case IsKey(msg, KeyCtrlU):
		mpr := m.MinutesPerRow()
		quarterPage := mpr * (m.viewportHeight() / 4)
		if quarterPage < mpr {
			quarterPage = mpr
		}
		m.moveAdjustBy(-quarterPage)

	case IsKey(msg, KeyShiftJ):
		m.moveAdjustBy(m.MinutesPerRow())

	case IsKey(msg, KeyShiftK):
		m.moveAdjustBy(-m.MinutesPerRow())

	case IsKey(msg, KeyG):
		m.gotoReturnMode = ModeAdjust
		m.mode = ModeGoto
		m.gotoBuffer = ""
		return m, nil

	case IsKey(msg, KeyShiftG):
		m.gotoReturnMode = ModeAdjust
		m.mode = ModeGotoDay
		m.gotoBuffer = ""
		return m, nil

	case IsKey(msg, KeyH):
		// Move event to previous day
		if m.adjustRecurring || m.adjustRecurringSelection {
			m.moveAdjustBy(-MinutesPerDay)
			return m, nil
		}
		if len(m.adjustEventIDs) > 0 {
			newIDs := make([]string, 0, len(m.adjustEventIDs))
			for _, id := range m.adjustEventIDs {
				newDate, newID, err := m.store.ShiftEventByID(id, -MinutesPerDay)
				if err != nil {
					m.statusMsg = err.Error()
					return m, nil
				}
				newIDs = append(newIDs, newID)
				if id == m.adjustEventID {
					m.adjustEventID = newID
					date = newDate
				}
			}
			m.adjustEventIDs = newIDs
			m.saveEvents()
			if m.cursorCol > 0 {
				m.cursorCol--
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, -1)
			}
			newDate := m.SelectedDate()
			events := m.store.GetByDate(newDate)
			for i, ev := range events {
				if ev.ID == m.adjustEventID {
					m.adjustIndex = i
					break
				}
			}
			return m, nil
		}
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
		if m.adjustRecurring || m.adjustRecurringSelection {
			m.moveAdjustBy(MinutesPerDay)
			return m, nil
		}
		if len(m.adjustEventIDs) > 0 {
			newIDs := make([]string, 0, len(m.adjustEventIDs))
			for _, id := range m.adjustEventIDs {
				newDate, newID, err := m.store.ShiftEventByID(id, MinutesPerDay)
				if err != nil {
					m.statusMsg = err.Error()
					return m, nil
				}
				newIDs = append(newIDs, newID)
				if id == m.adjustEventID {
					m.adjustEventID = newID
					date = newDate
				}
			}
			m.adjustEventIDs = newIDs
			m.saveEvents()
			if m.cursorCol < m.dayCount-1 {
				m.cursorCol++
			} else {
				m.windowStart = m.windowStart.AddDate(0, 0, 1)
			}
			newDate := m.SelectedDate()
			events := m.store.GetByDate(newDate)
			for i, ev := range events {
				if ev.ID == m.adjustEventID {
					m.adjustIndex = i
					break
				}
			}
			return m, nil
		}
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

	case IsKey(msg, KeyEsc), IsKey(msg, KeyEnter):
		if m.adjustRecurringSelection && IsKey(msg, KeyEnter) {
			m.mode = ModeConfirmRecurMove
			m.confirmVisualRecurring = true
			m.statusMsg = ""
			return m, nil
		}
		if m.adjustRecurring && IsKey(msg, KeyEnter) {
			m.mode = ModeConfirmRecurMove
			m.statusMsg = ""
			return m, nil
		}
		m.mode = ModeNavigate
		m.adjustIndex = -1
		m.adjustEventID = ""
		m.adjustEventIDs = nil
		m.adjustRecurring = false
		m.adjustRecurringSelection = false
		m.adjustSelectedOccurrences = nil
		m.adjustBasePartIDs = nil
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

func (m Model) updateConfirmRecurMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	handledMoveOne := msg.String() == "o" || IsKey(msg, "o")
	handledMoveAll := msg.String() == "a" || IsKey(msg, "a")
	handledCancel := msg.String() == KeyEsc || msg.String() == KeyEnter || IsKey(msg, KeyEsc) || IsKey(msg, KeyEnter)
	clearState := func() {
		m.mode = ModeNavigate
		m.adjustIndex = -1
		m.adjustEventID = ""
		m.adjustEventIDs = nil
		m.adjustRecurring = false
		m.adjustRecurringSelection = false
		m.adjustBasePartIDs = nil
		m.adjustPreviewDelta = 0
		m.adjustPreviewBase = Event{}
		m.adjustPreviewDuration = 0
		m.adjustOccurrenceDate = time.Time{}
		m.adjustPreviewGroupID = ""
		m.adjustSelectedOccurrences = nil
		m.confirmVisualRecurring = false
	}
	if m.confirmVisualRecurring {
		switch {
		case handledMoveOne:
			count := len(m.adjustSelectedOccurrences)
			preview := m.recurringSelectionPreviewEvents()
			if len(preview) == 0 {
				m.statusMsg = "Nothing to move"
				return m, nil
			}
			m.pushUndo()
			previewByKey := make(map[string]Event)
			for _, seg := range preview {
				if _, ok := previewByKey[seg.ID]; !ok {
					previewByKey[seg.ID] = seg
				}
			}
			for i, occ := range m.adjustSelectedOccurrences {
				if err := m.store.AddException(occ.ID, occ.Date); err != nil {
					m.statusMsg = err.Error()
					return m, nil
				}
				start, duration, err := m.store.LogicalEventByID(occ.ID)
				if err != nil {
					m.statusMsg = err.Error()
					return m, nil
				}
				p, ok := previewByKey[fmt.Sprintf("__selection_preview__%d", i)]
				if !ok {
					continue
				}
				newEvent := start
				newEvent.Recurrence = ""
				newEvent.RecurUntilStr = ""
				newEvent.ExceptionDates = nil
				newEvent.GroupID = ""
				newEvent.Date = p.Date
				newEvent.StartMin = p.StartMin
				newEvent.EndMin = p.StartMin + duration
				newEvent.ID = GenerateID()
				if err := m.store.AddSpanningEvent(newEvent); err != nil {
					m.statusMsg = err.Error()
					return m, nil
				}
			}
			m.saveEvents()
			clearState()
			m.statusMsg = fmt.Sprintf("Moved %d occurrence(s)", count)
			return m, nil
		case handledMoveAll:
			if len(m.adjustSelectedOccurrences) == 0 {
				clearState()
				return m, nil
			}
			ev := m.adjustSelectedOccurrences[0]
			m.pushUndo()
			m.adjustRecurring = true
			m.adjustRecurringSelection = false
			m.adjustEventID = ev.ID
			m.adjustEventIDs = nil
			baseDate, baseIdx := m.store.FindEventByID(ev.ID)
			if baseIdx >= 0 {
				base := m.store.events[DateKey(baseDate)][baseIdx]
				grouped := m.store.groupedEvents(base)
				m.adjustPreviewBase = grouped[0]
				_, duration, _ := m.store.LogicalEventByID(grouped[0].ID)
				m.adjustPreviewDuration = duration
				m.adjustPreviewGroupID = "__adjust_preview__"
				m.adjustBasePartIDs = make([]string, 0, len(grouped))
				anchorBaseDate := DateKey(grouped[0].Date)
				selectedBaseDate := DateKey(baseDate)
				m.adjustOccurrenceDate = DateKey(ev.Date).AddDate(0, 0, -int(selectedBaseDate.Sub(anchorBaseDate).Hours()/24))
				for _, part := range grouped {
					m.adjustBasePartIDs = append(m.adjustBasePartIDs, part.ID)
				}
			}
			baseDate, baseIdx = m.store.FindEventByID(m.adjustPreviewBase.ID)
			if baseIdx < 0 {
				m.statusMsg = "event not found"
				return m, nil
			}
			baseEv := m.store.events[DateKey(baseDate)][baseIdx]
			grouped := m.store.groupedEvents(baseEv)
			startDate := DateKey(grouped[0].Date)
			startMin := grouped[0].StartMin + m.adjustPreviewDelta
			for startMin < 0 {
				startMin += MinutesPerDay
				startDate = startDate.AddDate(0, 0, -1)
			}
			for startMin >= MinutesPerDay {
				startMin -= MinutesPerDay
				startDate = startDate.AddDate(0, 0, 1)
			}
			newParts, err := buildSpanningSegments(grouped[0], startDate, startMin, m.adjustPreviewDuration, groupedGroupID(grouped))
			if err != nil {
				m.statusMsg = err.Error()
				return m, nil
			}
			m.store.deleteGroupedEvents(groupKey(grouped[0]))
			for _, part := range newParts {
				m.store.Add(part)
			}
			m.saveEvents()
			clearState()
			m.statusMsg = "Moved all occurrences"
			return m, nil
		case handledCancel:
			m.mode = ModeAdjust
			m.confirmVisualRecurring = false
			return m, nil
		}
	}

	clear := func() {
		m.mode = ModeNavigate
		m.adjustIndex = -1
		m.adjustEventID = ""
		m.adjustEventIDs = nil
		m.adjustRecurring = false
		m.adjustRecurringSelection = false
		m.adjustBasePartIDs = nil
		m.adjustPreviewDelta = 0
		m.adjustPreviewBase = Event{}
		m.adjustPreviewDuration = 0
		m.adjustOccurrenceDate = time.Time{}
		m.adjustPreviewGroupID = ""
		m.adjustSelectedOccurrences = nil
		m.confirmVisualRecurring = false
	}

	switch {
	case handledMoveOne:
		m.pushUndo()
		if err := m.store.AddException(m.adjustPreviewBase.ID, m.adjustOccurrenceDate); err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		preview := m.recurringAdjustPreviewSegments()
		if len(preview) == 0 {
			m.statusMsg = "Nothing to move"
			return m, nil
		}
		newEvent := m.adjustPreviewBase
		newEvent.Recurrence = ""
		newEvent.RecurUntilStr = ""
		newEvent.ExceptionDates = nil
		newEvent.GroupID = ""
		newEvent.Date = preview[0].Date
		newEvent.StartMin = preview[0].StartMin
		newEvent.EndMin = preview[0].StartMin + m.adjustPreviewDuration
		newEvent.ID = GenerateID()
		if err := m.store.AddSpanningEvent(newEvent); err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		m.saveEvents()
		clear()
		m.statusMsg = "Moved one occurrence"
		return m, nil

	case handledMoveAll:
		m.pushUndo()
		baseDate, baseIdx := m.store.FindEventByID(m.adjustPreviewBase.ID)
		if baseIdx < 0 {
			m.statusMsg = "event not found"
			return m, nil
		}
		baseEv := m.store.events[DateKey(baseDate)][baseIdx]
		grouped := m.store.groupedEvents(baseEv)
		startDate := DateKey(grouped[0].Date)
		startMin := grouped[0].StartMin + m.adjustPreviewDelta
		for startMin < 0 {
			startMin += MinutesPerDay
			startDate = startDate.AddDate(0, 0, -1)
		}
		for startMin >= MinutesPerDay {
			startMin -= MinutesPerDay
			startDate = startDate.AddDate(0, 0, 1)
		}
		segments, err := buildSpanningSegments(grouped[0], startDate, startMin, m.adjustPreviewDuration, groupedGroupID(grouped))
		if err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		m.store.deleteGroupedEvents(groupKey(grouped[0]))
		for _, seg := range segments {
			if err := m.store.Add(seg); err != nil {
				m.statusMsg = err.Error()
				return m, nil
			}
		}
		m.saveEvents()
		clear()
		m.statusMsg = "Moved all occurrences"
		return m, nil

	case handledCancel:
		clear()
		return m, nil
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
		return m.renderScreenWithStatus(RenderDetail(&m))
	}

	// Settings view is fullscreen
	if m.mode == ModeSettings {
		return m.renderScreenWithStatus(RenderSettings(&m))
	}

	// Edit menu is fullscreen
	if m.mode == ModeEditMenu {
		return m.renderScreenWithStatus(RenderEditMenu(&m))
	}

	if m.mode == ModeHelp {
		return m.renderScreenWithStatus(m.renderHelpView())
	}

	// Month view
	if m.viewMode == ViewMonth {
		return m.renderScreenWithStatus(RenderMonth(&m))
	}

	// Year view
	if m.viewMode == ViewYear {
		return m.renderScreenWithStatus(RenderYear(&m))
	}

	grid := RenderGrid(&m)
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("prompt_fg", m.uiColor("accent", "39")))).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("hint_fg", "243")))

	if m.mode == ModeInput {
		label := "Event title: "
		prompt := "\n" + promptStyle.Render(label) + m.inputBuffer + "█"
		return m.renderScreenWithStatus(grid + prompt)
	}

	if m.mode == ModeInputDesc {
		label := "Description (Enter to skip): "
		prompt := "\n" + promptStyle.Render(label) + m.descBuffer + "█"
		return m.renderScreenWithStatus(grid + prompt)
	}

	if m.mode == ModeInputRecurrence {
		recLabel := RecurrenceLabel(m.createRecurrence)
		prompt := "\n" + promptStyle.Render("Repeat: ") + recLabel + "  " +
			hintStyle.Render("r: cycle  Enter: confirm  Esc: cancel")
		return m.renderScreenWithStatus(grid + prompt)
	}

	if m.mode == ModeSearch {
		prompt := "\n" + promptStyle.Render("/") + m.searchQuery + "█"
		matchInfo := ""
		if len(m.searchMatches) > 0 {
			matchInfo = fmt.Sprintf(" (%d matches)", len(m.searchMatches))
		}
		return m.renderScreenWithStatus(grid + prompt + hintStyle.Render(matchInfo))
	}

	if m.mode == ModeGoto {
		prompt := "\n" + promptStyle.Render("Go to time: ") + m.gotoBuffer + "█"
		return m.renderScreenWithStatus(grid + prompt)
	}

	if m.mode == ModeGotoDay {
		prompt := "\n" + promptStyle.Render("Go to day: ") + m.gotoBuffer + "█"
		return m.renderScreenWithStatus(grid + prompt)
	}

	return m.renderScreenWithStatus(grid)
}

func (m Model) renderScreenWithStatus(content string) string {
	body := lipgloss.Place(m.width, m.height-1, lipgloss.Left, lipgloss.Top, content)
	return body + "\n" + m.renderStatusBar()
}

type helpRow struct {
	section  string
	keyID    string
	keys     string
	action   string
	note     string
	editable bool
}

func (m Model) uiColor(name, fallback string) string {
	if m.settings.UIColors != nil {
		if value, ok := m.settings.UIColors[name]; ok && value != "" {
			return value
		}
	}
	return fallback
}

func helpRows() []helpRow {
	return []helpRow{
		{"Week View", KeyH, DisplayKey(KeyH), "Move left", "week and visual navigation", true},
		{"Week View", KeyJ, DisplayKey(KeyJ), "Move down", "week and visual navigation", true},
		{"Week View", KeyK, DisplayKey(KeyK), "Move up", "week and visual navigation", true},
		{"Week View", KeyL, DisplayKey(KeyL), "Move right", "week and visual navigation", true},
		{"Week View", KeyCtrlD, DisplayKey(KeyCtrlD), "Half page down", "week and visual navigation", true},
		{"Week View", KeyCtrlU, DisplayKey(KeyCtrlU), "Half page up", "week and visual navigation", true},
		{"Week View", KeyShiftJ, DisplayKey(KeyShiftJ), "Step minute down", "also expands create/visual", true},
		{"Week View", KeyShiftK, DisplayKey(KeyShiftK), "Step minute up", "also expands create/visual", true},
		{"Week View", KeyShiftH, DisplayKey(KeyShiftH), "Prev overlap", "previous overlapping event", true},
		{"Week View", KeyShiftL, DisplayKey(KeyShiftL), "Next overlap", "next overlapping event", true},
		{"Week View", KeyTab, DisplayKey(KeyTab), "Cycle overlap", "select next overlapping event", true},
		{"Week View", KeyC, DisplayKey(KeyC), "Jump to now", "today at current time", true},
		{"Week View", KeyPlus, DisplayKey(KeyPlus), "Zoom in", "denser timeline", true},
		{"Week View", KeyMinus, DisplayKey(KeyMinus), "Zoom out", "wider timeline", true},
		{"Week View", Key0, DisplayKey(Key0), "Reset zoom", "default zoom", true},
		{"Week View", KeyA, DisplayKey(KeyA), "Create event", "start a new event", true},
		{"Week View", KeyE, DisplayKey(KeyE), "Edit in editor", "open external editor", true},
		{"Week View", KeyM, DisplayKey(KeyM), "Move event", "move selected event", true},
		{"Week View", KeyY, DisplayKey(KeyY), "Copy", "copy selected event", true},
		{"Week View", KeyX, DisplayKey(KeyX), "Cut", "cut selected event", true},
		{"Week View", KeyP, DisplayKey(KeyP), "Paste", "paste clipboard event", true},
		{"Week View", KeyShiftV, DisplayKey(KeyShiftV), "Visual select", "select a time range", true},
		{"Week View", KeyU, DisplayKey(KeyU), "Undo", "undo last change", true},
		{"Week View", KeyCtrlR, DisplayKey(KeyCtrlR), "Redo", "redo last undone change", true},
		{"Week View", KeySlash, DisplayKey(KeySlash), "Search", "search titles and descriptions", true},
		{"Week View", KeyN, DisplayKey(KeyN), "Next match", "search result navigation", true},
		{"Week View", KeyShiftN, DisplayKey(KeyShiftN), "Prev match", "search result navigation", true},
		{"Week View", KeyCtrlN, DisplayKey(KeyCtrlN), "Next match alt", "search result navigation", true},
		{"Week View", KeyCtrlP, DisplayKey(KeyCtrlP), "Prev match alt", "search result navigation", true},
		{"Week View", KeyG, DisplayKey(KeyG), "Goto time", "jump to a typed time", true},
		{"Week View", KeyShiftG, DisplayKey(KeyShiftG), "Goto day", "jump to day of month", true},
		{"Week View", KeyShiftM, DisplayKey(KeyShiftM), "Month view", "open month view", true},
		{"Week View", KeyShiftY, DisplayKey(KeyShiftY), "Year view", "open year view", true},
		{"Week View", KeyShiftS, DisplayKey(KeyShiftS), "Settings", "open settings", true},
		{"Week View", KeyQuestion, DisplayKey(KeyQuestion), "Help", "open or close help", true},
		{"Visual Mode", KeyD, DisplayKey(KeyD), "Delete selection", "remove selected events", true},
		{"Visual Mode", KeyEsc, DisplayKey(KeyEsc), "Cancel / back", "leave prompts and views", true},
		{"Input Modes", KeyEnter, DisplayKey(KeyEnter), "Confirm input", "accept current prompt", true},
		{"Input Modes", KeySpace, DisplayKey(KeySpace), "Menu confirm", "toggle settings/menu items", true},
		{"Input Modes", KeyR, DisplayKey(KeyR), "Cycle recurrence", "forward", true},
		{"Input Modes", KeyShiftR, DisplayKey(KeyShiftR), "Cycle recurrence back", "backward", true},
		{"Move Mode", KeyS, DisplayKey(KeyS), "Edit menu", "open edit menu while moving", true},
		{"Custom Keybindings", "", "Enter", "Rebind selected row", "press a new key, Esc cancels", false},
		{"Custom Keybindings", "", "Backspace", "Reset selected row", "restore that action to default", false},
		{"Custom Keybindings", "", "~/.local/share/vimalender/settings.json", "Settings file", "autogenerated with every keybinding", false},
		{"Custom Keybindings", "", "settings.ui_colors", "Theme colors", "change accent and UI colors", false},
	}
}

func helpRowByKey(keyID string) (helpRow, bool) {
	for _, row := range helpRows() {
		if row.keyID == keyID {
			return row, true
		}
	}
	return helpRow{}, false
}

func canonicalKeyName(key string) string {
	if key == "backspace" {
		return key
	}
	if key == " " {
		return "space"
	}
	return key
}

func (m Model) selectedHelpRow() (helpRow, bool) {
	rows := helpRows()
	if m.helpCursor < 0 || m.helpCursor >= len(rows) {
		return helpRow{}, false
	}
	return rows[m.helpCursor], true
}

func (m *Model) resetHelpBinding(keyID string) {
	defaults := DefaultKeybindings()
	if m.settings.Keybindings == nil {
		m.settings.Keybindings = defaults
	}
	if def, ok := defaults[keyID]; ok {
		m.settings.Keybindings[keyID] = def
	}
	SetKeyBindingOverrides(m.settings.Keybindings)
	m.saveSettings()
	if row, ok := helpRowByKey(keyID); ok {
		m.statusMsg = fmt.Sprintf("Reset %s to %s", row.action, DisplayKey(keyID))
	}
	if m.helpRebindKey == keyID {
		m.helpRebinding = false
		m.helpRebindKey = ""
	}
}

func (m *Model) applyHelpBinding(keyID, newKey string) {
	newKey = canonicalKeyName(newKey)
	if m.settings.Keybindings == nil {
		m.settings.Keybindings = DefaultKeybindings()
	}
	for existingID, bound := range m.settings.Keybindings {
		if existingID != keyID && canonicalKeyName(bound) == newKey {
			if row, ok := helpRowByKey(existingID); ok {
				m.statusMsg = fmt.Sprintf("%s already used by %s", newKey, row.action)
			} else {
				m.statusMsg = fmt.Sprintf("%s already used", newKey)
			}
			return
		}
	}
	m.settings.Keybindings[keyID] = newKey
	SetKeyBindingOverrides(m.settings.Keybindings)
	m.saveSettings()
	if row, ok := helpRowByKey(keyID); ok {
		m.statusMsg = fmt.Sprintf("Bound %s to %s", row.action, newKey)
	}
	m.helpRebinding = false
	m.helpRebindKey = ""
}

func (m Model) helpWindow(rows []helpRow, start int) (int, int) {
	maxContentLines := m.height - 8
	if maxContentLines < 8 {
		maxContentLines = 8
	}
	if start < 0 {
		start = 0
	}
	if start >= len(rows) {
		start = len(rows) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start
	lineCount := 3
	lastSectionForFit := ""
	for end < len(rows) {
		extra := 1
		if rows[end].section != lastSectionForFit {
			if lastSectionForFit != "" {
				extra++
			}
			extra++
		}
		if lineCount+extra > maxContentLines {
			break
		}
		lineCount += extra
		lastSectionForFit = rows[end].section
		end++
	}
	if end <= start {
		end = start + 1
		if end > len(rows) {
			end = len(rows)
		}
	}
	return start, end
}

func (m Model) renderHelpView() string {
	rows := helpRows()

	if m.helpCursor < 0 {
		m.helpCursor = 0
	}
	if m.helpCursor >= len(rows) {
		m.helpCursor = len(rows) - 1
	}

	outerWidth := m.width - 8
	if outerWidth > 104 {
		outerWidth = 104
	}
	if outerWidth < 40 {
		outerWidth = 40
	}
	contentWidth := outerWidth - 6
	if contentWidth < 34 {
		contentWidth = 34
	}
	keyWidth := 28
	if contentWidth < 72 {
		keyWidth = 20
	}
	actionWidth := 22

	if m.helpScroll > m.helpCursor {
		m.helpScroll = m.helpCursor
	}
	if m.helpScroll < 0 {
		m.helpScroll = 0
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.uiColor("accent", "39")))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.uiColor("help_section", m.uiColor("accent", "39")))).Padding(0, 1)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	actionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("hint_fg", "245")))
	rowStyle := lipgloss.NewStyle().Padding(0, 1)
	selectedRowStyle := lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color(m.uiColor("help_selected_bg", "236")))
	rebindStyle := lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color(m.uiColor("accent", "39"))).Foreground(lipgloss.Color("255"))
	helpBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.uiColor("help_border", m.uiColor("accent", "39")))).
		Padding(1, 2)
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("hint_fg", "245")))

	start, end := m.helpWindow(rows, m.helpScroll)

	var body []string
	body = append(body, titleStyle.Render("Help"))
	if m.helpRebinding {
		row, _ := helpRowByKey(m.helpRebindKey)
		body = append(body, footerStyle.Render(fmt.Sprintf("Rebinding %s - press new key  Backspace: reset  Esc: cancel", row.action)))
	} else {
		body = append(body, footerStyle.Render("j/k: move  Enter: rebind  Backspace: reset  ctrl+d/u: faster scroll  ?: close"))
	}
	body = append(body, "")
	lastSection := ""
	for i := start; i < end; i++ {
		row := rows[i]
		if row.section != lastSection {
			if lastSection != "" {
				body = append(body, "")
			}
			body = append(body, sectionStyle.Render(" "+row.section+" "))
			lastSection = row.section
		}
		keys := padRight(truncLabel(row.keys, keyWidth), keyWidth)
		action := padRight(truncLabel(row.action, actionWidth), actionWidth)
		noteWidth := contentWidth - keyWidth - actionWidth - 6
		if noteWidth < 10 {
			noteWidth = 10
		}
		note := truncLabel(row.note, noteWidth)
		prefix := "  "
		if row.editable {
			prefix = "* "
		}
		line := prefix + keyStyle.Render(keys) + "  " + actionStyle.Render(action) + "  " + noteStyle.Render(note)
		if row.editable && row.keyID == m.helpRebindKey && m.helpRebinding {
			body = append(body, rebindStyle.Render(line))
		} else if i == m.helpCursor {
			body = append(body, selectedRowStyle.Render(line))
		} else {
			body = append(body, rowStyle.Render(line))
		}
	}

	if end < len(rows) {
		body = append(body, "")
		body = append(body, footerStyle.Render(fmt.Sprintf("More rows below (%d/%d)", end, len(rows))))
	}

	content := lipgloss.NewStyle().Width(contentWidth).Render(strings.Join(body, "\n"))
	box := helpBoxStyle.Render(content)
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(box)
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := helpRows()
	rowCount := len(rows)
	page := m.height - 12
	if page < 5 {
		page = 5
	}
	if m.helpRebinding {
		s := canonicalKeyName(msg.String())
		switch s {
		case KeyEsc:
			m.helpRebinding = false
			m.helpRebindKey = ""
			m.statusMsg = "Rebind cancelled"
			return m, nil
		case "backspace":
			m.resetHelpBinding(m.helpRebindKey)
			return m, nil
		case "":
			return m, nil
		default:
			m.applyHelpBinding(m.helpRebindKey, s)
			return m, nil
		}
	}
	switch {
	case IsKey(msg, KeyEsc) || IsKey(msg, KeyQ) || IsKey(msg, KeyQuestion):
		m.mode = ModeNavigate
		return m, nil
	case msg.String() == "backspace":
		if row, ok := m.selectedHelpRow(); ok && row.editable {
			m.resetHelpBinding(row.keyID)
			return m, nil
		}
	case IsKey(msg, KeyEnter):
		if row, ok := m.selectedHelpRow(); ok && row.editable {
			m.helpRebinding = true
			m.helpRebindKey = row.keyID
			m.statusMsg = ""
			return m, nil
		}
	case IsKey(msg, KeyJ):
		if m.helpCursor < rowCount-1 {
			m.helpCursor++
		}
	case IsKey(msg, KeyK):
		if m.helpCursor > 0 {
			m.helpCursor--
		}
	case IsKey(msg, KeyCtrlD):
		m.helpCursor += page / 2
		if m.helpCursor >= rowCount {
			m.helpCursor = rowCount - 1
		}
	case IsKey(msg, KeyCtrlU):
		m.helpCursor -= page / 2
		if m.helpCursor < 0 {
			m.helpCursor = 0
		}
	}
	for {
		start, end := m.helpWindow(rows, m.helpScroll)
		if m.helpCursor < start {
			m.helpScroll = m.helpCursor
			continue
		}
		if m.helpCursor >= end {
			m.helpScroll++
			continue
		}
		break
	}
	if m.helpScroll < 0 {
		m.helpScroll = 0
	}
	return m, nil
}

// renderStatusBar renders the bottom status bar.
func (m Model) renderStatusBar() string {
	date := m.SelectedDate().Format("Mon Jan 02 2006")
	cursorTime := MinToTime(m.cursorMin)
	accent := m.uiColor("accent", "39")
	statusBG := m.uiColor("status_bar_bg", "236")
	statusFG := m.uiColor("status_bar_fg", "255")
	hintFG := m.uiColor("hint_fg", "243")
	warningFG := m.uiColor("warning_fg", "111")
	modeStyle := lipgloss.NewStyle().Bold(true).Background(lipgloss.Color(accent)).Foreground(lipgloss.Color("255")).Padding(0, 1)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(hintFG))
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(warningFG))
	statusBarStyle := lipgloss.NewStyle().Background(lipgloss.Color(statusBG)).Foreground(lipgloss.Color(statusFG)).Padding(0, 1)

	var mode, hints string

	switch m.mode {
	case ModeNavigate:
		mode = modeStyle.Render(" WEEK ")
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
			info += fmt.Sprintf("  %s: settings  %s: help", DisplayKey(KeyShiftS), DisplayKey(KeyQuestion))
		}
		hints = hintStyle.Render(info)
	case ModeCreate:
		mode = modeStyle.Render(" CREATE ")
		hints = hintStyle.Render(fmt.Sprintf(" %s  %s", date, formatCreateTimeRange(m.createStart, m.createEnd)))
	case ModeInput:
		mode = modeStyle.Render(" CREATE ")
		hints = hintStyle.Render(" Type title")
	case ModeInputDesc:
		mode = modeStyle.Render(" CREATE ")
		hints = hintStyle.Render(fmt.Sprintf(" %q", m.inputBuffer))
	case ModeInputRecurrence:
		mode = modeStyle.Render(" CREATE ")
		hints = hintStyle.Render(fmt.Sprintf(" %q  Repeat: %s", m.inputBuffer, RecurrenceLabel(m.createRecurrence)))
	case ModeAdjust:
		mode = modeStyle.Render(" MOVE ")
		hints = hintStyle.Render(fmt.Sprintf(" %s  %s", date, cursorTime))
	case ModeDetail:
		mode = StatusDetailModeStyle.Render(" DETAIL ")
		hints = hintStyle.Render(" e: edit  Esc/q: back")
	case ModeGoto:
		mode = modeStyle.Render(" GOTO TIME ")
		hints = hintStyle.Render(" Type time (12, 1200, 12:00), Enter: go, Esc: cancel")
	case ModeGotoDay:
		mode = modeStyle.Render(" GOTO DAY ")
		hints = hintStyle.Render(" Type day of month (1-31), Enter: go, Esc: cancel")
	case ModeSearch:
		mode = modeStyle.Render(" SEARCH ")
		hints = hintStyle.Render(fmt.Sprintf(" %d matches", len(m.searchMatches)))
	case ModeVisual:
		mode = modeStyle.Render(" VISUAL ")
		count := len(m.visualSelectedEventIDs())
		hints = hintStyle.Render(fmt.Sprintf(" %d selected", count))
	case ModeMonth:
		mode = StatusMonthModeStyle.Render(" MONTH ")
		monthDate := m.monthCursor.Format("January 2006")
		selectedDay := m.monthCursor.Format("Mon Jan 02")
		eventCount := m.store.EventCount(m.monthCursor)
		evInfo := ""
		if eventCount > 0 {
			evInfo = fmt.Sprintf("  %d events", eventCount)
		}
		hints = hintStyle.Render(
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
		hints = hintStyle.Render(
			fmt.Sprintf(" %s  %s%s  h/l: day  j/k: week  H/L: month  J/K: year  Enter: open  c: today  M: month  Y/Esc: back  q: quit",
				yearLabel, selectedDay, evInfo))
	case ModeSettings:
		mode = modeStyle.Render(" SETTINGS ")
		hints = hintStyle.Render(" settings")
	case ModeEditMenu:
		mode = modeStyle.Render(" EDIT ")
		if m.editMenuActive {
			hints = hintStyle.Render(" editing")
		} else {
			hints = hintStyle.Render(" fields")
		}
	case ModeHelp:
		mode = modeStyle.Render(" HELP ")
		hints = hintStyle.Render(" keybindings")
	case ModeConfirmRecurDelete:
		mode = warningStyle.Render(" DELETE ")
		hints = hintStyle.Render(" (o): delete this occurrence  (a): delete all  Esc: cancel")
	case ModeConfirmRecurMove:
		mode = modeStyle.Render(" MOVE ")
		if m.confirmVisualRecurring {
			hints = hintStyle.Render(" (o): move selected occurrences  (a): move whole series  Esc: cancel")
		} else {
			hints = hintStyle.Render(" (o): move this occurrence  (a): move all  Esc: cancel")
		}
	}

	bar := mode + hints
	if m.statusMsg != "" {
		return statusBarStyle.Render(bar) + "  " + warningStyle.Render(m.statusMsg)
	}
	return statusBarStyle.Width(m.width).Render(bar)
}
