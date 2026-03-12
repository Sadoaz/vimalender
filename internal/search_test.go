package internal

import (
	"testing"
)

func TestSearchEvents_MatchesTitle(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Morning standup", "2025-03-15", 540, 570))
	store.Add(makeEvent("Lunch break", "2025-03-15", 720, 780))

	matches := SearchEvents(store, "standup")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].EventID == "" {
		t.Error("match should have an event ID")
	}
}

func TestSearchEvents_MatchesDescription(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Meeting", "2025-03-15", 540, 600)
	ev.Desc = "Discuss quarterly goals"
	store.Add(ev)

	matches := SearchEvents(store, "quarterly")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match on description, got %d", len(matches))
	}
}

func TestSearchEvents_CaseInsensitive(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Team RETRO", "2025-03-15", 540, 600))

	matches := SearchEvents(store, "retro")
	if len(matches) != 1 {
		t.Errorf("expected case-insensitive match, got %d", len(matches))
	}
	matches = SearchEvents(store, "RETRO")
	if len(matches) != 1 {
		t.Errorf("expected case-insensitive match for upper, got %d", len(matches))
	}
}

func TestSearchEvents_EmptyQuery(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Something", "2025-03-15", 540, 600))

	matches := SearchEvents(store, "")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty query, got %d", len(matches))
	}
}

func TestSearchEvents_NoMatch(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Meeting", "2025-03-15", 540, 600))

	matches := SearchEvents(store, "nonexistent")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestSearchEvents_MultipleMatches_SortedByDate(t *testing.T) {
	store := NewEventStore()
	store.Add(makeEvent("Planning session", "2025-03-20", 540, 600))
	store.Add(makeEvent("Planning review", "2025-03-10", 540, 600))
	store.Add(makeEvent("Planning kickoff", "2025-03-15", 540, 600))

	matches := SearchEvents(store, "planning")
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
	// Should be sorted by date
	for i := 1; i < len(matches); i++ {
		if matches[i].Date.Before(matches[i-1].Date) {
			t.Errorf("matches not sorted by date: %v before %v", matches[i].Date, matches[i-1].Date)
		}
	}
}

func TestSearchEvents_AfterEventDeleted(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Deletable", "2025-03-15", 540, 600)
	store.Add(ev)
	store.Add(makeEvent("Keeper", "2025-03-15", 660, 720))

	// Search before delete
	matches := SearchEvents(store, "Deletable")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match before delete, got %d", len(matches))
	}

	// Delete the event
	date := testDate(t, "2025-03-15")
	stored := store.GetStoredByDate(date)
	for i, s := range stored {
		if s.Title == "Deletable" {
			store.Delete(date, i)
			break
		}
	}

	// Search after delete - should find no matches
	matches = SearchEvents(store, "Deletable")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches after delete, got %d", len(matches))
	}

	// Other events should still be findable
	matches = SearchEvents(store, "Keeper")
	if len(matches) != 1 {
		t.Errorf("expected 1 match for Keeper, got %d", len(matches))
	}
}

func TestSearchEvents_AfterEventModified(t *testing.T) {
	store := NewEventStore()
	ev := makeEvent("Old title", "2025-03-15", 540, 600)
	store.Add(ev)

	// Search finds old title
	matches := SearchEvents(store, "Old title")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for old title, got %d", len(matches))
	}

	// Modify event title directly in store
	date := testDate(t, "2025-03-15")
	stored := store.GetStoredByDate(date)
	if len(stored) > 0 {
		// Delete and re-add with new title
		id := stored[0].ID
		store.Delete(date, 0)
		newEv := makeEvent("New title", "2025-03-15", 540, 600)
		newEv.ID = id
		store.Add(newEv)
	}

	// Old title should not be found
	matches = SearchEvents(store, "Old title")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for old title after modify, got %d", len(matches))
	}

	// New title should be found
	matches = SearchEvents(store, "New title")
	if len(matches) != 1 {
		t.Errorf("expected 1 match for new title, got %d", len(matches))
	}
}
