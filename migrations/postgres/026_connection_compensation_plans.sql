-- 5E-4D: signed divergence compensation PLAN only. No automatic secret repair.
-- Disposal-only; production requires independent Vault IAM/security approval.
CREATE TABLE IF NOT EXISTS connection_security_compensation_plans (
 request_id uuid PRIMARY KEY,
 rotation_id uuid NOT NULL REFERENCES connection_security_rotation_journal(id),
 review_request_id uuid NOT NULL REFERENCES connection_security_rotation_reviews(request_id),
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 actor_id text NOT NULL CHECK(actor_id ~ '^[0-9a-f]{64}$'),
 payload_sha256 text NOT NULL CHECK(payload_sha256 ~ '^[0-9a-f]{64}$'),
 signer_a text NOT NULL CHECK(signer_a ~ '^[A-Za-z0-9_.:-]{1,96}$'),
 signer_b text NOT NULL CHECK(signer_b ~ '^[A-Za-z0-9_.:-]{1,96}$'),
 expected_profile_revision bigint NOT NULL CHECK(expected_profile_revision > 0),
 expected_profile_version bigint NOT NULL CHECK(expected_profile_version > 0),
 observed_vault_version bigint NOT NULL CHECK(observed_vault_version > 0),
 plan text NOT NULL CHECK(plan='escalate_manual_reconciliation'),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(signer_a<>signer_b)
);
CREATE INDEX IF NOT EXISTS connection_security_compensation_plans_ws
 ON connection_security_compensation_plans(workspace_id,rotation_id,created_at DESC);
DROP TRIGGER IF EXISTS connection_security_compensation_plans_immutable
 ON connection_security_compensation_plans;
CREATE TRIGGER connection_security_compensation_plans_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_compensation_plans
 FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();
ALTER TABLE connection_security_audit
 DROP CONSTRAINT IF EXISTS connection_security_audit_action_check;
ALTER TABLE connection_security_audit
 ADD CONSTRAINT connection_security_audit_action_check CHECK(
  action IN ('draft_created','reference_staged','reference_rotated',
  'rotation_inspected','rotation_reviewed','rotation_reconciled',
  'compensation_planned')
 );
