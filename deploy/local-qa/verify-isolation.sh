#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); root=$(cd "$here/../.." && pwd); cd "$root"
set -a; source .env.qa.local; set +a
for c in dataflow-localqa-mssql dataflow-localqa-postgres dataflow-localqa-clickhouse; do
  [[ "$(docker inspect -f '{{index .Config.Labels "com.dataflow.owner"}}' "$c")" == dataflow-localqa ]] || { echo "bad owner label: $c" >&2; exit 1; }
done
docker exec dataflow-localqa-mssql /opt/mssql-tools18/bin/sqlcmd -C -b -S localhost -U qa_erp_reader -P "$QA_READER_PASSWORD" -d DataFlowFixtureQA -Q "SET NOCOUNT ON; SELECT DB_NAME(),COUNT_BIG(*) FROM Erp.PartTran;"
if docker exec dataflow-localqa-mssql /opt/mssql-tools18/bin/sqlcmd -C -b -S localhost -U qa_erp_reader -P "$QA_READER_PASSWORD" -d DataFlowFixtureQA -Q "BEGIN TRAN; UPDATE Erp.PartTran SET CommentText=N'forbidden' WHERE TranNum=1; ROLLBACK" >/tmp/dataflow-qa-deny.log 2>&1; then echo 'readonly MSSQL user unexpectedly wrote data' >&2; exit 1; fi
docker exec dataflow-localqa-postgres psql -U dataflow_qa -d dataflow_qa -Atc "select current_database(),current_user,count(*) from information_schema.tables where table_schema='public' group by 1,2"
curl -fsS -u "qa_admin:$CLICKHOUSE_ADMIN_PASSWORD" --data-binary "SELECT currentDatabase(),engine FROM system.databases WHERE name='scratch' FORMAT TSV" 'http://127.0.0.1:18123/?database=scratch'
echo 'Isolation and read-only checks PASS'

