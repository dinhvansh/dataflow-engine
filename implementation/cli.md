# DataFlow CLI v0.1

Go standard-library client; does not directly read databases.

```bash
go build -o dataflowctl ./cmd/dataflowctl
export DATAFLOW_API_URL=http://127.0.0.1:8080
export DATAFLOW_API_TOKEN=your-local-test-token
./dataflowctl health
./dataflowctl status 00000000-0000-0000-0000-000000000001
./dataflowctl test-load --schema Erp --table Part --database scratch --target part_test --batch 1000 --workers 2 --queue 4 --confirm-disposable
```

Experimental test-load is synchronous and append-only; backend independently requires DATAFLOW_ALLOW_TEST_LOAD=true. CLI refuses to start without explicit disposable-target acknowledgment. Source should use a read-only SQL Server login. Production CLI commands (job list, scheduler, cancellation, checkpoint, reconciliation) await actual APIs.


## v0.2 additions

```bash
dataflowctl tables Erp
dataflowctl columns Erp PartTran
dataflowctl rowversion-upper
dataflowctl runs
dataflowctl validate-pipeline pipeline.json
dataflowctl test-load --schema Erp --table Part --database scratch --target part_test --atomic-replace --confirm-disposable
```
`--atomic-replace` defaults to true and requires an Atomic ClickHouse scratch database plus SQL Server SNAPSHOT isolation. CLI remains a bootstrap client, not the final job-management CLI.
