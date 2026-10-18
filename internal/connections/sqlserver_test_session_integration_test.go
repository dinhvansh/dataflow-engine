package connectionsecurity

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 _ "github.com/microsoft/go-mssqldb"
 "github.com/jackc/pgx/v5/pgxpool"
)

func TestScopedSQLServerSessionIntegration(t *testing.T){
 pgdsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 sqldsn:=os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN")
 if pgdsn==""||sqldsn==""{t.Skip("requires disposable PostgreSQL and SQL Server")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"||
  os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true" {
  t.Fatal("refusing test with non-isolated databases")
 }
 t.Setenv(secureViewIsolatedFlag,"true")
 ctx,cancel:=context.WithTimeout(context.Background(),70*time.Second);defer cancel()
 pg,err:=pgxpool.New(ctx,pgdsn);if err!=nil{t.Fatal(err)}
 defer pg.Close()
 for _,filename:=range []string{
  "018_connection_security_metadata.sql",
  "027_connection_test_audit.sql",
 }{
  sqlBytes,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",filename))
  if err!=nil{t.Fatal(err)}
  for i:=0;i<2;i++ {
   if _,err=pg.Exec(ctx,string(sqlBytes));err!=nil{t.Fatalf("migration %s replay %d: %v",filename,i,err)}
  }
 }
 db,err:=sql.Open("sqlserver",sqldsn)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 db.SetMaxOpenConns(2)
 workspace:=fmt.Sprintf("sqltest%x",time.Now().UnixNano())
 other:=workspace+"b"
 for _,w:=range []string{workspace,other}{
  if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,w);err!=nil{t.Fatal(err)}
 }
 base,jwt,_,_:=makeSyntheticBootstrapFixture(t)
 identity,err:=base.verifier.Verify(jwt);if err!=nil{t.Fatal(err)}
 actor:=identity.ActorID()
 for _,scope:=range []string{"MANAGE","TEST"}{
  if _,err=pg.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
   VALUES($1,$2,$3)`,workspace,actor,scope);err!=nil{t.Fatal(err)}
 }
 catalog:=SecureCatalog{PG:pg}
 profile,err:=catalog.CreateDraft(ctx,actor,workspace,"Test SQL QA Reader","sqlserver","source")
 if err!=nil{t.Fatal(err)}
 ref,err:=NewSecretRef(workspace,"ref/qa-test-credential",1);if err!=nil{t.Fatal(err)}
 profile,err=catalog.StageReference(ctx,actor,workspace,profile.ID,ref,profile.Revision)
 if err!=nil{t.Fatal(err)}
 svc:=ScopedSQLServerSessionTest{Catalog:catalog,Verifier:base.verifier,DB:db}
 result,err:=svc.Test(ctx,jwt,workspace,profile.ID)
 if err!=nil||!result.Passed{t.Fatalf("real QA SQL Server reader session rejected: %v",err)}
 if _,err=svc.Test(ctx,jwt,other,profile.ID);!errors.Is(err,ErrForbidden) {
  t.Fatalf("cross-workspace TEST accepted: %v",err)
 }
 if _,err=svc.Test(ctx,"bogus.invalid.jwt",workspace,profile.ID);!errors.Is(err,ErrForbidden) {
  t.Fatalf("unverified actor accepted: %v",err)
 }
 bad:=svc
 bad.DB=nil
 if _,err=bad.Test(ctx,jwt,workspace,profile.ID);!errors.Is(err,ErrUnavailable) {
  t.Fatalf("missing injected reader permitted: %v",err)
 }
 // Fake a disconnected trusted injected DB, not an unapproved network target.
 closed,err:=sql.Open("sqlserver",sqldsn);if err!=nil{t.Fatal(err)}
 if err=closed.Close();err!=nil{t.Fatal(err)}
 bad.DB=closed
 if _,err=bad.Test(ctx,jwt,workspace,profile.ID);!errors.Is(err,ErrUnavailable) {
  t.Fatalf("closed DB session passed: %v",err)
 }
 var requested,passed,failed int
 if err=pg.QueryRow(ctx,`SELECT
  count(*) FILTER (WHERE action='test_requested'),
  count(*) FILTER (WHERE action='test_passed'),
  count(*) FILTER (WHERE action='test_failed')
  FROM connection_security_audit
  WHERE connection_id=$1::uuid AND workspace_id=$2`,
  profile.ID,workspace).Scan(&requested,&passed,&failed);err!=nil{
  t.Fatal(err)
 }
 if requested!=2||passed!=1||failed!=1{
  t.Fatalf("requested/pass/fail audit mismatch %d/%d/%d",requested,passed,failed)
 }
 if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
 WHERE workspace_id=$1 AND actor_id=$2 AND scope='TEST'`,workspace,actor);err!=nil{t.Fatal(err)}
 if _,err=svc.Test(ctx,jwt,workspace,profile.ID);!errors.Is(err,ErrForbidden){
  t.Fatalf("revoked TEST remained usable: %v",err)
 }
 if err=pg.QueryRow(ctx,`SELECT count(*) FROM connection_security_audit
 WHERE connection_id=$1::uuid AND action='test_requested'`,profile.ID).Scan(&requested);err!=nil||requested!=2{
  t.Fatalf("revocation permitted a probe: %d %v",requested,err)
 }
 if _,err=pg.Exec(ctx,`DELETE FROM connection_security_audit
 WHERE connection_id=$1::uuid`,profile.ID);err==nil{t.Fatal("audit mutable")}
 if strings.Contains(fmt.Sprint(result),sqldsn){t.Fatal("connection secret leaked")}
 // The test never issues SQL Server INSERT/UPDATE/DELETE/DDL.
}
