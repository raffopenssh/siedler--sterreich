package srv

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

const siteURL = "https://siedler-oesterreich.exe.xyz:8000"

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprintf(w, "# Agents: the text edition of this game lives at %s/llm/game (see also /llms.txt, /openapi.json)\n"+
		"# Data sources & licences: %s/lizenzen (EN: /licenses, JSON: /api/licenses)\n"+
		"# Game sessions (and agents playing) carry BEV cadastre tiles — never index them.\n"+
		"User-agent: *\nAllow: /$\nAllow: /lizenzen\nAllow: /licenses\nAllow: /impressum\nAllow: /imprint\nAllow: /datenschutz\nAllow: /privacy\n"+
		"Allow: /llm/game\nAllow: /llms.txt\nAllow: /openapi.json\nAllow: /agents\nAllow: /static/\nAllow: /og-image\nAllow: /sitemap.xml\n"+
		"Disallow: /api/\nDisallow: /admin/\nDisallow: /llm/ahead\nDisallow: /join/\nDisallow: /rejoin/\nDisallow: /*?*sid=\nDisallow: /*?*pid=\nDisallow: /*?*rejoin=\nDisallow: /*?*invite=\nDisallow: /\n\nSitemap: %s/sitemap.xml\n", siteURL, siteURL, siteURL)
}

// sitemapEntry is one indexable page. Alt holds hreflang → path for pages
// that exist in both languages (every alternate is listed on every variant,
// as the sitemaps protocol requires).
type sitemapEntry struct {
	Path, Changefreq, Priority string
	Alt                        map[string]string
}

var legalAlt = func(de, en string) map[string]string {
	return map[string]string{"de": de, "en": en, "x-default": de}
}

var sitemapEntries = []sitemapEntry{
	{Path: "/", Changefreq: "weekly", Priority: "1.0"},
	{Path: "/lizenzen", Changefreq: "monthly", Priority: "0.6", Alt: legalAlt("/lizenzen", "/licenses")},
	{Path: "/licenses", Changefreq: "monthly", Priority: "0.6", Alt: legalAlt("/lizenzen", "/licenses")},
	{Path: "/llm/game", Changefreq: "weekly", Priority: "0.8"},
	{Path: "/llms.txt", Changefreq: "weekly", Priority: "0.5"},
	{Path: "/agents", Changefreq: "hourly", Priority: "0.7"},
	{Path: "/impressum", Changefreq: "monthly", Priority: "0.3", Alt: legalAlt("/impressum", "/imprint")},
	{Path: "/imprint", Changefreq: "monthly", Priority: "0.3", Alt: legalAlt("/impressum", "/imprint")},
	{Path: "/datenschutz", Changefreq: "monthly", Priority: "0.3", Alt: legalAlt("/datenschutz", "/privacy")},
	{Path: "/privacy", Changefreq: "monthly", Priority: "0.3", Alt: legalAlt("/datenschutz", "/privacy")},
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\" xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n")
	for _, e := range sitemapEntries {
		b.WriteString("  <url>\n")
		fmt.Fprintf(&b, "    <loc>%s</loc>\n", html.EscapeString(siteURL+e.Path))
		for _, lang := range []string{"de", "en", "x-default"} {
			if p, ok := e.Alt[lang]; ok {
				fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s\"/>\n", lang, html.EscapeString(siteURL+p))
			}
		}
		fmt.Fprintf(&b, "    <changefreq>%s</changefreq>\n    <priority>%s</priority>\n  </url>\n", e.Changefreq, e.Priority)
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprint(w, b.String())
}

func (s *Server) handleOGImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	fmt.Fprint(w, `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#1a1408"/>
      <stop offset="100%" stop-color="#2a2010"/>
    </linearGradient>
    <linearGradient id="gold" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#d4a843"/>
      <stop offset="100%" stop-color="#f0c860"/>
    </linearGradient>
    <linearGradient id="green" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#4a8c3f"/>
      <stop offset="100%" stop-color="#6bb85a"/>
    </linearGradient>
  </defs>
  <rect width="1200" height="630" fill="url(#bg)"/>
  <rect x="20" y="20" width="1160" height="590" rx="12" fill="none" stroke="#6b5530" stroke-width="3"/>
  <rect x="30" y="30" width="1140" height="570" rx="8" fill="none" stroke="#3a3018" stroke-width="1.5"/>
  <g opacity="0.08" stroke="#d4a843" stroke-width="0.5">
    <line x1="200" y1="400" x2="400" y2="380"/><line x1="400" y1="380" x2="500" y2="420"/>
    <line x1="500" y1="420" x2="350" y2="460"/><line x1="350" y1="460" x2="200" y2="400"/>
    <line x1="700" y1="450" x2="850" y2="410"/><line x1="850" y1="410" x2="900" y2="460"/>
  </g>
  <circle cx="180" cy="520" r="60" fill="#4a8c3f" opacity="0.12"/>
  <circle cx="800" cy="530" r="70" fill="#4a8c3f" opacity="0.10"/>
  <text x="600" y="210" text-anchor="middle" font-size="100" fill="#f0c860" font-family="serif" opacity="0.9">&#x1F3F0;</text>
  <text x="600" y="310" text-anchor="middle" font-family="monospace" font-size="56" fill="url(#gold)" letter-spacing="8" font-weight="bold">SIEDLER</text>
  <text x="600" y="375" text-anchor="middle" font-family="monospace" font-size="36" fill="#e8dbb5" letter-spacing="12">&#214;STERREICH</text>
  <line x1="350" y1="405" x2="850" y2="405" stroke="url(#gold)" stroke-width="2" opacity="0.6"/>
  <text x="600" y="450" text-anchor="middle" font-family="monospace" font-size="28" fill="url(#green)">Entdecke und sch&#252;tze &#214;sterreichs Natur</text>
  <text x="600" y="500" text-anchor="middle" font-family="monospace" font-size="20" fill="#8a7e5a">Echte Katasterdaten &#183; Multiplayer &#183; Biodiversit&#228;t sch&#252;tzen</text>
  <text x="600" y="575" text-anchor="middle" font-family="monospace" font-size="18" fill="#6b5530">siedler-oesterreich.exe.xyz</text>
</svg>`)
}

// sessionURL reports whether a request addresses a running game (invite /
// rejoin links, session or player ids in the query). Those pages render BEV
// cadastre tiles and player state and must never land in a search index.
func sessionURL(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasPrefix(p, "/join/") || strings.HasPrefix(p, "/rejoin/") || strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/admin/") {
		return true
	}
	q := r.URL.Query()
	for _, k := range []string{"sid", "pid", "rejoin", "invite", "pname"} {
		if q.Has(k) {
			return true
		}
	}
	return false
}

// noindexMiddleware stamps X-Robots-Tag: noindex on every session / API
// response so the HTML <meta robots> of index.html cannot leak a game into
// an index via an invite or rejoin link.
func noindexMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sessionURL(r) {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		}
		next.ServeHTTP(w, r)
	})
}
