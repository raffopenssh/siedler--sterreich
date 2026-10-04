package srv

// Upstream data services after the Oct 2026 provider revision
// (docs/migration-2026-10.md). The browser never talks to any of these —
// every call goes through a proxy in this package.
//
//   - bevdirect-serve (local, :8787): cadastre assembled live from the BEV
//     vector tiles (kataster.bev.gv.at, CC BY 4.0). Parcels, footprints,
//     landuse, EZ-in-view, Gemeinde/KG lookup. Nothing per parcel is stored
//     there; we cache its cell responses for max 24 h.
//   - umfeld-at (public tier): non-cadastre context around a point
//     (Statistik Austria, EEA Natura 2000, WDPA, OSM, BEV DLM names, RIS).
//   - srtm-lidar-at (public tier): landscape per bbox/point/KG code
//     (terrain, land cover, tree apices, building heights, hillshade).

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	bevAPI    = envOr("SIEDLER_BEV_URL", "http://127.0.0.1:8787")
	umfeldAPI = envOr("SIEDLER_UMFELD_URL", "https://umfeld-at.exe.xyz/api/v1")
	lidarAPI  = envOr("SIEDLER_SRTM_URL", "https://srtm-lidar-at.exe.xyz/api/v1")
)

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return def
}

// bevNotice is the attribution every cadastre-derived response must carry
// (BEV Nutzungsbedingungen §2.3.3). bevdirect sends the year-bearing text
// in `notice`; this is the fallback when we synthesise a body ourselves.
// Format "© BEV, JJJJ" per §2.3.3; the year is the year of the tile data,
// i.e. always the current one (the VTC is rebuilt from the live cadastre).
var bevNotice = "© BEV, " + strconv.Itoa(time.Now().Year()) + " – Datenquelle: Bundesamt für Eich- und Vermessungswesen, Kataster (CC BY 4.0), bearbeitet"

// cadastreTTL is the hard upper bound for anything derived from the BEV
// tiles in our api_cache (policy: "max 24 h", see licenses.go).
const cadastreTTL = 24 * 60 * 60 * 1e9 // time.Duration in ns, avoids import cycle games

// ---------------------------------------------------------------------------
// Fixed 0.02° world grid — identical to bevdirect's cell grid
// (cellOf = floor(lon/0.02), floor(lat/0.02)). The client requests
// grid-aligned tiles, our cache keys are (i,j), and the warm planner
// enumerates the same cells, so all three layers agree byte for byte.

const gridDeg = 0.02

type cellID struct{ I, J int }

func cellOf(lon, lat float64) cellID {
	return cellID{int(math.Floor(lon / gridDeg)), int(math.Floor(lat / gridDeg))}
}

func (c cellID) bbox() (w, s, e, n float64) {
	return float64(c.I) * gridDeg, float64(c.J) * gridDeg, float64(c.I+1) * gridDeg, float64(c.J+1) * gridDeg
}

// cellsForBBox lists every grid cell intersecting the bbox (inclusive edges
// shrunk by an epsilon so a bbox ending exactly on a grid line does not pull
// in the next column).
func cellsForBBox(w, s, e, n float64) []cellID {
	const eps = 1e-9
	a, b := cellOf(w+eps, s+eps), cellOf(e-eps, n-eps)
	var out []cellID
	for i := a.I; i <= b.I; i++ {
		for j := a.J; j <= b.J; j++ {
			out = append(out, cellID{i, j})
		}
	}
	return out
}

// isAlignedCell reports whether the bbox is exactly one grid cell.
func isAlignedCell(w, s, e, n float64) (cellID, bool) {
	c := cellOf(w+1e-9, s+1e-9)
	cw, cs, ce, cn := c.bbox()
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-7 }
	return c, near(w, cw) && near(s, cs) && near(e, ce) && near(n, cn)
}

// ---------------------------------------------------------------------------
// Embedded KG register: BEV Verwaltungsgrenzen (VGD) 1:50 000, CC BY 4.0
// (copied from bevdirect's clients/bevdirect/admin.json.gz). Gives
// every KG a bbox + Gemeinde without touching any cadastre service; drives
// KG neighbours, the warm planner, the lucky pick and name lookups.

//go:embed data/admin.json.gz
var adminGz []byte

type kgAdmin struct {
	KG       string  `json:"kg_code"`
	Name     string  `json:"kg_name"`
	Gemeinde string  `json:"gemeinde_code"`
	GemName  string  `json:"gemeinde_name"`
	District string  `json:"district_name"`
	State    string  `json:"state_name"`
	MinLon   float64 `json:"min_lon"`
	MinLat   float64 `json:"min_lat"`
	MaxLon   float64 `json:"max_lon"`
	MaxLat   float64 `json:"max_lat"`
	AreaKm2  float64 `json:"area_sqkm"`
}

func (k *kgAdmin) center() (float64, float64) {
	return (k.MinLon + k.MaxLon) / 2, (k.MinLat + k.MaxLat) / 2
}

// cells enumerates the grid cells covering the KG's bbox.
func (k *kgAdmin) cells() []cellID { return cellsForBBox(k.MinLon, k.MinLat, k.MaxLon, k.MaxLat) }

type gemeindeAdmin struct {
	Code, Name, District, State    string
	KGs                            []string
	MinLon, MinLat, MaxLon, MaxLat float64
}

func (g *gemeindeAdmin) center() (float64, float64) {
	return (g.MinLon + g.MaxLon) / 2, (g.MinLat + g.MaxLat) / 2
}

type adminIndex struct {
	Source   string
	KGs      map[string]*kgAdmin
	Gemeinde map[string]*gemeindeAdmin
	// cell → KGs whose bbox touches it (for "which KGs does this tile load")
	byCell map[cellID][]string
	all    []*kgAdmin
}

var (
	adminOnce sync.Once
	adminIdx  *adminIndex
)

func admin() *adminIndex {
	adminOnce.Do(func() {
		idx := &adminIndex{KGs: map[string]*kgAdmin{}, Gemeinde: map[string]*gemeindeAdmin{}, byCell: map[cellID][]string{}}
		zr, err := gzip.NewReader(bytes.NewReader(adminGz))
		if err != nil {
			slog.Error("admin table: gunzip", "err", err)
			adminIdx = idx
			return
		}
		raw, _ := io.ReadAll(zr)
		var doc struct {
			Source string    `json:"source"`
			KGs    []kgAdmin `json:"kgs"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			slog.Error("admin table: parse", "err", err)
			adminIdx = idx
			return
		}
		idx.Source = doc.Source
		for i := range doc.KGs {
			k := &doc.KGs[i]
			if k.KG == "" || k.MaxLon <= k.MinLon || k.MaxLat <= k.MinLat {
				continue
			}
			idx.KGs[k.KG] = k
			idx.all = append(idx.all, k)
			g := idx.Gemeinde[k.Gemeinde]
			if g == nil {
				g = &gemeindeAdmin{Code: k.Gemeinde, Name: k.GemName, District: k.District, State: k.State, MinLon: k.MinLon, MinLat: k.MinLat, MaxLon: k.MaxLon, MaxLat: k.MaxLat}
				idx.Gemeinde[k.Gemeinde] = g
			}
			g.KGs = append(g.KGs, k.KG)
			g.MinLon, g.MinLat = math.Min(g.MinLon, k.MinLon), math.Min(g.MinLat, k.MinLat)
			g.MaxLon, g.MaxLat = math.Max(g.MaxLon, k.MaxLon), math.Max(g.MaxLat, k.MaxLat)
			for _, c := range k.cells() {
				idx.byCell[c] = append(idx.byCell[c], k.KG)
			}
		}
		sort.Slice(idx.all, func(i, j int) bool { return idx.all[i].KG < idx.all[j].KG })
		slog.Info("admin table loaded", "kgs", len(idx.KGs), "gemeinden", len(idx.Gemeinde), "source", idx.Source)
		adminIdx = idx
	})
	return adminIdx
}

// kgAt returns the KG whose bbox contains the point, smallest bbox first
// (bboxes overlap at the seams; the smaller one is the better guess).
func (a *adminIndex) kgAt(lon, lat float64) *kgAdmin {
	var best *kgAdmin
	for _, kg := range a.byCell[cellOf(lon, lat)] {
		k := a.KGs[kg]
		if lon < k.MinLon || lon > k.MaxLon || lat < k.MinLat || lat > k.MaxLat {
			continue
		}
		if best == nil || k.AreaKm2 < best.AreaKm2 {
			best = k
		}
	}
	return best
}

// neighbours returns KGs whose bbox touches kg's bbox (padded by pad°),
// nearest centre first, excluding kg itself.
func (a *adminIndex) neighbours(kg string, pad float64) []*kgAdmin {
	k := a.KGs[kg]
	if k == nil {
		return nil
	}
	seen := map[string]bool{kg: true}
	var out []*kgAdmin
	for _, c := range cellsForBBox(k.MinLon-pad, k.MinLat-pad, k.MaxLon+pad, k.MaxLat+pad) {
		for _, o := range a.byCell[c] {
			if seen[o] {
				continue
			}
			ok := a.KGs[o]
			if ok.MaxLon < k.MinLon-pad || ok.MinLon > k.MaxLon+pad || ok.MaxLat < k.MinLat-pad || ok.MinLat > k.MaxLat+pad {
				continue
			}
			seen[o] = true
			out = append(out, ok)
		}
	}
	cx, cy := k.center()
	sort.Slice(out, func(i, j int) bool {
		xi, yi := out[i].center()
		xj, yj := out[j].center()
		return math.Hypot(xi-cx, (yi-cy)/0.68) < math.Hypot(xj-cx, (yj-cy)/0.68)
	})
	return out
}

// kgsInBBox lists KG codes whose bbox intersects the bbox.
func (a *adminIndex) kgsInBBox(w, s, e, n float64) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cellsForBBox(w, s, e, n) {
		for _, kg := range a.byCell[c] {
			if seen[kg] {
				continue
			}
			k := a.KGs[kg]
			if k.MaxLon < w || k.MinLon > e || k.MaxLat < s || k.MinLat > n {
				continue
			}
			seen[kg] = true
			out = append(out, kg)
		}
	}
	sort.Strings(out)
	return out
}

// kgCodeOf extracts the KG from a parcel id "63349-348/6" → "63349".
func kgCodeOf(parcelID string) string {
	if i := strings.IndexByte(parcelID, '-'); i > 0 {
		return parcelID[:i]
	}
	return ""
}
