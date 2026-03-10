package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ImportIssue struct {
	Title  string
	Reason string
}

type ImportResult struct {
	Events         []Event
	Imported       int
	ImportedAdded  int
	Skipped        []ImportIssue
	SourceFilePath string
}

type icsProperty struct {
	Name   string
	Params map[string]string
	Value  string
}

type icsEvent struct {
	Properties map[string][]icsProperty
	ParseErr   error
}

func ImportICSFile(path string) (ImportResult, error) {
	resolved, err := resolveImportPath(path)
	if err != nil {
		return ImportResult{}, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return ImportResult{}, fmt.Errorf("read %s: %w", resolved, err)
	}
	parsed, err := parseICSCalendar(string(data))
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{SourceFilePath: resolved}
	for _, raw := range parsed {
		ev, issue := translateICSEvent(raw)
		if issue != nil {
			result.Skipped = append(result.Skipped, *issue)
			continue
		}
		result.Events = append(result.Events, ev)
	}
	result.Imported = len(result.Events)
	sort.Slice(result.Events, func(i, j int) bool {
		if !DateKey(result.Events[i].Date).Equal(DateKey(result.Events[j].Date)) {
			return DateKey(result.Events[i].Date).Before(DateKey(result.Events[j].Date))
		}
		if result.Events[i].StartMin != result.Events[j].StartMin {
			return result.Events[i].StartMin < result.Events[j].StartMin
		}
		return result.Events[i].Title < result.Events[j].Title
	})
	return result, nil
}

func resolveImportPath(path string) (string, error) {
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
		return "", fmt.Errorf("file must end with .ics")
	}
	return filepath.Clean(path), nil
}

func parseICSCalendar(data string) ([]icsEvent, error) {
	lines := unfoldICSLines(data)
	var events []icsEvent
	inCalendar := false
	inEvent := false
	nestedDepth := 0
	current := icsEvent{Properties: map[string][]icsProperty{}}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		switch upper {
		case "BEGIN:VCALENDAR":
			inCalendar = true
			continue
		case "END:VCALENDAR":
			inCalendar = false
			continue
		case "BEGIN:VEVENT":
			if !inCalendar {
				return nil, fmt.Errorf("invalid .ics data: VEVENT outside VCALENDAR")
			}
			inEvent = true
			nestedDepth = 0
			current = icsEvent{Properties: map[string][]icsProperty{}}
			continue
		case "END:VEVENT":
			if inEvent {
				events = append(events, current)
				current = icsEvent{Properties: map[string][]icsProperty{}}
				inEvent = false
			}
			continue
		}
		if !inEvent {
			continue
		}
		if strings.HasPrefix(upper, "BEGIN:") {
			nestedDepth++
			continue
		}
		if strings.HasPrefix(upper, "END:") {
			if nestedDepth > 0 {
				nestedDepth--
			}
			continue
		}
		if nestedDepth > 0 {
			continue
		}
		prop, err := parseICSProperty(line)
		if err != nil {
			if current.ParseErr == nil {
				current.ParseErr = err
			}
			continue
		}
		current.Properties[prop.Name] = append(current.Properties[prop.Name], prop)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no VEVENT entries found")
	}
	return events, nil
}

func unfoldICSLines(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	raw := strings.Split(data, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func parseICSProperty(line string) (icsProperty, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return icsProperty{}, fmt.Errorf("invalid .ics line: %q", line)
	}
	left := parts[0]
	value := unescapeICSValue(parts[1])
	segments := strings.Split(left, ";")
	prop := icsProperty{
		Name:   strings.ToUpper(strings.TrimSpace(segments[0])),
		Params: map[string]string{},
		Value:  value,
	}
	for _, segment := range segments[1:] {
		kv := strings.SplitN(segment, "=", 2)
		if len(kv) != 2 {
			continue
		}
		prop.Params[strings.ToUpper(strings.TrimSpace(kv[0]))] = strings.Trim(strings.TrimSpace(kv[1]), "\"")
	}
	return prop, nil
}

func unescapeICSValue(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\n`, "\n",
		`\N`, "\n",
		`\,`, ",",
		`\;`, ";",
	)
	return replacer.Replace(value)
}

func translateICSEvent(raw icsEvent) (Event, *ImportIssue) {
	if raw.ParseErr != nil {
		return Event{}, &ImportIssue{Reason: raw.ParseErr.Error()}
	}
	title := firstICSValue(raw, "SUMMARY")
	if title == "" {
		title = "Untitled"
	}
	dtStartProp, ok := firstICSProperty(raw, "DTSTART")
	if !ok {
		return Event{}, &ImportIssue{Title: title, Reason: "missing DTSTART"}
	}
	start, startIsDate, err := parseICSDateValue(dtStartProp)
	if err != nil {
		return Event{}, &ImportIssue{Title: title, Reason: fmt.Sprintf("invalid DTSTART: %v", err)}
	}
	if len(raw.Properties["EXDATE"]) > 0 {
		return Event{}, &ImportIssue{Title: title, Reason: "EXDATE is not supported"}
	}

	end, duration, err := parseICSEnd(raw, start, startIsDate)
	if err != nil {
		return Event{}, &ImportIssue{Title: title, Reason: err.Error()}
	}
	if !end.After(start) || duration <= 0 {
		return Event{}, &ImportIssue{Title: title, Reason: "event end must be after start"}
	}

	recurrence, until, recurIssue := parseICSRecurrence(raw, start)
	if recurIssue != nil {
		return Event{}, &ImportIssue{Title: title, Reason: recurIssue.Error()}
	}

	date := DateKey(start.In(time.Local))
	startMin := start.In(time.Local).Hour()*60 + start.In(time.Local).Minute()
	endLocal := end.In(time.Local)
	daySpan := int(DateKey(endLocal).Sub(date).Hours() / 24)
	endMin := daySpan*MinutesPerDay + endLocal.Hour()*60 + endLocal.Minute()
	if startIsDate {
		startMin = 0
		endMin = int(duration.Minutes())
	}

	notes := firstICSValue(raw, "COMMENT")
	if notes == "" {
		notes = firstICSValue(raw, "X-ALT-DESC")
	}

	return Event{
		Title:         title,
		Desc:          firstICSValue(raw, "DESCRIPTION"),
		Notes:         notes,
		Date:          date,
		DateStr:       date.Format("2006-01-02"),
		StartMin:      startMin,
		EndMin:        endMin,
		Recurrence:    recurrence,
		RecurUntilStr: until,
	}, nil
}

func parseICSEnd(raw icsEvent, start time.Time, startIsDate bool) (time.Time, time.Duration, error) {
	if prop, ok := firstICSProperty(raw, "DTEND"); ok {
		end, endIsDate, err := parseICSDateValue(prop)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("invalid DTEND: %w", err)
		}
		if endIsDate != startIsDate {
			return time.Time{}, 0, fmt.Errorf("DTSTART and DTEND must use the same date type")
		}
		return end, end.Sub(start), nil
	}
	if startIsDate {
		end := start.Add(24 * time.Hour)
		return end, end.Sub(start), nil
	}
	return time.Time{}, 0, fmt.Errorf("missing DTEND")
}

func parseICSRecurrence(raw icsEvent, start time.Time) (string, string, error) {
	for _, propName := range []string{"EXRULE", "RDATE", "RECURRENCE-ID"} {
		if len(raw.Properties[propName]) > 0 {
			return "", "", fmt.Errorf("%s is not supported", propName)
		}
	}
	rrules := raw.Properties["RRULE"]
	if len(rrules) == 0 {
		return RecurNone, "", nil
	}
	if len(rrules) > 1 {
		return "", "", fmt.Errorf("multiple RRULE entries are not supported")
	}
	rule := parseRuleMap(rrules[0].Value)
	if len(rule) == 0 {
		return "", "", fmt.Errorf("invalid RRULE")
	}
	for key := range rule {
		switch key {
		case "FREQ", "INTERVAL", "UNTIL", "BYDAY", "BYMONTHDAY", "BYMONTH", "WKST":
		default:
			return "", "", fmt.Errorf("RRULE field %s is not supported", key)
		}
	}
	if _, ok := rule["COUNT"]; ok {
		return "", "", fmt.Errorf("RRULE COUNT is not supported")
	}
	interval := 1
	if rawInterval, ok := rule["INTERVAL"]; ok && rawInterval != "" {
		value, err := strconv.Atoi(rawInterval)
		if err != nil || value <= 0 {
			return "", "", fmt.Errorf("invalid RRULE INTERVAL")
		}
		interval = value
	}
	until := ""
	if rawUntil, ok := rule["UNTIL"]; ok && rawUntil != "" {
		parsedUntil, _, err := parseICSTimestamp(rawUntil, "")
		if err != nil {
			return "", "", fmt.Errorf("invalid RRULE UNTIL")
		}
		until = DateKey(parsedUntil.In(time.Local)).Format("2006-01-02")
	}

	startLocal := start.In(time.Local)
	weekdayCode := weekdayToICS(startLocal.Weekday())
	byDay := strings.ToUpper(strings.TrimSpace(rule["BYDAY"]))
	switch strings.ToUpper(strings.TrimSpace(rule["FREQ"])) {
	case "DAILY":
		if interval != 1 {
			return "", "", fmt.Errorf("daily RRULE interval must be 1")
		}
		if byDay == "" {
			return RecurDaily, until, nil
		}
		if byDay == "MO,TU,WE,TH,FR" {
			return RecurWeekdays, until, nil
		}
		return "", "", fmt.Errorf("daily RRULE BYDAY is not supported")
	case "WEEKLY":
		if byDay != "" && byDay != weekdayCode {
			return "", "", fmt.Errorf("weekly RRULE BYDAY must match DTSTART weekday")
		}
		switch interval {
		case 1:
			return RecurWeekly, until, nil
		case 2:
			return RecurBiweekly, until, nil
		default:
			return "", "", fmt.Errorf("weekly RRULE interval must be 1 or 2")
		}
	case "MONTHLY":
		if interval != 1 {
			return "", "", fmt.Errorf("monthly RRULE interval must be 1")
		}
		if byDay != "" {
			return "", "", fmt.Errorf("monthly RRULE BYDAY is not supported")
		}
		if byMonthDay := strings.TrimSpace(rule["BYMONTHDAY"]); byMonthDay != "" && byMonthDay != strconv.Itoa(startLocal.Day()) {
			return "", "", fmt.Errorf("monthly RRULE BYMONTHDAY must match DTSTART day")
		}
		return RecurMonthly, until, nil
	case "YEARLY":
		if interval != 1 {
			return "", "", fmt.Errorf("yearly RRULE interval must be 1")
		}
		if byDay != "" {
			return "", "", fmt.Errorf("yearly RRULE BYDAY is not supported")
		}
		if byMonth := strings.TrimSpace(rule["BYMONTH"]); byMonth != "" && byMonth != strconv.Itoa(int(startLocal.Month())) {
			return "", "", fmt.Errorf("yearly RRULE BYMONTH must match DTSTART month")
		}
		if byMonthDay := strings.TrimSpace(rule["BYMONTHDAY"]); byMonthDay != "" && byMonthDay != strconv.Itoa(startLocal.Day()) {
			return "", "", fmt.Errorf("yearly RRULE BYMONTHDAY must match DTSTART day")
		}
		return RecurYearly, until, nil
	default:
		return "", "", fmt.Errorf("RRULE frequency is not supported")
	}
}

func parseRuleMap(value string) map[string]string {
	parts := strings.Split(value, ";")
	rule := make(map[string]string, len(parts))
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		rule[strings.ToUpper(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
	}
	return rule
}

func parseICSDateValue(prop icsProperty) (time.Time, bool, error) {
	valueType := strings.ToUpper(strings.TrimSpace(prop.Params["VALUE"]))
	if valueType == "DATE" {
		t, err := time.ParseInLocation("20060102", prop.Value, time.Local)
		if err != nil {
			return time.Time{}, true, err
		}
		return t, true, nil
	}
	locName := strings.TrimSpace(prop.Params["TZID"])
	t, isDateOnly, err := parseICSTimestamp(prop.Value, locName)
	return t, isDateOnly, err
}

func parseICSTimestamp(value, tzid string) (time.Time, bool, error) {
	if len(value) == len("20060102") && !strings.Contains(value, "T") {
		t, err := time.ParseInLocation("20060102", value, time.Local)
		return t, true, err
	}
	formats := []string{"20060102T150405Z", "20060102T1504Z", "20060102T150405", "20060102T1504"}
	if strings.HasSuffix(value, "Z") {
		for _, format := range formats[:2] {
			if t, err := time.Parse(format, value); err == nil {
				return t, false, nil
			}
		}
		return time.Time{}, false, fmt.Errorf("unsupported UTC timestamp %q", value)
	}
	loc := time.Local
	if tzid != "" {
		loaded, err := time.LoadLocation(tzid)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("unknown timezone %q", tzid)
		}
		loc = loaded
	}
	for _, format := range formats[2:] {
		if t, err := time.ParseInLocation(format, value, loc); err == nil {
			return t, false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("unsupported timestamp %q", value)
}

func firstICSProperty(raw icsEvent, name string) (icsProperty, bool) {
	props := raw.Properties[strings.ToUpper(name)]
	if len(props) == 0 {
		return icsProperty{}, false
	}
	return props[0], true
}

func firstICSValue(raw icsEvent, name string) string {
	prop, ok := firstICSProperty(raw, name)
	if !ok {
		return ""
	}
	return prop.Value
}

func weekdayToICS(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "MO"
	case time.Tuesday:
		return "TU"
	case time.Wednesday:
		return "WE"
	case time.Thursday:
		return "TH"
	case time.Friday:
		return "FR"
	case time.Saturday:
		return "SA"
	default:
		return "SU"
	}
}
