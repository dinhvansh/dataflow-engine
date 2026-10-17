-- V3.2B-2: disconnected security metadata. Apply ONLY to isolated TEST PostgreSQL.
-- Deliberately does not migrate V3.2A 'lab' records, enable endpoints or bind runtime.
-- Server-only vault references: NO secret values, DSNs, hosts, usernames or URLs.
CREATE TABLE IF NOT EXISTS connection_security_workspaces (
  id text PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9_-]{2,62}$'),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS connection_security_grants (
  workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
  actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 128 AND actor_id !~ '[[:space:][:cntrl:]]' AND actor_id <> '*'),
  scope text NOT NULL CHECK (scope IN ('VIEW','USE','MANAGE','TEST')),
  granted_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY(workspace_id, actor_id, scope),
  CHECK (revoked_at IS NULL OR revoked_at >= granted_at)
);
CREATE INDEX IF NOT EXISTS connection_security_grants_active
  ON connection_security_grants(workspace_id,actor_id,scope) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS connection_security_profiles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  driver text NOT NULL CHECK (driver IN ('sqlserver','clickhouse')),
  kind text NOT NULL CHECK (kind IN ('source','destination')),
  status text NOT NULL DEFAULT 'draft' CHECK (status = 'draft'),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision >= 1),
  secret_handle text CHECK (secret_handle IS NULL OR secret_handle ~ '^ref/[A-Za-z0-9_/-]{4,120}$'),
  secret_version bigint CHECK (secret_version IS NULL OR secret_version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT connection_security_role CHECK (
    (driver='sqlserver' AND kind='source') OR (driver='clickhouse' AND kind='destination')),
  CONSTRAINT connection_security_reference_pair CHECK (
    (secret_handle IS NULL) = (secret_version IS NULL)),
  UNIQUE (workspace_id,name)
);
CREATE INDEX IF NOT EXISTS connection_security_profiles_workspace
  ON connection_security_profiles(workspace_id,created_at DESC,id);

-- Audit has no credential, endpoint, opaque vault handle or arbitrary payload.
CREATE TABLE IF NOT EXISTS connection_security_audit (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
  actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 128),
  connection_id uuid,
  action text NOT NULL CHECK (action IN ('draft_created','reference_staged','reference_rotated')),
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS connection_security_audit_workspace
  ON connection_security_audit(workspace_id,recorded_at DESC,id);

CREATE OR REPLACE FUNCTION connection_security_block_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'connection security audit records are append-only';
END;
$$;
DROP TRIGGER IF EXISTS connection_security_audit_immutable ON connection_security_audit;
CREATE TRIGGER connection_security_audit_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_audit
FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
