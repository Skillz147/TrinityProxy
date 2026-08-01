package health

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

type mockHealthStore struct {
	mu    sync.Mutex
	nodes []storage.ProxyNode
	updates []healthUpdate
}

type healthUpdate struct {
	id       string
	healthy  bool
	probedAt time.Time
}

func (m *mockHealthStore) UpsertNode(node *storage.ProxyNode) error { return nil }
func (m *mockHealthStore) GetNodesByCountry(country string) ([]storage.ProxyNode, error) {
	return nil, nil
}
func (m *mockHealthStore) MarkOfflineNodes() error { return nil }

func (m *mockHealthStore) GetOnlineNodes() ([]storage.ProxyNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]storage.ProxyNode, len(m.nodes))
	copy(out, m.nodes)
	return out, nil
}

func (m *mockHealthStore) GetAllNodes() ([]storage.ProxyNode, error) {
	return m.GetOnlineNodes()
}

func (m *mockHealthStore) GetNodeByID(id string) (*storage.ProxyNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			node := m.nodes[i]
			return &node, nil
		}
	}
	return nil, nil
}

func (m *mockHealthStore) UpdateNodeHealth(id string, healthy bool, probedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, healthUpdate{id: id, healthy: healthy, probedAt: probedAt})
	return nil
}

func TestBackgroundProberUpdatesStore(t *testing.T) {
	store := &mockHealthStore{
		nodes: []storage.ProxyNode{
			{
				ID:       "203.0.113.80:1080",
				IP:       "203.0.113.80",
				Port:     1080,
				Username: "user",
				Password: "pass",
			},
		},
	}

	prober := NewProber(
		WithTimeout(time.Second),
		WithDialFunc(func(network, address string) (net.Conn, error) {
			server, client := net.Pipe()
			go func() {
				defer server.Close()
				greeting := make([]byte, 3)
				if _, err := io.ReadFull(server, greeting); err != nil {
					return
				}
				if _, err := server.Write([]byte{0x05, 0x02}); err != nil {
					return
				}
				authHeader := make([]byte, 2)
				if _, err := io.ReadFull(server, authHeader); err != nil {
					return
				}
				user := make([]byte, authHeader[1])
				if _, err := io.ReadFull(server, user); err != nil {
					return
				}
				passLen := make([]byte, 1)
				if _, err := io.ReadFull(server, passLen); err != nil {
					return
				}
				pass := make([]byte, passLen[0])
				if _, err := io.ReadFull(server, pass); err != nil {
					return
				}
				_, _ = server.Write([]byte{0x01, 0x00})
			}()
			return client, nil
		}),
	)

	bp := NewBackgroundProber(store, prober, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bp.runOnce()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.updates) != 1 {
		t.Fatalf("expected 1 health update, got %d", len(store.updates))
	}
	if store.updates[0].id != "203.0.113.80:1080" {
		t.Fatalf("update id = %q, want 203.0.113.80:1080", store.updates[0].id)
	}
	if !store.updates[0].healthy {
		t.Fatal("expected node to be marked healthy")
	}
}

func TestProbeFreshBypassesCache(t *testing.T) {
	var dialCount int32
	prober := NewProber(
		WithTimeout(100*time.Millisecond),
		WithCacheTTL(time.Minute),
		WithDialFunc(func(network, address string) (net.Conn, error) {
			dialCount++
			return nil, io.EOF
		}),
	)

	prober.ProbeFresh("127.0.0.1", 1, "user", "pass")
	prober.ProbeFresh("127.0.0.1", 1, "user", "pass")
	if dialCount != 2 {
		t.Fatalf("expected 2 fresh dials, got %d", dialCount)
	}
}
