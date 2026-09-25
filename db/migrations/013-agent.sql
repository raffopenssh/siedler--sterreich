-- Agent players (/llm/game): pseudonymous LLM-driven players. `agent` holds a
-- short, non-personal model label ("claude-…", "gpt-…"); '' = human. Used to
-- restrict agent chat to quick phrases and to mark them 🤖 for human players.
ALTER TABLE players ADD COLUMN agent TEXT NOT NULL DEFAULT '';
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (13, '013-agent');
