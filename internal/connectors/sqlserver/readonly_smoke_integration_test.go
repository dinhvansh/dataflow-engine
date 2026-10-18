package sqlserver

import (
    "context"
    "database/sql"
    "os"
    "testing"
    "time"
)

// TestDisposableSQLServerReadOnlyIntegration runs against the existing, seeded
// DataFlowFixtureQA Microsoft SQL Server container, NEVER Epicor. Test code
// performs SELECTs only. The CI fixture is provisioned separately by its
// disposable admin job; it is not an application permission or a live probe.
func TestDisposableSQLServerReadOnlyIntegration(t *testing.T) {
    dsn := os.Getenv("DATAFLOW_INTEGRATION_SQLSERVER_DSN")
    if dsn == "" {
        t.Skip("disposable SQL Server fixture not configured")
    }
    if os.Getenv("DATAFLOW_INTEGRATION_MSSQL_ISOLATED") != "true" {
        t.Fatal("refusing SQL Server test without isolated fixture flag")
    }
    db, err := sql.Open("sqlserver", dsn)
    if err != nil {
        t.Fatal("SQL Server driver setup failed")
    }
    defer db.Close()
    db.SetMaxOpenConns(2)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    var name, login string
    if err := db.QueryRowContext(ctx, "SELECT DB_NAME(), ORIGINAL_LOGIN()").Scan(&name, &login); err != nil {
        t.Fatal("disposable SQL Server handshake failed")
    }
    if name != "DataFlowFixtureQA" || login != "qa_erp_reader" {
        t.Fatalf("refusing non-QA database or principal: database=%q principal=%q", name, login)
    }

    // Guard effective object-level permissions WITHOUT attempting DML/DDL.
    // Every query performed by the application test remains SELECT-only.
    var selectPerm, insertPerm, updatePerm, deletePerm, alterPerm sql.NullInt64
    err = db.QueryRowContext(ctx, `SELECT
        HAS_PERMS_BY_NAME(N'Erp.Part', N'OBJECT', N'SELECT'),
        HAS_PERMS_BY_NAME(N'Erp.Part', N'OBJECT', N'INSERT'),
        HAS_PERMS_BY_NAME(N'Erp.Part', N'OBJECT', N'UPDATE'),
        HAS_PERMS_BY_NAME(N'Erp.Part', N'OBJECT', N'DELETE'),
        HAS_PERMS_BY_NAME(N'Erp.Part', N'OBJECT', N'ALTER')`).Scan(
        &selectPerm, &insertPerm, &updatePerm, &deletePerm, &alterPerm,
    )
    if err != nil {
        t.Fatal("read-only permission inspection failed")
    }
    if !selectPerm.Valid || selectPerm.Int64 != 1 {
        t.Fatal("fixture reader lacks SELECT on Erp.Part")
    }
    for permission, value := range map[string]sql.NullInt64{
        "INSERT": insertPerm, "UPDATE": updatePerm, "DELETE": deletePerm, "ALTER": alterPerm,
    } {
        if !value.Valid || value.Int64 != 0 {
            t.Fatalf("QA reader has unexpected %s permission (or indeterminate permission)", permission)
        }
    }

    var owners, writers int
    if err := db.QueryRowContext(ctx,
        "SELECT IS_ROLEMEMBER(N'db_owner'), IS_ROLEMEMBER(N'db_datawriter')",
    ).Scan(&owners, &writers); err != nil || owners != 0 || writers != 0 {
        t.Fatal("QA source reader has unsafe database role")
    }

    tables, err := DiscoverTables(ctx, db, "Erp")
    if err != nil {
        t.Fatal("source table discovery failed")
    }
    seenPart, seenTran := false, false
    for _, table := range tables {
        if table.Schema == "Erp" && table.Name == "Part" {
            seenPart = true
        }
        if table.Schema == "Erp" && table.Name == "PartTran" {
            seenTran = true
        }
    }
    if !seenPart || !seenTran {
        t.Fatal("expected synthetic ERP tables not discovered")
    }
    cols, err := DiscoverColumns(ctx, db, "Erp", "Part")
    if err != nil {
        t.Fatal("source column discovery failed")
    }
    hasRowVersion := false
    for _, col := range cols {
        if col.Name == "SysRevID" && col.CursorCandidate {
            hasRowVersion = true
        }
    }
    if !hasRowVersion {
        t.Fatal("expected rowversion cursor metadata missing")
    }
    var count int64
    if err := db.QueryRowContext(ctx, "SELECT COUNT_BIG(*) FROM [Erp].[Part]").Scan(&count); err != nil {
        t.Fatal("fixture SELECT failed")
    }
    if count != 200 {
        t.Fatalf("fixture mismatch: Erp.Part rows=%d, expected 200", count)
    }
    if _, err := SafeRowVersionUpper(ctx, db); err != nil {
        t.Fatal("read-only MIN_ACTIVE_ROWVERSION failed")
    }
}
