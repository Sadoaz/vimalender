package internal

import "github.com/charmbracelet/lipgloss"

// Style definitions for the entire application.
var (
	// Column header
	ColumnHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Align(lipgloss.Center)

	SelectedColumnHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("39")).
					Align(lipgloss.Center)

	TodayColumnHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				Underline(true).
				Align(lipgloss.Center)

	SelectedTodayColumnHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("39")).
					Underline(true).
					Align(lipgloss.Center)

	// Time gutter
	TimeGutterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	// Cursor highlight
	CursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236"))

	// Event block
	EventBlockStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("25")).
			Foreground(lipgloss.Color("255"))

	// Event in adjustment mode (legacy fallback; active rendering is themed in grid.go)
	AdjustEventStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1c1c2e")).
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true)

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	StatusModeStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("39")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	StatusCreateModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	StatusAdjustModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	StatusHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	// Input prompt
	InputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	// Warning/error messages
	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("111"))

	// Small terminal message
	SmallTermStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	// Detail view
	DetailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("25")).
				Padding(0, 1)

	// Search highlight (applied on top of event's own style)
	SearchHighlightStyle = lipgloss.NewStyle().
				Bold(true).
				Underline(true)

	// Search mode status bar
	StatusSearchModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Visual mode status bar
	StatusVisualModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Goto mode status bar
	StatusGotoModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Detail mode status bar
	StatusDetailModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("25")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Month view styles
	MonthDayStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	MonthTodayStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	MonthSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("255"))

	MonthEventCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))

	MonthHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	// Month view mode style
	StatusMonthModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("135")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Year view mode style
	StatusYearModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("166")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Settings mode style
	StatusSettingsModeStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Current time line — red marker for "now" in the time gutter
	NowGutterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000")).
			Bold(true)

	// Current time line — red line across the day column for today
	NowLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000"))
)
