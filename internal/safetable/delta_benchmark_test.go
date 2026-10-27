package safetable

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type deltaQAMeasure struct {
	Kind                 string  `json:"kind"`
	ChangedRows          int     `json:"changed_rows"`
	Trial                int     `json:"trial"`
	BaselineRows         int     `json:"baseline_rows"`
	Events               uint64  `json:"events"`
	OutputRows           uint64  `json:"output_rows"`
	StageMS              float64 `json:"stage_ms"`
	RecoveryMS           float64 `json:"recovery_ms"`
	PublishMS            float64 `json:"publish_ms"`
	RollbackMS           float64 `json:"rollback_ms"`
	ChangedRowsPerSecond float64 `json:"changed_rows_per_second"`
	Reconciliation       string  `json:"reconciliation"`
}

func benchmarkToken(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw)
}

func TestDeltaPerformanceQAIntegration(t *testing.T) {
	if os.Getenv("DATAFLOW_BENCHMARK_QA") != "true" {
		t.Skip("explicit disposable delta benchmark opt-in required")
	}
	if os.Getenv("DATAFLOW_INTEGRATION_CH_ISOLATED") != "true" {
		t.Fatal("refusing non-QA ClickHouse target")
	}
	endpoint := os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
	uri, err := url.Parse(endpoint)
	if err != nil || uri == nil || uri.Scheme != "http" || !(uri.Hostname() == "localhost" || uri.Hostname() == "127.0.0.1") {
		t.Fatal("refusing non-loopback endpoint; benchmark only QA-local ClickHouse")
	}
	client := Client{Endpoint: endpoint, User: os.Getenv("DATAFLOW_INTEGRATION_CH_USER"), Password: os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")}
	ctx, stop := context.WithTimeout(context.Background(), 12*time.Minute)
	defer stop()
	if err = client.RequireAtomic(ctx); err != nil {
		t.Fatalf("scratch must be Atomic: %v", err)
	}
	baseline := 100000
	if raw := os.Getenv("DATAFLOW_BENCHMARK_BASELINE_ROWS"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || (parsed != 100000 && parsed != 1000000) {
			t.Fatal("baseline must be 100000 or 1000000")
		}
		baseline = parsed
	}
	const trials = 3
	// No production-scale/7m tests. This measures physical copy-on-write stage
	// latency against 100k rows for 1k, 10k and 50k upsert events.
	all := make([]deltaQAMeasure, 0, 9)
	allocated := make([]string, 0)
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 45*time.Second)
		defer done()
		for i := len(allocated) - 1; i >= 0; i-- {
			_, _ = client.Query(cleanup, "DROP TABLE IF EXISTS scratch."+allocated[i])
		}
	}()
	query := func(sql string) {
		t.Helper()
		if _, e := client.Query(ctx, sql); e != nil {
			t.Fatalf("ClickHouse QA statement failed: %v", e)
		}
	}
	assertPhysical := func(table string, rows, sum uint64) {
		t.Helper()
		raw, e := client.Query(ctx, fmt.Sprintf("SELECT count(),uniqExact(ID),sum(Revenue) FROM scratch.%s FORMAT TabSeparatedRaw", table))
		if e != nil {
			t.Fatal(e)
		}
		parts := strings.Split(raw, "\t")
		if len(parts) != 3 {
			t.Fatalf("invalid reconciliation for %s", table)
		}
		for i, expected := range []uint64{rows, rows, sum} {
			got, parseErr := strconv.ParseUint(parts[i], 10, 64)
			if parseErr != nil || got != expected {
				t.Fatalf("physical reconciliation failed for %s field %d: got %q want %d", table, i, parts[i], expected)
			}
		}
	}
	for _, changed := range []int{1000, 10000, 50000} {
		key := "bench" + benchmarkToken(t)
		target := "dft_" + key
		delta := "dfd_" + key
		allocated = append(allocated, target, delta)
		query(fmt.Sprintf("CREATE TABLE scratch.%s (ID UInt64,Revenue UInt64) ENGINE=MergeTree ORDER BY ID", target))
		query(fmt.Sprintf("INSERT INTO scratch.%s SELECT number,number+1 FROM numbers(%d)", target, baseline))
		query(fmt.Sprintf("CREATE TABLE scratch.%s (ID UInt64,Revenue UInt64,_df_version UInt64,_df_deleted UInt8) ENGINE=MergeTree ORDER BY (ID,_df_version)", delta))
		query(fmt.Sprintf("INSERT INTO scratch.%s SELECT number,number+1000001,number+1,toUInt8(0) FROM numbers(%d)", delta, changed))
		uuid, e := client.UUID(ctx, target)
		if e != nil || uuid == "" {
			t.Fatalf("baseline UUID: %v %q", e, uuid)
		}
		elapsed := make([]float64, 0, trials)
		for trial := 1; trial <= trials; trial++ {
			stage := "dft_stage_" + benchmarkToken(t)
			allocated = append(allocated, stage)
			start := time.Now()
			stat, e := client.BuildDeltaStage(ctx, DeltaPlan{
				Target: target, Delta: delta, Stage: stage, Key: "ID",
				Lower: 0, Upper: uint64(changed), BaselineUUID: uuid,
			})
			dur := time.Since(start)
			if e != nil {
				t.Fatalf("batch=%d trial=%d stage failed: %v", changed, trial, e)
			}
			if stat.Events != uint64(changed) || stat.Rows != uint64(baseline) {
				t.Fatalf("batch=%d trial=%d unexpected stats %+v", changed, trial, stat)
			}
			// A worker can die after writing part of its private stage. Recovery
			// must truncate and rebuild that same stage without touching target.
			query("TRUNCATE TABLE scratch." + stage)
			query(fmt.Sprintf("INSERT INTO scratch.%s SELECT * FROM scratch.%s WHERE ID=0", stage, target))
			assertPhysical(stage, 1, 1)
			recoveryStart := time.Now()
			recovered, e := client.BuildDeltaStage(ctx, DeltaPlan{
				Target: target, Delta: delta, Stage: stage, Key: "ID",
				Lower: 0, Upper: uint64(changed), BaselineUUID: uuid,
			})
			recoveryDuration := time.Since(recoveryStart)
			if e != nil || recovered != stat {
				t.Fatalf("batch=%d trial=%d partial-stage recovery failed: %+v %v", changed, trial, recovered, e)
			}
			if current, uuidErr := client.UUID(ctx, target); uuidErr != nil || current != uuid {
				t.Fatalf("target changed before publish: %q %v", current, uuidErr)
			}
			raw, e := client.Query(ctx, fmt.Sprintf("SELECT count(),uniqExact(ID),sum(Revenue) FROM scratch.%s FORMAT TabSeparatedRaw", stage))
			if e != nil {
				t.Fatal(e)
			}
			stageUUID, e := client.UUID(ctx, stage)
			if e != nil || stageUUID == "" {
				t.Fatalf("stage UUID: %v %q", e, stageUUID)
			}
			parts := strings.Split(raw, "	")
			if len(parts) != 3 {
				t.Fatalf("invalid 3-column reconciliation %q", raw)
			}
			observed := make([]uint64, 3)
			for i := range parts {
				observed[i], e = strconv.ParseUint(parts[i], 10, 64)
				if e != nil {
					t.Fatal(e)
				}
			}
			expectedSum := uint64(baseline)*uint64(baseline+1)/2 + uint64(changed)*1000000
			if observed[0] != uint64(baseline) || observed[1] != uint64(baseline) || observed[2] != expectedSum {
				t.Fatalf("mismatch changed=%d trial=%d got=%v want rows=%d sum=%d",
					changed, trial, observed, baseline, expectedSum)
			}
			publication := Publication{Target: target, Stage: stage, StageUUID: stageUUID, OldUUID: uuid}
			publishStart := time.Now()
			if e = client.Publish(ctx, publication); e != nil {
				t.Fatalf("publish changed=%d trial=%d: %v", changed, trial, e)
			}
			publishDuration := time.Since(publishStart)
			if current, uuidErr := client.UUID(ctx, target); uuidErr != nil || current != stageUUID {
				t.Fatalf("published UUID mismatch: %q %v", current, uuidErr)
			}
			assertPhysical(target, uint64(baseline), expectedSum)
			if e = client.Publish(ctx, publication); e != nil {
				t.Fatalf("publish replay changed=%d trial=%d: %v", changed, trial, e)
			}
			rollbackStart := time.Now()
			if e = client.Rollback(ctx, publication); e != nil {
				t.Fatalf("rollback changed=%d trial=%d: %v", changed, trial, e)
			}
			rollbackDuration := time.Since(rollbackStart)
			if current, uuidErr := client.UUID(ctx, target); uuidErr != nil || current != uuid {
				t.Fatalf("rollback UUID mismatch: %q %v", current, uuidErr)
			}
			assertPhysical(target, uint64(baseline), uint64(baseline)*uint64(baseline+1)/2)
			if e = client.Rollback(ctx, publication); e != nil {
				t.Fatalf("rollback replay changed=%d trial=%d: %v", changed, trial, e)
			}
			ms := float64(dur.Microseconds()) / 1000
			elapsed = append(elapsed, ms)
			sample := deltaQAMeasure{
				Kind: "copy_on_write_stage", ChangedRows: changed, Trial: trial,
				BaselineRows: baseline, Events: stat.Events, OutputRows: stat.Rows,
				StageMS: ms, RecoveryMS: float64(recoveryDuration.Microseconds()) / 1000,
				ChangedRowsPerSecond: float64(changed) / dur.Seconds(), Reconciliation: "PASS",
				PublishMS: float64(publishDuration.Microseconds()) / 1000, RollbackMS: float64(rollbackDuration.Microseconds()) / 1000,
			}
			all = append(all, sample)
			encoded, _ := json.Marshal(sample)
			t.Logf("DELTA_QA_METRIC %s", encoded)
			// Only uniquely named scratch stages owned by this benchmark are dropped.
			query("DROP TABLE scratch." + stage)
		}
		sort.Float64s(elapsed)
		t.Logf("DELTA_QA_SUMMARY changed=%d trials=%d median_ms=%.3f min_ms=%.3f max_ms=%.3f",
			changed, trials, elapsed[1], elapsed[0], elapsed[2])
	}
	if output := os.Getenv("DATAFLOW_BENCHMARK_RESULT_FILE"); output != "" {
		if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
			t.Fatal(err)
		}
		var lines strings.Builder
		for _, sample := range all {
			encoded, _ := json.Marshal(sample)
			lines.Write(encoded)
			lines.WriteByte(10)
		}
		if err = os.WriteFile(output, []byte(lines.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
