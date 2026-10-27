package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Works on a preserved disposable QA database: it owns one new profile and
// does not create, reset, or change any pre-existing stream or checkpoint.
func TestRuntimeProfileMutationRealDisposableQA(t *testing.T) {
	ctx, svc, cleanup := runtimeConnectionRealFixture(t)
	defer cleanup()
	sqlURL, err := url.Parse(os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN"))
	if err != nil || sqlURL.Scheme != "sqlserver" || sqlURL.User == nil || sqlURL.User.Username() != "qa_erp_reader" {
		t.Skip("requires disposable SELECT-only QA SQL Server credentials")
	}
	secret, ok := sqlURL.User.Password()
	if !ok {
		t.Fatal("missing disposable reader password")
	}
	host, portText, err := net.SplitHostPort(sqlURL.Host)
	if err != nil {
		t.Fatal("QA source requires explicit port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB": "true",
		"DATAFLOW_INTEGRATION_PG_ISOLATED":        "true",
		"DATAFLOW_INTEGRATION_MSSQL_ISOLATED":     "true",
		"DATAFLOW_INTEGRATION_CH_ISOLATED":        "true",
		"DATAFLOW_ALLOW_TEST_LOAD":                "true",
		"DATAFLOW_TEST_TARGET_DATABASE":           "scratch",
		"DATAFLOW_QA_SQL_TRUST_CERT":              "true",
		"DATAFLOW_QA_SQLSERVER_ALLOWED_ENDPOINT":  sqlURL.Host,
		"DATAFLOW_CONNECTION_ADMIN_TOKEN":         "synthetic-independent-admin",
		"DATAFLOW_QA_CONNECTION_KEY_B64":          base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
	} {
		t.Setenv(key, value)
	}
	for _, migration := range []string{"031_connection_runtime_profiles.sql", "036_runtime_profile_revocation.sql"} {
		contents, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.pg.Exec(ctx, string(contents)); err != nil {
			t.Fatalf("isolated migration %s: %v", migration, err)
		}
	}
	call := func(method string, payload any) (int, map[string]any, string) {
		t.Helper()
		var body bytes.Buffer
		if payload != nil {
			if err := json.NewEncoder(&body).Encode(payload); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, "/api/v1/connections/runtime", &body)
		req.Header.Set("Authorization", "Bearer "+svc.token)
		req.Header.Set("X-DataFlow-Connection-Admin", "synthetic-independent-admin")
		rec := httptest.NewRecorder()
		svc.runtimeProfilesAPI(rec, req)
		var result map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		return rec.Code, result, rec.Body.String()
	}
	name := "QA mutation " + time.Now().UTC().Format("150405.000000000")
	created, response, body := call(http.MethodPost, createRuntimeConnection{
		Name: name, Driver: "sqlserver", Host: host, Port: port,
		Database: "DataFlowFixtureQA", Username: "qa_erp_reader", Password: secret,
	})
	if created != 201 || strings.Contains(body, secret) {
		t.Fatalf("profile creation failed or leaked credential: %d", created)
	}
	id, _ := response["id"].(string)
	if id == "" {
		t.Fatal("created profile lacks ID")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = svc.pg.Exec(cleanupCtx, `DELETE FROM connection_runtime_profiles WHERE id=$1::uuid`, id)
	}()
	request := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/connections/runtime/test", strings.NewReader(`{"id":"`+id+`"}`))
		req.Header.Set("Authorization", "Bearer "+svc.token)
		req.Header.Set("X-DataFlow-Connection-Admin", "synthetic-independent-admin")
		return req
	}
	rec := httptest.NewRecorder()
	svc.runtimeTestAPI(rec, request())
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("real QA reader test failed: %d", rec.Code)
	}
	var revision int64
	if err := svc.pg.QueryRow(ctx, `SELECT revision FROM connection_runtime_profiles WHERE id=$1::uuid`, id).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		rec = httptest.NewRecorder()
		svc.runtimeTestAPI(rec, request())
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"passed":true`) {
			t.Fatalf("successful verification repeat %d failed: %d", i, rec.Code)
		}
		var after int64
		if err := svc.pg.QueryRow(ctx, `SELECT revision FROM connection_runtime_profiles WHERE id=$1::uuid`, id).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != revision {
			t.Fatalf("successful verification changed profile revision: %d to %d", revision, after)
		}
	}
	change := changeRuntimeConnection{ID: id, Revision: revision, Name: name + " edited",
		Host: host, Port: port, Database: "DataFlowFixtureQA", Username: "qa_erp_reader"}
	code, updated, body := call(http.MethodPatch, change)
	if code != 200 || updated["status"] != "draft" || strings.Contains(body, secret) {
		t.Fatalf("profile edit failed, stayed verified, or leaked secret: %d", code)
	}
	if code, _, _ = call(http.MethodPatch, change); code != 409 {
		t.Fatalf("stale revision accepted: %d", code)
	}
	rec = httptest.NewRecorder()
	svc.runtimeTestAPI(rec, request())
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"passed":true`) {
		t.Fatalf("retest of preserved encrypted credential failed: %d", rec.Code)
	}
	if err := svc.pg.QueryRow(ctx, `SELECT revision FROM connection_runtime_profiles WHERE id=$1::uuid`, id).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if code, _, _ = call(http.MethodDelete, revokeRuntimeConnection{ID: id, Revision: revision}); code != 204 {
		t.Fatalf("revocation failed: %d", code)
	}
	rec = httptest.NewRecorder()
	svc.runtimeTestAPI(rec, request())
	if rec.Code != 409 {
		t.Fatalf("revoked profile re-tested: %d", rec.Code)
	}
	if code, _, body = call(http.MethodGet, nil); code != 200 || strings.Contains(body, secret) {
		t.Fatalf("redacted registry failed after revoke: %d", code)
	}
}
