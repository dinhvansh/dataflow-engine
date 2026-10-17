package sqlserver

import (
 "context"
 "database/sql"
 "os"
 "testing"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/engine"
)

func TestSnapshotReaderSQLServerIntegration(t *testing.T) {
 dsn := os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN")
 if dsn == "" { t.Skip("requires disposable SQL Server fixture") }
 if os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED") != "true" { t.Fatal("refusing non-isolated SQL Server") }
 db, err := sql.Open("sqlserver", dsn)
 if err != nil { t.Fatal(err) }
 defer db.Close()
 ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
 defer cancel()
 var database string
 if err = db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&database); err != nil { t.Fatal(err) }
 if database != "DataFlowFixtureQA" { t.Fatalf("unsafe database %q", database) }
 rows := 0
 reader := Reader{DB:db, Schema:"Erp", Table:"Part", BatchSize:50, Snapshot:true}
 if err = reader.Read(ctx, func(batch engine.Batch) error { rows += len(batch.Rows); return nil }); err != nil { t.Fatal(err) }
 if rows != 200 { t.Fatalf("rows=%d want 200", rows) }
 rows=0
 incremental:=RowVersionReader{DB:db,Schema:"Erp",Table:"Part",BatchSize:37}
 if err=incremental.Read(ctx,func(batch engine.Batch)error{rows+=len(batch.Rows);return nil});err!=nil{t.Fatal(err)}
 if rows!=200{t.Fatalf("incremental rows=%d want 200",rows)}
}

// Regression for a SQL Server driver/conversion path that truncated rowversion
// to 8 hex characters. All queries are SELECT-only on a disposable fixture.
func TestKeySnapshotBinaryRowVersionSQLServerIntegration(t *testing.T) {
 dsn:=os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN")
 if dsn==""{t.Skip("disposable SQL Server fixture required")}
 if os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED")!="true"{t.Fatal("isolated SQL Server required")}
 db,err:=sql.Open("sqlserver",dsn);if err!=nil{t.Fatal(err)};defer db.Close()
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 var name string
 if err=db.QueryRowContext(ctx,"SELECT DB_NAME()").Scan(&name);err!=nil{t.Fatal(err)}
 if name!="DataFlowFixtureQA"{t.Fatalf("unsafe database %q",name)}
 var raw []byte
 err=db.QueryRowContext(ctx,"SELECT TOP (1) CONVERT(binary(8),[SysRevID]) FROM [Erp].[Part] ORDER BY [SysRowID]").Scan(&raw)
 if err!=nil{t.Fatal(err)}
 formatted,err:=snapshotRowVersion(raw)
 if err!=nil||len(formatted)!=18||formatted[:2]!="0x"{t.Fatalf("binary rowversion got %q bytes=%d error=%v",formatted,len(raw),err)}
}
