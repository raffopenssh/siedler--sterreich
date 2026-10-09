package srv

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

// Raw-cell stash for the nightly NE report — RAM only.
//
// bevdirect-serve assembles a /viewport cell from vector tiles on every
// request (1–2.5 s of CPU per cell, tiles RAM-only). The midnight contrib
// prewarm (warmContribRun) already builds every cell of tonight's KGs
// through buildCell — before this stash the nightly ne-report run asked
// bevdirect for the very same documents again, so each cell was assembled
// twice a day and the night run pinned both cores for 1–2 h (bevdirect
// ~140 % + python). Now buildCell keeps the *raw* bevdirect document of every
// ready, untruncated aligned cell whose KGs are on tonight's contrib list in
// memory (gzipped, expires with the cell ≤ 24 h, ≤ contribStashMaxBytes,
// lost on restart) and ne_report.py fetches it from
// GET /api/contrib/cell?i&j (loopback only) before falling back to bevdirect.
// The stash is byte-for-byte what bevdirect answered (same query parameters
// as vtcseamless' BevDirect.cell), so the report digest is unchanged; only
// the enriched /api/viewport document is ever served to browsers. Cadastre
// hygiene: nothing of this touches the disk.

const contribStashMaxBytes = 768 << 20 // ~2 400 gzipped cells (~320 KB each) — a 300-KG night is ~2 000

type stashEntry struct {
	gz  []byte
	exp time.Time
	at  time.Time
}

var contribStash struct {
	mu                         sync.Mutex
	kgsAt                      time.Time
	kgs                        map[string]bool
	cells                      map[cellID]*stashEntry
	bytes                      int64
	hits, misses, put, evicted int64
}

// contribStashKGs is tonight's contrib KG set, memoised for 10 min (the
// plan scans the report directory, buildCell runs on every cell).
func (s *Server) contribStashKGs() map[string]bool {
	contribStash.mu.Lock()
	if contribStash.kgs != nil && time.Since(contribStash.kgsAt) < 10*time.Minute {
		m := contribStash.kgs
		contribStash.mu.Unlock()
		return m
	}
	contribStash.mu.Unlock()
	m := map[string]bool{}
	if kgUniverseOK() {
		p := s.contribPlanNow(nextContribNight(time.Now()))
		for _, kg := range p.KGs {
			m[kg] = true
		}
	}
	contribStash.mu.Lock()
	contribStash.kgs, contribStash.kgsAt = m, time.Now()
	contribStash.mu.Unlock()
	return m
}

// contribStashPut keeps the raw bevdirect document of an aligned cell when
// one of its KGs is reported tonight.
// force (a contrib warm job, warmKG) stashes the cell regardless of which KGs
// it holds — the report's viewport includes cells without a parcel of the KG.
func (s *Server) contribStashPut(c cellID, kgs map[string]*vpKG, raw []byte, ttl time.Duration, force bool) {
	if len(raw) == 0 {
		return
	}
	hit := force
	if !hit {
		want := s.contribStashKGs()
		for kg := range kgs {
			if want[kg] {
				hit = true
				break
			}
		}
	}
	var gz []byte // nil = negative marker: built, no planned KG in it (so warmKG does not rebuild it)
	if hit {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		zw.Write(raw)
		zw.Close()
		gz = buf.Bytes()
	}
	now := time.Now()
	contribStash.mu.Lock()
	defer contribStash.mu.Unlock()
	if contribStash.cells == nil {
		contribStash.cells = map[cellID]*stashEntry{}
	}
	if old := contribStash.cells[c]; old != nil {
		contribStash.bytes -= int64(len(old.gz))
	}
	// expire + evict oldest until the new entry fits
	for k, e := range contribStash.cells {
		if now.After(e.exp) {
			contribStash.bytes -= int64(len(e.gz))
			delete(contribStash.cells, k)
		}
	}
	for contribStash.bytes+int64(len(gz)) > contribStashMaxBytes && len(contribStash.cells) > 0 {
		var ok cellID
		var oe *stashEntry
		for k, e := range contribStash.cells {
			if oe == nil || e.at.Before(oe.at) {
				ok, oe = k, e
			}
		}
		contribStash.bytes -= int64(len(oe.gz))
		delete(contribStash.cells, ok)
		contribStash.evicted++
	}
	contribStash.cells[c] = &stashEntry{gz: gz, exp: now.Add(ttl), at: now}
	contribStash.bytes += int64(len(gz))
	if gz != nil {
		contribStash.put++
	}
}

// contribStashHas reports whether cell c was considered for the stash (document or
// negative marker) and is unexpired.
func contribStashHas(c cellID) bool {
	contribStash.mu.Lock()
	defer contribStash.mu.Unlock()
	e := contribStash.cells[c]
	return e != nil && time.Now().Before(e.exp)
}

// contribStashComplete: every cell of the list is stashed.
func contribStashComplete(cells []cellID) bool {
	for _, c := range cells {
		if !contribStashHas(c) {
			return false
		}
	}
	return len(cells) > 0
}

// handleContribCell serves a stashed raw cell to the local ne-report run.
// GET /api/contrib/cell?i=&j= → the bevdirect /viewport document, 404 when
// not stashed (the reporter then asks bevdirect itself). Loopback only.
func (s *Server) handleContribCell(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		http.Error(w, "local only", http.StatusForbidden)
		return
	}
	i, err1 := strconv.Atoi(r.URL.Query().Get("i"))
	j, err2 := strconv.Atoi(r.URL.Query().Get("j"))
	if err1 != nil || err2 != nil {
		http.Error(w, "i, j required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	contribStash.mu.Lock()
	e := contribStash.cells[cellID{I: i, J: j}]
	if e != nil && time.Now().After(e.exp) {
		contribStash.bytes -= int64(len(e.gz))
		delete(contribStash.cells, cellID{I: i, J: j})
		e = nil
	}
	if e == nil || e.gz == nil {
		contribStash.misses++
		contribStash.mu.Unlock()
		http.Error(w, `{"error":"not stashed"}`, http.StatusNotFound)
		return
	}
	contribStash.hits++
	gz := e.gz
	contribStash.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "STASH")
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		slog.Warn("contrib stash: gunzip", "err", err)
		http.Error(w, `{"error":"stash corrupt"}`, http.StatusInternalServerError)
		return
	}
	io.Copy(w, zr)
}

func contribStashStatus() map[string]any {
	contribStash.mu.Lock()
	defer contribStash.mu.Unlock()
	docs := 0
	for _, e := range contribStash.cells {
		if e.gz != nil {
			docs++
		}
	}
	return map[string]any{"cells": len(contribStash.cells), "docs": docs, "mb": fmt.Sprintf("%.1f", float64(contribStash.bytes)/1e6), "max_mb": contribStashMaxBytes >> 20,
		"put": contribStash.put, "hits": contribStash.hits, "misses": contribStash.misses, "evicted": contribStash.evicted}
}

// contribKGCells: the aligned cells the NE report of a KG is built from —
// those of umfeld's declared viewport (`/ne/{kg}/head` lu.header.input_bbox, the same
// rule as ne_report.py / vtcseamless cells_for), which is wider than the
// generalised admin bbox (03012: 15 cells vs 9). The head is a bbox + epoch,
// not cadastre data: cached 7 d (`ne-head:v1:<kg>`). Falls back to the admin
// bbox when umfeld has no head (KG not built) or is unreachable.
func (s *Server) contribKGCells(kg string) []cellID {
	k := admin().KGs[kg]
	if k == nil {
		return nil
	}
	key := "ne-head:v1:" + kg
	var bb []float64
	if c, err := s.Q.GetCachedData(context.Background(), key); err == nil {
		json.Unmarshal([]byte(c), &bb)
	} else {
		time.Sleep(220 * time.Millisecond) // umfeld ≤ 5 req/s; a first run asks for ~300 heads
		resp, err := upstreamGet(umfeldAPI + "/ne/" + kg + "/head")
		if err == nil {
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if resp.StatusCode == 200 {
				var h struct {
					LU struct {
						Header struct {
							InputBBox []float64 `json:"input_bbox"`
						} `json:"header"`
					} `json:"lu"`
				}
				if json.Unmarshal(raw, &h) == nil && len(h.LU.Header.InputBBox) == 4 {
					bb = h.LU.Header.InputBBox
				}
			}
			if resp.StatusCode == 200 || resp.StatusCode == 404 {
				enc, _ := json.Marshal(bb) // 404 → "null": remembered too, re-asked after 7 d
				s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(enc), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)})
			}
		}
	}
	if len(bb) != 4 || bb[2] <= bb[0] || bb[3] <= bb[1] || bb[2]-bb[0] > 1 || bb[3]-bb[1] > 1 {
		return k.cells()
	}
	return cellsForBBox(bb[0], bb[1], bb[2], bb[3])
}
