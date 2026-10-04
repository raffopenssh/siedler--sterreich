-- NE cells (srtm v2.4 observed layer): the consistency verdict of the parcel at
-- claim time (consistent | forest_loss | forest_gain | sealed_new | structure_new |
-- green_new | unknown | '' = no observation). Drives the "Spurenleser" quest and
-- the Wiederbewaldung bonus on Naturschutz conversion. No cadastre id involved.
ALTER TABLE parcel_claims ADD COLUMN ne_verdict TEXT NOT NULL DEFAULT '';
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (16, '016-ne-verdict');
