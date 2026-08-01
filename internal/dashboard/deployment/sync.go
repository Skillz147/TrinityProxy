package deployment

import (
	"net/url"
	"strings"
)

// SyncOptions controls how external deployment metadata is merged into dashboard.db.
type SyncOptions struct {
	Sources               ExternalSources
	OverrideDomain        string
	OverrideSSLMode       string
	OverrideControllerURL string
	// Force applies overrides even when dashboard already has a public domain.
	Force bool
}

// SyncResult describes what changed during a sync.
type SyncResult struct {
	Updated       bool
	PublicDomain  string
	ControllerURL string
	SSLMode       string
}

// SyncFromExternal fills empty dashboard deployment fields from Caddy/controller.env.
// Explicit overrides (or Force) update the stored domain and derived URLs.
func (s *Store) SyncFromExternal(opts SyncOptions) (*SyncResult, error) {
	current, err := s.Get()
	if err != nil {
		return nil, err
	}

	external := DiscoverExternal(opts.Sources)

	domain := NormalizeDomain(current.PublicDomain)
	if opts.OverrideDomain != "" {
		domain = NormalizeDomain(opts.OverrideDomain)
	} else if domain == "" {
		domain = external.PublicDomain
	}

	if domain == "" {
		return syncResultFromSettings(current, false), nil
	}

	sslMode := strings.TrimSpace(current.SSLMode)
	if sslMode == "" {
		sslMode = SSLModeNone
	}
	if opts.OverrideSSLMode != "" {
		sslMode = strings.TrimSpace(opts.OverrideSSLMode)
	} else if sslMode == SSLModeNone && external.PublicDomain == domain && external.SSLMode == SSLModeCaddy {
		sslMode = SSLModeCaddy
	}

	controllerURL := strings.TrimSpace(current.ControllerPublicURL)
	if opts.OverrideControllerURL != "" {
		controllerURL = opts.OverrideControllerURL
	} else if controllerURL == "" || (opts.Force && opts.OverrideDomain != "") {
		switch sslMode {
		case SSLModeCaddy:
			controllerURL = DeriveControllerURL(domain, SSLModeCaddy)
		case SSLModeNone:
			if external.ControllerURL != "" {
				controllerURL = external.ControllerURL
			} else {
				controllerURL = DeriveControllerURL(domain, SSLModeNone)
			}
		default:
			controllerURL = DeriveControllerURL(domain, sslMode)
		}
	}

	if !opts.Force && opts.OverrideDomain == "" && current.PublicDomain != "" {
		if normalizeView(current) == normalizeTriplet(domain, controllerURL, sslMode) {
			return syncResultFromSettings(current, false), nil
		}
		if controllerURL == "" {
			return syncResultFromSettings(current, false), nil
		}
	}

	if normalizeView(current) == normalizeTriplet(domain, controllerURL, sslMode) {
		return syncResultFromSettings(current, false), nil
	}

	view, err := s.Update(domain, controllerURL, sslMode)
	if err != nil {
		return nil, err
	}

	return &SyncResult{
		Updated:       true,
		PublicDomain:  view.PublicDomain,
		ControllerURL: view.ControllerPublicURL,
		SSLMode:       view.SSLMode,
	}, nil
}

func syncResultFromSettings(settings *Settings, updated bool) *SyncResult {
	view := settings.toPublicView()
	return &SyncResult{
		Updated:       updated,
		PublicDomain:  view.PublicDomain,
		ControllerURL: view.ControllerPublicURL,
		SSLMode:       view.SSLMode,
	}
}

func normalizeView(settings *Settings) string {
	view := settings.toPublicView()
	return normalizeTriplet(view.PublicDomain, view.ControllerPublicURL, view.SSLMode)
}

func normalizeTriplet(domain, controllerURL, sslMode string) string {
	domain = NormalizeDomain(domain)
	sslMode = strings.TrimSpace(sslMode)
	if sslMode == "" {
		sslMode = SSLModeNone
	}
	url := NormalizeControllerURL(controllerURL, sslMode)
	if url == "" && domain != "" {
		url = DeriveControllerURL(domain, sslMode)
	}
	return domain + "|" + url + "|" + sslMode
}

func hostFromControllerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return u.Hostname()
}
