// internal/storage/database.go
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/geo"
	_ "github.com/mattn/go-sqlite3"
)

// DefaultNodeStaleAfter is how long without a heartbeat before a node is marked offline.
// Matches 2× the default 60s HEARTBEAT_INTERVAL.
const DefaultNodeStaleAfter = 120 * time.Second

func nodeStaleAfter() time.Duration {
	if v := strings.TrimSpace(os.Getenv("NODE_STALE_AFTER")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return DefaultNodeStaleAfter
}

func staleCutoffSQL() string {
	secs := int(nodeStaleAfter().Seconds())
	if secs < 1 {
		secs = 1
	}
	return fmt.Sprintf("datetime('now', '-%d seconds')", secs)
}

const nodeSelectColumns = `id, ip, port, username, password, country, region, city, zip,
	       platform, device_class, network_type,
	       is_online, last_seen, created_at, updated_at, is_healthy, last_probe_at,
	       token_hash, token_created_at`

type ProxyNode struct {
	ID        string    `json:"id" db:"id"`
	IP        string    `json:"ip" db:"ip"`
	Port      int       `json:"port" db:"port"`
	Username  string    `json:"username" db:"username"`
	Password  string    `json:"password" db:"password"`
	Country   string    `json:"country" db:"country"`
	Region    string    `json:"region" db:"region"`
	City      string    `json:"city" db:"city"`
	Zip         string `json:"zip" db:"zip"`
	Platform    string `json:"platform" db:"platform"`
	DeviceClass string `json:"device_class" db:"device_class"`
	NetworkType string `json:"network_type" db:"network_type"`
	IsOnline    bool       `json:"is_online" db:"is_online"`
	LastSeen    time.Time  `json:"last_seen" db:"last_seen"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	IsHealthy       bool       `json:"is_healthy" db:"is_healthy"`
	LastProbeAt     *time.Time `json:"last_probe_at,omitempty" db:"last_probe_at"`
	TokenHash       string     `json:"-" db:"token_hash"`
	TokenCreatedAt  *time.Time `json:"token_created_at,omitempty" db:"token_created_at"`
	HasNodeToken    bool       `json:"has_node_token" db:"-"`
}

type NodeStorage struct {
	db *sql.DB
}

func NewNodeStorage(dbPath string) (*NodeStorage, error) {
	dsn := dbPath
	if !strings.Contains(dsn, "?") {
		dsn += "?_journal_mode=WAL&_busy_timeout=5000"
	}
	db, err := sql.Open("sqlite3", dsn)
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
		zip TEXT,
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
	if err != nil {
		return err
	}
	if err := s.createCommandsTable(); err != nil {
		return err
	}
	return s.migrateSchema()
}

func (s *NodeStorage) migrateSchema() error {
	columns := map[string]string{
		"zip":           "TEXT",
		"platform":      "TEXT",
		"device_class":  "TEXT",
		"network_type":  "TEXT",
		"last_probe_at": "DATETIME",
		"is_healthy":        "BOOLEAN DEFAULT 0",
		"token_hash":        "TEXT NOT NULL DEFAULT ''",
		"token_created_at":  "DATETIME",
	}
	for name, def := range columns {
		var count int
		err := s.db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('proxy_nodes') WHERE name = ?`,
			name,
		).Scan(&count)
		if err != nil {
			return err
		}
		if count == 0 {
			if _, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE proxy_nodes ADD COLUMN %s %s`, name, def)); err != nil {
				return err
			}
		}
	}
	return nil
}

func scanNode(rows *sql.Rows) (ProxyNode, error) {
	var node ProxyNode
	var lastProbe, tokenCreated sql.NullTime
	var tokenHash sql.NullString
	err := rows.Scan(
		&node.ID, &node.IP, &node.Port, &node.Username, &node.Password,
		&node.Country, &node.Region, &node.City, &node.Zip,
		&node.Platform, &node.DeviceClass, &node.NetworkType,
		&node.IsOnline, &node.LastSeen, &node.CreatedAt, &node.UpdatedAt,
		&node.IsHealthy, &lastProbe,
		&tokenHash, &tokenCreated,
	)
	if err != nil {
		return ProxyNode{}, err
	}
	if lastProbe.Valid {
		node.LastProbeAt = &lastProbe.Time
	}
	if tokenHash.Valid {
		node.TokenHash = tokenHash.String
		node.HasNodeToken = tokenHash.String != ""
	}
	if tokenCreated.Valid {
		node.TokenCreatedAt = &tokenCreated.Time
	}
	return node, nil
}

func (s *NodeStorage) UpsertNode(node *ProxyNode) error {
	// Use ON CONFLICT so heartbeats refresh metadata without wiping probe results.
	// INSERT OR REPLACE would reset is_healthy and last_probe_at to defaults.
	query := `
	INSERT INTO proxy_nodes
	(id, ip, port, username, password, country, region, city, zip,
	 platform, device_class, network_type, is_online, last_seen, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, true, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		ip = excluded.ip,
		port = excluded.port,
		username = excluded.username,
		password = excluded.password,
		country = excluded.country,
		region = excluded.region,
		city = excluded.city,
		zip = excluded.zip,
		platform = excluded.platform,
		device_class = excluded.device_class,
		network_type = excluded.network_type,
		is_online = true,
		last_seen = excluded.last_seen,
		updated_at = excluded.updated_at
	`

	nodeID := fmt.Sprintf("%s:%d", node.IP, node.Port)
	now := time.Now()

	_, err := s.db.Exec(query, nodeID, node.IP, node.Port, node.Username,
		node.Password, node.Country, node.Region, node.City, node.Zip,
		node.Platform, node.DeviceClass, node.NetworkType, now, now)
	return err
}

func (s *NodeStorage) GetNodeByID(id string) (*ProxyNode, error) {
	query := fmt.Sprintf(`
	SELECT %s
	FROM proxy_nodes
	WHERE id = ?
	`, nodeSelectColumns)

	row := s.db.QueryRow(query, id)
	var node ProxyNode
	var lastProbe, tokenCreated sql.NullTime
	var tokenHash sql.NullString
	err := row.Scan(
		&node.ID, &node.IP, &node.Port, &node.Username, &node.Password,
		&node.Country, &node.Region, &node.City, &node.Zip,
		&node.Platform, &node.DeviceClass, &node.NetworkType,
		&node.IsOnline, &node.LastSeen, &node.CreatedAt, &node.UpdatedAt,
		&node.IsHealthy, &lastProbe,
		&tokenHash, &tokenCreated,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastProbe.Valid {
		node.LastProbeAt = &lastProbe.Time
	}
	if tokenHash.Valid {
		node.TokenHash = tokenHash.String
		node.HasNodeToken = tokenHash.String != ""
	}
	if tokenCreated.Valid {
		node.TokenCreatedAt = &tokenCreated.Time
	}
	return &node, nil
}

func (s *NodeStorage) GetAllNodes() ([]ProxyNode, error) {
	query := fmt.Sprintf(`
	SELECT %s
	FROM proxy_nodes
	ORDER BY is_online DESC, last_seen DESC
	`, nodeSelectColumns)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ProxyNode
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *NodeStorage) GetOnlineNodes() ([]ProxyNode, error) {
	query := fmt.Sprintf(`
	SELECT %s
	FROM proxy_nodes 
	WHERE is_online = true AND last_seen > %s
	ORDER BY last_seen DESC
	`, nodeSelectColumns, staleCutoffSQL())

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ProxyNode
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *NodeStorage) GetNodesByCountry(country string) ([]ProxyNode, error) {
	values := geo.CountryQueryValues(country)
	placeholders := make([]string, len(values))
	args := make([]interface{}, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = v
	}

	query := fmt.Sprintf(`
	SELECT %s
	FROM proxy_nodes 
	WHERE country IN (%s) AND is_online = true AND last_seen > %s
	ORDER BY last_seen DESC
	`, nodeSelectColumns, strings.Join(placeholders, ", "), staleCutoffSQL())

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ProxyNode
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *NodeStorage) UpdateNodeHealth(id string, healthy bool, probedAt time.Time) error {
	_, err := s.db.Exec(`
	UPDATE proxy_nodes
	SET is_healthy = ?, last_probe_at = ?, updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	`, healthy, probedAt, id)
	return err
}

func (s *NodeStorage) MarkOfflineNodes() error {
	query := fmt.Sprintf(`
	UPDATE proxy_nodes 
	SET is_online = false, is_healthy = false, updated_at = CURRENT_TIMESTAMP
	WHERE is_online = true AND last_seen < %s
	`, staleCutoffSQL())
	_, err := s.db.Exec(query)
	return err
}

func (s *NodeStorage) DeleteNode(id string) error {
	if err := s.ClearCommandsForNode(id); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM proxy_nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *NodeStorage) Close() error {
	return s.db.Close()
}
