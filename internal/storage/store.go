package storage

import "time"

// NodeStore abstracts node persistence for handler tests and production use.
type NodeStore interface {
	UpsertNode(node *ProxyNode) error
	GetOnlineNodes() ([]ProxyNode, error)
	GetAllNodes() ([]ProxyNode, error)
	GetNodeByID(id string) (*ProxyNode, error)
	GetNodesByCountry(country string) ([]ProxyNode, error)
	UpdateNodeHealth(id string, healthy bool, probedAt time.Time) error
	MarkOfflineNodes() error
	DeleteNode(id string) error
}

// NodeTokenStore manages per-node agent authentication tokens.
type NodeTokenStore interface {
	GetNodeTokenHash(nodeID string) (string, error)
	IssueNodeToken(nodeID string) (string, error)
	RevokeNodeToken(nodeID string) error
	NodeHasToken(nodeID string) (bool, error)
	ValidateNodeToken(nodeID, token string) (bool, error)
}

// CommandStore abstracts remote agent command queue persistence.
type CommandStore interface {
	EnqueueCommand(nodeID, action string, params map[string]string) (*AgentCommand, error)
	GetPendingCommands(nodeID string) ([]AgentCommand, error)
	MarkCommandsRunning(ids []string) error
	CompleteCommand(id, status, result string) error
	CompleteCommandForNode(id, nodeID, status, result string) error
	GetCommandByID(id string) (*AgentCommand, error)
	GetLatestCommandForNode(nodeID string) (*AgentCommand, error)
	GetLatestCommandsForNodes(nodeIDs []string) (map[string]AgentCommand, error)
	ClearCommandsForNode(nodeID string) error
}

var _ NodeStore = (*NodeStorage)(nil)
var _ NodeTokenStore = (*NodeStorage)(nil)
var _ CommandStore = (*NodeStorage)(nil)
