-- Transformation Studio visual models: metadata only.
-- No mart writes or dbt execution are granted by this migration.
CREATE TABLE IF NOT EXISTS transform_models (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE CHECK(length(name) BETWEEN 1 AND 120),
 definition jsonb NOT NULL,
 compiled_sql text NOT NULL CHECK(length(compiled_sql) BETWEEN 1 AND 16384),
 updated_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS transform_models_updated_idx ON transform_models(updated_at DESC);
