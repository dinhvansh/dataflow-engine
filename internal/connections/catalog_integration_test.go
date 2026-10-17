package connectionsecurity

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

// Runs exclusively in the ephemeral CI PostgreSQL fixture. It MUST NOT point
// at a production control plane or the ERP source.
func TestConnectionCatalogIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("requires isolated synthetic PostgreSQL")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {t.Fatal("refusing non-isolated PostgreSQL")}
 ctx,cancel:=context.WithTimeout(context.Background(),35*time.Second);defer cancel()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)}
 defer pg.Close()
 migration,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres","018_connection_security_metadata.sql"))
 if err!=nil{t.Fatal(err)}
 for i:=0;i<2;i++{
  if _,err=pg.Exec(ctx,string(migration));err!=nil{t.Fatalf("migration pass %d: %v",i+1,err)}
 }
 // Unique fixture per run. Never access another workspace's rows.
 slug:="lab"+time.Now().UTC().Format("150405")+"a"
 secondary:=slug+"b"
 for _,ws:=range []string{slug,secondary}{
  if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id)VALUES($1)`,ws);err!=nil{t.Fatal(err)}
 }
 for _,g:=range []struct{ws,actor,scope string}{
  {slug,"owner-ci","MANAGE"},
  {slug,"viewer-ci","VIEW"},
  {slug,"runner-ci","USE"},
  {secondary,"owner-elsewhere","MANAGE"},
  {secondary,"viewer-elsewhere","VIEW"},
 } {
  if _,err=pg.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)VALUES($1,$2,$3)`,g.ws,g.actor,g.scope);err!=nil{t.Fatal(err)}
 }
 store:=SecureCatalog{PG:pg}
 if err=store.RequireScope(ctx,"viewer-ci",slug,View);err!=nil{t.Fatal(err)}
 for _,deny:=range []struct{actor,ws string;scope Scope}{
  {"viewer-ci",slug,Use},{"runner-ci",slug,View},
  {"viewer-ci",secondary,View},{"owner-elsewhere",slug,Manage},
  {"unknown",slug,Manage},{"owner-ci","*",Manage},
 } {
  if err=store.RequireScope(ctx,deny.actor,deny.ws,deny.scope);!errors.Is(err,ErrForbidden){t.Fatalf("access allowed %+v: %v",deny,err)}
 }
 if _,err=store.CreateDraft(ctx,"viewer-ci",slug,"Not Allowed","sqlserver","source");!errors.Is(err,ErrForbidden){t.Fatalf("viewer can create: %v",err)}
 item,err:=store.CreateDraft(ctx,"owner-ci",slug,"CI SQL Source","sqlserver","source")
 if err!=nil{t.Fatal(err)}
 if item.Revision!=1 || item.Status!="draft" || item.SecretConfigured {t.Fatal("draft not inert",item)}
 if _,err=store.CreateDraft(ctx,"owner-ci",slug,"CI SQL Source","sqlserver","source");!errors.Is(err,ErrInvalid){t.Fatalf("duplicate allowed: %v",err)}
 if _,err=store.CreateDraft(ctx,"owner-ci",slug,"CI Source","sqlserver","destination");!errors.Is(err,ErrInvalid){t.Fatalf("illegal role allowed: %v",err)}
 if _,err=store.ListDrafts(ctx,"owner-ci",slug);!errors.Is(err,ErrForbidden){t.Fatalf("MANAGE implicitly VIEW: %v",err)}
 if _,err=store.ListDrafts(ctx,"viewer-ci",secondary);!errors.Is(err,ErrForbidden){t.Fatalf("workspace list crossed: %v",err)}
 list,err:=store.ListDrafts(ctx,"viewer-ci",slug)
 if err!=nil||len(list)!=1||list[0].ID!=item.ID{t.Fatalf("list failed: %+v %v",list,err)}
 ref,err:=NewSecretRef(slug,"ref/synthetic-credential",1);if err!=nil{t.Fatal(err)}
 if _,err=store.StageReference(ctx,"viewer-ci",slug,item.ID,ref,1);!errors.Is(err,ErrForbidden){t.Fatalf("viewer staged ref: %v",err)}
 if _,err=store.StageReference(ctx,"owner-elsewhere",secondary,item.ID,ref,1);!errors.Is(err,ErrInvalid){t.Fatalf("wrong workspace reference: %v",err)}
 staged,err:=store.StageReference(ctx,"owner-ci",slug,item.ID,ref,1)
 if err!=nil{t.Fatal(err)}
 if !staged.SecretConfigured || staged.Revision!=2 || staged.Status!="draft"{t.Fatal("stage incorrectly activated",staged)}
 if _,err=store.StageReference(ctx,"owner-ci",slug,item.ID,ref,1);!errors.Is(err,ErrForbidden){t.Fatalf("CAS replay succeeded: %v",err)}
 next,err:=NewSecretRef(slug,"ref/synthetic-credential",2);if err!=nil{t.Fatal(err)}
 rotated,err:=store.StageReference(ctx,"owner-ci",slug,item.ID,next,2);if err!=nil{t.Fatal(err)}
 if rotated.Revision!=3{t.Fatalf("expected third metadata revision: %+v",rotated)}
 swapped,err:=NewSecretRef(slug,"ref/another-credential",3);if err!=nil{t.Fatal(err)}
 if _,err=store.StageReference(ctx,"owner-ci",slug,item.ID,swapped,3);!errors.Is(err,ErrForbidden){t.Fatalf("reference handle changed: %v",err)}
 // A revocation is observed on the NEXT request, not a long-lived cache.
 if _,err=pg.Exec(ctx,`UPDATE connection_security_grants
 SET revoked_at=now() WHERE workspace_id=$1 AND actor_id='viewer-ci' AND scope='VIEW'`,slug);err!=nil{t.Fatal(err)}
 if _,err=store.ListDrafts(ctx,"viewer-ci",slug);!errors.Is(err,ErrForbidden){t.Fatalf("revoked actor listed: %v",err)}
 // Serialization MUST never include the opaque handle, source address, or password.
 encoded,err:=json.Marshal(rotated);if err!=nil{t.Fatal(err)}
 if strings.Contains(string(encoded),"synthetic-credential") || strings.Contains(string(encoded),"password") || strings.Contains(string(encoded),"dsn") {t.Fatalf("leak: %s",encoded)}
 var count int
 if err=pg.QueryRow(ctx,`SELECT count(*) FROM connection_security_audit
 WHERE workspace_id=$1 AND connection_id=$2::uuid`,slug,item.ID).Scan(&count);err!=nil{t.Fatal(err)}
 if count!=3 {t.Fatalf("missing atomic audit: %d",count)}
 if _,err=pg.Exec(ctx,`UPDATE connection_security_audit SET action='draft_created'
 WHERE workspace_id=$1`,slug);err==nil{t.Fatal("mutable audit allowed")}
 if _,err=pg.Exec(ctx,`DELETE FROM connection_security_audit WHERE workspace_id=$1`,slug);err==nil{t.Fatal("audit delete allowed")}
 if _,err=pg.Exec(ctx,`TRUNCATE connection_security_audit`);err==nil{t.Fatal("audit truncate allowed")}
 // This test intentionally leaves synthetic fixtures in its disposable CI DB:
 // append-only audit records must NOT be cleaned up via DELETE.
}
