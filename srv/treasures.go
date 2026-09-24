package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

// ---- Treasure placement ----
//
// Treasures are placed on *real parcels* whose BEV land use fits the species
// (Huchen in a river, Feldhamster on a field, Auerhahn in the forest), spread
// over the municipality with blue-noise spacing (best-candidate sampling: of
// K random habitat parcels take the one farthest from everything already
// placed), with a guaranteed "starter" chest near the spawn point so the
// first treasure is discovered within a minute. Everything is deterministic
// per session (seeded by the session id) so a re-run yields the same map.
//
// If the cadastre API is unavailable we fall back to a jittered sunflower
// spiral — still no straight lines.

// speciesHabitat maps a species group (and a few individual species) to the
// BEV Nutzungssymbol codes it can spawn on, best first.
var speciesHabitat = map[string][]string{
	"fish":      {"59", "60", "64", "61"},
	"amphibian": {"61", "60", "64", "59", "48"},
	"dragonfly": {"61", "64", "59", "60", "48"},
	"butterfly": {"48", "57", "54", "53", "40"},
	"reptile":   {"57", "48", "54", "87", "62"},
	"bird":      {"56", "48", "87", "61", "57"},
	"mammal":    {"56", "57", "55", "54"},
	// per-species overrides (key = scientific name)
	"Cricetus cricetus": {"48", "40", "53"},       // Feldhamster: farmland
	"Otis tarda":        {"48"},                   // Großtrappe: open fields
	"Ciconia nigra":     {"56", "61", "64"},       // Schwarzstorch: forest near water
	"Aquila chrysaetos": {"87", "54", "55", "56"}, // Steinadler: rock/alpine
	"Bubo bubo":         {"87", "84", "56"},       // Uhu: cliffs, quarries, forest
	"Parnassius apollo": {"87", "62", "54", "48"}, // Apollofalter: rocky slopes
	"Gulo gulo":         {"55", "54", "56", "87"}, // Vielfraß: alpine forest
	"Vipera ursinii":    {"48", "57", "54"},       // Wiesenotter: dry meadows
}

// Codes no treasure should ever sit on (asphalt, rails, buildings, cemeteries).
var treasureNoGo = map[string]bool{"41": true, "42": true, "63": true, "65": true, "72": true, "83": true, "92": true, "95": true, "58": true}

type tParcel struct {
	ID       string
	Lon, Lat float64
	Area     float64
	Codes    map[string]bool
	Dom      string // most frequent code
	Bldg     int
	Rank     int // habitat rank (0 = best-fitting code present), set per species
}

// fetchTreasureParcels loads point parcels around (lon,lat) within ~radiusM.
// Two boxes (inner + outer) so the result mixes the big outer fields/forests
// with the smaller village-scale parcels near spawn (bbox is sorted by area
// desc and capped at 800 rows).
func fetchTreasureParcels(lon, lat, radiusM float64) []tParcel {
	type row struct {
		ParcelID      string  `json:"parcel_id"`
		Lon           float64 `json:"lon"`
		Lat           float64 `json:"lat"`
		Area          float64 `json:"area_sqm"`
		Codes         string  `json:"landuse_codes"`
		BuildingCount int     `json:"building_count"`
	}
	get := func(r float64, limit int) []row {
		dLat := r / 111320
		dLon := r / (111320 * math.Cos(lat*math.Pi/180))
		u := fmt.Sprintf("%s/spatial/bbox?layers=parcels&west=%.5f&south=%.5f&east=%.5f&north=%.5f&limit=%d",
			cadastreAPI, lon-dLon, lat-dLat, lon+dLon, lat+dLat, limit)
		st, _, body, err := upstreamGetWait(u, 4*time.Second, 16<<20)
		if err != nil || st != 200 {
			return nil
		}
		var res struct {
			Data struct {
				Parcels []row `json:"parcels"`
			} `json:"data"`
		}
		json.Unmarshal(body, &res)
		return res.Data.Parcels
	}
	seen := map[string]bool{}
	var out []tParcel
	for _, rows := range [][]row{get(radiusM, 800), get(radiusM*0.3, 400)} {
		for _, r := range rows {
			if r.Lon == 0 || seen[r.ParcelID] {
				continue
			}
			seen[r.ParcelID] = true
			p := tParcel{ID: r.ParcelID, Lon: r.Lon, Lat: r.Lat, Area: r.Area, Bldg: r.BuildingCount, Codes: map[string]bool{}}
			cnt := map[string]int{}
			for _, c := range strings.Split(r.Codes, ",") {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				p.Codes[c] = true
				cnt[c]++
			}
			best := -1
			for c, n := range cnt {
				if n > best || (n == best && c < p.Dom) {
					best, p.Dom = n, c
				}
			}
			out = append(out, p)
		}
	}
	return out
}

func distM(lon1, lat1, lon2, lat2 float64) float64 {
	dx := (lon2 - lon1) * 111320 * math.Cos((lat1+lat2)/2*math.Pi/180)
	dy := (lat2 - lat1) * 111320
	return math.Hypot(dx, dy)
}

// treasureSpot is a candidate location with the parcel it sits on.
type treasureSpot struct {
	Lon, Lat float64
	Parcel   string
}

func (s *Server) generateTreasures(ctx context.Context, sessionID string, lon, lat float64) {
	hash := uint64(0)
	for _, c := range sessionID {
		hash = hash*31 + uint64(c)
	}
	rng := rand.New(rand.NewSource(int64(hash)))

	const radius = 1400.0  // m — the playable core of a municipality
	const minGap = 220.0   // m — blue-noise spacing between treasures
	const starterMax = 320 // m — the first chest is always this close to spawn

	parcels := fetchTreasureParcels(lon, lat, radius)

	// Habitat-eligible parcels: no roads/buildings, roughly inside radius.
	// Returns the habitat rank (index of the best matching code) or -1.
	eligible := func(p tParcel, codes []string) int {
		if p.Bldg > 0 || treasureNoGo[p.Dom] {
			return -1
		}
		if distM(lon, lat, p.Lon, p.Lat) > radius*1.15 {
			return -1
		}
		if codes == nil {
			return 0
		}
		for i, c := range codes {
			if p.Codes[c] {
				return i
			}
		}
		return -1
	}
	habitatOf := func(sp struct {
		Name, German, Category, Group string
		Value                         int64
	}) []string {
		if h, ok := speciesHabitat[sp.Name]; ok {
			return h
		}
		return speciesHabitat[sp.Group]
	}

	// Pick species whose habitat actually exists here (an eel in a vineyard
	// village would be silly). Shuffle deterministically, take 7, pad with
	// whatever if the municipality is monotone.
	order := rng.Perm(len(redListSpecies))
	var chosen []int
	var candidates [][]tParcel
	for pass := 0; pass < 2 && len(chosen) < 7; pass++ {
		for _, i := range order {
			if len(chosen) >= 7 {
				break
			}
			dup := false
			for _, c := range chosen {
				if c == i {
					dup = true
				}
			}
			if dup {
				continue
			}
			h := habitatOf(redListSpecies[i])
			var cands []tParcel
			for _, p := range parcels {
				if rk := eligible(p, h); rk >= 0 {
					p.Rank = rk
					cands = append(cands, p)
				}
			}
			if len(cands) >= 3 || pass == 1 {
				chosen = append(chosen, i)
				candidates = append(candidates, cands)
			}
		}
	}

	var placed []treasureSpot
	usedParcel := map[string]bool{}

	// Blue-noise pick: of K random candidates take the one whose nearest
	// placed treasure is farthest away, with a soft preference for the
	// target ring distance from spawn (so we neither cluster at the centre
	// nor push everything to the edge). Falls back to any eligible parcel,
	// then to a spiral point.
	pick := func(cands []tParcel, targetR float64, maxR float64) treasureSpot {
		if len(cands) == 0 {
			for _, p := range parcels {
				if eligible(p, nil) >= 0 {
					cands = append(cands, p)
				}
			}
		}
		best, bestScore := treasureSpot{}, -1.0
		K := 24
		if len(cands) < K {
			K = len(cands)
		}
		for k := 0; k < K; k++ {
			p := cands[rng.Intn(len(cands))]
			if usedParcel[p.ID] {
				continue
			}
			d0 := distM(lon, lat, p.Lon, p.Lat)
			if d0 > maxR {
				continue
			}
			nearest := math.Inf(1)
			for _, q := range placed {
				if d := distM(p.Lon, p.Lat, q.Lon, q.Lat); d < nearest {
					nearest = d
				}
			}
			if nearest == math.Inf(1) {
				nearest = 2 * minGap
			}
			score := math.Min(nearest, 2*minGap) / (2 * minGap) // 0..1, saturates at 2×gap
			if nearest < minGap {
				score *= 0.25
			}
			score += 0.35 * (1 - math.Min(1, math.Abs(d0-targetR)/radius)) // ring preference
			score += 0.05 * math.Min(1, p.Area/20000)                      // bigger parcels are easier to spot
			if score > bestScore {
				bestScore, best = score, treasureSpot{p.Lon, p.Lat, p.ID}
			}
		}
		if bestScore < 0 {
			// Sunflower-spiral fallback (golden angle) with jitter.
			n := float64(len(placed)) + 1
			r := targetR * (0.6 + 0.4*rng.Float64())
			a := n*2.399963 + rng.Float64()*0.6
			dLat := r * math.Sin(a) / 111320
			dLon := r * math.Cos(a) / (111320 * math.Cos(lat*math.Pi/180))
			best = treasureSpot{lon + dLon, lat + dLat, ""}
		}
		if best.Parcel != "" {
			usedParcel[best.Parcel] = true
		}
		placed = append(placed, best)
		return best
	}

	type item struct {
		tType                                  string
		value                                  int64
		speciesName, speciesGerman, speciesCat string
		targetR, maxR                          float64
		cands                                  []tParcel
	}
	var items []item
	// Starter chest close to spawn; second chest on the mid ring; XP scroll far out.
	items = append(items,
		item{"coins", 100, "", "", "", 200, starterMax, nil},
		item{"coins", 200, "", "", "", 650, radius, nil},
		item{"xp", 150, "", "", "", 1100, radius * 1.15, nil},
	)
	for j, i := range chosen {
		sp := redListSpecies[i]
		// Spread species over rings 350..1300 m, shuffled so ring ≠ list order.
		tr := 350 + float64((j*5)%7)/6*950
		items = append(items, item{"species", sp.Value, sp.Name, sp.German, sp.Category, tr, radius * 1.15, candidates[j]})
	}
	// Place the most habitat-constrained species first so they get their
	// scarce parcels before generic ones eat the spacing budget.
	sort.SliceStable(items, func(a, b int) bool {
		ca, cb := len(items[a].cands), len(items[b].cands)
		if items[a].tType != "species" {
			ca = 1 << 30
		}
		if items[b].tType != "species" {
			cb = 1 << 30
		}
		return ca < cb
	})
	// …but the starter chest is always placed first so nothing steals its spot.
	for i := range items {
		if items[i].tType == "coins" && items[i].value == 100 {
			items[0], items[i] = items[i], items[0]
			break
		}
	}

	for _, it := range items {
		spot := pick(it.cands, it.targetR, it.maxR)
		s.Q.CreateTreasure(ctx, dbgen.CreateTreasureParams{
			SessionID:       sessionID,
			Lon:             spot.Lon,
			Lat:             spot.Lat,
			TreasureType:    it.tType,
			Value:           it.value,
			SpeciesName:     it.speciesName,
			SpeciesGerman:   it.speciesGerman,
			SpeciesCategory: it.speciesCat,
		})
	}
}

// ReseedTreasures deletes a session's unfound classic/species treasures (N2K
// bonus treasures are kept) and places them again with the current algorithm.
// CLI: ./siedler -reseed-treasures <session id | invite code>
func (s *Server) ReseedTreasures(key string) error {
	ctx := context.Background()
	var id string
	var lon, lat float64
	err := s.DB.QueryRowContext(ctx, "SELECT id, center_lon, center_lat FROM game_sessions WHERE id = ? OR invite_code = ?", key, key).Scan(&id, &lon, &lat)
	if err != nil {
		return fmt.Errorf("session %q: %w", key, err)
	}
	res, err := s.DB.ExecContext(ctx, "DELETE FROM treasures WHERE session_id = ? AND found_by IS NULL AND treasure_type IN ('coins','xp','species')", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	s.generateTreasures(ctx, id, lon, lat)
	var m int
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM treasures WHERE session_id = ? AND found_by IS NULL", id).Scan(&m)
	fmt.Printf("session %s: removed %d, now %d unfound treasures\n", id, n, m)
	return nil
}
