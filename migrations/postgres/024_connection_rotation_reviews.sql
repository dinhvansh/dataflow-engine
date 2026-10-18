-- V3.2B-5E-4B: append-only two-operator rotation review evidence.
-- Isolated synthetic PostgreSQL ONLY until production security approval.
-- Review does NOT clear needs_review or activate a connection.
CREATE TABLE IF NOT EXISTS connection_security_rotation_reviews (
 request_id uuid PRIMARY KEY,
 rotation_id uuid NOT NULL REFERENCES connection_security_rotation_journal(id),
 workspace_id text NOT NULL REFERENCES connection_security_workspaces(id),
 reviewer_actor_id text NOT NULL CHECK (reviewer_actor_id ~ '^[0-9a-f]{64}$'),
 signed_payload_sha256 text NOT NULL CHECK (signed_payload_sha256 ~ '^[0-9a-f]{64}$'),
 signer_a text NOT NULL CHECK (signer_a ~ '^[A-Za-z0-9_.:-]{1,96}$'),
 signer_b text NOT NULL CHECK (signer_b ~ '^[A-Za-z0-9_.:-]{1,96}$'),
 assessment text NOT NULL CHECK (assessment IN ('prior','target','diverged')),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK (signer_a <> signer_b)
);
CREATE INDEX IF NOT EXISTS connection_security_rotation_reviews_lookup
 ON connection_security_rotation_reviews(workspace_id,rotation_id,created_at DESC);
DROP TRIGGER IF EXISTS connection_security_rotation_reviews_immutable ON connection_security_rotation_reviews;
CREATE TRIGGER connection_security_rotation_reviews_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON connection_security_rotation_reviews
 FOR EACH STATEMENT EXECUTE FUNCTION connection_security_block_audit_mutation();

-- Extend, never disable, the existing append-only audit action constraint.
ALTER TABLE connection_security_audit
 DROP CONSTRAINT IF EXISTS connection_security_audit_action_check;
ALTER TABLE connection_security_audit
 ADD CONSTRAINT connection_security_audit_action_check CHECK (
  action IN ('draft_created','reference_staged','reference_rotated',
             'rotation_inspected','rotation_reviewed','rotation_reconciled','compensation_planned')
 );
