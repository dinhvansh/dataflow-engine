-- V3.2A: non-executable connection *names only*, confined to the lab workspace.
-- Deliberately no host, port, DSN, username, secret, credential reference,
-- activation, binding or arbitrary user-controlled network endpoint.
CREATE TABLE IF NOT EXISTS connection_definitions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
 driver text NOT NULL CHECK (driver IN ('sqlserver', 'clickhouse')),
 kind text NOT NULL CHECK (kind IN ('source', 'destination')),
 workspace text NOT NULL DEFAULT 'lab' CHECK (workspace = 'lab'),
 status text NOT NULL DEFAULT 'draft' CHECK (status = 'draft'),
 revision integer NOT NULL DEFAULT 1 CHECK (revision = 1),
 created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT connection_definitions_driver_role CHECK (
   (driver = 'sqlserver' AND kind = 'source') OR
   (driver = 'clickhouse' AND kind = 'destination')
 )
);
CREATE TABLE IF NOT EXISTS connection_definition_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 definition_id uuid NOT NULL REFERENCES connection_definitions(id),
 action text NOT NULL CHECK (action = 'created'),
 occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS connection_definitions_by_created
 ON connection_definitions(created_at DESC, id);
