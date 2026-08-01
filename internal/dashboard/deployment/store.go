package deployment

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	SSLModeCaddy       = "caddy"
	SSLModeDevMkcert   = "dev-mkcert"
	SSLModeNone        = "none"
)

var (
	ErrInvalidSSLMode = errors.New("invalid ssl_mode")
	validSSLModes     = map[string]struct{}{
		SSLModeCaddy:       {},
		SSLModeDevMkcert:   {},
		SSLModeNone:        {},
	}
)

// Settings holds per-deployment configuration persisted in the dashboard DB.
type Settings struct {
	PublicDomain        string `json:"public_domain"`
	ControllerPublicURL string `json:"controller_public_url"`
	SSLMode             string `json:"ssl_mode"`
	AgentKey            string `json:"-"`
}

// PublicView is returned by API handlers (agent key is never exposed).
type PublicView struct {
	PublicDomain        string `json:"public_domain"`
	ControllerPublicURL string `json:"controller_public_url"`
	SSLMode             string `json:"ssl_mode"`
	HasAgentKey         bool   `json:"has_agent_key"`
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS dashboard_deployment (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			public_domain TEXT NOT NULL DEFAULT '',
			controller_public_url TEXT NOT NULL DEFAULT '',
			ssl_mode TEXT NOT NULL DEFAULT 'none',
			agent_key TEXT NOT NULL DEFAULT ''
		);
		INSERT OR IGNORE INTO dashboard_deployment (id) VALUES (1);
	`)
	return err
}

func (s *Store) Get() (*Settings, error) {
	var settings Settings
	err := s.db.QueryRow(`
		SELECT public_domain, controller_public_url, ssl_mode, agent_key
		FROM dashboard_deployment WHERE id = 1
	`).Scan(
		&settings.PublicDomain,
		&settings.ControllerPublicURL,
		&settings.SSLMode,
		&settings.AgentKey,
	)
	if err != nil {
		return nil, err
	}
	if settings.SSLMode == "" {
		settings.SSLMode = SSLModeNone
	}
	return &settings, nil
}

func (s *Store) PublicView() (*PublicView, error) {
	settings, err := s.Get()
	if err != nil {
		return nil, err
	}
	return settings.toPublicView(), nil
}

func (s *Store) Update(publicDomain, controllerURL, sslMode string) (*PublicView, error) {
	sslMode = strings.TrimSpace(sslMode)
	if sslMode == "" {
		sslMode = SSLModeNone
	}
	if _, ok := validSSLModes[sslMode]; !ok {
		return nil, ErrInvalidSSLMode
	}

	publicDomain = NormalizeDomain(publicDomain)
	if err := ValidateDomain(publicDomain); err != nil {
		return nil, err
	}

	controllerURL = NormalizeControllerURL(controllerURL, sslMode)
	if controllerURL == "" && publicDomain != "" {
		controllerURL = DeriveControllerURL(publicDomain, sslMode)
	}

	_, err := s.db.Exec(`
		UPDATE dashboard_deployment
		SET public_domain = ?, controller_public_url = ?, ssl_mode = ?
		WHERE id = 1
	`, publicDomain, controllerURL, sslMode)
	if err != nil {
		return nil, err
	}

	return s.PublicView()
}

func (s *Store) EnsureAgentKey(fallback string) (string, error) {
	settings, err := s.Get()
	if err != nil {
		return "", err
	}
	if settings.AgentKey != "" {
		return settings.AgentKey, nil
	}

	key := strings.TrimSpace(fallback)
	if key == "" {
		key, err = generateAgentKey()
		if err != nil {
			return "", err
		}
	}

	_, err = s.db.Exec(`UPDATE dashboard_deployment SET agent_key = ? WHERE id = 1`, key)
	if err != nil {
		return "", err
	}
	return key, nil
}

func (s *Store) RegenerateAgentKey() (string, error) {
	key, err := generateAgentKey()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`UPDATE dashboard_deployment SET agent_key = ? WHERE id = 1`, key)
	if err != nil {
		return "", err
	}
	return key, nil
}

func (s *Store) EffectiveControllerURL(envFallback string) (string, error) {
	settings, err := s.Get()
	if err != nil {
		return "", err
	}
	if url := NormalizeControllerURL(settings.ControllerPublicURL, settings.SSLMode); url != "" {
		return url, nil
	}
	if domain := NormalizeDomain(settings.PublicDomain); domain != "" {
		return DeriveControllerURL(domain, settings.SSLMode), nil
	}
	return strings.TrimRight(strings.TrimSpace(envFallback), "/"), nil
}

func DeriveControllerURL(domain, sslMode string) string {
	domain = NormalizeDomain(domain)
	if domain == "" {
		return ""
	}
	apiHost := APIHost(domain)
	switch sslMode {
	case SSLModeNone:
		return fmt.Sprintf("http://%s:3100", apiHost)
	case SSLModeDevMkcert:
		return fmt.Sprintf("https://%s", apiHost)
	default:
		return fmt.Sprintf("https://%s", apiHost)
	}
}

func (settings *Settings) toPublicView() *PublicView {
	controllerURL := NormalizeControllerURL(settings.ControllerPublicURL, settings.SSLMode)
	if controllerURL == "" && settings.PublicDomain != "" {
		controllerURL = DeriveControllerURL(settings.PublicDomain, settings.SSLMode)
	}
	return &PublicView{
		PublicDomain:        settings.PublicDomain,
		ControllerPublicURL: controllerURL,
		SSLMode:             settings.SSLMode,
		HasAgentKey:         settings.AgentKey != "",
	}
}

func generateAgentKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate agent key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
