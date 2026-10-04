package srv

// srtm KG registry: one fetch of the whole KG universe (all 7 850
// Katastralgemeinden, processed or not) from srtm-lidar-at /api/v1/kgs,
// mirrored to a local file so the registry survives an srtm outage.
//
//   fetch  : at most hourly (enhanced-kgs TTL). Step 1: /kgs?fields=codes
//            (~60 KB, ETag = universe_hash) with If-None-Match → 304 means
//            the universe is unchanged. Step 2: /llm/manifest.json with
//            If-None-Match (weak ETag moves when products are rebuilt) → 304
//            means no per-KG detail changed either and the mirror is simply
//            re-stamped. Only when either moved do we pull the full rows
//            /kgs?processed_only=0&limit=5000&offset=… (2 pages) — must
//            return exactly `total` rows or the fetch is rejected.
//   mirror : data/srtm-kgs.json.gz {fetched_at, etag, kgs_total,
//            universe_hash, kgs[]} — written atomically after every good
//            fetch, read when srtm is unreachable (source:"local-copy").
//   hash   : universe_hash = universeHashOf(codes) — the shared recipe
//            (kguniverse.go) so it compares 1:1 with umfeld-at's hash;
//            registry_hash additionally folds in product_version + updated_at.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	srtmKGMirrorPath = "data/srtm-kgs.json.gz"      // live mirror (gitignored, rewritten after every good fetch)
	srtmKGSeedPath   = "data/srtm-kgs.seed.json.gz" // committed seed so a fresh checkout has a fallback too
	srtmKGPage       = 5000
	// kgUniverseCount is the contract: all Austrian KGs (same universe as
	// umfeld-at and our embedded admin table). A fetch that disagrees is
	// logged loudly but still accepted — srtm is the registry of record.
	kgUniverseCount = 7850
)

type srtmKGMirror struct {
	FetchedAt    time.Time `json:"fetched_at"`
	ETag         string    `json:"etag,omitempty"`          // universe ETag (= universe_hash) of /kgs?fields=codes
	ManifestETag string    `json:"manifest_etag,omitempty"` // /llm/manifest.json weak ETag at the last full fetch
	KGsTotal     int       `json:"kgs_total"`
	UniverseHash string    `json:"universe_hash"`
	RegistryHash string    `json:"registry_hash"`
	Attribution  string    `json:"attribution,omitempty"`
	KGs          []srtmKG  `json:"kgs"`
}

// kgUniverseHashes returns (universe_hash, registry_hash) for a KG list.
func kgUniverseHashes(kgs []srtmKG) (string, string) {
	codes := make([]string, 0, len(kgs))
	byCode := map[string]srtmKG{}
	for _, k := range kgs {
		codes = append(codes, k.KgCode)
		byCode[k.KgCode] = k
	}
	sort.Strings(codes)
	hr := sha256.New()
	for _, c := range codes {
		k := byCode[c]
		fmt.Fprintf(hr, "%s|%s|%s\n", c, k.ProductVer, k.UpdatedAt)
	}
	return universeHashOf(codes), hex.EncodeToString(hr.Sum(nil))[:20]
}

// conditionalGet: one GET with If-None-Match. 304 → (304, hdr, nil).
func conditionalGet(u, etag string, maxBody int64) (int, http.Header, []byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := upstreamClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, body, nil
}

// fetchSrtmKGUniverse revalidates the registry against srtm with as little
// traffic as possible (see header): codes list + manifest with If-None-Match;
// the full per-KG rows only when one of them moved or we have no mirror.
// Any transport/parse/consistency problem → error; the caller then falls
// back to the mirror. The returned mirror is always a complete registry.
func fetchSrtmKGUniverse(prev *srtmKGMirror) (*srtmKGMirror, error) {
	// 1. universe (set of codes), ETag = universe_hash
	prevETag, prevMan := "", ""
	if prev != nil {
		prevETag, prevMan = prev.ETag, prev.ManifestETag
	}
	code, hdr, body, err := conditionalGet(lidarAPI+"/kgs?fields=codes&processed_only=0", prevETag, 2<<20)
	if err != nil {
		return nil, err
	}
	universeSame := false
	uniETag := prevETag
	switch code {
	case 304:
		universeSame = true
	case 200:
		var cl struct {
			Codes        []string `json:"codes"`
			UniverseHash string   `json:"universe_hash"`
			KGsTotal     int      `json:"kgs_total"`
		}
		if err := json.Unmarshal(body, &cl); err != nil {
			return nil, fmt.Errorf("srtm /kgs?fields=codes: parse: %w", err)
		}
		if h := universeHashOf(cl.Codes); h != cl.UniverseHash {
			return nil, fmt.Errorf("srtm /kgs?fields=codes: universe_hash %s… ≠ recomputed %s…", cl.UniverseHash[:12], h[:12])
		}
		uniETag = hdr.Get("ETag")
		universeSame = prev != nil && prev.UniverseHash == cl.UniverseHash && len(prev.KGs) == len(cl.Codes)
	default:
		return nil, fmt.Errorf("srtm /kgs?fields=codes: http %d", code)
	}
	// 2. per-KG products: the manifest's weak ETag moves when a product is rebuilt
	manETag := prevMan
	detailsSame := false
	if universeSame && prev != nil && len(prev.KGs) > 0 {
		mc, mh, _, merr := conditionalGet(lidarAPI[:len(lidarAPI)-len("/api/v1")]+"/llm/manifest.json", prevMan, 4<<20)
		switch {
		case merr != nil:
			return nil, merr
		case mc == 304:
			detailsSame = true
		case mc == 200:
			manETag = mh.Get("ETag")
			detailsSame = prevMan != "" && manETag == prevMan
		default:
			return nil, fmt.Errorf("srtm /llm/manifest.json: http %d", mc)
		}
	}
	if universeSame && detailsSame {
		m := *prev
		m.FetchedAt = time.Now().UTC()
		m.KGs = prev.KGs
		if err := writeSrtmKGMirror(&m); err != nil {
			slog.Warn("srtm KG registry: mirror write failed", "err", err)
		}
		return &m, nil
	}
	m, err := fetchSrtmKGRows()
	if err != nil {
		return nil, err
	}
	m.ETag, m.ManifestETag = uniETag, manETag
	if err := writeSrtmKGMirror(m); err != nil {
		slog.Warn("srtm KG registry: mirror write failed", "err", err)
	}
	slog.Info("srtm KG registry: full refresh", "kgs", len(m.KGs), "universe_same", universeSame, "manifest_etag", manETag)
	return m, nil
}

// fetchSrtmKGRows pulls every KG row (processed or not), paginated.
func fetchSrtmKGRows() (*srtmKGMirror, error) {
	var all []srtmKG
	attrib := ""
	total := -1
	for offset := 0; ; offset += srtmKGPage {
		u := fmt.Sprintf("%s/kgs?processed_only=0&limit=%d&offset=%d", lidarAPI, srtmKGPage, offset)
		code, _, body, err := upstreamGetWait(u, 20*time.Second, 30<<20)
		if err != nil {
			return nil, err
		}
		if code != 200 {
			return nil, fmt.Errorf("srtm /kgs: http %d", code)
		}
		var page struct {
			KGs   []srtmKG `json:"kgs"`
			Total int      `json:"total"`
			Meta  struct {
				Attribution string `json:"attribution"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("srtm /kgs: parse: %w", err)
		}
		if offset == 0 {
			attrib = page.Meta.Attribution
			total = page.Total
		}
		all = append(all, page.KGs...)
		if len(page.KGs) == 0 || len(all) >= total {
			break
		}
		if offset > 10*srtmKGPage { // runaway guard
			return nil, errors.New("srtm /kgs: pagination did not converge")
		}
	}
	if total <= 0 || len(all) != total {
		return nil, fmt.Errorf("srtm /kgs: got %d rows, total says %d", len(all), total)
	}
	if total != kgUniverseCount {
		slog.Warn("srtm KG registry: universe size differs from contract", "got", total, "contract", kgUniverseCount)
	}
	m := &srtmKGMirror{FetchedAt: time.Now().UTC(), KGsTotal: total, Attribution: attrib, KGs: all}
	m.UniverseHash, m.RegistryHash = kgUniverseHashes(all)
	return m, nil
}

func writeSrtmKGMirror(m *srtmKGMirror) error {
	if err := os.MkdirAll(filepath.Dir(srtmKGMirrorPath), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(m); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	tmp := srtmKGMirrorPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, srtmKGMirrorPath)
}

func readSrtmKGMirror() (*srtmKGMirror, error) {
	m, err := readSrtmKGFile(srtmKGMirrorPath)
	if err != nil {
		if sm, serr := readSrtmKGFile(srtmKGSeedPath); serr == nil {
			return sm, nil
		}
		return nil, err
	}
	return m, nil
}

func readSrtmKGFile(path string) (*srtmKGMirror, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var m srtmKGMirror
	if err := json.NewDecoder(zr).Decode(&m); err != nil {
		return nil, err
	}
	if len(m.KGs) == 0 {
		return nil, errors.New("empty mirror")
	}
	// Hashes are recomputed, never trusted from disk (recipe may have moved).
	m.UniverseHash, m.RegistryHash = kgUniverseHashes(m.KGs)
	return &m, nil
}

// loadSrtmKGUniverse: live srtm first, local mirror second. source is
// "srtm" or "local-copy".
func loadSrtmKGUniverse() (*srtmKGMirror, string, error) {
	prev, lerr := readSrtmKGMirror()
	if lerr != nil {
		prev = nil
	}
	m, err := fetchSrtmKGUniverse(prev)
	if err == nil {
		return m, "srtm", nil
	}
	slog.Warn("srtm KG registry: live fetch failed, using local copy", "err", err)
	if prev != nil {
		return prev, "local-copy", nil
	}
	return nil, "", fmt.Errorf("srtm: %v; mirror: %v", err, lerr)
}
