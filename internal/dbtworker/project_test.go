package dbtworker

import(
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func config()Config{return Config{Host:"clickhouse",Port:8123,Database:"scratch",Username:"limited_worker",Password:"test-secret"}}
func TestProjectAndValidation(t *testing.T){
 cfg:=config();p:=Plan{Name:"revenue",Target:"dfv_revenue",SQL:"SELECT 'Hello' AS label"}
 dir:=t.TempDir()
 if err:=createProject(dir,p,cfg);err!=nil{t.Fatal(err)}
 for _,f:=range []string{"dbt_project.yml","profiles.yml","models/dfv_revenue.sql"}{
  if _,err:=os.Stat(filepath.Join(dir,f));err!=nil{t.Fatal(err)}
 }
 raw,_:=os.ReadFile(filepath.Join(dir,"profiles.yml"))
 if !strings.Contains(string(raw),"schema: scratch")||!strings.Contains(string(raw),"test-secret"){t.Fatal("dbt connection YAML missing")}
 for _,bad:=range []Plan{
  {Name:"../escape",Target:"dfv_escape",SQL:"SELECT 1"},
  {Name:"revenue",Target:"other",SQL:"SELECT 1"},
  {Name:"revenue",Target:"dfv_revenue",SQL:"SELECT 1; DROP TABLE x"},
  {Name:"revenue",Target:"dfv_revenue",SQL:"SELECT '{{ var(\\\"oops\\\") }}'"},
 }{if ValidatePlan(bad,cfg)==nil{t.Fatalf("unsafe plan accepted: %+v",bad)}}
}
func TestFakeDBTRunner(t *testing.T){
 bin:=filepath.Join(t.TempDir(),"dbt")
 script:="#!/bin/sh\nmkdir -p \"$DBT_TARGET_PATH\"\nprintf '{\"results\":[{\"status\":\"success\"}]}' > \"$DBT_TARGET_PATH/run_results.json\"\necho 'dbt finished'\n"
 if err:=os.WriteFile(bin,[]byte(script),0700);err!=nil{t.Fatal(err)}
 cfg:=config();cfg.Binary=bin
 ctx,cancel:=context.WithTimeout(context.Background(),4*time.Second);defer cancel()
 result,err:=Execute(ctx,Plan{Name:"revenue",Target:"dfv_revenue",SQL:"SELECT 1 AS x"},cfg)
 if err!=nil||!result.Success||!strings.Contains(result.Log,"dbt finished"){t.Fatalf("%+v %v",result,err)}
}
