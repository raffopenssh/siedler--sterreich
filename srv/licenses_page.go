package srv

// GET /lizenzen (DE) and /licenses (EN): the data-provenance page in app
// style. Same data as /api/licenses (licenseSources, licensesDoc).

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

type licPageStrings struct {
	Lang, Title, Back, Switch, SwitchHref, Lead, ShortHead                        string
	Facts                                                                         [][2]string // icon+title, text
	AttribHead, AttribNote                                                        string
	SourcesHead, Provider, Fetched, Via, Licence, Modified, UsedFor, Cache, Notes string
	CacheNone, Static                                                             string
	LegalHead                                                                     string
	Legal                                                                         [][2]string
	FooterJSON, FooterNoEndorse, FooterImprint, ImprintHref                       string
	HomeHref, FooterHome, PrivacyHref, FooterPrivacy, OtherLangHref, OtherLang    string
}

type licCard struct {
	Icon, Name, Provider, Fetched, Via, License, LicenseURL, Attribution, Modified, Notes, Cache string
	UsedFor                                                                                      []string
}

func licStrings(lang string) licPageStrings {
	daily := fmt.Sprintf("%d", warmDailyKGs)
	patches := fmt.Sprintf("%d", warmPatches)
	if lang == "en" {
		return licPageStrings{
			Lang: "en", Title: "Data sources & licences", Back: "◀ Back to the game",
			Switch: "Deutsch", SwitchHref: "/lizenzen",
			Lead:      "Every map in this game is drawn from open Austrian data. Here is where it comes from, under which licence, and how long we keep it.",
			ShortHead: "In short",
			Facts: [][2]string{
				{"🗺️ Live from the official tiles", "Parcels, building footprints and land-use classes are assembled on our server from the BEV cadastral map vector tiles (kataster.bev.gv.at) for exactly the area you are looking at."},
				{"⏳ Gone after 24 hours", "We keep no cadastre database. An assembled grid cell (0.02°) lives in our cache for at most 24 hours. Each day " + daily + " cadastral communities in " + patches + " connected areas are pre-warmed, plus the neighbours of whatever you load – and all of it expires again."},
				{"🔒 No look-ups by number", "Nothing can be queried by parcel number, folio (EZ) or owner. The game only ever asks “what is at this spot?”."},
				{"🎮 A game, not a register", "Prices, ownership and coins are game fiction. No legally binding coordinates can be derived from the tiles – this is not an official extract from the cadastre or land register."},
				{"🌱 Non-commercial", "The service is non-commercial. Two sources (WDPA protected areas, MERIT Hydro flow paths) are only permitted because of that."},
			},
			AttribHead: "Attribution shown with every map", AttribNote: "This notice appears under every map view, on this page, in the imprint and as a notice field in every cadastre answer of our API.",
			SourcesHead: "The sources", Provider: "Provider", Fetched: "Fetched from", Via: "Used through", Licence: "Licence", Modified: "Our changes", UsedFor: "Used for", Cache: "Max. cache", Notes: "Details & legal basis",
			CacheNone: "not cached", Static: "embedded, no refresh",
			LegalHead: "Legal basis",
			Legal: [][2]string{
				{"BEV VTC licence", "The vector tiles are the BEV product “Katastralmappe VTC”; its product description (BEV_S_KA_Katastralmappe_VTC.pdf, section 2.3) places it under CC BY 4.0."},
				{"Database right", "CC BY 4.0 section 4 (Sui Generis Database Rights) licenses extraction and re-use of substantial parts, which covers §76c ff of the Austrian Copyright Act for the VTC product. Our 24-hour limit and the absence of any parcel/folio look-up go beyond that."},
				{"Attribution format", "“© BEV, YYYY” + source + licence + note of modification (BEV terms §2.3.3; CC BY 4.0 §3(a)) – on every map view, on this page, in the imprint and as a notice in every cadastre API answer."},
				{"Non-commercial only", "WDPA (non-commercial) and MERIT Hydro (CC BY-NC-SA 4.0) are only permissible because the service is non-commercial. Should it ever be commercialised, both sources would have to go."},
				{"Share-alike", "geoBoundaries gbOpen (CC BY-SA 4.0): the simplified border file /static/austria.json is passed on under CC BY-SA 4.0. OSM lines are only rendered (Produced Work, ODbL §4.3)."},
			},
			FooterJSON: "Machine-readable version:", FooterNoEndorse: "This game is not operated or endorsed by BEV, Statistics Austria, the EEA, ESA or OpenStreetMap.", FooterImprint: "Imprint", ImprintHref: "/imprint", HomeHref: "/", FooterHome: "Home", PrivacyHref: "/privacy", FooterPrivacy: "Privacy", OtherLangHref: "/lizenzen", OtherLang: "Deutsch",
		}
	}
	return licPageStrings{
		Lang: "de", Title: "Datenquellen & Lizenzen", Back: "◀ Zurück zum Spiel",
		Switch: "English", SwitchHref: "/licenses",
		Lead:      "Jede Karte in diesem Spiel entsteht aus offenen österreichischen Daten. Hier steht, woher sie kommen, unter welcher Lizenz – und wie lange wir sie behalten.",
		ShortHead: "Kurz gesagt",
		Facts: [][2]string{
			{"🗺️ Live aus den amtlichen Kacheln", "Grundstücke, Gebäudegrundrisse und Benützungsarten werden auf unserem Server aus den Vektorkacheln der BEV-Katastralmappe (kataster.bev.gv.at) zusammengesetzt – genau für den Ausschnitt, den du gerade siehst."},
			{"⏳ Nach 24 Stunden wieder weg", "Wir führen keine Katasterdatenbank. Eine zusammengesetzte Rasterzelle (0,02°) bleibt höchstens 24 Stunden im Zwischenspeicher. Täglich werden " + daily + " Katastralgemeinden in " + patches + " zusammenhängenden Gebieten vorgewärmt, dazu die Nachbarn dessen, was du lädst – und alles verfällt wieder."},
			{"🔒 Keine Abfrage nach Nummer", "Nichts ist nach Grundstücksnummer, Einlagezahl oder Eigentümer abfragbar. Das Spiel fragt immer nur: „Was liegt an dieser Stelle?“"},
			{"🎮 Ein Spiel, kein Register", "Preise, Besitz und Münzen sind Spielfiktion. Aus den Kacheln lassen sich keine rechtsverbindlichen Koordinaten ableiten – das ist kein amtlicher Auszug aus Kataster oder Grundbuch."},
			{"🌱 Nicht-kommerziell", "Der Dienst ist nicht-kommerziell. Zwei Quellen (WDPA-Schutzgebiete, MERIT-Hydro-Fließwege) sind nur deshalb zulässig."},
		},
		AttribHead: "Quellenhinweis auf jeder Karte", AttribNote: "Dieser Hinweis steht unter jeder Kartenansicht, auf dieser Seite, im Impressum und als notice-Feld in jeder Kataster-Antwort unserer API.",
		SourcesHead: "Die Quellen", Provider: "Anbieter", Fetched: "Woher wir laden", Via: "Verwendet über", Licence: "Lizenz", Modified: "Unsere Bearbeitung", UsedFor: "Verwendung", Cache: "Max. Cache", Notes: "Details & Rechtsgrundlage",
		CacheNone: "nicht gecacht", Static: "eingebettet, keine Aktualisierung",
		LegalHead: "Rechtsgrundlage",
		Legal: [][2]string{
			{"BEV-VTC-Lizenz", "Die Vektorkacheln sind das BEV-Produkt „Katastralmappe VTC“; dessen Produktbeschreibung (BEV_S_KA_Katastralmappe_VTC.pdf, Abschnitt 2.3) stellt es unter CC BY 4.0."},
			{"Datenbankrecht", "CC BY 4.0 Abschnitt 4 (Sui Generis Database Rights) lizenziert Entnahme und Weiterverwendung wesentlicher Teile; damit ist §76c ff UrhG für das VTC-Produkt abgedeckt. Unsere 24-h-Grenze und der Verzicht auf jede Gst./EZ-Abfrage gehen darüber hinaus."},
			{"Attributionsformat", "„© BEV, JJJJ“ + Quelle + Lizenz + Bearbeitungshinweis (BEV Nutzungsbedingungen §2.3.3; CC BY 4.0 §3(a)) – auf jeder Kartenansicht, auf dieser Seite, im Impressum und als notice in jeder Kataster-Antwort der API."},
			{"Nur nicht-kommerziell", "WDPA (nicht-kommerziell) und MERIT Hydro (CC BY-NC-SA 4.0) sind nur zulässig, weil der Dienst nicht-kommerziell ist. Bei einer Kommerzialisierung müssten beide Quellen entfallen."},
			{"Share-alike", "geoBoundaries gbOpen (CC BY-SA 4.0): die vereinfachte Grenzdatei /static/austria.json wird unter CC BY-SA 4.0 weitergegeben. OSM-Linien werden nur dargestellt (Produced Work, ODbL §4.3)."},
		},
		FooterJSON: "Maschinenlesbar:", FooterNoEndorse: "Dieses Spiel wird nicht vom BEV, von Statistik Austria, der EEA, ESA oder OSM betrieben oder unterstützt.", FooterImprint: "Impressum", ImprintHref: "/impressum", HomeHref: "/", FooterHome: "Startseite", PrivacyHref: "/datenschutz", FooterPrivacy: "Datenschutz", OtherLangHref: "/licenses", OtherLang: "English",
	}
}

func licCards(lang string, t licPageStrings) []licCard {
	var out []licCard
	for _, src := range licenseSources() {
		c := licCard{Icon: src.Icon, Name: src.Name, Provider: src.Provider, Fetched: src.FetchedFrom, Via: src.Via,
			License: src.License, LicenseURL: src.LicenseURL, Attribution: src.Attribution, Modified: src.Modified, Notes: src.Notes, UsedFor: src.UsedFor}
		if lang == "en" {
			c.Name, c.Fetched, c.Via, c.Modified, c.Notes, c.UsedFor = src.NameEN, src.FetchedEN, src.ViaEN, src.ModifiedEN, src.NotesEN, src.UsedForEN
			switch c.License {
			case "je Quelle (siehe attribution)":
				c.License = "per source (see attribution)"
			case "Faktendaten (Artname, europäische Gefährdungskategorie); EU-Weiterverwendung / IUCN-Nutzungsbedingungen":
				c.License = "factual data (species name, European threat category); EU re-use / IUCN terms of use"
			}
			c.Provider = strings.NewReplacer("nicht-kommerziell", "non-commercial", "nur Darstellung", "display only", "bearbeitet", "modified", "Dürreindex", "drought index", "Mitwirkende", "contributors", "Geographische Namen", "Geographic Names", "Bundeskanzleramt", "Federal Chancellery", "Fließweg", "flow path", "IUCN im Auftrag der Europäischen Kommission, veröffentlicht über die EEA", "IUCN on behalf of the European Commission, published via the EEA").Replace(c.Provider)
		}
		switch {
		case src.MaxCacheH > 0:
			c.Cache = fmt.Sprintf("%g h", src.MaxCacheH)
		case src.Via == "statisch" || src.Via == "eingebettete Tabelle" || src.Via == "eingebettet":
			c.Cache = t.Static
		default:
			c.Cache = t.CacheNone
		}
		out = append(out, c)
	}
	return out
}

var licTmpl = template.Must(template.New("lic").Parse(`<!doctype html>
<html lang="{{.T.Lang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.T.Title}} – Siedler Österreich</title>
<meta name="description" content="{{.T.Lead}}">
<meta name="robots" content="index, follow">
<link rel="canonical" href="{{.Site}}{{if eq .T.Lang "de"}}/lizenzen{{else}}/licenses{{end}}">
<link rel="alternate" hreflang="de" href="{{.Site}}/lizenzen">
<link rel="alternate" hreflang="en" href="{{.Site}}/licenses">
<link rel="alternate" hreflang="x-default" href="{{.Site}}/lizenzen">
<link rel="alternate" type="application/json" href="{{.Site}}/api/licenses">
<link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🏰</text></svg>">
<link rel="stylesheet" href="/static/style.css?v={{.V}}">
<link rel="stylesheet" href="/static/legal.css?v={{.V}}">
</head><body class="legal lic">
<div class="legal-wrap lic-wrap">
  <div class="lic-top"><a class="legal-back" href="/">{{.T.Back}}</a><a class="lic-lang" href="{{.T.SwitchHref}}" hreflang="{{if eq .T.Lang "de"}}en{{else}}de{{end}}">🌐 {{.T.Switch}}</a></div>
  <h1>{{.T.Title}}</h1>
  <p class="lic-lead">{{.T.Lead}}</p>

  <h2>{{.T.ShortHead}}</h2>
  <div class="lic-facts">{{range .T.Facts}}
    <div class="lic-fact"><div class="lic-fact-h">{{index . 0}}</div><div>{{index . 1}}</div></div>{{end}}
  </div>

  <h2>{{.T.AttribHead}}</h2>
  <div class="lic-attrib">{{.Notice}}</div>
  <p class="lic-dim">{{.T.AttribNote}}</p>

  <h2>{{.T.SourcesHead}}</h2>
  {{range .Cards}}
  <section class="lic-card">
    <div class="lic-card-h"><span class="lic-icon">{{.Icon}}</span><span class="lic-name">{{.Name}}</span>
      <span class="lic-badges"><a class="lic-badge" href="{{.LicenseURL}}" rel="license noopener" target="_blank">{{.License}}</a><span class="lic-badge lic-badge-cache">⏳ {{.Cache}}</span></span></div>
    <dl class="lic-rows">
      <dt>{{$.T.Provider}}</dt><dd>{{.Provider}}</dd>
      <dt>{{$.T.Fetched}}</dt><dd>{{.Fetched}}</dd>
      <dt>{{$.T.Via}}</dt><dd>{{.Via}}</dd>
      {{if .Modified}}<dt>{{$.T.Modified}}</dt><dd>{{.Modified}}</dd>{{end}}
      <dt>{{$.T.UsedFor}}</dt><dd class="lic-chips">{{range .UsedFor}}<span class="lic-chip">{{.}}</span>{{end}}</dd>
    </dl>
    <div class="lic-cite">{{.Attribution}}</div>
    {{if .Notes}}<details class="lic-notes"><summary>{{$.T.Notes}}</summary><p>{{.Notes}}</p></details>{{end}}
  </section>
  {{end}}

  <h2>{{.T.LegalHead}}</h2>
  <div class="lic-legal">{{range .T.Legal}}
    <div class="lic-legal-row"><div class="lic-legal-k">{{index . 0}}</div><div>{{index . 1}}</div></div>{{end}}
  </div>

  <div class="legal-footer">
    <p>{{.T.FooterNoEndorse}}</p>
    <p>{{.T.FooterJSON}} <a href="/api/licenses">/api/licenses</a></p>
    <p><a href="{{.T.HomeHref}}">{{.T.FooterHome}}</a> · <a href="{{.T.ImprintHref}}">{{.T.FooterImprint}}</a> · <a href="{{.T.PrivacyHref}}">{{.T.FooterPrivacy}}</a> · <a href="{{.T.OtherLangHref}}">{{.T.OtherLang}}</a></p>
  </div>
</div>
</body></html>`))

func (s *Server) handleLicensesPage(w http.ResponseWriter, r *http.Request) {
	lang := "de"
	if r.URL.Path == "/licenses" || r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	t := licStrings(lang)
	var buf bytes.Buffer
	err := licTmpl.Execute(&buf, map[string]any{"T": t, "Cards": licCards(lang, t), "Notice": bevNotice, "V": "lic20261003c", "Site": siteURL})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Language", lang)
	w.Write(buf.Bytes())
}
