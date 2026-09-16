package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/schedule"
)

func statusEmoji(status schedule.Status) string {
	switch status {
	case schedule.StatusAttended:
		return "✅"
	case schedule.StatusMissed:
		return "❌"
	case schedule.StatusRescheduled:
		return "🔁"
	default:
		return "❓"
	}
}

func statusLabel(status schedule.Status) string {
	switch status {
	case schedule.StatusAttended:
		return "Посетили"
	case schedule.StatusMissed:
		return "Пропустили"
	case schedule.StatusRescheduled:
		return "Перенесли"
	default:
		return "Ещё не отмечено"
	}
}

// sendCalendarDay показывает список занятий на дату. Если withAttendance — материализует
// occurrence-записи и добавляет отдельными сообщениями кнопки отметки посещаемости
// для ещё не отмеченных занятий (используется только для "Сегодня").
func (b *Bot) sendCalendarDay(chatID int64, date time.Time, label string, withAttendance bool) {
	ctx := context.Background()

	if !withAttendance {
		activities, err := b.schedule.ForWeekday(ctx, date.Weekday())
		if err != nil {
			log.Printf("ошибка получения расписания: %v", err)
			b.reply(chatID, "Не получилось получить расписание, попробуй позже.")
			return
		}
		b.reply(chatID, formatActivityList(activities, label, date.Weekday()))
		return
	}

	occurrences, err := b.schedule.EnsureOccurrencesForDate(ctx, date)
	if err != nil {
		log.Printf("ошибка получения занятий на дату: %v", err)
		b.reply(chatID, "Не получилось получить расписание, попробуй позже.")
		return
	}

	if len(occurrences) == 0 {
		b.reply(chatID, fmt.Sprintf("%s (%s) занятий нет.", label, schedule.WeekdayName(date.Weekday())))
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s):\n", label, schedule.WeekdayName(date.Weekday()))
	for _, o := range occurrences {
		fmt.Fprintf(&sb, "%s — %s %s\n", o.StartTime, o.ActivityName, statusEmoji(o.Status))
	}
	b.reply(chatID, strings.TrimRight(sb.String(), "\n"))

	for _, o := range occurrences {
		if o.Status != schedule.StatusNotMarked {
			continue
		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s — %s", o.StartTime, o.ActivityName))
		msg.ReplyMarkup = attendanceKeyboard(o.ID)
		b.send(msg)
	}
}

func formatActivityList(activities []schedule.Activity, label string, wd time.Weekday) string {
	if len(activities) == 0 {
		return fmt.Sprintf("%s (%s) занятий нет.", label, schedule.WeekdayName(wd))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s):\n", label, schedule.WeekdayName(wd))
	for _, a := range activities {
		fmt.Fprintf(&sb, "%s — %s\n", a.StartTime, a.Name)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (b *Bot) handleAttendanceCallback(chatID int64, data, prefix string, status schedule.Status) {
	idStr := strings.TrimPrefix(data, prefix)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Printf("некорректный id occurrence в callback %q: %v", data, err)
		b.reply(chatID, "Не получилось разобрать занятие, попробуй ещё раз.")
		return
	}

	ctx := context.Background()

	updated, err := b.schedule.SetOccurrenceStatus(ctx, id, status)
	if err != nil {
		log.Printf("ошибка обновления статуса занятия: %v", err)
		b.reply(chatID, "Не получилось сохранить отметку, попробуй ещё раз.")
		return
	}

	b.reply(chatID, fmt.Sprintf("Отмечено: %s — %s (%s)", updated.StartTime, updated.ActivityName, statusLabel(status)))

	if status != schedule.StatusAttended && status != schedule.StatusMissed {
		return
	}
	b.consumeAbonementLesson(ctx, updated.ActivityName)
}

// consumeAbonementLesson списывает одно занятие с абонемента кружка (если он на нём оформлен)
// и рассылает всем подписчикам предупреждение, когда остаток впервые опускается до порога
// (не только тому, кто отметил посещаемость, — это должны видеть оба родителя).
func (b *Bot) consumeAbonementLesson(ctx context.Context, activityName string) {
	plan, ok, err := b.payments.PlanByName(ctx, activityName)
	if err != nil {
		log.Printf("ошибка получения плана оплаты: %v", err)
		return
	}
	if !ok || plan.Type != payment.PlanAbonement {
		return
	}

	if err := b.payments.DecrementLesson(ctx, activityName); err != nil {
		log.Printf("ошибка списания занятия с абонемента: %v", err)
		return
	}

	updated, ok, err := b.payments.PlanByName(ctx, activityName)
	if err != nil || !ok {
		if err != nil {
			log.Printf("ошибка получения плана оплаты: %v", err)
		}
		return
	}

	if updated.LessonsRemaining <= lowBalanceThreshold && !updated.LowBalanceReminded {
		chatIDs, err := b.settings.Subscribers(ctx)
		if err != nil {
			log.Printf("ошибка получения подписчиков: %v", err)
		}
		b.broadcast(chatIDs, fmt.Sprintf(
			"⚠️ У «%s» осталось %d %s по абонементу.",
			activityName, updated.LessonsRemaining, payment.LessonsWord(updated.LessonsRemaining),
		))
		if err := b.payments.SetLowBalanceReminded(ctx, activityName, true); err != nil {
			log.Printf("ошибка отметки напоминания об остатке: %v", err)
		}
	}
}
