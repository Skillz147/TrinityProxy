package health

import (
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSocks5AuthHandshakeSuccess(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		greeting := make([]byte, 3)
		if _, err := io.ReadFull(serverConn, greeting); err != nil {
			return
		}
		if _, err := serverConn.Write([]byte{0x05, 0x02}); err != nil {
			return
		}

		authHeader := make([]byte, 2)
		if _, err := io.ReadFull(serverConn, authHeader); err != nil {
			return
		}
		user := make([]byte, authHeader[1])
		if _, err := io.ReadFull(serverConn, user); err != nil {
			return
		}
		passLen := make([]byte, 1)
		if _, err := io.ReadFull(serverConn, passLen); err != nil {
			return
		}
		pass := make([]byte, passLen[0])
		if _, err := io.ReadFull(serverConn, pass); err != nil {
			return
		}

		if string(user) != "alice" || string(pass) != "secret" {
			_, _ = serverConn.Write([]byte{0x01, 0x01})
			return
		}
		_, _ = serverConn.Write([]byte{0x01, 0x00})
	}()

	if !socks5AuthHandshake(clientConn, "alice", "secret", time.Second) {
		t.Fatal("expected successful SOCKS5 auth handshake")
	}
}

func TestSocks5AuthHandshakeAuthFailure(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		greeting := make([]byte, 3)
		_, _ = io.ReadFull(serverConn, greeting)
		_, _ = serverConn.Write([]byte{0x05, 0x02})

		authHeader := make([]byte, 2)
		_, _ = io.ReadFull(serverConn, authHeader)
		user := make([]byte, authHeader[1])
		_, _ = io.ReadFull(serverConn, user)
		passLen := make([]byte, 1)
		_, _ = io.ReadFull(serverConn, passLen)
		pass := make([]byte, passLen[0])
		_, _ = io.ReadFull(serverConn, pass)
		_, _ = serverConn.Write([]byte{0x01, 0x01})
	}()

	if socks5AuthHandshake(clientConn, "alice", "wrong", time.Second) {
		t.Fatal("expected auth failure")
	}
}

func TestIsHealthyWithMockDialer(t *testing.T) {
	addr, cleanup := startMockSOCKS5Server(t, "bob", "pass")
	defer cleanup()

	prober := NewProber(WithTimeout(time.Second), WithCacheTTL(30*time.Second))

	if !prober.IsHealthy("127.0.0.1", addr.Port, "bob", "pass") {
		t.Fatal("expected node to be healthy")
	}

	badAddr, badCleanup := startMockSOCKS5Server(t, "bob", "pass")
	defer badCleanup()

	if prober.IsHealthy("127.0.0.1", badAddr.Port, "bob", "wrong") {
		t.Fatal("expected node with bad password to be unhealthy")
	}
}

func TestProbeCachesResults(t *testing.T) {
	addr, cleanup := startMockSOCKS5Server(t, "cache", "pass")
	defer cleanup()

	var dialCount int32
	prober := NewProber(
		WithTimeout(time.Second),
		WithCacheTTL(200*time.Millisecond),
		WithDialFunc(func(network, address string) (net.Conn, error) {
			atomic.AddInt32(&dialCount, 1)
			return net.Dial(network, address)
		}),
	)

	if !prober.IsHealthy("127.0.0.1", addr.Port, "cache", "pass") {
		t.Fatal("expected first probe to succeed")
	}
	if !prober.IsHealthy("127.0.0.1", addr.Port, "cache", "pass") {
		t.Fatal("expected cached probe to succeed")
	}
	if atomic.LoadInt32(&dialCount) != 1 {
		t.Fatalf("expected 1 dial within cache TTL, got %d", dialCount)
	}

	time.Sleep(250 * time.Millisecond)

	if !prober.IsHealthy("127.0.0.1", addr.Port, "cache", "pass") {
		t.Fatal("expected probe after cache expiry to succeed")
	}
	if atomic.LoadInt32(&dialCount) != 2 {
		t.Fatalf("expected 2 dials after cache expiry, got %d", dialCount)
	}
}

func TestIsHealthyDialFailure(t *testing.T) {
	prober := NewProber(
		WithTimeout(100*time.Millisecond),
		WithDialFunc(func(network, address string) (net.Conn, error) {
			return nil, io.EOF
		}),
	)

	if prober.IsHealthy("127.0.0.1", 1, "user", "pass") {
		t.Fatal("expected unhealthy when dial fails")
	}
}

func TestLocalFallbackProbesLoopback(t *testing.T) {
	addr, cleanup := startMockSOCKS5Server(t, "dev", "dev")
	defer cleanup()

	var dialed []string
	prober := NewProber(
		WithTimeout(time.Second),
		WithLocalFallback(true),
		WithDialFunc(func(network, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			if strings.HasPrefix(address, "203.0.113.") {
				return nil, io.EOF
			}
			return net.Dial(network, address)
		}),
	)

	if !prober.ProbeFresh("203.0.113.99", addr.Port, "dev", "dev") {
		t.Fatal("expected localhost fallback probe to succeed")
	}
	if len(dialed) != 2 {
		t.Fatalf("expected 2 dial attempts (WAN then loopback), got %d: %v", len(dialed), dialed)
	}
	if !strings.HasPrefix(dialed[0], "203.0.113.99:") {
		t.Fatalf("first dial = %q, want WAN address", dialed[0])
	}
	if !strings.HasPrefix(dialed[1], "127.0.0.1:") {
		t.Fatalf("second dial = %q, want loopback", dialed[1])
	}
}

func TestLocalFallbackDisabledSkipsLoopback(t *testing.T) {
	var dialed []string
	prober := NewProber(
		WithTimeout(100*time.Millisecond),
		WithLocalFallback(false),
		WithDialFunc(func(network, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return nil, io.EOF
		}),
	)

	if prober.ProbeFresh("203.0.113.99", 1080, "dev", "dev") {
		t.Fatal("expected probe to fail")
	}
	if len(dialed) != 1 {
		t.Fatalf("expected 1 dial without fallback, got %d: %v", len(dialed), dialed)
	}
}

type mockAddr struct {
	Port int
}

func startMockSOCKS5Server(t *testing.T, user, pass string) (mockAddr, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleMockSOCKS5(conn, user, pass)
		}
	}()

	tcpAddr := ln.Addr().(*net.TCPAddr)
	cleanup := func() {
		_ = ln.Close()
		<-done
	}
	return mockAddr{Port: tcpAddr.Port}, cleanup
}

func handleMockSOCKS5(conn net.Conn, user, pass string) {
	defer conn.Close()

	greeting := make([]byte, 3)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x02}); err != nil {
		return
	}

	authHeader := make([]byte, 2)
	if _, err := io.ReadFull(conn, authHeader); err != nil {
		return
	}
	gotUser := make([]byte, authHeader[1])
	if _, err := io.ReadFull(conn, gotUser); err != nil {
		return
	}
	passLen := make([]byte, 1)
	if _, err := io.ReadFull(conn, passLen); err != nil {
		return
	}
	gotPass := make([]byte, passLen[0])
	if _, err := io.ReadFull(conn, gotPass); err != nil {
		return
	}

	status := byte(0x01)
	if string(gotUser) == user && string(gotPass) == pass {
		status = 0x00
	}
	_, _ = conn.Write([]byte{0x01, status})
}
