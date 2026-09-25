package srv

import (
	"math"
	"time"
)

// Field crop cycle — shared contract with fieldStage() in game.js.
//
// Every Acker (NS 48, non-meadow kind) runs through ploughed → growing → ripe
// → stubble on a fixed real-time cycle. The phase offset comes from the parcel
// id hash, so neighbouring fields ripen at different times and at any moment
// roughly a quarter of the fields are ripe. Unowned fields (and owned ones the
// player forgets) are harvested by the NPC farmers at the end of the ripe
// window. Nothing about this is stored: the only persisted state is
// parcel_claims.harvested_at for fields a player actually harvested.
const (
	fieldCycle = 40 * time.Minute
	// stage boundaries as cycle fractions
	fieldGrowAt    = 0.30 // ploughed → growing
	fieldRipeAt    = 0.60 // growing → ripe
	fieldStubbleAt = 0.85 // ripe → NPC harvest / stubble
)

// jsHash mirrors simpleHash() in game.js: h = h*31 + c, mod 2^32.
func jsHash(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

// hashMix mirrors hashMix() in game.js — decorrelates sequential parcel ids.
func hashMix(h uint32) uint32 {
	h = (h ^ (h >> 16)) * 0x45d9f3b
	return h ^ (h >> 16)
}

// fieldKind mirrors fieldKind(hash) in game.js: 0,1,2 crop field · 3 meadow.
func fieldKind(parcelID string) int { return int(jsHash(parcelID) % 4) }

type fieldPhase struct {
	Kind       int       // 0..3
	Stage      string    // meadow|ploughed|growing|ripe|stubble
	T          float64   // 0..1 within the cycle
	CycleStart time.Time // when the current cycle (ploughing) began
	RipeAt     time.Time // start of the ripe window (this or next cycle)
	RipeUntil  time.Time // NPC harvest time of that window
}

// cropMeadow mirrors CROP_MEADOW in game.js (FARM-2 INVEKOS crop groups that
// are grassland, not a harvestable crop).
var cropMeadow = map[string]bool{"gruenland": true, "alm": true, "brache": true}

func fieldPhaseAt(parcelID string, now time.Time) fieldPhase {
	return fieldPhaseAtCrop(parcelID, now, "")
}

// fieldPhaseAtCrop: like fieldPhaseAt, but when the client knows the real
// INVEKOS crop group (fieldKindFor in game.js) that decides meadow-vs-crop
// instead of the hash; the cycle phase stays hash-based either way.
func fieldPhaseAtCrop(parcelID string, now time.Time, crop string) fieldPhase {
	k := fieldKind(parcelID)
	if crop != "" {
		if cropMeadow[crop] {
			k = 3
		} else if k == 3 {
			k = 0
		}
	}
	cs := fieldCycle.Seconds()
	off := float64(hashMix(jsHash(parcelID)) % uint32(cs))
	t := math.Mod(float64(now.Unix())+off, cs) / cs
	fp := fieldPhase{Kind: k, T: t}
	fp.CycleStart = now.Add(-time.Duration(t * cs * float64(time.Second)))
	if k == 3 {
		fp.Stage = "meadow"
		return fp
	}
	switch {
	case t < fieldGrowAt:
		fp.Stage = "ploughed"
	case t < fieldRipeAt:
		fp.Stage = "growing"
	case t < fieldStubbleAt:
		fp.Stage = "ripe"
	default:
		fp.Stage = "stubble"
	}
	base := fp.CycleStart
	if t >= fieldStubbleAt {
		base = base.Add(fieldCycle)
	}
	fp.RipeAt = base.Add(time.Duration(fieldRipeAt * cs * float64(time.Second)))
	fp.RipeUntil = base.Add(time.Duration(fieldStubbleAt * cs * float64(time.Second)))
	return fp
}

// harvestYield: coins for one harvest — mirrors harvestYield() in game.js.
// 1 ha ≈ 120 coins (a 1 ha field costs ~1500), clamped 5..300.
func harvestYield(areaSqm float64) int64 {
	c := math.Round(areaSqm * 0.012)
	if c < 5 {
		c = 5
	}
	if c > 300 {
		c = 300
	}
	return int64(c)
}
