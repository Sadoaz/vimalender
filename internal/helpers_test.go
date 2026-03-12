package internal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testDate returns a local time.Time for the given date string (YYYY-MM-DD).
func testDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatalf("testDate(%q): %v", s, err)
	}
	return d
}

// makeEvent creates a minimal Event for testing.
func makeEvent(title, dateStr string, startMin, endMin int) Event {
	d, _ := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	return Event{
		ID:       GenerateID(),
		Title:    title,
		Date:     d,
		DateStr:  dateStr,
		StartMin: startMin,
		EndMin:   endMin,
	}
}

// makeRecurringEvent creates an Event with a recurrence pattern.
func makeRecurringEvent(title, dateStr string, startMin, endMin int, recurrence string) Event {
	ev := makeEvent(title, dateStr, startMin, endMin)
	ev.Recurrence = recurrence
	return ev
}

// tempDataDir creates a temporary directory and sets XDG_DATA_HOME so that
// LoadEvents/SaveEvents/LoadSettings/SaveSettings use it for test isolation.
// Returns the temp dir path and a cleanup function.
func tempDataDir(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	orig := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmp)
	// Create the vimalender subdirectory
	os.MkdirAll(filepath.Join(tmp, "vimalender"), 0755)
	return filepath.Join(tmp, "vimalender"), func() {
		if orig == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", orig)
		}
	}
}

// readFixture reads a file from the testdata directory.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("readFixture(%q): %v", name, err)
	}
	return string(data)
}
