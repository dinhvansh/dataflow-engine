package connectionsecurity

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)
func signSyntheticRecovery(t *testing.T,r *RecoveryApproval,keys []ed25519.PrivateKey){
	t.Helper()
	message,err:=RecoverySigningMessage(*r)
	if err!=nil{t.Fatal(err)}
	r.Approvals=[]BootstrapSignature{
		{KeyID:"operator-a",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys[0],message))},
		{KeyID:"operator-b",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys[1],message))},
	}
}
func TestSignedRecoveryGuardrails(t *testing.T){
	t.Setenv(secureViewIsolatedFlag,"true")
	t.Setenv("DATAFLOW_INTEGRATION_PG_ISOLATED","true")
	manager,token,bootstrap,keys:=makeSyntheticBootstrapFixture(t)
	now:=time.Now().UTC()
	req:=RecoveryApproval{
		RequestID:"a141c547-740b-4bb0-a111-001000000022",
		ReviewRequestID:"b141c547-740b-4bb0-a111-001000000033",
		RotationID:"c141c547-740b-4bb0-a111-001000000044",
		Workspace:bootstrap.Workspace,ActorID:bootstrap.TargetActorID,
		Issuer:bootstrap.Issuer,Action:"confirm_already_applied",
		ExpectedRevision:2,TargetVersion:2,
		IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
	}
	signSyntheticRecovery(t,&req,keys)
	id,hash,err:=manager.verifyRecovery(token,req)
	if err!=nil||id!=req.ActorID||hash==([32]byte{}){t.Fatalf("valid recovery rejected %v",err)}
	for _,tc:=range []struct{name string;mutate func(*RecoveryApproval)}{
		{"wrong workspace",func(x *RecoveryApproval){x.Workspace="other-lab"}},
		{"wrong actor",func(x *RecoveryApproval){x.ActorID=strings.Repeat("a",64)}},
		{"wrong issuer",func(x *RecoveryApproval){x.Issuer="https://foreign.invalid"}},
		{"wrong rotation",func(x *RecoveryApproval){x.RotationID="c141c547-740b-4bb0-a111-001000000045"}},
		{"wrong review",func(x *RecoveryApproval){x.ReviewRequestID="b141c547-740b-4bb0-a111-001000000034"}},
		{"wrong revision",func(x *RecoveryApproval){x.ExpectedRevision=3}},
		{"wrong target",func(x *RecoveryApproval){x.TargetVersion=3}},
		{"wrong action",func(x *RecoveryApproval){x.Action="force_rollback"}},
		{"expired",func(x *RecoveryApproval){x.ExpiresAt=time.Now().Add(-time.Second).Unix()}},
		{"stale",func(x *RecoveryApproval){x.IssuedAt=time.Now().Add(-30*time.Minute).Unix()}},
		{"unsigned change",func(x *RecoveryApproval){x.RequestID="a141c547-740b-4bb0-a111-001000000023"}},
		{"duplicate signer",func(x *RecoveryApproval){x.Approvals[1]=x.Approvals[0]}},
		{"unknown signer",func(x *RecoveryApproval){x.Approvals[1].KeyID="unapproved"}},
	}{
		t.Run(tc.name,func(t *testing.T){
			copyReq:=req
			copyReq.Approvals=append([]BootstrapSignature(nil),req.Approvals...)
			tc.mutate(&copyReq)
			if _,_,err:=manager.verifyRecovery(token,copyReq);!errors.Is(err,ErrForbidden){
				t.Fatalf("unauthorized recovery accepted: %v",err)
			}
		})
	}
	if _,_,err=manager.verifyRecovery("bogus",req);!errors.Is(err,ErrForbidden){t.Fatal("JWT not verified")}
	raw,err:=json.Marshal(req)
	if err!=nil||string(raw)!=`"[REDACTED]"`||strings.Contains(req.GoString(),req.RotationID){
		t.Fatal("recovery payload leaked")
	}
	target:=RotationReviewRequest{
		RequestID:req.ReviewRequestID,RotationID:req.RotationID,
		Workspace:req.Workspace,ReviewerActorID:req.ActorID,
		Issuer:req.Issuer,Action:"acknowledge_needs_review",
		ExpectedRevision:req.ExpectedRevision,TargetVersion:req.TargetVersion,
		IssuedAt:req.IssuedAt,ExpiresAt:req.ExpiresAt,
	}
	reviewBytes,_:=RotationReviewSigningMessage(target)
	recoveryBytes,_:=RecoverySigningMessage(req)
	if string(reviewBytes)==string(recoveryBytes){t.Fatal("review signatures replayable for recovery")}
	manager.isolated=false
	if _,_,err=manager.verifyRecovery(token,req);!errors.Is(err,ErrUnavailable){t.Fatal("live runtime accepted synthetic recovery")}
	manager.isolated=true
	t.Setenv("DATAFLOW_INTEGRATION_PG_ISOLATED","false")
	if _,_,err=manager.verifyRecovery(token,req);!errors.Is(err,ErrUnavailable){t.Fatal("unsafe metadata accepted")}
	if _,err=manager.FinalizeAlreadyApplied(context.Background(),token,req,nil);!errors.Is(err,ErrUnavailable){t.Fatal("no vault accepted")}
}
