package payment

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

func TestAdvanceDueMonthly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	due := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertPlan(ctx, Plan{ActivityName: "Английский", Type: PlanMonthly, Amount: 3000, NextDueDate: &due}); err != nil {
		t.Fatal(err)
	}

	if err := store.AdvanceDue(ctx, "Английский"); err != nil {
		t.Fatal(err)
	}

	plan, ok, err := store.PlanByName(ctx, "английский") // регистр не важен (COLLATE NOCASE)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("план не найден")
	}
	want := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if !plan.NextDueDate.Equal(want) {
		t.Fatalf("next_due_date = %v, want %v", plan.NextDueDate, want)
	}
	if plan.DueReminderSentFor != nil {
		t.Fatal("due_reminder_sent_for должен сброситься после сдвига даты")
	}
}

func TestAdvanceDueSemester(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	due := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	err := store.UpsertPlan(ctx, Plan{
		ActivityName: "Футбол", Type: PlanSemester, Amount: 15000, IntervalMonths: 3, NextDueDate: &due,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AdvanceDue(ctx, "Футбол"); err != nil {
		t.Fatal(err)
	}

	plan, _, err := store.PlanByName(ctx, "Футбол")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if !plan.NextDueDate.Equal(want) {
		t.Fatalf("next_due_date = %v, want %v", plan.NextDueDate, want)
	}
}

func TestAbonementLessons(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.UpsertPlan(ctx, Plan{ActivityName: "Плавание", Type: PlanAbonement, Amount: 4000, LessonsTotal: 8, LessonsRemaining: 8})
	if err != nil {
		t.Fatal(err)
	}

	for range 7 {
		if err := store.DecrementLesson(ctx, "Плавание"); err != nil {
			t.Fatal(err)
		}
	}
	plan, _, err := store.PlanByName(ctx, "Плавание")
	if err != nil {
		t.Fatal(err)
	}
	if plan.LessonsRemaining != 1 {
		t.Fatalf("lessons_remaining = %d, want 1", plan.LessonsRemaining)
	}

	// списание не должно уходить ниже нуля
	if err := store.DecrementLesson(ctx, "Плавание"); err != nil {
		t.Fatal(err)
	}
	if err := store.DecrementLesson(ctx, "Плавание"); err != nil {
		t.Fatal(err)
	}
	plan, _, err = store.PlanByName(ctx, "Плавание")
	if err != nil {
		t.Fatal(err)
	}
	if plan.LessonsRemaining != 0 {
		t.Fatalf("lessons_remaining = %d, want 0 (не должно уходить в минус)", plan.LessonsRemaining)
	}

	if err := store.SetLowBalanceReminded(ctx, "Плавание", true); err != nil {
		t.Fatal(err)
	}
	if err := store.AddLessons(ctx, "Плавание", 8); err != nil {
		t.Fatal(err)
	}
	plan, _, err = store.PlanByName(ctx, "Плавание")
	if err != nil {
		t.Fatal(err)
	}
	if plan.LessonsRemaining != 8 || plan.LessonsTotal != 16 {
		t.Fatalf("пополнение абонемента: %+v", plan)
	}
	if plan.LowBalanceReminded {
		t.Fatal("low_balance_reminded должен сброситься после пополнения")
	}
}

func TestPlanByNameCaseInsensitiveCyrillic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// SQLite-коллация COLLATE NOCASE сворачивает регистр только для ASCII, поэтому
	// сопоставление имён кружков сделано через нормализацию в Go — проверяем это явно.
	if err := store.UpsertPlan(ctx, Plan{ActivityName: "Английский", Type: PlanPerVisit, Amount: 500}); err != nil {
		t.Fatal(err)
	}

	for _, variant := range []string{"английский", "АНГЛИЙСКИЙ", "  Английский  "} {
		plan, ok, err := store.PlanByName(ctx, variant)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("план не найден по варианту %q", variant)
		}
		if plan.ActivityName != "Английский" {
			t.Fatalf("отображаемое имя должно сохранять исходный регистр, получено %q", plan.ActivityName)
		}
	}
}

func TestRemovePlan(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertPlan(ctx, Plan{ActivityName: "Шахматы", Type: PlanPerVisit, Amount: 500}); err != nil {
		t.Fatal(err)
	}
	if err := store.RemovePlan(ctx, "Шахматы"); err != nil {
		t.Fatal(err)
	}
	_, ok, err := store.PlanByName(ctx, "Шахматы")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("план должен быть удалён")
	}
}
