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

	"github.com/jackc/pgx/v5"
)

// RecoveryApproval is a NEW two-operator decision, separate from the signed
// inspection/review. It can finalize only an already-CONSISTENT metadata state;
// it cannot repair diverged metadata or roll Vault back to an earlier version.
type RecoveryApproval struct {
	RequestID string
	ReviewRequestID string
	RotationID string
	Workspace string
	ActorID string
	Issuer string
	Action string
	ExpectedRevision int64
	TargetVersion int64
	IssuedAt int64
	ExpiresAt int64
	Approvals []BootstrapSignature
}
func(r RecoveryApproval) String()string{return "RecoveryApproval([REDACTED])"}
func(r RecoveryApproval) GoString()string{return r.String()}
func(r RecoveryApproval) MarshalJSON()([]byte,error){return json.Marshal("[REDACTED]")}

type recoveryPayload struct {
	RequestID string `json:"request_id"`
	ReviewRequestID string `json:"review_request_id"`
	RotationID string `json:"rotation_id"`
	Workspace string `json:"workspace"`
	ActorID string `json:"actor_id"`
	Issuer string `json:"issuer"`
	Action string `json:"action"`
	ExpectedRevision int64 `json:"expected_revision"`
	TargetVersion int64 `json:"target_version"`
	IssuedAt int64 `json:"issued_at"`
	ExpiresAt int64 `json:"expires_at"`
}

func RecoverySigningMessage(r RecoveryApproval)([]byte,error){
	p:=recoveryPayload{
		RequestID:r.RequestID,ReviewRequestID:r.ReviewRequestID,
		RotationID:r.RotationID,Workspace:r.Workspace,ActorID:r.ActorID,
		Issuer:r.Issuer,Action:r.Action,ExpectedRevision:r.ExpectedRevision,
		TargetVersion:r.TargetVersion,IssuedAt:r.IssuedAt,ExpiresAt:r.ExpiresAt,
	}
	buf,err:=json.Marshal(p)
	if err!=nil{return nil,ErrInvalid}
	return append([]byte("dataflow/confirmed-applied-recovery/v1\n"),buf...),nil
}

func(b *ManageBootstrap) verifyRecovery(token string,r RecoveryApproval)(string,[32]byte,error){
	var empty [32]byte
	if b==nil||!b.isolated||b.verifier==nil||b.now==nil||len(b.operators)<2||
		os.Getenv(secureViewIsolatedFlag)!="true"||os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {
		return "",empty,ErrUnavailable
	}
	actor,err:=b.verifier.Verify(token)
	if err!=nil||!actor.Valid()||actor.issuer!=r.Issuer||actor.ActorID()!=r.ActorID||
		!validWorkspace(r.Workspace)||!bootstrapRequestID.MatchString(r.RequestID)||
		!bootstrapRequestID.MatchString(r.ReviewRequestID)||
		!bootstrapRequestID.MatchString(r.RotationID)||
		r.Action!="confirm_already_applied"||r.ExpectedRevision<1||r.TargetVersion<2||
		len(r.Approvals)!=2{return "",empty,ErrForbidden}
	now:=b.now().UTC().Unix()
	if r.IssuedAt<=0||r.IssuedAt>now+60||r.IssuedAt<now-900||
		r.ExpiresAt<=now||r.ExpiresAt<=r.IssuedAt||r.ExpiresAt-r.IssuedAt>900{
		return "",empty,ErrForbidden
	}
	message,err:=RecoverySigningMessage(r)
	if err!=nil{return "",empty,ErrForbidden}
	seen:=map[string]bool{}
	for _,signature:=range r.Approvals {
		key,ok:=b.operators[signature.KeyID]
		if !ok||seen[signature.KeyID]{return "",empty,ErrForbidden}
		raw,err:=base64.RawURLEncoding.DecodeString(signature.Signature)
		if err!=nil||len(raw)!=ed25519.SignatureSize||
			base64.RawURLEncoding.EncodeToString(raw)!=signature.Signature||
			!ed25519.Verify(key,message,raw){return "",empty,ErrForbidden}
		seen[signature.KeyID]=true
	}
	return actor.ActorID(),sha256.Sum256(message),nil
}

// FinalizeAlreadyApplied is an opt-in isolated TEST recovery primitive.
// Only a prior NEEDS_REVIEW journal for which the metadata is ALREADY at the
// target version+expected revision AND an independently signed review classified
// 'target' may move to applied. This NEVER writes the profile or Vault.
//
// A current-version Vault metadata proof + pinned target read are both required.
// Neither proves cross-system atomicity when an external party can rotate Vault:
// production requires coordinated external locks/policies and a separate review.
func(b *ManageBootstrap) FinalizeAlreadyApplied(ctx context.Context,token string,
	r RecoveryApproval,provider Vault)(bool,error){
	actor,digest,err:=b.verifyRecovery(token,r)
	if err!=nil{return false,err}
	latest,ok:=provider.(CurrentVaultVersion)
	if !ok||b.catalog.PG==nil{return false,ErrUnavailable}
	ctx,cancel:=context.WithTimeout(ctx,3*time.Second)
	defer cancel()
	tx,err:=b.catalog.beginAuthorized(ctx,actor,r.Workspace,Manage)
	if err!=nil{return false,err}
	defer tx.Rollback(ctx)
	var rotationState,handle,profileHandle string
	var previous,target,expected,actualVersion,actualRevision int64
	err=tx.QueryRow(ctx,`SELECT j.state,j.secret_handle,p.secret_handle,j.from_version,
		j.target_version,j.expected_revision,p.secret_version,p.revision
		FROM connection_security_rotation_journal j
		JOIN connection_security_profiles p
		 ON p.id=j.connection_id AND p.workspace_id=j.workspace_id
		WHERE j.id::text=$1 AND j.workspace_id=$2
		FOR UPDATE OF j,p`,r.RotationID,r.Workspace).
		Scan(&rotationState,&handle,&profileHandle,&previous,&target,&expected,
			&actualVersion,&actualRevision)
	if errors.Is(err,pgx.ErrNoRows){return false,ErrForbidden}
	if err!=nil{return false,ErrUnavailable}
	// Exact request replay must still pass current permission and metadata
	// checks; it can never re-enable an altered/revoked connection.
	var oldDigest string
	err=tx.QueryRow(ctx,`SELECT signed_payload_sha256 FROM connection_security_rotation_recoveries
		WHERE request_id=$1::uuid AND workspace_id=$2 AND rotation_id=$3::uuid FOR SHARE`,
		r.RequestID,r.Workspace,r.RotationID).Scan(&oldDigest)
	if err==nil{
		if oldDigest!=hex.EncodeToString(digest[:])||rotationState!="applied"||
			actualVersion!=target||actualRevision!=expected+1{return false,ErrForbidden}
		if err=tx.Commit(ctx);err!=nil{return false,ErrUnavailable}
		return false,nil
	}
	if !errors.Is(err,pgx.ErrNoRows){return false,ErrUnavailable}
	if rotationState!="needs_review"||previous<=0||target!=previous+1||
		expected!=r.ExpectedRevision||target!=r.TargetVersion||
		actualVersion!=target||actualRevision!=expected+1||profileHandle!=handle {
		return false,ErrForbidden
	}
	// A human review of the exact rotation MUST independently record target
	// state, using a different signed request from the recovery authorization.
	var classification string
	err=tx.QueryRow(ctx,`SELECT assessment FROM connection_security_rotation_reviews
		WHERE request_id=$1::uuid AND rotation_id=$2::uuid AND workspace_id=$3
		 AND reviewer_actor_id=$4 FOR SHARE`,
		r.ReviewRequestID,r.RotationID,r.Workspace,actor).Scan(&classification)
	if errors.Is(err,pgx.ErrNoRows){return false,ErrForbidden}
	if err!=nil{return false,ErrUnavailable}
	if classification!="target"{return false,ErrForbidden}
	ref,err:=NewSecretRef(r.Workspace,handle,uint64(target))
	if err!=nil{return false,ErrUnavailable}
	current,err:=latest.LatestVersion(ctx,ref)
	if err!=nil||current!=uint64(target){return false,ErrUnavailable}
	secret,readErr:=provider.Read(ctx,ref)
	ok= len(secret)>0
	clear(secret)
	if readErr!=nil||!ok{return false,ErrUnavailable}
	// Recheck the latest version after read to reduce, NOT remove, external
	// rotation races. No production availability or safety claim is made.
	after,err:=latest.LatestVersion(ctx,ref)
	if err!=nil||after!=uint64(target){return false,ErrUnavailable}
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_rotation_recoveries
		(request_id,rotation_id,review_request_id,workspace_id,actor_id,
		signed_payload_sha256,signer_a,signer_b,outcome)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,'confirmed_applied')`,
		r.RequestID,r.RotationID,r.ReviewRequestID,r.Workspace,actor,
		hex.EncodeToString(digest[:]),r.Approvals[0].KeyID,r.Approvals[1].KeyID);err!=nil{
		return false,ErrUnavailable
	}
	tag,err:=tx.Exec(ctx,`UPDATE connection_security_rotation_journal
		SET state='applied',finalized_at=now()
		WHERE id::text=$1 AND workspace_id=$2 AND state='needs_review'`,r.RotationID,r.Workspace)
	if err!=nil||tag.RowsAffected()!=1{return false,ErrUnavailable}
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_rotation_events
		(rotation_id,workspace_id,actor_id,action)
		VALUES($1::uuid,$2,$3,'reconciled')`,r.RotationID,r.Workspace,actor);err!=nil{return false,ErrUnavailable}
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit
		(workspace_id,actor_id,connection_id,action)
		SELECT workspace_id,$2,connection_id,'rotation_reconciled'
		FROM connection_security_rotation_journal
		WHERE id::text=$1 AND workspace_id=$3`,r.RotationID,actor,r.Workspace);err!=nil{
		return false,ErrUnavailable
	}
	if err=tx.Commit(ctx);err!=nil{return false,ErrUnavailable}
	return true,nil
}
