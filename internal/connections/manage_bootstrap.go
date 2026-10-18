package connectionsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

// ManageBootstrapRequest is a strictly offline, operator-signed authorization
// envelope. It is NOT an HTTP payload and MUST NOT be trusted just because
// someone can supply an OIDC token or choose a target actor identifier.
type ManageBootstrapRequest struct {
	RequestID string
	Workspace string
	TargetActorID string
	Issuer string
	Action string
	IssuedAt int64
	ExpiresAt int64
	Approvals []BootstrapSignature
}
type BootstrapSignature struct {
	KeyID string
	Signature string // base64url Ed25519 signature of the domain-separated payload
}
func (r ManageBootstrapRequest) String() string { return "ManageBootstrapRequest([REDACTED])" }
func (r ManageBootstrapRequest) GoString() string { return r.String() }
func (r ManageBootstrapRequest) MarshalJSON() ([]byte,error) { return json.Marshal("[REDACTED]") }

// ManageBootstrap is deliberately disconnected from every HTTP endpoint and
// production runtime. Isolated mode MUST be enabled by a trusted operator.
type ManageBootstrap struct {
	catalog SecureCatalog
	verifier *RS256Verifier
	operators map[string]ed25519.PublicKey
	isolated bool
	now func() time.Time
}
var bootstrapRequestID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// NewIsolatedManageBootstrap accepts independently approved, PINNED operator
// Ed25519 public keys, not a JWKS URL, browser-provided key or shared lab token.
// At least two distinct public keys must be present, and exactly two different
// signers must approve a specific workspace+target+request with a short expiry.
func NewIsolatedManageBootstrap(catalog SecureCatalog, verifier *RS256Verifier, operatorKeys map[string]ed25519.PublicKey, isolated bool) (*ManageBootstrap,error) {
	if !isolated || os.Getenv(secureViewIsolatedFlag)!="true" ||
		os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" ||
		catalog.PG==nil || verifier==nil || len(operatorKeys)<2 || len(operatorKeys)>16 {
		return nil,ErrUnavailable
	}
	keys:=make(map[string]ed25519.PublicKey,len(operatorKeys))
	seen:=map[string]bool{}
	for id,key:=range operatorKeys {
		if !keyIDPattern.MatchString(id)||len(key)!=ed25519.PublicKeySize {return nil,ErrInvalid}
		k:=append(ed25519.PublicKey(nil),key...)
		digest:=sha256.Sum256(k)
		fingerprint:=hex.EncodeToString(digest[:])
		if seen[fingerprint] {return nil,ErrInvalid}
		seen[fingerprint]=true
		keys[id]=k
	}
	return &ManageBootstrap{catalog:catalog,verifier:verifier,operators:keys,isolated:true,now:time.Now},nil
}

type manageBootstrapPayload struct {
	RequestID string `json:"request_id"`
	Workspace string `json:"workspace"`
	TargetActorID string `json:"target_actor_id"`
	Issuer string `json:"issuer"`
	Action string `json:"action"`
	IssuedAt int64 `json:"issued_at"`
	ExpiresAt int64 `json:"expires_at"`
}

// BootstrapSigningMessage is the deterministic, domain-separated bytes two
// authorized, independent operators must sign AFTER validating the issuer,
// target user and workspace ownership out of band. No secrets are included.
func BootstrapSigningMessage(request ManageBootstrapRequest) ([]byte,error) {
	p:=manageBootstrapPayload{
		RequestID:request.RequestID,Workspace:request.Workspace,
		TargetActorID:request.TargetActorID,Issuer:request.Issuer,
		Action:request.Action,IssuedAt:request.IssuedAt,ExpiresAt:request.ExpiresAt,
	}
	b,err:=json.Marshal(p)
	if err!=nil{return nil,ErrInvalid}
	return append([]byte("dataflow/manage-bootstrap/v1\n"),b...),nil
}

func (b *ManageBootstrap) authorizeEnvelope(targetJWT string, request ManageBootstrapRequest) (string,[32]byte,error) {
	var zero [32]byte
	if b==nil||!b.isolated||b.now==nil||b.verifier==nil||len(b.operators)<2 {
		return "",zero,ErrUnavailable
	}
	target,err:=b.verifier.Verify(targetJWT)
	if err!=nil||!target.Valid()||target.issuer!=request.Issuer||
		target.ActorID()!=request.TargetActorID||!validWorkspace(request.Workspace)||
		!bootstrapRequestID.MatchString(request.RequestID)||
		request.Action!="bootstrap_manage"||len(request.Approvals)!=2 {
		return "",zero,ErrForbidden
	}
	now:=b.now().UTC().Unix()
	if request.IssuedAt<=0||request.IssuedAt>now+60||
		request.IssuedAt<now-900||request.ExpiresAt<=now||
		request.ExpiresAt<=request.IssuedAt||
		request.ExpiresAt-request.IssuedAt>900 {
		return "",zero,ErrForbidden
	}
	signed,err:=BootstrapSigningMessage(request)
	if err!=nil{return "",zero,ErrForbidden}
	approved:=map[string]bool{}
	for _,approval:=range request.Approvals {
		key,found:=b.operators[approval.KeyID]
		if !found||approved[approval.KeyID] {return "",zero,ErrForbidden}
		sig,err:=base64.RawURLEncoding.DecodeString(approval.Signature)
		if err!=nil||len(sig)!=ed25519.SignatureSize||
			base64.RawURLEncoding.EncodeToString(sig)!=approval.Signature||
			!ed25519.Verify(key,signed,sig) {
			return "",zero,ErrForbidden
		}
		approved[approval.KeyID]=true
	}
	return target.ActorID(),sha256.Sum256(signed),nil
}

// Apply is a ONE-TIME, signed and workspace-bound isolated-lab MANAGE
// enrollment. Unlike GrantAdmin, this function is NOT callable by a workspace
// manager. A new, independently signed request is required to re-grant after
// revocation; replay of an old request NEVER restores revoked access.
//
// The workspace lock serializes concurrent bootstrap attempts. The grant and
// immutable evidence commit atomically; failed audit rolls both back.
func (b *ManageBootstrap) Apply(ctx context.Context,targetJWT string,request ManageBootstrapRequest)(bool,error){
	if os.Getenv(secureViewIsolatedFlag)!="true" ||
		os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {
		return false,ErrUnavailable
	}
	actor,digest,err:=b.authorizeEnvelope(targetJWT,request)
	if err!=nil{return false,err}
	if b.catalog.PG==nil{return false,ErrUnavailable}
	tx,err:=b.catalog.PG.Begin(ctx)
	if err!=nil{return false,ErrUnavailable}
	defer tx.Rollback(ctx)
	var workspace string
	err=tx.QueryRow(ctx,`SELECT id FROM connection_security_workspaces WHERE id=$1 FOR UPDATE`,
		request.Workspace).Scan(&workspace)
	if errors.Is(err,pgx.ErrNoRows){return false,ErrForbidden}
	if err!=nil{return false,ErrUnavailable}

	hash:=hex.EncodeToString(digest[:])
	var storedHash string
	err=tx.QueryRow(ctx,`SELECT payload_sha256 FROM connection_security_manage_bootstrap
		WHERE request_id=$1::uuid FOR UPDATE`,request.RequestID).Scan(&storedHash)
	if err==nil {
		if storedHash!=hash {return false,ErrForbidden}
		// An already accepted approval may be retried ONLY while the grant
		// remains active. It cannot undo a later operator revocation.
		var active bool
		err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM connection_security_grants
			WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE' AND revoked_at IS NULL)`,
			request.Workspace,actor).Scan(&active)
		if err!=nil{return false,ErrUnavailable}
		if !active{return false,ErrForbidden}
		if err=tx.Commit(ctx);err!=nil{return false,ErrUnavailable}
		return false,nil
	}
	if !errors.Is(err,pgx.ErrNoRows){return false,ErrUnavailable}
	var active bool
	err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM connection_security_grants
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE' AND revoked_at IS NULL)`,
		request.Workspace,actor).Scan(&active)
	if err!=nil{return false,ErrUnavailable}
	if active{return false,ErrForbidden}

	_,err=tx.Exec(ctx,`INSERT INTO connection_security_manage_bootstrap
		(request_id,workspace_id,target_actor_id,payload_sha256,signer_a,signer_b)
		VALUES($1::uuid,$2,$3,$4,$5,$6)`,request.RequestID,request.Workspace,actor,hash,
		request.Approvals[0].KeyID,request.Approvals[1].KeyID)
	if err!=nil{return false,ErrUnavailable}
	var changed bool
	err=tx.QueryRow(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
		VALUES($1,$2,'MANAGE')
		ON CONFLICT(workspace_id,actor_id,scope)
		DO UPDATE SET granted_at=now(),revoked_at=NULL
		WHERE connection_security_grants.revoked_at IS NOT NULL
		RETURNING true`,request.Workspace,actor).Scan(&changed)
	if err!=nil{return false,ErrUnavailable}
	if err=tx.Commit(ctx);err!=nil{return false,ErrUnavailable}
	return true,nil
}
