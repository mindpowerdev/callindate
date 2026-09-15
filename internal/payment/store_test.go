package payment

import (
	"context"
	"testing"
	"time"
)

func TestSmoke(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	older := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Add(ctx, Payment{ActivityName: "Английский", Amount: 3200, LessonsCount: 4, PaidAt: older}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ctx, Payment{ActivityName: "Плавание", Amount: 5000, LessonsCount: 8, PaidAt: newer}); err != nil {
		t.Fatal(err)
	}

	payments, err := store.Recent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(payments) != 2 || payments[0].ActivityName != "Плавание" || payments[1].ActivityName != "Английский" {
		t.Fatalf("неверный порядок платежей: %+v", payments)
	}

	if amount, ok := ParseAmount("5000,50 ₽"); !ok || amount != 5000.50 {
		t.Fatalf("ParseAmount: %v %v", amount, ok)
	}
	if _, ok := ParseAmount("-100"); ok {
		t.Fatal("ParseAmount должен отклонять отрицательные суммы")
	}

	if got := FormatAmount(5000); got != "5000" {
		t.Fatalf("FormatAmount(5000) = %q", got)
	}
	if got := FormatAmount(5000.5); got != "5000.50" {
		t.Fatalf("FormatAmount(5000.5) = %q", got)
	}

	cases := map[int]string{1: "занятие", 2: "занятия", 5: "занятий", 11: "занятий", 21: "занятие"}
	for n, want := range cases {
		if got := LessonsWord(n); got != want {
			t.Fatalf("LessonsWord(%d) = %q, want %q", n, got, want)
		}
	}
}
