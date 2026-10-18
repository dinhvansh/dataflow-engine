-- V3.2C-2B1: audit isolated authenticated QA SQL Server TEST attempts.
-- No DSN or credential is persisted; production execution remains disabled.
ALTER TABLE connection_security_audit
 DROP CONSTRAINT IF EXISTS connection_security_audit_action_check;
ALTER TABLE connection_security_audit
 ADD CONSTRAINT connection_security_audit_action_check
 CHECK(action IN ('draft_created','reference_staged','reference_rotated',
  'rotation_inspected','rotation_reviewed','rotation_reconciled',
  'compensation_planned','test_requested','test_passed','test_failed'));
