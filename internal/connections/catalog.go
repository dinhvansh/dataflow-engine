package connectionsecurity

import (
 "context"
 "errors"
 "regexp"

 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgconn"
 "github.com/jackc/pgx/v5/pgxpool"
)

// SecureCatalog is an unexposed synthetic metadata store; it MUST be wired
// only after authenticating actor identities with a trusted server verifier.
// Existing V3.2A endpoints and execution remain untouched.
type SecureCatalog struct { PG *pgxpool.Pool }

var safeWorkspace = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,62}$`)
var safeDraftName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _.-]{0,79}$`)

type DraftProfile struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Workspace string `json:"workspace"`
 Driver string `json:"driver"`
 Kind string `json:"kind"`
 Status string `json:"status"`
 Revision int64 `json:"revision"`
 SecretConfigured bool `json:"secret_configured"`
}

func validWorkspace(workspace string) bool {return safeWorkspace.MatchString(workspace)}

// RequireScope is for server-verified actors ONLY. No actor/workspace/permission
// headers are accepted here, and no HTTP handler is wired to this catalog.
func (c SecureCatalog) RequireScope(ctx context.Context, actor, workspace string, scope Scope) error {
 if c.PG==nil {return ErrUnavailable}
 if !validIdentity(actor)||!validWorkspace(workspace)||!validScope(scope){return ErrForbidden}
 tx,err:=c.PG.BeginTx(ctx,pgx.TxOptions{AccessMode:pgx.ReadOnly})
 if err!=nil{return ErrUnavailable}
 defer tx.Rollback(ctx)
 if err:=requireScopeTx(ctx,tx,actor,workspace,scope);err!=nil{return err}
 if err:=tx.Commit(ctx);err!=nil{return ErrUnavailable}
 return nil
}

func requireScopeTx(ctx context.Context,tx pgx.Tx,actor,workspace string,scope Scope)error{
 var authorized bool
 err:=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM connection_security_grants
 WHERE workspace_id=$1 AND actor_id=$2 AND scope=$3 AND revoked_at IS NULL)`,
 workspace,actor,string(scope)).Scan(&authorized)
 if err!=nil{return ErrUnavailable}
 if !authorized{return ErrForbidden}
 return nil
}

func (c SecureCatalog) beginAuthorized(ctx context.Context,actor,workspace string,scope Scope)(pgx.Tx,error){
 if c.PG==nil{return nil,ErrUnavailable}
 if !validIdentity(actor)||!validWorkspace(workspace)||!validScope(scope){return nil,ErrForbidden}
 tx,err:=c.PG.Begin(ctx)
 if err!=nil{return nil,ErrUnavailable}
 // Read grant inside the SAME database transaction as subsequent mutation.
 // Grant revocation is serialized by taking a row lock.
 var granted bool
 err=tx.QueryRow(ctx,`SELECT true FROM connection_security_grants
 WHERE workspace_id=$1 AND actor_id=$2 AND scope=$3 AND revoked_at IS NULL
 FOR SHARE`,workspace,actor,string(scope)).Scan(&granted)
 if errors.Is(err,pgx.ErrNoRows){tx.Rollback(ctx);return nil,ErrForbidden}
 if err!=nil{tx.Rollback(ctx);return nil,ErrUnavailable}
 return tx,nil
}

func (c SecureCatalog) CreateDraft(ctx context.Context,actor,workspace,name,driver,kind string)(DraftProfile,error){
 if !safeDraftName.MatchString(name)||
  !((driver=="sqlserver"&&kind=="source")||(driver=="clickhouse"&&kind=="destination")){
  return DraftProfile{},ErrInvalid
 }
 tx,err:=c.beginAuthorized(ctx,actor,workspace,Manage)
 if err!=nil{return DraftProfile{},err}
 defer tx.Rollback(ctx)
 var d DraftProfile
 err=tx.QueryRow(ctx,`INSERT INTO connection_security_profiles(workspace_id,name,driver,kind)
 VALUES($1,$2,$3,$4)
 RETURNING id::text,name,workspace_id,driver,kind,status,revision`,
 workspace,name,driver,kind).
 Scan(&d.ID,&d.Name,&d.Workspace,&d.Driver,&d.Kind,&d.Status,&d.Revision)
 if err!=nil{
  var pgErr *pgconn.PgError
  if errors.As(err,&pgErr)&&pgErr.Code=="23505"{return DraftProfile{},ErrInvalid}
  return DraftProfile{},ErrUnavailable
 }
 if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit(workspace_id,actor_id,connection_id,action)
 VALUES($1,$2,$3::uuid,'draft_created')`,workspace,actor,d.ID);err!=nil{return DraftProfile{},ErrUnavailable}
 if err=tx.Commit(ctx);err!=nil{return DraftProfile{},ErrUnavailable}
 return d,nil
}

func (c SecureCatalog) ListDrafts(ctx context.Context,actor,workspace string)([]DraftProfile,error){
 // Keep VIEW grant row-locked in the same transaction as the list query:
 // concurrent revocation cannot race past the authorization gate.
 tx,err:=c.beginAuthorized(ctx,actor,workspace,View)
 if err!=nil{return nil,err}
 defer tx.Rollback(ctx)
 rows,err:=tx.Query(ctx,`SELECT id::text,name,workspace_id,driver,kind,status,revision,
 (secret_handle IS NOT NULL) FROM connection_security_profiles
 WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100`,workspace)
 if err!=nil{return nil,ErrUnavailable}
 defer rows.Close()
 out:=make([]DraftProfile,0)
 for rows.Next(){
  var d DraftProfile
  if err=rows.Scan(&d.ID,&d.Name,&d.Workspace,&d.Driver,&d.Kind,&d.Status,&d.Revision,&d.SecretConfigured);err!=nil{return nil,ErrUnavailable}
  out=append(out,d)
 }
 if rows.Err()!=nil{return nil,ErrUnavailable}
 rows.Close()
 if err=tx.Commit(ctx);err!=nil{return nil,ErrUnavailable}
 return out,nil
}

// StageReference only associates a trusted *opaque* vault pointer to an
// INACTIVE draft. There is NO connector login/credential transport or activation.
// The caller must first independently prove that the pointer is in its vault.
func (c SecureCatalog) StageReference(ctx context.Context,actor,workspace,id string,ref SecretRef,expectedRevision int64)(DraftProfile,error){
 if ref.WorkspaceID()!=workspace || ref.Version()==0 || ref.Version()>uint64(^uint64(0)>>1) || expectedRevision<1 {return DraftProfile{},ErrInvalid}
 tx,err:=c.beginAuthorized(ctx,actor,workspace,Manage)
 if err!=nil{return DraftProfile{},err}
 defer tx.Rollback(ctx)
 var d DraftProfile
 err=tx.QueryRow(ctx,`UPDATE connection_security_profiles
 SET secret_handle=$4,secret_version=$5,revision=revision+1
 WHERE id::text=$1 AND workspace_id=$2 AND revision=$3 AND status='draft'
   AND (
     (secret_handle IS NULL AND $5::bigint=1)
     OR (secret_handle=$4 AND secret_version=$5::bigint-1)
   )
 RETURNING id::text,name,workspace_id,driver,kind,status,revision,(secret_handle IS NOT NULL)`,
 id,workspace,expectedRevision,ref.OpaqueHandle(),int64(ref.Version())).
 Scan(&d.ID,&d.Name,&d.Workspace,&d.Driver,&d.Kind,&d.Status,&d.Revision,&d.SecretConfigured)
 if errors.Is(err,pgx.ErrNoRows){return DraftProfile{},ErrForbidden}
 if err!=nil{return DraftProfile{},ErrUnavailable}
 action:="reference_staged"
 if ref.Version()>1 {action="reference_rotated"}
 if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit(workspace_id,actor_id,connection_id,action)
 VALUES($1,$2,$3::uuid,$4)`,workspace,actor,id,action);err!=nil{return DraftProfile{},ErrUnavailable}
 if err=tx.Commit(ctx);err!=nil{return DraftProfile{},ErrUnavailable}
 return d,nil
}
