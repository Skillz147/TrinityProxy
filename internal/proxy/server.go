package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/Skillz147/TrinityProxy/internal/logutil"
)

// Server is a running embedded SOCKS5 listener.
type Server struct {
	Port     int
	Username string
	Password string

	listener net.Listener
	wg       sync.WaitGroup
	mu       sync.Mutex
	closed   bool
}

var (
	activeMu     sync.RWMutex
	activeServer *Server
)

// Active returns the running embedded SOCKS server, if any.
func Active() *Server {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeServer
}

// NewServer returns a server that has not started listening yet.
func NewServer(cfg Config) *Server {
	if cfg.Port < 0 {
		cfg.Port = defaultPort
	}
	if cfg.Username == "" {
		cfg.Username = defaultUsername
	}
	if cfg.Password == "" {
		cfg.Password = defaultPassword
	}
	return &Server{
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
	}
}

// Credentials returns the SOCKS auth settings for this server.
func (s *Server) Credentials() Credentials {
	return Credentials{
		Port:     s.Port,
		Username: s.Username,
		Password: s.Password,
	}
}

// ListenAddr returns the bind address (e.g. ":1080").
func (s *Server) ListenAddr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf(":%d", s.Port)
}

// StartEmbedded loads config from the environment, listens on TRINITY_SOCKS_PORT,
// and serves SOCKS5 in the background. Credentials are available via Active()
// or the returned Server's Credentials() method.
func StartEmbedded() (*Server, error) {
	srv := NewServer(ConfigFromEnv())
	return srv.startBackground()
}

// Start launches the embedded SOCKS5 server in a background goroutine.
func Start(creds Credentials) error {
	_, err := NewServer(Config{
		Port:     creds.Port,
		Username: creds.Username,
		Password: creds.Password,
	}).startBackground()
	return err
}

// Serve listens until ctx is cancelled, then closes the listener and waits for
// in-flight connections to finish.
func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.ListenAddr())
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.ListenAddr(), err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.bindPortFromListener(ln)

	auth := s.Credentials().authenticator()
	log := logutil.Component("proxy")
	log.Info("embedded SOCKS5 listening", "addr", ln.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.acceptLoop(ln, auth)
	}()

	select {
	case <-ctx.Done():
		s.shutdown(ln)
		s.wg.Wait()
		return nil
	case err := <-errCh:
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				s.wg.Wait()
				return nil
			}
		}
		return err
	}
}

func (s *Server) startBackground() (*Server, error) {
	ln, err := net.Listen("tcp", s.ListenAddr())
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", s.ListenAddr(), err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.bindPortFromListener(ln)

	activeMu.Lock()
	activeServer = s
	activeMu.Unlock()

	auth := s.Credentials().authenticator()
	log := logutil.Component("proxy")
	log.Info("embedded SOCKS5 listening", "addr", ln.Addr().String())

	go func() {
		_ = s.acceptLoop(ln, auth)
	}()

	return s, nil
}

func (s *Server) bindPortFromListener(ln net.Listener) {
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok && tcpAddr.Port > 0 {
		s.Port = tcpAddr.Port
	}
}

func (s *Server) acceptLoop(ln net.Listener, auth authenticator) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			serveConn(conn, auth)
		}()
	}
}

func (s *Server) shutdown(ln net.Listener) {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	_ = ln.Close()
}

func (s *Server) handleConn(conn net.Conn) {
	serveConn(conn, s.Credentials().authenticator())
}
