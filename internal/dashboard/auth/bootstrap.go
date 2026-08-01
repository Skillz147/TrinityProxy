package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

const defaultAdminUsername = "admin"

// BootstrapResult holds credentials created on first dashboard setup.
type BootstrapResult struct {
	Username    string
	TempPassword string
	Created     bool
}

// BootstrapAdmin creates the initial admin user when none exist.
func (s *Store) BootstrapAdmin(username string) (*BootstrapResult, error) {
	hasUsers, err := s.HasUsers()
	if err != nil {
		return nil, err
	}
	if hasUsers {
		return &BootstrapResult{Created: false}, nil
	}

	if username == "" {
		username = defaultAdminUsername
	}

	tempPassword, err := generateTempPassword()
	if err != nil {
		return nil, err
	}

	user, err := s.CreateAdminUser(username, tempPassword)
	if err != nil {
		return nil, err
	}

	return &BootstrapResult{
		Username:     user.Username,
		TempPassword: tempPassword,
		Created:      true,
	}, nil
}

func generateTempPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("generate temp password: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
