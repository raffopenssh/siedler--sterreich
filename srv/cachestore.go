package srv

// Transparent compression for api_cache bodies + routing of cadastre keys to
// the RAM-only store (ramcache.go — rule 2: nothing BEV-derived on disk).
//
// Cadastre cells (`vp:v1:*`) are ~1 MB of JSON each and were stored as plain
// text — 645 cells ≈ 650 MB, the bulk of db.sqlite3. Store wraps the sqlc
// Queries and gzips every body above compressThreshold on write; reads detect
// the gzip magic and inflate, so every call site (cachedFetchX, cellstore,
// warm, timber …) keeps working unchanged. Small rows stay plain so cheap
// lookups don't pay for a gzip reader. Legacy plain rows are migrated in the
// background on startup (compressLegacyCache).

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

const compressThreshold = 2048

type Store struct {
	*dbgen.Queries
}

func isGzipped(s string) bool {
	return len(s) > 2 && s[0] == 0x1f && s[1] == 0x8b
}

func cachePack(s string) string {
	if len(s) < compressThreshold || isGzipped(s) {
		return s
	}
	var buf bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	gw.Write([]byte(s))
	gw.Close()
	if buf.Len() >= len(s) {
		return s
	}
	return buf.String()
}

func cacheUnpack(s string) string {
	if !isGzipped(s) {
		return s
	}
	zr, err := gzip.NewReader(strings.NewReader(s))
	if err != nil {
		return s
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		return s
	}
	return string(b)
}

func (q Store) GetCachedData(ctx context.Context, key string) (string, error) {
	if isRAMKey(key) {
		if v, ok := cadastreRAM.get(key); ok {
			return cacheUnpack(v), nil
		}
		return "", sql.ErrNoRows
	}
	s, err := q.Queries.GetCachedData(ctx, key)
	if err != nil {
		return s, err
	}
	return cacheUnpack(s), nil
}

func (q Store) GetStaleCachedData(ctx context.Context, key string) (dbgen.GetStaleCachedDataRow, error) {
	if isRAMKey(key) {
		return dbgen.GetStaleCachedDataRow{}, sql.ErrNoRows
	}
	r, err := q.Queries.GetStaleCachedData(ctx, key)
	if err == nil {
		r.Data = cacheUnpack(r.Data)
	}
	return r, err
}

func (q Store) SetCachedData(ctx context.Context, p dbgen.SetCachedDataParams) error {
	p.Data = cachePack(p.Data)
	if isRAMKey(p.CacheKey) {
		cadastreRAM.set(p.CacheKey, p.Data, p.ExpiresAt)
		return nil
	}
	return q.Queries.SetCachedData(ctx, p)
}

func (q Store) SetCachedDataEtag(ctx context.Context, p dbgen.SetCachedDataEtagParams) error {
	p.Data = cachePack(p.Data)
	if isRAMKey(p.CacheKey) {
		cadastreRAM.set(p.CacheKey, p.Data, p.ExpiresAt)
		return nil
	}
	return q.Queries.SetCachedDataEtag(ctx, p)
}

func (q Store) TouchCache(ctx context.Context, p dbgen.TouchCacheParams) error {
	if isRAMKey(p.CacheKey) {
		cadastreRAM.touch(p.CacheKey, p.ExpiresAt)
		return nil
	}
	return q.Queries.TouchCache(ctx, p)
}

// DeleteCacheLike covers both stores: a pattern may match RAM keys (cell
// purges on NE adoption) as well as SQLite rows.
func (q Store) DeleteCacheLike(ctx context.Context, pattern string) (int64, error) {
	n := cadastreRAM.deleteLike(pattern)
	m, err := q.Queries.DeleteCacheLike(ctx, pattern)
	return n + m, err
}

func (q Store) DeleteExpiredCache(ctx context.Context) (int64, error) {
	n := cadastreRAM.sweep()
	m, err := q.Queries.DeleteExpiredCache(ctx)
	return n + m, err
}

// compressLegacyCache gzips pre-existing plain rows in small batches (keeps the
// write lock short), then truncates the WAL. One-off after deploy; a no-op later.
func (s *Server) compressLegacyCache() {
	ctx := context.Background()
	total, saved := 0, int64(0)
	for {
		rows, err := s.DB.QueryContext(ctx,
			`SELECT cache_key, data FROM api_cache WHERE length(data) > ? AND substr(data,1,1) != char(31) LIMIT 20`, compressThreshold)
		if err != nil {
			slog.Warn("cache compress: query", "err", err)
			return
		}
		type kv struct{ k, v string }
		var batch []kv
		for rows.Next() {
			var r kv
			if rows.Scan(&r.k, &r.v) == nil {
				batch = append(batch, r)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			break
		}
		for _, r := range batch {
			// Always gzip here (even if it wouldn't shrink) so the row
			// leaves the WHERE set and the loop terminates.
			var buf bytes.Buffer
			gw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
			gw.Write([]byte(r.v))
			gw.Close()
			packed := buf.String()
			if _, err := s.DB.ExecContext(ctx, `UPDATE api_cache SET data = ? WHERE cache_key = ?`, packed, r.k); err != nil {
				slog.Warn("cache compress: update", "err", err)
				return
			}
			total++
			saved += int64(len(r.v) - len(packed))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if total > 0 {
		s.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
		slog.Info("cache compress: legacy rows gzipped", "rows", total, "saved_mb", saved/1e6)
	}
}
