package storage

import (
	"database/sql"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/auth"
)

// SetNodeTokenHash stores the hashed node token for an existing node.
func (s *NodeStorage) SetNodeTokenHash(nodeID, tokenHash string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE proxy_nodes
		SET token_hash = ?, token_created_at = ?, updated_at = ?
		WHERE id = ?
	`, tokenHash, now, now, nodeID)
	return err
}

// GetNodeTokenHash returns the stored token hash for a node (empty if none).
func (s *NodeStorage) GetNodeTokenHash(nodeID string) (string, error) {
	var hash sql.NullString
	err := s.db.QueryRow(`SELECT token_hash FROM proxy_nodes WHERE id = ?`, nodeID).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if hash.Valid {
		return hash.String, nil
	}
	return "", nil
}

// NodeHasToken reports whether the node has a per-node token enrolled.
func (s *NodeStorage) NodeHasToken(nodeID string) (bool, error) {
	hash, err := s.GetNodeTokenHash(nodeID)
	if err != nil {
		return false, err
	}
	return hash != "", nil
}

// RevokeNodeToken clears the per-node token so the agent must re-enroll.
func (s *NodeStorage) RevokeNodeToken(nodeID string) error {
	_, err := s.db.Exec(`
		UPDATE proxy_nodes
		SET token_hash = '', token_created_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, nodeID)
	return err
}

// IssueNodeToken generates a new token, stores its hash, and returns the plaintext once.
func (s *NodeStorage) IssueNodeToken(nodeID string) (string, error) {
	token, err := auth.GenerateNodeToken()
	if err != nil {
		return "", err
	}
	if err := s.SetNodeTokenHash(nodeID, auth.HashNodeToken(token)); err != nil {
		return "", err
	}
	return token, nil
}

// ValidateNodeToken checks the provided token against the stored hash for nodeID.
func (s *NodeStorage) ValidateNodeToken(nodeID, token string) (bool, error) {
	hash, err := s.GetNodeTokenHash(nodeID)
	if err != nil {
		return false, err
	}
	if hash == "" {
		return false, nil
	}
	return auth.ValidNodeToken(token, hash), nil
}
