package clickhouse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dinhvansh/dataflow-engine/internal/engine"
)

const fixtureUUID = "cc000000-0000-4000-8000-000000000003"

func sampleTyped(w *Writer) *TypedDestination {
	return &TypedDestination{Writer: w, StreamID: fixtureUUID, RunID: fixtureUUID, KeyColumn: "OrderID",
		Columns: []TypedColumn{{Source: "OrderID", Target: "OrderID", CHType: "Int64"},
			{Source: "Amount", Target: "TotalAmount", CHType: "Nullable(Decimal(18,2))"}}}
}
func TestTypedDestinationRejectsInjectionAndUnowned(t *testing.T) {
	w := &Writer{Database: "scratch", Table: "invoices"}
	d := sampleTyped(w)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*TypedDestination){
		func(x *TypedDestination) { x.Writer.Table = "x;DROP" },
		func(x *TypedDestination) { x.Columns[0].Target = "_df_key" },
		func(x *TypedDestination) { x.Columns[0].CHType = "String) ENGINE=Memory" },
		func(x *TypedDestination) { x.Columns[0].Target = "TotalAmount" },
		func(x *TypedDestination) { x.KeyColumn = "Unknown" },
	} {
		candidate := sampleTyped(&Writer{Database: "scratch", Table: "invoices"})
		change(candidate)
		if e := candidate.Validate(); e == nil {
			t.Fatal("unsafe typed configuration accepted")
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if strings.HasPrefix(q, "SELECT comment FROM system.tables") {
			_, _ = w.Write([]byte("other-owner"))
		}
	}))
	defer srv.Close()
	d.Writer.Endpoint = srv.URL
	if e := d.EnsureOwned(context.Background()); e == nil {
		t.Fatal("adopted an unrelated table")
	}
}
func TestTypedDestinationRejectsOwnedPhysicalDriftBeforeDDL(t *testing.T) {
	for _, drift := range []string{"type", "extra-column", "engine"} {
		t.Run(drift, func(t *testing.T) {
			var creates int
			d := sampleTyped(&Writer{Database: "scratch", Table: "invoices"})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query().Get("query")
				switch {
				case strings.HasPrefix(q, "SELECT comment FROM system.tables"):
					_, _ = w.Write([]byte(d.ownerComment()))
				case strings.HasPrefix(q, "SELECT name,type,default_kind,default_expression FROM system.columns"):
					typ := "Int64"
					if drift == "type" {
						typ = "String"
					}
					_, _ = w.Write([]byte(`{"name":"OrderID","type":"` + typ + `","default_kind":"","default_expression":""}` + "\n" +
						`{"name":"TotalAmount","type":"Nullable(Decimal(18,2))","default_kind":"","default_expression":""}` + "\n" +
						`{"name":"_df_key","type":"String","default_kind":"","default_expression":""}` + "\n" +
						`{"name":"_df_version","type":"UInt64","default_kind":"","default_expression":""}` + "\n" +
						`{"name":"_run_id","type":"String","default_kind":"","default_expression":""}` + "\n" +
						`{"name":"_batch_seq","type":"UInt64","default_kind":"","default_expression":""}` + "\n" +
						`{"name":"_df_ingested_at","type":"DateTime64(3, 'UTC')","default_kind":"DEFAULT","default_expression":"now64(3)"}` + "\n"))
					if drift == "extra-column" {
						_, _ = w.Write([]byte(`{"name":"Unexpected","type":"String"}` + "\n"))
					}
				case strings.HasPrefix(q, "SELECT engine,engine_full,sorting_key FROM system.tables"):
					engine := "ReplacingMergeTree(_df_version) ORDER BY _df_key SETTINGS index_granularity = 8192"
					if drift == "engine" {
						engine = "MergeTree ORDER BY _df_key"
					}
					_, _ = w.Write([]byte(`{"engine":"ReplacingMergeTree","engine_full":"` + engine + `","sorting_key":"_df_key"}`))
				case strings.HasPrefix(q, "CREATE TABLE"):
					creates++
				default:
					t.Errorf("unexpected query: %s", q)
				}
			}))
			defer srv.Close()
			d.Writer.Endpoint = srv.URL
			err := d.EnsureOwned(context.Background())
			if err == nil || creates != 0 {
				t.Fatalf("drift %s accepted or DDL executed: %v, %d", drift, err, creates)
			}
		})
	}
}
func TestTypedDestinationFormatsVersionedRows(t *testing.T) {
	var inserts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if !strings.HasPrefix(q, "INSERT INTO") {
			t.Errorf("not an insert %q", q)
			return
		}
		var record map[string]any
		if e := json.NewDecoder(r.Body).Decode(&record); e != nil {
			t.Error(e)
			return
		}
		inserts++
		if record["TotalAmount"] != 12.5 || record["_df_key"] != "1001" ||
			record["_df_version"] != float64(7) {
			t.Errorf("bad typed row %#v", record)
		}
	}))
	defer srv.Close()
	d := sampleTyped(&Writer{Endpoint: srv.URL, Database: "scratch", Table: "invoices"})
	batch := engine.Batch{Rows: []map[string]any{{
		"OrderID": int64(1001), "Amount": 12.5,
		"SysRowID": "1001", "SysRevID": "0x0000000000000007",
	}}}
	if e := d.Write(context.Background(), batch); e != nil {
		t.Fatal(e)
	}
	if inserts != 1 {
		t.Fatal("typed write was not executed")
	}
}
