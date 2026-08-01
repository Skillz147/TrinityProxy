package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	socksVersion       = 0x05
	authMethodUserPass = 0x02
	authMethodNoAccept = 0xFF
	cmdConnect         = 0x01
	atypIPv4           = 0x01
	atypDomain         = 0x03
	atypIPv6           = 0x04
	replySuccess       = 0x00
	replyGeneralFail   = 0x01
	replyNetUnreach    = 0x03
	replyHostUnreach   = 0x04
	replyConnRefused   = 0x05
)

type authenticator func(username, password string) bool

func (c Credentials) authenticator() authenticator {
	user := c.Username
	pass := c.Password
	return func(username, password string) bool {
		return username == user && password == pass
	}
}

func serveConn(conn net.Conn, auth authenticator) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))

	if !negotiateAuth(conn, auth) {
		return
	}

	target, err := readConnectRequest(conn)
	if err != nil {
		return
	}

	remote, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		writeConnectReply(conn, mapDialError(err))
		return
	}
	defer remote.Close()

	if err := writeConnectReply(conn, replySuccess); err != nil {
		return
	}

	_ = conn.SetDeadline(time.Time{})
	relay(conn, remote)
}

func negotiateAuth(conn net.Conn, auth authenticator) bool {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return false
	}
	if header[0] != socksVersion {
		return false
	}

	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return false
	}

	supportsUserPass := false
	for _, m := range methods {
		if m == authMethodUserPass {
			supportsUserPass = true
			break
		}
	}

	if !supportsUserPass {
		_, _ = conn.Write([]byte{socksVersion, authMethodNoAccept})
		return false
	}

	if _, err := conn.Write([]byte{socksVersion, authMethodUserPass}); err != nil {
		return false
	}

	return userPassAuth(conn, auth)
}

func userPassAuth(conn net.Conn, auth authenticator) bool {
	version := make([]byte, 1)
	if _, err := io.ReadFull(conn, version); err != nil || version[0] != 0x01 {
		return false
	}

	ulen := make([]byte, 1)
	if _, err := io.ReadFull(conn, ulen); err != nil {
		return false
	}

	username := make([]byte, int(ulen[0]))
	if _, err := io.ReadFull(conn, username); err != nil {
		return false
	}

	plen := make([]byte, 1)
	if _, err := io.ReadFull(conn, plen); err != nil {
		return false
	}

	password := make([]byte, int(plen[0]))
	if _, err := io.ReadFull(conn, password); err != nil {
		return false
	}

	ok := auth(string(username), string(password))
	status := byte(0x01)
	if ok {
		status = 0x00
	}
	_, err := conn.Write([]byte{0x01, status})
	return ok && err == nil
}

func readConnectRequest(conn net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != socksVersion || header[1] != cmdConnect {
		return "", fmt.Errorf("unsupported socks request")
	}

	var host string
	switch header[3] {
	case atypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	case atypDomain:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return "", err
		}
		domain := make([]byte, int(lenByte[0]))
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		host = string(domain)
	case atypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	default:
		return "", fmt.Errorf("unsupported address type %d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	return fmt.Sprintf("%s:%d", host, port), nil
}

func writeConnectReply(conn net.Conn, status byte) error {
	reply := []byte{socksVersion, status, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}
	_, err := conn.Write(reply)
	return err
}

func mapDialError(err error) byte {
	if opErr, ok := err.(*net.OpError); ok {
		switch opErr.Err.Error() {
		case "connection refused":
			return replyConnRefused
		case "no route to host", "network is unreachable":
			return replyNetUnreach
		case "host unreachable", "no such host":
			return replyHostUnreach
		}
	}
	return replyGeneralFail
}

func relay(left, right net.Conn) {
	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(right, left); errCh <- err }()
	go func() { _, err := io.Copy(left, right); errCh <- err }()
	<-errCh
}
