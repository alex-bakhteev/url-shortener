package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/mattn/go-sqlite3"
	"url-shortener/internal/storage"
)

type Storage struct {
	db *sql.DB
}

func New(storagePath string) (*Storage, error) {
	const op = "storage.sqlite.New"

	db, err := sql.Open("sqlite3", storagePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Создание таблиц пользователей и URL
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users(
			id INTEGER PRIMARY KEY,
			nickname TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS urls(
			id INTEGER PRIMARY KEY,
			alias TEXT NOT NULL UNIQUE,
			url TEXT NOT NULL,
			user_id INTEGER,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Создание индекса для ускорения поиска по alias
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_alias ON urls(alias);
	`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Storage{db: db}, nil
}

// Метод для сохранения URL с проверкой существования пользователя
func (s *Storage) SaveURL(urlToSave, alias string, userID int64) error {
	const op = "storage.sqlite.SaveURL"

	stmt, err := s.db.Prepare(`
		INSERT INTO urls (url, alias, user_id)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("%s: prepare statement: %w", op, err)
	}
	defer stmt.Close()

	res, err := stmt.Exec(urlToSave, alias, userID)
	if err != nil {
		if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return fmt.Errorf("%s: %w", op, storage.ErrURLExists)
		}
		return fmt.Errorf("%s: exec statement: %w", op, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("%s: failed to get last insert id: %d. %w", op, id, err)
	}

	return nil
}

// Метод для получения URL по алиасу с проверкой принадлежности alias указанному пользователю
func (s *Storage) GetURL(alias string, userID int64) (string, error) {
	const op = "storage.sqlite.GetURL"

	// Сначала проверяем, существует ли alias в базе
	stmtCheckExistence, err := s.db.Prepare("SELECT 1 FROM urls WHERE alias = ?")
	if err != nil {
		return "", fmt.Errorf("%s: prepare existence check statement: %w", op, err)
	}
	defer stmtCheckExistence.Close()

	var exists int
	err = stmtCheckExistence.QueryRow(alias).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Если alias вообще не существует в базе
			return "", storage.ErrURLNotFound
		}
		return "", fmt.Errorf("%s: execute existence check statement: %w", op, err)
	}

	// Если alias существует, проверяем принадлежность alias пользователю
	stmtCheckOwnership, err := s.db.Prepare("SELECT user_id FROM urls WHERE alias = ?")
	if err != nil {
		return "", fmt.Errorf("%s: prepare ownership check statement: %w", op, err)
	}
	defer stmtCheckOwnership.Close()

	var dbUserID int64
	err = stmtCheckOwnership.QueryRow(alias).Scan(&dbUserID)
	if err != nil {
		return "", fmt.Errorf("%s: execute ownership check statement: %w", op, err)
	}

	// Если alias принадлежит другому пользователю
	if dbUserID != userID {
		return "", storage.ErrUnauthorized
	}

	// Получаем URL, если alias принадлежит указанному пользователю
	stmtGetURL, err := s.db.Prepare("SELECT url FROM urls WHERE alias = ? AND user_id = ?")
	if err != nil {
		return "", fmt.Errorf("%s: prepare get URL statement: %w", op, err)
	}
	defer stmtGetURL.Close()

	var resURL string
	err = stmtGetURL.QueryRow(alias, userID).Scan(&resURL)
	if err != nil {
		return "", fmt.Errorf("%s: execute get URL statement: %w", op, err)
	}

	return resURL, nil
}

// Метод для удаления URL по алиасу и проверке владельца (user_id)
func (s *Storage) DeleteURL(alias string, userID int64) error {
	const op = "storage.sqlite.DeleteURL"

	var dbUserID int64
	err := s.db.QueryRow("SELECT user_id FROM urls WHERE alias = ?", alias).Scan(&dbUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s: url not found: %w", op, storage.ErrURLNotFound)
		}
		return fmt.Errorf("%s: query error: %w", op, err)
	}

	if dbUserID != userID {
		return fmt.Errorf("%s: unauthorized: %w", op, storage.ErrUnauthorized)
	}

	stmt, err := s.db.Prepare("DELETE FROM urls WHERE alias = ?")
	if err != nil {
		return fmt.Errorf("%s: prepare statement: %w", op, err)
	}

	_, err = stmt.Exec(alias)
	if err != nil {
		return fmt.Errorf("%s: execute statement: %w", op, err)
	}

	return nil
}

// Метод для сохранения пользователя
func (s *Storage) SaveUser(nickname, passwordHash string) (int64, error) {
	const op = "storage.sqlite.SaveUser"

	stmt, err := s.db.Prepare("INSERT INTO users(nickname, password_hash) VALUES(?, ?)")
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	// Выполняем запрос
	res, err := stmt.Exec(nickname, passwordHash)
	if err != nil {
		// Проверяем на уникальное ограничение
		if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return 0, fmt.Errorf("%s: %w", op, storage.ErrUserExists)
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	// Получаем ID последней вставленной записи
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("%s: failed to get last insert id: %w", op, err)
	}

	return id, nil
}

// Метод для получения пользователя по никнейму
func (s *Storage) GetUserByNickname(nickname string) (int64, string, error) {
	const op = "storage.sqlite.GetUserByNickname"

	stmt, err := s.db.Prepare("SELECT id, password_hash FROM users WHERE nickname = ?")
	if err != nil {
		return 0, "", fmt.Errorf("%s: prepare statement: %w", op, err)
	}

	var id int64
	var passwordHash string

	err = stmt.QueryRow(nickname).Scan(&id, &passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", storage.ErrUserNotFound
		}
		return 0, "", fmt.Errorf("%s: execute statement: %w", op, err)
	}

	return id, passwordHash, nil
}

// Метод для удаления пользователя и связанных URL по user_id
func (s *Storage) DeleteUserByNickname(nickname string) error {
	const op = "storage.sqlite.DeleteUserByNickname"

	// Начало транзакции
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	// Получение userID по nickname
	var userID int64
	err = tx.QueryRow("SELECT id FROM users WHERE nickname = ?", nickname).Scan(&userID)
	if err != nil {
		return fmt.Errorf("%s: execute get user ID statement: %w", op, err)
	}

	// Проверяем наличие связанных URL для этого пользователя
	var urlCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM urls WHERE user_id = ?", userID).Scan(&urlCount)
	if err != nil {
		return fmt.Errorf("%s: failed to count user URLs: %w", op, err)
	}

	if urlCount == 0 {
		return fmt.Errorf("%s: no URLs found for user", op)
	}

	// Удаление всех URL, связанных с пользователем
	stmtDeleteURLs, err := tx.Prepare("DELETE FROM urls WHERE user_id = ?")
	if err != nil {
		return fmt.Errorf("%s: prepare delete URLs statement: %w", op, err)
	}
	defer stmtDeleteURLs.Close()

	_, err = stmtDeleteURLs.Exec(userID)
	if err != nil {
		return fmt.Errorf("%s: execute delete URLs statement: %w", op, err)
	}

	// Удаление пользователя
	stmtDeleteUser, err := tx.Prepare("DELETE FROM users WHERE id = ?")
	if err != nil {
		return fmt.Errorf("%s: prepare delete user statement: %w", op, err)
	}
	defer stmtDeleteUser.Close()

	_, err = stmtDeleteUser.Exec(userID)
	if err != nil {
		return fmt.Errorf("%s: execute delete user statement: %w", op, err)
	}

	// Завершение транзакции
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: failed to commit transaction: %w", op, err)
	}

	return nil
}
