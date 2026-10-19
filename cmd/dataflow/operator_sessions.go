package main

// Local single-instance operator sessions. This is a deliberately bounded
// transition away from pasting API/admin tokens in every UI panel; enterprise
// production still requires externally approved OIDC/SSO, durable sessions,
// Vault/KMS and deployment-grade TLS. Never use this across public networks.
import (
 "context"
 "crypto/rand"
 "crypto/sha256"
  "encoding/base64"
 "encoding/json"
 "io"
 "net/http"
 "os"
 "strings"
 "sync"
 "time"

 "golang.org/x/crypto/bcrypt"
)

type sessionRole string
const (operatorRole sessionRole="operator"; administratorRole sessionRole="admin")
type sessionState struct{Role sessionRole;Expires time.Time}
type sessionKey struct{}
type loginSessions struct{
 mu sync.Mutex
 active map[[32]byte]sessionState
}
func(s *server) sessionEnabled()bool{
 return os.Getenv("DATAFLOW_AUTH_SESSION_MODE")=="local" &&
 os.Getenv("DATAFLOW_LOCAL_OPERATOR_PASSWORD_HASH")!="" &&
 os.Getenv("DATAFLOW_LOCAL_ADMIN_PASSWORD_HASH")!="" &&
 s!=nil && s.token!=""
}
func(s *server)sessionStore()*loginSessions{
 if s.operatorSessions==nil{s.operatorSessions=&loginSessions{active:make(map[[32]byte]sessionState)}}
 return s.operatorSessions
}
func (s *server) localAuthAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Content-Type","application/json")
 if !s.sessionEnabled(){http.Error(w,"session login not configured",http.StatusNotFound);return}
 switch {
 case r.Method==http.MethodGet:
  role,ok:=s.roleFromSession(r)
  _=json.NewEncoder(w).Encode(map[string]any{"authenticated":ok,"role":role})
 case r.Method==http.MethodDelete:
  if !sameOriginSessionRequest(r){http.Error(w,"invalid origin",403);return}
  s.expireSession(r)
  w.WriteHeader(http.StatusNoContent)
 case r.Method==http.MethodPost:
  if !sameOriginSessionRequest(r){http.Error(w,"invalid origin",403);return}
  r.Body=http.MaxBytesReader(w,r.Body,4096)
  d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
  var payload struct{Username string `json:"username"`;Password string `json:"password"`}
  if d.Decode(&payload)!=nil||d.Decode(new(any))!=io.EOF{
   http.Error(w,"invalid credentials",400);return
  }
  var role sessionRole
  var hashed string
  switch payload.Username{
  case "admin":role=administratorRole;hashed=os.Getenv("DATAFLOW_LOCAL_ADMIN_PASSWORD_HASH")
  case "operator":role=operatorRole;hashed=os.Getenv("DATAFLOW_LOCAL_OPERATOR_PASSWORD_HASH")
  default:hashed="$2a$12$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalid"
  }
  // Never log the supplied password or distinguish unknown users.
  if len(payload.Password)<8||len(payload.Password)>1024||
   bcrypt.CompareHashAndPassword([]byte(hashed),[]byte(payload.Password))!=nil{
   http.Error(w,"invalid credentials",401);return
  }
  raw:=make([]byte,32);if _,e:=rand.Read(raw);e!=nil{http.Error(w,"session unavailable",503);return}
  value:=base64.RawURLEncoding.EncodeToString(raw)
  key:=sha256.Sum256(raw)
  store:=s.sessionStore()
  store.mu.Lock()
  now:=time.Now()
  for k,v:=range store.active{if now.After(v.Expires){delete(store.active,k)}}
  if len(store.active)>256{store.mu.Unlock();http.Error(w,"session capacity reached",503);return}
  store.active[key]=sessionState{Role:role,Expires:now.Add(4*time.Hour)}
  store.mu.Unlock()
  http.SetCookie(w,&http.Cookie{Name:"dataflow_session",Value:value,
   Path:"/api/v1",HttpOnly:true,SameSite:http.SameSiteStrictMode,
   Secure:r.TLS!=nil,MaxAge:4*3600})
  _=json.NewEncoder(w).Encode(map[string]any{"authenticated":true,"role":role})
 default:http.Error(w,"GET, POST or DELETE required",405)
 }
}
func(s *server)roleFromSession(r *http.Request)(sessionRole,bool){
 var empty sessionRole
 cookie,e:=r.Cookie("dataflow_session");if e!=nil{return empty,false}
 value,e:=base64.RawURLEncoding.DecodeString(cookie.Value)
 if e!=nil||len(value)!=32{return empty,false}
 digest:=sha256.Sum256(value)
 store:=s.sessionStore()
 store.mu.Lock();defer store.mu.Unlock()
 current,ok:=store.active[digest]
 if !ok||time.Now().After(current.Expires){delete(store.active,digest);return empty,false}
 return current.Role,true
}
func(s *server)expireSession(r *http.Request){
 cookie,e:=r.Cookie("dataflow_session")
 if e==nil {
  if b,e:=base64.RawURLEncoding.DecodeString(cookie.Value);e==nil&&len(b)==32{
   hash:=sha256.Sum256(b);s.sessionStore().mu.Lock();delete(s.sessionStore().active,hash);s.sessionStore().mu.Unlock()
  }
 }
}
func sameOriginSessionRequest(r *http.Request)bool{
 origin:=r.Header.Get("Origin")
 if origin!=""{
  u:=strings.TrimSuffix(origin,"/")
  return u=="http://"+r.Host || u=="https://"+r.Host
 }
 // Host-level Origin is expected for browser POST/DELETE. Disallow requests
 // without either origin or a same-origin Fetch Metadata assertion.
 return r.Header.Get("Sec-Fetch-Site")=="same-origin"
}
func hasAdminSession(r *http.Request)bool{
 role,ok:=r.Context().Value(sessionKey{}).(sessionRole)
 return ok&&role==administratorRole
}
func(s *server)sessionMiddleware(next http.Handler)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if !s.sessionEnabled()||!strings.HasPrefix(r.URL.Path,"/api/v1/")||
   r.URL.Path=="/api/v1/auth/session"||
   strings.HasPrefix(r.URL.Path,"/api/v1/secure/"){
   next.ServeHTTP(w,r);return
  }
  role,ok:=s.roleFromSession(r)
  if !ok{
   http.Error(w,"sign in required",http.StatusUnauthorized);return
  }
  if r.Method!=http.MethodGet&&r.Method!=http.MethodHead&&r.Method!=http.MethodOptions&&
   !sameOriginSessionRequest(r){
   http.Error(w,"cross-origin request refused",http.StatusForbidden);return
  }
  // Only the in-process trusted middleware may translate an HttpOnly session
  // into the existing API's internal bearer. No static API token is sent to UI.
  clone:=r.Clone(context.WithValue(r.Context(),sessionKey{},role))
  clone.Header=r.Header.Clone()
  clone.Header.Set("Authorization","Bearer "+s.token)
  clone.Header.Del("X-DataFlow-Connection-Admin")
  if role==administratorRole {clone.Header.Set("X-DataFlow-Connection-Admin",os.Getenv("DATAFLOW_CONNECTION_ADMIN_TOKEN"))}
  next.ServeHTTP(w,clone)
 })
}
