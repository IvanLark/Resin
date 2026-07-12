ALTER TABLE platforms
ADD COLUMN max_acceptable_latency_ms INTEGER NOT NULL DEFAULT 0;
