package payment

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PlanType — способ оплаты кружка.
type PlanType string

const (
	PlanMonthly   PlanType = "monthly"   // ежемесячно: сумма и дата следующего платежа
	PlanSemester  PlanType = "semester"  // раз в несколько месяцев (учебный семестр)
	PlanAbonement PlanType = "abonement" // абонемент на определённое количество занятий
	PlanPerVisit  PlanType = "per_visit" // сумма за каждое фактически проведённое занятие
	PlanOneTime   PlanType = "one_time"  // разовый платёж на конкретную дату
)

// Plan — план оплаты кружка. Хранится по имени кружка (activity_name), а не по конкретному
// занятию в расписании: один кружок может встречаться в расписании несколько раз в неделю,
// но оплата у него одна.
type Plan struct {
	ActivityName       string
	Type               PlanType
	Amount             float64
	IntervalMonths     int // используется только для PlanSemester
	NextDueDate        *time.Time
	LessonsTotal       int
	LessonsRemaining   int
	DueReminderSentFor *time.Time
	LowBalanceReminded bool
}

// normalizeName приводит имя кружка к ключу для регистронезависимого сопоставления.
// SQLite-коллация COLLATE NOCASE сворачивает регистр только для ASCII, поэтому для кириллицы
// (все названия в этом боте — русские) сравнение делаем в Go через strings.ToLower, который
// корректно работает с юникодом.
func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (s *Store) migratePlans() error {
	const schema = `
CREATE TABLE IF NOT EXISTS payment_plans (
	name_key TEXT PRIMARY KEY,
	activity_name TEXT NOT NULL,
	type TEXT NOT NULL,
	amount REAL NOT NULL DEFAULT 0,
	interval_months INTEGER NOT NULL DEFAULT 0,
	next_due_date DATE,
	lessons_total INTEGER NOT NULL DEFAULT 0,
	lessons_remaining INTEGER NOT NULL DEFAULT 0,
	due_reminder_sent_for DATE,
	low_balance_reminded INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("миграция схемы планов оплаты: %w", err)
	}
	return nil
}

// UpsertPlan создаёт или полностью пересоздаёт план оплаты для кружка (сбрасывает флаги напоминаний).
func (s *Store) UpsertPlan(ctx context.Context, p Plan) error {
	const q = `
INSERT INTO payment_plans (
	name_key, activity_name, type, amount, interval_months, next_due_date,
	lessons_total, lessons_remaining, due_reminder_sent_for, low_balance_reminded
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 0)
ON CONFLICT(name_key) DO UPDATE SET
	activity_name = excluded.activity_name,
	type = excluded.type,
	amount = excluded.amount,
	interval_months = excluded.interval_months,
	next_due_date = excluded.next_due_date,
	lessons_total = excluded.lessons_total,
	lessons_remaining = excluded.lessons_remaining,
	due_reminder_sent_for = NULL,
	low_balance_reminded = 0
`
	_, err := s.db.ExecContext(ctx, q,
		normalizeName(p.ActivityName), p.ActivityName, string(p.Type), p.Amount, p.IntervalMonths, dateValue(p.NextDueDate),
		p.LessonsTotal, p.LessonsRemaining,
	)
	if err != nil {
		return fmt.Errorf("сохранение плана оплаты: %w", err)
	}
	return nil
}

// RemovePlan удаляет план оплаты кружка, если он есть.
func (s *Store) RemovePlan(ctx context.Context, activityName string) error {
	const q = `DELETE FROM payment_plans WHERE name_key = ?`
	if _, err := s.db.ExecContext(ctx, q, normalizeName(activityName)); err != nil {
		return fmt.Errorf("удаление плана оплаты: %w", err)
	}
	return nil
}

// PlanByName возвращает план оплаты кружка по имени (без учёта регистра), если он есть.
func (s *Store) PlanByName(ctx context.Context, activityName string) (*Plan, bool, error) {
	const q = `
SELECT activity_name, type, amount, interval_months, next_due_date,
	lessons_total, lessons_remaining, due_reminder_sent_for, low_balance_reminded
FROM payment_plans WHERE name_key = ?
`
	p, err := scanPlan(s.db.QueryRowContext(ctx, q, normalizeName(activityName)))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("получение плана оплаты: %w", err)
	}
	return p, true, nil
}

// AllPlans возвращает все планы оплаты.
func (s *Store) AllPlans(ctx context.Context) ([]Plan, error) {
	const q = `
SELECT activity_name, type, amount, interval_months, next_due_date,
	lessons_total, lessons_remaining, due_reminder_sent_for, low_balance_reminded
FROM payment_plans ORDER BY activity_name ASC
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("получение планов оплаты: %w", err)
	}
	defer rows.Close()

	var result []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение плана оплаты: %w", err)
		}
		result = append(result, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение планов оплаты: %w", err)
	}

	return result, nil
}

// AdvanceDue сдвигает дату следующего платежа для monthly/semester плана на его интервал
// (1 месяц для monthly, IntervalMonths для semester) и сбрасывает флаг отправленного напоминания.
func (s *Store) AdvanceDue(ctx context.Context, activityName string) error {
	plan, ok, err := s.PlanByName(ctx, activityName)
	if err != nil {
		return err
	}
	if !ok || plan.NextDueDate == nil {
		return nil
	}

	months := 1
	if plan.Type == PlanSemester && plan.IntervalMonths > 0 {
		months = plan.IntervalMonths
	}
	next := plan.NextDueDate.AddDate(0, months, 0)

	const q = `UPDATE payment_plans SET next_due_date = ?, due_reminder_sent_for = NULL WHERE name_key = ?`
	if _, err := s.db.ExecContext(ctx, q, dateValue(&next), normalizeName(activityName)); err != nil {
		return fmt.Errorf("сдвиг даты оплаты: %w", err)
	}
	return nil
}

// AddLessons прибавляет n занятий к остатку и общему количеству абонемента, сбрасывая напоминание о низком остатке.
func (s *Store) AddLessons(ctx context.Context, activityName string, n int) error {
	const q = `
UPDATE payment_plans
SET lessons_total = lessons_total + ?, lessons_remaining = lessons_remaining + ?, low_balance_reminded = 0
WHERE name_key = ?
`
	if _, err := s.db.ExecContext(ctx, q, n, n, normalizeName(activityName)); err != nil {
		return fmt.Errorf("пополнение абонемента: %w", err)
	}
	return nil
}

// DecrementLesson списывает одно занятие с остатка абонемента (не уходит ниже нуля).
func (s *Store) DecrementLesson(ctx context.Context, activityName string) error {
	const q = `
UPDATE payment_plans
SET lessons_remaining = MAX(lessons_remaining - 1, 0)
WHERE name_key = ?
`
	if _, err := s.db.ExecContext(ctx, q, normalizeName(activityName)); err != nil {
		return fmt.Errorf("списание занятия с абонемента: %w", err)
	}
	return nil
}

// MarkDueReminderSent запоминает, что напоминание об оплате на конкретную дату уже отправлено.
func (s *Store) MarkDueReminderSent(ctx context.Context, activityName string, dueDate time.Time) error {
	const q = `UPDATE payment_plans SET due_reminder_sent_for = ? WHERE name_key = ?`
	if _, err := s.db.ExecContext(ctx, q, dateValue(&dueDate), normalizeName(activityName)); err != nil {
		return fmt.Errorf("отметка напоминания об оплате: %w", err)
	}
	return nil
}

// SetLowBalanceReminded запоминает, отправлено ли напоминание о низком остатке абонемента.
func (s *Store) SetLowBalanceReminded(ctx context.Context, activityName string, reminded bool) error {
	const q = `UPDATE payment_plans SET low_balance_reminded = ? WHERE name_key = ?`
	value := 0
	if reminded {
		value = 1
	}
	if _, err := s.db.ExecContext(ctx, q, value, normalizeName(activityName)); err != nil {
		return fmt.Errorf("отметка напоминания об остатке: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlan(row rowScanner) (*Plan, error) {
	var p Plan
	var planType string
	var nextDue, dueReminderSentFor sql.NullTime
	var lowBalanceReminded int

	err := row.Scan(
		&p.ActivityName, &planType, &p.Amount, &p.IntervalMonths, &nextDue,
		&p.LessonsTotal, &p.LessonsRemaining, &dueReminderSentFor, &lowBalanceReminded,
	)
	if err != nil {
		return nil, err
	}

	p.Type = PlanType(planType)
	if nextDue.Valid {
		p.NextDueDate = &nextDue.Time
	}
	if dueReminderSentFor.Valid {
		p.DueReminderSentFor = &dueReminderSentFor.Time
	}
	p.LowBalanceReminded = lowBalanceReminded != 0

	return &p, nil
}

func dateValue(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}
