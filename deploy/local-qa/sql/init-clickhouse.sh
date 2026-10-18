#!/usr/bin/env bash
set -euo pipefail
ch=(clickhouse-client --host clickhouse --user qa_admin --password "$CLICKHOUSE_ADMIN_PASSWORD" --multiquery)
"${ch[@]}" --query "CREATE DATABASE IF NOT EXISTS scratch ENGINE=Atomic; CREATE USER IF NOT EXISTS qa_ro IDENTIFIED WITH sha256_password BY '$CLICKHOUSE_READER_PASSWORD'; CREATE USER IF NOT EXISTS qa_dbt_writer IDENTIFIED WITH sha256_password BY '$CLICKHOUSE_WRITER_PASSWORD'; GRANT SHOW PROCESSLIST TO qa_admin; GRANT SELECT ON scratch.* TO qa_ro; GRANT SELECT,INSERT,CREATE TABLE,CREATE VIEW,DROP TABLE,ALTER,TRUNCATE,SHOW ON scratch.* TO qa_dbt_writer;"

