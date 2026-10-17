// Package connectionsecurity defines fail-closed, server-only boundaries for
// V3.2B. Nothing in this package is an HTTP endpoint or a configured vault.
// Do not wire this into a network path until identity and provider are approved.
package connectionsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrForbidden   = errors.New("connection access denied")
	ErrUnavailable = errors.New("connection security service unavailable")
	ErrInvalid     = errors.New("invalid connection security metadata")
)

type Scope string

const (
	View   Scope = "VIEW"
	Use    Scope = "USE"
	Manage Scope = "MANAGE"
	Test   Scope = "TEST"
)

func validScope(s Scope) bool {
	return s == View || s == Use || s == Manage || s == Test
}

func validIdentity(s string) bool {
	return s != "" && s == strings.TrimSpace(s) &&
		!strings.ContainsAny(s, "\r\n\t\x00") && s != "*" && len(s) <= 128
}

// Grant is a trusted, server-derived authorization input, never a client header.
// A scope in one workspace grants no access in any other workspace.
type Grant struct {
	ActorID     string
	WorkspaceID string
	Scopes      []Scope
}

// Policy holds a copied, immutable authorization snapshot. It is suitable for
// synthetic tests; production must supply authenticated identities and refresh
// grants upon revocation, rather than using this snapshot as an SSO substitute.
type Policy struct {
	grants map[string]map[string]map[Scope]bool
}

func NewPolicy(grants []Grant) (*Policy, error) {
	p := &Policy{grants: make(map[string]map[string]map[Scope]bool)}
	for _, g := range grants {
		if !validIdentity(g.ActorID) || !validIdentity(g.WorkspaceID) || len(g.Scopes) == 0 {
			return nil, ErrInvalid
		}
		if p.grants[g.ActorID] == nil {
			p.grants[g.ActorID] = make(map[string]map[Scope]bool)
		}
		if p.grants[g.ActorID][g.WorkspaceID] == nil {
			p.grants[g.ActorID][g.WorkspaceID] = make(map[Scope]bool)
		}
		for _, scope := range g.Scopes {
			if !validScope(scope) {
				return nil, ErrInvalid
			}
			p.grants[g.ActorID][g.WorkspaceID][scope] = true
		}
	}
	return p, nil
}

func (p *Policy) Authorize(ctx context.Context, actorID, workspaceID string, scope Scope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || !validIdentity(actorID) || !validIdentity(workspaceID) || !validScope(scope) {
		return ErrForbidden
	}
	if !p.grants[actorID][workspaceID][scope] {
		return ErrForbidden
	}
	return nil
}

// SecretRef is an opaque workspace-bound pointer into a future server vault.
// It MUST NOT contain passwords, connection URLs or network endpoints.
// JSON and String are deliberately redacted to resist accidental disclosure.
type SecretRef struct {
	workspaceID string
	handle      string
	version     uint64
}

var opaqueHandle = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_/-]{3,127}$")

func NewSecretRef(workspaceID, handle string, version uint64) (SecretRef, error) {
	if !validIdentity(workspaceID) || !opaqueHandle.MatchString(handle) ||
		strings.Contains(handle, "//") || version == 0 {
		return SecretRef{}, ErrInvalid
	}
	return SecretRef{workspaceID: workspaceID, handle: handle, version: version}, nil
}

func (s SecretRef) WorkspaceID() string { return s.workspaceID }
func (s SecretRef) Version() uint64      { return s.version }
func (s SecretRef) String() string       { return "[REDACTED]" }
func (s SecretRef) MarshalJSON() ([]byte, error) {
	return json.Marshal("[REDACTED]")
}

// OpaqueHandle is only for implementing a trusted server-side vault adapter.
// Never log it, persist it in a client payload, or use it as a network endpoint.
func (s SecretRef) OpaqueHandle() string { return s.handle }

type Connection struct {
	ID          string
	WorkspaceID string
	Secret      SecretRef
}

// SafeConnection is the only client-safe projection supplied by this package.
// Even opaque secret handles, provider names and secret versions stay server-only.
type SafeConnection struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace"`
	Configured  bool   `json:"secret_configured"`
}

func (c Connection) Redacted() SafeConnection {
	return SafeConnection{ID: c.ID, WorkspaceID: c.WorkspaceID, Configured: c.Secret.version != 0}
}

func validConnection(c Connection) bool {
	return validIdentity(c.ID) && validIdentity(c.WorkspaceID) &&
		c.Secret.version > 0 && c.Secret.workspaceID == c.WorkspaceID
}

// Vault is a provider-neutral interface, not a default storage choice. A real
// adapter must enforce tenant isolation, version CAS, encrypted storage,
// credential lifecycle, auditing and egress restrictions separately.
type Vault interface {
	Read(ctx context.Context, ref SecretRef) ([]byte, error)
	Rotate(ctx context.Context, previous SecretRef, replacement []byte) (SecretRef, error)
}

type AuditEvent struct {
	ActorID      string
	WorkspaceID  string
	ConnectionID string
	Action       string
	OccurredAt   time.Time
}

type AuditSink interface {
	Append(ctx context.Context, event AuditEvent) error
}

// Service remains disconnected from all HTTP routes and runtime database
// connections in V3.2B-1. No production identity or vault provider is assumed.
type Service struct {
	Policy *Policy
	Vault  Vault
	Audit  AuditSink
}

func (s Service) authorize(ctx context.Context, actor string, conn Connection, scope Scope) error {
	if !validConnection(conn) {
		return ErrInvalid
	}
	if s.Policy == nil {
		return ErrForbidden
	}
	return s.Policy.Authorize(ctx, actor, conn.WorkspaceID, scope)
}

func (s Service) audit(ctx context.Context, actor string, conn Connection, action string) error {
	if s.Audit == nil {
		return ErrUnavailable
	}
	if err := s.Audit.Append(ctx, AuditEvent{
		ActorID: actor, WorkspaceID: conn.WorkspaceID, ConnectionID: conn.ID,
		Action: action, OccurredAt: time.Now().UTC(),
	}); err != nil {
		return ErrUnavailable
	}
	return nil
}

// WithSecret exposes credential bytes only to a server callback for USE or
// TEST; bytes are zeroed on return. Never pass the callback to an HTTP handler.
func (s Service) WithSecret(ctx context.Context, actor string, conn Connection, scope Scope, consume func([]byte) error) error {
	if scope != Use && scope != Test {
		return ErrForbidden
	}
	if err := s.authorize(ctx, actor, conn, scope); err != nil {
		return err
	}
	if s.Vault == nil || consume == nil {
		return ErrUnavailable
	}
	if err := s.audit(ctx, actor, conn, "credential_access_requested"); err != nil {
		return err
	}
	secret, err := s.Vault.Read(ctx, conn.Secret)
	if err != nil {
		return ErrUnavailable
	}
	defer clear(secret)
	return consume(secret)
}

// RotateSecret is a synthetic server-side contract. It audits intent BEFORE
// mutating the vault. It does not claim transactional completion/audit or
// persisted metadata updates; those are required in the provider integration.
func (s Service) RotateSecret(ctx context.Context, actor string, conn Connection, replacement []byte) (Connection, error) {
	if err := s.authorize(ctx, actor, conn, Manage); err != nil {
		return Connection{}, err
	}
	if s.Vault == nil || len(replacement) == 0 || len(replacement) > 65536 {
		return Connection{}, ErrUnavailable
	}
	if err := s.audit(ctx, actor, conn, "credential_rotation_requested"); err != nil {
		return Connection{}, err
	}
	next, err := s.Vault.Rotate(ctx, conn.Secret, replacement)
	if err != nil {
		return Connection{}, ErrUnavailable
	}
	if next.workspaceID != conn.WorkspaceID || next.handle != conn.Secret.handle || next.version <= conn.Secret.version {
		return Connection{}, ErrUnavailable
	}
	conn.Secret = next
	return conn, nil
}
