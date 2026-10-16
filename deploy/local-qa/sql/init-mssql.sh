#!/usr/bin/env bash
set -euo pipefail
sqlcmd=/opt/mssql-tools18/bin/sqlcmd
"$sqlcmd" -C -b -S mssql -U sa -P "$MSSQL_SA_PASSWORD" -v SeedPassword="$QA_SEED_PASSWORD" ReaderPassword="$QA_READER_PASSWORD" -i /qa/sql/001_fixture.sql
"$sqlcmd" -C -b -S mssql -U sa -P "$MSSQL_SA_PASSWORD" -d DataFlowFixtureQA -i /qa/sql/002_seed.sql

