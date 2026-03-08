package internal

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// updateGoto handles keys in go-to time input mode.
func (m Model) updateGoto(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = m.gotoReturnMode
		if m.mode == 0 {
			m.mode = ModeNavigate
		}
		m.gotoReturnMode = ModeNavigate
		m.gotoBuffer = ""

	case IsKey(msg, KeyEnter):
		min, err := parseTime(m.gotoBuffer)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Invalid time: %s", m.gotoBuffer)
		} else {
			if m.gotoReturnMode == ModeAdjust {
				delta := min - m.cursorMin
				m.moveAdjustBy(delta)
			} else {
				m.cursorMin = min
				m.ensureCursorVisible()
			}
			// Center viewport on cursor
			if m.zoomLevel != ZoomAuto {
				vpHeight := m.viewportHeight()
				mpr := m.MinutesPerRow()
				m.viewportOffset = m.cursorMin - mpr*(vpHeight/2)
				if m.viewportOffset < 0 {
					m.viewportOffset = 0
				}
			}
		}
		m.mode = m.gotoReturnMode
		if m.mode == 0 {
			m.mode = ModeNavigate
		}
		m.gotoReturnMode = ModeNavigate
		m.gotoBuffer = ""

	case msg.String() == "backspace":
		if len(m.gotoBuffer) > 0 {
			m.gotoBuffer = m.gotoBuffer[:len(m.gotoBuffer)-1]
		}

	default:
		s := msg.String()
		if len(s) == 1 && (s[0] >= '0' && s[0] <= '9' || s[0] == ':') {
			m.gotoBuffer += s
		}
	}
	return m, nil
}

// updateGotoDay handles keys in go-to day of month input mode.
func (m Model) updateGotoDay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = m.gotoReturnMode
		if m.mode == 0 {
			m.mode = ModeNavigate
		}
		m.gotoReturnMode = ModeNavigate
		m.gotoBuffer = ""

	case IsKey(msg, KeyEnter):
		day, err := strconv.Atoi(m.gotoBuffer)
		if err != nil || day < 1 {
			m.statusMsg = fmt.Sprintf("Invalid day: %s", m.gotoBuffer)
		} else {
			cur := m.SelectedDate()
			lastDay := time.Date(cur.Year(), cur.Month()+1, 0, 0, 0, 0, 0, cur.Location()).Day()
			if day > lastDay {
				day = lastDay
			}
			target := time.Date(cur.Year(), cur.Month(), day, 0, 0, 0, 0, cur.Location())
			if m.gotoReturnMode == ModeAdjust {
				deltaDays := int(DateKey(target).Sub(DateKey(cur)).Hours() / 24)
				m.moveAdjustBy(deltaDays * MinutesPerDay)
			} else {
				m.windowStart = target
				m.cursorCol = 0
				m.resetOverlapIndex()
			}
		}
		m.mode = m.gotoReturnMode
		if m.mode == 0 {
			m.mode = ModeNavigate
		}
		m.gotoReturnMode = ModeNavigate
		m.gotoBuffer = ""

	case msg.String() == "backspace":
		if len(m.gotoBuffer) > 0 {
			m.gotoBuffer = m.gotoBuffer[:len(m.gotoBuffer)-1]
		}

	default:
		s := msg.String()
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
			m.gotoBuffer += s
		}
	}
	return m, nil
}

// updateSearch handles keys in search input mode.
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case IsKey(msg, KeyEsc):
		m.mode = ModeNavigate
		m.searchQuery = ""
		m.searchMatches = nil
		m.searchActive = false

	case IsKey(msg, KeyEnter):
		m.mode = ModeNavigate
		if len(m.searchMatches) > 0 {
			m.searchActive = true
			m.jumpToMatch(0)
		} else {
			m.statusMsg = "No matches"
			m.searchActive = false
		}

	case msg.String() == "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.updateSearchMatches()
		}

	default:
		s := msg.String()
		if len(s) == 1 || s == " " {
			m.searchQuery += s
			m.updateSearchMatches()
		}
	}
	return m, nil
}

// updateSearchMatches recalculates matches for the current query.
// Only base (stored) events are searched — virtual occurrences of recurring
// events are not expanded, so each recurring event appears as a single match
// on its original date.
func (m *Model) updateSearchMatches() {
	m.searchMatches = nil
	if m.searchQuery == "" {
		return
	}
	query := strings.ToLower(m.searchQuery)

	for date, events := range m.store.AllEvents() {
		for i, ev := range events {
			if strings.Contains(strings.ToLower(ev.Title), query) ||
				strings.Contains(strings.ToLower(ev.Desc), query) {
				m.searchMatches = append(m.searchMatches, SearchMatch{Date: date, Index: i, EventID: ev.ID})
			}
		}
	}

	// Sort matches by date then index for deterministic navigation order
	sort.Slice(m.searchMatches, func(i, j int) bool {
		if m.searchMatches[i].Date.Equal(m.searchMatches[j].Date) {
			return m.searchMatches[i].Index < m.searchMatches[j].Index
		}
		return m.searchMatches[i].Date.Before(m.searchMatches[j].Date)
	})
}

// jumpToMatch navigates to the search match at the given index.
func (m *Model) jumpToMatch(idx int) {
	if idx < 0 || idx >= len(m.searchMatches) {
		return
	}
	m.searchIndex = idx
	match := m.searchMatches[idx]

	// Set window to show the match date
	m.windowStart = match.Date.AddDate(0, 0, -(m.dayCount / 2))
	m.cursorCol = m.dayCount / 2

	// Find event by ID in GetByDate results and set cursor to its start
	events := m.store.GetByDate(match.Date)
	for _, ev := range events {
		if ev.ID == match.EventID {
			m.cursorMin = ev.StartMin
			m.ensureCursorVisible()
			break
		}
	}
}

// nextMatch moves to the next search match.
func (m *Model) nextMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.jumpToMatch((m.searchIndex + 1) % len(m.searchMatches))
}

// prevMatch moves to the previous search match.
func (m *Model) prevMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	idx := m.searchIndex - 1
	if idx < 0 {
		idx = len(m.searchMatches) - 1
	}
	m.jumpToMatch(idx)
}
