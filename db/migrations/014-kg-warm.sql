-- Warm registry for the BEV-tile cadastre cells (docs/migration-2026-10.md).
-- One row per KG whose grid cells we have assembled into api_cache; rows
-- expire with the 24 h cache policy. Drives "Auf Glück" (lucky picks only
-- warm Gemeinden) and the daily/neighbour prewarm planner.
CREATE TABLE IF NOT EXISTS kg_warm (
  kg_code    TEXT PRIMARY KEY,
  warmed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL,
  cells      INTEGER NOT NULL DEFAULT 0,
  parcels    INTEGER NOT NULL DEFAULT 0,
  reason     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_kg_warm_expires ON kg_warm(expires_at);
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (14, '014-kg-warm');
