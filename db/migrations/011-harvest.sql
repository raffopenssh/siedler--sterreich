-- Field harvest: the crop cycle itself is pure math (parcel hash + wall clock,
-- see fieldcycle.go / fieldStage in game.js) — we only remember WHEN an owner
-- last harvested a claim, so nothing is stored for the millions of NPC fields.
ALTER TABLE parcel_claims ADD COLUMN harvested_at TIMESTAMP;
ALTER TABLE parcel_claims ADD COLUMN harvests INTEGER NOT NULL DEFAULT 0;
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (11, '011-harvest');
