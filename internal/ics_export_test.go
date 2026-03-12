package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Export regression tests (task 3.3)
// ---------------------------------------------------------------------------

func TestExportICS_BasicTimedEvent(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Test Export", "2025-03-15", 540, 600) // 9:00-10:00
	ev.Desc = "A test event"
	store.Add(ev)

	content, count := buildICSExport(store)
	if count != 1 {
		t.Fatalf("exported %d events, want 1", count)
	}

	if !strings.Contains(content, "BEGIN:VCALENDAR") {
		t.Error("missing VCALENDAR begin")
	}
	if !strings.Contains(content, "END:VCALENDAR") {
		t.Error("missing VCALENDAR end")
	}
	if !strings.Contains(content, "BEGIN:VEVENT") {
		t.Error("missing VEVENT begin")
	}
	if !strings.Contains(content, "SUMMARY:Test Export") {
		t.Error("missing SUMMARY")
	}
	if !strings.Contains(content, "DESCRIPTION:A test event") {
		t.Error("missing DESCRIPTION")
	}
	if !strings.Contains(content, "DTSTART:") {
		t.Error("missing DTSTART")
	}
	if !strings.Contains(content, "DTEND:") {
		t.Error("missing DTEND")
	}
	if !strings.Contains(content, "PRODID:-//vimalender//EN") {
		t.Error("missing PRODID")
	}
}

func TestExportICS_RecurrenceRules(t *testing.T) {
	tests := []struct {
		recurrence string
		wantRule   string
	}{
		{RecurDaily, "FREQ=DAILY"},
		{RecurWeekdays, "BYDAY=MO,TU,WE,TH,FR"},
		{RecurWeekly, "FREQ=WEEKLY"},
		{RecurBiweekly, "INTERVAL=2"},
		{RecurMonthly, "FREQ=MONTHLY"},
		{RecurYearly, "FREQ=YEARLY"},
	}
	for _, tt := range tests {
		store := NewEventStore()
		ev := makeRecurringEvent("Recurring", "2025-03-10", 540, 600, tt.recurrence) // Monday
		store.Add(ev)

		content, _ := buildICSExport(store)
		if !strings.Contains(content, "RRULE:") {
			t.Errorf("recurrence %q: missing RRULE line", tt.recurrence)
			continue
		}
		if !strings.Contains(content, tt.wantRule) {
			t.Errorf("recurrence %q: expected RRULE to contain %q in:\n%s", tt.recurrence, tt.wantRule, content)
		}
	}
}

func TestExportICS_NoRecurrence(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("One-time", "2025-03-15", 540, 600)
	store.Add(ev)

	content, _ := buildICSExport(store)
	if strings.Contains(content, "RRULE:") {
		t.Error("non-recurring event should not have RRULE")
	}
}

func TestExportICS_WithRecurUntil(t *testing.T) {
	store := NewEventStore()
	ev := makeRecurringEvent("Limited", "2025-03-10", 540, 600, RecurWeekly)
	ev.RecurUntilStr = "2025-06-30"
	store.Add(ev)

	content, _ := buildICSExport(store)
	if !strings.Contains(content, "UNTIL=") {
		t.Error("expected UNTIL in RRULE")
	}
}

func TestExportICS_AllDayEvent(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("All day", "2025-03-15", 0, MinutesPerDay) // midnight to midnight = 1 day
	store.Add(ev)

	content, _ := buildICSExport(store)
	if !strings.Contains(content, "VALUE=DATE") {
		t.Error("all-day event should use VALUE=DATE format")
	}
}

func TestExportICS_WithNotes(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("With notes", "2025-03-15", 540, 600)
	ev.Notes = "Some important notes"
	store.Add(ev)

	content, _ := buildICSExport(store)
	if !strings.Contains(content, "COMMENT:Some important notes") {
		t.Error("expected COMMENT field for notes")
	}
}

func TestExportICS_WithExceptionDates(t *testing.T) {
	store := NewEventStore()
	ev := makeRecurringEvent("Excepted", "2025-03-10", 540, 600, RecurWeekly)
	ev.ExceptionDates = []string{"2025-03-17", "2025-03-24"}
	store.Add(ev)

	content, _ := buildICSExport(store)
	exdateCount := strings.Count(content, "EXDATE:")
	if exdateCount != 2 {
		t.Errorf("expected 2 EXDATE lines, got %d", exdateCount)
	}
}

func TestExportICS_EscapesSpecialChars(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Title, with; special\\chars", "2025-03-15", 540, 600)
	ev.Desc = "Line one\nLine two"
	store.Add(ev)

	content, _ := buildICSExport(store)
	if !strings.Contains(content, `\,`) {
		t.Error("commas should be escaped in SUMMARY")
	}
	if !strings.Contains(content, `\;`) {
		t.Error("semicolons should be escaped in SUMMARY")
	}
	if !strings.Contains(content, `\\`) {
		t.Error("backslashes should be escaped in SUMMARY")
	}
	if !strings.Contains(content, `\n`) {
		t.Error("newlines should be escaped in DESCRIPTION")
	}
}

func TestExportICS_MultipleEventsSorted(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Later", "2025-03-20", 540, 600))
	store.Add(makeEvent("Earlier", "2025-03-10", 540, 600))

	content, count := buildICSExport(store)
	if count != 2 {
		t.Fatalf("exported %d events, want 2", count)
	}

	// Events should appear in date order
	earlierIdx := strings.Index(content, "SUMMARY:Earlier")
	laterIdx := strings.Index(content, "SUMMARY:Later")
	if earlierIdx < 0 || laterIdx < 0 {
		t.Fatal("both events should appear in output")
	}
	if earlierIdx > laterIdx {
		t.Error("events should be sorted by date (Earlier before Later)")
	}
}

func TestExportICSFile_WritesFile(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("File test", "2025-03-15", 540, 600)
	store.Add(ev)

	dir := t.TempDir()
	path := filepath.Join(dir, "export.ics")

	result, err := ExportICSFile(path, store)
	if err != nil {
		t.Fatalf("ExportICSFile: %v", err)
	}
	if result.Exported != 1 {
		t.Errorf("Exported = %d, want 1", result.Exported)
	}
	if result.OutputPath == "" {
		t.Error("OutputPath should not be empty")
	}

	// Verify file exists and is readable
	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "BEGIN:VCALENDAR") {
		t.Error("exported file should contain VCALENDAR")
	}
}

func TestExportICSFile_AddsExtension(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Ext test", "2025-03-15", 540, 600))

	dir := t.TempDir()
	path := filepath.Join(dir, "export") // no .ics extension

	result, err := ExportICSFile(path, store)
	if err != nil {
		t.Fatalf("ExportICSFile: %v", err)
	}
	if !strings.HasSuffix(result.OutputPath, ".ics") {
		t.Errorf("OutputPath = %q, should end with .ics", result.OutputPath)
	}
}

func TestFoldICSLine(t *testing.T) {
	short := "SUMMARY:Short"
	if got := foldICSLine(short); got != short {
		t.Errorf("short line should not be folded: %q", got)
	}

	long := "SUMMARY:" + strings.Repeat("x", 80)
	folded := foldICSLine(long)
	if !strings.Contains(folded, "\r\n ") {
		t.Error("long line should be folded with CRLF+space")
	}
}

func TestEscapeICSValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`plain`, `plain`},
		{`comma, here`, `comma\, here`},
		{`semi; here`, `semi\; here`},
		{`back\slash`, `back\\slash`},
		{"new\nline", `new\nline`},
	}
	for _, tt := range tests {
		got := escapeICSValue(tt.input)
		if got != tt.want {
			t.Errorf("escapeICSValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExportRecurrenceRule(t *testing.T) {
	base := time.Date(2025, 3, 10, 0, 0, 0, 0, time.Local) // Monday
	tests := []struct {
		recurrence string
		wantParts  []string
	}{
		{RecurNone, nil},
		{RecurDaily, []string{"FREQ=DAILY"}},
		{RecurWeekdays, []string{"FREQ=DAILY", "BYDAY=MO,TU,WE,TH,FR"}},
		{RecurWeekly, []string{"FREQ=WEEKLY", "BYDAY=MO"}},
		{RecurBiweekly, []string{"FREQ=WEEKLY", "INTERVAL=2", "BYDAY=MO"}},
		{RecurMonthly, []string{"FREQ=MONTHLY", "BYMONTHDAY=10"}},
		{RecurYearly, []string{"FREQ=YEARLY", "BYMONTH=3", "BYMONTHDAY=10"}},
	}
	for _, tt := range tests {
		ev := Event{Date: base, Recurrence: tt.recurrence}
		rule := exportRecurrenceRule(ev)
		if tt.wantParts == nil {
			if rule != "" {
				t.Errorf("recurrence %q: expected empty rule, got %q", tt.recurrence, rule)
			}
			continue
		}
		for _, part := range tt.wantParts {
			if !strings.Contains(rule, part) {
				t.Errorf("recurrence %q: rule %q missing %q", tt.recurrence, rule, part)
			}
		}
	}
}
