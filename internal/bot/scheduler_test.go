package bot

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, value)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func TestDueDailyDigest(t *testing.T) {
	now := mustTime(t, "2006-01-02 15:04", "2026-09-15 09:05")

	if !dueDailyDigest(now, "09:00", "") {
		t.Fatal("должна отправиться: время наступило, сводки сегодня ещё не было")
	}
	if dueDailyDigest(now, "09:00", "2026-09-15") {
		t.Fatal("не должна отправиться: сводка на сегодня уже была")
	}
	if dueDailyDigest(now, "10:00", "") {
		t.Fatal("не должна отправиться: заданное время ещё не наступило")
	}
	// бот был выключен в 9:00, включился в 11:00 — сводка должна уйти в тот же день
	late := mustTime(t, "2006-01-02 15:04", "2026-09-15 11:00")
	if !dueDailyDigest(late, "09:00", "") {
		t.Fatal("сводка должна уйти при следующем тике после простоя, в тот же день")
	}
}

func TestHourReminderDue(t *testing.T) {
	now := mustTime(t, "2006-01-02 15:04", "2026-09-15 16:05")

	cases := []struct {
		name        string
		startAt     string
		alreadySent bool
		want        bool
	}{
		{"через 55 минут, не отправлено", "2026-09-15 17:00", false, true},
		{"уже отправлено", "2026-09-15 17:00", true, false},
		{"через 2 часа — рано", "2026-09-15 18:05", false, false},
		{"уже началось", "2026-09-15 16:00", false, false},
	}

	for _, c := range cases {
		startAt := mustTime(t, "2006-01-02 15:04", c.startAt)
		if got := hourReminderDue(now, startAt, c.alreadySent); got != c.want {
			t.Errorf("%s: hourReminderDue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDueSoon(t *testing.T) {
	now := mustTime(t, "2006-01-02", "2026-09-15")

	inTwoDays := now.AddDate(0, 0, 2)
	if !dueSoon(now, inTwoDays, nil, 3) {
		t.Fatal("должно напомнить: до оплаты 2 дня, порог 3")
	}

	inTenDays := now.AddDate(0, 0, 10)
	if dueSoon(now, inTenDays, nil, 3) {
		t.Fatal("не должно напомнить: до оплаты 10 дней")
	}

	remindedFor := inTwoDays
	if dueSoon(now, inTwoDays, &remindedFor, 3) {
		t.Fatal("не должно напомнить повторно про ту же дату")
	}

	newDue := now.AddDate(0, 0, 1)
	if !dueSoon(now, newDue, &remindedFor, 3) {
		t.Fatal("должно напомнить снова: дата оплаты изменилась после предыдущего напоминания")
	}
}

func TestAcademicYearStart(t *testing.T) {
	sep := mustTime(t, "2006-01-02", "2026-10-15")
	if got := academicYearStart(sep); got.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("academicYearStart(октябрь 2026) = %v", got)
	}

	spring := mustTime(t, "2006-01-02", "2026-03-01")
	if got := academicYearStart(spring); got.Format("2006-01-02") != "2025-09-01" {
		t.Fatalf("academicYearStart(март 2026) = %v", got)
	}
}
