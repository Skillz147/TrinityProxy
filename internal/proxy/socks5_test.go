package proxy

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestNegotiateAuthSuccess(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	auth := Credentials{Username: "alice", Password: "secret"}.authenticator()
	go func() {
		if !negotiateAuth(serverConn, auth) {
			t.Error("server auth negotiation failed")
		}
	}()

	if !clientAuthHandshake(t, clientConn, "alice", "secret") {
		t.Fatal("expected successful auth handshake")
	}
}

func TestNegotiateAuthFailure(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	auth := Credentials{Username: "alice", Password: "secret"}.authenticator()
	go func() {
		negotiateAuth(serverConn, auth)
	}()

	if clientAuthHandshake(t, clientConn, "alice", "wrong") {
		t.Fatal("expected auth failure")
	}
}

func TestConnectRelay(t *testing.T) {
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("target listen: %v", err)
	}
	defer targetLn.Close()

	go func() {
		conn, err := targetLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		_, _ = conn.Write(append([]byte("echo:"), buf[:n]...))
	}()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	creds := Credentials{Port: port, Username: "user", Password: "pass"}
	if err := Start(creds); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	client, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	if !clientAuthHandshake(t, client, "user", "pass") {
		t.Fatal("auth failed")
	}

	host, sport, _ := net.SplitHostPort(targetLn.Addr().String())
	if err := clientConnect(t, client, host, sport); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp := make([]byte, 16)
	n, err := io.ReadFull(client, resp[:9])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(resp[:n]) != "echo:ping" {
		t.Fatalf("response = %q, want echo:ping", string(resp[:n]))
	}
}

func clientAuthHandshake(t *testing.T, conn net.Conn, username, password string) bool {
	t.Helper()

	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		t.Fatalf("write greeting: %v", err)
	}

	methodSel := make([]byte, 2)
	if _, err := io.ReadFull(conn, methodSel); err != nil {
		t.Fatalf("read method: %v", err)
	}
	if methodSel[0] != 0x05 || methodSel[1] != 0x02 {
		t.Fatalf("method selection = %v", methodSel)
	}

	ulen := len(username)
	plen := len(password)
	req := make([]byte, 3+ulen+plen)
	req[0] = 0x01
	req[1] = byte(ulen)
	copy(req[2:], username)
	req[2+ulen] = byte(plen)
	copy(req[3+ulen:], password)

	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("read auth resp: %v", err)
	}

	return resp[0] == 0x01 && resp[1] == 0x00
}

func clientConnect(t *testing.T, conn net.Conn, host, port string) error {
	t.Helper()

	req := []byte{0x05, 0x01, 0x00, atypDomain, byte(len(host))}
	req = append(req, host...)
	p, err := parsePort(port)
	if err != nil {
		return err
	}
	req = append(req, byte(p>>8), byte(p))

	if _, err := conn.Write(req); err != nil {
		return err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[1] != replySuccess {
		return io.EOF
	}

	switch header[3] {
	case atypIPv4:
		if _, err := io.ReadFull(conn, make([]byte, 6)); err != nil {
			return err
		}
	case atypIPv6:
		if _, err := io.ReadFull(conn, make([]byte, 18)); err != nil {
			return err
		}
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return err
		}
		domain := make([]byte, int(lenBuf[0])+2)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return err
		}
	default:
		return io.EOF
	}

	return nil
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("TRINITY_DATA_DIR", t.TempDir())
	t.Setenv(envSocksPort, "")
	t.Setenv(envDevProxyPort, "")
	t.Setenv(envSocksUser, "")
	t.Setenv(envSocksPass, "")
	t.Setenv(envSocksPassAlt, "")

	cfg := ConfigFromEnv()
	if cfg.Port != DefaultPort {
		t.Fatalf("port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.Username == "" || cfg.Password == "" {
		t.Fatal("expected generated credentials")
	}
}

func TestConfigFromEnvProductionEmbedded(t *testing.T) {
	t.Setenv("TRINITY_DATA_DIR", t.TempDir())
	t.Setenv("TRINITY_SKIP_INSTALLER", "1")
	t.Setenv("TRINITY_NONINTERACTIVE", "1")
	t.Setenv("TRINITY_DEV", "")
	t.Setenv(envSocksPort, "10855")
	t.Setenv(envSocksUser, "")
	t.Setenv(envSocksPass, "")
	t.Setenv(envSocksPassAlt, "")

	cfg := ConfigFromEnv()
	if cfg.Port != 10855 {
		t.Fatalf("port = %d, want 10855", cfg.Port)
	}
	if cfg.Username == defaultUsername || cfg.Password == defaultPassword {
		t.Fatalf("production embedded mode must not use dev/dev, got %q/%q", cfg.Username, cfg.Password)
	}
	if cfg.Username == "" || cfg.Password == "" {
		t.Fatal("expected generated credentials")
	}
}

func TestConfigFromEnvDevMode(t *testing.T) {
	t.Setenv("TRINITY_DATA_DIR", t.TempDir())
	t.Setenv("TRINITY_DEV", "1")
	t.Setenv("TRINITY_SKIP_INSTALLER", "1")
	t.Setenv(envSocksPort, "1080")
	t.Setenv(envSocksUser, "")
	t.Setenv(envSocksPass, "")
	t.Setenv(envSocksPassAlt, "")

	cfg := ConfigFromEnv()
	if cfg.Username != defaultUsername || cfg.Password != defaultPassword {
		t.Fatalf("dev mode credentials = %q/%q, want dev/dev", cfg.Username, cfg.Password)
	}
}

func TestConfigFromEnvExplicit(t *testing.T) {
	t.Setenv(envSocksPort, "9050")
	t.Setenv(envSocksUser, "alice")
	t.Setenv(envSocksPass, "secret")

	cfg := ConfigFromEnv()
	if cfg.Port != 9050 {
		t.Fatalf("port = %d, want 9050", cfg.Port)
	}
	if cfg.Username != "alice" || cfg.Password != "secret" {
		t.Fatalf("credentials = %q/%q, want alice/secret", cfg.Username, cfg.Password)
	}
}

func parsePort(port string) (int, error) {
	var p int
	for _, c := range port {
		if c < '0' || c > '9' {
			return 0, io.EOF
		}
		p = p*10 + int(c-'0')
	}
	return p, nil
}
