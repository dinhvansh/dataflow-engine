package main

import (
	"encoding/json"
	"net/http"
)

// connectionInventory deliberately has no host, port, DSN, username, password
// or free-form configuration fields. Backend environment is the only source of
// truth in V3.1; an inventory read never dials a database or mutates state.
type connectionInventoryItem struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	Driver                string `json:"driver"`
	Usage                 string `json:"usage"`
	ConfigurationState    string `json:"configuration_state"`
	ManagementMode        string `json:"management_mode"`
	SupportsMultiple      bool   `json:"supports_multiple_instances"`
	SupportsTestConnection bool  `json:"supports_test_connection"`
}

func (s *server) connectionsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if !s.validBearer(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	state := func(configured bool) string {
		if configured {
			return "configured"
		}
		return "not_configured"
	}
	items := []connectionInventoryItem{
		{
			ID: "env-sqlserver-source", Name: "SQL Server source", Kind: "source",
			Driver: "sqlserver", Usage: "Read-only source discovery and test ingestion",
			ConfigurationState: state(s.sql != nil),
			ManagementMode: "server_environment",
		},
		{
			ID: "env-clickhouse-target", Name: "ClickHouse scratch target", Kind: "destination",
			Driver: "clickhouse", Usage: "Disposable scratch warehouse only",
			ConfigurationState: state(s.chURL != ""),
			ManagementMode: "server_environment",
		},
		{
			ID: "env-postgres-control", Name: "PostgreSQL control plane", Kind: "internal",
			Driver: "postgresql", Usage: "Metadata, queue and run state; not a selectable data source/sink",
			ConfigurationState: state(s.pg != nil),
			ManagementMode: "server_environment",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Connections []connectionInventoryItem `json:"connections"`
	}{Connections: items})
}
