package internal

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// editorResultMsg is sent when the external editor finishes.
type editorResultMsg struct {
	date       time.Time
	index      int
	title      string
	desc       string
	startMin   int
	endMin     int
	notes      string
	recurrence string
	recurUntil string
	err        error
}

// openEditor returns a tea.Cmd that opens $EDITOR with event details.
func (m *Model) openEditor(date time.Time, index int) tea.Cmd {
	events := m.store.GetByDate(date)
	if index < 0 || index >= len(events) {
		return nil
	}
	ev := events[index]

	// Write temp file
	content := formatEventForEditor(ev)
	notesLine := notesCursorLine(content)
	tmpFile, err := os.CreateTemp("", "vimalender-*.txt")
	if err != nil {
		m.statusMsg = fmt.Sprintf("Editor error: %v", err)
		return nil
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		m.statusMsg = fmt.Sprintf("Editor error: %v", err)
		return nil
	}
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	editorFields := strings.Fields(editor)
	if len(editorFields) == 0 {
		editorFields = []string{"vi"}
	}
	cmdName := editorFields[0]
	args := append([]string{}, editorFields[1:]...)
	base := strings.ToLower(cmdName)
	if strings.Contains(base, "vim") || strings.Contains(base, "vi") || strings.Contains(base, "nvim") || strings.Contains(base, "hx") || strings.Contains(base, "helix") {
		args = append(args, fmt.Sprintf("+%d", notesLine))
	}
	args = append(args, tmpPath)
	c := exec.Command(cmdName, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return editorResultMsg{date: date, index: index, err: err}
		}
		title, desc, startMin, endMin, notes, recurrence, recurUntil, parseErr := parseEditorResult(tmpPath)
		return editorResultMsg{
			date:       date,
			index:      index,
			title:      title,
			desc:       desc,
			startMin:   startMin,
			endMin:     endMin,
			notes:      notes,
			recurrence: recurrence,
			recurUntil: recurUntil,
			err:        parseErr,
		}
	})
}

func notesCursorLine(content string) int {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "Notes:" {
			return i + 3
		}
	}
	return len(lines)
}

// formatEventForEditor creates the temp file content for editing.
func formatEventForEditor(ev Event) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Title: %s\n", ev.Title))
	sb.WriteString(fmt.Sprintf("Desc: %s\n", ev.Desc))
	sb.WriteString(fmt.Sprintf("Start: %s\n", MinToTime(ev.StartMin)))
	sb.WriteString(fmt.Sprintf("End: %s\n", MinToTime(ev.EndMin)))
	sb.WriteString(fmt.Sprintf("Repeat: %s\n", RecurrenceLabel(ev.Recurrence)))
	if ev.RecurUntilStr != "" {
		sb.WriteString(fmt.Sprintf("Until: %s\n", ev.RecurUntilStr))
	} else {
		sb.WriteString("Until: \n")
	}
	sb.WriteString("\n")
	sb.WriteString("# Edit the fields above using these formats:\n")
	sb.WriteString("#   Title: free text\n")
	sb.WriteString("#   Desc: free text\n")
	sb.WriteString("#   Start: HH:MM\n")
	sb.WriteString("#   End: HH:MM\n")
	sb.WriteString("#   Repeat: None, Daily, Weekdays, Weekly, Biweekly, Monthly, Yearly\n")
	sb.WriteString("#   Until: YYYY-MM-DD or leave blank\n")
	sb.WriteString("#\n")
	sb.WriteString("# Write longer notes under the --- separator below.\n")
	sb.WriteString("---\n")
	sb.WriteString("Notes:\n")
	sb.WriteString(ev.Notes)
	return sb.String()
}

// parseEditorResult reads the temp file and extracts event fields.
func parseEditorResult(path string) (title string, desc string, startMin, endMin int, notes string, recurrence string, recurUntil string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", 0, 0, "", "", "", fmt.Errorf("read editor file: %w", err)
	}

	content := string(data)
	parts := strings.SplitN(content, "---\n", 2)
	header := parts[0]
	if len(parts) > 1 {
		notes = strings.TrimRight(parts[1], "\n")
		notes = strings.TrimPrefix(notes, "Notes:\n")
	}

	repeatLabel := ""
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
		} else if strings.HasPrefix(line, "Desc:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "Desc:"))
		} else if strings.HasPrefix(line, "Start:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Start:"))
			startMin, err = parseTime(val)
			if err != nil {
				return "", "", 0, 0, "", "", "", fmt.Errorf("invalid start time %q: %w", val, err)
			}
		} else if strings.HasPrefix(line, "End:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "End:"))
			endMin, err = parseTime(val)
			if err != nil {
				return "", "", 0, 0, "", "", "", fmt.Errorf("invalid end time %q: %w", val, err)
			}
		} else if strings.HasPrefix(line, "Repeat:") {
			repeatLabel = strings.TrimSpace(strings.TrimPrefix(line, "Repeat:"))
		} else if strings.HasPrefix(line, "Until:") {
			recurUntil = strings.TrimSpace(strings.TrimPrefix(line, "Until:"))
		}
	}

	// Convert repeat label back to recurrence constant
	recurrence = parseLabelToRecurrence(repeatLabel)

	if title == "" {
		title = "Untitled"
	}
	return title, desc, startMin, endMin, notes, recurrence, recurUntil, nil
}

// parseLabelToRecurrence converts a human-readable label back to a recurrence constant.
func parseLabelToRecurrence(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "daily":
		return RecurDaily
	case "weekdays":
		return RecurWeekdays
	case "weekly":
		return RecurWeekly
	case "biweekly":
		return RecurBiweekly
	case "monthly":
		return RecurMonthly
	case "yearly":
		return RecurYearly
	default:
		return RecurNone
	}
}

// parseTime parses time strings to minutes since midnight.
// Accepts: "HH:MM", "HHMM", "HMM", "HH", "H" formats.
// Examples: "12:30" → 750, "1200" → 720, "130" → 90, "12" → 720, "9" → 540
func parseTime(s string) (int, error) {
	s = strings.TrimSpace(s)
	var h, m int
	if strings.Contains(s, ":") {
		_, err := fmt.Sscanf(s, "%d:%d", &h, &m)
		if err != nil {
			return 0, err
		}
	} else {
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("invalid time format")
		}
		switch {
		case len(s) <= 2:
			// 1-2 digits: treat as hours (e.g. "9" → 09:00, "12" → 12:00)
			h = n
			m = 0
		case len(s) == 3:
			// 3 digits: first digit is hour, last two are minutes (e.g. "130" → 1:30, "930" → 9:30)
			h = n / 100
			m = n % 100
		case len(s) == 4:
			// 4 digits: HHMM (e.g. "1200" → 12:00, "0930" → 09:30)
			h = n / 100
			m = n % 100
		default:
			return 0, fmt.Errorf("invalid time format")
		}
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("time out of range")
	}
	return h*60 + m, nil
}
