-- V3.3A: IMMUTABLE non-executable connection binding plan snapshots.
-- Isolated identity-verified synthetic PostgreSQL ONLY. Does not change job
-- runtime, read SQL Server, dial a target, or activate any Draft.
CREATE UNIQUE INDEX IF NOT EXISTS connection_security_profile_workspace_key
 ON connection_security_profiles(id, workspace_id);

CREATE TABLE IF NOT EXISTS connection_security_binding_plans (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 pipeline_id text NOT NULL CHECK (pipeline_id ~ '^[A-Za-z][A-Za-z0-9_-]{0,63}$'),
 node_id text NOT NULL CHECK (node_id ~ '^[A-Za-z][A-Za-z0-9_-]{0,63}$'),
 profile_id uuid NOT NULL,
 profile_revision bigint NOT NULL CHECK (profile_revision > 0),
 role text NOT NULL CHECK (role IN ('source','destination')),
 status text NOT NULL DEFAULT 'planned' CHECK (status = 'planned'),
 actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 128),
 created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT connection_binding_workspace_fk FOREIGN KEY(profile_id,workspace_id)
 REFERENCES connection_security_profiles(id,workspace_id),
 UNIQUE(workspace_id,pipeline_id,node_id)
);
CREATE INDEX IF NOT EXISTS connection_security_binding_by_profile
 ON connection_security_binding_plans(profile_id,workspace_id);
DROP TRIGGER IF EXISTS connection_security_binding_immutable ON connection_security_binding_plans;
CREATE TRIGGER connection_security_binding_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_binding_plans
FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
