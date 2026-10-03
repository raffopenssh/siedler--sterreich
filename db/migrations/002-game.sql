-- Game schema

-- Players table - anonymous login with synonym only
CREATE TABLE IF NOT EXISTS players (
    id TEXT PRIMARY KEY,              -- random token
    name TEXT NOT NULL UNIQUE,         -- player chosen synonym
    rejoin_token TEXT NOT NULL UNIQUE,  -- secret link token
    municipality_code TEXT,            -- chosen municipality
    municipality_name TEXT,
    coins INTEGER NOT NULL DEFAULT 1000,
    biodiversity_score REAL NOT NULL DEFAULT 0.0,
    level INTEGER NOT NULL DEFAULT 1,
    xp INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Game sessions (multiplayer rooms)
CREATE TABLE IF NOT EXISTS game_sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    invite_code TEXT NOT NULL UNIQUE,
    municipality_code TEXT NOT NULL,
    municipality_name TEXT NOT NULL,
    center_lon REAL NOT NULL,
    center_lat REAL NOT NULL,
    created_by TEXT NOT NULL REFERENCES players(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'active'  -- active, finished
);

-- Player <-> session membership
CREATE TABLE IF NOT EXISTS session_players (
    session_id TEXT NOT NULL REFERENCES game_sessions(id),
    player_id TEXT NOT NULL REFERENCES players(id),
    joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (session_id, player_id)
);

-- Claimed/owned parcels
CREATE TABLE IF NOT EXISTS parcel_claims (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES game_sessions(id),
    player_id TEXT NOT NULL REFERENCES players(id),
    parcel_id TEXT NOT NULL,           -- cadastre parcel_id
    kg_code TEXT NOT NULL,
    gnr TEXT NOT NULL,
    area_sqm REAL NOT NULL DEFAULT 0,
    landuse TEXT,                       -- original landuse code
    converted_to TEXT,                  -- what player converted it to (e.g. 'biodiversity')
    purchase_price INTEGER NOT NULL DEFAULT 0,
    claimed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(session_id, parcel_id)
);

-- Challenges / quests
CREATE TABLE IF NOT EXISTS challenges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES game_sessions(id),
    player_id TEXT NOT NULL REFERENCES players(id),
    challenge_type TEXT NOT NULL,       -- 'explore', 'restore', 'quiz', 'treasure'
    title TEXT NOT NULL,
    description TEXT,
    target_parcel_id TEXT,
    reward_coins INTEGER NOT NULL DEFAULT 50,
    reward_xp INTEGER NOT NULL DEFAULT 25,
    completed INTEGER NOT NULL DEFAULT 0,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Treasures hidden on map
CREATE TABLE IF NOT EXISTS treasures (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES game_sessions(id),
    lon REAL NOT NULL,
    lat REAL NOT NULL,
    treasure_type TEXT NOT NULL,        -- 'coins', 'xp', 'rare_seed', 'ancient_map'
    value INTEGER NOT NULL DEFAULT 100,
    found_by TEXT REFERENCES players(id),
    found_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Chat messages
CREATE TABLE IF NOT EXISTS chat_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES game_sessions(id),
    player_id TEXT NOT NULL REFERENCES players(id),
    message TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Cached API data
CREATE TABLE IF NOT EXISTS api_cache (
    cache_key TEXT PRIMARY KEY,
    data TEXT NOT NULL,                 -- JSON
    fetched_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (002, '002-game');
