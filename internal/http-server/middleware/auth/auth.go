package auth

import (
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"
	"net/http"
	"strings"
	"time"
	"url-shortener/internal/config"
)

var JWTSecret = []byte(config.MustLoad().JWTSecret)

// Функция для хэширования пароля
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// Функция для проверки пароля с хэшем
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Пример регистрации пользователя
func RegisterUser(username, password string) (string, error) {
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return "", err
	}

	// Здесь логика для сохранения пользователя и хэша пароля в базу данных
	fmt.Printf("User %s registered with password hash: %s\n", username, hashedPassword)
	return hashedPassword, nil
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateJWT(username string) (string, error) {
	expirationTime := time.Now().Add(5 * time.Minute)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(JWTSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// Проверка токена
func ValidateJWT(tokenString string) (string, error) {
	claims := &Claims{}

	// Парсинг токена и проверка подписи
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Проверяем метод подписи
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return JWTSecret, nil // Возвращаем секретный ключ
	})

	if err != nil {
		return "", err
	}

	// Проверка валидности токена
	if !token.Valid {
		return "", errors.New("invalid token")
	}

	return claims.Username, nil // Возвращаем имя пользователя из токена
}

// Логин с проверкой пароля и генерацией JWT токена
func Login(username, password, hash string) (string, error) {
	// Проверяем пароль
	if !CheckPasswordHash(password, hash) {
		return "", fmt.Errorf("invalid password")
	}

	// Генерируем JWT токен
	token, err := GenerateJWT(username)
	if err != nil {
		return "", err
	}

	return token, nil
}

// TokenAuthMiddleware проверяет наличие и валидность Bearer токена в заголовках
func TokenAuthMiddleware(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			http.Error(w, "Authorization header is missing", http.StatusUnauthorized)
			return
		}

		// Удаляем префикс "Bearer " из строки токена
		if strings.HasPrefix(tokenString, "Bearer ") {
			tokenString = strings.TrimPrefix(tokenString, "Bearer ")
		} else {
			http.Error(w, "Invalid token format", http.StatusUnauthorized)
			return
		}

		// Проверяем токен
		nickname, err := ValidateJWT(tokenString)
		if err != nil {
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		fmt.Println(nickname)

		// Добавляем имя пользователя в контекст запроса
		ctx := context.WithValue(r.Context(), "nickname", nickname)
		next.ServeHTTP(w, r.WithContext(ctx)) // Переходим к следующему обработчику с обновленным контекстом
	})
}
