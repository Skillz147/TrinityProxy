package health

import (
	"os"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/config"
)

// NewProberFromConfig builds a SOCKS prober using controller/dashboard runtime settings.
func NewProberFromConfig(cfg config.Config) *Prober {
	opts := []ProberOption{WithLocalFallback(cfg.ProbeLocalFallback())}
	for _, ip := range probeSameHostIPs() {
		opts = append(opts, WithSameHostIP(ip))
	}
	return NewProber(opts...)
}

func probeSameHostIPs() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" || isLoopbackHost(ip) {
			return
		}
		if _, ok := seen[ip]; ok {
			return
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}

	add(os.Getenv("SERVER_PUBLIC_IP"))
	add(os.Getenv("TRINITY_CONTROLLER_PUBLIC_IP"))
	return out
}
