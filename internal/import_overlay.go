package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) openImportOverlay() {
	m.importReturnMode = m.mode
	m.importReturnView = m.viewMode
	m.mode = ModeImport
	m.importMatches = nil
	if m.importSummary == nil {
		m.importSummary = []string{}
	}
}

func (m *Model) closeImportOverlay() {
	m.mode = m.importReturnMode
	m.viewMode = m.importReturnView
	if m.mode == 0 {
		m.mode = ModeNavigate
	}
}

func (m Model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc), IsKey(msg, KeyQ):
		m.closeImportOverlay()
		return m, nil

	case IsKey(msg, KeyTab):
		m.applyImportCompletion()
		return m, nil

	case IsKey(msg, KeyEnter):
		m.importMatches = nil
		path := strings.TrimSpace(m.importPath)
		result, err := ImportICSFile(path)
		if err != nil {
			m.importSummary = []string{fmt.Sprintf("Import failed: %v", err)}
			return m, nil
		}
		pushedUndo := false
		if result.Imported > 0 {
			for _, ev := range result.Events {
				if !pushedUndo {
					m.pushUndo()
					pushedUndo = true
				}
				if err := m.store.AddSpanningEvent(ev); err != nil {
					result.Skipped = append(result.Skipped, ImportIssue{Title: ev.Title, Reason: err.Error()})
					continue
				}
				result.ImportedAdded++
			}
			if result.ImportedAdded > 0 {
				m.saveEvents()
			}
		}
		m.importSummary = formatImportSummary(result)
		m.mode = ModeNavigate
		m.viewMode = ViewWeek
		m.settingsSearchActive = false
		m.settingsSearchQuery = ""
		return m, m.setTimedStatus(importStatusMessage(result), 4*time.Second)

	case msg.String() == "backspace":
		if len(m.importPath) > 0 {
			m.importPath = m.importPath[:len(m.importPath)-1]
		}
		m.importMatches = nil
		return m, nil

	case msg.String() == "ctrl+w":
		m.importPath = deletePreviousWord(m.importPath)
		m.importMatches = nil
		return m, nil
	}

	if len(msg.Runes) > 0 {
		m.importPath += string(msg.Runes)
		m.importMatches = nil
	}
	return m, nil
}

func (m *Model) applyImportCompletion() {
	completed, matches, err := completeICSPath(m.importPath, false)
	if err != nil {
		m.importMatches = nil
		m.importSummary = []string{fmt.Sprintf("Path completion failed: %v", err)}
		return
	}
	if len(matches) == 0 {
		m.importMatches = nil
		m.importSummary = []string{"No matching folders or .ics files found."}
		return
	}
	m.importPath = completed
	m.importMatches = matches
	if len(matches) == 1 {
		m.importSummary = []string{fmt.Sprintf("Completed path: %s", completed)}
		m.importMatches = nil
		return
	}
	m.importSummary = []string{fmt.Sprintf("%d matches. Press Tab again after typing more.", len(matches))}
}

func RenderImport(m *Model) string {
	accent := m.uiColor("accent", "39")
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.uiColor("hint_fg", "243")))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.uiColor("help_border", accent))).Padding(1, 2)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accent)).Padding(0, 1)

	panelWidth := m.width - 10
	if panelWidth > 84 {
		panelWidth = 84
	}
	if panelWidth < 40 {
		panelWidth = 40
	}
	contentWidth := panelWidth - 6
	if contentWidth < 34 {
		contentWidth = 34
	}

	var body []string
	body = append(body, titleStyle.Render("Import Events"))
	body = append(body, mutedStyle.Render("Type a relative or absolute .ics path manually. Press Tab to complete folders and .ics files, then Enter to import."))
	body = append(body, "")
	body = append(body, sectionStyle.Render(" Import "))
	body = append(body, fmt.Sprintf("  Action   %s", valueStyle.Render("Import events")))
	body = append(body, fmt.Sprintf("  File     %s_", m.importPath))
	body = append(body, "")
	if len(m.importMatches) > 0 {
		body = append(body, sectionStyle.Render(" Matches "))
		limit := len(m.importMatches)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			body = append(body, "  "+m.importMatches[i])
		}
		if len(m.importMatches) > limit {
			body = append(body, fmt.Sprintf("  ...and %d more", len(m.importMatches)-limit))
		}
		body = append(body, "")
	}
	if len(m.importSummary) > 0 {
		body = append(body, sectionStyle.Render(" Results "))
		body = append(body, m.importSummary...)
		body = append(body, "")
	}
	body = append(body, mutedStyle.Render("Tab: complete path  Enter: import  Ctrl+w: delete word  Backspace: delete  Esc/q: close"))

	content := lipgloss.NewStyle().Width(contentWidth).Render(strings.Join(body, "\n"))
	box := boxStyle.Render(content)
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(box)
}

func formatImportSummary(result ImportResult) []string {
	lines := []string{
		fmt.Sprintf("File: %s", result.SourceFilePath),
		fmt.Sprintf("Imported %d event(s).", result.ImportedAdded),
		fmt.Sprintf("Skipped %d event(s).", len(result.Skipped)),
	}
	if len(result.Skipped) > 0 {
		limit := len(result.Skipped)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			issue := result.Skipped[i]
			title := issue.Title
			if title == "" {
				title = "(untitled event)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", title, issue.Reason))
		}
		if len(result.Skipped) > limit {
			lines = append(lines, fmt.Sprintf("- ...and %d more", len(result.Skipped)-limit))
		}
	}
	lines = append(lines, "Duplicate prevention is not automatic; re-importing the same file adds new events.")
	return lines
}

func importStatusMessage(result ImportResult) string {
	if result.ImportedAdded > 0 && len(result.Skipped) == 0 {
		return fmt.Sprintf("Imported %d event(s)", result.ImportedAdded)
	}
	if result.ImportedAdded > 0 {
		return fmt.Sprintf("Imported %d event(s), skipped %d", result.ImportedAdded, len(result.Skipped))
	}
	return fmt.Sprintf("Imported 0 event(s), skipped %d", len(result.Skipped))
}

func completeICSPath(input string, includeNonExistingFile bool) (string, []string, error) {
	dirPrefix, partial := splitImportPath(input)
	searchDir, err := resolveImportSearchDir(dirPrefix)
	if err != nil {
		return input, nil, err
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return input, nil, err
	}
	matches := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, partial) {
			continue
		}
		if !entry.IsDir() && strings.ToLower(filepath.Ext(name)) != ".ics" {
			continue
		}
		candidate := dirPrefix + name
		if entry.IsDir() {
			candidate += string(os.PathSeparator)
		}
		matches = append(matches, candidate)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		if includeNonExistingFile && partial != "" {
			candidate := dirPrefix + partial
			if strings.HasSuffix(strings.ToLower(candidate), ".ics") {
				return candidate, []string{candidate}, nil
			}
		}
		return input, nil, nil
	}
	if len(matches) == 1 {
		return matches[0], matches, nil
	}
	common := longestCommonPathPrefix(matches)
	if len(common) > len(input) {
		return common, matches, nil
	}
	return input, matches, nil
}

func splitImportPath(input string) (string, string) {
	if input == "" {
		return "", ""
	}
	sep := strings.LastIndexAny(input, "/\\")
	if sep == -1 {
		return "", input
	}
	return input[:sep+1], input[sep+1:]
}

func resolveImportSearchDir(dirPrefix string) (string, error) {
	if dirPrefix == "" {
		return ".", nil
	}
	if dirPrefix == string(os.PathSeparator) {
		return dirPrefix, nil
	}
	if strings.HasPrefix(dirPrefix, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Clean(filepath.Join(home, dirPrefix[2:])), nil
	}
	if dirPrefix == "~"+string(os.PathSeparator) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	return filepath.Clean(dirPrefix), nil
}

func longestCommonPathPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, value := range values[1:] {
		max := len(prefix)
		if len(value) < max {
			max = len(value)
		}
		i := 0
		for i < max && prefix[i] == value[i] {
			i++
		}
		prefix = prefix[:i]
		if prefix == "" {
			break
		}
	}
	return prefix
}
