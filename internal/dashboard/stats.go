package dashboard

import (
	"sort"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/api"
	"github.com/Skillz147/TrinityProxy/internal/metrics"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

const (
	SystemHealthHealthy   = "healthy"
	SystemHealthDegraded  = "degraded"
	SystemHealthCritical  = "critical"
)

// DashboardStats is the JSON payload for GET /api/dashboard/stats.
type DashboardStats struct {
	TotalAgents          int                   `json:"total_agents"`
	Online               int                   `json:"online"`
	Offline              int                   `json:"offline"`
	Unhealthy            int                   `json:"unhealthy"`
	ProbeFailures        uint64                `json:"probe_failures"`
	NodesOnline          int64                 `json:"nodes_online"`
	SystemHealth         string                `json:"system_health"`
	StatusBreakdown      map[string]int        `json:"status_breakdown"`
	PlatformBreakdown    map[string]int        `json:"platform_breakdown"`
	DeviceClassBreakdown map[string]int        `json:"device_class_breakdown"`
	CountryBreakdown     map[string]int        `json:"country_breakdown"`
	RecentNodes          []api.ProxyNodePublic `json:"recent_nodes"`
}

func computeDashboardStats(nodes []storage.ProxyNode, snapshot metrics.Snapshot) DashboardStats {
	online := 0
	unhealthy := 0
	statusBreakdown := map[string]int{
		"online":    0,
		"offline":   0,
		"unhealthy": 0,
	}
	platformBreakdown := make(map[string]int)
	deviceClassBreakdown := make(map[string]int)
	countryBreakdown := make(map[string]int)

	for _, node := range nodes {
		platform := normalizeLabel(node.Platform, "unknown")
		deviceClass := normalizeLabel(node.DeviceClass, "unknown")
		country := normalizeLabel(node.Country, "Unknown")

		platformBreakdown[platform]++
		deviceClassBreakdown[deviceClass]++
		countryBreakdown[country]++

		if node.IsOnline {
			online++
			if node.IsHealthy {
				statusBreakdown["online"]++
			} else {
				statusBreakdown["unhealthy"]++
				unhealthy++
			}
		} else {
			statusBreakdown["offline"]++
			if !node.IsHealthy {
				unhealthy++
			}
		}
	}

	total := len(nodes)
	offline := total - online

	recent := make([]storage.ProxyNode, len(nodes))
	copy(recent, nodes)
	sort.Slice(recent, func(i, j int) bool {
		return recent[i].LastSeen.After(recent[j].LastSeen)
	})
	if len(recent) > 5 {
		recent = recent[:5]
	}

	return DashboardStats{
		TotalAgents:          total,
		Online:               online,
		Offline:              offline,
		Unhealthy:            unhealthy,
		ProbeFailures:        snapshot.ProbeFailures,
		NodesOnline:          snapshot.NodesOnline,
		SystemHealth:         computeSystemHealth(total, online, offline, unhealthy),
		StatusBreakdown:      statusBreakdown,
		PlatformBreakdown:    platformBreakdown,
		DeviceClassBreakdown: deviceClassBreakdown,
		CountryBreakdown:     countryBreakdown,
		RecentNodes:          api.ToPublicSlice(recent),
	}
}

func computeSystemHealth(total, online, offline, unhealthy int) string {
	if total == 0 {
		return SystemHealthHealthy
	}
	if online == 0 {
		return SystemHealthCritical
	}
	half := (total + 1) / 2
	if unhealthy >= half {
		return SystemHealthCritical
	}
	if offline > 0 || unhealthy > 0 {
		return SystemHealthDegraded
	}
	return SystemHealthHealthy
}

func normalizeLabel(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}
