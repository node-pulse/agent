package prometheus

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScraper_Success(t *testing.T) {
	// Mock Prometheus exporter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		response := `# HELP test_metric A test metric
# TYPE test_metric counter
test_metric 42
# HELP another_metric Another test metric
# TYPE another_metric gauge
another_metric{label="value"} 123.45
`
		w.Write([]byte(response))
	}))
	defer server.Close()

	scraper := NewScraper(&ScraperConfig{
		Endpoint: server.URL,
		Timeout:  3 * time.Second,
	})

	data, err := scraper.Scrape()
	if err != nil {
		t.Fatalf("Scrape failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Expected non-empty data")
	}

	dataStr := string(data)
	if !strings.Contains(dataStr, "test_metric 42") {
		t.Errorf("Expected 'test_metric 42' in scraped data, got: %s", dataStr)
	}

	if !strings.Contains(dataStr, "another_metric{label=\"value\"} 123.45") {
		t.Errorf("Expected 'another_metric{label=\"value\"} 123.45' in scraped data, got: %s", dataStr)
	}
}

func TestScraper_ServerDown(t *testing.T) {
	scraper := NewScraper(&ScraperConfig{
		Endpoint: "http://localhost:19999/metrics", // Non-existent server
		Timeout:  1 * time.Second,
	})

	_, err := scraper.Scrape()
	if err == nil {
		t.Fatal("Expected error when scraping non-existent server")
	}

	if !strings.Contains(err.Error(), "failed to scrape") {
		t.Errorf("Expected 'failed to scrape' error, got: %v", err)
	}
}

func TestScraper_NonOKStatus(t *testing.T) {
	// Mock server returning 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	scraper := NewScraper(&ScraperConfig{
		Endpoint: server.URL,
		Timeout:  3 * time.Second,
	})

	_, err := scraper.Scrape()
	if err == nil {
		t.Fatal("Expected error when server returns 500")
	}

	if !strings.Contains(err.Error(), "scrape returned status 500") {
		t.Errorf("Expected status code error, got: %v", err)
	}
}

func TestScraper_Verify(t *testing.T) {
	// Mock Prometheus exporter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte("test_metric 1\n"))
	}))
	defer server.Close()

	scraper := NewScraper(&ScraperConfig{
		Endpoint: server.URL,
		Timeout:  3 * time.Second,
	})

	if err := scraper.Verify(); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
}

func TestScraper_VerifyFails(t *testing.T) {
	scraper := NewScraper(&ScraperConfig{
		Endpoint: "http://localhost:19999/metrics", // Non-existent
		Timeout:  1 * time.Second,
	})

	err := scraper.Verify()
	if err == nil {
		t.Fatal("Expected verify to fail for non-existent server")
	}

	if !strings.Contains(err.Error(), "prometheus exporter verification failed") {
		t.Errorf("Expected verification error, got: %v", err)
	}
}
