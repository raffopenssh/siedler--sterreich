// ============================================================
// Siedler Österreich — runtime i18n dictionary (DE → EN)
// I18N_EXACT: exact-match lookup (DOM text walker + tr())
// I18N_RX:    parameterized runtime strings (toasts etc.)
// ============================================================

const I18N_EXACT = {
  'Dieser Durchzügler ist weitergezogen': 'This wanderer has moved on',
  // ---------- minimap ----------
  'ziehen': 'drag',
  'Klick: springen · Ziehen: schwenken · Doppelklick: heranzoomen': 'Click: jump · Drag: pan · Double-click: zoom in',
  'Zurück zur vorigen Ansicht': 'Back to previous view',
  // ---------- index.html: welcome screen ----------
  'Dein Pseudonym...': 'Your nickname...',
  'Neuer Vorschlag': 'New suggestion',
  'Neues Spiel': 'New Game',
  '🍀 Auf Glück': '🍀 Feeling lucky',
  'Erkunden': 'Explore',
  'Zufälliges Fleckchen Österreich': 'A random patch of Austria',
  '📍 Gemeinde selbst wählen': '📍 Pick a place yourself',
  '⚔️ Mitspielen': '⚔️ Join game',
  'Beta · keine Cookies · kein Tracking': 'Beta · no cookies · no tracking',
  '⚒ BETA · PROTOTYP': '⚒ BETA · PROTOTYPE',
  'In Entwicklung – Spielstände, Preise und Regeln können sich noch ändern.': 'In development – saves, prices and rules may still change.',
  'Impressum': 'Imprint',
  'Daten: ': 'Data: ',
  ' Kataster & ALS, ': ' cadastre & ALS, ',
  ' (bearbeitet) · ': ' (modified) · ',
  '© OpenStreetMap-Mitwirkende': '© OpenStreetMap contributors',
  'weitere Quellen': 'more sources',
  'Datenquellen & Lizenzen': 'Data sources & licences',
  '📜 Datenquellen': '📜 Data sources',
  'Kataster & ALS-Höhenmodell: ': 'Cadastre & ALS elevation model: ',
  '© BEV – Bundesamt für Eich- und Vermessungswesen': '© BEV – Federal Office of Metrology and Surveying',
  ', bearbeitet (vereinfacht, segmentiert, angereichert)': ', modified (simplified, segmented, enriched)',
  'Straßen, Gewässer, Bahn: ': 'Roads, water, rail: ',
  'Schutzgebiete: EEA Natura 2000 · Landbedeckung: Copernicus / ESA WorldCover': 'Protected areas: EEA Natura 2000 · Land cover: Copernicus / ESA WorldCover',
  'Spielerische Darstellung – keine amtliche Auskunft. ': 'Playful rendering – not an official statement. ',
  'Datenschutz': 'Privacy',

  // ---------- index.html: map picker ----------
  '🗺️ Wähle deine Gemeinde': '🗺️ Pick your municipality',
  'Gemeinde, Adresse oder PLZ...': 'Municipality, address or postal code...',
  '◀ Zurück': '◀ Back',

  // ---------- index.html: lobby ----------
  '🏰 Spiellobby': '🏰 Game Lobby',
  'Neues Spiel erstellen': 'Create new game',
  '— oder mit Code beitreten —': '— or join with a code —',
  'Einladungscode...': 'Invite code...',
  'Beitreten': 'Join',
  'Einladungslink:': 'Invite link:',
  'Spieler:': 'Players:',
  '🎮 Spiel starten': '🎮 Start game',

  // ---------- index.html: loading screen ----------
  'Siedlung wird vorbereitet...': 'Preparing your settlement...',
  'Spielsitzung erstellen': 'Creating game session',
  'Kataster-Parzellen & EZ-Daten laden': 'Loading cadastre parcels & EZ data',
  'Polygone, Gebäude & Nutzungsflächen laden': 'Loading polygons, buildings & land use',
  'Arten & Schätze platzieren': 'Placing species & treasures',
  'Karte rendern': 'Rendering map',
  'Echte Katasterdaten': 'Real cadastre data',
  'Jede Parzelle in diesem Spiel basiert auf echten österreichischen Grundstücksdaten — mit realen Flächen, Nutzungsarten und Grenzen.': 'Every parcel in this game is based on real Austrian land register data — with real areas, land-use types and boundaries.',
  'Naturschutz-Ziel: 30%': 'Conservation goal: 30%',
  'Kaufe Parzellen und wandle sie in Naturschutzgebiete um. Dein Ziel: 30% der Fläche deiner Gemeinde unter Schutz stellen!': 'Buy parcels and convert them into nature reserves. Your goal: protect 30% of your municipality\'s area!',
  'Wirtschaft & Strategie': 'Economy & strategy',
  'Du startest mit 10.000 Münzen. Kaufe klug — dicht bebaute Parzellen kosten mehr als ländliche Flächen!': 'You start with 10,000 coins. Buy smart — densely built-up parcels cost more than rural land!',
  'Seltene Arten entdecken': 'Discover rare species',
  'Auf der Karte verstecken sich bedrohte Tierarten aus der Europäischen Roten Liste — Luchs, Steinadler, Apollofalter und mehr. Finde sie und lerne über Artenschutz!': 'Endangered species from the European Red List are hidden on the map — lynx, golden eagle, Apollo butterfly and more. Find them and learn about species conservation!',
  'Multiplayer': 'Multiplayer',
  'Lade Freunde über einen Einladungslink ein und siedelt gemeinsam — oder gegeneinander — in derselben Gemeinde!': 'Invite friends with an invite link and settle together — or against each other — in the same municipality!',
  'Europäische Rote Liste': 'European Red List',
  'Die Artenfunde basieren auf der EU-Roten-Liste gefährdeter Arten. Der Huchen (stark gefährdet) und der Sterlet (gefährdet) leben in Österreichs Flüssen — hilf, ihren Lebensraum zu schützen!': 'Species finds are based on the EU Red List of threatened species. The Danube salmon (endangered) and the sterlet (vulnerable) live in Austria\'s rivers — help protect their habitat!',
  'Freunde einladen:': 'Invite friends:',
  '📋 Kopieren': '📋 Copy',

  // ---------- index.html: game sidebar ----------
  '🪙 Münzen': '🪙 Coins',
  '⚡ Erfahrung': '⚡ Experience',
  '🎯 Level': '🎯 Level',
  '📍 Parzellen': '📍 Parcels',
  '🌿 Naturschutz-Ziel: 30%': '🌿 Conservation goal: 30%',
  '⚔️ Mitspieler': '⚔️ Players',
  '📜 Aufgaben': '📜 Quests',
  'Wiedereinstiegs-Link kopieren': 'Copy rejoin link',
  'Einladungs-Link kopieren': 'Copy invite link',
  '💬 Chat': '💬 Chat',
  'Nachricht...': 'Message...',

  // ---------- index.html: game main / popups ----------
  'Adresse suchen... (/)': 'Search address... (/)',
  'Parzelle': 'Parcel',
  'KG': 'KG',
  'EZ': 'EZ',
  'Fläche': 'Area',
  'Nutzung': 'Land use',
  'Bebauung': 'Buildings',
  'Besitzer': 'Owner',
  'Preis': 'Price',
  '🏚️ Gebäude': '🏚️ Buildings',
  '⛰️ Gelände & Umgebung': '⛰️ Terrain & surroundings',
  '🌲 Riesenbaum': '🌲 Giant tree',
  'Höhe': 'Height',
  'Geschätztes Alter': 'Estimated age',
  'Rang': 'Rank',
  'Höhenvergleich (Bäume in der Nähe) — Balken antippen = hinfliegen': 'Height comparison (nearby trees) — tap a bar to fly there',
  '🏘️ Katastralgemeinde': '🏘️ Cadastral municipality',
  '📋 Einlagezahl (EZ)': '📋 Land register folio (EZ)',
  'In Google Earth öffnen': 'Open in Google Earth',
  'Diese Ansicht teilen — Einladungslink kopieren': 'Share this view — copy invite link',
  'Natura-2000-Schutzgebiete ein/aus': 'Toggle Natura 2000 protected areas',
  'Mein Standort': 'My location',
  '✨ Enhanced Gelände': '✨ Enhanced terrain',
  'Würfle eine Gemeinde…': 'Rolling the dice for a municipality…',
  '✨ Enhanced Gelände 🌲': '✨ Enhanced terrain 🌲',
  '✕ Ähnliche ausblenden': '✕ Hide similar',

  // ---------- game.js: static toasts / errors ----------
  'Einladung ungültig': 'Invalid invite',
  'Keine Gemeinde gefunden': 'No municipality found',
  'Fehler': 'Error',
  '📋 Kopiert!': '📋 Copied!',
  'Noch keine KG-Daten geladen': 'No KG data loaded yet',
  '📋 Einladung kopiert!': '📋 Invite copied!',
  '🔑 Wiedereinstiegs-Link kopiert!': '🔑 Rejoin link copied!',
  '⚔️ Einladungs-Link kopiert!': '⚔️ Invite link copied!',
  '🛡️ Seltene Arten in Natura-2000-Gebieten entdeckt!': '🛡️ Rare species discovered in Natura 2000 areas!',
  '❌ Dein Angebot wurde abgelehnt': '❌ Your offer was declined',
  'Kein Einladungscode verfügbar': 'No invite code available',
  '🔗 Link zu dieser Ansicht kopiert — einfach weiterschicken!': '🔗 Link to this view copied — just pass it on!',
  '🛡️ Schutzgebiete sichtbar': '🛡️ Protected areas visible',
  '🛡️ Schutzgebiete ausgeblendet': '🛡️ Protected areas hidden',
  '🌲 Noch keine Riesenbäume geladen …': '🌲 No giant trees loaded yet …',
  '📍 Auf Standort zentriert — nochmal tippen zum Ausschalten': '📍 Centered on your location — tap again to turn off',
  '📍 Standort aus': '📍 Location off',
  '📍 Standort wird ermittelt…': '📍 Getting your location…',
  '📍 Außerhalb Österreichs — Position wird nicht angezeigt': '📍 Outside Austria — position not shown',
  '✨ Der Nebel führt dich zu einem Riesenbaum...': '✨ The mist guides you to a giant tree...',
  '🔍 Ähnlichkeitssuche fehlgeschlagen': '🔍 Similarity search failed',
  'Alle Parzellen dieser EZ sind bereits vergeben': 'All parcels of this EZ are already taken',
  'Mindestangebot: 10 Münzen': 'Minimum offer: 10 coins',
  '✅ Angebot angenommen! Parzelle verkauft.': '✅ Offer accepted! Parcel sold.',
  '❌ Angebot abgelehnt.': '❌ Offer declined.',
  '🌲 Gerücht: Irgendwo da steht ein Riesenbaum... Find ihn und tipp ihn an!': '🌲 Rumor: a giant tree stands somewhere around here... Find it and tap it!',
  'Link kopieren:': 'Copy link:',
  'Komm zu mir auf die Karte!': 'Join me on the map!',

  // ---------- game.js: loading status (setLoadSub / loading-muni) ----------
  'Parzellen-Punkte werden geladen...': 'Loading parcel points...',
  'Katastralgemeinden werden ermittelt...': 'Detecting cadastral municipalities...',
  'Bedrohte Arten und Schätze werden platziert...': 'Placing endangered species and treasures...',
  'Karte wird gerendert...': 'Rendering map...',
  '✅ Bereit — Viel Spaß beim Siedeln!': '✅ Ready — happy settling!',
  '✅ Bereit!': '✅ Ready!',
  'Geometrien für den sichtbaren Bereich werden geladen...': 'Loading geometries for the visible area...',
  '🍀 Zufallsgemeinde wird gewählt...': '🍀 Picking a random municipality...',
  'KG-Übersicht anzeigen': 'Show KG overview',

  // ---------- game.js: search dropdowns ----------
  'Suche…': 'Searching…',
  'Keine Ergebnisse': 'No results',
  'Fehler bei der Suche': 'Search error',

  // ---------- game.js: KG/EZ popup labels ----------
  'Lädt…': 'Loading…',
  'Keine Daten verfügbar': 'No data available',
  'Gemeinde': 'Municipality',
  'KG-Code': 'KG code',
  'Ø Parzelle': 'Ø parcel',
  '⛰️ Seehöhe': '⛰️ Elevation',
  '🌲 Höchster Baum': '🌲 Tallest tree',
  '⚖️ Rechtsbezüge': '⚖️ Legal references',
  '🏴 Dein Besitz': '🏴 Your holdings',
  'Nutzung (nach Parzellenzahl)': 'Land use (by parcel count)',
  '✨ Enhanced — LiDAR-Geländedaten aktiv': '✨ Enhanced — LiDAR terrain data active',
  'Gesamtfläche': 'Total area',
  'Dein Besitz': 'Your holdings',
  '📋 Ganze EZ kaufen:': '📋 Buy entire EZ:',
  'frei': 'free',

  // ---------- game.js: parcel popup runtime values ----------
  'Frei': 'Unclaimed',
  'Besetzt': 'Taken',
  'Keine': 'None',
  '🏙️ Dicht': '🏙️ Dense',
  '🏡 Mittel': '🏡 Medium',
  '🌾 Gering': '🌾 Low',
  '🌾 Minimal': '🌾 Minimal',
  ' (du)': ' (you)',
  'Alle erledigt!': 'All done!',

  // ---------- game.js: action buttons / offers ----------
  '🏴 Kaufen': '🏴 Buy',
  '🌿 Naturschutz': '🌿 Nature reserve',
  '🌳 Aufforsten': '🌳 Reforest',
  '💰 Verkaufen': '💰 Sell',
  '📨 Kaufangebote:': '📨 Purchase offers:',
  '📨 Anbieten': '📨 Make offer',
  'Kaufangebot an': 'Purchase offer to',
  '(wartet)': '(pending)',
  '📨 Angebot:': '📨 Offer:',

  // ---------- game.js: similar parcels ----------
  '🔍 Ähnliche Parzellen': '🔍 Similar parcels',
  '⏳ Suche ähnliche Parzellen…': '⏳ Searching for similar parcels…',
  '🔍 Vergleich': '🔍 Comparison',
  'Referenzparzelle': 'Reference parcel',
  '🔍 Ähnlichkeit': '🔍 Similarity',
  'entfernt': 'away',
  '→ zur Referenzparzelle': '→ to reference parcel',
  '📏 Größe': '📏 Size',
  '🌾 Nutzung': '🌾 Land use',
  '🏗️ Bebauung': '🏗️ Buildings',
  '⛰️ Gelände': '⛰️ Terrain',
  '🌿 Bewuchs': '🌿 Vegetation',

  // ---------- game.js: enhanced popup rows ----------
  '⛰️ Höhe': '⛰️ Elevation',
  '⛰️ Hang': '⛰️ Slope',
  '🛣️ Straße': '🛣️ Road',
  '🚌 Öffi': '🚌 Transit',
  '🚉 Bahnhof': '🚉 Train station',
  '💧 Gewässer': '💧 Water',
  '🏘️ Ort': '🏘️ Settlement',
  '🧭 Lage': '🧭 Location',
  '(am Grundstück)': '(on the parcel)',
  'zentral': 'central',
  'gut erschlossen': 'well connected',
  'ländlich': 'rural',
  'abgelegen': 'remote',
  '💶 Marktwert': '💶 Market value',
  'Spielpreis:': 'Game price:',
  '€/m² echt': '€/m² real',
  'Bauland (bebaut)': 'Building land (built-up)',
  'Bauland': 'Building land',
  'Ackerland': 'Farmland',
  'Sonstig': 'Other',

  // ---------- game.js: building rows ----------
  '📏 Grundfläche': '📏 Footprint area',
  '🏷️ Typ': '🏷️ Type',
  '📐 Höhe (LiDAR)': '📐 Height (LiDAR)',
  '🏠 Dach': '🏠 Roof',
  'Flachdach': 'Flat roof',
  'Steildach': 'Pitched roof',
  '🧭 Ausrichtung': '🧭 Orientation',
  'Baufläche (befestigt)': 'Building area (paved)',
  'Keller/Tiefgarage': 'Basement/underground garage',

  // ---------- game.js: land-use vocabulary (BEV Nutzungssymbole, NS_TABLE) ----------
  'Dauerkulturen': 'Permanent crops',
  'Gebäude': 'Building',
  'Äcker/Wiesen/Weiden': 'Fields/meadows/pastures',
  'Alm': 'Alpine pasture',
  'Verbuschte Fläche': 'Scrubland',
  'Forststraße': 'Forest road',
  'Fließgewässer': 'Running water',
  'Stehendes Gewässer': 'Standing water',
  'Feuchtgebiet': 'Wetland',
  'Vegetationsarm': 'Sparse vegetation',
  'Betriebsfläche': 'Commercial site',
  'Gewässerrand': 'Waterside area',
  'Verkehrsrand': 'Roadside area',
  'Friedhof': 'Cemetery',
  'Gebäudenebenfläche': 'Building ancillary area',
  'Abbau/Halde/Deponie': 'Quarry/spoil/landfill',
  'Fels/Geröll': 'Rock/scree',
  'Bahnanlage': 'Railway facility',
  'Freizeitfläche': 'Recreation area',
  'Naturschutz': 'Nature reserve',
  // legacy / generic vocabulary still used elsewhere in the UI
  'Baufläche': 'Building area',
  'Acker': 'Cropland',
  'Wiese': 'Meadow',
  'Weide': 'Pasture',
  'Grünland': 'Grassland',
  'Alpe': 'Alpine pasture',
  'Wald': 'Forest',
  'Krummholz': 'Krummholz',
  'Weingarten': 'Vineyard',
  'Garten': 'Garden',
  'Obstgarten': 'Orchard',
  'Gewässer': 'Water body',
  'Bach': 'Stream',
  'See': 'Lake',
  'Fluss': 'River',
  'Ödland': 'Wasteland',
  'Sumpf': 'Marsh',
  'Gletscher': 'Glacier',
  'Fels': 'Rock',
  'Straße': 'Road',
  'Weg': 'Path',
  'Platz': 'Square',
  'Bahn': 'Railway',
  'Brücke': 'Bridge',
  'Sonstige': 'Other',
  'Quelle': 'Spring',
  'Bäume': 'Trees',
  'beobachtet': 'observed', 'abgestorben': 'dead',
  'Steht hier etwas?': 'Is something standing here?', 'Die Beobachtung zeigt ein dachartiges Bauwerk': 'The observation shows a roof-like structure', 'das im Kataster nicht eingetragen ist.': 'that is not registered in the cadastre.',
  'Nur ein Hinweis für dich als Besitzer – ob und was, klärt das Vermessungsamt.': 'Just a hint for you as the owner – whether and what is for the surveying office to settle.',
  'Spähbericht': 'Scout report',
  'auf der Karte': 'on the map', 'auf der Karte zeigen': 'show on the map', 'Bauwerk fehlt': 'structure missing', 'neu begrünt': 'newly green', 'neu versiegelt': 'newly sealed', 'Wald nachgewachsen': 'forest regrown',
  'stimmt mit Kataster überein': 'matches the cadastre',
  'auf der Karte · antippen = aus': 'on the map · tap to hide',
  'antippen → auf der Karte zeigen': 'tap → show on the map',
  'Wald beobachtet': 'Forest observed',
  'kein Verlust': 'no loss',
  'erfasst': 'covered',
  'Beobachtet (Luftbild/LiDAR)': 'Observed (aerial/LiDAR)',
  'Abweichung vom Kataster': 'Differs from cadastre',
  'der Fläche': 'of the area',
  'Bauwerke': 'Structures',
  'Kronendach': 'canopy',
  'zuletzt': 'last',
  'Bauflächen': 'Built-up', 'Verkehr': 'Traffic', 'Alpin': 'Alpine', 'Sonstiges': 'Other',
  'Wasser': 'Water',
  'Parkplatz': 'Parking',
  'Gestrüpp': 'Scrub',
  'Hecke': 'Hedge',
  'Offen': 'Open ground',
  'Schüttung': 'Fill',
  'Aushub': 'Excavation',
  'Baustelle': 'Construction site',
  'Rodung': 'Clearing',
  'Baumbestand': 'Tree cover',
  'Bebaut': 'Built-up',

  // ---------- game.js: terrain classes ----------
  'eben': 'level',
  'fast eben': 'nearly level',
  'sanft': 'gentle',
  'wellig': 'undulating',
  'mäßig': 'moderate',
  'hügelig': 'hilly',
  'steil': 'steep',
  'gebirgig': 'mountainous',
  'schroff': 'rugged',
  'leicht schroff': 'slightly rugged',

  // ---------- game.js: red-list categories ----------
  'Stark gefährdet': 'Endangered',
  'Gefährdet': 'Vulnerable',
  'Potenziell gefährdet': 'Near threatened',
  'Nicht gefährdet': 'Least concern',

  // ---------- supplemental fragments (wrapped via tr() in game.js) ----------
  ' Siedlung': ' Settlement',
  'Zuletzt als': 'Last played as',
  'gespielt': 'played',
  'Weiter ▸': 'Continue ▸',
  'Geb.': 'bldg.',
  'Riesen': 'giants',
  'Kaufen': 'Buy',
  'Spielpreis:': 'Game price:',
  'Komm zu mir auf die Karte!': 'Join me on the map!',
  'Link kopieren:': 'Copy link:',
  'E-Mail anzeigen ▸': 'Show e-mail ▸',

  // ---------- quests (server-generated, canonical German) ----------
  'Erkunde deine Gemeinde': 'Explore your municipality',
  'Kaufe deine erste Parzelle': 'Buy your first parcel',
  'Naturschützer': 'Conservationist',
  'Wandle eine Parzelle in ein Naturschutzgebiet um': 'Convert a parcel into a nature reserve',
  'Landvermesser': 'Land surveyor',
  'Kaufe 5 Parzellen': 'Buy 5 parcels',
  'Schatzsucher': 'Treasure hunter',
  'Finde einen versteckten Schatz': 'Find a hidden treasure',
  'Waldmeister': 'Forest master',
  'Erntedank': 'Harvest festival',
  'Ernte 3 reife Äcker': 'Harvest 3 ripe fields',
  // ---------- forest plots (timber.go / forest overlay) ----------
  'Holzknecht': 'Lumberjack',
  'Schlägere 2 Waldparzellen': 'Log 2 forest plots',
  'Waldhüter': 'Forest warden',
  'Stelle einen Wald außer Nutzung (Naturwald)': 'Set a forest aside as wild forest',
  'Holzernte': 'Timber harvest',
  'Naturwald': 'Wild forest',
  'außer Nutzung': 'set aside',
  'Kahlschlag': 'Clear-cut',
  'Jungwuchs': 'Regrowth',
  'Jungwuchs in': 'regrowth in',
  'Stangenholz': 'Pole stage',
  'Stangenholz in': 'pole stage in',
  'erntereif in': 'harvestable in',
  'Baumholz': 'Timber stand',
  'hiebsreif': 'ready to fell',
  'Wert': 'Value',
  'Wald': 'Forest',
  'Feld': 'Field',
  'Holzvorrat': 'Growing stock',
  'Bestand': 'Stand',
  'Bäume': 'trees',
  'Holzerlös': 'Timber revenue',
  'netto': 'net',
  'Fichte': 'Spruce',
  'Fichte/Tanne': 'Spruce/fir',
  'Lärche': 'Larch',
  'Kiefer': 'Pine',
  'Laubholz': 'Broadleaf',
  'Richtwert': 'reference',
  'im Holz gespeichert': 'stored in the wood',
  'Einzelbaum-Inventur (ALS)': 'single-tree inventory (ALS)',
  'ALS-Kronenhöhe': 'ALS canopy height',
  'Nutzungsart': 'land use',
  'Holz geerntet': 'Timber harvested',
  'Verkaufen': 'Sell',
  'Schutz aufgeben': 'Give up protection',
  'VERKAUFT': 'SOLD',
  'statt': 'instead of',
  'voll in': 'full in',
  'Wert erholt sich': 'value recovering',
  'des Kaufpreises': 'of the purchase price',
  'Günstig erworben': 'Bought cheap',
  'Bestand erholt sich noch': 'the stand is still recovering',
  'bleiben im Wald': 'stay in the forest',
  'Der Wald muss erst nachwachsen': 'The forest has to regrow first',
  'Aufforstung': 'Reforestation',
  'Öffne eine deiner Waldparzellen und tipp auf': 'Open one of your forest plots and tap',
  'Der Erlös richtet sich nach dem echten Holzvorrat und den aktuellen Holzpreisen.': 'The payout follows the real growing stock and current timber prices.',
  'Der Wald bleibt dann für immer außer Nutzung — je älter der Bestand, desto mehr ⚡.': 'The forest then stays unmanaged forever — the older the stand, the more ⚡.',
  'fehlen.': 'to go.',
  'Zum Wald': 'To the forest',
  'Dein Wald wächst nach: Schlag → Jungwuchs → Stangenholz. Hiebsreif in': 'Your forest is regrowing: clear-cut → regrowth → pole stage. Ready to fell in',
  'Kauf dir eine Waldparzelle (Nutzung „Wald“, dunkelgrün). Wald ist billig — und steht voller Holz.': 'Buy a forest plot (land use "Wald", dark green). Forest is cheap — and full of timber.',
  'Wald zeigen': 'Show a forest',
  'Brache': 'Fallow',
  'Weide': 'Pasture',
  'Gepflügt': 'Ploughed',
  'Wächst': 'Growing',
  'Reif!': 'Ripe!',
  'Geerntet': 'Harvested',
  'Abgeerntet': 'Harvested (by farmers)',
  'reif in': 'ripe in',
  'noch': 'for',
  'nächste Ernte in': 'next harvest in',
  'Ernten!': 'Harvest!',
  'Dein Acker': 'Your field',
  'Jetzt!': 'Now!',
  'Geduld': 'Patience',
  'Zum reifen Acker': 'To the ripe field',
  'Zum Acker': 'To the field',
  'Reifen Acker zeigen': 'Show a ripe field',
  'Ernten fehlen.': 'harvests to go.',
  'Äcker reifen alle 60 Minuten — jeder zu seiner Zeit. Ist deiner golden, zeigt ein 🌾-Marker: ernten bringt Münzen. Wartest du zu lang, ernten die Bauern. Oder lass ihn als': 'Fields ripen every 60 minutes — each in its own time. When yours turns golden a 🌾 marker appears: harvesting earns coins. Wait too long and the farmers take it. Or leave it as',
  'liegen — das zählt zum Naturschutz.': '— that counts towards nature conservation.',
  'Äcker reifen alle 60 Minuten. Golden + 🌾-Marker = ernten, sonst tun es die Bauern. Oder als': 'Fields ripen every 60 minutes. Golden + 🌾 marker = harvest, or the farmers will. Or leave it as',
  'liegen lassen — zählt zum Naturschutz.': '— counts towards nature conservation.',
  'Dein erstes Stückerl Land! Öffne es nochmal und wandle es in': 'Your first piece of land! Open it again and convert it to',
  'um — XP und 30 %-Ziel.': '— XP and the 30 % goal.',
  'Riesenbäume sichtbar — goldene Bäume zeigen, wo sie stehen. Eine Parzelle mit so einem Riesen erfüllt': 'Giant trees revealed — golden trees show where they stand. A parcel with such a giant completes',
  'unter normal — Felder tragen nur': 'below normal — fields yield only',
  'Ein 🕳️ Brunnen schützt.': 'A 🕳️ well protects.',
  'Echte Baumhöhen aus Laserscans — und versteckte Riesenbäume. Find zuerst einen Schatz.': 'Real tree heights from laser scans — and hidden giant trees. Find a treasure first.',
  'Werkzeuge & Ebenen': 'Tools & layers',
  'Ein Acker von dir ist reif — die 🌾-Marker zeigen ihn. Tipp drauf und ernte, bevor die Bauern es tun.': 'One of your fields is ripe — the 🌾 marker shows it. Tap it and harvest before the farmers do.',
  'Äcker reifen alle 60 Minuten, jeder zu seiner Zeit. Dein nächster ist in': 'Fields ripen every 60 minutes, each in its own time. Your next one is ripe in',
  'reif — dann erscheint ein 🌾-Marker.': '— a 🌾 marker will appear then.',
  'Kauf dir einen Acker (Nutzung „Äcker/Wiesen/Weiden“). Goldene Felder sind gerade reif — ein Kauf zur Erntezeit zahlt sich sofort aus.': 'Buy a field (use “Äcker/Wiesen/Weiden”). Golden fields are ripe right now — buying at harvest time pays off immediately.',
  'Wandle 3 Parzellen in Wald oder Naturschutz um': 'Convert 3 parcels to forest or nature reserve',
  'Artenforscher': 'Species researcher',
  'Entdecke eine seltene Art der Roten Liste': 'Discover a rare Red List species',
  'Baumriese': 'Tree giant',
  'Kaufe eine Parzelle mit einem Riesenbaum': 'Buy a parcel with a giant tree',
  'Aufgabe': 'Quest',
  'Servus': 'Welcome',
  'Servus in': 'Welcome to',
  'Almhütte': 'alpine hut', 'Hof': 'farmstead', 'aufgelassener Hof': 'abandoned farmstead', 'Gipfel': 'summit',
  'Übergang': 'pass', 'Tal': 'valley', 'Gletscher': 'glacier', 'Gebiet': 'area', 'Riedname': 'field name',
  'Ried': 'Field name', 'Flur': 'Place', 'BEV Geographische Namen': 'BEV official geographic names',
  'Flurnamen sichtbar': 'Place names shown', 'Flurnamen ausgeblendet': 'Place names hidden',
  'Alles hier ist echt — jede Parzelle stammt aus dem österreichischen Kataster.': 'Everything here is real — every parcel comes from the Austrian cadastre.',
  'So geht’s': 'How it works',
  'Tipp auf eine Parzelle und kauf sie dir.': 'Tap a parcel and buy it.',
  'hast du im Börserl.': 'are in your purse.',
  'Was dir gehört, kannst du in 🌿 Naturschutz umwandeln — Ziel: 30 % der Gemeinde.': 'Land you own can be turned into 🌿 nature reserve — goal: 30 % of the municipality.',
  'Unterwegs': 'On the way',
  'Halt die Augen offen nach 💎 Schätzen und 🦎 seltenen Arten der Roten Liste — beides bringt Münzen und XP.': 'Watch out for 💎 treasures and 🦎 rare Red List species — both earn coins and XP.',
  'Deine erste Aufgabe': 'Your first quest',
  'Tipp': 'Tip',
  'Dein erstes Stückerl Land! Mach es noch einmal auf und wandle es in': 'Your first piece of land! Open it again and convert it to',
  'Naturschutz': 'nature reserve',
  'um — das bringt XP und zählt zum 30 %-Ziel.': '— that earns XP and counts towards the 30 % goal.',
  'Freigeschaltet': 'Unlocked',
  'Riesen entdeckt': 'giants discovered',
  'Chronik': 'Chronicle',
  'weitere · näher zoomen': 'more · zoom in',
  'Riesenbaum entdeckt!': 'Giant tree discovered!',
  'Riesen in Sicht — erkunde das Land und finde alle': 'giants in sight — explore the land and find all',
  'Grundstücke mit Riesenbäumen bringen Bonus-XP!': 'Parcels with giant trees earn bonus XP!',
  'von': 'of',
  'entdeckt': 'discovered',
  'Riesenbäume sichtbar! Goldene Bäume zeigen dir, wo sie stehen. Kauf dir eine Parzelle mit so einem Riesen für die Aufgabe': 'Giant trees revealed! Golden trees show where they stand. Buy a parcel with a giant for the quest',
  'Enhanced Gelände': 'Enhanced terrain',
  'Da gibt’s echte Baumhöhen aus Laserscans — und versteckte Riesenbäume. Find zuerst einen Schatz, dann siehst du sie.': 'Real tree heights from laser scans live here — and hidden giant trees. Find a treasure first to see them.',
  'Passt!': 'Done!',
  'Nächste Aufgabe': 'Next quest',
  'Ausblenden': 'Hide',
  'Weiter': 'Next',
  'Antippen für Details': 'Tap for details',
  'Fortschritt': 'Progress',
  'Wo?': 'Where?',
  'Versteckt': 'Hidden',
  'Noch': 'Still',
  'Parzellen fehlen.': 'parcels to go.',
  'Umwandlungen fehlen.': 'conversions to go.',
  'Tipp auf eine Parzelle am Kartenrand — Wiesen und Wald sind billig, Bauland teuer.': 'Tap a parcel on the map — meadows and forest are cheap, building land is pricey.',
  'Günstige Parzelle zeigen': 'Show a cheap parcel',
  'Schatzkisten liegen offen auf der Karte — die nächste ist': 'Treasure chests sit openly on the map — the nearest is',
  'entfernt. Zoom hin und tipp sie an.': 'away. Zoom in and tap it.',
  'entfernt.': 'away.',
  'ist': 'is',
  'Hier liegt gerade kein Schatz. Fahr ein Stück weiter — jede Gemeinde hat welche.': 'No treasure around here right now. Move on a bit — every municipality has some.',
  'Zum Schatz fliegen': 'Fly to treasure',
  'Seltene Arten verstecken sich als 🦎-Marker, oft in Natura-2000-Gebieten (🛡️). Die nächste ist': 'Rare species hide as 🦎 markers, often in Natura 2000 sites (🛡️). The nearest is',
  'Hier ist gerade keine Art bekannt. Schalte 🛡️ Natura 2000 ein und such in Schutzgebieten.': 'No species known around here. Turn on 🛡️ Natura 2000 and search protected areas.',
  'Zur Art fliegen': 'Fly to species',
  'Natura 2000 einblenden': 'Show Natura 2000',
  'Öffne eine Parzelle, die dir gehört, und tipp auf': 'Open a parcel you own and tap',
  'Du hast': 'You have',
  'Eine Parzelle wartet schon auf dich.': 'One parcel is already waiting for you.',
  'Parzellen, die noch warten.': 'parcels still waiting.',
  'Meine Parzelle öffnen': 'Open my parcel',
  'Dafür brauchst du zuerst Land: Kauf eine Parzelle, öffne sie dann noch einmal und wandle sie um.': 'You need land first: buy a parcel, then open it again and convert it.',
  'Riesenbäume zeigen sich erst, wenn du deinen ersten Schatz gefunden hast.': 'Giant trees only appear once you have found your first treasure.',
  'Kauf die Parzelle, auf der ein Riesenbaum steht. Der nächste': 'Buy the parcel a giant tree stands on. The nearest',
  'Goldene Bäume zeigen dir Riesen. Der nächste': 'Golden trees mark giants. The nearest',
  'Riese hier': 'Giant here',
  'Riese': 'giant',
  'In dieser Gegend sind noch keine Riesenbäume geladen — fahr ins ✨ Enhanced Gelände.': 'No giant trees loaded around here — head into ✨ enhanced terrain.',
  'Zum Baum fliegen': 'Fly to tree',

};

// ---------- chat safety ----------
Object.assign(I18N_EXACT, {
  'Chat: frei': 'Chat: free text', 'Chat: Schnellnachrichten': 'Chat: quick phrases', 'Chat: aus': 'Chat: off',
  'Chat-Modus (nur Spielersteller)': 'Chat mode (game creator only)', 'Schnellnachrichten': 'Quick phrases',
  'Chat-Regeln & Sicherheit': 'Chat rules & safety', 'Der Chat ist in diesem Spiel deaktiviert.': 'Chat is disabled in this game.',
  '🛡️ Chat-Regeln': '🛡️ Chat rules', 'Verstanden ✓': 'Got it ✓', 'Später': 'Later', '⚑ Melden': '⚑ Report', 'Melden': 'Report', 'Abbrechen': 'Cancel',
  'Optional: Was ist passiert?': 'Optional: what happened?', 'Chat-Modus geändert': 'Chat mode changed', 'Aufgabe erledigt': 'Quest complete', 'Wird automatisch erledigt': 'Completes automatically', 'Aufgabe noch nicht erfüllt': 'Quest not yet fulfilled', 'blockiert': 'blocked', 'Spieler: ': 'Player: ',
  'Spieler blockieren? Du siehst dann keine Nachrichten mehr von dieser Person.': 'Block this player? You will no longer see their messages.',
  '⚑ Danke für deine Meldung. Der Spieler wurde für dich blockiert.': '⚑ Thanks for reporting. The player has been blocked for you.',
  'Die Nachricht wird sofort ausgeblendet und der Spieler für dich blockiert. Bei mehreren Meldungen wird der Spieler automatisch stummgeschaltet.': 'The message is hidden immediately and the player is blocked for you. Several reports mute the player automatically.',
  'Sei freundlich – keine Beleidigungen, kein Hass.': 'Be kind – no insults, no hate.',
  'Teile nichts Persönliches: kein Alter, keine Adresse, keine Schule, keine Telefonnummer, kein echter Name.': 'Share nothing personal: no age, address, school, phone number or real name.',
  'Keine Links, keine Social-Media-Namen, keine Treffen außerhalb des Spiels.': 'No links, no social-media handles, no meeting up outside the game.',
  'Wenn dir etwas komisch vorkommt: Nachricht melden (⚑) oder Spieler blockieren (🚫) – und einer erwachsenen Vertrauensperson erzählen.': 'If something feels wrong: report the message (⚑) or block the player (🚫) – and tell a trusted adult.',
  'Der Chat wird automatisch gefiltert. Verstöße führen zu Sperren.': 'Chat is filtered automatically. Violations lead to mutes.',
  'Beleidigung / Belästigung': 'Insult / harassment', 'Hassrede': 'Hate speech', 'Sexuelle Inhalte': 'Sexual content',
  'Fragt nach Alter, Fotos, Treffen oder Kontakt': 'Asks for age, photos, meeting or contact', 'Teilt persönliche Daten': 'Shares personal data', 'Spam / Werbung': 'Spam / advertising', 'Sonstiges': 'Other',
  'Hallo! 👋': 'Hello! 👋', 'Gut gespielt! 👏': 'Well played! 👏', 'Danke!': 'Thanks!', 'Ja': 'Yes', 'Nein': 'No', 'Schau mal hier! 📍': 'Look here! 📍',
  'Ich brauche Hilfe': 'I need help', 'Wollen wir tauschen?': 'Want to trade?', 'Bis später!': 'See you later!', 'Glückwunsch! 🎉': 'Congrats! 🎉',
  'Schöne Parzelle!': 'Nice parcel!', 'Lass uns Natur schützen 🌿': "Let's protect nature 🌿", 'Gute Idee!': 'Good idea!', 'Moment...': 'One moment...',
  'Dein Chat ist dauerhaft gesperrt.': 'Your chat is permanently disabled.', 'In diesem Spiel sind nur Schnellnachrichten erlaubt.': 'Only quick phrases are allowed in this game.',
  'Nur der Spielersteller kann den Chat-Modus ändern.': 'Only the game creator can change the chat mode.', 'Langsam! Bitte warte ein paar Sekunden.': 'Slow down! Please wait a few seconds.',
  'Diese Nachricht hast du gerade schon gesendet.': 'You just sent that message.', '🔒 Bitte keine Telefonnummern im Chat teilen – zu deiner Sicherheit.': '🔒 Please don\'t share phone numbers in chat – for your safety.',
  '🔒 Bitte keine E-Mail-Adressen im Chat teilen.': '🔒 Please don\'t share e-mail addresses in chat.', '🔒 Links sind im Chat nicht erlaubt.': '🔒 Links are not allowed in chat.',
  '🔒 Bitte keine Kontaktdaten oder Social-Media-Namen austauschen – der Chat bleibt hier im Spiel.': '🔒 Please don\'t exchange contact details or social-media handles – chat stays in the game.',
  '⛔ Diese Nachricht wurde blockiert. Fragen nach Alter, Wohnort, Fotos oder Treffen sind hier nicht erlaubt.': '⛔ Message blocked. Asking for age, location, photos or meeting up is not allowed here.',
  '⛔ Sexuelle Inhalte sind hier nicht erlaubt.': '⛔ Sexual content is not allowed here.', '⛔ Beleidigungen und Hassrede sind nicht erlaubt.': '⛔ Insults and hate speech are not allowed.',
  '🔒 Bitte verrate im Chat nichts Persönliches über dich (Alter, Wohnort, Schule, echter Name).': '🔒 Please don\'t reveal personal details about yourself in chat (age, location, school, real name).',
  'ab 14 Jahren': 'ages 14+', 'Beta · keine Cookies · kein Tracking · Chat automatisch gefiltert · ': 'Beta · no cookies · no tracking · chat auto-filtered · ', ' (jünger nur mit Einwilligung der Eltern)': ' (younger only with parental consent)',
});

// ---------- water & Gemeinde-Chronik (GW-1…8, HOLZ-1, FARM-1) ----------
Object.assign(I18N_EXACT, {
  '💧 Grundwasser': '💧 Groundwater', 'Grundwasser heute': 'Groundwater today', 'Gemeinde-Chronik öffnen': 'Open municipality chronicle',
  'Grundwasser heute — Gemeinde-Chronik öffnen': 'Groundwater today — open municipality chronicle',
  '📖 Gemeinde-Chronik': '📖 Municipality Chronicle', 'Gemeinde-Chronik': 'Municipality Chronicle', 'Chronik': 'Chronicle', 'Wasser · Wald · Höfe': 'Water · Forest · Farms',
  '💧 Wasser': '💧 Water', '🌲 Wald': '🌲 Forest', '🚜 Höfe': '🚜 Farms', '📏 Messstelle': '📏 Gauging station',
  'Chronik wird aufgeschlagen…': 'Opening the chronicle…', 'Chronik gerade nicht erreichbar — bitte nochmal antippen': 'Chronicle unavailable right now — please tap again',
  'Noch keine Katastralgemeinde geladen — zoom näher ran.': 'No cadastral municipality loaded yet — zoom in.',
  'normal': 'normal', 'niedrig': 'low', 'sehr niedrig': 'very low', 'hoch': 'high', 'keine Messung': 'no reading', 'keine Live-Messung': 'no live reading',
  'gut': 'good', 'beobachten': 'watch', 'belastet': 'stressed', 'Dürre': 'Drought', 'trocken': 'dry', 'schwere Dürre': 'severe drought', 'Heute': 'Today', 'Wasserstress': 'Water stress',
  'über Grenzwert': 'above limit', 'erhöht': 'elevated', 'unauffällig': 'unremarkable', 'Dürre-Risiko': 'Drought risk', 'der Jahre': 'of years', 'schlimmstes': 'worst',
  'Messstellen': 'Gauging stations', 'Messstelle': 'Gauging station', 'Dürre-Kalender': 'Drought calendar', 'Ø Klasse pro Monat': 'Ø class per month', 'Trockenheit': 'Dryness',
  'Ernte heute': 'Harvest today', 'Brunnen schützt': 'Well protects', 'Dürre: Felder tragen weniger — Brunnen und Brache lohnen sich.': 'Drought: fields yield less — wells and fallow pay off.',
  'Weg des Wassers': 'Path of the water', 'Wasserweg': 'Water path', 'Fließweg': 'Flow path', 'bis zur Grenze': 'to the border', 'Angekommen': 'Arrived',
  'Der Tropfen sucht seinen Bach…': 'The drop is looking for its brook…', 'Flussdaten werden geladen — gleich nochmal': 'River data loading — try again shortly', 'Kein Fließweg gefunden': 'No flow path found',
  'Schwarzes Meer': 'Black Sea', 'Nordsee': 'North Sea', 'Adria': 'Adriatic', 'Mittelmeer': 'Mediterranean', 'Wasserweg ausblenden': 'Hide water path',
  'Waldfläche': 'Forest area', 'Verlust': 'Loss', 'letztes Jahr': 'last year', 'seit 2001': 'since 2001', 'Ernte': 'Harvest', 'gebunden/Jahr': 'stored/year', 'Fichte': 'Spruce',
  'Waldverlust': 'Forest loss', 'je dichter der Bestand, desto mehr XP; zählt zum 30-%-Ziel.': 'the denser the stand, the more XP; counts toward the 30 % goal.',
  'Betriebe': 'Farms', 'Förderung': 'Subsidy', 'Median': 'Median', 'Bergbauern': 'Mountain farms', 'Hoftypen': 'Farm types', 'Top-Maßnahmen': 'Top measures', 'pro Ernte': 'per harvest',
  'Wiesen holen sie alle 60 Minuten ab, Bio-Schläge kriegen mehr.': 'Meadows collect it every 60 minutes, organic plots get more.',
  'Keine Grundwasserdaten für diese Gemeinde.': 'No groundwater data for this municipality.', 'Keine Walddaten für diese Gemeinde.': 'No forest data for this municipality.', 'Keine Förderdaten für diese Gemeinde.': 'No subsidy data for this municipality.',
  'Förderung abholen': 'Collect subsidy', 'Nächste Auszahlung in': 'Next payout in', 'alle 60 min': 'every 60 min', 'keine Förderung': 'no subsidy', 'Bio-Prämie': 'organic premium', 'Ernten': 'Harvest', 'in': 'in',
  'holt Förderung': 'collects subsidy', 'Geschützt': 'Protected', 'Kataster nicht erreichbar – neuer Versuch in': 'Cadastre unreachable – retrying in', 'Rechtsbezüge': 'Legal references', 'findet einen Schatz': 'finds a treasure', 'Brunnen': 'Well', 'Brunnen graben': 'Dig well', 'Brunnen gegraben': 'Well dug', 'gräbt einen Brunnen': 'digs a well', 'schützt': 'protects', 'der Ernte': 'of the harvest',
  'Grundwasser in': 'Groundwater at', 'der Ernte vor Dürre': 'of the harvest from drought', 'porous aquifer': 'porous aquifer', 'karst aquifer': 'karst aquifer',
  'Pegelwart': 'Gauge keeper', 'für die Messstelle': 'for the gauging station', 'Wer diese Parzelle kauft, wird': 'Whoever buys this parcel becomes',
  'Grundwasser-Messstelle': 'Groundwater station', 'Nitrat-Messstelle': 'Nitrate station', 'Wasserkraftwerk': 'Hydropower plant', 'Gewässergüte-Messstelle': 'Water quality site',
  'Art': 'Type', 'Pegel': 'Level', 'Trend': 'Trend', 'signifikant': 'significant', 'Leistung': 'Capacity', 'Chemie': 'Chemistry', 'Ökologie': 'Ecology', 'Risiko': 'Risk', 'Verlauf': 'History', 'Lade Verlauf…': 'Loading history…', 'Kein Verlauf verfügbar': 'No history available',
  'Good': 'Good', 'Poor': 'Poor', 'Parzelle': 'Parcel',
  'Schutzgebiet': 'Protected area', 'Wasserschutzgebiet': 'Water protection area', 'Wasserschongebiet': 'Water conservation area', 'Wasserschutz': 'Water protection', 'Schongebiet': 'Conservation area',
  'Trinkwasser-Bonus': 'drinking-water bonus', 'Wasserschutzgebiet: Trinkwasser-Bonus ×1,5': 'Water protection area: drinking-water bonus ×1.5',
  'Schutzgebiete sichtbar': 'Protected areas shown', 'Schutzgebiete ausgeblendet': 'Protected areas hidden', 'Schutzgebiete (Natura 2000 & Wasserschutz) ein/aus': 'Protected areas (Natura 2000 & water protection) on/off',
  'Das Grundwasser steht hier': 'Groundwater here is', 'unter normal — deine Felder tragen nur': 'below normal — your fields yield only', 'Ein 🕳️ Brunnen schützt, Brache zählt zum Naturschutz.': 'A 🕳️ well protects, fallow counts as conservation.',
  '💧 Wasserstress: ': '💧 Water stress: ', ' beobachten ': ' watch ', ' belastet': ' stressed', 'Grundwasser-Status-Index (groundwater-at, CC BY 4.0)': 'Groundwater status index (groundwater-at, CC BY 4.0)',
});

const I18N_RX = [
  // ---------- registration / joining ----------
  [/^🎉 Servus, (.+)!$/, '🎉 Welcome, $1!'],
  [/^Fehler beim Beitreten: (.+)$/, 'Error joining: $1'],
  [/^Fehler beim Zufallsstart: (.+)$/, 'Random start failed: $1'],
  [/^Lade (.+)\.\.\.$/, 'Loading $1...'],
  [/^(.*) Dein Chat ist für (.+) gesperrt\.$/, function(_,a,b){return trx(a)+' Your chat is muted for '+b.replace('Min.','min').replace('Std.','h').replace('Tagen','days')+'.';}],
  [/^Dein Chat ist noch (.+) gesperrt\.$/, function(_,b){return 'Your chat is still muted for '+b.replace('Min.','min').replace('Std.','h').replace('Tagen','days')+'.';}],
  [/^Dein Chat wurde dauerhaft gesperrt\.$/, 'Your chat has been permanently disabled.'],

  [/^(\d+) Aufgaben erledigt$/, '$1 quests completed'],

  // ---------- loading progress ----------
  [/^⏳ Lade Gelände…$/, '⏳ Loading terrain…'],
  [/^⏳ Kataster wird vom Datenarchiv geholt…$/, '⏳ Fetching cadastre from the data archive…'],
  [/^Archiv langsam$/, 'archive slow'],
  [/^Katasterdaten derzeit nicht erreichbar – wir versuchen es gleich wieder\.$/, 'Cadastre data currently unreachable – retrying shortly.'],
  [/^(\d+) Parzellen gefunden$/, '$1 parcels found'],
  [/^(\d+) Polygon-Geometrien, (\d+) Gebäude geladen$/, '$1 polygon geometries, $2 buildings loaded'],
  [/^(\d+) Parzellen, (\d+) Gebäude geladen$/, '$1 parcels, $2 buildings loaded'],
  [/^(\d+) seltene Arten versteckt, (\d+) Schätze total$/, '$1 rare species hidden, $2 treasures total'],

  // ---------- SSE multiplayer events ----------
  [/^⚔️ (.+) beigetreten!$/, '⚔️ $1 joined!'],
  [/^💰 (.+) verkauft$/, '💰 $1 sold'],
  [/^📋 (.+) → EZ (.+) \((\d+) Parzellen\)$/, '📋 $1 → EZ $2 ($3 parcels)'],
  [/^🏆 (.+) Aufgabe!$/, '🏆 $1 completed a quest!'],
  [/^📨 (.+) bietet (\d+)🪙 für deine Parzelle!$/, '📨 $1 offers $2🪙 for your parcel!'],
  [/^✅ (.+) kauft Parzelle von (.+) für (\d+)🪙$/, '✅ $1 buys a parcel from $2 for $3🪙'],
  [/^⚠️ Du brauchst (\d+)🪙 aber hast nur (\d+)🪙 — verkaufe Parzellen!$/, '⚠️ You need $1🪙 but only have $2🪙 — sell some parcels!'],

  // ---------- buy / sell / convert / EZ ----------
  [/^🏴 Gekauft für (\d+)🪙! 🌲 Riesenbaum-Bonus: \+(\d+)⚡$/, '🏴 Bought for $1🪙! 🌲 Giant tree bonus: +$2⚡'],
  [/^🏴 Gekauft für (\d+)🪙!$/, '🏴 Bought for $1🪙!'],
  [/^🌿 Umgewandelt! \+(\d+)⚡$/, '🌿 Converted! +$1⚡'],
  [/^💰 Verkauft für (\d+)🪙$/, '💰 Sold for $1🪙'],
  [/^📋 EZ (.+): (\d+) Parzellen \((.+) gespart!\)$/, '📋 EZ $1: $2 parcels ($3 saved!)'],
  [/^Nicht genug Münzen! Du hast (\d+)🪙$/, 'Not enough coins! You have $1🪙'],
  [/^📨 Angebot gesendet: (\d+)🪙$/, '📨 Offer sent: $1🪙'],

  // ---------- treasures / species ----------
  // two-line species toast: "🦎 [🛡️ Natura-2000-Bonus! ]Artenfund: Uhu (Bubo bubo)\n🟢 Nicht gefährdet — +600🪙"
  [/^🦎 (🛡️ Natura-2000-Bonus! )?Artenfund: (.+?) \((.+?)\)\n(\S+ )?(.+?) — \+(\d+)🪙$/, function(_,n2k,sp,lat,em,cat,v){ var ex = function(s){ return I18N_EXACT[s] !== undefined ? I18N_EXACT[s] : s; }; return '🦎 ' + (n2k ? '🛡️ Natura 2000 bonus! ' : '') + 'Species found: ' + ex(sp) + ' (' + lat + ')\n' + (em || '') + ex(cat) + ' — +' + v + '🪙'; }],
  [/^🦎 🛡️ Natura-2000-Bonus! Artenfund: ([\s\S]+)$/, '🦎 🛡️ Natura 2000 bonus! Species found: $1'],
  [/^🦎 Artenfund: ([\s\S]+)$/, '🦎 Species found: $1'],
  // two-line roaming toast (built as one German string in claimTreasure)
  [/^🐾 Wildtier-Begegnung: (.+?) \((.+?)\)\nEin Durchzügler — du hast ihn gesichtet, bevor er weiterzog — (.+)$/, function(_,sp,lat,rest){ return '🐾 Wildlife encounter: ' + (I18N_EXACT[sp] !== undefined ? I18N_EXACT[sp] : sp) + ' (' + lat + ')\nA wanderer — you spotted it before it moved on — ' + rest; }],
  [/^💎 Schatz! \+(\d+)(.+)$/, '💎 Treasure! +$1$2'],

  // ---------- giant trees ----------
  [/^🌲 \+(\d+) Riesen entdeckt · Chronik (\d+)\/(\d+)$/, '🌲 +$1 giants discovered · Chronicle $2/$3'],
  [/^🔓 Entdeckermodus: (.+) \((.+) m\) freigeschaltet!$/, '🔓 Explorer mode: $1 ($2 m) unlocked!'],
  [/^🌲 Nächster Riesenbaum: (.+) \((.+) m\)$/, '🌲 Nearest giant tree: $1 ($2 m)'],
  [/^✨ Noch 1 Tap …$/, '✨ 1 more tap …'],
  [/^✨ Noch (\d+) Taps …$/, '✨ $1 more taps …'],
  [/^(\d+)–(\d+) Jahre \(auf (\d+) m Seehöhe\)$/, '$1–$2 years (at $3 m elevation)'],
  [/^(\d+)–(\d+) Jahre$/, '$1–$2 years'],
  [/^(\d+)\. von (\d+) Riesen in der Nähe$/, '#$1 of $2 giants nearby'],
  [/^(\d+) Baum\/Bäume (\d+)–(\d+)m$/, '$1 tree(s) $2–$3m'],

  // ---------- GPS ----------
  [/^📍 Standort nicht verfügbar: (.*)$/, '📍 Location unavailable: $1'],

  // ---------- similar parcels ----------
  [/^🔍 Keine ähnlichen Parzellen im Umkreis von (.+) gefunden$/, '🔍 No similar parcels found within $1'],
  [/^🔍 (\d+) ähnliche Parzellen im Umkreis von (.+) \(von (.+) Kandidaten\) · mit LiDAR-Geländeabgleich ✨$/, '🔍 $1 similar parcels within $2 (of $3 candidates) · with LiDAR terrain matching ✨'],
  [/^🔍 (\d+) ähnliche Parzellen im Umkreis von (.+) \(von (.+) Kandidaten\)$/, '🔍 $1 similar parcels within $2 (of $3 candidates)'],
  [/^⏳ Suche… \((.+), dauert etwas\)$/, '⏳ Searching… ($1, takes a moment)'],
  [/^🔍 Ähnliche Parzellen \((\d+)\)$/, '🔍 Similar parcels ($1)'],

  // ---------- parcel popup values ----------
  [/^🏙️ Dicht \((\d+) Geb\.\)$/, '🏙️ Dense ($1 bldg.)'],
  [/^🏡 Mittel \((\d+) Geb\.\)$/, '🏡 Medium ($1 bldg.)'],
  [/^🌾 Gering \((\d+) Geb\.\)$/, '🌾 Low ($1 bldg.)'],
  [/^🏴 Kaufen \((\d+)🪙\)$/, '🏴 Buy ($1🪙)'],

  // ---------- building rows ----------
  [/^≈ (\d+) m · 1 Etage$/, '≈ $1 m · 1 story'],
  [/^≈ (\d+) m · (\d+) Etagen$/, '≈ $1 m · $2 stories'],

  [/^(.+) \(du\)$/, '$1 (you)'],
  [/^([\d.,]+ Mio €) \((Bauland \(bebaut\)|Bauland|Ackerland|Grünland|Wald|Sonstig)\)$/, function(_,a,c){return a+' ('+({'Bauland (bebaut)':'building land (built-up)','Bauland':'building land','Ackerland':'farmland','Grünland':'grassland','Wald':'forest','Sonstig':'other'})[c]+')';}],
  [/^([\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}\u2696\u26F0]+ )(Erkunde deine Gemeinde|Naturschützer|Landvermesser|Schatzsucher|Waldmeister|Artenforscher|Baumriese|Erntedank|Holzknecht|Waldhüter)$/u, function(_,e,t){return e+({'Erkunde deine Gemeinde':'Explore your municipality','Naturschützer':'Conservationist','Landvermesser':'Land surveyor','Schatzsucher':'Treasure hunter','Waldmeister':'Forest master','Artenforscher':'Species researcher','Baumriese':'Tree giant','Erntedank':'Harvest festival','Holzknecht':'Lumberjack','Waldhüter':'Forest warden'})[t];}],
  // Landuse summary lists like "Sonstige (×5), Straße, Wald" — translate each term.
  [/^([A-Za-zÄÖÜäöüß()×X\d ]+)(, [A-Za-zÄÖÜäöüß()×X\d ]+)+$/, function(m0){
    return m0.split(', ').map(function(part){
      var mm = part.match(/^(.+?)( \([×X]?\d+\))?$/);
      var base = mm[1], suff = mm[2]||'';
      return (I18N_EXACT[base]!==undefined?I18N_EXACT[base]:base)+suff;
    }).join(', ');
  }],
  [/^([A-Za-zÄÖÜäöüß ]+?) (\([×X]?\d+\))$/, function(_,base,suff){return (I18N_EXACT[base]!==undefined?I18N_EXACT[base]:base)+' '+suff;}],
  [/^([A-Za-zÄÖÜäöüß/ ]+?) (\d+ ?%)( …)?$/, function(_,base,pct,t){ return (I18N_EXACT[base]!==undefined?I18N_EXACT[base]:base)+' '+pct+(t||''); }],
  [/^([A-Za-zÄÖÜäöüß ]+?) (\d+%)$/, function(_,base,pct){return (I18N_EXACT[base]!==undefined?I18N_EXACT[base]:base)+' '+pct;}],
  [/^\((Bauland \(bebaut\)|Bauland|Ackerland|Grünland|Wald|Sonstig)\)$/, function(_,c){return '('+({'Bauland (bebaut)':'building land (built-up)','Bauland':'building land','Ackerland':'farmland','Grünland':'grassland','Wald':'forest','Sonstig':'other'})[c]+')';}],
  [/^Spielpreis: (.+?) · (.+?) €\/m² echt$/, 'Game price: $1 · $2 €/m² real'],
  // ---------- supplemental ----------
  [/^⚠️ Gebäude erstreckt sich über (\d+) Parzellen$/, '⚠️ Building spans $1 parcels'],
  [/^⚔️ In (.+)s Spiel$/, "⚔️ Joining $1's game"],
  [/^Zuletzt als (.+) gespielt — (.*)$/, 'Last played as $1 — $2'],
  [/^🗺️ Du verlässt (.+) — Parzellen aus (.+) werden geladen$/, '🗺️ Leaving $1 — loading parcels from $2'],
  [/^(\d+)× \(max (.+)m\) — (.*)$/, '$1× (max $2m) — $3'],
  [/^(.+) · (\d+)% Wald$/, '$1 · $2% forest'],
  [/^(\d+)% · (.+) entfernt$/, '$1% · $2 away'],
  [/^Spielpreis: (.+)$/, 'Game price: $1'],
  [/^📨 Angebot: (\d+)🪙 \(wartet\)$/, '📨 Offer: $1🪙 (pending)'],
  [/^Kaufangebot an (.+):$/, 'Purchase offer to $1:'],
  [/^📋 Ganze EZ kaufen: (.+)$/, '📋 Buy whole EZ: $1'],
  [/^\((\d+) frei\)$/, '($1 available)'],
  [/^ \((\d+) Riesen\)$/, ' ($1 giants)'],
  [/^(\d+) Parzellen · (.+)$/, '$1 parcels · $2'],
  [/^(\d+) Parzellen$/, '$1 parcels'],
  [/^Mindestangebot: (\d+) Münzen$/, 'Minimum offer: $1 coins'],
  [/^EZ (\S+) ▸ \((\d+) Parzellen\)$/, 'EZ $1 ▸ ($2 parcels)'],
  [/^([\d.,]+° \S*)( · )(fast eben|leicht schroff|eben|sanft|wellig|mäßig|hügelig|steil|gebirgig|schroff)$/, function(_,a,b,c){return a+b+({'eben':'level','fast eben':'nearly level','sanft':'gentle','wellig':'undulating','mäßig':'moderate','hügelig':'hilly','steil':'steep','gebirgig':'mountainous','schroff':'rugged','leicht schroff':'slightly rugged'})[c];}],
];



// ---------- 2026-10 bilingual pass: welcome/footer, loading, attribution, search, NE, server messages ----------
Object.assign(I18N_EXACT, {
  // index.html welcome / footer / legal
  'Beta · keine Cookies, kein Tracking ·': 'Beta · no cookies, no tracking ·',
  'Kataster & ALS (': 'cadastre & ALS (',
  ', bearbeitet) ·': ', modified) ·',
  'Lizenzen': 'Licences',
  '🤖 Für Agenten': '🤖 For agents',
  'Text-Edition für KI-Agenten': 'Text edition for AI agents',
  'Nachrichten werden automatisch geprüft. Melde alles, was dir unangenehm ist – wir sehen es uns an. Bei Gefahr:': 'Messages are checked automatically. Report anything that makes you uncomfortable – we will look into it. In danger:',
  '(kostenlos, 24h)': '(free, 24h)',
  'Gemeinde, KG, Adresse, PLZ, Grundstück…': 'Municipality, KG, address, postal code, parcel…',
  'Ort, KG, Adresse, Grundstück 68/3 … (/)': 'Place, KG, address, parcel 68/3 … (/)',
  'Zurück zur Gemeinde-Auswahl': 'Back to municipality picker',
  'Link zum Wiedereinstieg in die Zwischenablage': 'Copy rejoin link to clipboard',
  'Einladungs-Link für Mitspieler in die Zwischenablage': 'Copy invite link for other players to clipboard',
  '🔑 Wiedereinstieg': '🔑 Rejoin', '⚔️ Einladen': '⚔️ Invite',
  'Münzen': 'Coins', '✅ Kopiert!': '✅ Copied!', 'Grundwasser': 'Groundwater', 'du': 'you',
  'Flurnamen & Ortsnamen (BEV) ein/aus': 'Field & place names (BEV) on/off',
  'Datenquellen & Lizenzen': 'Data sources & licences',
  'Beobachtung (LiDAR & Satellit) vs. Kataster — aus / Abweichungen / Kronendach': 'Observation (LiDAR & satellite) vs. cadastre — off / discrepancies / canopy',
  '🚧 Außerhalb Österreichs — keine Katasterdaten': '🚧 Outside Austria — no cadastre data',
  // loading screen
  'Landschaft: Relief, Bäume, Gebäude, Straßen & Flüsse': 'Landscape: relief, trees, buildings, roads & rivers',
  'Kataster live aus BEV-Kacheln (Parzellen, Gebäude, Nutzung)': 'Cadastre live from BEV tiles (parcels, buildings, land use)',
  'Landschaft wird geladen …': 'Loading landscape …',
  'Bedrohte Arten und Schätze werden platziert …': 'Placing endangered species and treasures …',
  'seltene Arten versteckt': 'rare species hidden', 'Schätze total': 'treasures in total',
  '⏳ Kataster wird geladen …': '⏳ Loading cadastre …', 'Kataster wird geladen …': 'Loading cadastre …',
  '⏳ Kataster wird live aus BEV-Kacheln zusammengesetzt …': '⏳ Assembling cadastre live from BEV tiles …',
  'Kataster wird live aus BEV-Kacheln zusammengesetzt …': 'Assembling cadastre live from BEV tiles …',
  'Kataster kommt gleich nach – die Karte öffnet schon': 'Cadastre follows in a moment – the map is opening already',
  'Zelle': 'cell', 'Zellen': 'cells', 'Parzellen': 'Parcels',
  'aus Zwischenspeicher': 'from cache', 'live aus BEV-Kacheln': 'live from BEV tiles',
  'vorgewärmt': 'prewarmed', 'Gelände': 'terrain', 'kein Vorschlag': 'no suggestion',
  'Kataster gerade nicht erreichbar': 'Cadastre currently unreachable',
  'Enhanced Gelände 🌲': 'Enhanced terrain 🌲',
  // map attribution rows
  'Kataster live aus den': 'Cadastre assembled live from',
  'BEV-Kacheln': 'BEV tiles',
  'zusammengesetzt (max. 24 h zwischengespeichert) · Höhenmodell, Bäume & Gebäudehöhen: BEV ALS, CC BY 4.0, bearbeitet · Flur- & Ortsnamen: BEV DLM Geographische Namen, Stichtag 2025-03-25, CC BY 4.0, bearbeitet ·': '(cached ≤ 24 h) · Elevation model, trees & building heights: BEV ALS, CC BY 4.0, modified · Field & place names: BEV DLM Geographic Names, as of 2025-03-25, CC BY 4.0, modified ·',
  'Beobachtete Landschaft (Bewuchs, Baumkronen, Bauwerkshöhen, Veränderung):': 'Observed landscape (vegetation, tree crowns, structure heights, change):',
  ', NE-Zellen, CC BY 4.0 — Datenquelle BEV ALS/DOP (CC BY 4.0, bearbeitet), Contains modified Copernicus Sentinel data 2022–2025, © ESA WorldCover 2021, Hansen GFC 2000–2024 v1.12 · Abgleich mit deklarierter Nutzung: BEV Kataster, CC BY 4.0, bearbeitet (Statistik je Zelle, keine Objektgeometrie, via': ', NE cells, CC BY 4.0 — data source BEV ALS/DOP (CC BY 4.0, modified), Contains modified Copernicus Sentinel data 2022–2025, © ESA WorldCover 2021, Hansen GFC 2000–2024 v1.12 · Comparison with declared land use: BEV cadastre, CC BY 4.0, modified (statistics per cell, no object geometry, via',
  'Schutzgebiete: Source: European Environment Agency (EEA), Natura 2000 · UNEP-WCMC & IUCN WDPA · Gemeinden & Bodenpreise: Statistik Austria (CC BY 4.0, modelliert) · Rechtsbezüge: RIS, Bundeskanzleramt · Landbedeckung: Copernicus / ESA WorldCover 2021 (CC BY 4.0) · Umfeld via': 'Protected areas: Source: European Environment Agency (EEA), Natura 2000 · UNEP-WCMC & IUCN WDPA · Municipalities & land prices: Statistik Austria (CC BY 4.0, modelled) · Legal references: RIS, Federal Chancellery · Land cover: Copernicus / ESA WorldCover 2021 (CC BY 4.0) · Surroundings via',
  ', Landschaft via': ', landscape via',
  'Felder (Schläge) & Hofstellen:': 'Fields & farmsteads:',
  'via data.gv.at, CC BY 4.0 (aggregiert, keine Namen) · Förderprofile: AMA Transparenzdatenbank, Gemeinde-Aggregat · Waldverlust & CO₂: Hansen/GFW GFC-2024, Harris et al. (CC BY 4.0) · Holzpreise: LK Holzmarktberichte / Statistik Austria (CC BY 4.0)': 'via data.gv.at, CC BY 4.0 (aggregated, no names) · Subsidy profiles: AMA transparency database, municipality aggregate · Forest loss & CO₂: Hansen/GFW GFC-2024, Harris et al. (CC BY 4.0) · Timber prices: LK timber market reports / Statistik Austria (CC BY 4.0)',
  'Grundwasser, Pegel & Wasserschutzgebiete:': 'Groundwater, gauges & water protection areas:',
  ', Wasserschatz 2021, WISE/EEA (CC BY 4.0) · Fließweg: MERIT Hydro / mghydro.com (CC BY-NC-SA 4.0, nur Darstellung) · Dürre: Copernicus EDO': ', Wasserschatz 2021, WISE/EEA (CC BY 4.0) · Flow path: MERIT Hydro / mghydro.com (CC BY-NC-SA 4.0, display only) · Drought: Copernicus EDO',
  'Relief: BEV ALS-DTM 25 m (CC BY 4.0) · Baumhöhen & Gebäudehöhen: ALS-Ableitung (BEV, CC BY 4.0) · Staatsgrenze: geoBoundaries gbOpen (CC BY-SA 4.0) · Rote Liste: IUCN / EEA': 'Relief: BEV ALS-DTM 25 m (CC BY 4.0) · Tree & building heights: ALS derivative (BEV, CC BY 4.0) · State border: geoBoundaries gbOpen (CC BY-SA 4.0) · Red List: IUCN / EEA',
  // search
  'Suche …': 'Searching …', 'Grundstück': 'Parcel', 'Gemeinden': 'Municipalities', 'Katastralgemeinden': 'Cadastral municipalities', 'Orte': 'Places', 'Adressen': 'Addresses', 'Ähnliche Namen': 'Similar names', 'Zuletzt gesucht': 'Recent searches',
  'Orte & Fluren': 'Places & fields', 'nicht gefunden': 'not found', '– Nummer prüfen': '– check the number',
  'Du verlässt': 'Leaving', '🗺️ Du verlässt': '🗺️ Leaving', '— Parzellen aus': '— loading parcels from', 'werden geladen': '',
  // quests / herald
  'Hier weicht die Beobachtung (LiDAR & Satellit) vom Kataster ab:': 'Here the observation (LiDAR & satellite) differs from the cadastre:',
  'entfernt. Kauf die Parzelle — Naturschutz auf einer Waldverlust-Fläche zählt doppelt.': 'away. Buy the parcel — a nature reserve on a forest-loss area counts double.',
  'Schalte die Beobachtungs-Karte 👁 ein: orange = Waldverlust, rot = neu versiegelt oder Bauwerk nicht im Kataster. Kauf so eine Parzelle.': 'Turn on the observation map 👁: orange = forest loss, red = newly sealed or structure missing from the cadastre. Buy such a parcel.',
  'Beobachtung einblenden': 'Show observation', 'Spurenleser': 'Tracker',
  'Kaufe eine Parzelle, bei der die Beobachtung vom Kataster abweicht': 'Buy a parcel where the observation differs from the cadastre',
  'Wiederbewaldung!': 'Reforestation!', 'Beobachteter Waldverlust unter Schutz': 'Observed forest loss now protected',
  'Der Kompass führt dich hin': 'The compass leads you there',
  'Spuren in der Gegend — ein Durchzügler ist hier unterwegs': 'Tracks in the area — a wanderer is passing through',
  'Geheimnis entdeckt: Schilder zerschlagen bringt Münzen — je frischer das Schild, desto mehr!': 'Secret found: smashing signs earns coins — the fresher the sign, the more!',
  'Schilderstürmer! 25 Schilder zerlegt.': 'Sign smasher! 25 signs wrecked.',
  // chat / multiplayer
  '🚫 Nachricht entfernt': '🚫 Message removed',
  'Noch keine Nachrichten — sag Hallo! Mitspieler sehen den Chat sofort.': 'No messages yet — say hello! Other players see the chat instantly.',
  'gräbt einen Brunnen': 'digs a well',
  // treasures / rarity (canvas labels go through tr())
  'Schatz': 'Treasure', 'Erfahrung': 'Experience', 'Seltener Samen': 'Rare seed', 'Alte Karte': 'Old map', 'Durchzügler': 'Wanderer',
  'zieht weiter': 'moves on',
  'Wildtier-Begegnung': 'Wildlife encounter', 'Ein Durchzügler — du hast ihn gesichtet!': 'A wanderer — you spotted it!',
  // species (German common names from srv/server.go + srv/treasures.go)
  'Eurasischer Luchs': 'Eurasian lynx', 'Mopsfledermaus': 'Barbastelle bat', 'Feldhamster': 'European hamster', 'Wisent': 'European bison',
  'Steinadler': 'Golden eagle', 'Uhu': 'Eagle-owl', 'Schwarzstorch': 'Black stork', 'Großtrappe': 'Great bustard', 'Auerhahn': 'Capercaillie',
  'Wald-Wiesenvögelchen': 'Scarce heath', 'Goldene Acht': 'Lesser clouded yellow', 'Apollofalter': 'Apollo butterfly',
  'Rotbauchunke': 'Fire-bellied toad', 'Wiesenotter': 'Meadow viper', 'Donau-Kammmolch': 'Danube crested newt',
  'Vogel-Azurjungfer': 'Ornate bluet', 'Große Quelljungfer': 'Balkan goldenring', 'Huchen': 'Danube salmon', 'Sterlet': 'Sterlet',
  'Vielfraß': 'Wolverine', 'Elch': 'Moose', 'Goldschakal': 'Golden jackal', 'Wolf': 'Wolf', 'Wildkatze': 'Wildcat',
  'Fischotter': 'Otter', 'Biber': 'Beaver', 'Kranich': 'Crane', 'Weißstorch': 'White stork', 'Bartgeier': 'Bearded vulture',
  // giant tree names (adjective + noun, see giantTreeName)
  'Ehrwürdiger': 'Venerable', 'Flüsternder': 'Whispering', 'Uralter': 'Ancient', 'Schlafender': 'Sleeping', 'Erwachter': 'Awakened',
  'Singender': 'Singing', 'Träumender': 'Dreaming', 'Wandernder': 'Wandering', 'Leuchtender': 'Shining', 'Verwunschener': 'Enchanted', 'Erhabener': 'Sublime',
  'Wolkenwächter': 'Cloud Warden', 'Himmelsgreifer': 'Sky Reacher', 'Sturmhüter': 'Storm Keeper', 'Waldkönig': 'Forest King',
  'Nebelfürst': 'Mist Prince', 'Wurzelweiser': 'Root Sage', 'Sternenlauscher': 'Star Listener', 'Riesenherz': 'Giant Heart', 'Donnerwipfel': 'Thunder Crown',
  'Morgengrauen': 'Daybreak', 'Ahnenbaum': 'Ancestor Tree', 'Bergflüsterer': 'Mountain Whisperer', 'Lichtfänger': 'Light Catcher', 'Windtänzer': 'Wind Dancer',
  'Zeitzeuge': 'Witness of Time', 'Kronenträger': 'Crown Bearer',
  // KG card / popup
  'Daten momentan nicht erreichbar — bitte nochmal antippen': 'Data currently unavailable — please tap again',
  'Nutzung (nach Fläche)': 'Land use (by area)',
  'Als Referenz': 'As reference',
  '🔍 Ähnliche in der Nähe': '🔍 Similar nearby',
  'Suche ähnliche Parzellen…': 'Searching for similar parcels…', 'Suche ähnliche Parzellen in der Nähe…': 'Searching for similar parcels nearby…',
  'Ähnliche Parzellen in der Nähe': 'Similar parcels nearby',
  '🔍 Keine ähnlichen Parzellen in der Nähe gefunden': '🔍 No similar parcels found nearby',
  'ähnliche Parzellen in der Nähe': 'similar parcels nearby', 'verglichen': 'compared',
  'kein Einschlag seit 2001': 'no logging since 2001', 'Bilanz seit 2001': 'balance since 2001',
  'Waldgeschichte': 'Forest history', 'Jungbestand': 'young stand', 'Vorrat': 'stock',
  '🧪 Nitrat': '🧪 Nitrate', '🌿 Bio': '🌿 Organic',
  'Bergbauer': 'Mountain farm', 'Bio-Bergbauer': 'Organic mountain farm', 'Bio': 'Organic', 'Ackerbau': 'Arable', 'Viehhaltung': 'Livestock',
  'Wein': 'Wine', 'Obst': 'Fruit', 'ohne Fläche': 'landless', 'sonstige': 'other',
  'kleiner Hof': 'small farm', 'mittlerer Hof': 'medium farm', 'großer Hof': 'large farm',
  'Getreide': 'Cereals', 'Mais': 'Maize', 'Feldfrucht': 'Field crop',
  // NE / observed layer
  '👁 Beobachtet': '👁 Observed', '📡 Veränderung': '📡 Change', '📐 Abweichung': '📐 Discrepancy', '🌲 Bäume': '🌲 Trees',
  '🏠 Bauwerke': '🏠 Structures', '🛰️ Satellit': '🛰️ Satellite', '📜 Gefüge': '📜 Structure',
  'Raster auf der Karte': 'grid on the map', 'Kataster hier lückenhaft': 'cadastre incomplete here',
  'Waldverlust beobachtet': 'forest loss observed', 'Wald nachgewachsen': 'forest regrown', 'neu versiegelt': 'newly sealed',
  'Bauwerk nicht im Kataster': 'structure not in the cadastre', 'begrünt (nicht im Kataster)': 'greened (not in the cadastre)',
  'kein Befund': 'no finding', 'neu begrünt': 'newly greened',
  'In dieser Gegend gibt es noch keine Beobachtungsdaten (srtm v2.4).': 'No observation data in this area yet (srtm v2.4).',
  'Kronendach (LiDAR) eingeblendet': 'Canopy (LiDAR) shown', 'Beobachtung ausgeblendet': 'Observation hidden',
  'Beobachtung vs. Kataster als Schleier: orange Waldverlust · rot neu versiegelt · grün nachgewachsen — Details beim Antippen einer Parzelle': 'Observation vs. cadastre as a veil: orange forest loss · red newly sealed · green regrown — details when tapping a parcel',
  'Tanne': 'Fir', 'Buche': 'Beech', 'Eiche': 'Oak', 'Ahorn': 'Maple', 'Esche': 'Ash', 'Birke': 'Birch', 'Erle': 'Alder', 'Pappel': 'Poplar',
  'Obstbaum': 'Fruit tree', 'Nadelholz': 'Conifer',
  'gestresst': 'stressed', 'absterbend': 'declining', 'tot': 'dead',
  'Dach': 'Roof', 'Glashaus': 'Greenhouse', 'PV-Anlage': 'Solar panel', 'Mast': 'Mast', 'Windrad': 'Wind turbine', 'Umspannwerk': 'Substation', 'Mauer': 'Wall', 'Zaun': 'Fence',
  'Ackerkultur': 'Crop', 'Saisonbewuchs': 'Seasonal vegetation', 'versiegelt/offen': 'sealed/bare',
  'Anteil rechtsverbindlich vermessener Grenzen': 'Share of legally binding surveyed boundaries',
  'Gewässergüte-Messstelle': 'Water quality site',
  // server messages (srv/server.go, water.go, timber.go, search.go — canonical German)
  'Dieser Name ist nicht erlaubt': 'This name is not allowed',
  'Das ist kein Wald': 'That is not a forest', 'Das ist kein Acker': 'That is not a field',
  'Diese Parzelle wird nicht mehr bewirtschaftet': 'This parcel is no longer farmed',
  'Das Feld ist noch nicht reif': 'The field is not ripe yet',
  'Schon geerntet — das Feld muss erst wieder wachsen': 'Already harvested — the field has to grow back first',
  'Eine Weide erntet man nicht — die Kühe machen das': 'You do not harvest a pasture — the cows do that',
  'Angebot muss zwischen 10 und 1.000.000 Münzen liegen': 'Offer must be between 10 and 1,000,000 coins',
  'Parzelle nicht gefunden': 'Parcel not found', 'Du besitzt diese Parzelle bereits': 'You already own this parcel',
  'Diese Parzelle ist geschützt und nicht verkäuflich': 'This parcel is protected and not for sale',
  'Angebot konnte nicht erstellt werden': 'Offer could not be created', 'Angebot gesendet!': 'Offer sent!',
  'Angebot nicht gefunden': 'Offer not found', 'Angebot nicht mehr gültig': 'Offer no longer valid', 'Nicht dein Angebot': 'Not your offer',
  'Käufer nicht gefunden': 'Buyer not found', 'Aufgabe noch nicht erfüllt': 'Quest not completed yet',
  'Zu viele Meldungen – bitte später erneut versuchen.': 'Too many reports – please try again later.',
  'Nicht deine Parzelle': 'Not your parcel', 'Ein Brunnen lohnt sich nur auf Äckern und Wiesen': 'A well only pays off on fields and meadows',
  'Dieser Wald ist außer Nutzung gestellt': 'This forest has been set aside',
  'Grundstück in der KG nicht gefunden (Nummer prüfen)': 'Parcel not found in this KG (check the number)',
  // parcel popup rows
  'Anbau': 'Crop', 'Brache': 'Fallow', 'Grünland': 'Grassland', 'Grünbrache': 'Green fallow',
  'Beobachtung': 'Observation', 'Keine': 'None', '🌾 Minimal': '🌾 Minimal',
  'Adressen werden gesucht …': 'Searching addresses …', 'Suche Adressen & Orte …': 'Searching addresses & places …',
  'Nicht genug Münzen': 'Not enough coins', 'Parzelle gekauft · +120 ⚡': 'Parcel bought · +120 ⚡',
  'Rat auf Draht 147': 'Rat auf Draht 147 (youth helpline)',
  'Flachdach': 'Flat roof', 'Steildach': 'Pitched roof',
  // AMA subsidy schemes (dossier farm tab, farm-subsidies-austria)
  'Einkommensgrundstützung für Nachhaltigkeit': 'Basic income support for sustainability',
  'Regelungen für Klima und Umwelt': 'Eco-schemes for climate and environment',
  'Umwelt-, Klima- und andere Bewirtschaftungsverpflichtungen': 'Environmental, climate and other management commitments',
  'Zahlung für Gebiete mit naturbedingten Benachteiligungen': 'Payment for areas with natural constraints',
  'Ergänzende Umverteilungseinkommensstützung': 'Complementary redistributive income support',
  'Junglandwirte': 'Young farmers', 'Investitionen in materielle Vermögenswerte': 'Investments in physical assets', 'Vorhaben im Weinsektor': 'Wine sector measures', 'Bio-Bergbauer': 'Organic mountain farm',
  // server nsNames (srv/agent.go, plural BEV labels in agent/similar rows)
  'Parkplätze': 'Parking lots', 'Äcker, Wiesen oder Weiden': 'Fields, meadows or pastures', 'Gärten': 'Gardens', 'Weingärten': 'Vineyards',
  'Alpen': 'Alpine pastures', 'Krummholzflächen': 'Krummholz areas', 'Wälder': 'Forests', 'Verbuschte Flächen': 'Scrubland', 'Forststraßen': 'Forest roads',
  'Fließende Gewässer': 'Flowing waters', 'Stehende Gewässer': 'Standing waters', 'Feuchtgebiete': 'Wetlands', 'Vegetationsarme Flächen': 'Sparsely vegetated areas',
  'Betriebsflächen': 'Industrial areas', 'Gewässerrandflächen': 'Riparian areas', 'Verkehrsrandflächen': 'Roadside areas', 'Friedhöfe': 'Cemeteries',
  'Gebäudenebenflächen': 'Building ancillary areas', 'Abbauflächen, Halden, Deponien': 'Quarries, dumps, landfills', 'Fels- und Geröllflächen': 'Rock and scree areas',
  'Schienenverkehrsanlagen': 'Rail facilities', 'Straßenverkehrsanlagen': 'Road facilities', 'Freizeitflächen': 'Recreation areas',
});
I18N_RX.push(
  // "Weingarten 99 %, Straße 1 % …" / "Wald 60 %, Äcker/Wiesen/Weiden 40 %" — landuse share lists
  [/^([^,]+?( \d+ ?%| \([×X]?\d+\))?)(, [^,]+?( \d+ ?%| \([×X]?\d+\))?)+( …)?$/, function(m0){
    var tail = / …$/.test(m0) ? ' …' : ''; var body = tail ? m0.slice(0, -2) : m0;
    var parts = body.split(', '); var any = false;
    var out = parts.map(function(part){ var mm = part.match(/^(.+?)( \d+ ?%| \([×X]?\d+\))?$/); var base = mm[1], suff = mm[2] || ''; var hit = I18N_EXACT[base]; if (hit !== undefined) any = true; return (hit !== undefined ? hit : base) + suff; });
    return any ? out.join(', ') + tail : m0;
  }],
  // crop row "🍇 Wein · 976 m²" / "🌾 Winterweizen · 1,2 ha · 🌿 Bio" (INVEKOS crop names are German data)
  [/^(\S+ )(.+?)( · [\d.,]+ (?:ha|m²))( · 🌿 Bio)?$/, function(_,e,n,a,o){ var hit = I18N_EXACT[n]; return e + (hit !== undefined ? hit : n) + a + (o ? ' · 🌿 Organic' : ''); }],
  [/^LiDAR-Beobachtung ?(.*)$/, 'LiDAR observation $1'],
  [/^(\d+) % Wert$/, '$1 % value'],
  [/^([\d.,]+) km Umkreis$/, 'within $1 km'],
  [/^(· )?([\d,]+) Gst\. je Einlage$/, '$1$2 parcels per folio'],
  [/^· 1 Etage$/, '· 1 story'], [/^· (\d+) Etagen$/, '· $1 stories'],
  [/^(🏙️ Dicht|🏡 Mittel|🌾 Gering|Dicht|Mittel|Gering) \((\d+) Geb\.\)$/, function(_,d,n){ return ({'🏙️ Dicht':'🏙️ Dense','🏡 Mittel':'🏡 Medium','🌾 Gering':'🌾 Low','Dicht':'Dense','Mittel':'Medium','Gering':'Low'})[d]+' ('+n+' bldg.)'; }],
  [/^(.+) \((\d+) Riesen\)$/, '$1 ($2 giants)'],
  [/^(📍 .+), Bezirk (.+)$/, '$1, district $2'],
  [/^(📍 .+), Katastralgemeinde (.+)$/, '$1, cadastral municipality $2'],
  [/^📍 Katastralgemeinde (.+)$/, '📍 Cadastral municipality $1'],
  [/^(.+) ha\/Jahr$/, '$1 ha/year'], [/^(.+) €\/Jahr$/, '$1 €/year'],
  [/^✨ Enhanced — LiDAR-Geländedaten aktiv( · 👁 .+)?$/, '✨ Enhanced — LiDAR terrain data active$1'],
  [/^(.+) · mit LiDAR-Geländeabgleich ✨$/, '$1 · with LiDAR terrain matching ✨'],
  [/^(.+) · (Steildach|Flachdach)$/, function(_,a,r){ return a + ' · ' + ({Steildach:'pitched roof',Flachdach:'flat roof'})[r]; }],
  [/^🐾 (?:Wildtier-Begegnung|Wildlife encounter): (.+?) \((.+)\)$/, function(_,sp,lat){ return '🐾 Wildlife encounter: ' + (I18N_EXACT[sp] !== undefined ? I18N_EXACT[sp] : sp) + ' (' + lat + ')'; }],
  [/^Ein Durchzügler — du hast ihn gesichtet, bevor er weiterzog — (.+)$/, 'A wanderer — you spotted it before it moved on — $1'],
  [/^(\d+) \((\d+) frei\)$/, '$1 ($2 available)'],
  [/^🏴 Gekauft für (\d+)🪙! 🌾 Ried (.+)$/, '🏴 Bought for $1🪙! 🌾 Field $2'],
  [/^🏴 Gekauft für (\d+)🪙! · (.+)$/, '🏴 Bought for $1🪙! · $2'],
  [/^Name war vergeben — du spielst als (.+)$/, 'Name was taken — you are playing as $1'],
  [/^© BEV, (\d{4}) – Datenquelle: Bundesamt für Eich- und Vermessungswesen, Kataster \(CC BY 4\.0\), bearbeitet$/, '© BEV, $1 – data source: Federal Office of Metrology and Surveying, cadastre (CC BY 4.0), modified'],
  [/^Durchsuche (.+) … (\d+)\/(\d+) Zellen$/, 'Searching $1 … $2/$3 cells'],
  [/^(🟩|🔎) Grundstück (.+)$/, '$1 Parcel $2'],
  [/^Grundstück (\S+)$/, 'Parcel $1'],
  [/^KG (.+) · geladen$/, 'KG $1 · loaded'],
  [/^Ortschaft · (.+)$/, 'Locality · $1'],
  [/^💧 (.+) \((\d+)% Wasser\)$/, '💧 $1 ($2% water)'],
  [/^(🪓|🏛|🌾) (.+) erntet (\d+)🪙( ☀️)?$/, '$1 $2 harvests $3🪙$4'],
  [/^🌳 Naturwald! ~(\d+) t CO₂ bleiben im Wald · \+(\d+)⚡$/, '🌳 Wild forest! ~$1 t CO₂ stay in the forest · +$2⚡'],
  [/^💰 Verkauft für (\d+)🪙 · (\d+) % Wert( · −\d+⚡)?$/, '💰 Sold for $1🪙 · $2 % value$3'],
  [/^💰 Verkauft für (\d+)🪙 · −(\d+)⚡$/, '💰 Sold for $1🪙 · −$2⚡'],
  [/^✨ Enhanced — LiDAR-Geländedaten aktiv · 👁 beobachtet$/, '✨ Enhanced — LiDAR terrain data active · 👁 observed'],
  [/^🔍 Ähnliche in der Nähe \((\d+)\)$/, '🔍 Similar nearby ($1)'],
  [/^Ähnliche Parzellen in der Nähe( \(\d+\))? · (.+)$/, 'Similar parcels nearby$1 · $2'],
  [/^🔍 (\d+) ähnliche Parzellen in der Nähe \((.+) verglichen(, \d+ Zellen)?\)( · mit LiDAR-Geländeabgleich ✨)?$/, function(_,n,c,z,l){return '🔍 '+n+' similar parcels nearby ('+c+' compared'+(z?z.replace('Zellen','cells'):'')+')'+(l?' · with LiDAR terrain matching ✨':'');}],
  [/^([+-]?[\d.]+) m seit letzter Befliegung$/, '$1 m since the last survey flight'],
  [/^([+-]?[\d.]+) m Oberfläche$/, '$1 m surface'],
  [/^(\d+) Zellen · (.+)$/, '$1 cells · $2'],
  [/^(\d+) Baumkronen?( · max (\d+) m)?( ·)?$/, function(_,n,m,h,t){return n+(n==='1'?' tree crown':' tree crowns')+(m?' · max '+h+' m':'')+(t||'');}],
  [/^(\d+) absterbend$/, '$1 declining'],
  [/^(\d+) Bauwerke?( · bis (\d+) m)?( ·)?$/, function(_,n,m,h,t){return n+(n==='1'?' structure':' structures')+(m?' · up to '+h+' m':'')+(t||'');}],
  [/^Median (.+)$/, 'Median $1'],
  [/^\((\d+) Gst\.\)$/, '($1 parcels)'],
  [/^([\d,]+) Gst\. je Einlage$/, '$1 parcels per folio'],
  [/^Grenzkataster (\d+) %$/, 'Boundary cadastre $1 %'],
  [/^(\d+)% ([A-Za-zÄÖÜäöüß/ -]+)$/, function(_,p,w){return p+'% '+(I18N_EXACT[w]!==undefined?I18N_EXACT[w]:w);}],
  [/^Waldverlust ha\/Jahr$/, 'Forest loss ha/year'],
  [/^Förderung €\/Jahr$/, 'Subsidy €/year'],
  [/^Förderung schon abgeholt — nächste Auszahlung in (.+)$/, 'Subsidy already collected — next payout in $1'],
  [/^Nachricht muss 1-(\d+) Zeichen lang sein$/, 'Message must be 1-$1 characters long'],
  [/^Nicht genug Münzen! Brauchst (\d+), hast (\d+)$/, 'Not enough coins! Need $1, have $2'],
  [/^Nicht genug Münzen! Brauche (\d+), habe (\d+)$/, 'Not enough coins! Need $1, have $2'],
  [/^Käufer hat nur (\d+) Münzen, braucht (\d+)\. Käufer muss Parzellen verkaufen!$/, 'Buyer only has $1 coins, needs $2. Buyer must sell parcels!'],
  [/^(\d+) Min\.?$/, '$1 min'],
  [/^(\d+) Minuten$/, '$1 minutes'],
  [/^(\d+) Stunden$/, '$1 hours'],
  [/^(\d+) Tage$/, '$1 days'],
  [/^(\d+) ha$/, '$1 ha']
);


// ============================================================
// Runtime: auto-detect language; translate DOM for non-German
// users. German markup/code stays canonical.
// ============================================================
(function(){
  var forced = null;
  try { forced = new URLSearchParams(location.search).get('lang'); } catch(e) {}
  var de = forced ? /^de/i.test(forced)
    : /^de/i.test(navigator.language || (navigator.languages||[])[0] || 'de');
  window.LANG = de ? 'de' : 'en';
  // ---- audit helpers (both languages) ----
  // Heuristics for "looks German" / "looks English". Used by i18nSweep() and the
  // tr() miss log; shared with tools/i18n/sweep.js and tools/xbrowser (--lang=en).
  var DE_RX = /[äöüÄÖÜß]|\b(und|oder|der|die|das|mit|für|nicht|kein|keine|wird|werden|Gemeinde|Parzelle|Parzellen|Kataster|bearbeitet|Spiel|wählen|zurück|Münzen|Schätze|Lizenzen|laden|Karte|noch|schon|dein|deine|hier|jetzt|alle|wieder|Baum|Bäume|Gebäude|Fläche|Nutzung|Wert|Preis|kaufen|Kaufen|verkaufen|Verkaufen|Ernte|ernten|Brunnen|Wasser|Angebot|Spieler|Aufgabe|Zelle|Zellen|Gelände|frei|Besitzer|Daten|Fehler|Wald|Wiese|Acker|Feld|Bauland|bei|von|zum|zur|auf|aus|im|ein|eine|Jahre|Riesen|entfernt|geladen|gefunden|Suche|Ort|Lage|Höhe|Hang|Boden|Tag|Nacht|Keine|Kein|Noch|Deine|Dein|Anbau|Ried|Flur|Etage|Etagen|Stunden|Minuten|Tage|Jahr|seit|bis|nach|vor|über|unter|Wein|Obst|Holz|Stadt|Dorf|Gemeinden|Bauwerk|Bauwerke|Beobachtung|beobachtet|Abweichung|Veränderung|Sonstige|Sonstiges|Gewässer|Straße|Garten|Wiesen|Weiden|Äcker|Alm|Fels|Geb\.|Etage|Dicht|Mittel|Gering|gesucht|Jahr|Förderung|Waldverlust|aktiv|Bezirk|Katastralgemeinde|gekauft|Dach|Ausrichtung|Datenquelle|Keine|Minimal|Flachdach|Steildach)\b/;
  var EN_RX = /\b(the|and|with|for|your|you|parcel|parcels|forest|level|rugged|loading|load|buy|sell|owner|price|area|coins|quest|quests|treasure|water|field|fields|building|buildings|nearly|slightly|not|no|yet|found|search|searching|click|tap|again)\b/;
  var isDE = function(s){ return DE_RX.test(s); };
  var isEN = function(s){ return EN_RX.test(s); };
  // Strings that tr()/the DOM walker could not translate (only kept when they
  // look like the *wrong* language for the active LANG). Map text → {n, src}.
  var MISS = {};
  function noteMiss(s, src) {
    var t = String(s).trim(); if (!t) return;
    var m = MISS[t]; if (m) { m.n++; return; }
    MISS[t] = { n: 1, src: src };
  }
  // i18nSweep(): what a player sees right now in the wrong language — visible
  // DOM text nodes + placeholder/title/aria-label + tr() misses (canvas labels,
  // toasts…) since the last reset. Returns {lang, dom[], misses[], text}.
  // DEV.i18n() is the in-game alias; tools/xbrowser --lang=en runs it per scene.
  window.i18nSweep = function(opts) {
    opts = opts || {};
    var bad = de ? isEN : isDE; var misses_filter = function(){ return false; };
    // proper nouns are not leaks: municipality / KG / player names, toponyms
    var names = {};
    try { var g = window.G || {}; if (g.session) { names[g.session.municipality_name] = 1; names[g.session.name] = 1; } var kn = g.kgNames || {}; for (var k in kn) names[kn[k]] = 1; (g.players || []).forEach(function(pl){ names[pl.name] = 1; }); if (g.player) names[g.player.name] = 1; } catch (e) {}
    try { var si = window.SIDX; if (si && si.ready) { (si.g || []).forEach(function(r){ names[r.name] = 1; names[r.district] = 1; names[r.state] = 1; }); (si.k || []).forEach(function(r){ names[r.name] = 1; }); } } catch (e) {}
    try { (window.G && G.toponyms || []).forEach(function(t){ if (t && t.name) names[t.name] = 1; }); } catch (e) {}
    ['Niederösterreich','Oberösterreich','Steiermark','Kärnten','Salzburg','Tirol','Vorarlberg','Burgenland','Wien','Österreich','ÖSTERREICH','Rat auf Draht'].forEach(function(n){ names[n] = 1; });
    delete names[undefined]; delete names[''];
    // strip known names (longest first) before the language test — "Groundwater · Dürnstein" is English
    var nameList = Object.keys(names).sort(function(a, b){ return b.length - a.length; });
    // toponym shapes "Mautern an der Donau", "Kainach bei Voitsberg", "Sankt Ruprecht ob Murau" are names, not UI text
    var TOPO_RX = /\b[A-ZÄÖÜ][\wäöüß.-]+(?: [A-ZÄÖÜ][\wäöüß.-]+)? (?:an|in|im|am|bei|ob|unter|ober|vor|auf|zu) (?:der |dem |den |des )?[A-ZÄÖÜ][\wäöüß.-]+/g;
    var stripNames = function(v){ for (var i = 0; i < nameList.length; i++) { if (nameList[i].length > 2 && v.indexOf(nameList[i]) >= 0) v = v.split(nameList[i]).join('X'); } return v.replace(TOPO_RX, 'X'); };
    var badS = bad; bad = function(v){ return badS(v) && badS(stripNames(v)); };
    var out = {}; var w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT); var t;
    var vis = function(e){ for (var n = e; n; n = n.parentElement) { var cs = getComputedStyle(n); if (cs.display === 'none' || cs.visibility === 'hidden') return false; } return true; };
    var skipEl = function(e){ return e.closest && e.closest('#chat-log, script, style, .i18n-user'); };
    while ((t = w.nextNode())) { var p = t.parentElement; if (!p || skipEl(p)) continue; var v = t.nodeValue.trim(); if (v && bad(v) && vis(p)) out[v.slice(0, 160)] = (out[v.slice(0, 160)] || 0) + 1; }
    var els = document.querySelectorAll('[placeholder],[title],[aria-label]');
    for (var i = 0; i < els.length; i++) { var e = els[i]; if (skipEl(e)) continue; for (var a = 0; a < 3; a++) { var an = ['placeholder','title','aria-label'][a]; var av = e.getAttribute(an); if (av && bad(av) && vis(e)) out['@' + an + ': ' + av.slice(0, 160)] = 1; } }
    // toponym heuristic (en only): an umlaut-only hit whose words are all capitalised / numeric /
    // name particles (an, der, bei, ob, am, im, a.d.) is a place or person name, not UI text.
    var PART = /^(an|in|der|die|dem|des|bei|ob|am|im|a\.d\.|von|zu|und|·|–|—|-|km|%|m|m²|ha|KG|EZ|PLZ)$/;
    var looksName = function(v){ if (de) return false; var w = v.replace(/^[^\wÄÖÜäöüß]+/, '').split(/[\s,()·]+/).filter(Boolean); if (!w.length) return false; var stop = v.replace(/[äöüÄÖÜß]/g, 'x').replace(/\b(an|in|im|am|bei|ob|a\.d\.) (der|dem|den|des)?\b/g, ''); if (DE_RX.test(stop)) return false; return w.every(function(x){ return /^[A-ZÄÖÜ\d]/.test(x) || PART.test(x) || /^\d/.test(x); }); };
    var dom = Object.keys(out).filter(function(v){ return !names[v] && !names[v.replace(/ ▸$/, '')] && !looksName(v); });
    misses_filter = looksName;
    var misses = Object.keys(MISS).filter(function(k){ return !names[k] && !misses_filter(k) && bad(k); }).map(function(k){ return { text: k, n: MISS[k].n, src: MISS[k].src }; });
    if (opts.reset) MISS = {};
    var text = (dom.length ? 'DOM (' + dom.length + '):\n  ' + dom.join('\n  ') : 'DOM: clean') + '\n' +
      (misses.length ? 'tr() misses (' + misses.length + '):\n  ' + misses.map(function(m){ return m.text + '  ×' + m.n + (m.src ? '  [' + m.src + ']' : ''); }).join('\n  ') : 'tr(): clean');
    return { lang: window.LANG, dom: dom, misses: misses, text: text };
  };
  window.i18nMisses = function(){ return MISS; };

  if (de) {
    // German is canonical: tr() is identity, but English literals leaking in
    // (upstream labels, mapping-table values) are still logged for DEV.i18n().
    window.tr  = function(s){ if (typeof s === 'string' && isEN(s)) noteMiss(s, 'tr'); return s; };
    window.trx = window.tr;
    return;
  }
  document.documentElement.lang = 'en';

  function trx(s, src) {
    if (typeof s !== 'string' || !s) return s;
    var hit = I18N_EXACT[s];
    if (hit !== undefined) return hit;
    var t = s.trim();
    if (t !== s) {
      hit = I18N_EXACT[t];
      if (hit !== undefined) return s.replace(t, hit);
    }
    for (var i = 0; i < I18N_RX.length; i++) {
      if (I18N_RX[i][0].test(t)) { var r = t.replace(I18N_RX[i][0], I18N_RX[i][1]); if (r !== t) return s.replace(t, r); }
    }
    if (isDE(t)) noteMiss(t, src || 'tr');
    return s;
  }
  window.tr = trx;
  window.trx = trx;

  var ATTRS = ['placeholder', 'title', 'aria-label'];
  function skip(el) {
    for (var n = el; n; n = n.parentElement) {
      if (n.id === 'chat-log' || n.tagName === 'SCRIPT' || n.tagName === 'STYLE') return true;
    }
    return false;
  }
  function translateAttrs(el) {
    for (var a = 0; a < ATTRS.length; a++) {
      if (el.hasAttribute && el.hasAttribute(ATTRS[a])) {
        var av = el.getAttribute(ATTRS[a]);
        var tv = trx(av, '@' + ATTRS[a]);
        if (tv !== av) el.setAttribute(ATTRS[a], tv);
      }
    }
  }
  function translateNode(root) {
    if (root.nodeType === 3) { // text node
      if (root.parentElement && skip(root.parentElement)) return;
      var v = trx(root.nodeValue, 'dom');
      if (v !== root.nodeValue) root.nodeValue = v;
      return;
    }
    if (root.nodeType !== 1 || skip(root)) return;
    translateAttrs(root);
    if (root.querySelectorAll) {
      var els = root.querySelectorAll('[placeholder],[title],[aria-label]');
      for (var e = 0; e < els.length; e++) if (!skip(els[e])) translateAttrs(els[e]);
    }
    var w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    var t;
    while ((t = w.nextNode())) {
      if (t.parentElement && skip(t.parentElement)) continue;
      var nv = trx(t.nodeValue, 'dom');
      if (nv !== t.nodeValue) t.nodeValue = nv;
    }
  }

  function relink() {
    // Point legal links at the English pages.
    document.querySelectorAll('a[href="/impressum"]').forEach(function(a){ a.href = '/imprint'; });
    document.querySelectorAll('a[href="/datenschutz"]').forEach(function(a){ a.href = '/privacy'; });
  }

  function boot() {
    document.title = 'Siedler Österreich – Explore and protect Austria’s nature';
    translateNode(document.body);
    document.querySelectorAll('[placeholder],[title],[aria-label]').forEach(translateNode);
    relink();
    var mo = new MutationObserver(function(muts){
      for (var i = 0; i < muts.length; i++) {
        var m = muts[i];
        if (m.type === 'characterData') { translateNode(m.target); continue; }
        if (m.type === 'attributes') { translateNode(m.target); continue; }
        for (var j = 0; j < m.addedNodes.length; j++) translateNode(m.addedNodes[j]);
      }
    });
    mo.observe(document.body, {
      childList: true, subtree: true, characterData: true,
      attributes: true, attributeFilter: ATTRS
    });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
