// Package bot содержит Telegram-обвязку: команды, меню, обработчики кнопок и фоновый
// планировщик напоминаний. Сама предметная логика (расписание, платежи, статистика) живёт
// в internal/schedule, internal/payment и internal/stats — этот пакет их вызывает и форматирует
// ответы на русском.
package bot

import (
	"context"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/schedule"
	"github.com/mindpowerdev/callindate.git/internal/settings"
)

// Bot связывает Telegram API с хранилищами расписания, платежей и настроек.
type Bot struct {
	api      *tgbotapi.BotAPI
	schedule *schedule.Store
	payments *payment.Store
	settings *settings.Store
	location *time.Location
}

// New создаёт бота поверх готового Telegram-клиента, хранилищ и часового пояса напоминаний.
func New(api *tgbotapi.BotAPI, scheduleStore *schedule.Store, paymentStore *payment.Store, settingsStore *settings.Store, location *time.Location) *Bot {
	return &Bot{
		api:      api,
		schedule: scheduleStore,
		payments: paymentStore,
		settings: settingsStore,
		location: location,
	}
}

// Run запускает фоновый планировщик напоминаний и (блокирующий) цикл получения обновлений.
func (b *Bot) Run() {
	go b.runScheduler(context.Background())

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := b.api.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		}

		if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
		}
	}
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	switch message.Command() {
	case "start":
		if err := b.settings.SetChatID(context.Background(), message.Chat.ID); err != nil {
			log.Printf("ошибка сохранения chat_id: %v", err)
		}

		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Привет! Я помогу вести учёт детских кружков и секций.",
		)
		msg.ReplyMarkup = mainMenuKeyboard()
		b.send(msg)

	case "help":
		b.reply(message.Chat.ID,
			"Используй /start, чтобы открыть главное меню.\n\n"+
				"Добавить занятие:\n/add <день> <ЧЧ:ММ> <название>\nНапример: /add Пн 17:00 Плавание\n\n"+
				"Записать оплату:\n/pay <сумма> <кол-во занятий> [название]\nНапример: /pay 5000 8 Плавание\n\n"+
				"Настроить план оплаты:\n"+
				"/plan monthly <сумма> <ДД.ММ.ГГГГ> <название>\n"+
				"/plan semester <сумма> <интервал_мес> <ДД.ММ.ГГГГ> <название>\n"+
				"/plan abonement <сумма> <кол-во_занятий> <название>\n"+
				"/plan per_visit <сумма_за_занятие> <название>\n"+
				"/plan one_time <сумма> <ДД.ММ.ГГГГ> <название>\n"+
				"/plan remove <название>\n\n"+
				"Время ежедневной сводки:\n/remind_time <ЧЧ:ММ>",
		)

	case "add":
		b.handleAddCommand(message)

	case "pay":
		b.handlePayCommand(message)

	case "plan":
		b.handlePlanCommand(message)

	case "remind_time":
		b.handleRemindTimeCommand(message)

	default:
		b.reply(message.Chat.ID, "Я пока понимаю только команды /start, /help, /add, /pay, /plan и /remind_time.")
	}
}

func (b *Bot) handleAddCommand(message *tgbotapi.Message) {
	args := strings.SplitN(strings.TrimSpace(message.CommandArguments()), " ", 3)
	if len(args) < 3 {
		b.reply(message.Chat.ID,
			"Формат: /add <день> <ЧЧ:ММ> <название>\nНапример: /add Пн 17:00 Плавание",
		)
		return
	}

	wd, ok := schedule.ParseWeekday(args[0])
	if !ok {
		b.reply(message.Chat.ID, "Не понял день недели. Используй: Пн, Вт, Ср, Чт, Пт, Сб, Вс.")
		return
	}

	startTime, ok := schedule.ParseStartTime(args[1])
	if !ok {
		b.reply(message.Chat.ID, "Время должно быть в формате ЧЧ:ММ, например 17:00.")
		return
	}

	name := strings.TrimSpace(args[2])
	if name == "" {
		b.reply(message.Chat.ID, "Не хватает названия занятия.")
		return
	}

	_, err := b.schedule.Add(context.Background(), schedule.Activity{
		Name:      name,
		Weekday:   wd,
		StartTime: startTime,
	})
	if err != nil {
		log.Printf("ошибка добавления занятия: %v", err)
		b.reply(message.Chat.ID, "Не получилось сохранить занятие, попробуй ещё раз.")
		return
	}

	b.reply(message.Chat.ID, "✅ Добавлено: "+schedule.WeekdayName(wd)+", "+startTime+" — "+name)
}

func activitySuffix(activityName string) string {
	if activityName == "" {
		return ""
	}
	return " (" + activityName + ")"
}

func (b *Bot) reply(chatID int64, text string) {
	b.send(tgbotapi.NewMessage(chatID, text))
}

func (b *Bot) send(msg tgbotapi.MessageConfig) {
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("ошибка отправки сообщения: %v", err)
	}
}

func (b *Bot) today() time.Time {
	return startOfDay(time.Now().In(b.location))
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// lowBalanceThreshold — при таком остатке занятий по абонементу (или ниже) шлём напоминание.
const lowBalanceThreshold = 2

// dueSoonDays — за сколько дней до next_due_date начинаем напоминать об оплате.
const dueSoonDays = 3
