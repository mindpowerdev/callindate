package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/schedule"
)

const schedulerTick = time.Minute

// runScheduler — фоновый цикл: раз в минуту проверяет, не пора ли отправить одно из
// напоминаний. Устойчив к простою бота — решения принимаются не по точному совпадению
// времени тика, а по сравнению "уже отправлено/ещё нет" (см. dueDailyDigest, dueHourReminder и т.д.).
func (b *Bot) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()

	for {
		b.checkReminders(ctx, time.Now().In(b.location))
		<-ticker.C
	}
}

func (b *Bot) checkReminders(ctx context.Context, now time.Time) {
	chatID, ok, err := b.settings.ChatID(ctx)
	if err != nil {
		log.Printf("ошибка получения chat_id: %v", err)
		return
	}
	if !ok {
		return // бот ещё не запускали через /start — некому слать напоминания
	}

	b.checkDailyDigest(ctx, chatID, now)
	b.checkHourReminders(ctx, chatID, now)
	b.checkPaymentDue(ctx, chatID, now)
	b.checkLowBalance(ctx, chatID)
}

// dueDailyDigest — пора ли слать ежедневную сводку: текущее время не раньше настроенного,
// а сегодняшняя сводка ещё не отправлена. Не завязано на точное совпадение минуты — переживает простой бота.
func dueDailyDigest(now time.Time, dailyTime string, lastSentDate string) bool {
	today := now.Format("2006-01-02")
	if lastSentDate == today {
		return false
	}
	return now.Format("15:04") >= dailyTime
}

func (b *Bot) checkDailyDigest(ctx context.Context, chatID int64, now time.Time) {
	dailyTime, err := b.settings.DailyReminderTime(ctx)
	if err != nil {
		log.Printf("ошибка получения времени сводки: %v", err)
		return
	}
	lastSent, _, err := b.settings.LastDailyDigestDate(ctx)
	if err != nil {
		log.Printf("ошибка получения даты последней сводки: %v", err)
		return
	}
	if !dueDailyDigest(now, dailyTime, lastSent) {
		return
	}

	b.reply(chatID, b.dailyDigestText(ctx, now))

	if err := b.settings.SetLastDailyDigestDate(ctx, now.Format("2006-01-02")); err != nil {
		log.Printf("ошибка сохранения даты последней сводки: %v", err)
	}
}

func (b *Bot) dailyDigestText(ctx context.Context, now time.Time) string {
	date := startOfDay(now)
	occurrences, err := b.schedule.EnsureOccurrencesForDate(ctx, date)
	if err != nil {
		log.Printf("ошибка получения занятий на сегодня: %v", err)
		occurrences = nil
	}

	var sb strings.Builder
	sb.WriteString("🔔 Доброе утро! Вот сводка на сегодня.\n\n")

	if len(occurrences) == 0 {
		sb.WriteString("Занятий сегодня нет.")
	} else {
		sb.WriteString("Занятия:\n")
		for _, o := range occurrences {
			fmt.Fprintf(&sb, "%s — %s\n", o.StartTime, o.ActivityName)
		}
	}

	plans, err := b.payments.AllPlans(ctx)
	if err != nil {
		log.Printf("ошибка получения планов оплаты: %v", err)
		return sb.String()
	}
	var dueToday []payment.Plan
	for _, p := range plans {
		if p.NextDueDate != nil && sameDate(*p.NextDueDate, date) {
			dueToday = append(dueToday, p)
		}
	}
	if len(dueToday) > 0 {
		sb.WriteString("\n\nСегодня нужно оплатить:\n")
		for _, p := range dueToday {
			fmt.Fprintf(&sb, "%s — %s ₽\n", p.ActivityName, payment.FormatAmount(p.Amount))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// combineDateAndTime собирает дату occurrence и время начала занятия ("ЧЧ:ММ") в один time.Time
// в заданном часовом поясе.
func combineDateAndTime(date time.Time, hhmm string, loc *time.Location) (time.Time, bool) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, false
	}
	y, m, d := date.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, loc), true
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// hourReminderDue — пора ли слать напоминание за час: до начала осталось от 0 до 60 минут
// и напоминание ещё не отправлено. Не завязано на точную минуту — переживает пропущенные тики.
func hourReminderDue(now, startAt time.Time, alreadySent bool) bool {
	if alreadySent {
		return false
	}
	remaining := startAt.Sub(now)
	return remaining > 0 && remaining <= time.Hour
}

func (b *Bot) checkHourReminders(ctx context.Context, chatID int64, now time.Time) {
	occurrences, err := b.schedule.EnsureOccurrencesForDate(ctx, startOfDay(now))
	if err != nil {
		log.Printf("ошибка получения занятий на сегодня: %v", err)
		return
	}

	for _, o := range occurrences {
		if o.Status != schedule.StatusNotMarked {
			continue
		}
		startAt, ok := combineDateAndTime(o.Date, o.StartTime, b.location)
		if !ok {
			continue
		}
		if !hourReminderDue(now, startAt, o.HourReminderSent) {
			continue
		}

		b.reply(chatID, fmt.Sprintf("⏰ Через час: %s в %s", o.ActivityName, o.StartTime))
		if err := b.schedule.MarkHourReminderSent(ctx, o.ID); err != nil {
			log.Printf("ошибка отметки напоминания за час: %v", err)
		}
	}
}

// dueSoon — пора ли слать напоминание об оплате: срок в пределах dueSoonDays и ещё не напоминали
// про этот конкретный next_due_date (при продвижении плана дата меняется — напоминание всплывёт снова).
func dueSoon(now, nextDueDate time.Time, remindedFor *time.Time, withinDays int) bool {
	if remindedFor != nil && sameDate(*remindedFor, nextDueDate) {
		return false
	}
	daysLeft := int(startOfDay(nextDueDate).Sub(startOfDay(now)).Hours() / 24)
	return daysLeft >= 0 && daysLeft <= withinDays
}

func (b *Bot) checkPaymentDue(ctx context.Context, chatID int64, now time.Time) {
	plans, err := b.payments.AllPlans(ctx)
	if err != nil {
		log.Printf("ошибка получения планов оплаты: %v", err)
		return
	}

	for _, p := range plans {
		if p.NextDueDate == nil {
			continue
		}
		if !dueSoon(now, *p.NextDueDate, p.DueReminderSentFor, dueSoonDays) {
			continue
		}

		days := int(startOfDay(*p.NextDueDate).Sub(startOfDay(now)).Hours() / 24)
		b.reply(chatID, fmt.Sprintf("💳 %s — оплатить %s ₽ %s", p.ActivityName, payment.FormatAmount(p.Amount), dueInWords(days)))

		if err := b.payments.MarkDueReminderSent(ctx, p.ActivityName, *p.NextDueDate); err != nil {
			log.Printf("ошибка отметки напоминания об оплате: %v", err)
		}
	}
}

func dueInWords(daysLeft int) string {
	switch {
	case daysLeft <= 0:
		return "сегодня"
	case daysLeft == 1:
		return "завтра"
	default:
		return fmt.Sprintf("через %d дня(-ей)", daysLeft)
	}
}

func (b *Bot) checkLowBalance(ctx context.Context, chatID int64) {
	plans, err := b.payments.AllPlans(ctx)
	if err != nil {
		log.Printf("ошибка получения планов оплаты: %v", err)
		return
	}

	for _, p := range plans {
		if p.Type != payment.PlanAbonement || p.LowBalanceReminded {
			continue
		}
		if p.LessonsRemaining > lowBalanceThreshold {
			continue
		}

		b.reply(chatID, fmt.Sprintf(
			"⚠️ У «%s» осталось %d %s по абонементу.",
			p.ActivityName, p.LessonsRemaining, payment.LessonsWord(p.LessonsRemaining),
		))
		if err := b.payments.SetLowBalanceReminded(ctx, p.ActivityName, true); err != nil {
			log.Printf("ошибка отметки напоминания об остатке: %v", err)
		}
	}
}
