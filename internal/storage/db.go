// Package storage открывает общее соединение с базой данных SQLite,
// которым пользуются доменные хранилища (schedule, payment и т.д.).
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open открывает (и при необходимости создаёт) файл базы данных SQLite по указанному пути.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("открытие БД: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("проверка соединения с БД: %w", err)
	}

	return db, nil
}
