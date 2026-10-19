-- Isolated QA runtime connector profiles. Only enable through explicit lab flags.
-- Encrypted secrets are stored as opaque AES-GCM bytes with key outside PostgreSQL.
-- No free-form execution or access to actual ERP is granted by this migration.
CREATE TABLE IF NOT EXISTS connection_runtime_profiles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace text NOT NULL DEFAULT 'lab' CHECK (workspace='lab'),
 name text NOT NULL UNIQUE CHECK(length(name) BETWEEN 1 AND 80),
 driver text NOT NULL CHECK(driver IN ('sqlserver','clickhouse')),
 kind text NOT NULL CHECK(kind IN ('source','destination')),
 host text NOT NULL CHECK(length(host) BETWEEN 1 AND 253),
 port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
 database_name text NOT NULL CHECK(length(database_name) BETWEEN 1 AND 128),
 username text NOT NULL CHECK(length(username) BETWEEN 1 AND 128),
 secret_encrypted bytea NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','verified')),
 revision bigint NOT NULL DEFAULT 1,
 checked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((driver='sqlserver' AND kind='source') OR (driver='clickhouse' AND kind='destination'))
);
CREATE TABLE IF NOT EXISTS connection_runtime_bindings (
 stream_id uuid PRIMARY KEY REFERENCES sync_streams(id) ON DELETE RESTRICT,
 source_profile_id uuid NOT NULL REFERENCES connection_runtime_profiles(id),
 target_profile_id uuid NOT NULL REFERENCES connection_runtime_profiles(id),
 source_revision bigint NOT NULL,
 target_revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
