package internal

import (
	"testing"
)

// ---------------------------------------------------------------------------
// formatCreateTimeRange (pure helper from model.go)
// ---------------------------------------------------------------------------

func TestFormatCreateTimeRange(t *testing.T) {
	tests := []struct {
		startMin int
		endMin   int
		want     string
	}{
		{540, 600, "09:00-10:00"},
		{0, 60, "00:00-01:00"},
		{1380, 1440, "23:00-00:00"},
		// Multi-day: endMin > 1440
		{540, 1440 + 600, "09:00-10:00 (+1d)"},
		{540, 2*1440 + 600, "09:00-10:00 (+2d)"},
	}
	for _, tt := range tests {
		got := formatCreateTimeRange(tt.startMin, tt.endMin)
		if got != tt.want {
			t.Errorf("formatCreateTimeRange(%d, %d) = %q, want %q", tt.startMin, tt.endMin, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// buildSpanningSegments (pure helper from event.go)
// ---------------------------------------------------------------------------

func TestBuildSpanningSegments_SingleDay(t *testing.T) {
	template := makeEvent("Single", "2025-03-15", 540, 600)
	date := testDate(t, "2025-03-15")
	segments, err := buildSpanningSegments(template, date, 540, 60, "")
	if err != nil {
		t.Fatalf("buildSpanningSegments: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].StartMin != 540 || segments[0].EndMin != 600 {
		t.Errorf("segment: start=%d end=%d, want 540/600", segments[0].StartMin, segments[0].EndMin)
	}
	// Single segment should NOT have a GroupID
	if segments[0].GroupID != "" {
		t.Error("single segment should not have GroupID")
	}
}

func TestBuildSpanningSegments_MultiDay(t *testing.T) {
	template := makeEvent("Spanning", "2025-03-15", 0, 0)
	date := testDate(t, "2025-03-15")

	// 2-day event: start at 22:00, duration 4 hours (240 min)
	segments, err := buildSpanningSegments(template, date, 22*60, 240, "")
	if err != nil {
		t.Fatalf("buildSpanningSegments: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}

	// First segment: 22:00 to midnight (120 min)
	if segments[0].StartMin != 22*60 || segments[0].EndMin != MinutesPerDay {
		t.Errorf("seg 0: start=%d end=%d, want %d/%d", segments[0].StartMin, segments[0].EndMin, 22*60, MinutesPerDay)
	}
	// Second segment: midnight to 02:00 (120 min)
	if segments[1].StartMin != 0 || segments[1].EndMin != 120 {
		t.Errorf("seg 1: start=%d end=%d, want 0/120", segments[1].StartMin, segments[1].EndMin)
	}

	// Both should share a GroupID
	if segments[0].GroupID == "" {
		t.Error("multi-day segments should have GroupID")
	}
	if segments[0].GroupID != segments[1].GroupID {
		t.Error("segments should share the same GroupID")
	}
}

func TestBuildSpanningSegments_ZeroDuration(t *testing.T) {
	template := makeEvent("Zero", "2025-03-15", 0, 0)
	_, err := buildSpanningSegments(template, testDate(t, "2025-03-15"), 540, 0, "")
	if err == nil {
		t.Error("expected error for zero duration")
	}
}

func TestBuildSpanningSegments_NegativeDuration(t *testing.T) {
	template := makeEvent("Neg", "2025-03-15", 0, 0)
	_, err := buildSpanningSegments(template, testDate(t, "2025-03-15"), 540, -60, "")
	if err == nil {
		t.Error("expected error for negative duration")
	}
}

// ---------------------------------------------------------------------------
// layoutEventsList (pure helper from event.go)
// ---------------------------------------------------------------------------

func TestLayoutEventsList_SingleEvent(t *testing.T) {
	events := []Event{
		makeEvent("Solo", "2025-03-15", 540, 600),
	}
	layout := layoutEventsList(events, "", 0)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}
	l, ok := layout[0]
	if !ok {
		t.Fatal("event 0 not in layout")
	}
	if l.Col != 0 {
		t.Errorf("Col = %d, want 0", l.Col)
	}
	if l.TotalCol != 1 {
		t.Errorf("TotalCol = %d, want 1", l.TotalCol)
	}
}

func TestLayoutEventsList_OverlappingEvents(t *testing.T) {
	events := []Event{
		{ID: "a", StartMin: 540, EndMin: 660, Title: "A"}, // 9:00-11:00
		{ID: "b", StartMin: 600, EndMin: 720, Title: "B"}, // 10:00-12:00
	}
	layout := layoutEventsList(events, "", 0)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}
	la := layout[0]
	lb := layout[1]
	// They overlap, so they should be in different columns
	if la.Col == lb.Col {
		t.Error("overlapping events should be in different columns")
	}
	if la.TotalCol < 2 || lb.TotalCol < 2 {
		t.Errorf("TotalCol should be >= 2 for overlapping events: a=%d b=%d", la.TotalCol, lb.TotalCol)
	}
}

func TestLayoutEventsList_NonOverlapping(t *testing.T) {
	events := []Event{
		{ID: "a", StartMin: 540, EndMin: 600, Title: "A"}, // 9:00-10:00
		{ID: "b", StartMin: 660, EndMin: 720, Title: "B"}, // 11:00-12:00
	}
	layout := layoutEventsList(events, "", 0)
	la := layout[0]
	lb := layout[1]
	// Non-overlapping: each should be full width
	if la.TotalCol != 1 {
		t.Errorf("non-overlapping event A: TotalCol = %d, want 1", la.TotalCol)
	}
	if lb.TotalCol != 1 {
		t.Errorf("non-overlapping event B: TotalCol = %d, want 1", lb.TotalCol)
	}
}

func TestLayoutEventsList_Empty(t *testing.T) {
	layout := layoutEventsList(nil, "", 0)
	if layout != nil {
		t.Error("expected nil layout for empty event list")
	}
}
