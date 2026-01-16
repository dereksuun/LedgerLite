-- 004_outbox_retry.sql

ALTER TABLE outbox_events
  ADD COLUMN IF NOT EXISTS processing_at    timestamptz,
  ADD COLUMN IF NOT EXISTS attempts         int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at  timestamptz,
  ADD COLUMN IF NOT EXISTS last_error       text;

-- Ajuda o worker a achar o que está pronto pra publicar
CREATE INDEX IF NOT EXISTS idx_outbox_ready
  ON outbox_events (created_at, id)
  WHERE published_at IS NULL;

--
