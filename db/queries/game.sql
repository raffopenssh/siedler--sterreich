-- name: CreatePlayer :exec
INSERT INTO players (id, name, rejoin_token, coins) VALUES (?, ?, ?, 10000);

-- name: GetPlayerByName :one
SELECT * FROM players WHERE name = ?;

-- name: GetPlayerByToken :one
SELECT * FROM players WHERE rejoin_token = ?;

-- name: GetPlayerByID :one
SELECT * FROM players WHERE id = ?;

-- name: UpdatePlayerMunicipality :exec
UPDATE players SET municipality_code = ?, municipality_name = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdatePlayerCoins :exec
UPDATE players SET coins = coins + ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdatePlayerXP :exec
UPDATE players SET xp = xp + ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdatePlayerLevel :exec
UPDATE players SET level = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdatePlayerBiodiversity :exec
UPDATE players SET biodiversity_score = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?;

-- name: CreateSession :exec
INSERT INTO game_sessions (id, name, invite_code, municipality_code, municipality_name, center_lon, center_lat, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM game_sessions WHERE id = ?;

-- name: GetSessionByInvite :one
SELECT * FROM game_sessions WHERE invite_code = ?;

-- name: JoinSession :exec
INSERT OR IGNORE INTO session_players (session_id, player_id) VALUES (?, ?);

-- name: GetSessionPlayers :many
SELECT p.* FROM players p
JOIN session_players sp ON sp.player_id = p.id
WHERE sp.session_id = ?;

-- name: GetPlayerSessions :many
SELECT gs.* FROM game_sessions gs
JOIN session_players sp ON sp.session_id = gs.id
WHERE sp.player_id = ? AND gs.status = 'active';

-- name: ClaimParcel :exec
INSERT INTO parcel_claims (session_id, player_id, parcel_id, kg_code, gnr, ez, area_sqm, landuse, purchase_price)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetParcelClaim :one
SELECT * FROM parcel_claims WHERE session_id = ? AND parcel_id = ?;

-- name: GetPlayerParcels :many
SELECT * FROM parcel_claims WHERE session_id = ? AND player_id = ?;

-- name: GetSessionParcels :many
SELECT * FROM parcel_claims WHERE session_id = ?;

-- name: ConvertParcel :exec
UPDATE parcel_claims SET converted_to = ? WHERE id = ?;

-- name: CreateChallenge :exec
INSERT INTO challenges (session_id, player_id, challenge_type, title, description, target_parcel_id, reward_coins, reward_xp)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPlayerChallenges :many
SELECT * FROM challenges WHERE session_id = ? AND player_id = ? AND completed = 0;

-- name: CompleteChallenge :exec
UPDATE challenges SET completed = 1, completed_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: CreateTreasure :exec
INSERT INTO treasures (session_id, lon, lat, treasure_type, value, species_name, species_german, species_category) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSessionTreasures :many
SELECT * FROM treasures WHERE session_id = ? AND found_by IS NULL;

-- name: ClaimTreasure :execrows
UPDATE treasures SET found_by = ?, found_at = CURRENT_TIMESTAMP WHERE id = ? AND found_by IS NULL;

-- name: CreateChatMessage :one
INSERT INTO chat_messages (session_id, player_id, message) VALUES (?, ?, ?) RETURNING *;

-- name: GetRecentChat :many
SELECT cm.*, p.name as player_name FROM chat_messages cm
JOIN players p ON p.id = cm.player_id
WHERE cm.session_id = ?
ORDER BY cm.created_at DESC LIMIT ?;

-- name: GetCachedData :one
SELECT data FROM api_cache WHERE cache_key = ? AND expires_at > CURRENT_TIMESTAMP;

-- name: SetCachedData :exec
INSERT OR REPLACE INTO api_cache (cache_key, data, fetched_at, expires_at)
VALUES (?, ?, CURRENT_TIMESTAMP, ?);

-- name: GetSessionBiodiversityPercent :one
SELECT
    COALESCE(SUM(CASE WHEN converted_to = 'biodiversity' THEN area_sqm ELSE 0 END), 0) as bio_area,
    COALESCE(SUM(area_sqm), 0) as total_area
FROM parcel_claims WHERE session_id = ?;

-- name: CreateParcelOffer :exec
INSERT INTO parcel_offers (session_id, parcel_id, claim_id, buyer_id, seller_id, offer_price)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetPendingOffersForParcel :many
SELECT po.*, bp.name as buyer_name, sp.name as seller_name
FROM parcel_offers po
JOIN players bp ON bp.id = po.buyer_id
JOIN players sp ON sp.id = po.seller_id
WHERE po.session_id = ? AND po.parcel_id = ? AND po.status = 'pending'
ORDER BY po.offer_price DESC;

-- name: GetPendingOffersForSeller :many
SELECT po.*, bp.name as buyer_name, sp.name as seller_name
FROM parcel_offers po
JOIN players bp ON bp.id = po.buyer_id
JOIN players sp ON sp.id = po.seller_id
WHERE po.session_id = ? AND po.seller_id = ? AND po.status = 'pending'
ORDER BY po.created_at DESC;

-- name: GetPendingOffersForBuyer :many
SELECT po.*, bp.name as buyer_name, sp.name as seller_name
FROM parcel_offers po
JOIN players bp ON bp.id = po.buyer_id
JOIN players sp ON sp.id = po.seller_id
WHERE po.session_id = ? AND po.buyer_id = ? AND po.status = 'pending'
ORDER BY po.created_at DESC;

-- name: GetOfferByID :one
SELECT * FROM parcel_offers WHERE id = ?;

-- name: UpdateOfferStatus :exec
UPDATE parcel_offers SET status = ?, resolved_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: CancelPendingOffersForParcel :exec
UPDATE parcel_offers SET status = 'cancelled', resolved_at = CURRENT_TIMESTAMP
WHERE parcel_id = ? AND session_id = ? AND status = 'pending';

-- name: GetSessionOffers :many
SELECT po.*, bp.name as buyer_name, sp.name as seller_name
FROM parcel_offers po
JOIN players bp ON bp.id = po.buyer_id
JOIN players sp ON sp.id = po.seller_id
WHERE po.session_id = ? AND po.status = 'pending'
ORDER BY po.created_at DESC;
