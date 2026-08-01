package metrics

import "sync/atomic"

var (
	heartbeatsReceived atomic.Uint64
	probeFailures      atomic.Uint64
	nodesOnline        atomic.Int64
)

// IncHeartbeatsReceived increments the heartbeat counter.
func IncHeartbeatsReceived() {
	heartbeatsReceived.Add(1)
}

// IncProbeFailures increments the SOCKS probe failure counter.
// Task 3.7 health probes should call this when a probe fails.
func IncProbeFailures() {
	probeFailures.Add(1)
}

// SetNodesOnline updates the cached online node count.
func SetNodesOnline(n int) {
	nodesOnline.Store(int64(n))
}

// Snapshot returns current counter values for the /metrics endpoint.
type Snapshot struct {
	HeartbeatsReceived uint64 `json:"heartbeats_received"`
	NodesOnline        int64  `json:"nodes_online"`
	ProbeFailures      uint64 `json:"probe_failures"`
}

// Current returns a point-in-time snapshot of all counters.
func Current() Snapshot {
	return Snapshot{
		HeartbeatsReceived: heartbeatsReceived.Load(),
		NodesOnline:        nodesOnline.Load(),
		ProbeFailures:      probeFailures.Load(),
	}
}
