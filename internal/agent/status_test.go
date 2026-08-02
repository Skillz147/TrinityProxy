package agent

import (
	"net"
	"testing"
	"time"
)

func TestPortListeningDetectsLocalListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	if !portListening(port) {
		t.Fatalf("expected port %d to be listening", port)
	}
}

func TestPortListeningReturnsFalseForClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	time.Sleep(10 * time.Millisecond)

	if portListening(port) {
		t.Fatalf("expected closed port %d to be unavailable", port)
	}
}
