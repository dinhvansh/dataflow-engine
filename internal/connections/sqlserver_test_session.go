package connectionsecurity

import (
 "context"
 "database/sql"
 "errors"
 "os"
 "time"

 "github.com/jackc/pgx/v5"
)

// ScopedSQLServerSessionTest verifies a real SQL Server driver session against
// an operator-provisioned, disposable QA connection. It does NOT construct a
// DSN, connect to Epicor, accept credentials, expose an HTTP endpoint or
// activate a source connection. Production needs independently pinned network
// transport and real IdP/Vault/SQL permission approval before use.
type ScopedSQLServerSessionTest struct {
 Catalog SecureCatalog
 Verifier *RS256Verifier
 DB *sql.DB // trusted server-injected, pre-opened disposable reader only
}

type SQLServerTestResult struct { Passed bool `json:"passed"` }

// Test requires issuer-verified TEST in the requested workspace and a
// configured but INACTIVE SQL Server source draft. It holds the grant lock
// throughout fixed SELECT-only SQL metadata & permissions checks so a
// concurrent TEST revocation cannot race the query.
// Success and ordinary driver/permission failure are audited atomically in
// disposable control PostgreSQL; driver errors are always sanitized.
func (s ScopedSQLServerSessionTest) Test(ctx context.Context,jwt,workspace,profileID string)(SQLServerTestResult,error) {
 empty:=SQLServerTestResult{}
 if os.Getenv(secureViewIsolatedFlag)!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true"||
  s.Verifier==nil||s.Catalog.PG==nil||s.DB==nil{return empty,ErrUnavailable}
 person,err:=s.Verifier.Verify(jwt)
 if err!=nil||!person.Valid()||!validWorkspace(workspace)||!bootstrapRequestID.MatchString(profileID){
  return empty,ErrForbidden
 }
 ctx,cancel:=context.WithTimeout(ctx,5*time.Second)
 defer cancel()
 tx,err:=s.Catalog.beginAuthorized(ctx,person.ActorID(),workspace,Test)
 if err!=nil{return empty,err}
 defer tx.Rollback(ctx)
 var driver,kind,status string
 var handle sql.NullString
 var version int64
 err=tx.QueryRow(ctx,`SELECT driver,kind,status,secret_handle,secret_version
 FROM connection_security_profiles
 WHERE id::text=$1 AND workspace_id=$2 FOR SHARE`,profileID,workspace).
 Scan(&driver,&kind,&status,&handle,&version)
 if errors.Is(err,pgx.ErrNoRows){return empty,ErrForbidden}
 if err!=nil{return empty,ErrUnavailable}
 if driver!="sqlserver"||kind!="source"||status!="draft"||!handle.Valid||version<=0{
  return empty,ErrForbidden
 }
 if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit
 (workspace_id,actor_id,connection_id,action)
 VALUES($1,$2,$3::uuid,'test_requested')`,workspace,person.ActorID(),profileID);err!=nil{
  return empty,ErrUnavailable
 }
 // The caller must already have constructed a verified QA SQL driver
 // connection. Never return driver errors, database contents or credentials.
 passed:=checkDisposableSQLServerSession(ctx,s.DB)
 action:="test_failed"
 if passed{action="test_passed"}
 if _,err=tx.Exec(ctx,`INSERT INTO connection_security_audit
 (workspace_id,actor_id,connection_id,action)
 VALUES($1,$2,$3::uuid,$4)`,workspace,person.ActorID(),profileID,action);err!=nil{
  return empty,ErrUnavailable
 }
 if err=tx.Commit(ctx);err!=nil{return empty,ErrUnavailable}
 if !passed{return empty,ErrUnavailable}
 return SQLServerTestResult{Passed:true},nil
}

// Strictly fixed, read-only metadata and permission statements. This is
// purposely bound to the disposable fixture and its read-only SQL principal;
// it must never be reconfigured to probe production without security review.
func checkDisposableSQLServerSession(ctx context.Context,db *sql.DB)bool{
 if db==nil{return false}
 var database,login string
 var owner,writer sql.NullInt64
 err:=db.QueryRowContext(ctx,`SELECT DB_NAME(),ORIGINAL_LOGIN(),
 IS_ROLEMEMBER(N'db_owner'),IS_ROLEMEMBER(N'db_datawriter')`).
 Scan(&database,&login,&owner,&writer)
 if err!=nil||database!="DataFlowFixtureQA"||login!="qa_erp_reader"||
  !owner.Valid||owner.Int64!=0||!writer.Valid||writer.Int64!=0{return false}
 var selectPerm,insertPerm,updatePerm,deletePerm,alterPerm sql.NullInt64
 err=db.QueryRowContext(ctx,`SELECT
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'SELECT'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'INSERT'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'UPDATE'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'DELETE'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'ALTER')`).
 Scan(&selectPerm,&insertPerm,&updatePerm,&deletePerm,&alterPerm)
 return err==nil&&selectPerm.Valid&&selectPerm.Int64==1&&
  insertPerm.Valid&&insertPerm.Int64==0&&updatePerm.Valid&&updatePerm.Int64==0&&
  deletePerm.Valid&&deletePerm.Int64==0&&alterPerm.Valid&&alterPerm.Int64==0
}

// Avoid advertising Test Connection for a DB session unbound to an admitted
// connectionnet Plan, approved Vault credential reference and TLS identity.
var _ = time.Second
