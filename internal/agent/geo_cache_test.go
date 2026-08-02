package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGeoCacheReusesLookup(t *testing.T) {
	resetGeoCacheForTests()
	geoCacheDisableFileForTests()
	geoCacheSetTTLForTests(time.Hour)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"country_name":"United States","country_code":"US","city":"Austin"}`))
	}))
	t.Cleanup(server.Close)

	geoHTTPClient = server.Client()
	oldServices := geoServicesForIP
	geoServicesForIP = func(ip string) []geoService {
		return []geoService{{"test", server.URL}}
	}
	t.Cleanup(func() {
		geoServicesForIP = oldServices
		geoHTTPClient = http.DefaultClient
	})

	first, err := getCachedGeoInfo("203.0.113.10")
	if err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	second, err := getCachedGeoInfo("203.0.113.10")
	if err != nil {
		t.Fatalf("second lookup: %v", err)
	}
	if first["city"] != "Austin" || second["city"] != "Austin" {
		t.Fatalf("unexpected geo data: first=%v second=%v", first, second)
	}
	if calls != 1 {
		t.Fatalf("expected 1 geo HTTP call, got %d", calls)
	}
}

func TestGeoCacheExpiresByTTL(t *testing.T) {
	resetGeoCacheForTests()
	geoCacheDisableFileForTests()
	geoCacheSetTTLForTests(20 * time.Millisecond)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"country_name":"Canada","country_code":"CA"}`))
	}))
	t.Cleanup(server.Close)

	geoHTTPClient = server.Client()
	oldServices := geoServicesForIP
	geoServicesForIP = func(ip string) []geoService {
		return []geoService{{"test", server.URL}}
	}
	t.Cleanup(func() {
		geoServicesForIP = oldServices
		geoHTTPClient = http.DefaultClient
	})

	if _, err := getCachedGeoInfo("203.0.113.11"); err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := getCachedGeoInfo("203.0.113.11"); err != nil {
		t.Fatalf("second lookup: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 geo HTTP calls after TTL expiry, got %d", calls)
	}
}

func TestGeoCacheUsesSeededEntry(t *testing.T) {
	resetGeoCacheForTests()
	geoCacheDisableFileForTests()
	geoCacheSetTTLForTests(time.Hour)
	geoCacheSeedForTests("203.0.113.12", map[string]string{
		"country_name": "Germany",
		"country_code": "DE",
	}, time.Now())

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"country_name":"Should Not Call"}`))
	}))
	t.Cleanup(server.Close)

	geoHTTPClient = server.Client()
	oldServices := geoServicesForIP
	geoServicesForIP = func(ip string) []geoService {
		return []geoService{{"test", server.URL}}
	}
	t.Cleanup(func() {
		geoServicesForIP = oldServices
		geoHTTPClient = http.DefaultClient
	})

	data, err := getCachedGeoInfo("203.0.113.12")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if data["country_code"] != "DE" {
		t.Fatalf("country_code = %q, want DE", data["country_code"])
	}
	if calls != 0 {
		t.Fatalf("expected 0 geo HTTP calls for seeded cache, got %d", calls)
	}
}
