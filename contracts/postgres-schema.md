# PostgreSQL metadata schema — preliminary DDL

All business data stays in ClickHouse; PostgreSQL stores metadata, state and audit.

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE connections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE,
 kind text NOT NULL CHECK (kind IN ('sqlserver','clickhouse')),
 config jsonb NOT NULL DEFAULT '{}',
 secret_ref text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE job_definitions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE,
 job_type text NOT NULL CHECK (job_type IN ('sync','sql','quality','pipeline','webhook')),
 current_version integer NOT NULL DEFAULT 1,
 enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE job_versions (
 job_id uuid NOT NULL REFERENCES job_definitions(id),
 version integer NOT NULL,
 config jsonb NOT NULL,
 config_hash text NOT NULL,
 PRIMARY KEY(job_id,version)
);
CREATE TABLE streams (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 job_id uuid NOT NULL REFERENCES job_definitions(id),
 source_table text NOT NULL,
 destination_table text NOT NULL,
 schema_fingerprint text NOT NULL
);
CREATE TABLE checkpoints (
 stream_id uuid PRIMARY KEY REFERENCES streams(id),
 state jsonb NOT NULL,
 generation bigint NOT NULL DEFAULT 0,
 lease_fence bigint NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE job_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 job_id uuid NOT NULL REFERENCES job_definitions(id),
 job_version integer NOT NULL,
 status text NOT NULL CHECK (status IN ('queued','running','succeeded','failed','cancelled')),
 config_snapshot jsonb NOT NULL,
 attempt integer NOT NULL DEFAULT 1,
 started_at timestamptz, finished_at timestamptz,
 error_code text,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(job_id,job_version) REFERENCES job_versions(job_id,version)
);
CREATE TABLE run_leases (
 resource_key text PRIMARY KEY,
 run_id uuid NOT NULL REFERENCES job_runs(id),
 owner_id text NOT NULL,
 fencing_token bigint NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE TABLE batch_commits (
 run_id uuid NOT NULL REFERENCES job_runs(id),
 stream_id uuid NOT NULL REFERENCES streams(id),
 seq bigint NOT NULL,
 status text NOT NULL CHECK(status IN ('staged','written','verified')),
 lower_key jsonb, upper_key jsonb,
 row_count bigint NOT NULL DEFAULT 0,
 acked_at timestamptz,
 PRIMARY KEY(run_id,stream_id,seq)
);
CREATE TABLE schedules (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 job_id uuid NOT NULL REFERENCES job_definitions(id),
 cron text NOT NULL,
 time_zone text NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
 enabled boolean NOT NULL DEFAULT true,
 next_run_at timestamptz NOT NULL
);
CREATE TABLE audit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 actor text NOT NULL,
 action text NOT NULL,
 target_id text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}'
);
```

Checkpoint advancement requires compare-and-swap generation, matching lease-fencing token and atomic PostgreSQL transaction. Freeze run configuration version. Add DAG node state/outbox migrations before that feature ships. Never save plaintext credentials.
