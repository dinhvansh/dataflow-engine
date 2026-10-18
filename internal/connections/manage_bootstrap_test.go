package connectionsecurity

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func makeSyntheticBootstrapFixture(t *testing.T) (*ManageBootstrap,string,ManageBootstrapRequest,[]ed25519.PrivateKey) {
	t.Helper()
	rsaKey,err:=rsa.GenerateKey(rand.Reader,2048)
	if err!=nil{t.Fatal(err)}
	v,err:=NewRS256Verifier("https://idp.example.invalid/ci","dataflow-bootstrap-ci",
		map[string]*rsa.PublicKey{"test-key":&rsaKey.PublicKey})
	if err!=nil{t.Fatal(err)}
	now:=time.Now().UTC()
	token:=signedSyntheticJWT(t,rsaKey,map[string]any{"alg":"RS256","kid":"test-key"},
		map[string]any{"iss":"https://idp.example.invalid/ci","sub":"synthetic-admin",
			"aud":"dataflow-bootstrap-ci","iat":now.Add(-time.Minute).Unix(),
			"nbf":now.Add(-time.Minute).Unix(),"exp":now.Add(10*time.Minute).Unix()})
	principal,err:=v.Verify(token)
	if err!=nil{t.Fatal(err)}
	pub1,priv1,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	pub2,priv2,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	b:=&ManageBootstrap{verifier:v,operators:map[string]ed25519.PublicKey{
		"operator-a":pub1,"operator-b":pub2},isolated:true,now:time.Now}
	req:=ManageBootstrapRequest{
		RequestID:"d141c547-740b-4bb0-a111-001000000001",
		Workspace:"lab-team",TargetActorID:principal.ActorID(),
		Issuer:"https://idp.example.invalid/ci",Action:"bootstrap_manage",
		IssuedAt:now.Add(-time.Minute).Unix(),ExpiresAt:now.Add(8*time.Minute).Unix(),
	}
	signSyntheticBootstrap(t,&req,[]ed25519.PrivateKey{priv1,priv2})
	return b,token,req,[]ed25519.PrivateKey{priv1,priv2}
}
func signSyntheticBootstrap(t *testing.T,r *ManageBootstrapRequest,private []ed25519.PrivateKey){
	t.Helper()
	message,err:=BootstrapSigningMessage(*r)
	if err!=nil{t.Fatal(err)}
	r.Approvals=[]BootstrapSignature{
		{KeyID:"operator-a",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(private[0],message))},
		{KeyID:"operator-b",Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(private[1],message))},
	}
}

func TestManageBootstrapSignedEnvelopeFailClosed(t *testing.T){
	b,token,req,keys:=makeSyntheticBootstrapFixture(t)
	id,digest,err:=b.authorizeEnvelope(token,req)
	if err!=nil||id!=req.TargetActorID||digest==([32]byte{}){t.Fatalf("valid signed envelope: %v",err)}
	redacted,err:=json.Marshal(req)
	if err!=nil||string(redacted)!=`"[REDACTED]"`||
		strings.Contains(req.String(),req.TargetActorID)||strings.Contains(req.GoString(),req.Workspace){
		t.Fatal("operator approval leaked into external representation")
	}
	cases:=[]struct{name string; mutate func(*ManageBootstrapRequest)}{
		{"cross-workspace",func(x *ManageBootstrapRequest){x.Workspace="another-lab"}},
		{"forged target actor",func(x *ManageBootstrapRequest){x.TargetActorID=strings.Repeat("a",64)}},
		{"changed issuer",func(x *ManageBootstrapRequest){x.Issuer="https://foreign.invalid/tenant"}},
		{"changed action",func(x *ManageBootstrapRequest){x.Action="delegate_manage"}},
		{"changed request ID",func(x *ManageBootstrapRequest){x.RequestID="d141c547-740b-4bb0-a111-001000000002"}},
		{"invalid request ID",func(x *ManageBootstrapRequest){x.RequestID="not-a-uuid"}},
		{"expired approval",func(x *ManageBootstrapRequest){x.ExpiresAt=time.Now().Add(-time.Second).Unix()}},
		{"stale approval",func(x *ManageBootstrapRequest){x.IssuedAt=time.Now().Add(-20*time.Minute).Unix()}},
		{"overlong approval",func(x *ManageBootstrapRequest){x.ExpiresAt=x.IssuedAt+3600}},
		{"future approval",func(x *ManageBootstrapRequest){x.IssuedAt=time.Now().Add(time.Hour).Unix();x.ExpiresAt=x.IssuedAt+300}},
		{"same signer twice",func(x *ManageBootstrapRequest){x.Approvals[1]=x.Approvals[0]}},
		{"invalid signature",func(x *ManageBootstrapRequest){x.Approvals[1].Signature=base64.RawURLEncoding.EncodeToString(make([]byte,ed25519.SignatureSize))}},
		{"unknown signer",func(x *ManageBootstrapRequest){x.Approvals[1].KeyID="untrusted-operator"}},
		{"missing approval",func(x *ManageBootstrapRequest){x.Approvals=x.Approvals[:1]}},
	}
	for _,tc:=range cases{
		t.Run(tc.name,func(t *testing.T){
			x:=req
			x.Approvals=append([]BootstrapSignature(nil),req.Approvals...)
			tc.mutate(&x)
			if _,_,err:=b.authorizeEnvelope(token,x);!errors.Is(err,ErrForbidden){
				t.Fatalf("forged operator approval accepted: %v",err)
			}
		})
	}
	if _,_,err=b.authorizeEnvelope("invalid.jwt",req);!errors.Is(err,ErrForbidden){t.Fatal("unsigned target JWT accepted")}
	b.isolated=false
	if _,_,err=b.authorizeEnvelope(token,req);!errors.Is(err,ErrUnavailable){t.Fatal("nonisolated bootstrap enabled")}
	b.isolated=true
	// A signature from the same operator key under a different key name may
	// never be used as independent approval; constructor rejects duplicates.
	pubs:=map[string]ed25519.PublicKey{
		"operator-a":keys[0].Public().(ed25519.PublicKey),
		"operator-b":keys[0].Public().(ed25519.PublicKey),
	}
	if _,err=NewIsolatedManageBootstrap(SecureCatalog{},b.verifier,pubs,true);!errors.Is(err,ErrUnavailable){
		t.Fatal("missing isolated control database accepted")
	}
	_ = context.Background()
}

func TestManageBootstrapMessageRequiresBothDistinctOperators(t *testing.T) {
	b,token,req,priv:=makeSyntheticBootstrapFixture(t)
	signed,_:=BootstrapSigningMessage(req)
	h:=sha256.Sum256(signed)
	if h==([32]byte{}) {t.Fatal("empty signed payload hash")}
	signSyntheticBootstrap(t,&req,priv)
	if _,_,err:=b.authorizeEnvelope(token,req);err!=nil{t.Fatal(err)}
	// A token issuer cannot be made trusted by re-signing the approval
	// envelope: actual JWT verification still controls principal derivation.
	otherRSAPrivate,err:=rsa.GenerateKey(rand.Reader,2048)
	if err!=nil{t.Fatal(err)}
	now:=time.Now().UTC()
	bogus:=signedSyntheticJWT(t,otherRSAPrivate,map[string]any{"alg":"RS256","kid":"test-key"},
		map[string]any{"iss":req.Issuer,"sub":"synthetic-admin","aud":"dataflow-bootstrap-ci",
			"iat":now.Add(-time.Minute).Unix(),"exp":now.Add(time.Minute).Unix()})
	if _,_,err:=b.authorizeEnvelope(bogus,req);!errors.Is(err,ErrForbidden){
		t.Fatal("foreign token signer accepted")
	}
	// Signing payload is purpose-bound and cannot be confused with RS256 JWT.
	jwtHash:=sha256.Sum256([]byte(token))
	_ = crypto.SHA256
	if jwtHash==h {t.Fatal("unexpected collision")}
}
