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
type syntheticLatestVault struct {
	*fakeJournalVault
	current uint64
	probes int
	movedAfterRead bool
	unavailable bool
}
func(v *syntheticLatestVault) LatestVersion(_ context.Context,ref SecretRef)(uint64,error){
	v.probes++
	if v.unavailable{return 0,ErrUnavailable}
	if v.movedAfterRead&&v.probes%2==0{return v.current+1,nil}
	return v.current,nil
}
func TestRotationRecoveryIntegration(t *testing.T) {
	dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn==""{t.Skip("requires disposable PostgreSQL")}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated PostgreSQL")}
	t.Setenv(secureViewIsolatedFlag,"true")
	ctx,cancel:=context.WithTimeout(context.Background(),55*time.Second)
	defer cancel()
	pool,err:=pgxpool.New(ctx,dsn)
	if err!=nil{t.Fatal(err)}
	defer pool.Close()
	for _,name:=range []string{
		"018_connection_security_metadata.sql",
		"020_connection_rotation_journal.sql",
		"022_connection_rotation_inspection_audit.sql",
		"024_connection_rotation_reviews.sql",
		"025_connection_rotation_safe_recovery.sql",
	}{
		data,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name))
		if err!=nil{t.Fatal(err)}
		for i:=0;i<2;i++ {
			if _,err=pool.Exec(ctx,string(data));err!=nil{t.Fatalf("%s run %d: %v",name,i+1,err)}
		}
	}
	ws:=fmt.Sprintf("recover%x",time.Now().UnixNano())
	other:=ws+"b"
	for _,x:=range []string{ws,other} {
		if _,err=pool.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,x);err!=nil{t.Fatal(err)}
	}
	base,jwt,_,keys:=makeSyntheticBootstrapFixture(t)
	actor,err:=base.verifier.Verify(jwt)
	if err!=nil{t.Fatal(err)}
	if _,err=pool.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
		VALUES($1,$2,'MANAGE')`,ws,actor.ActorID());err!=nil{t.Fatal(err)}
	catalog:=SecureCatalog{PG:pool}
	manager,err:=NewIsolatedManageBootstrap(catalog,base.verifier,base.operators,true)
	if err!=nil{t.Fatal(err)}
	profile,err:=catalog.CreateDraft(ctx,actor.ActorID(),ws,"QA Confirmed-Applied","clickhouse","destination")
	if err!=nil{t.Fatal(err)}
	ref1,err:=NewSecretRef(ws,"ref/recover-test",1)
	if err!=nil{t.Fatal(err)}
	profile,err=catalog.StageReference(ctx,actor.ActorID(),ws,profile.ID,ref1,1)
	if err!=nil{t.Fatal(err)}
	journal:=RotationJournal{Catalog:catalog}
	ticket,err:=journal.Prepare(ctx,actor.ActorID(),ws,profile.ID,profile.Revision)
	if err!=nil{t.Fatal(err)}
	vault:=&syntheticLatestVault{
		fakeJournalVault:&fakeJournalVault{versions:map[uint64][]byte{2:[]byte("DISPOSABLE-ONLY-SECRET-V2")}},
		current:2,
	}
	// Force CAS conflict and durable quarantine in disposable PG.
	if _,err=pool.Exec(ctx,`UPDATE connection_security_profiles SET revision=revision+1
		WHERE id::text=$1 AND workspace_id=$2`,profile.ID,ws);err!=nil{t.Fatal(err)}
	if err=journal.Apply(ctx,actor.ActorID(),ws,ticket.ID(),vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("expected needs_review conflict: %v",err)
	}
	now:=time.Now().UTC()
	review:=RotationReviewRequest{
		RequestID:uuid.NewString(),RotationID:ticket.ID(),Workspace:ws,
		ReviewerActorID:actor.ActorID(),Issuer:"https://idp.example.invalid/ci",
		Action:"acknowledge_needs_review",ExpectedRevision:profile.Revision,TargetVersion:2,
		IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
	}
	signSyntheticRotationReview(t,&review,keys)
	assessment,created,err:=manager.AcknowledgeRotationReview(ctx,jwt,review,vault)
	if err!=nil||!created||assessment.Classification()!="diverged"{t.Fatalf("expected diverged review: %v",err)}
	approval:=RecoveryApproval{
		RequestID:uuid.NewString(),ReviewRequestID:review.RequestID,
		RotationID:ticket.ID(),Workspace:ws,ActorID:actor.ActorID(),
		Issuer:review.Issuer,Action:"confirm_already_applied",
		ExpectedRevision:profile.Revision,TargetVersion:2,
		IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
	}
	signSyntheticRecovery(t,&approval,keys)
	before:=vault.probes
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault);!errors.Is(err,ErrForbidden)||vault.probes!=before {
		t.Fatalf("divergent metadata was finalized: %v",err)
	}
	// Only the synthetic fixture independently brings profile metadata to
	// target version at the ORIGINAL intended revision. Recovery may never do it.
	if _,err=pool.Exec(ctx,`UPDATE connection_security_profiles SET secret_version=2
		WHERE id::text=$1 AND workspace_id=$2 AND revision=$3`,
		profile.ID,ws,profile.Revision+1);err!=nil{t.Fatal(err)}
	// Old 'diverged' review is NOT enough even after synthetic metadata repair.
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault);!errors.Is(err,ErrForbidden) {
		t.Fatalf("old divergent signed review cleared quarantine: %v",err)
	}
	review2:=review
	review2.RequestID=uuid.NewString()
	signSyntheticRotationReview(t,&review2,keys)
	assessment,created,err=manager.AcknowledgeRotationReview(ctx,jwt,review2,vault)
	if err!=nil||!created||assessment.Classification()!="target" {t.Fatalf("review target missing: %v",err)}
	approval.ReviewRequestID=review2.RequestID
	signSyntheticRecovery(t,&approval,keys)

	// No cross-tenant read, wrong version or stale Vault newest pointer.
	foreign:=approval
	foreign.Workspace=other
	priorProbes:=vault.probes
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,foreign,vault);!errors.Is(err,ErrForbidden)||vault.probes!=priorProbes{
		t.Fatalf("cross tenant probe: %v",err)
	}
	vault.current=3
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("stale external Vault target accepted: %v",err)
	}
	vault.current=2
	vault.unavailable=true
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("unavailable Vault metadata accepted: %v",err)
	}
	vault.unavailable=false
	vault.movedAfterRead=true
	vault.probes=0
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("Vault changed between probes but accepted: %v",err)
	}
	vault.movedAfterRead=false
	vault.probes=0
	changed,err:=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault)
	if err!=nil||!changed{t.Fatalf("consistent reviewed target not finalized: %v",err)}
	for _,b:=range vault.lastBytes{if b!=0{t.Fatal("credential memory not zeroed")}}
	var state string
	if err=pool.QueryRow(ctx,`SELECT state FROM connection_security_rotation_journal
		WHERE id::text=$1`,ticket.ID()).Scan(&state);err!=nil||state!="applied"{
		t.Fatalf("not finalized safely: %s %v",state,err)
	}
	var version,revision int64
	if err=pool.QueryRow(ctx,`SELECT secret_version,revision FROM connection_security_profiles
		WHERE id::text=$1`,profile.ID).Scan(&version,&revision);err!=nil||version!=2||revision!=3{
		t.Fatalf("recovery touched profile: %d %d %v",version,revision,err)
	}
	changed,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault)
	if err!=nil||changed{t.Fatalf("identical approval replay duplicated: %v",err)}
	var count int
	if err=pool.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_rotation_recoveries
		WHERE rotation_id=$1::uuid AND workspace_id=$2`,ticket.ID(),ws).Scan(&count);err!=nil||count!=1{
		t.Fatalf("recovery audit duplicated: %d %v",count,err)
	}
	if err=pool.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_rotation_events
		WHERE rotation_id=$1::uuid AND action='reconciled'`,ticket.ID()).Scan(&count);err!=nil||count!=1{
		t.Fatalf("transition evidence incorrect %d %v",count,err)
	}
	if err=pool.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_audit
		WHERE connection_id=$1::uuid AND action='rotation_reconciled'`,profile.ID).Scan(&count);err!=nil||count!=1{
		t.Fatalf("immutable audit missing %d %v",count,err)
	}
	if _,err=pool.Exec(ctx,`DELETE FROM connection_security_rotation_recoveries WHERE workspace_id=$1`,ws);err==nil{
		t.Fatal("recovery history mutable")
	}
	// Subsequent new rotation may be prepared; no live credential activated.
	newTicket,err:=journal.Prepare(ctx,actor.ActorID(),ws,profile.ID,revision)
	if err!=nil||newTicket.ID()==""{t.Fatalf("finalized journal still blocks new intent: %v",err)}
	// Revoking MANAGE blocks even an idempotent approval replay.
	if _,err=pool.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE'`,ws,actor.ActorID());err!=nil{t.Fatal(err)}
	readBefore:=vault.reads
	if _,err=manager.FinalizeAlreadyApplied(ctx,jwt,approval,vault);!errors.Is(err,ErrForbidden)||vault.reads!=readBefore{
		t.Fatalf("revoked manager replayed recovery or accessed Vault: %v",err)
	}
}
