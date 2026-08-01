package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "auth.db")

	store, err := NewStore(dbPath, time.Hour)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(dbPath)
	})
	return store
}

func TestChangePasswordSuccess(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateAdminUser("admin", "temp-pass")
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	if !user.MustChangePassword {
		t.Fatal("expected must_change_password=true for new admin")
	}

	if err := store.ChangePassword(user.ID, "temp-pass", "new-secure-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	updated, err := store.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if updated.MustChangePassword {
		t.Fatal("expected must_change_password=false after change")
	}

	_, err = store.Login("admin", "new-secure-password")
	if err != nil {
		t.Fatalf("Login with new password: %v", err)
	}

	if _, err := store.Login("admin", "temp-pass"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected old password to fail, got %v", err)
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateAdminUser("admin", "temp-pass")
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	err = store.ChangePassword(user.ID, "wrong-pass", "new-secure-password")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestChangePasswordTooShort(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateAdminUser("admin", "temp-pass")
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	err = store.ChangePassword(user.ID, "temp-pass", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestChangePasswordUnchanged(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateAdminUser("admin", "temp-pass")
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	err = store.ChangePassword(user.ID, "temp-pass", "temp-pass")
	if !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("expected ErrPasswordUnchanged, got %v", err)
	}
}

func TestChangePasswordSessionRemainsValid(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateAdminUser("admin", "temp-pass")
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	login, err := store.Login("admin", "temp-pass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := store.ChangePassword(user.ID, "temp-pass", "new-secure-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	sessionUser, err := store.ValidateSession(login.Token)
	if err != nil {
		t.Fatalf("ValidateSession after password change: %v", err)
	}
	if sessionUser.MustChangePassword {
		t.Fatal("expected must_change_password=false in session user")
	}
}
