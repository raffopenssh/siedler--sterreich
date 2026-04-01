-- Migration 005: Create parcel_offers table for the parcel offers system
-- 
-- This table tracks offers from players to buy parcels that are already claimed by other players.
-- Key features:
-- - Buyer's coins are NOT locked until the owner accepts the offer
-- - Offers can be in various states: pending, accepted, rejected, expired, cancelled
-- - If accepted, the buyer must have sufficient coins (enforced by application logic)

CREATE TABLE IF NOT EXISTS parcel_offers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    parcel_id TEXT NOT NULL,
    claim_id INTEGER NOT NULL,
    buyer_id TEXT NOT NULL,
    seller_id TEXT NOT NULL,
    offer_price INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, accepted, rejected, expired, cancelled
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    resolved_at DATETIME,
    FOREIGN KEY (session_id) REFERENCES game_sessions(id),
    FOREIGN KEY (buyer_id) REFERENCES players(id),
    FOREIGN KEY (seller_id) REFERENCES players(id),
    FOREIGN KEY (claim_id) REFERENCES parcel_claims(id)
);

-- Track this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (5, '005-parcel-offers');
