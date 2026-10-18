package connectionsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RotationInspection is server-only, read-only operator evidence. Never
// serialize ticket IDs, handles, actor IDs, vault values or exact versions.
type RotationInspection struct {
	state string
	targetReadable bool
	metadataPrepared bool
	metadataApplied bool
	needsReview bool
}

func (r RotationInspection) State() string { return r.state }
func (r RotationInspection) TargetReadable() bool { return r.targetReadable }
func (r RotationInspection) MetadataPrepared() bool { return r.metadataPrepared }
func (r RotationInspection) MetadataApplied() bool { return r.metadataApplied }
func (r RotationInspection) NeedsReview() bool { return r.needsReview }
func (r RotationInspection) String() string { return "RotationInspection([REDACTED])" }
func (r RotationInspection) GoString() string { return r.String() }
func (r RotationInspection) MarshalJSON() ([]byte,error) { return json.Marshal("[REDACTED]") }

// Inspect is a bounded, fail-closed diagnostic for an operator-approved
// isolated environment. It does NOT transition a journal entry, clear
// NEEDS_REVIEW, repair metadata or activate the credential. Vault Read proves
// only that one pinned version was readable, never identity or provenance.
//
// MANAGE authorization is checked AND row-locked in the same PG transaction
// held during the bounded version probe, preventing revocation races.
func (j RotationJournal) Inspect(ctx context.Context,actor,workspace,id string,provider Vault)(RotationInspection,error) {
	if provider==nil { return RotationInspection{},ErrUnavailable }
	if !validWorkspace(workspace)||!validIdentity(id) { return RotationInspection{},ErrForbidden }
	ctx,cancel:=context.WithTimeout(ctx,3*time.Second)
	defer cancel()
	tx,err:=j.Catalog.beginAuthorized(ctx,actor,workspace,Manage)
	if err!=nil{return RotationInspection{},err}
	defer tx.Rollback(ctx)

	var state,handle string
	var fromVersion,targetVersion,expectedRevision,actualVersion,actualRevision int64
	err=tx.QueryRow(ctx,`SELECT j.state,j.secret_handle,j.from_version,j.target_version,j.expected_revision,
			p.secret_version,p.revision
		FROM connection_security_rotation_journal j
		JOIN connection_security_profiles p
		  ON p.id=j.connection_id AND p.workspace_id=j.workspace_id
		WHERE j.id::text=$1 AND j.workspace_id=$2 FOR SHARE OF j,p`,id,workspace).
		Scan(&state,&handle,&fromVersion,&targetVersion,&expectedRevision,&actualVersion,&actualRevision)
	if errors.Is(err,pgx.ErrNoRows){return RotationInspection{},ErrForbidden}
	if err!=nil{return RotationInspection{},ErrUnavailable}
	if (state!="prepared"&&state!="applied"&&state!="needs_review")||
		fromVersion<=0||targetVersion!=fromVersion+1||expectedRevision<1{
		return RotationInspection{},ErrUnavailable
	}
	ref,err:=NewSecretRef(workspace,handle,uint64(targetVersion))
	if err!=nil{return RotationInspection{},ErrUnavailable}
	secret,err:=provider.Read(ctx,ref)
	readable:=len(secret)>0
	// A provider may return a partial buffer along with an error.
	clear(secret)
	if err!=nil||!readable{return RotationInspection{},ErrUnavailable}
	prepared:=actualVersion==fromVersion&&actualRevision==expectedRevision
	applied:=actualVersion==targetVersion&&actualRevision==expectedRevision+1
	review:=state=="needs_review"||(state=="prepared"&&!prepared)||(state=="applied"&&!applied)
	// Successful credential-probe inspection is append-only audited, in the
	// same transaction as the locked MANAGE check. Failure to audit fails
	// the entire operation; neither profile nor journal state is mutated.
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit
		(workspace_id,actor_id,connection_id,action)
		SELECT workspace_id,$2,connection_id,'rotation_inspected'
		FROM connection_security_rotation_journal
		WHERE id::text=$1 AND workspace_id=$3`,id,actor,workspace);err!=nil{
		return RotationInspection{},ErrUnavailable
	}
	if err=tx.Commit(ctx);err!=nil{return RotationInspection{},ErrUnavailable}
	return RotationInspection{
		state:state,targetReadable:true,metadataPrepared:prepared,
		metadataApplied:applied,needsReview:review,
	},nil
}
