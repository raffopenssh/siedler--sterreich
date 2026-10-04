package srv

// srtm KG registry: one fetch of the whole KG universe (all 7 850
// Katastralgemeinden, processed or not) from srtm-lidar-at /api/v1/kgs,
// mirrored to a local file so the registry survives an srtm outage.
//
//   fetch  : /kgs?processed_only=0&limit=5000&offset=… (2 pages) — must
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
	"log/slog"
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
	ETag         string    `json:"etag,omitempty"`
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

// fetchSrtmKGUniverse pulls every KG (processed or not) from srtm and
// mirrors it to disk. Any transport/parse/consistency problem → error; the
// caller then falls back to the mirror.
func fetchSrtmKGUniverse() (*srtmKGMirror, error) {
	var all []srtmKG
	etag, attrib := "", ""
	total := -1
	for offset := 0; ; offset += srtmKGPage {
		u := fmt.Sprintf("%s/kgs?processed_only=0&limit=%d&offset=%d", lidarAPI, srtmKGPage, offset)
		code, hdr, body, err := upstreamGetWait(u, 20*time.Second, 30<<20)
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
			etag = hdr.Get("ETag")
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
	m := &srtmKGMirror{FetchedAt: time.Now().UTC(), ETag: etag, KGsTotal: total, Attribution: attrib, KGs: all}
	m.UniverseHash, m.RegistryHash = kgUniverseHashes(all)
	if err := writeSrtmKGMirror(m); err != nil {
		slog.Warn("srtm KG registry: mirror write failed", "err", err)
	}
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
	m, err := fetchSrtmKGUniverse()
	if err == nil {
		return m, "srtm", nil
	}
	slog.Warn("srtm KG registry: live fetch failed, using local copy", "err", err)
	if lm, lerr := readSrtmKGMirror(); lerr == nil {
		return lm, "local-copy", nil
	} else {
		return nil, "", fmt.Errorf("srtm: %v; mirror: %v", err, lerr)
	}
}
