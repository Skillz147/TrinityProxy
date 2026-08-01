package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultPort          = 1080
	defaultPort          = DefaultPort
	defaultUsername      = "dev"
	defaultPassword      = "dev"
	defaultUsernameBytes = 8
	defaultPasswordBytes = 16

	envSocksPort     = "TRINITY_SOCKS_PORT"
	envSocksUser     = "TRINITY_SOCKS_USER"
	envSocksPass     = "TRINITY_SOCKS_PASS"
	envSocksPassAlt  = "TRINITY_SOCKS_PASSWORD"
	envDevProxyPort  = "TRINITY_DEV_PROXY_PORT"

	usernameFile = "trinityproxy-username"
	passwordFile = "trinityproxy-password"
	portFile     = "trinityproxy-port"
)

// Config holds embedded SOCKS5 listen settings and credentials.
type Config struct {
	Port     int
	Username string
	Password string
}

// Credentials is the SOCKS endpoint reported in agent heartbeats.
type Credentials struct {
	Port     int
	Username string
	Password string
}

// SocksPort returns the configured listen port from TRINITY_SOCKS_PORT or
// TRINITY_DEV_PROXY_PORT (legacy), defaulting to 1080.
func SocksPort() int {
	for _, key := range []string{envSocksPort, envDevProxyPort} {
		if p := strings.TrimSpace(os.Getenv(key)); p != "" {
			if n, err := strconv.Atoi(p); err == nil && n >= 0 && n <= 65535 {
				return n
			}
		}
	}
	return defaultPort
}

// DataDir returns the directory used for credential persistence.
// TRINITY_DATA_DIR overrides; otherwise the executable's directory is used.
func DataDir() string {
	if dir := strings.TrimSpace(os.Getenv("TRINITY_DATA_DIR")); dir != "" {
		return dir
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ConfigFromEnv builds config from environment variables, generating credentials when unset.
func ConfigFromEnv() Config {
	port := SocksPort()

	username := strings.TrimSpace(os.Getenv(envSocksUser))
	password := firstNonEmpty(
		strings.TrimSpace(os.Getenv(envSocksPass)),
		strings.TrimSpace(os.Getenv(envSocksPassAlt)),
	)

	if username == "" && password == "" && os.Getenv("TRINITY_SKIP_INSTALLER") == "1" {
		return Config{Port: port, Username: defaultUsername, Password: defaultPassword}
	}

	if username == "" || password == "" {
		if persisted, err := loadPersistedCredentials(); err == nil {
			if username == "" {
				username = persisted.Username
			}
			if password == "" {
				password = persisted.Password
			}
			if os.Getenv(envSocksPort) == "" && os.Getenv(envDevProxyPort) == "" && persisted.Port > 0 {
				port = persisted.Port
			}
		}
	}

	if (username == "" || password == "") && os.Getenv("TRINITY_SKIP_INSTALLER") == "1" {
		if username == "" {
			username = defaultUsername
		}
		if password == "" {
			password = defaultPassword
		}
		return Config{Port: port, Username: username, Password: password}
	}

	if username == "" || password == "" {
		var err error
		username, password, err = generateCredentials()
		if err != nil {
			username = defaultUsername
			password = defaultPassword
		} else if port > 0 {
			_ = persistCredentials(port, username, password)
		}
	}

	return Config{Port: port, Username: username, Password: password}
}

func loadPersistedCredentials() (Credentials, error) {
	dir := DataDir()
	username, uErr := readCredentialFile(filepath.Join(dir, usernameFile))
	password, pErr := readCredentialFile(filepath.Join(dir, passwordFile))
	if uErr != nil || pErr != nil || username == "" || password == "" {
		return Credentials{}, fmt.Errorf("credentials not found")
	}
	port := defaultPort
	if portStr, err := readCredentialFile(filepath.Join(dir, portFile)); err == nil && portStr != "" {
		if n, err := strconv.Atoi(portStr); err == nil && n > 0 {
			port = n
		}
	}
	return Credentials{Port: port, Username: username, Password: password}, nil
}

func persistCredentials(port int, username, password string) error {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, usernameFile), []byte(username), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, passwordFile), []byte(password), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, portFile), []byte(strconv.Itoa(port)), 0o600)
}

func readCredentialFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func generateCredentials() (username, password string, err error) {
	username, err = randomHex(defaultUsernameBytes)
	if err != nil {
		return "", "", err
	}
	password, err = randomHex(defaultPasswordBytes)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
