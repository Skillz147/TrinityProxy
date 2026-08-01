package dashboard

import (
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/metrics"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestComputeSystemHealth(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		online    int
		offline   int
		unhealthy int
		want      string
	}{
		{"empty fleet", 0, 0, 0, 0, SystemHealthHealthy},
		{"all healthy online", 4, 4, 0, 0, SystemHealthHealthy},
		{"some offline", 4, 3, 1, 0, SystemHealthDegraded},
		{"some unhealthy", 4, 4, 0, 1, SystemHealthDegraded},
		{"all offline", 3, 0, 3, 0, SystemHealthCritical},
		{"majority unhealthy", 4, 4, 0, 2, SystemHealthCritical},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSystemHealth(tc.total, tc.online, tc.offline, tc.unhealthy)
			if got != tc.want {
				t.Fatalf("computeSystemHealth() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestComputeDashboardStats(t *testing.T) {
	nodes := []storage.ProxyNode{
		{Platform: "linux", DeviceClass: "vps", Country: "US", IsOnline: true, IsHealthy: true},
		{Platform: "windows", DeviceClass: "desktop", Country: "DE", IsOnline: true, IsHealthy: false},
		{Platform: "", DeviceClass: "", Country: "", IsOnline: false, IsHealthy: true},
	}

	stats := computeDashboardStats(nodes, metrics.Snapshot{ProbeFailures: 7, NodesOnline: 2})

	if stats.TotalAgents != 3 {
		t.Fatalf("total_agents = %d, want 3", stats.TotalAgents)
	}
	if stats.Online != 2 {
		t.Fatalf("online = %d, want 2", stats.Online)
	}
	if stats.Offline != 1 {
		t.Fatalf("offline = %d, want 1", stats.Offline)
	}
	if stats.Unhealthy != 1 {
		t.Fatalf("unhealthy = %d, want 1", stats.Unhealthy)
	}
	if stats.ProbeFailures != 7 {
		t.Fatalf("probe_failures = %d, want 7", stats.ProbeFailures)
	}
	if stats.SystemHealth != SystemHealthDegraded {
		t.Fatalf("system_health = %q, want degraded", stats.SystemHealth)
	}
	if stats.StatusBreakdown["online"] != 1 {
		t.Fatalf("status online = %d, want 1", stats.StatusBreakdown["online"])
	}
	if stats.StatusBreakdown["unhealthy"] != 1 {
		t.Fatalf("status unhealthy = %d, want 1", stats.StatusBreakdown["unhealthy"])
	}
	if stats.StatusBreakdown["offline"] != 1 {
		t.Fatalf("status offline = %d, want 1", stats.StatusBreakdown["offline"])
	}
	if stats.PlatformBreakdown["linux"] != 1 || stats.PlatformBreakdown["windows"] != 1 || stats.PlatformBreakdown["unknown"] != 1 {
		t.Fatalf("unexpected platform breakdown: %v", stats.PlatformBreakdown)
	}
	if stats.DeviceClassBreakdown["vps"] != 1 || stats.DeviceClassBreakdown["desktop"] != 1 || stats.DeviceClassBreakdown["unknown"] != 1 {
		t.Fatalf("unexpected device_class breakdown: %v", stats.DeviceClassBreakdown)
	}
	if stats.CountryBreakdown["US"] != 1 || stats.CountryBreakdown["DE"] != 1 || stats.CountryBreakdown["Unknown"] != 1 {
		t.Fatalf("unexpected country breakdown: %v", stats.CountryBreakdown)
	}
	if len(stats.RecentNodes) != 3 {
		t.Fatalf("recent_nodes len = %d, want 3", len(stats.RecentNodes))
	}
}

func TestComputeDashboardStatsRecentNodesLimit(t *testing.T) {
	nodes := make([]storage.ProxyNode, 8)
	for i := range nodes {
		nodes[i].IsOnline = true
		nodes[i].IsHealthy = true
	}

	stats := computeDashboardStats(nodes, metrics.Snapshot{})
	if len(stats.RecentNodes) != 5 {
		t.Fatalf("recent_nodes len = %d, want 5", len(stats.RecentNodes))
	}
}
