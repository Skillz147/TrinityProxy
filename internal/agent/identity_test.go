package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	geopkg "github.com/Skillz147/TrinityProxy/internal/geo"
	"github.com/Skillz147/TrinityProxy/internal/proxy"
)

func TestPlatformFromGOOS(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{"linux", "linux"},
		{"windows", "windows"},
		{"darwin", "darwin"},
		{"android", "unknown"},
		{"ios", "unknown"},
		{"freebsd", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			if got := platformFromGOOS(tc.goos); got != tc.want {
				t.Fatalf("platformFromGOOS(%q) = %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	got := detectPlatform()
	want := platformFromGOOS(runtime.GOOS)
	if got != want {
		t.Fatalf("detectPlatform() = %q, want %q", got, want)
	}
}

func TestDetectDeviceClassEnvOverride(t *testing.T) {
	t.Setenv("TRINITY_DEVICE_CLASS", "vps")
	if got := detectDeviceClass(); got != "vps" {
		t.Fatalf("detectDeviceClass() = %q, want vps", got)
	}

	t.Setenv("TRINITY_DEVICE_CLASS", "DESKTOP")
	if got := detectDeviceClass(); got != "desktop" {
		t.Fatalf("detectDeviceClass() = %q, want desktop", got)
	}

	t.Setenv("TRINITY_DEVICE_CLASS", "invalid")
	switch runtime.GOOS {
	case "darwin", "windows":
		if got := detectDeviceClass(); got != "desktop" {
			t.Fatalf("invalid override on %s: got %q, want desktop", runtime.GOOS, got)
		}
	case "linux":
		want := "unknown"
		if inContainer() {
			want = "vps"
		}
		if got := detectDeviceClass(); got != want {
			t.Fatalf("invalid override on linux: got %q, want %q", got, want)
		}
	}
}

func TestDetectDeviceClassHeuristics(t *testing.T) {
	t.Setenv("TRINITY_DEVICE_CLASS", "")

	switch runtime.GOOS {
	case "darwin", "windows":
		if got := detectDeviceClass(); got != "desktop" {
			t.Fatalf("detectDeviceClass() = %q, want desktop on %s", got, runtime.GOOS)
		}
	case "linux":
		want := "unknown"
		if inContainer() {
			want = "vps"
		}
		if got := detectDeviceClass(); got != want {
			t.Fatalf("detectDeviceClass() = %q, want %q on linux", got, want)
		}
	}
}

func TestInContainerDockerenv(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("container detection test requires linux")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Skip("not running in docker")
	}
	if !inContainer() {
		t.Fatal("expected inContainer() true when /.dockerenv exists")
	}
}

func TestDetectNetworkTypeEnvOverride(t *testing.T) {
	t.Setenv("TRINITY_NETWORK_TYPE", "datacenter")
	if got := detectNetworkType(); got != "datacenter" {
		t.Fatalf("detectNetworkType() = %q, want datacenter", got)
	}

	t.Setenv("TRINITY_NETWORK_TYPE", "")
	if got := detectNetworkType(); got != "unknown" {
		t.Fatalf("detectNetworkType() = %q, want unknown", got)
	}

	t.Setenv("TRINITY_NETWORK_TYPE", "bogus")
	if got := detectNetworkType(); got != "unknown" {
		t.Fatalf("detectNetworkType() = %q, want unknown for invalid value", got)
	}
}

func TestGetGeoFieldCountryNormalization(t *testing.T) {
	tests := []struct {
		name     string
		geo      map[string]string
		expected string
	}{
		{
			name:     "ipapi country_name",
			geo:      map[string]string{"country_name": "United States", "country_code": "US"},
			expected: "United States",
		},
		{
			name:     "ip-api country",
			geo:      map[string]string{"country": "United States", "countryCode": "US"},
			expected: "United States",
		},
		{
			name:     "ipinfo country_code",
			geo:      map[string]string{"country": "US"},
			expected: "US",
		},
		{
			name:     "countryName fallback",
			geo:      map[string]string{"countryName": "Canada"},
			expected: "Canada",
		},
		{
			name:     "missing country",
			geo:      map[string]string{"city": "Berlin"},
			expected: "Unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getGeoField(tc.geo, "country_name", "country", "country_code")
			if got != tc.expected {
				t.Fatalf("getGeoField country = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestGetGeoFieldRegionAndPostalNormalization(t *testing.T) {
	regionGeo := map[string]string{"regionName": "California", "state": "CA"}
	if got := getGeoField(regionGeo, "region", "region_code", ""); got != "California" {
		t.Fatalf("region = %q, want California", got)
	}

	postalGeo := map[string]string{"postal_code": "90001"}
	if got := getGeoField(postalGeo, "postal", "zip", ""); got != "90001" {
		t.Fatalf("postal = %q, want 90001", got)
	}
}

func TestFetchGeoInfoFallbackChain(t *testing.T) {
	ipapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer ipapi.Close()

	ipAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"country":"United States",
			"countryCode":"US",
			"region":"CA",
			"regionName":"California",
			"city":"Los Angeles",
			"zip":"90001"
		}`))
	}))
	defer ipAPI.Close()

	services := []geoService{
		{"ipapi.co", ipapi.URL},
		{"ip-api.com", ipAPI.URL},
	}

	result, err := fetchGeoInfo(services, http.DefaultClient)
	if err != nil {
		t.Fatalf("fetchGeoInfo: %v", err)
	}
	if result["country"] != "United States" {
		t.Fatalf("country = %q, want United States", result["country"])
	}
	if result["city"] != "Los Angeles" {
		t.Fatalf("city = %q, want Los Angeles", result["city"])
	}
}

func TestFetchGeoInfoIPAPICoFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"country_name":"United States",
			"country_code":"US",
			"region":"California",
			"city":"San Francisco",
			"postal":"94105"
		}`))
	}))
	defer server.Close()

	result, err := fetchGeoInfo([]geoService{{"ipapi.co", server.URL}}, http.DefaultClient)
	if err != nil {
		t.Fatalf("fetchGeoInfo: %v", err)
	}
	if result["country_name"] != "United States" {
		t.Fatalf("country_name = %q", result["country_name"])
	}
	if result["country_code"] != "US" {
		t.Fatalf("country_code = %q", result["country_code"])
	}
}

func TestFetchGeoInfoIPInfoFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"country":"US",
			"region":"Texas",
			"city":"Austin",
			"postal":"78701"
		}`))
	}))
	defer server.Close()

	result, err := fetchGeoInfo([]geoService{{"ipinfo.io", server.URL}}, http.DefaultClient)
	if err != nil {
		t.Fatalf("fetchGeoInfo: %v", err)
	}
	if result["country"] != "US" {
		t.Fatalf("country = %q, want US", result["country"])
	}
}

func TestFetchGeoInfoServiceErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error": true, "reason": "RateLimited"}`))
	}))
	defer server.Close()

	_, err := fetchGeoInfo([]geoService{{"ip-api.com", server.URL}}, http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for service error response")
	}
	if !strings.Contains(err.Error(), "RateLimited") {
		t.Fatalf("expected RateLimited in error, got: %v", err)
	}
}

func TestFetchGeoInfoAllServicesFail(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer down.Close()

	noCountry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"city":"Nowhere"}`))
	}))
	defer noCountry.Close()

	services := []geoService{
		{"ipapi.co", down.URL},
		{"ip-api.com", noCountry.URL},
	}

	_, err := fetchGeoInfo(services, http.DefaultClient)
	if err == nil {
		t.Fatal("expected error when all geo services fail")
	}
	if !strings.Contains(err.Error(), "all geo services failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchGeoInfoInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	_, err := fetchGeoInfo([]geoService{{"ipapi.co", server.URL}}, http.DefaultClient)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeoFailureProducesUnknownLocationFields(t *testing.T) {
	geoData := map[string]string{}

	country := geopkg.NormalizeCountry(getGeoField(geoData, "country_name", "country", "country_code"))
	if country != "Unknown" {
		t.Fatalf("country = %q, want Unknown", country)
	}
	if getGeoField(geoData, "region", "region_code", "") != "Unknown" {
		t.Fatal("expected Unknown region on empty geo data")
	}
	if getGeoField(geoData, "city", "", "") != "Unknown" {
		t.Fatal("expected Unknown city on empty geo data")
	}
	if getGeoField(geoData, "postal", "zip", "") != "Unknown" {
		t.Fatal("expected Unknown zip on empty geo data")
	}
}

func TestEmbeddedProxyCredentialsFromActiveServer(t *testing.T) {
	t.Setenv("TRINITY_DATA_DIR", t.TempDir())
	t.Setenv("TRINITY_SKIP_INSTALLER", "1")
	t.Setenv("TRINITY_SOCKS_PORT", "0")
	t.Setenv("TRINITY_SOCKS_USER", "probe")
	t.Setenv("TRINITY_SOCKS_PASSWORD", "check")

	srv, err := proxy.StartEmbedded()
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}

	port, user, pass := embeddedProxyCredentials()
	if port != srv.Port {
		t.Fatalf("port = %d, want %d", port, srv.Port)
	}
	if user != "probe" || pass != "check" {
		t.Fatalf("credentials = %q/%q, want probe/check", user, pass)
	}
}

func TestEmbeddedSOCKSMode(t *testing.T) {
	t.Setenv("TRINITY_SKIP_INSTALLER", "1")
	if !embeddedSOCKSMode() {
		t.Fatal("expected embedded SOCKS mode with TRINITY_SKIP_INSTALLER=1")
	}
}

func TestNormalizeCountryFromGeoResponse(t *testing.T) {
	geoData := map[string]string{"country_name": "United States", "country_code": "US"}
	raw := getGeoField(geoData, "country_name", "country", "country_code")
	if got := geopkg.NormalizeCountry(raw); got != "US" {
		t.Fatalf("NormalizeCountry = %q, want US", got)
	}
}
