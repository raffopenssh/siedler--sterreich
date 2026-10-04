package srv

// KG universe — the set of all Austrian Katastralgemeinden (contract:
// kg_count = 7 850), published by umfeld-at as ONE gzipped list:
//
//   GET https://umfeld-at.exe.xyz/api/v1/kgs
//     → data{kg_count, kg_count_contract, universe_hash, generated_at,
//            kgs[{kg_code, kg_name, gemeinde_code, gemeinde_name,
//                 district_code, district_name, state_code, state_name,
//                 bbox[minlon,minlat,maxlon,maxlat], centroid[lon,lat]}]}
//     universe_hash = sha256(join(sorted(kg_code), "\n")) hex
//                   = header X-KG-Universe-Hash (ETag prefix); If-None-Match → 304.
//
// We fetch it once at startup, revalidate hourly with If-None-Match, mirror
// it to data/kg-universe.json.gz (seed data/kg-universe.seed.json.gz committed)
// and recompute the hash from OUR OWN KG list (the embedded BEV admin table,
// srv/data/admin.json.gz — the codes every cached cell / claim / warm row is
// keyed on). All three must agree:
//
//   umfeld universe_hash == ours == srtm-lidar-at's (kgregistry.go)
//   kg_count == 7850
//
// Anything else is a universe change (new epoch): slog.Error, one e-mail to
// the owner per distinct fingerprint, `kg_universe.alert` in /api/metrics and
// /api/warm/status, and the daily warm plan + v2.4 adoption pause (they key
// derived caches on kg_code). Session warming and the game keep running.
// Per-KG state is always keyed on the 5-digit kg_code string with leading
// zero (never an int) — see unpadKG for the umfeld /lookup special case.

import (
	"bytes"
	"compress/gzip"
	"context"
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
	"strings"
	"sync"
	"time"
)

const (
	kgUniverseMirrorPath = "data/kg-universe.json.gz"
	kgUniverseSeedPath   = "data/kg-universe.seed.json.gz"
	kgUniverseRevalidate = time.Hour // umfeld Cache-Control max-age 3600
)

// universeHashOf implements umfeld's recipe: sha256(join(sorted(codes),"\n")) hex.
func universeHashOf(codes []string) string {
	c := append([]string(nil), codes...)
	sort.Strings(c)
	h := sha256.Sum256([]byte(strings.Join(c, "\n")))
	return hex.EncodeToString(h[:])
}

type kgUniverseRow struct {
	KgCode       string    `json:"kg_code"`
	KgName       string    `json:"kg_name"`
	GemeindeCode string    `json:"gemeinde_code"`
	GemeindeName string    `json:"gemeinde_name"`
	DistrictCode string    `json:"district_code"`
	DistrictName string    `json:"district_name"`
	StateCode    string    `json:"state_code"`
	StateName    string    `json:"state_name"`
	BBox         []float64 `json:"bbox"`
	Centroid     []float64 `json:"centroid"`
}

// kgUniverseDoc is the mirrored file (umfeld body + our fetch metadata).
type kgUniverseDoc struct {
	FetchedAt    time.Time       `json:"fetched_at"`
	ETag         string          `json:"etag"`
	GeneratedAt  string          `json:"generated_at"`
	KGCount      int             `json:"kg_count"`
	UniverseHash string          `json:"universe_hash"`
	KGs          []kgUniverseRow `json:"kgs"`
}

type kgUniverseState struct {
	mu          sync.Mutex
	doc         *kgUniverseDoc
	source      string // umfeld | local-copy | none
	ourHash     string
	ourCount    int
	srtmHash    string // from kgregistry.go, when known
	lastCheck   time.Time
	lastStatus  int // 200 / 304 / 0
	lastErr     string
	alert       string // "" = all good
	alertedHash string // fingerprint that was e-mailed
}

var kgUni kgUniverseState

// ourKGCodes: the KG list we key everything on (BEV admin table).
func ourKGCodes() []string {
	a := admin()
	codes := make([]string, 0, len(a.KGs))
	for c := range a.KGs {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

// kgUniverseOK reports whether the universe is verified and consistent.
// While false the daily warm plan and v2.4 adoption hold off.
func kgUniverseOK() bool {
	kgUni.mu.Lock()
	defer kgUni.mu.Unlock()
	return kgUni.doc != nil && kgUni.alert == ""
}

// kgUniverseBoot runs synchronously in Serve() before any goroutine can
// call noteSrtmUniverseHash: our own hash + the local copy are in place, so
// the first check never sees an empty state.
func (s *Server) kgUniverseBoot() {
	codes := ourKGCodes()
	kgUni.mu.Lock()
	kgUni.ourHash, kgUni.ourCount = universeHashOf(codes), len(codes)
	if m, err := readKGUniverseMirror(); err == nil {
		kgUni.doc, kgUni.source = m, "local-copy"
	}
	kgUni.mu.Unlock()
}

// kgUniverseInit: startup fetch (mirror as fallback), then hourly revalidation.
func (s *Server) kgUniverseInit() {
	for {
		s.kgUniverseRefresh()
		time.Sleep(kgUniverseRevalidate)
	}
}

// kgUniverseRefresh fetches / revalidates the umfeld list and re-runs the
// consistency check. Safe to call from anywhere (serialised by kgUni.mu
// around state only; the network part runs unlocked).
func (s *Server) kgUniverseRefresh() {
	kgUni.mu.Lock()
	prev := kgUni.doc
	if prev == nil {
		if m, err := readKGUniverseMirror(); err == nil {
			prev = m
			kgUni.doc, kgUni.source = m, "local-copy"
		}
	}
	etag := ""
	if prev != nil {
		etag = prev.ETag
	}
	kgUni.mu.Unlock()

	doc, status, err := fetchKGUniverse(etag)
	kgUni.mu.Lock()
	kgUni.lastCheck, kgUni.lastStatus = time.Now(), status
	kgUni.lastErr = ""
	switch {
	case err != nil:
		kgUni.lastErr = err.Error()
		if kgUni.doc == nil {
			kgUni.source = "none"
		}
		slog.Warn("kg universe: umfeld fetch failed", "err", err, "source", kgUni.source)
	case status == http.StatusNotModified:
		kgUni.source = "umfeld"
		if kgUni.doc != nil {
			kgUni.doc.FetchedAt = time.Now().UTC()
		}
	default:
		kgUni.doc, kgUni.source = doc, "umfeld"
		if werr := writeKGUniverseMirror(doc); werr != nil {
			slog.Warn("kg universe: mirror write failed", "err", werr)
		}
	}
	kgUni.mu.Unlock()
	s.kgUniverseCheck()
}

// fetchKGUniverse: one call, If-None-Match aware. status 304 → doc nil.
func fetchKGUniverse(etag string) (*kgUniverseDoc, int, error) {
	req, _ := http.NewRequest("GET", umfeldAPI+"/kgs", nil)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := upstreamClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, 304, nil
	}
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, fmt.Errorf("umfeld /kgs: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 40<<20))
	if err != nil {
		return nil, 0, err
	}
	var env struct {
		Data struct {
			KGCount      int             `json:"kg_count"`
			Contract     int             `json:"kg_count_contract"`
			UniverseHash string          `json:"universe_hash"`
			GeneratedAt  string          `json:"generated_at"`
			KGs          []kgUniverseRow `json:"kgs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, 200, fmt.Errorf("umfeld /kgs: parse: %w", err)
	}
	d := env.Data
	if len(d.KGs) == 0 {
		return nil, 200, errors.New("umfeld /kgs: empty list")
	}
	// The hash must be reproducible from the rows we received, or the
	// body is damaged / truncated — never accept it as the universe.
	codes := make([]string, len(d.KGs))
	for i, r := range d.KGs {
		codes[i] = r.KgCode
	}
	if got := universeHashOf(codes); got != d.UniverseHash {
		return nil, 200, fmt.Errorf("umfeld /kgs: body hash %s ≠ declared %s", got[:12], d.UniverseHash[:12])
	}
	if hh := resp.Header.Get("X-KG-Universe-Hash"); hh != "" && hh != d.UniverseHash {
		return nil, 200, fmt.Errorf("umfeld /kgs: header hash %s ≠ body %s", hh[:12], d.UniverseHash[:12])
	}
	return &kgUniverseDoc{
		FetchedAt: time.Now().UTC(), ETag: resp.Header.Get("ETag"), GeneratedAt: d.GeneratedAt,
		KGCount: d.KGCount, UniverseHash: d.UniverseHash, KGs: d.KGs,
	}, 200, nil
}

// kgUniverseCheck compares umfeld ↔ ours (↔ srtm when known) and raises /
// clears the alert.
func (s *Server) kgUniverseCheck() {
	kgUni.mu.Lock()
	if kgUni.ourCount == 0 { // not booted yet
		kgUni.mu.Unlock()
		return
	}
	d := kgUni.doc
	var problems []string
	if d == nil {
		problems = append(problems, "no KG universe available (umfeld unreachable, no local copy)")
	} else {
		if d.KGCount != kgUniverseCount {
			problems = append(problems, fmt.Sprintf("umfeld kg_count %d ≠ contract %d", d.KGCount, kgUniverseCount))
		}
		if len(d.KGs) != d.KGCount {
			problems = append(problems, fmt.Sprintf("umfeld rows %d ≠ kg_count %d", len(d.KGs), d.KGCount))
		}
		if d.UniverseHash != kgUni.ourHash {
			problems = append(problems, fmt.Sprintf("umfeld universe_hash %s… ≠ ours (BEV admin table) %s…", d.UniverseHash[:12], kgUni.ourHash[:12]))
		}
		if kgUni.srtmHash != "" && kgUni.srtmHash != d.UniverseHash {
			problems = append(problems, fmt.Sprintf("srtm-lidar-at universe_hash %s… ≠ umfeld %s…", kgUni.srtmHash[:12], d.UniverseHash[:12]))
		}
	}
	if kgUni.ourCount != kgUniverseCount {
		problems = append(problems, fmt.Sprintf("our admin table has %d KGs ≠ contract %d", kgUni.ourCount, kgUniverseCount))
	}
	alert := strings.Join(problems, "; ")
	changed := alert != kgUni.alert
	kgUni.alert = alert
	fp := ""
	if alert != "" {
		sum := sha256.Sum256([]byte(alert))
		fp = hex.EncodeToString(sum[:8])
	}
	needMail := alert != "" && fp != kgUni.alertedHash
	if needMail {
		kgUni.alertedHash = fp
	}
	source, ourHash := kgUni.source, kgUni.ourHash
	kgUni.mu.Unlock()

	if alert == "" {
		if changed || d != nil && kgUni.lastStatus == 200 {
			slog.Info("kg universe: verified", "kg_count", kgUniverseCount, "universe_hash", ourHash[:12], "source", source)
		}
		return
	}
	slog.Error("kg universe: CHANGED / INCONSISTENT — daily warm plan and v2.4 adoption paused", "alert", alert, "source", source)
	if needMail {
		go sendOwnerMail("[siedler] KG universe alert", "The KG universe check failed on "+s.Hostname+":\n\n  "+
			strings.ReplaceAll(alert, "; ", "\n  ")+"\n\nContract: kg_count = 7850, universe_hash = sha256(join(sorted(kg_code), \"\\n\")).\n"+
			"umfeld-at: https://umfeld-at.exe.xyz/api/v1/kgs  ·  our list: srv/data/admin.json.gz (BEV VGD)\n"+
			"The daily warm plan and v2.4 adoption are paused until the hashes agree (see /api/metrics kg_universe). "+
			"If the universe really changed: rebuild admin.json.gz, purge vp:v1/ne: caches and kg_warm, then restart.\n")
	}
}

// noteSrtmUniverseHash feeds the srtm registry's hash into the check.
func (s *Server) noteSrtmUniverseHash(h string) {
	kgUni.mu.Lock()
	changed := kgUni.srtmHash != h
	kgUni.srtmHash = h
	kgUni.mu.Unlock()
	if changed {
		s.kgUniverseCheck()
	}
}

// kgUniverseStatus is the /api/metrics + /api/warm/status block.
func kgUniverseStatus() map[string]any {
	kgUni.mu.Lock()
	defer kgUni.mu.Unlock()
	m := map[string]any{
		"contract_kg_count": kgUniverseCount,
		"our_kg_count":      kgUni.ourCount,
		"our_hash":          kgUni.ourHash,
		"source":            kgUni.source,
		"ok":                kgUni.doc != nil && kgUni.alert == "",
		"alert":             kgUni.alert,
		"last_check":        kgUni.lastCheck,
		"last_status":       kgUni.lastStatus,
	}
	if kgUni.lastErr != "" {
		m["last_error"] = kgUni.lastErr
	}
	if kgUni.srtmHash != "" {
		m["srtm_hash"] = kgUni.srtmHash
	}
	if d := kgUni.doc; d != nil {
		m["umfeld_kg_count"] = d.KGCount
		m["umfeld_hash"] = d.UniverseHash
		m["umfeld_generated_at"] = d.GeneratedAt
		m["umfeld_etag"] = d.ETag
		m["fetched_at"] = d.FetchedAt
	}
	return m
}

// kgUniverseRow looks a KG up in the umfeld list (district/state codes that
// the admin table lacks). nil when unknown or the list is not loaded.
func kgUniverseLookup(kg string) *kgUniverseRow {
	kgUni.mu.Lock()
	defer kgUni.mu.Unlock()
	if kgUni.doc == nil {
		return nil
	}
	for i := range kgUni.doc.KGs {
		if kgUni.doc.KGs[i].KgCode == kg {
			return &kgUni.doc.KGs[i]
		}
	}
	return nil
}

func writeKGUniverseMirror(d *kgUniverseDoc) error {
	if err := os.MkdirAll(filepath.Dir(kgUniverseMirrorPath), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(d); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	tmp := kgUniverseMirrorPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, kgUniverseMirrorPath)
}

func readKGUniverseMirror() (*kgUniverseDoc, error) {
	d, err := readKGUniverseFile(kgUniverseMirrorPath)
	if err != nil {
		if sd, serr := readKGUniverseFile(kgUniverseSeedPath); serr == nil {
			return sd, nil
		}
	}
	return d, err
}

func readKGUniverseFile(path string) (*kgUniverseDoc, error) {
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
	var d kgUniverseDoc
	if err := json.NewDecoder(zr).Decode(&d); err != nil {
		return nil, err
	}
	if len(d.KGs) == 0 {
		return nil, errors.New("empty universe file")
	}
	codes := make([]string, len(d.KGs))
	for i, r := range d.KGs {
		codes[i] = r.KgCode
	}
	d.UniverseHash, d.KGCount = universeHashOf(codes), len(codes)
	return &d, nil
}

// sendOwnerMail: exe.dev gateway, plain text, best effort.
func sendOwnerMail(subject, body string) {
	to := os.Getenv("SIEDLER_ALERT_MAIL")
	if to == "" {
		to = "raffaelhickisch+exedev@gmail.com"
	}
	payload, _ := json.Marshal(map[string]string{"to": to, "subject": subject, "body": body})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://169.254.169.254/gateway/email/send", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("alert mail failed", "err", err)
		return
	}
	resp.Body.Close()
	slog.Info("alert mail sent", "to", to, "status", resp.StatusCode)
}
