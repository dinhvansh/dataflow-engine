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

// CompensationPlanRequest is a fresh, two-operator signed ESCALATION, NOT an
// instruction to delete/rollback/overwrite secrets or to release quarantine.
type CompensationPlanRequest struct {
 RequestID string
 ReviewRequestID string
 RotationID string
 Workspace string
 ActorID string
 Issuer string
 Action string
 ExpectedJournalRevision int64
 ExpectedProfileRevision int64
 ExpectedProfileVersion int64
 TargetVersion int64
 ObservedVaultVersion int64
 IssuedAt int64
 ExpiresAt int64
 Approvals []BootstrapSignature
}
func (r CompensationPlanRequest) String()string{return "CompensationPlanRequest([REDACTED])"}
func (r CompensationPlanRequest) GoString()string{return r.String()}
func (r CompensationPlanRequest) MarshalJSON()([]byte,error){return json.Marshal("[REDACTED]")}

type compensationPayload struct {
 RequestID string `json:"request_id"`
 ReviewRequestID string `json:"review_request_id"`
 RotationID string `json:"rotation_id"`
 Workspace string `json:"workspace"`
 ActorID string `json:"actor_id"`
 Issuer string `json:"issuer"`
 Action string `json:"action"`
 ExpectedJournalRevision int64 `json:"expected_journal_revision"`
 ExpectedProfileRevision int64 `json:"expected_profile_revision"`
 ExpectedProfileVersion int64 `json:"expected_profile_version"`
 TargetVersion int64 `json:"target_version"`
 ObservedVaultVersion int64 `json:"observed_vault_version"`
 IssuedAt int64 `json:"issued_at"`
 ExpiresAt int64 `json:"expires_at"`
}
func CompensationSigningMessage(r CompensationPlanRequest)([]byte,error){
 p:=compensationPayload{
  RequestID:r.RequestID,ReviewRequestID:r.ReviewRequestID,
  RotationID:r.RotationID,Workspace:r.Workspace,ActorID:r.ActorID,
  Issuer:r.Issuer,Action:r.Action,ExpectedJournalRevision:r.ExpectedJournalRevision,
  ExpectedProfileRevision:r.ExpectedProfileRevision,ExpectedProfileVersion:r.ExpectedProfileVersion,
  TargetVersion:r.TargetVersion,ObservedVaultVersion:r.ObservedVaultVersion,
  IssuedAt:r.IssuedAt,ExpiresAt:r.ExpiresAt,
 }
 buf,err:=json.Marshal(p);if err!=nil{return nil,ErrInvalid}
 return append([]byte("dataflow/compensation-escalation/v1\n"),buf...),nil
}
func(b *ManageBootstrap) verifyCompensation(token string,r CompensationPlanRequest)(string,[32]byte,error){
 var zero [32]byte
 if b==nil||!b.isolated||b.verifier==nil||b.now==nil||len(b.operators)<2||
  os.Getenv(secureViewIsolatedFlag)!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {
  return "",zero,ErrUnavailable
 }
 person,err:=b.verifier.Verify(token)
 if err!=nil||!person.Valid()||person.issuer!=r.Issuer||person.ActorID()!=r.ActorID||
  !validWorkspace(r.Workspace)||
  !bootstrapRequestID.MatchString(r.RequestID)||
  !bootstrapRequestID.MatchString(r.ReviewRequestID)||
  !bootstrapRequestID.MatchString(r.RotationID)||
  r.Action!="escalate_manual_reconciliation"||
  r.ExpectedJournalRevision<1||r.ExpectedProfileRevision<1||
  r.ExpectedProfileVersion<1||r.TargetVersion<2||r.ObservedVaultVersion<r.TargetVersion||
  len(r.Approvals)!=2 {return "",zero,ErrForbidden}
 now:=b.now().UTC().Unix()
 if r.IssuedAt<=0||r.IssuedAt>now+60||r.IssuedAt<now-900||
  r.ExpiresAt<=now||r.ExpiresAt<=r.IssuedAt||r.ExpiresAt-r.IssuedAt>900 {
  return "",zero,ErrForbidden
 }
 msg,err:=CompensationSigningMessage(r);if err!=nil{return "",zero,ErrForbidden}
 seen:=map[string]bool{}
 for _,sig:=range r.Approvals {
  k,ok:=b.operators[sig.KeyID]
  if !ok||seen[sig.KeyID]{return "",zero,ErrForbidden}
  raw,err:=base64.RawURLEncoding.DecodeString(sig.Signature)
  if err!=nil||len(raw)!=ed25519.SignatureSize||
   base64.RawURLEncoding.EncodeToString(raw)!=sig.Signature||
   !ed25519.Verify(k,msg,raw){return "",zero,ErrForbidden}
  seen[sig.KeyID]=true
 }
 return person.ActorID(),sha256.Sum256(msg),nil
}

// PlanDivergentCompensation persists an immutable operator decision to ESCALATE.
// A compensating change cannot be safely inferred from a Vault read, so this
// deliberately NEVER updates profile/journal/Vault or activates credentials.
//
// The signed plan freezes profile metadata + observed current Vault version.
// Reconciliation after manual intervention requires a new review and a
// separately signed confirmed-applied recovery; this record alone is NEVER
// accepted by FinalizeAlreadyApplied.
func(b *ManageBootstrap) PlanDivergentCompensation(ctx context.Context,jwt string,
 r CompensationPlanRequest,provider Vault)(bool,error) {
 actor,hash,err:=b.verifyCompensation(jwt,r)
 if err!=nil{return false,err}
 latest,ok:=provider.(CurrentVaultVersion)
 if !ok||b.catalog.PG==nil{return false,ErrUnavailable}
 ctx,cancel:=context.WithTimeout(ctx,3*time.Second);defer cancel()
 tx,err:=b.catalog.beginAuthorized(ctx,actor,r.Workspace,Manage)
 if err!=nil{return false,err}
 defer tx.Rollback(ctx)
 var state,status,journalHandle,profileHandle string
 var expected,target,fromVersion,revision,profileVersion int64
 err=tx.QueryRow(ctx,`SELECT j.state,p.status,j.secret_handle,p.secret_handle,
 j.expected_revision,j.target_version,j.from_version,p.revision,p.secret_version
 FROM connection_security_rotation_journal j
 JOIN connection_security_profiles p ON p.id=j.connection_id AND p.workspace_id=j.workspace_id
 WHERE j.id::text=$1 AND j.workspace_id=$2 FOR UPDATE OF j,p`,
 r.RotationID,r.Workspace).Scan(&state,&status,&journalHandle,&profileHandle,
 &expected,&target,&fromVersion,&revision,&profileVersion)
 if errors.Is(err,pgx.ErrNoRows){return false,ErrForbidden}
 if err!=nil{return false,ErrUnavailable}
 if state!="needs_review"||status!="draft"||journalHandle!=profileHandle||
  fromVersion<1||target!=fromVersion+1||
  expected!=r.ExpectedJournalRevision||target!=r.TargetVersion||
  revision!=r.ExpectedProfileRevision||profileVersion!=r.ExpectedProfileVersion||
  (revision==expected+1&&profileVersion==target) {
  return false,ErrForbidden
 }
 var existing string
 err=tx.QueryRow(ctx,`SELECT payload_sha256 FROM connection_security_compensation_plans
 WHERE request_id=$1::uuid AND rotation_id=$2::uuid AND workspace_id=$3 FOR SHARE`,
 r.RequestID,r.RotationID,r.Workspace).Scan(&existing)
 if err==nil {
  if existing!=hex.EncodeToString(hash[:]){return false,ErrForbidden}
  if err=tx.Commit(ctx);err!=nil{return false,ErrUnavailable}
  return false,nil
 }
 if !errors.Is(err,pgx.ErrNoRows){return false,ErrUnavailable}
 var assessment string
 err=tx.QueryRow(ctx,`SELECT assessment FROM connection_security_rotation_reviews
 WHERE request_id=$1::uuid AND rotation_id=$2::uuid AND workspace_id=$3
  AND reviewer_actor_id=$4 FOR SHARE`,
 r.ReviewRequestID,r.RotationID,r.Workspace,actor).Scan(&assessment)
 if errors.Is(err,pgx.ErrNoRows){return false,ErrForbidden}
 if err!=nil{return false,ErrUnavailable}
 if assessment!="diverged"{return false,ErrForbidden}
 ref,err:=NewSecretRef(r.Workspace,journalHandle,uint64(target))
 if err!=nil{return false,ErrUnavailable}
 current,err:=latest.LatestVersion(ctx,ref)
 if err!=nil||current!=uint64(r.ObservedVaultVersion){return false,ErrUnavailable}
 secret,readErr:=provider.Read(ctx,ref)
 valid:=len(secret)>0
 clear(secret)
 if readErr!=nil||!valid{return false,ErrUnavailable}
 after,err:=latest.LatestVersion(ctx,ref)
 if err!=nil||after!=current{return false,ErrUnavailable}
 _,err=tx.Exec(ctx,`INSERT INTO connection_security_compensation_plans
 (request_id,rotation_id,review_request_id,workspace_id,actor_id,payload_sha256,
 signer_a,signer_b,expected_profile_revision,expected_profile_version,
 observed_vault_version,plan)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,'escalate_manual_reconciliation')`,
 r.RequestID,r.RotationID,r.ReviewRequestID,r.Workspace,actor,
 hex.EncodeToString(hash[:]),r.Approvals[0].KeyID,r.Approvals[1].KeyID,
 revision,profileVersion,int64(current))
 if err!=nil{return false,ErrUnavailable}
 _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit
 (workspace_id,actor_id,connection_id,action)
 SELECT workspace_id,$2,connection_id,'compensation_planned'
 FROM connection_security_rotation_journal WHERE id::text=$1 AND workspace_id=$3`,
 r.RotationID,actor,r.Workspace)
 if err!=nil{return false,ErrUnavailable}
 if err=tx.Commit(ctx);err!=nil{return false,ErrUnavailable}
 return true,nil
}
