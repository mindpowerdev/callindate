// Package settings хранит небольшие настройки бота (chat_id, время напоминаний и т.д.)
// в виде ключ-значение — бот рассчитан на один чат/семью, поэтому без пользователей и профилей.
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

const keyChatID = "chat_id"

// ChatID возвращает сохранённый чат бота (0, false — если бот ещё не запускали через /start).
func (s *Store) ChatID(ctx context.Context) (int64, bool, error) {
	value, ok, err := s.Get(ctx, keyChatID)
	if err != nil || !ok {
		return 0, false, err
	}
	var id int64
	if _, err := fmt.Sscanf(value, "%d", &id); err != nil {
		return 0, false, fmt.Errorf("разбор chat_id: %w", err)
	}
	return id, true, nil
}

// SetChatID сохраняет чат, в который бот шлёт напоминания.
func (s *Store) SetChatID(ctx context.Context, chatID int64) error {
	return s.Set(ctx, keyChatID, fmt.Sprintf("%d", chatID))
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
