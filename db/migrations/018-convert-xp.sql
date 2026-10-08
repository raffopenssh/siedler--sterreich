-- XP the player earned when converting the parcel (Naturschutz/Aufforstung/
-- Naturwald). Selling a converted parcel gives the protection up and takes
-- this XP back, so convert → sell → rebuy → convert cannot farm XP.
ALTER TABLE parcel_claims ADD COLUMN convert_xp INTEGER NOT NULL DEFAULT 0;
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (18, '018-convert-xp');
