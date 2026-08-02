//internal/agent/identity.go

package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	geopkg "github.com/Skillz147/TrinityProxy/internal/geo"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
	"github.com/Skillz147/TrinityProxy/internal/proxy"
)

type NodeMetadata struct {
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Country     string `json:"country"`
	Region      string `json:"region"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	Platform    string `json:"platform"`
	DeviceClass string `json:"device_class"`
	NetworkType string `json:"network_type"`
}

// detectPlatform maps runtime.GOOS to a supported platform label.
// Mobile platforms are intentionally excluded and reported as unknown.
func detectPlatform() string {
	return platformFromGOOS(runtime.GOOS)
}

func platformFromGOOS(goos string) string {
	switch goos {
	case "linux", "windows", "darwin":
		return goos
	default:
		return "unknown"
	}
}

// detectDeviceClass returns device class from TRINITY_DEVICE_CLASS or heuristics.
func detectDeviceClass() string {
	if v := strings.TrimSpace(os.Getenv("TRINITY_DEVICE_CLASS")); v != "" {
		switch strings.ToLower(v) {
		case "vps", "desktop", "unknown":
			return strings.ToLower(v)
		}
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return "desktop"
	case "linux":
		if inContainer() {
			return "vps"
		}
	}
	return "unknown"
}

// detectNetworkType returns network type from TRINITY_NETWORK_TYPE or unknown.
func detectNetworkType() string {
	if v := strings.TrimSpace(os.Getenv("TRINITY_NETWORK_TYPE")); v != "" {
		switch strings.ToLower(v) {
		case "datacenter", "unknown":
			return strings.ToLower(v)
		}
	}
	return "unknown"
}

func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "docker") ||
		strings.Contains(s, "containerd") ||
		strings.Contains(s, "kubepods")
}

// readFile reads and trims content from a file
func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// getPublicIP fetches the VPS's public IP
func getPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org?format=text")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ip)), nil
}

type geoService struct {
	name string
	url  string
}

type httpGetter interface {
	Get(url string) (*http.Response, error)
}

var geoHTTPClient httpGetter = http.DefaultClient

var geoServicesForIP = defaultGeoServices

func defaultGeoServices(ip string) []geoService {
	return []geoService{
		{"ipapi.co", "https://ipapi.co/" + ip + "/json/"},
		{"ip-api.com", "http://ip-api.com/json/" + ip},
		{"ipinfo.io", "https://ipinfo.io/" + ip + "/json"},
	}
}

// getGeoInfo gets location data for an IP with multiple fallback services.
func getGeoInfo(ip string) (map[string]string, error) {
	return fetchGeoInfo(geoServicesForIP(ip), geoHTTPClient)
}

func fetchGeoInfo(services []geoService, client httpGetter) (map[string]string, error) {
	log := logutil.Component("agent")
	var lastError error
	for _, service := range services {
		log.Debug("trying geo service", "service", service.name)

		resp, err := client.Get(service.url)
		if err != nil {
			lastError = fmt.Errorf("%s failed: %v", service.name, err)
			continue
		}

		result, err := decodeGeoResponse(service.name, resp.Body)
		resp.Body.Close()
		if err != nil {
			lastError = err
			continue
		}

		if result["country_name"] != "" || result["country"] != "" || result["country_code"] != "" {
			log.Info("geo data retrieved", "service", service.name)
			return result, nil
		}

		lastError = fmt.Errorf("%s returned no country data", service.name)
	}

	return nil, fmt.Errorf("all geo services failed, last error: %v", lastError)
}

func decodeGeoResponse(serviceName string, body io.Reader) (map[string]string, error) {
	var rawResult map[string]interface{}
	if err := json.NewDecoder(body).Decode(&rawResult); err != nil {
		return nil, fmt.Errorf("%s decode error: %v", serviceName, err)
	}

	if errorVal, exists := rawResult["error"]; exists {
		if errorBool, ok := errorVal.(bool); ok && errorBool {
			if reason, exists := rawResult["reason"]; exists {
				return nil, fmt.Errorf("%s error: %v", serviceName, reason)
			}
		}
	}

	result := make(map[string]string)
	for key, value := range rawResult {
		if value != nil {
			switch v := value.(type) {
			case string:
				result[key] = v
			case bool:
				if v {
					result[key] = "true"
				} else {
					result[key] = "false"
				}
			case float64:
				result[key] = strconv.FormatFloat(v, 'f', -1, 64)
			default:
				result[key] = fmt.Sprintf("%v", v)
			}
		} else {
			result[key] = ""
		}
	}

	return result, nil
}

func embeddedSOCKSMode() bool {
	return proxy.UseEmbedded()
}

func embeddedProxyCredentials() (port int, username, password string) {
	if srv := proxy.Active(); srv != nil && srv.Port > 0 {
		return srv.Port, srv.Username, srv.Password
	}

	cfg := proxy.ConfigFromEnv()
	return cfg.Port, cfg.Username, cfg.Password
}

// GatherMetadata builds the full metadata package
func GatherMetadata() (*NodeMetadata, error) {
	ip, err := getPublicIP()
	if err != nil {
		return nil, err
	}

	geoData, err := getCachedGeoInfo(ip)
	if err != nil {
		logutil.Component("agent").Warn("geo lookup failed, continuing with unknown location", "err", err)
		geoData = map[string]string{}
	}

	rawCountry := getGeoField(geoData, "country_name", "country", "country_code")
	region := getGeoField(geoData, "region", "region_code", "")
	city := getGeoField(geoData, "city", "", "")
	zip := getGeoField(geoData, "postal", "zip", "")

	platform := detectPlatform()
	deviceClass := detectDeviceClass()
	networkType := detectNetworkType()

	if embeddedSOCKSMode() {
		port, username, password := embeddedProxyCredentials()
		return &NodeMetadata{
			IP:          ip,
			Port:        port,
			Username:    username,
			Password:    password,
			Country:     geopkg.NormalizeCountry(rawCountry),
			Region:      region,
			City:        city,
			Zip:         zip,
			Platform:    platform,
			DeviceClass: deviceClass,
			NetworkType: networkType,
		}, nil
	}

	username, err := readFile("/etc/trinityproxy-username")
	if err != nil {
		return nil, err
	}

	password, err := readFile("/etc/trinityproxy-password")
	if err != nil {
		return nil, err
	}

	portStr, err := readFile("/etc/trinityproxy-port")
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}

	return &NodeMetadata{
		IP:          ip,
		Port:        port,
		Username:    username,
		Password:    password,
		Country:     geopkg.NormalizeCountry(rawCountry),
		Region:      region,
		City:        city,
		Zip:         zip,
		Platform:    platform,
		DeviceClass: deviceClass,
		NetworkType: networkType,
	}, nil
}

// getGeoField tries multiple field names as fallbacks for different geo services
func getGeoField(geo map[string]string, primary, secondary, tertiary string) string {
	if val := geo[primary]; val != "" {
		return val
	}
	if secondary != "" {
		if val := geo[secondary]; val != "" {
			return val
		}
	}
	if tertiary != "" {
		if val := geo[tertiary]; val != "" {
			return val
		}
	}

	// Additional fallbacks for different geo service field names
	switch primary {
	case "country_name":
		// Try different country field variations
		if val := geo["countryName"]; val != "" {
			return val
		}
		if val := geo["country"]; val != "" {
			return val
		}
	case "region":
		// Try different region field variations
		if val := geo["regionName"]; val != "" {
			return val
		}
		if val := geo["region_name"]; val != "" {
			return val
		}
		if val := geo["state"]; val != "" {
			return val
		}
	case "city":
		// Try different city field variations
		if val := geo["cityName"]; val != "" {
			return val
		}
	case "postal":
		// Try different postal code field variations
		if val := geo["zip"]; val != "" {
			return val
		}
		if val := geo["zipcode"]; val != "" {
			return val
		}
		if val := geo["postal_code"]; val != "" {
			return val
		}
	}

	return "Unknown"
}
