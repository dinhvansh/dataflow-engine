-- Per-stream bounded resource controls; old rows receive conservative defaults.
ALTER TABLE sync_streams ADD COLUMN IF NOT EXISTS batch_size integer NOT NULL DEFAULT 10000;
ALTER TABLE sync_streams ADD COLUMN IF NOT EXISTS workers integer NOT NULL DEFAULT 2;
ALTER TABLE sync_streams ADD COLUMN IF NOT EXISTS queue_capacity integer NOT NULL DEFAULT 4;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sync_streams'::regclass AND conname='batch_size_bounds') THEN
  ALTER TABLE sync_streams ADD CONSTRAINT batch_size_bounds CHECK (batch_size BETWEEN 100 AND 100000);
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sync_streams'::regclass AND conname='workers_bounds') THEN
  ALTER TABLE sync_streams ADD CONSTRAINT workers_bounds CHECK (workers BETWEEN 1 AND 32);
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sync_streams'::regclass AND conname='queue_capacity_bounds') THEN
  ALTER TABLE sync_streams ADD CONSTRAINT queue_capacity_bounds CHECK (queue_capacity BETWEEN 1 AND 100);
 END IF;
END $$;
