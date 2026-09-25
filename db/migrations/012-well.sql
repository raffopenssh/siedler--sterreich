-- Brunnen (GW-1): a dug well on an owned field protects the harvest from
-- the real, local drought state (GW-3/GW-4). Depth comes from the groundwater
-- service's depth_to_gw_m_est at the parcel centroid and sets the price.
ALTER TABLE parcel_claims ADD COLUMN well_at TIMESTAMP;
ALTER TABLE parcel_claims ADD COLUMN well_depth_m REAL NOT NULL DEFAULT 0;
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (12, '012-well');
