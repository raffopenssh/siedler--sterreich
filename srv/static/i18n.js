// ============================================================
// Siedler Österreich — runtime i18n dictionary (DE → EN)
// I18N_EXACT: exact-match lookup (DOM text walker + tr())
// I18N_RX:    parameterized runtime strings (toasts etc.)
// ============================================================

const I18N_EXACT = {
  // ---------- index.html: welcome screen ----------
  'Dein Pseudonym...': 'Your nickname...',
  'Neuer Vorschlag': 'New suggestion',
  'Neues Spiel': 'New Game',
  '🍀 Auf Glück': '🍀 Feeling lucky',
  '⚔️ Mitspielen': '⚔️ Join game',
  'Beta · keine Cookies · kein Tracking': 'Beta · no cookies · no tracking',
  '⚒ BETA · PROTOTYP': '⚒ BETA · PROTOTYPE',
  'In Entwicklung – Spielstände, Preise und Regeln können sich noch ändern.': 'In development – saves, prices and rules may still change.',
  'Impressum': 'Imprint',
  'Daten: ': 'Data: ',
  ' Kataster & ALS, ': ' cadastre & ALS, ',
  ' (bearbeitet) · ': ' (modified) · ',
  '© OpenStreetMap-Mitwirkende': '© OpenStreetMap contributors',
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
  '🌲 Gerücht: Irgendwo hier steht ein Riesenbaum... Finde und tippe ihn an!': '🌲 Rumor: a giant tree stands somewhere around here... Find it and tap it!',
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
  'Wandle 3 Parzellen in Wald oder Naturschutz um': 'Convert 3 parcels to forest or nature reserve',

};

// ---------- chat safety ----------
Object.assign(I18N_EXACT, {
  'Chat: frei': 'Chat: free text', 'Chat: Schnellnachrichten': 'Chat: quick phrases', 'Chat: aus': 'Chat: off',
  'Chat-Modus (nur Spielersteller)': 'Chat mode (game creator only)', 'Schnellnachrichten': 'Quick phrases',
  'Chat-Regeln & Sicherheit': 'Chat rules & safety', 'Der Chat ist in diesem Spiel deaktiviert.': 'Chat is disabled in this game.',
  '🛡️ Chat-Regeln': '🛡️ Chat rules', 'Verstanden ✓': 'Got it ✓', 'Später': 'Later', '⚑ Melden': '⚑ Report', 'Melden': 'Report', 'Abbrechen': 'Cancel',
  'Optional: Was ist passiert?': 'Optional: what happened?', 'Chat-Modus geändert': 'Chat mode changed', 'blockiert': 'blocked', 'Spieler: ': 'Player: ',
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

const I18N_RX = [
  // ---------- registration / joining ----------
  [/^🎉 Willkommen, (.+)!$/, '🎉 Welcome, $1!'],
  [/^Fehler beim Beitreten: (.+)$/, 'Error joining: $1'],
  [/^Fehler beim Zufallsstart: (.+)$/, 'Random start failed: $1'],
  [/^Lade (.+)\.\.\.$/, 'Loading $1...'],
  [/^(.*) Dein Chat ist für (.+) gesperrt\.$/, function(_,a,b){return trx(a)+' Your chat is muted for '+b.replace('Min.','min').replace('Std.','h').replace('Tagen','days')+'.';}],
  [/^Dein Chat ist noch (.+) gesperrt\.$/, function(_,b){return 'Your chat is still muted for '+b.replace('Min.','min').replace('Std.','h').replace('Tagen','days')+'.';}],
  [/^Dein Chat wurde dauerhaft gesperrt\.$/, 'Your chat has been permanently disabled.'],

  // ---------- loading progress ----------
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
  [/^🦎 🛡️ Natura-2000-Bonus! Artenfund: ([\s\S]+)$/, '🦎 🛡️ Natura 2000 bonus! Species found: $1'],
  [/^🦎 Artenfund: ([\s\S]+)$/, '🦎 Species found: $1'],
  [/^💎 Schatz! \+(\d+)(.+)$/, '💎 Treasure! +$1$2'],

  // ---------- giant trees ----------
  [/^🌲 Riesenbaum entdeckt! (\d+) Riesenbäume sind nun sichtbar — Grundstücke mit Riesenbäumen bringen Bonus-XP!$/, '🌲 Giant tree discovered! $1 giant trees are now visible — parcels with giant trees earn bonus XP!'],
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
  [/^([\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}\u2696\u26F0]+ )(Erkunde deine Gemeinde|Naturschützer|Landvermesser|Schatzsucher|Waldmeister)$/u, function(_,e,t){return e+({'Erkunde deine Gemeinde':'Explore your municipality','Naturschützer':'Conservationist','Landvermesser':'Land surveyor','Schatzsucher':'Treasure hunter','Waldmeister':'Forest master'})[t];}],
  // Landuse summary lists like "Sonstige (×5), Straße, Wald" — translate each term.
  [/^([A-Za-zÄÖÜäöüß()×X\d ]+)(, [A-Za-zÄÖÜäöüß()×X\d ]+)+$/, function(m0){
    return m0.split(', ').map(function(part){
      var mm = part.match(/^(.+?)( \([×X]?\d+\))?$/);
      var base = mm[1], suff = mm[2]||'';
      return (I18N_EXACT[base]!==undefined?I18N_EXACT[base]:base)+suff;
    }).join(', ');
  }],
  [/^([A-Za-zÄÖÜäöüß ]+?) (\([×X]?\d+\))$/, function(_,base,suff){return (I18N_EXACT[base]!==undefined?I18N_EXACT[base]:base)+' '+suff;}],
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


// ============================================================
// Runtime: auto-detect language; translate DOM for non-German
// users. German markup/code stays canonical.
// ============================================================
(function(){
  var de = /^de/i.test(navigator.language || (navigator.languages||[])[0] || 'de');
  window.LANG = de ? 'de' : 'en';
  if (de) {
    window.tr  = function(s){ return s; };
    window.trx = function(s){ return s; };
    return;
  }
  document.documentElement.lang = 'en';

  function trx(s) {
    if (typeof s !== 'string' || !s) return s;
    var hit = I18N_EXACT[s];
    if (hit !== undefined) return hit;
    var t = s.trim();
    if (t !== s) {
      hit = I18N_EXACT[t];
      if (hit !== undefined) return s.replace(t, hit);
    }
    for (var i = 0; i < I18N_RX.length; i++) {
      if (I18N_RX[i][0].test(t)) return s.replace(t, t.replace(I18N_RX[i][0], I18N_RX[i][1]));
    }
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
  function translateNode(root) {
    if (root.nodeType === 3) { // text node
      if (root.parentElement && skip(root.parentElement)) return;
      var v = trx(root.nodeValue);
      if (v !== root.nodeValue) root.nodeValue = v;
      return;
    }
    if (root.nodeType !== 1 || skip(root)) return;
    for (var a = 0; a < ATTRS.length; a++) {
      if (root.hasAttribute && root.hasAttribute(ATTRS[a])) {
        var av = root.getAttribute(ATTRS[a]);
        var tv = trx(av);
        if (tv !== av) root.setAttribute(ATTRS[a], tv);
      }
    }
    var w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    var t;
    while ((t = w.nextNode())) {
      if (t.parentElement && skip(t.parentElement)) continue;
      var nv = trx(t.nodeValue);
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
