-- V3.2B-5E-2: append-only evidence of successful, MANAGE-locked,
-- bounded diagnostic reads against isolated metadata and pinned Vault versions.
-- Apply only in identity-verified disposable control PostgreSQL for QA.
-- No ERP source DML/DDL and no credential values are stored here.
ALTER TABLE connection_security_audit
  DROP CONSTRAINT IF EXISTS connection_security_audit_action_check;
ALTER TABLE connection_security_audit
  ADD CONSTRAINT connection_security_audit_action_check
  CHECK (action IN (
    'draft_created', 'reference_staged', 'reference_rotated',
    'rotation_inspected'
  ));
