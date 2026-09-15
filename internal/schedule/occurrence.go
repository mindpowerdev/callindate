package schedule

import (
	"context"
	"fmt"
	"time"
)

// Status — отметка посещаемости конкретного занятия в конкретную дату.
type Status string

const (
	StatusNotMarked   Status = "not_marked"
	StatusAttended    Status = "attended"
	StatusMissed      Status = "missed"
	StatusRescheduled Status = "rescheduled"
)

// Occurrence — конкретное проведение повторяющегося занятия в определённую дату.
type Occurrence struct {
	ID               int64
	ActivityID       int64
	Date             time.Time
	Status           Status
	HourReminderSent bool
}

// OccurrenceWithActivity — occurrence вместе с данными занятия, к которому он относится.
type OccurrenceWithActivity struct {
	Occurrence
	ActivityName string
	StartTime    string
}

// Counts — статистика по кружку за период: сколько занятий запланировано и как они отмечены.
type Counts struct {
	Planned     int
	Attended    int
	Missed      int
	Rescheduled int
}

func (s *Store) migrateOccurrences() error {
	const schema = `
CREATE TABLE IF NOT EXISTS occurrences (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	activity_id INTEGER NOT NULL REFERENCES activities(id),
	occurrence_date DATE NOT NULL,
	status TEXT NOT NULL DEFAULT 'not_marked',
	hour_reminder_sent INTEGER NOT NULL DEFAULT 0,
	UNIQUE(activity_id, occurrence_date)
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("миграция схемы посещаемости: %w", err)
	}
	return nil
}

// EnsureOccurrencesForDate создаёт (если их ещё нет) occurrence-записи для всех занятий,
// приходящихся на день недели указанной даты, и возвращает их вместе с данными занятий,
// отсортированными по времени начала.
func (s *Store) EnsureOccurrencesForDate(ctx context.Context, date time.Time) ([]OccurrenceWithActivity, error) {
	activities, err := s.ForWeekday(ctx, date.Weekday())
	if err != nil {
		return nil, err
	}

	dateStr := date.Format("2006-01-02")
	const insertQ = `INSERT INTO occurrences (activity_id, occurrence_date) VALUES (?, ?) ON CONFLICT(activity_id, occurrence_date) DO NOTHING`
	for _, a := range activities {
		if _, err := s.db.ExecContext(ctx, insertQ, a.ID, dateStr); err != nil {
			return nil, fmt.Errorf("создание occurrence: %w", err)
		}
	}

	return s.occurrencesForDate(ctx, dateStr)
}

func (s *Store) occurrencesForDate(ctx context.Context, dateStr string) ([]OccurrenceWithActivity, error) {
	const q = `
SELECT o.id, o.activity_id, o.occurrence_date, o.status, o.hour_reminder_sent, a.name, a.start_time
FROM occurrences o
JOIN activities a ON a.id = o.activity_id
WHERE o.occurrence_date = ?
ORDER BY a.start_time ASC
`
	rows, err := s.db.QueryContext(ctx, q, dateStr)
	if err != nil {
		return nil, fmt.Errorf("получение занятий на дату: %w", err)
	}
	defer rows.Close()

	var result []OccurrenceWithActivity
	for rows.Next() {
		var o OccurrenceWithActivity
		var status string
		var hourReminderSent int
		if err := rows.Scan(&o.ID, &o.ActivityID, &o.Date, &status, &hourReminderSent, &o.ActivityName, &o.StartTime); err != nil {
			return nil, fmt.Errorf("чтение occurrence: %w", err)
		}
		o.Status = Status(status)
		o.HourReminderSent = hourReminderSent != 0
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение занятий на дату: %w", err)
	}

	return result, nil
}

// SetOccurrenceStatus обновляет статус посещаемости и возвращает обновлённую запись.
func (s *Store) SetOccurrenceStatus(ctx context.Context, id int64, status Status) (OccurrenceWithActivity, error) {
	const q = `UPDATE occurrences SET status = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, string(status), id); err != nil {
		return OccurrenceWithActivity{}, fmt.Errorf("обновление статуса occurrence: %w", err)
	}

	const selectQ = `
SELECT o.id, o.activity_id, o.occurrence_date, o.status, o.hour_reminder_sent, a.name, a.start_time
FROM occurrences o
JOIN activities a ON a.id = o.activity_id
WHERE o.id = ?
`
	var o OccurrenceWithActivity
	var st string
	var hourReminderSent int
	err := s.db.QueryRowContext(ctx, selectQ, id).Scan(
		&o.ID, &o.ActivityID, &o.Date, &st, &hourReminderSent, &o.ActivityName, &o.StartTime,
	)
	if err != nil {
		return OccurrenceWithActivity{}, fmt.Errorf("чтение occurrence: %w", err)
	}
	o.Status = Status(st)
	o.HourReminderSent = hourReminderSent != 0

	return o, nil
}

// MarkHourReminderSent помечает, что напоминание за час до занятия уже отправлено.
func (s *Store) MarkHourReminderSent(ctx context.Context, id int64) error {
	const q = `UPDATE occurrences SET hour_reminder_sent = 1 WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("отметка напоминания за час: %w", err)
	}
	return nil
}

// StatsForRange считает план (по дням недели в диапазоне [from, to]) и факт (по статусам occurrence)
// для каждого кружка (группировка по имени занятия — один кружок может встречаться несколько раз в неделю).
func (s *Store) StatsForRange(ctx context.Context, from, to time.Time) (map[string]Counts, error) {
	activities, err := s.All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]Counts)

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		for _, a := range activities {
			if a.Weekday == d.Weekday() {
				c := result[a.Name]
				c.Planned++
				result[a.Name] = c
			}
		}
	}

	const q = `
SELECT a.name, o.status
FROM occurrences o
JOIN activities a ON a.id = o.activity_id
WHERE o.occurrence_date BETWEEN ? AND ?
`
	rows, err := s.db.QueryContext(ctx, q, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("получение статистики посещаемости: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			return nil, fmt.Errorf("чтение статистики посещаемости: %w", err)
		}
		c := result[name]
		switch Status(status) {
		case StatusAttended:
			c.Attended++
		case StatusMissed:
			c.Missed++
		case StatusRescheduled:
			c.Rescheduled++
		}
		result[name] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение статистики посещаемости: %w", err)
	}

	return result, nil
}
