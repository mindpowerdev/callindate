package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store — хранилище расписания занятий на базе SQLite.
type Store struct {
	db *sql.DB
}

// NewStore оборачивает открытое соединение с БД и приводит схему расписания к актуальному виду.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.migrateOccurrences(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS activities (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	weekday INTEGER NOT NULL,
	start_time TEXT NOT NULL,
	location TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("миграция схемы: %w", err)
	}
	return nil
}

// Add сохраняет новое занятие и возвращает его id.
func (s *Store) Add(ctx context.Context, a Activity) (int64, error) {
	const q = `INSERT INTO activities (name, weekday, start_time, location) VALUES (?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, q, a.Name, int(a.Weekday), a.StartTime, a.Location)
	if err != nil {
		return 0, fmt.Errorf("добавление занятия: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("получение id занятия: %w", err)
	}
	return id, nil
}

// All возвращает все занятия, отсортированные по дню недели и времени начала.
func (s *Store) All(ctx context.Context) ([]Activity, error) {
	const q = `SELECT id, name, weekday, start_time, location FROM activities ORDER BY weekday ASC, start_time ASC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("получение списка занятий: %w", err)
	}
	defer rows.Close()

	var result []Activity
	for rows.Next() {
		var a Activity
		var weekday int
		if err := rows.Scan(&a.ID, &a.Name, &weekday, &a.StartTime, &a.Location); err != nil {
			return nil, fmt.Errorf("чтение строки расписания: %w", err)
		}
		a.Weekday = time.Weekday(weekday)
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение расписания: %w", err)
	}

	return result, nil
}

// ForWeekday возвращает занятия для указанного дня недели, отсортированные по времени начала.
func (s *Store) ForWeekday(ctx context.Context, wd time.Weekday) ([]Activity, error) {
	const q = `SELECT id, name, weekday, start_time, location FROM activities WHERE weekday = ? ORDER BY start_time ASC`

	rows, err := s.db.QueryContext(ctx, q, int(wd))
	if err != nil {
		return nil, fmt.Errorf("получение расписания: %w", err)
	}
	defer rows.Close()

	var result []Activity
	for rows.Next() {
		var a Activity
		var weekday int
		if err := rows.Scan(&a.ID, &a.Name, &weekday, &a.StartTime, &a.Location); err != nil {
			return nil, fmt.Errorf("чтение строки расписания: %w", err)
		}
		a.Weekday = time.Weekday(weekday)
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение расписания: %w", err)
	}

	return result, nil
}
