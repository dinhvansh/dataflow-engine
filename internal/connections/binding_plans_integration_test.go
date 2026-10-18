package connectionsecurity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestImmutableBindingPlanIntegration(t *testing.T){
	dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn==""{t.Skip("disposable PostgreSQL only")}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated database")}
	ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second)
	defer cancel()
	pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)}
	defer pg.Close()
	for _,name:=range []string{"018_connection_security_metadata.sql","021_connection_binding_plans.sql"}{
		sql,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",name));if err!=nil{t.Fatal(err)}
		for i:=0;i<2;i++{if _,err=pg.Exec(ctx,string(sql));err!=nil{t.Fatalf("migration %s pass %d: %v",name,i,err)}}
	}
	ws:=fmt.Sprintf("binding%x",time.Now().UnixNano())
	other:=ws+"b"
	for _,v:=range []string{ws,other}{
		if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,v);err!=nil{t.Fatal(err)}
	}
	admin:=fixtureActorKey("synthetic-binding-manager")
	user:=fixtureActorKey("synthetic-binding-user")
	for _,seed:=range []struct{ws,actor,scope string}{
		{ws,admin,"MANAGE"},{ws,user,"USE"},{other,user,"USE"},
	}{
		if _,err=pg.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
			VALUES($1,$2,$3)`,seed.ws,seed.actor,seed.scope);err!=nil{t.Fatal(err)}
	}
	catalog:=SecureCatalog{PG:pg}
	profile,err:=catalog.CreateDraft(ctx,admin,ws,"Synthetic Binder Source","sqlserver","source")
	if err!=nil{t.Fatal(err)}
	bindings:=BindingPlans{Catalog:catalog}
	binding,err:=bindings.Create(ctx,user,ws,"pipeline1","sourceNode",profile.ID,1)
	if err!=nil{t.Fatal(err)}
	if binding.ProfileRevision!=1||binding.Role!="source"||binding.Status!="planned"{
		t.Fatalf("binding activated or misbound: %+v",binding)
	}
	if err=bindings.CheckPlannedCurrent(ctx,user,ws,binding.ID);err!=nil{t.Fatal(err)}
	if _,err=bindings.Create(ctx,user,ws,"pipeline1","sourceNode",profile.ID,1);!errors.Is(err,ErrInvalid){
		t.Fatalf("duplicate node binding accepted: %v",err)
	}
	if _,err=bindings.Create(ctx,user,other,"pipeline2","sourceNode",profile.ID,1);!errors.Is(err,ErrForbidden){
		t.Fatalf("cross-workspace profile bound: %v",err)
	}
	if err=bindings.CheckPlannedCurrent(ctx,user,other,binding.ID);!errors.Is(err,ErrForbidden){
		t.Fatalf("cross-workspace snapshot visible: %v",err)
	}
	if _,err=bindings.Create(ctx,user,ws,"pipeline3","sourceNode",profile.ID,2);!errors.Is(err,ErrForbidden){
		t.Fatalf("stale profile revision bound: %v",err)
	}
	if _,err=bindings.Create(ctx,admin,ws,"pipeline4","sourceNode",profile.ID,1);!errors.Is(err,ErrForbidden){
		t.Fatalf("MANAGE implicitly granted USE: %v",err)
	}
	if _,err=bindings.Create(ctx,user,ws,"pipeline_X;DROP TABLE", "node",profile.ID,1);!errors.Is(err,ErrInvalid){
		t.Fatalf("SQL-injected pipeline id accepted: %v",err)
	}
	// Database-level tenant isolation: composite FK must reject mismatched
	// profile workspace even from incorrectly written application SQL.
	if _,err=pg.Exec(ctx,`INSERT INTO connection_security_binding_plans
		(workspace_id,pipeline_id,node_id,profile_id,profile_revision,role,actor_id)
		VALUES($1,'directAttempt','nodeA',$2::uuid,1,'source',$3)`,
		other,profile.ID,user);err==nil{t.Fatal("database composite FK allowed cross-tenant reference")}
	// Immutable binding stays frozen; subsequent profile revision update must
	// deny use without rewriting the historical snapshot.
	if _,err=pg.Exec(ctx,`UPDATE connection_security_profiles SET revision=revision+1
		WHERE id::text=$1 AND workspace_id=$2`,profile.ID,ws);err!=nil{t.Fatal(err)}
	if err=bindings.CheckPlannedCurrent(ctx,user,ws,binding.ID);!errors.Is(err,ErrForbidden){
		t.Fatalf("stale frozen binding accepted: %v",err)
	}
	var storedRevision int64
	if err=pg.QueryRow(ctx,`SELECT profile_revision FROM connection_security_binding_plans
		WHERE id::text=$1`,binding.ID).Scan(&storedRevision);err!=nil{t.Fatal(err)}
	if storedRevision!=1{t.Fatal("binding snapshot revision changed")}
	if _,err=pg.Exec(ctx,`UPDATE connection_security_binding_plans SET profile_revision=2 WHERE id::text=$1`,binding.ID);err==nil{
		t.Fatal("immutable binding was updated")
	}
	if _,err=pg.Exec(ctx,`DELETE FROM connection_security_binding_plans WHERE id::text=$1`,binding.ID);err==nil{
		t.Fatal("immutable binding was deleted")
	}
	if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='USE'`,ws,user);err!=nil{t.Fatal(err)}
	if err=bindings.CheckPlannedCurrent(ctx,user,ws,binding.ID);!errors.Is(err,ErrForbidden){
		t.Fatalf("revoked actor still holds binding: %v",err)
	}
	// Audit evidence remains only inside disposable PostgreSQL.
}
