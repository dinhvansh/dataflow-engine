# Environment

- Linux container runtime, 3 CPU, 9.7 GiB RAM, no swap, 32 GiB filesystem.
- Docker 28.4.0; Compose 2.40.3; Go tests in Go 1.23.12 container; Node 24.19.0; npm 11.9.0; Python 3.12.14.
- QA project/network: `dataflow-localqa` / `dataflow-localqa-net`.
- SQL Server 2022 fixture `DataFlowFixtureQA`: `127.0.0.1:11433`.
- PostgreSQL 16 `dataflow_qa`: `127.0.0.1:15433`; migrations 001–014, 17 public tables.
- ClickHouse 25.3 `scratch` Atomic: `127.0.0.1:18123`.
- API: `127.0.0.1:18080`; dbt worker pinned dbt-core 1.11.0 / dbt-clickhouse 1.10.3.
- All services carry `com.dataflow.role=local-qa` and `com.dataflow.owner=dataflow-localqa`.
- `.env.qa.local` is mode 600 and ignored. No production DSN or Epicor credentials were used.

Raw proof: `artifacts/environment.txt`.

