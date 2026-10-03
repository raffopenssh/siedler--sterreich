-- Migration 008: chat safety / protection of minors (DSA Art. 16 + 28)
-- Automated moderation: strikes, mutes, bans, reports, blocks, audit log.

ALTER TABLE players ADD COLUMN chat_strikes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE players ADD COLUMN chat_muted_until TIMESTAMP;
ALTER TABLE players ADD COLUMN chat_banned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE players ADD COLUMN chat_rules_accepted INTEGER NOT NULL DEFAULT 0;

-- 'free' = free text (filtered), 'quick' = preset phrases only, 'off' = no chat
ALTER TABLE game_sessions ADD COLUMN chat_mode TEXT NOT NULL DEFAULT 'free';

ALTER TABLE chat_messages ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_messages ADD COLUMN flag TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    message_id INTEGER,
    reported_player_id TEXT NOT NULL,
    reporter_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_reports_message ON reports(message_id);
CREATE INDEX IF NOT EXISTS idx_reports_reported ON reports(reported_player_id, created_at);
CREATE INDEX IF NOT EXISTS idx_reports_reporter ON reports(reporter_id, created_at);

CREATE TABLE IF NOT EXISTS player_blocks (
    player_id TEXT NOT NULL,
    blocked_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (player_id, blocked_id)
);

CREATE TABLE IF NOT EXISTS safety_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    player_id TEXT,
    session_id TEXT,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (008, '008-safety');
