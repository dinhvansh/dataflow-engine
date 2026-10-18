package connectionsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRotationReviewIntegration(t *testing.T) {
	dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn=="" {t.Skip("requires disposable control PostgreSQL")}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {t.Fatal("refusing nonisolated PostgreSQL")}
	t.Setenv(secureViewIsolatedFlag,"true")
	ctx,cancel:=context.WithTimeout(context.Background(),55*time.Second)
	defer cancel()
	pg,err:=pgxpool.New(ctx,dsn)
	if err!=nil{t.Fatal(err)}
	defer pg.Close()
	for _,file:=range []string{
		"018_connection_security_metadata.sql",
		"020_connection_rotation_journal.sql",
		"022_connection_rotation_inspection_audit.sql",
		"024_connection_rotation_reviews.sql",
	}{
		src,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",file))
		if err!=nil{t.Fatal(err)}
		for i:=0;i<2;i++{
			if _,err=pg.Exec(ctx,string(src));err!=nil{t.Fatalf("migration %s pass %d: %v",file,i+1,err)}
		}
	}
	workspace:=fmt.Sprintf("review%x",time.Now().UnixNano())
	other:=workspace+"b"
	for _,w:=range []string{workspace,other}{
		if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,w);err!=nil{t.Fatal(err)}
	}
	base,token,_,keys:=makeSyntheticBootstrapFixture(t)
	actor,err:=base.verifier.Verify(token)
	if err!=nil{t.Fatal(err)}
	if _,err=pg.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
		VALUES($1,$2,'MANAGE')`,workspace,actor.ActorID());err!=nil{t.Fatal(err)}
	catalog:=SecureCatalog{PG:pg}
	manager,err:=NewIsolatedManageBootstrap(catalog,base.verifier,base.operators,true)
	if err!=nil{t.Fatal(err)}
	profile,err:=catalog.CreateDraft(ctx,actor.ActorID(),workspace,"Test Review Target","clickhouse","destination")
	if err!=nil{t.Fatal(err)}
	ref,err:=NewSecretRef(workspace,"ref/review-test",1)
	if err!=nil{t.Fatal(err)}
	profile,err=catalog.StageReference(ctx,actor.ActorID(),workspace,profile.ID,ref,profile.Revision)
	if err!=nil{t.Fatal(err)}
	journal:=RotationJournal{Catalog:catalog}
	ticket,err:=journal.Prepare(ctx,actor.ActorID(),workspace,profile.ID,profile.Revision)
	if err!=nil{t.Fatal(err)}
	vault:=&fakeJournalVault{versions:map[uint64][]byte{
		2:[]byte("SYNTHETIC-DISPOSABLE-VAULT-BYTES"),
	}}
	// Generate a real CAS conflict. The journal must fail closed into NEEDS_REVIEW.
	if _,err=pg.Exec(ctx,`UPDATE connection_security_profiles SET revision=revision+1
		WHERE id::text=$1 AND workspace_id=$2`,profile.ID,workspace);err!=nil{t.Fatal(err)}
	if err=journal.Apply(ctx,actor.ActorID(),workspace,ticket.ID(),vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("conflicting metadata incorrectly rotated: %v",err)
	}
	req:=RotationReviewRequest{
		RequestID:uuid.NewString(),RotationID:ticket.ID(),Workspace:workspace,
		ReviewerActorID:actor.ActorID(),Issuer:"https://idp.example.invalid/ci",
		Action:"acknowledge_needs_review",
		ExpectedRevision:profile.Revision,TargetVersion:2,
		IssuedAt:time.Now().Add(-time.Minute).Unix(),
		ExpiresAt:time.Now().Add(8*time.Minute).Unix(),
	}
	signSyntheticRotationReview(t,&req,keys)
	before:=vault.reads
	bad:=req
	bad.Workspace=other
	if _,_,err=manager.AcknowledgeRotationReview(ctx,token,bad,vault);!errors.Is(err,ErrForbidden)||
		vault.reads!=before {t.Fatalf("foreign workspace got Vault read: %v",err)}
	bad=req
	bad.TargetVersion=3
	if _,_,err=manager.AcknowledgeRotationReview(ctx,token,bad,vault);!errors.Is(err,ErrForbidden)||
		vault.reads!=before{t.Fatalf("tampered version got Vault read: %v",err)}
	if _,_,err=manager.AcknowledgeRotationReview(ctx,token,req,nil);!errors.Is(err,ErrUnavailable){
		t.Fatalf("missing vault accepted: %v",err)
	}
	empty:=&fakeJournalVault{versions:map[uint64][]byte{}}
	if _,_,err=manager.AcknowledgeRotationReview(ctx,token,req,empty);!errors.Is(err,ErrUnavailable){
		t.Fatalf("unreachable version accepted: %v",err)
	}
	assessment,created,err:=manager.AcknowledgeRotationReview(ctx,token,req,vault)
	if err!=nil||!created||assessment.Classification()!="diverged"{
		t.Fatalf("review evidence failed: %v %v",created,err)
	}
	for _,b:=range vault.lastBytes{if b!=0{t.Fatal("Vault credential not cleared")}}
	encoded,err:=json.Marshal(assessment)
	if err!=nil||string(encoded)!=`"[REDACTED]"`{t.Fatal("review assessment disclosed internal state")}
	_,created,err=manager.AcknowledgeRotationReview(ctx,token,req,vault)
	if err!=nil||created{t.Fatalf("replayed review duplicated: %v",err)}
	var count int
	if err=pg.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_rotation_reviews
		WHERE rotation_id=$1::uuid AND workspace_id=$2`,ticket.ID(),workspace).Scan(&count);err!=nil||count!=1{
		t.Fatalf("review evidence replay count=%d %v",count,err)
	}
	var state string
	if err=pg.QueryRow(ctx,`SELECT state FROM connection_security_rotation_journal WHERE id::text=$1`,ticket.ID()).Scan(&state);err!=nil||state!="needs_review"{
		t.Fatalf("quarantine cleared by review: %s %v",state,err)
	}
	if _,err=journal.Prepare(ctx,actor.ActorID(),workspace,profile.ID,profile.Revision+1);!errors.Is(err,ErrForbidden){
		t.Fatalf("review unblocked new rotation: %v",err)
	}
	if err=pg.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_audit
		WHERE workspace_id=$1 AND connection_id=$2::uuid AND action='rotation_reviewed'`,
		workspace,profile.ID).Scan(&count);err!=nil||count!=1{t.Fatalf("audit count=%d %v",count,err)}
	if _,err=pg.Exec(ctx,`DELETE FROM connection_security_rotation_reviews WHERE workspace_id=$1`,workspace);err==nil{
		t.Fatal("immutable operator review deleted")
	}
	if _,err=pg.Exec(ctx,`UPDATE connection_security_rotation_reviews SET assessment='target'
		WHERE workspace_id=$1`,workspace);err==nil{t.Fatal("immutable review modified")}

	// Revocation checked BEFORE any Vault read, including accepted replay.
	if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
		WHERE actor_id=$1 AND workspace_id=$2 AND scope='MANAGE'`,actor.ActorID(),workspace);err!=nil{t.Fatal(err)}
	before=vault.reads
	if _,_,err=manager.AcknowledgeRotationReview(ctx,token,req,vault);!errors.Is(err,ErrForbidden)||
		vault.reads!=before{t.Fatalf("revoked actor accessed Vault: %v",err)}
	if strings.Contains(assessment.String(),ticket.ID()) {t.Fatal("debug string leaked journal ID")}
}
