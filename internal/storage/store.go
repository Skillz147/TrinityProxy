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
}

var _ NodeStore = (*NodeStorage)(nil)
