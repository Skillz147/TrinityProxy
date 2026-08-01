package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newTestStorage(t *testing.T) *NodeStorage {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	storage, err := NewNodeStorage(dbPath)
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { storage.Close() })
	return storage
}

func testNode(ip string, port int, country string) *ProxyNode {
	return &ProxyNode{
		IP:       ip,
		Port:     port,
		Username: "user",
		Password: "pass",
		Country:  country,
		Region:   "TestRegion",
		City:     "TestCity",
	}
}

func setLastSeenMinutesAgo(t *testing.T, s *NodeStorage, nodeID string, minutes int) {
	t.Helper()

	_, err := s.db.Exec(
		`UPDATE proxy_nodes SET last_seen = datetime('now', ?) WHERE id = ?`,
		fmt.Sprintf("-%d minutes", minutes),
		nodeID,
	)
	if err != nil {
		t.Fatalf("setLastSeenMinutesAgo: %v", err)
	}
}

func TestUpsertNodeCreatesAndUpdates(t *testing.T) {
	s := newTestStorage(t)
	node := testNode("203.0.113.10", 1080, "US")

	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode create: %v", err)
	}

	nodes, err := s.GetOnlineNodes()
	if err != nil {
		t.Fatalf("GetOnlineNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Country != "US" {
		t.Fatalf("expected country US, got %q", nodes[0].Country)
	}
	if nodes[0].ID != "203.0.113.10:1080" {
		t.Fatalf("expected id 203.0.113.10:1080, got %q", nodes[0].ID)
	}

	node.Country = "CA"
	node.Region = "Ontario"
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode update: %v", err)
	}

	nodes, err = s.GetOnlineNodes()
	if err != nil {
		t.Fatalf("GetOnlineNodes after update: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after update, got %d", len(nodes))
	}
	if nodes[0].Country != "CA" {
		t.Fatalf("expected updated country CA, got %q", nodes[0].Country)
	}
	if nodes[0].Region != "Ontario" {
		t.Fatalf("expected updated region Ontario, got %q", nodes[0].Region)
	}
}

func TestMarkOfflineNodesTimeout(t *testing.T) {
	s := newTestStorage(t)

	fresh := testNode("203.0.113.20", 1080, "US")
	stale := testNode("203.0.113.21", 1081, "DE")

	for _, node := range []*ProxyNode{fresh, stale} {
		if err := s.UpsertNode(node); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}

	setLastSeenMinutesAgo(t, s, "203.0.113.21:1081", 10)

	if err := s.MarkOfflineNodes(); err != nil {
		t.Fatalf("MarkOfflineNodes: %v", err)
	}

	var freshOnline, staleOnline bool
	row := s.db.QueryRow(`SELECT is_online FROM proxy_nodes WHERE id = ?`, "203.0.113.20:1080")
	if err := row.Scan(&freshOnline); err != nil {
		t.Fatalf("scan fresh node: %v", err)
	}
	row = s.db.QueryRow(`SELECT is_online FROM proxy_nodes WHERE id = ?`, "203.0.113.21:1081")
	if err := row.Scan(&staleOnline); err != nil {
		t.Fatalf("scan stale node: %v", err)
	}

	if !freshOnline {
		t.Fatal("expected fresh node to remain online")
	}
	if staleOnline {
		t.Fatal("expected stale node to be marked offline")
	}

	online, err := s.GetOnlineNodes()
	if err != nil {
		t.Fatalf("GetOnlineNodes: %v", err)
	}
	if len(online) != 1 {
		t.Fatalf("expected 1 online node, got %d", len(online))
	}
	if online[0].IP != "203.0.113.20" {
		t.Fatalf("expected fresh node online, got %s", online[0].IP)
	}
}

func TestGetNodesByCountry(t *testing.T) {
	s := newTestStorage(t)

	nodes := []*ProxyNode{
		testNode("203.0.113.30", 1080, "US"),
		testNode("203.0.113.31", 1081, "US"),
		testNode("203.0.113.32", 1082, "DE"),
	}
	for _, node := range nodes {
		if err := s.UpsertNode(node); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}

	usNodes, err := s.GetNodesByCountry("US")
	if err != nil {
		t.Fatalf("GetNodesByCountry US: %v", err)
	}
	if len(usNodes) != 2 {
		t.Fatalf("expected 2 US nodes, got %d", len(usNodes))
	}

	deNodes, err := s.GetNodesByCountry("DE")
	if err != nil {
		t.Fatalf("GetNodesByCountry DE: %v", err)
	}
	if len(deNodes) != 1 {
		t.Fatalf("expected 1 DE node, got %d", len(deNodes))
	}

	empty, err := s.GetNodesByCountry("FR")
	if err != nil {
		t.Fatalf("GetNodesByCountry FR: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 FR nodes, got %d", len(empty))
	}
}

func TestGetNodesByCountryExcludesStaleNodes(t *testing.T) {
	s := newTestStorage(t)

	node := testNode("203.0.113.40", 1080, "US")
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	setLastSeenMinutesAgo(t, s, "203.0.113.40:1080", 10)

	nodes, err := s.GetNodesByCountry("US")
	if err != nil {
		t.Fatalf("GetNodesByCountry: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected stale node excluded, got %d nodes", len(nodes))
	}
}

func TestGetNodesByCountryNormalizedName(t *testing.T) {
	s := newTestStorage(t)

	node := testNode("203.0.113.50", 1080, "US")
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	byCode, err := s.GetNodesByCountry("US")
	if err != nil {
		t.Fatalf("GetNodesByCountry US: %v", err)
	}
	if len(byCode) != 1 {
		t.Fatalf("expected 1 node for US code, got %d", len(byCode))
	}

	byName, err := s.GetNodesByCountry("United States")
	if err != nil {
		t.Fatalf("GetNodesByCountry United States: %v", err)
	}
	if len(byName) != 1 {
		t.Fatalf("expected 1 node for United States name, got %d", len(byName))
	}
}

func TestUpsertNodePersistsZip(t *testing.T) {
	s := newTestStorage(t)

	node := testNode("203.0.113.60", 1080, "US")
	node.Zip = "94105"
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	nodes, err := s.GetOnlineNodes()
	if err != nil {
		t.Fatalf("GetOnlineNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Zip != "94105" {
		t.Fatalf("expected zip 94105, got %q", nodes[0].Zip)
	}
}

func TestMigrateSchemaAddsHealthColumns(t *testing.T) {
	s := newTestStorage(t)

	var lastProbeCount, healthyCount int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('proxy_nodes') WHERE name = 'last_probe_at'`,
	).Scan(&lastProbeCount); err != nil {
		t.Fatalf("check last_probe_at column: %v", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('proxy_nodes') WHERE name = 'is_healthy'`,
	).Scan(&healthyCount); err != nil {
		t.Fatalf("check is_healthy column: %v", err)
	}
	if lastProbeCount != 1 || healthyCount != 1 {
		t.Fatalf("expected health columns, got last_probe_at=%d is_healthy=%d", lastProbeCount, healthyCount)
	}
}

func TestUpsertNodePersistsDeviceMetadata(t *testing.T) {
	s := newTestStorage(t)

	node := testNode("203.0.113.65", 1080, "US")
	node.Platform = "linux"
	node.DeviceClass = "vps"
	node.NetworkType = "datacenter"
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	nodes, err := s.GetOnlineNodes()
	if err != nil {
		t.Fatalf("GetOnlineNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Platform != "linux" {
		t.Fatalf("expected platform linux, got %q", nodes[0].Platform)
	}
	if nodes[0].DeviceClass != "vps" {
		t.Fatalf("expected device_class vps, got %q", nodes[0].DeviceClass)
	}
	if nodes[0].NetworkType != "datacenter" {
		t.Fatalf("expected network_type datacenter, got %q", nodes[0].NetworkType)
	}
}

func TestMigrateSchemaAddsDeviceMetadataColumns(t *testing.T) {
	s := newTestStorage(t)

	for _, col := range []string{"platform", "device_class", "network_type"} {
		var count int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('proxy_nodes') WHERE name = ?`, col,
		).Scan(&count); err != nil {
			t.Fatalf("check %s column: %v", col, err)
		}
		if count != 1 {
			t.Fatalf("expected %s column to exist", col)
		}
	}
}

func TestUpsertNodePreservesHealthFields(t *testing.T) {
	s := newTestStorage(t)
	node := testNode("203.0.113.71", 1080, "US")
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	probedAt := time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateNodeHealth("203.0.113.71:1080", true, probedAt); err != nil {
		t.Fatalf("UpdateNodeHealth: %v", err)
	}

	node.City = "UpdatedCity"
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode after health update: %v", err)
	}

	got, err := s.GetNodeByID("203.0.113.71:1080")
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected node to exist")
	}
	if !got.IsHealthy {
		t.Fatal("expected heartbeat upsert to preserve is_healthy=true")
	}
	if got.LastProbeAt == nil || !got.LastProbeAt.Equal(probedAt) {
		t.Fatalf("expected last_probe_at to be preserved, got %v", got.LastProbeAt)
	}
	if got.City != "UpdatedCity" {
		t.Fatalf("expected city to update, got %q", got.City)
	}
}

func TestUpdateNodeHealth(t *testing.T) {
	s := newTestStorage(t)
	node := testNode("203.0.113.70", 1080, "US")
	if err := s.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	probedAt := time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateNodeHealth("203.0.113.70:1080", true, probedAt); err != nil {
		t.Fatalf("UpdateNodeHealth: %v", err)
	}

	nodes, err := s.GetOnlineNodes()
	if err != nil {
		t.Fatalf("GetOnlineNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if !nodes[0].IsHealthy {
		t.Fatal("expected node to be healthy")
	}
	if nodes[0].LastProbeAt == nil {
		t.Fatal("expected last_probe_at to be set")
	}
	if !nodes[0].LastProbeAt.Equal(probedAt) {
		t.Fatalf("last_probe_at = %v, want %v", nodes[0].LastProbeAt, probedAt)
	}
}
