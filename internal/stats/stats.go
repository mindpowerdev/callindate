// Package stats считает статистику посещаемости и трат за период. Не зависит от Telegram —
// принимает готовые хранилища и диапазон дат, возвращает данные; текст формирует вызывающий код.
package stats

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/schedule"
)

// ActivityStats — статистика по одному кружку за период.
type ActivityStats struct {
	Name string
	schedule.Counts
	Spent          float64 // цена занятия × посещено за период (см. PricePerLesson)
	PricePerLesson float64
	HasPrice       bool
}

// Report — статистика за период по всем кружкам плюс итоги.
type Report struct {
	From, To      time.Time
	Activities    []ActivityStats
	TotalSpent    float64
	TotalAttended int
}

// CostPerVisit возвращает цену одного занятия по кружку (см. payment.Store.PricePerLesson;
// false, если по кружку ещё нет ни одного платежа).
func (a ActivityStats) CostPerVisit() (float64, bool) {
	if !a.HasPrice {
		return 0, false
	}
	return a.PricePerLesson, true
}

// BuildReport считает план/факт по расписанию за [from, to] (обе даты включительно) и траты —
// как цену занятия (по всей истории платежей кружка) на фактически посещённые в периоде занятия.
func BuildReport(ctx context.Context, scheduleStore *schedule.Store, paymentStore *payment.Store, from, to time.Time) (Report, error) {
	counts, err := scheduleStore.StatsForRange(ctx, from, to)
	if err != nil {
		return Report{}, fmt.Errorf("статистика посещаемости: %w", err)
	}

	periodPayments, err := paymentStore.SumByActivity(ctx, from, to)
	if err != nil {
		return Report{}, fmt.Errorf("статистика трат: %w", err)
	}

	prices, err := paymentStore.PricePerLesson(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("статистика цены занятия: %w", err)
	}

	names := make(map[string]struct{}, len(counts)+len(periodPayments))
	for name := range counts {
		names[name] = struct{}{}
	}
	for name := range periodPayments {
		names[name] = struct{}{}
	}

	report := Report{From: from, To: to}
	for name := range names {
		as := ActivityStats{
			Name:   name,
			Counts: counts[name],
		}
		if price, ok := prices[name]; ok {
			as.PricePerLesson = price
			as.HasPrice = true
			as.Spent = price * float64(as.Attended)
		}
		report.Activities = append(report.Activities, as)
		report.TotalSpent += as.Spent
		report.TotalAttended += as.Attended
	}

	sort.Slice(report.Activities, func(i, j int) bool {
		return report.Activities[i].Name < report.Activities[j].Name
	})

	return report, nil
}
