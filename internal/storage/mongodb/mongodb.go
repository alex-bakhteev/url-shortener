package mongodb

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"url-shortener/internal/storage"
)

type Storage struct {
	db *mongo.Database
}

// NewClient создает новое хранилище MongoDB
func NewClient(ctx context.Context, host, port, username, password, database, authDB, uri string) (*Storage, error) {
	var mongoDBURL string
	var isAuth bool

	if uri == "" {
		if username == "" && password == "" {
			mongoDBURL = "mongodb://" + host + ":" + port
		} else {
			isAuth = true
			mongoDBURL = "mongodb://" + username + ":" + password + "@" + host + ":" + port
		}
	} else {
		mongoDBURL = uri
	}

	clientOptions := options.Client().ApplyURI(mongoDBURL)
	if isAuth {
		if authDB == "" {
			authDB = database
		}
		clientOptions.SetAuth(options.Credential{
			AuthSource: authDB,
			Username:   username,
			Password:   password,
		})
	}
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("connect to MongoDB: %w", err)
	}

	if err = client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}
	return &Storage{db: client.Database(database)}, nil
}

// SaveURL сохраняет новый URL в MongoDB
func (s *Storage) SaveURL(ctx context.Context, urlToSave, alias string, userID int64) (interface{}, error) {
	const op = "mongodb.SaveURL"

	collection := s.db.Collection("urls")

	doc := bson.M{
		"url":     urlToSave,
		"alias":   alias,
		"user_id": userID,
	}

	// Проверка на существование alias
	count, err := collection.CountDocuments(ctx, bson.M{"alias": alias})
	if err != nil {
		return nil, fmt.Errorf("%s: count documents: %w", op, err)
	}
	if count > 0 {
		return nil, fmt.Errorf("%s: %w", op, storage.ErrURLExists)
	}

	// Вставка нового URL
	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("%s: insert document: %w", op, err)
	}

	return res.InsertedID, nil
}

// GetURL получает URL по alias и проверяет принадлежность пользователя
func (s *Storage) GetURL(ctx context.Context, alias string, userID int64) (string, error) {
	const op = "mongodb.GetURL"

	collection := s.db.Collection("urls")

	// Сначала проверяем, существует ли alias в базе
	var doc struct {
		URL    string `bson:"url"`
		UserID int64  `bson:"user_id"`
	}

	err := collection.FindOne(ctx, bson.M{"alias": alias}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return "", storage.ErrURLNotFound
	} else if err != nil {
		return "", fmt.Errorf("%s: find document: %w", op, err)
	}

	// Проверяем принадлежность alias пользователю
	if doc.UserID != userID {
		return "", storage.ErrUnauthorized
	}

	return doc.URL, nil
}

// DeleteURL удаляет URL по alias и проверяет владельца
func (s *Storage) DeleteURL(ctx context.Context, alias string, userID int64) error {
	const op = "mongodb.DeleteURL"

	collection := s.db.Collection("urls")

	// Проверка принадлежности alias пользователю
	var doc struct {
		UserID int64 `bson:"user_id"`
	}
	err := collection.FindOne(ctx, bson.M{"alias": alias}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return storage.ErrURLNotFound
	} else if err != nil {
		return fmt.Errorf("%s: find document: %w", op, err)
	}

	if doc.UserID != userID {
		return storage.ErrUnauthorized
	}

	// Удаление документа
	_, err = collection.DeleteOne(ctx, bson.M{"alias": alias})
	if err != nil {
		return fmt.Errorf("%s: delete document: %w", op, err)
	}

	return nil
}

// SaveUser сохраняет нового пользователя в MongoDB
func (s *Storage) SaveUser(ctx context.Context, nickname, passwordHash string, userID int64) (interface{}, error) {
	const op = "mongodb.SaveUser"

	collection := s.db.Collection("users")

	doc := bson.M{
		"nickname":      nickname,
		"password_hash": passwordHash,
		"user_id":       userID,
	}

	// Проверка на существование пользователя
	count, err := collection.CountDocuments(ctx, bson.M{"nickname": nickname})
	if err != nil {
		return nil, fmt.Errorf("%s: count documents: %w", op, err)
	}
	if count > 0 {
		return nil, fmt.Errorf("%s: %w", op, storage.ErrUserExists)
	}

	// Вставка нового пользователя
	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("%s: insert document: %w", op, err)
	}

	return res.InsertedID, nil
}

// GetUserByNickname получает пользователя по никнейму
func (s *Storage) GetUserByNickname(ctx context.Context, nickname string) (int64, string, error) {
	const op = "mongodb.GetUserByNickname"

	collection := s.db.Collection("users")

	var doc struct {
		ID           primitive.ObjectID `bson:"_id"`
		PasswordHash string             `bson:"password_hash"`
	}

	err := collection.FindOne(ctx, bson.M{"nickname": nickname}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 0, "", storage.ErrUserNotFound
	} else if err != nil {
		return 0, "", fmt.Errorf("%s: find document: %w", op, err)
	}

	// Если необходимо преобразовать ObjectID в int64 (не рекомендуется, так как это может привести к потерям данных)
	// userID := doc.ID.Hex() // Если нужно использовать в качестве строки
	userID := int64(doc.ID.Timestamp().Unix()) // Пример получения значения времени создания ObjectID в качестве int64

	return userID, doc.PasswordHash, nil
}

// DeleteUserByNickname удаляет пользователя и все связанные URL
func (s *Storage) DeleteUserByNickname(ctx context.Context, nickname string) error {
	const op = "mongodb.DeleteUserByNickname"

	// Начинаем транзакцию
	session, err := s.db.Client().StartSession()
	if err != nil {
		return fmt.Errorf("%s: start session: %w", op, err)
	}
	defer session.EndSession(ctx)

	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		collectionUsers := s.db.Collection("users")
		collectionURLs := s.db.Collection("urls")

		// Находим пользователя
		var doc struct {
			ID int64 `bson:"user_id"` // Извлекаем user_id как int64
		}
		err := collectionUsers.FindOne(sc, bson.M{"nickname": nickname}).Decode(&doc)
		if err == mongo.ErrNoDocuments {
			return storage.ErrUserNotFound
		} else if err != nil {
			return fmt.Errorf("%s: find user: %w", op, err)
		}

		// Удаляем все URL, связанные с пользователем
		_, err = collectionURLs.DeleteMany(sc, bson.M{"user_id": doc.ID}) // Удаляем URL по user_id
		if err != nil {
			return fmt.Errorf("%s: delete URLs: %w", op, err)
		}

		// Удаляем пользователя
		_, err = collectionUsers.DeleteOne(sc, bson.M{"_id": doc.ID})
		if err != nil {
			return fmt.Errorf("%s: delete user: %w", op, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("%s: transaction failed: %w", op, err)
	}

	return nil
}
