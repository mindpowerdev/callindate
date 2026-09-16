package main

import (
	"log"
	"os"
	"time"
	_ "time/tzdata" // зашивает базу часовых поясов IANA в бинарник — на минимальных Docker-образах (Railway и т.п.) её может не быть на диске

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/mindpowerdev/callindate.git/internal/bot"
	"github.com/mindpowerdev/callindate.git/internal/payment"
	"github.com/mindpowerdev/callindate.git/internal/schedule"
	"github.com/mindpowerdev/callindate.git/internal/settings"
	"github.com/mindpowerdev/callindate.git/internal/storage"
)

func main() {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("переменная окружения BOT_TOKEN не задана")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "calindate.db"
	}

	tz := os.Getenv("REMINDER_TZ")
	if tz == "" {
		tz = "Europe/Moscow"
	}
	location, err := time.LoadLocation(tz)
	if err != nil {
		log.Fatalf("ошибка часового пояса REMINDER_TZ=%q: %v", tz, err)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("ошибка открытия базы данных: %v", err)
	}
	defer db.Close()

	scheduleStore, err := schedule.NewStore(db)
	if err != nil {
		log.Fatalf("ошибка инициализации хранилища расписания: %v", err)
	}

	paymentStore, err := payment.NewStore(db)
	if err != nil {
		log.Fatalf("ошибка инициализации хранилища платежей: %v", err)
	}

	settingsStore, err := settings.NewStore(db)
	if err != nil {
		log.Fatalf("ошибка инициализации хранилища настроек: %v", err)
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("ошибка создания Telegram-бота: %v", err)
	}
	api.Debug = false

	log.Printf("Бот запущен: @%s", api.Self.UserName)

	bot.New(api, scheduleStore, paymentStore, settingsStore, location).Run()
}
