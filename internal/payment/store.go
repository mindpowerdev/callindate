package payment

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store — хранилище истории платежей на базе SQLite.
type Store struct {
	db *sql.DB
}

// NewStore оборачивает открытое соединение с БД и приводит схему платежей к актуальному виду.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.migratePlans(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS payments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	activity_name TEXT NOT NULL DEFAULT '',
	amount REAL NOT NULL,
	lessons_count INTEGER NOT NULL,
	paid_at DATE NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("миграция схемы платежей: %w", err)
	}
	return nil
}

// Add сохраняет новую запись об оплате.
func (s *Store) Add(ctx context.Context, p Payment) error {
	const q = `INSERT INTO payments (activity_name, amount, lessons_count, paid_at) VALUES (?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q, p.ActivityName, p.Amount, p.LessonsCount, p.PaidAt.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("добавление платежа: %w", err)
	}
	return nil
}

// SumByActivity возвращает сумму платежей по каждому кружку за период [from, to] (обе даты включительно).
func (s *Store) SumByActivity(ctx context.Context, from, to time.Time) (map[string]float64, error) {
	const q = `
SELECT activity_name, SUM(amount)
FROM payments
WHERE paid_at BETWEEN ? AND ?
GROUP BY activity_name
`
	rows, err := s.db.QueryContext(ctx, q, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("получение сумм по кружкам: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var name string
		var sum float64
		if err := rows.Scan(&name, &sum); err != nil {
			return nil, fmt.Errorf("чтение сумм по кружкам: %w", err)
		}
		result[name] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение сумм по кружкам: %w", err)
	}

	return result, nil
}

// Recent возвращает последние платежи, отсортированные от новых к старым.
func (s *Store) Recent(ctx context.Context, limit int) ([]Payment, error) {
	const q = `SELECT id, activity_name, amount, lessons_count, paid_at FROM payments ORDER BY paid_at DESC, id DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("получение платежей: %w", err)
	}
	defer rows.Close()

	var result []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.ActivityName, &p.Amount, &p.LessonsCount, &p.PaidAt); err != nil {
			return nil, fmt.Errorf("чтение платежа: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение платежей: %w", err)
	}

	return result, nil
}
