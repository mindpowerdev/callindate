package schedule

import (
	"strings"
	"time"
)

// Activity — занятие ребёнка, повторяющееся каждую неделю в один и тот же день и время.
type Activity struct {
	ID        int64
	Name      string
	Weekday   time.Weekday
	StartTime string // формат "15:04"
	Location  string
}

var weekdayNames = map[time.Weekday]string{
	time.Monday:    "Понедельник",
	time.Tuesday:   "Вторник",
	time.Wednesday: "Среда",
	time.Thursday:  "Четверг",
	time.Friday:    "Пятница",
	time.Saturday:  "Суббота",
	time.Sunday:    "Воскресенье",
}

// WeekdayName возвращает название дня недели на русском.
func WeekdayName(wd time.Weekday) string {
	return weekdayNames[wd]
}

var weekdayAliases = map[string]time.Weekday{
	"пн": time.Monday, "понедельник": time.Monday,
	"вт": time.Tuesday, "вторник": time.Tuesday,
	"ср": time.Wednesday, "среда": time.Wednesday,
	"чт": time.Thursday, "четверг": time.Thursday,
	"пт": time.Friday, "пятница": time.Friday,
	"сб": time.Saturday, "суббота": time.Saturday,
	"вс": time.Sunday, "воскресенье": time.Sunday,
}

// ParseWeekday разбирает день недели из короткого или полного русского названия.
func ParseWeekday(s string) (time.Weekday, bool) {
	wd, ok := weekdayAliases[strings.ToLower(strings.TrimSpace(s))]
	return wd, ok
}

// ParseStartTime проверяет и нормализует время занятия в формате "ЧЧ:ММ".
func ParseStartTime(s string) (string, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return "", false
	}
	return t.Format("15:04"), true
}
