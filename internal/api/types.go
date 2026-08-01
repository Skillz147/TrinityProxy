package api

import (
	"time"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

// ProxyNodePublic is the default API view of a proxy node (no credentials).
type ProxyNodePublic struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Country   string    `json:"country"`
	Region    string    `json:"region"`
	City      string    `json:"city"`
	Zip         string `json:"zip"`
	Platform    string `json:"platform"`
	DeviceClass string `json:"device_class"`
	NetworkType string `json:"network_type"`
	IsOnline    bool       `json:"is_online"`
	LastSeen    time.Time  `json:"last_seen"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	IsHealthy   bool       `json:"is_healthy"`
	LastProbeAt *time.Time `json:"last_probe_at,omitempty"`
}

// ProxyNodeAdmin includes full node credentials for trusted admin consumers.
type ProxyNodeAdmin struct {
	ProxyNodePublic
	Password string `json:"password"`
}

func ToPublic(node storage.ProxyNode) ProxyNodePublic {
	return ProxyNodePublic{
		ID:          node.ID,
		IP:          node.IP,
		Port:        node.Port,
		Username:    node.Username,
		Country:     node.Country,
		Region:      node.Region,
		City:        node.City,
		Zip:         node.Zip,
		Platform:    node.Platform,
		DeviceClass: node.DeviceClass,
		NetworkType: node.NetworkType,
		IsOnline:    node.IsOnline,
		LastSeen:    node.LastSeen,
		CreatedAt:   node.CreatedAt,
		UpdatedAt:   node.UpdatedAt,
		IsHealthy:   node.IsHealthy,
		LastProbeAt: node.LastProbeAt,
	}
}

func ToAdmin(node storage.ProxyNode) ProxyNodeAdmin {
	return ProxyNodeAdmin{
		ProxyNodePublic: ToPublic(node),
		Password:        node.Password,
	}
}

func ToPublicSlice(nodes []storage.ProxyNode) []ProxyNodePublic {
	out := make([]ProxyNodePublic, len(nodes))
	for i, node := range nodes {
		out[i] = ToPublic(node)
	}
	return out
}

func ToAdminSlice(nodes []storage.ProxyNode) []ProxyNodeAdmin {
	out := make([]ProxyNodeAdmin, len(nodes))
	for i, node := range nodes {
		out[i] = ToAdmin(node)
	}
	return out
}
