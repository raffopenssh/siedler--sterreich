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

// bevdirectVersion is the vtcseamless release installed in /opt/bevdirect (see
// /opt/bevdirect/SOURCE.txt); bump together with the upgrade. /api/metrics
// reports the live value from bevdirect /health.
const bevdirectVersion = "v0.3.1"

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
			NotesEN:     "Legal basis: product description “Katastralmappe VTC” (BEV_S_KA_Katastralmappe_VTC.pdf) section 2.3 Licence: “The standard licence Creative Commons CC BY 4.0 applies to the products described here” – the tileset https://kataster.bev.gv.at/at.gv.bev.kataster/tiles/{kataster|symbole}/{z}/{x}/{y}.pbf is that product. CC BY 4.0 section 4 explicitly grants the sui generis database right (§§ 76c–76e Austrian Copyright Act) as well. We use neither the paid DKM (BEV terms of use, Amtsblatt 3/2022) nor the Katasterservice (search / information API). The tiles are assembled into parcels by a service of our own whose source code is open (MIT): https://github.com/raffopenssh/vtcseamless (preset bevdirect, version " + bevdirectVersion + "). Assembled grid cells are cached for at most 24 h and pruned continuously; raw tiles live only in the assembler's RAM (LRU, ≤ 24 h) and are never written to disk. Parcels and folios (EZ) can only be found inside a visible map extent, never without a location; the tiles contain no owner data. BEV note: no legally binding coordinates can be derived from the tiles – the game is not an official register extract. Every rendering carries the notice “© BEV, 2026 … CC BY 4.0, bearbeitet”.",
			Name:        "Kataster (Grundstücke, Gebäude-Grundrisse, Benützungsarten)",
			Provider:    "BEV – Bundesamt für Eich- und Vermessungswesen",
			FetchedFrom: "https://kataster.bev.gv.at (Vektorkacheln der Katastralmappe, z15 Grundstücke / z16 Nutzungsflächen)",
			Via:         "bevdirect-serve auf diesem Server (127.0.0.1:8787): setzt die Kacheln für den sichtbaren 0,02°-Rasterausschnitt live zusammen; keine Grundstücksdatenbank, kein Zugriff auf das Katasterservice (JSON-API, §76c)",
			License:     "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/deed.de",
			Attribution: bevNotice,
			Modified:    "zusammengesetzt aus Kacheln, vereinigt, Koordinaten gerundet, mit Flächen/Benützungsart je Grundstück angereichert",
			UsedFor:     []string{"Grundstückspolygone", "Gebäudegrundrisse", "Benützungsarten-Flächen", "EZ-Gruppierung im Sichtbereich", "Gemeinde/KG am Punkt"},
			MaxCacheH:   24,
			Notes:       "Rechtsgrundlage: Produktbeschreibung „Katastralmappe VTC“ (BEV_S_KA_Katastralmappe_VTC.pdf) Abschnitt 2.3 Lizenz: „Für die hier beschriebenen Produkte gilt die Standardlizenz Creative Commons CC BY 4.0“ – das Tileset https://kataster.bev.gv.at/at.gv.bev.kataster/tiles/{kataster|symbole}/{z}/{x}/{y}.pbf ist dieses Produkt. CC BY 4.0 Abschnitt 4 räumt ausdrücklich auch das Sui-generis-Datenbankrecht (§§ 76c–76e UrhG) ein. Die kostenpflichtige DKM (BEV-Nutzungsbedingungen Amtsblatt 3/2022) und das Katasterservice (Such-/Auskunfts-API) nutzen wir nicht. Die Kacheln werden von einem eigenen Dienst zu Parzellen zusammengesetzt; dessen Quellcode ist offen (MIT): https://github.com/raffopenssh/vtcseamless (Preset bevdirect, Version " + bevdirectVersion + "). Zusammengesetzte Rasterzellen werden höchstens 24 h zwischengespeichert und laufend ausgedünnt; rohe Kacheln hält nur der Assembler im Arbeitsspeicher (LRU, ≤ 24 h), auf die Platte werden sie nie geschrieben. Parzellen und Einlagezahlen sind nur innerhalb eines sichtbaren Kartenausschnitts auffindbar, nicht ohne Ortsangabe; Eigentümerdaten sind in den Kacheln nicht enthalten. Hinweis BEV: aus den Kacheln können keine rechtsverbindlichen Koordinaten abgeleitet werden – das Spiel ist keine amtliche Auskunft. Jede Darstellung trägt den Hinweis „© BEV, 2026 … CC BY 4.0, bearbeitet“.",
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
			FetchedEN:   "https://srtm-lidar-at.exe.xyz/api/v1 (keyed by bbox / point / KG code / H3 cell) — service page https://srtm-lidar-at.exe.xyz/, reference /api/v1/docs/llm.txt, licence /api/v1/license",
			ViaEN:       "/api/viewport (NE cells per 0.02° cell → cover, canopy, heights, consistency, every tree apex & structure per parcel; else the 25 m height field → elevation, slope, aspect, land cover), /api/ne, /api/trees, /api/buildings, /api/landmarks, /api/landscape, /api/lidar/kg, /api/tiles/hillshade",
			ModifiedEN:  "segmented, aggregated to H3 res-12 cells (~307 m²) and a 25 m grid, averaged / assigned per parcel by point-in-polygon",
			UsedForEN:   []string{"Elevation / slope shading", "Ground colour (dominant natural cover)", "Observed layer (Spähbericht, verdict, heat overlay)", "Giant trees & tree apices", "Building heights & roof shape", "Landmarks", "Relief tiles"},
			NotesEN:     "Upstream licence text (/api/v1/license): the segmentation classes, statistics, QA filters and the H3 cell layer are srtm-lidar-at's own work, CC BY 4.0 — credit “srtm-lidar-at landscape segmentation”. Inputs: BEV ALS DGM/DOM 1 m (mosaic epochs 2022-09-15 / 2023-09-15 / 2024-09-15), BEV Orthophoto DOP RGBI (series 2022-01-28 … 2025-04-15), Copernicus Sentinel-1/-2 2022–2025 via the Copernicus Data Space Ecosystem, ESA WorldCover 10 m 2021 v200 (Zanaga et al. 2022), Hansen Global Forest Change 2000–2024 v1.12 (Hansen et al. 2013). The `consistency` verdict compares the observed cell against the declared land-use statistics of umfeld-at's NE cells (BEV Kataster, CC BY 4.0, bearbeitet — statistics per cell, no object geometry). OSM-derived values are aggregate counts only (infrastructure_count, ODbL) and landmarks are cross-checked against austria-power (Austro Control, IG Windkraft, OSM ODbL) — we store neither. The original providers are not responsible for and do not endorse this derivative work; not operated by or affiliated with BEV, ESA or the Copernicus programme. Stage alpha.",
			Name:        "Gelände, Landbedeckung, Baum- und Gebäudehöhen, beobachtete Schicht (NE-Zellen), Relief",
			Provider:    "srtm-lidar-at (Segmentierung, Statistik, Zellschicht: CC BY 4.0) aus BEV ALS DGM/DOM 1 m & Orthophoto DOP RGBI (CC BY 4.0, bearbeitet) · Copernicus Sentinel-1/-2 2022–2025 (EU/ESA) · ESA WorldCover 10 m 2021 v200 (CC BY 4.0) · Hansen/UMD/Google/USGS/NASA Global Forest Change 2000–2024 v1.12 (CC BY 4.0) · OpenStreetMap-Mitwirkende (ODbL, nur Zählwerte)",
			FetchedFrom: "https://srtm-lidar-at.exe.xyz/api/v1 (abgefragt je bbox/Punkt/KG-Code/H3-Zelle) — Dienstseite https://srtm-lidar-at.exe.xyz/, Referenz /api/v1/docs/llm.txt, Lizenz /api/v1/license",
			Via:         "/api/viewport (NE-Zellen je 0,02°-Zelle → Bedeckung, Kronenanteil, Höhen, Abgleich, jede Baumspitze & jedes Bauwerk je Grundstück; sonst 25-m-Höhenfeld → Höhe, Hangneigung, Exposition, Landbedeckung), /api/ne, /api/trees, /api/buildings, /api/landmarks, /api/landscape, /api/lidar/kg, /api/tiles/hillshade",
			License:     "CC BY 4.0", LicenseURL: "https://srtm-lidar-at.exe.xyz/api/v1/license",
			Attribution: "Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen, ALS DGM/DOM 1 m & Orthophoto DOP RGBI (CC BY 4.0, bearbeitet) · Contains modified Copernicus Sentinel data 2022–2025 · © ESA WorldCover 2021 · Hansen/UMD/Google/USGS/NASA GFC 2000–2024 v1.12 · © OpenStreetMap-Mitwirkende (ODbL) · srtm-lidar-at landscape segmentation (CC BY 4.0) · Abgleich: BEV Kataster, CC BY 4.0, bearbeitet (Nutzungseinheit-Zellen, keine Objektgeometrie)",
			Modified:    "segmentiert, auf H3-Zellen (Res 12, ~307 m²) und 25-m-Raster aggregiert, je Grundstück gemittelt bzw. per Punkt-in-Polygon zugeordnet", UsedFor: []string{"Höhen-/Hangschattierung", "Bodenfarbe (dominante natürliche Bedeckung)", "Beobachtete Schicht (Spähbericht, Abgleich, Heat-Overlay)", "Riesenbäume & Baumspitzen", "Gebäudehöhen & Dachform", "Landmarken", "Relief-Kacheln"}, MaxCacheH: 24,
			Notes:       "Lizenztext des Dienstes (/api/v1/license): Segmentierungsklassen, Statistiken, QA-Filter und die H3-Zellschicht sind eigene Werke von srtm-lidar-at, CC BY 4.0 — Nennung „srtm-lidar-at landscape segmentation“. Eingangsdaten: BEV ALS DGM/DOM 1 m (Mosaik-Epochen 2022-09-15 / 2023-09-15 / 2024-09-15), BEV Orthophoto DOP RGBI (Serien 2022-01-28 … 2025-04-15), Copernicus Sentinel-1/-2 2022–2025 über das Copernicus Data Space Ecosystem, ESA WorldCover 10 m 2021 v200 (Zanaga et al. 2022), Hansen Global Forest Change 2000–2024 v1.12 (Hansen et al. 2013). Der Abgleich (`consistency`) vergleicht die beobachtete Zelle mit der deklarierten Nutzungsstatistik der NE-Zellen von umfeld-at (BEV Kataster, CC BY 4.0, bearbeitet — Statistik je Zelle, keine Objektgeometrie). OSM-Ableitungen sind nur Zählwerte (infrastructure_count, ODbL), Landmarken werden gegen austria-power (Austro Control, IG Windkraft, OSM ODbL) gegengeprüft — beides speichern wir nicht. Die ursprünglichen Anbieter sind für dieses abgeleitete Werk nicht verantwortlich und unterstützen es nicht; der Dienst wird nicht von BEV, ESA oder dem Copernicus-Programm betrieben. Stadium alpha.",
		},
		{
			ID: "umfeld_context", Icon: "🧭", NameEN: "Surroundings: municipalities & land prices, Natura 2000, protected areas, OSM proximity, field names, legal references, declared land use per cell (NE)",
			FetchedEN:   "https://umfeld-at.exe.xyz/api/v1 (keyed by point / bbox / code) — service page https://umfeld-at.exe.xyz/, reference /api/v1/docs/llm.txt, licence /api/v1/license",
			ViaEN:       "/api/parcel-context (one call per clicked parcel, with our own area figures), /api/osm-lines, /api/n2k, /api/municipalities, /api/umfeld/{lookup,toponyms,legal,…}, NE cells /api/v1/ne/{kg} · /ne/manifest (declared twin of the observed layer)",
			ModifiedEN:  "prices are model outputs (“bearbeitet/modelliert”, not official valuations); distances computed for the requested point; toponyms re-projected, ±50 m, never snapped to parcels",
			UsedForEN:   []string{"Municipality picker & outlines", "Market-value estimate", "Natura 2000 overlay & bonus treasures", "Road / rail / water lines", "Transit stops & addresses (proximity)", "Field & place names", "Legal references", "Declared land use per cell (NE, observed-vs-declared verdict)"},
			NotesEN:     "Per-source obligations from umfeld-at's /api/v1/license: Statistik Austria CC BY 4.0 (“Datenquelle: STATISTIK AUSTRIA”, price estimates are model outputs); EEA Natura 2000 (“Source: European Environment Agency (EEA)”); WDPA — attribution, NON-COMMERCIAL use only (we are non-commercial); OpenStreetMap ODbL (Geofabrik extract 2026-07-02, “© OpenStreetMap contributors”; osm{} fields stay separable, we only display them — ODbL §4.3 produced work); BEV DLM 7000 NAMEN, Stichtag 2025-03-25, CC BY 4.0, bearbeitet; RIS CC BY 4.0 (“Datenquelle: RIS, Bundeskanzleramt Österreich”). NE cells (/api/v1/ne/*, declared land-use statistics per H3 res-12 cell derived from BEV survey data): CC BY 4.0 — “© BEV (CC BY 4.0), bearbeitet · observed layer © srtm-lidar-at (CC BY 4.0)”; statistics per cell, no object id, number or geometry contained or recoverable; redistributable under CC BY 4.0 with that attribution. Not used by us: GADM (academic / non-commercial). No personal data, no parcel identifiers on either side. We contribute back: nightly NE epoch reports (digests per cell block only, observer siedler-oesterreich, see docs/ne-report.md) built from the cells we assemble for players anyway. Stage alpha.",
			Name:        "Umfeld: Gemeinden & Bodenpreise, Natura 2000, Schutzgebiete, OSM-Nähe, Flurnamen, Rechtsbezüge, deklarierte Nutzung je Zelle (NE)",
			Provider:    "umfeld-at (Code MIT) aus: Statistik Austria (CC BY 4.0) · EEA Natura 2000 · WDPA – UNEP-WCMC & IUCN (nicht-kommerziell) · OpenStreetMap-Mitwirkende (ODbL 1.0, Geofabrik-Auszug 2026-07-02) · BEV DLM 7000 Geographische Namen, Stichtag 2025-03-25 (CC BY 4.0) · RIS, Bundeskanzleramt (CC BY 4.0) · NE-Zellen: BEV Kataster (CC BY 4.0, bearbeitet, Statistik je Zelle)",
			FetchedFrom: "https://umfeld-at.exe.xyz/api/v1 (abgefragt je Punkt/bbox/Code) — Dienstseite https://umfeld-at.exe.xyz/, Referenz /api/v1/docs/llm.txt, Lizenz /api/v1/license",
			Via:         "/api/parcel-context (ein Aufruf je angeklicktem Grundstück mit unseren eigenen Flächenangaben), /api/osm-lines, /api/n2k, /api/municipalities, /api/umfeld/{lookup,toponyms,legal,…}, NE-Zellen /api/v1/ne/{kg} · /ne/manifest (deklarierter Zwilling der beobachteten Schicht)",
			License:     "je Quelle (siehe attribution)", LicenseURL: "https://umfeld-at.exe.xyz/api/v1/license",
			Attribution: "Datenquelle: STATISTIK AUSTRIA (CC BY 4.0, bearbeitet/modelliert) · Source: European Environment Agency (EEA), Natura 2000 data · UNEP-WCMC and IUCN, Protected Planet: WDPA · © OpenStreetMap contributors (ODbL) · Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen – Geographische Namen (DLM), Stichtag 2025-03-25, CC BY 4.0, bearbeitet · Datenquelle: RIS, Bundeskanzleramt Österreich · NE-Zellen: © BEV (CC BY 4.0), bearbeitet (Nutzungseinheit-Zellen, keine Objektgeometrie) · beobachtete Schicht © srtm-lidar-at (CC BY 4.0)",
			Modified:    "Preise sind Modellwerte („bearbeitet/modelliert“, keine amtliche Bewertung); Distanzen für den angefragten Punkt berechnet; Flurnamen umprojiziert, ±50 m, nie auf Grundstücke gesnappt", UsedFor: []string{"Gemeindewahl & Umrisse", "Marktwert-Schätzung", "Natura-2000-Overlay & Bonus-Schätze", "Straßen/Bahn/Gewässer-Linien", "Haltestellen & Adressen (Nähe)", "Ried- & Ortsnamen", "Gesetzesbezüge", "Deklarierte Nutzung je Zelle (NE, Abgleich beobachtet/deklariert)"}, MaxCacheH: 24,
			Notes: "Pflichten je Quelle laut /api/v1/license von umfeld-at: Statistik Austria CC BY 4.0 („Datenquelle: STATISTIK AUSTRIA“, Preisschätzungen sind Modellwerte); EEA Natura 2000 („Source: European Environment Agency (EEA)“); WDPA — Nennung, NUR nicht-kommerzielle Nutzung (wir sind nicht-kommerziell); OpenStreetMap ODbL (Geofabrik-Auszug 2026-07-02, „© OpenStreetMap contributors“; osm{}-Felder bleiben getrennt, wir stellen sie nur dar — ODbL §4.3 Produced Work); BEV DLM 7000 NAMEN, Stichtag 2025-03-25, CC BY 4.0, bearbeitet; RIS CC BY 4.0 („Datenquelle: RIS, Bundeskanzleramt Österreich“). NE-Zellen (/api/v1/ne/*, deklarierte Nutzungsstatistik je H3-Zelle Res 12 aus BEV-Vermessungsdaten): CC BY 4.0 — „© BEV (CC BY 4.0), bearbeitet · beobachtete Schicht © srtm-lidar-at (CC BY 4.0)“; Statistik je Zelle, keine Objekt-Id, -Nummer oder -Geometrie enthalten oder rekonstruierbar; Weitergabe unter CC BY 4.0 mit dieser Nennung. Von uns nicht genutzt: GADM (nur akademisch/nicht-kommerziell). Keine personenbezogenen Daten, keine Grundstückskennungen auf beiden Seiten. Wir tragen bei: nächtliche NE-Epochenberichte (nur Prüfsummen je Zellblock, Beobachter siedler-oesterreich, siehe docs/ne-report.md) aus den Zellen, die wir ohnehin für Spielende zusammensetzen. Stadium alpha.",
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
			"cadastre_source":    "BEV Katastralmappe – Vektorkacheln (kataster.bev.gv.at), CC BY 4.0, live zusammengesetzt durch bevdirect-serve auf diesem Server (Quellcode MIT: https://github.com/raffopenssh/vtcseamless, Preset bevdirect, " + bevdirectVersion + ")",
			"cadastre_assembler": map[string]string{"name": "bevdirect-serve", "version": bevdirectVersion, "source": "https://github.com/raffopenssh/vtcseamless", "preset": "bevdirect", "license": "MIT", "cell_ttl": "24h", "tile_ttl": "24h"},
			"cadastre_max_age":   "24h",
			"cadastre_storage":   "keine Datenbank; Rasterzellen (0,02°) im Zwischenspeicher mit Ablauf ≤ 24 h; kg_warm-Register läuft mit ab; nichts ist nach Grundstücksnummer/EZ abfragbar",
			"prewarm":            fmt.Sprintf("%d Katastralgemeinden pro Tag in %d zusammenhängenden Gebieten; beim Laden einer KG werden Nachbar-KGs im Hintergrund vorgewärmt; alles verfällt nach 24 h", warmDailyKGs, warmPatches),
			"context_max_age":    "24h (Umfeld, Landschaft), 6h (Wasser)",
			"no_endorsement":     "Dieses Spiel wird nicht vom BEV, von Statistik Austria, der EEA, ESA/Copernicus, UNEP-WCMC, dem Bundeskanzleramt oder OSM betrieben oder unterstützt; die Datendienste umfeld-at und srtm-lidar-at (Stadium alpha) sind ebenfalls unabhängige Ableitungen.",
			"upstream_licences":  map[string]string{"umfeld-at": "https://umfeld-at.exe.xyz/api/v1/license", "srtm-lidar-at": "https://srtm-lidar-at.exe.xyz/api/v1/license", "umfeld-at_docs": "https://umfeld-at.exe.xyz/api/v1/docs/llm.txt", "srtm-lidar-at_docs": "https://srtm-lidar-at.exe.xyz/api/v1/docs/llm.txt"},
			"legal_basis": map[string]string{
				"bev_vtc_licence":    "BEV Produktbeschreibung Katastralmappe VTC, Abschnitt 2.3: CC BY 4.0 (https://www.bev.gv.at/Services/Geoinformationsdienste/Services/Katasterservice.html → Download BEV_S_KA_Katastralmappe_VTC.pdf; archiviert: https://web.archive.org/web/20261004193544/https://www.bev.gv.at/dam/jcr:0b1df72b-93a6-46c0-aac2-1662cb33b429/BEV_S_KA_Katastralmappe_VTC.pdf)",
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
