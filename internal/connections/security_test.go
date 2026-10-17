package connectionsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fakeVault struct {
	reads      int
	rotations  int
	bytes      []byte
	next       SecretRef
}
func (v *fakeVault) Read(_ context.Context, _ SecretRef) ([]byte, error) {
	v.reads++
	v.bytes = []byte("SYNTHETIC-PASSWORD-NOT-A-REAL-SECRET")
	return v.bytes, nil
}
func (v *fakeVault) Rotate(_ context.Context, _ SecretRef, _ []byte) (SecretRef, error) {
	v.rotations++
	return v.next, nil
}
type fakeAudit struct {
	events []AuditEvent
	fail bool
}
func (a *fakeAudit) Append(_ context.Context, event AuditEvent) error {
	if a.fail { return errors.New("audit offline") }
	a.events = append(a.events, event)
	return nil
}

func fixture(t *testing.T) (Service, Connection, *fakeVault, *fakeAudit) {
	t.Helper()
	p, err := NewPolicy([]Grant{
		{ActorID:"viewer",WorkspaceID:"lab-a",Scopes:[]Scope{View}},
		{ActorID:"runner",WorkspaceID:"lab-a",Scopes:[]Scope{Use}},
		{ActorID:"tester",WorkspaceID:"lab-a",Scopes:[]Scope{Test}},
		{ActorID:"manager",WorkspaceID:"lab-a",Scopes:[]Scope{Manage}},
		{ActorID:"manager",WorkspaceID:"lab-b",Scopes:[]Scope{View}},
	})
	if err != nil {t.Fatal(err)}
	ref,err := NewSecretRef("lab-a","ref/synthetic-handle",1)
	if err != nil {t.Fatal(err)}
	next,err := NewSecretRef("lab-a","ref/synthetic-handle",2)
	if err != nil {t.Fatal(err)}
	vault:=&fakeVault{next:next}
	audit:=&fakeAudit{}
	conn:=Connection{ID:"connection-a",WorkspaceID:"lab-a",Secret:ref}
	return Service{Policy:p,Vault:vault,Audit:audit},conn,vault,audit
}

func TestLeastPrivilegeScopesAndWorkspaceIsolation(t *testing.T) {
	s,_,_,_:=fixture(t)
	ctx:=context.Background()
	for _,tc:=range []struct{
		actor,ws string
		scope Scope
		ok bool
	}{
		{"viewer","lab-a",View,true},
		{"viewer","lab-a",Use,false},
		{"runner","lab-a",Use,true},
		{"runner","lab-a",Test,false},
		{"tester","lab-a",Test,true},
		{"tester","lab-a",Use,false},
		{"manager","lab-a",Manage,true},
		{"manager","lab-a",View,false},
		{"manager","lab-b",View,true},
		{"manager","lab-b",Manage,false},
		{"manager","lab-c",Manage,false},
		{"stranger","lab-a",View,false},
		{"","lab-a",View,false},
		{"viewer","",View,false},
		{"viewer","*",View,false},
		{"viewer","lab-a",Scope("OWNER"),false},
	} {
		err:=s.Policy.Authorize(ctx,tc.actor,tc.ws,tc.scope)
		if (err==nil)!=tc.ok {t.Errorf("%s %s %s: got err %v, allowed=%v",tc.actor,tc.ws,tc.scope,err,tc.ok)}
		if !tc.ok && !errors.Is(err,ErrForbidden){t.Errorf("not fail closed: %v",err)}
	}
	var absent *Policy
	if err:=absent.Authorize(ctx,"viewer","lab-a",View);!errors.Is(err,ErrForbidden){t.Fatal("nil policy granted access")}
	cancelled,stop:=context.WithCancel(ctx)
	stop()
	if err:=s.Policy.Authorize(cancelled,"viewer","lab-a",View);!errors.Is(err,context.Canceled){t.Fatal("cancellation ignored")}
}

func TestPoliciesRejectMalformedGrantsAndCopyInput(t *testing.T) {
	for _,grant:=range []Grant{
		{ActorID:"",WorkspaceID:"lab-a",Scopes:[]Scope{Use}},
		{ActorID:"actor",WorkspaceID:"",Scopes:[]Scope{Use}},
		{ActorID:"*",WorkspaceID:"lab-a",Scopes:[]Scope{Use}},
		{ActorID:"actor",WorkspaceID:"*",Scopes:[]Scope{Use}},
		{ActorID:"actor",WorkspaceID:"lab-a",Scopes:[]Scope{}},
		{ActorID:"actor",WorkspaceID:"lab-a",Scopes:[]Scope{"ADMIN"}},
		{ActorID:"actor\n",WorkspaceID:"lab-a",Scopes:[]Scope{Use}},
	} {
		if _,err:=NewPolicy([]Grant{grant});!errors.Is(err,ErrInvalid){t.Errorf("accepted invalid grant %+v",grant)}
	}
	grants:=[]Grant{{ActorID:"actor",WorkspaceID:"lab-a",Scopes:[]Scope{View}}}
	p,err:=NewPolicy(grants);if err!=nil{t.Fatal(err)}
	grants[0].Scopes[0]=Manage
	grants[0].WorkspaceID="lab-b"
	if err:=p.Authorize(context.Background(),"actor","lab-a",View);err!=nil{t.Fatal("input modified policy")}
	if err:=p.Authorize(context.Background(),"actor","lab-b",Manage);err==nil{t.Fatal("mutated input expanded grant")}
}

func TestOpaqueSecretReferencesAndResponseRedaction(t *testing.T){
	for _,tc:=range []struct{workspace,handle string;version uint64}{
		{"lab-a","",1},
		{"lab-a","https://internal.host",1},
		{"lab-a","server:1433",1},
		{"lab-a","ref/synthetic",0},
		{"","ref/synthetic",1},
		{"*","ref/synthetic",1},
		{"lab-a","ref//invalid",1},
	}{
		if _,err:=NewSecretRef(tc.workspace,tc.handle,tc.version);!errors.Is(err,ErrInvalid){t.Errorf("accepted unsafe reference %+v",tc)}
	}
	_,conn,_,_:=fixture(t)
	if conn.Secret.WorkspaceID()!="lab-a" || conn.Secret.Version()!=1 {t.Fatal("reference metadata mismatch")}
	if fmt.Sprintf("%v",conn.Secret)!="[REDACTED]" {t.Fatal("Stringer exposed ref")}
	raw,err:=json.Marshal(conn);if err!=nil{t.Fatal(err)}
	public,err:=json.Marshal(conn.Redacted());if err!=nil{t.Fatal(err)}
	for _,out:=range []string{string(raw),string(public)} {
		for _,forbidden:=range []string{"ref/synthetic-handle","SYNTHETIC-PASSWORD","opaqueID","version","password","dsn","hostname"}{
			if strings.Contains(strings.ToLower(out),strings.ToLower(forbidden)){t.Errorf("serialized secret metadata %s: %s",forbidden,out)}
		}
	}
	if !strings.Contains(string(public),`"secret_configured":true`){t.Fatal("no bounded configured state")}
}

func TestVaultDeniedBeforeAccessAndRequiresAudit(t *testing.T){
	s,conn,vault,audit:=fixture(t)
	ctx:=context.Background()
	called:=false
	consume:=func(_ []byte)error{called=true;return nil}
	for _,tc:=range []struct{actor string;scope Scope}{
		{"viewer",Use},{"manager",Use},{"runner",Test},{"tester",Use},{"stranger",Use},{"runner",Manage},
	}{
		if err:=s.WithSecret(ctx,tc.actor,conn,tc.scope,consume);err==nil{t.Fatalf("granted %+v",tc)}
	}
	connOther:=conn;connOther.WorkspaceID="lab-b"
	if err:=s.WithSecret(ctx,"runner",connOther,Use,consume);!errors.Is(err,ErrInvalid){t.Fatalf("cross-workspace reference accepted: %v",err)}
	if called || vault.reads!=0 || len(audit.events)!=0{t.Fatal("denied access leaked to vault, callback or audit")}
	s.Audit=nil
	if err:=s.WithSecret(ctx,"runner",conn,Use,consume);!errors.Is(err,ErrUnavailable){t.Fatal("missing audit failed open")}
	s.Audit=audit;audit.fail=true
	if err:=s.WithSecret(ctx,"runner",conn,Use,consume);!errors.Is(err,ErrUnavailable){t.Fatal("failed audit failed open")}
	if vault.reads!=0 {t.Fatal("read before durable audit")}
	s.Vault=nil
	if err:=s.WithSecret(ctx,"runner",conn,Use,consume);!errors.Is(err,ErrUnavailable){t.Fatal("missing vault failed open")}
}

func TestAuthorizedVaultUseTestAndMemoryZeroization(t *testing.T) {
	s,conn,vault,audit:=fixture(t)
	for _,tc:=range []struct{actor string;scope Scope}{{"runner",Use},{"tester",Test}}{
		err:=s.WithSecret(context.Background(),tc.actor,conn,tc.scope,func(secret []byte)error {
			if string(secret)!="SYNTHETIC-PASSWORD-NOT-A-REAL-SECRET"{t.Fatal("synthetic callback input mismatch")}
			return nil
		})
		if err!=nil{t.Fatal(err)}
		for _,b:=range vault.bytes{if b!=0{t.Fatal("secret not zeroed after callback")}}
	}
	if vault.reads!=2 || len(audit.events)!=2 {t.Fatal("authorized calls not audited")}
	for _,event:=range audit.events {
		if event.Action!="credential_access_requested"||event.WorkspaceID!="lab-a"||event.ConnectionID!="connection-a"||event.OccurredAt.IsZero(){t.Fatal("invalid audit event")}
	}
	safe:=fmt.Sprintf("%+v",audit.events)
	if strings.Contains(safe,"PASSWORD")||strings.Contains(safe,"synthetic-handle"){t.Fatal("audit emitted a secret")}
}

func TestRotationRequiresManageAndRejectsNonMonotonicOrTenantLeak(t *testing.T){
	s,conn,vault,audit:=fixture(t)
	original:=conn
	if _,err:=s.RotateSecret(context.Background(),"runner",conn,[]byte("new-test-only"));!errors.Is(err,ErrForbidden){t.Fatalf("nonmanager could rotate: %v",err)}
	if vault.rotations!=0||len(audit.events)!=0{t.Fatal("denied rotation had side effects")}
	next,err:=s.RotateSecret(context.Background(),"manager",conn,[]byte("new-test-only"))
	if err!=nil{t.Fatal(err)}
	if next.Secret.Version()!=2 || original.Secret.Version()!=1{t.Fatal("not immutable/cas-like version change")}
	if len(audit.events)!=1 || audit.events[0].Action!="credential_rotation_requested"{t.Fatal("rotation intent not audited")}
	if _,err:=s.RotateSecret(context.Background(),"manager",conn,nil);!errors.Is(err,ErrUnavailable){t.Fatal("accepted empty secret")}
	if vault.rotations!=1{t.Fatal("rotation called on invalid input")}
	bad,err:=NewSecretRef("lab-b","ref/synthetic-handle",3);if err!=nil{t.Fatal(err)}
	vault.next=bad
	if _,err:=s.RotateSecret(context.Background(),"manager",conn,[]byte("synthetic"));!errors.Is(err,ErrUnavailable){t.Fatal("vault changed tenant silently")}
	bad,err=NewSecretRef("lab-a","ref/synthetic-handle",1);if err!=nil{t.Fatal(err)}
	vault.next=bad
	if _,err:=s.RotateSecret(context.Background(),"manager",conn,[]byte("synthetic"));!errors.Is(err,ErrUnavailable){t.Fatal("vault returned stale revision")}
	bad,err=NewSecretRef("lab-a","ref/different-handle",4);if err!=nil{t.Fatal(err)}
	vault.next=bad
	if _,err:=s.RotateSecret(context.Background(),"manager",conn,[]byte("synthetic"));!errors.Is(err,ErrUnavailable){t.Fatal("vault switched handle")}
	audit.fail=true
	if _,err:=s.RotateSecret(context.Background(),"manager",conn,[]byte("synthetic"));!errors.Is(err,ErrUnavailable){t.Fatal("audit failure did not block rotation")}
	if vault.rotations!=4{t.Fatalf("audit fail unexpectedly rotated, count=%d",vault.rotations)}
}
