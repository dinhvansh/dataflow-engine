package connectionsecurity

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

// WorkspaceKVv2Vault is an operator-configured, single-workspace boundary
// around the HTTPS KV-v2 adapter. Each instance receives a separate token and
// an exact approved handle subtree. This is defense in depth; REAL Vault ACLs
// must independently restrict the token to the same exact subtree.
//
// No HTTP handler, connection runtime or live provider is wired to this type.
type WorkspaceKVv2Vault struct {
	delegate *KVv2Vault
	workspace string
	prefix string
}

var workspaceVaultPrefix = regexp.MustCompile(`^ref/[a-z][a-z0-9_-]{2,31}/$`)

// prefix must be an exact subtree like "ref/team_a/" and may not be the
// workspace root, an arbitrary URL, or a slash-prefix of a neighboring subtree.
// endpoint, mount and token come only from trusted server configuration.
func NewWorkspaceKVv2Vault(endpoint, mount, token, workspace, prefix string, client *http.Client) (*WorkspaceKVv2Vault, error) {
	if !validWorkspace(workspace) || !workspaceVaultPrefix.MatchString(prefix) {
		return nil, ErrInvalid
	}
	v, err := NewKVv2Vault(endpoint, mount, token, client)
	if err != nil {
		return nil, err
	}
	return &WorkspaceKVv2Vault{delegate: v, workspace: workspace, prefix: prefix}, nil
}

func (v *WorkspaceKVv2Vault) String() string { return "WorkspaceKVv2Vault([REDACTED])" }
func (v *WorkspaceKVv2Vault) GoString() string { return v.String() }

func (v *WorkspaceKVv2Vault) allowed(ref SecretRef) bool {
	return v != nil && v.delegate != nil && validWorkspace(v.workspace) &&
		ref.WorkspaceID() == v.workspace && ref.Version() != 0 &&
		strings.HasPrefix(ref.OpaqueHandle(), v.prefix) &&
		len(ref.OpaqueHandle()) > len(v.prefix) &&
		!strings.HasSuffix(ref.OpaqueHandle(), "/") &&
		!strings.Contains(ref.OpaqueHandle(), "//")
}

// Read is version-pinned. Every denied reference is rejected without sending
// a Vault HTTP request or exposing the provider endpoint or token.
func (v *WorkspaceKVv2Vault) Read(ctx context.Context, ref SecretRef) ([]byte, error) {
	if !v.allowed(ref) { return nil, ErrForbidden }
	return v.delegate.Read(ctx, ref)
}

// Rotate is CAS-only and cannot mutate another workspace or prefix.
// The returned reference must remain within the same approved scope.
func (v *WorkspaceKVv2Vault) Rotate(ctx context.Context, previous SecretRef, replacement []byte) (SecretRef,error) {
	if !v.allowed(previous) { return SecretRef{}, ErrForbidden }
	next,err:=v.delegate.Rotate(ctx,previous,replacement)
	if err!=nil{return SecretRef{},err}
	if !v.allowed(next) || next.OpaqueHandle()!=previous.OpaqueHandle() ||
		next.Version()!=previous.Version()+1 {return SecretRef{},ErrUnavailable}
	return next,nil
}

// Create is a separately available operator-side primitive, never exposed
// to clients. It can only create an approved key in this one workspace.
// Authorization and durable journal coordination remain caller obligations.
func (v *WorkspaceKVv2Vault) Create(ctx context.Context, handle string, secret []byte) (SecretRef,error) {
	if v == nil {return SecretRef{},ErrForbidden}
	ref,err:=NewSecretRef(v.workspace,handle,1)
	if err!=nil || !v.allowed(ref) {return SecretRef{},ErrForbidden}
	next,err:=v.delegate.Create(ctx,v.workspace,handle,secret)
	if err!=nil{return SecretRef{},err}
	if !v.allowed(next)||next.OpaqueHandle()!=handle||next.Version()!=1 {
		return SecretRef{},ErrUnavailable
	}
	return next,nil
}

var _ Vault = (*WorkspaceKVv2Vault)(nil)
