// cmd/installer/installer.go

package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"text/template"

	"github.com/Skillz147/TrinityProxy/internal/logutil"
)

const (
	serviceName       = "trinityproxy"
	confPath          = "/etc/danted.conf"
	usernamePath      = "/etc/trinityproxy-username"
	passwordPath      = "/etc/trinityproxy-password"
	portPath          = "/etc/trinityproxy-port"
	serviceFile       = "/etc/systemd/system/trinityproxy.service"
	danteUser         = "nobody"
	agentRuntimeGroup = "trinityproxy-agent"
)

var log *slog.Logger

// Generate secure hex string
func GenerateRandomString(n int) string {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err != nil {
		panic("unable to generate secure random string")
	}
	return hex.EncodeToString(bytes)
}

// Choose random high port (range: 20000–59999)
func getRandomPort() int {
	portRange := big.NewInt(40000)
	start := int64(20000)
	n, err := rand.Int(rand.Reader, portRange)
	if err != nil {
		panic("failed to generate random port")
	}
	return int(start + n.Int64())
}

// detectPrimaryInterface finds the primary network interface
func detectPrimaryInterface() string {
	// Method 1: Use ip route to find default gateway interface
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "default via") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "dev" && i+1 < len(fields) {
						iface := fields[i+1]
						log.Info("detected primary interface", "interface", iface, "method", "ip route")
						return iface
					}
				}
			}
		}
	}

	// Method 2: Find interface with default route using route command
	cmd = exec.Command("route", "-n")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "0.0.0.0") {
				fields := strings.Fields(line)
				if len(fields) >= 8 {
					iface := fields[7]
					log.Info("detected primary interface", "interface", iface, "method", "route")
					return iface
				}
			}
		}
	}

	// Method 3: Use Go's net package to find interface with global unicast address
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
				addrs, err := iface.Addrs()
				if err == nil {
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok {
							if ipnet.IP.IsGlobalUnicast() && ipnet.IP.To4() != nil {
								log.Info("detected primary interface", "interface", iface.Name, "method", "go net")
								return iface.Name
							}
						}
					}
				}
			}
		}
	}

	// Method 4: Check common interface names
	commonNames := []string{"ens5", "ens3", "enp0s3", "enp0s5", "eth0", "ens160"}
	for _, name := range commonNames {
		if _, err := os.Stat("/sys/class/net/" + name); err == nil {
			// Check if interface is up
			cmd := exec.Command("ip", "link", "show", name)
			output, err := cmd.Output()
			if err == nil && strings.Contains(string(output), "state UP") {
				log.Info("detected primary interface", "interface", name, "method", "fallback check")
				return name
			}
		}
	}

	// Final fallback
	log.Warn("could not detect interface, using fallback", "interface", "eth0")
	return "eth0"
}

func credentialsExist() bool {
	for _, path := range []string{usernamePath, passwordPath, portPath} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func readExistingCredentials() (string, string, int, error) {
	usernameBytes, err := os.ReadFile(usernamePath)
	if err != nil {
		return "", "", 0, fmt.Errorf("read username: %w", err)
	}
	passwordBytes, err := os.ReadFile(passwordPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("read password: %w", err)
	}
	portBytes, err := os.ReadFile(portPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("read port: %w", err)
	}

	username := strings.TrimSpace(string(usernameBytes))
	password := strings.TrimSpace(string(passwordBytes))
	port, err := strconv.Atoi(strings.TrimSpace(string(portBytes)))
	if err != nil {
		return "", "", 0, fmt.Errorf("parse port: %w", err)
	}

	return username, password, port, nil
}

func isRoot() bool {
	return os.Geteuid() == 0
}

func setCredentialPermissions() {
	// Allow the agent runtime user to read credentials without world-readable modes.
	g, err := user.LookupGroup(agentRuntimeGroup)
	if err != nil {
		log.Warn("agent runtime group not found; credential files remain root-only",
			"group", agentRuntimeGroup, "err", err,
			"hint", "run install-agent-service.sh")
		return
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return
	}

	for _, path := range []string{usernamePath, passwordPath, portPath} {
		if err := os.Chown(path, 0, gid); err != nil {
			log.Warn("failed to chown credential file", "path", path, "err", err)
			continue
		}
		if err := os.Chmod(path, 0640); err != nil {
			log.Warn("failed to chmod credential file", "path", path, "err", err)
		}
	}
}

func generateCredentials() (string, string, int) {
	username := "u_" + GenerateRandomString(4)
	password := GenerateRandomString(12)
	port := resolveSOCKSPort()

	os.WriteFile(usernamePath, []byte(username), 0600)
	os.WriteFile(passwordPath, []byte(password), 0600)
	os.WriteFile(portPath, []byte(fmt.Sprintf("%d", port)), 0600)
	if isRoot() {
		setCredentialPermissions()
	}

	return username, password, port
}

func resolveSOCKSPort() int {
	if v := strings.TrimSpace(os.Getenv("TRINITY_SOCKS_PORT")); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			return port
		}
	}
	return getRandomPort()
}

func writeDanteConf(username, password string, port int) error {
	danteInterface := detectPrimaryInterface()

	conf := `# Dante SOCKS5 Server Configuration

logoutput: /var/log/danted.log
internal: {{.Interface}} port = {{.Port}}
external: {{.Interface}}

# Require username authentication (no anonymous access)
socksmethod: username
user.notprivileged: {{.User}}

client pass {
  from: 0.0.0.0/0 to: 0.0.0.0/0
  log: connect disconnect
}

socks pass {
  from: 0.0.0.0/0 to: 0.0.0.0/0
  protocol: tcp udp
  command: connect
  log: connect disconnect
  socksmethod: username
}
`
	tmpl, err := template.New("danted").Parse(conf)
	if err != nil {
		return err
	}

	file, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer file.Close()

	data := map[string]interface{}{
		"Interface": danteInterface,
		"Port":      port,
		"User":      danteUser,
	}

	return tmpl.Execute(file, data)
}

func findDanteBinary() string {
	// Check common Dante binary locations and names
	candidates := []string{
		"/usr/sbin/danted", // Ubuntu/Debian
		"/usr/sbin/sockd",  // CentOS/RHEL/AlmaLinux
		"/usr/bin/danted",  // Alternative location
		"/usr/bin/sockd",   // Alternative location
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Fallback to PATH search
	if path, err := exec.LookPath("danted"); err == nil {
		return path
	}
	if path, err := exec.LookPath("sockd"); err == nil {
		return path
	}

	// Default fallback
	return "/usr/sbin/sockd"
}

func writeSystemdService() error {
	danteBinary := findDanteBinary()
	log.Info("using Dante binary", "path", danteBinary)

	service := `[Unit]
Description=TrinityProxy SOCKS5 Service
After=network.target

[Service]
ExecStart=` + danteBinary + ` -f /etc/danted.conf
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`
	return os.WriteFile(serviceFile, []byte(service), 0644)
}

func createSystemUser(username, password string) error {
	// Create system user for SOCKS authentication
	cmd := exec.Command("useradd", "-r", "-s", "/bin/false", username)
	if err := cmd.Run(); err != nil {
		// User might already exist, that's okay
		log.Info("system user may already exist", "username", username, "err", err)
	}

	// Set password for the user
	cmd = exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(username + ":" + password)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set password for user %s: %v", username, err)
	}

	log.Info("created system user", "username", username)
	return nil
}

func reloadAndStartService() {
	exec.Command("systemctl", "daemon-reexec").Run()
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", serviceName).Run()
	exec.Command("systemctl", "restart", serviceName).Run()
}

func main() {
	log = logutil.New("installer")

	force := flag.Bool("force", false, "regenerate credentials and overwrite existing configuration")
	flag.Parse()

	// Runtime (non-root): agent service invokes installer on every start; skip privileged
	// work when credentials already exist. Initial install requires root (one-time sudo).
	if credentialsExist() && !*force && !isRoot() {
		log.Info("using existing credentials", "mode", "non-root runtime", "action", "skipping privileged install steps")
		return
	}

	if !credentialsExist() && !isRoot() {
		logutil.Fatal(log, "initial install requires root",
			"hint", "sudo ./build/installer or sudo make install-agent-service")
	}

	log.Info("setting up TrinityProxy SOCKS5 service")

	var username, password string
	var port int

	if credentialsExist() && !*force {
		var err error
		username, password, port, err = readExistingCredentials()
		if err != nil {
			logutil.Fatal(log, "failed to read existing credentials", "err", err)
		}
		log.Info("using existing credentials")
	} else {
		if *force && credentialsExist() {
			log.Info("--force specified; regenerating credentials")
		}
		username, password, port = generateCredentials()
	}

	if *force || !danteConfValid() {
		if err := writeDanteConf(username, password, port); err != nil {
			logutil.Fatal(log, "failed to write danted.conf", "err", err)
		}
	} else {
		log.Info("preserving existing danted.conf", "path", confPath)
	}

	if err := createSystemUser(username, password); err != nil {
		logutil.Fatal(log, "failed to create system user", "err", err)
	}

	if err := writeSystemdService(); err != nil {
		logutil.Fatal(log, "failed to write systemd service", "err", err)
	}

	setCredentialPermissions()
	reloadAndStartService()

	// Operator-facing output (not structured logs); password is shown once at install time.
	fmt.Printf("[+] TrinityProxy SOCKS5 is live on port %d\n", port)
	fmt.Printf("[+] Username: %s\n", username)
	fmt.Printf("[+] Password: %s\n", password)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// danteConfValid reports whether an existing danted.conf has the minimum fields
// TrinityProxy needs. Debian's dante-server package ships a stub config that must
// not be preserved.
func danteConfValid() bool {
	data, err := os.ReadFile(confPath)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "internal:") && strings.Contains(content, "socksmethod:")
}
