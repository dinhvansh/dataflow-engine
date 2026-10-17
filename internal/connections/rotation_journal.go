package connectionsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RotationJournal coordinates ONLY detached inactive connection Draft metadata.
// The trusted caller must independently perform a KV-v2 CAS rotation (or create
// the replacement) after Prepare, and call Apply once the version exists.
// No HTTP route, secret input, job runtime or real Vault configuration is wired.
type RotationJournal struct {
	Catalog SecureCatalog
}

type RotationTicket struct {
	id string
	workspace string
	connectionID string
	handle string
	fromVersion int64
	nextVersion int64
	revision int64
}

func (t RotationTicket) ID() string { return t.id }
func (t RotationTicket) String() string { return "RotationTicket([REDACTED])" }
func (t RotationTicket) GoString() string { return "RotationTicket([REDACTED])" }
func (t RotationTicket) MarshalJSON() ([]byte,error) { return json.Marshal("[REDACTED]") }

func (j RotationJournal) Prepare(ctx context.Context, actor, workspace, connectionID string, revision int64) (RotationTicket,error) {
	if !validWorkspace(workspace) || !validIdentity(connectionID) || revision < 1 {
		return RotationTicket{}, ErrInvalid
	}
	tx,err:=j.Catalog.beginAuthorized(ctx,actor,workspace,Manage)
	if err!=nil { return RotationTicket{},err }
	defer tx.Rollback(ctx)
	t:=RotationTicket{workspace:workspace,connectionID:connectionID}
	err=tx.QueryRow(ctx,`SELECT secret_handle,secret_version,revision
		FROM connection_security_profiles
		WHERE id::text=$1 AND workspace_id=$2 AND revision=$3
		AND status='draft' AND secret_handle IS NOT NULL
		FOR UPDATE`,connectionID,workspace,revision).
		Scan(&t.handle,&t.fromVersion,&t.revision)
	if errors.Is(err,pgx.ErrNoRows){return RotationTicket{},ErrForbidden}
	if err!=nil{return RotationTicket{},ErrUnavailable}
	if t.fromVersion<=0||t.fromVersion>=math.MaxInt64{return RotationTicket{},ErrInvalid}
	t.nextVersion=t.fromVersion+1
	err=tx.QueryRow(ctx,`INSERT INTO connection_security_rotation_journal
		(workspace_id,connection_id,actor_id,secret_handle,from_version,target_version,expected_revision)
		VALUES($1,$2::uuid,$3,$4,$5,$6,$7) RETURNING id::text`,
		workspace,connectionID,actor,t.handle,t.fromVersion,t.nextVersion,revision).Scan(&t.id)
	if err!=nil {
		var pgErr *pgconn.PgError
		if errors.As(err,&pgErr)&&pgErr.Code=="23505"{return RotationTicket{},ErrForbidden}
		return RotationTicket{},ErrUnavailable
	}
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_rotation_events
		(rotation_id,workspace_id,actor_id,action) VALUES($1::uuid,$2,$3,'prepared')`,
		t.id,workspace,actor);err!=nil{return RotationTicket{},ErrUnavailable}
	if err=tx.Commit(ctx);err!=nil{return RotationTicket{},ErrUnavailable}
	return t,nil
}

func (j RotationJournal) lookup(ctx context.Context,workspace,id string)(RotationTicket,string,error){
	if !validWorkspace(workspace)||!validIdentity(id)||j.Catalog.PG==nil{
		return RotationTicket{},"",ErrInvalid
	}
	t:=RotationTicket{}
	var state string
	err:=j.Catalog.PG.QueryRow(ctx,`SELECT id::text,workspace_id,connection_id::text,secret_handle,
		from_version,target_version,expected_revision,state
		FROM connection_security_rotation_journal WHERE id::text=$1 AND workspace_id=$2`,
		id,workspace).Scan(&t.id,&t.workspace,&t.connectionID,&t.handle,&t.fromVersion,&t.nextVersion,&t.revision,&state)
	if errors.Is(err,pgx.ErrNoRows){return RotationTicket{},"",ErrForbidden}
	if err!=nil{return RotationTicket{},"",ErrUnavailable}
	return t,state,nil
}

// Apply probes a *version-pinned* vault entry before making any metadata
// change. A crash after Vault wrote the version leaves the PREPARED row and
// can be retried with Apply; absence/outage leaves it PREPARED, not invented.
// Returned credential bytes are always cleared; never exposed to HTTP callers.
func (j RotationJournal) Apply(ctx context.Context,actor,workspace,id string,provider Vault) error {
	if provider==nil{return ErrUnavailable}
	if err:=j.Catalog.RequireScope(ctx,actor,workspace,Manage);err!=nil{return err}
	t,state,err:=j.lookup(ctx,workspace,id)
	if err!=nil{return err}
	if state=="applied"{return nil}
	if state!="prepared"{return ErrUnavailable}
	ref,err:=NewSecretRef(t.workspace,t.handle,uint64(t.nextVersion))
	if err!=nil{return ErrInvalid}
	value,err:=provider.Read(ctx,ref)
	if err!=nil{return ErrUnavailable}
	if len(value)==0{clear(value);return ErrUnavailable}
	clear(value)

	// A fresh MANAGE check is row-locked for the full transaction. Never
	// trust the earlier check when permissions may have been revoked.
	tx,err:=j.Catalog.beginAuthorized(ctx,actor,workspace,Manage)
	if err!=nil{return err}
	defer tx.Rollback(ctx)
	var current string
	err=tx.QueryRow(ctx,`SELECT state FROM connection_security_rotation_journal
		WHERE id::text=$1 AND workspace_id=$2 FOR UPDATE`,id,workspace).Scan(&current)
	if errors.Is(err,pgx.ErrNoRows){return ErrForbidden}
	if err!=nil{return ErrUnavailable}
	if current=="applied"{return tx.Commit(ctx)}
	if current!="prepared"{return ErrUnavailable}
	var newRevision int64
	err=tx.QueryRow(ctx,`UPDATE connection_security_profiles
		SET secret_version=$5,revision=revision+1
		WHERE id::text=$1 AND workspace_id=$2 AND revision=$3
			AND secret_handle=$4 AND secret_version=$6 AND status='draft'
		RETURNING revision`,
		t.connectionID,workspace,t.revision,t.handle,t.nextVersion,t.fromVersion).Scan(&newRevision)
	if errors.Is(err,pgx.ErrNoRows){
		// A conflicting metadata change needs human reconciliation. Keep the
		// entry blocking new rotations to avoid silently losing Vault writes.
		if _,err=tx.Exec(ctx,`UPDATE connection_security_rotation_journal
			SET state='needs_review',finalized_at=now() WHERE id::text=$1`,id);err!=nil{return ErrUnavailable}
		if _,err=tx.Exec(ctx,`INSERT INTO connection_security_rotation_events
			(rotation_id,workspace_id,actor_id,action)
			VALUES($1::uuid,$2,$3,'needs_review')`,id,workspace,actor);err!=nil{return ErrUnavailable}
		if err=tx.Commit(ctx);err!=nil{return ErrUnavailable}
		return ErrUnavailable
	}
	if err!=nil{return ErrUnavailable}
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit
		(workspace_id,actor_id,connection_id,action)
		VALUES($1,$2,$3::uuid,'reference_rotated')`,workspace,actor,t.connectionID);err!=nil{return ErrUnavailable}
	if _,err=tx.Exec(ctx,`UPDATE connection_security_rotation_journal
		SET state='applied',finalized_at=now() WHERE id::text=$1`,id);err!=nil{return ErrUnavailable}
	if _,err=tx.Exec(ctx,`INSERT INTO connection_security_rotation_events
		(rotation_id,workspace_id,actor_id,action)
		VALUES($1::uuid,$2,$3,'applied')`,id,workspace,actor);err!=nil{return ErrUnavailable}
	if err=tx.Commit(ctx);err!=nil{return ErrUnavailable}
	return nil
}
