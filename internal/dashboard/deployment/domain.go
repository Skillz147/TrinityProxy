package deployment

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var ErrInvalidDomain = errors.New("invalid public_domain")

// NormalizeDomain strips protocol, paths, ports, and optional api. prefix so
// settings store a bare hostname (e.g. trinityproxy.local).
func NormalizeDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}

	if idx := strings.Index(raw, "://"); idx >= 0 {
		raw = raw[idx+3:]
	}

	if idx := strings.IndexAny(raw, "/?#"); idx >= 0 {
		raw = raw[:idx]
	}

	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}

	raw = strings.TrimSuffix(raw, ".")

	raw = strings.TrimPrefix(raw, "api.")

	return raw
}

func ValidateDomain(domain string) error {
	if domain == "" {
		return nil
	}
	if strings.ContainsAny(domain, ":/\\") {
		return ErrInvalidDomain
	}
	if len(domain) > 253 {
		return ErrInvalidDomain
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return ErrInvalidDomain
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return ErrInvalidDomain
		}
	}
	return nil
}

func APIHost(domain string) string {
	domain = NormalizeDomain(domain)
	if domain == "" {
		return ""
	}
	return "api." + domain
}

// NormalizeControllerURL ensures a full URL with scheme. Bare hostnames get a
// scheme derived from sslMode (http for none/VPS, https otherwise).
func NormalizeControllerURL(raw, sslMode string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}

	if !strings.Contains(raw, "://") {
		switch sslMode {
		case SSLModeNone:
			raw = "http://" + raw
		default:
			if strings.Contains(raw, ":3100") {
				raw = "http://" + raw
			} else {
				raw = "https://" + raw
			}
		}
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}

	host := u.Hostname()
	port := u.Port()
	switch port {
	case "":
		if scheme == "http" && sslMode == SSLModeNone {
			return fmt.Sprintf("http://%s:3100", host)
		}
		return fmt.Sprintf("%s://%s", scheme, host)
	default:
		return fmt.Sprintf("%s://%s:%s", scheme, host, port)
	}
}
