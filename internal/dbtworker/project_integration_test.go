package dbtworker

import (
	"context"
	"fmt"
	"github.com/dinhvansh/dataflow-engine/internal/connectors/clickhouse"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDBTRealClickHouseIntegration(t *testing.T) {
	endpoint := os.Getenv("DATAFLOW_INTEGRATION_CH_URL")
	if endpoint == "" || os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN") == "" {
		t.Skip("requires disposable ClickHouse and installed dbt CLI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	timestamp := time.Now().UnixNano()
	source := fmt.Sprintf("df_dbt_source_%d", timestamp)
	name := fmt.Sprintf("ci_view_%d", timestamp)
	target := "dfv_" + name
	writerUser := os.Getenv("DATAFLOW_INTEGRATION_CH_USER")
	writerPass := os.Getenv("DATAFLOW_INTEGRATION_CH_PASSWORD")
	sql := func(statement string) error {
		u, err := url.Parse(endpoint)
		if err != nil {
			return err
		}
		q := u.Query()
		q.Set("query", statement)
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
		if err != nil {
			return err
		}
		if writerUser != "" {
			req.SetBasicAuth(writerUser, writerPass)
		}
		res, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode/100 != 2 {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			return fmt.Errorf("ClickHouse status %d: %s", res.StatusCode, body)
		}
		return nil
	}
	if err := sql(fmt.Sprintf("CREATE TABLE scratch.%s (Customer String, Amount UInt64) ENGINE=MergeTree ORDER BY Customer", source)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = sql("DROP VIEW IF EXISTS scratch." + target)
		_ = sql("DROP TABLE IF EXISTS scratch." + source)
	}()
	if err := sql(fmt.Sprintf("INSERT INTO scratch.%s VALUES ('A',10),('A',20),('B',5)", source)); err != nil {
		t.Fatal(err)
	}
	host := os.Getenv("DATAFLOW_INTEGRATION_CH_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 8123
	if raw := os.Getenv("DATAFLOW_INTEGRATION_CH_PORT"); raw != "" {
		var err error
		if port, err = strconv.Atoi(raw); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{Binary: os.Getenv("DATAFLOW_INTEGRATION_DBT_BIN"), Host: host, Port: port, Database: "scratch", Username: writerUser, Password: writerPass}
	p := Plan{Name: name, Target: target, SQL: "SELECT Customer, sum(Amount) AS Total FROM scratch." + source + " GROUP BY Customer"}
	result, err := Execute(ctx, p, cfg)
	if err != nil {
		t.Fatalf("real dbt failed: %v; log=%s", err, result.Log)
	}
	if !result.Success || len(result.Results) == 0 {
		t.Fatalf("dbt missing structured run_results: %+v", result)
	}
	reader := clickhouse.QueryJob{Endpoint: endpoint, Database: "scratch", Username: writerUser, Password: writerPass}
	data, err := reader.Execute(ctx, "SELECT Customer, Total FROM scratch."+target+" ORDER BY Customer")
	if err != nil {
		t.Fatal(err)
	}
	if data.Rows != 2 || len(data.Data) != 2 || data.Data[0]["Customer"] != "A" {
		t.Fatalf("view not materialized correctly: %+v", data)
	}
	t.Log("dbt created real scratch view and exposed it through ClickHouse")
}
func TestDBTTemplateGuard(t *testing.T) {
	p := Plan{Name: "safe", Target: "dfv_safe", SQL: "SELECT '{{ config(materialized=\"table\") }}' AS unsafe"}
	err := ValidatePlan(p, Config{Host: "localhost", Port: 8123, Database: "scratch", Username: "u"})
	if err == nil || !strings.Contains(err.Error(), "template") {
		t.Fatalf("templating not blocked: %v", err)
	}
}
