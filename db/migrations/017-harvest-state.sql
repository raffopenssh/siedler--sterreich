-- Harvest state outlives ownership: a clear-cut stand or a stubble field stays
-- that way when the owner sells, and the next buyer inherits the regrowth
-- clock (and pays less while it regrows). Keyed by the HMAC parcel hash only —
-- no cadastre id, no geometry, no area. kind = forest|field|meadow;
-- crop_group = INVEKOS class the harvester saw ("" = hash kind) so the per-crop
-- cycle survives too.
CREATE TABLE IF NOT EXISTS parcel_harvest_state (
  session_id   TEXT NOT NULL,
  parcel_hash  TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT '',
  crop_group   TEXT NOT NULL DEFAULT '',
  harvested_at TIMESTAMP NOT NULL,
  harvests     INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (session_id, parcel_hash)
);
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (17, '017-harvest-state');
