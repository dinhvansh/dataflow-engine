package main
import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "golang.org/x/crypto/bcrypt"
)
func TestLocalSessionLoginRoleCSRFAndLogout(t *testing.T){
 operatorHash,e:=bcrypt.GenerateFromPassword([]byte("qa-operator-password-2026"),bcrypt.MinCost)
 if e!=nil{t.Fatal(e)}
 adminHash,e:=bcrypt.GenerateFromPassword([]byte("qa-admin-password-2026"),bcrypt.MinCost)
 if e!=nil{t.Fatal(e)}
 t.Setenv("DATAFLOW_AUTH_SESSION_MODE","local")
 t.Setenv("DATAFLOW_LOCAL_OPERATOR_PASSWORD_HASH",string(operatorHash))
 t.Setenv("DATAFLOW_LOCAL_ADMIN_PASSWORD_HASH",string(adminHash))
 t.Setenv("DATAFLOW_CONNECTION_ADMIN_TOKEN","independent-qa-admin")
 s:=&server{token:"backend-only-api",operatorSessions:&loginSessions{active:make(map[[32]byte]sessionState)}}
 login:=func(user,password string)*http.Cookie{
  t.Helper()
  raw,_:=json.Marshal(map[string]string{"username":user,"password":password})
  req:=httptest.NewRequest(http.MethodPost,"http://example.com/api/v1/auth/session",strings.NewReader(string(raw)))
  req.Header.Set("Origin","http://example.com")
  out:=httptest.NewRecorder();s.localAuthAPI(out,req)
  if out.Code!=200{t.Fatalf("login status=%d",out.Code)}
  cookies:=out.Result().Cookies()
  if len(cookies)!=1||!cookies[0].HttpOnly||cookies[0].SameSite!=http.SameSiteStrictMode{t.Fatal("session cookie not protected")}
  return cookies[0]
 }
 operator:=login("operator","qa-operator-password-2026")
 admin:=login("admin","qa-admin-password-2026")
 handler:=s.sessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  _=json.NewEncoder(w).Encode(map[string]any{
   "auth":r.Header.Get("Authorization"),
   "admin":r.Header.Get("X-DataFlow-Connection-Admin"),
  })
 }))
 post:=func(cookie *http.Cookie,origin string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(http.MethodPost,"http://example.com/api/v1/connections/runtime",nil)
  req.Header.Set("Origin",origin)
  req.Header.Set("X-DataFlow-Connection-Admin","forged-admin")
  req.Header.Set("Authorization","Bearer forged-user")
  req.AddCookie(cookie)
  out:=httptest.NewRecorder();handler.ServeHTTP(out,req)
  return out
 }
 if result:=post(operator,"http://attacker.com");result.Code!=403{t.Fatalf("cross-origin allowed: %d",result.Code)}
 operatorResult:=post(operator,"http://example.com")
 if operatorResult.Code!=200{t.Fatalf("operator auth failed: %d",operatorResult.Code)}
 var headers map[string]string
 if e=json.Unmarshal(operatorResult.Body.Bytes(),&headers);e!=nil{t.Fatal(e)}
 if headers["admin"]!=""||headers["auth"]!="Bearer backend-only-api"{t.Fatalf("operator escalated: %+v",headers)}
 adminResult:=post(admin,"http://example.com")
 if e=json.Unmarshal(adminResult.Body.Bytes(),&headers);e!=nil{t.Fatal(e)}
 if headers["admin"]!="independent-qa-admin"{t.Fatal("admin session not server-injected")}
 logout:=httptest.NewRequest(http.MethodDelete,"http://example.com/api/v1/auth/session",nil)
 logout.Header.Set("Origin","http://example.com");logout.AddCookie(operator)
 result:=httptest.NewRecorder();s.localAuthAPI(result,logout)
 if result.Code!=204{t.Fatal("logout failed")}
 if post(operator,"http://example.com").Code!=401{t.Fatal("revoked session accepted")}
 bad:=httptest.NewRequest(http.MethodPost,"http://example.com/api/v1/auth/session",strings.NewReader(`{"username":"admin","password":"wrong"}`))
 bad.Header.Set("Origin","http://example.com")
 result=httptest.NewRecorder();s.localAuthAPI(result,bad)
 if result.Code!=401||len(result.Result().Cookies())!=0{t.Fatal("invalid password accepted")}
}

func TestIncompleteLocalAuthFailsClosed(t *testing.T){
 t.Setenv("DATAFLOW_AUTH_SESSION_MODE","local")
 t.Setenv("DATAFLOW_LOCAL_ADMIN_PASSWORD_HASH","")
 t.Setenv("DATAFLOW_LOCAL_OPERATOR_PASSWORD_HASH","")
 svc:=&server{token:"not-for-browser"}
 wrapped:=svc.sessionMiddleware(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(200)}))
 r:=httptest.NewRequest("GET","http://example.com/api/v1/source/tables",nil)
 r.Header.Set("Authorization","Bearer not-for-browser")
 rec:=httptest.NewRecorder()
 wrapped.ServeHTTP(rec,r)
 if rec.Code!=http.StatusServiceUnavailable{t.Fatalf("failed open with code %d",rec.Code)}
 login:=httptest.NewRecorder()
 svc.localAuthAPI(login,httptest.NewRequest("GET","http://example.com/api/v1/auth/session",nil))
 if login.Code!=http.StatusServiceUnavailable{t.Fatal("auth misconfiguration hidden behind legacy fallback")}
}
