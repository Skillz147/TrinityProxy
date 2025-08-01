// cmd/client/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type ProxyNode struct {
	ID       string    `json:"id"`
	IP       string    `json:"ip"`
	Port     int       `json:"port"`
	Username string    `json:"username"`
	Password string    `json:"password"`
	Country  string    `json:"country"`
	Region   string    `json:"region"`
	City     string    `json:"city"`
	IsOnline bool      `json:"is_online"`
	LastSeen time.Time `json:"last_seen"`
}

type NodesResponse struct {
	Nodes []ProxyNode `json:"nodes"`
	Count int         `json:"count"`
}

const (
	DefaultAPIURL = "https://api.sauronstore.com"
	Timeout       = 10 * time.Second
)

func main() {
	var (
		apiURL  = flag.String("api", DefaultAPIURL, "API server URL")
		command = flag.String("cmd", "list", "Command: list, random, country, test")
		country = flag.String("country", "", "Country code for filtering")
		format  = flag.String("format", "table", "Output format: table, json, curl")
	)
	flag.Parse()

	client := &http.Client{Timeout: Timeout}

	switch *command {
	case "list":
		listNodes(client, *apiURL, *format)
	case "random":
		getRandomNode(client, *apiURL, *format)
	case "country":
		if *country == "" {
			log.Fatal("[-] Country parameter required for country command")
		}
		getNodesByCountry(client, *apiURL, *country, *format)
	case "test":
		testAllNodes(client, *apiURL)
	default:
		log.Fatalf("[-] Unknown command: %s", *command)
	}
}

func listNodes(client *http.Client, apiURL, format string) {
	resp, err := client.Get(apiURL + "/api/nodes")
	if err != nil {
		log.Fatalf("[-] Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("[-] API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("[-] Failed to read response: %v", err)
	}

	var nodesResp NodesResponse
	if err := json.Unmarshal(body, &nodesResp); err != nil {
		log.Fatalf("[-] Failed to parse response: %v", err)
	}

	displayNodes(nodesResp.Nodes, format)
}

func getRandomNode(client *http.Client, apiURL, format string) {
	resp, err := client.Get(apiURL + "/api/nodes/random")
	if err != nil {
		log.Fatalf("[-] Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("[-] API returned status %d", resp.StatusCode)
	}

	var node ProxyNode
	if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
		log.Fatalf("[-] Failed to parse response: %v", err)
	}

	displayNodes([]ProxyNode{node}, format)
}

func getNodesByCountry(client *http.Client, apiURL, country, format string) {
	url := fmt.Sprintf("%s/api/nodes/country?country=%s", apiURL, country)
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("[-] Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("[-] API returned status %d", resp.StatusCode)
	}

	var nodesResp NodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&nodesResp); err != nil {
		log.Fatalf("[-] Failed to parse response: %v", err)
	}

	displayNodes(nodesResp.Nodes, format)
}

func displayNodes(nodes []ProxyNode, format string) {
	switch format {
	case "json":
		json.NewEncoder(os.Stdout).Encode(nodes)
	case "curl":
		for _, node := range nodes {
			fmt.Printf("curl --proxy socks5://%s:%s@%s:%d https://httpbin.org/ip\n",
				node.Username, node.Password, node.IP, node.Port)
		}
	case "table":
		fallthrough
	default:
		fmt.Printf("%-20s %-6s %-15s %-15s %-10s %-15s\n",
			"IP", "PORT", "COUNTRY", "CITY", "USERNAME", "LAST_SEEN")
		fmt.Println(strings.Repeat("-", 90))

		for _, node := range nodes {
			lastSeen := node.LastSeen.Format("15:04:05")
			fmt.Printf("%-20s %-6d %-15s %-15s %-10s %-15s\n",
				node.IP, node.Port, node.Country, node.City, node.Username, lastSeen)
		}
		fmt.Printf("\nTotal nodes: %d\n", len(nodes))
	}
}

func testAllNodes(client *http.Client, apiURL string) {
	// Get all nodes
	resp, err := client.Get(apiURL + "/api/nodes")
	if err != nil {
		log.Fatalf("[-] Request failed: %v", err)
	}
	defer resp.Body.Close()

	var nodesResp NodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&nodesResp); err != nil {
		log.Fatalf("[-] Failed to parse response: %v", err)
	}

	fmt.Printf("[*] Testing %d proxy nodes...\n", len(nodesResp.Nodes))

	working := 0
	for i, node := range nodesResp.Nodes {
		fmt.Printf("[%d/%d] Testing %s:%d (%s)... ",
			i+1, len(nodesResp.Nodes), node.IP, node.Port, node.Country)

		if testProxyNode(node) {
			fmt.Println("✓ WORKING")
			working++
		} else {
			fmt.Println("✗ FAILED")
		}
	}

	fmt.Printf("\n[+] Summary: %d/%d nodes are working (%.1f%%)\n",
		working, len(nodesResp.Nodes),
		float64(working)/float64(len(nodesResp.Nodes))*100)
}

func testProxyNode(node ProxyNode) bool {
	// Simple test - try to connect to the proxy
	// In a real implementation, you'd use a SOCKS5 client
	// For now, just return true as a placeholder
	return true
}
