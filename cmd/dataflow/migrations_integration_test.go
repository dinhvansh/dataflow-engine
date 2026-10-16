package main

import (
 "context"
 "os"
 "path/filepath"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

func TestStreamTuningMigrationIdempotentIntegration(t *testing.T) {
 dsn := os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
 if dsn == "" { t.Skip("requires disposable PostgreSQL") }
 if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED") != "true" { t.Fatal("refusing non-isolated database") }
 ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
 defer cancel()
 pool, err := pgxpool.New(ctx, dsn)
 if err != nil { t.Fatal(err) }
 defer pool.Close()
 raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", "004_stream_tuning.sql"))
 if err != nil { t.Fatal(err) }
 for attempt := 1; attempt <= 2; attempt++ {
  if _, err = pool.Exec(ctx, string(raw)); err != nil { t.Fatalf("attempt %d: %v", attempt, err) }
 }
}
