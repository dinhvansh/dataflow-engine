-- CORE-RUN-4: auditable, idempotent, test-only rollback of published
-- scratch artifacts. Never permits rolling back a real ERP source.
ALTER TABLE core_pipeline_runs
 ADD COLUMN IF NOT EXISTS rolled_back_at timestamptz;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_constraint
  WHERE conrelid='core_pipeline_runs'::regclass
    AND conname='core_pipeline_runs_status_check') THEN
  ALTER TABLE core_pipeline_runs DROP CONSTRAINT core_pipeline_runs_status_check;
 END IF;
 ALTER TABLE core_pipeline_runs ADD CONSTRAINT core_pipeline_runs_status_check
  CHECK(status IN('waiting_sync','waiting_flow','publishing',
    'succeeded','failed','rolled_back'));
END $$;
