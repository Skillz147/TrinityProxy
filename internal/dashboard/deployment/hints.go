package deployment

import (
	"fmt"
	"net"
	"strings"
)

type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Notes string `json:"notes,omitempty"`
}

type DNSHints struct {
	Domain  string      `json:"domain"`
	Records []DNSRecord `json:"records"`
	Summary string      `json:"summary"`
}

type DevSetup struct {
	PublicDomain         string `json:"public_domain"`
	SuggestedController  string `json:"suggested_controller_url"`
	HostsFileEntry       string `json:"hosts_file_entry"`
	MkcertInstructions   string `json:"mkcert_instructions"`
	ControllerNote       string `json:"controller_note"`
}

func BuildDNSHints(domain, serverIP, sslMode string) DNSHints {
	domain = NormalizeDomain(domain)
	serverIP = strings.TrimSpace(serverIP)

	if domain == "" {
		return DNSHints{
			Records: []DNSRecord{},
			Summary: "Set a public domain in deployment settings to see DNS records.",
		}
	}

	apiHost := APIHost(domain)

	switch sslMode {
	case SSLModeDevMkcert, SSLModeNone:
		return buildDevDNSHints(domain, apiHost, serverIP)
	default:
		return buildProductionDNSHints(domain, apiHost, serverIP)
	}
}

func buildDevDNSHints(domain, apiHost, serverIP string) DNSHints {
	ip := "127.0.0.1"
	if serverIP != "" {
		ip = serverIP
	}

	hostsLine := fmt.Sprintf("%s %s %s", ip, domain, apiHost)
	summary := fmt.Sprintf(
		"Add this line to /etc/hosts for local development (IP%s). No Cloudflare A records needed.",
		formatIPHint(ip),
	)

	return DNSHints{
		Domain: domain,
		Records: []DNSRecord{
			{
				Type:  "hosts",
				Name:  "/etc/hosts",
				Value: hostsLine,
				Notes: "Controller API and dashboard hostnames for local dev",
			},
		},
		Summary: summary,
	}
}

func buildProductionDNSHints(domain, apiHost, serverIP string) DNSHints {
	ip := fallbackIP(serverIP)

	records := []DNSRecord{
		{
			Type:  "A",
			Name:  apiHost,
			Value: ip,
			Notes: "Required — controller API. For Caddy + Cloudflare wildcard SSL, use Proxied (orange cloud) and provision via Settings → Set up Cloudflare SSL.",
		},
		{
			Type:  "A",
			Name:  domain,
			Value: ip,
			Notes: "Required — dashboard at apex domain. Use Proxied (orange cloud) when using DNS-01 wildcard provisioning.",
		},
	}

	summary := fmt.Sprintf(
		"For Caddy SSL with Cloudflare, create proxied A records: %s → %s and %s → same IP. "+
			"Use Settings → Set up Cloudflare SSL to issue a wildcard certificate via DNS-01.",
		apiHost,
		ip,
		domain,
	)

	return DNSHints{
		Domain:  domain,
		Records: records,
		Summary: summary,
	}
}

func BuildDevSetup(domain, serverIP string) DevSetup {
	domain = NormalizeDomain(domain)
	if domain == "" {
		domain = "trinityproxy.local"
	}

	localAPI := APIHost(domain)
	suggested := fmt.Sprintf("http://%s:3100", localAPI)

	hostsEntry := fmt.Sprintf("127.0.0.1 %s %s", domain, localAPI)
	if serverIP != "" && serverIP != "127.0.0.1" {
		hostsEntry = fmt.Sprintf("%s %s %s", serverIP, domain, localAPI)
	}

	mkcert := fmt.Sprintf(`# Install mkcert (https://github.com/FiloSottile/mkcert)
mkcert -install
mkcert %s %s
# Configure Caddy to use the generated certs, then set ssl_mode to "dev-mkcert"
# and controller URL to https://%s`, domain, localAPI, localAPI)

	return DevSetup{
		PublicDomain:        domain,
		SuggestedController: suggested,
		HostsFileEntry:      hostsEntry,
		MkcertInstructions:  mkcert,
		ControllerNote:      "For VPS IP-only dev, use the suggested controller URL and ssl_mode \"none\". Set TRINITY_AGENT_KEY on the controller process to match the dashboard agent key. On macOS, test heartbeats locally with make run-agent-dev; use the install script on a Linux VPS for production agents.",
	}
}

func ResolveServerIP(host string, remoteAddr string) string {
	host = strings.TrimSpace(host)
	if ip := net.ParseIP(host); ip != nil {
		return host
	}

	if host != "" && !strings.Contains(host, ":") {
		if ips, err := net.LookupIP(host); err == nil {
			for _, ip := range ips {
				if v4 := ip.To4(); v4 != nil {
					return v4.String()
				}
			}
		}
	}

	if remoteAddr != "" {
		if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
			if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
				return host
			}
		}
	}

	return ""
}

func fallbackIP(serverIP string) string {
	if serverIP != "" {
		return serverIP
	}
	return "YOUR_VPS_IP"
}

func formatIPHint(ip string) string {
	if ip == "" {
		return " (set SERVER_PUBLIC_IP or configure domain first)"
	}
	return " " + ip
}
