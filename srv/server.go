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
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

const cadastreAPI = "https://cadastre-process-api.exe.xyz/api/v1"
const lidarAPI = "https://srtm-lidar-at.exe.xyz:8000/api/v1"

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	Q            *dbgen.Queries

	// SSE connections for real-time updates
	sseClients map[string]map[chan string]bool // session_id -> set of channels
	sseMu      sync.RWMutex
}

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
		sseClients:   make(map[string]map[chan string]bool),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	srv.Q = dbgen.New(srv.DB)
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

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()

	// SEO / meta
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /static/og-image.png", s.handleOGImage)

	// Static files and main page
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /join/{code}", s.handleIndex) // invite link
	mux.HandleFunc("GET /rejoin/{token}", s.handleRejoin)

	// Auth API
	mux.HandleFunc("POST /api/register", s.handleRegister)
	mux.HandleFunc("POST /api/login", s.handleLogin)

	// Game API
	mux.HandleFunc("POST /api/session/create", s.handleCreateSession)
	mux.HandleFunc("GET /api/invite/{code}", s.handleInvitePreview)
	mux.HandleFunc("POST /api/session/join", s.handleJoinSession)
	mux.HandleFunc("GET /api/session/{id}", s.handleGetSession)
	mux.HandleFunc("GET /api/session/{id}/players", s.handleGetSessionPlayers)
	mux.HandleFunc("GET /api/session/{id}/parcels", s.handleGetSessionParcels)
	mux.HandleFunc("GET /api/session/{id}/treasures", s.handleGetSessionTreasures)
	mux.HandleFunc("GET /api/session/{id}/challenges", s.handleGetChallenges)
	mux.HandleFunc("GET /api/session/{id}/biodiversity", s.handleGetBiodiversity)
	mux.HandleFunc("GET /api/session/{id}/chat", s.handleGetChat)
	mux.HandleFunc("POST /api/session/{id}/chat", s.handlePostChat)
	mux.HandleFunc("GET /api/session/{id}/events", s.handleSSE)

	// Game actions
	mux.HandleFunc("POST /api/claim-parcel", s.handleClaimParcel)
	mux.HandleFunc("POST /api/claim-ez", s.handleClaimEZ)
	mux.HandleFunc("POST /api/convert-parcel", s.handleConvertParcel)
	mux.HandleFunc("POST /api/claim-treasure", s.handleClaimTreasure)
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
	mux.HandleFunc("GET /api/kg/{code}", s.handleKGData)
	// Viewport fast path: batch geometry by ID (R-tree cached upstream, single-digit ms)
	mux.HandleFunc("POST /api/geometry-batch/{kind}", s.handleGeometryBatch)
	// Viewport polygon geometry (parcels + footprints) straight from upstream R-tree.
	// Replaces whole-KG export/geojson loads for map rendering.
	mux.HandleFunc("GET /api/viewport", s.handleViewport)

	// Cadastre proxy with caching
	mux.HandleFunc("GET /api/cadastre/", s.handleCadastreProxy)

	// LiDAR (srtm-lidar) proxy + enhanced-KG registry
	mux.HandleFunc("GET /api/lidar/kg/{code}", s.handleLidarKG)
	mux.HandleFunc("GET /api/lidar/", s.handleLidarProxy)
	mux.HandleFunc("GET /api/enhanced-kgs", s.handleEnhancedKGs)
	mux.HandleFunc("GET /api/similar", s.handleSimilarParcels)

	slog.Info("starting Siedler Österreich", "addr", addr)
	return http.ListenAndServe(addr, gzipMiddleware(mux))
}

// ---- Gzip Middleware ----

type gzipResponseWriter struct {
	http.ResponseWriter
	gz            *gzip.Writer
	headerWritten bool
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length") // compressed size differs
	w.Header().Set("Content-Encoding", "gzip")
	w.headerWritten = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	return w.gz.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	w.gz.Flush()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip gzip for SSE and non-gzip clients
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			strings.HasSuffix(r.URL.Path, "/events") {
			next.ServeHTTP(w, r)
			return
		}
		gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// ---- Helpers ----

func randomID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func jsonResp(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
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
	http.ServeFile(w, r, filepath.Join(s.StaticDir, "index.html"))
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

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
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

	// Check if name exists
	if _, err := s.Q.GetPlayerByName(r.Context(), req.Name); err == nil {
		jsonErr(w, "Name already taken", 409)
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

	player, _ := s.Q.GetPlayerByID(r.Context(), playerID)

	jsonResp(w, map[string]any{
		"player":       player,
		"rejoin_token": rejoinToken,
		"rejoin_url":   fmt.Sprintf("/rejoin/%s", rejoinToken),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}
	player, err := s.Q.GetPlayerByName(r.Context(), strings.TrimSpace(req.Name))
	if err != nil {
		jsonErr(w, "Player not found", 404)
		return
	}
	jsonResp(w, map[string]any{
		"player":       player,
		"rejoin_token": player.RejoinToken,
	})
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
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	sessionID := randomID(16)
	inviteCode := randomID(6)

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

	// Natura-2000 bonus treasures: placed asynchronously (fast API, but don't block session create)
	go s.generateN2KTreasures(context.Background(), sessionID, req.MunicipalityName)

	session, _ := s.Q.GetSession(r.Context(), sessionID)
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
	challenges, err := s.Q.GetPlayerChallenges(r.Context(), dbgen.GetPlayerChallengesParams{
		SessionID: r.PathValue("id"),
		PlayerID:  playerID,
	})
	if err != nil {
		jsonErr(w, "error", 500)
		return
	}
	jsonResp(w, challenges)
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
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

func (s *Server) handleClaimParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
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
		TallTreeCount     int     `json:"tall_tree_count"`
		TallTreeMaxH      float64 `json:"tall_tree_max_h"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	// Check if already claimed
	if _, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
		SessionID: req.SessionID,
		ParcelID:  req.ParcelID,
	}); err == nil {
		jsonErr(w, "Parcel already claimed", 409)
		return
	}

	// Calculate price based on area, landuse, and building density
	price := calculatePrice(req.AreaSqm, req.Landuse, req.BuildingCount, req.TotalBuildingArea)

	// Check player has enough coins
	player, err := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	if err != nil {
		jsonErr(w, "Player not found", 404)
		return
	}
	if player.Coins < int64(price) {
		jsonErr(w, fmt.Sprintf("Not enough coins! Need %d, have %d", price, player.Coins), 400)
		return
	}

	landuse := req.Landuse
	err = s.Q.ClaimParcel(r.Context(), dbgen.ClaimParcelParams{
		SessionID:     req.SessionID,
		PlayerID:      req.PlayerID,
		ParcelID:      req.ParcelID,
		KgCode:        req.KgCode,
		Gnr:           req.Gnr,
		Ez:            req.Ez,
		AreaSqm:       req.AreaSqm,
		Landuse:       &landuse,
		PurchasePrice: int64(price),
	})
	if err != nil {
		slog.Error("claim parcel", "error", err)
		jsonErr(w, "Failed to claim parcel", 500)
		return
	}

	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: int64(-price),
		ID:    req.PlayerID,
	})
	// Tall-tree bonus: parcels containing lidar-confirmed landmark trees award extra XP
	tallBonus := 0
	if req.TallTreeCount > 0 && req.TallTreeCount <= 10 && req.TallTreeMaxH > 0 && req.TallTreeMaxH <= 60 {
		tallBonus = req.TallTreeCount*40 + int(req.TallTreeMaxH)
		if tallBonus > 300 {
			tallBonus = 300
		}
	}
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{
		Xp: int64(10 + tallBonus),
		ID: req.PlayerID,
	})

	// Broadcast
	s.broadcast(req.SessionID, map[string]any{
		"type":      "parcel_claimed",
		"parcel_id": req.ParcelID,
		"player":    player.Name,
	})

	updatedPlayer, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	jsonResp(w, map[string]any{
		"success":       true,
		"price":         price,
		"player":        updatedPlayer,
		"tall_bonus_xp": tallBonus,
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
			SessionID: req.SessionID,
			ParcelID:  p.ParcelID,
		}); err == nil {
			continue
		}
		totalPrice += calculatePrice(p.AreaSqm, p.Landuse, p.BuildingCount, p.TotalBuildingArea)
	}

	// 20% discount for bulk EZ claim
	discountedPrice := int(float64(totalPrice) * 0.8)

	// Check player has enough coins
	player, err := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	if err != nil {
		jsonErr(w, "Player not found", 404)
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
			SessionID: req.SessionID,
			ParcelID:  p.ParcelID,
		}); err == nil {
			continue
		}
		price := calculatePrice(p.AreaSqm, p.Landuse, p.BuildingCount, p.TotalBuildingArea)
		// Each parcel gets its proportional discounted price
		discPrice := int64(float64(price) * 0.8)
		landuse := p.Landuse
		s.Q.ClaimParcel(r.Context(), dbgen.ClaimParcelParams{
			SessionID:     req.SessionID,
			PlayerID:      req.PlayerID,
			ParcelID:      p.ParcelID,
			KgCode:        req.KgCode,
			Gnr:           p.Gnr,
			Ez:            req.Ez,
			AreaSqm:       p.AreaSqm,
			Landuse:       &landuse,
			PurchasePrice: discPrice,
		})
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

	updatedPlayer, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	jsonResp(w, map[string]any{
		"success":       true,
		"claimed_count": claimed,
		"total_price":   discountedPrice,
		"discount":      totalPrice - discountedPrice,
		"xp_reward":     xpReward,
		"player":        updatedPlayer,
	})
}

func (s *Server) handleConvertParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		ParcelID  string `json:"parcel_id"`
		ConvertTo string `json:"convert_to"` // biodiversity, forest, wetland, meadow
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
		SessionID: req.SessionID,
		ParcelID:  req.ParcelID,
	})
	if err != nil {
		jsonErr(w, "Parcel not found", 404)
		return
	}
	if claim.PlayerID != req.PlayerID {
		jsonErr(w, "Not your parcel", 403)
		return
	}

	convertTo := req.ConvertTo
	s.Q.ConvertParcel(r.Context(), dbgen.ConvertParcelParams{
		ConvertedTo: &convertTo,
		ID:          claim.ID,
	})

	// Award XP for conversion
	xpReward := int64(50)
	if req.ConvertTo == "biodiversity" {
		xpReward = 100
	}
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{
		Xp: xpReward,
		ID: req.PlayerID,
	})

	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type":       "parcel_converted",
		"parcel_id":  req.ParcelID,
		"convert_to": req.ConvertTo,
		"player":     player.Name,
	})

	jsonResp(w, map[string]any{"success": true, "xp_reward": xpReward, "player": player})
}

func (s *Server) handleSellParcel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		PlayerID  string `json:"player_id"`
		ClaimID   int64  `json:"claim_id"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
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

	// Sell at 60% of purchase price
	sellPrice := int64(float64(claim.PurchasePrice) * 0.6)
	s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{
		Coins: sellPrice,
		ID:    req.PlayerID,
	})

	// Delete claim
	s.DB.ExecContext(r.Context(), "DELETE FROM parcel_claims WHERE id = ?", req.ClaimID)

	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	s.broadcast(req.SessionID, map[string]any{
		"type":      "parcel_sold",
		"parcel_id": claim.ParcelID,
		"player":    player.Name,
	})

	jsonResp(w, map[string]any{"success": true, "sell_price": sellPrice, "player": player})
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
	if req.OfferPrice < 10 {
		jsonErr(w, "Mindestangebot: 10 Münzen", 400)
		return
	}

	// Verify parcel is claimed
	claim, err := s.Q.GetParcelClaim(r.Context(), dbgen.GetParcelClaimParams{
		SessionID: req.SessionID,
		ParcelID:  req.ParcelID,
	})
	if err != nil {
		jsonErr(w, "Parzelle nicht gefunden", 404)
		return
	}
	if claim.PlayerID == req.BuyerID {
		jsonErr(w, "Du besitzt diese Parzelle bereits", 400)
		return
	}

	// Verify buyer exists and has enough coins (soft check — coins aren't locked)
	buyer, err := s.Q.GetPlayerByID(r.Context(), req.BuyerID)
	if err != nil {
		jsonErr(w, "Spieler nicht gefunden", 404)
		return
	}
	if buyer.Coins < req.OfferPrice {
		jsonErr(w, fmt.Sprintf("Nicht genug Münzen! Brauchst %d, hast %d", req.OfferPrice, buyer.Coins), 400)
		return
	}

	// Create the offer
	err = s.Q.CreateParcelOffer(r.Context(), dbgen.CreateParcelOfferParams{
		SessionID:  req.SessionID,
		ParcelID:   req.ParcelID,
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
			"type":      "offer_rejected",
			"parcel_id": offer.ParcelID,
			"buyer_id":  offer.BuyerID,
			"seller_id": offer.SellerID,
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
			"parcel_id":   offer.ParcelID,
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
		ParcelID:  offer.ParcelID,
		SessionID: offer.SessionID,
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
		"parcel_id":   offer.ParcelID,
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

	err := s.Q.ClaimTreasure(r.Context(), dbgen.ClaimTreasureParams{
		FoundBy: &req.PlayerID,
		ID:      req.TreasureID,
	})
	if err != nil {
		jsonErr(w, "Treasure already claimed", 409)
		return
	}

	// Get treasure value and award
	var value int64
	var ttype, speciesName, speciesGerman, speciesCat string
	s.DB.QueryRowContext(r.Context(),
		"SELECT value, treasure_type, species_name, species_german, species_category FROM treasures WHERE id = ?", req.TreasureID).Scan(&value, &ttype, &speciesName, &speciesGerman, &speciesCat)

	if ttype == "xp" {
		s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{Xp: value, ID: req.PlayerID})
	} else {
		// species and coins both award coins
		s.Q.UpdatePlayerCoins(r.Context(), dbgen.UpdatePlayerCoinsParams{Coins: value, ID: req.PlayerID})
	}

	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	resp := map[string]any{"success": true, "type": ttype, "value": value, "player": player}
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

	// Get challenge
	var coins, xp int64
	var sessionID string
	err := s.DB.QueryRowContext(r.Context(),
		"SELECT reward_coins, reward_xp, session_id FROM challenges WHERE id = ? AND player_id = ? AND completed = 0",
		req.ChallengeID, req.PlayerID).Scan(&coins, &xp, &sessionID)
	if err != nil {
		jsonErr(w, "Challenge not found or already completed", 404)
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
	if l, err := strconv.ParseInt(limitStr, 10, 64); err == nil && l > 0 {
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
	jsonResp(w, msgs)
}

func (s *Server) handlePostChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
		Message  string `json:"message"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, "invalid request", 400)
		return
	}

	msg, err := s.Q.CreateChatMessage(r.Context(), dbgen.CreateChatMessageParams{
		SessionID: r.PathValue("id"),
		PlayerID:  req.PlayerID,
		Message:   strings.TrimSpace(req.Message),
	})
	if err != nil {
		jsonErr(w, "Failed to send", 500)
		return
	}

	player, _ := s.Q.GetPlayerByID(r.Context(), req.PlayerID)
	s.broadcast(r.PathValue("id"), map[string]any{
		"type":    "chat",
		"message": msg.Message,
		"player":  player.Name,
		"time":    msg.CreatedAt,
	})

	jsonResp(w, msg)
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

// ---- Cadastre Proxy with Caching ----

// handleKGData serves KG geojson data in pages to avoid proxy size limits.
// GET /api/kg/{code}?layer=parcels&page=0&pagesize=200
// Returns {features: [...], page, pagesize, total, hasMore}
func (s *Server) handleKGData(w http.ResponseWriter, r *http.Request) {
	kg := r.PathValue("code")
	layer := r.URL.Query().Get("layer")
	if layer == "" {
		layer = "parcels"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pagesize"))
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 200
	}

	// Get cached full GeoJSON or fetch from upstream
	cacheKey := "/export/geojson?kg=" + kg + "&layers=" + layer
	var body []byte
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		body = []byte(cached)
	} else {
		url := cadastreAPI + "/export/geojson?kg=" + kg + "&layers=" + layer
		resp, err := http.Get(url)
		if err != nil {
			jsonErr(w, "Cadastre API error", 502)
			return
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
		if err != nil {
			jsonErr(w, "Read error", 502)
			return
		}
		// Compact coordinates
		if compacted, err := compactGeoJSON(raw); err == nil {
			body = compacted
		} else {
			body = raw
		}
		expiry := time.Now().Add(1 * time.Hour)
		s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
			CacheKey: cacheKey, Data: string(body), ExpiresAt: expiry,
		})
	}

	// Parse features and paginate
	var fc struct {
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(body, &fc); err != nil {
		jsonErr(w, "Parse error", 500)
		return
	}

	total := len(fc.Features)
	start := page * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	// Build compact response
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"features":[`))
	for i, f := range fc.Features[start:end] {
		if i > 0 {
			w.Write([]byte{','})
		}
		w.Write(f)
	}
	fmt.Fprintf(w, `],"page":%d,"pagesize":%d,"total":%d,"has_more":%v}`, page, pageSize, total, end < total)
}

// handleGeometryBatch forwards a list of parcel or footprint IDs to the cadastre
// batch-geometry fast path (R-tree cached upstream, single-digit ms warm). This is
// the viewport-scoped alternative to pulling a whole KG's export/geojson: the
// frontend already has the visible IDs from /spatial/bbox, so we upgrade exactly
// those to full polygons instead of thousands of off-screen ones.
//
// kind = "parcels" | "footprints". Body: {"ids": ["...", ...]} (max 5000).
// Per-ID geometry is static, so we cache each ID individually (24h) and only
// forward the cache-miss IDs upstream — a pan that re-touches known parcels is free.
func (s *Server) handleGeometryBatch(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	var upstreamPath, itemsKey, idField string
	switch kind {
	case "parcels":
		upstreamPath, itemsKey, idField = "/parcels/geometry/batch", "parcels", "parcel_id"
	case "footprints":
		upstreamPath, itemsKey, idField = "/footprints/geometry/batch", "footprints", "footprint_id"
	default:
		jsonErr(w, "kind must be parcels or footprints", 400)
		return
	}

	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		jsonErr(w, "bad body", 400)
		return
	}
	if len(req.IDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
		return
	}
	if len(req.IDs) > 5000 {
		req.IDs = req.IDs[:5000]
	}

	// Split into cache hits and misses.
	out := make([]json.RawMessage, 0, len(req.IDs))
	var miss []string
	seen := map[string]bool{}
	for _, id := range req.IDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ck := "geom:" + kind + ":" + id
		if cached, err := s.Q.GetCachedData(r.Context(), ck); err == nil {
			out = append(out, json.RawMessage(cached))
		} else {
			miss = append(miss, id)
		}
	}

	// Fetch misses from upstream in one batch call.
	if len(miss) > 0 {
		body, _ := json.Marshal(map[string]any{"ids": miss})
		resp, err := http.Post(cadastreAPI+upstreamPath, "application/json", bytes.NewReader(body))
		if err != nil {
			jsonErr(w, "Cadastre API error", 502)
			return
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
		if err != nil {
			jsonErr(w, "Read error", 502)
			return
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &parsed); err != nil {
			jsonErr(w, "Parse error", 502)
			return
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(parsed[itemsKey], &items); err == nil {
			expiry := time.Now().Add(24 * time.Hour)
			for _, it := range items {
				// Round coordinates to trim payload, then cache per-ID.
				var itObj map[string]any
				if b, err := json.Marshal(it); err == nil {
					if json.Unmarshal(b, &itObj) == nil {
						roundCoords(itObj)
					}
				}
				enc, err := json.Marshal(itObj)
				if err != nil {
					continue
				}
				out = append(out, json.RawMessage(enc))
				var idv string
				json.Unmarshal(it[idField], &idv)
				if idv != "" {
					s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
						CacheKey: "geom:" + kind + ":" + idv, Data: string(enc), ExpiresAt: expiry,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"items":[`))
	for i, it := range out {
		if i > 0 {
			w.Write([]byte{','})
		}
		w.Write(it)
	}
	w.Write([]byte(`]}`))
}

// handleViewport is the fast map-render path. It pulls polygon geometry for JUST
// the current viewport from the upstream R-tree endpoints (/spatial/parcels and
// /spatial/footprints) instead of loading whole KGs' export/geojson. Upstream
// serves these in ~100ms straight from a cached R*Tree (no json.gz load), and we
// gzip the compacted result down to ~40KB. Both layers are fetched in parallel
// and merged into one response so the client makes a single round-trip per pan.
//
// Query: west,south,east,north (WGS84). Optional: limit (default 6000).
// Response: {parcels:[...], footprints:[...], ready:bool, truncated:bool}
//
// Cached 6h keyed by a quantized bbox so nearby pans reuse tiles.
func (s *Server) handleViewport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	west, south := q.Get("west"), q.Get("south")
	east, north := q.Get("east"), q.Get("north")
	if west == "" || south == "" || east == "" || north == "" {
		jsonErr(w, "west,south,east,north required", 400)
		return
	}
	limit := q.Get("limit")
	if limit == "" {
		limit = "6000"
	}
	bboxQS := "west=" + west + "&south=" + south + "&east=" + east + "&north=" + north + "&limit=" + limit

	// Quantize bbox to ~0.002° (~150m) grid for cache reuse across small pans.
	qz := func(s string) string {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		return strconv.FormatFloat(math.Round(f/0.002)*0.002, 'f', 3, 64)
	}
	cacheKey := "viewport:" + qz(west) + "," + qz(south) + "," + qz(east) + "," + qz(north) + "," + limit
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}

	// Fetch both layers in parallel.
	type res struct {
		body []byte
		err  error
	}
	fetch := func(path string) res {
		resp, err := http.Get(cadastreAPI + path + "?" + bboxQS)
		if err != nil {
			return res{err: err}
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
		return res{body: b, err: err}
	}
	pCh := make(chan res, 1)
	fCh := make(chan res, 1)
	go func() { pCh <- fetch("/spatial/parcels") }()
	go func() { fCh <- fetch("/spatial/footprints") }()
	pr, fr := <-pCh, <-fCh
	if pr.err != nil || fr.err != nil {
		jsonErr(w, "Cadastre API error", 502)
		return
	}

	// Extract the arrays + ready flags, round coords, re-emit as one object.
	extract := func(body []byte, key string) (items []json.RawMessage, ready bool, truncated bool) {
		var parsed map[string]json.RawMessage
		if json.Unmarshal(body, &parsed) != nil {
			return nil, false, false
		}
		json.Unmarshal(parsed["ready"], &ready)
		json.Unmarshal(parsed["truncated"], &truncated)
		var arr []map[string]any
		if json.Unmarshal(parsed[key], &arr) == nil {
			for _, it := range arr {
				roundCoords(it)
				if enc, err := json.Marshal(it); err == nil {
					items = append(items, enc)
				}
			}
		}
		return
	}
	parcels, pReady, pTrunc := extract(pr.body, "parcels")
	foots, fReady, fTrunc := extract(fr.body, "footprints")

	var b bytes.Buffer
	b.WriteString(`{"parcels":[`)
	for i, it := range parcels {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(it)
	}
	b.WriteString(`],"footprints":[`)
	for i, it := range foots {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(it)
	}
	fmt.Fprintf(&b, `],"ready":%v,"truncated":%v}`, pReady && fReady, pTrunc || fTrunc)
	out := b.Bytes()

	// Only cache once upstream reports the tile fully warm, so we don't pin a
	// half-loaded viewport for 6h.
	if pReady && fReady {
		s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
			CacheKey: cacheKey, Data: string(out), ExpiresAt: time.Now().Add(6 * time.Hour),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(out)
}

func (s *Server) handleCadastreProxy(w http.ResponseWriter, r *http.Request) {
	// Strip our prefix and forward to cadastre API
	path := strings.TrimPrefix(r.URL.Path, "/api/cadastre")
	query := r.URL.RawQuery
	cacheKey := path + "?" + query

	// Check cache
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}

	url := cadastreAPI + path
	if query != "" {
		url += "?" + query
	}

	resp, err := http.Get(url)
	if err != nil {
		jsonErr(w, "Cadastre API error", 502)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB max
	if err != nil {
		jsonErr(w, "Read error", 502)
		return
	}

	// Compact GeoJSON: round coordinates to 6 decimals, minify JSON
	// This reduces ~1.6MB responses to ~1MB (and ~300KB with gzip)
	if strings.Contains(path, "/export/geojson") || strings.Contains(path, "/spatial/") {
		if compacted, err := compactGeoJSON(body); err == nil {
			body = compacted
		}
	}

	// Cache: KG geometry exports are static → 24h; everything else 1h
	ttl := 1 * time.Hour
	if strings.Contains(path, "/export/geojson") || strings.Contains(path, "/osm/geometry") || strings.Contains(path, "/natura2000/") {
		ttl = 24 * time.Hour
	}
	expiry := time.Now().Add(ttl)
	s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
		CacheKey:  cacheKey,
		Data:      string(body),
		ExpiresAt: expiry,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

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

func calculatePrice(areaSqm float64, landuse string, buildingCount int, totalBuildingArea float64) int {
	// Base price per sqm varies by landuse
	// NOTE: specific codes must come before prefix matches
	var pricePerSqm float64
	switch {
	case landuse == "48": // Verkehr (Straße)
		pricePerSqm = 0.1
	case strings.HasPrefix(landuse, "4"): // Baufläche
		pricePerSqm = 0.5
	case landuse == "56": // Wald
		pricePerSqm = 0.2
	case landuse == "52": // Grünland
		pricePerSqm = 0.3
	case strings.HasPrefix(landuse, "7") || strings.HasPrefix(landuse, "8"): // Gewässer, Ödland
		pricePerSqm = 0.05
	default:
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

	price := int(areaSqm * pricePerSqm * densityMult)
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

func (s *Server) generateTreasures(ctx context.Context, sessionID string, lon, lat float64) {
	// Generate a mix: some classic coin/xp treasures + species encounters
	type treasure struct {
		tType                                  string
		value                                  int64
		speciesName, speciesGerman, speciesCat string
	}
	var treasures []treasure

	// 3 classic treasures
	treasures = append(treasures,
		treasure{"coins", 100, "", "", ""},
		treasure{"coins", 200, "", "", ""},
		treasure{"xp", 150, "", "", ""},
	)

	// 7 species encounters (pick pseudo-random from list using session ID hash)
	hash := uint64(0)
	for _, c := range sessionID {
		hash = hash*31 + uint64(c)
	}
	for i := 0; i < 7; i++ {
		idx := int((hash + uint64(i)*7919) % uint64(len(redListSpecies)))
		sp := redListSpecies[idx]
		treasures = append(treasures, treasure{"species", sp.Value, sp.Name, sp.German, sp.Category})
	}

	for i, t := range treasures {
		dLon := (float64(i)*0.0012 - 0.005) + float64(i%3)*0.0006
		dLat := (float64(i)*0.0009 - 0.004) + float64(i%2)*0.0005
		s.Q.CreateTreasure(ctx, dbgen.CreateTreasureParams{
			SessionID:       sessionID,
			Lon:             lon + dLon,
			Lat:             lat + dLat,
			TreasureType:    t.tType,
			Value:           t.value,
			SpeciesName:     t.speciesName,
			SpeciesGerman:   t.speciesGerman,
			SpeciesCategory: t.speciesCat,
		})
	}
}

func (s *Server) generateChallenges(ctx context.Context, sessionID, playerID string, lon, lat float64) {
	challenges := []struct {
		cType, title, desc string
		coins, xp          int64
	}{
		{"explore", "Erkunde deine Gemeinde", "Claim your first parcel to begin exploring", 100, 50},
		{"restore", "Naturschützer", "Convert a parcel to biodiversity", 200, 100},
		{"explore", "Landvermesser", "Claim 5 parcels", 300, 150},
		{"treasure", "Schatzsucher", "Find a hidden treasure", 150, 75},
		{"restore", "Waldmeister", "Convert 3 parcels to forest or biodiversity", 500, 250},
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
func (s *Server) handleLidarProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/lidar")
	// Block slow endpoints
	if strings.Contains(path, "/overlay") || strings.Contains(path, "/elevation") || strings.Contains(path, "/dtm") {
		jsonErr(w, "endpoint too slow for gameplay", 400)
		return
	}
	query := r.URL.RawQuery
	cacheKey := "lidar:" + path + "?" + query
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}
	url := lidarAPI + path
	if query != "" {
		url += "?" + query
	}
	resp, err := http.Get(url)
	if err != nil {
		jsonErr(w, "LiDAR API error", 502)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		jsonErr(w, "Read error", 502)
		return
	}
	if resp.StatusCode == 200 {
		s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
			CacheKey: cacheKey, Data: string(body), ExpiresAt: time.Now().Add(1 * time.Hour),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
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
	cacheKey := "lidar-slim:/kg/" + kg
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}
	out, status := s.buildLidarSlim(r.Context(), kg)
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
// Returns (payload, 200) or (errMsg, status).
func (s *Server) buildLidarSlim(ctx context.Context, kg string) ([]byte, int) {
	cacheKey := "lidar-slim:/kg/" + kg

	// Fetch flags first (fast) to filter top trees/objects
	flagged := map[string]bool{} // obj_ref -> true for severity high/critical
	if fr, err := http.Get(lidarAPI + "/flags?kg=" + kg + "&limit=3000"); err == nil {
		var fd struct {
			Flags []struct {
				ObjRef    string `json:"obj_ref"`
				Severity  string `json:"severity"`
				Aggregate struct {
					MaxSeverity string `json:"max_severity"`
				} `json:"aggregate"`
			} `json:"flags"`
		}
		if b, err := io.ReadAll(io.LimitReader(fr.Body, 10<<20)); err == nil {
			json.Unmarshal(b, &fd)
			for _, f := range fd.Flags {
				sev := f.Aggregate.MaxSeverity
				if sev == "" {
					sev = f.Severity
				}
				if sev == "high" || sev == "critical" {
					flagged[f.ObjRef] = true
				}
			}
		}
		fr.Body.Close()
	}

	resp, err := http.Get(lidarAPI + "/kg/" + kg)
	if err != nil {
		return []byte("LiDAR API error"), 502
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return []byte("KG not processed"), resp.StatusCode
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
	if err != nil {
		return []byte("Read error"), 502
	}

	var full map[string]any
	if err := json.Unmarshal(raw, &full); err != nil {
		return []byte("Parse error"), 500
	}

	slim := map[string]any{
		"kg_code": kg,
		"kg_name": full["kg_name"],
	}
	if t, ok := full["terrain"].(map[string]any); ok {
		slim["terrain"] = map[string]any{
			"elevation_min_m":    t["elevation_min_m"],
			"elevation_max_m":    t["elevation_max_m"],
			"elevation_mean_m":   t["elevation_mean_m"],
			"terrain_class":      t["terrain_class"],
			"steepness_mean_deg": t["steepness_mean_deg"],
		}
	}

	// Slim parcels. While iterating, also harvest tall trees from the per-parcel
	// top_trees arrays — the KG-level top_10_trees only has 10, but each parcel
	// carries its own tallest trees, giving us hundreds of giants to show.
	var parcels []map[string]any
	type giantTree struct {
		h, lon, lat float64
	}
	var harvested []giantTree
	if p, ok := full["parcels"].(map[string]any); ok {
		if details, ok := p["details"].([]any); ok {
			for _, d := range details {
				pd, ok := d.(map[string]any)
				if !ok {
					continue
				}
				// top_trees rows: [hmax, hmean, hp90, area, lon, lat, ndvi_mean,
				// ndvi_fused, height_change_m, phenology, conf, rf_conf]
				pid, _ := pd["parcel_id"].(string)
				if tts, ok := pd["top_trees"].([]any); ok {
					for i, tr := range tts {
						row, ok := tr.([]any)
						if !ok || len(row) < 6 {
							continue
						}
						// Skip trees flagged high/critical by srtm QA (bad classification).
						if pid != "" && flagged[fmt.Sprintf("%s:parcel_top_tree:%s:%d", kg, pid, i)] {
							continue
						}
						h, _ := row[0].(float64)
						lon, _ := row[4].(float64)
						lat, _ := row[5].(float64)
						// Only real giants; clamp junk. rf_conf (idx 11) gate when present.
						if h < 25 || h > 60 || lon == 0 || lat == 0 {
							continue
						}
						if len(row) >= 12 {
							if rf, ok := row[11].(float64); ok && rf < 0.5 {
								continue
							}
						}
						harvested = append(harvested, giantTree{h: h, lon: lon, lat: lat})
					}
				}
				parcels = append(parcels, map[string]any{
					"parcel_id":       pd["parcel_id"],
					"elevation_m":     pd["elevation_m"],
					"elevation_min_m": pd["elevation_min_m"],
					"elevation_max_m": pd["elevation_max_m"],
					"slope_mean_deg":  pd["slope_mean_deg"],
					"aspect_dominant": pd["aspect_dominant"],
					"terrain_class":   pd["terrain_class"],
					"dominant_type":   pd["dominant_type"],
					// Corrected dominant land cover for TERRAIN fill: srtm often
					// mislabels a parcel's dominant as road/roof (impervious). For
					// ground coloring we want the dominant *natural* cover, so skip
					// the impervious family and fall back to the next-largest type.
					// Buildings are drawn separately as footprints (roof color there).
					"dom_terrain":       correctedDomTerrain(pd),
					"forested_fraction": pd["forested_fraction"],
					"ndsm_max_m":        pd["ndsm_max_m"],
					// Compact land-cover composition vector from area_summary:
					// {type: fraction} rounded to 2 decimals, tiny slivers dropped.
					// This 1m-resolution "what is actually ON the parcel" mix is the
					// strongest similarity signal srtm offers.
					"fracs": compactFracs(pd),
				})
			}
		}
	}
	slim["parcels"] = parcels

	// Slim buildings (keyed by centroid for client-side matching)
	var buildings []map[string]any
	if b, ok := full["building_footprints"].(map[string]any); ok {
		if details, ok := b["details"].([]any); ok {
			for _, d := range details {
				bd, ok := d.(map[string]any)
				if !ok {
					continue
				}
				c, _ := bd["centroid"].(map[string]any)
				if c == nil {
					continue
				}
				buildings = append(buildings, map[string]any{
					"lon":            c["lon"],
					"lat":            c["lat"],
					"max_height_m":   bd["max_height_m"],
					"stories_est":    bd["stories_est"],
					"roof_type_hint": bd["roof_type_hint"],
					"area_sqm":       bd["footprint_area_sqm"],
				})
			}
		}
	}
	slim["buildings"] = buildings

	// Flag-filtered top trees (clamp height <= 60m)
	var topTrees []map[string]any
	if tt, ok := full["top_10_trees"].([]any); ok {
		for i, t := range tt {
			td, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if flagged[fmt.Sprintf("%s:top_tree:%d", kg, i)] {
				continue
			}
			h, _ := td["height_m"].(float64)
			if h > 60 || h <= 0 {
				continue
			}
			c, _ := td["coordinate"].(map[string]any)
			if c == nil {
				continue
			}
			topTrees = append(topTrees, map[string]any{
				"height_m": math.Round(h*10) / 10,
				"lon":      c["lon"],
				"lat":      c["lat"],
			})
		}
	}
	// Merge in the per-parcel harvested giants (tallest first), deduping trees that
	// land on the same ~15m grid cell (KG-level and per-parcel lists overlap).
	sort.Slice(harvested, func(i, j int) bool { return harvested[i].h > harvested[j].h })
	seenTree := map[string]bool{}
	gridKey := func(lon, lat float64) string {
		return fmt.Sprintf("%.4f,%.4f", lon, lat)
	}
	for _, t := range topTrees {
		lon, _ := t["lon"].(float64)
		lat, _ := t["lat"].(float64)
		seenTree[gridKey(lon, lat)] = true
	}
	const maxGiants = 120
	for _, g := range harvested {
		if len(topTrees) >= maxGiants {
			break
		}
		k := gridKey(g.lon, g.lat)
		if seenTree[k] {
			continue
		}
		seenTree[k] = true
		topTrees = append(topTrees, map[string]any{
			"height_m": math.Round(g.h*10) / 10,
			"lon":      g.lon,
			"lat":      g.lat,
		})
	}
	slim["top_trees"] = topTrees

	// Flag-filtered top objects (non-tree landmarks: tall roofs/masts confirmed OK)
	var topObjects []map[string]any
	if to, ok := full["top_10_objects"].([]any); ok {
		for i, t := range to {
			td, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if flagged[fmt.Sprintf("%s:top_object:%d", kg, i)] || flagged[fmt.Sprintf("%s:top_obj:%d", kg, i)] {
				continue
			}
			typ, _ := td["type"].(string)
			if typ == "tree" {
				continue // trees handled above
			}
			h, _ := td["height_max_m"].(float64)
			if h > 120 || h <= 0 {
				continue
			}
			c, _ := td["coordinate"].(map[string]any)
			if c == nil {
				continue
			}
			topObjects = append(topObjects, map[string]any{
				"type":     typ,
				"height_m": math.Round(h*10) / 10,
				"lon":      c["lon"],
				"lat":      c["lat"],
			})
		}
	}
	slim["top_objects"] = topObjects

	out, err := json.Marshal(slim)
	if err != nil {
		return []byte("Marshal error"), 500
	}
	s.Q.SetCachedData(ctx, dbgen.SetCachedDataParams{
		CacheKey: cacheKey, Data: string(out), ExpiresAt: time.Now().Add(6 * time.Hour),
	})
	return out, 200
}

// ---- Similar parcels ("show all the power" feature) ----
//
// GET /api/similar?parcel_id=&lon=&lat=&area=&lu=&bcount=&barea=&radius=5000&limit=40
//
// Finds parcels similar to a reference parcel within radius, combining:
//   - cadastre /spatial/point (size band + landuse-prefiltered candidates, FAST R-tree)
//   - cached lidar slim KG data (slope / aspect / elevation / dominant cover)
//
// Scores each candidate 0..1 on size, landuse mix, terrain and built density.
// Fully cached 1h per reference parcel; typical cold latency < 1.5s.
func (s *Server) handleSimilarParcels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pid := q.Get("parcel_id")
	lon, _ := strconv.ParseFloat(q.Get("lon"), 64)
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	area, _ := strconv.ParseFloat(q.Get("area"), 64)
	if pid == "" || lon == 0 || lat == 0 || area <= 0 {
		jsonErr(w, "parcel_id, lon, lat, area required", 400)
		return
	}
	lu := q.Get("lu") // dominant landuse code (e.g. "48") — optional prefilter
	bcount, _ := strconv.Atoi(q.Get("bcount"))
	barea, _ := strconv.ParseFloat(q.Get("barea"), 64)
	radius := 5000.0
	if v, err := strconv.ParseFloat(q.Get("radius"), 64); err == nil && v >= 500 && v <= 50000 {
		radius = v
	}
	limit := 40
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}

	cacheKey := fmt.Sprintf("similar:v2:%s:%s:%.0f:%d", pid, lu, radius, limit)
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}
	t0 := time.Now()

	// 1. Candidate parcels from the cadastre R-tree: same size band, same dominant
	// landuse, within radius. attrs_only keeps the payload tiny.
	minA := area * 0.4
	maxA := area * 2.5
	cu := fmt.Sprintf("%s/spatial/point?lon=%.6f&lat=%.6f&radius=%.0f&layer=parcels&attrs_only=true&min_area=%.0f&max_area=%.0f&limit=4000",
		cadastreAPI, lon, lat, radius, minA, maxA)
	if lu != "" {
		cu += "&landuse=" + url.QueryEscape(lu)
	}
	// Cadastre R-tree latency grows with radius (~0.7s @5km, ~14s @50km cold);
	// scale the timeout so large-radius searches don't get cut off.
	clientTimeout := 12 * time.Second
	if radius > 10000 {
		clientTimeout = 30 * time.Second
	}
	client := &http.Client{Timeout: clientTimeout}
	resp, err := client.Get(cu)
	if err != nil {
		jsonErr(w, "cadastre error", 502)
		return
	}
	var cres struct {
		Data struct {
			Parcels []map[string]any `json:"parcels"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
	resp.Body.Close()
	if err := json.Unmarshal(body, &cres); err != nil {
		jsonErr(w, "cadastre parse error", 502)
		return
	}

	// 2. Reference landuse mix + built density from the reference row (find it in
	// candidates, or fall back to query params).
	type cand struct {
		row      map[string]any
		luSet    map[string]bool
		bc       int
		ba, area float64
	}
	luSetOf := func(row map[string]any) map[string]bool {
		set := map[string]bool{}
		if ls, ok := row["landuse_summary"].(map[string]any); ok {
			for k := range ls {
				set[k] = true
			}
		}
		return set
	}
	var refLU map[string]bool
	cands := make([]cand, 0, len(cres.Data.Parcels))
	for _, row := range cres.Data.Parcels {
		id, _ := row["parcel_id"].(string)
		a, _ := row["area_sqm"].(float64)
		bcF, _ := row["building_count"].(float64)
		baF, _ := row["total_building_area_sqm"].(float64)
		c := cand{row: row, luSet: luSetOf(row), bc: int(bcF), ba: baF, area: a}
		if id == pid {
			refLU = c.luSet
			continue
		}
		cands = append(cands, c)
	}
	// The size/landuse-filtered candidate query can miss the reference parcel
	// itself (filters, dedup) — fetch its row directly so the landuse-mix
	// Jaccard term always has real reference data. Tiny radius → few ms.
	if len(refLU) == 0 {
		ru := fmt.Sprintf("%s/spatial/point?lon=%.6f&lat=%.6f&radius=25&layer=parcels&attrs_only=true&limit=20", cadastreAPI, lon, lat)
		if rr, err := client.Get(ru); err == nil {
			var rres struct {
				Data struct {
					Parcels []map[string]any `json:"parcels"`
				} `json:"data"`
			}
			rb, _ := io.ReadAll(io.LimitReader(rr.Body, 2<<20))
			rr.Body.Close()
			if json.Unmarshal(rb, &rres) == nil {
				for _, row := range rres.Data.Parcels {
					if id, _ := row["parcel_id"].(string); id == pid {
						refLU = luSetOf(row)
						break
					}
				}
			}
		}
	}

	// 3. Lidar terrain attributes: pull from already-cached slim KG JSONs (never
	// block on cold KGs — warm them in the background instead).
	type terr struct {
		slope, elev, forest float64
		aspect, dom         string
		fracs               map[string]float64
		ok                  bool
	}
	lidarByPid := map[string]terr{}
	refKG := pid[:strings.Index(pid, "-")]
	ingestSlim := func(data string) {
		var slim struct {
			Parcels []struct {
				ParcelID   string             `json:"parcel_id"`
				Elev       *float64           `json:"elevation_m"`
				Slope      *float64           `json:"slope_mean_deg"`
				Aspect     string             `json:"aspect_dominant"`
				DomTerrain string             `json:"dom_terrain"`
				ForestFrac *float64           `json:"forested_fraction"`
				Fracs      map[string]float64 `json:"fracs"`
			} `json:"parcels"`
		}
		if json.Unmarshal([]byte(data), &slim) != nil {
			return
		}
		for _, p := range slim.Parcels {
			t := terr{aspect: p.Aspect, dom: p.DomTerrain, fracs: p.Fracs, ok: true}
			if p.Elev != nil {
				t.elev = *p.Elev
			}
			if p.Slope != nil {
				t.slope = *p.Slope
			}
			if p.ForestFrac != nil {
				t.forest = *p.ForestFrac
			}
			lidarByPid[p.ParcelID] = t
		}
	}
	kgSeen := map[string]bool{}
	kgOf := func(row map[string]any) string { s, _ := row["kg_code"].(string); return s }
	allKGs := append([]cand{{row: map[string]any{"kg_code": refKG}}}, cands...)
	for _, c := range allKGs {
		kg := kgOf(c.row)
		if kg == "" || kgSeen[kg] {
			continue
		}
		kgSeen[kg] = true
		cached, err := s.Q.GetCachedData(r.Context(), "lidar-slim:/kg/"+kg)
		if err != nil {
			if kg == refKG {
				// The srtm terms are the whole point — warm the reference KG
				// synchronously (bounded) so terrain scoring is available even on
				// a cold cache. Candidate KGs warm in the background.
				done := make(chan []byte, 1)
				go func() {
					out, st := s.buildLidarSlim(context.Background(), kg)
					if st == 200 {
						done <- out
					} else {
						done <- nil
					}
				}()
				select {
				case out := <-done:
					if out != nil {
						ingestSlim(string(out))
					}
				case <-time.After(3 * time.Second):
					// keeps warming in background; this request goes cadastre-only
				}
			} else {
				go s.buildLidarSlim(context.Background(), kg)
			}
			continue
		}
		ingestSlim(cached)
	}
	refT, refHasT := lidarByPid[pid]

	// 4. Score candidates.
	aspectIdx := map[string]int{"N": 0, "NE": 1, "E": 2, "SE": 3, "S": 4, "SW": 5, "W": 6, "NW": 7}
	scoreOf := func(c cand) (float64, map[string]float64) {
		parts := map[string]float64{}
		// size: ratio of smaller/larger
		sz := math.Min(c.area, area) / math.Max(c.area, area)
		parts["size"] = sz
		// landuse mix: Jaccard of landuse_summary keys
		luS := 0.5
		if len(refLU) > 0 && len(c.luSet) > 0 {
			inter, uni := 0, 0
			seen := map[string]bool{}
			for k := range refLU {
				seen[k] = true
				uni++
				if c.luSet[k] {
					inter++
				}
			}
			for k := range c.luSet {
				if !seen[k] {
					uni++
				}
			}
			luS = float64(inter) / float64(uni)
		}
		parts["landuse"] = luS
		// built density
		bld := 1.0
		refDens := 0.0
		if area > 0 {
			refDens = barea / area
		}
		cDens := 0.0
		if c.area > 0 {
			cDens = c.ba / c.area
		}
		switch {
		case bcount == 0 && c.bc == 0:
			bld = 1
		case bcount > 0 && c.bc > 0:
			bld = 1 - math.Min(1, math.Abs(refDens-cDens)*2.5)
		default:
			bld = 0.15
		}
		parts["building"] = bld
		// terrain (only when both sides have lidar)
		hasTerr := false
		terrS := 0.0
		if refHasT {
			id, _ := c.row["parcel_id"].(string)
			if ct, ok := lidarByPid[id]; ok {
				hasTerr = true
				slopeS := 1 - math.Min(1, math.Abs(refT.slope-ct.slope)/15)
				elevS := 1 - math.Min(1, math.Abs(refT.elev-ct.elev)/250)
				aspS := 0.5
				if ai, ok1 := aspectIdx[refT.aspect]; ok1 {
					if bi, ok2 := aspectIdx[ct.aspect]; ok2 {
						d := int(math.Abs(float64(ai - bi)))
						if d > 4 {
							d = 8 - d
						}
						aspS = 1 - float64(d)/4
					}
				}
				domS := 0.5
				if refT.dom != "" && ct.dom != "" {
					if refT.dom == ct.dom {
						domS = 1
					} else {
						domS = 0
					}
				}
				// forested fraction: srtm's 1m canopy measurement — far sharper
				// than cadastre "Wald" codes
				forS := 1 - math.Min(1, math.Abs(refT.forest-ct.forest)*1.5)
				// land-cover composition: histogram intersection of the srtm
				// area_summary fraction vectors (1m-resolution actual cover).
				// This tells us more about what the parcel IS than any single
				// attribute — weight it dominantly when both sides have it.
				if len(refT.fracs) > 0 && len(ct.fracs) > 0 {
					compS := 0.0
					for t, fv := range refT.fracs {
						compS += math.Min(fv, ct.fracs[t])
					}
					parts["composition"] = compS
					terrS = compS*0.45 + slopeS*0.18 + elevS*0.15 + aspS*0.08 + domS*0.04 + forS*0.1
				} else {
					terrS = slopeS*0.28 + elevS*0.24 + aspS*0.12 + domS*0.16 + forS*0.2
				}
				parts["terrain"] = terrS
			}
		}
		if hasTerr {
			return sz*0.28 + luS*0.24 + bld*0.14 + terrS*0.34, parts
		}
		return sz*0.42 + luS*0.36 + bld*0.22, parts
	}

	type scored struct {
		Score    float64            `json:"score"`
		Parts    map[string]float64 `json:"parts"`
		ParcelID string             `json:"parcel_id"`
		KgCode   string             `json:"kg_code"`
		Gnr      string             `json:"gnr"`
		Ez       string             `json:"ez"`
		Lon      float64            `json:"lon"`
		Lat      float64            `json:"lat"`
		AreaSqm  float64            `json:"area_sqm"`
		DistM    float64            `json:"distance_m"`
		Slope    *float64           `json:"slope,omitempty"`
		Elev     *float64           `json:"elev,omitempty"`
		Aspect   string             `json:"aspect,omitempty"`
		Forest   *float64           `json:"forest_frac,omitempty"`
		Dom      string             `json:"dom,omitempty"`
		Fracs    map[string]float64 `json:"fracs,omitempty"`
		Landuse  map[string]any     `json:"landuse_summary,omitempty"`
		BCount   int                `json:"building_count"`
	}
	var out []scored
	for _, c := range cands {
		sc, parts := scoreOf(c)
		id, _ := c.row["parcel_id"].(string)
		kgc, _ := c.row["kg_code"].(string)
		gnr, _ := c.row["gnr"].(string)
		ez, _ := c.row["ez"].(string)
		clon, _ := c.row["lon"].(float64)
		clat, _ := c.row["lat"].(float64)
		dm, _ := c.row["distance_m"].(float64)
		lsum, _ := c.row["landuse_summary"].(map[string]any)
		rec := scored{Score: math.Round(sc*1000) / 1000, Parts: parts, ParcelID: id, KgCode: kgc,
			Gnr: gnr, Ez: ez, Lon: clon, Lat: clat, AreaSqm: math.Round(c.area), DistM: math.Round(dm),
			Landuse: lsum, BCount: c.bc}
		if t, ok := lidarByPid[id]; ok {
			sl, el, ff := math.Round(t.slope*10)/10, math.Round(t.elev), math.Round(t.forest*100)/100
			rec.Slope, rec.Elev, rec.Aspect = &sl, &el, t.aspect
			rec.Forest, rec.Dom = &ff, t.dom
			rec.Fracs = t.fracs
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}

	respMap := map[string]any{
		"parcel_id":   pid,
		"radius_m":    radius,
		"candidates":  cres.Meta.Total,
		"scored":      len(cands),
		"lidar_terms": refHasT,
		"results":     out,
		"took_ms":     time.Since(t0).Milliseconds(),
	}
	if refHasT {
		respMap["ref"] = map[string]any{
			"slope": math.Round(refT.slope*10) / 10, "elev": math.Round(refT.elev),
			"aspect": refT.aspect, "forest_frac": math.Round(refT.forest*100) / 100,
			"dom": refT.dom, "fracs": refT.fracs,
		}
	}
	payload, _ := json.Marshal(respMap)
	s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
		CacheKey: cacheKey, Data: string(payload), ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(payload)
}

// handleEnhancedKGs returns the list of lidar-processed KGs (the "enhanced" set).
// Cached 15 minutes — the lidar service processes more KGs continuously.
func (s *Server) handleEnhancedKGs(w http.ResponseWriter, r *http.Request) {
	cacheKey := "enhanced-kgs:v1"
	if cached, err := s.Q.GetCachedData(r.Context(), cacheKey); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}

	type kgEntry struct {
		KgCode       string  `json:"kg_code"`
		KgName       string  `json:"kg_name"`
		GemeindeCode string  `json:"gemeinde_code"`
		GemeindeName string  `json:"gemeinde_name"`
		Lon          float64 `json:"lon"`
		Lat          float64 `json:"lat"`
	}
	var all []kgEntry
	offset := 0
	for {
		url := fmt.Sprintf("%s/query?bbox=9,46,18,49.5&processed_only=true&limit=1000&offset=%d", lidarAPI, offset)
		resp, err := http.Get(url)
		if err != nil {
			jsonErr(w, "LiDAR API error", 502)
			return
		}
		var page struct {
			Total   int `json:"total"`
			Results []struct {
				KgCode       string  `json:"kg_code"`
				KgName       string  `json:"kg_name"`
				GemeindeCode string  `json:"gemeinde_code"`
				GemeindeName string  `json:"gemeinde_name"`
				CentroidLon  float64 `json:"centroid_lon"`
				CentroidLat  float64 `json:"centroid_lat"`
			} `json:"results"`
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
		resp.Body.Close()
		if err != nil || json.Unmarshal(body, &page) != nil {
			jsonErr(w, "LiDAR API parse error", 502)
			return
		}
		for _, res := range page.Results {
			all = append(all, kgEntry{res.KgCode, res.KgName, res.GemeindeCode, res.GemeindeName, res.CentroidLon, res.CentroidLat})
		}
		offset += len(page.Results)
		if offset >= page.Total || len(page.Results) == 0 {
			break
		}
	}

	out, _ := json.Marshal(map[string]any{"count": len(all), "kgs": all})
	s.Q.SetCachedData(r.Context(), dbgen.SetCachedDataParams{
		CacheKey: cacheKey, Data: string(out), ExpiresAt: time.Now().Add(15 * time.Minute),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(out)
}

// generateN2KTreasures places extra high-value rare-species treasures on parcels
// inside Natura-2000 sites overlapping the session's municipality KGs.
// Runs in a goroutine at session create; broadcasts treasures_updated via SSE when done.
func (s *Server) generateN2KTreasures(ctx context.Context, sessionID, muniName string) {
	if muniName == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	// 1. Find KGs of the municipality
	resp, err := client.Get(cadastreAPI + "/search/kg?gemeinde=" + url.QueryEscape(muniName) + "&limit=50")
	if err != nil {
		return
	}
	var kgRes struct {
		Data []struct {
			KgCode string `json:"kg_code"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	resp.Body.Close()
	json.Unmarshal(body, &kgRes)
	if len(kgRes.Data) == 0 {
		return
	}
	kgSet := map[string]bool{}
	for _, kg := range kgRes.Data {
		kgSet[kg.KgCode] = true
	}

	// 2. For each KG, find inside Natura-2000 sites (dedupe by sitecode)
	sites := map[string]bool{}
	for kg := range kgSet {
		r2, err := client.Get(cadastreAPI + "/natura2000/kg/" + kg)
		if err != nil {
			continue
		}
		var n2k struct {
			Data struct {
				InsideSites []struct {
					Sitecode string `json:"sitecode"`
				} `json:"inside_sites"`
			} `json:"data"`
		}
		b, _ := io.ReadAll(io.LimitReader(r2.Body, 5<<20))
		r2.Body.Close()
		json.Unmarshal(b, &n2k)
		for _, st := range n2k.Data.InsideSites {
			sites[st.Sitecode] = true
		}
	}
	if len(sites) == 0 {
		return
	}

	// 3. Fetch parcels inside each site, keep those in our KGs, place treasures
	hash := uint64(0)
	for _, c := range sessionID {
		hash = hash*31 + uint64(c)
	}
	placed := 0
	for code := range sites {
		if placed >= 5 {
			break
		}
		r3, err := client.Get(cadastreAPI + "/natura2000/site_parcels/" + code + "?limit=200")
		if err != nil {
			continue
		}
		var sp struct {
			Data struct {
				Results []struct {
					KgCode string  `json:"kg_code"`
					Lon    float64 `json:"lon"`
					Lat    float64 `json:"lat"`
				} `json:"results"`
			} `json:"data"`
		}
		b, _ := io.ReadAll(io.LimitReader(r3.Body, 10<<20))
		r3.Body.Close()
		json.Unmarshal(b, &sp)
		var local []struct {
			Lon, Lat float64
		}
		for _, pr := range sp.Data.Results {
			if kgSet[pr.KgCode] && pr.Lon != 0 {
				local = append(local, struct{ Lon, Lat float64 }{pr.Lon, pr.Lat})
			}
		}
		if len(local) == 0 {
			continue
		}
		// place up to 3 per site at pseudo-random parcels
		n := 3
		if len(local) < n {
			n = len(local)
		}
		for i := 0; i < n && placed < 5; i++ {
			idx := int((hash + uint64(i)*104729 + uint64(placed)*7919) % uint64(len(local)))
			si := int((hash + uint64(placed)*31) % uint64(len(redListSpecies)))
			sp2 := redListSpecies[si]
			err := s.Q.CreateTreasure(ctx, dbgen.CreateTreasureParams{
				SessionID:       sessionID,
				Lon:             local[idx].Lon,
				Lat:             local[idx].Lat,
				TreasureType:    "n2k_species",
				Value:           sp2.Value * 2, // Natura-2000 bonus: double value
				SpeciesName:     sp2.Name,
				SpeciesGerman:   sp2.German,
				SpeciesCategory: sp2.Category,
			})
			if err == nil {
				placed++
			}
		}
	}
	if placed > 0 {
		slog.Info("placed N2K treasures", "session", sessionID, "count", placed)
		s.broadcast(sessionID, map[string]any{"type": "treasures_updated"})
	}
}
