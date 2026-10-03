-- Add species_name column to treasures for red list species treasure types
ALTER TABLE treasures ADD COLUMN species_name TEXT NOT NULL DEFAULT '';
ALTER TABLE treasures ADD COLUMN species_german TEXT NOT NULL DEFAULT '';
ALTER TABLE treasures ADD COLUMN species_category TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (6, '006-treasure-species');
