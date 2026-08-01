package auth

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 64
	minPasswordLen = 8
	maxPasswordLen = 128
)

var (
	ErrEmptyUsername    = errors.New("username must not be empty")
	ErrUsernameTooShort = errors.New("username is too short")
	ErrUsernameTooLong  = errors.New("username is too long")
	ErrInvalidUsername  = errors.New("username contains invalid characters")
	ErrEmptyPassword    = errors.New("password must not be empty")
	ErrPasswordTooShort = errors.New("password is too short")
	ErrPasswordTooLong  = errors.New("password is too long")
)

func SanitizeUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if username == "" {
		return "", ErrEmptyUsername
	}
	if utf8.RuneCountInString(username) < minUsernameLen {
		return "", ErrUsernameTooShort
	}
	if utf8.RuneCountInString(username) > maxUsernameLen {
		return "", ErrUsernameTooLong
	}
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return "", ErrInvalidUsername
	}
	return username, nil
}

func SanitizePassword(raw string) (string, error) {
	password := strings.TrimSpace(raw)
	if password == "" {
		return "", ErrEmptyPassword
	}
	if utf8.RuneCountInString(password) < minPasswordLen {
		return "", ErrPasswordTooShort
	}
	if utf8.RuneCountInString(password) > maxPasswordLen {
		return "", ErrPasswordTooLong
	}
	return password, nil
}

func SanitizeTempPassword(raw string) (string, error) {
	password := strings.TrimSpace(raw)
	if password == "" {
		return "", ErrEmptyPassword
	}
	if utf8.RuneCountInString(password) > maxPasswordLen {
		return "", ErrPasswordTooLong
	}
	return password, nil
}
