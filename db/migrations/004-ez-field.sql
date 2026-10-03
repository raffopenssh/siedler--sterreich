-- Add EZ (Einlagezahl) field to parcel_claims for grouping parcels
ALTER TABLE parcel_claims ADD COLUMN ez TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (004, '004-ez-field');
