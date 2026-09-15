package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/stats"
)

type period int

const (
	periodWeek period = iota
	periodMonth
	periodAcademicYear
)

func periodRange(p period, today time.Time) (from, to time.Time, label string) {
	switch p {
	case periodMonth:
		from = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		return from, today, "Статистика за месяц"
	case periodAcademicYear:
		return academicYearStart(today), today, "Статистика за учебный год"
	default:
		return today.AddDate(0, 0, -6), today, "Статистика за неделю"
	}
}

// academicYearStart возвращает 1 сентября учебного года, к которому относится дата today
// (с сентября по декабрь — 1 сентября текущего года, иначе — прошлого).
func academicYearStart(today time.Time) time.Time {
	year := today.Year()
	if today.Month() < time.September {
		year--
	}
	return time.Date(year, time.September, 1, 0, 0, 0, 0, today.Location())
}

func (b *Bot) sendStats(chatID int64, p period) {
	from, to, label := periodRange(p, b.today())

	ctx := context.Background()
	report, err := stats.BuildReport(ctx, b.schedule, b.payments, from, to)
	if err != nil {
		log.Printf("ошибка построения статистики: %v", err)
		b.reply(chatID, "Не получилось посчитать статистику, попробуй позже.")
		return
	}

	plans, err := b.payments.AllPlans(ctx)
	if err != nil {
		log.Printf("ошибка получения планов оплаты: %v", err)
	}

	b.reply(chatID, formatReport(report, label, plans))
	b.sendMenu(chatID, "📊 Статистика — выбери период:", statsMenuKeyboard())
}

func formatReport(report stats.Report, label string, plans []payment.Plan) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s — %s):\n\n", label, report.From.Format("02.01.2006"), report.To.Format("02.01.2006"))

	if len(report.Activities) == 0 {
		sb.WriteString("Данных за период нет.")
	} else {
		for _, a := range report.Activities {
			fmt.Fprintf(&sb, "• %s: план %d, посещено %d, пропущено %d, перенесено %d",
				a.Name, a.Planned, a.Attended, a.Missed, a.Rescheduled)
			if cost, ok := a.CostPerVisit(); ok {
				fmt.Fprintf(&sb, ", цена посещения %s ₽", payment.FormatAmount(cost))
			}
			if a.Spent > 0 {
				fmt.Fprintf(&sb, ", потрачено %s ₽", payment.FormatAmount(a.Spent))
			}
			sb.WriteString("\n")
		}
	}

	fmt.Fprintf(&sb, "\nВсего потрачено: %s ₽\nВсего посещений: %d", payment.FormatAmount(report.TotalSpent), report.TotalAttended)

	var abonementLines []string
	for _, p := range plans {
		if p.Type != payment.PlanAbonement {
			continue
		}
		abonementLines = append(abonementLines, fmt.Sprintf("%s — %d %s", p.ActivityName, p.LessonsRemaining, payment.LessonsWord(p.LessonsRemaining)))
	}
	if len(abonementLines) > 0 {
		sb.WriteString("\n\nОстаток по абонементам:\n")
		sb.WriteString(strings.Join(abonementLines, "\n"))
	}

	return sb.String()
}
