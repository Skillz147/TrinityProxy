package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/logutil"
)

const defaultGeoCacheTTL = 24 * time.Hour

type geoCacheEntry struct {
	IP        string            `json:"ip"`
	Data      map[string]string `json:"data"`
	FetchedAt time.Time         `json:"fetched_at"`
}

type geoCache struct {
	mu      sync.RWMutex
	entries map[string]geoCacheEntry
	ttl     time.Duration
	file    string
}

var globalGeoCache = newGeoCache()

func newGeoCache() *geoCache {
	c := &geoCache{
		entries: make(map[string]geoCacheEntry),
		ttl:     geoCacheTTL(),
		file:    geoCacheFilePath(),
	}
	c.loadFromDisk()
	return c
}

func geoCacheTTL() time.Duration {
	if v := strings.TrimSpace(os.Getenv("TRINITY_GEO_CACHE_TTL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultGeoCacheTTL
}

func geoCacheFilePath() string {
	if v := strings.TrimSpace(os.Getenv("TRINITY_GEO_CACHE_FILE")); v != "" {
		return v
	}
	if dir := strings.TrimSpace(os.Getenv("TRINITY_GEO_CACHE_DIR")); dir != "" {
		return filepath.Join(dir, "geo-cache.json")
	}
	if inContainer() {
		return "/var/lib/trinityproxy-agent/geo-cache.json"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".trinityproxy", "geo-cache.json")
}

func (c *geoCache) get(ip string) (map[string]string, bool) {
	c.mu.RLock()
	entry, ok := c.entries[ip]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(entry.FetchedAt) > c.ttl {
		return nil, false
	}
	return cloneGeoData(entry.Data), true
}

func (c *geoCache) set(ip string, data map[string]string) {
	entry := geoCacheEntry{
		IP:        ip,
		Data:      cloneGeoData(data),
		FetchedAt: time.Now(),
	}
	c.mu.Lock()
	c.entries[ip] = entry
	c.mu.Unlock()
	c.persistToDisk(entry)
}

func (c *geoCache) loadFromDisk() {
	if c.file == "" {
		return
	}
	raw, err := os.ReadFile(c.file)
	if err != nil {
		return
	}
	var entry geoCacheEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return
	}
	if entry.IP == "" || len(entry.Data) == 0 {
		return
	}
	if time.Since(entry.FetchedAt) > c.ttl {
		return
	}
	c.mu.Lock()
	c.entries[entry.IP] = entry
	c.mu.Unlock()
}

func (c *geoCache) persistToDisk(entry geoCacheEntry) {
	if c.file == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.file), 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(c.file, raw, 0o644)
}

func cloneGeoData(data map[string]string) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}

func resetGeoCacheForTests() {
	globalGeoCache = newGeoCache()
}

func getCachedGeoInfo(ip string) (map[string]string, error) {
	if cached, ok := globalGeoCache.get(ip); ok {
		return cached, nil
	}

	data, err := getGeoInfo(ip)
	if err != nil {
		return nil, err
	}

	globalGeoCache.set(ip, data)
	logutil.Component("agent").Info("geo location cached", "ip", ip, "ttl", globalGeoCache.ttl.String())
	return cloneGeoData(data), nil
}

func geoCacheEntriesForTests() int {
	globalGeoCache.mu.RLock()
	defer globalGeoCache.mu.RUnlock()
	return len(globalGeoCache.entries)
}

func geoCacheTTLForTests() time.Duration {
	return globalGeoCache.ttl
}

func geoCacheSetTTLForTests(d time.Duration) {
	globalGeoCache.mu.Lock()
	globalGeoCache.ttl = d
	globalGeoCache.mu.Unlock()
}

func geoCacheSeedForTests(ip string, data map[string]string, fetchedAt time.Time) {
	globalGeoCache.mu.Lock()
	globalGeoCache.entries[ip] = geoCacheEntry{
		IP:        ip,
		Data:      cloneGeoData(data),
		FetchedAt: fetchedAt,
	}
	globalGeoCache.mu.Unlock()
}

func geoCacheDisableFileForTests() {
	globalGeoCache.mu.Lock()
	globalGeoCache.file = ""
	globalGeoCache.mu.Unlock()
}
