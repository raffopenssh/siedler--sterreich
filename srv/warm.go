package srv

// Prewarming of cadastre cells (bevdirect-serve → api_cache, 24 h max).
//
//   - Daily plan: 100 KGs/day in 20 patches (5 KGs each): a small, fully
//     srtm-enhanced Gemeinde (grid25 → relief, giants) plus its nearest
//     neighbour KGs (enhanced or not), one patch every 24h/20, spread over
//     the Bundesländer. 20 destinations → 20 lucky players land in 20 places.
//   - Neighbour warming: the first viewport build in a cell enqueues the KGs
//     touching that cell plus their adjacent KGs (low priority).
//   - Session warming: POST /api/session/create enqueues the Gemeinde's KGs.
//   - One worker, polite pacing (~0.8 s between cells, yields to foreground
//     viewport builds), never re-warms a KG that is fresh for ≥ 2 h.
//
// GET /api/lucky picks a Gemeinde whose KGs are warm and enhanced. GET /api/warm/status
// shows plan + queue.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"srv.exe.dev/db/dbgen"
)

const (
	warmDailyKGs   = 100
	warmPatches    = 20 // 20 destinations a day → 20 lucky players land in 20 places
	warmPatchSize  = warmDailyKGs / warmPatches
	warmPlanVer    = "v4"
	warmCellPause  = 800 * time.Millisecond
	warmFreshGuard = 2 * time.Hour // don't re-warm what is still fresh for this long
)

type warmJob struct {
	KG     string `json:"kg"`
	Reason string `json:"reason"`
	Prio   int    `json:"prio"` // 0 session, 1 neighbour, 2 daily
	At     time.Time
}

type warmer struct {
	mu      sync.Mutex
	queue   []warmJob
	queued  map[string]bool
	wake    chan struct{}
	current atomic.Value // string
	done    atomic.Int64
	cells   atomic.Int64
	fg      atomic.Int32 // foreground cell builds in flight
	plan    atomic.Value // *warmPlan
	seen    sync.Map     // cellID → time of last neighbour trigger
}

type warmPlan struct {
	Date    string          `json:"date"`
	Patches [][]string      `json:"patches"`
	Seeds   []string        `json:"seeds"`
	Started map[string]bool `json:"started"` // patch index → kicked
}

func newWarmer() *warmer {
	w := &warmer{queued: map[string]bool{}, wake: make(chan struct{}, 1)}
	w.current.Store("")
	return w
}

// enqueueWarm adds a KG unless it is fresh or already queued.
func (s *Server) enqueueWarm(kg, reason string, prio int) bool {
	if admin().KGs[kg] == nil {
		return false
	}
	w := s.warm
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.queued[kg] || w.current.Load().(string) == kg {
		return false
	}
	if exp, ok := s.kgWarmExpiry(kg); ok && time.Until(exp) > warmFreshGuard {
		return false
	}
	w.queued[kg] = true
	w.queue = append(w.queue, warmJob{KG: kg, Reason: reason, Prio: prio, At: time.Now()})
	sort.SliceStable(w.queue, func(i, j int) bool { return w.queue[i].Prio < w.queue[j].Prio })
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return true
}

func (s *Server) kgWarmExpiry(kg string) (time.Time, bool) {
	var ts string
	err := s.DB.QueryRowContext(context.Background(), "SELECT expires_at FROM kg_warm WHERE kg_code = ?", kg).Scan(&ts)
	if err != nil {
		return time.Time{}, false
	}
	t, err := parseSQLiteTime(ts)
	return t, err == nil
}

func parseSQLiteTime(ts string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05.999999999-07:00"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time %q", ts)
}

// noteCellLoaded is called after a foreground cell build: warm the rest of
// the KGs in that cell and their neighbours in the background (debounced
// per cell for an hour).
func (s *Server) noteCellLoaded(c cellID, reason string) {
	if t, ok := s.warm.seen.Load(c); ok && time.Since(t.(time.Time)) < time.Hour {
		return
	}
	s.warm.seen.Store(c, time.Now())
	adm := admin()
	n := 0
	for _, kg := range adm.byCell[c] {
		if s.enqueueWarm(kg, "neighbour", 1) {
			n++
		}
		for i, nb := range adm.neighbours(kg, 0.002) {
			if i >= 6 {
				break
			}
			if s.enqueueWarm(nb.KG, "neighbour", 2) {
				n++
			}
		}
	}
	if n > 0 {
		slog.Info("warm: neighbour jobs queued", "cell", c, "kgs", n, "trigger", reason)
	}
}

// warmGemeinde enqueues every KG of a Gemeinde (session create).
func (s *Server) warmGemeinde(code string, prio int, reason string) int {
	g := admin().Gemeinde[code]
	if g == nil {
		return 0
	}
	n := 0
	for _, kg := range g.KGs {
		if s.enqueueWarm(kg, reason, prio) {
			n++
		}
	}
	return n
}

// warmLoop is the single background worker.
func (s *Server) warmLoop() {
	for {
		s.warm.mu.Lock()
		var job *warmJob
		if len(s.warm.queue) > 0 {
			j := s.warm.queue[0]
			s.warm.queue = s.warm.queue[1:]
			delete(s.warm.queued, j.KG)
			job = &j
		}
		s.warm.mu.Unlock()
		if job == nil {
			select {
			case <-s.warm.wake:
			case <-time.After(time.Minute):
			}
			continue
		}
		s.warm.current.Store(job.KG)
		s.warmKG(job.KG, job.Reason)
		s.warm.current.Store("")
		s.warm.done.Add(1)
	}
}

// warmKG builds every uncached cell of the KG's bbox (polite pacing,
// yields to foreground builds) and records the result in kg_warm.
func (s *Server) warmKG(kg, reason string) {
	k := admin().KGs[kg]
	if k == nil {
		return
	}
	t0 := time.Now()
	cells := k.cells()
	built, parcels := 0, 0
	for _, c := range cells {
		for s.warm.fg.Load() > 0 { // foreground first
			time.Sleep(300 * time.Millisecond)
		}
		w, so, e, n := c.bbox()
		b := bbox{w, so, e, n}
		key, _ := vpCacheKey(b)
		if _, err := s.Q.GetCachedData(context.Background(), key); err == nil {
			continue
		}
		var body []byte
		var st int
		for attempt := 0; attempt < 4; attempt++ {
			body, st = s.cellJSON(b, key)
			if st != 200 {
				break
			}
			var r struct {
				Ready      bool    `json:"ready"`
				RetryAfter float64 `json:"retry_after_s"`
				Parcels    []struct {
					KG string `json:"kg_code"`
				} `json:"parcels"`
			}
			json.Unmarshal(body, &r)
			if r.Ready {
				for _, p := range r.Parcels {
					if p.KG == kg {
						parcels++
					}
				}
				built++
				break
			}
			wait := time.Duration(r.RetryAfter * float64(time.Second))
			if wait < 2*time.Second {
				wait = 2 * time.Second
			}
			if wait > 15*time.Second {
				wait = 15 * time.Second
			}
			time.Sleep(wait)
		}
		if st != 200 {
			slog.Warn("warm: cell failed", "kg", kg, "cell", c, "status", st)
			if st == 503 { // cadastre down: back off hard
				time.Sleep(30 * time.Second)
				return
			}
		}
		s.warm.cells.Add(1)
		time.Sleep(warmCellPause)
	}
	s.DB.ExecContext(context.Background(),
		`INSERT INTO kg_warm (kg_code, warmed_at, expires_at, cells, parcels, reason) VALUES (?, CURRENT_TIMESTAMP, ?, ?, ?, ?)
		 ON CONFLICT(kg_code) DO UPDATE SET warmed_at=excluded.warmed_at, expires_at=excluded.expires_at, cells=excluded.cells, parcels=excluded.parcels, reason=excluded.reason`,
		kg, time.Now().Add(time.Duration(cadastreTTL)).UTC().Format("2006-01-02 15:04:05"), len(cells), parcels, reason)
	slog.Info("warm: KG done", "kg", kg, "name", k.Name, "cells", len(cells), "built", built, "parcels", parcels, "reason", reason, "s", int(time.Since(t0).Seconds()))
}

// ---------------------------------------------------------------------------
// Daily plan

func (s *Server) warmPlanner() {
	// The plan prefers v2.4 (NE cells) Gemeinden — make sure the srtm KG
	// registry is loaded before the first plan of the day is made.
	if _, err := s.Q.GetCachedData(context.Background(), enhancedKGsKey); err != nil {
		s.buildEnhancedKGs(enhancedKGsKey)
	}
	// v2.4 KGs are always kept warm: whatever the plan says, re-queue the
	// ones that are not fresh (the in-memory queue does not survive a restart).
	go func() {
		for {
			n := 0
			for kg := range s.neReadyKGSet() {
				if len(kg) == 5 && s.enqueueWarm(kg, "v24", 1) {
					n++
				}
			}
			if n > 0 {
				slog.Info("warm: v2.4 KGs queued", "kgs", n)
			}
			time.Sleep(2 * time.Hour)
		}
	}()
	for {
		now := time.Now()
		if !kgUniverseOK() {
			// Universe unverified / changed (kguniverse.go): a plan keyed on
			// kg codes could warm the wrong KGs. Wait, re-check every 10 min.
			time.Sleep(10 * time.Minute)
			continue
		}
		plan := s.loadOrMakePlan(now)
		// Patch k is due at k × (24 h / patches) after local midnight.
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		slot := 24 * time.Hour / warmPatches
		for i := range plan.Patches {
			due := midnight.Add(time.Duration(i) * slot)
			if now.After(due) && !plan.Started[fmt.Sprint(i)] {
				plan.Started[fmt.Sprint(i)] = true
				s.savePlan(plan)
				n := 0
				for _, kg := range plan.Patches[i] {
					if s.enqueueWarm(kg, "daily", 2) {
						n++
					}
				}
				slog.Info("warm: daily patch queued", "patch", i, "seed", plan.Seeds[i], "kgs", n)
			}
		}
		// Sleep until the next slot or midnight, whichever is first.
		next := midnight.Add(24 * time.Hour)
		for i := range plan.Patches {
			if due := midnight.Add(time.Duration(i) * slot); due.After(now) && due.Before(next) {
				next = due
			}
		}
		time.Sleep(time.Until(next) + 5*time.Second)
	}
}

func (s *Server) planKey(now time.Time) string {
	return "warm-plan:" + warmPlanVer + ":" + now.Format("2006-01-02")
}

func (s *Server) loadOrMakePlan(now time.Time) *warmPlan {
	if p, ok := s.warm.plan.Load().(*warmPlan); ok && p != nil && p.Date == now.Format("2006-01-02") {
		return p
	}
	if raw, err := s.Q.GetCachedData(context.Background(), s.planKey(now)); err == nil {
		var p warmPlan
		if json.Unmarshal([]byte(raw), &p) == nil && len(p.Patches) > 0 {
			if p.Started == nil {
				p.Started = map[string]bool{}
			}
			s.warm.plan.Store(&p)
			return &p
		}
	}
	p := s.makePlan(now)
	s.savePlan(p)
	return p
}

func (s *Server) savePlan(p *warmPlan) {
	s.warm.plan.Store(p)
	enc, _ := json.Marshal(p)
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{
		CacheKey: "warm-plan:" + warmPlanVer + ":" + p.Date, Data: string(enc), ExpiresAt: time.Now().Add(48 * time.Hour),
	})
}

// makePlan picks warmPatches seed Gemeinden — each one fully srtm-enhanced
// (every KG grid25, so /api/lucky accepts it as a destination) and small
// enough to fit the per-patch budget — spread across Bundesländer, and
// grows each into a patch: the Gemeinde's own KGs first, then the nearest
// neighbouring KGs (enhanced or not) so a lucky player can roam a bit.
// Deterministic per date so a restart keeps the plan.
func (s *Server) makePlan(now time.Time) *warmPlan {
	adm := admin()
	rng := rand.New(rand.NewSource(int64(now.Year())*10000 + int64(now.YearDay())))
	enhKG := s.enhancedKGSet()
	var seeds []string
	for code, g := range adm.Gemeinde {
		if len(g.KGs) == 0 || len(g.KGs) > warmPatchSize {
			continue
		}
		ok := true
		for _, kg := range g.KGs {
			if len(enhKG) > 0 && !enhKG[kg] {
				ok = false
				break
			}
		}
		if ok {
			seeds = append(seeds, code)
		}
	}
	if len(seeds) == 0 { // registry unknown: any Gemeinde
		for code := range adm.Gemeinde {
			seeds = append(seeds, code)
		}
	}
	sort.Strings(seeds)
	rng.Shuffle(len(seeds), func(i, j int) { seeds[i], seeds[j] = seeds[j], seeds[i] })
	// v2.4 first: Gemeinden with an NE-cells KG (observed layer, the richest
	// enrichment we have) lead the day's plan, regardless of size or state
	// quota; the rest of the shuffled seeds follow.
	v24 := s.neReadyKGSet()
	if len(v24) > 0 {
		var first, rest []string
		seen := map[string]bool{}
		for code, g := range adm.Gemeinde {
			for _, kg := range g.KGs {
				if v24[kg] {
					first = append(first, code)
					seen[code] = true
					break
				}
			}
		}
		// Densest enhanced neighbourhood first: the biggest cluster a
		// Gemeinde's KGs sit in (luckycluster.go), ties by code — so the
		// day's early patches build contiguous enhanced blocks.
		size := map[string]int{}
		for _, code := range first {
			for _, kg := range adm.Gemeinde[code].KGs {
				if n := enhancedClusterSize(kg, enhKG); n > size[code] {
					size[code] = n
				}
			}
		}
		sort.Slice(first, func(i, j int) bool {
			if size[first[i]] != size[first[j]] {
				return size[first[i]] > size[first[j]]
			}
			return first[i] < first[j]
		})
		for _, c := range seeds {
			if !seen[c] {
				rest = append(rest, c)
			}
		}
		seeds = append(first, rest...)
	}
	used := map[string]bool{}
	fresh := s.freshWarmSet()
	plan := &warmPlan{Date: now.Format("2006-01-02"), Started: map[string]bool{}}
	usedState := map[string]int{}
	perState := (warmPatches + 8) / 9 * 2 // ≈ 2× the fair share per Bundesland
	for _, seed := range seeds {
		if len(plan.Patches) >= warmPatches {
			break
		}
		g := adm.Gemeinde[seed]
		isV24 := false
		for _, kg := range g.KGs {
			if v24[kg] {
				isV24 = true
				break
			}
		}
		if usedState[g.State] >= perState && !isV24 {
			continue
		}
		allFresh := true
		for _, kg := range g.KGs {
			if used[kg] {
				allFresh = false // overlaps an earlier patch
				break
			}
			if !fresh[kg] {
				allFresh = false
			}
		}
		if allFresh {
			continue // already warm (or taken) — pick a different destination
		}
		patch := s.growPatch(seed, used, fresh, enhKG)
		if len(patch) == 0 {
			continue
		}
		usedState[g.State]++
		plan.Patches = append(plan.Patches, patch)
		tag := ""
		if isV24 {
			tag = " · v2.4"
		}
		plan.Seeds = append(plan.Seeds, g.Name+" ("+g.State+")"+tag)
	}
	return plan
}

// growPatch: the seed Gemeinde's KGs, then BFS over KG adjacency (enhanced
// or not — roaming across the border is fine, only the spawn must be
// enhanced) until warmPatchSize KGs, skipping already-fresh ones.
func (s *Server) growPatch(seed string, used, fresh, enhanced map[string]bool) []string {
	adm := admin()
	g := adm.Gemeinde[seed]
	var out []string
	queue := append([]string{}, g.KGs...)
	for len(queue) > 0 && len(out) < warmPatchSize {
		kg := queue[0]
		queue = queue[1:]
		if used[kg] {
			continue
		}
		used[kg] = true
		if !fresh[kg] {
			out = append(out, kg)
		}
		// Enhanced neighbours first so the patch becomes a contiguous
		// enhanced block (a lucky spawn in its middle stays enhanced when
		// the player pans); plain neighbours fill the rest.
		var enh, plain []string
		for i, nb := range adm.neighbours(kg, 0.001) {
			if i >= 8 {
				break
			}
			if used[nb.KG] {
				continue
			}
			if enhanced[nb.KG] {
				enh = append(enh, nb.KG)
			} else {
				plain = append(plain, nb.KG)
			}
		}
		queue = append(queue, enh...)
		queue = append(queue, plain...)
	}
	return out
}

func (s *Server) freshWarmSet() map[string]bool {
	out := map[string]bool{}
	rows, err := s.DB.QueryContext(context.Background(), "SELECT kg_code FROM kg_warm WHERE expires_at > datetime('now', '+2 hours')")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kg string
		if rows.Scan(&kg) == nil {
			out[kg] = true
		}
	}
	return out
}

// enhancedGemeindeSet reads the cached enhanced-KG registry (srtm grid25)
// without triggering a fetch: Gemeinden with at least one v2/grid25 KG.
func (s *Server) enhancedGemeindeSet() map[string]bool {
	out := map[string]bool{}
	adm := admin()
	for kg := range s.enhancedKGSet() {
		if k := adm.KGs[kg]; k != nil {
			out[k.Gemeinde] = true
		}
	}
	return out
}

// enhancedKGSet: the KG codes with srtm grid25 terrain (v2) — the only ones
// with hillshade tiles, heightfields and the giant-tree index. Unpadded and
// padded forms both present so callers can look up either.
func (s *Server) enhancedKGSet() map[string]bool {
	out := map[string]bool{}
	raw, err := s.Q.GetCachedData(context.Background(), enhancedKGsKey)
	if err != nil {
		return out
	}
	var d struct {
		KGs []struct {
			KG string `json:"kg_code"`
			V2 bool   `json:"v2"`
		} `json:"kgs"`
	}
	if json.Unmarshal([]byte(raw), &d) != nil {
		return out
	}
	for _, k := range d.KGs {
		if k.V2 {
			out[k.KG] = true
			out[unpadKG(k.KG)] = true
			if len(k.KG) == 4 {
				out["0"+k.KG] = true
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// GET /api/lucky — a warm Gemeinde for "Auf Glück".

type luckyPick struct {
	GemeindeCode string   `json:"gemeinde_code"`
	Name         string   `json:"name"`
	Lon          float64  `json:"lon"`
	Lat          float64  `json:"lat"`
	State        string   `json:"state"`
	Enhanced     bool     `json:"enhanced"`
	NE           bool     `json:"ne"` // destination has NE cells (srtm v2.4 observed layer)
	Warm         bool     `json:"warm"`
	WarmKGs      int      `json:"warm_kgs"`
	KGs          []string `json:"kgs"`
	Pool         int      `json:"pool"`
	SpawnKG      string   `json:"spawn_kg,omitempty"`
	ClusterKGs   int      `json:"cluster_kgs,omitempty"`   // warm+enhanced KGs within ~1.5 km of the spawn
	ClusterTotal int      `json:"cluster_total,omitempty"` // all KGs within that box
	ClusterShare float64  `json:"cluster_share,omitempty"`
	Interest     float64  `json:"interest"` // luckyInterest 0..1 of the spawn surroundings (luckyinterest.go)
	InterestWhy  string   `json:"interest_why,omitempty"`
}

func (s *Server) handleLucky(w http.ResponseWriter, r *http.Request) {
	// ?lon&lat → only the interest score of that spot (QA, DEV.interest()).
	if q := r.URL.Query(); q.Get("lon") != "" {
		lon, _ := strconv.ParseFloat(q.Get("lon"), 64)
		lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
		sc, known := s.luckyInterest(lon, lat)
		jsonResp(w, map[string]any{"lon": lon, "lat": lat, "known": known, "interest": sc})
		return
	}
	jsonResp(w, s.luckyPick())
}

func (s *Server) luckyPick() luckyPick {
	adm := admin()
	fresh := s.freshWarmSet()
	enhKG := s.enhancedKGSet()
	enh := s.enhancedGemeindeSet()
	type cand struct {
		g    *gemeindeAdmin
		warm int
	}
	var full, partial []cand
	// A KG only counts as "playable" when it is warm AND srtm-enhanced: a
	// warm-but-bare KG has parcels, but no relief, no giant trees, no
	// heightfield — the player would think the landscape layer is broken.
	// Only when the registry is unknown (fresh install, srtm down) does warm
	// alone count.
	playable := map[string]bool{}
	for kg := range fresh {
		if len(enhKG) == 0 || enhKG[kg] {
			playable[kg] = true
		}
	}
	// Dense first: a spawn in the middle of several warm+enhanced KGs
	// (luckycluster.go). Falls through to the per-Gemeinde tiers when no
	// cluster qualifies yet (fresh install, few KGs warm).
	if len(enhKG) > 0 {
		if lp, ok := s.luckyClusterPick(playable, enhKG, s.neReadyKGSet()); ok {
			return lp
		}
	}
	byGem := map[string]int{}
	for kg := range playable {
		if k := adm.KGs[kg]; k != nil {
			byGem[k.Gemeinde]++
		}
	}
	for code, n := range byGem {
		g := adm.Gemeinde[code]
		if g == nil {
			continue
		}
		c := cand{g, n}
		if n >= len(g.KGs) || (len(g.KGs) >= 5 && n >= (len(g.KGs)*3+3)/4) {
			full = append(full, c)
		} else {
			// Partially playable Gemeinde: fine as long as the settlement
			// centre (where the player spawns) lies in a playable KG —
			// checked in luckyCenter below; keep them as a second tier.
			partial = append(partial, c)
		}
	}
	pick := func(pool []cand) (luckyPick, bool) {
		// Try a few random candidates; skip ones whose centre is not playable.
		var bestLP *luckyPick
		order := rand.Perm(len(pool))
		for i, idx := range order {
			if i >= 6 {
				break
			}
			c := pool[idx]
			lon, lat, ok := s.luckyCenter(c.g, playable)
			if !ok {
				continue
			}
			lp := luckyPick{GemeindeCode: c.g.Code, Name: c.g.Name, Lon: lon, Lat: lat, State: c.g.State, Enhanced: enh[c.g.Code], Warm: true, WarmKGs: c.warm, KGs: c.g.KGs, Pool: len(pool)}
			// Nudge the spawn to the most interesting spot nearby (brook,
			// forest edge, roofs) — never into a non-playable KG.
			if k := adm.kgAt(lon, lat); k != nil && playable[k.KG] {
				spot, sc := s.luckySpot(luckyCluster{kg: k, lon: lon, lat: lat}, playable, enhKG)
				lp.Lon, lp.Lat, lp.Interest, lp.InterestWhy = spot.lon, spot.lat, sc.Score, sc.Why
				if spot.kg != nil && (spot.lon != lon || spot.lat != lat) {
					lp.SpawnKG = spot.kg.KG // spawn_exact: session create must not re-snap
				}
			}
			if lp.Interest < interestMin && i < len(order)-1 && i < 5 {
				if bestLP == nil || lp.Interest > bestLP.Interest {
					cp := lp
					bestLP = &cp
				}
				continue
			}
			slog.Info("lucky: pick", "gemeinde", c.g.Code, "name", c.g.Name, "enhanced", lp.Enhanced, "warm_kgs", c.warm, "pool", len(pool), "interest", lp.Interest, "why", lp.InterestWhy)
			return lp, true
		}
		if bestLP != nil {
			slog.Info("lucky: pick (best of dull)", "gemeinde", bestLP.GemeindeCode, "interest", bestLP.Interest)
			return *bestLP, true
		}
		return luckyPick{}, false
	}
	// Prefer destinations with the observed layer (v2.4 NE cells) — about
	// 2 of 3 picks when any are warm, so other warm places still get visits.
	v24 := s.neReadyKGSet()
	if len(v24) > 0 && rand.Intn(3) != 0 {
		var pref []cand
		for _, c := range append(append([]cand{}, full...), partial...) {
			for _, kg := range c.g.KGs {
				if v24[kg] && playable[kg] {
					pref = append(pref, c)
					break
				}
			}
		}
		if len(pref) > 0 {
			if lp, ok := pick(pref); ok {
				lp.NE = true
				return lp
			}
		}
	}
	if len(full) > 0 {
		if lp, ok := pick(full); ok {
			return lp
		}
	}
	if len(partial) > 0 {
		if lp, ok := pick(partial); ok {
			return lp
		}
	}
	// Nothing warm yet (fresh install): enhanced Gemeinde at random, and
	// start warming it right away so the player's first pans are cheap.
	var codes []string
	for code := range enh {
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		for code := range adm.Gemeinde {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	g := adm.Gemeinde[codes[rand.Intn(len(codes))]]
	s.warmGemeinde(g.Code, 0, "lucky")
	lon, lat := g.center()
	if slon, slat, ok := s.settlementCenter(g.Name, lon, lat); ok {
		lon, lat = slon, slat
	}
	slog.Info("lucky: cold pick (nothing warm+enhanced)", "gemeinde", g.Code, "name", g.Name, "enhanced", enh[g.Code])
	return luckyPick{GemeindeCode: g.Code, Name: g.Name, Lon: lon, Lat: lat, State: g.State, Enhanced: enh[g.Code], KGs: g.KGs, Pool: 0}
}

// luckyCenter: where a lucky player spawns. The Gemeinde bbox centre is a
// bad spawn (St. Gallenkirch: 3.8 km up the valley from the village, zero
// parcels within 300 m), so: the OSM settlement (settlementCenter, memoised
// 24 h) when it lies in a warm KG; otherwise the centre of the warm KG with
// the most parcels (kg_warm) — never a cold KG. ok=false when no warm KG of
// the Gemeinde qualifies.
func (s *Server) luckyCenter(g *gemeindeAdmin, fresh map[string]bool) (float64, float64, bool) {
	adm := admin()
	inWarm := func(lon, lat float64) bool {
		k := adm.kgAt(lon, lat)
		return k != nil && k.Gemeinde == g.Code && fresh[k.KG]
	}
	blon, blat := g.center()
	if lon, lat, ok := s.settlementCenter(g.Name, blon, blat); ok && inWarm(lon, lat) {
		return lon, lat, true
	}
	// Warm KG with the most parcels (ties → larger area).
	var best *kgAdmin
	bestN := -1
	for _, kg := range g.KGs {
		if !fresh[kg] {
			continue
		}
		k := adm.KGs[kg]
		if k == nil {
			continue
		}
		var n int
		s.DB.QueryRowContext(context.Background(), "SELECT parcels FROM kg_warm WHERE kg_code = ?", kg).Scan(&n)
		if n > bestN || (n == bestN && best != nil && k.AreaKm2 > best.AreaKm2) {
			best, bestN = k, n
		}
	}
	if best == nil {
		return 0, 0, false
	}
	lon, lat := best.center()
	return lon, lat, true
}

// GET /api/warm/status
func (s *Server) handleWarmStatus(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, s.warmStatusMap())
}

// warmStatusMap is the /api/warm/status body; also embedded in /api/metrics.
func (s *Server) warmStatusMap() map[string]any {
	s.warm.mu.Lock()
	q := make([]warmJob, len(s.warm.queue))
	copy(q, s.warm.queue)
	s.warm.mu.Unlock()
	var warmKGs, warmGem int
	gem := map[string]bool{}
	rows, err := s.DB.QueryContext(context.Background(), "SELECT kg_code FROM kg_warm WHERE expires_at > CURRENT_TIMESTAMP")
	if err == nil {
		for rows.Next() {
			var kg string
			rows.Scan(&kg)
			warmKGs++
			if k := admin().KGs[kg]; k != nil {
				gem[k.Gemeinde] = true
			}
		}
		rows.Close()
	}
	warmGem = len(gem)
	queueLen := len(q)
	if len(q) > 40 {
		q = q[:40]
	}
	plan, _ := s.warm.plan.Load().(*warmPlan)
	var v24 []string
	for kg := range s.neReadyKGSet() {
		if len(kg) == 5 {
			v24 = append(v24, kg)
		}
	}
	sort.Strings(v24)
	v24Warm := 0
	fresh := s.freshWarmSet()
	for _, kg := range v24 {
		if fresh[kg] {
			v24Warm++
		}
	}
	return map[string]any{
		"current": s.warm.current.Load(), "queue": q, "queue_len": queueLen, "done": s.warm.done.Load(),
		"cells_built": s.warm.cells.Load(), "warm_kgs": warmKGs, "warm_gemeinden": warmGem,
		"v24_kgs": v24, "v24_warm": v24Warm, "kg_universe": kgUniverseStatus(),
		"plan": plan, "policy": map[string]any{"daily_kgs": warmDailyKGs, "patches": warmPatches, "ttl_h": 24},
	}
}

// GET /api/kg-geo/{kg} — admin row + neighbours + warm state (no cadastre).
func (s *Server) handleKGGeo(w http.ResponseWriter, r *http.Request) {
	kg := r.PathValue("kg")
	k := admin().KGs[kg]
	if k == nil {
		jsonErr(w, "unknown kg", 404)
		return
	}
	var nbs []map[string]any
	for i, n := range admin().neighbours(kg, 0.001) {
		if i >= 12 {
			break
		}
		nbs = append(nbs, map[string]any{"kg_code": n.KG, "kg_name": n.Name, "gemeinde_code": n.Gemeinde, "gemeinde_name": n.GemName})
	}
	exp, warm := s.kgWarmExpiry(kg)
	out := map[string]any{"kg": k, "neighbours": nbs, "warm": warm && exp.After(time.Now()), "source": admin().Source}
	if warm {
		out["warm_until"] = exp.UTC().Format(time.RFC3339)
	}
	jsonResp(w, out)
}
