package srv

// Which KGs *really* carry the observed layer. srtm's registry flags a KG
// v2.4 before its NE cells are servable (seen 2026-10-07: /kg/19454 said
// v2.4 + ready while /cells listed 19454 in kgs_missing for hours). Every
// cell build knows the truth per KG (kgs[].ne), so we remember it for 24 h
// (api_cache `ne-obs:v1:<kg>` = "1"/"0", positive wins) and the lucky picker
// never trusts the flag alone.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

const neObsPrefix = "ne-obs:v1:"

func (s *Server) noteNEObserved(kg string, present bool) {
	if kg == "" {
		return
	}
	key := neObsPrefix + kg
	if !present {
		if raw, err := s.Q.GetCachedData(context.Background(), key); err == nil && raw == "1" {
			return // a cell with this KG's NE cells wins over a border cell without
		}
	}
	v := "0"
	if present {
		v = "1"
	}
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: v, ExpiresAt: time.Now().Add(time.Duration(cadastreTTL))})
}

// neObservedSet: explicit knowledge from built cells — kg → NE present.
// KGs without a built cell are absent from the map.
func (s *Server) neObservedSet() map[string]bool {
	out := map[string]bool{}
	rows, err := s.DB.QueryContext(context.Background(), "SELECT cache_key, data FROM api_cache WHERE cache_key LIKE ? AND expires_at > datetime('now')", neObsPrefix+"%")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[strings.TrimPrefix(k, neObsPrefix)] = v == "1"
		}
	}
	return out
}

// neConfirmedKGSet: registry v2.4 KGs minus those a built cell proved to
// have no NE cells yet (padded + unpadded like neReadyKGSet).
func (s *Server) neConfirmedKGSet() map[string]bool {
	v24 := s.neReadyKGSet()
	for kg, present := range s.neObservedSet() {
		if !present {
			delete(v24, kg)
			delete(v24, unpadKG(kg))
		}
	}
	return v24
}

// spawnNEOK: the cached cell at the spawn point must not list the spawn KG
// as NE-less when the registry claims v2.4 (cells built before this guard
// existed have no ne-obs note).
func (s *Server) spawnNEOK(lon, lat float64, kg string, v24 map[string]bool) bool {
	if !v24[kg] {
		return true
	}
	cd := s.cachedCell(cellOf(lon, lat))
	if cd == nil {
		return true
	}
	if ne, ok := cd.KGsNE[kg]; ok && !ne {
		s.noteNEObserved(kg, false)
		return false
	}
	return true
}

// neObservedSeed: cells built before the notes existed still know their
// kgs[].ne — scan them once (background, startup) when no note exists yet.
func (s *Server) neObservedSeed() {
	if len(s.neObservedSet()) > 0 {
		return
	}
	keys := cadastreRAM.keys(vpKeyPrefix)
	n, kgs := 0, 0
	for _, k := range keys {
		var c cellID
		if _, err := fmt.Sscanf(strings.TrimPrefix(k, vpKeyPrefix), "%d:%d", &c.I, &c.J); err != nil {
			continue
		}
		cd := s.cachedCell(c)
		if cd == nil {
			continue
		}
		n++
		for kg, ne := range cd.KGsNE {
			s.noteNEObserved(kg, ne)
			kgs++
		}
		time.Sleep(20 * time.Millisecond)
	}
	slog.Info("ne-obs: seeded from cached cells", "cells", n, "kg_notes", kgs)
}
