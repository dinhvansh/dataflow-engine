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
	"time"

	"github.com/jackc/pgx/v5"
)

// RotationReviewRequest records a TWO-operator signed acknowledgement that a
// quarantined rotation requires manual reconciliation. It NEVER instructs the
// system to overwrite a Vault secret, advance metadata or unblock a journal.
type RotationReviewRequest struct {
	RequestID string
	RotationID string
	Workspace string
	ReviewerActorID string
	Issuer string
	Action string
	ExpectedRevision int64
	TargetVersion int64
	IssuedAt int64
	ExpiresAt int64
	Approvals []BootstrapSignature
}

func (r RotationReviewRequest) String()string{return "RotationReviewRequest([REDACTED])"}
func (r RotationReviewRequest) GoString()string{return r.String()}
func (r RotationReviewRequest) MarshalJSON()([]byte,error){return json.Marshal("[REDACTED]")}

type rotationReviewPayload struct {
	RequestID string `json:"request_id"`
	RotationID string `json:"rotation_id"`
	Workspace string `json:"workspace"`
	ReviewerActorID string `json:"reviewer_actor_id"`
	Issuer string `json:"issuer"`
	Action string `json:"action"`
	ExpectedRevision int64 `json:"expected_revision"`
	TargetVersion int64 `json:"target_version"`
	IssuedAt int64 `json:"issued_at"`
	ExpiresAt int64 `json:"expires_at"`
}

// RotationReviewSigningMessage is purpose-separated from MANAGE bootstrap.
func RotationReviewSigningMessage(r RotationReviewRequest)([]byte,error){
	p:=rotationReviewPayload{
		RequestID:r.RequestID,RotationID:r.RotationID,Workspace:r.Workspace,
		ReviewerActorID:r.ReviewerActorID,Issuer:r.Issuer,Action:r.Action,
		ExpectedRevision:r.ExpectedRevision,TargetVersion:r.TargetVersion,
		IssuedAt:r.IssuedAt,ExpiresAt:r.ExpiresAt,
	}
	b,err:=json.Marshal(p);if err!=nil{return nil,ErrInvalid}
	return append([]byte("dataflow/rotation-review/v1\n"),b...),nil
}

// RotationReviewAssessment intentionally has NO journal-secret or signer data.
type RotationReviewAssessment struct {
	classification string
}
func(a RotationReviewAssessment) Classification()string{return a.classification}
func(a RotationReviewAssessment) String()string{return "RotationReviewAssessment([REDACTED])"}
func(a RotationReviewAssessment) GoString()string{return a.String()}
func(a RotationReviewAssessment) MarshalJSON()([]byte,error){return json.Marshal("[REDACTED]")}

func (b *ManageBootstrap) verifyRotationReview(token string,r RotationReviewRequest)(string,[32]byte,error){
	var none [32]byte
	if b==nil||!b.isolated||b.verifier==nil||b.now==nil||len(b.operators)<2||
		os.Getenv(secureViewIsolatedFlag)!="true"||os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {
		return "",none,ErrUnavailable
	}
	person,err:=b.verifier.Verify(token)
	if err!=nil||!person.Valid()||person.issuer!=r.Issuer||
		person.ActorID()!=r.ReviewerActorID||!validWorkspace(r.Workspace)||
		!bootstrapRequestID.MatchString(r.RequestID)||!bootstrapRequestID.MatchString(r.RotationID)||
		r.Action!="acknowledge_needs_review"||r.ExpectedRevision<1||r.TargetVersion<2||
		len(r.Approvals)!=2 {return "",none,ErrForbidden}
	now:=b.now().UTC().Unix()
	if r.IssuedAt<=0||r.IssuedAt>now+60||r.IssuedAt<now-900||
		r.ExpiresAt<=now||r.ExpiresAt<=r.IssuedAt||r.ExpiresAt-r.IssuedAt>900 {
		return "",none,ErrForbidden
	}
	msg,err:=RotationReviewSigningMessage(r)
	if err!=nil{return "",none,ErrForbidden}
	seen:=map[string]bool{}
	for _,a:=range r.Approvals {
		key,ok:=b.operators[a.KeyID]
		if !ok||seen[a.KeyID]{return "",none,ErrForbidden}
		signature,err:=base64.RawURLEncoding.DecodeString(a.Signature)
		if err!=nil||len(signature)!=ed25519.SignatureSize||
			base64.RawURLEncoding.EncodeToString(signature)!=a.Signature||
			!ed25519.Verify(key,msg,signature) {return "",none,ErrForbidden}
		seen[a.KeyID]=true
	}
	return person.ActorID(),sha256.Sum256(msg),nil
}

// AcknowledgeRotationReview records a signed MANAGE-authorized assessment.
// This is NOT resolution/compensation. A NEEDS_REVIEW journal stays blocked.
// The Vault read is version-pinned, bounded, and performed while the MANAGE,
// journal and profile rows are locked; secrets are always cleared.
func(b *ManageBootstrap) AcknowledgeRotationReview(ctx context.Context, reviewerJWT string,
	r RotationReviewRequest, provider Vault)(RotationReviewAssessment,bool,error) {
	empty:=RotationReviewAssessment{}
	if provider==nil{return empty,false,ErrUnavailable}
	actor,digest,err:=b.verifyRotationReview(reviewerJWT,r)
	if err!=nil{return empty,false,err}
	if b.catalog.PG==nil{return empty,false,ErrUnavailable}
	ctx,cancel:=context.WithTimeout(ctx,3*time.Second)
	defer cancel()
	tx,err:=b.catalog.beginAuthorized(ctx,actor,r.Workspace,Manage)
	if err!=nil{return empty,false,err}
	defer tx.Rollback(ctx)
	var prior,requested,revision,actualVersion,actualRevision int64
	var handle,state string
	err=tx.QueryRow(ctx,`SELECT j.from_version,j.target_version,j.expected_revision,
		p.secret_version,p.revision,j.secret_handle,j.state
		FROM connection_security_rotation_journal j
		JOIN connection_security_profiles p
		 ON p.id=j.connection_id AND p.workspace_id=j.workspace_id
		WHERE j.id::text=$1 AND j.workspace_id=$2 FOR SHARE OF j,p`,
		r.RotationID,r.Workspace).Scan(&prior,&requested,&revision,&actualVersion,
		&actualRevision,&handle,&state)
	if errors.Is(err,pgx.ErrNoRows){return empty,false,ErrForbidden}
	if err!=nil{return empty,false,ErrUnavailable}
	if state!="needs_review"||revision!=r.ExpectedRevision||requested!=r.TargetVersion||
		prior<=0||requested!=prior+1 {return empty,false,ErrForbidden}
	var knownHash string
	err=tx.QueryRow(ctx,`SELECT signed_payload_sha256 FROM connection_security_rotation_reviews
		WHERE request_id=$1::uuid FOR SHARE`,r.RequestID).Scan(&knownHash)
	if err==nil {
		if knownHash!=hex.EncodeToString(digest[:]){return empty,false,ErrForbidden}
		if err=tx.Commit(ctx);err!=nil{return empty,false,ErrUnavailable}
		return empty,false,nil // Exact replay is not an extra review.
	}
	if !errors.Is(err,pgx.ErrNoRows){return empty,false,ErrUnavailable}
	ref,err:=NewSecretRef(r.Workspace,handle,uint64(requested))
	if err!=nil{return empty,false,ErrUnavailable}
	secret,readErr:=provider.Read(ctx,ref)
	readable:=len(secret)>0
	clear(secret)
	if readErr!=nil||!readable{return empty,false,ErrUnavailable}
	category:="diverged"
	if actualVersion==prior&&actualRevision==revision {category="prior"}
	if actualVersion==requested&&actualRevision==revision+1 {category="target"}
	_,err=tx.Exec(ctx,`INSERT INTO connection_security_rotation_reviews
	(request_id,rotation_id,workspace_id,reviewer_actor_id,signed_payload_sha256,
	 signer_a,signer_b,assessment)
	 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8)`,
		r.RequestID,r.RotationID,r.Workspace,actor,hex.EncodeToString(digest[:]),
		r.Approvals[0].KeyID,r.Approvals[1].KeyID,category)
	if err!=nil{return empty,false,ErrUnavailable}
	_,err=tx.Exec(ctx,`INSERT INTO connection_security_audit(workspace_id,actor_id,connection_id,action)
		SELECT workspace_id,$2,connection_id,'rotation_reviewed'
		FROM connection_security_rotation_journal
		WHERE id::text=$1 AND workspace_id=$3`,r.RotationID,actor,r.Workspace)
	if err!=nil{return empty,false,ErrUnavailable}
	if err=tx.Commit(ctx);err!=nil{return empty,false,ErrUnavailable}
	return RotationReviewAssessment{classification:category},true,nil
}
