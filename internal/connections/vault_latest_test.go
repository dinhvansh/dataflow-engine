package connectionsecurity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceVaultLatestVersionTLSSynthetic(t *testing.T) {
	const token="SYNTHETIC-METADATA-ONLY-VERSION-TOKEN"
	requests:=0
	mode:="valid"
	server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		requests++
		if r.URL.Path!="/v1/kv/metadata/workspaces/alpha/area_a/item"||
			r.Method!=http.MethodGet||r.Header.Get("X-Vault-Token")!=token {
			http.Error(w,"wrong scope/token",403);return
		}
		w.Header().Set("Content-Type","application/json")
		switch mode{
		case "valid":fmt.Fprint(w,`{"data":{"current_version":2,"versions":{"2":{"deletion_time":"","destroyed":false}}}}`)
		case "deleted":fmt.Fprint(w,`{"data":{"current_version":2,"versions":{"2":{"deletion_time":"2025-01-01T00:00:00Z","destroyed":false}}}}`)
		case "destroyed":fmt.Fprint(w,`{"data":{"current_version":2,"versions":{"2":{"deletion_time":"","destroyed":true}}}}`)
		case "missing":fmt.Fprint(w,`{"data":{"current_version":2,"versions":{"1":{"deletion_time":"","destroyed":false}}}}`)
		case "malformed":fmt.Fprint(w,`{"data":{"current_version":0,"versions":{}}}`)
		case "redirect":w.Header().Set("Location","https://example.invalid/steal");w.WriteHeader(http.StatusFound)
		}
	}))
	defer server.Close()
	v,err:=NewWorkspaceKVv2Vault(server.URL,"kv",token,"alpha","ref/area_a/",server.Client())
	if err!=nil{t.Fatal(err)}
	ok,err:=NewSecretRef("alpha","ref/area_a/item",2)
	if err!=nil{t.Fatal(err)}
	foreign,err:=NewSecretRef("beta","ref/area_b/item",2)
	if err!=nil{t.Fatal(err)}
	before:=requests
	if _,err=v.LatestVersion(context.Background(),foreign);!errors.Is(err,ErrForbidden)||requests!=before{
		t.Fatalf("foreign workspace reached metadata endpoint: %v",err)
	}
	version,err:=v.LatestVersion(context.Background(),ok)
	if err!=nil||version!=2{t.Fatalf("safe synthetic Vault metadata proof: %v %d",err,version)}
	for _,kind:=range []string{"deleted","destroyed","missing","malformed","redirect"} {
		mode=kind
		if _,err=v.LatestVersion(context.Background(),ok);!errors.Is(err,ErrUnavailable){
			t.Fatalf("unsafe metadata %s accepted: %v",kind,err)
		}
	}
}
