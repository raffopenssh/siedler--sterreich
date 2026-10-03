-- Privacy / BEV terms: the game never stores cadastre identifiers. Ownership,
-- harvest state, offers and quest targets are keyed by an HMAC-SHA256 of the
-- parcel id (server-secret key in ./parcel.key), the folio by an HMAC of
-- kg+EZ. The columns are renamed so no code path can mistake them for ids;
-- legacy plain values are hashed by the server on startup (hashLegacyParcelRows).
ALTER TABLE parcel_claims RENAME COLUMN parcel_id TO parcel_hash;
ALTER TABLE parcel_claims RENAME COLUMN ez TO ez_hash;
ALTER TABLE parcel_offers RENAME COLUMN parcel_id TO parcel_hash;
ALTER TABLE challenges RENAME COLUMN target_parcel_id TO target_parcel_hash;
UPDATE parcel_claims SET gnr = '';
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (15, '015-parcel-hash');
