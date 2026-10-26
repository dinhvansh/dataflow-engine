package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type qaDestinationGrants struct {
	Create bool `json:"create"`
	Insert bool `json:"insert"`
	Select bool `json:"select"`
	Drop   bool `json:"drop"`
	Alter  bool `json:"alter"`
}

func (g qaDestinationGrants) usable() bool {
	return g.Create && g.Insert && g.Select && g.Drop && g.Alter
}

// All queries are assembled from fixed SQL and an identifier validated by the
// server allowlist. The ClickHouse response and credentials never reach logs.
func labCHCatalogQuery(ctx context.Context, p runtimeConnection, secret, query string) (string, error) {
	scheme := "https"
	if os.Getenv("DATAFLOW_QA_CLICKHOUSE_ALLOW_HTTP") == "true" {
		scheme = "http"
	}
	endpoint := scheme + "://" + net.JoinHostPort(p.Host, strconv.Itoa(p.Port)) + "/"
	u, e := url.Parse(endpoint)
	if e != nil {
		return "", errUnsafeLabConnection
	}
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if e != nil {
		return "", errUnsafeLabConnection
	}
	req.SetBasicAuth(p.Username, secret)
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return "", errUnsafeLabConnection
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 65537))
	if e != nil || len(raw) > 65536 || res.StatusCode != http.StatusOK {
		return "", errUnsafeLabConnection
	}
	return strings.TrimSpace(string(raw)), nil
}

func labCHGrants(ctx context.Context, p runtimeConnection, secret string) (qaDestinationGrants, error) {
	var g qaDestinationGrants
	if p.Driver != "clickhouse" || !approvedTypedDatabase(p.Database) || !approvedRuntimeEndpoint(p.Driver, p.Host, p.Port) {
		return g, errUnsafeLabConnection
	}
	for _, entry := range []struct {
		name string
		out  *bool
	}{
		{"CREATE TABLE", &g.Create}, {"INSERT", &g.Insert}, {"SELECT", &g.Select}, {"DROP TABLE", &g.Drop}, {"ALTER TABLE", &g.Alter},
	} {
		result, e := labCHCatalogQuery(ctx, p, secret, fmt.Sprintf("CHECK GRANT %s ON `%s`.*", entry.name, p.Database))
		if e != nil {
			return g, e
		}
		if result != "0" && result != "1" {
			return g, errors.New("invalid ClickHouse grant response")
		}
		*entry.out = result == "1"
	}
	return g, nil
}

func (s *server) runtimeDestinationDiscoveryAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", 405)
		return
	}
	if !runtimeLabEnabled() || !runtimeConnectionAuthorized(s, r) {
		http.Error(w, "not permitted", 403)
		return
	}
	id := r.URL.Query().Get("target_connection_id")
	if uuid.Validate(id) != nil {
		http.Error(w, "invalid destination profile", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	p, secret, e := s.readRuntimeProfile(ctx, id)
	if e != nil || !verifiedLabProfile(p, "clickhouse") || p.Kind != "destination" || !approvedTypedDatabase(p.Database) {
		http.Error(w, "destination profile not currently verified", 409)
		return
	}
	configured, e := url.Parse(s.chURL)
	if e != nil || configured.Host != net.JoinHostPort(p.Host, strconv.Itoa(p.Port)) {
		http.Error(w, "destination differs from isolated worker", 409)
		return
	}
	grants, e := labCHGrants(ctx, p, secret)
	if e != nil {
		http.Error(w, "destination grant discovery unavailable", 503)
		return
	}
	query := fmt.Sprintf("SELECT name FROM system.tables WHERE database='%s' AND startsWith(comment,'dataflow-owned:') ORDER BY name LIMIT 100 FORMAT JSONEachRow", p.Database)
	raw, e := labCHCatalogQuery(ctx, p, secret, query)
	if e != nil {
		http.Error(w, "destination catalog unavailable", 503)
		return
	}
	tables := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		var item struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(line), &item) != nil || !validTypedColumnName(item.Name) {
			http.Error(w, "destination catalog invalid", 503)
			return
		}
		tables = append(tables, item.Name)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"database": p.Database, "grants": grants, "can_create_typed": grants.usable(), "owned_tables": tables})
}
