package srv

// GET /api/tiles/hillshade/{z}/{x}/{y}.png — proxy for the srtm sibling's
// pre-rendered hillshade tiles (LID-4), so no upstream host is exposed to
// the browser. Tiles are
// immutable upstream (1 y), so we keep them 30 d in api_cache (base64, the
// column is TEXT) and let the client cache for a year. 204 = no data.

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"srv.exe.dev/db/dbgen"
	"strconv"
	"strings"
	"time"
)

const hillshadeUpstream = "https://srtm-lidar-at.exe.xyz/tiles/hillshade/"

func (s *Server) handleHillshadeTile(w http.ResponseWriter, r *http.Request) {
	z, x, y := r.PathValue("z"), r.PathValue("x"), strings.TrimSuffix(r.PathValue("y"), ".png")
	zi, err1 := strconv.Atoi(z)
	xi, err2 := strconv.Atoi(x)
	yi, err3 := strconv.Atoi(y)
	if err1 != nil || err2 != nil || err3 != nil || zi < 8 || zi > 17 || xi < 0 || yi < 0 || xi >= 1<<zi || yi >= 1<<zi {
		http.Error(w, "bad tile", 400)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	key := "hs:" + z + "/" + x + "/" + y
	ctx := context.Background()
	if cached, err := s.Q.GetCachedData(ctx, key); err == nil {
		w.Header().Set("X-Cache", "HIT")
		if cached == "" {
			w.WriteHeader(204)
			return
		}
		b, _ := base64.StdEncoding.DecodeString(cached)
		w.Write(b)
		return
	}
	v, _, _ := s.sf.Do(key, func() (any, error) {
		resp, err := upstreamGet(hillshadeUpstream + z + "/" + x + "/" + y + ".png")
		if err != nil {
			return sfRes{nil, 502}, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == 204 || resp.StatusCode == 404 {
			s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: "", ExpiresAt: time.Now().Add(30 * 24 * time.Hour)})
			return sfRes{nil, 204}, nil
		}
		if resp.StatusCode != 200 {
			return sfRes{nil, resp.StatusCode}, nil
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil || len(b) == 0 {
			return sfRes{nil, 502}, nil
		}
		s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{CacheKey: key, Data: base64.StdEncoding.EncodeToString(b), ExpiresAt: time.Now().Add(30 * 24 * time.Hour)})
		return sfRes{b, 200}, nil
	})
	rr := v.(sfRes)
	w.Header().Set("X-Cache", "MISS")
	if rr.status != 200 {
		if rr.status != 204 {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.WriteHeader(rr.status)
		return
	}
	w.Write(rr.body)
}
