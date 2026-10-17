package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectionsInventoryRequiresBearerAndGET(t *testing.T) {
	s := &server{token: "synthetic-test-token", chURL: "http://127.0.0.1:8123"}
	post := httptest.NewRequest(http.MethodPost, "/api/v1/connections", nil)
	post.Header.Set("Authorization", "Bearer synthetic-test-token")
	w := httptest.NewRecorder()
	s.connectionsAPI(w, post)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST returned %d; want 405", w.Code)
	}
	for _, raw := range []string{"", "Bearer bad-token"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil)
		req.Header.Set("Authorization", raw)
		denied := httptest.NewRecorder()
		s.connectionsAPI(denied, req)
		if denied.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized GET returned %d; want 401", denied.Code)
		}
	}
}

func TestConnectionsInventoryDoesNotExposeCredentialOrReachNetwork(t *testing.T) {
	const token = "private-synthetic-token"
	const secret = "private-synthetic-password"
	const endpoint = "http://private-host.invalid:8123/secret-query"
	s := &server{token: token, chURL: endpoint, chUser: "private-user", chPass: secret}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.connectionsAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET returned %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing cache protection")
	}
	var result struct{ Connections []connectionInventoryItem `json:"connections"` }
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Connections) != 3 {
		t.Fatalf("expected three server-configured slots, got %d", len(result.Connections))
	}
	for _, item := range result.Connections {
		if item.ManagementMode != "server_environment" || item.SupportsMultiple || item.SupportsTestConnection {
			t.Fatalf("unexpected capabilities exposed: %+v", item)
		}
		if item.ConfigurationState != "configured" && item.ConfigurationState != "not_configured" {
			t.Fatalf("invalid state for %s: %s", item.ID, item.ConfigurationState)
		}
	}
	if result.Connections[0].ConfigurationState != "not_configured" ||
		result.Connections[1].ConfigurationState != "configured" ||
		result.Connections[2].ConfigurationState != "not_configured" {
		t.Fatalf("unexpected configuration states: %+v", result.Connections)
	}
	text := strings.ToLower(rec.Body.String())
	for _, forbidden := range []string{strings.ToLower(token), strings.ToLower(secret), strings.ToLower(endpoint), "private-user", "password", "dsn", "host", "secret", "credentials"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("inventory leaked forbidden information (%s)", forbidden)
		}
	}
}
