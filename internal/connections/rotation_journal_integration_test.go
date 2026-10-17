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

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeJournalVault struct {
	versions map[uint64][]byte
	lastBytes []byte
	reads int
}
func (v *fakeJournalVault) Read(_ context.Context,ref SecretRef)([]byte,error){
	v.reads++
	value,ok:=v.versions[ref.Version()]
	if !ok{return nil,ErrUnavailable}
	v.lastBytes=append([]byte(nil),value...)
	return v.lastBytes,nil
}
func (v *fakeJournalVault) Rotate(context.Context,SecretRef,[]byte)(SecretRef,error){
	return SecretRef{},ErrUnavailable // Not called by the detached journal.
}
func TestRotationJournalIntegration(t *testing.T){
	dsn:=os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	if dsn==""{t.Skip("requires disposable CI PostgreSQL")}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")!="true"{t.Fatal("refusing non-isolated PostgreSQL")}
	ctx,cancel:=context.WithTimeout(context.Background(),55*time.Second)
	defer cancel()
	pg,err:=pgxpool.New(ctx,dsn);if err!=nil{t.Fatal(err)}
	defer pg.Close()
	for _,file:=range []string{"018_connection_security_metadata.sql","020_connection_rotation_journal.sql"}{
		src,err:=os.ReadFile(filepath.Join("..","..","migrations","postgres",file))
		if err!=nil{t.Fatal(err)}
		for i:=0;i<2;i++{
			if _,err=pg.Exec(ctx,string(src));err!=nil{t.Fatalf("migration %s pass %d: %v",file,i+1,err)}
		}
	}
	ws:=fmt.Sprintf("rotation%x",time.Now().UnixNano())
	other:=ws+"b"
	for _,w:=range []string{ws,other}{
		if _,err=pg.Exec(ctx,`INSERT INTO connection_security_workspaces(id) VALUES($1)`,w);err!=nil{t.Fatal(err)}
	}
	admin:=fixtureActorKey("synthetic-rotation-admin")
	for _,w:=range []string{ws,other}{
		if _,err=pg.Exec(ctx,`INSERT INTO connection_security_grants(workspace_id,actor_id,scope)
			VALUES($1,$2,'MANAGE')`,w,admin);err!=nil{t.Fatal(err)}
	}
	catalog:=SecureCatalog{PG:pg}
	profile,err:=catalog.CreateDraft(ctx,admin,ws,"Synthetic Rotation Destination","clickhouse","destination")
	if err!=nil{t.Fatal(err)}
	ref1,err:=NewSecretRef(ws,"ref/ci-rotation",1);if err!=nil{t.Fatal(err)}
	staged,err:=catalog.StageReference(ctx,admin,ws,profile.ID,ref1,1)
	if err!=nil{t.Fatal(err)}
	if staged.Revision!=2||!staged.SecretConfigured{t.Fatalf("bad seed: %+v",staged)}
	journal:=RotationJournal{Catalog:catalog}
	ticket,err:=journal.Prepare(ctx,admin,ws,profile.ID,staged.Revision)
	if err!=nil{t.Fatal(err)}
	if ticket.ID()==""{t.Fatal("journal id missing")}
	serialized,err:=json.Marshal(ticket);if err!=nil{t.Fatal(err)}
	for _,s:=range []string{ticket.handle,ws,profile.ID}{
		if strings.Contains(string(serialized),s)||strings.Contains(ticket.String(),s){t.Fatalf("secret journal data exposed: %s",serialized)}
	}
	if _,err=journal.Prepare(ctx,admin,ws,profile.ID,staged.Revision);!errors.Is(err,ErrForbidden){
		t.Fatalf("duplicate unfinished journal accepted: %v",err)
	}
	if _,err=journal.Prepare(ctx,admin,other,profile.ID,staged.Revision);!errors.Is(err,ErrForbidden){
		t.Fatalf("cross-workspace journal accepted: %v",err)
	}
	vault:=&fakeJournalVault{versions:map[uint64][]byte{}}
	if err=journal.Apply(ctx,admin,ws,ticket.ID(),vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("missing vault version moved metadata: %v",err)
	}
	var version,revision int64
	if err=pg.QueryRow(ctx,`SELECT secret_version,revision FROM connection_security_profiles
		WHERE id::text=$1`,profile.ID).Scan(&version,&revision);err!=nil{t.Fatal(err)}
	if version!=1||revision!=2{t.Fatalf("failed probe mutated metadata: %d/%d",version,revision)}
	vault.versions[2]=[]byte("synthetic-rotated-only-in-disposable-memory")
	if err=journal.Apply(ctx,admin,ws,ticket.ID(),vault);err!=nil{t.Fatal(err)}
	for _,b:=range vault.lastBytes{if b!=0{t.Fatal("credential bytes not zeroed")}}
	if err=journal.Apply(ctx,admin,ws,ticket.ID(),vault);err!=nil{t.Fatalf("replay not idempotent: %v",err)}
	if err=pg.QueryRow(ctx,`SELECT secret_version,revision FROM connection_security_profiles
		WHERE id::text=$1`,profile.ID).Scan(&version,&revision);err!=nil{t.Fatal(err)}
	if version!=2||revision!=3{t.Fatalf("metadata CAS not applied exactly once: %d/%d",version,revision)}
	var applied int
	if err=pg.QueryRow(ctx,`SELECT COUNT(*) FROM connection_security_rotation_events
		WHERE rotation_id=$1::uuid AND action='applied'`,ticket.ID()).Scan(&applied);err!=nil{t.Fatal(err)}
	if applied!=1{t.Fatal("duplicate applied audit",applied)}
	if err=journal.Apply(ctx,admin,other,ticket.ID(),vault);!errors.Is(err,ErrForbidden){
		t.Fatalf("cross workspace journal readable: %v",err)
	}

	// A revoked administrator cannot continue an already-prepared rotation.
	ticket2,err:=journal.Prepare(ctx,admin,ws,profile.ID,3);if err!=nil{t.Fatal(err)}
	vault.versions[3]=[]byte("synthetic-version-3")
	if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET revoked_at=now()
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE'`,ws,admin);err!=nil{t.Fatal(err)}
	if err=journal.Apply(ctx,admin,ws,ticket2.ID(),vault);!errors.Is(err,ErrForbidden){
		t.Fatalf("revoked admin can apply: %v",err)
	}
	if _,err=pg.Exec(ctx,`UPDATE connection_security_grants SET granted_at=now(),revoked_at=NULL
		WHERE workspace_id=$1 AND actor_id=$2 AND scope='MANAGE'`,ws,admin);err!=nil{t.Fatal(err)}
	if err=journal.Apply(ctx,admin,ws,ticket2.ID(),vault);err!=nil{t.Fatal(err)}

	// Inject a metadata change after the provider has written the new version.
	// It MUST NOT overwrite newer metadata; durable needs_review blocks attempts.
	ticket3,err:=journal.Prepare(ctx,admin,ws,profile.ID,4);if err!=nil{t.Fatal(err)}
	vault.versions[4]=[]byte("synthetic-version-4")
	if _,err=pg.Exec(ctx,`UPDATE connection_security_profiles SET revision=revision+1
		WHERE id::text=$1 AND workspace_id=$2`,profile.ID,ws);err!=nil{t.Fatal(err)}
	if err=journal.Apply(ctx,admin,ws,ticket3.ID(),vault);!errors.Is(err,ErrUnavailable){
		t.Fatalf("metadata CAS conflict succeeded: %v",err)
	}
	var state string
	if err=pg.QueryRow(ctx,`SELECT state FROM connection_security_rotation_journal
		WHERE id::text=$1`,ticket3.ID()).Scan(&state);err!=nil{t.Fatal(err)}
	if state!="needs_review"{t.Fatalf("CAS conflict not quarantined: %s",state)}
	if err=pg.QueryRow(ctx,`SELECT secret_version FROM connection_security_profiles
		WHERE id::text=$1`,profile.ID).Scan(&version);err!=nil{t.Fatal(err)}
	if version!=3{t.Fatalf("conflict unexpectedly changed version: %d",version)}
	if _,err=journal.Prepare(ctx,admin,ws,profile.ID,5);!errors.Is(err,ErrForbidden){
		t.Fatalf("unreconciled failed rotation unblocked: %v",err)
	}
	if _,err=pg.Exec(ctx,`DELETE FROM connection_security_rotation_events WHERE workspace_id=$1`,ws);err==nil{
		t.Fatal("rotation journal event audit removable")
	}
	// Fixture audit deliberately retained only in disposable CI PostgreSQL.
}
