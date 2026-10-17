package clickhouse

import (
	"context"
	"fmt"
	"github.com/dinhvansh/dataflow-engine/internal/engine"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeySnapshotWriterValidation(t *testing.T) {
	w := &KeySnapshotWriter{Destination: &Writer{Database: "scratch", Table: "dataflow_key_snapshots"}, StreamID: "12345678-1234-1234-1234-123456789012", SnapshotID: "12345678-1234-1234-1234-123456789013"}
	bad := engine.Batch{Rows: []map[string]any{{"SysRowID": "not-a-uuid", "SysRevID": "0x0000000000000001"}}}
	if err := w.Write(context.Background(), bad); err == nil {
		t.Fatal("invalid key accepted")
	}
}
func TestSnapshotStatsAndDiffQueries(t *testing.T) {
	var queries []string
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		if strings.Contains(q, "uniqExact") {
			fmt.Fprintln(w, "3")
			return
		}
		fmt.Fprintln(w, "3")
	}))
	defer svr.Close()
	w := &Writer{Endpoint: svr.URL, Database: "scratch", Table: "dataflow_key_snapshots"}
	stream := "12345678-1234-1234-1234-123456789012"
	old := "12345678-1234-1234-1234-123456789013"
	newID := "12345678-1234-1234-1234-123456789014"
	stats, err := w.SnapshotStats(context.Background(), stream, newID)
	if err != nil || stats.Rows != 3 || stats.DistinctKeys != 3 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	diff, err := w.SnapshotDiff(context.Background(), stream, old, newID)
	if err != nil || diff.Added != 3 || diff.Deleted != 3 || diff.Changed != 3 {
		t.Fatalf("diff=%+v err=%v", diff, err)
	}
	if len(queries) != 5 {
		t.Fatalf("got %d queries", len(queries))
	}
	if !strings.Contains(queries[2], "LEFT ANTI JOIN") || !strings.Contains(queries[4], "INNER JOIN") {
		t.Fatal(queries)
	}
}
func TestSnapshotRejectsIdentifiers(t *testing.T) {
	w := &Writer{Database: "unsafe;db", Table: "snapshots"}
	_, err := w.SnapshotStats(context.Background(), "12345678-1234-1234-1234-123456789012", "12345678-1234-1234-1234-123456789012")
	if err == nil {
		t.Fatal("unsafe identifier")
	}
}

func TestDeleteEvidenceRequiresTwoMissingSnapshots(t *testing.T) {
	var queries []string
	const key = "12345678-1234-1234-1234-123456789099"
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		fmt.Fprintln(w, key)
	}))
	defer svr.Close()
	w := &Writer{Endpoint: svr.URL, Database: "scratch", Table: "dataflow_key_snapshots"}
	stream := "12345678-1234-1234-1234-123456789012"
	old := "12345678-1234-1234-1234-123456789013"
	missing := "12345678-1234-1234-1234-123456789014"
	confirm := "12345678-1234-1234-1234-123456789015"
	if keys, err := w.MissingSnapshotKeys(context.Background(), stream, old, missing); err != nil || len(keys) != 1 {
		t.Fatalf("candidate=%v err=%v", keys, err)
	}
	if keys, err := w.ConfirmedMissingSnapshotKeys(context.Background(), stream, old, missing, confirm); err != nil || len(keys) != 1 {
		t.Fatalf("confirmed=%v err=%v", keys, err)
	}
	if len(queries) != 2 || strings.Count(queries[0], "LEFT ANTI JOIN") != 1 || strings.Count(queries[1], "LEFT ANTI JOIN") != 2 {
		t.Fatalf("queries did not enforce two omissions: %v", queries)
	}
}

func TestHardDeleteTombstoneValidationAndIdempotentKey(t *testing.T) {
	var bodies, queries []string
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		queries = append(queries, r.URL.Query().Get("query"))
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()
	w := &Writer{Endpoint: svr.URL, Database: "scratch"}
	item := HardDeleteTombstone{
		StreamID: "12345678-1234-1234-1234-123456789012", Key: "12345678-1234-1234-1234-123456789099",
		WitnessSnapshotID: "12345678-1234-1234-1234-123456789013", ConfirmSnapshotID: "12345678-1234-1234-1234-123456789014", Version: 7,
	}
	if err := w.WriteHardDeleteTombstones(context.Background(), []HardDeleteTombstone{item, item}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || !strings.Contains(queries[0], "ReplacingMergeTree") || strings.Count(bodies[1], `"_evidence_sequence":7`) != 2 {
		t.Fatalf("unexpected idempotent tombstone request: %#v", bodies)
	}
	item.Version = 0
	if err := w.WriteHardDeleteTombstones(context.Background(), []HardDeleteTombstone{item}); err == nil {
		t.Fatal("zero version accepted")
	}
}

// A newly reappeared key is the only key that may invalidate a pending
// delete candidate. A key that stays absent must remain for confirmation.
func TestAddedSnapshotKeysOnlyClearsReappearances(t *testing.T) {
 var query string
 const returnedKey = "12345678-1234-1234-1234-123456789099"
 svr:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  query=r.URL.Query().Get("query")
  fmt.Fprintln(w,returnedKey)
 }))
 defer svr.Close()
 writer:=&Writer{Endpoint:svr.URL,Database:"scratch",Table:"dataflow_key_snapshots"}
 stream:="12345678-1234-1234-1234-123456789012"
 old:="12345678-1234-1234-1234-123456789013"
 current:="12345678-1234-1234-1234-123456789014"
 keys,err:=writer.AddedSnapshotKeys(context.Background(),stream,old,current)
 if err!=nil||len(keys)!=1||keys[0]!=returnedKey{t.Fatalf("added keys=%v err=%v",keys,err)}
 if !strings.Contains(query,"SELECT n._pk FROM")||!strings.Contains(query,"LEFT ANTI JOIN")||!strings.Contains(query,"LIMIT 100001"){
  t.Fatalf("reappearance must test new minus old, got %q",query)
 }
}
