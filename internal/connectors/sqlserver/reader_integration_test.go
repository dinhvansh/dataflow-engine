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
