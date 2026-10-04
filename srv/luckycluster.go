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
	luckyMinN   = 3   // ≥ 3 playable KGs around the spawn = "centre of several"
	luckyMinSh  = 0.6 // ≥ 60 % of the KGs around the spawn playable
)

type luckyCluster struct {
	kg    *kgAdmin
	lon   float64
	lat   float64
	n     int     // playable KGs in the box
	total int     // all KGs in the box
	share float64 // n/total
	cold  []string
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
	if c.total > 0 {
		c.share = float64(c.n) / float64(c.total)
	}
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
			if c.total > 0 {
				c.share = float64(c.n) / float64(c.total)
			}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
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
		if c.n >= luckyMinN && c.share >= luckyMinSh {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		return luckyPick{}, false
	}
	// Weighted draw; a few attempts so a bad spawn point is skipped.
	var sum float64
	for _, c := range pool {
		sum += float64(c.n * c.n)
	}
	for attempt := 0; attempt < 6; attempt++ {
		r := rand.Float64() * sum
		c := pool[len(pool)-1]
		for _, p := range pool {
			r -= float64(p.n * p.n)
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
			return a.n > b.n || (a.n == b.n && a.share > b.share)
		}
		if slon, slat, ok := s.settlementCenter(g.Name, c.lon, c.lat); ok {
			if sc, ok := s.clusterAt(slon, slat, playable, enhanced); ok && sc.n >= c.n-1 && sc.share >= luckyMinSh {
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
		spawnKG := best.kg
		g = adm.Gemeinde[spawnKG.Gemeinde]
		if g == nil {
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
			SpawnKG: spawnKG.KG, ClusterKGs: best.n, ClusterTotal: best.total, ClusterShare: math.Round(best.share*100) / 100,
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
			"cluster", best.n, "of", best.total, "share", lp.ClusterShare, "pool", len(pool), "cold_queued", queued)
		return lp, true
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
