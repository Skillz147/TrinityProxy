// internal/storage/database.go
package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type ProxyNode struct {
	ID        string    `json:"id" db:"id"`
	IP        string    `json:"ip" db:"ip"`
	Port      int       `json:"port" db:"port"`
	Username  string    `json:"username" db:"username"`
	Password  string    `json:"password" db:"password"`
	Country   string    `json:"country" db:"country"`
	Region    string    `json:"region" db:"region"`
	City      string    `json:"city" db:"city"`
	IsOnline  bool      `json:"is_online" db:"is_online"`
	LastSeen  time.Time `json:"last_seen" db:"last_seen"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type NodeStorage struct {
	db *sql.DB
}

func NewNodeStorage(dbPath string) (*NodeStorage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	storage := &NodeStorage{db: db}
	if err := storage.createTables(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *NodeStorage) createTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS proxy_nodes (
		id TEXT PRIMARY KEY,
		ip TEXT NOT NULL,
		port INTEGER NOT NULL,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		country TEXT,
		region TEXT,
		city TEXT,
		is_online BOOLEAN DEFAULT true,
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_nodes_online ON proxy_nodes(is_online);
	CREATE INDEX IF NOT EXISTS idx_nodes_country ON proxy_nodes(country);
	CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON proxy_nodes(last_seen);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *NodeStorage) UpsertNode(node *ProxyNode) error {
	query := `
	INSERT OR REPLACE INTO proxy_nodes 
	(id, ip, port, username, password, country, region, city, is_online, last_seen, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, true, ?, ?)
	`

	nodeID := fmt.Sprintf("%s:%d", node.IP, node.Port)
	now := time.Now()

	_, err := s.db.Exec(query, nodeID, node.IP, node.Port, node.Username,
		node.Password, node.Country, node.Region, node.City, now, now)
	return err
}

func (s *NodeStorage) GetOnlineNodes() ([]ProxyNode, error) {
	query := `
	SELECT id, ip, port, username, password, country, region, city, 
	       is_online, last_seen, created_at, updated_at 
	FROM proxy_nodes 
	WHERE is_online = true AND last_seen > datetime('now', '-5 minutes')
	ORDER BY last_seen DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ProxyNode
	for rows.Next() {
		var node ProxyNode
		err := rows.Scan(&node.ID, &node.IP, &node.Port, &node.Username,
			&node.Password, &node.Country, &node.Region, &node.City,
			&node.IsOnline, &node.LastSeen, &node.CreatedAt, &node.UpdatedAt)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *NodeStorage) GetNodesByCountry(country string) ([]ProxyNode, error) {
	query := `
	SELECT id, ip, port, username, password, country, region, city,
	       is_online, last_seen, created_at, updated_at 
	FROM proxy_nodes 
	WHERE country = ? AND is_online = true AND last_seen > datetime('now', '-5 minutes')
	ORDER BY last_seen DESC
	`

	rows, err := s.db.Query(query, country)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ProxyNode
	for rows.Next() {
		var node ProxyNode
		err := rows.Scan(&node.ID, &node.IP, &node.Port, &node.Username,
			&node.Password, &node.Country, &node.Region, &node.City,
			&node.IsOnline, &node.LastSeen, &node.CreatedAt, &node.UpdatedAt)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *NodeStorage) MarkOfflineNodes() error {
	query := `
	UPDATE proxy_nodes 
	SET is_online = false, updated_at = CURRENT_TIMESTAMP
	WHERE last_seen < datetime('now', '-5 minutes')
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *NodeStorage) Close() error {
	return s.db.Close()
}
