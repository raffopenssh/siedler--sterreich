-- Migration 007: indexes for hot query paths + api_cache expiry cleanup support

CREATE INDEX IF NOT EXISTS idx_chat_session ON chat_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_treasures_session ON treasures(session_id, found_by);
CREATE INDEX IF NOT EXISTS idx_challenges_session_player ON challenges(session_id, player_id, completed);
CREATE INDEX IF NOT EXISTS idx_offers_session_status ON parcel_offers(session_id, status);
CREATE INDEX IF NOT EXISTS idx_offers_parcel ON parcel_offers(session_id, parcel_id, status);
CREATE INDEX IF NOT EXISTS idx_session_players_player ON session_players(player_id);
CREATE INDEX IF NOT EXISTS idx_claims_session_player ON parcel_claims(session_id, player_id);
CREATE INDEX IF NOT EXISTS idx_api_cache_expires ON api_cache(expires_at);

INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (007, '007-indexes');
