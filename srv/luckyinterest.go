package srv

// "Auf Glück" should land somewhere worth looking at. The cluster logic
// (luckycluster.go) decides *where roughly* (middle of warm + enhanced
// KGs); this file decides *where exactly*: luckyInterest scores the ~350 m
// around a point from the cached cells only (no network) — land-use
// variety, water, a forest edge, some houses, relief, observed trees — and
// luckySpot searches a small grid around a proposed spawn for the most
// interesting point whose KG is still playable. A flat field with nothing
// in it scores ~0.1; the village edge with a brook, forest and roofs ~0.8.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	interestRadiusM = 350  // what a phone shows at zoom ~16.5 plus a pan
	interestGood    = 0.45 // accept a spawn at this score, else keep searching
	interestMin     = 0.25 // below this a cluster draw is retried
)

// NS code → visual group for the variety measure.
var interestGroup = map[string]string{
	"41": "siedlung", "83": "siedlung", "52": "siedlung", "40": "siedlung", "72": "siedlung", "63": "siedlung",
	"48": "feld", "53": "feld", "57": "feld",
	"56": "wald", "55": "wald", "58": "wald",
	"59": "wasser", "60": "wasser", "61": "wasser", "64": "wasser",
	"54": "alpin", "87": "alpin", "88": "alpin", "62": "alpin", "84": "alpin",
	"95": "verkehr", "92": "verkehr", "42": "verkehr", "65": "verkehr",
	"96": "feld",
}

type interestScore struct {
	Score  float64            `json:"score"`
	Why    string             `json:"why,omitempty"`
	Shares map[string]float64 `json:"shares,omitempty"`
	N      int                `json:"parcels"`
}

// luckyInterest scores the surroundings of a point 0..1 from cached cells.
// known=false when we hold no cell there (score 0 — not a judgement).
func (s *Server) luckyInterest(lon, lat float64) (interestScore, bool) {
	ps, read := s.parcelsNear(lon, lat, interestRadiusM, 4)
	if read == 0 || len(ps) < 8 {
		return interestScore{}, false
	}
	shares := map[string]float64{}
	var total, bld, bldArea float64
	var elMin, elMax = math.Inf(1), math.Inf(-1)
	var slopeSum, slopeW float64
	var treeTall, treeN, structN int
	var canopySum, canopyW float64
	hasWater := false
	for i := range ps {
		p := &ps[i]
		area := p.AreaSqm
		if len(p.LanduseAreas) > 0 {
			for ns, a := range p.LanduseAreas {
				g := interestGroup[ns]
				if g == "" {
					g = "sonst"
				}
				shares[g] += a
				total += a
				if g == "wasser" && a > 50 {
					hasWater = true
				}
			}
		} else {
			g := interestGroup[p.DominantNS]
			if g == "" {
				g = "sonst"
			}
			shares[g] += area
			total += area
		}
		bld += float64(p.BuildingCnt)
		bldArea += p.BuildingArea
		if p.ElevMin != nil && p.ElevMax != nil {
			elMin = math.Min(elMin, *p.ElevMin)
			elMax = math.Max(elMax, *p.ElevMax)
		} else if p.Elev != nil {
			elMin = math.Min(elMin, *p.Elev)
			elMax = math.Max(elMax, *p.Elev)
		}
		if p.Slope != nil && area > 0 {
			slopeSum += *p.Slope * area
			slopeW += area
		}
		if p.NE != nil {
			treeTall += p.NE.TreesTall
			treeN += p.NE.TreeN
			structN += p.NE.StructN
			if p.NE.Cells > 0 {
				canopySum += p.NE.Canopy * area
				canopyW += area
			}
			if p.NE.Cover["wasser"] >= 0.05 {
				hasWater = true
			}
		}
	}
	if total <= 0 {
		return interestScore{}, false
	}
	for g := range shares {
		shares[g] /= total
	}
	var why []string
	score := 0.0

	// 1. Variety: Shannon entropy over the visual groups, 1.0 ≈ four equal
	// groups. Roads/rail count only half — they are everywhere.
	ent := 0.0
	for g, sh := range shares {
		if sh <= 0 {
			continue
		}
		w := sh
		if g == "verkehr" {
			w *= 0.5
		}
		ent -= w * math.Log(w)
	}
	variety := math.Min(1, ent/math.Log(4))
	score += 0.35 * variety
	why = append(why, fmt.Sprintf("variety %.2f", variety))

	// 2. Forest edge: some forest but not only forest.
	if w := shares["wald"]; w > 0.03 {
		f := math.Min(1, (w-0.03)/0.17) // full at ≥ 20 %
		if w > 0.6 {
			f *= math.Max(0, (0.95-w)/0.35) // fades towards closed forest
		}
		score += 0.2 * f
		why = append(why, fmt.Sprintf("wald %.0f%%", w*100))
	}
	// 3. Water: a brook or pond in view.
	if hasWater || shares["wasser"] >= 0.005 {
		score += 0.15
		why = append(why, "wasser")
	}
	// 4. Settlement: a handful of roofs makes it a place; a solid town
	// centre still counts, but less (the cadastre is all roofs there).
	if bld > 0 {
		b := math.Min(1, bld/25)
		if sh := shares["siedlung"]; sh > 0.6 {
			b *= 0.5
		}
		score += 0.15 * b
		why = append(why, fmt.Sprintf("%d Gebäude", int(bld)))
	}
	// 5. Relief: elevation range and slope.
	if !math.IsInf(elMin, 1) && elMax > elMin {
		r := math.Min(1, (elMax-elMin)/80)
		if slopeW > 0 {
			r = math.Max(r, math.Min(1, (slopeSum/slopeW)/12))
		}
		score += 0.15 * r
		why = append(why, fmt.Sprintf("Δh %.0f m", elMax-elMin))
	}
	// 6. Observed layer: tall trees / canopy texture / structures.
	if treeTall > 0 || structN > 0 {
		t := math.Min(1, float64(treeTall)/40)
		if canopyW > 0 {
			c := canopySum / canopyW
			if c > 0.1 && c < 0.8 {
				t = math.Max(t, 0.5)
			}
		}
		score += 0.1 * t
		why = append(why, fmt.Sprintf("%d hohe Bäume", treeTall))
	}
	// Monotony penalty: one group ≥ 85 % of the area.
	for g, sh := range shares {
		if sh >= 0.85 && g != "siedlung" {
			score *= 0.4
			why = append(why, "monoton: "+g)
			break
		}
	}
	for g := range shares {
		shares[g] = math.Round(shares[g]*100) / 100
	}
	return interestScore{Score: math.Round(math.Min(1, score)*100) / 100, Why: strings.Join(why, ", "), Shares: shares, N: len(ps)}, true
}

// luckySpot searches a ~1 km × 0.9 km grid around (lon,lat) for the most
// interesting point whose spawn KG is playable and whose cluster is not
// worse than the proposal (playable km² ≥ 70 %, share ≥ luckyMinSh). Returns the
// proposal itself (scored) when nothing beats it. Only cached cells are
// read; ~60 points × parcelsNear over the in-memory parsed cells.
func (s *Server) luckySpot(c luckyCluster, playable, enhanced map[string]bool) (luckyCluster, interestScore) {
	base, _ := s.luckyInterest(c.lon, c.lat)
	best, bestScore := c, base
	type pt struct{ lon, lat float64 }
	var pts []pt
	const stepLon, stepLat = 0.0036, 0.0025 // ≈ 270 m
	for i := -3; i <= 3; i++ {
		for j := -3; j <= 3; j++ {
			if i == 0 && j == 0 {
				continue
			}
			pts = append(pts, pt{c.lon + float64(i)*stepLon, c.lat + float64(j)*stepLat})
		}
	}
	// Nearest first so ties keep the spawn close to the proposal.
	sort.Slice(pts, func(a, b int) bool {
		return math.Hypot(pts[a].lon-c.lon, (pts[a].lat-c.lat)/0.68) < math.Hypot(pts[b].lon-c.lon, (pts[b].lat-c.lat)/0.68)
	})
	for _, p := range pts {
		sc, ok := s.luckyInterest(p.lon, p.lat)
		if !ok || sc.Score <= bestScore.Score+0.04 {
			continue
		}
		cc, ok := s.clusterAt(p.lon, p.lat, playable, enhanced)
		if !ok || cc.weight() < c.weight()*0.7 || cc.share < luckyMinSh {
			continue
		}
		best, bestScore = cc, sc
	}
	return best, bestScore
}
