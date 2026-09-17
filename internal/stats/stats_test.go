package stats

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/schedule"
	"github.com/mindpowerdev/callindate.git/internal/storage"
)

func TestBuildReport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	scheduleStore, err := schedule.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	paymentStore, err := payment.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Плавание по понедельникам, два занятия в диапазоне; одно посетили, одно пропустили.
	if _, err := scheduleStore.Add(ctx, schedule.Activity{Name: "Плавание", Weekday: time.Monday, StartTime: "17:00"}); err != nil {
		t.Fatal(err)
	}

	monday1 := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	monday2 := monday1.AddDate(0, 0, 7)

	occ1, err := scheduleStore.EnsureOccurrencesForDate(ctx, monday1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduleStore.SetOccurrenceStatus(ctx, occ1[0].ID, schedule.StatusAttended); err != nil {
		t.Fatal(err)
	}

	occ2, err := scheduleStore.EnsureOccurrencesForDate(ctx, monday2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduleStore.SetOccurrenceStatus(ctx, occ2[0].ID, schedule.StatusMissed); err != nil {
		t.Fatal(err)
	}

	// Оплата разом покрывает 8 занятий вперёд — цена одного занятия 4000/8 = 500,
	// а не "потрачено / посещено за период" (иначе цена посещения "плыла" бы в зависимости
	// от того, сколько занятий уже отметили в текущем периоде).
	if err := paymentStore.Add(ctx, payment.Payment{ActivityName: "Плавание", Amount: 4000, LessonsCount: 8, PaidAt: monday1}); err != nil {
		t.Fatal(err)
	}

	report, err := BuildReport(ctx, scheduleStore, paymentStore, monday1, monday2)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Activities) != 1 {
		t.Fatalf("ожидался 1 кружок в отчёте, получено %d", len(report.Activities))
	}
	a := report.Activities[0]
	if a.Name != "Плавание" || a.Planned != 2 || a.Attended != 1 || a.Missed != 1 || a.Spent != 500 {
		t.Fatalf("неверная статистика: %+v", a)
	}

	cost, ok := a.CostPerVisit()
	if !ok || cost != 500 {
		t.Fatalf("CostPerVisit = %v, %v, want 500, true", cost, ok)
	}

	if report.TotalSpent != 500 || report.TotalAttended != 1 {
		t.Fatalf("неверные итоги: TotalSpent=%v TotalAttended=%v", report.TotalSpent, report.TotalAttended)
	}
}

// TestBuildReportAbonementCostPerVisitSplitAcrossLessons воспроизводит баг: абонемент оплачен
// разом на несколько занятий вперёд, но цена посещения (и "потрачено" в отчёте) должна делиться
// на оплаченное количество занятий, а не приравниваться ко всей сумме платежа.
func TestBuildReportAbonementCostPerVisitSplitAcrossLessons(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	scheduleStore, err := schedule.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	paymentStore, err := payment.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := scheduleStore.Add(ctx, schedule.Activity{Name: "Каратэ", Weekday: time.Monday, StartTime: "18:00"}); err != nil {
		t.Fatal(err)
	}

	monday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	occ, err := scheduleStore.EnsureOccurrencesForDate(ctx, monday)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduleStore.SetOccurrenceStatus(ctx, occ[0].ID, schedule.StatusAttended); err != nil {
		t.Fatal(err)
	}

	// 1750 ₽ за 7 занятий абонемента, посетили пока только одно.
	if err := paymentStore.Add(ctx, payment.Payment{ActivityName: "Каратэ", Amount: 1750, LessonsCount: 7, PaidAt: monday}); err != nil {
		t.Fatal(err)
	}

	report, err := BuildReport(ctx, scheduleStore, paymentStore, monday, monday)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Activities) != 1 {
		t.Fatalf("ожидался 1 кружок в отчёте, получено %d", len(report.Activities))
	}
	a := report.Activities[0]
	cost, ok := a.CostPerVisit()
	if !ok || cost != 250 {
		t.Fatalf("CostPerVisit = %v, %v, want 250, true", cost, ok)
	}
	if a.Spent != 250 {
		t.Fatalf("Spent = %v, want 250 (250 ₽ за занятие × 1 посещение)", a.Spent)
	}
}

func TestCostPerVisitNoAttendance(t *testing.T) {
	a := ActivityStats{Name: "Шахматы"}
	if _, ok := a.CostPerVisit(); ok {
		t.Fatal("CostPerVisit должен вернуть false при нулевой посещаемости")
	}
}
