# ClickHouse destination semantics

Pipeline: SQL Server → run-scoped staging → raw history → current-state ODS → data mart.

## Type mapping
- SQL Server rowversion binary(8): FixedString(8) or hex; preserve byte order.
- uniqueidentifier: UUID/canonical text; verify SQL Server sort semantics.
- decimal: ClickHouse Decimal with validation.
- datetime2: DateTime64 with explicit timezone rules.
- nullable/varbinary: preserve NULL and raw bytes.

## Upsert
ReplacingMergeTree merges asynchronously; plain queries can show several versions. V1 stores append-only changes and exposes version-aware current-state queries such as argMax. Other choices require benchmarking.

## Idempotency
Identify logical changes using stream, source key, version and operation. Identify retried batches by run and sequence. Stage full refresh and publish only after validation. Detect/represent physical deletions using reconciliation records and explicit precedence over delayed writes. Do not fabricate source rowversion for deleted records.

## Schema evolution
Persist schema fingerprint; review additive columns and pause on breaking type changes.
