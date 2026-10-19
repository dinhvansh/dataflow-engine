package main

// The runtime-profile API is limited to a disposable, independently isolated
// lab. User-supplied endpoints are never free-form egress: trusted operator
// configuration must exactly approve each host:port. This is not production
// Vault/IdP/TLS certification, nor an ERP connector provisioning endpoint.
import (
 "bytes"
 "context"
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "database/sql"
 "encoding/base64"
 "encoding/json"
 "errors"
 "io"
 "net"
 "net/http"
 "net/url"
 "os"
 "strconv"
 "strings"
 "time"

 "github.com/google/uuid"
)

type runtimeConnection struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Driver string `json:"driver"`
 Kind string `json:"kind"`
 Host string `json:"host"`
 Port int `json:"port"`
 Database string `json:"database"`
 Username string `json:"username"`
 Status string `json:"status"`
 Revision int64 `json:"revision"`
 CheckedAt *time.Time `json:"checked_at,omitempty"`
}
type createRuntimeConnection struct {
 Name string `json:"name"`
 Driver string `json:"driver"`
 Host string `json:"host"`
 Port int `json:"port"`
 Database string `json:"database"`
 Username string `json:"username"`
 Password string `json:"password"`
}
type connectionBindingInput struct {
 StreamID string `json:"stream_id"`
 SourceID string `json:"source_connection_id"`
 TargetID string `json:"target_connection_id"`
}
var errUnsafeLabConnection=errors.New("isolated connection unavailable")

func runtimeLabEnabled() bool {
 return os.Getenv("DATAFLOW_ENABLE_RUNTIME_CONNECTIONS_LAB")=="true" &&
 os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED")=="true" &&
 os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")=="true" &&
 os.Getenv("DATAFLOW_INTEGRATION_CH_ISOLATED")=="true" &&
 os.Getenv("DATAFLOW_ALLOW_TEST_LOAD")=="true" &&
 os.Getenv("DATAFLOW_TEST_TARGET_DATABASE")=="scratch"
}
func runtimeConnectionAuthorized(s *server,r *http.Request) bool {
 return s!=nil && s.validBearer(r) && s.token!=os.Getenv("DATAFLOW_CONNECTION_ADMIN_TOKEN") &&
 adminConnectionAllowed(r)
}
func runtimeConnectionKey()([]byte,error){
 key,e:=base64.StdEncoding.DecodeString(os.Getenv("DATAFLOW_QA_CONNECTION_KEY_B64"))
 if e!=nil||len(key)!=32{return nil,errUnsafeLabConnection}
 return key,nil
}
func encryptConnectionSecret(id,secret string)([]byte,error){
 key,e:=runtimeConnectionKey();if e!=nil{return nil,e}
 block,e:=aes.NewCipher(key);if e!=nil{return nil,errUnsafeLabConnection}
 cipherA,e:=cipher.NewGCM(block);if e!=nil{return nil,errUnsafeLabConnection}
 nonce:=make([]byte,cipherA.NonceSize())
 if _,e=rand.Read(nonce);e!=nil{return nil,errUnsafeLabConnection}
 return cipherA.Seal(nonce,nonce,[]byte(secret),[]byte(id)),nil
}
func decryptConnectionSecret(id string,ciphertext []byte)(string,error){
 key,e:=runtimeConnectionKey();if e!=nil{return "",e}
 block,e:=aes.NewCipher(key);if e!=nil{return "",errUnsafeLabConnection}
 cipherA,e:=cipher.NewGCM(block);if e!=nil||len(ciphertext)<cipherA.NonceSize(){return "",errUnsafeLabConnection}
 value,e:=cipherA.Open(nil,ciphertext[:cipherA.NonceSize()],ciphertext[cipherA.NonceSize():],[]byte(id))
 if e!=nil{return "",errUnsafeLabConnection}
 defer clear(value)
 return string(value),nil
}
func approvedRuntimeEndpoint(driver,host string,port int)bool{
 flag:="DATAFLOW_QA_SQLSERVER_ALLOWED_ENDPOINT"
 if driver=="clickhouse"{flag="DATAFLOW_QA_CLICKHOUSE_ALLOWED_ENDPOINT"}
 allowed:=os.Getenv(flag)
 if allowed==""||host==""||port<1||port>65535||strings.ContainsAny(host,"/\\@?# \\t\\r\\n"){return false}
 name,p,e:=net.SplitHostPort(allowed)
 if e!=nil||name==""||p==""{return false}
 parsed,e:=strconv.Atoi(p)
 return e==nil&&parsed==port&&host==name
}
func validRuntimeProfile(p createRuntimeConnection)bool{
 if len(strings.TrimSpace(p.Name))<1||len(p.Name)>80||
  len(p.Username)<1||len(p.Username)>128||len(p.Password)<1||len(p.Password)>2048||
  strings.ContainsAny(p.Name,"\\r\\n\\x00")||strings.ContainsAny(p.Username,"\\r\\n\\x00")||
  !approvedRuntimeEndpoint(p.Driver,p.Host,p.Port){return false}
 if p.Driver=="sqlserver"{return p.Database=="DataFlowFixtureQA" && p.Username=="qa_erp_reader"}
 return p.Driver=="clickhouse" && p.Database=="scratch"
}
func decodeRuntimeJSON(w http.ResponseWriter,r *http.Request,target any)error{
 r.Body=http.MaxBytesReader(w,r.Body,8192)
 d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
 if e:=d.Decode(target);e!=nil{return e}
 var extra any
 if e:=d.Decode(&extra);e!=io.EOF{return errors.New("extra JSON")}
 return nil
}
func (s *server) runtimeProfilesAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff")
 if !runtimeLabEnabled(){http.Error(w,"isolated runtime connection disabled",http.StatusForbidden);return}
 if !runtimeConnectionAuthorized(s,r){http.Error(w,"independent administrator authorization required",http.StatusForbidden);return}
 if s.pg==nil{http.Error(w,"database unavailable",http.StatusServiceUnavailable);return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 switch r.Method {
 case http.MethodGet:
  rows,e:=s.pg.Query(ctx,`SELECT id::text,name,driver,kind,host,port,database_name,username,status,revision,checked_at
   FROM connection_runtime_profiles WHERE workspace='lab' ORDER BY created_at DESC LIMIT 100`)
  if e!=nil{http.Error(w,"connection registry unavailable",503);return}
  defer rows.Close()
  out:=make([]runtimeConnection,0)
  for rows.Next(){
   var p runtimeConnection
   if e=rows.Scan(&p.ID,&p.Name,&p.Driver,&p.Kind,&p.Host,&p.Port,&p.Database,&p.Username,&p.Status,&p.Revision,&p.CheckedAt);e!=nil{
    http.Error(w,"registry unavailable",503);return
   }
   out=append(out,p)
  }
  if rows.Err()!=nil{http.Error(w,"registry unavailable",503);return}
  w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(map[string]any{"profiles":out})
 case http.MethodPost:
  var input createRuntimeConnection
  if decodeRuntimeJSON(w,r,&input)!=nil||!validRuntimeProfile(input){http.Error(w,"invalid or unapproved QA connection",422);return}
  id:=uuid.NewString();ciphertext,e:=encryptConnectionSecret(id,input.Password)
  if e!=nil{http.Error(w,"server credential store unavailable",503);return}
  kind:="source";if input.Driver=="clickhouse"{kind="destination"}
  var p runtimeConnection
  e=s.pg.QueryRow(ctx,`INSERT INTO connection_runtime_profiles
    (id,name,driver,kind,host,port,database_name,username,secret_encrypted)
    VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9)
    RETURNING id::text,name,driver,kind,host,port,database_name,username,status,revision,checked_at`,
    id,strings.TrimSpace(input.Name),input.Driver,kind,input.Host,input.Port,input.Database,input.Username,ciphertext).
    Scan(&p.ID,&p.Name,&p.Driver,&p.Kind,&p.Host,&p.Port,&p.Database,&p.Username,&p.Status,&p.Revision,&p.CheckedAt)
  if e!=nil{http.Error(w,"cannot save profile (duplicate or unavailable)",409);return}
  w.Header().Set("Content-Type","application/json");w.WriteHeader(201);_ = json.NewEncoder(w).Encode(p)
 default:http.Error(w,"GET or POST required",405)
 }
}
func (s *server) readRuntimeProfile(ctx context.Context,id string)(runtimeConnection,string,error){
 var p runtimeConnection
 if _,e:=uuid.Parse(id);e!=nil{return p,"",errUnsafeLabConnection}
 var ciphertext []byte
 e:=s.pg.QueryRow(ctx,`SELECT id::text,name,driver,kind,host,port,database_name,username,status,revision,checked_at,secret_encrypted
 FROM connection_runtime_profiles WHERE id=$1::uuid AND workspace='lab'`,id).
 Scan(&p.ID,&p.Name,&p.Driver,&p.Kind,&p.Host,&p.Port,&p.Database,&p.Username,&p.Status,&p.Revision,&p.CheckedAt,&ciphertext)
 if e!=nil||!approvedRuntimeEndpoint(p.Driver,p.Host,p.Port){return p,"",errUnsafeLabConnection}
 secret,e:=decryptConnectionSecret(p.ID,ciphertext)
 return p,secret,e
}
func openLabSQLServer(ctx context.Context,p runtimeConnection,password string)(*sql.DB,error){
 if p.Driver!="sqlserver"||p.Database!="DataFlowFixtureQA"||p.Username!="qa_erp_reader"||
 !approvedRuntimeEndpoint(p.Driver,p.Host,p.Port){return nil,errUnsafeLabConnection}
 u:=&url.URL{Scheme:"sqlserver",Host:net.JoinHostPort(p.Host,strconv.Itoa(p.Port)),User:url.UserPassword(p.Username,password)}
 q:=u.Query();q.Set("database",p.Database);q.Set("encrypt","true")
 // Certificate bypass is allowed ONLY in fully isolated, operator-approved
 // disposable QA. Production endpoints are never admitted.
 if os.Getenv("DATAFLOW_QA_SQL_TRUST_CERT")=="true"{q.Set("TrustServerCertificate","true")}
 u.RawQuery=q.Encode()
 db,e:=sql.Open("sqlserver",u.String());if e!=nil{return nil,errUnsafeLabConnection}
 db.SetMaxOpenConns(2);db.SetMaxIdleConns(1);db.SetConnMaxLifetime(2*time.Minute)
 if e=db.PingContext(ctx);e!=nil{db.Close();return nil,errUnsafeLabConnection}
 return db,nil
}
func checkLabSQLSource(ctx context.Context,db *sql.DB)bool{
 var name string;var read,insert,update,del,alter sql.NullInt64
 e:=db.QueryRowContext(ctx,`SELECT DB_NAME(),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'SELECT'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'INSERT'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'UPDATE'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'DELETE'),
 HAS_PERMS_BY_NAME(N'Erp.Part',N'OBJECT',N'ALTER')`).
 Scan(&name,&read,&insert,&update,&del,&alter)
 return e==nil&&name=="DataFlowFixtureQA"&&read.Valid&&read.Int64==1&&
 insert.Valid&&insert.Int64==0&&update.Valid&&update.Int64==0&&del.Valid&&del.Int64==0&&alter.Valid&&alter.Int64==0
}
func checkLabCH(ctx context.Context,p runtimeConnection,password string)bool{
 if p.Driver!="clickhouse"||p.Database!="scratch"||!approvedRuntimeEndpoint(p.Driver,p.Host,p.Port){return false}
 scheme:="https"
 if os.Getenv("DATAFLOW_QA_CLICKHOUSE_ALLOW_HTTP")=="true"{scheme="http"}
 endpoint:=scheme+"://"+net.JoinHostPort(p.Host,strconv.Itoa(p.Port))+"/"
 req,e:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint,bytes.NewBufferString("SELECT 1"))
 if e!=nil{return false}
 req.SetBasicAuth(p.Username,password)
 req.Header.Set("Content-Type","text/plain")
 client:=&http.Client{Timeout:5*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 resp,e:=client.Do(req);if e!=nil{return false}
 defer resp.Body.Close()
 b,e:=io.ReadAll(io.LimitReader(resp.Body,64))
 return e==nil&&resp.StatusCode==200&&strings.TrimSpace(string(b))=="1"
}
func (s *server) runtimeTestAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodPost{http.Error(w,"POST required",405);return}
 if !runtimeLabEnabled()||!runtimeConnectionAuthorized(s,r){http.Error(w,"not permitted",403);return}
 var input struct{ID string `json:"id"`}
 if decodeRuntimeJSON(w,r,&input)!=nil{http.Error(w,"invalid request",400);return}
 ctx,cancel:=context.WithTimeout(r.Context(),6*time.Second);defer cancel()
 p,secret,e:=s.readRuntimeProfile(ctx,input.ID)
 if e!=nil{http.Error(w,"profile unavailable",404);return}
 passed:=false
 if p.Driver=="sqlserver"{
  db,e:=openLabSQLServer(ctx,p,secret);if e==nil{passed=checkLabSQLSource(ctx,db);db.Close()}
 }else{passed=checkLabCH(ctx,p,secret)}
 // Every re-test invalidates previous binding snapshots, including failures.
 // Never let an old successful status survive a failed credential check.
 status:="draft";if passed{status="verified"}
 _,e=s.pg.Exec(ctx,`UPDATE connection_runtime_profiles SET
 status=$2,checked_at=CASE WHEN $2='verified' THEN now() ELSE NULL END,
 revision=revision+1 WHERE id=$1::uuid AND status IN ('draft','verified')`,p.ID,status)
 if e!=nil{http.Error(w,"test evidence persistence unavailable",503);return}
 w.Header().Set("Content-Type","application/json")
 _ = json.NewEncoder(w).Encode(map[string]any{"id":p.ID,"passed":passed,"tested_at":time.Now().UTC().Format(time.RFC3339)})
}
func (s *server) runtimeBindAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodPost{http.Error(w,"POST required",405);return}
 if !runtimeLabEnabled()||!runtimeConnectionAuthorized(s,r){http.Error(w,"not permitted",403);return}
 var b connectionBindingInput
 if decodeRuntimeJSON(w,r,&b)!=nil{http.Error(w,"invalid binding",400);return}
 if _,e:=uuid.Parse(b.StreamID);e!=nil{http.Error(w,"invalid stream",400);return}
 if b.SourceID==b.TargetID{http.Error(w,"source/destination must differ",422);return}
 ctx,cancel:=context.WithTimeout(r.Context(),8*time.Second);defer cancel()
 tx,e:=s.pg.Begin(ctx);if e!=nil{http.Error(w,"registry unavailable",503);return}
 defer tx.Rollback(ctx)
 var src,tgt runtimeConnection
 // Locks prevent testing/revision drift during binding registration.
 for _,entry:=range []struct{id string;p *runtimeConnection}{{b.SourceID,&src},{b.TargetID,&tgt}}{
  if _,e=uuid.Parse(entry.id);e!=nil{http.Error(w,"invalid profile",400);return}
  e=tx.QueryRow(ctx,`SELECT id::text,driver,kind,host,port,database_name,status,revision,checked_at
   FROM connection_runtime_profiles WHERE id=$1::uuid AND workspace='lab' FOR SHARE`,entry.id).
   Scan(&entry.p.ID,&entry.p.Driver,&entry.p.Kind,&entry.p.Host,&entry.p.Port,&entry.p.Database,&entry.p.Status,&entry.p.Revision,&entry.p.CheckedAt)
  if e!=nil{http.Error(w,"profile unavailable",404);return}
  if entry.p.Status!="verified"||entry.p.CheckedAt==nil||time.Since(*entry.p.CheckedAt)>30*time.Minute||
   !approvedRuntimeEndpoint(entry.p.Driver,entry.p.Host,entry.p.Port){http.Error(w,"profile not recently verified",409);return}
 }
 if src.Driver!="sqlserver"||src.Kind!="source"||src.Database!="DataFlowFixtureQA"||
 tgt.Driver!="clickhouse"||tgt.Kind!="destination"||tgt.Database!="scratch"{
 http.Error(w,"only isolated SQL Server to scratch ClickHouse may bind",403);return}
 var db,table string
 e=tx.QueryRow(ctx,`SELECT target_database,target_table FROM sync_streams WHERE id=$1::uuid FOR SHARE`,b.StreamID).Scan(&db,&table)
 if e!=nil||db!="scratch"||table!="dataflow_raw_events"{http.Error(w,"only scratch sync stream allowed",403);return}
 _,e=tx.Exec(ctx,`INSERT INTO connection_runtime_bindings(stream_id,source_profile_id,target_profile_id,source_revision,target_revision)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5)
 ON CONFLICT(stream_id) DO UPDATE SET
 source_profile_id=EXCLUDED.source_profile_id,target_profile_id=EXCLUDED.target_profile_id,
 source_revision=EXCLUDED.source_revision,target_revision=EXCLUDED.target_revision,
 created_at=now()`,b.StreamID,src.ID,tgt.ID,src.Revision,tgt.Revision)
 if e!=nil||tx.Commit(ctx)!=nil{http.Error(w,"could not bind stream",503);return}
 w.Header().Set("Content-Type","application/json")
 _ = json.NewEncoder(w).Encode(map[string]any{"bound":true,"stream_id":b.StreamID,"source_connection_id":src.ID,"target_connection_id":tgt.ID})
}
