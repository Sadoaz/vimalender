package internal

import (
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// GetByDate with recurring events (hot path when scrolling)
// ---------------------------------------------------------------------------

func BenchmarkGetByDate_NoRecurrence(b *testing.B) {
	store := NewEventStore()
	date := time.Date(2025, 3, 15, 0, 0, 0, 0, time.Local)
	for i := 0; i < 20; i++ {
		ev := Event{
			ID:       fmt.Sprintf("ev-%d", i),
			Title:    fmt.Sprintf("Event %d", i),
			Date:     date,
			DateStr:  "2025-03-15",
			StartMin: 480 + i*30,
			EndMin:   480 + i*30 + 30,
		}
		store.Add(ev)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetByDate(date)
	}
}

func BenchmarkGetByDate_ManyRecurring(b *testing.B) {
	store := NewEventStore()
	base := time.Date(2025, 1, 6, 0, 0, 0, 0, time.Local) // Monday
	for i := 0; i < 50; i++ {
		ev := Event{
			ID:         fmt.Sprintf("rec-%d", i),
			Title:      fmt.Sprintf("Recurring %d", i),
			Date:       base,
			DateStr:    "2025-01-06",
			StartMin:   480 + (i%24)*30,
			EndMin:     480 + (i%24)*30 + 30,
			Recurrence: RecurWeekly,
		}
		store.Add(ev)
	}
	// Query a date 10 weeks out - forces matchesDate on every recurring event
	query := base.AddDate(0, 0, 70) // 10 weeks later, same weekday
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetByDate(query)
	}
}

// ---------------------------------------------------------------------------
// matchesDate (called per recurring event per visible day)
// ---------------------------------------------------------------------------

func BenchmarkMatchesDate_Weekly(b *testing.B) {
	base := time.Date(2025, 1, 6, 0, 0, 0, 0, time.Local)
	ev := Event{Date: base, DateStr: "2025-01-06", Recurrence: RecurWeekly}
	query := base.AddDate(0, 0, 70)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchesDate(ev, query)
	}
}

func BenchmarkMatchesDate_Monthly(b *testing.B) {
	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.Local)
	ev := Event{Date: base, DateStr: "2025-01-15", Recurrence: RecurMonthly}
	query := time.Date(2025, 12, 15, 0, 0, 0, 0, time.Local)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchesDate(ev, query)
	}
}

// ---------------------------------------------------------------------------
// layoutEventsList (runs per column per frame)
// ---------------------------------------------------------------------------

func BenchmarkLayoutEventsList_Sparse(b *testing.B) {
	events := make([]Event, 5)
	for i := range events {
		events[i] = Event{
			ID:       fmt.Sprintf("ev-%d", i),
			StartMin: 480 + i*120,
			EndMin:   480 + i*120 + 60,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layoutEventsList(events, "", 0)
	}
}

func BenchmarkLayoutEventsList_Dense(b *testing.B) {
	events := make([]Event, 15)
	for i := range events {
		events[i] = Event{
			ID:       fmt.Sprintf("ev-%d", i),
			StartMin: 480 + (i%5)*30,
			EndMin:   480 + (i%5)*30 + 90, // heavy overlap
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layoutEventsList(events, "", 0)
	}
}

// ---------------------------------------------------------------------------
// SearchEvents (runs on every keystroke in search mode)
// ---------------------------------------------------------------------------

func BenchmarkSearchEvents_SmallStore(b *testing.B) {
	store := NewEventStore()
	for i := 0; i < 50; i++ {
		d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, i)
		ev := Event{
			ID:       fmt.Sprintf("ev-%d", i),
			Title:    fmt.Sprintf("Meeting %d about project alpha", i),
			Date:     d,
			DateStr:  d.Format("2006-01-02"),
			StartMin: 540,
			EndMin:   600,
		}
		store.Add(ev)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SearchEvents(store, "alpha")
	}
}

func BenchmarkSearchEvents_LargeStore(b *testing.B) {
	store := NewEventStore()
	for i := 0; i < 500; i++ {
		d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, i%365)
		ev := Event{
			ID:       fmt.Sprintf("ev-%d", i),
			Title:    fmt.Sprintf("Event %d category-%d", i, i%10),
			Desc:     fmt.Sprintf("Description for event number %d", i),
			Date:     d,
			DateStr:  d.Format("2006-01-02"),
			StartMin: 480 + (i%20)*30,
			EndMin:   480 + (i%20)*30 + 30,
		}
		store.Add(ev)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SearchEvents(store, "category-5")
	}
}

// ---------------------------------------------------------------------------
// ICS export (full calendar serialization)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Stress tests — thousands of events to find scaling bottlenecks
// ---------------------------------------------------------------------------

func BenchmarkGetByDate_5000Events(b *testing.B) {
	store := NewEventStore()
	// 5000 events spread across 365 days
	for i := 0; i < 5000; i++ {
		d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, i%365)
		ev := Event{
			ID:       fmt.Sprintf("ev-%d", i),
			Title:    fmt.Sprintf("Event %d", i),
			Date:     d,
			DateStr:  d.Format("2006-01-02"),
			StartMin: 480 + (i%20)*30,
			EndMin:   480 + (i%20)*30 + 30,
		}
		store.Add(ev)
	}
	query := time.Date(2025, 6, 15, 0, 0, 0, 0, time.Local)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetByDate(query)
	}
}

func BenchmarkGetByDate_5000Recurring(b *testing.B) {
	store := NewEventStore()
	// 5000 recurring events on different base dates — worst case for GetByDate
	// which must check matchesDate on every one
	for i := 0; i < 5000; i++ {
		d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, i%365)
		ev := Event{
			ID:         fmt.Sprintf("rec-%d", i),
			Title:      fmt.Sprintf("Recurring %d", i),
			Date:       d,
			DateStr:    d.Format("2006-01-02"),
			StartMin:   480 + (i%20)*30,
			EndMin:     480 + (i%20)*30 + 30,
			Recurrence: RecurWeekly,
		}
		store.Add(ev)
	}
	query := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetByDate(query)
	}
}

func BenchmarkSearchEvents_5000(b *testing.B) {
	store := NewEventStore()
	for i := 0; i < 5000; i++ {
		d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, i%365)
		ev := Event{
			ID:       fmt.Sprintf("ev-%d", i),
			Title:    fmt.Sprintf("Event %d category-%d", i, i%10),
			Desc:     fmt.Sprintf("Description for event number %d", i),
			Date:     d,
			DateStr:  d.Format("2006-01-02"),
			StartMin: 480 + (i%20)*30,
			EndMin:   480 + (i%20)*30 + 30,
		}
		store.Add(ev)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SearchEvents(store, "category-5")
	}
}

func BenchmarkBuildICSExport(b *testing.B) {
	store := NewEventStore()
	for i := 0; i < 100; i++ {
		d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, i)
		ev := Event{
			ID:         fmt.Sprintf("ev-%d", i),
			Title:      fmt.Sprintf("Event %d", i),
			Desc:       "A description with special chars: commas, semicolons; and\nnewlines",
			Date:       d,
			DateStr:    d.Format("2006-01-02"),
			StartMin:   540,
			EndMin:     600,
			Recurrence: RecurWeekly,
		}
		store.Add(ev)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildICSExport(store)
	}
}
