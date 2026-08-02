package agent

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func linuxDynamicStatus() (string, error) {
	var b strings.Builder
	b.WriteString("=== TrinityProxy Agent Status")
	if inContainer() {
		b.WriteString(" (container)")
	}
	b.WriteByte('\n')

	if embeddedSOCKSMode() {
		b.WriteString("Proxy mode: embedded SOCKS5 (Go)\n")
		port, _, _ := embeddedProxyCredentials()
		if port > 0 {
			fmt.Fprintf(&b, "SOCKS port: %d\n", port)
		}
		if proxyProcessRunning() {
			b.WriteString("Embedded proxy: running\n")
		} else {
			b.WriteString("Embedded proxy: not running\n")
		}
	} else {
		b.WriteString("Proxy mode: Dante SOCKS5\n")
		if danteRunning() {
			b.WriteString("Dante: running\n")
		} else {
			b.WriteString("Dante: not running\n")
		}
		port := resolveStatusPort()
		if port > 0 {
			fmt.Fprintf(&b, "SOCKS port: %d\n", port)
		}
	}

	port := resolveStatusPort()
	if port <= 0 {
		b.WriteString("SOCKS port: unknown (no port file or config)\n")
		return b.String(), nil
	}

	if portListening(port) {
		fmt.Fprintf(&b, "Port %d: LISTENING\n", port)
	} else {
		fmt.Fprintf(&b, "Port %d: NOT LISTENING\n", port)
	}
	return b.String(), nil
}

func resolveStatusPort() int {
	if embeddedSOCKSMode() {
		port, _, _ := embeddedProxyCredentials()
		if port > 0 {
			return port
		}
	}

	if v := strings.TrimSpace(os.Getenv("TRINITY_SOCKS_PORT")); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			return port
		}
	}

	for _, path := range []string{"/etc/trinityproxy-port", filepath.Join(proxyDataDir(), "trinityproxy-port")} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if port, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && port > 0 {
			return port
		}
	}
	return 0
}

func proxyDataDir() string {
	if dir := strings.TrimSpace(os.Getenv("TRINITY_DATA_DIR")); dir != "" {
		return dir
	}
	if root := strings.TrimSpace(os.Getenv("TRINITY_ROOT")); root != "" {
		return filepath.Join(root, "build")
	}
	return "."
}

func danteRunning() bool {
	return processNameRunning("danted") || processNameRunning("sockd")
}

func proxyProcessRunning() bool {
	return processNameRunning("trinityproxy")
}

func processNameRunning(name string) bool {
	if running, ok := processNameRunningProc(name); ok {
		return running
	}
	out, err := exec.Command("pgrep", "-x", name).Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func processNameRunningProc(name string) (bool, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == name {
			return true, true
		}
	}
	return false, true
}

func portListening(port int) bool {
	if port <= 0 {
		return false
	}
	hosts := localProbeHosts()
	addr := strconv.Itoa(port)
	for _, host := range hosts {
		if canConnect(host, addr) {
			return true
		}
	}
	return false
}

func localProbeHosts() []string {
	seen := make(map[string]struct{})
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
	}

	add("127.0.0.1")
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			add(ipNet.IP.String())
		}
	}
	hosts := make([]string, 0, len(seen))
	for host := range seen {
		hosts = append(hosts, host)
	}
	return hosts
}

func canConnect(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
