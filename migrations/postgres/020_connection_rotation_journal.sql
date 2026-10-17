-- V3.2B-5C: durable, disconnected Vault→metadata reconciliation intents.
-- NEVER stores credential bytes. Apply only on independently confirmed
-- disposable control-PostgreSQL until provider/IdP security is approved.
CREATE TABLE IF NOT EXISTS connection_security_rotation_journal (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 connection_id uuid NOT NULL REFERENCES connection_security_profiles(id),
 actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 128),
 secret_handle text NOT NULL CHECK (secret_handle ~ '^ref/[A-Za-z0-9_/-]{4,120}$'),
 from_version bigint NOT NULL CHECK (from_version > 0),
 target_version bigint NOT NULL CHECK (target_version > 1),
 expected_revision bigint NOT NULL CHECK (expected_revision > 0),
 state text NOT NULL DEFAULT 'prepared'
   CHECK (state IN ('prepared','applied','needs_review')),
 created_at timestamptz NOT NULL DEFAULT now(),
 finalized_at timestamptz,
 CHECK (target_version = from_version + 1),
 CHECK ((state = 'prepared' AND finalized_at IS NULL)
    OR (state IN ('applied','needs_review') AND finalized_at IS NOT NULL))
);
-- Exactly one unfinished rotation per profile. An operator must reconcile a
-- needs_review intent before new rotation; auto-clearing may lose data.
CREATE UNIQUE INDEX IF NOT EXISTS connection_security_rotation_one_pending
 ON connection_security_rotation_journal(connection_id)
 WHERE state IN ('prepared','needs_review');
CREATE INDEX IF NOT EXISTS connection_security_rotation_workspace
 ON connection_security_rotation_journal(workspace_id,created_at DESC,id);

-- Journal evidence itself can only transition through a trusted server flow;
-- append-only transition events retain immutable history.
CREATE TABLE IF NOT EXISTS connection_security_rotation_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 rotation_id uuid NOT NULL REFERENCES connection_security_rotation_journal(id),
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 128),
 action text NOT NULL CHECK (action IN ('prepared','applied','needs_review')),
 created_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS connection_security_rotation_events_immutable ON connection_security_rotation_events;
CREATE TRIGGER connection_security_rotation_events_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_rotation_events
FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
