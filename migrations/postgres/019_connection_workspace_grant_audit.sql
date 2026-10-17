-- V3.2B-5B: append-only lifecycle evidence for delegated workspace scopes.
-- Apply ONLY in identity-verified isolated control-PostgreSQL QA at this stage.
-- MANAGE bootstrapping remains an out-of-band trusted-operator procedure.
CREATE TABLE IF NOT EXISTS connection_security_grant_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 0 AND 128),
 target_actor_id text NOT NULL CHECK (target_actor_id ~ '^[0-9a-f]{64}$'),
 scope text NOT NULL CHECK (scope IN ('VIEW','USE','TEST')),
 action text NOT NULL CHECK (action IN ('granted','revoked')),
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS connection_security_grant_audit_workspace
 ON connection_security_grant_audit(workspace_id, recorded_at DESC, id);
DROP TRIGGER IF EXISTS connection_security_grant_audit_immutable ON connection_security_grant_audit;
CREATE TRIGGER connection_security_grant_audit_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_grant_audit
FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
