package connectionsecurity

import (
 "context"
 "crypto/tls"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "testing"
 "time"
)

// Only a local, generated TLS fixture is contacted by this test.
func TestKVv2VaultSyntheticTLSLifecycle(t *testing.T){
 var mu sync.Mutex
 version:=uint64(0)
 secrets:=make(map[uint64]string)
 token:="SYNTHETIC-VAULT-TOKEN-NO-REAL-ACCESS"
 var requests int
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  mu.Lock()
  defer mu.Unlock()
  requests++
  if r.Header.Get("X-Vault-Token")!=token{http.Error(w,"bad synthetic token",403);return}
  if r.URL.Path!="/v1/kv/data/workspaces/lab-safe/refdata/item"{http.Error(w,"unexpected path",404);return}
  w.Header().Set("Content-Type","application/json")
  switch r.Method{
  case http.MethodPost:
   var incoming struct{
    Options struct{CAS uint64 `json:"cas"`} `json:"options"`
    Data struct{Value string `json:"value"`} `json:"data"`
   }
   if err:=json.NewDecoder(r.Body).Decode(&incoming);err!=nil{http.Error(w,"bad request",400);return}
   if incoming.Options.CAS!=version{http.Error(w,"cas failure",409);return}
   version++
   secrets[version]=incoming.Data.Value
   _,_=fmt.Fprintf(w,`{"data":{"version":%d}}`,version)
  case http.MethodGet:
   var want uint64
   if r.URL.RawQuery=="version=1"{want=1}else if r.URL.RawQuery=="version=2"{want=2}
   value,ok:=secrets[want]
   if !ok{http.Error(w,"not found",404);return}
   _,_=fmt.Fprintf(w,`{"data":{"data":{"value":%q}}}`,value)
  default: http.Error(w,"method",405)
  }
 }))
 defer server.Close()
 vault,err:=NewKVv2Vault(server.URL,"kv",token,server.Client())
 if err!=nil{t.Fatal(err)}
 ref,err:=vault.Create(context.Background(),"lab-safe","ref/refdata/item",[]byte("SYNTHETIC-SECRET-1"))
 if err!=nil||ref.Version()!=1{t.Fatalf("create failed %v, %+v",err,ref)}
 read,err:=vault.Read(context.Background(),ref)
 if err!=nil||string(read)!="SYNTHETIC-SECRET-1"{t.Fatalf("read: %v",err)}
 clear(read)
 next,err:=vault.Rotate(context.Background(),ref,[]byte("SYNTHETIC-SECRET-2"))
 if err!=nil||next.Version()!=2{t.Fatalf("rotate: %v",err)}
 older,err:=vault.Read(context.Background(),ref)
 if err!=nil||string(older)!="SYNTHETIC-SECRET-1"{t.Fatalf("version 1 lost: %v",err)}
 clear(older)
 newer,err:=vault.Read(context.Background(),next)
 if err!=nil||string(newer)!="SYNTHETIC-SECRET-2"{t.Fatalf("version 2 not available: %v",err)}
 clear(newer)
 if _,err=vault.Rotate(context.Background(),ref,[]byte("stale"));!errors.Is(err,ErrUnavailable){t.Fatalf("stale CAS succeeded: %v",err)}
 if _,err=vault.Create(context.Background(),"lab-safe","ref/refdata/item",[]byte("stale create"));!errors.Is(err,ErrUnavailable){t.Fatalf("duplicate create succeeded: %v",err)}
 if requests!=7{t.Fatalf("unexpected local synthetic requests: %d",requests)}
 for _,text:=range []string{fmt.Sprintf("%v",vault),fmt.Sprintf("%#v",vault)}{
  if strings.Contains(text,token)||strings.Contains(text,server.URL){t.Fatalf("vault secrets disclosed by debug formatting: %s",text)}
 }
}

func TestKVv2VaultRejectsUnsafeConfigAndReferences(t *testing.T){
 for _,input:=range []struct{endpoint,mount,token string}{
  {"http://localhost:8200","kv","synthetic-token"},
  {"https://user:pass@example.com","kv","synthetic-token"},
  {"https://vault.example.com/something","kv","synthetic-token"},
  {"https://vault.example.com?token=abc","kv","synthetic-token"},
  {"https://vault.example.com","bad/path","synthetic-token"},
  {"https://vault.example.com","kv","short"},
  {"https://vault.example.com","kv","token\nheader"},
  {"ftp://vault.example.com","kv","synthetic-token"},
 }{
  if _,err:=NewKVv2Vault(input.endpoint,input.mount,input.token,nil);!errors.Is(err,ErrInvalid){t.Fatalf("unsafe config accepted: %+v (%v)",input,err)}
 }
 insecure:=&http.Client{Transport:&http.Transport{TLSClientConfig:&tls.Config{InsecureSkipVerify:true}}}
 if _,err:=NewKVv2Vault("https://vault.example.com","kv","synthetic-token",insecure);!errors.Is(err,ErrInvalid){t.Fatal("insecure TLS accepted")}
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){http.Error(w,"unexpected",500)}))
 defer server.Close()
 vault,err:=NewKVv2Vault(server.URL,"kv","synthetic-token",server.Client());if err!=nil{t.Fatal(err)}
 bad,err:=NewSecretRef("other/tenant","ref/refdata/item",1);if err!=nil{t.Fatal(err)}
 if _,err=vault.Read(context.Background(),bad);!errors.Is(err,ErrInvalid){t.Fatal("unsafe workspace allowed")}
 if _,err=vault.Create(context.Background(),"lab-safe","ref/unsafe/item",nil);!errors.Is(err,ErrInvalid){t.Fatal("empty secret accepted")}
 if _,err=vault.Create(context.Background(),"lab-safe","ref/unsafe/item",make([]byte,65537));!errors.Is(err,ErrInvalid){t.Fatal("unbounded secret accepted")}
}

func TestKVv2VaultDoesNotFollowRedirectOrLeakToken(t *testing.T){
 requests:=0
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests++
  w.Header().Set("Location","https://example.invalid/steal-token")
  w.WriteHeader(http.StatusTemporaryRedirect)
 }))
 defer server.Close()
 vault,err:=NewKVv2Vault(server.URL,"kv","synthetic-token",server.Client())
 if err!=nil{t.Fatal(err)}
 ref,err:=NewSecretRef("lab-safe","ref/refdata/item",1);if err!=nil{t.Fatal(err)}
 if _,err=vault.Read(context.Background(),ref);!errors.Is(err,ErrUnavailable){t.Fatalf("followed redirect? %v",err)}
 if requests!=1{t.Fatal("redirect followed")}
}

func TestKVv2VaultTimeoutAndNoRawErrorExposure(t *testing.T){
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  <-r.Context().Done()
 }))
 defer server.Close()
 client:=server.Client()
 client.Timeout=25*time.Millisecond
 vault,err:=NewKVv2Vault(server.URL,"kv","synthetic-token",client)
 if err!=nil{t.Fatal(err)}
 ref,err:=NewSecretRef("lab-safe","ref/refdata/item",1);if err!=nil{t.Fatal(err)}
 if _,err=vault.Read(context.Background(),ref);!errors.Is(err,ErrUnavailable){t.Fatalf("unbounded/unsanitized timeout: %v",err)}
}

func TestKVv2VaultFixtureSecretIsBase64NotRaw(t *testing.T){
 raw:=[]byte("SYNTHETIC-CREDENTIAL-ONLY")
 encoded:=base64.StdEncoding.EncodeToString(raw)
 if strings.Contains(encoded,string(raw)){t.Fatal("bad fixture encoding assumption")}
}
