// Package settings хранит небольшие настройки бота (список чатов-подписчиков на напоминания,
// время напоминаний и т.д.). Данные в других пакетах (schedule, payment) общие для всех, кто
// пишет боту — это не «настройки пользователя», а именно общие параметры бота.
package settings

import (
	"context"
	"database/sql"
	"fmt"
)

const defaultDailyReminderTime = "09:00"

// Store — хранилище настроек на базе SQLite.
type Store struct {
	db *sql.DB
}

// NewStore оборачивает открытое соединение с БД и приводит схему настроек к актуальному виду.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.migrateSubscribers(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("миграция схемы настроек: %w", err)
	}
	return nil
}

// Get возвращает значение ключа и признак, что он задан.
func (s *Store) Get(ctx context.Context, key string) (string, bool, error) {
	const q = `SELECT value FROM settings WHERE key = ?`

	var value string
	err := s.db.QueryRowContext(ctx, q, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("чтение настройки %q: %w", key, err)
	}
	return value, true, nil
}

// Set сохраняет значение ключа, перезаписывая предыдущее.
func (s *Store) Set(ctx context.Context, key, value string) error {
	const q = `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	if _, err := s.db.ExecContext(ctx, q, key, value); err != nil {
		return fmt.Errorf("сохранение настройки %q: %w", key, err)
	}
	return nil
}

// Delete удаляет ключ, если он есть.
func (s *Store) Delete(ctx context.Context, key string) error {
	const q = `DELETE FROM settings WHERE key = ?`
	if _, err := s.db.ExecContext(ctx, q, key); err != nil {
		return fmt.Errorf("удаление настройки %q: %w", key, err)
	}
	return nil
}

const keyDailyReminderTime = "daily_reminder_time"

// DailyReminderTime возвращает время ежедневной сводки в формате "ЧЧ:ММ" (по умолчанию 09:00).
func (s *Store) DailyReminderTime(ctx context.Context) (string, error) {
	value, ok, err := s.Get(ctx, keyDailyReminderTime)
	if err != nil {
		return "", err
	}
	if !ok {
		return defaultDailyReminderTime, nil
	}
	return value, nil
}

// SetDailyReminderTime задаёт время ежедневной сводки в формате "ЧЧ:ММ".
func (s *Store) SetDailyReminderTime(ctx context.Context, hhmm string) error {
	return s.Set(ctx, keyDailyReminderTime, hhmm)
}

const keyLastDailyDigestDate = "last_daily_digest_date"

// LastDailyDigestDate возвращает дату последней отправленной сводки в формате "2006-01-02".
func (s *Store) LastDailyDigestDate(ctx context.Context) (string, bool, error) {
	return s.Get(ctx, keyLastDailyDigestDate)
}

// SetLastDailyDigestDate запоминает дату последней отправленной сводки.
func (s *Store) SetLastDailyDigestDate(ctx context.Context, date string) error {
	return s.Set(ctx, keyLastDailyDigestDate, date)
}
