package connectionsecurity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
 "strings"
)

// CurrentVaultVersion is separate from Vault.Read. A pinned readable version
// does NOT prove it is still the latest version. This interface is synthetic/
// operator-only until Vault policy + concurrency controls are approved.
type CurrentVaultVersion interface {
	LatestVersion(context.Context, SecretRef) (uint64,error)
}

// latestVersion performs a bounded, TLS-verified KV-v2 metadata read. It does
// not follow redirects. Caller MUST independently enforce workspace/prefix.
func(v *KVv2Vault) latestVersion(ctx context.Context,ref SecretRef)(uint64,error){
	if v==nil||v.base==nil||v.client==nil||ref.Version()==0||
		!validWorkspace(ref.WorkspaceID())||ref.OpaqueHandle()=="" {
		return 0,ErrUnavailable
	}
	if !opaqueHandle.MatchString(ref.OpaqueHandle())||
  !strings.HasPrefix(ref.OpaqueHandle(),"ref/")||
  strings.Contains(ref.OpaqueHandle(),"//")||strings.HasSuffix(ref.OpaqueHandle(),"/"){return 0,ErrInvalid}
	dst:=*v.base
	dst.Path="/v1/"+v.mount+"/metadata/workspaces/"+ref.WorkspaceID()+"/"+ref.OpaqueHandle()[4:]
	dst.RawPath=""
	dst.RawQuery=""
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,dst.String(),nil)
	if err!=nil{return 0,ErrUnavailable}
	req.Header.Set("X-Vault-Token",v.token)
	req.Header.Set("Accept","application/json")
	resp,err:=v.client.Do(req)
	if err!=nil{return 0,ErrUnavailable}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK{return 0,ErrUnavailable}
	var obj struct {
		Data struct {
			CurrentVersion uint64 `json:"current_version"`
			Versions map[string]struct {
				DeletionTime *string `json:"deletion_time"`
				Destroyed *bool `json:"destroyed"`
			} `json:"versions"`
		} `json:"data"`
	}
	if err=json.NewDecoder(io.LimitReader(resp.Body,65536)).Decode(&obj);err!=nil||
		obj.Data.CurrentVersion==0||obj.Data.CurrentVersion>uint64(^uint64(0)>>1) {
		return 0,ErrUnavailable
	}
	item,ok:=obj.Data.Versions[strconv.FormatUint(obj.Data.CurrentVersion,10)]
	if !ok||item.Destroyed==nil||item.DeletionTime==nil||
  *item.Destroyed||*item.DeletionTime!="" {return 0,ErrUnavailable}
	return obj.Data.CurrentVersion,nil
}

func(v *WorkspaceKVv2Vault) LatestVersion(ctx context.Context,ref SecretRef)(uint64,error){
	if !v.allowed(ref){return 0,ErrForbidden}
	return v.delegate.latestVersion(ctx,ref)
}

var _ CurrentVaultVersion = (*WorkspaceKVv2Vault)(nil)
