package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureStdout runs fn with os.Stdout redirected to a pipe, returning
// whatever fn wrote.  The original stdout is restored before returning.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}

// ---------------------------------------------------------------------------
// parseTimeFlag
// ---------------------------------------------------------------------------

func TestParseTimeFlag(t *testing.T) {
	cases := []struct {
		in  string
		min int
		ok  bool
	}{
		{"9:00", 540, true},
		{"9", 540, true},
		{"930", 570, true},
		{"09:30", 570, true},
		{"1430", 870, true},
		{"14:30", 870, true},
		{"0:00", 0, true},
		{"abc", 0, false},
		{"25:00", 0, false},
	}
	for _, tc := range cases {
		m, err := parseTimeFlag(tc.in)
		if tc.ok {
			if err != nil {
				t.Errorf("parseTimeFlag(%q) unexpected error: %v", tc.in, err)
			} else if m != tc.min {
				t.Errorf("parseTimeFlag(%q) = %d, want %d", tc.in, m, tc.min)
			}
		} else {
			if err == nil {
				t.Errorf("parseTimeFlag(%q) expected error, got %d", tc.in, m)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// RunList
// ---------------------------------------------------------------------------

func TestRunList_DefaultToday(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	ev := makeEvent("Daily standup", todayStr(), 540, 570)
	store.Add(ev)
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunList(nil); err != nil {
			t.Fatalf("RunList: %v", err)
		}
	})
	if !strings.Contains(out, "Daily standup") {
		t.Errorf("expected 'Daily standup' in output, got:\n%s", out)
	}
}

func TestRunList_SpecificDate(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Meeting", "2025-06-15", 600, 660))
	store.Add(makeEvent("Lunch", "2025-06-16", 720, 780))
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunList([]string{"--date", "2025-06-15"}); err != nil {
			t.Fatalf("RunList: %v", err)
		}
	})
	if !strings.Contains(out, "Meeting") {
		t.Errorf("expected 'Meeting' in output, got:\n%s", out)
	}
	if strings.Contains(out, "Lunch") {
		t.Errorf("should not contain 'Lunch' for 2025-06-15, got:\n%s", out)
	}
}

func TestRunList_DateRange(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Day1", "2025-06-10", 540, 600))
	store.Add(makeEvent("Day3", "2025-06-12", 540, 600))
	store.Add(makeEvent("OutOfRange", "2025-06-14", 540, 600))
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunList([]string{"--from", "2025-06-10", "--to", "2025-06-12"}); err != nil {
			t.Fatalf("RunList: %v", err)
		}
	})
	if !strings.Contains(out, "Day1") {
		t.Errorf("expected Day1")
	}
	if !strings.Contains(out, "Day3") {
		t.Errorf("expected Day3")
	}
	if strings.Contains(out, "OutOfRange") {
		t.Errorf("should not contain OutOfRange")
	}
}

func TestRunList_JSON(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("JSONEvent", "2025-06-15", 600, 660))
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunList([]string{"--date", "2025-06-15", "--json"}); err != nil {
			t.Fatalf("RunList: %v", err)
		}
	})

	var ev cliEventJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ev); err != nil {
		t.Fatalf("JSON parse error: %v\nraw: %s", err, out)
	}
	if ev.Title != "JSONEvent" {
		t.Errorf("JSON title = %q, want JSONEvent", ev.Title)
	}
	if ev.Date != "2025-06-15" {
		t.Errorf("JSON date = %q, want 2025-06-15", ev.Date)
	}
	if ev.StartMin != 600 || ev.EndMin != 660 {
		t.Errorf("JSON times = %d-%d, want 600-660", ev.StartMin, ev.EndMin)
	}
}

func TestRunList_InvalidDate(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	err := RunList([]string{"--date", "not-a-date"})
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestRunList_RangeToBeforeFrom(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	err := RunList([]string{"--from", "2025-06-15", "--to", "2025-06-10"})
	if err == nil {
		t.Fatal("expected error when --to is before --from")
	}
}

func TestRunList_EmptyDay(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunList([]string{"--date", "2025-01-01"}); err != nil {
			t.Fatalf("RunList: %v", err)
		}
	})
	if !strings.Contains(out, "no events") {
		t.Errorf("expected '(no events)' for empty day, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// RunSearch
// ---------------------------------------------------------------------------

func TestRunSearch_Matches(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Weekly standup", "2025-06-15", 540, 570))
	store.Add(makeEvent("Lunch break", "2025-06-15", 720, 780))
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunSearch([]string{"standup"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}
	})
	if !strings.Contains(out, "Weekly standup") {
		t.Errorf("expected match for 'standup', got:\n%s", out)
	}
	if strings.Contains(out, "Lunch") {
		t.Errorf("should not match 'Lunch'")
	}
}

func TestRunSearch_NoMatches(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Meeting", "2025-06-15", 540, 570))
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunSearch([]string{"nonexistent"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output for no matches, got:\n%s", out)
	}
}

func TestRunSearch_JSON(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Find me", "2025-06-15", 600, 660))
	SaveEvents(store)

	// Flags must precede positional args (standard flag package behavior).
	out := captureStdout(t, func() {
		if err := RunSearch([]string{"--json", "find"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}
	})
	var ev cliEventJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ev); err != nil {
		t.Fatalf("JSON parse error: %v\nraw: %s", err, out)
	}
	if ev.Title != "Find me" {
		t.Errorf("JSON title = %q, want 'Find me'", ev.Title)
	}
}

func TestRunSearch_MissingQuery(t *testing.T) {
	err := RunSearch(nil)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

// ---------------------------------------------------------------------------
// RunAdd
// ---------------------------------------------------------------------------

func TestRunAdd_Success(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	// Start with empty store.
	SaveEvents(NewEventStore())

	out := captureStdout(t, func() {
		err := RunAdd([]string{
			"--title", "New event",
			"--date", "2025-06-15",
			"--start", "9:00",
			"--end", "10:00",
		})
		if err != nil {
			t.Fatalf("RunAdd: %v", err)
		}
	})

	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("expected event ID on stdout")
	}

	// Verify the event was persisted.
	store, _ := LoadEvents()
	events := store.GetByDate(testDate(t, "2025-06-15"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Title != "New event" {
		t.Errorf("title = %q, want 'New event'", events[0].Title)
	}
	if events[0].StartMin != 540 || events[0].EndMin != 600 {
		t.Errorf("time = %d-%d, want 540-600", events[0].StartMin, events[0].EndMin)
	}
}

func TestRunAdd_WithRecurrence(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()
	SaveEvents(NewEventStore())

	captureStdout(t, func() {
		err := RunAdd([]string{
			"--title", "Daily sync",
			"--date", "2025-06-15",
			"--start", "9",
			"--end", "930",
			"--recurrence", "daily",
		})
		if err != nil {
			t.Fatalf("RunAdd: %v", err)
		}
	})

	store, _ := LoadEvents()
	events := store.GetByDate(testDate(t, "2025-06-15"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Recurrence != RecurDaily {
		t.Errorf("recurrence = %q, want daily", events[0].Recurrence)
	}
}

func TestRunAdd_MissingFlags(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	err := RunAdd([]string{"--title", "Only title"})
	if err == nil {
		t.Fatal("expected error for missing flags")
	}
	if !strings.Contains(err.Error(), "--date") {
		t.Errorf("error should mention --date, got: %v", err)
	}
}

func TestRunAdd_InvalidTime(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	err := RunAdd([]string{
		"--title", "Bad time",
		"--date", "2025-06-15",
		"--start", "25:00",
		"--end", "10:00",
	})
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
}

func TestRunAdd_InvalidRecurrence(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	err := RunAdd([]string{
		"--title", "Test",
		"--date", "2025-06-15",
		"--start", "9:00",
		"--end", "10:00",
		"--recurrence", "every-full-moon",
	})
	if err == nil {
		t.Fatal("expected error for invalid recurrence")
	}
}

func TestRunAdd_EndBeforeStart(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()
	SaveEvents(NewEventStore())

	err := RunAdd([]string{
		"--title", "Backward",
		"--date", "2025-06-15",
		"--start", "10:00",
		"--end", "9:00",
	})
	if err == nil {
		t.Fatal("expected error when end < start")
	}
}

// ---------------------------------------------------------------------------
// RunDelete
// ---------------------------------------------------------------------------

func TestRunDelete_Success(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	ev := makeEvent("Delete me", "2025-06-15", 600, 660)
	store.Add(ev)
	SaveEvents(store)

	out := captureStdout(t, func() {
		if err := RunDelete([]string{ev.ID}); err != nil {
			t.Fatalf("RunDelete: %v", err)
		}
	})
	if strings.TrimSpace(out) != ev.ID {
		t.Errorf("stdout = %q, want %q", strings.TrimSpace(out), ev.ID)
	}

	// Verify deleted.
	store2, _ := LoadEvents()
	events := store2.GetByDate(testDate(t, "2025-06-15"))
	if len(events) != 0 {
		t.Errorf("expected 0 events after delete, got %d", len(events))
	}
}

func TestRunDelete_NotFound(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()
	SaveEvents(NewEventStore())

	err := RunDelete([]string{"nonexistent-id"})
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestRunDelete_MissingArg(t *testing.T) {
	err := RunDelete(nil)
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

// ---------------------------------------------------------------------------
// RunImport
// ---------------------------------------------------------------------------

func TestRunImport_Success(t *testing.T) {
	dataDir, cleanup := tempDataDir(t)
	defer cleanup()
	SaveEvents(NewEventStore())

	// Create a minimal ICS file in the temp dir.
	icsContent := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nDTSTART:20250615T090000\r\nDTEND:20250615T100000\r\nSUMMARY:Imported event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	icsPath := filepath.Join(dataDir, "test.ics")
	os.WriteFile(icsPath, []byte(icsContent), 0644)

	out := captureStdout(t, func() {
		if err := RunImport([]string{icsPath}); err != nil {
			t.Fatalf("RunImport: %v", err)
		}
	})
	if !strings.Contains(out, "imported 1") {
		t.Errorf("expected 'imported 1' in output, got:\n%s", out)
	}

	// Verify persisted.
	store, _ := LoadEvents()
	events := store.GetByDate(testDate(t, "2025-06-15"))
	if len(events) == 0 {
		t.Error("expected imported event to be stored")
	}
}

func TestRunImport_NonICSFile(t *testing.T) {
	err := RunImport([]string{"notes.txt"})
	if err == nil {
		t.Fatal("expected error for non-.ics file")
	}
	if !strings.Contains(err.Error(), ".ics") {
		t.Errorf("error should mention .ics, got: %v", err)
	}
}

func TestRunImport_FileNotFound(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	err := RunImport([]string{"/no/such/file.ics"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRunImport_MissingArg(t *testing.T) {
	err := RunImport(nil)
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

// ---------------------------------------------------------------------------
// RunExport
// ---------------------------------------------------------------------------

func TestRunExport_Success(t *testing.T) {
	dataDir, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Export me", "2025-06-15", 600, 660))
	SaveEvents(store)

	outPath := filepath.Join(dataDir, "out.ics")
	out := captureStdout(t, func() {
		if err := RunExport([]string{outPath}); err != nil {
			t.Fatalf("RunExport: %v", err)
		}
	})
	if !strings.Contains(out, "exported 1") {
		t.Errorf("expected 'exported 1' in output, got:\n%s", out)
	}
	if !strings.Contains(out, outPath) {
		t.Errorf("expected output path in output, got:\n%s", out)
	}

	// Verify file was written.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	if !strings.Contains(string(data), "Export me") {
		t.Error("export file should contain event title")
	}
}

func TestRunExport_AppendsExtension(t *testing.T) {
	dataDir, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	store.Add(makeEvent("Test", "2025-06-15", 600, 660))
	SaveEvents(store)

	outBase := filepath.Join(dataDir, "backup")
	out := captureStdout(t, func() {
		if err := RunExport([]string{outBase}); err != nil {
			t.Fatalf("RunExport: %v", err)
		}
	})
	if !strings.Contains(out, outBase+".ics") {
		t.Errorf("expected .ics extension appended, got:\n%s", out)
	}
}

func TestRunExport_EmptyStore(t *testing.T) {
	dataDir, cleanup := tempDataDir(t)
	defer cleanup()
	SaveEvents(NewEventStore())

	outPath := filepath.Join(dataDir, "empty.ics")
	out := captureStdout(t, func() {
		if err := RunExport([]string{outPath}); err != nil {
			t.Fatalf("RunExport: %v", err)
		}
	})
	if !strings.Contains(out, "exported 0") {
		t.Errorf("expected 'exported 0' in output, got:\n%s", out)
	}
}

func TestRunExport_MissingArg(t *testing.T) {
	err := RunExport(nil)
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func todayStr() string {
	return DateKey(time.Now()).Format("2006-01-02")
}
