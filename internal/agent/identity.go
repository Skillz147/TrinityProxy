//internal/agent/identity.go

package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type NodeMetadata struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	City     string `json:"city"`
	Zip      string `json:"zip"`
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

// getGeoInfo gets location data for an IP with multiple fallback services
func getGeoInfo(ip string) (map[string]string, error) {
	// Try multiple geo services as fallbacks
	geoServices := []struct {
		name string
		url  string
	}{
		{"ipapi.co", "https://ipapi.co/" + ip + "/json/"},
		{"ip-api.com", "http://ip-api.com/json/" + ip},
		{"ipinfo.io", "https://ipinfo.io/" + ip + "/json"},
	}

	var lastError error
	for _, service := range geoServices {
		fmt.Printf("[*] Trying geo service: %s\n", service.name)

		resp, err := http.Get(service.url)
		if err != nil {
			lastError = fmt.Errorf("%s failed: %v", service.name, err)
			continue
		}
		defer resp.Body.Close()

		var rawResult map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&rawResult); err != nil {
			lastError = fmt.Errorf("%s decode error: %v", service.name, err)
			continue
		}

		// Check for rate limiting or errors in response
		if errorVal, exists := rawResult["error"]; exists {
			if errorBool, ok := errorVal.(bool); ok && errorBool {
				if reason, exists := rawResult["reason"]; exists {
					lastError = fmt.Errorf("%s error: %v", service.name, reason)
					continue
				}
			}
		}

		// Convert all values to strings safely
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

		// Check if we got valid geo data (multiple fallbacks)
		if result["country_name"] != "" || result["country"] != "" || result["country_code"] != "" {
			fmt.Printf("[+] Geo data retrieved from %s\n", service.name)
			return result, nil
		}

		lastError = fmt.Errorf("%s returned no country data", service.name)
	}

	return nil, fmt.Errorf("all geo services failed, last error: %v", lastError)
}

// GatherMetadata builds the full metadata package
func GatherMetadata() (*NodeMetadata, error) {
	ip, err := getPublicIP()
	if err != nil {
		return nil, err
	}

	geo, err := getGeoInfo(ip)
	if err != nil {
		return nil, err
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
		IP:       ip,
		Port:     port,
		Username: username,
		Password: password,
		Country:  getGeoField(geo, "country_name", "country", "country_code"),
		Region:   getGeoField(geo, "region", "region_code", ""),
		City:     getGeoField(geo, "city", "", ""),
		Zip:      getGeoField(geo, "postal", "zip", ""),
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
