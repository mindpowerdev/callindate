package bot

import (
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/mindpowerdev/callindate.git/internal/schedule"
)

func mainMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Календарь", "menu:calendar"),
			tgbotapi.NewInlineKeyboardButtonData("💳 Оплаты", "menu:payments"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Статистика", "menu:stats"),
			tgbotapi.NewInlineKeyboardButtonData("🔔 Напоминания", "menu:reminders"),
		),
	)
}

func backButtonRow() []tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "menu:main"),
	)
}

func calendarMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Сегодня", "cal:today"),
			tgbotapi.NewInlineKeyboardButtonData("📆 Завтра", "cal:tomorrow"),
		),
		backButtonRow(),
	)
}

func paymentsMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🧾 Последние платежи", "pay:recent"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Активные планы", "pay:plans"),
		),
		backButtonRow(),
	)
}

func statsMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Неделя", "stats:week"),
			tgbotapi.NewInlineKeyboardButtonData("Месяц", "stats:month"),
			tgbotapi.NewInlineKeyboardButtonData("Учебный год", "stats:year"),
		),
		backButtonRow(),
	)
}

func remindersMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(backButtonRow())
}

func attendanceKeyboard(occurrenceID int64) tgbotapi.InlineKeyboardMarkup {
	id := strconv.FormatInt(occurrenceID, 10)
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Посетили", "occ:attend:"+id),
			tgbotapi.NewInlineKeyboardButtonData("❌ Пропустили", "occ:miss:"+id),
			tgbotapi.NewInlineKeyboardButtonData("🔁 Перенесли", "occ:resched:"+id),
		),
	)
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	// Убираем индикатор загрузки на кнопке.
	if _, err := b.api.Request(tgbotapi.NewCallback(callback.ID, "")); err != nil {
		log.Printf("ошибка callback: %v", err)
	}

	chatID := callback.Message.Chat.ID
	data := callback.Data

	switch {
	case data == "menu:main":
		b.sendMenu(chatID, "Главное меню:", mainMenuKeyboard())

	case data == "menu:calendar":
		b.sendMenu(chatID, "📅 Календарь:", calendarMenuKeyboard())

	case data == "menu:payments":
		b.sendMenu(chatID, "💳 Оплаты:", paymentsMenuKeyboard())

	case data == "menu:stats":
		b.sendMenu(chatID, "📊 Статистика — выбери период:", statsMenuKeyboard())

	case data == "menu:reminders":
		b.reply(chatID, b.remindersText())
		b.sendMenu(chatID, "🔔 Напоминания:", remindersMenuKeyboard())

	case data == "cal:today":
		b.sendCalendarDay(chatID, b.today(), "📅 Сегодня", true)
		b.sendMenu(chatID, "📅 Календарь:", calendarMenuKeyboard())

	case data == "cal:tomorrow":
		b.sendCalendarDay(chatID, b.today().AddDate(0, 0, 1), "📆 Завтра", false)
		b.sendMenu(chatID, "📅 Календарь:", calendarMenuKeyboard())

	case data == "pay:recent":
		b.reply(chatID, b.paymentsText())
		b.sendMenu(chatID, "💳 Оплаты:", paymentsMenuKeyboard())

	case data == "pay:plans":
		b.reply(chatID, b.plansText())
		b.sendMenu(chatID, "💳 Оплаты:", paymentsMenuKeyboard())

	case data == "stats:week":
		b.sendStats(chatID, periodWeek)

	case data == "stats:month":
		b.sendStats(chatID, periodMonth)

	case data == "stats:year":
		b.sendStats(chatID, periodAcademicYear)

	case strings.HasPrefix(data, "occ:attend:"):
		b.handleAttendanceCallback(chatID, data, "occ:attend:", schedule.StatusAttended)

	case strings.HasPrefix(data, "occ:miss:"):
		b.handleAttendanceCallback(chatID, data, "occ:miss:", schedule.StatusMissed)

	case strings.HasPrefix(data, "occ:resched:"):
		b.handleAttendanceCallback(chatID, data, "occ:resched:", schedule.StatusRescheduled)

	default:
		b.reply(chatID, "Неизвестная команда.")
	}
}

func (b *Bot) sendMenu(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	b.send(msg)
}
