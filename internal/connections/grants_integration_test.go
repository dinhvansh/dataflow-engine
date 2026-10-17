package connectionsecurity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func fixtureActorKey(v string) string {
	hash := sha256.Sum256([]byte(v))
	return hex.EncodeToString(hash[:])
}

func TestWorkspaceGrantLifecycleIntegration(t *testing.T) {
	dsn := os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires ephemeral CI PostgreSQL")
	}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED") != "true" {
		t.Fatal("refusing writes to non-isolated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for _, file := range []string{"018_connection_security_metadata.sql", "019_connection_workspace_grant_audit.sql"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", file))
		if err != nil {
			t.Fatal(err)
		}
		// Verify schema can be applied twice to synthetic fixture.
		for i := 0; i < 2; i++ {
			if _, err := pool.Exec(ctx, string(src)); err != nil {
				t.Fatalf("%s migration #%d: %v", file, i+1, err)
			}
		}
	}
	workspaceA := fmt.Sprintf("qa%x", time.Now().UnixNano())
	workspaceB := workspaceA + "b"
	for _, ws := range []string{workspaceA, workspaceB} {
		if _, err := pool.Exec(ctx, `INSERT INTO connection_security_workspaces(id) VALUES($1)`, ws); err != nil {
			t.Fatal(err)
		}
	}
	admin := fixtureActorKey("synthetic-admin")
	recipient := fixtureActorKey("synthetic-target")
	otherAdmin := fixtureActorKey("other-workspace-admin")
	for _, seed := range []struct{ ws, actor, scope string }{
		{workspaceA, admin, "MANAGE"},
		{workspaceB, otherAdmin, "MANAGE"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
			VALUES($1,$2,$3)`, seed.ws, seed.actor, seed.scope); err != nil {
			t.Fatal(err)
		}
	}
	catalog := SecureCatalog{PG: pool}
	manager := GrantAdmin{Catalog: catalog}
	test := func(actor, ws, target string, scope Scope, enable bool, expected bool) {
		t.Helper()
		changed, err := manager.SetScope(ctx, actor, ws, target, scope, enable)
		if err != nil || changed != expected {
			t.Fatalf("SetScope(%s,%s,%s,%v) changed=%v err=%v", actor, ws, scope, enable, changed, err)
		}
	}
	test(admin, workspaceA, recipient, View, true, true)
	test(admin, workspaceA, recipient, View, true, false)
	if err := catalog.RequireScope(ctx, recipient, workspaceA, View); err != nil {
		t.Fatalf("grant ineffective: %v", err)
	}
	if err := catalog.RequireScope(ctx, recipient, workspaceB, View); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-workspace grant: %v", err)
	}
	if _, err := manager.SetScope(ctx, recipient, workspaceA, otherAdmin, Test, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer delegated privileges: %v", err)
	}
	for _, banned := range []Scope{Manage, "OWNER", ""} {
		if _, err := manager.SetScope(ctx, admin, workspaceA, recipient, banned, true); !errors.Is(err, ErrForbidden) {
			t.Fatalf("MANAGE delegated by lower-privileged path: %v", err)
		}
	}
	if _, err := manager.SetScope(ctx, admin, workspaceB, recipient, Test, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("workspace MANAGE leaked: %v", err)
	}
	if _, err := manager.SetScope(ctx, admin, workspaceA, "X-Actor: spoof", View, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unverified target accepted: %v", err)
	}
	test(admin, workspaceA, recipient, View, false, true)
	test(admin, workspaceA, recipient, View, false, false)
	if err := catalog.RequireScope(ctx, recipient, workspaceA, View); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revocation not observed: %v", err)
	}
	test(admin, workspaceA, recipient, View, true, true)
	if err := catalog.RequireScope(ctx, recipient, workspaceA, View); err != nil {
		t.Fatalf("re-grant not effective: %v", err)
	}

	// Separate trusted operator control: revoke MANAGE directly in the disposable fixture.
	if _, err = pool.Exec(ctx, `UPDATE connection_security_grants SET revoked_at=now()
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE'`, workspaceA, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetScope(ctx, admin, workspaceA, recipient, Test, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked admin can grant: %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM connection_security_grant_audit
		WHERE workspace_id=$1 AND actor_id=$2 AND target_actor_id=$3`, workspaceA, admin, recipient).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 3 { // grant, revoke, re-grant (duplicate replay is no-op)
		t.Fatalf("expected 3 transition audits, got %d", auditCount)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM connection_security_grant_audit WHERE workspace_id=$1`, workspaceA); err == nil {
		t.Fatal("append-only lifecycle audit deleted")
	}
	if _, err := pool.Exec(ctx, `UPDATE connection_security_grant_audit SET action='granted' WHERE workspace_id=$1`, workspaceA); err == nil {
		t.Fatal("append-only lifecycle audit modified")
	}
	// Keep synthetic audit in this disposable CI database, never truncate real audit.
}

func TestAuthenticatedGrantAdminSynthetic(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://synthetic.invalid/issuer"
	verifier, err := NewRS256Verifier(issuer, "dataflow-lab", map[string]*rsa.PublicKey{"kid1": &key.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": "kid1"}
	claims := func(subject string) map[string]any {
		return map[string]any{"iss": issuer, "aud": "dataflow-lab", "sub": subject,
			"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Minute).Unix()}
	}
	adminJWT := signedSyntheticJWT(t, key, header, claims("admin"))
	targetJWT := signedSyntheticJWT(t, key, header, claims("target"))
	target, err := verifier.Verify(targetJWT)
	if err != nil {
		t.Fatal(err)
	}
	manager := AuthenticatedGrantAdmin{Verifier: verifier}
	if _, err := manager.SetScope(context.Background(), "malformed", "qa-lab", target, View, true); !errors.Is(err, ErrForbidden) {
		t.Fatal("invalid admin JWT accepted")
	}
	otherVerifier, err := NewRS256Verifier("https://other.invalid/issuer", "dataflow-lab", map[string]*rsa.PublicKey{"kid1": &key.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	otherTargetJWT := signedSyntheticJWT(t, key, header, map[string]any{
		"iss": "https://other.invalid/issuer", "aud": "dataflow-lab", "sub": "target",
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Minute).Unix()})
	otherTarget, err := otherVerifier.Verify(otherTargetJWT)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetScope(context.Background(), adminJWT, "qa-lab", otherTarget, View, true); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign IdP subject accepted")
	}
	// No DB pool: even valid JWTs MUST fail closed.
	if _, err := manager.SetScope(context.Background(), adminJWT, "qa-lab", target, View, true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured metadata accepted: %v", err)
	}
}
