package connectionsecurity

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func signSyntheticRotationReview(t *testing.T,r *RotationReviewRequest,keys []ed25519.PrivateKey) {
	t.Helper()
	message,err:=RotationReviewSigningMessage(*r)
	if err!=nil{t.Fatal(err)}
	r.Approvals=[]BootstrapSignature{
		{KeyID:"operator-a",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys[0],message))},
		{KeyID:"operator-b",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys[1],message))},
	}
}

func newSyntheticRotationReview(t *testing.T) (*ManageBootstrap,string,RotationReviewRequest,[]ed25519.PrivateKey) {
	t.Helper()
	t.Setenv(secureViewIsolatedFlag,"true")
	t.Setenv("DATAFLOW_INTEGRATION_PG_ISOLATED","true")
	b,token,bootstrap,keys:=makeSyntheticBootstrapFixture(t)
	now:=time.Now().UTC()
	req:=RotationReviewRequest{
		RequestID:"e141c547-740b-4bb0-a111-001000000011",
		RotationID:"f141c547-740b-4bb0-a111-001000000011",
		Workspace:bootstrap.Workspace,ReviewerActorID:bootstrap.TargetActorID,
		Issuer:bootstrap.Issuer,Action:"acknowledge_needs_review",
		ExpectedRevision:2,TargetVersion:2,
		IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
	}
	signSyntheticRotationReview(t,&req,keys)
	return b,token,req,keys
}

func TestRotationReviewSignedEnvelopeFailClosed(t *testing.T) {
	b,token,req,_:=newSyntheticRotationReview(t)
	id,hash,err:=b.verifyRotationReview(token,req)
	if err!=nil||id!=req.ReviewerActorID||hash==([32]byte{}){t.Fatalf("signed review rejected: %v",err)}
	serialized,err:=json.Marshal(req)
	if err!=nil||string(serialized)!=`"[REDACTED]"`{t.Fatal("signed review leaked")}
	if strings.Contains(req.GoString(),req.RotationID)||strings.Contains(req.String(),req.Workspace){t.Fatal("debug review leak")}
	for _,tc:=range []struct{name string;mutate func(*RotationReviewRequest)}{
		{"changed workspace",func(x *RotationReviewRequest){x.Workspace="lab-other"}},
		{"changed rotation",func(x *RotationReviewRequest){x.RotationID="f141c547-740b-4bb0-a111-001000000012"}},
		{"changed request",func(x *RotationReviewRequest){x.RequestID="e141c547-740b-4bb0-a111-001000000012"}},
		{"foreign actor",func(x *RotationReviewRequest){x.ReviewerActorID=strings.Repeat("a",64)}},
		{"foreign issuer",func(x *RotationReviewRequest){x.Issuer="https://other.invalid/idp"}},
		{"changed revision",func(x *RotationReviewRequest){x.ExpectedRevision=3}},
		{"changed target version",func(x *RotationReviewRequest){x.TargetVersion=3}},
		{"wrong action",func(x *RotationReviewRequest){x.Action="clear_needs_review"}},
		{"expired",func(x *RotationReviewRequest){x.ExpiresAt=time.Now().Add(-time.Second).Unix()}},
		{"stale",func(x *RotationReviewRequest){x.IssuedAt=time.Now().Add(-30*time.Minute).Unix()}},
		{"invalid signer",func(x *RotationReviewRequest){x.Approvals[1].KeyID="unknown"}},
		{"duplicated signer",func(x *RotationReviewRequest){x.Approvals[1]=x.Approvals[0]}},
		{"missing signature",func(x *RotationReviewRequest){x.Approvals=x.Approvals[:1]}},
	}{
		t.Run(tc.name,func(t *testing.T){
			copyReq:=req
			copyReq.Approvals=append([]BootstrapSignature(nil),req.Approvals...)
			tc.mutate(&copyReq)
			if _,_,err:=b.verifyRotationReview(token,copyReq);!errors.Is(err,ErrForbidden){
				t.Fatalf("unsafe review accepted: %v",err)
			}
		})
	}
	// Key material and signed payload domains differ from MANAGE bootstrap.
	bootstrap:=ManageBootstrapRequest{
		RequestID:req.RequestID,Workspace:req.Workspace,TargetActorID:req.ReviewerActorID,
		Issuer:req.Issuer,Action:req.Action,IssuedAt:req.IssuedAt,ExpiresAt:req.ExpiresAt,
	}
	a,_:=BootstrapSigningMessage(bootstrap)
	c,_:=RotationReviewSigningMessage(req)
	if string(a)==string(c){t.Fatal("bootstrap and rotation signatures share a domain")}
	t.Setenv(secureViewIsolatedFlag,"false")
	if _,_,err=b.verifyRotationReview(token,req);!errors.Is(err,ErrUnavailable){t.Fatal("non-isolated review accepted")}
}

func TestRotationReviewAssessmentRedaction(t *testing.T) {
	x:=RotationReviewAssessment{classification:"diverged"}
	raw,err:=json.Marshal(x)
	if err!=nil||string(raw)!=`"[REDACTED]"`{t.Fatal("metadata leak")}
	if strings.Contains(x.String(),"diverged") {t.Fatal("debug leak")}
}
