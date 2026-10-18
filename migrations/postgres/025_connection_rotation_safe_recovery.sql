-- V3.2B-5E-4C: bounded, isolated safe-case resolution only.
-- NEVER run in production; no Vault/ERP writes or divergent metadata repair.
ALTER TABLE connection_security_rotation_events
 DROP CONSTRAINT IF EXISTS connection_security_rotation_events_action_check;
ALTER TABLE connection_security_rotation_events
 ADD CONSTRAINT connection_security_rotation_events_action_check
 CHECK (action IN ('prepared','applied','needs_review','reconciled'));

ALTER TABLE connection_security_audit
 DROP CONSTRAINT IF EXISTS connection_security_audit_action_check;
ALTER TABLE connection_security_audit
 ADD CONSTRAINT connection_security_audit_action_check CHECK (
  action IN ('draft_created','reference_staged','reference_rotated',
             'rotation_inspected','rotation_reviewed','rotation_reconciled','compensation_planned','test_requested','test_passed','test_failed')
 );

CREATE TABLE IF NOT EXISTS connection_security_rotation_recoveries (
 request_id uuid PRIMARY KEY,
 rotation_id uuid NOT NULL UNIQUE REFERENCES connection_security_rotation_journal(id),
 review_request_id uuid NOT NULL REFERENCES connection_security_rotation_reviews(request_id),
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 actor_id text NOT NULL CHECK (actor_id ~ '^[0-9a-f]{64}$'),
 signed_payload_sha256 text NOT NULL CHECK (signed_payload_sha256 ~ '^[0-9a-f]{64}$'),
 signer_a text NOT NULL CHECK (signer_a ~ '^[A-Za-z0-9_.:-]{1,96}$'),
 signer_b text NOT NULL CHECK (signer_b ~ '^[A-Za-z0-9_.:-]{1,96}$'),
 outcome text NOT NULL CHECK (outcome='confirmed_applied'),
 created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT recovery_two_distinct_signers CHECK (signer_a<>signer_b)
);
CREATE INDEX IF NOT EXISTS connection_security_rotation_recoveries_workspace
 ON connection_security_rotation_recoveries(workspace_id,created_at DESC,request_id);
DROP TRIGGER IF EXISTS connection_security_rotation_recoveries_immutable
 ON connection_security_rotation_recoveries;
CREATE TRIGGER connection_security_rotation_recoveries_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_rotation_recoveries
 FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
