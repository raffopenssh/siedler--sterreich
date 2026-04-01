-- Give players more starting coins (10000 instead of 1000)
-- Update existing players who still have exactly 1000 coins (untouched starting balance)
UPDATE players SET coins = 10000 WHERE coins = 1000;

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (003, '003-more-coins');
