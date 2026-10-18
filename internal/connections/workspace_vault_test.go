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
)

func TestWorkspaceKVv2VaultSyntheticTLSIsolation(t *testing.T) {
	const tokA = "SYNTHETIC-ONLY-TOKEN-WORKSPACE-ALPHA"
	const tokB = "SYNTHETIC-ONLY-TOKEN-WORKSPACE-BETA"
	type state struct {version uint64; values map[uint64]string}
	var mu sync.Mutex
	requests:=0
	states:=map[string]*state{
		"alpha":{values:make(map[uint64]string)},
		"beta":{values:make(map[uint64]string)},
	}
	srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		ws:=""
		switch r.URL.Path {
		case "/v1/kv/data/workspaces/alpha/area_a/account":ws="alpha"
		case "/v1/kv/data/workspaces/beta/area_b/account":ws="beta"
		default:http.Error(w,"unexpected path",403);return
		}
		if (ws=="alpha"&&r.Header.Get("X-Vault-Token")!=tokA)||
			(ws=="beta"&&r.Header.Get("X-Vault-Token")!=tokB) {
			http.Error(w,"synthetic Vault ACL denial",403);return
		}
		w.Header().Set("Content-Type","application/json")
		st:=states[ws]
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Options struct{ CAS uint64 `json:"cas"` } `json:"options"`
				Data struct{ Value string `json:"value"` } `json:"data"`
			}
			if json.NewDecoder(r.Body).Decode(&body)!=nil || body.Options.CAS!=st.version {
				http.Error(w,"synthetic version conflict",409);return
			}
			st.version++
			st.values[st.version]=body.Data.Value
			fmt.Fprintf(w,`{"data":{"version":%d}}`,st.version)
		case http.MethodGet:
			var version uint64
			switch r.URL.Query().Get("version"){
			case "1":version=1
			case "2":version=2
			default:http.Error(w,"bad version",400);return
			}
			value,ok:=st.values[version]
			if !ok{http.Error(w,"missing",404);return}
			fmt.Fprintf(w,`{"data":{"data":{"value":%q}}}`,value)
		default:http.Error(w,"method denied",405)
		}
	}))
	defer srv.Close()
	client:=srv.Client()
	a,err:=NewWorkspaceKVv2Vault(srv.URL,"kv",tokA,"alpha","ref/area_a/",client)
	if err!=nil{t.Fatal(err)}
	b,err:=NewWorkspaceKVv2Vault(srv.URL,"kv",tokB,"beta","ref/area_b/",client)
	if err!=nil{t.Fatal(err)}
	ctx:=context.Background()
	alphaRef,err:=NewSecretRef("alpha","ref/area_a/account",1)
	if err!=nil{t.Fatal(err)}
	betaRef,err:=NewSecretRef("beta","ref/area_b/account",1)
	if err!=nil{t.Fatal(err)}
	wrongPrefix,err:=NewSecretRef("alpha","ref/area_b/account",1)
	if err!=nil{t.Fatal(err)}
	before:=requests
	for _,ref:=range []SecretRef{betaRef,wrongPrefix} {
		if _,err:=a.Read(ctx,ref);!errors.Is(err,ErrForbidden){t.Fatalf("cross-scope read accepted %v",err)}
		if _,err:=a.Rotate(ctx,ref,[]byte("synthetic"));!errors.Is(err,ErrForbidden){t.Fatalf("cross-scope rotate accepted %v",err)}
	}
	if _,err:=a.Create(ctx,"ref/area_b/account",[]byte("synthetic"));!errors.Is(err,ErrForbidden){
		t.Fatalf("cross-scope create accepted: %v",err)
	}
	if requests!=before{t.Fatal("a denied request reached Vault transport")}

	createdA,err:=a.Create(ctx,"ref/area_a/account",[]byte("ALPHA-ONLY-SYNTHETIC"))
	if err!=nil||createdA.Version()!=1{t.Fatalf("alpha create %v",err)}
	createdB,err:=b.Create(ctx,"ref/area_b/account",[]byte("BETA-ONLY-SYNTHETIC"))
	if err!=nil||createdB.Version()!=1{t.Fatalf("beta create %v",err)}
	secretA,err:=a.Read(ctx,alphaRef)
	if err!=nil||string(secretA)!="ALPHA-ONLY-SYNTHETIC"{t.Fatalf("alpha read %v",err)}
	clear(secretA)
	secretB,err:=b.Read(ctx,betaRef)
	if err!=nil||string(secretB)!="BETA-ONLY-SYNTHETIC"{t.Fatalf("beta read %v",err)}
	clear(secretB)
	rotated,err:=a.Rotate(ctx,alphaRef,[]byte("ALPHA-ROTATED-SYNTHETIC"))
	if err!=nil||rotated.Version()!=2{t.Fatalf("CAS rotation %v",err)}
	if _,err=a.Rotate(ctx,alphaRef,[]byte("stale"));!errors.Is(err,ErrUnavailable){t.Fatal("stale CAS accepted")}
	updated,err:=a.Read(ctx,rotated)
	if err!=nil||string(updated)!="ALPHA-ROTATED-SYNTHETIC"{t.Fatalf("new version read %v",err)}
	clear(updated)
	checkB,err:=b.Read(ctx,betaRef)
	if err!=nil||string(checkB)!="BETA-ONLY-SYNTHETIC"{t.Fatal("rotation leaked to other workspace")}
	clear(checkB)
	for _,v:=range []string{fmt.Sprintf("%v",a),fmt.Sprintf("%#v",a)} {
		if strings.Contains(v,tokA)||strings.Contains(v,srv.URL)||strings.Contains(v,"area_a"){
			t.Fatal("provider debug output disclosed token or handle")
		}
	}
	if got:=base64.StdEncoding.EncodeToString([]byte("ALPHA-ROTATED-SYNTHETIC"));got==""{t.Fatal("invalid local fixture")}
}

func TestWorkspaceKVv2VaultRejectsUnsafeConfigAndScope(t *testing.T) {
	acceptEndpoint:="https://vault.example.invalid"
	client:=&http.Client{Transport:&http.Transport{TLSClientConfig:&tls.Config{InsecureSkipVerify:true}}}
	for _,tc:=range []struct{ws,prefix string}{
		{"alpha","ref/"},{"alpha","ref/a/"},{"alpha","ref/area_a"},{"alpha","ref/area_a//"},
		{"alpha","/area_a/"},{"alpha","ref/area_a/../"},{"alpha","ref/area_a/x/"},
		{"*","ref/area_a/"},{"UPPER","ref/area_a/"},{"x","ref/area_a/"},
	}{
		if _,err:=NewWorkspaceKVv2Vault(acceptEndpoint,"kv","synthetic-token-allowlisted",tc.ws,tc.prefix,nil);!errors.Is(err,ErrInvalid){
			t.Fatalf("unsafe scope %q / %q accepted: %v",tc.ws,tc.prefix,err)
		}
	}
	if _,err:=NewWorkspaceKVv2Vault(acceptEndpoint,"kv","synthetic-token-allowlisted","alpha","ref/area_a/",client);!errors.Is(err,ErrInvalid){
		t.Fatal("insecure TLS override accepted")
	}
	var missing *WorkspaceKVv2Vault
	if _,err:=missing.Read(context.Background(),SecretRef{});!errors.Is(err,ErrForbidden){t.Fatal("nil vault readable")}
	if _,err:=missing.Create(context.Background(),"ref/area_a/item",[]byte("fake"));!errors.Is(err,ErrForbidden){
		t.Fatal("nil vault writable")
	}
}
