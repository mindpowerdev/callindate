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

const recentPaymentsLimit = 10

func (b *Bot) handlePayCommand(message *tgbotapi.Message) {
	args := strings.SplitN(strings.TrimSpace(message.CommandArguments()), " ", 3)
	if len(args) < 2 {
		b.reply(message.Chat.ID,
			"Формат: /pay <сумма> <кол-во занятий> [название]\nНапример: /pay 5000 8 Плавание",
		)
		return
	}

	amount, ok := payment.ParseAmount(args[0])
	if !ok {
		b.reply(message.Chat.ID, "Сумма должна быть положительным числом, например 5000 или 5000.50.")
		return
	}

	lessons, err := strconv.Atoi(args[1])
	if err != nil || lessons <= 0 {
		b.reply(message.Chat.ID, "Количество занятий должно быть положительным целым числом.")
		return
	}

	var activityName string
	if len(args) == 3 {
		activityName = strings.TrimSpace(args[2])
	}

	ctx := context.Background()

	err = b.payments.Add(ctx, payment.Payment{
		ActivityName: activityName,
		Amount:       amount,
		LessonsCount: lessons,
		PaidAt:       time.Now(),
	})
	if err != nil {
		log.Printf("ошибка добавления платежа: %v", err)
		b.reply(message.Chat.ID, "Не получилось сохранить платёж, попробуй ещё раз.")
		return
	}

	b.reply(message.Chat.ID, fmt.Sprintf(
		"✅ Записано: %s ₽ за %d %s%s",
		payment.FormatAmount(amount), lessons, payment.LessonsWord(lessons), activitySuffix(activityName),
	))

	if activityName == "" {
		return
	}
	b.advancePlanAfterPayment(ctx, activityName, lessons)
}

// advancePlanAfterPayment продвигает план оплаты кружка после ручной записи платежа через /pay:
// у monthly/semester — сдвигает дату следующего платежа, у abonement — пополняет остаток занятий.
func (b *Bot) advancePlanAfterPayment(ctx context.Context, activityName string, lessons int) {
	plan, ok, err := b.payments.PlanByName(ctx, activityName)
	if err != nil {
		log.Printf("ошибка получения плана оплаты: %v", err)
		return
	}
	if !ok {
		return
	}

	switch plan.Type {
	case payment.PlanMonthly, payment.PlanSemester:
		if err := b.payments.AdvanceDue(ctx, activityName); err != nil {
			log.Printf("ошибка сдвига даты оплаты: %v", err)
		}
	case payment.PlanAbonement:
		if err := b.payments.AddLessons(ctx, activityName, lessons); err != nil {
			log.Printf("ошибка пополнения абонемента: %v", err)
		}
	}
}

func (b *Bot) paymentsText() string {
	payments, err := b.payments.Recent(context.Background(), recentPaymentsLimit)
	if err != nil {
		log.Printf("ошибка получения платежей: %v", err)
		return "Не получилось получить историю платежей, попробуй позже."
	}

	if len(payments) == 0 {
		return "💳 Платежей пока нет.\nЧтобы добавить: /pay <сумма> <кол-во занятий> [название]"
	}

	var sb strings.Builder
	sb.WriteString("💳 Последние платежи:\n")
	for _, p := range payments {
		fmt.Fprintf(&sb, "%s — %s ₽ (%d %s)%s\n",
			p.PaidAt.Format("02.01.2006"), payment.FormatAmount(p.Amount), p.LessonsCount,
			payment.LessonsWord(p.LessonsCount), activitySuffix(p.ActivityName),
		)
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (b *Bot) handlePlanCommand(message *tgbotapi.Message) {
	raw := strings.TrimSpace(message.CommandArguments())
	head := strings.SplitN(raw, " ", 2)
	if len(head) < 2 {
		b.reply(message.Chat.ID, planUsage)
		return
	}
	planType := strings.ToLower(head[0])
	rest := head[1]

	switch planType {
	case "monthly":
		b.handlePlanMonthly(message.Chat.ID, rest)
	case "semester":
		b.handlePlanSemester(message.Chat.ID, rest)
	case "abonement":
		b.handlePlanAbonement(message.Chat.ID, rest)
	case "per_visit":
		b.handlePlanPerVisit(message.Chat.ID, rest)
	case "one_time":
		b.handlePlanOneTime(message.Chat.ID, rest)
	case "remove":
		b.handlePlanRemove(message.Chat.ID, rest)
	default:
		b.reply(message.Chat.ID, planUsage)
	}
}

const planUsage = "Формат:\n" +
	"/plan monthly <сумма> <ДД.ММ.ГГГГ> <название>\n" +
	"/plan semester <сумма> <интервал_мес> <ДД.ММ.ГГГГ> <название>\n" +
	"/plan abonement <сумма> <кол-во_занятий> <название>\n" +
	"/plan per_visit <сумма_за_занятие> <название>\n" +
	"/plan one_time <сумма> <ДД.ММ.ГГГГ> <название>\n" +
	"/plan remove <название>"

func (b *Bot) parseDate(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("02.01.2006", strings.TrimSpace(s), b.location)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (b *Bot) handlePlanMonthly(chatID int64, rest string) {
	fields := strings.SplitN(rest, " ", 3)
	if len(fields) < 3 {
		b.reply(chatID, planUsage)
		return
	}
	amount, ok := payment.ParseAmount(fields[0])
	if !ok {
		b.reply(chatID, "Сумма должна быть положительным числом.")
		return
	}
	due, ok := b.parseDate(fields[1])
	if !ok {
		b.reply(chatID, "Дата должна быть в формате ДД.ММ.ГГГГ.")
		return
	}
	name := strings.TrimSpace(fields[2])
	if name == "" {
		b.reply(chatID, "Не хватает названия кружка.")
		return
	}

	err := b.payments.UpsertPlan(context.Background(), payment.Plan{
		ActivityName: name, Type: payment.PlanMonthly, Amount: amount, NextDueDate: &due,
	})
	if err != nil {
		log.Printf("ошибка сохранения плана оплаты: %v", err)
		b.reply(chatID, "Не получилось сохранить план оплаты.")
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ План «%s»: ежемесячно, %s ₽, следующий платёж %s",
		name, payment.FormatAmount(amount), due.Format("02.01.2006")))
}

func (b *Bot) handlePlanSemester(chatID int64, rest string) {
	fields := strings.SplitN(rest, " ", 4)
	if len(fields) < 4 {
		b.reply(chatID, planUsage)
		return
	}
	amount, ok := payment.ParseAmount(fields[0])
	if !ok {
		b.reply(chatID, "Сумма должна быть положительным числом.")
		return
	}
	interval, err := strconv.Atoi(fields[1])
	if err != nil || interval <= 0 {
		b.reply(chatID, "Интервал должен быть положительным целым числом месяцев.")
		return
	}
	due, ok := b.parseDate(fields[2])
	if !ok {
		b.reply(chatID, "Дата должна быть в формате ДД.ММ.ГГГГ.")
		return
	}
	name := strings.TrimSpace(fields[3])
	if name == "" {
		b.reply(chatID, "Не хватает названия кружка.")
		return
	}

	err = b.payments.UpsertPlan(context.Background(), payment.Plan{
		ActivityName: name, Type: payment.PlanSemester, Amount: amount,
		IntervalMonths: interval, NextDueDate: &due,
	})
	if err != nil {
		log.Printf("ошибка сохранения плана оплаты: %v", err)
		b.reply(chatID, "Не получилось сохранить план оплаты.")
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ План «%s»: раз в %d мес., %s ₽, следующий платёж %s",
		name, interval, payment.FormatAmount(amount), due.Format("02.01.2006")))
}

func (b *Bot) handlePlanAbonement(chatID int64, rest string) {
	fields := strings.SplitN(rest, " ", 3)
	if len(fields) < 3 {
		b.reply(chatID, planUsage)
		return
	}
	amount, ok := payment.ParseAmount(fields[0])
	if !ok {
		b.reply(chatID, "Сумма должна быть положительным числом.")
		return
	}
	lessons, err := strconv.Atoi(fields[1])
	if err != nil || lessons <= 0 {
		b.reply(chatID, "Количество занятий должно быть положительным целым числом.")
		return
	}
	name := strings.TrimSpace(fields[2])
	if name == "" {
		b.reply(chatID, "Не хватает названия кружка.")
		return
	}

	err = b.payments.UpsertPlan(context.Background(), payment.Plan{
		ActivityName: name, Type: payment.PlanAbonement, Amount: amount,
		LessonsTotal: lessons, LessonsRemaining: lessons,
	})
	if err != nil {
		log.Printf("ошибка сохранения плана оплаты: %v", err)
		b.reply(chatID, "Не получилось сохранить план оплаты.")
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ План «%s»: абонемент на %d %s за %s ₽",
		name, lessons, payment.LessonsWord(lessons), payment.FormatAmount(amount)))
}

func (b *Bot) handlePlanPerVisit(chatID int64, rest string) {
	fields := strings.SplitN(rest, " ", 2)
	if len(fields) < 2 {
		b.reply(chatID, planUsage)
		return
	}
	amount, ok := payment.ParseAmount(fields[0])
	if !ok {
		b.reply(chatID, "Сумма должна быть положительным числом.")
		return
	}
	name := strings.TrimSpace(fields[1])
	if name == "" {
		b.reply(chatID, "Не хватает названия кружка.")
		return
	}

	err := b.payments.UpsertPlan(context.Background(), payment.Plan{
		ActivityName: name, Type: payment.PlanPerVisit, Amount: amount,
	})
	if err != nil {
		log.Printf("ошибка сохранения плана оплаты: %v", err)
		b.reply(chatID, "Не получилось сохранить план оплаты.")
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ План «%s»: %s ₽ за посещение", name, payment.FormatAmount(amount)))
}

func (b *Bot) handlePlanOneTime(chatID int64, rest string) {
	fields := strings.SplitN(rest, " ", 3)
	if len(fields) < 3 {
		b.reply(chatID, planUsage)
		return
	}
	amount, ok := payment.ParseAmount(fields[0])
	if !ok {
		b.reply(chatID, "Сумма должна быть положительным числом.")
		return
	}
	due, ok := b.parseDate(fields[1])
	if !ok {
		b.reply(chatID, "Дата должна быть в формате ДД.ММ.ГГГГ.")
		return
	}
	name := strings.TrimSpace(fields[2])
	if name == "" {
		b.reply(chatID, "Не хватает названия кружка.")
		return
	}

	err := b.payments.UpsertPlan(context.Background(), payment.Plan{
		ActivityName: name, Type: payment.PlanOneTime, Amount: amount, NextDueDate: &due,
	})
	if err != nil {
		log.Printf("ошибка сохранения плана оплаты: %v", err)
		b.reply(chatID, "Не получилось сохранить план оплаты.")
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ План «%s»: разовая оплата %s ₽ к %s",
		name, payment.FormatAmount(amount), due.Format("02.01.2006")))
}

func (b *Bot) handlePlanRemove(chatID int64, rest string) {
	name := strings.TrimSpace(rest)
	if name == "" {
		b.reply(chatID, planUsage)
		return
	}
	if err := b.payments.RemovePlan(context.Background(), name); err != nil {
		log.Printf("ошибка удаления плана оплаты: %v", err)
		b.reply(chatID, "Не получилось удалить план оплаты.")
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ План «%s» удалён.", name))
}

func (b *Bot) plansText() string {
	plans, err := b.payments.AllPlans(context.Background())
	if err != nil {
		log.Printf("ошибка получения планов оплаты: %v", err)
		return "Не получилось получить планы оплаты, попробуй позже."
	}

	if len(plans) == 0 {
		return "📋 Активных планов оплаты нет.\n" + planUsage
	}

	var sb strings.Builder
	sb.WriteString("📋 Активные планы:\n")
	for _, p := range plans {
		sb.WriteString(formatPlanLine(p))
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func formatPlanLine(p payment.Plan) string {
	amount := payment.FormatAmount(p.Amount)
	switch p.Type {
	case payment.PlanMonthly:
		return fmt.Sprintf("%s — ежемесячно, %s ₽, следующий платёж %s", p.ActivityName, amount, formatDate(p.NextDueDate))
	case payment.PlanSemester:
		return fmt.Sprintf("%s — раз в %d мес., %s ₽, следующий платёж %s", p.ActivityName, p.IntervalMonths, amount, formatDate(p.NextDueDate))
	case payment.PlanAbonement:
		return fmt.Sprintf("%s — абонемент, осталось %d %s (из %d)", p.ActivityName, p.LessonsRemaining, payment.LessonsWord(p.LessonsRemaining), p.LessonsTotal)
	case payment.PlanPerVisit:
		return fmt.Sprintf("%s — %s ₽ за посещение", p.ActivityName, amount)
	case payment.PlanOneTime:
		return fmt.Sprintf("%s — разовая оплата %s ₽ к %s", p.ActivityName, amount, formatDate(p.NextDueDate))
	default:
		return fmt.Sprintf("%s — %s", p.ActivityName, amount)
	}
}

func formatDate(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("02.01.2006")
}

func (b *Bot) remindersText() string {
	dailyTime, err := b.settings.DailyReminderTime(context.Background())
	if err != nil {
		log.Printf("ошибка получения времени напоминаний: %v", err)
		dailyTime = "09:00"
	}
	return fmt.Sprintf(
		"🔔 Ежедневная сводка приходит в %s (часовой пояс %s).\n"+
			"Также напоминаю за час до начала занятия, за %d дня до оплаты и когда в абонементе остаётся мало занятий.\n\n"+
			"Изменить время сводки: /remind_time <ЧЧ:ММ>",
		dailyTime, b.location.String(), dueSoonDays,
	)
}

func (b *Bot) handleRemindTimeCommand(message *tgbotapi.Message) {
	arg := strings.TrimSpace(message.CommandArguments())
	hhmm, ok := schedule.ParseStartTime(arg)
	if !ok {
		b.reply(message.Chat.ID, "Формат: /remind_time <ЧЧ:ММ>\nНапример: /remind_time 09:00")
		return
	}

	if err := b.settings.SetDailyReminderTime(context.Background(), hhmm); err != nil {
		log.Printf("ошибка сохранения времени напоминаний: %v", err)
		b.reply(message.Chat.ID, "Не получилось сохранить время, попробуй ещё раз.")
		return
	}

	b.reply(message.Chat.ID, fmt.Sprintf("✅ Ежедневная сводка теперь приходит в %s.", hhmm))
}
