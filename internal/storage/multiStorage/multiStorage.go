package multiStorage

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/exp/slog"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/storage/mongodb"
	"url-shortener/internal/storage/sqlite"
)

type DualStorage struct {
	sqliteDB *sqlite.Storage
	mongoDB  *mongodb.Storage
}

// NewDualStorage создает экземпляр DualStorage для двух баз данных
func NewDualStorage(sqliteDB *sqlite.Storage, mongoDB *mongodb.Storage) *DualStorage {
	return &DualStorage{
		sqliteDB: sqliteDB,
		mongoDB:  mongoDB,
	}
}

// SaveURL сохраняет URL в обе базы данных
func (ds *DualStorage) SaveURL(ctx context.Context, log *slog.Logger, urlToSave, alias string, userID int64) error {
	log.Info("attempting to save URL", slog.String("alias", alias), slog.Int64("userID", userID))

	// Сначала записываем в SQLite
	if err := ds.sqliteDB.SaveURL(urlToSave, alias, userID); err != nil {
		log.Error("failed to save URL in SQLite", sl.Err(err))
		return err
	}

	// Затем записываем в MongoDB
	if _, err := ds.mongoDB.SaveURL(ctx, urlToSave, alias, userID); err != nil {
		log.Error("failed to save URL in MongoDB", sl.Err(err))
		return err
	}

	log.Info("URL successfully saved in both databases", slog.String("alias", alias))
	return nil
}

// GetURL получает URL по alias из MongoDB или SQLite
func (ds *DualStorage) GetURL(ctx context.Context, log *slog.Logger, alias string, userID int64) (string, error) {
	log.Info("attempting to retrieve URL", slog.String("alias", alias), slog.Int64("userID", userID))

	// Попробуем получить URL из SQLite
	url, err := ds.sqliteDB.GetURL(alias, userID)
	if err == nil {
		log.Info("URL found in SQLite", slog.String("alias", alias), slog.Int64("userID", userID))
		return url, nil
	}
	log.Error("failed to get URL from SQLite", slog.String("alias", alias), sl.Err(err))

	// Если в SQLite не нашлось, попробуем MongoDB
	url, err = ds.mongoDB.GetURL(ctx, alias, userID)
	if err != nil {
		log.Error("failed to get URL from MongoDB", slog.String("alias", alias), sl.Err(err))
		return "", err
	}

	log.Info("URL found in MongoDB", slog.String("alias", alias), slog.Int64("userID", userID))
	return url, nil
}

// DeleteURL удаляет URL из обеих баз данных
func (ds *DualStorage) DeleteURL(ctx context.Context, log *slog.Logger, alias string, userID int64) error {
	log.Info("attempting to delete URL", slog.String("alias", alias), slog.Int64("userID", userID))

	// Сначала удаляем из SQLite
	if err := ds.sqliteDB.DeleteURL(alias, userID); err != nil {
		log.Error("failed to delete URL from SQLite", slog.String("alias", alias), sl.Err(err))
		return err
	}

	// Затем удаляем из MongoDB
	if err := ds.mongoDB.DeleteURL(ctx, alias, userID); err != nil {
		log.Error("failed to delete URL from MongoDB", slog.String("alias", alias), sl.Err(err))
		return err
	}

	log.Info("URL successfully deleted from both databases", slog.String("alias", alias))
	return nil
}

// SaveUser сохраняет пользователя в обе базы данных
func (ds *DualStorage) SaveUser(ctx context.Context, log *slog.Logger, nickname, passwordHash string) error {
	log.Info("attempting to save user", slog.String("nickname", nickname))

	// Сначала сохраняем пользователя в SQLite
	userID, err := ds.sqliteDB.SaveUser(nickname, passwordHash)
	if err != nil {
		log.Error("failed to save user in SQLite", slog.String("nickname", nickname), sl.Err(err))
		return err
	}

	// Затем сохраняем пользователя в MongoDB
	if _, err := ds.mongoDB.SaveUser(ctx, nickname, passwordHash, userID); err != nil {
		log.Error("failed to save user in MongoDB", slog.String("nickname", nickname), sl.Err(err))
		return err
	}

	log.Info("user successfully saved in both databases", slog.String("nickname", nickname), slog.Int64("userID", userID))
	return nil
}

// GetUserByNickname получает пользователя из любой базы
func (ds *DualStorage) GetUserByNickname(ctx context.Context, log *slog.Logger, nickname string) (int64, string, error) {
	var userID int64

	log.Info("attempting to retrieve user", slog.String("nickname", nickname))

	// Сначала ищем пользователя в SQLite
	sqliteUserID, hash, errSqliteGetUser := ds.sqliteDB.GetUserByNickname(nickname)
	if errSqliteGetUser != nil {
		log.Error("failed to get user from SQLite", slog.String("nickname", nickname), sl.Err(errSqliteGetUser))
	}

	// Параллельно ищем пользователя в MongoDB
	mongoUserID, _, errMongoGetUser := ds.mongoDB.GetUserByNickname(ctx, nickname)
	if errMongoGetUser != nil {
		log.Error("failed to get user from MongoDB", slog.String("nickname", nickname), sl.Err(errMongoGetUser))
	}

	switch {
	case errSqliteGetUser == nil && errMongoGetUser == nil:
		// Оба запроса успешны
		userID = sqliteUserID
		log.Info("user found", slog.Int64("userID", userID), slog.String("nickname", nickname))
		return userID, hash, nil
	case errSqliteGetUser != nil && errMongoGetUser != nil:
		// Оба запроса завершились с ошибками
		log.Error("both databases returned errors", slog.String("nickname", nickname))
		return 0, "", fmt.Errorf("SQLite error: %v, MongoDB error: %v", errSqliteGetUser, errMongoGetUser)
	case errSqliteGetUser != nil:
		// Ошибка в SQLite, но успех в MongoDB
		userID = mongoUserID
		log.Info("user found in MongoDB", slog.Int64("userID", userID), slog.String("nickname", nickname))
		return userID, "", fmt.Errorf("SQLite error: %v", errSqliteGetUser)
	case errMongoGetUser != nil:
		// Ошибка в MongoDB, но успех в SQLite
		userID = sqliteUserID
		log.Info("user found in SQLite", slog.Int64("userID", userID), slog.String("nickname", nickname))
		return userID, hash, fmt.Errorf("MongoDB error: %v", errMongoGetUser)
	default:
		// Непредвиденная ситуация (теоретически не должна возникать)
		log.Error("unexpected error occurred", slog.String("nickname", nickname))
		return 0, "", errors.New("unexpected error")
	}
}

// DeleteUserByNickname удаляет пользователя из обеих баз данных
func (ds *DualStorage) DeleteUserByNickname(ctx context.Context, log *slog.Logger, nickname string) error {
	log.Info("attempting to delete user", slog.String("nickname", nickname))

	// Сначала удаляем пользователя из SQLite
	if err := ds.sqliteDB.DeleteUserByNickname(nickname); err != nil {
		log.Error("failed to delete user from SQLite", slog.String("nickname", nickname), sl.Err(err))
		return err
	}

	// Затем удаляем пользователя из MongoDB
	if err := ds.mongoDB.DeleteUserByNickname(ctx, nickname); err != nil {
		log.Error("failed to delete user from MongoDB", slog.String("nickname", nickname), sl.Err(err))
		return err
	}

	log.Info("user successfully deleted from both databases", slog.String("nickname", nickname))
	return nil
}
