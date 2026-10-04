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
	Icon        string   `json:"-"`
	NameEN      string   `json:"name_en"`
	ViaEN       string   `json:"-"`
	ModifiedEN  string   `json:"-"`
	UsedForEN   []string `json:"used_for_en"`
	NotesEN     string   `json:"-"`
	FetchedEN   string   `json:"-"`
}

func licenseSources() []licenseSource {
	return []licenseSource{
		{
			ID: "bev_kataster_tiles", Icon: "🗺️", NameEN: "Cadastre (parcels, building footprints, land-use classes)",
			FetchedEN:   "https://kataster.bev.gv.at (vector tiles of the cadastral map, z15 parcels / z16 land-use areas)",
			ViaEN:       "bevdirect-serve on this server (127.0.0.1:8787): assembles the tiles live for the visible 0.02° grid cell; no parcel database, no access to the Katasterservice (JSON API, §76c)",
			ModifiedEN:  "assembled from tiles, merged, coordinates rounded, enriched with area / land-use per parcel",
			UsedForEN:   []string{"Parcel polygons", "Building footprints", "Land-use areas", "Folio (EZ) grouping in view", "Municipality/KG at a point"},
			NotesEN:     "Legal basis: product description “Katastralmappe VTC” (BEV_S_KA_Katastralmappe_VTC.pdf) section 2.3 Licence: “The standard licence Creative Commons CC BY 4.0 applies to the products described here” – the tileset https://kataster.bev.gv.at/tiles/{kataster|symbole}/{z}/{x}/{y}.pbf is that product. CC BY 4.0 section 4 explicitly grants the sui generis database right (§§ 76c–76e Austrian Copyright Act): extracting, reusing, reproducing and sharing substantial parts of the database is licensed. We do not use the paid DKM (digital cadastral map with attribute data) nor the Katasterservice (search / information API). Assembled grid cells stay in our cache for at most 24 h (api_cache, kg_warm) and are pruned daily; bevdirect keeps tiles 24 h and cells 6 h. Nothing can be looked up by parcel number, folio or owner. BEV note: no legally binding coordinates can be derived from the tiles – the game is not an official register extract.",
			Name:        "Kataster (Grundstücke, Gebäude-Grundrisse, Benützungsarten)",
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
			ID: "bev_vgd", Icon: "🏛️", NameEN: "Administrative boundaries (cadastral-community & municipality register with outlines)",
			FetchedEN: "BEV OGD administrative boundaries (VGD) 1:50 000, as of 2026-04-01 (embedded: srv/data/admin.json.gz)",
			ViaEN:     "embedded table", ModifiedEN: "reduced to KG code, name, municipality, outline",
			UsedForEN: []string{"KG neighbourhood", "Warm-up plan", "Random municipality", "KG names"},
			Name:      "Verwaltungsgrenzen (KG- und Gemeinde-Register mit Umgriffen)",
			Provider:  "BEV", FetchedFrom: "BEV OGD Verwaltungsgrenzen (VGD) 1:50 000, Stichtag 2026-04-01 (eingebettet: srv/data/admin.json.gz)",
			Via: "eingebettete Tabelle", License: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de",
			Attribution: "Datenquelle: BEV – Verwaltungsgrenzen (VGD) 1:50 000, CC BY 4.0, bearbeitet",
			Modified:    "auf KG-Code, Name, Gemeinde, Umgriff reduziert", UsedFor: []string{"KG-Nachbarschaft", "Vorwärmplan", "Zufallsgemeinde", "KG-Namen"}, MaxCacheH: 0,
		},
		{
			ID: "bev_als_srtm", Icon: "⛰️", NameEN: "Terrain, land cover, tree & building heights, relief",
			FetchedEN:   "https://srtm-lidar-at.exe.xyz/api/v1 (public tier: bbox / point / KG code, no parcel data)",
			ViaEN:       "/api/viewport (25 m height field per cell → elevation, slope, aspect, land cover per parcel), /api/trees, /api/buildings, /api/landmarks, /api/landscape, /api/lidar/kg, /api/tiles/hillshade",
			ModifiedEN:  "segmented, aggregated to a 25 m grid, averaged per parcel",
			UsedForEN:   []string{"Elevation / slope shading", "Ground colour (dominant natural cover)", "Giant trees", "Building heights & roof shape", "Landmarks", "Relief tiles"},
			Name:        "Gelände, Landbedeckung, Baum- und Gebäudehöhen, Relief",
			Provider:    "BEV ALS DTM/DSM 1 m & DOP (CC BY 4.0, bearbeitet); Copernicus Sentinel-1/-2 (ESA); ESA WorldCover (CC BY 4.0); Hansen/UMD Global Forest Change (CC BY 4.0)",
			FetchedFrom: "https://srtm-lidar-at.exe.xyz/api/v1 (öffentliche Schicht: bbox/Punkt/KG-Code, keine Grundstücksdaten)",
			Via:         "/api/viewport (25-m-Höhenfeld je Zelle → Höhe, Hangneigung, Exposition, Landbedeckung je Grundstück), /api/trees, /api/buildings, /api/landmarks, /api/landscape, /api/lidar/kg, /api/tiles/hillshade",
			License:     "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
			Attribution: "Datenquelle: BEV – ALS DTM/DSM & DOP (CC BY 4.0, bearbeitet); Contains modified Copernicus Sentinel data; © ESA WorldCover; Hansen/UMD/Google/USGS/NASA GFC; srtm-lidar-at landscape segmentation (CC BY 4.0)",
			Modified:    "segmentiert, auf 25-m-Raster aggregiert, je Grundstück gemittelt", UsedFor: []string{"Höhen-/Hangschattierung", "Bodenfarbe (dominante natürliche Bedeckung)", "Riesenbäume", "Gebäudehöhen & Dachform", "Landmarken", "Relief-Kacheln"}, MaxCacheH: 24,
		},
		{
			ID: "umfeld_context", Icon: "🧭", NameEN: "Surroundings: municipalities & land prices, Natura 2000, protected areas, OSM proximity, field names, legal references",
			FetchedEN:   "https://umfeld-at.exe.xyz/api/v1 (public tier, point / bbox / code only – no cadastre)",
			ViaEN:       "/api/parcel-context (one call per clicked parcel, with our own area figures), /api/osm-lines, /api/n2k, /api/municipalities, /api/umfeld/{lookup,toponyms,legal,…}",
			ModifiedEN:  "prices are model values; distances computed for the requested point",
			UsedForEN:   []string{"Municipality picker & outlines", "Market-value estimate", "Natura 2000 overlay & bonus treasures", "Road / rail / water lines", "Transit stops & addresses (proximity)", "Field & place names", "Legal references"},
			NotesEN:     "OSM-derived fields are stored separately (ODbL share-alike).",
			Name:        "Umfeld: Gemeinden & Bodenpreise, Natura 2000, Schutzgebiete, OSM-Nähe, Flurnamen, Rechtsbezüge",
			Provider:    "Statistik Austria (CC BY 4.0) · EEA Natura 2000 · WDPA – UNEP-WCMC & IUCN (nicht-kommerziell) · OpenStreetMap-Mitwirkende (ODbL 1.0) · BEV Geographische Namen DLM (CC BY 4.0) · RIS, Bundeskanzleramt (CC BY 4.0)",
			FetchedFrom: "https://umfeld-at.exe.xyz/api/v1 (öffentliche Schicht, nur Punkt/bbox/Code – kein Kataster)",
			Via:         "/api/parcel-context (ein Aufruf je angeklicktem Grundstück mit unseren eigenen Flächenangaben), /api/osm-lines, /api/n2k, /api/municipalities, /api/umfeld/{lookup,toponyms,legal,…}",
			License:     "je Quelle (siehe attribution)", LicenseURL: "https://umfeld-at.exe.xyz/api/v1/license",
			Attribution: "Datenquelle: STATISTIK AUSTRIA · Source: European Environment Agency (EEA), Natura 2000 · UNEP-WCMC & IUCN (WDPA) · © OpenStreetMap-Mitwirkende (ODbL) · Datenquelle: BEV – Geographische Namen (DLM), CC BY 4.0, bearbeitet · Datenquelle: RIS, Bundeskanzleramt",
			Modified:    "Preise sind Modellwerte; Distanzen für den angefragten Punkt berechnet", UsedFor: []string{"Gemeindewahl & Umrisse", "Marktwert-Schätzung", "Natura-2000-Overlay & Bonus-Schätze", "Straßen/Bahn/Gewässer-Linien", "Haltestellen & Adressen (Nähe)", "Ried- & Ortsnamen", "Gesetzesbezüge"}, MaxCacheH: 24,
			Notes: "OSM-abgeleitete Felder bleiben getrennt gespeichert (ODbL share-alike).",
		},
		{ID: "ama_invekos", Icon: "🚜", NameEN: "Fields (INVEKOS Schläge), farmsteads & municipal subsidy profiles", FetchedEN: "https://farm-subsidies-austria.exe.xyz (INSPIRE INVEKOS via data.gv.at; AMA transparency database as municipal aggregates only)", ViaEN: "/api/schlaege, /api/hofstellen, /api/dossier, /api/field-economy", ModifiedEN: "aggregated, no names, farmsteads only as hashed points; subsidy profiles per municipality, no recipient data", UsedForEN: []string{"Crop per field", "Farmstead sprites", "Harvest economy", "Subsidy per ha (Chronik)"},
			Name: "Feldstücke (INVEKOS-Schläge), Hofstellen & Förderprofile", Provider: "AMA / BML via data.gv.at (INSPIRE-INVEKOS); AMA Transparenzdatenbank", FetchedFrom: "https://farm-subsidies-austria.exe.xyz (INSPIRE-INVEKOS über data.gv.at; AMA Transparenzdatenbank nur als Gemeinde-Aggregate)", Via: "/api/schlaege, /api/hofstellen, /api/dossier, /api/field-economy", License: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de", Attribution: "Datenquelle: AMA / BML – INSPIRE-INVEKOS (data.gv.at), CC BY 4.0, aggregiert · AMA Transparenzdatenbank, CC BY 4.0, Gemeinde-Aggregate", Modified: "aggregiert, ohne Namen, Hofstellen nur als gehashte Punkte; Förderprofile je Gemeinde, keine Empfängerdaten", UsedFor: []string{"Anbau je Feld", "Hofstellen-Sprites", "Ernte-Ökonomie", "Förderung je ha (Chronik)"}, MaxCacheH: 24},
		{ID: "holz", Icon: "🪵", NameEN: "Timber harvest & prices, forest loss, carbon balance", FetchedEN: "https://holzeinschlag-at.exe.xyz", ViaEN: "/api/forest-value, /api/dossier", ModifiedEN: "model values, no offers; forest loss and carbon flux at KG / plot level", UsedForEN: []string{"Timber revenue", "Forest history", "CO₂ stored"},
			Name: "Holzeinschlag & Holzpreise, Waldverlust, Kohlenstoffbilanz", Provider: "Statistik Austria (CC BY 4.0) · LK-Holzmarktberichte (preise.agrarforschung.at) · Hansen et al. Global Forest Change 2024 v1.12 (CC BY 4.0) · Harris et al. Forest Carbon Flux, Global Forest Watch (CC BY 4.0)", FetchedFrom: "https://holzeinschlag-at.exe.xyz", Via: "/api/forest-value, /api/dossier", License: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/", Attribution: "Statistik Austria · LK-Holzmarktberichte · Hansen/UMD/Google/USGS/NASA Global Forest Change · Harris et al. (Global Forest Watch)", Modified: "Modellwerte, keine Angebote; Waldverlust und Kohlenstoffbilanz auf KG-/Flächenebene", UsedFor: []string{"Holzerlös", "Waldgeschichte", "CO₂-Speicher"}, MaxCacheH: 24},
		{ID: "gw", Icon: "💧", NameEN: "Groundwater, gauges, nitrate, water protection zones, drought, flow paths", FetchedEN: "https://groundwater-at.exe.xyz", ViaEN: "/api/water/*, /api/well-quote, /api/dossier", ModifiedEN: "groundwater index, drought level and game rules derived; flow path snapped to drawn watercourses", UsedForEN: []string{"Drought chip", "Gauging stations", "Nitrate & groundwater index", "Water-protection overlay", "Well", "Water-drop journey"},
			Name: "Grundwasser, Pegel, Nitrat, Wasserschutzgebiete, Dürre, Fließwege", Provider: "BML eHYD (CC BY 4.0) · Wasserschatz Österreich 2021 · WISE / EEA (CC BY 4.0) · Copernicus European Drought Observatory (Dürreindex) · MERIT Hydro / MERIT-Basins via mghydro.com (CC BY-NC-SA 4.0, nur Darstellung)", FetchedFrom: "https://groundwater-at.exe.xyz", Via: "/api/water/*, /api/well-quote, /api/dossier", License: "CC BY 4.0 / CC BY-NC-SA 4.0 (Fließweg)", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de", Attribution: "BML eHYD · Wasserschatz Österreich 2021 · WISE/EEA · Copernicus EDO · MERIT Hydro (mghydro.com)", Modified: "Grundwasserindex, Dürrestufe und Spielregeln abgeleitet; Fließweg auf gezeichnete Gewässer gesnappt", UsedFor: []string{"Dürre-Chip", "Messstellen", "Nitrat & Grundwasserindex", "Wasserschutz-Overlay", "Brunnen", "Wassertropfen-Reise"}, MaxCacheH: 6},
		{ID: "redlist", Icon: "🦋", NameEN: "European Red List species (treasures)", FetchedEN: "embedded species list (srv/server.go: redListSpecies)", ViaEN: "embedded", ModifiedEN: "selection of species occurring in Austria with their European Red List category; placed as game treasures", UsedForEN: []string{"Species treasures", "Biodiversity quests"},
			Name: "Arten der Europäischen Roten Liste (Schätze)", Provider: "European Red List – IUCN im Auftrag der Europäischen Kommission, veröffentlicht über die EEA", FetchedFrom: "eingebettete Artenliste (srv/server.go: redListSpecies)", Via: "eingebettet", License: "Faktendaten (Artname, europäische Gefährdungskategorie); EU-Weiterverwendung / IUCN-Nutzungsbedingungen", LicenseURL: "https://www.iucnredlist.org/regions/europe", Attribution: "IUCN European Red List (European Commission / EEA)", Modified: "Auswahl in Österreich vorkommender Arten mit europäischer Gefährdungskategorie; als Spielschätze platziert", UsedFor: []string{"Arten-Schätze", "Biodiversitäts-Quests"}, MaxCacheH: 0},
		{ID: "border", Icon: "🇦🇹", NameEN: "National border", FetchedEN: "static (srv/static/austria.json)", ViaEN: "static", UsedForEN: []string{"Abroad shading", "Border line"},
			Name: "Staatsgrenze", Provider: "geoBoundaries gbOpen (ADM0)", FetchedFrom: "statisch (srv/static/austria.json)", Via: "statisch", License: "CC BY-SA 4.0", LicenseURL: "https://creativecommons.org/licenses/by-sa/4.0/", Attribution: "geoBoundaries gbOpen, CC BY-SA 4.0 (vereinfacht)", UsedFor: []string{"Auslands-Schattierung", "Grenzlinie"}, MaxCacheH: 0},
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
