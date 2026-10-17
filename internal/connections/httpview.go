package connectionsecurity

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	secureViewFlag = "DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST"
	secureViewIsolatedFlag = "DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB"
)

// SecureViewHandler is a separate, read-only, opt-in connection Draft API.
// It does NOT share the V3.2A bearer/admin-token authorization mechanism.
// No source endpoint, credential, execution or Vault mutation is exposed here.
type SecureViewHandler struct {
	Catalog AuthenticatedCatalog
}

func (h SecureViewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Vary", "Authorization")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	// Direct TLS only: never trust spoofable X-Forwarded-Proto from the caller.
	// The current plain HTTP Go listener must NOT serve this route remotely;
	// a reviewed TLS listener is a separate deployment milestone.
	if r.TLS == nil {
		http.Error(w, "TLS required", http.StatusForbidden)
		return
	}
	values, ok := r.URL.Query()["workspace"]
	if !ok || len(values) != 1 || !validWorkspace(values[0]) || len(r.URL.Query()) != 1 {
		http.Error(w, "invalid workspace", http.StatusBadRequest)
		return
	}
	if r.ContentLength > 0 {
		http.Error(w, "GET body unsupported", http.StatusBadRequest)
		return
	}
	authorization := r.Header.Values("Authorization")
	if len(authorization) != 1 || len(authorization[0]) > 16391 ||
		!strings.HasPrefix(authorization[0], "Bearer ") ||
		strings.TrimSpace(strings.TrimPrefix(authorization[0], "Bearer ")) != strings.TrimPrefix(authorization[0], "Bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	jwt := strings.TrimPrefix(authorization[0], "Bearer ")
	if jwt == "" || strings.ContainsAny(jwt, " \r\n\t") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	definitions, err := h.Catalog.ListDrafts(ctx, jwt, values[0])
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			http.Error(w, "access denied", http.StatusForbidden)
		case errors.Is(err, ErrInvalid):
			http.Error(w, "invalid request", http.StatusBadRequest)
		default:
			http.Error(w, "secure catalog unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	if definitions == nil {
		definitions = []DraftProfile{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Definitions []DraftProfile `json:"definitions"`
	}{Definitions: definitions})
}

// NewSecureViewHandlerFromEnv never enables anything by default.
// It rejects incomplete configuration at startup when explicitly enabled.
// Public key is a trusted server-local PEM file, NEVER supplied by the browser.
func NewSecureViewHandlerFromEnv(ctx context.Context, pg *pgxpool.Pool) (http.Handler, error) {
	if os.Getenv(secureViewFlag) != "true" {
		return http.NotFoundHandler(), nil
	}
	if os.Getenv(secureViewIsolatedFlag) != "true" || pg == nil {
		return nil, errors.New("secure connection view requires reviewed isolated lab PostgreSQL")
	}
	issuer := os.Getenv("DATAFLOW_SECURE_OIDC_ISSUER")
	audience := os.Getenv("DATAFLOW_SECURE_OIDC_AUDIENCE")
	kid := os.Getenv("DATAFLOW_SECURE_OIDC_KID")
	keyFile := os.Getenv("DATAFLOW_SECURE_OIDC_RS256_PUBLIC_KEY_FILE")
	if issuer == "" || audience == "" || kid == "" || keyFile == "" {
		return nil, errors.New("secure connection view OIDC configuration incomplete")
	}
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil || len(keyBytes) > 16384 {
		return nil, errors.New("secure connection view public key unavailable")
	}
	block, rest := pem.Decode(keyBytes)
	if block == nil || block.Type != "PUBLIC KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("secure connection view public key invalid PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("secure connection view public key invalid")
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("secure connection view requires pinned RSA public key")
	}
	verifier, err := NewRS256Verifier(issuer, audience, map[string]*rsa.PublicKey{kid: publicKey})
	if err != nil {
		return nil, errors.New("secure connection view verifier configuration invalid")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var ready bool
	err = pg.QueryRow(checkCtx, `SELECT
		to_regclass('connection_security_workspaces') IS NOT NULL
		AND to_regclass('connection_security_grants') IS NOT NULL
		AND to_regclass('connection_security_profiles') IS NOT NULL
		AND to_regclass('connection_security_audit') IS NOT NULL`).Scan(&ready)
	if err != nil || !ready {
		return nil, errors.New("secure connection view requires migration 018")
	}
	return SecureViewHandler{Catalog: AuthenticatedCatalog{
		Verifier: verifier,
		Catalog: SecureCatalog{PG: pg},
	}}, nil
}
