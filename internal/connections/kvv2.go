package connectionsecurity

import (
 "bytes"
 "context"
 "encoding/base64"
 "encoding/json"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "time"
)

// KVv2Vault is a server-only, HTTPS-only HashiCorp Vault KV-v2 adapter.
// It never selects Vault host/mount/token from a DataFlow client request.
// Creating/rotating a secret here does NOT activate a DataFlow connection.
type KVv2Vault struct{
 base *url.URL
 mount string
 token string
 client *http.Client
}

var mountPattern=regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func NewKVv2Vault(endpoint,mount,token string,sourceClient *http.Client)(*KVv2Vault,error){
 u,err:=url.Parse(endpoint)
 if err!=nil||u.Scheme!="https"||u.Host==""||u.Hostname()==""||u.User!=nil||
  (u.Path!=""&&u.Path!="/")||u.RawQuery!=""||u.Fragment!=""||
  !mountPattern.MatchString(mount)||len(token)<8||len(token)>4096||strings.ContainsAny(token,"\r\n\x00"){
  return nil,ErrInvalid
 }
 client:=&http.Client{Timeout:5*time.Second}
 if sourceClient!=nil{
  *client=*sourceClient
  if client.Timeout<=0||client.Timeout>10*time.Second{client.Timeout=5*time.Second}
 }
 if transport,ok:=client.Transport.(*http.Transport);ok&&transport.TLSClientConfig!=nil&&transport.TLSClientConfig.InsecureSkipVerify{
  return nil,ErrInvalid
 }
 // Never forward X-Vault-Token to a redirect destination.
 client.CheckRedirect=func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse}
 return &KVv2Vault{base:u,mount:mount,token:token,client:client},nil
}

func (v *KVv2Vault) String()string{return "KVv2Vault([REDACTED])"}
func (v *KVv2Vault) GoString()string{return "KVv2Vault([REDACTED])"}

func (v *KVv2Vault) request(ctx context.Context,method string,ref SecretRef,version uint64,data []byte,cas uint64)(*http.Response,error){
 if v==nil||v.client==nil||!validWorkspace(ref.WorkspaceID())||version>uint64(^uint64(0)>>1)||
  ref.OpaqueHandle()==""||!strings.HasPrefix(ref.OpaqueHandle(),"ref/")||
  strings.HasSuffix(ref.OpaqueHandle(),"/")||strings.Contains(ref.OpaqueHandle(),"//"){
  return nil,ErrInvalid
 }
 suffix:=strings.TrimPrefix(ref.OpaqueHandle(),"ref/")
 if suffix==""{return nil,ErrInvalid}
 // Workspace and handle contain only previously validated safe path characters.
 urlPath:="/v1/"+v.mount+"/data/workspaces/"+ref.WorkspaceID()+"/"+suffix
 dest:=*v.base
 dest.Path=urlPath
 dest.RawPath=""
 if method==http.MethodGet{
  q:=url.Values{}
  q.Set("version",strconv.FormatUint(version,10))
  dest.RawQuery=q.Encode()
 }
 var body io.Reader
 if method==http.MethodPost{
  encoded:=base64.StdEncoding.EncodeToString(data)
  payload,err:=json.Marshal(struct{
   Options struct{CAS uint64 `json:"cas"`} `json:"options"`
   Data struct{Value string `json:"value"`} `json:"data"`
  }{
   Options:struct{CAS uint64 `json:"cas"`}{CAS:cas},
   Data:struct{Value string `json:"value"`}{Value:encoded},
  })
  if err!=nil{return nil,ErrUnavailable}
  defer clear(payload)
  body=bytes.NewReader(payload)
 }
 req,err:=http.NewRequestWithContext(ctx,method,dest.String(),body)
 if err!=nil{return nil,ErrUnavailable}
 req.Header.Set("X-Vault-Token",v.token)
 req.Header.Set("Accept","application/json")
 if method==http.MethodPost{req.Header.Set("Content-Type","application/json")}
 resp,err:=v.client.Do(req)
 if err!=nil{return nil,ErrUnavailable}
 if resp.StatusCode<200||resp.StatusCode>299{
  resp.Body.Close()
  return nil,ErrUnavailable
 }
 return resp,nil
}

func (v *KVv2Vault) Read(ctx context.Context,ref SecretRef)([]byte,error){
 if ref.Version()==0{return nil,ErrInvalid}
 resp,err:=v.request(ctx,http.MethodGet,ref,ref.Version(),nil,0)
 if err!=nil{return nil,err}
 defer resp.Body.Close()
 var payload struct{Data struct{Data struct{Value string `json:"value"`} `json:"data"`} `json:"data"`}
 dec:=json.NewDecoder(io.LimitReader(resp.Body,131072))
 if err=dec.Decode(&payload);err!=nil{return nil,ErrUnavailable}
 secret,err:=base64.StdEncoding.DecodeString(payload.Data.Data.Value)
 if err!=nil||len(secret)==0||len(secret)>65536{return nil,ErrUnavailable}
 return secret,nil
}

// Write new KV-v2 secret with CAS=0. Caller MUST authenticate and authorize
// MANAGE before invoking this server-only function; never expose it as HTTP.
func (v *KVv2Vault) Create(ctx context.Context,workspace,handle string,secret []byte)(SecretRef,error){
 ref,err:=NewSecretRef(workspace,handle,1)
 if err!=nil||!validWorkspace(workspace)||len(secret)==0||len(secret)>65536{return SecretRef{},ErrInvalid}
 if err:=v.write(ctx,ref,secret,0,1);err!=nil{return SecretRef{},err}
 return ref,nil
}

func (v *KVv2Vault) Rotate(ctx context.Context,previous SecretRef,replacement []byte)(SecretRef,error){
 if previous.Version()==0||previous.Version()>=uint64(^uint64(0)>>1)||
   !validWorkspace(previous.WorkspaceID())||len(replacement)==0||len(replacement)>65536{
  return SecretRef{},ErrInvalid
 }
 next,err:=NewSecretRef(previous.WorkspaceID(),previous.OpaqueHandle(),previous.Version()+1)
 if err!=nil{return SecretRef{},ErrInvalid}
 if err=v.write(ctx,next,replacement,previous.Version(),next.Version());err!=nil{return SecretRef{},err}
 return next,nil
}

func(v *KVv2Vault) write(ctx context.Context,ref SecretRef,secret []byte,cas,expected uint64)error{
 resp,err:=v.request(ctx,http.MethodPost,ref,0,secret,cas)
 if err!=nil{return err}
 defer resp.Body.Close()
 var response struct{Data struct{Version uint64 `json:"version"`} `json:"data"`}
 if err=json.NewDecoder(io.LimitReader(resp.Body,32768)).Decode(&response);err!=nil{return ErrUnavailable}
 if response.Data.Version!=expected{return ErrUnavailable}
 return nil
}

var _ Vault = (*KVv2Vault)(nil)
