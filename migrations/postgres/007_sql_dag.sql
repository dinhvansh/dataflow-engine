-- v0.5 experimental SELECT-only ClickHouse SQL jobs and runnable DAGs.
CREATE TABLE IF NOT EXISTS sql_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 120),
 database_name text NOT NULL,
 query_text text NOT NULL CHECK (length(query_text) BETWEEN 1 AND 16384),
 enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS dag_definitions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 120),
 definition jsonb NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS dag_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 dag_id uuid NOT NULL REFERENCES dag_definitions(id),
 status text NOT NULL CHECK(status IN ('queued','running','succeeded','failed')),
 result jsonb,
 error_message text,
 created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS dag_runs_by_dag ON dag_runs(dag_id,created_at DESC);
