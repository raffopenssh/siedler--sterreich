-- Persist lidar giant-tree count per claim so the "Baumriese" quest can be verified server-side.
ALTER TABLE parcel_claims ADD COLUMN tall_trees INTEGER NOT NULL DEFAULT 0;
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (9, '009-tall-trees');
