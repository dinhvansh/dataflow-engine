-- V3.2B-5E-3: explicitly signed, two-operator MANAGE bootstrap audit.
-- NEVER run against a real control DB without independent security approval.
-- This migration is exercised only in disposable, identity-verified CI PG.
CREATE TABLE IF NOT EXISTS connection_security_manage_bootstrap (
  request_id uuid PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
  target_actor_id text NOT NULL CHECK (target_actor_id ~ '^[0-9a-f]{64}$'),
  payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  signer_a text NOT NULL CHECK (signer_a ~ '^[A-Za-z0-9_.:-]{1,96}$'),
  signer_b text NOT NULL CHECK (signer_b ~ '^[A-Za-z0-9_.:-]{1,96}$'),
  accepted_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT bootstrap_independent_approvers CHECK (signer_a <> signer_b)
);
CREATE INDEX IF NOT EXISTS connection_security_manage_bootstrap_workspace
  ON connection_security_manage_bootstrap (workspace_id,accepted_at DESC,request_id);
DROP TRIGGER IF EXISTS connection_security_manage_bootstrap_immutable
  ON connection_security_manage_bootstrap;
CREATE TRIGGER connection_security_manage_bootstrap_immutable
  BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_manage_bootstrap
  FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
