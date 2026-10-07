package srv

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"time"

	"srv.exe.dev/db/dbgen"
)

// Harvest state that outlives ownership.
//
// A clear-cut stand or a freshly harvested field keeps its regrowth clock when
// the owner sells: the row in parcel_harvest_state (keyed by the HMAC parcel
// hash only — no cadastre id) is written on every harvest and on every sale,
// seeded into the next claim, and listed per session so unowned parcels are
// drawn (and priced) in their true state. Shared contract with
// harvestOf()/regenFactor() in game.js.
//
// Value: the standing crop is part of the price. A parcel is worth
// base × regenFactor, where regenFactor runs from regenFloor (just harvested)
// back to 1 as the stand/crop regrows — timber over forestFullValueMin
// (510 min), crops over their own cycle (cropCycle, per INVEKOS class).

const (
	regenFloorForest = 0.40 // Kahlschlag: 40 % of the mature-stand price
	regenFloorField  = 0.75 // stubble: the land is most of the value
)

// cropCycle: real-time cycle length per INVEKOS crop group (fieldKindFor in
// game.js), hash kind 0..2 when no Schlag covers the parcel. Mirrors
// cropCycleS() in game.js.
func cropCycle(crop string, kind int) time.Duration {
	switch crop {
	case "getreide":
		return 60 * time.Minute
	case "mais":
		return 90 * time.Minute
	case "sonst":
		return 45 * time.Minute
	case "obst":
		return 180 * time.Minute
	case "wein":
		return 240 * time.Minute
	}
	switch kind {
	case 1:
		return 90 * time.Minute
	case 2:
		return 150 * time.Minute
	}
	return fieldCycle
}

// harvestKindOf classifies a claim/parcel for the state row.
func harvestKindOf(landuse, crop string, parcelID string) string {
	switch landuse {
	case "56":
		return "forest"
	case "48":
		if fieldPhaseAtCrop(parcelID, time.Now(), crop).Stage == "meadow" {
			return "meadow"
		}
		return "field"
	}
	return ""
}

// regenFactor: price multiplier 0..1 for the parcel's current regrowth state.
// progress is the 0..1 regrowth (1 = mature / ripe), also returned for the UI.
func regenFactor(kind string, harvestedAt *time.Time, parcelID, crop string, now time.Time) (factor, progress float64) {
	switch kind {
	case "forest":
		if harvestedAt == nil {
			return 1, 1
		}
		p := clampF(now.Sub(*harvestedAt).Minutes()/forestFullValueMin, 0, 1)
		return regenFloorForest + (1-regenFloorForest)*p, p
	case "field":
		fp := fieldPhaseAtCrop(parcelID, now, crop)
		p := 0.0
		switch fp.Stage {
		case "meadow":
			return 1, 1
		case "ploughed", "growing":
			p = clampF(fp.T/fieldRipeAt, 0, 1)
		case "ripe":
			p = 1
		}
		if harvestedAt != nil && !harvestedAt.Before(fp.CycleStart) {
			p = 0 // harvested this cycle → stubble
		}
		return regenFloorField + (1-regenFloorField)*p, p
	}
	return 1, 1
}

// claimRegen: factor for an existing claim (its landuse + harvested_at).
func claimRegen(c *dbgen.ParcelClaim, parcelID, crop string, now time.Time) (factor, progress float64) {
	lu := ""
	if c.Landuse != nil {
		lu = *c.Landuse
	}
	k := harvestKindOf(lu, crop, parcelID)
	if k == "meadow" {
		return 1, 1
	}
	return regenFactor(k, c.HarvestedAt, parcelID, crop, now)
}

// recordHarvestState persists the regrowth clock outside the claim.
func (s *Server) recordHarvestState(ctx context.Context, sessionID, parcelID, kind, crop string, at time.Time, harvests int64) {
	if kind == "" || sessionID == "" {
		return
	}
	if harvests < 1 {
		harvests = 1
	}
	if err := s.Q.UpsertHarvestState(ctx, dbgen.UpsertHarvestStateParams{
		SessionID: sessionID, ParcelHash: parcelHash(parcelID), Kind: kind, CropGroup: crop,
		HarvestedAt: at.UTC(), Harvests: harvests,
	}); err != nil {
		slog.Warn("harvest state", "error", err)
	}
}

// inheritedHarvest: the regrowth clock a new buyer inherits (nil = none).
func (s *Server) inheritedHarvest(ctx context.Context, sessionID, parcelID string) *dbgen.ParcelHarvestState {
	st, err := s.Q.GetHarvestState(ctx, dbgen.GetHarvestStateParams{SessionID: sessionID, ParcelHash: parcelHash(parcelID)})
	if err != nil {
		return nil
	}
	// A forest clock older than the full regrowth, or a field clock older than
	// the longest cycle, carries no information any more.
	if st.Kind == "forest" && time.Since(st.HarvestedAt) > time.Duration(forestFullValueMin)*time.Minute {
		return nil
	}
	if st.Kind != "forest" && time.Since(st.HarvestedAt) > 4*time.Hour {
		return nil
	}
	return &st
}

// GET /api/session/{id}/harvests → [{parcel_hash, kind, crop_group, harvested_at, harvests}]
// Everything the client needs to draw stubble / Kahlschlag on unowned parcels
// and to quote the regrowth-discounted price. Hashes only.
func (s *Server) handleSessionHarvests(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.ListHarvestStates(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	cut := time.Now().Add(-time.Duration(forestFullValueMin) * time.Minute)
	for _, st := range rows {
		if st.HarvestedAt.Before(cut) {
			continue
		}
		out = append(out, map[string]any{
			"parcel_hash": st.ParcelHash, "kind": st.Kind, "crop_group": st.CropGroup,
			"harvested_at": st.HarvestedAt, "harvests": st.Harvests,
		})
	}
	jsonResp(w, out)
}

// sellPrice: 60 % of what was paid, scaled by the current regrowth — a stand
// that was just cut is worth less than the mature one that was bought.
func sellPrice(purchase int64, factor float64) int64 {
	v := int64(math.Round(float64(purchase) * 0.6 * factor))
	if v < 1 {
		v = 1
	}
	return v
}

// regenPrice: base price × regrowth factor of the state a previous owner left
// (claim-ez path; claimParcel does the same inline and seeds the claim).
func (s *Server) regenPrice(ctx context.Context, sessionID, parcelID, landuse string, base int) int {
	st := s.inheritedHarvest(ctx, sessionID, parcelID)
	if st == nil {
		return base
	}
	t := st.HarvestedAt
	f, _ := regenFactor(harvestKindOf(landuse, st.CropGroup, parcelID), &t, parcelID, st.CropGroup, time.Now())
	p := int(math.Round(float64(base) * f))
	if p < 10 {
		p = 10
	}
	return p
}

// seedInheritedHarvest copies the regrowth clock a previous owner left into
// the freshly inserted claim, so the new owner sees the same stand.
func (s *Server) seedInheritedHarvest(ctx context.Context, sessionID, parcelID string) {
	st := s.inheritedHarvest(ctx, sessionID, parcelID)
	if st == nil {
		return
	}
	c, err := s.Q.GetParcelClaim(ctx, dbgen.GetParcelClaimParams{SessionID: sessionID, ParcelHash: parcelHash(parcelID)})
	if err != nil {
		return
	}
	t := st.HarvestedAt
	s.Q.SeedClaimHarvest(ctx, dbgen.SeedClaimHarvestParams{HarvestedAt: &t, Harvests: st.Harvests, ID: c.ID})
}
