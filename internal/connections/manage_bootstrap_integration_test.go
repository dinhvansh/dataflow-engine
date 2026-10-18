package connectionsecurity

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestManageBootstrapIntegration(t *testing.T) {
	dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn=="" {t.Skip("requires disposable PostgreSQL integration fixture")}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true" {
		t.Fatal("refusing security bootstrap against non-isolated PostgreSQL")
	}
	t.Setenv(secureViewIsolatedFlag,"true")
	ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second)
	defer cancel()
	pg,err:=pgxpool.New(ctx,dsn)
	if err!=nil{t.Fatal(err)}
	defer pg.Close()
	for _,filename:=range []string{
		"018_connection_security_metadata.sql",
		"023_connection_manage_bootstrap.sql",
	}{
		raw,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",filename))
		if err!=nil{t.Fatal(err)}
		for n:=0;n<2;n++{
			if _,err=pg.Exec(ctx,string(raw));err!=nil{
				t.Fatalf("migration %s replay %d: %v",filename,n,err)
			}
		}
	}
	ws:=fmt.Sprintf("boot%x",time.Now().UnixNano())
	other:=ws+"b"
	for _,name:=range []string{ws,other} {
		if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,name);err!=nil{t.Fatal(err)}
	}
	base,token,req,keys:=makeSyntheticBootstrapFixture(t)
	req.RequestID=uuid.NewString()
	req.Workspace=ws
	signSyntheticBootstrap(t,&req,keys)
	svc,err:=NewIsolatedManageBootstrap(SecureCatalog{PG:pg},base.verifier,base.operators,true)
	if err!=nil{t.Fatal(err)}
	if _,err=NewIsolatedManageBootstrap(SecureCatalog{PG:pg},base.verifier,base.operators,false);!errors.Is(err,ErrUnavailable){
		t.Fatalf("nonisolated DB bootstrap accepted: %v",err)
	}
	duplicate:=map[string]ed25519.PublicKey{
		"two-names-a":keys[0].Public().(ed25519.PublicKey),
		"two-names-b":keys[0].Public().(ed25519.PublicKey),
	}
	if _,err=NewIsolatedManageBootstrap(SecureCatalog{PG:pg},base.verifier,duplicate,true);!errors.Is(err,ErrInvalid){
		t.Fatalf("same signer masquerading as two accepted: %v",err)
	}
	catalog:=SecureCatalog{PG:pg}
	if err=catalog.RequireScope(ctx,req.TargetActorID,ws,Manage);!errors.Is(err,ErrForbidden){
		t.Fatalf("unbootstrapped actor had MANAGE: %v",err)
	}
	changed,err:=svc.Apply(ctx,token,req)
	if err!=nil||!changed{t.Fatalf("first bootstrap failed: %v",err)}
	if err=catalog.RequireScope(ctx,req.TargetActorID,ws,Manage);err!=nil{
		t.Fatalf("signed MANAGE missing: %v",err)
	}
	if err=catalog.RequireScope(ctx,req.TargetActorID,other,Manage);!errors.Is(err,ErrForbidden){
		t.Fatalf("grant leaked across workspace: %v",err)
	}
	changed,err=svc.Apply(ctx,token,req)
	if err!=nil||changed{t.Fatalf("identical idempotent replay failed: %v",err)}
	var count int
	if err=pg.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_manage_bootstrap
		WHERE workspace_id=$1 AND target_actor_id=$2`,ws,req.TargetActorID).Scan(&count);err!=nil||count!=1{
		t.Fatalf("unexpected duplicated bootstrap evidence %d: %v",count,err)
	}
	// Approved MANAGE may delegate only VIEW/USE/TEST using existing locked
	// grant lifecycle, never MANAGE itself.
	target:=fixtureActorKey("separate-verified-recipient")
	grantAdmin:=GrantAdmin{Catalog:catalog}
	if changed,err=grantAdmin.SetScope(ctx,req.TargetActorID,ws,target,View,true);err!=nil||!changed{
		t.Fatalf("bootstrap admin cannot delegate VIEW: %v",err)
	}
	if _,err=grantAdmin.SetScope(ctx,req.TargetActorID,ws,target,Manage,true);!errors.Is(err,ErrForbidden){
		t.Fatalf("bootstrap enabled MANAGE delegation: %v",err)
	}

	tampered:=req
	tampered.Workspace=other
	if _,err=svc.Apply(ctx,token,tampered);!errors.Is(err,ErrForbidden){
		t.Fatalf("workspace-spoofed request accepted: %v",err)
	}
	tampered=req
	tampered.RequestID=uuid.NewString()
	if _,err=svc.Apply(ctx,token,tampered);!errors.Is(err,ErrForbidden){
		t.Fatalf("unsigned replay with new request ID accepted: %v",err)
	}
	if _,err=pg.Exec(ctx,`UPDATE connection_security_manage_bootstrap SET signer_a='forged'
		WHERE workspace_id=$1`,ws);err==nil {
		t.Fatal("append-only bootstrap audit modified")
	}
	if _,err=pg.Exec(ctx,`DELETE FROM connection_security_manage_bootstrap WHERE workspace_id=$1`,ws);err==nil{
		t.Fatal("append-only bootstrap audit deleted")
	}

	// A later authorized revocation must not be undone by a replay of an old
	// signed request (even while its token and approvals are still valid).
	if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE'`,ws,req.TargetActorID);err!=nil{
		t.Fatal(err)
	}
	if _,err=svc.Apply(ctx,token,req);!errors.Is(err,ErrForbidden){
		t.Fatalf("revoked MANAGE silently restored by old approval: %v",err)
	}
	if err=catalog.RequireScope(ctx,req.TargetActorID,ws,Manage);!errors.Is(err,ErrForbidden){
		t.Fatalf("replay restored revoked MANAGE: %v",err)
	}

	// A NEW two-operator decision, with new request ID and new signatures, is
	// mandatory before a revoked MANAGE grant may be restored.
	req2:=req
	req2.RequestID=uuid.NewString()
	signSyntheticBootstrap(t,&req2,keys)
	changed,err=svc.Apply(ctx,token,req2)
	if err!=nil||!changed{t.Fatalf("fresh approval cannot re-enroll MANAGE: %v",err)}
	if err=catalog.RequireScope(ctx,req.TargetActorID,ws,Manage);err!=nil{t.Fatalf("fresh grant missing: %v",err)}
	if err=pg.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_manage_bootstrap
		WHERE workspace_id=$1 AND target_actor_id=$2`,ws,req.TargetActorID).Scan(&count);err!=nil||count!=2{
		t.Fatalf("expected two signed approval records, got %d: %v",count,err)
	}
	// No actual IdP/Vault, Epicor or external network is accessed.
}
