-- Store upstream ETag next to cached bodies so expired entries can be
-- revalidated with If-None-Match (0-byte 304) instead of re-downloaded.
ALTER TABLE api_cache ADD COLUMN etag TEXT NOT NULL DEFAULT '';
INSERT OR IGNORE INTO migrations (migration_number, migration_name) VALUES (010, '010-cache-etag');
