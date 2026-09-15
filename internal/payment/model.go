package payment

import (
	"strconv"
	"strings"
	"time"
)

// Payment — запись об оплате: когда, сколько и за сколько занятий заплатили.
// Это просто учёт фактов, а не реальная обработка платежа.
type Payment struct {
	ID           int64
	ActivityName string // может быть пустым, если оплата не привязана к конкретному кружку
	Amount       float64
	LessonsCount int
	PaidAt       time.Time
}

// ParseAmount разбирает сумму, допуская запятую как десятичный разделитель и знак "₽".
func ParseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "₽")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSpace(s)

	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// FormatAmount форматирует сумму без лишних нулей после запятой.
func FormatAmount(a float64) string {
	if a == float64(int64(a)) {
		return strconv.FormatInt(int64(a), 10)
	}
	return strconv.FormatFloat(a, 'f', 2, 64)
}

// LessonsWord возвращает слово "занятие"/"занятия"/"занятий" в нужном падеже для числа n.
func LessonsWord(n int) string {
	if n < 0 {
		n = -n
	}

	rem100 := n % 100
	if rem100 >= 11 && rem100 <= 14 {
		return "занятий"
	}

	switch n % 10 {
	case 1:
		return "занятие"
	case 2, 3, 4:
		return "занятия"
	default:
		return "занятий"
	}
}
