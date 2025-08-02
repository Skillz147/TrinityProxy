// cmd/installer/installer.go

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

const (
	serviceName  = "trinityproxy"
	confPath     = "/etc/danted.conf"
	usernamePath = "/etc/trinityproxy-username"
	passwordPath = "/etc/trinityproxy-password"
	portPath     = "/etc/trinityproxy-port"
	serviceFile  = "/etc/systemd/system/trinityproxy.service"
	danteUser    = "nobody"
)

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
						fmt.Printf("[*] Detected primary interface: %s (via ip route)\n", iface)
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
					fmt.Printf("[*] Detected primary interface: %s (via route)\n", iface)
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
								fmt.Printf("[*] Detected primary interface: %s (via Go net)\n", iface.Name)
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
				fmt.Printf("[*] Detected primary interface: %s (fallback check)\n", name)
				return name
			}
		}
	}

	// Final fallback
	fmt.Printf("[!] Could not detect interface, using fallback: eth0\n")
	return "eth0"
}

func generateCredentials() (string, string, int) {
	username := "u_" + GenerateRandomString(4)
	password := GenerateRandomString(12)
	port := getRandomPort()

	os.WriteFile(usernamePath, []byte(username), 0600)
	os.WriteFile(passwordPath, []byte(password), 0600)
	os.WriteFile(portPath, []byte(fmt.Sprintf("%d", port)), 0600)

	return username, password, port
}

func writeDanteConf(username, password string, port int) error {
	danteInterface := detectPrimaryInterface()

	conf := `# Dante SOCKS5 Server Configuration

logoutput: /var/log/danted.log
internal: {{.Interface}} port = {{.Port}}
external: {{.Interface}}

# Support both username authentication and no authentication
socksmethod: username none
user.notprivileged: {{.User}}

client pass {
  from: 0.0.0.0/0 to: 0.0.0.0/0
  log: connect disconnect
}

# Allow authenticated connections
socks pass {
  from: 0.0.0.0/0 to: 0.0.0.0/0
  protocol: tcp udp
  command: connect
  log: connect disconnect
  socksmethod: username
}

# Allow anonymous connections
socks pass {
  from: 0.0.0.0/0 to: 0.0.0.0/0
  protocol: tcp udp
  command: connect
  log: connect disconnect
  socksmethod: none
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
	fmt.Printf("[*] Using Dante binary: %s\n", danteBinary)

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
		fmt.Printf("[*] User %s might already exist: %v\n", username, err)
	}

	// Set password for the user
	cmd = exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(username + ":" + password)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set password for user %s: %v", username, err)
	}

	fmt.Printf("[+] Created system user: %s\n", username)
	return nil
}

func reloadAndStartService() {
	exec.Command("systemctl", "daemon-reexec").Run()
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", serviceName).Run()
	exec.Command("systemctl", "restart", serviceName).Run()
}

func main() {
	fmt.Println("[+] Setting up TrinityProxy SOCKS5 service...")

	username, password, port := generateCredentials()

	if err := writeDanteConf(username, password, port); err != nil {
		log.Fatalf("[-] Failed to write danted.conf: %v", err)
	}

	if err := createSystemUser(username, password); err != nil {
		log.Fatalf("[-] Failed to create system user: %v", err)
	}

	if err := writeSystemdService(); err != nil {
		log.Fatalf("[-] Failed to write systemd service: %v", err)
	}

	reloadAndStartService()
	fmt.Printf("[+] TrinityProxy SOCKS5 is live on port %d\n", port)
	fmt.Printf("[+] Username: %s\n", username)
	fmt.Printf("[+] Password: %s\n", password)
}
