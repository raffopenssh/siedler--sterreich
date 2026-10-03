package srv

// The game never stores cadastre identifiers. Ownership, harvest state,
// offers, quest targets and per-parcel cache rows are keyed by
// parcelHash(pid) = HMAC-SHA256(key, "p:"+parcel_id)[:32 hex], folios by
// ezHash(kg, ez) = HMAC-SHA256(key, "e:"+kg+":"+ez)[:32 hex]. The key lives
// in env SIEDLER_PARCEL_KEY or ./parcel.key (0600, gitignored) — without it
// the hashes are opaque 128-bit tokens; with it they are still one-way.
//
// Clients match claims to the parcels they hold because every parcel row in
// /api/viewport carries `ph` (and `ezh`), computed on the fly from the same
// key. Plain ids only ever travel in requests and live SSE broadcasts.

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	parcelKeyOnce sync.Once
	parcelKey     []byte
	hexHash32     = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

func loadParcelKey(baseDir string) []byte {
	parcelKeyOnce.Do(func() {
		if k := os.Getenv("SIEDLER_PARCEL_KEY"); len(k) >= 32 {
			parcelKey = []byte(k)
			return
		}
		path := filepath.Join(baseDir, "parcel.key")
		if b, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(b)) >= 32 {
			parcelKey = bytes.TrimSpace(b)
			return
		}
		raw := make([]byte, 32)
		rand.Read(raw)
		parcelKey = []byte(hex.EncodeToString(raw))
		if err := os.WriteFile(path, parcelKey, 0o600); err != nil {
			slog.Error("parcel key not persisted — claims would be orphaned on restart", "err", err, "path", path)
		} else {
			slog.Info("generated parcel hash key", "path", path)
		}
	})
	return parcelKey
}

func hmacHex(prefix, msg string) string {
	m := hmac.New(sha256.New, parcelKey)
	m.Write([]byte(prefix))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// parcelHash maps a cadastre parcel id to its storage token ("" → "").
// Already-hashed input (32 hex) passes through, so handlers may receive either.
func parcelHash(pid string) string {
	pid = strings.TrimSpace(pid)
	if pid == "" {
		return ""
	}
	if hexHash32.MatchString(pid) {
		return pid
	}
	return hmacHex("p:", pid)
}

// ezHash maps a folio (kg + EZ) to its storage token ("" → "").
func ezHash(kg, ez string) string {
	ez = strings.TrimSpace(ez)
	if ez == "" {
		return ""
	}
	if hexHash32.MatchString(ez) {
		return ez
	}
	return hmacHex("e:", padKGCode(kg)+":"+ez)
}

func padKGCode(kg string) string {
	kg = strings.TrimSpace(kg)
	if len(kg) == 4 {
		return "0" + kg
	}
	return kg
}

// hashLegacyParcelRows converts plain ids left from before migration 015
// (they contain '-' / are not 32-hex) into hashes. Idempotent, runs at startup.
func (s *Server) hashLegacyParcelRows() {
	type row struct {
		id     int64
		p, ez  string
		kg     string
		hasEZ  bool
		update string
	}
	fix := func(table, col string, withEZ bool) int {
		q := "SELECT id, " + col
		if withEZ {
			q += ", ez_hash, kg_code"
		}
		q += " FROM " + table + " WHERE " + col + " <> '' AND " + col + " NOT GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'"
		rows, err := s.DB.Query(q)
		if err != nil {
			slog.Warn("hashLegacyParcelRows", "table", table, "err", err)
			return 0
		}
		var todo []row
		for rows.Next() {
			var r row
			if withEZ {
				rows.Scan(&r.id, &r.p, &r.ez, &r.kg)
			} else {
				rows.Scan(&r.id, &r.p)
			}
			todo = append(todo, r)
		}
		rows.Close()
		for _, r := range todo {
			if withEZ {
				s.DB.Exec("UPDATE "+table+" SET "+col+" = ?, ez_hash = ? WHERE id = ?", parcelHash(r.p), ezHash(r.kg, r.ez), r.id)
			} else {
				s.DB.Exec("UPDATE "+table+" SET "+col+" = ? WHERE id = ?", parcelHash(r.p), r.id)
			}
		}
		return len(todo)
	}
	n := fix("parcel_claims", "parcel_hash", true) + fix("parcel_offers", "parcel_hash", false) + fix("challenges", "target_parcel_hash", false)
	// ez_hash rows whose parcel was already hashed but ez still plain (none expected; cheap).
	s.DB.Exec("UPDATE parcel_claims SET ez_hash = '' WHERE ez_hash LIKE '%-%'")
	if n > 0 {
		slog.Info("hashed legacy parcel rows", "rows", n)
	}
}
