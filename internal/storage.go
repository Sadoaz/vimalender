package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DataDir returns the XDG-compliant data directory for vimalender.
func DataDir() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "vimalender")
}

// DataFilePath returns the full path to the events JSON file.
func DataFilePath() string {
	return filepath.Join(DataDir(), "events.json")
}

// SettingsFilePath returns the full path to the settings JSON file.
func SettingsFilePath() string {
	return filepath.Join(DataDir(), "settings.json")
}

// Settings holds persistent user preferences.
type Settings struct {
	ZoomLevel    int      `json:"zoom_level"`
	DayCount     int      `json:"day_count"`
	ShowHints    bool     `json:"show_hints"`
	JumpPercent  int      `json:"jump_percent"`   // j/k step as percentage of viewport (1-50)
	EventColors  []string `json:"event_colors"`   // hex colors for events, cycled by index
	ShowBorders  bool     `json:"show_borders"`   // show left color bar on events
	ShowDescs    bool     `json:"show_descs"`     // show event descriptions in grid
	RoundBorders bool     `json:"round_borders"`  // use rounded corners on events
	QuickCreate  bool     `json:"quick_create"`   // skip recurrence picker in create flow
	SkipDesc     bool     `json:"skip_desc"`      // skip description step in create flow
	DayStartHour int      `json:"day_start_hour"` // hour to start the day view (0-23)

	// Persisted position (restored on startup)
	LastDate      string `json:"last_date,omitempty"`       // window start date YYYY-MM-DD
	LastCursorCol int    `json:"last_cursor_col,omitempty"` // cursor column
	LastCursorMin int    `json:"last_cursor_min,omitempty"` // cursor minute offset
	LastViewport  int    `json:"last_viewport,omitempty"`   // viewport offset
}

// DefaultEventColors are the default palette for overlapping events.
var DefaultEventColors = []string{
	"#1a5fb4", // blue
	"#26a269", // green
	"#a51d2d", // red
	"#e5a50a", // yellow
	"#9141ac", // purple
	"#c64600", // orange
	"#218787", // teal
	"#813d9c", // violet
}

// DefaultSettings returns settings with default values.
func DefaultSettings() Settings {
	return Settings{
		ZoomLevel:    ZoomAuto,
		DayCount:     7,
		ShowHints:    true,
		JumpPercent:  5,
		EventColors:  DefaultEventColors,
		ShowBorders:  true,
		ShowDescs:    true,
		RoundBorders: false,
		QuickCreate:  false,
		SkipDesc:     false,
		DayStartHour: 0,
	}
}

// LoadSettings reads settings from the JSON file.
// Returns default settings and an error message if the file is malformed.
func LoadSettings() (Settings, string) {
	s := DefaultSettings()
	path := SettingsFilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, ""
		}
		return s, fmt.Sprintf("Failed to read %s: %v", path, err)
	}

	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), fmt.Sprintf("Malformed settings file: %v", err)
	}

	// Validate
	if s.DayCount < 1 {
		s.DayCount = 7
	}
	if s.JumpPercent < 1 || s.JumpPercent > 50 {
		s.JumpPercent = 5
	}
	if len(s.EventColors) == 0 {
		s.EventColors = DefaultEventColors
	}
	if s.DayStartHour < 0 || s.DayStartHour > 23 {
		s.DayStartHour = 8
	}

	return s, ""
}

// SaveSettings writes settings to the JSON file atomically.
func SaveSettings(s Settings) error {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	target := SettingsFilePath()
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// LoadEvents reads events from the JSON file.
// Returns an empty store and an error message if the file is malformed.
func LoadEvents() (*EventStore, string) {
	store := NewEventStore()
	path := DataFilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, ""
		}
		return store, fmt.Sprintf("Failed to read %s: %v", path, err)
	}

	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		return store, fmt.Sprintf("Malformed events file: %v", err)
	}

	for i := range events {
		// Parse date string to time.Time (in local timezone for consistent map keys)
		t, err := time.ParseInLocation("2006-01-02", events[i].DateStr, time.Local)
		if err != nil {
			continue // skip malformed entries
		}
		events[i].Date = t
		// Assign ID to legacy events without one
		if events[i].ID == "" {
			events[i].ID = GenerateID()
		}
		key := DateKey(t)
		store.events[key] = append(store.events[key], events[i])
	}

	return store, ""
}

// SaveEvents writes all events to the JSON file atomically.
func SaveEvents(store *EventStore) error {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Collect all events
	var all []Event
	for _, events := range store.events {
		for _, ev := range events {
			// Ensure DateStr is set
			ev.DateStr = ev.Date.Format("2006-01-02")
			all = append(all, ev)
		}
	}
	if all == nil {
		all = []Event{}
	}

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	// Atomic write: write to temp file, then rename
	target := DataFilePath()
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
