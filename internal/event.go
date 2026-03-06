package internal

import (
	"crypto/rand"
	"fmt"
	"sort"
	"time"
)

// MinutesPerDay is the number of minutes in a day.
const MinutesPerDay = 1440

// Recurrence patterns.
const (
	RecurNone     = ""
	RecurDaily    = "daily"
	RecurWeekdays = "weekdays"
	RecurWeekly   = "weekly"
	RecurBiweekly = "biweekly"
	RecurMonthly  = "monthly"
	RecurYearly   = "yearly"
)

// RecurrenceOptions are the display labels in order for cycling.
var RecurrenceOptions = []string{RecurNone, RecurDaily, RecurWeekdays, RecurWeekly, RecurBiweekly, RecurMonthly, RecurYearly}

// RecurrenceLabel returns a human-readable label for a recurrence pattern.
func RecurrenceLabel(r string) string {
	switch r {
	case RecurDaily:
		return "Daily"
	case RecurWeekdays:
		return "Weekdays"
	case RecurWeekly:
		return "Weekly"
	case RecurBiweekly:
		return "Biweekly"
	case RecurMonthly:
		return "Monthly"
	case RecurYearly:
		return "Yearly"
	default:
		return "None"
	}
}

// Event represents a calendar event with minute-level precision.
type Event struct {
	Title          string    `json:"title"`
	Desc           string    `json:"desc,omitempty"`
	Date           time.Time `json:"-"` // serialized separately as "YYYY-MM-DD"
	DateStr        string    `json:"date"`
	StartMin       int       `json:"start_min"`
	EndMin         int       `json:"end_min"`
	Notes          string    `json:"notes"`
	ID             string    `json:"id,omitempty"`
	Recurrence     string    `json:"recurrence,omitempty"`
	RecurUntilStr  string    `json:"recur_until,omitempty"`
	ExceptionDates []string  `json:"exception_dates,omitempty"`
}

// IsRecurring returns true if this event has a recurrence pattern.
func (e Event) IsRecurring() bool {
	return e.Recurrence != "" && e.Recurrence != RecurNone
}

// GenerateID creates a new UUID v4 string.
func GenerateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// StartTime returns "HH:MM" for the start.
func (e Event) StartTime() string {
	return MinToTime(e.StartMin)
}

// EndTime returns "HH:MM" for the end.
func (e Event) EndTime() string {
	return MinToTime(e.EndMin)
}

// MinToTime converts a minute offset to "HH:MM".
func MinToTime(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// DateKey returns a comparable date key with time zeroed.
func DateKey(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EventStore manages in-memory calendar events keyed by date.
type EventStore struct {
	events map[time.Time][]Event
}

// Snapshot returns a deep copy of the event store's data for undo.
func (s *EventStore) Snapshot() map[time.Time][]Event {
	snap := make(map[time.Time][]Event, len(s.events))
	for k, v := range s.events {
		cp := make([]Event, len(v))
		for i, ev := range v {
			cp[i] = ev
			// Deep copy slices
			if ev.ExceptionDates != nil {
				cp[i].ExceptionDates = make([]string, len(ev.ExceptionDates))
				copy(cp[i].ExceptionDates, ev.ExceptionDates)
			}
		}
		snap[k] = cp
	}
	return snap
}

// Restore replaces the event store's data with a previous snapshot.
func (s *EventStore) Restore(snap map[time.Time][]Event) {
	s.events = snap
}

// NewEventStore creates a new empty event store.
func NewEventStore() *EventStore {
	return &EventStore{
		events: make(map[time.Time][]Event),
	}
}

// GetByDate returns all events for the given date, including virtual
// occurrences of recurring events stored on other dates.
// Results are sorted deterministically (stored events first, then virtual
// occurrences sorted by ID) to ensure stable indexing.
func (s *EventStore) GetByDate(date time.Time) []Event {
	key := DateKey(date)
	keyStr := key.Format("2006-01-02")

	// Start with events explicitly stored on this date, skipping excepted ones
	var result []Event
	for _, ev := range s.events[key] {
		if ev.IsRecurring() && isExcepted(ev, keyStr) {
			continue
		}
		result = append(result, ev)
	}

	// Add virtual occurrences from recurring events on other dates
	for baseDate, events := range s.events {
		if baseDate.Equal(key) {
			continue // already handled above
		}
		for _, ev := range events {
			if !ev.IsRecurring() {
				continue
			}
			if matchesDate(ev, key) {
				virt := ev
				virt.Date = key
				virt.DateStr = keyStr
				result = append(result, virt)
			}
		}
	}

	// Sort all results deterministically by ID so indices are stable
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// isExcepted checks whether a date string is in the event's exception list.
func isExcepted(ev Event, dateStr string) bool {
	for _, exc := range ev.ExceptionDates {
		if exc == dateStr {
			return true
		}
	}
	return false
}

// IsVirtualIndex returns true if the given index (from GetByDate) refers to a
// virtual occurrence rather than a stored event on this date.
func (s *EventStore) IsVirtualIndex(date time.Time, index int) bool {
	events := s.GetByDate(date)
	if index < 0 || index >= len(events) {
		return true
	}
	ev := events[index]
	// Check if this event's ID exists among events stored on this date
	for _, stored := range s.events[DateKey(date)] {
		if stored.ID == ev.ID {
			return false
		}
	}
	return true
}

// GetStoredByDate returns only events explicitly stored on this date (no virtual occurrences).
func (s *EventStore) GetStoredByDate(date time.Time) []Event {
	return s.events[DateKey(date)]
}

// matchesDate checks if a recurring event should appear on queryDate.
// It respects the recurrence pattern, RecurUntil, and ExceptionDates.
func matchesDate(ev Event, queryDate time.Time) bool {
	baseDate := DateKey(ev.Date)
	query := DateKey(queryDate)

	// Must be after base date
	if query.Before(baseDate) || query.Equal(baseDate) {
		return false
	}

	// Check RecurUntil
	if ev.RecurUntilStr != "" {
		until, err := time.Parse("2006-01-02", ev.RecurUntilStr)
		if err == nil && query.After(until) {
			return false
		}
	}

	// Check exception dates
	qStr := query.Format("2006-01-02")
	for _, exc := range ev.ExceptionDates {
		if exc == qStr {
			return false
		}
	}

	// Check recurrence pattern
	switch ev.Recurrence {
	case RecurDaily:
		return true
	case RecurWeekdays:
		wd := query.Weekday()
		return wd >= time.Monday && wd <= time.Friday
	case RecurWeekly:
		return query.Weekday() == baseDate.Weekday()
	case RecurBiweekly:
		if query.Weekday() != baseDate.Weekday() {
			return false
		}
		days := int(query.Sub(baseDate).Hours() / 24)
		return (days/7)%2 == 0
	case RecurMonthly:
		return clampDay(query.Year(), query.Month(), baseDate.Day()) == query.Day()
	case RecurYearly:
		return query.Month() == baseDate.Month() &&
			clampDay(query.Year(), query.Month(), baseDate.Day()) == query.Day()
	}
	return false
}

// clampDay returns day clamped to the number of days in the given month.
func clampDay(year int, month time.Month, day int) int {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		return last
	}
	return day
}

// FindEventByID finds a base event by its ID, returning (date, index).
// Returns zero time and -1 if not found.
func (s *EventStore) FindEventByID(id string) (time.Time, int) {
	for date, events := range s.events {
		for i, ev := range events {
			if ev.ID == id {
				return date, i
			}
		}
	}
	return time.Time{}, -1
}

// AddException adds an exception date to a recurring event, identified by ID.
func (s *EventStore) AddException(id string, exceptionDate time.Time) error {
	date, idx := s.FindEventByID(id)
	if idx < 0 {
		return fmt.Errorf("event not found")
	}
	excStr := DateKey(exceptionDate).Format("2006-01-02")
	s.events[date][idx].ExceptionDates = append(s.events[date][idx].ExceptionDates, excStr)
	return nil
}

// DeleteByID deletes a base event entirely by its ID.
func (s *EventStore) DeleteByID(id string) error {
	date, idx := s.FindEventByID(id)
	if idx < 0 {
		return fmt.Errorf("event not found")
	}
	s.Delete(date, idx)
	return nil
}

// Add inserts a new event. Overlaps are allowed.
func (s *EventStore) Add(e Event) error {
	if e.EndMin <= e.StartMin {
		return fmt.Errorf("end must be greater than start")
	}
	if e.StartMin < 0 || e.EndMin > MinutesPerDay {
		return fmt.Errorf("minutes must be in range 0-%d", MinutesPerDay)
	}
	if e.ID == "" {
		e.ID = GenerateID()
	}

	key := DateKey(e.Date)
	e.Date = key
	s.events[key] = append(s.events[key], e)
	// Don't clear layout cache — existing events keep their columns,
	// new event will be placed in the next available slot.
	return nil
}

// Delete removes the event at the given index for the given date.
func (s *EventStore) Delete(date time.Time, index int) {
	key := DateKey(date)
	events := s.events[key]
	if index < 0 || index >= len(events) {
		return
	}
	s.events[key] = append(events[:index], events[index+1:]...)
	if len(s.events[key]) == 0 {
		delete(s.events, key)
	}
}

// MoveEvent shifts the event at index by delta minutes.
func (s *EventStore) MoveEvent(date time.Time, index, delta int) error {
	key := DateKey(date)
	events := s.events[key]
	if index < 0 || index >= len(events) {
		return fmt.Errorf("invalid event index")
	}

	ev := events[index]
	newStart := ev.StartMin + delta
	newEnd := ev.EndMin + delta

	if newStart < 0 || newEnd > MinutesPerDay {
		return fmt.Errorf("out of bounds")
	}

	s.events[key][index].StartMin = newStart
	s.events[key][index].EndMin = newEnd
	// Don't clear layout cache — event keeps its column during time adjustment.
	return nil
}

// MoveEventByID shifts a recurring/stored event by delta minutes, found by ID.
func (s *EventStore) MoveEventByID(id string, delta int) error {
	date, idx := s.FindEventByID(id)
	if idx < 0 {
		return fmt.Errorf("event not found")
	}
	return s.MoveEvent(date, idx, delta)
}

// MoveEventToDate moves an event from one date to another, returning the new index.
func (s *EventStore) MoveEventToDate(fromDate time.Time, index int, toDate time.Time) (int, error) {
	fromKey := DateKey(fromDate)
	events := s.events[fromKey]
	if index < 0 || index >= len(events) {
		return -1, fmt.Errorf("invalid event index")
	}

	ev := events[index]
	// Remove from old date
	s.events[fromKey] = append(events[:index], events[index+1:]...)
	if len(s.events[fromKey]) == 0 {
		delete(s.events, fromKey)
	}
	// Add to new date
	toKey := DateKey(toDate)
	ev.Date = toKey
	s.events[toKey] = append(s.events[toKey], ev)
	newIndex := len(s.events[toKey]) - 1
	return newIndex, nil
}

// MoveEventToDateByID moves an event to a different date, found by ID.
// Returns the new storage index on the target date.
func (s *EventStore) MoveEventToDateByID(id string, toDate time.Time) (int, error) {
	fromDate, idx := s.FindEventByID(id)
	if idx < 0 {
		return -1, fmt.Errorf("event not found")
	}
	return s.MoveEventToDate(fromDate, idx, toDate)
}

// EventAtMinute returns the index of the first event at the given minute, or -1.
// Uses GetByDate which includes virtual occurrences.
func (s *EventStore) EventAtMinute(date time.Time, minute int) int {
	events := s.GetByDate(date)
	for i, ev := range events {
		if minute >= ev.StartMin && minute < ev.EndMin {
			return i
		}
	}
	return -1
}

// EventsAtMinute returns all event indices overlapping the given minute.
// Uses GetByDate which includes virtual occurrences.
func (s *EventStore) EventsAtMinute(date time.Time, minute int) []int {
	events := s.GetByDate(date)
	var indices []int
	for i, ev := range events {
		if minute >= ev.StartMin && minute < ev.EndMin {
			indices = append(indices, i)
		}
	}
	return indices
}

// OverlapColumns computes how many columns are needed for overlapping events
// in a given minute range, and assigns each event a column index.
// Returns a map from event index to (column, totalColumns).
type EventLayout struct {
	Col      int
	TotalCol int
}

// LayoutEvents computes side-by-side layout for overlapping events on a date.
// All events in a connected overlap group share the same TotalCol so that
// sub-column widths are consistent across all rows of the group.
// pinnedID and pinnedCol, if pinnedID is non-empty, pin that event to the given
// column so it doesn't swap positions during adjust mode.
func (s *EventStore) LayoutEvents(date time.Time, pinnedID string, pinnedCol int) map[int]EventLayout {
	events := s.GetByDate(date)
	if len(events) == 0 {
		return nil
	}

	// Sort indices by start time, then by end time, then by ID for stability
	indices := make([]int, len(events))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(a, b int) bool {
		ea, eb := events[indices[a]], events[indices[b]]
		if ea.StartMin != eb.StartMin {
			return ea.StartMin < eb.StartMin
		}
		if ea.EndMin != eb.EndMin {
			return ea.EndMin < eb.EndMin
		}
		return ea.ID < eb.ID
	})

	layout := make(map[int]EventLayout)
	columns := []int{} // end minutes for each column

	// If there's a pinned event (adjust mode), place it first
	pinnedIdx := -1
	if pinnedID != "" {
		for i, ev := range events {
			if ev.ID == pinnedID {
				pinnedIdx = i
				break
			}
		}
	}
	if pinnedIdx >= 0 {
		for len(columns) <= pinnedCol {
			columns = append(columns, 0)
		}
		columns[pinnedCol] = events[pinnedIdx].EndMin
		layout[pinnedIdx] = EventLayout{Col: pinnedCol}
	}

	// Greedy placement for all other events
	for _, idx := range indices {
		if idx == pinnedIdx {
			continue
		}
		ev := events[idx]
		placed := false
		for c := range columns {
			if columns[c] <= ev.StartMin {
				columns[c] = ev.EndMin
				layout[idx] = EventLayout{Col: c}
				placed = true
				break
			}
		}
		if !placed {
			layout[idx] = EventLayout{Col: len(columns)}
			columns = append(columns, ev.EndMin)
		}
	}

	// Find connected overlap groups using union-find approach.
	// Two events are in the same group if they overlap in time.
	parent := make(map[int]int)
	for _, idx := range indices {
		parent[idx] = idx
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	// Union all overlapping pairs
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			a, b := indices[i], indices[j]
			ea, eb := events[a], events[b]
			if ea.StartMin < eb.EndMin && ea.EndMin > eb.StartMin {
				union(a, b)
			}
		}
	}

	// For each group, find the max column count (max Col + 1)
	groupMaxCol := make(map[int]int)
	for _, idx := range indices {
		root := find(idx)
		col := layout[idx].Col + 1
		if col > groupMaxCol[root] {
			groupMaxCol[root] = col
		}
	}

	// Set TotalCol for all events in each group
	for _, idx := range indices {
		root := find(idx)
		l := layout[idx]
		l.TotalCol = groupMaxCol[root]
		if l.TotalCol < 1 {
			l.TotalCol = 1
		}
		layout[idx] = l
	}

	return layout
}

// EventCount returns the number of events on the given date (including virtual occurrences).
func (s *EventStore) EventCount(date time.Time) int {
	return len(s.GetByDate(date))
}

// SearchMatch represents a search result.
type SearchMatch struct {
	Date    time.Time
	Index   int // index in GetByDate results for this date
	EventID string
}

// AllEvents returns all events across all dates.
func (s *EventStore) AllEvents() map[time.Time][]Event {
	return s.events
}
