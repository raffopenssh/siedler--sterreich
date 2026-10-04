package srv

// Declared layer (umfeld-at /api/v1/ne): the complement of the observed NE
// cells. We do not fetch the per-KG containers — the cadastre cells already
// hold every declared polygon. What umfeld adds that we cannot cheaply derive
// is the *fabric* statistics of the surrounding res-10 K cell (~1.5 ha):
// parcel-size percentiles, parcels per folio, structure count, the surveyed
// (Grenzkataster) share and settlement orientation. We read it through the
// `ne` block of the umfeld /context bundle we already fetch per parcel
// (`neContextInclude`), so it costs no extra round trip. Attribution: © BEV
// (CC BY 4.0), bearbeitet — statistics per H3 cell, no object geometry.

import (
	"fmt"
	"math"
	"strings"
)

// neContextInclude lists the /context blocks we want — umfeld's default set
// plus `ne`. `include=` is a whitelist upstream, so the default blocks must
// be spelled out.
const neContextInclude = "municipality,land_price,natura2000,protected_area,osm,toponyms,legal,ne"

// fabricOf condenses the context `ne` block into the agent-facing fabric
// summary. nil when the block is absent or the KG is not built yet.
func fabricOf(cx map[string]any) map[string]any {
	ne := sub(cx, "ne")
	if ne == nil {
		return nil
	}
	out := map[string]any{"cell_res": 12, "source": "umfeld-at NE cells (declared layer, BEV CC BY 4.0, bearbeitet)"}
	if lu := sub(ne, "lu"); lu != nil {
		if fl := sub(lu, "flags"); fl != nil && fl["declared_unknown"] == true {
			out["declared_unknown"] = true
		}
		if v, ok := num(lu, "gk"); ok {
			out["surveyed_boundary_share"] = r2(v / 255)
		}
		if v, ok := num(lu, "edge_m_ha"); ok {
			out["boundary_m_per_ha"] = math.Round(v)
		}
		if v, ok := num(lu, "orient_axis"); ok {
			out["building_axis_deg"] = math.Round(v * 2)
			if c, ok := num(lu, "orient_coh"); ok {
				out["building_axis_coherence"] = r2(c / 255)
			}
		}
	}
	if k := sub(ne, "k"); k != nil {
		kk := map[string]any{"area_ha": 1.5}
		for src, dst := range map[string]string{"n_units": "parcels", "unit_p10": "parcel_p10_sqm", "unit_p50": "parcel_median_sqm", "unit_p90": "parcel_p90_sqm", "reg_n": "folios", "units_per_reg": "parcels_per_folio", "str_n": "structures", "str_p50": "structure_median_sqm"} {
			if v, ok := num(k, src); ok {
				kk[dst] = v
			}
		}
		out["neighbourhood"] = kk
	}
	return out
}

// fabricSentence: one narration sentence for inspect, "" when nothing to say.
func fabricSentence(f map[string]any) string {
	if f == nil {
		return ""
	}
	var parts []string
	if k := sub(f, "neighbourhood"); k != nil {
		if m, ok := num(k, "parcel_median_sqm"); ok && m > 0 {
			parts = append(parts, fmt.Sprintf("the surrounding 1.5 ha holds %.0f parcels, median %s", numOr(k, "parcels", 0), fmtSqm(m)))
		}
		if v, ok := num(k, "parcels_per_folio"); ok && v >= 1.5 {
			parts = append(parts, fmt.Sprintf("%.1f parcels per folio", v))
		}
	}
	if v, ok := num(f, "surveyed_boundary_share"); ok && v >= 0.2 {
		parts = append(parts, fmt.Sprintf("%.0f%% of the boundaries are in the Grenzkataster (legally binding)", v*100))
	}
	if f["declared_unknown"] == true {
		parts = append(parts, "the declared land use of this cell is incomplete")
	}
	if len(parts) == 0 {
		return ""
	}
	return "Fabric: " + strings.Join(parts, "; ") + ". "
}

func fmtSqm(v float64) string {
	if v >= 10000 {
		return fmt.Sprintf("%.1f ha", v/10000)
	}
	return fmt.Sprintf("%.0f m²", v)
}
