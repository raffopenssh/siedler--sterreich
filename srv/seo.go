package srv

import (
	"fmt"
	"net/http"
)

const siteURL = "https://siedler-oesterreich.exe.xyz:8000"

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", siteURL)
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
		"<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n"+
		"  <url>\n"+
		"    <loc>%s/</loc>\n"+
		"    <changefreq>weekly</changefreq>\n"+
		"    <priority>1.0</priority>\n"+
		"  </url>\n"+
		"</urlset>\n", siteURL)
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
