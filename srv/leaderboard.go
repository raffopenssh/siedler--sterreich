package srv

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Agent leaderboard — GET /agents (HTML) and GET /api/agents/leaderboard (JSON).
// Ranks 🤖 players (players.agent != '') and aggregates per model label, so
// "which model is the better settler" is a public, self-refreshing page.
// Score = hectares protected (biodiversity + wildforest) — the game's actual
// goal — with XP as tie-breaker. Cached 60 s.

type lbRow struct {
	Rank        int     `json:"rank"`
	PlayerID    string  `json:"player_id,omitempty"`
	Name        string  `json:"name"`
	Agent       string  `json:"agent"`
	Players     int     `json:"players,omitempty"` // per-model rows only
	Parcels     int     `json:"parcels"`
	ClaimedHa   float64 `json:"claimed_ha"`
	ProtectedHa float64 `json:"protected_ha"`
	Protected   int     `json:"protected_parcels"`
	Harvests    int     `json:"harvests"`
	XP          int     `json:"xp"`
	Coins       int     `json:"coins"`
	Sessions    int     `json:"sessions"`
	LastSeen    string  `json:"last_seen"`
	WatchURL    string  `json:"watch_url,omitempty"`
	Municipal   string  `json:"municipality,omitempty"`
}

type leaderboard struct {
	GeneratedAt string  `json:"generated_at"`
	Agents      int     `json:"agents"`
	Humans      int     `json:"humans"`
	Models      []lbRow `json:"models"`
	Players     []lbRow `json:"players"`
	HumanTopHa  float64 `json:"human_top_protected_ha"`
	Docs        string  `json:"docs"`
}

var lbCache struct {
	sync.Mutex
	at  time.Time
	val *leaderboard
}

const lbPlayerSQL = `
SELECT p.id, p.name, p.agent, p.xp, p.coins, p.last_seen,
       COUNT(c.id), COALESCE(SUM(c.area_sqm),0),
       COALESCE(SUM(CASE WHEN c.converted_to IN ('biodiversity','wildforest') THEN c.area_sqm END),0),
       COALESCE(SUM(CASE WHEN c.converted_to IN ('biodiversity','wildforest') THEN 1 ELSE 0 END),0),
       COALESCE(SUM(c.harvests),0),
       (SELECT COUNT(*) FROM session_players sp WHERE sp.player_id = p.id)
FROM players p LEFT JOIN parcel_claims c ON c.player_id = p.id
WHERE p.agent != ''
GROUP BY p.id
ORDER BY 9 DESC, p.xp DESC, 7 DESC
LIMIT 200`

func (s *Server) buildLeaderboard() *leaderboard {
	lb := &leaderboard{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Docs: siteURL + "/llm/game"}
	s.DB.QueryRow("SELECT COUNT(*) FROM players WHERE agent != ''").Scan(&lb.Agents)
	s.DB.QueryRow("SELECT COUNT(*) FROM players WHERE agent = ''").Scan(&lb.Humans)
	s.DB.QueryRow(`SELECT COALESCE(MAX(ha),0) FROM (SELECT SUM(c.area_sqm)/10000.0 ha FROM parcel_claims c JOIN players p ON p.id=c.player_id
		WHERE p.agent='' AND c.converted_to IN ('biodiversity','wildforest') GROUP BY c.player_id)`).Scan(&lb.HumanTopHa)

	rows, err := s.DB.Query(lbPlayerSQL)
	if err != nil {
		return lb
	}
	defer rows.Close()
	models := map[string]*lbRow{}
	order := []string{}
	for rows.Next() {
		var r lbRow
		var claimed, prot float64
		var last time.Time
		if err := rows.Scan(&r.PlayerID, &r.Name, &r.Agent, &r.XP, &r.Coins, &last, &r.Parcels, &claimed, &prot, &r.Protected, &r.Harvests, &r.Sessions); err != nil {
			continue
		}
		r.ClaimedHa, r.ProtectedHa = claimed/1e4, prot/1e4
		r.LastSeen = last.UTC().Format(time.RFC3339)
		r.Rank = len(lb.Players) + 1
		// latest session → watch link
		var code, muni string
		var lon, lat float64
		if s.DB.QueryRow(`SELECT g.invite_code, g.municipality_name, g.center_lon, g.center_lat FROM session_players sp JOIN game_sessions g ON g.id=sp.session_id
			WHERE sp.player_id=? ORDER BY sp.joined_at DESC LIMIT 1`, r.PlayerID).Scan(&code, &muni, &lon, &lat) == nil {
			r.WatchURL = fmt.Sprintf("%s/join/%s#v=%.5f,%.5f,16.0", siteURL, code, lon, lat)
			r.Municipal = muni
		}
		lb.Players = append(lb.Players, r)
		m, ok := models[r.Agent]
		if !ok {
			m = &lbRow{Agent: r.Agent, Name: r.Agent}
			models[r.Agent] = m
			order = append(order, r.Agent)
		}
		m.Players++
		m.Parcels += r.Parcels
		m.ClaimedHa += r.ClaimedHa
		m.ProtectedHa += r.ProtectedHa
		m.Protected += r.Protected
		m.Harvests += r.Harvests
		m.XP += r.XP
		m.Coins += r.Coins
		m.Sessions += r.Sessions
		if r.LastSeen > m.LastSeen {
			m.LastSeen = r.LastSeen
		}
	}
	for _, k := range order {
		lb.Models = append(lb.Models, *models[k])
	}
	// models: sort by protected ha, then xp (stable insertion sort — n is tiny)
	for i := 1; i < len(lb.Models); i++ {
		for j := i; j > 0 && lbLess(lb.Models[j], lb.Models[j-1]); j-- {
			lb.Models[j], lb.Models[j-1] = lb.Models[j-1], lb.Models[j]
		}
	}
	for i := range lb.Models {
		lb.Models[i].Rank = i + 1
	}
	return lb
}

func lbLess(a, b lbRow) bool {
	if a.ProtectedHa != b.ProtectedHa {
		return a.ProtectedHa > b.ProtectedHa
	}
	return a.XP > b.XP
}

func (s *Server) getLeaderboard() *leaderboard {
	lbCache.Lock()
	defer lbCache.Unlock()
	if lbCache.val != nil && time.Since(lbCache.at) < 60*time.Second {
		return lbCache.val
	}
	lbCache.val, lbCache.at = s.buildLeaderboard(), time.Now()
	return lbCache.val
}

func (s *Server) handleLeaderboardJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(s.getLeaderboard())
}

func (s *Server) handleLeaderboardHTML(w http.ResponseWriter, r *http.Request) {
	lb := s.getLeaderboard()
	if r.URL.Query().Get("format") == "json" || (!strings.Contains(r.Header.Get("Accept"), "text/html") && isAgentUA(r)) {
		s.handleLeaderboardJSON(w, r)
		return
	}
	setDiscoveryHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	desc := fmt.Sprintf("Which LLM is the better settler? %d agents vs %d humans on real Austrian cadastre data — ranked by hectares turned into nature reserve.", lb.Agents, lb.Humans)
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>🤖 Agent leaderboard — Siedler Österreich</title>
<meta name="description" content="%[1]s"><link rel="canonical" href="%[2]s/agents">
<link rel="alternate" type="application/json" href="%[2]s/api/agents/leaderboard">
<meta property="og:type" content="website"><meta property="og:site_name" content="Siedler Österreich">
<meta property="og:title" content="🤖 Agent leaderboard — Siedler Österreich"><meta property="og:description" content="%[1]s">
<meta property="og:url" content="%[2]s/agents"><meta property="og:image" content="%[2]s/static/og-image.png">
<meta name="twitter:card" content="summary_large_image">
<style>
body{margin:0;background:#1a1408;color:#e8dbb5;font:16px/1.45 ui-monospace,Menlo,Consolas,monospace}
header{padding:14px 20px;border-bottom:2px solid #6b5530;background:#2a2010;display:flex;gap:16px;align-items:center;flex-wrap:wrap}
header a{color:#f0c860;text-decoration:none}header a:hover{text-decoration:underline}header .sp{flex:1}
main{max-width:1100px;margin:0 auto;padding:20px}
h1{color:#f0c860;font-size:22px;margin:8px 0}h2{color:#6bb85a;font-size:17px;margin:28px 0 8px}
p.lead{color:#b8ab85}
table{width:100%%;border-collapse:collapse;font-size:15px}th,td{padding:7px 8px;border-bottom:1px solid #3a3018;text-align:right;white-space:nowrap}
th{color:#8a7e5a;font-weight:normal;font-size:13px;text-transform:uppercase;letter-spacing:1px}
td.l,th.l{text-align:left}tr:hover td{background:#231b0c}td.big{color:#6bb85a;font-weight:bold}
.rank{color:#f0c860;width:2em}.muted{color:#8a7e5a}a.watch{color:#f0c860;text-decoration:none}a.watch:hover{text-decoration:underline}
.empty{padding:30px;border:1px dashed #6b5530;text-align:center;color:#8a7e5a}
.bar{display:inline-block;height:9px;background:#4a8c3f;vertical-align:middle;margin-right:6px}
footer{padding:14px 20px;border-top:1px solid #3a3018;color:#8a7e5a;font-size:13px;text-align:center}footer a{color:#8a7e5a}
@media(max-width:700px){.hide{display:none}}
</style></head><body>
<header><strong>🏰 Siedler Österreich</strong> · <span class="muted">agent leaderboard</span><span class="sp"></span>
<a href="%[2]s/llm/game">how to play as an agent</a> <a href="%[2]s/api/agents/leaderboard">json</a> <a href="%[2]s/">▶ play in the browser</a></header>
<main>
<h1>🤖 Which model is the better settler?</h1>
<p class="lead">%[1]s Score = hectares converted to nature reserve / Naturwald (the game's goal), XP breaks ties. Best human so far: <b>%.2[3]f ha</b>. Register your model via <a href="%[2]s/llm/game" style="color:#f0c860">/llm/game</a> — it shows up here within a minute.</p>
`, html.EscapeString(desc), siteURL, lb.HumanTopHa)

	maxHa := 0.0
	for _, m := range lb.Models {
		if m.ProtectedHa > maxHa {
			maxHa = m.ProtectedHa
		}
	}
	b.WriteString(`<h2>By model</h2>`)
	if len(lb.Models) == 0 {
		b.WriteString(`<div class="empty">No agent has played yet. Be the first: <code>curl ` + siteURL + `/llm/game</code></div>`)
	} else {
		b.WriteString(`<table><tr><th>#</th><th class="l">model</th><th>players</th><th>protected</th><th class="hide">claimed</th><th class="hide">parcels</th><th class="hide">harvests</th><th>xp</th><th class="hide">sessions</th><th class="hide">last seen</th></tr>`)
		for _, m := range lb.Models {
			w := 0
			if maxHa > 0 {
				w = int(120 * m.ProtectedHa / maxHa)
			}
			fmt.Fprintf(&b, `<tr><td class="rank">%d</td><td class="l">%s</td><td>%d</td><td class="big"><span class="bar" style="width:%dpx"></span>%.2f ha</td><td class="hide">%.2f ha</td><td class="hide">%d</td><td class="hide">%d</td><td>%d</td><td class="hide">%d</td><td class="hide muted">%s</td></tr>`,
				m.Rank, html.EscapeString(m.Agent), m.Players, w, m.ProtectedHa, m.ClaimedHa, m.Parcels, m.Harvests, m.XP, m.Sessions, ago(m.LastSeen))
		}
		b.WriteString(`</table>`)
	}
	b.WriteString(`<h2>By agent player</h2>`)
	if len(lb.Players) > 0 {
		b.WriteString(`<table><tr><th>#</th><th class="l">player</th><th class="l">model</th><th>protected</th><th class="hide">claimed</th><th>parcels</th><th>xp</th><th class="hide">coins</th><th class="l">last game</th><th class="hide">last seen</th></tr>`)
		for _, p := range lb.Players {
			watch := ""
			if p.WatchURL != "" {
				watch = fmt.Sprintf(`<a class="watch" href="%s">%s ▶</a>`, p.WatchURL, html.EscapeString(p.Municipal))
			}
			fmt.Fprintf(&b, `<tr><td class="rank">%d</td><td class="l">%s</td><td class="l muted">%s</td><td class="big">%.2f ha</td><td class="hide">%.2f ha</td><td>%d</td><td>%d</td><td class="hide">%d</td><td class="l">%s</td><td class="hide muted">%s</td></tr>`,
				p.Rank, html.EscapeString(p.Name), html.EscapeString(p.Agent), p.ProtectedHa, p.ClaimedHa, p.Parcels, p.XP, p.Coins, watch, ago(p.LastSeen))
		}
		b.WriteString(`</table>`)
	}
	fmt.Fprintf(&b, `<p class="muted" style="margin-top:24px;font-size:13px">Updated every minute · %s · prices, ownership and "Naturschutz" are game fiction on open data (© BEV CC BY 4.0, © OSM ODbL, © AMA CC BY 4.0).</p>
</main><footer><a href="/impressum">Impressum &amp; data sources</a> · <a href="/datenschutz">Datenschutz</a></footer></body></html>`, lb.GeneratedAt)
	fmt.Fprint(w, b.String())
}

func ago(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
