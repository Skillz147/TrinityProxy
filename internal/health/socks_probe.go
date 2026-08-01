package health

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/metrics"
)

const (
	defaultProbeTimeout = 5 * time.Second
	defaultCacheTTL     = 30 * time.Second
)

// DialFunc dials a network address. Injectable for tests.
type DialFunc func(network, address string) (net.Conn, error)

type cacheEntry struct {
	healthy   bool
	expiresAt time.Time
}

type probeCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

func newProbeCache(ttl time.Duration) *probeCache {
	return &probeCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *probeCache) get(key string) (healthy bool, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[key]
	if !found || time.Now().After(entry.expiresAt) {
		return false, false
	}
	return entry.healthy, true
}

func (c *probeCache) set(key string, healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		healthy:   healthy,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Prober performs SOCKS5 TCP connect and username/password auth probes.
type Prober struct {
	dial          DialFunc
	cache         *probeCache
	timeout       time.Duration
	localFallback bool
	sameHostIPs   map[string]struct{}
}

// ProberOption configures a Prober.
type ProberOption func(*Prober)

// WithTimeout sets the per-probe dial and handshake timeout.
func WithTimeout(d time.Duration) ProberOption {
	return func(p *Prober) {
		if d > 0 {
			p.timeout = d
		}
	}
}

// WithCacheTTL sets how long probe results are cached.
func WithCacheTTL(d time.Duration) ProberOption {
	return func(p *Prober) {
		if d > 0 {
			p.cache.ttl = d
		}
	}
}

// WithDialFunc overrides the default TCP dialer (for tests).
func WithDialFunc(dial DialFunc) ProberOption {
	return func(p *Prober) {
		if dial != nil {
			p.dial = dial
		}
	}
}

// WithLocalFallback retries failed probes via 127.0.0.1. Use when the controller
// and agent run on the same host (dev) and the reported WAN IP is not reachable
// locally without port forwarding or NAT hairpin.
func WithLocalFallback(enabled bool) ProberOption {
	return func(p *Prober) {
		p.localFallback = enabled
	}
}

// WithSameHostIP allows loopback fallback when the node IP matches a known
// controller public address (SERVER_PUBLIC_IP), even in strict production mode.
func WithSameHostIP(ip string) ProberOption {
	return func(p *Prober) {
		ip = strings.TrimSpace(ip)
		if ip == "" || isLoopbackHost(ip) {
			return
		}
		if p.sameHostIPs == nil {
			p.sameHostIPs = make(map[string]struct{})
		}
		p.sameHostIPs[ip] = struct{}{}
	}
}

// NewProber returns a SOCKS5 prober with sensible defaults.
func NewProber(opts ...ProberOption) *Prober {
	p := &Prober{
		timeout: defaultProbeTimeout,
		cache:   newProbeCache(defaultCacheTTL),
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.dial == nil {
		timeout := p.timeout
		p.dial = func(network, address string) (net.Conn, error) {
			return net.DialTimeout(network, address, timeout)
		}
	}
	return p
}

func (p *Prober) cacheKey(ip string, port int, username, password string) string {
	return fmt.Sprintf("%s:%d:%s:%s", ip, port, username, password)
}

// IsHealthy probes SOCKS5 connectivity and auth, using a brief in-memory cache.
func (p *Prober) IsHealthy(ip string, port int, username, password string) bool {
	key := p.cacheKey(ip, port, username, password)
	if healthy, ok := p.cache.get(key); ok {
		return healthy
	}
	return p.ProbeFresh(ip, port, username, password)
}

// ProbeFresh runs a live probe, updates the in-memory cache, and counts failures once.
func (p *Prober) ProbeFresh(ip string, port int, username, password string) bool {
	healthy := p.probe(ip, port, username, password)
	if !healthy {
		metrics.IncProbeFailures()
	}
	p.cache.set(p.cacheKey(ip, port, username, password), healthy)
	return healthy
}

func (p *Prober) probe(ip string, port int, username, password string) bool {
	for _, host := range p.probeHosts(ip) {
		addr := fmt.Sprintf("%s:%d", host, port)
		conn, err := p.dial("tcp", addr)
		if err != nil {
			continue
		}
		ok := socks5AuthHandshake(conn, username, password, p.timeout)
		_ = conn.Close()
		if ok {
			return true
		}
	}
	return false
}

func (p *Prober) probeHosts(ip string) []string {
	hosts := []string{ip}
	if p.allowLoopbackFallback(ip) {
		hosts = append(hosts, "127.0.0.1")
	}
	return hosts
}

func (p *Prober) allowLoopbackFallback(ip string) bool {
	if isLoopbackHost(ip) {
		return false
	}
	if p.localFallback {
		return true
	}
	if p.sameHostIPs != nil {
		_, ok := p.sameHostIPs[ip]
		return ok
	}
	return false
}

func isLoopbackHost(ip string) bool {
	switch ip {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

// socks5AuthHandshake verifies SOCKS5 username/password authentication (RFC 1929).
func socks5AuthHandshake(conn net.Conn, username, password string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	if err := conn.SetDeadline(deadline); err != nil {
		return false
	}

	// Client greeting: VER=5, NMETHODS=1, METHOD=0x02 (username/password).
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return false
	}

	methodSel := make([]byte, 2)
	if _, err := io.ReadFull(conn, methodSel); err != nil {
		return false
	}
	if methodSel[0] != 0x05 || methodSel[1] != 0x02 {
		return false
	}

	ulen := len(username)
	plen := len(password)
	if ulen > 255 || plen > 255 {
		return false
	}

	authReq := make([]byte, 3+ulen+plen)
	authReq[0] = 0x01 // subnegotiation version
	authReq[1] = byte(ulen)
	copy(authReq[2:], username)
	authReq[2+ulen] = byte(plen)
	copy(authReq[3+ulen:], password)

	if _, err := conn.Write(authReq); err != nil {
		return false
	}

	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		return false
	}

	return authResp[0] == 0x01 && authResp[1] == 0x00
}
