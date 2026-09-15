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
	Spent float64
}

// Report — статистика за период по всем кружкам плюс итоги.
type Report struct {
	From, To      time.Time
	Activities    []ActivityStats
	TotalSpent    float64
	TotalAttended int
}

// CostPerVisit возвращает среднюю стоимость одного фактического посещения по кружку
// (0, если посещений не было — делить не на что).
func (a ActivityStats) CostPerVisit() (float64, bool) {
	if a.Attended == 0 {
		return 0, false
	}
	return a.Spent / float64(a.Attended), true
}

// BuildReport считает план/факт по расписанию и траты по платежам за [from, to] (обе даты включительно).
func BuildReport(ctx context.Context, scheduleStore *schedule.Store, paymentStore *payment.Store, from, to time.Time) (Report, error) {
	counts, err := scheduleStore.StatsForRange(ctx, from, to)
	if err != nil {
		return Report{}, fmt.Errorf("статистика посещаемости: %w", err)
	}

	spent, err := paymentStore.SumByActivity(ctx, from, to)
	if err != nil {
		return Report{}, fmt.Errorf("статистика трат: %w", err)
	}

	names := make(map[string]struct{}, len(counts)+len(spent))
	for name := range counts {
		names[name] = struct{}{}
	}
	for name := range spent {
		names[name] = struct{}{}
	}

	report := Report{From: from, To: to}
	for name := range names {
		as := ActivityStats{
			Name:   name,
			Counts: counts[name],
			Spent:  spent[name],
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
