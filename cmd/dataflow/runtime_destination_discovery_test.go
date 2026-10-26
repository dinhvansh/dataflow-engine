package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestLabCHGrantsRefusesMissingPrivilege(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "qa" || pass != "secret" {
			w.WriteHeader(401)
			return
		}
		query := r.URL.Query().Get("query")
		if !strings.HasPrefix(query, "CHECK GRANT ") {
			t.Errorf("unexpected catalog query")
			w.WriteHeader(400)
			return
		}
		if strings.Contains(query, "DROP TABLE") {
			_, _ = w.Write([]byte("0\n"))
		} else {
			_, _ = w.Write([]byte("1\n"))
		}
	}))
	defer srv.Close()
	u, e := url.Parse(srv.URL)
	if e != nil {
		t.Fatal(e)
	}
	host, portText, e := net.SplitHostPort(u.Host)
	if e != nil {
		t.Fatal(e)
	}
	port, e := strconv.Atoi(portText)
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("DATAFLOW_QA_CLICKHOUSE_ALLOWED_ENDPOINT", u.Host)
	t.Setenv("DATAFLOW_QA_CLICKHOUSE_ALLOW_HTTP", "true")
	t.Setenv("DATAFLOW_QA_CLICKHOUSE_DATABASE_ALLOWLIST", "scratch")
	grants, e := labCHGrants(context.Background(), runtimeConnection{Driver: "clickhouse", Host: host, Port: port, Database: "scratch", Username: "qa"}, "secret")
	if e != nil || grants.usable() || grants.Drop || !grants.Create || !grants.Insert || !grants.Select || !grants.Alter {
		t.Fatalf("incorrect grant denial: %+v, %v", grants, e)
	}
}
