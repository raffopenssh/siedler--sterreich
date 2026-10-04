package srv

// /api/viewport — one grid cell of cadastre (bevdirect-serve, live from the
// BEV vector tiles) enriched with 25 m terrain from the srtm public tier.
//
// Contract for the browser (docs/migration-2026-10.md):
//   {parcels[], footprints[], landuse[], kgs[], ready, truncated,
//    retry_after_s?, notice, cell:{i,j}?, source}
// Parcel rows are bevdirect rows + (when a grid25 heightfield exists)
// elev_m, elev_min_m, elev_max_m, slope_deg, aspect, dom_terrain, fracs,
// tree_frac. Cached 24 h max (cadastreTTL) — only when ready.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

const vpKeyPrefix = "vp:v1:"

// bevWait is how long bevdirect may block on cold cells before answering
// partial + ready:false. A dense town cell assembles in 3–5 s, alpine cells
// in 1–3 s; the client retries ready:false tiles anyway, so this only
// decides how often a first visit needs a second round-trip.
const bevWait = 10

type bbox struct{ W, S, E, N float64 }

func parseBBox(q url.Values) (bbox, bool) {
	var b bbox
	var err error
	get := func(k string) float64 {
		f, e := strconv.ParseFloat(q.Get(k), 64)
		if e != nil {
			err = e
		}
		return f
	}
	b.W, b.S, b.E, b.N = get("west"), get("south"), get("east"), get("north")
	if err != nil || b.E <= b.W || b.N <= b.S || b.E-b.W > 0.05 || b.N-b.S > 0.05 {
		return b, false
	}
	return b, true
}

func (b bbox) qs() string {
	return fmt.Sprintf("west=%.6f&south=%.6f&east=%.6f&north=%.6f", b.W, b.S, b.E, b.N)
}

func (b bbox) pad(d float64) bbox { return bbox{b.W - d, b.S - d, b.E + d, b.N + d} }

// vpCacheKey: aligned cells get the canonical key the warm planner fills;
// anything else is keyed by its ~150 m quantised bbox.
func vpCacheKey(b bbox) (string, *cellID) {
	if c, ok := isAlignedCell(b.W, b.S, b.E, b.N); ok {
		return fmt.Sprintf("%s%d:%d", vpKeyPrefix, c.I, c.J), &c
	}
	q := func(f float64) string { return strconv.FormatFloat(math.Round(f/0.002)*0.002, 'f', 3, 64) }
	return vpKeyPrefix + "b:" + q(b.W) + "," + q(b.S) + "," + q(b.E) + "," + q(b.N), nil
}

func (s *Server) handleViewport(w http.ResponseWriter, r *http.Request) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required (span ≤ 0.05°)", 400)
		return
	}
	key, cell := vpCacheKey(b)
	// Hot path: a pre-gzipped, hash-annotated copy of recently served cells
	// (≈150 KB each). Under load this turns a HIT from "1 MB SQLite read +
	// byte scan + gzip-5" (~25 ms CPU) into a memcpy.
	if gz := hotCellGet(key); gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(browserCacheMaxAge))
		w.Write(gz)
		return
	}
	rec := &recordWriter{ResponseWriter: w}
	s.cachedFetchX(rec, key, func() ([]byte, int) {
		s.warm.fg.Add(1) // the warm loop yields while a player waits for a cell
		defer s.warm.fg.Add(-1)
		out, st := s.buildCell(b, key)
		if st == 200 && cell != nil {
			s.noteCellLoaded(*cell, "viewport")
		}
		return out, st
	}, addParcelHashes)
	if rec.status == 0 || rec.status == 200 {
		if xc := rec.Header().Get("X-Cache"); xc == "HIT" || xc == "MISS" || xc == "MISS-SHARED" {
			hotCellPut(key, rec.buf.Bytes())
		}
	}
}

// recordWriter captures the plain body written by cachedFetchX so it can be
// gzipped once into the hot-cell cache (it still streams to the client).
type recordWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recordWriter) WriteHeader(c int) { r.status = c; r.ResponseWriter.WriteHeader(c) }
func (r *recordWriter) Write(b []byte) (int, error) {
	if r.status == 0 || r.status == 200 {
		r.buf.Write(b)
	}
	return r.ResponseWriter.Write(b)
}

// hot cells: small LRU of gzipped cell bodies (post addParcelHashes).
const hotCellMax = 80

var (
	hotMu    sync.Mutex
	hotCells = map[string]*hotCell{}
)

type hotCell struct {
	gz  []byte
	at  time.Time // last use (LRU)
	exp time.Time // hard expiry: never outlive the api_cache row
}

func hotCellGet(key string) []byte {
	hotMu.Lock()
	defer hotMu.Unlock()
	h := hotCells[key]
	if h == nil || time.Since(h.at) > 2*time.Hour || time.Now().After(h.exp) {
		delete(hotCells, key)
		return nil
	}
	h.at = time.Now()
	return h.gz
}

func hotCellPut(key string, plain []byte) {
	if len(plain) == 0 || bytes.Contains(plain, []byte(`"ready":false`)) {
		return
	}
	exp := time.Now().Add(time.Duration(cadastreTTL))
	go func() {
		var buf bytes.Buffer
		gw, _ := gzip.NewWriterLevel(&buf, 6)
		gw.Write(plain)
		gw.Close()
		hotMu.Lock()
		defer hotMu.Unlock()
		if len(hotCells) >= hotCellMax {
			var oldest string
			var ot time.Time
			for k, h := range hotCells {
				if oldest == "" || h.at.Before(ot) {
					oldest, ot = k, h.at
				}
			}
			delete(hotCells, oldest)
		}
		hotCells[key] = &hotCell{gz: buf.Bytes(), at: time.Now(), exp: exp}
	}()
}

// addParcelHashes inserts "ph" (parcel hash) and "ezh" (folio hash) after
// every "parcel_id":"…" / "ez":"…" of a cell body at serve time. Byte scan,
// no JSON round-trip (~0.3 ms per MB); the cached body stays hash-free so a
// cache row never pairs an id with its storage token.
func addParcelHashes(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"parcel_id":"`)) {
		return body
	}
	out := make([]byte, 0, len(body)+len(body)/20)
	pidKey, ezKey := []byte(`"parcel_id":"`), []byte(`"ez":"`)
	i := 0
	curKG := ""
	for i < len(body) {
		j := bytes.Index(body[i:], []byte(`"`))
		if j < 0 {
			break
		}
		j += i
		if bytes.HasPrefix(body[j:], pidKey) {
			st := j + len(pidKey)
			en := bytes.IndexByte(body[st:], '"')
			if en < 0 {
				break
			}
			pid := string(body[st : st+en])
			curKG = kgCodeOf(pid)
			out = append(out, body[i:st+en+1]...)
			out = append(out, []byte(`,"ph":"`+parcelHash(pid)+`"`)...)
			i = st + en + 1
			continue
		}
		if bytes.HasPrefix(body[j:], ezKey) {
			st := j + len(ezKey)
			en := bytes.IndexByte(body[st:], '"')
			if en < 0 {
				break
			}
			ez := string(body[st : st+en])
			out = append(out, body[i:st+en+1]...)
			if ez != "" {
				out = append(out, []byte(`,"ezh":"`+ezHash(curKG, ez)+`"`)...)
			}
			i = st + en + 1
			continue
		}
		out = append(out, body[i:j+1]...)
		i = j + 1
	}
	out = append(out, body[i:]...)
	return out
}

// handleViewportLanduse: same cell build as /api/viewport, only the landuse
// array returned (lighter for callers that only need Benützungsart polygons).
func (s *Server) handleViewportLanduse(w http.ResponseWriter, r *http.Request) {
	b, ok := parseBBox(r.URL.Query())
	if !ok {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	key, _ := vpCacheKey(b)
	body, st := s.cellJSON(b, key)
	if st != 200 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		w.Write(body)
		return
	}
	var full map[string]json.RawMessage
	json.Unmarshal(body, &full)
	out := map[string]json.RawMessage{"landuse": full["landuse"], "ready": full["ready"], "notice": full["notice"]}
	if v, ok := full["retry_after_s"]; ok {
		out["retry_after_s"] = v
	}
	jsonResp(w, out)
}

// cellJSON returns the cached cell body or builds it (singleflight).
func (s *Server) cellJSON(b bbox, key string) ([]byte, int) {
	if c, err := s.Q.GetCachedData(context.Background(), key); err == nil {
		return []byte(c), 200
	}
	v, _, _ := s.sf.Do(key, func() (any, error) {
		if c, err := s.Q.GetCachedData(context.Background(), key); err == nil {
			return sfRes{[]byte(c), 200}, nil
		}
		out, st := s.buildCell(b, key)
		return sfRes{out, st}, nil
	})
	r := v.(sfRes)
	return r.body, r.status
}

// ---------------------------------------------------------------------------

type bevParcel struct {
	ParcelID     string             `json:"parcel_id"`
	KG           string             `json:"kg_code"`
	GNR          string             `json:"gnr"`
	EZ           string             `json:"ez"`
	RStatus      string             `json:"rstatus,omitempty"`
	AreaSqm      float64            `json:"area_sqm"`
	Lon          float64            `json:"lon"`
	Lat          float64            `json:"lat"`
	Complete     bool               `json:"complete"`
	Parts        int                `json:"parts,omitempty"`
	DominantNS   string             `json:"dominant_ns,omitempty"`
	LanduseAreas map[string]float64 `json:"landuse_areas,omitempty"`
	BuildingCnt  int                `json:"building_count"`
	BuildingArea float64            `json:"total_building_area_sqm"`
	Geometry     json.RawMessage    `json:"geometry"`
	// terrain enrichment (srtm heightfield), omitted when no grid25
	Elev     *float64           `json:"elev_m,omitempty"`
	ElevMin  *float64           `json:"elev_min_m,omitempty"`
	ElevMax  *float64           `json:"elev_max_m,omitempty"`
	Slope    *float64           `json:"slope_deg,omitempty"`
	Aspect   string             `json:"aspect,omitempty"`
	DomTerr  string             `json:"dom_terrain,omitempty"`
	Fracs    map[string]float64 `json:"fracs,omitempty"`
	TreeFrac *float64           `json:"tree_frac,omitempty"`
	// observed block (srtm NE cells, product ≥ v2.4) — necells.go
	NE *neParcel `json:"ne,omitempty"`
}

type bevFootprint struct {
	ID          string          `json:"id"`
	FootprintID string          `json:"footprint_id"`
	ParcelID    string          `json:"parcel_id"`
	NSCode      string          `json:"ns_code"`
	AreaSqm     float64         `json:"area_sqm"`
	Lon         float64         `json:"lon"`
	Lat         float64         `json:"lat"`
	OBBLen      float64         `json:"obb_length_m,omitempty"`
	OBBWid      float64         `json:"obb_width_m,omitempty"`
	Orient      float64         `json:"orientation_deg,omitempty"`
	Geometry    json.RawMessage `json:"geometry"`
	NE          *neFootprint    `json:"ne,omitempty"` // segmented structure inside the ring (necells.go)
}

type bevLanduse struct {
	NS       int             `json:"ns"`
	Code     string          `json:"code"`
	AreaSqm  float64         `json:"area_sqm"`
	Geometry json.RawMessage `json:"geometry"`
}

type bevViewport struct {
	Parcels    []bevParcel     `json:"parcels"`
	Footprints []bevFootprint  `json:"footprints"`
	Landuse    []bevLanduse    `json:"landuse"`
	Ready      bool            `json:"ready"`
	Pending    bool            `json:"pending"`
	Truncated  bool            `json:"truncated"`
	RetryAfter float64         `json:"retry_after_s"`
	Notice     string          `json:"notice"`
	Stats      json.RawMessage `json:"stats,omitempty"`
	QueryMs    float64         `json:"query_time_ms"`
}

type vpKG struct {
	KG       string `json:"kg_code"`
	Name     string `json:"kg_name"`
	Gemeinde string `json:"gemeinde_code"`
	GemName  string `json:"gemeinde_name"`
	Enhanced bool   `json:"enhanced"` // grid25 terrain present in this cell
	NE       bool   `json:"ne"`       // NE cells (observed layer) present in this cell
}

// buildCell fetches bevdirect + the srtm heightfield in parallel, enriches,
// re-emits, caches (24 h, only when ready). Returns (body, status).
func (s *Server) buildCell(b bbox, key string) ([]byte, int) {
	t0 := time.Now()
	var (
		vp     bevViewport
		vpErr  error
		vpSt   int
		vpDown []byte // breaker body when the cadastre breaker is open
		hf     *heightfield
		ne     *neCols
		neSt   *neStatus
		neCode int
		wg     sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		u := bevAPI + "/viewport?" + b.qs() + "&layers=parcels,footprints,landuse&wait=" + strconv.Itoa(bevWait)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(bevWait+20)*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := upstreamClient.Do(req)
		if err != nil {
			vpErr = err
			return
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 80<<20))
		vpSt = resp.StatusCode
		if err != nil {
			vpErr = err
			return
		}
		if resp.StatusCode == 503 && resp.Header.Get("X-Upstream") == "down" {
			// breaker open: keep its {status:"down", retry_after_s} body so
			// the client paces instead of seeing a generic 502.
			vpDown = raw
			return
		}
		if resp.StatusCode != 200 && resp.StatusCode != 202 {
			vpErr = fmt.Errorf("bevdirect %d: %s", resp.StatusCode, truncStr(string(raw), 200))
			return
		}
		vpErr = json.Unmarshal(raw, &vp)
	}()
	go func() {
		defer wg.Done()
		hf = s.fetchHeightfield(b.pad(0.004))
	}()
	go func() {
		defer wg.Done()
		if c, ok := isAlignedCell(b.W, b.S, b.E, b.N); ok {
			ne, neSt, neCode = s.neCells(c)
		}
	}()
	wg.Wait()
	if vpDown != nil {
		return vpDown, 503
	}
	if vpErr != nil {
		slog.Warn("viewport: bevdirect", "err", vpErr, "status", vpSt)
		if upstreamBreaker != nil && vpSt == 0 {
			return []byte(`{"error":"cadastre service unavailable","status":"down","service":"cadastre","retry_after_s":20}`), 503
		}
		return jsonErrBody("cadastre service error"), 502
	}
	ready := vp.Ready && !vp.Pending

	// --- enrich parcels ---
	kgSeen := map[string]*vpKG{}
	adm := admin()
	neParcels := 0
	for i := range vp.Parcels {
		p := &vp.Parcels[i]
		if p.KG == "" {
			p.KG = kgCodeOf(p.ParcelID)
		}
		if _, ok := kgSeen[p.KG]; !ok {
			e := &vpKG{KG: p.KG}
			if a := adm.KGs[p.KG]; a != nil {
				e.Name, e.Gemeinde, e.GemName = a.Name, a.Gemeinde, a.GemName
			}
			kgSeen[p.KG] = e
		}
		// Observed layer first (NE cells, res-12 hexagons incl. trees and
		// structures); the 25 m heightfield only where the KG has no NE
		// product yet.
		if ne != nil && neEnrichParcel(p, ne) {
			kgSeen[p.KG].Enhanced = true
			// A parcel of an unprocessed KG can still be enriched from the
			// neighbour KG's cells along the boundary — that does not make
			// the KG "observed".
			kgSeen[p.KG].NE = !neKGMissing(ne, p.KG)
			neParcels++
		} else if hf != nil {
			if enrichParcel(p, hf) {
				kgSeen[p.KG].Enhanced = true
			}
		}
		p.Geometry = roundGeomRaw(p.Geometry)
	}
	for i := range vp.Footprints {
		f := &vp.Footprints[i]
		f.FootprintID = f.ID
		f.ID = ""
		if ne != nil {
			neEnrichFootprint(f, ne)
		}
		f.Geometry = roundGeomRaw(f.Geometry)
	}
	for i := range vp.Landuse {
		l := &vp.Landuse[i]
		l.Code = strconv.Itoa(l.NS)
		l.Geometry = roundGeomRaw(l.Geometry)
	}
	kgs := make([]*vpKG, 0, len(kgSeen))
	for _, k := range kgSeen {
		kgs = append(kgs, k)
	}
	sort.Slice(kgs, func(i, j int) bool { return kgs[i].KG < kgs[j].KG })

	notice := vp.Notice
	if notice == "" {
		notice = bevNotice
	}
	// complete:false = the parcel runs past this cell's 0.004° assembly pad; the
	// neighbouring cell carries a complete copy. The client dedups "complete
	// copy wins" (glitch #39) — the count is informational.
	incomplete := 0
	for i := range vp.Parcels {
		if !vp.Parcels[i].Complete {
			incomplete++
		}
	}
	ttl := time.Duration(cadastreTTL)
	out := map[string]any{
		"incomplete": incomplete,
		"parcels":    nonNil(vp.Parcels),
		"footprints": nonNil(vp.Footprints),
		"landuse":    nonNil(vp.Landuse),
		"kgs":        kgs,
		"ready":      ready,
		"truncated":  vp.Truncated,
		"notice":     notice,
		"license":    "https://creativecommons.org/licenses/by/4.0/",
		"source":     "bev-tiles",
		"terrain":    hf != nil || ne != nil,
		"built_ms":   time.Since(t0).Milliseconds(),
	}
	if ne != nil {
		out["ne"] = map[string]any{"ready": true, "epoch": ne.Meta.Epoch, "partial": ne.Meta.Partial, "kgs": ne.Meta.KGs,
			"kgs_missing": ne.Meta.KGsMissing, "cells": len(ne.Cells), "trees": len(ne.Trees.Lon), "structures": len(ne.Structures.Lon),
			"parcels": neParcels, "attribution": neAttribution}
	} else if neSt != nil {
		out["ne"] = map[string]any{"ready": false, "status": neSt.Status, "retry_after_s": neSt.RetryAfter, "kgs_missing": neSt.KGsMissing, "code": neCode}
	}
	if c, ok := isAlignedCell(b.W, b.S, b.E, b.N); ok {
		out["cell"] = map[string]int{"i": c.I, "j": c.J}
	}
	if !ready {
		ra := vp.RetryAfter
		if ra <= 0 {
			ra = 4
		}
		out["retry_after_s"] = ra
		out["pending"] = true
	}
	enc, err := json.Marshal(out)
	if err != nil {
		return jsonErrBody("encode error"), 500
	}
	if ready {
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{
			CacheKey: key, Data: string(enc), ExpiresAt: time.Now().Add(ttl),
		})
	}
	slog.Info("viewport cell built", "key", key, "parcels", len(vp.Parcels), "incomplete", incomplete, "footprints", len(vp.Footprints),
		"landuse", len(vp.Landuse), "ready", ready, "terrain", hf != nil, "ne", ne != nil, "ne_parcels", neParcels, "ms", time.Since(t0).Milliseconds(), "bev_ms", vp.QueryMs)
	return enc, 200
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func truncStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// roundGeomRaw rounds coordinates to 1e-6° (~0.1 m) — the geometry already
// comes rounded to 1e-7 from bevdirect; this mostly normalises and shrinks.
func roundGeomRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var g map[string]any
	if json.Unmarshal(raw, &g) != nil {
		return raw
	}
	roundCoords(g)
	enc, err := json.Marshal(g)
	if err != nil {
		return raw
	}
	return enc
}

// ---------------------------------------------------------------------------
// srtm heightfield: 25 m DTM + land-cover class per cell for a bbox.

type heightfield struct {
	W, S, E, N float64
	Cols, Rows int
	CellM      float64
	Z          [][]*float64
	LC         [][]int
	Legend     map[int]string
}

var lcNatural = map[string]bool{"tree": true, "shrub": true, "grass": true, "water": true, "crop": true, "orchard": true, "garden": true, "rock": true, "vineyard": true, "bare_soil": true, "glacier": true, "hedge": true, "wetland": true}

// fetchHeightfield returns nil where srtm has no grid25 product (404) or on
// any error — enrichment is optional, the cadastre never waits for it.
// Cached 24 h per padded cell.
func (s *Server) fetchHeightfield(b bbox) *heightfield {
	key := fmt.Sprintf("hf:v1:%.4f,%.4f,%.4f,%.4f", b.W, b.S, b.E, b.N)
	u := fmt.Sprintf("%s/heightfield?bbox=%.5f,%.5f,%.5f,%.5f&cell=25&landcover=1", lidarAPI, b.W, b.S, b.E, b.N)
	body, st := s.llmGet(key, u, 24*time.Hour)
	if st != 200 {
		return nil
	}
	var d struct {
		BBox   []float64         `json:"bbox"`
		Cols   int               `json:"cols"`
		Rows   int               `json:"rows"`
		CellM  float64           `json:"cell_m"`
		Z      [][]*float64      `json:"z"`
		LC     [][]int           `json:"landcover"`
		Legend map[string]string `json:"landcover_legend"`
		Ready  *bool             `json:"ready"`
	}
	if json.Unmarshal(body, &d) != nil || d.Cols == 0 || d.Rows == 0 || len(d.Z) != d.Rows || len(d.BBox) != 4 {
		return nil
	}
	if d.Ready != nil && !*d.Ready {
		return nil
	}
	hf := &heightfield{W: d.BBox[0], S: d.BBox[1], E: d.BBox[2], N: d.BBox[3], Cols: d.Cols, Rows: d.Rows, CellM: d.CellM, Z: d.Z, LC: d.LC, Legend: map[int]string{}}
	if hf.CellM == 0 {
		hf.CellM = 25
	}
	for k, v := range d.Legend {
		if n, err := strconv.Atoi(k); err == nil {
			hf.Legend[n] = v
		}
	}
	return hf
}

// cell (col,row) centre in lon/lat; rows run north→south.
func (h *heightfield) centre(c, r int) (float64, float64) {
	return h.W + (float64(c)+0.5)*(h.E-h.W)/float64(h.Cols), h.N - (float64(r)+0.5)*(h.N-h.S)/float64(h.Rows)
}

func (h *heightfield) colRow(lon, lat float64) (int, int) {
	c := int((lon - h.W) / (h.E - h.W) * float64(h.Cols))
	r := int((h.N - lat) / (h.N - h.S) * float64(h.Rows))
	return c, r
}

func (h *heightfield) z(c, r int) (float64, bool) {
	if r < 0 || r >= h.Rows || c < 0 || c >= h.Cols || h.Z[r] == nil || c >= len(h.Z[r]) || h.Z[r][c] == nil {
		return 0, false
	}
	return *h.Z[r][c], true
}

func (h *heightfield) lc(c, r int) string {
	if h.LC == nil || r < 0 || r >= len(h.LC) || c < 0 || c >= len(h.LC[r]) {
		return ""
	}
	return h.Legend[h.LC[r][c]]
}

// slopeAspect from central differences (Horn-lite) in metres.
func (h *heightfield) slopeAspect(c, r int) (slope, aspect float64, ok bool) {
	zw, ok1 := h.z(c-1, r)
	ze, ok2 := h.z(c+1, r)
	zn, ok3 := h.z(c, r-1)
	zs, ok4 := h.z(c, r+1)
	if !(ok1 && ok2 && ok3 && ok4) {
		return 0, 0, false
	}
	dzdx := (ze - zw) / (2 * h.CellM)
	dzdy := (zn - zs) / (2 * h.CellM)
	slope = math.Atan(math.Hypot(dzdx, dzdy)) * 180 / math.Pi
	aspect = math.Mod(math.Atan2(dzdx, dzdy)*180/math.Pi+180+360, 360) // downslope direction, 0 = N
	return slope, aspect, true
}

func aspectName(deg float64) string {
	names := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	return names[int(math.Round(deg/45))%8]
}

// enrichParcel samples every heightfield cell whose centre lies inside the
// parcel (even-odd over all rings; small parcels fall back to the cell under
// the representative point). Returns true when something was written.
func enrichParcel(p *bevParcel, hf *heightfield) bool {
	rings := geomRings(p.Geometry)
	if len(rings) == 0 {
		return false
	}
	minLon, minLat, maxLon, maxLat := ringsBounds(rings)
	c0, r0 := hf.colRow(minLon, maxLat)
	c1, r1 := hf.colRow(maxLon, minLat)
	if c1 < c0 {
		c0, c1 = c1, c0
	}
	if r1 < r0 {
		r0, r1 = r1, r0
	}
	// Big alpine parcels: cap the work by striding.
	n := (c1 - c0 + 1) * (r1 - r0 + 1)
	stride := 1
	for n/(stride*stride) > 3000 {
		stride++
	}
	var sum, mn, mx float64
	mn, mx = math.Inf(1), math.Inf(-1)
	cnt := 0
	var slopeSum, sinA, cosA float64
	slopeN := 0
	lcCount := map[string]int{}
	lcTotal := 0
	visit := func(c, r int) {
		if z, ok := hf.z(c, r); ok {
			sum += z
			cnt++
			if z < mn {
				mn = z
			}
			if z > mx {
				mx = z
			}
			if sl, as, ok := hf.slopeAspect(c, r); ok {
				slopeSum += sl
				slopeN++
				sinA += math.Sin(as * math.Pi / 180)
				cosA += math.Cos(as * math.Pi / 180)
			}
		}
		if t := hf.lc(c, r); t != "" {
			lcCount[t]++
			lcTotal++
		}
	}
	for r := r0; r <= r1; r += stride {
		for c := c0; c <= c1; c += stride {
			lon, lat := hf.centre(c, r)
			if pipRingsGo(lon, lat, rings) {
				visit(c, r)
			}
		}
	}
	if cnt == 0 {
		c, r := hf.colRow(p.Lon, p.Lat)
		visit(c, r)
	}
	if cnt == 0 && lcTotal == 0 {
		return false
	}
	round1 := func(v float64) *float64 { x := math.Round(v*10) / 10; return &x }
	if cnt > 0 {
		p.Elev = round1(sum / float64(cnt))
		p.ElevMin, p.ElevMax = round1(mn), round1(mx)
	}
	if slopeN > 0 {
		p.Slope = round1(slopeSum / float64(slopeN))
		p.Aspect = aspectName(math.Mod(math.Atan2(sinA, cosA)*180/math.Pi+360, 360))
	}
	// A parcel smaller than a 25 m cell inherits one cell's class — noise next
	// to the measured Benützungsart; keep only the elevation then.
	if lcTotal > 0 && (cnt > 1 || p.AreaSqm >= 400) {
		fr := map[string]float64{}
		for t, c := range lcCount {
			f := float64(c) / float64(lcTotal)
			if f >= 0.02 {
				fr[t] = math.Round(f*100) / 100
			}
		}
		p.Fracs = fr
		tf := float64(lcCount["tree"]) / float64(lcTotal)
		tf = math.Round(tf*100) / 100
		p.TreeFrac = &tf
		best, bestN := "", 0
		for t, c := range lcCount {
			if lcNatural[t] && c > bestN {
				best, bestN = t, c
			}
		}
		p.DomTerr = best
	}
	return true
}

// --- tiny geometry helpers on raw GeoJSON (Polygon / MultiPolygon) ---

func geomRings(raw json.RawMessage) [][][2]float64 {
	var g struct {
		Type   string          `json:"type"`
		Coords json.RawMessage `json:"coordinates"`
	}
	if json.Unmarshal(raw, &g) != nil {
		return nil
	}
	var polys [][][][2]float64
	switch g.Type {
	case "Polygon":
		var p [][][2]float64
		if json.Unmarshal(g.Coords, &p) != nil {
			return nil
		}
		polys = [][][][2]float64{p}
	case "MultiPolygon":
		if json.Unmarshal(g.Coords, &polys) != nil {
			return nil
		}
	default:
		return nil
	}
	var rings [][][2]float64
	for _, p := range polys {
		for _, r := range p {
			if len(r) >= 3 {
				rings = append(rings, r)
			}
		}
	}
	return rings
}

func ringsBounds(rings [][][2]float64) (minLon, minLat, maxLon, maxLat float64) {
	minLon, minLat, maxLon, maxLat = 999, 999, -999, -999
	for _, r := range rings {
		for _, c := range r {
			minLon, maxLon = math.Min(minLon, c[0]), math.Max(maxLon, c[0])
			minLat, maxLat = math.Min(minLat, c[1]), math.Max(maxLat, c[1])
		}
	}
	return
}

// pipRingsGo: even-odd point-in-polygon over all rings (holes excluded).
func pipRingsGo(lon, lat float64, rings [][][2]float64) bool {
	in := false
	for _, r := range rings {
		n := len(r)
		for i, j := 0, n-1; i < n; j, i = i, i+1 {
			yi, yj := r[i][1], r[j][1]
			if (yi > lat) != (yj > lat) {
				x := (r[j][0]-r[i][0])*(lat-yi)/(yj-yi) + r[i][0]
				if lon < x {
					in = !in
				}
			}
		}
	}
	return in
}

// bboxOfRings is used by callers that need the bbox of a parcel geometry.
func bboxOfRaw(raw json.RawMessage) (bbox, bool) {
	rings := geomRings(raw)
	if len(rings) == 0 {
		return bbox{}, false
	}
	w, s, e, n := ringsBounds(rings)
	return bbox{w, s, e, n}, true
}

var _ = strings.TrimSpace

// neKGMissing: srtm lists KGs without an NE product in meta.kgs_missing.
func neKGMissing(ne *neCols, kg string) bool {
	for _, m := range ne.Meta.KGsMissing {
		if m == kg {
			return true
		}
	}
	return false
}
