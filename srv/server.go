package srv

import (
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
	"strconv"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

const cadastreAPI = "https://cadastre-process-api.exe.xyz/api/v1"

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	Q            *dbgen.Queries

	// SSE connections for real-time updates
	sseClients   map[string]map[chan string]bool // session_id -> set of channels
	sseMu        sync.RWMutex
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

	// Cadastre proxy with caching
	mux.HandleFunc("GET /api/cadastre/", s.handleCadastreProxy)

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

	session, _ := s.Q.GetSession(r.Context(), sessionID)
	jsonResp(w, map[string]any{
		"session":     session,
		"invite_code": inviteCode,
		"invite_url":  fmt.Sprintf("/join/%s", inviteCode),
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
		SessionID string  `json:"session_id"`
		PlayerID  string  `json:"player_id"`
		ParcelID  string  `json:"parcel_id"`
		KgCode    string  `json:"kg_code"`
		Gnr       string  `json:"gnr"`
		Ez        string  `json:"ez"`
		AreaSqm            float64 `json:"area_sqm"`
		Landuse            string  `json:"landuse"`
		BuildingCount      int     `json:"building_count"`
		TotalBuildingArea  float64 `json:"total_building_area"`
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
	s.Q.UpdatePlayerXP(r.Context(), dbgen.UpdatePlayerXPParams{
		Xp: 10,
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
		"success": true,
		"price":   price,
		"player":  updatedPlayer,
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
		"success":         true,
		"claimed_count":   claimed,
		"total_price":     discountedPrice,
		"discount":        totalPrice - discountedPrice,
		"xp_reward":       xpReward,
		"player":          updatedPlayer,
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
		"success":   true,
		"action":    "accepted",
		"buyer":     updatedBuyer,
		"seller":    seller,
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
	jsonResp(w, player)
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

	// Cache for 1 hour
	expiry := time.Now().Add(1 * time.Hour)
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
		tType                          string
		value                          int64
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
