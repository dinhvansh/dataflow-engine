package connectionsecurity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// BindingPlans persists immutable, non-executable connection-plan snapshots.
// A PLAN is never a credential, live connection, Test Connection, or execution
// authorization. Runtime dispatch MUST separately check a future ACTIVE state,
// fresh USE rights and version before dispatching any real connector.
type BindingPlans struct{ Catalog SecureCatalog }

type BindingPlan struct {
	ID string `json:"id"`
	Workspace string `json:"workspace"`
	PipelineID string `json:"pipeline_id"`
	NodeID string `json:"node_id"`
	ProfileID string `json:"connection_id"`
	ProfileRevision int64 `json:"connection_revision"`
	Role string `json:"role"`
	Status string `json:"status"`
}

func (s BindingPlans) Create(ctx context.Context,actor,workspace,pipelineID,nodeID,profileID string,revision int64)(BindingPlan,error){
	if !actorKeyPattern.MatchString(actor)||!validWorkspace(workspace)||
		!safeBindingID.MatchString(pipelineID)||!safeBindingID.MatchString(nodeID)||
		!validIdentity(profileID)||revision<=0{
		return BindingPlan{},ErrInvalid
	}
	tx,err:=s.Catalog.beginAuthorized(ctx,actor,workspace,Use)
	if err!=nil{return BindingPlan{},err}
	defer tx.Rollback(ctx)
	var role,status string
	var actualRevision int64
	err=tx.QueryRow(ctx,`SELECT kind,status,revision
		FROM connection_security_profiles
		WHERE id::text=$1 AND workspace_id=$2 FOR SHARE`,
		profileID,workspace).Scan(&role,&status,&actualRevision)
	if errors.Is(err,pgx.ErrNoRows){return BindingPlan{},ErrForbidden}
	if err!=nil{return BindingPlan{},ErrUnavailable}
	if status!="draft"||actualRevision!=revision{return BindingPlan{},ErrForbidden}
	item:=BindingPlan{Workspace:workspace,PipelineID:pipelineID,NodeID:nodeID,
		ProfileID:profileID,ProfileRevision:revision,Role:role}
	err=tx.QueryRow(ctx,`INSERT INTO connection_security_binding_plans
		(workspace_id,pipeline_id,node_id,profile_id,profile_revision,role,actor_id)
		VALUES($1,$2,$3,$4::uuid,$5,$6,$7)
		RETURNING id::text,status`,
		workspace,pipelineID,nodeID,profileID,revision,role,actor).
		Scan(&item.ID,&item.Status)
	if err!=nil{
		var pgErr *pgconn.PgError
		if errors.As(err,&pgErr)&&pgErr.Code=="23505"{return BindingPlan{},ErrInvalid}
		return BindingPlan{},ErrUnavailable
	}
	if err=tx.Commit(ctx);err!=nil{return BindingPlan{},ErrUnavailable}
	return item,nil
}

// CheckPlannedCurrent exists for *synthetic QA* only. It checks that the
// planning snapshot still refers to the same inactive profile revision.
// It deliberately NEVER returns a live connection/credential or permission to
// execute a job. An ACTIVE binding/execution gate is a separate future feature.
func (s BindingPlans) CheckPlannedCurrent(ctx context.Context,actor,workspace,bindingID string)error{
	if !actorKeyPattern.MatchString(actor)||!validWorkspace(workspace)||!validIdentity(bindingID){
		return ErrForbidden
	}
	tx,err:=s.Catalog.beginAuthorized(ctx,actor,workspace,Use)
	if err!=nil{return err}
	defer tx.Rollback(ctx)
	var id string
	err=tx.QueryRow(ctx,`SELECT b.id::text
		FROM connection_security_binding_plans b
		JOIN connection_security_profiles p
			ON p.id=b.profile_id AND p.workspace_id=b.workspace_id
		WHERE b.id::text=$1 AND b.workspace_id=$2
			AND b.status='planned' AND p.status='draft'
			AND p.revision=b.profile_revision AND p.kind=b.role
		FOR SHARE OF b,p`,bindingID,workspace).Scan(&id)
	if errors.Is(err,pgx.ErrNoRows){return ErrForbidden}
	if err!=nil{return ErrUnavailable}
	if err=tx.Commit(ctx);err!=nil{return ErrUnavailable}
	return nil
}
