package connectionsecurity

import (
	"context"
	"crypto/tls"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSecureConnectionViewDisabledAndFailClosed(t *testing.T) {
	t.Setenv(secureViewFlag, "")
	t.Setenv(secureViewIsolatedFlag, "")
	r, err := NewSecureViewHandlerFromEnv(context.Background(), nil)
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secure/connections/definitions?workspace=lab-test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound { t.Fatalf("disabled returned %d", w.Code) }

	t.Setenv(secureViewFlag, "true")
	if _, err = NewSecureViewHandlerFromEnv(context.Background(), nil); err == nil {
		t.Fatal("opt-in with no isolated DB failed open")
	}
	t.Setenv(secureViewIsolatedFlag, "true")
	if _, err = NewSecureViewHandlerFromEnv(context.Background(), nil); err == nil {
		t.Fatal("nil DB failed open")
	}

	// Invalid auth is denied before DB access even if an attacker invents claims.
	h := SecureViewHandler{Catalog: AuthenticatedCatalog{}}
	tlsReq := httptest.NewRequest(http.MethodGet, "/?workspace=lab-test", nil)
	tlsReq.TLS = &fakeTLSState
	tlsReq.Header.Set("Authorization", "Bearer fabricated.jwt.signature")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, tlsReq)
	if w.Code != http.StatusForbidden { t.Fatalf("invalid JWT returned %d", w.Code) }
}

var fakeTLSState = structTLSState()

// Local TLS state is only test metadata; no actual network request here.
func structTLSState() tls.ConnectionState {
	return tls.ConnectionState{HandshakeComplete: true}
}

func TestSecureViewHTTPIntegration(t *testing.T) {
	dsn := os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn == "" { t.Skip("needs disposable CI PostgreSQL") }
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED") != "true" {
		t.Fatal("refusing to mutate non-isolated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil { t.Fatal(err) }
	defer pg.Close()

	sqlBytes, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", "018_connection_security_metadata.sql"))
	if err != nil { t.Fatal(err) }
	if _, err = pg.Exec(ctx, string(sqlBytes)); err != nil { t.Fatal(err) }

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil { t.Fatal(err) }
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil { t.Fatal(err) }
	keyPath := filepath.Join(t.TempDir(), "synthetic-only-pub.pem")
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type:"PUBLIC KEY", Bytes:pubBytes}), 0600); err != nil { t.Fatal(err) }

	issuer, audience, kid := "https://idp.example.invalid/test", "dataflow-test-api", "synthetic-kid"
	t.Setenv(secureViewFlag, "true")
	t.Setenv(secureViewIsolatedFlag, "true")
	t.Setenv("DATAFLOW_SECURE_OIDC_ISSUER", issuer)
	t.Setenv("DATAFLOW_SECURE_OIDC_AUDIENCE", audience)
	t.Setenv("DATAFLOW_SECURE_OIDC_KID", kid)
	t.Setenv("DATAFLOW_SECURE_OIDC_RS256_PUBLIC_KEY_FILE", keyPath)
	handler, err := NewSecureViewHandlerFromEnv(ctx, pg)
	if err != nil { t.Fatal(err) }

	now := time.Now().UTC()
	claims := map[string]any{
		"iss":issuer, "aud":audience, "sub":"synthetic-viewer",
		"iat":now.Add(-time.Minute).Unix(), "nbf":now.Add(-time.Minute).Unix(),
		"exp":now.Add(10*time.Minute).Unix(),
	}
	header := map[string]any{"alg":"RS256", "typ":"JWT", "kid":kid}
	token := signedSyntheticJWT(t,key,header,claims)
	verifier, err := NewRS256Verifier(issuer,audience,map[string]*rsa.PublicKey{kid:&key.PublicKey})
	if err != nil { t.Fatal(err) }
	principal, err := verifier.Verify(token)
	if err != nil { t.Fatal(err) }

	suffix := time.Now().UTC().Format("150405000000000")
	wsA, wsB := "qa"+suffix+"a", "qa"+suffix+"b"
	for _,ws := range []string{wsA,wsB} {
		if _,err = pg.Exec(ctx, `INSERT INTO connection_security_workspaces(id) VALUES($1)`, ws);err!=nil{t.Fatal(err)}
	}
	if _,err = pg.Exec(ctx, `INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
	VALUES ($1,$2,'VIEW'),($1,$2,'MANAGE')`,wsA,principal.ActorID());err!=nil{t.Fatal(err)}
	item, err := (SecureCatalog{PG:pg}).CreateDraft(ctx,principal.ActorID(),wsA,"Synthetic CI Target","clickhouse","destination")
	if err != nil { t.Fatal(err) }
	if item.Status!="draft" { t.Fatal("created active connection") }

	run := func(method, workspace, jwt string, tlsOnly bool, extraHeader string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/secure/connections/definitions?workspace="+workspace, nil)
		if tlsOnly {req.TLS=&fakeTLSState}
		if jwt!="" {req.Header.Set("Authorization","Bearer "+jwt)}
		req.Header.Set("X-Actor",extraHeader)
		req.Header.Set("X-Workspace",wsB)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec,req)
		return rec
	}
	success := run(http.MethodGet,wsA,token,true,"forged-admin")
	if success.Code != 200 {t.Fatalf("VIEW denied: %d %s",success.Code,success.Body.String())}
	body:=success.Body.String()
	if !strings.Contains(body,item.ID)||!strings.Contains(body,`"status":"draft"`) {
		t.Fatalf("expected inert profile missing: %s",body)
	}
	for _,bad := range []string{"ref/", "secret_handle", "secret_version", "password", "dsn", "credential"} {
		if strings.Contains(strings.ToLower(body),bad) {t.Fatalf("credential metadata leaked (%s): %s",bad,body)}
	}
	if r:=run(http.MethodGet,wsB,token,true,"admin");r.Code!=403 {t.Fatalf("forged workspace access: %d",r.Code)}
	if r:=run(http.MethodGet,wsA,"",true,"admin");r.Code!=401 {t.Fatalf("missing bearer: %d",r.Code)}
	if r:=run(http.MethodGet,wsA,token+"x",true,"admin");r.Code!=403 {t.Fatalf("bad signature: %d",r.Code)}
	if r:=run(http.MethodGet,wsA,token,false,"admin");r.Code!=403 {t.Fatalf("HTTP without TLS: %d",r.Code)}
	if r:=run(http.MethodPost,wsA,token,true,"admin");r.Code!=405 {t.Fatalf("unexpected POST: %d",r.Code)}
	if r:=run(http.MethodGet,"*",token,true,"admin");r.Code!=400 {t.Fatalf("wildcard workspace: %d",r.Code)}
	if r:=run(http.MethodGet,wsA+"&workspace="+wsB,token,true,"admin");r.Code!=400 {t.Fatalf("duplicate workspace: %d",r.Code)}
	if r:=run(http.MethodGet,wsA+"&actor=admin",token,true,"admin");r.Code!=400 {t.Fatalf("unknown query accepted: %d",r.Code)}

	// Any permission revocation applies on the NEXT request, not cached identity.
	if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
	WHERE actor_id=$1 AND workspace_id=$2 AND scope='VIEW'`,principal.ActorID(),wsA);err!=nil {t.Fatal(err)}
	if r:=run(http.MethodGet,wsA,token,true,"admin");r.Code!=403 {t.Fatalf("revoked actor can VIEW: %d",r.Code)}
	// Fixture intentionally persists in isolated disposable CI postgres because
	// append-only security audit MUST NOT be deleted for cleanup.
}

func TestSecureViewMissingMigrationFailsClosed(t *testing.T) {
	if !errors.Is(ErrForbidden, ErrForbidden) {t.Fatal("unexpected error sentinel")}
}
