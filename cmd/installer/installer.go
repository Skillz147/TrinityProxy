// cmd/installer/installer.go

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

const (
	serviceName    = "trinityproxy"
	confPath       = "/etc/danted.conf"
	usernamePath   = "/etc/trinityproxy-username"
	passwordPath   = "/etc/trinityproxy-password"
	portPath       = "/etc/trinityproxy-port"
	serviceFile    = "/etc/systemd/system/trinityproxy.service"
	danteUser      = "nobody"
	danteInterface = "eth0"
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

func writeSystemdService() error {
	service := `[Unit]
Description=TrinityProxy SOCKS5 Service
After=network.target

[Service]
ExecStart=/usr/sbin/sockd -f /etc/danted.conf
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
