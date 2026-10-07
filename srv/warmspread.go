package srv

// Geographic spread of warm lucky destinations.
//
// /api/lucky avoids everything within ~30 km of where the player just was
// (luckyAvoid). For that to be a warm spawn and not a cold build there must
// always be several warm, v2.4-confirmed destinations ≥ warmSpreadKm apart —
// independent of the boost focus (one dense blob by design) and of the daily
// plan (deterministic, tomorrow). This file:
//
//   - warmSpreadGroups(): clusters the warm (fresh ≥ 2 h) v2.4-confirmed KGs
//     greedily into groups whose centroids are ≥ warmSpreadKm apart and marks
//     the ones that are real destinations (a lucky cluster — ≥ luckyMinN
//     playable KGs around a KG centre — or a whole Gemeinde warm).
//   - warmSpreadRun(): every warmSpreadEvery (and after startup) tops the set
//     up to warmSpreadTarget() destination groups — counting only groups that
//     outlive warmSpreadExpiring — by queueing the *cheapest* far seeds
//     (reason "spread", prio 1: ahead of the daily patches, behind sessions).
//     A seed is a cluster-grade v2.4 KG (≥ 3 v2.4 KGs in the lucky box) or an
//     all-v2.4 Gemeinde; Bundesländer without a destination go first, then
//     fewer cells. ≤ warmSpreadDailyCap seeds a day — never the universe.
//   - luckyPickAvoid prefers the farthest group (luckyFarGroup).
//
// /api/warm/status → spread{target, groups[], destinations, min_km,
// missing_regions, seeds_today, last_run}.

import (
	"context"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

const (
	warmSpreadKm           = warmPatchSpacingKm // 30 — the lucky "somewhere new" radius
	warmSpreadTargetActive = 9                  // destination groups ≥ 30 km apart (active / boost) — one per Bundesland
	warmSpreadTargetIdle   = 3                  // idle tier: still enough for one far hop
	warmSpreadExpiring     = 3 * time.Hour      // a group this close to expiry is replaced early
	warmSpreadDailyCap     = 16                 // seeds queued per day (≈ 16 × 10–25 cells)
	warmSpreadSeedKGs      = 8                  // KGs per seed (cluster + nearest v2.4 neighbours)
	warmSpreadSeedMaxCells = 60                 // cheap seeds only; big Gemeinden come via the daily plan
	warmSpreadEvery        = 30 * time.Minute
)

var austriaStates = []string{"Burgenland", "Kärnten", "Niederösterreich", "Oberösterreich", "Salzburg", "Steiermark", "Tirol", "Vorarlberg", "Wien"}

type spreadGroup struct {
	Lon         float64  `json:"lon"`
	Lat         float64  `json:"lat"`
	Label       string   `json:"label"` // biggest Gemeinde in the group
	State       string   `json:"state"`
	KGs         int      `json:"kgs"`
	Gemeinden   int      `json:"gemeinden"`
	Destination bool     `json:"destination"` // lucky would land here (cluster or whole Gemeinde)
	ExpiresInH  float64  `json:"expires_in_h"`
	NearestKm   float64  `json:"nearest_km"` // to the nearest other group
	PatchKGs    int      `json:"patch_kgs"`  // biggest contiguous v2.4 patch the group touches
	PatchKm2    float64  `json:"patch_km2"`
	PatchName   string   `json:"patch,omitempty"`
	kgs         []string // members
	states      map[string]int
}

func (g *spreadGroup) live() bool {
	return g.Destination && g.ExpiresInH*float64(time.Hour) > float64(warmSpreadExpiring)
}

type spreadState struct {
	sync.Mutex
	day    string
	seeds  []string // labels queued today
	lastAt time.Time
	last   map[string]any
}

var spread spreadState

func warmSpreadTarget() int {
	if warmIdle() {
		return warmSpreadTargetIdle
	}
	return warmSpreadTargetActive
}

// freshWarmExpiry: kg → expires_at for every warm KG (fresh ≥ 2 h).
func (s *Server) freshWarmExpiry() map[string]time.Time {
	out := map[string]time.Time{}
	rows, err := s.DB.QueryContext(context.Background(), "SELECT kg_code, expires_at FROM kg_warm WHERE expires_at > datetime('now', '+2 hours')")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kg, ts string
		if rows.Scan(&kg, &ts) == nil {
			if t, err := parseSQLiteTime(ts); err == nil {
				out[kg] = t
			}
		}
	}
	return out
}

// groupKGs clusters KGs greedily: sorted by code, each joins the first group
// whose centroid lies within km, else opens one. Centroids are running means.
func groupKGs(kgs []string, km float64) []*spreadGroup {
	adm := admin()
	sort.Strings(kgs)
	var groups []*spreadGroup
	for _, kg := range kgs {
		k := adm.KGs[kg]
		if k == nil {
			continue
		}
		lon, lat := k.center()
		var g *spreadGroup
		for _, c := range groups {
			if distM(lon, lat, c.Lon, c.Lat) < km*1000 {
				g = c
				break
			}
		}
		if g == nil {
			g = &spreadGroup{Lon: lon, Lat: lat, states: map[string]int{}}
			groups = append(groups, g)
		}
		n := float64(len(g.kgs))
		g.Lon, g.Lat = (g.Lon*n+lon)/(n+1), (g.Lat*n+lat)/(n+1)
		g.kgs = append(g.kgs, kg)
		g.states[k.State]++
	}
	return groups
}

// warmSpreadGroups: the warm v2.4-confirmed set as ≥ km-separated groups,
// biggest first. playable = the KGs grouped (so lucky can reuse it with its
// own avoid-filtered set).
func (s *Server) spreadGroupsOf(playable map[string]bool, expiry map[string]time.Time) []*spreadGroup {
	adm := admin()
	var kgs []string
	for kg := range playable {
		if len(kg) == 5 {
			kgs = append(kgs, kg)
		}
	}
	groups := groupKGs(kgs, warmSpreadKm)
	patchOf, patches := s.v24Patches(s.neConfirmedKGSet())
	for _, g := range groups {
		g.KGs = len(g.kgs)
		for _, kg := range g.kgs {
			if id, ok := patchOf[kg]; ok && patches[id].Km2 > g.PatchKm2 {
				g.PatchKGs, g.PatchKm2, g.PatchName = patches[id].KGs, patches[id].Km2, patches[id].Name
			}
		}
		gem := map[string]int{}
		var exp time.Time
		for _, kg := range g.kgs {
			k := adm.KGs[kg]
			gem[k.Gemeinde]++
			if t := expiry[kg]; t.After(exp) {
				exp = t
			}
		}
		g.Gemeinden = len(gem)
		best, bestN := "", 0
		for code, n := range gem {
			if n > bestN || (n == bestN && code < best) {
				best, bestN = code, n
			}
		}
		if gg := adm.Gemeinde[best]; gg != nil {
			g.Label = gg.Name
		}
		for st, n := range g.states {
			if n > g.states[g.State] || g.State == "" {
				g.State = st
			}
		}
		if !exp.IsZero() {
			g.ExpiresInH = math.Round(time.Until(exp).Hours()*10) / 10
		}
		// Destination: a lucky cluster around some member, or a Gemeinde
		// that is warm as a whole (the per-Gemeinde tier).
		for code, n := range gem {
			if gg := adm.Gemeinde[code]; gg != nil && n >= len(gg.KGs) {
				g.Destination = true
				break
			}
		}
		if !g.Destination {
			for _, kg := range g.kgs {
				lon, lat := adm.KGs[kg].center()
				if c, ok := s.clusterAt(lon, lat, playable, nil); ok && c.grade() {
					g.Destination = true
					break
				}
			}
		}
	}
	for i, a := range groups {
		a.NearestKm = -1
		for j, b := range groups {
			if i == j {
				continue
			}
			if d := distM(a.Lon, a.Lat, b.Lon, b.Lat) / 1000; a.NearestKm < 0 || d < a.NearestKm {
				a.NearestKm = math.Round(d)
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Destination != groups[j].Destination {
			return groups[i].Destination
		}
		return groups[i].KGs > groups[j].KGs
	})
	return groups
}

func (s *Server) warmSpreadGroups() []*spreadGroup {
	expiry := s.freshWarmExpiry()
	v24 := s.neConfirmedKGSet()
	playable := map[string]bool{}
	for kg := range expiry {
		if v24[kg] {
			playable[kg] = true
		}
	}
	return s.spreadGroupsOf(playable, expiry)
}

// v24Patches: connected components of the v2.4-confirmed KGs (admin
// adjacency, bbox touch) — "contiguous enhanced patches". kg → patch id,
// and per patch the KG count + km². Memoised 10 min (the set moves slowly).
type v24Patch struct {
	ID   int     `json:"id"`
	KGs  int     `json:"kgs"`
	Km2  float64 `json:"km2"`
	Name string  `json:"name"` // biggest Gemeinde inside
}

var patchCache struct {
	sync.Mutex
	at   time.Time
	n    int
	of   map[string]int
	list []v24Patch
}

func (s *Server) v24Patches(v24 map[string]bool) (map[string]int, []v24Patch) {
	patchCache.Lock()
	defer patchCache.Unlock()
	if time.Since(patchCache.at) < 10*time.Minute && patchCache.n == len(v24) {
		return patchCache.of, patchCache.list
	}
	adm := admin()
	of := map[string]int{}
	var list []v24Patch
	var codes []string
	for kg := range v24 {
		if len(kg) == 5 && adm.KGs[kg] != nil {
			codes = append(codes, kg)
		}
	}
	sort.Strings(codes)
	for _, start := range codes {
		if _, done := of[start]; done {
			continue
		}
		id := len(list)
		p := v24Patch{ID: id}
		gem := map[string]int{}
		queue := []string{start}
		of[start] = id
		for len(queue) > 0 {
			kg := queue[0]
			queue = queue[1:]
			k := adm.KGs[kg]
			p.KGs++
			p.Km2 += k.AreaKm2
			gem[k.Gemeinde]++
			for _, nb := range adm.neighbours(kg, 0.001) {
				if _, seen := of[nb.KG]; !seen && v24[nb.KG] {
					of[nb.KG] = id
					queue = append(queue, nb.KG)
				}
			}
		}
		best, bestN := "", 0
		for code, n := range gem {
			if n > bestN || (n == bestN && code < best) {
				best, bestN = code, n
			}
		}
		if g := adm.Gemeinde[best]; g != nil {
			p.Name = g.Name
		}
		p.Km2 = math.Round(p.Km2)
		list = append(list, p)
	}
	patchCache.at, patchCache.n, patchCache.of, patchCache.list = time.Now(), len(v24), of, list
	return of, list
}

// spreadSeed: a candidate far destination and what warming it costs.
type spreadSeed struct {
	label string
	state string
	lon   float64
	lat   float64
	kgs   []string
	cells int
	km2   float64  // playable v2.4 land in the ~4.5 km neighbourhood (luckycluster.go)
	patch v24Patch // the contiguous v2.4 patch the seed sits in
	code  string   // Gemeinde code (rotation memory key)
	used  int      // 0 fresh, 1 patch used in the last spreadPatchMemory, 2 seed used in the last spreadSeedMemory
}

// Rotation: a destination that was a spread seed in the last week — or sits
// in a patch that hosted one in the last 3 days — goes to the back of the
// queue, so every day sends lucky players to different corners of the same
// regions (and different patches when a region has several).
const (
	spreadSeedMemory  = 7 * 24 * time.Hour
	spreadPatchMemory = 3 * 24 * time.Hour
	spreadUsedPrefix  = "spread-used:v1:"
)

func (s *Server) spreadUsedSet() map[string]bool {
	out := map[string]bool{}
	rows, err := s.DB.QueryContext(context.Background(), "SELECT cache_key FROM api_cache WHERE cache_key LIKE ? AND expires_at > datetime('now')", spreadUsedPrefix+"%")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			out[strings.TrimPrefix(k, spreadUsedPrefix)] = true
		}
	}
	return out
}

func (s *Server) spreadMarkUsed(sd spreadSeed) {
	now := time.Now()
	s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: spreadUsedPrefix + "gem:" + sd.code, Data: now.Format(time.RFC3339), ExpiresAt: now.Add(spreadSeedMemory)})
	if sd.patch.Name != "" {
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: spreadUsedPrefix + "patch:" + sd.patch.Name, Data: now.Format(time.RFC3339), ExpiresAt: now.Add(spreadPatchMemory)})
	}
}

// spreadSeeds lists every candidate destination that is not warm yet:
// cluster-grade v2.4 KGs (luckyCluster.grade: ≥ luckyMinN v2.4 KGs *or* ≥
// luckyMinKm2 of v2.4 land in the neighbourhood, share ≥ luckyMinSh — one
// big KG counts like several small ones) and all-v2.4 Gemeinden, each with its KG list (own cluster +
// nearest v2.4 neighbours up to warmSpreadSeedKGs) and cell cost.
func (s *Server) spreadSeeds(v24, fresh map[string]bool) []spreadSeed {
	adm := admin()
	var out []spreadSeed
	seenGem := map[string]bool{}
	patchOf, patches := s.v24Patches(v24)
	patch := func(kg string) v24Patch {
		if id, ok := patchOf[kg]; ok {
			return patches[id]
		}
		return v24Patch{}
	}
	grow := func(base []string, lon, lat float64) []string {
		set := map[string]bool{}
		for _, kg := range base {
			set[kg] = true
		}
		type kd struct {
			kg string
			d  float64
		}
		var nb []kd
		for _, kg := range adm.kgsInBBox(lon-0.06, lat-0.04, lon+0.06, lat+0.04) {
			if !set[kg] && v24[kg] {
				x, y := adm.KGs[kg].center()
				nb = append(nb, kd{kg, distM(lon, lat, x, y)})
			}
		}
		sort.Slice(nb, func(i, j int) bool { return nb[i].d < nb[j].d })
		for _, n := range nb {
			if len(base) >= warmSpreadSeedKGs {
				break
			}
			base = append(base, n.kg)
		}
		return base
	}
	cost := func(kgs []string) int {
		seen := map[cellID]bool{}
		for _, kg := range kgs {
			if k := adm.KGs[kg]; k != nil {
				for _, c := range k.cells() {
					seen[c] = true
				}
			}
		}
		return len(seen)
	}
	allFresh := func(kgs []string) bool {
		for _, kg := range kgs {
			if !fresh[kg] {
				return false
			}
		}
		return true
	}
	// All-v2.4 Gemeinden (whole-Gemeinde destinations, any KG count).
	for code, g := range adm.Gemeinde {
		if len(g.KGs) == 0 {
			continue
		}
		ok := true
		for _, kg := range g.KGs {
			if !v24[kg] {
				ok = false
				break
			}
		}
		if !ok || allFresh(g.KGs) {
			continue
		}
		lon, lat := g.center()
		kgs := grow(append([]string{}, g.KGs...), lon, lat)
		seenGem[code] = true
		out = append(out, spreadSeed{label: g.Name, state: g.State, lon: lon, lat: lat, kgs: kgs, cells: cost(kgs), patch: patch(g.KGs[0]), code: code})
	}
	// Cluster-grade KGs (one per Gemeinde, the densest box).
	bestOf := map[string]spreadSeed{}
	for kg := range v24 {
		k := adm.KGs[kg]
		if k == nil || len(kg) != 5 || seenGem[k.Gemeinde] {
			continue
		}
		lon, lat := k.center()
		var in []string
		total := 0
		for _, o := range adm.kgsInBBox(lon-luckyBoxLon, lat-luckyBoxLat, lon+luckyBoxLon, lat+luckyBoxLat) {
			total++
			if v24[o] {
				in = append(in, o)
			}
		}
		c := luckyCluster{kg: k, lon: lon, lat: lat, n: len(in), total: total}
		clusterArea(&c, lon, lat, v24)
		if !c.grade() || allFresh(in) {
			continue
		}
		if b, ok := bestOf[k.Gemeinde]; ok && b.km2 >= c.km2 {
			continue
		}
		bestOf[k.Gemeinde] = spreadSeed{label: k.GemName + " · " + k.Name, state: k.State, lon: lon, lat: lat, kgs: in, km2: c.km2, patch: patch(kg), code: k.Gemeinde}
	}
	for _, sd := range bestOf {
		sd.kgs = grow(sd.kgs, sd.lon, sd.lat)
		sd.cells = cost(sd.kgs)
		out = append(out, sd)
	}
	used := s.spreadUsedSet()
	for i := range out {
		if used["gem:"+out[i].code] {
			out[i].used = 2
		} else if used["patch:"+out[i].patch.Name] {
			out[i].used = 1
		}
	}
	return out
}

// warmSpreadRun tops the destination groups up to the tier's target.
func (s *Server) warmSpreadRun() {
	if !kgUniverseOK() {
		return
	}
	t0 := time.Now()
	groups := s.warmSpreadGroups()
	target := warmSpreadTarget()
	live := 0
	stateLive := map[string]bool{}
	var centres [][2]float64
	for _, g := range groups {
		if g.live() {
			live++
			stateLive[g.State] = true
			centres = append(centres, [2]float64{g.Lon, g.Lat})
		}
	}
	// Queued-but-unbuilt spread seeds count as occupied spots too.
	s.warm.mu.Lock()
	for _, j := range s.warm.queue {
		if j.Reason == "spread" {
			if k := admin().KGs[j.KG]; k != nil {
				lon, lat := k.center()
				centres = append(centres, [2]float64{lon, lat})
			}
		}
	}
	s.warm.mu.Unlock()
	spaced := func(lon, lat float64) bool {
		for _, c := range centres {
			if distM(lon, lat, c[0], c[1]) < warmSpreadKm*1000 {
				return false
			}
		}
		return true
	}
	spread.Lock()
	day := t0.Format("2006-01-02")
	if spread.day != day {
		spread.day, spread.seeds = day, nil
	}
	budget := warmSpreadDailyCap - len(spread.seeds)
	spread.Unlock()
	need := target - live
	queued := []string{}
	if need > 0 && budget > 0 {
		v24 := s.neConfirmedKGSet()
		fresh := s.freshWarmSet()
		seeds := s.spreadSeeds(v24, fresh)
		var cand []spreadSeed
		for _, sd := range seeds {
			if sd.cells <= warmSpreadSeedMaxCells && spaced(sd.lon, sd.lat) {
				cand = append(cand, sd)
			}
		}
		// Each round: a Bundesland without a live destination first, then
		// rotation (seeds / patches not used recently), then the *biggest
		// contiguous v2.4 patch* (a seed in its middle gives the player a
		// large enhanced region to explore; neighbour warming grows it as
		// they roam), then cheap — with a little jitter so the same village
		// is not always the one.
		rng := rand.New(rand.NewSource(t0.UnixNano()))
		jit := map[string]float64{}
		for _, sd := range cand {
			jit[sd.label] = rng.Float64() * 8
		}
		for need > 0 && budget > 0 && len(cand) > 0 {
			sort.Slice(cand, func(i, j int) bool {
				a, b := cand[i], cand[j]
				if stateLive[a.state] != stateLive[b.state] {
					return !stateLive[a.state]
				}
				if a.used != b.used {
					return a.used < b.used
				}
				if a.patch.Km2 != b.patch.Km2 {
					return a.patch.Km2 > b.patch.Km2
				}
				return float64(a.cells)+jit[a.label] < float64(b.cells)+jit[b.label]
			})
			sd := cand[0]
			cand = cand[1:]
			if !spaced(sd.lon, sd.lat) {
				continue
			}
			n := 0
			for _, kg := range sd.kgs {
				if s.enqueueWarm(kg, "spread", 1) {
					n++
				}
			}
			if n == 0 {
				continue
			}
			centres = append(centres, [2]float64{sd.lon, sd.lat})
			stateLive[sd.state] = true
			s.spreadMarkUsed(sd)
			need--
			budget--
			queued = append(queued, sd.label+" ("+sd.state+")")
			slog.Info("warm: spread seed queued", "seed", sd.label, "state", sd.state, "kgs", n, "cells", sd.cells, "patch", sd.patch.Name, "patch_kgs", sd.patch.KGs, "patch_km2", sd.patch.Km2, "rotation", sd.used, "live", live, "target", target)
		}
		spread.Lock()
		spread.seeds = append(spread.seeds, queued...)
		spread.Unlock()
	}
	var missing []string
	for _, st := range austriaStates {
		if !stateLive[st] {
			missing = append(missing, st)
		}
	}
	spread.Lock()
	spread.lastAt = t0
	spread.last = map[string]any{"at": t0.UTC().Format(time.RFC3339), "live": live, "target": target, "queued": queued, "missing_regions": missing, "ms": time.Since(t0).Milliseconds()}
	spread.Unlock()
	if need > 0 && len(queued) == 0 {
		slog.Warn("warm: spread short of target", "live", live, "target", target, "budget", budget)
	}
}

func (s *Server) warmSpreadLoop() {
	time.Sleep(20 * time.Second) // let the registry + boost focus settle first
	for {
		s.warmSpreadRun()
		time.Sleep(warmSpreadEvery)
	}
}

// warmSpreadStatus is the /api/warm/status → spread block.
func (s *Server) warmSpreadStatus() map[string]any {
	groups := s.warmSpreadGroups()
	live, minKm := 0, -1.0
	stateLive := map[string]bool{}
	var dest []*spreadGroup
	for _, g := range groups {
		if g.live() {
			live++
			stateLive[g.State] = true
			dest = append(dest, g)
		}
	}
	for i, a := range dest {
		for _, b := range dest[i+1:] {
			if d := distM(a.Lon, a.Lat, b.Lon, b.Lat) / 1000; minKm < 0 || d < minKm {
				minKm = math.Round(d)
			}
		}
	}
	var missing []string
	for _, st := range austriaStates {
		if !stateLive[st] {
			missing = append(missing, st)
		}
	}
	if len(groups) > 24 {
		groups = groups[:24]
	}
	spread.Lock()
	seeds := append([]string{}, spread.seeds...)
	last := spread.last
	spread.Unlock()
	_, patches := s.v24Patches(s.neConfirmedKGSet())
	top := append([]v24Patch{}, patches...)
	sort.Slice(top, func(i, j int) bool { return top[i].Km2 > top[j].Km2 })
	if len(top) > 12 {
		top = top[:12]
	}
	return map[string]any{
		"patches": len(patches), "top_patches": top,
		"groups": groups, "count": len(groups), "destinations": live, "target": warmSpreadTarget(), "min_km": minKm,
		"spacing_km": warmSpreadKm, "missing_regions": missing, "seeds_today": seeds, "daily_cap": warmSpreadDailyCap, "last_run": last,
		"rotation": map[string]any{"seed_memory_d": int(spreadSeedMemory.Hours() / 24), "patch_memory_d": int(spreadPatchMemory.Hours() / 24), "used": len(s.spreadUsedSet())},
	}
}

// luckyFarGroups orders the (avoid-filtered) playable KGs' groups for the
// lucky picker: a weighted draw favouring groups far from the avoid points,
// so a player leaving Wien sees Salzburg or Kärnten, not just the nearest
// warm village 31 km out — and 10 players don't all land in the same one.
func (s *Server) luckyFarGroups(playable map[string]bool, av *luckyAvoid) []*spreadGroup {
	groups := s.spreadGroupsOf(playable, nil)
	var dest []*spreadGroup
	for _, g := range groups {
		if g.Destination {
			dest = append(dest, g)
		}
	}
	if len(dest) == 0 {
		return nil
	}
	w := make([]float64, len(dest))
	for i, g := range dest {
		d := math.Inf(1)
		for _, p := range av.pts {
			d = math.Min(d, distM(g.Lon, g.Lat, p[0], p[1]))
		}
		if math.IsInf(d, 1) {
			d = 0
		}
		// Far and big: distance^1.5 × log of the contiguous patch area, so a
		// seed inside a large enhanced region beats a lone warm village.
		w[i] = (math.Pow(d/1000, 1.5) + 1) * math.Log2(2+g.PatchKm2)
	}
	var out []*spreadGroup
	for len(dest) > 0 {
		sum := 0.0
		for _, x := range w {
			sum += x
		}
		r := rand.Float64() * sum
		i := len(dest) - 1
		for j, x := range w {
			r -= x
			if r <= 0 {
				i = j
				break
			}
		}
		out = append(out, dest[i])
		dest = append(dest[:i], dest[i+1:]...)
		w = append(w[:i], w[i+1:]...)
	}
	return out
}

// GET /api/warm/gemeinden — the Gemeinden that are warm *now* (cadastre
// cells ≤ 24 h, v2.4-confirmed KGs only) for the municipality picker's
// warmer glow. Short-lived by nature (cells expire within a day), so the
// browser may cache it only for a few minutes.
func (s *Server) handleWarmGemeinden(w http.ResponseWriter, r *http.Request) {
	adm := admin()
	expiry := s.freshWarmExpiry()
	v24 := s.neConfirmedKGSet()
	type row struct {
		Warm      int     `json:"warm"`
		KGs       int     `json:"kgs"`
		ExpiresH  float64 `json:"expires_in_h"`
		Dest      bool    `json:"destination,omitempty"`
		GroupName string  `json:"group,omitempty"`
	}
	out := map[string]*row{}
	for kg, exp := range expiry {
		k := adm.KGs[kg]
		if k == nil || !v24[kg] {
			continue
		}
		g := adm.Gemeinde[k.Gemeinde]
		if g == nil {
			continue
		}
		rw := out[g.Code]
		if rw == nil {
			rw = &row{KGs: len(g.KGs)}
			out[g.Code] = rw
		}
		rw.Warm++
		if h := math.Round(time.Until(exp).Hours()*10) / 10; h > rw.ExpiresH {
			rw.ExpiresH = h
		}
	}
	for _, grp := range s.warmSpreadGroups() {
		if !grp.live() {
			continue
		}
		for _, kg := range grp.kgs {
			if k := adm.KGs[kg]; k != nil {
				if rw := out[k.Gemeinde]; rw != nil {
					rw.Dest, rw.GroupName = true, grp.Label
				}
			}
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=180")
	jsonResp(w, map[string]any{"gemeinden": out, "count": len(out), "ttl_h": 24, "at": time.Now().UTC().Format(time.RFC3339)})
}
