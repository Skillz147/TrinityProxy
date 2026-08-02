package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Agent command actions and statuses.
const (
	CommandActionUninstall = "uninstall"
	CommandActionRestart   = "restart"
	CommandActionStatus    = "status"
	CommandActionRepair    = "repair"

	CommandStatusPending = "pending"
	CommandStatusRunning = "running"
	CommandStatusSuccess = "success"
	CommandStatusFailure = "failure"
)

var validCommandActions = map[string]struct{}{
	CommandActionUninstall: {},
	CommandActionRestart:   {},
	CommandActionStatus:    {},
	CommandActionRepair:    {},
}

// AgentCommand is a queued remote operation for an agent node.
type AgentCommand struct {
	ID          string            `json:"id"`
	NodeID      string            `json:"node_id"`
	Action      string            `json:"action"`
	Status      string            `json:"status"`
	Params      map[string]string `json:"params,omitempty"`
	Result      string            `json:"result,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

func (s *NodeStorage) createCommandsTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS agent_commands (
		id TEXT PRIMARY KEY,
		node_id TEXT NOT NULL,
		action TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		params TEXT,
		result TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_agent_commands_node ON agent_commands(node_id);
	CREATE INDEX IF NOT EXISTS idx_agent_commands_status ON agent_commands(status);
	`
	_, err := s.db.Exec(query)
	return err
}

func newCommandID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func encodeParams(params map[string]string) (string, error) {
	if len(params) == 0 {
		return "", nil
	}
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeParams(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var params map[string]string
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return nil
	}
	return params
}

func scanCommand(row interface {
	Scan(dest ...any) error
}) (AgentCommand, error) {
	var cmd AgentCommand
	var paramsRaw sql.NullString
	var resultRaw sql.NullString
	var completedAt sql.NullTime
	err := row.Scan(
		&cmd.ID, &cmd.NodeID, &cmd.Action, &cmd.Status, &paramsRaw, &resultRaw,
		&cmd.CreatedAt, &cmd.UpdatedAt, &completedAt,
	)
	if err != nil {
		return AgentCommand{}, err
	}
	if paramsRaw.Valid {
		cmd.Params = decodeParams(paramsRaw.String)
	}
	if resultRaw.Valid {
		cmd.Result = resultRaw.String
	}
	if completedAt.Valid {
		cmd.CompletedAt = &completedAt.Time
	}
	return cmd, nil
}

// EnqueueCommand queues a remote action for a node.
func (s *NodeStorage) EnqueueCommand(nodeID, action string, params map[string]string) (*AgentCommand, error) {
	if _, ok := validCommandActions[action]; !ok {
		return nil, fmt.Errorf("invalid command action: %s", action)
	}

	id, err := newCommandID()
	if err != nil {
		return nil, err
	}

	paramsJSON, err := encodeParams(params)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	_, err = s.db.Exec(`
		INSERT INTO agent_commands (id, node_id, action, status, params, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, nodeID, action, CommandStatusPending, paramsJSON, now, now)
	if err != nil {
		return nil, err
	}

	return &AgentCommand{
		ID:        id,
		NodeID:    nodeID,
		Action:    action,
		Status:    CommandStatusPending,
		Params:    params,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetPendingCommands returns pending commands for a node.
func (s *NodeStorage) GetPendingCommands(nodeID string) ([]AgentCommand, error) {
	rows, err := s.db.Query(`
		SELECT id, node_id, action, status, params, result, created_at, updated_at, completed_at
		FROM agent_commands
		WHERE node_id = ? AND status = ?
		ORDER BY created_at ASC
	`, nodeID, CommandStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []AgentCommand
	for rows.Next() {
		cmd, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}
	return commands, nil
}

// MarkCommandsRunning marks the given command IDs as running.
func (s *NodeStorage) MarkCommandsRunning(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	for _, id := range ids {
		res, err := s.db.Exec(`
			UPDATE agent_commands
			SET status = ?, updated_at = ?
			WHERE id = ? AND status = ?
		`, CommandStatusRunning, now, id, CommandStatusPending)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("command not pending: %s", id)
		}
	}
	return nil
}

// GetCommandByID returns a command by its ID.
func (s *NodeStorage) GetCommandByID(id string) (*AgentCommand, error) {
	row := s.db.QueryRow(`
		SELECT id, node_id, action, status, params, result, created_at, updated_at, completed_at
		FROM agent_commands
		WHERE id = ?
	`, id)

	cmd, err := scanCommand(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// CompleteCommand records the outcome of a remote command.
func (s *NodeStorage) CompleteCommand(id, status, result string) error {
	return s.completeCommand(id, "", status, result)
}

// CompleteCommandForNode records the outcome only when the command belongs to nodeID.
// Prevents cross-node ACK spoofing when combined with agent credential checks.
func (s *NodeStorage) CompleteCommandForNode(id, nodeID, status, result string) error {
	if nodeID == "" {
		return fmt.Errorf("node id is required")
	}
	return s.completeCommand(id, nodeID, status, result)
}

func (s *NodeStorage) completeCommand(id, nodeID, status, result string) error {
	if status != CommandStatusSuccess && status != CommandStatusFailure {
		return fmt.Errorf("invalid completion status: %s", status)
	}
	now := time.Now()
	query := `
		UPDATE agent_commands
		SET status = ?, result = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND status IN (?, ?)
	`
	args := []any{status, result, now, now, id, CommandStatusPending, CommandStatusRunning}
	if nodeID != "" {
		query += ` AND node_id = ?`
		args = append(args, nodeID)
	}
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("command not found: %s", id)
	}
	return nil
}

// ClearCommandsForNode removes all queued and completed commands for a node.
func (s *NodeStorage) ClearCommandsForNode(nodeID string) error {
	_, err := s.db.Exec(`DELETE FROM agent_commands WHERE node_id = ?`, nodeID)
	return err
}

// GetLatestCommandForNode returns the most recent command for a node.
func (s *NodeStorage) GetLatestCommandForNode(nodeID string) (*AgentCommand, error) {
	row := s.db.QueryRow(`
		SELECT id, node_id, action, status, params, result, created_at, updated_at, completed_at
		FROM agent_commands
		WHERE node_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, nodeID)

	cmd, err := scanCommand(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// GetLatestCommandsForNodes returns the latest command per node ID.
func (s *NodeStorage) GetLatestCommandsForNodes(nodeIDs []string) (map[string]AgentCommand, error) {
	out := make(map[string]AgentCommand)
	for _, nodeID := range nodeIDs {
		cmd, err := s.GetLatestCommandForNode(nodeID)
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			out[nodeID] = *cmd
		}
	}
	return out, nil
}
