package srv

// Boost days — a deliberately larger prewarm budget for a bounded period
// (a presentation, a workshop), so GET /api/lucky has plenty of warm, enhanced
// destinations and nobody waits on a cold cell.
//
// Switch: a file ./WARM_BOOST (or SIEDLER_WARM_BOOST). Its first line is the
// end of the boost as `2006-01-02` (local midnight at the *start* of that
// day, i.e. "boost through the 8th" = `2026-10-09`) or RFC 3339; an empty
// file means 48 h from its mtime. Delete the file to stop early.
//
// Budget while boosted (warmBoostPatches × warmPatchSize KGs/day):
//   ~0.3 MB of BEV tiles per cell, ~7 cells per KG → ~2 MB/KG, so 250 KGs
//   ≈ 0.5 GB of cadastre tiles a day plus NE documents — about half the
//   1 GB/day ceiling we allow ourselves on such days, 2.5× the normal plan.
//   The worker still paces one cell at a time (warmCellPause, yields to
//   foreground builds), so this never bursts at BEV; it just runs longer.
// Boost also pins the activity tier to "boost" (never idle: every patch
// runs, recently played v2.4 KGs are kept warm) and the plan key carries a
// `boost` tag so turning it on mid-day builds a bigger plan immediately
// (patches already past due are queued at once — the queue serialises them).

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	warmBoostFile    = "WARM_BOOST"
	warmBoostPatches = 50 // 250 KGs/day ≈ 0.5 GB BEV tiles; keep ≤ 100 (1 GB)
	warmBoostDefault = 48 * time.Hour
)

var boostCache struct {
	sync.Mutex
	at    time.Time
	until time.Time
}

// warmBoostUntil returns the end of the current boost period (zero when not
// boosted). Stat-ed at most every 20 s.
func warmBoostUntil() time.Time {
	boostCache.Lock()
	defer boostCache.Unlock()
	if time.Since(boostCache.at) < 20*time.Second {
		return boostCache.until
	}
	boostCache.at = time.Now()
	boostCache.until = readBoostFile()
	return boostCache.until
}

func readBoostFile() time.Time {
	path := warmBoostFile
	if p := os.Getenv("SIEDLER_WARM_BOOST"); p != "" {
		path = p
	}
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	raw, _ := os.ReadFile(path)
	line := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	if line == "" {
		return st.ModTime().Add(warmBoostDefault)
	}
	if t, err := time.ParseInLocation("2006-01-02", line, time.Local); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, line); err == nil {
		return t
	}
	return st.ModTime().Add(warmBoostDefault) // unparsable → default window
}

func warmBoosted() bool {
	u := warmBoostUntil()
	return !u.IsZero() && time.Now().Before(u)
}

// warmPatchesNow is the number of daily patches the planner should build.
func warmPatchesNow() int {
	if warmBoosted() {
		return warmBoostPatches
	}
	return warmPatches
}

func warmBoostStatus() map[string]any {
	u := warmBoostUntil()
	m := map[string]any{"active": warmBoosted(), "patches": warmBoostPatches, "daily_kgs": warmBoostPatches * warmPatchSize,
		"est_mb_per_day": warmBoostPatches * warmPatchSize * 2, "file": warmBoostFile}
	if !u.IsZero() {
		m["until"] = u.UTC().Format(time.RFC3339)
	}
	return m
}

// ---------------------------------------------------------------------------
// Boost focus — "keep everything v2.4 around X warm while boosted".
//
// Second line of WARM_BOOST: `focus=<Gemeinde name|Gemeinde code|lon,lat>,<km>`
// e.g. `focus=Wien,30`. While the boost is active every v2.4 KG whose bbox
// centre lies within the radius is (re-)queued every 2 h (prio 1, fresh ones
// skipped), so the whole area is warm for the duration — not just the
// 20-50 daily destinations. Wien + 15 km ≈ 120 v2.4 KGs ≈ 270 distinct cells ≈ 80 MB
// of BEV tiles per 24 h, inside the boost budget (30 km would be ~960 cells).

type boostFocus struct {
	Label    string  `json:"label"`
	Lon      float64 `json:"lon"`
	Lat      float64 `json:"lat"`
	RadiusKm float64 `json:"radius_km"`
}

var focusCache struct {
	sync.Mutex
	at time.Time
	f  *boostFocus
}

func warmBoostFocus() *boostFocus {
	if !warmBoosted() {
		return nil
	}
	focusCache.Lock()
	defer focusCache.Unlock()
	if time.Since(focusCache.at) < 20*time.Second {
		return focusCache.f
	}
	focusCache.at = time.Now()
	focusCache.f = readBoostFocus()
	return focusCache.f
}

func readBoostFocus() *boostFocus {
	path := warmBoostFile
	if p := os.Getenv("SIEDLER_WARM_BOOST"); p != "" {
		path = p
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "focus=") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(line, "focus="), ",")
		if len(parts) < 2 {
			return nil
		}
		km, err := strconv.ParseFloat(strings.TrimSpace(parts[len(parts)-1]), 64)
		if err != nil || km <= 0 {
			return nil
		}
		if km > 60 {
			km = 60 // sanity: ~ a Bundesland
		}
		f := &boostFocus{RadiusKm: km}
		if len(parts) == 3 { // lon,lat,km
			lon, e1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lat, e2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if e1 != nil || e2 != nil {
				return nil
			}
			f.Lon, f.Lat, f.Label = lon, lat, fmt.Sprintf("%.3f,%.3f", lon, lat)
			return f
		}
		name := strings.TrimSpace(parts[0])
		adm := admin()
		var g *gemeindeAdmin
		if gg := adm.Gemeinde[name]; gg != nil {
			g = gg
		} else {
			for _, gg := range adm.Gemeinde {
				if strings.EqualFold(gg.Name, name) {
					g = gg
					break
				}
			}
		}
		if g == nil {
			return nil
		}
		// Gemeinde centre = centre of its KGs' joint bbox.
		minLon, minLat, maxLon, maxLat := 180.0, 90.0, -180.0, -90.0
		for _, kg := range g.KGs {
			if k := adm.KGs[kg]; k != nil {
				minLon, minLat = math.Min(minLon, k.MinLon), math.Min(minLat, k.MinLat)
				maxLon, maxLat = math.Max(maxLon, k.MaxLon), math.Max(maxLat, k.MaxLat)
			}
		}
		f.Lon, f.Lat, f.Label = (minLon+maxLon)/2, (minLat+maxLat)/2, g.Name
		return f
	}
	return nil
}

// boostFocusKGs lists the v2.4 KGs inside the focus radius, nearest first.
func (s *Server) boostFocusKGs(f *boostFocus, v24 map[string]bool) []string {
	if f == nil {
		return nil
	}
	adm := admin()
	type kd struct {
		kg string
		d  float64
	}
	var out []kd
	for kg, k := range adm.KGs {
		if !v24[kg] {
			continue
		}
		cx, cy := k.center()
		if d := distM(f.Lon, f.Lat, cx, cy); d <= f.RadiusKm*1000 {
			out = append(out, kd{kg, d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].d < out[j].d })
	kgs := make([]string, len(out))
	for i, x := range out {
		kgs[i] = x.kg
	}
	return kgs
}

// warmBoostFocusRun queues the focus KGs (fresh ones are skipped by
// enqueueWarm). Returns the number queued.
func (s *Server) warmBoostFocusRun() int {
	f := warmBoostFocus()
	if f == nil {
		return 0
	}
	n := 0
	for _, kg := range s.boostFocusKGs(f, s.neReadyKGSet()) {
		if s.enqueueWarm(kg, "boost-focus", 1) {
			n++
		}
	}
	if n > 0 {
		slog.Info("warm: boost focus queued", "focus", f.Label, "km", f.RadiusKm, "kgs", n)
	}
	return n
}

func (s *Server) warmBoostFocusStatus() map[string]any {
	f := warmBoostFocus()
	if f == nil {
		return nil
	}
	kgs := s.boostFocusKGs(f, s.neReadyKGSet())
	fresh := s.freshWarmSet()
	warm := 0
	adm := admin()
	seen := map[cellID]bool{}
	for _, kg := range kgs {
		if fresh[kg] {
			warm++
		}
		if k := adm.KGs[kg]; k != nil {
			for _, c := range k.cells() {
				seen[c] = true
			}
		}
	}
	return map[string]any{"label": f.Label, "lon": f.Lon, "lat": f.Lat, "radius_km": f.RadiusKm,
		"v24_kgs": len(kgs), "warm": warm, "cells": len(seen)}
}
