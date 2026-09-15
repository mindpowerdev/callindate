package schedule

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mindpowerdev/callindate.git/internal/storage"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSmoke(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, store, Activity{Name: "Плавание", Weekday: time.Monday, StartTime: "17:00"})
	mustAdd(t, store, Activity{Name: "Английский", Weekday: time.Monday, StartTime: "15:30"})
	mustAdd(t, store, Activity{Name: "Шахматы", Weekday: time.Tuesday, StartTime: "16:00"})

	acts, err := store.ForWeekday(ctx, time.Monday)
	must(t, err)
	if len(acts) != 2 || acts[0].StartTime != "15:30" || acts[1].StartTime != "17:00" {
		t.Fatalf("неверная сортировка/количество: %+v", acts)
	}

	wd, ok := ParseWeekday("пн")
	if !ok || wd != time.Monday {
		t.Fatalf("ParseWeekday: %v %v", wd, ok)
	}

	tm, ok := ParseStartTime("09:05")
	if !ok || tm != "09:05" {
		t.Fatalf("ParseStartTime: %v %v", tm, ok)
	}

	if _, ok := ParseStartTime("25:99"); ok {
		t.Fatal("ParseStartTime должен отклонять некорректное время")
	}
}

func TestEnsureOccurrencesForDateIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, store, Activity{Name: "Плавание", Weekday: time.Monday, StartTime: "17:00"})
	monday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC) // понедельник

	first, err := store.EnsureOccurrencesForDate(ctx, monday)
	must(t, err)
	if len(first) != 1 {
		t.Fatalf("ожидался 1 occurrence, получено %d", len(first))
	}

	second, err := store.EnsureOccurrencesForDate(ctx, monday)
	must(t, err)
	if len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("повторный вызов не должен плодить дубликаты: %+v", second)
	}
}

func TestSetOccurrenceStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, store, Activity{Name: "Плавание", Weekday: time.Monday, StartTime: "17:00"})
	monday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)

	occs, err := store.EnsureOccurrencesForDate(ctx, monday)
	must(t, err)

	updated, err := store.SetOccurrenceStatus(ctx, occs[0].ID, StatusAttended)
	must(t, err)
	if updated.Status != StatusAttended {
		t.Fatalf("статус не обновился: %+v", updated)
	}
}

func TestStatsForRange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, store, Activity{Name: "Плавание", Weekday: time.Monday, StartTime: "17:00"})

	monday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	nextMonday := monday.AddDate(0, 0, 7)

	occs, err := store.EnsureOccurrencesForDate(ctx, monday)
	must(t, err)
	_, err = store.SetOccurrenceStatus(ctx, occs[0].ID, StatusAttended)
	must(t, err)

	counts, err := store.StatsForRange(ctx, monday, nextMonday)
	must(t, err)

	c := counts["Плавание"]
	if c.Planned != 2 || c.Attended != 1 {
		t.Fatalf("неверная статистика: %+v", c)
	}
}

func mustAdd(t *testing.T, store *Store, a Activity) int64 {
	t.Helper()
	id, err := store.Add(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
