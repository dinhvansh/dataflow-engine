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

func signCompensation(t *testing.T,r *CompensationPlanRequest,keys []ed25519.PrivateKey){
 t.Helper()
 msg,err:=CompensationSigningMessage(*r)
 if err!=nil{t.Fatal(err)}
 r.Approvals=[]BootstrapSignature{
  {KeyID:"operator-a",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys[0],msg))},
  {KeyID:"operator-b",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys[1],msg))},
 }
}
func TestCompensationEnvelopeDenials(t *testing.T){
 t.Setenv(secureViewIsolatedFlag,"true")
 t.Setenv("DATAFLOW_INTEGRATION_PG_ISOLATED","true")
 m,jwt,bs,keys:=makeSyntheticBootstrapFixture(t)
 now:=time.Now().UTC()
 req:=CompensationPlanRequest{
  RequestID:"a141c547-740b-4bb0-a111-001000000060",
  ReviewRequestID:"b141c547-740b-4bb0-a111-001000000060",
  RotationID:"c141c547-740b-4bb0-a111-001000000060",
  Workspace:bs.Workspace,ActorID:bs.TargetActorID,Issuer:bs.Issuer,
  Action:"escalate_manual_reconciliation",ExpectedJournalRevision:2,
  ExpectedProfileRevision:3,ExpectedProfileVersion:1,TargetVersion:2,
  ObservedVaultVersion:2,IssuedAt:now.Add(-time.Minute).Unix(),
  ExpiresAt:now.Add(8*time.Minute).Unix(),
 }
 signCompensation(t,&req,keys)
 id,digest,err:=m.verifyCompensation(jwt,req)
 if err!=nil||id!=req.ActorID||digest==([32]byte{}){t.Fatalf("safe plan rejected: %v",err)}
 for _,tc:=range []struct{name string;mut func(*CompensationPlanRequest)}{
  {"workspace",func(x *CompensationPlanRequest){x.Workspace="elsewhere"}},
  {"identity",func(x *CompensationPlanRequest){x.ActorID=strings.Repeat("a",64)}},
  {"issuer",func(x *CompensationPlanRequest){x.Issuer="https://untrusted.invalid"}},
  {"rotation",func(x *CompensationPlanRequest){x.RotationID="c141c547-740b-4bb0-a111-001000000061"}},
  {"review",func(x *CompensationPlanRequest){x.ReviewRequestID="b141c547-740b-4bb0-a111-001000000061"}},
  {"profile revision",func(x *CompensationPlanRequest){x.ExpectedProfileRevision++}},
  {"profile version",func(x *CompensationPlanRequest){x.ExpectedProfileVersion++}},
  {"journal revision",func(x *CompensationPlanRequest){x.ExpectedJournalRevision++}},
  {"vault observation",func(x *CompensationPlanRequest){x.ObservedVaultVersion++}},
  {"target",func(x *CompensationPlanRequest){x.TargetVersion++}},
  {"unapproved action",func(x *CompensationPlanRequest){x.Action="overwrite_vault"}},
  {"expired",func(x *CompensationPlanRequest){x.ExpiresAt=now.Add(-time.Second).Unix()}},
  {"stale",func(x *CompensationPlanRequest){x.IssuedAt=now.Add(-time.Hour).Unix()}},
  {"same signer",func(x *CompensationPlanRequest){x.Approvals[1]=x.Approvals[0]}},
  {"invalid signer",func(x *CompensationPlanRequest){x.Approvals[1].KeyID="unknown"}},
  {"missing signer",func(x *CompensationPlanRequest){x.Approvals=x.Approvals[:1]}},
 }{
  t.Run(tc.name,func(t *testing.T){
   bad:=req
   bad.Approvals=append([]BootstrapSignature(nil),req.Approvals...)
   tc.mut(&bad)
   if _,_,err:=m.verifyCompensation(jwt,bad);!errors.Is(err,ErrForbidden){
    t.Fatalf("unsafe plan accepted: %v",err)
   }
  })
 }
 prior:=RecoveryApproval{RequestID:req.RequestID,ReviewRequestID:req.ReviewRequestID,
  RotationID:req.RotationID,Workspace:req.Workspace,ActorID:req.ActorID,
  Issuer:req.Issuer,Action:"confirm_already_applied",TargetVersion:req.TargetVersion,
  ExpectedRevision:req.ExpectedJournalRevision,IssuedAt:req.IssuedAt,ExpiresAt:req.ExpiresAt}
 a,_:=RecoverySigningMessage(prior)
 c,_:=CompensationSigningMessage(req)
 if string(a)==string(c){t.Fatal("compensation signature reused from recovery")}
 raw,err:=json.Marshal(req)
 if err!=nil||string(raw)!=`"[REDACTED]"`||strings.Contains(req.String(),req.Workspace){
  t.Fatal("secret-related approval identifiers leaked")
 }
 m.isolated=false
 if _,_,err=m.verifyCompensation(jwt,req);!errors.Is(err,ErrUnavailable){t.Fatal("live plan enabled")}
}
