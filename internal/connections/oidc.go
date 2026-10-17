package connectionsecurity

import (
 "context"
 "crypto"
 "crypto/rsa"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "math/big"
 "net/url"
 "regexp"
 "strings"
 "time"
)

// VerifiedIdentity can only be constructed inside this package after
// signature + issuer/audience + lifetime verification.
// Database permissions are checked separately per workspace.
type VerifiedIdentity struct {
 actorID string
 issuer string
}

func (p VerifiedIdentity) ActorID()string{return p.actorID}
func (p VerifiedIdentity) Valid()bool{return len(p.actorID)==64&&p.issuer!=""}
func (p VerifiedIdentity) String()string{return "VerifiedIdentity([REDACTED])"}
func (p VerifiedIdentity) GoString()string{return "VerifiedIdentity([REDACTED])"}

// RS256Verifier accepts ONLY pre-pinned RSA signing keys. It never follows
// jku/x5u header URLs, fetches JWKS or trusts a caller's actor/workspace header.
type RS256Verifier struct{
 issuer string
 audience string
 keys map[string]*rsa.PublicKey
 now func()time.Time
}

var keyIDPattern=regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,96}$`)

func NewRS256Verifier(issuer,audience string,keys map[string]*rsa.PublicKey)(*RS256Verifier,error){
 u,err:=url.Parse(issuer)
 if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.RawQuery!=""||u.Fragment!=""||u.User!=nil||
  audience==""||len(audience)>256||strings.ContainsAny(audience,"\r\n\x00")||len(keys)==0||len(keys)>32{
  return nil,ErrInvalid
 }
 copied:=make(map[string]*rsa.PublicKey,len(keys))
 for kid,key:=range keys{
  if !keyIDPattern.MatchString(kid)||key==nil||key.N==nil||key.N.BitLen()<2048||key.E!=65537{
   return nil,ErrInvalid
  }
  copied[kid]=&rsa.PublicKey{N:new(big.Int).Set(key.N),E:key.E}
 }
 return &RS256Verifier{issuer:issuer,audience:audience,keys:copied,now:time.Now},nil
}

type jwtHeader struct {
 Algorithm string `json:"alg"`
 Type string `json:"typ"`
 KeyID string `json:"kid"`
 Critical json.RawMessage `json:"crit"`
}
type jwtClaims struct {
 Issuer string `json:"iss"`
 Subject string `json:"sub"`
 Audience json.RawMessage `json:"aud"`
 Expires int64 `json:"exp"`
 Issued int64 `json:"iat"`
 NotBefore int64 `json:"nbf"`
}

func audienceMatches(raw json.RawMessage,expected string)bool{
 var single string
 if json.Unmarshal(raw,&single)==nil{return subtle.ConstantTimeCompare([]byte(single),[]byte(expected))==1}
 var many []string
 if json.Unmarshal(raw,&many)!=nil||len(many)==0||len(many)>16{return false}
 for _,candidate:=range many{
  if subtle.ConstantTimeCompare([]byte(candidate),[]byte(expected))==1{return true}
 }
 return false
}

func (v *RS256Verifier) Verify(token string)(VerifiedIdentity,error){
 if v==nil||v.now==nil||len(token)==0||len(token)>16384||strings.TrimSpace(token)!=token{
  return VerifiedIdentity{},ErrForbidden
 }
 pieces:=strings.Split(token,".")
 if len(pieces)!=3 {return VerifiedIdentity{},ErrForbidden}
 headerRaw,err:=base64.RawURLEncoding.DecodeString(pieces[0])
 if err!=nil||len(headerRaw)>2048 {return VerifiedIdentity{},ErrForbidden}
 claimsRaw,err:=base64.RawURLEncoding.DecodeString(pieces[1])
 if err!=nil||len(claimsRaw)>8192 {return VerifiedIdentity{},ErrForbidden}
 sig,err:=base64.RawURLEncoding.DecodeString(pieces[2])
 if err!=nil{return VerifiedIdentity{},ErrForbidden}
 var header jwtHeader
 if json.Unmarshal(headerRaw,&header)!=nil||
  header.Algorithm!="RS256"||(header.Type!=""&&header.Type!="JWT")||
  len(header.Critical)!=0||!keyIDPattern.MatchString(header.KeyID){
  return VerifiedIdentity{},ErrForbidden
 }
 key,exists:=v.keys[header.KeyID]
 if !exists{return VerifiedIdentity{},ErrForbidden}
 hash:=sha256.Sum256([]byte(pieces[0]+"."+pieces[1]))
 if rsa.VerifyPKCS1v15(key,crypto.SHA256,hash[:],sig)!=nil{return VerifiedIdentity{},ErrForbidden}
 var claims jwtClaims
 if json.Unmarshal(claimsRaw,&claims)!=nil||
  claims.Issuer!=v.issuer||
  !audienceMatches(claims.Audience,v.audience)||
  !validIdentity(claims.Subject){
  return VerifiedIdentity{},ErrForbidden
 }
 now:=v.now().UTC().Unix()
 // Max 2-hour token lifetime. Prevent issuing in the future or using tokens
 // past exp; small clock skew for nbf/iat, but no grace after expiration.
 if claims.Issued<=0||claims.Expires<=now||
  claims.Expires<=claims.Issued||claims.Expires-claims.Issued>7200||
  claims.Issued>now+60||claims.NotBefore>now+60{
  return VerifiedIdentity{},ErrForbidden
 }
 // Stable, pseudonymous, issuer-scoped actor key; never trust sub alone.
 id:=sha256.Sum256([]byte(claims.Issuer+"\x00"+claims.Subject))
 return VerifiedIdentity{actorID:hex.EncodeToString(id[:]),issuer:claims.Issuer},nil
}

// AuthenticatedCatalog is deliberately not HTTP-wired. All externally supplied
// actor IDs must pass signature validation *before* any metadata authorization.
type AuthenticatedCatalog struct{
 Verifier *RS256Verifier
 Catalog SecureCatalog
}

func (a AuthenticatedCatalog) ListDrafts(ctx context.Context,bearerJWT,workspace string)([]DraftProfile,error){
 p,err:=a.Verifier.Verify(bearerJWT)
 if err!=nil{return nil,ErrForbidden}
 return a.Catalog.ListDrafts(ctx,p.ActorID(),workspace)
}

func (a AuthenticatedCatalog) CreateDraft(ctx context.Context,bearerJWT,workspace,name,driver,kind string)(DraftProfile,error){
 p,err:=a.Verifier.Verify(bearerJWT)
 if err!=nil{return DraftProfile{},ErrForbidden}
 return a.Catalog.CreateDraft(ctx,p.ActorID(),workspace,name,driver,kind)
}

