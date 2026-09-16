package settings

import (
	"context"
	"fmt"
)

func (s *Store) migrateSubscribers() error {
	const schema = `
CREATE TABLE IF NOT EXISTS subscribers (
	chat_id INTEGER PRIMARY KEY,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("миграция схемы подписчиков: %w", err)
	}
	return nil
}

// AddSubscriber добавляет чат в список получателей напоминаний (например, при /start).
// Повторное добавление того же чата ничего не меняет — так семья из нескольких людей
// (каждый пишет боту в своём приватном чате) получает напоминания все разом, а не только
// тот, кто запускал бота последним.
func (s *Store) AddSubscriber(ctx context.Context, chatID int64) error {
	const q = `INSERT INTO subscribers (chat_id) VALUES (?) ON CONFLICT(chat_id) DO NOTHING`
	if _, err := s.db.ExecContext(ctx, q, chatID); err != nil {
		return fmt.Errorf("добавление подписчика: %w", err)
	}
	return nil
}

// RemoveSubscriber убирает чат из списка получателей напоминаний (например, при /stop).
func (s *Store) RemoveSubscriber(ctx context.Context, chatID int64) error {
	const q = `DELETE FROM subscribers WHERE chat_id = ?`
	if _, err := s.db.ExecContext(ctx, q, chatID); err != nil {
		return fmt.Errorf("удаление подписчика: %w", err)
	}
	return nil
}

// Subscribers возвращает все чаты, которым нужно слать напоминания.
func (s *Store) Subscribers(ctx context.Context) ([]int64, error) {
	const q = `SELECT chat_id FROM subscribers ORDER BY chat_id ASC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("получение подписчиков: %w", err)
	}
	defer rows.Close()

	var result []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("чтение подписчика: %w", err)
		}
		result = append(result, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение подписчиков: %w", err)
	}

	return result, nil
}
