package internal

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	isatty "github.com/mattn/go-isatty"
)

// ---------------------------------------------------------------------------
// Flag parsing helpers
// ---------------------------------------------------------------------------

// errHelpShown is returned when --help was requested. The caller should
// return nil to exit 0 (the usage text was already printed by flag).
var errHelpShown = fmt.Errorf("")

// setUsage sets a custom usage function that shows double-dash flags,
// an optional positional argument hint, and an example.
func setUsage(fs *flag.FlagSet, positional, example string) {
	fs.Usage = func() {
		usage := "Usage: vimalender " + fs.Name()
		if positional != "" {
			usage += " [flags] " + positional
		} else {
			usage += " [flags]"
		}
		fmt.Fprintln(os.Stderr, usage)
		fmt.Fprintln(os.Stderr)
		fs.VisitAll(func(f *flag.Flag) {
			def := ""
			if f.DefValue != "" && f.DefValue != "false" {
				def = " (default: " + f.DefValue + ")"
			}
			fmt.Fprintf(os.Stderr, "  --%s\t%s%s\n", f.Name, f.Usage, def)
		})
		if example != "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintf(os.Stderr, "Example: %s\n", example)
		}
	}
}

// parseFlags parses a FlagSet. Returns errHelpShown if --help was requested.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return errHelpShown
		}
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// TTY detection & style setup
// ---------------------------------------------------------------------------

// isTTY returns true when stdout is an interactive terminal.
func isTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// cliStyles holds pre-built Lip Gloss renderers for CLI output.
// When stdout is not a TTY the styles produce plain text (no ANSI).
type cliStyles struct {
	title  lipgloss.Style
	time   lipgloss.Style
	dimID  lipgloss.Style
	header lipgloss.Style
	recur  lipgloss.Style
}

func newCLIStyles() cliStyles {
	tty := isTTY()
	if !tty {
		// No-op styles when piped.
		plain := lipgloss.NewStyle()
		return cliStyles{
			title:  plain,
			time:   plain,
			dimID:  plain,
			header: plain,
			recur:  plain,
		}
	}
	return cliStyles{
		title:  lipgloss.NewStyle().Bold(true),
		time:   lipgloss.NewStyle().Foreground(lipgloss.Color("#00a8ff")),
		dimID:  lipgloss.NewStyle().Faint(true),
		header: lipgloss.NewStyle().Bold(true).Underline(true),
		recur:  lipgloss.NewStyle().Foreground(lipgloss.Color("#9141ac")),
	}
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

// cliError writes a formatted error to stderr.
func cliError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
}

// writeJSON writes v as a single JSON line to stdout.
func writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}

// cliEventJSON is the JSON representation for CLI output.
type cliEventJSON struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	StartMin   int    `json:"start_min"`
	EndMin     int    `json:"end_min"`
	Recurrence string `json:"recurrence,omitempty"`
	Desc       string `json:"desc,omitempty"`
}

func eventToJSON(ev Event) cliEventJSON {
	return cliEventJSON{
		ID:         ev.ID,
		Title:      ev.Title,
		Date:       ev.DateStr,
		StartMin:   ev.StartMin,
		EndMin:     ev.EndMin,
		Recurrence: ev.Recurrence,
		Desc:       ev.Desc,
	}
}

// printStyledEvent prints a single event line with Lip Gloss styling.
func printStyledEvent(s cliStyles, ev Event) {
	timeStr := s.time.Render(ev.StartTime() + "-" + ev.EndTime())
	title := s.title.Render(ev.Title)
	id := s.dimID.Render(ev.ID)
	parts := []string{timeStr, " ", title, " ", id}
	if ev.Recurrence != "" && ev.Recurrence != RecurNone {
		parts = append(parts, " ", s.recur.Render("["+RecurrenceLabel(ev.Recurrence)+"]"))
	}
	fmt.Fprintln(os.Stdout, strings.Join(parts, ""))
}

// ---------------------------------------------------------------------------
// parseTimeFlag - reuses the existing parseTime from editor.go
// ---------------------------------------------------------------------------

// parseTimeFlag parses a time flag value ("HH:MM", "9", "930", "1430") into
// minutes since midnight. It delegates to the existing parseTime function.
func parseTimeFlag(val string) (int, error) {
	return parseTime(val)
}

// parseDate parses a "YYYY-MM-DD" string into a local time.Time.
func parseDate(val string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", val, time.Local)
}

// ---------------------------------------------------------------------------
// loadStoreOrDie loads the event store, reporting any warning to stderr.
// ---------------------------------------------------------------------------

func loadStoreOrDie() (*EventStore, error) {
	store, warn := LoadEvents()
	if warn != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
	}
	return store, nil
}

// ---------------------------------------------------------------------------
// RunList
// ---------------------------------------------------------------------------

// RunList implements `vimalender list`.
func RunList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	setUsage(fs, "", "vimalender list --from 2025-03-10 --to 2025-03-16")
	dateFlag := fs.String("date", "", "date to list events for (YYYY-MM-DD, default: today)")
	fromFlag := fs.String("from", "", "range start date (YYYY-MM-DD)")
	toFlag := fs.String("to", "", "range end date (YYYY-MM-DD)")
	jsonFlag := fs.Bool("json", false, "output as NDJSON")
	if err := parseFlags(fs, args); err != nil {
		if err == errHelpShown {
			return nil
		}
		return err
	}

	store, err := loadStoreOrDie()
	if err != nil {
		return err
	}

	var dates []time.Time
	switch {
	case *fromFlag != "" || *toFlag != "":
		from := time.Now()
		to := time.Now()
		if *fromFlag != "" {
			from, err = parseDate(*fromFlag)
			if err != nil {
				return fmt.Errorf("invalid --from date: %w", err)
			}
		}
		if *toFlag != "" {
			to, err = parseDate(*toFlag)
			if err != nil {
				return fmt.Errorf("invalid --to date: %w", err)
			}
		}
		from = DateKey(from)
		to = DateKey(to)
		if to.Before(from) {
			return fmt.Errorf("--to must not be before --from")
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			dates = append(dates, d)
		}
	case *dateFlag != "":
		d, err := parseDate(*dateFlag)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
		dates = []time.Time{DateKey(d)}
	default:
		dates = []time.Time{DateKey(time.Now())}
	}

	s := newCLIStyles()
	for i, date := range dates {
		events := store.GetByDate(date)
		// Sort by start time for human-friendly output.
		sort.Slice(events, func(a, b int) bool {
			if events[a].StartMin != events[b].StartMin {
				return events[a].StartMin < events[b].StartMin
			}
			return events[a].ID < events[b].ID
		})

		if *jsonFlag {
			for _, ev := range events {
				ej := eventToJSON(ev)
				ej.Date = date.Format("2006-01-02")
				if err := writeJSON(ej); err != nil {
					return err
				}
			}
		} else {
			if len(dates) > 1 || len(dates) == 1 {
				if i > 0 {
					fmt.Fprintln(os.Stdout)
				}
				fmt.Fprintln(os.Stdout, s.header.Render(date.Format("Mon 2006-01-02")))
			}
			if len(events) == 0 {
				fmt.Fprintln(os.Stdout, "  (no events)")
			}
			for _, ev := range events {
				fmt.Fprint(os.Stdout, "  ")
				printStyledEvent(s, ev)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// RunSearch
// ---------------------------------------------------------------------------

// RunSearch implements `vimalender search <query>`.
func RunSearch(args []string) error {
	// Extract flags manually so --json works before or after the query.
	useJSON := false
	var rest []string
	for _, a := range args {
		switch a {
		case "--json", "-json":
			useJSON = true
		case "--help", "-help", "-h":
			fmt.Fprintln(os.Stderr, "Usage: vimalender search [--json] <query>")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "  --json\toutput as NDJSON")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Example: vimalender search \"standup\"")
			return nil
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) < 1 {
		return fmt.Errorf("usage: vimalender search <query> [--json]\n  e.g. vimalender search \"standup\"")
	}
	query := strings.Join(rest, " ")

	store, err := loadStoreOrDie()
	if err != nil {
		return err
	}

	matches := SearchEvents(store, query)
	if useJSON {
		for _, m := range matches {
			events := store.GetByDate(m.Date)
			for _, ev := range events {
				if ev.ID == m.EventID {
					if err := writeJSON(eventToJSON(ev)); err != nil {
						return err
					}
					break
				}
			}
		}
	} else {
		s := newCLIStyles()
		for _, m := range matches {
			events := store.GetByDate(m.Date)
			for _, ev := range events {
				if ev.ID == m.EventID {
					printStyledEvent(s, ev)
					break
				}
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// RunAdd
// ---------------------------------------------------------------------------

// RunAdd implements `vimalender add`.
func RunAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	setUsage(fs, "", "vimalender add --title \"Standup\" --date 2025-03-15 --start 9:00 --end 9:30")
	title := fs.String("title", "", "event title (required)")
	dateStr := fs.String("date", "", "date in YYYY-MM-DD (required)")
	startStr := fs.String("start", "", "start time, e.g. 9:00 or 930 (required)")
	endStr := fs.String("end", "", "end time, e.g. 10:00 or 1030 (required)")
	recurrence := fs.String("recurrence", "", "recurrence pattern: daily, weekdays, weekly, biweekly, monthly, yearly")
	desc := fs.String("desc", "", "event description")
	if err := parseFlags(fs, args); err != nil {
		if err == errHelpShown {
			return nil
		}
		return err
	}

	// Validate required flags.
	var missing []string
	if *title == "" {
		missing = append(missing, "--title")
	}
	if *dateStr == "" {
		missing = append(missing, "--date")
	}
	if *startStr == "" {
		missing = append(missing, "--start")
	}
	if *endStr == "" {
		missing = append(missing, "--end")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s\n  e.g. vimalender add --title \"Standup\" --date 2025-03-15 --start 9:00 --end 9:30", strings.Join(missing, ", "))
	}

	date, err := parseDate(*dateStr)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}
	startMin, err := parseTimeFlag(*startStr)
	if err != nil {
		return fmt.Errorf("invalid --start: %w", err)
	}
	endMin, err := parseTimeFlag(*endStr)
	if err != nil {
		return fmt.Errorf("invalid --end: %w", err)
	}

	// Validate recurrence if provided.
	rec := ""
	if *recurrence != "" {
		rec = strings.ToLower(*recurrence)
		valid := false
		for _, opt := range RecurrenceOptions {
			if rec == opt {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid --recurrence %q; valid options: daily, weekdays, weekly, biweekly, monthly, yearly", *recurrence)
		}
	}

	store, err := loadStoreOrDie()
	if err != nil {
		return err
	}

	ev := Event{
		Title:      *title,
		Desc:       *desc,
		Date:       DateKey(date),
		DateStr:    DateKey(date).Format("2006-01-02"),
		StartMin:   startMin,
		EndMin:     endMin,
		Recurrence: rec,
	}

	// Use AddSpanningEvent if event crosses midnight.
	if endMin > MinutesPerDay {
		if err := store.AddSpanningEvent(ev); err != nil {
			return fmt.Errorf("add event: %w", err)
		}
	} else {
		if err := store.Add(ev); err != nil {
			return fmt.Errorf("add event: %w", err)
		}
	}

	if err := SaveEvents(store); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	// Find the event we just added to print its ID.
	// For spanning events, the first segment on the original date has the ID.
	events := store.GetStoredByDate(DateKey(date))
	if len(events) > 0 {
		last := events[len(events)-1]
		fmt.Fprintln(os.Stdout, last.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// RunDelete
// ---------------------------------------------------------------------------

// RunDelete implements `vimalender delete <id>`.
func RunDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	setUsage(fs, "<event-id>", "vimalender delete abc123-def456")
	if err := parseFlags(fs, args); err != nil {
		if err == errHelpShown {
			return nil
		}
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: vimalender delete <event-id>\n  e.g. vimalender delete abc123-def456")
	}
	id := fs.Arg(0)

	store, err := loadStoreOrDie()
	if err != nil {
		return err
	}

	if err := store.DeleteByID(id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if err := SaveEvents(store); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	fmt.Fprintln(os.Stdout, id)
	return nil
}

// ---------------------------------------------------------------------------
// RunImport
// ---------------------------------------------------------------------------

// RunImport implements `vimalender import <file.ics>`.
func RunImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	setUsage(fs, "<file.ics>", "vimalender import ~/calendar.ics")
	if err := parseFlags(fs, args); err != nil {
		if err == errHelpShown {
			return nil
		}
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: vimalender import <file.ics>\n  e.g. vimalender import ~/calendar.ics")
	}
	path := fs.Arg(0)

	// Validate extension before anything else.
	if strings.ToLower(filepath.Ext(path)) != ".ics" {
		return fmt.Errorf("file must end with .ics")
	}

	result, err := ImportICSFile(path)
	if err != nil {
		return err
	}

	// Report skipped events to stderr.
	for _, issue := range result.Skipped {
		name := issue.Title
		if name == "" {
			name = "(unknown)"
		}
		fmt.Fprintf(os.Stderr, "skipped: %s — %s\n", name, issue.Reason)
	}

	// Merge imported events into store.
	store, err := loadStoreOrDie()
	if err != nil {
		return err
	}

	added := 0
	for _, ev := range result.Events {
		ev.ID = GenerateID()
		if err := store.Add(ev); err != nil {
			fmt.Fprintf(os.Stderr, "skipped: %s — %v\n", ev.Title, err)
			continue
		}
		added++
	}

	if err := SaveEvents(store); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	fmt.Fprintf(os.Stdout, "imported %d, skipped %d\n", added, len(result.Skipped)+(result.Imported-added))
	return nil
}

// ---------------------------------------------------------------------------
// RunExport
// ---------------------------------------------------------------------------

// RunExport implements `vimalender export <file.ics>`.
func RunExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	setUsage(fs, "<file.ics>", "vimalender export ~/backup.ics")
	if err := parseFlags(fs, args); err != nil {
		if err == errHelpShown {
			return nil
		}
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: vimalender export <file.ics>\n  e.g. vimalender export ~/backup.ics")
	}
	path := fs.Arg(0)

	store, err := loadStoreOrDie()
	if err != nil {
		return err
	}

	result, err := ExportICSFile(path, store)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "exported %d events to %s\n", result.Exported, result.OutputPath)
	return nil
}
