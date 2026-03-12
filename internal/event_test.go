package internal

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestMinToTime(t *testing.T) {
	tests := []struct {
		min  int
		want string
	}{
		{0, "00:00"},
		{60, "01:00"},
		{90, "01:30"},
		{720, "12:00"},
		{1439, "23:59"},
	}
	for _, tt := range tests {
		got := MinToTime(tt.min)
		if got != tt.want {
			t.Errorf("MinToTime(%d) = %q, want %q", tt.min, got, tt.want)
		}
	}
}

func TestRecurrenceLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{RecurNone, "None"},
		{"", "None"},
		{RecurDaily, "Daily"},
		{RecurWeekdays, "Weekdays"},
		{RecurWeekly, "Weekly"},
		{RecurBiweekly, "Biweekly"},
		{RecurMonthly, "Monthly"},
		{RecurYearly, "Yearly"},
		{"unknown", "None"},
	}
	for _, tt := range tests {
		got := RecurrenceLabel(tt.input)
		if got != tt.want {
			t.Errorf("RecurrenceLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDateKey(t *testing.T) {
	loc := time.Local
	input := time.Date(2025, 3, 15, 14, 30, 45, 123, loc)
	got := DateKey(input)
	want := time.Date(2025, 3, 15, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("DateKey(%v) = %v, want %v", input, got, want)
	}
}

func TestIsRecurring(t *testing.T) {
	ev := Event{Recurrence: RecurWeekly}
	if !ev.IsRecurring() {
		t.Error("expected IsRecurring() true for weekly event")
	}
	ev.Recurrence = ""
	if ev.IsRecurring() {
		t.Error("expected IsRecurring() false for empty recurrence")
	}
	ev.Recurrence = RecurNone
	if ev.IsRecurring() {
		t.Error("expected IsRecurring() false for RecurNone")
	}
}

func TestClampDay(t *testing.T) {
	tests := []struct {
		year  int
		month time.Month
		day   int
		want  int
	}{
		{2025, time.February, 28, 28},
		{2025, time.February, 30, 28}, // Feb has 28 days in 2025
		{2024, time.February, 29, 29}, // leap year
		{2024, time.February, 30, 29},
		{2025, time.January, 31, 31},
		{2025, time.April, 31, 30}, // April has 30 days
		{2025, time.April, 15, 15},
	}
	for _, tt := range tests {
		got := clampDay(tt.year, tt.month, tt.day)
		if got != tt.want {
			t.Errorf("clampDay(%d, %v, %d) = %d, want %d", tt.year, tt.month, tt.day, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Recurrence: matchesDate
// ---------------------------------------------------------------------------

func TestMatchesDate_Daily(t *testing.T) {
	base := testDate(t, "2025-03-10")
	ev := Event{Date: base, DateStr: "2025-03-10", Recurrence: RecurDaily}

	// Same day should not match (matchesDate requires after base)
	if matchesDate(ev, base) {
		t.Error("daily: should not match base date itself")
	}
	// Next day
	if !matchesDate(ev, base.AddDate(0, 0, 1)) {
		t.Error("daily: should match next day")
	}
	// Before base
	if matchesDate(ev, base.AddDate(0, 0, -1)) {
		t.Error("daily: should not match before base")
	}
	// Far future
	if !matchesDate(ev, base.AddDate(1, 0, 0)) {
		t.Error("daily: should match far future")
	}
}

func TestMatchesDate_Weekdays(t *testing.T) {
	base := testDate(t, "2025-03-10") // Monday
	ev := Event{Date: base, DateStr: "2025-03-10", Recurrence: RecurWeekdays}

	// Tuesday
	if !matchesDate(ev, testDate(t, "2025-03-11")) {
		t.Error("weekdays: should match Tuesday")
	}
	// Friday
	if !matchesDate(ev, testDate(t, "2025-03-14")) {
		t.Error("weekdays: should match Friday")
	}
	// Saturday
	if matchesDate(ev, testDate(t, "2025-03-15")) {
		t.Error("weekdays: should not match Saturday")
	}
	// Sunday
	if matchesDate(ev, testDate(t, "2025-03-16")) {
		t.Error("weekdays: should not match Sunday")
	}
}

func TestMatchesDate_Weekly(t *testing.T) {
	base := testDate(t, "2025-03-10") // Monday
	ev := Event{Date: base, DateStr: "2025-03-10", Recurrence: RecurWeekly}

	// Next Monday
	if !matchesDate(ev, testDate(t, "2025-03-17")) {
		t.Error("weekly: should match next Monday")
	}
	// Tuesday (wrong day)
	if matchesDate(ev, testDate(t, "2025-03-11")) {
		t.Error("weekly: should not match Tuesday")
	}
	// Monday 4 weeks later
	if !matchesDate(ev, testDate(t, "2025-04-07")) {
		t.Error("weekly: should match Monday 4 weeks later")
	}
}

func TestMatchesDate_Biweekly(t *testing.T) {
	// Use dates well outside DST transitions to avoid Hours()/24 rounding issues.
	base := testDate(t, "2025-01-06") // Monday in January (no DST)
	ev := Event{Date: base, DateStr: "2025-01-06", Recurrence: RecurBiweekly}

	// Implementation uses (days/7)%2==0 where days = int(query.Sub(base).Hours()/24).
	// 1 week: 7/7=1, 1%2=1 -> skip
	if matchesDate(ev, testDate(t, "2025-01-13")) {
		t.Error("biweekly: should not match 1 week later")
	}
	// 2 weeks: 14/7=2, 2%2=0 -> match
	if !matchesDate(ev, testDate(t, "2025-01-20")) {
		t.Error("biweekly: should match 2 weeks later")
	}
	// 3 weeks: 21/7=3, 3%2=1 -> skip
	if matchesDate(ev, testDate(t, "2025-01-27")) {
		t.Error("biweekly: should not match 3 weeks later")
	}
	// 4 weeks: 28/7=4, 4%2=0 -> match
	if !matchesDate(ev, testDate(t, "2025-02-03")) {
		t.Error("biweekly: should match 4 weeks later")
	}
	// Wrong weekday should not match
	if matchesDate(ev, testDate(t, "2025-01-21")) {
		t.Error("biweekly: should not match wrong weekday")
	}
}

func TestMatchesDate_Monthly(t *testing.T) {
	base := testDate(t, "2025-01-15")
	ev := Event{Date: base, DateStr: "2025-01-15", Recurrence: RecurMonthly}

	// Feb 15
	if !matchesDate(ev, testDate(t, "2025-02-15")) {
		t.Error("monthly: should match Feb 15")
	}
	// Feb 14 (wrong day)
	if matchesDate(ev, testDate(t, "2025-02-14")) {
		t.Error("monthly: should not match Feb 14")
	}
}

func TestMatchesDate_Monthly_Clamp(t *testing.T) {
	// Event on Jan 31 should clamp to last day of shorter months
	base := testDate(t, "2025-01-31")
	ev := Event{Date: base, DateStr: "2025-01-31", Recurrence: RecurMonthly}

	// Feb 28 (last day of Feb 2025)
	if !matchesDate(ev, testDate(t, "2025-02-28")) {
		t.Error("monthly clamp: should match Feb 28 for Jan 31 event")
	}
	// Apr 30 (last day of April)
	if !matchesDate(ev, testDate(t, "2025-04-30")) {
		t.Error("monthly clamp: should match Apr 30 for Jan 31 event")
	}
}

func TestMatchesDate_Yearly(t *testing.T) {
	base := testDate(t, "2025-06-15")
	ev := Event{Date: base, DateStr: "2025-06-15", Recurrence: RecurYearly}

	// Same date next year
	if !matchesDate(ev, testDate(t, "2026-06-15")) {
		t.Error("yearly: should match same date next year")
	}
	// Different month
	if matchesDate(ev, testDate(t, "2026-03-15")) {
		t.Error("yearly: should not match different month")
	}
	// Different day
	if matchesDate(ev, testDate(t, "2026-06-14")) {
		t.Error("yearly: should not match different day")
	}
}

func TestMatchesDate_RecurUntil(t *testing.T) {
	base := testDate(t, "2025-03-10")
	ev := Event{
		Date:          base,
		DateStr:       "2025-03-10",
		Recurrence:    RecurDaily,
		RecurUntilStr: "2025-03-15",
	}

	// Within range
	if !matchesDate(ev, testDate(t, "2025-03-12")) {
		t.Error("until: should match within range")
	}
	// On until date
	if !matchesDate(ev, testDate(t, "2025-03-15")) {
		t.Error("until: should match on until date")
	}
	// After until date
	if matchesDate(ev, testDate(t, "2025-03-16")) {
		t.Error("until: should not match after until date")
	}
}

func TestMatchesDate_ExceptionDates(t *testing.T) {
	base := testDate(t, "2025-03-10")
	ev := Event{
		Date:           base,
		DateStr:        "2025-03-10",
		Recurrence:     RecurDaily,
		ExceptionDates: []string{"2025-03-12"},
	}

	// Normal day
	if !matchesDate(ev, testDate(t, "2025-03-11")) {
		t.Error("exception: should match non-excepted day")
	}
	// Excepted day
	if matchesDate(ev, testDate(t, "2025-03-12")) {
		t.Error("exception: should not match excepted day")
	}
}

// ---------------------------------------------------------------------------
// EventStore operations
// ---------------------------------------------------------------------------

func TestEventStore_AddAndGetByDate(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Test", "2025-03-15", 540, 600) // 9:00-10:00
	if err := store.Add(ev); err != nil {
		t.Fatalf("Add: %v", err)
	}

	date := testDate(t, "2025-03-15")
	events := store.GetByDate(date)
	if len(events) != 1 {
		t.Fatalf("GetByDate: got %d events, want 1", len(events))
	}
	if events[0].Title != "Test" {
		t.Errorf("Title = %q, want %q", events[0].Title, "Test")
	}
}

func TestEventStore_AddValidation(t *testing.T) {
	store := NewEventStore()

	// End before start
	ev := makeEvent("Bad", "2025-03-15", 600, 540)
	if err := store.Add(ev); err == nil {
		t.Error("expected error for end < start")
	}

	// Equal start and end
	ev = makeEvent("Bad", "2025-03-15", 600, 600)
	if err := store.Add(ev); err == nil {
		t.Error("expected error for end == start")
	}

	// Negative start
	ev = makeEvent("Bad", "2025-03-15", -1, 60)
	if err := store.Add(ev); err == nil {
		t.Error("expected error for negative start")
	}

	// End beyond midnight
	ev = makeEvent("Bad", "2025-03-15", 0, MinutesPerDay+1)
	if err := store.Add(ev); err == nil {
		t.Error("expected error for end > MinutesPerDay")
	}
}

func TestEventStore_Delete(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("ToDelete", "2025-03-15", 540, 600)
	store.Add(ev)

	date := testDate(t, "2025-03-15")
	events := store.GetByDate(date)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	store.Delete(date, 0)
	events = store.GetByDate(date)
	if len(events) != 0 {
		t.Errorf("expected 0 events after delete, got %d", len(events))
	}
}

func TestEventStore_MoveEvent(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Movable", "2025-03-15", 540, 600)
	store.Add(ev)

	date := testDate(t, "2025-03-15")
	// Move down 60 min
	if err := store.MoveEvent(date, 0, 60); err != nil {
		t.Fatalf("MoveEvent: %v", err)
	}
	events := store.GetByDate(date)
	if events[0].StartMin != 600 || events[0].EndMin != 660 {
		t.Errorf("after move: start=%d end=%d, want 600/660", events[0].StartMin, events[0].EndMin)
	}

	// Move out of bounds
	if err := store.MoveEvent(date, 0, -700); err == nil {
		t.Error("expected error for out of bounds move")
	}
}

func TestEventStore_GetByDate_VirtualOccurrences(t *testing.T) {
	store := NewEventStore()
	ev := makeRecurringEvent("Weekly", "2025-03-10", 540, 600, RecurWeekly)
	store.Add(ev)

	// Check base date
	base := testDate(t, "2025-03-10")
	events := store.GetByDate(base)
	if len(events) != 1 {
		t.Fatalf("base date: got %d events, want 1", len(events))
	}

	// Check next occurrence (same weekday, 1 week later)
	next := testDate(t, "2025-03-17")
	events = store.GetByDate(next)
	if len(events) != 1 {
		t.Fatalf("virtual occurrence: got %d events, want 1", len(events))
	}
	if events[0].Title != "Weekly" {
		t.Errorf("virtual Title = %q, want %q", events[0].Title, "Weekly")
	}

	// Non-matching day
	nonMatch := testDate(t, "2025-03-11")
	events = store.GetByDate(nonMatch)
	if len(events) != 0 {
		t.Errorf("non-match: got %d events, want 0", len(events))
	}
}

func TestEventStore_FindEventByID(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Findable", "2025-03-15", 540, 600)
	store.Add(ev)

	date := testDate(t, "2025-03-15")
	stored := store.GetByDate(date)
	id := stored[0].ID

	foundDate, foundIdx := store.FindEventByID(id)
	if foundIdx < 0 {
		t.Fatal("FindEventByID: not found")
	}
	if !DateKey(foundDate).Equal(date) {
		t.Errorf("FindEventByID date = %v, want %v", foundDate, date)
	}

	// Not found
	_, idx := store.FindEventByID("nonexistent")
	if idx != -1 {
		t.Error("expected -1 for nonexistent ID")
	}
}

func TestEventStore_Snapshot_Restore(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Snap", "2025-03-15", 540, 600)
	store.Add(ev)

	snap := store.Snapshot()

	// Modify store
	store.Delete(testDate(t, "2025-03-15"), 0)
	if len(store.GetByDate(testDate(t, "2025-03-15"))) != 0 {
		t.Fatal("expected 0 events after delete")
	}

	// Restore
	store.Restore(snap)
	events := store.GetByDate(testDate(t, "2025-03-15"))
	if len(events) != 1 {
		t.Fatalf("after restore: got %d events, want 1", len(events))
	}
	if events[0].Title != "Snap" {
		t.Errorf("restored Title = %q, want %q", events[0].Title, "Snap")
	}
}

func TestEventStore_EventAtMinute(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Overlap", "2025-03-15", 540, 600) // 9:00-10:00
	store.Add(ev)

	date := testDate(t, "2025-03-15")
	if idx := store.EventAtMinute(date, 540); idx < 0 {
		t.Error("expected event at start minute")
	}
	if idx := store.EventAtMinute(date, 570); idx < 0 {
		t.Error("expected event at middle minute")
	}
	if idx := store.EventAtMinute(date, 600); idx != -1 {
		t.Error("expected no event at end minute (exclusive)")
	}
	if idx := store.EventAtMinute(date, 500); idx != -1 {
		t.Error("expected no event before start")
	}
}

func TestEventStore_AddException(t *testing.T) {
	store := NewEventStore()
	ev := makeRecurringEvent("Recurring", "2025-03-10", 540, 600, RecurWeekly)
	store.Add(ev)

	base := testDate(t, "2025-03-10")
	stored := store.GetStoredByDate(base)
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(stored))
	}

	// Add exception for the next occurrence (March 17)
	err := store.AddException(stored[0].ID, testDate(t, "2025-03-17"))
	if err != nil {
		t.Fatalf("AddException: %v", err)
	}

	// March 17 should now have no events
	events := store.GetByDate(testDate(t, "2025-03-17"))
	if len(events) != 0 {
		t.Errorf("expected 0 events on excepted date, got %d", len(events))
	}

	// March 24 should still match
	events = store.GetByDate(testDate(t, "2025-03-24"))
	if len(events) != 1 {
		t.Errorf("expected 1 event on March 24, got %d", len(events))
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		input string
		want  int
		err   bool
	}{
		{"9:00", 540, false},
		{"14:30", 870, false},
		{"0:00", 0, false},
		{"23:59", 1439, false},
		{"9", 540, false},
		{"12", 720, false},
		{"930", 570, false},
		{"1430", 870, false},
		{"0930", 570, false},
		{"24:00", 0, true},
		{"12:60", 0, true},
		{"abc", 0, true},
		{"", 0, true},
		{"99999", 0, true},
	}
	for _, tt := range tests {
		got, err := parseTime(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("parseTime(%q): expected error, got %d", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTime(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseTime(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestEventStore_MoveEventToDate(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Movable", "2025-03-15", 540, 600)
	store.Add(ev)

	from := testDate(t, "2025-03-15")
	to := testDate(t, "2025-03-20")
	newIdx, err := store.MoveEventToDate(from, 0, to)
	if err != nil {
		t.Fatalf("MoveEventToDate: %v", err)
	}
	if newIdx < 0 {
		t.Error("expected valid new index")
	}

	// Old date should be empty
	if len(store.GetByDate(from)) != 0 {
		t.Error("expected 0 events on old date")
	}
	// New date should have the event
	events := store.GetByDate(to)
	if len(events) != 1 {
		t.Fatalf("expected 1 event on new date, got %d", len(events))
	}
	if events[0].Title != "Movable" {
		t.Errorf("Title = %q, want %q", events[0].Title, "Movable")
	}
}
