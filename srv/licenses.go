package srv

// Data provenance + caching policy, machine- and human-readable.
//
//   GET /api/licenses  — JSON: every source, where we fetch it, licence,
//                        attribution text, our max cache age.
//   GET /lizenzen      — the same as a page (linked from Impressum + map).
//
// Policy in one line: the cadastre is assembled live from the BEV vector
// tiles (kataster.bev.gv.at, CC BY 4.0) by bevdirect-serve on this VM, and
// nothing derived from it is kept longer than 24 hours. We hold no cadastre
// database; cells expire, the warm registry (kg_warm) expires with them.

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"time"
)

type licenseSource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	FetchedFrom string   `json:"fetched_from"`
	Via         string   `json:"via"`
	License     string   `json:"license"`
	LicenseURL  string   `json:"license_url"`
	Attribution string   `json:"attribution"`
	Modified    string   `json:"modified"`
	UsedFor     []string `json:"used_for"`
	MaxCacheH   float64  `json:"max_cache_hours"`
	Notes       string   `json:"notes,omitempty"`
}

func licenseSources() []licenseSource {
	return []licenseSource{
		{
			ID: "bev_kataster_tiles", Name: "Kataster (Grundstücke, Gebäude-Grundrisse, Benützungsarten)",
			Provider:    "BEV – Bundesamt für Eich- und Vermessungswesen",
			FetchedFrom: "https://kataster.bev.gv.at (Vektorkacheln der Katastralmappe, z15 Grundstücke / z16 Nutzungsflächen)",
			Via:         "bevdirect-serve auf diesem Server (127.0.0.1:8787): setzt die Kacheln für den sichtbaren 0,02°-Rasterausschnitt live zusammen; keine Grundstücksdatenbank, kein Zugriff auf das Katasterservice (JSON-API, §76c)",
			License:     "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de",
			Attribution: bevNotice,
			Modified:    "zusammengesetzt aus Kacheln, vereinigt, Koordinaten gerundet, mit Flächen/Benützungsart je Grundstück angereichert",
			UsedFor:     []string{"Grundstückspolygone", "Gebäudegrundrisse", "Benützungsarten-Flächen", "EZ-Gruppierung im Sichtbereich", "Gemeinde/KG am Punkt"},
			MaxCacheH:   24,
			Notes:       "Rechtsgrundlage: Produktbeschreibung „Katastralmappe VTC“ (BEV_S_KA_Katastralmappe_VTC.pdf) Abschnitt 2.3 Lizenz: „Für die hier beschriebenen Produkte gilt die Standardlizenz Creative Commons CC BY 4.0“ – das Tileset https://kataster.bev.gv.at/tiles/{kataster|symbole}/{z}/{x}/{y}.pbf ist dieses Produkt. CC BY 4.0 Abschnitt 4 räumt ausdrücklich auch das Sui-generis-Datenbankrecht (§§ 76c–76e UrhG) ein: Entnahme, Weiterverwendung, Vervielfältigung und Weitergabe wesentlicher Teile der Datenbank sind lizenziert. Die kostenpflichtige DKM (Katastralmappe und Sachdaten digital, BEV-Nutzungsbedingungen Amtsblatt 3/2022) und das Katasterservice (Such-/Auskunfts-API) nutzen wir nicht. Zusammengesetzte Rasterzellen werden höchstens 24 h in unserem Zwischenspeicher gehalten (api_cache, kg_warm) und täglich ausgedünnt; bevdirect hält Kacheln 24 h, Zellen 6 h. Nichts ist nach Grundstücksnummer, EZ oder Eigentümer abfragbar. Hinweis BEV: aus den Kacheln können keine rechtsverbindlichen Koordinaten abgeleitet werden – das Spiel ist keine amtliche Auskunft.",
		},
		{
			ID: "bev_vgd", Name: "Verwaltungsgrenzen (KG- und Gemeinde-Register mit Umgriffen)",
			Provider: "BEV", FetchedFrom: "BEV OGD Verwaltungsgrenzen (VGD) 1:50 000, Stichtag 2026-04-01 (eingebettet: srv/data/admin.json.gz)",
			Via: "eingebettete Tabelle", License: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de",
			Attribution: "Datenquelle: BEV – Verwaltungsgrenzen (VGD) 1:50 000, CC BY 4.0, bearbeitet",
			Modified:    "auf KG-Code, Name, Gemeinde, Umgriff reduziert", UsedFor: []string{"KG-Nachbarschaft", "Vorwärmplan", "Zufallsgemeinde", "KG-Namen"}, MaxCacheH: 0,
		},
		{
			ID: "bev_als_srtm", Name: "Gelände, Landbedeckung, Baum- und Gebäudehöhen, Relief",
			Provider:    "BEV ALS DTM/DSM 1 m & DOP (CC BY 4.0, bearbeitet); Copernicus Sentinel-1/-2 (ESA); ESA WorldCover; Hansen GFC",
			FetchedFrom: "https://srtm-lidar-at.exe.xyz/api/v1 (öffentliche Schicht: bbox/Punkt/KG-Code, keine Grundstücksdaten)",
			Via:         "/api/viewport (25-m-Höhenfeld je Zelle → Höhe, Hangneigung, Exposition, Landbedeckung je Grundstück), /api/trees, /api/buildings, /api/landmarks, /api/landscape, /api/lidar/kg, /api/tiles/hillshade",
			License:     "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
			Attribution: "Datenquelle: BEV – ALS DTM/DSM & DOP (CC BY 4.0, bearbeitet); Contains modified Copernicus Sentinel data; © ESA WorldCover; Hansen/UMD/Google/USGS/NASA GFC; srtm-lidar-at landscape segmentation (CC BY 4.0)",
			Modified:    "segmentiert, auf 25-m-Raster aggregiert, je Grundstück gemittelt", UsedFor: []string{"Höhen-/Hangschattierung", "Bodenfarbe (dominante natürliche Bedeckung)", "Riesenbäume", "Gebäudehöhen & Dachform", "Landmarken", "Relief-Kacheln"}, MaxCacheH: 24,
		},
		{
			ID: "umfeld_context", Name: "Umfeld: Gemeinden & Bodenpreise, Natura 2000, Schutzgebiete, OSM-Nähe, Flurnamen, Rechtsbezüge",
			Provider:    "Statistik Austria (CC BY 4.0) · EEA Natura 2000 · WDPA (nicht-kommerziell) · OpenStreetMap (ODbL) · BEV DLM-Namen (CC BY 4.0) · RIS (CC BY 4.0)",
			FetchedFrom: "https://umfeld-at.exe.xyz/api/v1 (öffentliche Schicht, nur Punkt/bbox/Code – kein Kataster)",
			Via:         "/api/parcel-context (ein Aufruf je angeklicktem Grundstück mit unseren eigenen Flächenangaben), /api/osm-lines, /api/n2k, /api/municipalities, /api/cadastre/{lookup,toponyms,legal,…}",
			License:     "je Quelle (siehe attribution)", LicenseURL: "https://umfeld-at.exe.xyz/api/v1/license",
			Attribution: "Datenquelle: STATISTIK AUSTRIA · Source: European Environment Agency (EEA), Natura 2000 · UNEP-WCMC & IUCN (WDPA) · © OpenStreetMap-Mitwirkende (ODbL) · Datenquelle: BEV – Geographische Namen (DLM), CC BY 4.0, bearbeitet · Datenquelle: RIS, Bundeskanzleramt",
			Modified:    "Preise sind Modellwerte; Distanzen für den angefragten Punkt berechnet", UsedFor: []string{"Gemeindewahl & Umrisse", "Marktwert-Schätzung", "Natura-2000-Overlay & Bonus-Schätze", "Straßen/Bahn/Gewässer-Linien", "Ried- & Ortsnamen", "Gesetzesbezüge"}, MaxCacheH: 24,
			Notes: "OSM-abgeleitete Felder bleiben getrennt gespeichert (ODbL share-alike).",
		},
		{ID: "ama_invekos", Name: "Feldstücke (Schläge) & Hofstellen", Provider: "AMA / BML via data.gv.at", FetchedFrom: "https://farm-subsidies-austria.exe.xyz", Via: "/api/schlaege, /api/hofstellen, /api/dossier", License: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de", Attribution: "Datenquelle: AMA INVEKOS (data.gv.at), CC BY 4.0, aggregiert", Modified: "ohne Namen, Hofstellen als Punkt", UsedFor: []string{"Anbau je Feld", "Hofstellen-Sprites", "Ernte-Ökonomie"}, MaxCacheH: 24},
		{ID: "holz", Name: "Holzpreise, Waldgeschichte, CO₂", Provider: "Statistik Austria, LK-Holzmarktberichte, Hansen/GFW, Harris et al.", FetchedFrom: "https://holzeinschlag-at.exe.xyz", Via: "/api/forest-value, /api/dossier", License: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/", Attribution: "Statistik Austria · LK-Holzmarktberichte · Hansen/UMD GFC · Harris et al. (GFW)", Modified: "Modellwerte, keine Angebote", UsedFor: []string{"Holzerlös", "Waldgeschichte"}, MaxCacheH: 24},
		{ID: "gw", Name: "Grundwasser, Pegel, Wasserschutzgebiete, Fließwege", Provider: "BML eHYD, Wasserschatz 2021, WISE/EEA, MERIT Hydro (CC BY-NC-SA, nur Darstellung), Copernicus EDO", FetchedFrom: "https://groundwater-at.exe.xyz", Via: "/api/water/*, /api/dossier", License: "CC BY 4.0 / CC BY-NC-SA 4.0 (Fließweg)", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de", Attribution: "BML eHYD · Wasserschatz Österreich 2021 · WISE/EEA · MERIT Hydro (mghydro.com)", Modified: "Dürre-Index und Spielregeln abgeleitet", UsedFor: []string{"Dürre-Chip", "Messstellen", "Wasserschutz-Overlay", "Wassertropfen-Reise"}, MaxCacheH: 6},
		{ID: "border", Name: "Staatsgrenze", Provider: "geoBoundaries gbOpen (ADM0)", FetchedFrom: "statisch (srv/static/austria.json)", Via: "statisch", License: "CC BY-SA 4.0", LicenseURL: "https://creativecommons.org/licenses/by-sa/4.0/", Attribution: "geoBoundaries gbOpen, CC BY-SA 4.0 (vereinfacht)", UsedFor: []string{"Auslands-Schattierung", "Grenzlinie"}, MaxCacheH: 0},
	}
}

func (s *Server) licensesDoc() map[string]any {
	return map[string]any{
		"service":      "Siedler Österreich",
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"policy": map[string]any{
			"cadastre_source":  "BEV Katastralmappe – Vektorkacheln (kataster.bev.gv.at), CC BY 4.0, live zusammengesetzt durch bevdirect-serve auf diesem Server",
			"cadastre_max_age": "24h",
			"cadastre_storage": "keine Datenbank; Rasterzellen (0,02°) im Zwischenspeicher mit Ablauf ≤ 24 h; kg_warm-Register läuft mit ab; nichts ist nach Grundstücksnummer/EZ abfragbar",
			"prewarm":          fmt.Sprintf("%d Katastralgemeinden pro Tag in %d zusammenhängenden Gebieten; beim Laden einer KG werden Nachbar-KGs im Hintergrund vorgewärmt; alles verfällt nach 24 h", warmDailyKGs, warmPatches),
			"context_max_age":  "24h (Umfeld, Landschaft), 6h (Wasser)",
			"no_endorsement":   "Dieses Spiel wird nicht vom BEV, von Statistik Austria, der EEA, ESA oder OSM betrieben oder unterstützt.",
			"legal_basis": map[string]string{
				"bev_vtc_licence":    "BEV Produktbeschreibung Katastralmappe VTC, Abschnitt 2.3: CC BY 4.0 (https://www.bev.gv.at/Services/Geoinformationsdienste/Services/Katasterservice.html → Download BEV_S_KA_Katastralmappe_VTC.pdf)",
				"database_right":     "CC BY 4.0 legalcode Abschnitt 4 (Sui Generis Database Rights) lizenziert Entnahme und Weiterverwendung wesentlicher Teile; damit ist §76c ff UrhG für das VTC-Produkt abgedeckt. Unsere 24-h-Grenze und der Verzicht auf eine Gst./EZ-Abfrage gehen darüber hinaus.",
				"attribution_format": "„© BEV, JJJJ“ + Quelle + Lizenz + Bearbeitungshinweis (BEV Nutzungsbedingungen §2.3.3; CC BY 4.0 §3(a)); auf jeder Kartenansicht (#map-attrib), in /lizenzen, im Impressum und als `notice` in jeder Kataster-Antwort unserer API.",
				"non_commercial":     "Nicht-kommerzieller Dienst (Impressum). Nur dadurch zulässig: WDPA (nicht-kommerziell) und MERIT Hydro (CC BY-NC-SA 4.0; alternativ ODbL). Bei einer Kommerzialisierung müssten diese beiden Quellen entfallen.",
				"share_alike":        "geoBoundaries gbOpen (CC BY-SA 4.0): die vereinfachte Grenzdatei /static/austria.json wird unter CC BY-SA 4.0 weitergegeben. OSM-Linien werden nur dargestellt (Produced Work, ODbL §4.3).",
			},
		},
		"sources": licenseSources(),
		"links":   map[string]string{"impressum": "/impressum#daten", "page": "/lizenzen"},
	}
}

func (s *Server) handleLicenses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(s.licensesDoc())
}

func (s *Server) handleLicensesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html lang="de"><head><meta charset="utf-8"><title>Datenquellen & Lizenzen – Siedler Österreich</title>
<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/static/legal.css"></head><body class="legal"><main>
<h1>Datenquellen &amp; Lizenzen</h1>
<p><strong>Kataster:</strong> Grundstücke, Gebäudegrundrisse und Benützungsarten werden <em>live aus den Vektorkacheln der BEV-Katastralmappe</em>
(kataster.bev.gv.at, CC BY 4.0) für den gerade sichtbaren Ausschnitt zusammengesetzt – durch <code>bevdirect-serve</code> auf unserem Server.
Wir führen keine Katasterdatenbank: zusammengesetzte Rasterzellen (0,02°) bleiben <strong>höchstens 24 Stunden</strong> im Zwischenspeicher,
täglich werden `+fmt.Sprint(warmDailyKGs)+` Katastralgemeinden in `+fmt.Sprint(warmPatches)+` zusammenhängenden Gebieten vorgewärmt, beim Laden einer KG die Nachbar-KGs –
und alles verfällt wieder nach 24 h. Nichts ist nach Grundstücksnummer, Einlagezahl oder Eigentümer abfragbar.</p>
<p>`+html.EscapeString(bevNotice)+`</p>
<p><small><strong>Rechtsgrundlage Kataster:</strong> Die Vektorkacheln sind das BEV-Produkt „Katastralmappe VTC“; dessen
<a href="https://www.bev.gv.at/Services/Geoinformationsdienste/Services/Katasterservice.html" rel="noopener">Produktbeschreibung</a>
(BEV_S_KA_Katastralmappe_VTC.pdf, Abschnitt 2.3) stellt es unter CC BY 4.0. Diese Lizenz umfasst in Abschnitt 4 ausdrücklich das
Sui-generis-Datenbankrecht (§§ 76c–76e UrhG), also Entnahme und Weiterverwendung wesentlicher Teile. Die kostenpflichtige DKM und das
Katasterservice (Such-/Auskunfts-API) nutzen wir nicht. Aus den Kacheln lassen sich laut BEV keine rechtsverbindlichen Koordinaten ableiten –
das Spiel ist keine amtliche Auskunft. Dieser Dienst ist nicht-kommerziell; WDPA und MERIT Hydro (CC BY-NC-SA) sind nur deshalb zulässig.</small></p>
<table><thead><tr><th>Quelle</th><th>Woher wir laden</th><th>Lizenz</th><th>Verwendung</th><th>max. Cache</th></tr></thead><tbody>`)
	for _, src := range licenseSources() {
		cache := "–"
		if src.MaxCacheH > 0 {
			cache = fmt.Sprintf("%g h", src.MaxCacheH)
		}
		fmt.Fprintf(w, `<tr><td><strong>%s</strong><br><small>%s</small><br><small><em>%s</em></small></td><td>%s<br><small>%s</small></td><td><a href="%s" rel="license noopener">%s</a><br><small>%s</small></td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(src.Name), html.EscapeString(src.Provider), html.EscapeString(src.Attribution),
			html.EscapeString(src.FetchedFrom), html.EscapeString(src.Via),
			src.LicenseURL, html.EscapeString(src.License), html.EscapeString(src.Modified),
			html.EscapeString(joinStr(src.UsedFor, ", ")), cache)
	}
	fmt.Fprint(w, `</tbody></table>
<p><small>Maschinenlesbar: <a href="/api/licenses">/api/licenses</a>. Dieses Spiel wird nicht vom BEV, von Statistik Austria, der EEA, ESA oder OSM betrieben oder unterstützt.
Weitere Angaben im <a href="/impressum#daten">Impressum</a>.</small></p>
<p><a href="/">← zurück zum Spiel</a></p></main></body></html>`)
}

func joinStr(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
