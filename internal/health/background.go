package health

import (
	"log/slog"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

const defaultBackgroundProbeInterval = 60 * time.Second

// BackgroundProber periodically probes online nodes and persists health state.
type BackgroundProber struct {
	store    storage.NodeStore
	prober   *Prober
	interval time.Duration
	log      *slog.Logger
}

// NewBackgroundProber returns a prober that writes health results to storage.
func NewBackgroundProber(store storage.NodeStore, prober *Prober, interval time.Duration, log *slog.Logger) *BackgroundProber {
	if interval <= 0 {
		interval = defaultBackgroundProbeInterval
	}
	if log == nil {
		log = slog.Default()
	}
	return &BackgroundProber{
		store:    store,
		prober:   prober,
		interval: interval,
		log:      log.With("component", "background_probe"),
	}
}

// Start launches the background probe loop.
func (bp *BackgroundProber) Start() {
	go func() {
		bp.runOnce()
		ticker := time.NewTicker(bp.interval)
		defer ticker.Stop()
		for range ticker.C {
			bp.runOnce()
		}
	}()
}

func (bp *BackgroundProber) runOnce() {
	nodes, err := bp.store.GetOnlineNodes()
	if err != nil {
		bp.log.Error("failed to list online nodes", "err", err)
		return
	}

	now := time.Now()
	for _, node := range nodes {
		healthy := bp.prober.ProbeFresh(node.IP, node.Port, node.Username, node.Password)
		if err := bp.store.UpdateNodeHealth(node.ID, healthy, now); err != nil {
			bp.log.Error("failed to update node health", "err", err, "node_id", node.ID)
		}
	}
}
