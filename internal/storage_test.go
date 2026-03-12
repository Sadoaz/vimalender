package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Event persistence
// ---------------------------------------------------------------------------

func TestSaveAndLoadEvents_RoundTrip(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	ev1 := makeEvent("Meeting", "2025-03-15", 540, 600)
	ev2 := makeEvent("Lunch", "2025-03-15", 720, 780)
	ev3 := makeRecurringEvent("Standup", "2025-03-10", 480, 510, RecurDaily)
	store.Add(ev1)
	store.Add(ev2)
	store.Add(ev3)

	if err := SaveEvents(store); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	loaded, errMsg := LoadEvents()
	if errMsg != "" {
		t.Fatalf("LoadEvents error: %s", errMsg)
	}

	// Check March 15 has 2 events
	date15 := testDate(t, "2025-03-15")
	events15 := loaded.GetStoredByDate(date15)
	if len(events15) != 2 {
		t.Errorf("loaded March 15: got %d events, want 2", len(events15))
	}

	// Check March 10 has 1 stored event
	date10 := testDate(t, "2025-03-10")
	events10 := loaded.GetStoredByDate(date10)
	if len(events10) != 1 {
		t.Errorf("loaded March 10: got %d events, want 1", len(events10))
	}

	// Verify recurrence survived
	if len(events10) > 0 && events10[0].Recurrence != RecurDaily {
		t.Errorf("recurrence = %q, want %q", events10[0].Recurrence, RecurDaily)
	}
}

func TestLoadEvents_EmptyFile(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	// No file → empty store, no error
	store, errMsg := LoadEvents()
	if errMsg != "" {
		t.Fatalf("LoadEvents error for missing file: %s", errMsg)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestLoadEvents_MalformedJSON(t *testing.T) {
	dir, cleanup := tempDataDir(t)
	defer cleanup()

	// Write garbage
	os.WriteFile(filepath.Join(dir, "events.json"), []byte("not json"), 0644)

	store, errMsg := LoadEvents()
	if errMsg == "" {
		t.Error("expected error message for malformed JSON")
	}
	// Should still return a usable store
	if store == nil {
		t.Fatal("expected non-nil store even with malformed file")
	}
}

func TestSaveEvents_AtomicWrite(t *testing.T) {
	dir, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	ev := makeEvent("Atomic", "2025-03-15", 540, 600)
	store.Add(ev)

	if err := SaveEvents(store); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(filepath.Join(dir, "events.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("Unmarshal saved events: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("saved %d events, want 1", len(events))
	}

	// Verify no temp file left behind
	tmpFile := filepath.Join(dir, "events.json.tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}
}

func TestSaveEvents_EmptyStore(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	store := NewEventStore()
	if err := SaveEvents(store); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	loaded, errMsg := LoadEvents()
	if errMsg != "" {
		t.Fatalf("LoadEvents error: %s", errMsg)
	}
	// Empty store should save/load cleanly
	all := loaded.AllEvents()
	total := 0
	for _, v := range all {
		total += len(v)
	}
	if total != 0 {
		t.Errorf("expected 0 events in empty store, got %d", total)
	}
}

func TestLoadEvents_LegacyEventsWithoutID(t *testing.T) {
	dir, cleanup := tempDataDir(t)
	defer cleanup()

	// Write events without IDs (legacy format)
	legacy := `[{"title":"Old Event","date":"2025-03-15","start_min":540,"end_min":600}]`
	os.WriteFile(filepath.Join(dir, "events.json"), []byte(legacy), 0644)

	loaded, errMsg := LoadEvents()
	if errMsg != "" {
		t.Fatalf("LoadEvents error: %s", errMsg)
	}

	date := testDate(t, "2025-03-15")
	events := loaded.GetStoredByDate(date)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	// Should have been assigned an ID
	if events[0].ID == "" {
		t.Error("legacy event should have been assigned an ID")
	}
}

// ---------------------------------------------------------------------------
// Settings persistence
// ---------------------------------------------------------------------------

func TestSaveAndLoadSettings_RoundTrip(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	s := DefaultSettings()
	s.ZoomLevel = 15
	s.EventColor = "#ff0000"
	s.ShowBorders = false

	if err := SaveSettings(s); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	loaded, errMsg := LoadSettings()
	if errMsg != "" {
		t.Fatalf("LoadSettings error: %s", errMsg)
	}

	if loaded.ZoomLevel != 15 {
		t.Errorf("ZoomLevel = %d, want 15", loaded.ZoomLevel)
	}
	if loaded.EventColor != "#ff0000" {
		t.Errorf("EventColor = %q, want %q", loaded.EventColor, "#ff0000")
	}
	if loaded.ShowBorders != false {
		t.Error("ShowBorders should be false")
	}
}

func TestLoadSettings_Defaults(t *testing.T) {
	_, cleanup := tempDataDir(t)
	defer cleanup()

	// No file → default settings
	s, errMsg := LoadSettings()
	if errMsg != "" {
		t.Fatalf("LoadSettings error: %s", errMsg)
	}

	defaults := DefaultSettings()
	if s.ZoomLevel != defaults.ZoomLevel {
		t.Errorf("default ZoomLevel = %d, want %d", s.ZoomLevel, defaults.ZoomLevel)
	}
	if s.EventColor != defaults.EventColor {
		t.Errorf("default EventColor = %q, want %q", s.EventColor, defaults.EventColor)
	}
}

func TestLoadSettings_MalformedJSON(t *testing.T) {
	dir, cleanup := tempDataDir(t)
	defer cleanup()

	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("broken"), 0644)

	_, errMsg := LoadSettings()
	if errMsg == "" {
		t.Error("expected error message for malformed settings")
	}
}

func TestLoadSettings_InvalidZoomSnap(t *testing.T) {
	dir, cleanup := tempDataDir(t)
	defer cleanup()

	// Write settings with an invalid zoom level
	s := DefaultSettings()
	s.ZoomLevel = 12 // not in ZoomLevels, should snap to nearest
	data, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644)

	loaded, errMsg := LoadSettings()
	if errMsg != "" {
		t.Fatalf("LoadSettings error: %s", errMsg)
	}

	// Should snap to 5 (largest valid level <= 12)
	if loaded.ZoomLevel != 5 {
		t.Errorf("ZoomLevel snapped to %d, want 5", loaded.ZoomLevel)
	}
}

func TestLoadSettings_MergesDefaults(t *testing.T) {
	dir, cleanup := tempDataDir(t)
	defer cleanup()

	// Write minimal settings (missing many fields)
	minimal := `{"zoom_level": 15}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(minimal), 0644)

	loaded, errMsg := LoadSettings()
	if errMsg != "" {
		t.Fatalf("LoadSettings error: %s", errMsg)
	}

	// UIColors should be merged with defaults
	if loaded.UIColors == nil {
		t.Fatal("UIColors should not be nil")
	}
	if _, ok := loaded.UIColors["accent"]; !ok {
		t.Error("UIColors should include default 'accent'")
	}

	// Keybindings should be merged
	if loaded.Keybindings == nil {
		t.Fatal("Keybindings should not be nil")
	}
	if _, ok := loaded.Keybindings["h"]; !ok {
		t.Error("Keybindings should include default 'h'")
	}
}
