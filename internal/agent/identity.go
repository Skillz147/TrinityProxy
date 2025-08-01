//internal/agent/identity.go

package agent

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
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
	data, err := ioutil.ReadFile(path)
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
	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ip)), nil
}

// getGeoInfo gets location data for an IP
func getGeoInfo(ip string) (map[string]string, error) {
	resp, err := http.Get("https://ipapi.co/" + ip + "/json/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result["country"] == "" {
		return nil, errors.New("geo lookup failed")
	}

	return result, nil
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
		Country:  geo["country_name"],
		Region:   geo["region"],
		City:     geo["city"],
		Zip:      geo["postal"],
	}, nil
}
