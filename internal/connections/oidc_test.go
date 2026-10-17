package connectionsecurity

import (
 "context"
 "crypto"
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

func signedSyntheticJWT(t *testing.T,key *rsa.PrivateKey,header map[string]any,claims map[string]any)string{
 t.Helper()
 a,err:=json.Marshal(header);if err!=nil{t.Fatal(err)}
 b,err:=json.Marshal(claims);if err!=nil{t.Fatal(err)}
 data:=base64.RawURLEncoding.EncodeToString(a)+"."+base64.RawURLEncoding.EncodeToString(b)
 hash:=sha256.Sum256([]byte(data))
 sig,err:=rsa.SignPKCS1v15(rand.Reader,key,crypto.SHA256,hash[:])
 if err!=nil{t.Fatal(err)}
 return data+"."+base64.RawURLEncoding.EncodeToString(sig)
}

func TestRS256VerifierSyntheticOIDC(t *testing.T){
 key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
 verifier,err:=NewRS256Verifier("https://idp.example.invalid/tenant","dataflow-api",map[string]*rsa.PublicKey{"key01":&key.PublicKey})
 if err!=nil{t.Fatal(err)}
 fixed:=time.Unix(1800000000,0)
 verifier.now=func()time.Time{return fixed}
 claims:=map[string]any{
  "iss":"https://idp.example.invalid/tenant",
  "sub":"user.opaque-id",
  "aud":"dataflow-api",
  "iat":fixed.Unix()-60,
  "nbf":fixed.Unix()-60,
  "exp":fixed.Unix()+300,
 }
 header:=map[string]any{"alg":"RS256","typ":"JWT","kid":"key01"}
 token:=signedSyntheticJWT(t,key,header,claims)
 id,err:=verifier.Verify(token)
 if err!=nil||!id.Valid()||len(id.ActorID())!=64||id.ActorID()=="user.opaque-id"{t.Fatalf("verification: %+v %v",id,err)}
 if strings.Contains(id.String(),"opaque-id")||strings.Contains(id.GoString(),"opaque-id"){t.Fatal("identity debug leak")}
 again,err:=verifier.Verify(token);if err!=nil||again.ActorID()!=id.ActorID(){t.Fatal("identity unstable")}
 mutate:=func(claimKey string,value any)string{
  copyClaims:=make(map[string]any)
  for k,v:=range claims{copyClaims[k]=v}
  copyClaims[claimKey]=value
  return signedSyntheticJWT(t,key,header,copyClaims)
 }
 arrayAudience:=mutate("aud",[]string{"extra-app","dataflow-api"})
 if _,err:=verifier.Verify(arrayAudience);err!=nil{t.Fatalf("array audience should work: %v",err)}
 for _,bad:=range []string{
  mutate("aud","untrusted-api"),
  mutate("iss","https://idp.example.invalid/other-tenant"),
  mutate("sub","*"),
  mutate("sub","\r\nspoof"),
  mutate("exp",fixed.Unix()-1),
  mutate("nbf",fixed.Unix()+3600),
  mutate("iat",fixed.Unix()+3600),
  mutate("exp",fixed.Unix()+8000),
  signedSyntheticJWT(t,key,map[string]any{"alg":"none","kid":"key01"},claims),
  signedSyntheticJWT(t,key,map[string]any{"alg":"HS256","kid":"key01"},claims),
  signedSyntheticJWT(t,key,map[string]any{"alg":"RS256","kid":"unknown"},claims),
  signedSyntheticJWT(t,key,map[string]any{"alg":"RS256","kid":"key01","crit":[]string{"something"}},claims),
  "", "Bearer "+token, token+"junk", strings.Repeat("x",16385),
 }{
  if _,err:=verifier.Verify(bad);!errors.Is(err,ErrForbidden){t.Fatalf("invalid JWT accepted: %v",err)}
 }
 // Valid issuer+audiance claims with a DIFFERENT signing key must fail.
 wrongKey,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
 if _,err:=verifier.Verify(signedSyntheticJWT(t,wrongKey,header,claims));!errors.Is(err,ErrForbidden){t.Fatalf("foreign signature accepted: %v",err)}
 // For the same identity, different issuer generates a different scoped ID.
 verifier2,err:=NewRS256Verifier("https://idp.example.invalid/other-tenant","dataflow-api",map[string]*rsa.PublicKey{"key01":&key.PublicKey})
 if err!=nil{t.Fatal(err)}
 verifier2.now=verifier.now
 claims2:=make(map[string]any)
 for k,v:=range claims{claims2[k]=v}
 claims2["iss"]="https://idp.example.invalid/other-tenant"
 scoped,err:=verifier2.Verify(signedSyntheticJWT(t,key,header,claims2))
 if err!=nil||scoped.ActorID()==id.ActorID(){t.Fatal("issuer-scoped subject collision")}
 // Bad token is denied before even attempting to connect to metadata DB.
 protected:=AuthenticatedCatalog{Verifier:verifier}
 if _,err=protected.ListDrafts(context.Background(),"bad token","lab-a");!errors.Is(err,ErrForbidden){t.Fatal("authentication failed open")}
 if _,err=protected.CreateDraft(context.Background(),"bad token","lab-a","X","sqlserver","source");!errors.Is(err,ErrForbidden){t.Fatal("create authentication failed open")}
 if _,err=protected.ListDrafts(context.Background(),token,"lab-a");!errors.Is(err,ErrUnavailable){t.Fatal("authorized but missing database failed open")}
}

func TestRS256VerifierRejectsWeakOrUntrustedConfiguration(t *testing.T){
 strong,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
 weak,err:=rsa.GenerateKey(rand.Reader,1024);if err!=nil{t.Fatal(err)}
 for _,c:=range []struct{iss,aud string;key map[string]*rsa.PublicKey}{
  {"http://idp.example.invalid","dataflow-api",map[string]*rsa.PublicKey{"key":&strong.PublicKey}},
  {"https://idp.example.invalid","",map[string]*rsa.PublicKey{"key":&strong.PublicKey}},
  {"https://idp.example.invalid","dataflow-api",nil},
  {"https://idp.example.invalid","dataflow-api",map[string]*rsa.PublicKey{"bad key":&strong.PublicKey}},
  {"https://idp.example.invalid","dataflow-api",map[string]*rsa.PublicKey{"key":&weak.PublicKey}},
 }{
  if _,err:=NewRS256Verifier(c.iss,c.aud,c.key);!errors.Is(err,ErrInvalid){t.Fatalf("accepted bad verifier config: %+v %v",c,err)}
 }
 var zero VerifiedIdentity
 if zero.Valid(){t.Fatal("zero identity valid")}
 var nilVerifier *RS256Verifier
 if _,err:=nilVerifier.Verify("x.y.z");!errors.Is(err,ErrForbidden){t.Fatal("nil verifier trusted")}
}
