package storage

import "errors"

var (
	ErrURLNotFound  = errors.New("Url not found")
	ErrURLExists    = errors.New("Url exists")
	ErrUserExists   = errors.New("User exists")
	ErrUserNotFound = errors.New("User not found")
	ErrUnauthorized = errors.New("Unauthorized")
)
