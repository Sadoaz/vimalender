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
	ZoomLevel      int               `json:"zoom_level"`
	DayCount       int               `json:"day_count"`
	ShowHints      bool              `json:"show_hints"`
	JumpPercent    int               `json:"jump_percent"` // j/k step as percentage of viewport (1-50)
	EventColor     string            `json:"event_color"`
	EventColors    []string          `json:"event_colors,omitempty"` // legacy palette field kept for backwards compatibility
	UIColors       map[string]string `json:"ui_colors,omitempty"`
	ShowBorders    bool              `json:"show_borders"`   // show left color bar on events
	ShowDescs      bool              `json:"show_descs"`     // show event descriptions in grid
	RoundBorders   bool              `json:"round_borders"`  // use rounded corners on events
	QuickCreate    bool              `json:"quick_create"`   // skip recurrence picker in create flow
	SkipDesc       bool              `json:"skip_desc"`      // skip description step in create flow
	DayStartHour   int               `json:"day_start_hour"` // hour to start the day view (0-23)
	Keybindings    map[string]string `json:"keybindings,omitempty"`
	KeybindingHelp map[string]string `json:"keybinding_help,omitempty"`
	EventColorHelp string            `json:"event_color_help,omitempty"`
	UIColorHelp    map[string]string `json:"ui_color_help,omitempty"`

	// Persisted position (restored on startup)
	LastDate      string `json:"last_date,omitempty"`       // window start date YYYY-MM-DD
	LastCursorCol int    `json:"last_cursor_col,omitempty"` // cursor column
	LastCursorMin int    `json:"last_cursor_min,omitempty"` // cursor minute offset
	LastViewport  int    `json:"last_viewport,omitempty"`   // viewport offset
}

const DefaultEventColor = "#1a5fb4"

// DefaultEventColors are kept for backwards compatibility with older settings files.
var DefaultEventColors = []string{
	DefaultEventColor, // blue
	"#26a269",         // green
	"#a51d2d",         // red
	"#e5a50a",         // yellow
	"#9141ac",         // purple
	"#c64600",         // orange
	"#218787",         // teal
	"#813d9c",         // violet
}

func DefaultUIColors() map[string]string {
	return map[string]string{
		"accent":            "#00a8ff",
		"header_accent":     "#00a8ff",
		"create_preview":    "#00a8ff",
		"event_bg":          "#1c1c2e",
		"status_bar_bg":     "236",
		"status_bar_fg":     "255",
		"hint_fg":           "243",
		"warning_fg":        "111",
		"help_border":       "#00a8ff",
		"help_section":      "#00a8ff",
		"help_selected_bg":  "236",
		"prompt_fg":         "39",
		"now_fg":            "#ff0000",
		"consecutive_color": "#26a269",
	}
}

func DefaultEventColorHelp() string {
	return "global color for all events; use a hex value like #1a5fb4"
}

func DefaultKeybindingHelp() map[string]string {
	return map[string]string{
		"0":      "reset to default zoom",
		"a":      "create event",
		"c":      "jump to now / today",
		"ctrl+d": "half-page down",
		"ctrl+n": "next search match",
		"ctrl+p": "previous search match",
		"ctrl+r": "redo",
		"ctrl+u": "half-page up",
		"d":      "delete in visual mode / dd sequence",
		"e":      "edit in editor",
		"enter":  "confirm current action",
		"=":      "reset to default zoom",
		"esc":    "cancel / back",
		"g":      "goto time",
		"h":      "move left",
		"j":      "move down",
		"k":      "move up",
		"l":      "move right",
		"m":      "move selected event or selection",
		"-":      "zoom out",
		"n":      "next search match",
		"p":      "paste",
		"+":      "zoom in",
		"q":      "quit / close",
		"?":      "open help",
		"r":      "cycle recurrence forward",
		"s":      "edit menu in move mode",
		"G":      "goto day",
		"H":      "previous overlapping event",
		"J":      "step one minute down",
		"K":      "step one minute up",
		"L":      "next overlapping event",
		"M":      "month view",
		"N":      "previous search match",
		"R":      "cycle recurrence backward",
		"S":      "settings view",
		"V":      "visual select",
		"Y":      "year view",
		"/":      "search",
		" ":      "space / confirm in menus",
		"tab":    "cycle overlapping events",
		"u":      "undo",
		"x":      "cut",
		"y":      "copy",
	}
}

func DefaultUIColorHelp() map[string]string {
	return map[string]string{
		"accent":            "primary blue accent used by status tags and headings",
		"header_accent":     "top calendar header accent color",
		"create_preview":    "create-preview border color",
		"event_bg":          "main event background fill color",
		"status_bar_bg":     "bottom bar background",
		"status_bar_fg":     "bottom bar text color",
		"hint_fg":           "secondary hint text color",
		"warning_fg":        "warning and error text color",
		"help_border":       "help popup border color",
		"help_section":      "help section heading color",
		"help_selected_bg":  "selected help row background",
		"prompt_fg":         "input prompt accent color",
		"now_fg":            "current time indicator color",
		"consecutive_color": "border color for back-to-back consecutive events",
	}
}

func mergeDefaults(dst, defaults map[string]string) map[string]string {
	if dst == nil {
		dst = map[string]string{}
	}
	for k, v := range defaults {
		if _, ok := dst[k]; !ok {
			dst[k] = v
		}
	}
	return dst
}

// DefaultSettings returns settings with default values.
func DefaultSettings() Settings {
	return Settings{
		ZoomLevel:      DefaultZoomLevel,
		DayCount:       7,
		ShowHints:      false,
		JumpPercent:    5,
		EventColor:     DefaultEventColor,
		EventColors:    nil,
		UIColors:       DefaultUIColors(),
		ShowBorders:    true,
		ShowDescs:      true,
		RoundBorders:   false,
		QuickCreate:    false,
		SkipDesc:       false,
		DayStartHour:   0,
		Keybindings:    DefaultKeybindings(),
		KeybindingHelp: DefaultKeybindingHelp(),
		EventColorHelp: DefaultEventColorHelp(),
		UIColorHelp:    DefaultUIColorHelp(),
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
	if s.ZoomLevel <= 0 || s.ZoomLevel < MinZoomLevel || s.ZoomLevel > MaxPreciseZoomLevel {
		s.ZoomLevel = DefaultZoomLevel
	}
	// Snap to nearest valid zoom level
	best := ZoomLevels[0]
	for _, level := range ZoomLevels {
		if level <= s.ZoomLevel {
			best = level
		}
	}
	s.ZoomLevel = best
	if s.JumpPercent < 1 || s.JumpPercent > 50 {
		s.JumpPercent = 5
	}
	if s.EventColor == "" {
		if len(s.EventColors) > 0 && s.EventColors[0] != "" {
			s.EventColor = s.EventColors[0]
		} else {
			s.EventColor = DefaultEventColor
		}
	}
	s.EventColors = nil
	s.UIColors = mergeDefaults(s.UIColors, DefaultUIColors())
	if s.DayStartHour < 0 || s.DayStartHour > 23 {
		s.DayStartHour = 8
	}
	s.Keybindings = mergeDefaults(s.Keybindings, DefaultKeybindings())
	s.KeybindingHelp = mergeDefaults(s.KeybindingHelp, DefaultKeybindingHelp())
	if s.EventColorHelp == "" {
		s.EventColorHelp = DefaultEventColorHelp()
	}
	s.UIColorHelp = mergeDefaults(s.UIColorHelp, DefaultUIColorHelp())

	return s, ""
}

// SaveSettings writes settings to the JSON file atomically.
func SaveSettings(s Settings) error {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	s.Keybindings = mergeDefaults(s.Keybindings, DefaultKeybindings())
	s.KeybindingHelp = mergeDefaults(s.KeybindingHelp, DefaultKeybindingHelp())
	if s.EventColor == "" {
		s.EventColor = DefaultEventColor
	}
	s.EventColors = nil
	if s.EventColorHelp == "" {
		s.EventColorHelp = DefaultEventColorHelp()
	}
	s.UIColors = mergeDefaults(s.UIColors, DefaultUIColors())
	s.UIColorHelp = mergeDefaults(s.UIColorHelp, DefaultUIColorHelp())

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
