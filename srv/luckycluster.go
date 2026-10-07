package srv

// "Auf Glück" destinations as clusters: with hundreds of enhanced (srtm
// v2.4) KGs available, a lucky player should spawn in the *middle* of
// several warm + enhanced KGs — not on the edge of a single one — so the
// first pans stay enhanced and cached. luckyClusters() scores every
// playable KG by the warm+enhanced KGs around it (~1.5 km box) and the
// pick is drawn from the densest clusters; the cold enhanced KGs around the
// spawn are queued for warming right away, so the cluster grows.

import (
	"log/slog"
	"math"
	"math/rand"
	"sort"
)

const (
	luckyBoxLon = 0.020 // ≈ 1.5 km at 47.5° N
	luckyBoxLat = 0.0135
	luckyMinN   = 3   // ≥ 3 playable KGs around the spawn = "centre of several" …
	luckyMinKm2 = 10  // … or ≥ 10 km² of playable land in the neighbourhood (one big Alpine/Seewinkel KG)
	luckyMinSh  = 0.6 // ≥ 60 % of the land around the spawn playable
	// Neighbourhood for the area measures: ± 0.03° × 0.02° ≈ 4.5 × 4.5 km,
	// what the first pans show. KG count alone punished big KGs (Vorarlberg,
	// Burgenland, Alpine valleys): one 30 km² v2.4 KG is as good a destination
	// as three 4 km² ones — the player sees cells, not register rows.
	luckyNbLon = 0.030
	luckyNbLat = 0.020
)

type luckyCluster struct {
	kg    *kgAdmin
	lon   float64
	lat   float64
	n     int     // playable KGs in the box
	total int     // all KGs in the box
	km2   float64 // playable land in the neighbourhood (bbox-clipped km²)
	tot   float64 // all land in the neighbourhood
	share float64 // km2/tot (area share)
	cold  []string
}

// grade: cluster-grade = enough playable land (KG count or km²) and share.
func (c luckyCluster) grade() bool {
	return (c.n >= luckyMinN || c.km2 >= luckyMinKm2) && c.share >= luckyMinSh
}

// weight: how much playable land the cluster offers (draw weight, ranking).
func (c luckyCluster) weight() float64 { return c.km2 }

// clipKm2 approximates the KG's land inside the box by bbox overlap.
func clipKm2(k *kgAdmin, w, s, e, n float64) float64 {
	bw, bh := k.MaxLon-k.MinLon, k.MaxLat-k.MinLat
	if bw <= 0 || bh <= 0 {
		return 0
	}
	ow := math.Min(k.MaxLon, e) - math.Max(k.MinLon, w)
	oh := math.Min(k.MaxLat, n) - math.Max(k.MinLat, s)
	if ow <= 0 || oh <= 0 {
		return 0
	}
	return k.AreaKm2 * (ow * oh) / (bw * bh)
}

// clusterArea fills km2/tot/share from the neighbourhood around (lon,lat).
func clusterArea(c *luckyCluster, lon, lat float64, playable map[string]bool) {
	adm := admin()
	w, s, e, n := lon-luckyNbLon, lat-luckyNbLat, lon+luckyNbLon, lat+luckyNbLat
	c.km2, c.tot = 0, 0
	for _, kg := range adm.kgsInBBox(w, s, e, n) {
		k := adm.KGs[kg]
		if k == nil {
			continue
		}
		a := clipKm2(k, w, s, e, n)
		c.tot += a
		if playable[kg] {
			c.km2 += a
		}
	}
	c.share = 0
	if c.tot > 0 {
		c.share = c.km2 / c.tot
	}
}

// clusterAt counts the KGs whose bbox intersects the ~1.5 km box around
// (lon,lat): playable ones, all of them, and the enhanced-but-cold ones
// (warm candidates). The spawn KG itself must be playable (ok=false else).
func (s *Server) clusterAt(lon, lat float64, playable, enhanced map[string]bool) (c luckyCluster, ok bool) {
	adm := admin()
	k := adm.kgAt(lon, lat)
	if k == nil || !playable[k.KG] {
		return c, false
	}
	c.kg, c.lon, c.lat = k, lon, lat
	for _, kg := range adm.kgsInBBox(lon-luckyBoxLon, lat-luckyBoxLat, lon+luckyBoxLon, lat+luckyBoxLat) {
		c.total++
		if playable[kg] {
			c.n++
		} else if enhanced[kg] {
			c.cold = append(c.cold, kg)
		}
	}
	clusterArea(&c, lon, lat, playable)
	return c, true
}

// luckyClusters: one candidate per playable KG (at its bbox centre), best
// first (playable count, then share). Pure admin-table work (~500 KGs).
func (s *Server) luckyClusters(playable, enhanced map[string]bool) []luckyCluster {
	adm := admin()
	var out []luckyCluster
	for kg := range playable {
		k := adm.KGs[kg]
		if k == nil {
			continue
		}
		lon, lat := k.center()
		c, ok := s.clusterAt(lon, lat, playable, enhanced)
		if !ok {
			// The bbox centre lies in a neighbouring KG (concave shape):
			// score from the KG anyway, spawn snapped later.
			c = luckyCluster{kg: k, lon: lon, lat: lat}
			for _, o := range adm.kgsInBBox(lon-luckyBoxLon, lat-luckyBoxLat, lon+luckyBoxLon, lat+luckyBoxLat) {
				c.total++
				if playable[o] {
					c.n++
				} else if enhanced[o] {
					c.cold = append(c.cold, o)
				}
			}
			clusterArea(&c, lon, lat, playable)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].weight() != out[j].weight() {
			return out[i].weight() > out[j].weight()
		}
		if out[i].share != out[j].share {
			return out[i].share > out[j].share
		}
		return out[i].kg.KG < out[j].kg.KG
	})
	return out
}

// luckyClusterPick draws a destination from the dense clusters: all
// candidates with ≥ luckyMinN playable KGs around and ≥ luckyMinSh share,
// weighted by n² so the best clusters dominate but 20 lucky players still
// land in different places. ok=false when no cluster qualifies (caller
// falls back to the per-Gemeinde pick).
func (s *Server) luckyClusterPick(playable, enhanced, v24 map[string]bool) (luckyPick, bool) {
	adm := admin()
	all := s.luckyClusters(playable, enhanced)
	var pool []luckyCluster
	for _, c := range all {
		if c.grade() {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		return luckyPick{}, false
	}
	// Weighted draw; a few attempts so a bad spawn point is skipped.
	var sum float64
	for _, c := range pool {
		sum += c.weight() * c.weight()
	}
	var dull *luckyPick
	var dullCold []string
	for attempt := 0; attempt < 6; attempt++ {
		r := rand.Float64() * sum
		c := pool[len(pool)-1]
		for _, p := range pool {
			r -= p.weight() * p.weight()
			if r <= 0 {
				c = p
				break
			}
		}
		g := adm.Gemeinde[c.kg.Gemeinde]
		if g == nil {
			continue
		}
		best := c
		// Spawn point candidates: the OSM settlement centre (where the
		// houses are), the centroid of the playable KGs around, the KG
		// centre. Take the one with the best cluster whose spawn KG is
		// playable; the settlement wins ties (it is where parcels are dense).
		better := func(a, b luckyCluster) bool {
			return a.weight() > b.weight() || (a.weight() == b.weight() && a.share > b.share)
		}
		if slon, slat, ok := s.settlementCenter(g.Name, c.lon, c.lat); ok {
			if sc, ok := s.clusterAt(slon, slat, playable, enhanced); ok && sc.weight() >= c.weight()*0.7 && sc.share >= luckyMinSh {
				best = sc
			}
		}
		if clon, clat, ok := s.playableCentroid(c, playable); ok {
			if cc, ok := s.clusterAt(clon, clat, playable, enhanced); ok && better(cc, best) {
				best = cc
			}
		}
		if _, ok := s.clusterAt(best.lon, best.lat, playable, enhanced); !ok {
			continue // KG centre outside its own (concave) KG and no better point
		}
		// Within the cluster, spawn where there is something to see
		// (luckyinterest.go): brook, forest edge, roofs, relief — not the
		// middle of a flat field.
		best, interest := s.luckySpot(best, playable, enhanced)
		spawnKG := best.kg
		// clusterAt uses the register's bbox heuristic; confirm with the real
		// KG polygon (bevdirect) that the spawn point is in a playable KG.
		if real := s.kgCodeAt(best.lon, best.lat); real != "" && real != spawnKG.KG {
			if !playable[real] {
				slog.Info("lucky: spawn rejected, real KG not playable", "bbox_kg", spawnKG.KG, "real_kg", real)
				continue
			}
			if k := adm.KGs[real]; k != nil {
				spawnKG = k
			}
		}
		g = adm.Gemeinde[spawnKG.Gemeinde]
		if g == nil {
			continue
		}
		// The registry may flag a KG v2.4 before srtm serves its cells; the
		// cached cell at the spawn knows (neobserved.go).
		if !s.spawnNEOK(best.lon, best.lat, spawnKG.KG, v24) {
			slog.Info("lucky: spawn rejected, v2.4 KG without NE cells", "kg", spawnKG.KG)
			delete(playable, spawnKG.KG)
			continue
		}
		warm := 0
		for _, kg := range g.KGs {
			if playable[kg] {
				warm++
			}
		}
		lp := luckyPick{
			GemeindeCode: g.Code, Name: g.Name, Lon: best.lon, Lat: best.lat, State: g.State,
			Enhanced: true, NE: v24[spawnKG.KG], Warm: true, WarmKGs: warm, KGs: g.KGs, Pool: len(pool),
			SpawnKG: spawnKG.KG, ClusterKGs: best.n, ClusterTotal: best.total, ClusterKm2: math.Round(best.km2*10) / 10, ClusterShare: math.Round(best.share*100) / 100,
			Interest: interest.Score, InterestWhy: interest.Why,
		}
		// Dull surroundings (flat field, nothing in view): draw another
		// cluster, but remember this one in case all draws are dull.
		if interest.Score < interestGood && attempt < 5 {
			if dull == nil || lp.Interest > dull.Interest {
				cp := lp
				dull = &cp
				dullCold = best.cold
			}
			continue
		}
		// Grow the cluster: enhanced-but-cold KGs around the spawn get
		// warmed now (neighbour priority), so the player's pans and the
		// next lucky pick here find them cached.
		queued := 0
		for _, kg := range best.cold {
			if s.enqueueWarm(kg, "lucky-cluster", 1) {
				queued++
			}
		}
		slog.Info("lucky: cluster pick", "gemeinde", g.Code, "name", g.Name, "spawn_kg", spawnKG.KG,
			"cluster", best.n, "of", best.total, "share", lp.ClusterShare, "pool", len(pool), "cold_queued", queued,
			"interest", lp.Interest, "why", lp.InterestWhy)
		return lp, true
	}
	if dull != nil {
		for _, kg := range dullCold {
			s.enqueueWarm(kg, "lucky-cluster", 1)
		}
		slog.Info("lucky: cluster pick (best of dull)", "gemeinde", dull.GemeindeCode, "spawn_kg", dull.SpawnKG, "interest", dull.Interest, "why", dull.InterestWhy)
		return *dull, true
	}
	return luckyPick{}, false
}

// playableCentroid: centroid of the playable KG centres in the cluster's
// box — a point that tends to sit between several enhanced KGs.
func (s *Server) playableCentroid(c luckyCluster, playable map[string]bool) (float64, float64, bool) {
	adm := admin()
	var sx, sy float64
	n := 0
	for _, kg := range adm.kgsInBBox(c.lon-luckyBoxLon, c.lat-luckyBoxLat, c.lon+luckyBoxLon, c.lat+luckyBoxLat) {
		if !playable[kg] {
			continue
		}
		if k := adm.KGs[kg]; k != nil {
			x, y := k.center()
			sx, sy = sx+x, sy+y
			n++
		}
	}
	if n < 2 {
		return 0, 0, false
	}
	return sx / float64(n), sy / float64(n), true
}

// enhancedClusterSize: enhanced KGs within the box around a KG's centre —
// used to rank daily-plan seeds so dense enhanced regions are warmed as
// contiguous blocks (lucky destinations with room to roam).
func enhancedClusterSize(kg string, enhanced map[string]bool) int {
	adm := admin()
	k := adm.KGs[kg]
	if k == nil {
		return 0
	}
	lon, lat := k.center()
	n := 0
	for _, o := range adm.kgsInBBox(lon-luckyBoxLon, lat-luckyBoxLat, lon+luckyBoxLon, lat+luckyBoxLat) {
		if enhanced[o] {
			n++
		}
	}
	return n
}
