package srv

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB        *sql.DB
	Hostname  string
	StaticDir string
	Q         Store

	// CAD-5: last seen geometry assembly tag per KG (+ purge counter for /llm/ahead & DEV)
	assemblySeen   sync.Map
	assemblyPurges atomic.Int64

	// SSE connections for real-time updates
	sseClients map[string]map[chan string]bool // session_id -> set of channels
	sseMu      sync.RWMutex

	// sf collapses concurrent cache-miss fetches for the same key into a
	// single upstream request (see cachedFetch).
	sf singleflight.Group

	// warm is the cadastre cell prewarmer (warm.go).
	warm *warmer
}

// cachedFetch serves cacheKey from api_cache if present; on a miss it runs
// fetch exactly once across all concurrent requests for the same key
// (singleflight) — the other requests wait and share the result. This
// prevents N users panning over the same cold tile/KG from triggering N
// identical multi-second upstream fetches.
//
// fetch returns (body, status). status==200 bodies are written as JSON;
// any other status is passed through as-is with Content-Type JSON. Caching
// (with the right TTL / conditions) remains the fetch closure's job.
// fetch must not depend on a single request's context — use
// context.Background() inside, so one client disconnecting doesn't fail
// the waiters sharing the flight.
func (s *Server) cachedFetch(w http.ResponseWriter, cacheKey string, fetch func() ([]byte, int)) {
	s.cachedFetchX(w, cacheKey, fetch, nil)
}

// sfRes is the single result type every singleflight flight on an api_cache
// key must return — cachedFetch, cellJSON and llmGet can share a key.
type sfRes struct {
	body   []byte
	status int
}

// cachedFetchX is cachedFetch with a per-response transform applied at serve
// time (never cached) — used to attach parcel hashes to viewport rows.
func (s *Server) cachedFetchX(w http.ResponseWriter, cacheKey string, fetch func() ([]byte, int), transform func([]byte) []byte) {
	if transform == nil {
		transform = func(b []byte) []byte { return b }
	}
	if cached, err := s.Q.GetCachedData(context.Background(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		browserCache(w, []byte(cached))
		w.Write(transform([]byte(cached)))
		return
	}
	v, _, shared := s.sf.Do(cacheKey, func() (any, error) {
		// Another flight may have filled the cache while we queued.
		if cached, err := s.Q.GetCachedData(context.Background(), cacheKey); err == nil {
			return sfRes{[]byte(cached), 200}, nil
		}
		body, status := fetch()
		return sfRes{body, status}, nil
	})
	res := v.(sfRes)
	xc := "MISS"
	if shared {
		xc = "MISS-SHARED"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", xc)
	if res.status == http.StatusAccepted {
		// Upstream product still coming from Zenodo: relay the normalised
		// pending body + Retry-After so the client re-GETs (see upstream_pending.go).
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterOf(res.body)))
		w.Header().Set("Cache-Control", "no-store")
	}
	if res.status == 503 && bytes.Contains(res.body, []byte(`"status":"down"`)) {
		w.Header().Set("X-Upstream", "down")
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterOf(res.body)))
		w.Header().Set("Cache-Control", "no-store")
	}
	if res.status == 503 && bytes.Contains(res.body, []byte(`"status":"busy"`)) {
		// sibling overloaded / no concurrency slot (siblings.go busyBody):
		// paced retry, counted as upstream (not ours) in /api/metrics.
		w.Header().Set("X-Upstream", "busy")
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterOf(res.body)))
		w.Header().Set("Cache-Control", "no-store")
	}
	if res.status != 200 {
		w.WriteHeader(res.status)
		w.Write(res.body)
		return
	}
	browserCache(w, res.body)
	w.Write(transform(res.body))
}

// browserCacheMaxAge lets the browser reuse a ready cached body (cells,
// bbox layers) across page reloads instead of re-downloading ~150 KB gz per
// cell; data upstream changes at most daily, and the client dedups in memory
// anyway. Pending / partial answers (`ready:false`) are never cacheable.
const browserCacheMaxAge = 3600

func browserCache(w http.ResponseWriter, body []byte) {
	if w.Header().Get("Cache-Control") != "" || bytes.Contains(body, []byte(`"ready":false`)) {
		return
	}
	w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(browserCacheMaxAge))
}

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:   hostname,
		StaticDir:  filepath.Join(baseDir, "static"),
		sseClients: make(map[string]map[chan string]bool),
		warm:       newWarmer(),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	srv.Q = Store{dbgen.New(srv.DB)}
	return srv, nil
}

func (s *Server) setUpDatabase(dbPath string) error {
	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	s.DB = wdb
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// cacheJanitor periodically deletes expired api_cache rows so the SQLite file
// doesn't accumulate gigabytes of dead cached API blobs (which slows every
// table scan and bloats backups). Runs hourly.
func (s *Server) cacheJanitor() {
	for {
		n, err := s.Q.DeleteExpiredCache(context.Background())
		if err != nil {
			slog.Warn("cache janitor", "err", err)
		} else if n > 0 {
			slog.Info("cache janitor: pruned expired entries", "rows", n)
			// Hand the freed pages back to the OS: the file has auto_vacuum=INCREMENTAL
			// (set once by hand, see docs/ops.md), so this actually shrinks db.sqlite3
			// instead of leaving hundreds of MB on the freelist; a no-op otherwise.
			s.DB.Exec("PRAGMA incremental_vacuum;")
			s.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
		}
		time.Sleep(1 * time.Hour)
	}
}

func (s *Server) Serve(addr string) error {
	loadParcelKey(filepath.Join(filepath.Dir(s.StaticDir), ".."))
	s.hashLegacyParcelRows()
	go s.cacheJanitor()
	go s.compressLegacyCache()
	go s.safetyJanitor()
	s.kgUniverseBoot()
	go s.kgUniverseInit()
	s.seedPlayerActivity()
	go s.warmLoop()
	go s.neObservedSeed()
	go s.warmPlanner()

	mux := http.NewServeMux()

	// SEO / meta
	mux.HandleFunc("GET /robots.txt", s.handleRobots)

	// Roadmap + conformance harness for sibling data services (see llmahead.go)
	mux.HandleFunc("GET /llms.txt", s.handleLLMsTxt)
	mux.HandleFunc("GET /llm/game", s.handleLLMGame)
	mux.HandleFunc("GET /llm/game/{$}", s.handleLLMGame)
	mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	mux.HandleFunc("GET /agents", s.handleLeaderboardHTML)
	mux.HandleFunc("GET /agents/{$}", s.handleLeaderboardHTML)
	mux.HandleFunc("GET /api/agents/leaderboard", s.handleLeaderboardJSON)
	mux.HandleFunc("GET /.well-known/openapi.json", s.handleOpenAPI)
	mux.HandleFunc("/api/", s.handleAPINotFound) // catch-all: JSON 404 with docs link
	mux.HandleFunc("GET /api/agent/look", s.handleAgentLook)
	mux.HandleFunc("GET /api/agent/inspect", s.handleAgentInspect)
	mux.HandleFunc("GET /api/agent/municipality", s.handleAgentMunicipality)
	mux.HandleFunc("POST /api/agent/claim", s.handleAgentClaim)
	// Sibling-service roadmap: internal, token-gated (see aheadauth.go).
	mux.HandleFunc("GET /llm/ahead", s.requireAhead(s.handleLLMAhead))
	mux.HandleFunc("GET /llm/ahead/{$}", s.requireAhead(s.handleLLMAhead))
	mux.HandleFunc("GET /llm/ahead/check/{service}", s.requireAhead(s.handleLLMAheadCheck))
	mux.HandleFunc("GET /llm/ahead/status", s.requireAhead(s.handleLLMAheadStatus))
	// LID-4 hillshade tiles, proxied so the browser never talks to the srtm host.
	mux.HandleFunc("GET /api/tiles/hillshade/{z}/{x}/{y}", s.handleHillshadeTile)
	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /static/og-image.png", s.handleOGImage)

	// Static files and main page
	staticFS := http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir)))
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			// Versioned asset (game.js?v=..., style.css?v=...): the URL changes
			// on deploy, so it's safe to cache forever.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Force revalidation: without this, mobile browsers heuristically
			// cache assets and serve stale code after deploys.
			w.Header().Set("Cache-Control", "no-cache")
		}
		staticFS.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /impressum", s.handleLegalPage("impressum.html"))
	mux.HandleFunc("GET /datenschutz", s.handleLegalPage("datenschutz.html"))
	mux.HandleFunc("GET /imprint", s.handleLegalPage("imprint.html"))
	mux.HandleFunc("GET /privacy", s.handleLegalPage("privacy.html"))
	mux.HandleFunc("GET /join/{code}", s.handleIndex) // invite link
	mux.HandleFunc("GET /rejoin/{token}", s.handleRejoin)

	// Auth API
	mux.HandleFunc("POST /api/register", s.handleRegister)
	mux.HandleFunc("GET /api/suggest-name", s.handleSuggestName)

	// Game API
	mux.HandleFunc("POST /api/session/create", s.handleCreateSession)
	mux.HandleFunc("GET /api/invite/{code}", s.handleInvitePreview)
	mux.HandleFunc("POST /api/session/join", s.handleJoinSession)
	mux.HandleFunc("GET /api/session/{id}", s.handleGetSession)
	mux.HandleFunc("GET /api/session/{id}/players", s.handleGetSessionPlayers)
	mux.HandleFunc("GET /api/session/{id}/parcels", s.handleGetSessionParcels)
	mux.HandleFunc("GET /api/session/{id}/harvests", s.handleSessionHarvests)
	mux.HandleFunc("GET /api/session/{id}/treasures", s.handleGetSessionTreasures)
	mux.HandleFunc("POST /api/session/{id}/treasures/roam", s.handleRoamTreasures)
	mux.HandleFunc("GET /api/session/{id}/challenges", s.handleGetChallenges)
	mux.HandleFunc("GET /api/session/{id}/biodiversity", s.handleGetBiodiversity)
	mux.HandleFunc("GET /api/session/{id}/chat", s.handleGetChat)
	mux.HandleFunc("POST /api/session/{id}/chat", s.handlePostChat)
	mux.HandleFunc("POST /api/session/{id}/chat-mode", s.handleSetChatMode)
	mux.HandleFunc("GET /api/chat/rules", s.handleChatRules)
	mux.HandleFunc("POST /api/chat/accept-rules", s.handleAcceptChatRules)
	mux.HandleFunc("POST /api/report", s.handleReport)
	mux.HandleFunc("POST /api/block", s.handleBlock)
	mux.HandleFunc("GET /api/player/{id}/blocks", s.handleListBlocks)
	mux.HandleFunc("GET /admin/safety/{action}", s.handleAdminSafety)
	mux.HandleFunc("GET /api/session/{id}/events", s.handleSSE)

	// Game actions
	mux.HandleFunc("POST /api/claim-parcel", s.handleClaimParcel)
	mux.HandleFunc("POST /api/claim-ez", s.handleClaimEZ)
	mux.HandleFunc("POST /api/convert-parcel", s.handleConvertParcel)
	mux.HandleFunc("POST /api/harvest-parcel", s.handleHarvestParcel)
	mux.HandleFunc("POST /api/harvest-forest", s.handleHarvestForest)
	// Sibling dossiers (GW-*, HOLZ-1, FARM-1) + water mechanics — dossier.go / water.go
	mux.HandleFunc("GET /api/dossier/{kg}", s.handleDossier)
	mux.HandleFunc("GET /api/drought", s.handleSessionDrought)
	mux.HandleFunc("GET /api/water/point", s.handleWaterPoint)
	mux.HandleFunc("GET /api/water/points", s.handleWaterPoints)
	mux.HandleFunc("GET /api/water/station/{id}", s.handleWaterStation)
	mux.HandleFunc("GET /api/water/protection", s.handleWaterProtection)
	mux.HandleFunc("GET /api/water/flowpath", s.handleWaterFlowpath)
	mux.HandleFunc("GET /api/water/gwi", s.handleWaterGWI)
	mux.HandleFunc("GET /api/water/parcel/{pid...}", s.handleWaterParcel)
	mux.HandleFunc("GET /api/well-quote", s.handleWellQuote)
	mux.HandleFunc("POST /api/dig-well", s.handleDigWell)
	mux.HandleFunc("GET /api/field-economy", s.handleFieldEconomy)
	mux.HandleFunc("GET /api/forest-value", s.handleForestValue)
	mux.HandleFunc("POST /api/claim-treasure", s.handleClaimTreasure)
	mux.HandleFunc("POST /api/smash-label", s.handleSmashLabel)
	mux.HandleFunc("POST /api/complete-challenge", s.handleCompleteChallenge)
	mux.HandleFunc("POST /api/sell-parcel", s.handleSellParcel)

	// Parcel offers (buy-back system)
	mux.HandleFunc("POST /api/offer-parcel", s.handleOfferParcel)
	mux.HandleFunc("POST /api/offer-respond", s.handleOfferRespond)
	mux.HandleFunc("GET /api/session/{id}/offers", s.handleGetOffers)

	// Player info
	mux.HandleFunc("GET /api/player/{id}", s.handleGetPlayer)
	mux.HandleFunc("GET /api/player/{id}/sessions", s.handleGetPlayerSessions)

	// KG data endpoint (paginated, avoids proxy size limits)
	// Viewport fast path: batch geometry by ID (R-tree cached upstream, single-digit ms)
	// Viewport polygon geometry (parcels + footprints) straight from upstream R-tree.
	// Replaces whole-KG export/geojson loads for map rendering.
	mux.HandleFunc("GET /api/viewport", s.handleViewport)
	// Warm registry / lucky pick / KG register (warm.go, upstreams.go)
	mux.HandleFunc("GET /api/lucky", s.handleLucky)
	mux.HandleFunc("GET /api/warm/status", s.handleWarmStatus)
	mux.HandleFunc("GET /api/warm/gemeinden", s.handleWarmGemeinden)
	mux.HandleFunc("POST /api/warm/run-plan", s.requireAhead(s.handleWarmRunPlan))
	mux.HandleFunc("POST /api/warm/trim", s.requireAhead(s.handleWarmTrim))
	mux.HandleFunc("GET /api/contrib/plan", s.handleContribPlan)
	mux.HandleFunc("GET /api/contrib/stats", s.handleContribStats)
	mux.HandleFunc("GET /api/kg-geo/{kg}", s.handleKGGeo)
	mux.HandleFunc("GET /api/search-index", s.handleSearchIndex)
	mux.HandleFunc("GET /api/parcel-find", s.handleParcelFind)
	// Public-tier adapters (landscape.go)
	mux.HandleFunc("GET /api/landmarks", s.handleLandmarks)
	mux.HandleFunc("GET /api/landscape", s.handleLandscape)
	mux.HandleFunc("GET /api/parcel-context", s.handleParcelContext)
	mux.HandleFunc("GET /api/osm-lines", s.handleOsmLines)
	mux.HandleFunc("GET /api/n2k", s.handleN2K)
	mux.HandleFunc("GET /api/municipality", s.handleMunicipalityAt)
	mux.HandleFunc("GET /api/municipalities", s.handleMunicipalities)
	mux.HandleFunc("GET /api/licenses", s.handleLicenses)
	mux.HandleFunc("GET /lizenzen", s.handleLicensesPage)
	mux.HandleFunc("GET /licenses", s.handleLicensesPage)
	// Sibling roadmap consumers (see siblings.go): viewport landuse slice, INVEKOS fields
	mux.HandleFunc("GET /api/viewport-landuse", s.handleViewportLanduse)
	mux.HandleFunc("GET /api/schlaege", s.handleSchlaege)
	mux.HandleFunc("GET /api/ne", s.handleNE)                  // NE cells heat-layer columns (srtm v2.4 observed layer)
	mux.HandleFunc("GET /api/trees", s.handleTrees)            // LID-2 tree apices
	mux.HandleFunc("GET /api/giants-near", s.handleGiantsNear) // nearest giants ring search (hint scout)
	mux.HandleFunc("GET /api/hofstellen", s.handleHofstellen)  // FARM-4 farmsteads
	mux.HandleFunc("GET /api/buildings", s.handleBuildings)    // LID-3 measured heights by footprint_id

	// Building & KG info (slim, aggregated, cached)
	mux.HandleFunc("GET /api/building-info", s.handleBuildingInfo)
	mux.HandleFunc("GET /api/kg-summary/{code}", s.handleKGSummary)

	// Cadastre proxy with caching
	mux.HandleFunc("GET /api/umfeld/", s.handleUmfeldProxy)

	// LiDAR (srtm-lidar) proxy + enhanced-KG registry
	mux.HandleFunc("GET /api/lidar/kg/{code}", s.handleLidarKG)
	mux.HandleFunc("GET /api/lidar/", s.handleLidarProxy)
	mux.HandleFunc("GET /api/enhanced-kgs", s.handleEnhancedKGs)
	mux.HandleFunc("GET /api/upstreams", s.handleUpstreamStatus)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics) // ops snapshot (metrics.go)
	mux.HandleFunc("GET /api/similar", s.handleSimilarParcels)

	slog.Info("starting Siedler Österreich", "addr", addr)
	return http.ListenAndServe(addr, securityHeaders(noindexMiddleware(s.maintenanceMiddleware(rateLimitMiddleware(gzipMiddleware(metricsMiddleware(mux)))))))
}

// ---- Gzip Middleware ----

type gzipResponseWriter struct {
	http.ResponseWriter
	gz            *gzip.Writer
	headerWritten bool
	passthrough   bool // handler sent pre-compressed bytes (Content-Encoding already set)
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.headerWritten = true
	if w.Header().Get("Content-Encoding") != "" {
		// Handler serves a pre-gzipped body (hot viewport cells): don't double-encode.
		w.passthrough = true
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.Header().Del("Content-Length") // compressed size differs
	w.Header().Set("Content-Encoding", "gzip")
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	if w.passthrough {
		return w.ResponseWriter.Write(b)
	}
	return w.gz.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	if !w.passthrough {
		w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip gzip for SSE and non-gzip clients
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			strings.HasSuffix(r.URL.Path, "/events") ||
			strings.HasSuffix(r.URL.Path, ".png") || strings.HasSuffix(r.URL.Path, ".jpg") {
			next.ServeHTTP(w, r)
			return
		}
		// BestSpeed is the wrong tradeoff for our payloads: a viewport response
		// gzips to 161KB at level 1 vs 130KB at level 5 (-19%) for ~1ms more CPU
		// on a ~1MB body. Bandwidth is the bottleneck on mobile, not our CPU.
		gz, _ := gzip.NewWriterLevel(w, 5)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// ---- Helpers ----

// upstreamClient is the shared client for ALL calls to the cadastre / lidar
// APIs. Two reasons not to use http.DefaultClient:
//
//  1. DefaultTransport keeps only 2 idle conns per host. We fan out 4-12
//     concurrent viewport tiles × 2 layers to one host, so most requests were
//     paying a fresh TCP + TLS handshake (~100ms to an HTTPS host) instead of
//     reusing a connection. Upstream docs say ~8-16 in flight is the sweet spot,
//     so size the pool for that.
//  2. DefaultClient has NO timeout — one hung upstream request pinned a
//     goroutine (and a singleflight key, blocking every waiter) forever.
//
// Go's transport also sets Accept-Encoding: gzip and decompresses transparently,
// so we get upstream's compression (a viewport is ~-60% on the wire) for free
// as long as we never set that header by hand.
var upstreamClient = &http.Client{
	Timeout: 60 * time.Second,
	// Wrapped in the per-host circuit breaker (breaker.go): a sibling that is
	// down answers a synthetic 503 in ~0 ms instead of a 10 s proxy stall.
	Transport: upstreamBreaker,
}

func init() {
	upstreamBreaker.base = &http.Transport{
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 24,
		MaxConnsPerHost:     32,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}

func upstreamGet(url string) (*http.Response, error) { return upstreamClient.Get(url) }

func compactOrRaw(raw []byte) []byte {
	if c, err := compactGeoJSON(raw); err == nil {
		return c
	}
	return raw
}

// unpadKG strips leading zeros from a KG code. Upstream is inconsistent about
// zero-padding: /lookup echoes back "3301" for a query of "03301", while
// /query?kg= only matches the zero-padded form. Compare codes unpadded.
func unpadKG(s string) string {
	return strings.TrimLeft(s, "0")
}

// validKG matches Austrian KG codes (5 digits). Path values are interpolated
// into upstream URLs and cache keys, so reject anything else.
func validKG(kg string) bool {
	if len(kg) != 5 {
		return false
	}
	for _, c := range kg {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func randomID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func jsonResp(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// jsonErrBody builds an error body for use inside cachedFetch closures.
func jsonErrBody(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	body := map[string]string{"error": msg}
	if code == 401 || code == 403 || code == 404 || code == 405 {
		body["docs"] = siteURL + "/llm/game" // agents: how auth/endpoints work
	}
	json.NewEncoder(w).Encode(body)
}

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// ---- Index ----

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	setDiscoveryHeaders(w)
	if r.URL.Path == "/" && wantsAgentEdition(r) {
		s.writeLLMGameMarkdown(w) // agents / curl get the text edition
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(s.StaticDir, "index.html"))
}

// handleLegalPage serves a static legal page (Impressum / Datenschutz).
func (s *Server) handleLegalPage(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(s.StaticDir, file))
	}
}

func (s *Server) handleRejoin(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	player, err := s.Q.GetPlayerByToken(r.Context(), token)
	if err != nil {
		http.Redirect(w, r, "/?error=invalid_token", http.StatusFound)
		return
	}
	// Find player's most recent session
	sessions, _ := s.Q.GetPlayerSessions(r.Context(), player.ID)
	q := url.Values{}
	q.Set("pid", player.ID)
	q.Set("pname", player.Name)
	q.Set("rejoin", token)
	if len(sessions) > 0 {
		q.Set("sid", sessions[0].ID)
	}
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusFound)
}

// ---- Auth ----

var nameAdj = []string{
	"Tapfer", "Kühn", "Edel", "Stolz", "Wild", "Flink", "Mutig", "Weise",
	"Stark", "Listig", "Grimmig", "Eisern", "Treu", "Finster", "Feurig",
	"Schnell", "Leise", "Dunkel", "Golden", "Silbern", "Steinig", "Kalt",
	"Schattig", "Stürmisch", "Sanft", "Alt", "Jung", "Groß", "Klein", "Mächtig",
}

var nameNoun = []string{
	"Ritter", "Jäger", "Bauer", "Schmied", "Falke", "Wolf", "Bär", "Adler",
	"Fuchs", "Hirsch", "Löwe", "Drache", "Rabe", "Stein", "Berg", "Bach",
	"Wald", "Turm", "Schild", "Schwert", "Eiche", "Linde", "Fels", "Blitz",
	"Donner", "Schatten", "Flamme", "Frost", "Stern", "Mond",
}

func (s *Server) nameFree(ctx context.Context, name string) bool {
	_, err := s.Q.GetPlayerByName(ctx, name)
	return err != nil
}

// suggestName returns a random Adjective+Noun name that is not yet taken.
// 30×30 = 900 base combos vs. hundreds of registered players, so a plain random
// pick collides often — try many combos, then fall back to suffixing a number
// (still checked for uniqueness).
func (s *Server) suggestName(ctx context.Context) string {
	for i := 0; i < 40; i++ {
		n := nameAdj[mrand.IntN(len(nameAdj))] + nameNoun[mrand.IntN(len(nameNoun))]
		if s.nameFree(ctx, n) {
			return n
		}
	}
	base := nameAdj[mrand.IntN(len(nameAdj))] + nameNoun[mrand.IntN(len(nameNoun))]
	for i := 0; i < 200; i++ {
		n := base + strconv.Itoa(2+mrand.IntN(9000))
		if len(n) <= 30 && s.nameFree(ctx, n) {
			return n
		}
	}
	return base + strconv.FormatInt(time.Now().UnixMilli()%100000, 10)
}

// handleSuggestName gives the client a guaranteed-free name suggestion so the
// dice button / prefill never proposes something that is already taken.
func (s *Server) handleSuggestName(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]any{"name": s.suggestName(r.Context())})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Agent string `json:"agent"` // /llm/game: short model label → 🤖 player, quick-phrase chat only
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) < 2 || len(req.Name) > 30 {
		jsonErr(w, "Name must be 2-30 characters", 400)
		return
	}
	req.Agent = strings.TrimSpace(req.Agent)
	if req.Agent != "" {
		if len(req.Agent) > 40 || !filterName(req.Agent) {
			jsonErr(w, "agent label must be a short, non-personal model name", 400)
			return
		}
		// Humans must always be able to tell: agents wear the 🤖 prefix.
		if !strings.HasPrefix(req.Name, agentPrefix) {
			req.Name = agentPrefix + req.Name
		}
	}
	if !filterName(req.Name) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{
			"error":     "Dieser Name ist nicht erlaubt",
			"suggested": s.suggestName(r.Context()),
		})
		return
	}

	// Check if name exists
	if _, err := s.Q.GetPlayerByName(r.Context(), req.Name); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]any{
			"error":     "Name already taken",
			"suggested": s.suggestName(r.Context()),
		})
		return
	}

	playerID := randomID(16)
	rejoinToken := randomID(24)

	err := s.Q.CreatePlayer(r.Context(), dbgen.CreatePlayerParams{
		ID:          playerID,
		Name:        req.Name,
		RejoinToken: rejoinToken,
	})
	if err != nil {
		jsonErr(w, "Failed to create player", 500)
		return
	}
	if req.Agent != "" {
		s.Q.SetPlayerAgent(r.Context(), dbgen.SetPlayerAgentParams{Agent: req.Agent, ID: playerID})
	}

	player, _ := s.Q.GetPlayerByID(r.Context(), playerID)

	jsonResp(w, map[string]any{
		"player":       player,
		"rejoin_token": rejoinToken,
		"rejoin_url":   fmt.Sprintf("/rejoin/%s", rejoinToken),
	})
}

// authPlayer authenticates a request as the given player via the
// X-Player-Token header (the player's rejoin token). Every mutating
// endpoint that acts on behalf of a player MUST call this — the
// client-supplied player_id alone is spoofable.
func (s *Server) authPlayer(r *http.Request, playerID string) (dbgen.Player, bool) {
	token := r.Header.Get("X-Player-Token")
	if token == "" || playerID == "" {
		return dbgen.Player{}, false
	}
	p, err := s.Q.GetPlayerByToken(r.Context(), token)
	if err != nil || p.ID != playerID {
		return dbgen.Player{}, false
	}
	return p, true
}

// ---- Session Management ----

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID         string  `json:"player_id"`
		Name             string  `json:"name"`
		MunicipalityCode string  `json:"municipality_code"`
		MunicipalityName string  `json:"municipality_name"`
		CenterLon        float64 `json:"center_lon"`
		CenterLat        float64 `json:"center_lat"`
		SpawnExact       bool    `json:"spawn_exact"` // lucky cluster pick: the point was chosen deliberately, never re-snap
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	playerSeen()
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	sessionID := randomID(16)
	inviteCode := randomID(6)

	// Glitch #5: the picker sends the Gemeinde centroid, which for elongated
	// municipalities sits in the forest 2 km from the village. Snap to the
	// settlement (OSM place) when one is close by — but never from an
	// enhanced (srtm v2.4) KG into a bare one: /api/lucky spawns in the
	// middle of a warm+enhanced cluster, and the village centre of the same
	// Gemeinde may well lie in an unprocessed KG (Hafnerbach 19469 vs Korning 19500).
	t0 := time.Now()
	if !req.SpawnExact {
		if lon, lat, ok := s.settlementCenter(req.MunicipalityName, req.CenterLon, req.CenterLat); ok {
			if s.snapDowngrades(req.CenterLon, req.CenterLat, lon, lat) {
				slog.Info("session: settlement snap skipped (would leave enhanced KG)", "gemeinde", req.MunicipalityCode)
			} else {
				req.CenterLon, req.CenterLat = lon, lat
			}
		}
	}

	tSnap := time.Since(t0)
	err := s.Q.CreateSession(r.Context(), dbgen.CreateSessionParams{
		ID:               sessionID,
		Name:             req.Name,
		InviteCode:       inviteCode,
		MunicipalityCode: req.MunicipalityCode,
		MunicipalityName: req.MunicipalityName,
		CenterLon:        req.CenterLon,
		CenterLat:        req.CenterLat,
		CreatedBy:        req.PlayerID,
	})
	if err != nil {
		slog.Error("create session", "error", err)
		jsonErr(w, "Failed to create session", 500)
		return
	}

	// Auto-join the creator
	s.Q.JoinSession(r.Context(), dbgen.JoinSessionParams{
		SessionID: sessionID,
		PlayerID:  req.PlayerID,
	})

	// Generate initial treasures
	s.generateTreasures(r.Context(), sessionID, req.CenterLon, req.CenterLat)

	// Quests for the session creator (joiners get theirs in handleJoinSession)
	s.generateChallenges(r.Context(), sessionID, req.PlayerID, req.CenterLon, req.CenterLat)

	// Natura-2000 bonus treasures: placed asynchronously (fast API, but don't block session create)
	go s.generateN2KTreasures(context.Background(), sessionID, req.MunicipalityCode)
	s.warmGemeinde(req.MunicipalityCode, 0, "session") // cadastre cells for the whole Gemeinde, background

	session, _ := s.Q.GetSession(r.Context(), sessionID)
	slog.Info("session: created", "id", sessionID, "gemeinde", req.MunicipalityCode, "snap_ms", tSnap.Milliseconds(), "total_ms", time.Since(t0).Milliseconds(), "spawn_exact", req.SpawnExact)
	jsonResp(w, map[string]any{
		"session":     session,
		"invite_code": inviteCode,
		"invite_url":  fmt.Sprintf("/join/%s", inviteCode),
	})
}

func (s *Server) handleInvitePreview(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	session, err := s.Q.GetSessionByInvite(r.Context(), code)
	if err != nil {
		jsonErr(w, "Invalid invite code", 404)
		return
	}
	creator, _ := s.Q.GetPlayerByID(r.Context(), session.CreatedBy)
	creatorName := "???"
	if creator.Name != "" {
		creatorName = creator.Name
	}
	jsonResp(w, map[string]any{
		"session":      session,
		"creator_name": creatorName,
	})
}

func (s *Server) handleJoinSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   string `json:"player_id"`
		InviteCode string `json:"invite_code"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	playerSeen()
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	session, err := s.Q.GetSessionByInvite(r.Context(), req.InviteCode)
	if err != nil {
		jsonErr(w, "Invalid invite code", 404)
		return
	}

	s.Q.JoinSession(r.Context(), dbgen.JoinSessionParams{
		SessionID: session.ID,
		PlayerID:  req.PlayerID,
	})

	// Generate challenges for new player
	s.generateChallenges(r.Context(), session.ID, req.PlayerID, session.CenterLon, session.CenterLat)

	// Notify other players
	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	s.broadcast(session.ID, map[string]any{
		"type":   "player_joined",
		"player": player,
	})

	jsonResp(w, map[string]any{"session": session})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.Q.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "Session not found", 404)
		return
	}
	jsonResp(w, session)
}

func (s *Server) handleGetSessionPlayers(w http.ResponseWriter, r *http.Request) {
	players, err := s.Q.GetSessionPlayers(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, players)
}

func (s *Server) handleGetSessionParcels(w http.ResponseWriter, r *http.Request) {
	parcels, err := s.Q.GetSessionParcels(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, parcels)
}

func (s *Server) handleGetSessionTreasures(w http.ResponseWriter, r *http.Request) {
	treasures, err := s.Q.GetSessionTreasures(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, treasures)
}

func (s *Server) handleGetChallenges(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("player_id")
	if playerID == "" {
		jsonErr(w, "player_id required", 400)
		return
	}
	s.backfillChallenges(r.Context(), r.PathValue("id"), playerID)
	// Glitch #1: quests satisfied before the client reconnected (or by actions
	// that don't call autoComplete) stayed open forever. Sweep on every fetch.
	s.autoCompleteChallenges(r.Context(), r.PathValue("id"), playerID)
	challenges, err := s.Q.GetPlayerChallenges(r.Context(), dbgen.GetPlayerChallengesParams{
		SessionID: r.PathValue("id"),
		PlayerID:  playerID,
	})
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	// Attach live progress (have/goal) so the client can render bars + briefings.
	prog := s.questProgress(r.Context(), r.PathValue("id"), playerID)
	type withProg struct {
		dbgen.Challenge
		Progress int64 `json:"progress"`
		Goal     int64 `json:"goal"`
	}
	out := make([]withProg, 0, len(challenges))
	for _, c := range challenges {
		have, goal := questProgressFor(c.Title, prog)
		out = append(out, withProg{c, have, goal})
	}
	jsonResp(w, out)
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case nil:
		return 0
	default:
		return 0
	}
}

func (s *Server) handleGetBiodiversity(w http.ResponseWriter, r *http.Request) {
	row, err := s.Q.GetSessionBiodiversityPercent(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	bioArea := toFloat(row.BioArea)
	totalArea := toFloat(row.TotalArea)
	var pct float64
	if totalArea > 0 {
		pct = (bioArea / totalArea) * 100
	}
	jsonResp(w, map[string]any{
		"biodiversity_area":  bioArea,
		"total_claimed_area": totalArea,
		"percent":            math.Round(pct*10) / 10,
		"target_percent":     30.0,
	})
}

// ---- Game Actions ----

// claimReq is the body of POST /api/claim-parcel. The browser fills it from
// the viewport data it rendered; the agent API (agent.go) fills it
// server-side from cadastre so a script cannot understate area or landuse.
type claimReq struct {
	SessionID         string  `json:"session_id"`
	PlayerID          string  `json:"player_id"`
	ParcelID          string  `json:"parcel_id"`
	KgCode            string  `json:"kg_code"`
	Gnr               string  `json:"gnr"`
	Ez                string  `json:"ez"`
	AreaSqm           float64 `json:"area_sqm"`
	Landuse           string  `json:"landuse"`
	BuildingCount     int     `json:"building_count"`
	TotalBuildingArea float64 `json:"total_building_area"`
	GwStation         bool    `json:"gw_station"`    // GW-8 hint: a Messstelle is snapped to this parcel
	Lon               float64 `json:"lon,omitempty"` // optional: parcel point (speeds up the gauge check)
	Lat               float64 `json:"lat,omitempty"`
	TallTreeCount     int     `json:"tall_tree_count"`
	TallTreeMaxH      float64 `json:"tall_tree_max_h"`
	CropGroup         string  `json:"crop_group"` // FARM-2 INVEKOS class under the parcel ("" = hash kind)
}

func (s *Server) handleClaimParcel(w http.ResponseWriter, r *http.Request) {
	var req claimReq
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	s.claimParcel(w, r, req)
}

// claimParcel is the shared purchase core for the browser and agent APIs.
func (s *Server) claimParcel(w http.ResponseWriter, r *http.Request, req claimReq) {
	// Check if already claimed
	if _, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
		SessionID:  req.SessionID,
		ParcelHash: parcelHash(req.ParcelID),
	}); err == nil {
		jsonErr(w, "Parcel already claimed", 409)
		return
	}

	// Calculate price based on area, landuse, and building density — then the
	// state of the stand/crop: a clear-cut or stubble parcel (harvest state
	// left by a previous owner, parcel_harvest_state) is cheaper while it regrows.
	basePrice := calculatePrice(req.AreaSqm, req.Landuse, req.BuildingCount, req.TotalBuildingArea)
	inherited := s.inheritedHarvest(r.Context(), req.SessionID, req.ParcelID)
	var inheritedAt *time.Time
	if inherited != nil {
		t := inherited.HarvestedAt
		inheritedAt = &t
	}
	regen, regenProgress := regenFactor(harvestKindOf(req.Landuse, req.CropGroup, req.ParcelID), inheritedAt, req.ParcelID, req.CropGroup, time.Now())
	price := int(math.Round(float64(basePrice) * regen))
	if price < 10 {
		price = 10
	}

	// Authenticate and check player has enough coins
	player, ok := s.authPlayer(r, req.PlayerID)
	if !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	if player.Coins < int64(price) {
		jsonErr(w, fmt.Sprintf("Not enough coins! Need %d, have %d", price, player.Coins), 400)
		return
	}

	// Tall-tree bonus: parcels containing lidar-confirmed landmark trees award extra XP
	tallBonus, tallTrees := 0, 0
	tallN, tallH := req.TallTreeCount, req.TallTreeMaxH
	// NE cells: the parcel's measured stand is authoritative — every apex of
	// the parcel is in the cell row, so the client's count is only a fallback
	// for KGs without an observed layer.
	if p, ok := s.lookupParcelCached(req.ParcelID, req.Lon, req.Lat); ok && p != nil && p.NE != nil && p.NE.Cells > 0 {
		// Giants stay rare: a bonus needs a ≥ 28 m crown; 33 / 38 m count double / triple
		// (trees_tall counts every apex ≥ 20 m — that is most of any forest).
		tallN, tallH = 0, 0
		if h := p.NE.TreeHMaxM; h >= 28 {
			tallN, tallH = 1, h
			if h >= 33 {
				tallN++
			}
			if h >= 38 {
				tallN++
			}
		}
	}
	if tallN > 0 && tallH > 0 && tallH <= 60 {
		if tallN > 10 {
			tallN = 10
		}
		tallTrees = tallN
		tallBonus = tallN*40 + int(tallH)
		if tallBonus > 300 {
			tallBonus = 300
		}
	}

	// NE cells (observed layer): the consistency verdict of the parcel from our
	// cached cell — "" when the KG has no v2.4 observation. A discrepancy
	// (observation ≠ cadastre) pays a small Spurenleser bonus and counts for
	// the quest of the same name; forest_loss unlocks the Wiederbewaldung bonus
	// on Naturschutz conversion (handleConvertParcel).
	verdict := s.neVerdictOf(req.ParcelID, req.Lon, req.Lat)
	neBonus := 0
	if neDiscrepant(verdict) {
		neBonus = neBonusXP
	}

	landuse := req.Landuse
	err := s.Q.ClaimParcel(r.Context(), dbgen.ClaimParcelParams{
		SessionID:     req.SessionID,
		PlayerID:      req.PlayerID,
		ParcelHash:    parcelHash(req.ParcelID),
		KgCode:        req.KgCode,
		Gnr:           "",
		EzHash:        ezHash(req.KgCode, req.Ez),
		AreaSqm:       req.AreaSqm,
		Landuse:       &landuse,
		PurchasePrice: int64(price),
		TallTrees:     int64(tallTrees),
		NeVerdict:     verdict,
	})
	if err != nil {
		slog.Error("claim parcel", "error", err)
		jsonErr(w, "Failed to claim parcel", 500)
		return
	}
	if inherited != nil {
		s.seedInheritedHarvest(r.Context(), req.SessionID, req.ParcelID)
	}

	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: int64(-price),
		ID:    req.PlayerID,
	})
	// GW-8 Pegelwart: the parcel carries a real measuring point (groundwater
	// or nitrate station, water-quality site, power plant). Only checked when
	// the client saw one there (G.gwPoints parcel_id) or the agent path asks;
	// verified against gw /llm/points + the parcel polygon (≤ 1.5 s, water.go).
	stationBonus, stationCat := 0, ""
	if req.GwStation {
		if ok, cat := s.stationOnParcel(req.ParcelID, req.Lon, req.Lat); ok {
			stationBonus, stationCat = 80, cat
		}
	}
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{
		Xp: int64(10 + tallBonus + stationBonus + neBonus),
		ID: req.PlayerID,
	})

	// Broadcast
	s.broadcast(req.SessionID, map[string]any{
		"type":      "parcel_claimed",
		"parcel_id": req.ParcelID,
		"player":    player.Name,
	})

	quests := s.autoCompleteChallenges(r.Context(), req.SessionID, req.PlayerID)
	updatedPlayer, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	jsonResp(w, map[string]any{
		"success":          true,
		"price":            price,
		"base_price":       basePrice,
		"regen":            regen,
		"regen_progress":   regenProgress,
		"inherited":        inherited != nil,
		"player":           updatedPlayer,
		"tall_bonus_xp":    tallBonus,
		"station_bonus_xp": stationBonus,
		"station_category": stationCat,
		"ne_verdict":       verdict,
		"ne_bonus_xp":      neBonus,
		"quests":           quests,
		"notice":           bevNotice,
	})
}

func (s *Server) handleClaimEZ(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		KgCode    string `json:"kg_code"`
		Ez        string `json:"ez"`
		Parcels   []struct {
			ParcelID          string  `json:"parcel_id"`
			Gnr               string  `json:"gnr"`
			AreaSqm           float64 `json:"area_sqm"`
			Landuse           string  `json:"landuse"`
			BuildingCount     int     `json:"building_count"`
			TotalBuildingArea float64 `json:"total_building_area"`
		} `json:"parcels"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	if len(req.Parcels) == 0 {
		jsonErr(w, "No parcels to claim", 400)
		return
	}
	if len(req.Parcels) > 100 {
		jsonErr(w, "Too many parcels (max 100)", 400)
		return
	}

	// Calculate total price with 20% EZ bulk discount
	totalPrice := 0
	for _, p := range req.Parcels {
		// Skip already claimed
		if _, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
			SessionID:  req.SessionID,
			ParcelHash: parcelHash(p.ParcelID),
		}); err == nil {
			continue
		}
		totalPrice += s.regenPrice(r.Context(), req.SessionID, p.ParcelID, p.Landuse, calculatePrice(p.AreaSqm, p.Landuse, p.BuildingCount, p.TotalBuildingArea))
	}

	// 20% discount for bulk EZ claim
	discountedPrice := int(float64(totalPrice) * 0.8)

	// Authenticate and check player has enough coins
	player, ok := s.authPlayer(r, req.PlayerID)
	if !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	if player.Coins < int64(discountedPrice) {
		jsonErr(w, fmt.Sprintf("Not enough coins! Need %d, have %d", discountedPrice, player.Coins), 400)
		return
	}

	// Claim all unclaimed parcels
	claimed := 0
	for _, p := range req.Parcels {
		// Skip already claimed
		if _, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
			SessionID:  req.SessionID,
			ParcelHash: parcelHash(p.ParcelID),
		}); err == nil {
			continue
		}
		price := s.regenPrice(r.Context(), req.SessionID, p.ParcelID, p.Landuse, calculatePrice(p.AreaSqm, p.Landuse, p.BuildingCount, p.TotalBuildingArea))
		// Each parcel gets its proportional discounted price
		discPrice := int64(float64(price) * 0.8)
		landuse := p.Landuse
		s.Q.ClaimParcel(r.Context(), dbgen.ClaimParcelParams{
			SessionID:     req.SessionID,
			PlayerID:      req.PlayerID,
			ParcelHash:    parcelHash(p.ParcelID),
			KgCode:        req.KgCode,
			Gnr:           "",
			EzHash:        ezHash(req.KgCode, req.Ez),
			AreaSqm:       p.AreaSqm,
			Landuse:       &landuse,
			PurchasePrice: discPrice,
		})
		s.seedInheritedHarvest(r.Context(), req.SessionID, p.ParcelID)
		claimed++
	}

	if claimed == 0 {
		jsonErr(w, "All parcels in this EZ are already claimed", 409)
		return
	}

	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: int64(-discountedPrice),
		ID:    req.PlayerID,
	})
	xpReward := int64(claimed * 15) // Bonus XP for bulk EZ claim
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{
		Xp: xpReward,
		ID: req.PlayerID,
	})

	s.broadcast(req.SessionID, map[string]any{
		"type":    "ez_claimed",
		"ez":      req.Ez,
		"kg_code": req.KgCode,
		"count":   claimed,
		"player":  player.Name,
	})

	quests := s.autoCompleteChallenges(r.Context(), req.SessionID, req.PlayerID)
	updatedPlayer, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	jsonResp(w, map[string]any{
		"success":       true,
		"quests":        quests,
		"claimed_count": claimed,
		"total_price":   discountedPrice,
		"discount":      totalPrice - discountedPrice,
		"xp_reward":     xpReward,
		"player":        updatedPlayer,
	})
}

func (s *Server) handleConvertParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string  `json:"session_id"`
		PlayerID  string  `json:"player_id"`
		ParcelID  string  `json:"parcel_id"`
		ConvertTo string  `json:"convert_to"` // biodiversity, forest, wetland, meadow
		Lon       float64 `json:"lon"`        // parcel centroid (optional) → GW-5 Wasserschutzgebiet bonus
		Lat       float64 `json:"lat"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
		SessionID:  req.SessionID,
		ParcelHash: parcelHash(req.ParcelID),
	})
	if err != nil {
		jsonErr(w, "Parcel not found", 404)
		return
	}
	if claim.PlayerID != req.PlayerID {
		jsonErr(w, "Not your parcel", 403)
		return
	}

	if claim.ConvertedTo != nil && *claim.ConvertedTo != "" {
		jsonErr(w, "Schon umgewandelt", 400)
		return
	}
	// Award XP for conversion
	xpReward := int64(50)
	resp := map[string]any{"success": true}
	switch req.ConvertTo {
	case "biodiversity":
		xpReward = 100
		// Wiederbewaldung: the observed layer saw this forest disappear
		// (NE verdict forest_loss at claim time) — protecting it now is
		// worth double, and shows up as the Spurenleser's follow-through.
		if claim.NeVerdict == "forest_loss" {
			xpReward = 200
			resp["ne_restore"] = "forest_loss"
		}
	case "wildforest":
		// Naturwald / Außernutzungstellung: only on forest, only on a grown
		// stand; XP scales with the standing stock given up (timber.go).
		lu := ""
		if claim.Landuse != nil {
			lu = *claim.Landuse
		}
		est := s.estimateTimber(r.Context(), claim.KgCode, req.ParcelID, claim.AreaSqm, lu, false)
		if !est.IsForest {
			jsonErr(w, "Das ist kein Wald", 400)
			return
		}
		if stage, _, _ := forestPhase(claim.HarvestedAt, time.Now()); stage != "baumholz" {
			jsonErr(w, "Der Wald muss erst nachwachsen", 409)
			return
		}
		xpReward = est.WildXP
		resp["vfm"] = est.Vfm
		resp["co2_t"] = est.CO2t
	case "forest":
	default:
		jsonErr(w, "unknown conversion", 400)
		return
	}
	// GW-5: a meadow that stays a meadow above a drinking-water aquifer is
	// worth more — Naturschutz/Naturwald inside a Wasserschutz-/Schongebiet ×1.5 XP.
	if (req.ConvertTo == "biodiversity" || req.ConvertTo == "wildforest") && req.Lon != 0 && req.Lat != 0 {
		if zt, zone := s.inWaterProtection(req.Lon, req.Lat); zt != "" {
			xpReward = xpReward * 3 / 2
			resp["water_protection"] = map[string]any{"type": zt, "zone": zone}
		}
	}
	convertTo := req.ConvertTo
	s.Q.ConvertParcel(r.Context(), dbgen.ConvertParcelParams{
		ConvertedTo: &convertTo,
		ConvertXp:   xpReward,
		ID:          claim.ID,
	})
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{
		Xp: xpReward,
		ID: req.PlayerID,
	})

	quests := s.autoCompleteChallenges(r.Context(), req.SessionID, req.PlayerID)
	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type":       "parcel_converted",
		"parcel_id":  req.ParcelID,
		"convert_to": req.ConvertTo,
		"player":     player.Name,
	})

	resp["xp_reward"], resp["player"], resp["quests"] = xpReward, player, quests
	jsonResp(w, resp)
}

// treeHeights extracts {mean,max} of the tree class from a v2 parcel's
// height_distribution; nil when the parcel carries no trees.
func treeHeights(pd map[string]any) map[string]any {
	hd, _ := pd["height_distribution"].(map[string]any)
	tr, _ := hd["tree"].(map[string]any)
	if tr == nil {
		return nil
	}
	mean, _ := tr["mean"].(float64)
	max, _ := tr["max"].(float64)
	if mean <= 0 {
		return nil
	}
	return map[string]any{"mean": math.Round(mean*10) / 10, "max": math.Round(max*10) / 10}
}

// handleHarvestParcel: harvest an owned Acker during its ripe window (see
// fieldcycle.go). Idempotent per cycle — a second harvest in the same cycle
// is rejected, and once the ripe window closes the NPC farmers took it.
func (s *Server) handleHarvestParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		ParcelID  string `json:"parcel_id"`
		CropGroup string `json:"crop_group"` // FARM-2 INVEKOS class, "" = hash kind
		Organic   bool   `json:"organic"`    // FARM-2 Schlag organic flag → ÖPUL premium on the Förderung
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{SessionID: req.SessionID, ParcelHash: parcelHash(req.ParcelID)})
	if err != nil {
		jsonErr(w, "Parcel not found", 404)
		return
	}
	if claim.PlayerID != req.PlayerID {
		jsonErr(w, "Not your parcel", 403)
		return
	}
	if claim.ConvertedTo != nil && *claim.ConvertedTo != "" {
		jsonErr(w, "Diese Parzelle wird nicht mehr bewirtschaftet", 400)
		return
	}
	if claim.Landuse == nil || *claim.Landuse != "48" {
		jsonErr(w, "Das ist kein Acker", 400)
		return
	}
	now := time.Now()
	fp := fieldPhaseAtCrop(req.ParcelID, now, req.CropGroup)
	base := int64(0)
	if fp.Stage == "meadow" {
		// Meadows are not harvested — but they collect the Förderung (FARM-1)
		// once per cycle, gated by the same harvested_at timestamp.
		if claim.HarvestedAt != nil && now.Sub(*claim.HarvestedAt) < fieldCycle {
			jsonErr(w, "Förderung schon abgeholt — nächste Auszahlung in "+fmtMinutes(fieldCycle-now.Sub(*claim.HarvestedAt)), 409)
			return
		}
	} else {
		if fp.Stage != "ripe" {
			jsonErr(w, "Das Feld ist noch nicht reif", 409)
			return
		}
		if claim.HarvestedAt != nil && !claim.HarvestedAt.Before(fp.CycleStart) {
			jsonErr(w, "Schon geerntet — das Feld muss erst wieder wachsen", 409)
			return
		}
		base = harvestYield(claim.AreaSqm)
	}
	eco := s.harvestPayout(claim.KgCode, claim, base, req.Organic)
	if eco.Total <= 0 {
		jsonErr(w, "Eine Weide erntet man nicht — die Kühe machen das", 400)
		return
	}
	coins := eco.Total
	xp := 10 + coins/5
	ctx := r.Context()
	s.Q.HarvestParcel(ctx, claim.ID)
	s.recordHarvestState(ctx, req.SessionID, req.ParcelID, map[bool]string{true: "meadow", false: "field"}[fp.Stage == "meadow"], req.CropGroup, now, claim.Harvests+1)
	s.Q.UpdatePlayerCoins(ctx, dbgen.UpdatePlayerCoinsParams{Coins: coins, ID: req.PlayerID})
	s.Q.UpdatePlayerXP(ctx, dbgen.UpdatePlayerXPParams{Xp: xp, ID: req.PlayerID})
	quests := s.autoCompleteChallenges(ctx, req.SessionID, req.PlayerID)
	player, _ := s.Q.GetPlayerByID(ctx, req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type": "parcel_harvested", "parcel_id": req.ParcelID, "player": player.Name, "coins": coins,
		"meadow": fp.Stage == "meadow", "drought": eco.Drought,
	})
	jsonResp(w, map[string]any{
		"success": true, "coins": coins, "xp": xp, "player": player, "quests": quests, "economy": eco,
		"harvested_at": now, "next_ripe_at": nextPayoutAt(fp, now), "cycle_s": fp.Cycle.Seconds(),
	})
}

func (s *Server) handleSellParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		ClaimID   int64  `json:"claim_id"`
		ParcelID  string `json:"parcel_id"`  // needed for the regrowth clock (hash) + broadcast
		CropGroup string `json:"crop_group"` // FARM-2 class, decides the field cycle
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	// Get the claim and verify ownership
	parcels, err := s.Q.GetPlayerParcels(r.Context(), dbgen.GetPlayerParcelsParams{
		SessionID: req.SessionID,
		PlayerID:  req.PlayerID,
	})
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}

	var claim *dbgen.ParcelClaim
	for _, p := range parcels {
		if p.ID == req.ClaimID {
			claim = &p
			break
		}
	}
	if claim == nil {
		jsonErr(w, "Not your parcel", 403)
		return
	}

	// Sell at 60 % of the purchase price × current regrowth: a stand cut
	// after buying is worth less than the mature one that was paid for.
	now := time.Now()
	regen, regenProgress := claimRegen(claim, req.ParcelID, req.CropGroup, now)
	payout := sellPrice(claim.PurchasePrice, regen)
	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: payout,
		ID:    req.PlayerID,
	})
	// A protected parcel (Naturschutz / Aufforstung / Naturwald) can be sold
	// too, but the protection is given up and the XP it earned goes back —
	// otherwise convert → sell → rebuy → convert would farm XP.
	xpLost := convertXPOf(claim)
	if xpLost > 0 {
		s.DB.ExecContext(r.Context(), "UPDATE players SET xp = MAX(0, xp - ?) WHERE id = ?", xpLost, req.PlayerID)
	}

	// The regrowth clock survives the sale (parcel_harvest_state, hash only).
	if claim.HarvestedAt != nil && req.ParcelID != "" {
		lu := ""
		if claim.Landuse != nil {
			lu = *claim.Landuse
		}
		s.recordHarvestState(r.Context(), req.SessionID, req.ParcelID, harvestKindOf(lu, req.CropGroup, req.ParcelID), req.CropGroup, *claim.HarvestedAt, claim.Harvests)
	}

	// Delete claim
	s.DB.ExecContext(r.Context(), "DELETE FROM parcel_claims WHERE id = ?", req.ClaimID)

	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type":        "parcel_sold",
		"parcel_id":   req.ParcelID,
		"parcel_hash": claim.ParcelHash,
		"player":      player.Name,
	})

	jsonResp(w, map[string]any{"success": true, "sell_price": payout, "regen": regen, "regen_progress": regenProgress, "xp_lost": xpLost, "player": player})
}

// convertXPOf is the XP a sale of a converted parcel takes back: the stored
// reward, or (claims converted before migration 018) the base reward of the kind.
func convertXPOf(c *dbgen.ParcelClaim) int64 {
	if c.ConvertedTo == nil || *c.ConvertedTo == "" {
		return 0
	}
	if c.ConvertXp > 0 {
		return c.ConvertXp
	}
	switch *c.ConvertedTo {
	case "biodiversity":
		return 100
	case "wildforest":
		return 150
	}
	return 50
}

// ---- Parcel Offer System ----

func (s *Server) handleOfferParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID  string `json:"session_id"`
		BuyerID    string `json:"buyer_id"`
		ParcelID   string `json:"parcel_id"`
		OfferPrice int64  `json:"offer_price"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	if req.OfferPrice < 10 || req.OfferPrice > 1_000_000 {
		jsonErr(w, "Angebot muss zwischen 10 und 1.000.000 Münzen liegen", 400)
		return
	}

	// Verify parcel is claimed
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
		SessionID:  req.SessionID,
		ParcelHash: parcelHash(req.ParcelID),
	})
	if err != nil {
		jsonErr(w, "Parzelle nicht gefunden", 404)
		return
	}
	if claim.PlayerID == req.BuyerID {
		jsonErr(w, "Du besitzt diese Parzelle bereits", 400)
		return
	}
	if claim.ConvertedTo != nil && *claim.ConvertedTo != "" {
		// reserves / afforestation are permanent — the owner has no sell or
		// accept button there, so an offer could never be answered
		jsonErr(w, "Diese Parzelle ist geschützt und nicht verkäuflich", 400)
		return
	}

	// Authenticate buyer and check coins (soft check — coins aren't locked)
	buyer, ok := s.authPlayer(r, req.BuyerID)
	if !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	if buyer.Coins < req.OfferPrice {
		jsonErr(w, fmt.Sprintf("Nicht genug Münzen! Brauchst %d, hast %d", req.OfferPrice, buyer.Coins), 400)
		return
	}

	// Create the offer
	err = s.Q.CreateParcelOffer(r.Context(), dbgen.CreateParcelOfferParams{
		SessionID:  req.SessionID,
		ParcelHash: parcelHash(req.ParcelID),
		ClaimID:    claim.ID,
		BuyerID:    req.BuyerID,
		SellerID:   claim.PlayerID,
		OfferPrice: req.OfferPrice,
	})
	if err != nil {
		slog.Error("create offer", "error", err)
		jsonErr(w, "Angebot konnte nicht erstellt werden", 500)
		return
	}

	// Notify seller via SSE
	seller, _ := s.Q.GetPlayerByID(r.Context(), claim.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type":        "offer_made",
		"parcel_id":   req.ParcelID,
		"buyer":       buyer.Name,
		"seller":      seller.Name,
		"seller_id":   claim.PlayerID,
		"offer_price": req.OfferPrice,
	})

	jsonResp(w, map[string]any{"success": true, "message": "Angebot gesendet!"})
}

func (s *Server) handleOfferRespond(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OfferID  int64  `json:"offer_id"`
		PlayerID string `json:"player_id"` // must be seller
		Accept   bool   `json:"accept"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	offer, err := s.Q.GetOfferByID(r.Context(), req.OfferID)
	if err != nil {
		jsonErr(w, "Angebot nicht gefunden", 404)
		return
	}
	if offer.Status != "pending" {
		jsonErr(w, "Angebot nicht mehr gültig", 400)
		return
	}
	if offer.SellerID != req.PlayerID {
		jsonErr(w, "Nicht dein Angebot", 403)
		return
	}

	if !req.Accept {
		// Reject
		s.Q.UpdateOfferStatus(r.Context(), dbgen.UpdateOfferStatusParams{
			Status: "rejected",
			ID:     req.OfferID,
		})
		s.broadcast(offer.SessionID, map[string]any{
			"type":        "offer_rejected",
			"parcel_hash": offer.ParcelHash,
			"buyer_id":    offer.BuyerID,
			"seller_id":   offer.SellerID,
		})
		jsonResp(w, map[string]any{"success": true, "action": "rejected"})
		return
	}

	// Accept — check buyer has enough coins
	buyer, err := s.Q.GetPlayerByID(r.Context(), offer.BuyerID)
	if err != nil {
		jsonErr(w, "Käufer nicht gefunden", 404)
		return
	}
	if buyer.Coins < offer.OfferPrice {
		// Buyer doesn't have enough — they need to sell parcels first
		// We notify them and keep the offer pending
		s.broadcast(offer.SessionID, map[string]any{
			"type":        "offer_funds_needed",
			"parcel_hash": offer.ParcelHash,
			"buyer_id":    offer.BuyerID,
			"offer_id":    offer.ID,
			"offer_price": offer.OfferPrice,
			"buyer_coins": buyer.Coins,
			"shortfall":   offer.OfferPrice - buyer.Coins,
		})
		jsonErr(w, fmt.Sprintf("Käufer hat nur %d Münzen, braucht %d. Käufer muss Parzellen verkaufen!", buyer.Coins, offer.OfferPrice), 400)
		return
	}

	// Execute the trade
	// 1. Mark offer as accepted
	s.Q.UpdateOfferStatus(r.Context(), dbgen.UpdateOfferStatusParams{
		Status: "accepted",
		ID:     req.OfferID,
	})

	// 2. Cancel other pending offers for this parcel
	s.Q.CancelPendingOffersForParcel(r.Context(), dbgen.CancelPendingOffersForParcelParams{
		ParcelHash: offer.ParcelHash,
		SessionID:  offer.SessionID,
	})

	// 3. Transfer coins: buyer pays, seller receives
	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: -offer.OfferPrice,
		ID:    offer.BuyerID,
	})
	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: offer.OfferPrice,
		ID:    offer.SellerID,
	})

	// 4. Transfer parcel ownership: update the claim in place
	s.DB.ExecContext(r.Context(),
		"UPDATE parcel_claims SET player_id = ?, purchase_price = ?, converted_to = NULL WHERE id = ?",
		offer.BuyerID, offer.OfferPrice, offer.ClaimID)

	// 5. Broadcast
	seller, _ := s.Q.GetPlayerByID(r.Context(), offer.SellerID)
	updatedBuyer, _ := s.Q.GetPlayerByID(r.Context(), offer.BuyerID)
	s.broadcast(offer.SessionID, map[string]any{
		"type":        "offer_accepted",
		"parcel_hash": offer.ParcelHash,
		"buyer":       updatedBuyer.Name,
		"buyer_id":    offer.BuyerID,
		"seller":      seller.Name,
		"seller_id":   offer.SellerID,
		"offer_price": offer.OfferPrice,
	})

	jsonResp(w, map[string]any{
		"success": true,
		"action":  "accepted",
		"buyer":   updatedBuyer,
		"seller":  seller,
	})
}

func (s *Server) handleGetOffers(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	offers, err := s.Q.GetSessionOffers(r.Context(), sessionID)
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, offers)
}

func (s *Server) handleClaimTreasure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   string `json:"player_id"`
		TreasureID int64  `json:"treasure_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	// Roaming wildlife that has moved on is gone, even if the next /roam
	// call has not pruned the row yet (agents see moves_on_in_s in look).
	var pre struct {
		Type      string
		CreatedAt time.Time
		FoundBy   *string
	}
	if s.DB.QueryRowContext(r.Context(), "SELECT treasure_type, created_at, found_by FROM treasures WHERE id = ?", req.TreasureID).Scan(&pre.Type, &pre.CreatedAt, &pre.FoundBy) == nil &&
		roamingGone(dbgen.Treasure{TreasureType: pre.Type, CreatedAt: pre.CreatedAt, FoundBy: pre.FoundBy}, time.Now()) {
		s.DB.ExecContext(r.Context(), "DELETE FROM treasures WHERE id = ? AND found_by IS NULL", req.TreasureID)
		jsonErr(w, "This animal has moved on", 410)
		return
	}

	rows, err := s.Q.ClaimTreasure(r.Context(), dbgen.ClaimTreasureParams{
		FoundBy: &req.PlayerID,
		ID:      req.TreasureID,
	})
	if err != nil || rows == 0 {
		// rows==0: treasure doesn't exist or was already claimed (guarded by
		// "AND found_by IS NULL" in the UPDATE — prevents double-claiming).
		jsonErr(w, "Treasure already claimed", 409)
		return
	}

	// Get treasure value and award
	var value int64
	var ttype, speciesName, speciesGerman, speciesCat, tSession string
	s.DB.QueryRowContext(r.Context(),
		"SELECT value, treasure_type, species_name, species_german, species_category, session_id FROM treasures WHERE id = ?", req.TreasureID).Scan(&value, &ttype, &speciesName, &speciesGerman, &speciesCat, &tSession)

	if ttype == "xp" {
		s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{Xp: value, ID: req.PlayerID})
	} else if ttype == "roaming" {
		// a wildlife encounter: coins + half as XP (Kundschafter)
		s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{Coins: value, ID: req.PlayerID})
		s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{Xp: value / 2, ID: req.PlayerID})
	} else {
		// species and coins both award coins
		s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{Coins: value, ID: req.PlayerID})
	}

	quests := s.autoCompleteChallenges(r.Context(), tSession, req.PlayerID)
	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	resp := map[string]any{"success": true, "type": ttype, "value": value, "player": player, "quests": quests}
	// others drop the chest from their map instead of running into a 409
	s.broadcast(tSession, map[string]any{"type": "treasure_claimed", "id": req.TreasureID, "player": player.Name, "player_id": req.PlayerID, "treasure_type": ttype})
	if speciesName != "" {
		resp["species_name"] = speciesName
		resp["species_german"] = speciesGerman
		resp["species_category"] = speciesCat
	}
	jsonResp(w, resp)
}

func (s *Server) handleCompleteChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID    string `json:"player_id"`
		ChallengeID int64  `json:"challenge_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}

	// Get challenge
	var coins, xp int64
	var sessionID, title string
	err := s.DB.QueryRowContext(r.Context(),
		"SELECT reward_coins, reward_xp, session_id, title FROM challenges WHERE id = ? AND player_id = ? AND completed = 0",
		req.ChallengeID, req.PlayerID).Scan(&coins, &xp, &sessionID, &title)
	if err != nil {
		jsonErr(w, "Challenge not found or already completed", 404)
		return
	}
	if !questSatisfied(title, s.questProgress(r.Context(), sessionID, req.PlayerID)) {
		jsonErr(w, "Aufgabe noch nicht erfüllt", 400)
		return
	}

	s.Q.CompleteChallenge(r.Context(), req.ChallengeID)
	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{Coins: coins, ID: req.PlayerID})
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{Xp: xp, ID: req.PlayerID})

	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)

	s.broadcast(sessionID, map[string]any{
		"type":   "challenge_completed",
		"player": player.Name,
	})

	jsonResp(w, map[string]any{"success": true, "coins": coins, "xp": xp, "player": player})
}

// ---- Chat ----

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := int64(50)
	if l, err := strconv.ParseInt(limitStr, 10, 64); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	msgs, err := s.Q.GetRecentChat(r.Context(), dbgen.GetRecentChatParams{
		SessionID: r.PathValue("id"),
		Limit:     limit,
	})
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	// Hide messages from players the viewer has blocked.
	if pid := r.URL.Query().Get("player_id"); pid != "" {
		if blocks, err := s.Q.ListBlocks(r.Context(), pid); err == nil && len(blocks) > 0 {
			bl := map[string]bool{}
			for _, b := range blocks {
				bl[b.BlockedID] = true
			}
			out := msgs[:0]
			for _, m := range msgs {
				if !bl[m.PlayerID] {
					out = append(out, m)
				}
			}
			msgs = out
		}
	}
	jsonResp(w, msgs)
}

func (s *Server) handlePostChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
		Message  string `json:"message"`
		Quick    int    `json:"quick"` // index into quickPhrases (+1), 0 = free text
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	player, ok := s.authPlayer(r, req.PlayerID)
	if !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	ctx := r.Context()
	sessionID := r.PathValue("id")
	sess, err := s.Q.GetSession(ctx, sessionID)
	if err != nil {
		jsonErr(w, "session not found", 404)
		return
	}
	if player.ChatBanned != 0 {
		jsonErr(w, "Dein Chat ist dauerhaft gesperrt.", 403)
		return
	}
	if until, muted := mutedUntil(player); muted {
		jsonErr(w, "Dein Chat ist noch "+fmtRemaining(until)+" gesperrt.", 403)
		return
	}
	if player.ChatRulesAccepted == 0 {
		jsonErr(w, "rules_required", 428)
		return
	}
	if player.Agent != "" && (req.Quick < 1 || req.Quick > len(quickPhrases)) {
		// Datenschutz §7: agents never free-text at (possibly minor) humans.
		jsonErr(w, "agents may only send quick phrases (quick=1..N, see GET /api/chat/rules)", 403)
		return
	}
	switch sess.ChatMode {
	case "off":
		jsonErr(w, "Der Chat ist in diesem Spiel deaktiviert.", 403)
		return
	case "quick":
		if req.Quick < 1 || req.Quick > len(quickPhrases) {
			jsonErr(w, "In diesem Spiel sind nur Schnellnachrichten erlaubt.", 403)
			return
		}
	}
	if req.Quick >= 1 && req.Quick <= len(quickPhrases) {
		req.Message = quickPhrases[req.Quick-1]
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || len(req.Message) > chatMaxLen {
		jsonErr(w, fmt.Sprintf("Nachricht muss 1-%d Zeichen lang sein", chatMaxLen), 400)
		return
	}

	if ok, dup, strike := chatLim.allow(player.ID, req.Message); !ok {
		if dup {
			jsonErr(w, "Diese Nachricht hast du gerade schon gesendet.", 429)
			return
		}
		if strike {
			s.applyStrikes(ctx, player.ID, 1, "flood")
		}
		jsonErr(w, "Langsam! Bitte warte ein paar Sekunden.", 429)
		return
	}

	if req.Quick == 0 {
		fr := filterChat(req.Message)
		switch {
		case fr.Severity >= 2:
			strikes := int64(1)
			if fr.Severity == 3 {
				strikes = 3
			}
			s.Q.LogSafetyEvent(ctx, dbgen.LogSafetyEventParams{Kind: "filter_" + fr.Category, PlayerID: &player.ID, SessionID: &sessionID, Detail: req.Message})
			cons := s.applyStrikes(ctx, player.ID, strikes, "filter:"+fr.Category)
			if fr.Severity == 3 || fr.Category == "grooming" {
				s.notifySafety("filter_"+fr.Category, sessionID, player, "Blockierte Nachricht: "+req.Message)
			}
			msg := fr.Message
			if cons != "" {
				msg += " " + cons
			}
			jsonErr(w, msg, 422)
			return
		case fr.Severity == 1:
			req.Message = fr.Masked
		}
	}

	msg, err := s.Q.CreateChatMessage(ctx, dbgen.CreateChatMessageParams{
		SessionID: sessionID,
		PlayerID:  req.PlayerID,
		Message:   req.Message,
	})
	if err != nil {
		jsonErr(w, "Failed to send", 500)
		return
	}

	s.broadcast(sessionID, map[string]any{
		"type":      "chat",
		"id":        msg.ID,
		"message":   msg.Message,
		"player":    player.Name,
		"player_id": player.ID,
		"time":      msg.CreatedAt,
	})

	jsonResp(w, msg)
}

// handleSetChatMode lets the session creator switch chat between
// free / quick (preset phrases only) / off — e.g. for classrooms.
func (s *Server) handleSetChatMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
		Mode     string `json:"mode"`
	}
	if err := readJSON(r, &req); err != nil || !validChatModes[req.Mode] {
		jsonErr(w, "invalid request", 400)
		return
	}
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	sess, err := s.Q.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "session not found", 404)
		return
	}
	if sess.CreatedBy != req.PlayerID {
		jsonErr(w, "Nur der Spielersteller kann den Chat-Modus ändern.", 403)
		return
	}
	if err := s.Q.SetSessionChatMode(r.Context(), dbgen.SetSessionChatModeParams{ChatMode: req.Mode, ID: sess.ID}); err != nil {
		jsonErr(w, "error", 500)
		return
	}
	s.broadcast(sess.ID, map[string]any{"type": "chat_mode", "mode": req.Mode})
	jsonResp(w, map[string]any{"ok": true, "mode": req.Mode})
}

func (s *Server) handleChatRules(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]any{
		"quick_phrases": quickPhrases,
		"rules": []string{
			"Sei freundlich – keine Beleidigungen, kein Hass.",
			"Teile nichts Persönliches: kein Alter, keine Adresse, keine Schule, keine Telefonnummer, kein echter Name.",
			"Keine Links, keine Social-Media-Namen, keine Treffen außerhalb des Spiels.",
			"Wenn dir etwas komisch vorkommt: Nachricht melden (⚑) oder Spieler blockieren (🚫) – und einer erwachsenen Vertrauensperson erzählen.",
			"Der Chat wird automatisch gefiltert. Verstöße führen zu Sperren.",
		},
	})
}

func (s *Server) handleAcceptChatRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	s.Q.AcceptChatRules(r.Context(), req.PlayerID)
	jsonResp(w, map[string]any{"ok": true})
}

var reportReasons = map[string]string{
	"harassment": "Beleidigung / Belästigung",
	"hate":       "Hassrede",
	"sexual":     "Sexuelle Inhalte",
	"grooming":   "Fragt nach Alter / Fotos / Treffen / Kontakt",
	"personal":   "Teilt persönliche Daten",
	"spam":       "Spam / Werbung",
	"other":      "Sonstiges",
}

// handleReport implements notice-and-action (DSA Art. 16) without human
// staff: the message is hidden for everyone immediately, the reporter
// auto-blocks the target, independent reports escalate to mutes, and the
// operator gets a mail with context and signed action links.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  string `json:"player_id"`
		SessionID string `json:"session_id"`
		MessageID int64  `json:"message_id"`
		TargetID  string `json:"target_id"`
		Reason    string `json:"reason"`
		Note      string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	reporter, ok := s.authPlayer(r, req.PlayerID)
	if !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	if _, ok := reportReasons[req.Reason]; !ok {
		jsonErr(w, "invalid reason", 400)
		return
	}
	ctx := r.Context()
	if n, _ := s.Q.CountReportsByReporterRecent(ctx, reporter.ID); n >= 10 {
		jsonErr(w, "Zu viele Meldungen – bitte später erneut versuchen.", 429)
		return
	}
	var msg dbgen.ChatMessage
	var msgID *int64
	if req.MessageID > 0 {
		m, err := s.Q.GetChatMessage(ctx, req.MessageID)
		if err != nil || m.SessionID != req.SessionID {
			jsonErr(w, "message not found", 404)
			return
		}
		msg = m
		msgID = &m.ID
		req.TargetID = m.PlayerID
	}
	if req.TargetID == "" || req.TargetID == reporter.ID {
		jsonErr(w, "invalid target", 400)
		return
	}
	target, err := s.Q.GetPlayerByID(ctx, req.TargetID)
	if err != nil {
		jsonErr(w, "player not found", 404)
		return
	}
	if len(req.Note) > 300 {
		req.Note = req.Note[:300]
	}
	dup, _ := s.Q.ReporterAlreadyReported(ctx, dbgen.ReporterAlreadyReportedParams{ReporterID: reporter.ID, ReportedPlayerID: target.ID, SessionID: req.SessionID})

	actions := []string{}
	// 1. hide the message for everyone, immediately
	if msgID != nil && msg.Hidden == 0 {
		s.Q.HideChatMessage(ctx, dbgen.HideChatMessageParams{Flag: "reported:" + req.Reason, ID: *msgID})
		s.broadcast(req.SessionID, map[string]any{"type": "chat_hidden", "id": *msgID})
		actions = append(actions, "hidden")
	}
	// 2. the reporter never sees the target again
	s.Q.AddBlock(ctx, dbgen.AddBlockParams{PlayerID: reporter.ID, BlockedID: target.ID})
	actions = append(actions, "blocked")

	// 3. escalation — one report from one person is not proof; several
	//    independent reporters in 24h are. Severe categories act faster.
	if dup == 0 {
		severe := req.Reason == "grooming" || req.Reason == "sexual"
		strikes := int64(1)
		if severe {
			strikes = 2
		}
		if c := s.applyStrikes(ctx, target.ID, strikes, "report:"+req.Reason); c != "" {
			actions = append(actions, "strike→"+c)
		} else {
			actions = append(actions, "strike")
		}
		reporters, _ := s.Q.CountDistinctReportersForPlayer(ctx, target.ID)
		reporters++ // include this one (row not inserted yet)
		switch {
		case severe && reporters >= 2, reporters >= 3:
			s.muteFor(ctx, target.ID, 24*time.Hour, fmt.Sprintf("%d independent reports (%s)", reporters, req.Reason))
			s.Q.HidePlayerChatInSession(ctx, dbgen.HidePlayerChatInSessionParams{Flag: "mass_report", SessionID: req.SessionID, PlayerID: target.ID})
			s.broadcast(req.SessionID, map[string]any{"type": "chat_refresh"})
			actions = append(actions, "mute24h")
		case severe:
			s.muteFor(ctx, target.ID, time.Hour, "severe report ("+req.Reason+")")
			actions = append(actions, "mute1h")
		}
	}
	rep, _ := s.Q.CreateReport(ctx, dbgen.CreateReportParams{
		SessionID: req.SessionID, MessageID: msgID, ReportedPlayerID: target.ID,
		ReporterID: reporter.ID, Reason: req.Reason, Note: req.Note, Action: strings.Join(actions, ","),
	})
	detail := fmt.Sprintf("Meldung #%d – Grund: %s (%s)\nMelder: %s\nNotiz: %s\nAutomatische Maßnahmen: %s",
		rep.ID, reportReasons[req.Reason], req.Reason, reporter.Name, req.Note, strings.Join(actions, ", "))
	if msgID != nil {
		detail += "\nGemeldete Nachricht: " + msg.Message
	}
	s.notifySafety("report_"+req.Reason, req.SessionID, target, detail)

	jsonResp(w, map[string]any{"ok": true, "actions": actions})
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
		TargetID string `json:"target_id"`
		Unblock  bool   `json:"unblock"`
	}
	if err := readJSON(r, &req); err != nil || req.TargetID == "" || req.TargetID == req.PlayerID {
		jsonErr(w, "invalid request", 400)
		return
	}
	if _, ok := s.authPlayer(r, req.PlayerID); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	if req.Unblock {
		s.Q.RemoveBlock(r.Context(), dbgen.RemoveBlockParams{PlayerID: req.PlayerID, BlockedID: req.TargetID})
	} else {
		s.Q.AddBlock(r.Context(), dbgen.AddBlockParams{PlayerID: req.PlayerID, BlockedID: req.TargetID})
	}
	jsonResp(w, map[string]any{"ok": true})
}

func (s *Server) handleListBlocks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authPlayer(r, r.PathValue("id")); !ok {
		jsonErr(w, "unauthorized", 401)
		return
	}
	rows, err := s.Q.ListBlocks(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, rows)
}

// ---- Player ----

func (s *Server) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	player, err := s.Q.GetPlayerByID(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "Player not found", 404)
		return
	}
	var treasuresFound int64
	s.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM treasures WHERE found_by = ?", player.ID).Scan(&treasuresFound)
	jsonResp(w, struct {
		dbgen.Player
		TreasuresFound int64 `json:"treasures_found"`
	}{player, treasuresFound})
}

func (s *Server) handleGetPlayerSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.Q.GetPlayerSessions(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, sessions)
}

// ---- umfeld-at context proxy (cached) ----

// waterParcelsComplete reads meta.water_parcels.complete from an
// /osm/geometry?cat=water_parcels response (true when the block is absent).
func waterParcelsComplete(body []byte) bool {
	var d struct {
		Meta struct {
			WaterParcels *struct {
				Complete *bool `json:"complete"`
			} `json:"water_parcels"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &d); err != nil || d.Meta.WaterParcels == nil || d.Meta.WaterParcels.Complete == nil {
		return true
	}
	return *d.Meta.WaterParcels.Complete
}

// umfeldPublicPrefixes are the only umfeld-at paths the generic proxy
// forwards — point/bbox/code keyed context. Parcel data lives in our cached
// cadastre cells (/api/viewport), never behind this proxy.
var umfeldPublicPrefixes = []string{
	"/lookup", "/search/municipalities", "/search/address_osm", "/search/protected_area",
	"/toponyms/", "/kg/", "/natura2000/point", "/natura2000/site/", "/natura2000/search", "/natura2000/stats",
	"/osm/point", "/osm/stats", "/legal/", "/land_prices/point", "/land_prices/predict", "/land_prices/gemeinde/",
	"/land_prices/bezirk/", "/land_prices/bundesland/", "/context",
}

func (s *Server) handleUmfeldProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/umfeld")
	allowed := false
	for _, p := range umfeldPublicPrefixes {
		if strings.HasPrefix(path, p) {
			allowed = true
			break
		}
	}
	if !allowed {
		jsonErr(w, "not an umfeld context path; parcels come from GET /api/viewport, context from /api/parcel-context, /api/osm-lines, /api/n2k, /api/municipality", 404)
		return
	}
	// /kg/{kg}/toponyms is public; a bare /kg/{kg} is not an umfeld route.
	if strings.HasPrefix(path, "/kg/") && !strings.HasSuffix(path, "/toponyms") {
		jsonErr(w, "not found", 404)
		return
	}
	query := r.URL.RawQuery
	cacheKey := "umfeld:" + path + "?" + query
	s.cachedFetch(w, cacheKey, func() ([]byte, int) {
		u := umfeldAPI + path
		if query != "" {
			u += "?" + query
		}
		code, hdr, body, err := upstreamGetWait(u, 15*time.Second, 20<<20)
		if err != nil {
			return jsonErrBody("data service error"), 502
		}
		if code == 200 {
			ttl := 24 * time.Hour // addresses included: Nominatim upstream is 1 req/s, answers are stable
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(body), ExpiresAt: time.Now().Add(ttl)})
			return body, 200
		}
		if code == http.StatusAccepted {
			return parsePending(hdr, body).body(), http.StatusAccepted
		}
		if hdr.Get("X-Upstream") == "down" {
			return body, 503
		}
		return jsonErrBody("upstream error"), code
	})
}

// compactGeoJSON rounds coordinates to 6 decimal places and minifies JSON.
func compactGeoJSON(data []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	roundCoords(obj)
	return json.Marshal(obj)
}

func roundCoords(v any) {
	switch val := v.(type) {
	case map[string]any:
		if coords, ok := val["coordinates"]; ok {
			val["coordinates"] = roundCoordValue(coords)
		}
		for _, child := range val {
			roundCoords(child)
		}
	case []any:
		for _, child := range val {
			roundCoords(child)
		}
	}
}

func roundCoordValue(v any) any {
	switch val := v.(type) {
	case float64:
		return math.Round(val*1e6) / 1e6
	case []any:
		out := make([]any, len(val))
		for i, c := range val {
			out[i] = roundCoordValue(c)
		}
		return out
	}
	return v
}

// ---- SSE (Server-Sent Events) ----

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonErr(w, "SSE not supported", 500)
		return
	}

	ch := make(chan string, 16)
	s.sseMu.Lock()
	if s.sseClients[sessionID] == nil {
		s.sseClients[sessionID] = make(map[chan string]bool)
	}
	s.sseClients[sessionID][ch] = true
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients[sessionID], ch)
		s.sseMu.Unlock()
		close(ch)
	}()

	// Send initial ping
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) broadcast(sessionID string, data map[string]any) {
	msg, _ := json.Marshal(data)
	s.sseMu.RLock()
	defer s.sseMu.RUnlock()
	for ch := range s.sseClients[sessionID] {
		select {
		case ch <- string(msg):
		default:
		}
	}
}

// ---- Game Logic Helpers ----

// nsPricePerSqm returns the base price in coins/m² for a BEV Nutzungssymbol (NS)
// code. Source: BEV Schnittstellenbeschreibung "Katastralmappe SHP" V2.9, Tab. 8.
// The upstream cadastre API corrected its German NS labels in Aug 2026 (codes
// unchanged); notably 48 is farmland (Äcker/Wiesen/Weiden — the most common code
// in Austria), NOT Verkehrsfläche, and 83 is Gebäudenebenfläche, not Fels.
// MUST stay in sync with NS_TABLE in srv/static/game.js.
var nsBasePrice = map[string]float64{
	"40": 0.30, // Dauerkulturanlagen oder Erwerbsgärten
	"41": 0.50, // Gebäude
	"42": 0.25, // Parkplätze
	"48": 0.30, // Äcker, Wiesen oder Weiden
	"52": 0.45, // Gärten
	"53": 0.35, // Weingärten
	"54": 0.12, // Alpen
	"55": 0.10, // Krummholzflächen
	"56": 0.20, // Wälder
	"57": 0.15, // Verbuschte Flächen
	"58": 0.10, // Forststraßen
	"59": 0.05, // Fließende Gewässer
	"60": 0.05, // Stehende Gewässer
	"61": 0.08, // Feuchtgebiete
	"62": 0.05, // Vegetationsarme Flächen
	"63": 0.40, // Betriebsflächen
	"64": 0.08, // Gewässerrandflächen
	"65": 0.10, // Verkehrsrandflächen
	"72": 0.20, // Friedhöfe
	"83": 0.45, // Gebäudenebenflächen
	"84": 0.15, // Abbauflächen, Halden, Deponien
	"87": 0.03, // Fels- und Geröllflächen
	"88": 0.03, // Gletscher
	"92": 0.15, // Schienenverkehrsanlagen
	"95": 0.10, // Straßenverkehrsanlagen
	"96": 0.35, // Freizeitflächen
}

func calculatePrice(areaSqm float64, landuse string, buildingCount int, totalBuildingArea float64) int {
	pricePerSqm, ok := nsBasePrice[landuse]
	if !ok {
		pricePerSqm = 0.15
	}

	// Density multiplier: built-up ratio drives price
	// Urban dense (>0.3) = 2x, suburban (0.05-0.3) = 1-2x, rural (<0.01) = 0.5x
	densityMult := 1.0
	if areaSqm > 0 && totalBuildingArea > 0 {
		builtRatio := totalBuildingArea / areaSqm
		if builtRatio > 0.3 {
			densityMult = 2.0
		} else if builtRatio > 0.05 {
			densityMult = 1.0 + (builtRatio-0.05)/0.25
		} else {
			densityMult = 0.5 + builtRatio/0.05*0.5
		}
	} else if buildingCount == 0 {
		densityMult = 0.5 // no buildings = cheap rural land
	}

	price := int(math.Round(areaSqm * pricePerSqm * densityMult)) // same rounding as calcPrice() in game.js
	if price < 10 {
		price = 10
	}
	if price > 5000 {
		price = 5000
	}
	return price
}

// European Red List species found in Austria — used as treasure encounters
var redListSpecies = []struct {
	Name, German, Category, Group string
	Value                         int64
}{
	{"Lynx lynx", "Eurasischer Luchs", "LC", "mammal", 300},
	{"Barbastella barbastellus", "Mopsfledermaus", "VU", "mammal", 400},
	{"Cricetus cricetus", "Feldhamster", "LC", "mammal", 250},
	{"Bison bonasus", "Wisent", "VU", "mammal", 500},
	{"Aquila chrysaetos", "Steinadler", "LC", "bird", 350},
	{"Bubo bubo", "Uhu", "LC", "bird", 300},
	{"Ciconia nigra", "Schwarzstorch", "LC", "bird", 350},
	{"Otis tarda", "Großtrappe", "LC", "bird", 400},
	{"Tetrao urogallus", "Auerhahn", "LC", "bird", 350},
	{"Coenonympha hero", "Wald-Wiesenvögelchen", "VU", "butterfly", 200},
	{"Colias chrysotheme", "Goldene Acht", "VU", "butterfly", 200},
	{"Parnassius apollo", "Apollofalter", "NT", "butterfly", 250},
	{"Bombina bombina", "Rotbauchunke", "LC", "amphibian", 200},
	{"Vipera ursinii", "Wiesenotter", "VU", "reptile", 350},
	{"Triturus dobrogicus", "Donau-Kammmolch", "NT", "amphibian", 250},
	{"Coenagrion ornatum", "Vogel-Azurjungfer", "NT", "dragonfly", 200},
	{"Cordulegaster heros", "Große Quelljungfer", "NT", "dragonfly", 200},
	{"Hucho hucho", "Huchen", "EN", "fish", 500},
	{"Acipenser ruthenus", "Sterlet", "VU", "fish", 400},
	{"Gulo gulo", "Vielfraß", "VU", "mammal", 450},
}

// backfillChallenges adds quests introduced after a session was created (only
// for players who already have quests there, i.e. are members).
func (s *Server) backfillChallenges(ctx context.Context, sessionID, playerID string) {
	var n int64
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM challenges WHERE session_id=? AND player_id=?", sessionID, playerID).Scan(&n)
	if n == 0 {
		return
	}
	add := []struct {
		cType, title, desc string
		coins, xp          int64
	}{
		{"species", "Artenforscher", "Entdecke eine seltene Art der Roten Liste", 250, 150},
		{"tree", "Baumriese", "Kaufe eine Parzelle mit einem Riesenbaum", 400, 300},
		{"timber", "Holzknecht", "Schlägere 2 Waldparzellen", 250, 100},
		{"restore", "Waldhüter", "Stelle einen Wald außer Nutzung (Naturwald)", 350, 200},
		{"observe", "Spurenleser", "Kaufe eine Parzelle, bei der die Beobachtung vom Kataster abweicht", 300, 150},
	}
	for _, c := range add {
		var have int64
		s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM challenges WHERE session_id=? AND player_id=? AND title=?", sessionID, playerID, c.title).Scan(&have)
		if have > 0 {
			continue
		}
		desc := c.desc
		s.Q.CreateChallenge(ctx, dbgen.CreateChallengeParams{SessionID: sessionID, PlayerID: playerID, ChallengeType: c.cType,
			Title: c.title, Description: &desc, RewardCoins: c.coins, RewardXp: c.xp})
	}
}

func (s *Server) generateChallenges(ctx context.Context, sessionID, playerID string, lon, lat float64) {
	challenges := []struct {
		cType, title, desc string
		coins, xp          int64
	}{
		// Canonical German — translated client-side via the i18n exact dictionary.
		{"explore", "Erkunde deine Gemeinde", "Kaufe deine erste Parzelle", 100, 50},
		{"treasure", "Schatzsucher", "Finde einen versteckten Schatz", 150, 75},
		{"restore", "Naturschützer", "Wandle eine Parzelle in ein Naturschutzgebiet um", 200, 100},
		{"species", "Artenforscher", "Entdecke eine seltene Art der Roten Liste", 250, 150},
		{"explore", "Landvermesser", "Kaufe 5 Parzellen", 300, 150},
		{"restore", "Waldmeister", "Wandle 3 Parzellen in Wald oder Naturschutz um", 500, 250},
		{"harvest", "Erntedank", "Ernte 3 reife Äcker", 300, 150},
		{"timber", "Holzknecht", "Schlägere 2 Waldparzellen", 250, 100},
		{"restore", "Waldhüter", "Stelle einen Wald außer Nutzung (Naturwald)", 350, 200},
		// Only achievable in lidar-enhanced KGs; the client hides it until one is loaded.
		{"tree", "Baumriese", "Kaufe eine Parzelle mit einem Riesenbaum", 400, 300},
		// Only achievable in NE-observed KGs (srtm v2.4); the client hides it until one is loaded.
		{"observe", "Spurenleser", "Kaufe eine Parzelle, bei der die Beobachtung vom Kataster abweicht", 300, 150},
	}

	for _, c := range challenges {
		desc := c.desc
		s.Q.CreateChallenge(ctx, dbgen.CreateChallengeParams{
			SessionID:     sessionID,
			PlayerID:      playerID,
			ChallengeType: c.cType,
			Title:         c.title,
			Description:   &desc,
			RewardCoins:   c.coins,
			RewardXp:      c.xp,
		})
	}
}

// ---- LiDAR (srtm-lidar) proxy & enhanced mode ----

// handleLidarProxy forwards GET requests to the srtm-lidar API with 1h caching.
// Only fast endpoints should be requested (query, flags) — never overlay/elevation.
// srtmPublicPrefixes: the public srtm tier (bbox / point / KG-code keyed).
var srtmPublicPrefixes = []string{"/landscape", "/context", "/trees/bbox", "/landmarks/bbox", "/buildings/bbox", "/heightfield", "/kgs", "/kg/", "/status", "/license"}

func (s *Server) handleLidarProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/lidar")
	ok := false
	for _, p := range srtmPublicPrefixes {
		if strings.HasPrefix(path, p) {
			ok = true
			break
		}
	}
	if !ok {
		jsonErr(w, "not found", 404)
		return
	}
	query := r.URL.RawQuery
	cacheKey := "lidar2:" + path + "?" + query
	s.cachedFetch(w, cacheKey, func() ([]byte, int) {
		u := lidarAPI + path
		if query != "" {
			u += "?" + query
		}
		code, hdr, body, err := upstreamGetWait(u, 15*time.Second, 20<<20)
		if err != nil {
			return jsonErrBody("data service error"), 502
		}
		if code == 200 {
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(body), ExpiresAt: time.Now().Add(6 * time.Hour)})
			return body, 200
		}
		if code == http.StatusAccepted {
			return parsePending(hdr, body).body(), http.StatusAccepted
		}
		if hdr.Get("X-Upstream") == "down" {
			return body, 503
		}
		return jsonErrBody("upstream error"), code
	})
}

// imperviousCover are srtm land-cover classes that are frequently the *reported*
// dominant type on a parcel even when they cover only a sliver (a driveway edge,
// a shed roof) — using them as the ground-fill color paints whole meadows grey.
// For terrain fill we skip these and fall back to the next-largest natural cover.
var imperviousCover = map[string]bool{
	"road": true, "roof": true, "parking": true, "path": true,
}

// correctedDomTerrain picks the land-cover type to use for a parcel's ground
// fill: the highest-area entry in area_summary that is NOT impervious
// (road/roof/parking/path). If the parcel is genuinely all-impervious (a real
// building lot or a road parcel) nothing natural remains, so we return "" and the
// client falls back to cadastre landuse. Buildings themselves are drawn as
// footprints on top, so roofs still render — just not as the terrain backdrop.
// landmarkClass maps a srtm TYPE_LETTER to a landmark type plus the plausible
// height window [minH, maxH] that makes it worth showing as a map landmark.
// Only man-made verticals qualify; "" means not a landmark.
func landmarkClass(letter string) (typ string, minH, maxH float64) {
	switch letter {
	case "R":
		return "roof", 18, 60 // church towers, silos, high-rises
	case "M":
		return "mast", 15, 120
	case "T":
		return "wind_turbine", 40, 200
	case "b":
		return "bridge", 20, 120
	}
	return "", 0, 0
}

// landmarkLetter maps a long srtm object_type back to its TYPE_LETTER.
func landmarkLetter(typ string) string {
	switch typ {
	case "roof":
		return "R"
	case "mast":
		return "M"
	case "wind_turbine":
		return "T"
	case "bridge":
		return "b"
	}
	return ""
}

// isV2Product: the srtm index reports "v2", "v2.1", "v2.2", "v2.3", ... while
// the KG JSON carries "2.1"/"2.3". Anything ≥ 2 counts as the v2 product line.
// isV24Product: srtm product v2.4 = NE cells published (observed layer).
// "v2.4-partial" counts too — cells exist for the processed part.
// isV24Product: NE cells are published for this KG. "v2.4-partial" means the
// fleet is still processing — srtm /cells answers 404 not_processed for it, so
// adopting/warming it would only burn cycles; it flips when the suffix drops.
func isV24Product(pv string) bool {
	return (strings.HasPrefix(pv, "v2.4") || strings.HasPrefix(pv, "2.4")) && !strings.Contains(pv, "partial")
}

func isV2Product(pv string) bool {
	return strings.HasPrefix(pv, "v2") || strings.HasPrefix(pv, "2.")
}

// robustHeight returns srtm v2.3's height_robust_m (= min(hmax, p90+10), the
// metric upstream now ranks on) when present, else the legacy field.
func robustHeight(td map[string]any, legacy string) float64 {
	if h, ok := td["height_robust_m"].(float64); ok && h > 0 {
		return h
	}
	h, _ := td[legacy].(float64)
	return h
}

// badQualityFlags: v2.3 top lists carry inline quality_flags; anything
// implausible/critical or a low-confidence classification is dropped.
func badQualityFlags(td map[string]any) bool {
	qf, ok := td["quality_flags"].([]any)
	if !ok {
		return false
	}
	for _, f := range qf {
		fs, _ := f.(string)
		if strings.Contains(fs, "implausible") || strings.Contains(fs, "critical") ||
			strings.Contains(fs, "low_rf") || strings.Contains(fs, "spike") {
			return true
		}
	}
	return false
}

func correctedDomTerrain(pd map[string]any) any {
	as, ok := pd["area_summary"].(map[string]any)
	if !ok || len(as) == 0 {
		// No breakdown — only trust the raw dominant if it isn't impervious.
		if dt, ok := pd["dominant_type"].(string); ok && !imperviousCover[dt] {
			return dt
		}
		return ""
	}
	bestType := ""
	bestArea := -1.0
	for t, v := range as {
		if imperviousCover[t] {
			continue
		}
		vm, _ := v.(map[string]any)
		area := 0.0
		if vm != nil {
			if a, ok := vm["area_sqm"].(float64); ok {
				area = a
			} else if f, ok := vm["fraction"].(float64); ok {
				area = f
			}
		}
		if area > bestArea {
			bestArea = area
			bestType = t
		}
	}
	return bestType
}

// handleLidarKG fetches the full ~4-7MB KG JSON from the lidar API, strips it
// down to what the game needs (per-parcel terrain, building heights, flag-filtered
// top trees/objects), and caches the slim result for 6h.
func (s *Server) handleLidarKG(w http.ResponseWriter, r *http.Request) {
	kg := r.PathValue("code")
	if !validKG(kg) {
		jsonErr(w, "invalid kg code", 400)
		return
	}
	cacheKey := "lidar-slim2:/kg/" + kg
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}
	// buildLidarSlim is itself singleflight-wrapped, so concurrent requests
	// for the same KG share one upstream fetch.
	out, status := s.buildLidarSlim(r.Context(), kg)
	if status == http.StatusAccepted {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterOf(out)))
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		w.Write(out)
		return
	}
	if status != 200 {
		jsonErr(w, string(out), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(out)
}

// compactFracs extracts the parcel's land-cover fraction vector from srtm's
// area_summary: {type: fraction}, rounded to 2 decimals, entries <2% dropped.
func compactFracs(pd map[string]any) map[string]float64 {
	as, ok := pd["area_summary"].(map[string]any)
	if !ok || len(as) == 0 {
		return nil
	}
	out := map[string]float64{}
	for t, v := range as {
		vm, _ := v.(map[string]any)
		if vm == nil {
			continue
		}
		f, _ := vm["fraction"].(float64)
		if f < 0.02 {
			continue
		}
		out[t] = math.Round(f*100) / 100
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildLidarSlim fetches + slims the KG JSON and stores it in the cache.
// Returns (payload, 200) or (errMsg, status). Wrapped in singleflight so a
// handler request and concurrent background warms (handleSimilarParcels)
// for the same KG share one upstream fetch of the ~4-7MB source JSON.
func (s *Server) buildLidarSlim(ctx context.Context, kg string) ([]byte, int) {
	type sfRes struct {
		body   []byte
		status int
	}
	v, _, _ := s.sf.Do("lidar-slim2:/kg/"+kg, func() (any, error) {
		// A parallel flight may have cached it while we queued.
		if cached, err := s.Q.GetCachedData(context.Background(), "lidar-slim2:/kg/"+kg); err == nil {
			return sfRes{[]byte(cached), 200}, nil
		}
		b, st := s.buildLidarSlimUncached(kg)
		return sfRes{b, st}, nil
	})
	res := v.(sfRes)
	return res.body, res.status
}

// handleEnhancedKGs returns the list of lidar-processed KGs (the "enhanced" set).
// Cached 15 minutes — the lidar service processes more KGs continuously.
// invalidateRegeneratedKGs makes the v1 → v2 (and any future) product upgrade
// transparent: srtm re-generates a KG in place under the same code, so our
// 6h lidar-slim and 1h similar caches would keep serving the previous product.
// We persist the index's generated_at per KG and, on every registry refresh
// (15 min, FAST index query — no extra upstream cost), purge our derived
// caches for KGs whose timestamp changed. The next client request rebuilds
// them from the new product. The first run (no snapshot) only records.
func (s *Server) invalidateRegeneratedKGs(gen map[string]string) {
	const snapKey = "enhanced-gen:v1"
	ctx := context.Background()
	prev := map[string]string{}
	hadSnap := false
	if cached, err := s.Q.GetCachedData(ctx, snapKey); err == nil {
		hadSnap = json.Unmarshal([]byte(cached), &prev) == nil
	}
	if hadSnap {
		purged := 0
		for kg, g := range gen {
			if pg, ok := prev[kg]; !ok || pg == g {
				continue
			}
			for _, pat := range []string{
				"lidar-slim2:/kg/" + kg,
				"lidar:/kg/" + kg + "%",
				"similar:v3:" + kg + "-%",
			} {
				s.Q.DeleteCacheLike(ctx, pat)
			}
			purged++
		}
		if purged > 0 {
			slog.Info("lidar registry: purged derived caches for regenerated KGs", "kgs", purged)
		}
	}
	b, _ := json.Marshal(gen)
	s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{
		CacheKey: snapKey, Data: string(b), ExpiresAt: time.Now().Add(10 * 365 * 24 * time.Hour),
	})
}

// generateN2KTreasures places extra high-value rare-species treasures inside
// Natura-2000 sites overlapping the session's Gemeinde (EEA site polygons via
// umfeld); points are sampled inside the site ∩ Gemeinde bbox and snapped
// to a cached parcel point when one is there. Runs in a goroutine at session create; broadcasts treasures_updated.
func (s *Server) generateN2KTreasures(ctx context.Context, sessionID, muniCode string) {
	g := admin().Gemeinde[muniCode]
	if g == nil {
		return
	}
	hash := uint64(0)
	for _, c := range sessionID {
		hash = hash*31 + uint64(c)
	}
	rng := mrand.New(mrand.NewPCG(hash, hash^0x9E3779B97F4A7C15))
	placed := 0
	for _, st := range s.n2kSites() {
		if placed >= 5 {
			break
		}
		if st.BBox.MaxLon < g.MinLon || st.BBox.MinLon > g.MaxLon || st.BBox.MaxLat < g.MinLat || st.BBox.MinLat > g.MaxLat {
			continue
		}
		body, code := s.llmGet("n2k:site:v1:"+st.Code, umfeldAPI+"/natura2000/site/"+url.PathEscape(st.Code)+"?geometry=1", 7*24*time.Hour)
		if code != 200 {
			continue
		}
		var d struct {
			Data struct {
				Geometry json.RawMessage `json:"geometry"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &d) != nil {
			continue
		}
		rings := geomRings(d.Data.Geometry)
		if len(rings) == 0 {
			continue
		}
		// sample inside the site ∩ Gemeinde bbox
		w, so := math.Max(st.BBox.MinLon, g.MinLon), math.Max(st.BBox.MinLat, g.MinLat)
		e, n := math.Min(st.BBox.MaxLon, g.MaxLon), math.Min(st.BBox.MaxLat, g.MaxLat)
		if e <= w || n <= so {
			continue
		}
		perSite := 0
		for tries := 0; tries < 400 && perSite < 3 && placed < 5; tries++ {
			lon := w + rng.Float64()*(e-w)
			lat := so + rng.Float64()*(n-so)
			if !pipRingsGo(lon, lat, rings) {
				continue
			}
			// snap to a small cached parcel's point when we hold the cell
			if cd := s.cachedCell(cellOf(lon, lat)); cd != nil {
				snapped := false
				for i := range cd.Parcels {
					p := &cd.Parcels[i]
					if p.AreaSqm > 30000 || p.BuildingCnt > 0 || math.Abs(p.Lon-lon) > 0.002 || math.Abs(p.Lat-lat) > 0.0015 {
						continue
					}
					if pipRingsGo(lon, lat, geomRings(p.Geometry)) {
						lon, lat = p.Lon, p.Lat
						snapped = true
						break
					}
				}
				_ = snapped
			}
			si := int((hash + uint64(placed)*31) % uint64(len(redListSpecies)))
			sp2 := redListSpecies[si]
			if err := s.Q.CreateTreasure(ctx, dbgen.CreateTreasureParams{
				SessionID: sessionID, Lon: lon, Lat: lat, TreasureType: "n2k_species", Value: sp2.Value * 2,
				SpeciesName: sp2.Name, SpeciesGerman: sp2.German, SpeciesCategory: sp2.Category,
			}); err == nil {
				placed++
				perSite++
			}
		}
	}
	if placed > 0 {
		slog.Info("placed N2K treasures", "session", sessionID, "count", placed)
		s.broadcast(sessionID, map[string]any{"type": "treasures_updated"})
	}
}

// ---- Building info (tap on a building footprint) ----
// GET /api/building-info?fp={footprint_id}&lon=&lat=
// From the cached cell: the footprint's shape metrics and every parcel it
// touches (a footprint id is unique per tile piece; a building spanning
// parcels shows up as several footprints sharing the centroid). Address
// points are not part of the data tiers. Cached 24 h per footprint.
func (s *Server) handleBuildingInfo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fp := q.Get("fp")
	lon, _ := strconv.ParseFloat(q.Get("lon"), 64)
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	if fp == "" || lon == 0 || lat == 0 {
		jsonErr(w, "fp, lon, lat required", 400)
		return
	}
	cacheKey := "bldg-info:v2:" + fp
	s.cachedFetch(w, cacheKey, func() ([]byte, int) {
		cd := s.cachedCell(cellOf(lon, lat))
		if cd == nil {
			return jsonErrBody("cell not loaded"), 404
		}
		var me *bevFootprint
		for i := range cd.Footprints {
			if cd.Footprints[i].FootprintID == fp {
				me = &cd.Footprints[i]
				break
			}
		}
		if me == nil {
			return jsonErrBody("footprint not found"), 404
		}
		// Parcels under this building: footprints with (nearly) the same
		// centroid belong to the same structure split along parcel lines.
		pids := map[string]bool{}
		for _, f := range cd.Footprints {
			if f.ParcelID != "" && distM(me.Lon, me.Lat, f.Lon, f.Lat) < 12 && f.NSCode == me.NSCode {
				pids[f.ParcelID] = true
			}
		}
		var parcels []map[string]any
		for _, p := range cd.Parcels {
			if pids[p.ParcelID] {
				parcels = append(parcels, map[string]any{"parcel_id": p.ParcelID, "gnr": p.GNR, "ez": p.EZ, "area_sqm": p.AreaSqm, "dominant_ns": p.DominantNS})
			}
		}
		out := map[string]any{
			"footprint_id": fp, "parcel_id": me.ParcelID, "ns_code": me.NSCode, "area_sqm": me.AreaSqm,
			"obb_length_m": me.OBBLen, "obb_width_m": me.OBBWid, "orientation_deg": me.Orient,
			"parcels": nonNil(parcels), "addresses": []string{}, "notice": bevNotice,
		}
		enc, _ := json.Marshal(out)
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(enc), ExpiresAt: time.Now().Add(time.Duration(cadastreTTL))})
		return enc, 200
	})
}

// ---- KG summary card ----
// GET /api/kg-summary/{code}: register names (embedded VGD), srtm KG dossier
// (terrain, land cover, buildings), Natura-2000 sites overlapping the KG,
// RIS legal refs (umfeld), the observed layer aggregated over the cached NE
// cells (`ne`, neKGSummary) and what we hold of the KG in cached cadastre
// cells (parcel count / area). Cached 1 h.
func (s *Server) handleKGSummary(w http.ResponseWriter, r *http.Request) {
	kg := r.PathValue("code")
	if !validKG(kg) {
		jsonErr(w, "invalid kg code", 400)
		return
	}
	cacheKey := "kg-summary:v3:" + kg
	s.cachedFetch(w, cacheKey, func() ([]byte, int) {
		out := map[string]any{"kg_code": kg}
		a := admin().KGs[kg]
		if a == nil {
			return jsonErrBody("unknown KG code"), 404
		}
		out["kg_name"], out["gemeinde_name"], out["gemeinde_code"], out["state"] = a.Name, a.GemName, a.Gemeinde, a.State
		out["area_sqkm"] = a.AreaKm2
		var wg sync.WaitGroup
		var mu sync.Mutex
		set := func(k string, v any) { mu.Lock(); out[k] = v; mu.Unlock() }
		wg.Add(5)
		go func() { // srtm dossier
			defer wg.Done()
			body, st := s.llmGet("srtmkg:"+kg, lidarAPI+"/kg/"+kg, 6*time.Hour)
			if st != 200 {
				return
			}
			var d struct {
				Terrain   map[string]any `json:"terrain"`
				Landcover struct {
					Fractions map[string]float64 `json:"fractions"`
				} `json:"landcover"`
				Built struct {
					BuildingCount any `json:"building_count"`
				} `json:"built"`
				Vegetation struct {
					TreeMaxH any `json:"tree_max_height_m"`
				} `json:"vegetation"`
			}
			if json.Unmarshal(body, &d) != nil {
				return
			}
			if t := d.Terrain; t != nil {
				set("elev_min", t["elevation_min_m"])
				set("elev_max", t["elevation_max_m"])
				set("terrain_class", t["terrain_class"])
			}
			if d.Built.BuildingCount != nil {
				set("buildings", d.Built.BuildingCount)
			}
			if len(d.Landcover.Fractions) > 0 {
				set("landcover", d.Landcover.Fractions)
			}
			set("enhanced", true)
		}()
		go func() { // N2K sites by bbox
			defer wg.Done()
			var names []string
			for _, st := range s.n2kSites() {
				if st.BBox.MaxLon < a.MinLon || st.BBox.MinLon > a.MaxLon || st.BBox.MaxLat < a.MinLat || st.BBox.MinLat > a.MaxLat {
					continue
				}
				names = append(names, st.Name)
			}
			if len(names) > 0 {
				set("n2k_sites", names)
			}
		}()
		go func() { // RIS
			defer wg.Done()
			body, st := s.llmGet("umfeld:/legal/kg/"+kg+"?", umfeldAPI+"/legal/kg/"+kg, 24*time.Hour)
			if st != 200 {
				return
			}
			var d struct {
				Data map[string]any `json:"data"`
			}
			if json.Unmarshal(body, &d) == nil && d.Data != nil {
				set("legal_refs", d.Data["total_refs"])
				set("legal_contexts", d.Data["legal_contexts"])
			}
		}()
		go func() { // observed layer (NE cells) aggregated over the KG
			defer wg.Done()
			if ne := s.neKGSummary(kg); ne != nil {
				set("ne", ne)
			}
		}()
		go func() { // what we hold in cells + giant trees from the slim
			defer wg.Done()
			n, bl := 0, 0
			var area float64
			lu := map[string]float64{}
			for _, c := range a.cells() {
				cd := s.cachedCell(c)
				if cd == nil {
					continue
				}
				for _, p := range cd.Parcels {
					if p.KG != kg || !p.Complete {
						continue
					}
					n++
					area += p.AreaSqm
					bl += p.BuildingCnt
					for code, m2 := range p.LanduseAreas {
						lu[code] += m2
					}
				}
			}
			if n > 0 {
				set("parcels", n)
				set("parcels_scope", "geladene Zellen")
				set("area_ha", math.Round(area/1e3)/10)
				set("avg_area_sqm", math.Round(area/float64(n)))
				if _, ok := out["buildings"]; !ok {
					set("buildings", bl)
				}
				type kv struct {
					c string
					v float64
				}
				var top []kv
				for c, v := range lu {
					top = append(top, kv{c, v})
				}
				sort.Slice(top, func(i, j int) bool { return top[i].v > top[j].v })
				if len(top) > 6 {
					top = top[:6]
				}
				var tl []map[string]any
				for _, t := range top {
					tl = append(tl, map[string]any{"code": t.c, "share": math.Round(t.v/area*1000) / 1000})
				}
				set("landuse", tl)
			}
			if cached, err := s.Q.GetCachedData(context.Background(), "lidar-slim2:/kg/"+kg); err == nil {
				var slim struct {
					TopTrees []map[string]any `json:"top_trees"`
				}
				if json.Unmarshal([]byte(cached), &slim) == nil && len(slim.TopTrees) > 0 {
					set("tallest_tree_m", slim.TopTrees[0]["height_m"])
					set("giant_trees", len(slim.TopTrees))
				}
			}
		}()
		wg.Wait()
		out["notice"] = bevNotice
		enc, _ := json.Marshal(out)
		s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: cacheKey, Data: string(enc), ExpiresAt: time.Now().Add(time.Hour)})
		return enc, 200
	})
}

type questCounters struct {
	claims, converted, treasures, species, tallTrees, harvests, timber, wildforest, neDiscrepant int64
}

func (s *Server) questProgress(ctx context.Context, sessionID, playerID string) (q questCounters) {
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM parcel_claims WHERE session_id=? AND player_id=?", sessionID, playerID).Scan(&q.claims)
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM parcel_claims WHERE session_id=? AND player_id=? AND converted_to IS NOT NULL AND converted_to<>''", sessionID, playerID).Scan(&q.converted)
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM parcel_claims WHERE session_id=? AND player_id=? AND tall_trees>0", sessionID, playerID).Scan(&q.tallTrees)
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM treasures WHERE session_id=? AND found_by=?", sessionID, playerID).Scan(&q.treasures)
	s.DB.QueryRowContext(ctx, "SELECT COALESCE(SUM(harvests),0) FROM parcel_claims WHERE session_id=? AND player_id=?", sessionID, playerID).Scan(&q.harvests)
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM treasures WHERE session_id=? AND found_by=? AND treasure_type IN ('species','n2k_species')", sessionID, playerID).Scan(&q.species)
	s.DB.QueryRowContext(ctx, "SELECT COALESCE(SUM(harvests),0) FROM parcel_claims WHERE session_id=? AND player_id=? AND landuse='56'", sessionID, playerID).Scan(&q.timber)
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM parcel_claims WHERE session_id=? AND player_id=? AND converted_to='wildforest'", sessionID, playerID).Scan(&q.wildforest)
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM parcel_claims WHERE session_id=? AND player_id=? AND ne_verdict NOT IN ('', 'consistent', 'unknown')", sessionID, playerID).Scan(&q.neDiscrepant)
	// Erntedank counts field harvests only
	q.harvests -= q.timber
	return
}

// questProgressFor returns (have, goal) for a built-in quest — the counter it
// tracks, clamped to the goal, so the client can draw a progress bar.
func questProgressFor(title string, q questCounters) (int64, int64) {
	var have, goal int64
	switch title {
	case "Artenforscher":
		have, goal = q.species, 1
	case "Baumriese":
		have, goal = q.tallTrees, 1
	case "Erkunde deine Gemeinde":
		have, goal = q.claims, 1
	case "Landvermesser":
		have, goal = q.claims, 5
	case "Naturschützer":
		have, goal = q.converted, 1
	case "Waldmeister":
		have, goal = q.converted, 3
	case "Schatzsucher":
		have, goal = q.treasures, 1
	case "Erntedank":
		have, goal = q.harvests, 3
	case "Holzknecht":
		have, goal = q.timber, 2
	case "Waldhüter":
		have, goal = q.wildforest, 1
	case "Spurenleser":
		have, goal = q.neDiscrepant, 1
	default:
		return 0, 1
	}
	if have > goal {
		have = goal
	}
	return have, goal
}

// questSatisfied maps the built-in quest titles (see generateChallenges) to their condition.
func questSatisfied(title string, q questCounters) bool {
	claims, converted, treasures := q.claims, q.converted, q.treasures
	switch title {
	case "Artenforscher":
		return q.species >= 1
	case "Baumriese":
		return q.tallTrees >= 1
	case "Erkunde deine Gemeinde":
		return claims >= 1
	case "Landvermesser":
		return claims >= 5
	case "Naturschützer":
		return converted >= 1
	case "Waldmeister":
		return converted >= 3
	case "Schatzsucher":
		return treasures >= 1
	case "Erntedank":
		return q.harvests >= 3
	case "Holzknecht":
		return q.timber >= 2
	case "Waldhüter":
		return q.wildforest >= 1
	case "Spurenleser":
		return q.neDiscrepant >= 1
	}
	return false
}

// autoCompleteChallenges completes every open quest whose condition the player
// now satisfies, awards the rewards and broadcasts. Called after claim/convert/
// treasure actions so the sidebar quest list keeps up without a manual click.
func (s *Server) autoCompleteChallenges(ctx context.Context, sessionID, playerID string) []map[string]any {
	prog := s.questProgress(ctx, sessionID, playerID)
	rows, err := s.DB.QueryContext(ctx, "SELECT id, title, reward_coins, reward_xp FROM challenges WHERE session_id=? AND player_id=? AND completed=0", sessionID, playerID)
	if err != nil {
		return nil
	}
	type ch struct {
		id        int64
		title     string
		coins, xp int64
	}
	var open []ch
	for rows.Next() {
		var c ch
		if rows.Scan(&c.id, &c.title, &c.coins, &c.xp) == nil {
			open = append(open, c)
		}
	}
	rows.Close()
	var done []map[string]any
	var name string
	for _, c := range open {
		if !questSatisfied(c.title, prog) {
			continue
		}
		s.Q.CompleteChallenge(ctx, c.id)
		s.Q.UpdatePlayerCoins(ctx, dbgen.UpdatePlayerCoinsParams{Coins: c.coins, ID: playerID})
		s.Q.UpdatePlayerXP(ctx, dbgen.UpdatePlayerXPParams{Xp: c.xp, ID: playerID})
		if name == "" {
			if p, err := s.Q.GetPlayerByID(ctx, playerID); err == nil {
				name = p.Name
			}
		}
		s.broadcast(sessionID, map[string]any{"type": "challenge_completed", "player": name, "title": c.title})
		done = append(done, map[string]any{"id": c.id, "title": c.title, "coins": c.coins, "xp": c.xp})
	}
	return done
}

// snapDowngrades reports whether moving the spawn from (lon0,lat0) to
// (lon1,lat1) leaves an enhanced KG for a non-enhanced one (admin bbox
// lookup; unknown registry = never a downgrade).
func (s *Server) snapDowngrades(lon0, lat0, lon1, lat1 float64) bool {
	enh := s.enhancedKGSet()
	if len(enh) == 0 {
		return false
	}
	k0, k1 := s.kgCodeAt(lon0, lat0), s.kgCodeAt(lon1, lat1)
	if k0 == "" || k1 == "" || k0 == k1 {
		return false
	}
	return enh[k0] && !enh[k1]
}

// kgCodeAt: the KG containing a point. Our cached viewport cell first
// (point-in-parcel over the cell's parcel polygons — no network; lucky
// spawns are warm by definition), then bevdirect /municipality (memoised
// 24 h like /api/municipality), finally the embedded register's bbox
// heuristic (smallest bbox — wrong near KG borders: Hafnerbach village lies
// in 19469 but the smaller 19624 bbox also covers it).
func (s *Server) kgCodeAt(lon, lat float64) string {
	if d := s.cachedCell(cellOf(lon, lat)); d != nil {
		for _, p := range d.Parcels {
			if p.Geometry != nil && pipRingsGo(lon, lat, geomRings(p.Geometry)) {
				return p.KG
			}
		}
	}
	key := fmt.Sprintf("muni:at:v1:%.4f,%.4f", math.Round(lon*2500)/2500, math.Round(lat*2500)/2500)
	var raw []byte
	if c, err := s.Q.GetCachedData(context.Background(), key); err == nil {
		raw = []byte(c)
	} else {
		// Session create sits on this path: bevdirect assembles the cell's
		// tiles for a cold point (5–10 s) — never wait for that, the
		// register's bbox heuristic below is good enough for the snap guard.
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		u := fmt.Sprintf("%s/municipality?lon=%.6f&lat=%.6f", bevAPI, lon, lat)
		if rq, err := http.NewRequestWithContext(ctx, "GET", u, nil); err == nil {
			if resp, err := upstreamClient.Do(rq); err == nil {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				resp.Body.Close()
				if resp.StatusCode == 200 {
					raw = body
					s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: key, Data: string(raw), ExpiresAt: time.Now().Add(time.Duration(cadastreTTL))})
				}
			} else {
				slog.Info("kgCodeAt: bevdirect /municipality skipped", "err", err)
			}
		}
	}
	if len(raw) > 0 {
		var d struct {
			KG struct {
				Code string `json:"kg_code"`
			} `json:"kg"`
		}
		if json.Unmarshal(raw, &d) == nil && d.KG.Code != "" {
			return d.KG.Code
		}
	}
	if k := admin().kgAt(lon, lat); k != nil {
		return k.KG
	}
	return ""
}

// settlementCenter resolves the main settlement of a municipality via the
// cadastre OSM address search (place nodes: city/town/village/hamlet). Returns
// ok=false when nothing plausible lies within 8 km of the given centroid, so a
// wrong geocode can never move a session to another valley. Budget 2.5 s.
func (s *Server) settlementCenter(name string, lon, lat float64) (float64, float64, bool) {
	if name == "" {
		return 0, 0, false
	}
	// Memoised per name (24 h): umfeld answers in ~200 ms, but session create
	// must stay instant for every later player of the same Gemeinde.
	var body []byte
	if c, err := s.Q.GetCachedData(context.Background(), "settle:v1:"+name); err == nil {
		body = []byte(c)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", umfeldAPI+"/search/address_osm?limit=8&q="+url.QueryEscape(name), nil)
		resp, err := upstreamClient.Do(req)
		if err != nil {
			return 0, 0, false
		}
		defer resp.Body.Close()
		body, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode == 200 && len(body) > 0 {
			s.Q.SetCachedData(context.Background(), dbgen.SetCachedDataParams{CacheKey: "settle:v1:" + name, Data: string(body), ExpiresAt: time.Now().Add(24 * time.Hour)})
		}
	}
	var d struct {
		Data []struct {
			Lon       float64 `json:"lon"`
			Lat       float64 `json:"lat"`
			PlaceType string  `json:"place_type"`
			OsmType   string  `json:"osm_type"`
			Class     string  `json:"class"`
			Address   struct {
				City    string `json:"city"`
				Town    string `json:"town"`
				Village string `json:"village"`
			} `json:"address"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &d) != nil {
		return 0, 0, false
	}
	rank := map[string]int{"city": 4, "town": 4, "village": 3, "hamlet": 2, "administrative": 1}
	best, bestR := -1, 0
	for i, it := range d.Data {
		r := rank[it.PlaceType]
		if r == 0 || it.Lon == 0 {
			continue
		}
		if distM(lon, lat, it.Lon, it.Lat) > 8000 {
			continue
		}
		if r > bestR {
			best, bestR = i, r
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return d.Data[best].Lon, d.Data[best].Lat, true
}
