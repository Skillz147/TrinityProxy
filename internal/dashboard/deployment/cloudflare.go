package deployment

import "fmt"

const cloudflareTokenURL = "https://dash.cloudflare.com/profile/api-tokens"

// SetupStep is one instruction shown in the Cloudflare SSL modal.
type SetupStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CloudflareSetup is returned by GET /api/dashboard/deployment/cloudflare-setup.
type CloudflareSetup struct {
	Domain      string      `json:"domain"`
	APIHost     string      `json:"api_host"`
	ServerIP    string      `json:"server_ip"`
	TokenURL    string      `json:"token_url"`
	TokenSteps  []SetupStep `json:"token_steps"`
	DNSRecords  []DNSRecord `json:"dns_records"`
	Summary     string      `json:"summary"`
	RenewalNote string      `json:"renewal_note"`
}

// BuildCloudflareSetup returns helper content for wildcard DNS-01 provisioning
// with proxied (orange cloud) Cloudflare records.
func BuildCloudflareSetup(domain, serverIP string) CloudflareSetup {
	domain = NormalizeDomain(domain)
	serverIP = fallbackIP(serverIP)
	apiHost := APIHost(domain)

	if domain == "" {
		return CloudflareSetup{
			TokenURL: cloudflareTokenURL,
			TokenSteps: buildCloudflareTokenSteps(),
			Summary:    "Set a public domain in deployment settings to see Cloudflare DNS records.",
			RenewalNote: renewalNote(),
		}
	}

	records := []DNSRecord{
		{
			Type:  "A",
			Name:  apiHost,
			Value: serverIP,
			Notes: "Required — controller API. Proxy status: Proxied (orange cloud).",
		},
		{
			Type:  "A",
			Name:  domain,
			Value: serverIP,
			Notes: "Required — dashboard at apex domain. Proxy status: Proxied (orange cloud).",
		},
	}

	summary := fmt.Sprintf(
		"Create proxied A records for %s and %s pointing to %s before provisioning. "+
			"Caddy uses Cloudflare DNS-01 to issue a wildcard certificate for *.%s and %s, "+
			"so orange cloud (proxied) mode is supported during issuance and renewal.",
		apiHost, domain, serverIP, domain, domain,
	)

	return CloudflareSetup{
		Domain:      domain,
		APIHost:     apiHost,
		ServerIP:    serverIP,
		TokenURL:    cloudflareTokenURL,
		TokenSteps:  buildCloudflareTokenSteps(),
		DNSRecords:  records,
		Summary:     summary,
		RenewalNote: renewalNote(),
	}
}

func buildCloudflareTokenSteps() []SetupStep {
	return []SetupStep{
		{
			Title:       "Open Cloudflare API tokens",
			Description: "Go to your Cloudflare profile → API Tokens → Create Token.",
		},
		{
			Title:       "Use the Edit zone DNS template",
			Description: "Choose “Edit zone DNS” or create a custom token with Zone → DNS → Edit for your zone.",
		},
		{
			Title:       "Restrict to your zone",
			Description: "Under Zone Resources, select Include → Specific zone → your domain. This limits the token to DNS changes only.",
		},
		{
			Title:       "Copy the token",
			Description: "Create the token and copy it immediately — Cloudflare only shows it once. Paste it below to provision SSL.",
		},
	}
}

func renewalNote() string {
	return "The Cloudflare API token is written to /etc/caddy/cloudflare.env (mode 600) on the server " +
		"so Caddy can renew the wildcard certificate automatically. It is not stored in the dashboard database. " +
		"Rotate the token in Cloudflare and re-run provisioning if you revoke it."
}
