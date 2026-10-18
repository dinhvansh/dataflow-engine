package connectionsecurity

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)
func TestCompensationPlanIntegration(t *testing.T){
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn==""{t.Skip("disposable PostgreSQL only")}
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated database")}
 t.Setenv(secureViewIsolatedFlag,"true")
 ctx,cancel:=context.WithTimeout(context.Background(),55*time.Second);defer cancel()
 pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)}
 defer pg.Close()
 for _,p:=range []string{
  "018_connection_security_metadata.sql",
  "020_connection_rotation_journal.sql",
  "022_connection_rotation_inspection_audit.sql",
  "024_connection_rotation_reviews.sql",
  "025_connection_rotation_safe_recovery.sql",
  "026_connection_compensation_plans.sql",
 }{
  data,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",p))
  if err!=nil{t.Fatal(err)}
  for i:=0;i<2;i++ {
   if _,err=pg.Exec(ctx,string(data));err!=nil{t.Fatalf("migration %s replay %d: %v",p,i,err)}
  }
 }
 ws:=fmt.Sprintf("comp%x",time.Now().UnixNano())
 if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,ws);err!=nil{t.Fatal(err)}
 base,jwt,_,keys:=makeSyntheticBootstrapFixture(t)
 verified,err:=base.verifier.Verify(jwt);if err!=nil{t.Fatal(err)}
 actor:=verified.ActorID()
 if _,err=pg.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
 VALUES($1,$2,'MANAGE')`,ws,actor);err!=nil{t.Fatal(err)}
 catalog:=SecureCatalog{PG:pg}
 svc,err:=NewIsolatedManageBootstrap(catalog,base.verifier,base.operators,true)
 if err!=nil{t.Fatal(err)}
 profile,err:=catalog.CreateDraft(ctx,actor,ws,"Synthetic Divergence","clickhouse","destination")
 if err!=nil{t.Fatal(err)}
 ref,err:=NewSecretRef(ws,"ref/compensation-test",1)
 if err!=nil{t.Fatal(err)}
 profile,err=catalog.StageReference(ctx,actor,ws,profile.ID,ref,profile.Revision)
 if err!=nil{t.Fatal(err)}
 journal:=RotationJournal{Catalog:catalog}
 ticket,err:=journal.Prepare(ctx,actor,ws,profile.ID,profile.Revision)
 if err!=nil{t.Fatal(err)}
 vault:=&syntheticLatestVault{
  fakeJournalVault:&fakeJournalVault{versions:map[uint64][]byte{2:[]byte("SYNTHETIC-TARGET-SECRET")}},
  current:2,
 }
 if _,err=pg.Exec(ctx,`UPDATE connection_security_profiles SET revision=revision+1
 WHERE id::text=$1 AND workspace_id=$2`,profile.ID,ws);err!=nil{t.Fatal(err)}
 if err=journal.Apply(ctx,actor,ws,ticket.ID(),vault);!errors.Is(err,ErrUnavailable){
  t.Fatalf("expected quarantined CAS conflict: %v",err)
 }
 now:=time.Now().UTC()
 review:=RotationReviewRequest{
  RequestID:uuid.NewString(),RotationID:ticket.ID(),Workspace:ws,ReviewerActorID:actor,
  Issuer:"https://idp.example.invalid/ci",Action:"acknowledge_needs_review",
  ExpectedRevision:profile.Revision,TargetVersion:2,
  IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
 }
 signSyntheticRotationReview(t,&review,keys)
 assessment,created,err:=svc.AcknowledgeRotationReview(ctx,jwt,review,vault)
 if err!=nil||!created||assessment.Classification()!="diverged"{t.Fatalf("missing prior review: %v",err)}
 req:=CompensationPlanRequest{
  RequestID:uuid.NewString(),ReviewRequestID:review.RequestID,RotationID:ticket.ID(),
  Workspace:ws,ActorID:actor,Issuer:review.Issuer,Action:"escalate_manual_reconciliation",
  ExpectedJournalRevision:profile.Revision,ExpectedProfileRevision:profile.Revision+1,
  ExpectedProfileVersion:1,TargetVersion:2,ObservedVaultVersion:2,
  IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
 }
 signCompensation(t,&req,keys)
 // Detect stale Vault current metadata before persisting anything.
 vault.current=3
 if _,err=svc.PlanDivergentCompensation(ctx,jwt,req,vault);!errors.Is(err,ErrUnavailable){
  t.Fatalf("stale Vault evidence accepted: %v",err)
 }
 vault.current=2
 vault.movedAfterRead=true;vault.probes=0
 if _,err=svc.PlanDivergentCompensation(ctx,jwt,req,vault);!errors.Is(err,ErrUnavailable){
  t.Fatalf("race in Vault metadata accepted: %v",err)
 }
 vault.movedAfterRead=false;vault.probes=0
 changed,err:=svc.PlanDivergentCompensation(ctx,jwt,req,vault)
 if err!=nil||!changed{t.Fatalf("valid signed escalation rejected: %v",err)}
 for _,v:=range vault.lastBytes{if v!=0{t.Fatal("Vault credential buffer not cleared")}}
 changed,err=svc.PlanDivergentCompensation(ctx,jwt,req,vault)
 if err!=nil||changed{t.Fatalf("duplicate accepted plan: %v",err)}
 var state string
 if err=pg.QueryRow(ctx,`SELECT state FROM connection_security_rotation_journal
 WHERE id::text=$1`,ticket.ID()).Scan(&state);err!=nil||state!="needs_review"{
  t.Fatalf("compensation bypassed quarantine: %v %s",err,state)
 }
 var ver,rev int64
 if err=pg.QueryRow(ctx,`SELECT secret_version,revision FROM connection_security_profiles
 WHERE id::text=$1`,profile.ID).Scan(&ver,&rev);err!=nil||ver!=1||rev!=3{
  t.Fatalf("escalation changed profile: %v %d %d",err,ver,rev)
 }
 if _,err=journal.Prepare(ctx,actor,ws,profile.ID,rev);!errors.Is(err,ErrForbidden){
  t.Fatalf("escalation unblocked a new rotation: %v",err)
 }
 var n int
 if err=pg.QueryRow(ctx,`SELECT count(*) FROM connection_security_compensation_plans
 WHERE rotation_id=$1::uuid`,ticket.ID()).Scan(&n);err!=nil||n!=1{
  t.Fatalf("immutable plan not unique: %v %d",err,n)
 }
 if err=pg.QueryRow(ctx,`SELECT count(*) FROM connection_security_audit
 WHERE connection_id=$1::uuid AND action='compensation_planned'`,profile.ID).Scan(&n);err!=nil||n!=1{
  t.Fatalf("compensation audit missing: %v %d",err,n)
 }
 if _,err=pg.Exec(ctx,`UPDATE connection_security_compensation_plans SET plan='other'
 WHERE rotation_id=$1::uuid`,ticket.ID());err==nil{t.Fatal("immutable plan mutated")}
 if _,err=pg.Exec(ctx,`DELETE FROM connection_security_compensation_plans WHERE rotation_id=$1::uuid`,
 ticket.ID());err==nil{t.Fatal("immutable plan deleted")}
 // Permission revocation is checked even for old idempotent approvals.
 if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
 WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE'`,ws,actor);err!=nil{t.Fatal(err)}
 reads:=vault.reads
 if _,err=svc.PlanDivergentCompensation(ctx,jwt,req,vault);!errors.Is(err,ErrForbidden)||vault.reads!=reads{
  t.Fatalf("revoked manager reached provider: %v",err)
 }
}
