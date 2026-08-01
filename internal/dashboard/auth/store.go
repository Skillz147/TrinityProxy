package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/mattn/go-sqlite3"
)

var (
	ErrUserNotFound      = errors.New("invalid username or password")
	ErrInvalidPassword   = errors.New("invalid username or password")
	ErrUserExists        = errors.New("dashboard admin already exists")
	ErrSessionNotFound   = errors.New("invalid or expired session")
	ErrPasswordUnchanged = errors.New("new password must differ from current password")
)

const (
	defaultBcryptCost = bcrypt.DefaultCost
	sessionTokenBytes = 32
)

type User struct {
	ID                 int64     `json:"id"`
	Username           string    `json:"username"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type LoginResult struct {
	Token              string `json:"token"`
	MustChangePassword bool   `json:"must_change_password"`
	User               User   `json:"user"`
}

type Store struct {
	db         *sql.DB
	sessionTTL time.Duration
}

func NewStore(dbPath string, sessionTTL time.Duration) (*Store, error) {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db, sessionTTL: sessionTTL}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS dashboard_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		must_change_password BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS dashboard_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES dashboard_users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_dashboard_sessions_user ON dashboard_sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_dashboard_sessions_expires ON dashboard_sessions(expires_at);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Store) HasUsers() (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM dashboard_users`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) CreateAdminUser(username, tempPassword string) (*User, error) {
	username, err := SanitizeUsername(username)
	if err != nil {
		return nil, err
	}
	tempPassword, err = SanitizeTempPassword(tempPassword)
	if err != nil {
		return nil, err
	}

	hasUsers, err := s.HasUsers()
	if err != nil {
		return nil, err
	}
	if hasUsers {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), defaultBcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	res, err := s.db.Exec(`
		INSERT INTO dashboard_users (username, password_hash, must_change_password, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)
	`, username, string(hash), now, now)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:                 id,
		Username:           username,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

func (s *Store) Login(username, password string) (*LoginResult, error) {
	username, err := SanitizeUsername(username)
	if err != nil {
		return nil, ErrUserNotFound
	}
	password, err = SanitizeTempPassword(password)
	if err != nil {
		return nil, ErrInvalidPassword
	}

	user, hash, err := s.getUserByUsername(username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	token, err := s.createSession(user.ID)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		Token:              token,
		MustChangePassword: user.MustChangePassword,
		User:               *user,
	}, nil
}

func (s *Store) ChangePassword(userID int64, currentPassword, newPassword string) error {
	currentPassword, err := SanitizeTempPassword(currentPassword)
	if err != nil {
		return ErrInvalidPassword
	}
	newPassword, err = SanitizePassword(newPassword)
	if err != nil {
		return err
	}

	user, hash, err := s.getUserByID(userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(currentPassword)); err != nil {
		return ErrInvalidPassword
	}
	if subtle.ConstantTimeCompare([]byte(currentPassword), []byte(newPassword)) == 1 {
		return ErrPasswordUnchanged
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), defaultBcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.Exec(`
		UPDATE dashboard_users
		SET password_hash = ?, must_change_password = 0, updated_at = ?
		WHERE id = ?
	`, string(newHash), now, user.ID)
	return err
}

func (s *Store) GetUserByID(userID int64) (*User, error) {
	user, _, err := s.getUserByID(userID)
	return user, err
}

func (s *Store) Logout(token string) error {
	tokenHash := hashToken(token)
	res, err := s.db.Exec(`DELETE FROM dashboard_sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Store) TouchSession(token string) error {
	if token == "" {
		return ErrSessionNotFound
	}

	tokenHash := hashToken(token)
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	res, err := s.db.Exec(`
		UPDATE dashboard_sessions
		SET expires_at = ?
		WHERE token_hash = ? AND expires_at > ?
	`, expiresAt, tokenHash, time.Now().UTC())
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Store) ValidateSession(token string) (*User, error) {
	if token == "" {
		return nil, ErrSessionNotFound
	}

	tokenHash := hashToken(token)
	var user User
	var expiresAt time.Time
	err := s.db.QueryRow(`
		SELECT u.id, u.username, u.must_change_password, u.created_at, u.updated_at, s.expires_at
		FROM dashboard_sessions s
		JOIN dashboard_users u ON u.id = s.user_id
		WHERE s.token_hash = ?
	`, tokenHash).Scan(
		&user.ID, &user.Username, &user.MustChangePassword,
		&user.CreatedAt, &user.UpdatedAt, &expiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	if time.Now().UTC().After(expiresAt) {
		_, _ = s.db.Exec(`DELETE FROM dashboard_sessions WHERE token_hash = ?`, tokenHash)
		return nil, ErrSessionNotFound
	}

	return &user, nil
}

func (s *Store) createSession(userID int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	_, err = s.db.Exec(`
		INSERT INTO dashboard_sessions (user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, userID, hashToken(token), expiresAt, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) getUserByUsername(username string) (*User, string, error) {
	var user User
	var hash string
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, must_change_password, created_at, updated_at
		FROM dashboard_users
		WHERE username = ?
	`, username).Scan(
		&user.ID, &user.Username, &hash, &user.MustChangePassword,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, "", err
	}
	return &user, hash, nil
}

func (s *Store) getUserByID(userID int64) (*User, string, error) {
	var user User
	var hash string
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, must_change_password, created_at, updated_at
		FROM dashboard_users
		WHERE id = ?
	`, userID).Scan(
		&user.ID, &user.Username, &hash, &user.MustChangePassword,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, "", err
	}
	return &user, hash, nil
}

func randomToken() (string, error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
