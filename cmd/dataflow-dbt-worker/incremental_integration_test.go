package main

import (
	"context"
	"fmt"
	"github.com/dinhvansh/dataflow-engine/internal/dbtworker"
	"github.com/dinhvansh/dataflow-engine/internal/safetable"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSafeIncrementalFullIntegration(t *testing.T) {
	dsn := os.Getenv("DATAFLOW_INTEGRATION_POSTGRES_DSN")
	endpoint := os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
	if dsn == "" || endpoint == "" {
		t.Skip("requires disposable PostgreSQL and ClickHouse")
	}
	if os.Getenv("DATAFLOW_INTEGRATION_PG_ISOLATED") != "true" {
		t.Fatal("refusing non-isolated database")
	}
	ctx, done := context.WithTimeout(context.Background(), 3*time.Minute)
	defer done()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	schema, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", "014_safe_incremental.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pg.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	historyMigration, e := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", "015_incremental_history_guard.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pg.Exec(ctx, string(historyMigration)); e != nil {
		t.Fatal(e)
	}
	t.Setenv("DATAFLOW_ENABLE_INCREMENTAL_HISTORY_GUARD_TEST", "true")
	t.Setenv("DATAFLOW_ALLOW_TEST_LOAD", "true")
	t.Setenv("DATAFLOW_ENABLE_DBT_TEST", "true")
	t.Setenv("DATAFLOW_ENABLE_SAFE_TABLE_TEST", "true")
	t.Setenv("DATAFLOW_ENABLE_INCREMENTAL_TEST", "true")
	t.Setenv("DATAFLOW_TEST_TARGET_DATABASE", "scratch")
	if err = checkIncrementalSchema(ctx, pg); err != nil {
		t.Fatal(err)
	}
	host := os.Getenv("DATAFLOW_INTEGRATION_CH_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 8123
	if raw := os.Getenv("DATAFLOW_INTEGRATION_CH_PORT"); raw != "" {
		if port, err = strconv.Atoi(raw); err != nil {
			t.Fatal(err)
		}
	}
	cfg := dbtworker.Config{Host: host, Port: port, Database: "scratch", Username: os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_USER"), Password: os.Getenv("DATAFLOW_INTEGRATION_CH_WRITER_PASSWORD")}
	cli := tableClickhouse(cfg)
	seed := fmt.Sprintf("incfull%d", time.Now().UnixNano())
	target := "dft_" + seed
	delta := "dfd_" + seed
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		for _, name := range []string{target, delta} {
			_, _ = cli.Query(cleanup, "DROP TABLE IF EXISTS scratch."+name)
		}
	}()
	for _, q := range []string{
		"CREATE TABLE scratch." + target + " (ID String, Revenue UInt64) ENGINE=MergeTree ORDER BY ID",
		"INSERT INTO scratch." + target + " VALUES ('A',10),('B',20),('C',30)",
		"CREATE TABLE scratch." + delta + " (ID String, Revenue UInt64, _df_version UInt64, _df_deleted UInt8) ENGINE=MergeTree ORDER BY (ID,_df_version)",
		"INSERT INTO scratch." + delta + " VALUES ('A',99,1,0),('B',20,2,1),('D',40,3,0),('A',100,4,0)",
	} {
		if _, err = cli.Query(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var firstUUID string
	firstUUID, err = cli.UUID(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pg.Exec(ctx, `INSERT INTO incremental_targets(target_name,delta_table,key_column,cursor_value,target_uuid)
 VALUES($1,$2,'ID',0,$3)`, target, delta, firstUUID)
	if err != nil {
		t.Fatal(err)
	}
	enqueue := func() string {
		t.Helper()
		var id string
		if err = pg.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		_, e := pg.Exec(ctx, `INSERT INTO incremental_runs(
  id,target_name,delta_table,key_column,stage_name,cursor_lower)
  SELECT $1::uuid,target_name,delta_table,key_column,$2,cursor_value FROM incremental_targets WHERE target_name=$3`,
			id, safetable.StageName(id), target)
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	state := func(id string) string {
		t.Helper()
		var status string
		if e := pg.QueryRow(ctx, "SELECT state FROM incremental_runs WHERE id=$1::uuid", id).Scan(&status); e != nil {
			t.Fatal(e)
		}
		return status
	}
	checkpoint := func() string {
		t.Helper()
		var cursor string
		if e := pg.QueryRow(ctx, "SELECT cursor_value::text FROM incremental_targets WHERE target_name=$1", target).Scan(&cursor); e != nil {
			t.Fatal(e)
		}
		return cursor
	}
	dispatch := func(expect string) {
		t.Helper()
		c, e := claimIncremental(ctx, pg)
		if e != nil || c.State != expect {
			t.Fatalf("claim state %+v err %v", c, e)
		}
		runIncrementalClaim(ctx, pg, c, cli)
	}
	publish := func(id string) {
		t.Helper()
		if _, err = pg.Exec(ctx, `UPDATE incremental_runs SET state='publishing',attempt_count=0 WHERE id=$1::uuid AND state='ready'`, id); err != nil {
			t.Fatal(err)
		}
		dispatch("publishing")
		if state(id) != "published" {
			t.Fatalf("publish did not finish: %s", state(id))
		}
	}
	first := enqueue()
	dispatch("building")
	if state(first) != "ready" || checkpoint() != "0" {
		t.Fatalf("staging advanced cursor: %s / %s", state(first), checkpoint())
	}
	publish(first)
	if checkpoint() != "4" {
		t.Fatalf("first cursor wrong %s", checkpoint())
	}
	// Guard cannot be disabled after publishing a witnessed target checkpoint.
	t.Setenv("DATAFLOW_ENABLE_INCREMENTAL_HISTORY_GUARD_TEST", "false")
	downgraded := enqueue()
	dispatch("building")
	if state(downgraded) != "failed" || checkpoint() != "4" {
		t.Fatalf("guard downgrade unexpectedly succeeded: %s/%s", state(downgraded), checkpoint())
	}
	t.Setenv("DATAFLOW_ENABLE_INCREMENTAL_HISTORY_GUARD_TEST", "true")
	out, err := cli.Query(ctx, "SELECT ID,Revenue FROM scratch."+target+" ORDER BY ID FORMAT TabSeparatedRaw")
	if err != nil || out != "A\t100\nC\t30\nD\t40" {
		t.Fatalf("first materialization wrong: %s %v", out, err)
	}
	// Stage may be replayed from scratch, but publish replay must be no-op.
	var stageName, stageUUID, oldUUID string
	if err = pg.QueryRow(ctx, `SELECT stage_name,stage_uuid,old_target_uuid FROM incremental_runs WHERE id=$1::uuid`, first).Scan(&stageName, &stageUUID, &oldUUID); err != nil {
		t.Fatal(err)
	}
	if err = cli.Publish(ctx, safetable.Publication{Target: target, Stage: stageName, StageUUID: stageUUID, OldUUID: oldUUID}); err != nil {
		t.Fatal(err)
	}
	if checkpoint() != "4" {
		t.Fatal("replay advanced cursor twice")
	}
	if _, err = cli.Query(ctx, "INSERT INTO scratch."+delta+" VALUES ('C',300,5,0),('D',40,6,1),('E',50,7,0)"); err != nil {
		t.Fatal(err)
	}
	second := enqueue()
	dispatch("building")
	if state(second) != "ready" || checkpoint() != "4" {
		t.Fatalf("second staging changed cursor %s", checkpoint())
	}
	publish(second)
	if checkpoint() != "7" {
		t.Fatalf("second cursor wrong %s", checkpoint())
	}
	out, err = cli.Query(ctx, "SELECT ID,Revenue FROM scratch."+target+" ORDER BY ID FORMAT TabSeparatedRaw")
	if err != nil || out != "A\t100\nC\t300\nE\t50" {
		t.Fatalf("second merge wrong: %s %v", out, err)
	}
	if _, err = pg.Exec(ctx, `UPDATE incremental_runs SET state='rolling_back',attempt_count=0 WHERE id=$1::uuid`, second); err != nil {
		t.Fatal(err)
	}
	dispatch("rolling_back")
	if state(second) != "rolled_back" || checkpoint() != "4" {
		t.Fatalf("rollback cursor wrong: %s/%s", state(second), checkpoint())
	}
	out, err = cli.Query(ctx, "SELECT ID,Revenue FROM scratch."+target+" ORDER BY ID FORMAT TabSeparatedRaw")
	if err != nil || out != "A\t100\nC\t30\nD\t40" {
		t.Fatalf("rollback dataset wrong: %s %v", out, err)
	}
	// Source still has old versions (5,6,7): a new run after rollback can
	// replay the window without double-counting or losing tombstones.
	replay := enqueue()
	dispatch("building")
	if state(replay) != "ready" || checkpoint() != "4" {
		t.Fatal("replay lost cursor")
	}
	t.Log("PASS insert/update/delete, two cursor commits, preserved backup and cursor rollback")
	// INC-11: a late append below the restored cursor must not be published.
	if _, err = cli.Query(ctx, "INSERT INTO scratch."+delta+" VALUES ('B',999,0,0)"); err != nil {
		t.Fatal(err)
	}
	if _, err = pg.Exec(ctx, `UPDATE incremental_runs SET state='publishing',attempt_count=0
  WHERE id=$1::uuid AND state='ready'`, replay); err != nil {
		t.Fatal(err)
	}
	dispatch("publishing")
	if state(replay) == "published" || checkpoint() != "4" {
		t.Fatalf("late CDC event was silently published; state=%s cursor=%s", state(replay), checkpoint())
	}
	// Requeue on the disposable fixture to verify detection also works when
	// there are no new high-version events. Never mutate production metadata.
	if _, err = pg.Exec(ctx, `UPDATE incremental_runs SET state='failed',
  lease_owner=NULL,lease_until=NULL WHERE id=$1::uuid`, replay); err != nil {
		t.Fatal(err)
	}
	late := enqueue()
	dispatch("building")
	if state(late) != "failed" || checkpoint() != "4" {
		t.Fatalf("late event was silently skipped on new run; state=%s cursor=%s", state(late), checkpoint())
	}
	t.Log("PASS INC-11: late event blocks publish and subsequent incremental run")
	// Only test-owned scratch staging tables are removed.
	for _, id := range []string{first, downgraded, second, replay, late} {
		var st string
		_ = pg.QueryRow(ctx, "SELECT stage_name FROM incremental_runs WHERE id=$1::uuid", id).Scan(&st)
		if st != "" {
			_, _ = cli.Query(ctx, "DROP TABLE IF EXISTS scratch."+st)
		}
	}
}
