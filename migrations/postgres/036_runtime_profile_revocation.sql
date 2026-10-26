-- Keep saved streams and checkpoints intact when a QA profile is revoked.
-- Existing verified bindings fail closed because the worker checks both
-- profile status and revision before opening the source or destination.
ALTER TABLE connection_runtime_profiles
 DROP CONSTRAINT IF EXISTS connection_runtime_profiles_status_check;
ALTER TABLE connection_runtime_profiles
 ADD CONSTRAINT connection_runtime_profiles_status_check
 CHECK (status IN ('draft','verified','revoked'));
