package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHealthAndReadinessRoutes(t *testing.T) {
	server := New(Config{
		Address: ":0",
		Ready: func(context.Context) error {
			return nil
		},
	})

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := server.App().Test(httptestRequest("GET", path))
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("GET %s status = %d", path, resp.StatusCode)
		}
	}
}

func TestCORSPrefightAllowsConfiguredOrigin(t *testing.T) {
	server := New(Config{
		Address:            ":0",
		CORSAllowedOrigins: []string{"http://localhost:5173"},
	})

	req := httptestRequest("OPTIONS", "/api/v1")
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")

	resp, err := server.App().Test(req)
	if err != nil {
		t.Fatalf("OPTIONS preflight error = %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("OPTIONS preflight status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}
