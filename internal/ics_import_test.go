package internal

import (
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Supported import cases (task 3.1)
// ---------------------------------------------------------------------------

func TestImportICS_BasicTimedEvent(t *testing.T) {
	path, _ := filepath.Abs("testdata/basic_timed.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	ev := result.Events[0]
	if ev.Title != "Morning standup" {
		t.Errorf("Title = %q, want %q", ev.Title, "Morning standup")
	}
	if ev.Desc != "Daily sync meeting" {
		t.Errorf("Desc = %q, want %q", ev.Desc, "Daily sync meeting")
	}
	// 09:00 local = 540 min, 10:30 local = 630 min (assuming local == file TZ)
	if ev.StartMin != 540 {
		t.Errorf("StartMin = %d, want 540", ev.StartMin)
	}
	if ev.EndMin != 630 {
		t.Errorf("EndMin = %d, want 630", ev.EndMin)
	}
	if ev.Recurrence != RecurNone {
		t.Errorf("Recurrence = %q, want none", ev.Recurrence)
	}
}

func TestImportICS_AllDayEvent(t *testing.T) {
	path, _ := filepath.Abs("testdata/allday.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	ev := result.Events[0]
	if ev.Title != "Team retreat" {
		t.Errorf("Title = %q, want %q", ev.Title, "Team retreat")
	}
	// All-day events: StartMin should be 0
	if ev.StartMin != 0 {
		t.Errorf("StartMin = %d, want 0 for all-day", ev.StartMin)
	}
	// 2-day event: EndMin should be 2 * 1440 = 2880
	if ev.EndMin != 2880 {
		t.Errorf("EndMin = %d, want 2880 for 2-day all-day", ev.EndMin)
	}
}

func TestImportICS_RecurringWeekly(t *testing.T) {
	path, _ := filepath.Abs("testdata/recurring_weekly.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	ev := result.Events[0]
	if ev.Recurrence != RecurWeekly {
		t.Errorf("Recurrence = %q, want %q", ev.Recurrence, RecurWeekly)
	}
}

func TestImportICS_RecurringDailyWeekdays(t *testing.T) {
	path, _ := filepath.Abs("testdata/recurring_daily_weekdays.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	ev := result.Events[0]
	if ev.Recurrence != RecurWeekdays {
		t.Errorf("Recurrence = %q, want %q", ev.Recurrence, RecurWeekdays)
	}
}

func TestImportICS_RecurringMonthly(t *testing.T) {
	path, _ := filepath.Abs("testdata/recurring_monthly.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	if result.Events[0].Recurrence != RecurMonthly {
		t.Errorf("Recurrence = %q, want %q", result.Events[0].Recurrence, RecurMonthly)
	}
}

func TestImportICS_RecurringWithUntil(t *testing.T) {
	path, _ := filepath.Abs("testdata/recurring_with_until.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	ev := result.Events[0]
	if ev.Recurrence != RecurWeekly {
		t.Errorf("Recurrence = %q, want %q", ev.Recurrence, RecurWeekly)
	}
	if ev.RecurUntilStr == "" {
		t.Error("expected RecurUntilStr to be set")
	}
}

func TestImportICS_MultipleEvents(t *testing.T) {
	path, _ := filepath.Abs("testdata/multi_event.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 3 {
		t.Fatalf("expected 3 imported events, got %d", result.Imported)
	}
	// Events should be sorted by date then time
	titles := make([]string, len(result.Events))
	for i, ev := range result.Events {
		titles[i] = ev.Title
	}
	if titles[0] != "First event" || titles[1] != "Second event" || titles[2] != "Third event" {
		t.Errorf("event order = %v, want [First event, Second event, Third event]", titles)
	}
}

func TestImportICS_UTCTimestamps(t *testing.T) {
	path, _ := filepath.Abs("testdata/utc_timestamps.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported event, got %d", result.Imported)
	}
	ev := result.Events[0]
	if ev.Title != "UTC meeting" {
		t.Errorf("Title = %q, want %q", ev.Title, "UTC meeting")
	}
	// UTC timestamps should be converted to local time
	// Exact values depend on test machine's timezone, but start should be set
	if ev.StartMin < 0 || ev.StartMin >= MinutesPerDay {
		t.Errorf("StartMin = %d, should be in valid range", ev.StartMin)
	}
}

// ---------------------------------------------------------------------------
// Malformed / unsupported import cases (task 3.2)
// ---------------------------------------------------------------------------

func TestImportICS_NoVEvent(t *testing.T) {
	path, _ := filepath.Abs("testdata/malformed_no_vevent.ics")
	_, err := ImportICSFile(path)
	if err == nil {
		t.Error("expected error for .ics with no VEVENT")
	}
}

func TestImportICS_MissingDTSTART(t *testing.T) {
	path, _ := filepath.Abs("testdata/malformed_missing_dtstart.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	// Should be skipped, not imported
	if result.Imported != 0 {
		t.Errorf("expected 0 imported events, got %d", result.Imported)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped event, got %d", len(result.Skipped))
	}
}

func TestImportICS_UnsupportedRRULECount(t *testing.T) {
	path, _ := filepath.Abs("testdata/unsupported_rrule_count.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 0 {
		t.Errorf("expected 0 imported for COUNT rule, got %d", result.Imported)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestImportICS_UnsupportedEXDATE(t *testing.T) {
	path, _ := filepath.Abs("testdata/unsupported_exdate.ics")
	result, err := ImportICSFile(path)
	if err != nil {
		t.Fatalf("ImportICSFile: %v", err)
	}
	if result.Imported != 0 {
		t.Errorf("expected 0 imported for EXDATE, got %d", result.Imported)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestImportICS_NonexistentFile(t *testing.T) {
	_, err := ImportICSFile("/nonexistent/path.ics")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestImportICS_EmptyPath(t *testing.T) {
	_, err := ImportICSFile("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestImportICS_NonICSExtension(t *testing.T) {
	_, err := ImportICSFile("testdata/basic_timed.txt")
	if err == nil {
		t.Error("expected error for non-.ics extension")
	}
}

// ---------------------------------------------------------------------------
// ICS parser internals
// ---------------------------------------------------------------------------

func TestParseICSCalendar_UnfoldLines(t *testing.T) {
	// RFC 5545 line folding: continuation lines start with a single space/tab
	// which is stripped during unfolding. The content continues directly.
	data := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nBEGIN:VEVENT\r\nDTSTART:20250315T090000\r\nDTEND:20250315T100000\r\nSUMMARY:A very long title that gets \r\n folded across multiple lines\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	events, err := parseICSCalendar(data)
	if err != nil {
		t.Fatalf("parseICSCalendar: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	summary := firstICSValue(events[0], "SUMMARY")
	want := "A very long title that gets folded across multiple lines"
	if summary != want {
		t.Errorf("summary = %q, want %q", summary, want)
	}
}

func TestParseICSRecurrence_UnsupportedFrequency(t *testing.T) {
	raw := icsEvent{Properties: map[string][]icsProperty{
		"DTSTART": {{Name: "DTSTART", Value: "20250310T090000", Params: map[string]string{}}},
		"RRULE":   {{Name: "RRULE", Value: "FREQ=SECONDLY;INTERVAL=30", Params: map[string]string{}}},
	}}
	start := testDate(t, "2025-03-10")
	_, _, err := parseICSRecurrence(raw, start)
	if err == nil {
		t.Error("expected error for unsupported frequency")
	}
}

func TestUnescapeICSValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello\\world`, `hello\world`},
		{`line one\nline two`, "line one\nline two"},
		{`line one\Nline two`, "line one\nline two"},
		{`comma\, separated`, `comma, separated`},
		{`semi\; colon`, `semi; colon`},
		{`plain text`, `plain text`},
	}
	for _, tt := range tests {
		got := unescapeICSValue(tt.input)
		if got != tt.want {
			t.Errorf("unescapeICSValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWeekdayToICS(t *testing.T) {
	tests := map[string]string{
		"Monday": "MO", "Tuesday": "TU", "Wednesday": "WE",
		"Thursday": "TH", "Friday": "FR", "Saturday": "SA", "Sunday": "SU",
	}
	days := []struct {
		name string
		day  int
	}{
		{"Sunday", 0}, {"Monday", 1}, {"Tuesday", 2}, {"Wednesday", 3},
		{"Thursday", 4}, {"Friday", 5}, {"Saturday", 6},
	}
	for _, d := range days {
		got := weekdayToICS(testDate(t, "2025-03-09").AddDate(0, 0, d.day).Weekday()) // 2025-03-09 is Sunday
		want := tests[d.name]
		if got != want {
			t.Errorf("weekdayToICS(%s) = %q, want %q", d.name, got, want)
		}
	}
}
