package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ExportResult struct {
	OutputPath string
	Exported   int
}

func ExportICSFile(path string, store *EventStore) (ExportResult, error) {
	resolved, err := resolveExportPath(path)
	if err != nil {
		return ExportResult{}, err
	}
	content, count := buildICSExport(store)
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("create export directory: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return ExportResult{}, fmt.Errorf("write %s: %w", resolved, err)
	}
	return ExportResult{OutputPath: resolved, Exported: count}, nil
}

func resolveExportPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("enter an .ics file path")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
	}
	if strings.ToLower(filepath.Ext(path)) != ".ics" {
		path += ".ics"
	}
	return filepath.Clean(path), nil
}

func buildICSExport(store *EventStore) (string, int) {
	events := exportedStoredEvents(store)
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//vimalender//EN",
		"CALSCALE:GREGORIAN",
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, ev := range events {
		lines = append(lines, exportEventLines(ev, stamp)...)
	}
	lines = append(lines, "END:VCALENDAR", "")
	return strings.Join(lines, "\r\n"), len(events)
}

func exportedStoredEvents(store *EventStore) []Event {
	var dates []time.Time
	for date := range store.events {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	var events []Event
	for _, date := range dates {
		stored := append([]Event(nil), store.events[date]...)
		sort.Slice(stored, func(i, j int) bool {
			if stored[i].StartMin != stored[j].StartMin {
				return stored[i].StartMin < stored[j].StartMin
			}
			return stored[i].Title < stored[j].Title
		})
		events = append(events, stored...)
	}
	return events
}

func exportEventLines(ev Event, stamp string) []string {
	uid := ev.ID
	if uid == "" {
		uid = GenerateID()
	}
	uid += "@vimalender"
	lines := []string{"BEGIN:VEVENT", "UID:" + foldICSLine(uid), "DTSTAMP:" + stamp}
	for _, line := range exportDateLines(ev) {
		lines = append(lines, line)
	}
	lines = append(lines, foldProperty("SUMMARY", ev.Title))
	if ev.Desc != "" {
		lines = append(lines, foldProperty("DESCRIPTION", ev.Desc))
	}
	if ev.Notes != "" {
		lines = append(lines, foldProperty("COMMENT", ev.Notes))
	}
	if rule := exportRecurrenceRule(ev); rule != "" {
		lines = append(lines, "RRULE:"+rule)
	}
	for _, exc := range ev.ExceptionDates {
		if excDate, err := time.Parse("2006-01-02", exc); err == nil {
			excDT := time.Date(excDate.Year(), excDate.Month(), excDate.Day(), ev.StartMin/60, ev.StartMin%60, 0, 0, time.Local)
			lines = append(lines, "EXDATE:"+excDT.Format("20060102T150405"))
		}
	}
	lines = append(lines, "END:VEVENT")
	return lines
}

func exportDateLines(ev Event) []string {
	date := DateKey(ev.Date)
	duration := ev.EndMin - ev.StartMin
	if isAllDayExport(ev) {
		start := date.Format("20060102")
		end := date.AddDate(0, 0, duration/MinutesPerDay).Format("20060102")
		return []string{
			"DTSTART;VALUE=DATE:" + start,
			"DTEND;VALUE=DATE:" + end,
		}
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), ev.StartMin/60, ev.StartMin%60, 0, 0, time.Local)
	end := start.Add(time.Duration(duration) * time.Minute)
	return []string{
		"DTSTART:" + start.Format("20060102T150405"),
		"DTEND:" + end.Format("20060102T150405"),
	}
}

func isAllDayExport(ev Event) bool {
	duration := ev.EndMin - ev.StartMin
	return ev.StartMin == 0 && duration >= MinutesPerDay && duration%MinutesPerDay == 0
}

func exportRecurrenceRule(ev Event) string {
	parts := []string{}
	switch ev.Recurrence {
	case RecurDaily:
		parts = append(parts, "FREQ=DAILY")
	case RecurWeekdays:
		parts = append(parts, "FREQ=DAILY", "BYDAY=MO,TU,WE,TH,FR")
	case RecurWeekly:
		parts = append(parts, "FREQ=WEEKLY", "BYDAY="+weekdayToICS(ev.Date.Weekday()))
	case RecurBiweekly:
		parts = append(parts, "FREQ=WEEKLY", "INTERVAL=2", "BYDAY="+weekdayToICS(ev.Date.Weekday()))
	case RecurMonthly:
		parts = append(parts, "FREQ=MONTHLY", fmt.Sprintf("BYMONTHDAY=%d", ev.Date.Day()))
	case RecurYearly:
		parts = append(parts, "FREQ=YEARLY", fmt.Sprintf("BYMONTH=%d", int(ev.Date.Month())), fmt.Sprintf("BYMONTHDAY=%d", ev.Date.Day()))
	default:
		return ""
	}
	if ev.RecurUntilStr != "" {
		if until, err := time.Parse("2006-01-02", ev.RecurUntilStr); err == nil {
			if isAllDayExport(ev) {
				parts = append(parts, "UNTIL="+until.Format("20060102"))
			} else {
				untilDT := time.Date(until.Year(), until.Month(), until.Day(), ev.StartMin/60, ev.StartMin%60, 0, 0, time.Local)
				parts = append(parts, "UNTIL="+untilDT.Format("20060102T150405"))
			}
		}
	}
	return strings.Join(parts, ";")
}

func foldProperty(name, value string) string {
	return foldICSLine(name + ":" + escapeICSValue(value))
}

func foldICSLine(line string) string {
	const limit = 75
	runes := []rune(line)
	if len(runes) <= limit {
		return line
	}
	var parts []string
	for len(runes) > limit {
		parts = append(parts, string(runes[:limit]))
		runes = runes[limit:]
	}
	parts = append(parts, string(runes))
	return strings.Join(parts, "\r\n ")
}

func escapeICSValue(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\n", "\\n",
		",", "\\,",
		";", "\\;",
	)
	return replacer.Replace(value)
}
