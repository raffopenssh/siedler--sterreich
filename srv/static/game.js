// ============================================================
//  SIEDLER ÖSTERREICH — Game Engine
//  Isometric Settlers IV style
// ============================================================
'use strict';

// Fallback if i18n.js failed to load: identity translator.
if (typeof window.tr !== 'function') window.tr = function(s){ return s; };

const CAD = '/api/cadastre';

// ---- Colors inspired by Settlers IV ----
const TERRAIN = {
  grass:    ['#4a8a30','#528e35','#5a9238','#4e8c32','#468828'],
  forest:   ['#1e5a1e','#245e22','#2a6228','#1c581c','#286026'],
  // One flat water colour: rivers/lakes are drawn from three sources (landuse
  // polygons, OSM water_area, water-dominant parcels) that must be seamless.
  water:    ['#2f7fbf','#2f7fbf','#2f7fbf','#2f7fbf','#2f7fbf'],
  farm:     ['#a8a040','#b0a848','#a09838','#b8b050','#989030'],
  meadow:   ['#5a9e3a','#62a240','#52963a','#6aaa48','#4e9234'],
  building: ['#c8b040','#d0b848','#bca838','#d8c050','#b4a030'],
  road:     ['#484848','#505050','#444444','#525252','#404040'],
  garden:   ['#6b8e4a','#739650','#638644','#7b9e58','#5b7e3e'],
  wetland:  ['#3a7a5a','#428260','#327254','#4a8a68','#2a6a4e'],
  waste:    ['#5a5848','#625e50','#525040','#6a6658','#4a4838'],
  glacier:  ['#cfe4f2','#dcecf8','#c2dced','#e6f2fa','#b8d4e8'],
  bio:      ['#2aaa4a','#32b252','#22a242','#3aba5a','#1a9a3a'],
};

// ---- BEV Nutzungssymbole (NS) — single source of truth ----
// Source: BEV Schnittstellenbeschreibung "Katastralmappe SHP" V2.9, Tabelle 8.
// The upstream API corrected its German labels in Aug 2026; the CODES never
// changed. Two corrections matter a lot for us:
//   48 = "Äcker, Wiesen oder Weiden" (farmland — Austria's most common code,
//        3.76M parcels) — we used to render and PRICE it as Verkehrsfläche.
//   83 = "Gebäudenebenflächen" (a Baufläche) — we used to treat it as Fels/Sumpf.
// Codes outside this table are not defined by BEV and do not occur in the data.
// `price` = base coins/m² used by calcPrice() — MUST stay in sync with
// nsPricePerSqm() in srv/server.go.
const NS_TABLE = {
  '40': {abbr:'LN(Dk)',  name:'Dauerkulturen',      terrain:TERRAIN.garden,   price:0.30},
  '41': {abbr:'B(Geb)',  name:'Gebäude',            terrain:TERRAIN.building, price:0.50},
  '42': {abbr:'So(Pp)',  name:'Parkplatz',          terrain:TERRAIN.road,     price:0.25},
  '48': {abbr:'LN',      name:'Äcker/Wiesen/Weiden',terrain:TERRAIN.farm,     price:0.30},
  '52': {abbr:'GA',      name:'Garten',             terrain:TERRAIN.garden,   price:0.45},
  '53': {abbr:'WG',      name:'Weingarten',         terrain:TERRAIN.garden,   price:0.35},
  '54': {abbr:'Alpe',    name:'Alm',                terrain:TERRAIN.meadow,   price:0.12},
  '55': {abbr:'W(Kr)',   name:'Krummholz',          terrain:TERRAIN.forest,   price:0.10},
  '56': {abbr:'W',       name:'Wald',               terrain:TERRAIN.forest,   price:0.20},
  '57': {abbr:'LN(vb)',  name:'Verbuschte Fläche',  terrain:TERRAIN.meadow,   price:0.15},
  '58': {abbr:'W(Fs)',   name:'Forststraße',        terrain:TERRAIN.road,     price:0.10},
  '59': {abbr:'GW(f)',   name:'Fließgewässer',      terrain:TERRAIN.water,    price:0.05},
  '60': {abbr:'GW(s)',   name:'Stehendes Gewässer', terrain:TERRAIN.water,    price:0.05},
  '61': {abbr:'GW(Fg)',  name:'Feuchtgebiet',       terrain:TERRAIN.wetland,  price:0.08},
  '62': {abbr:'So(vg)',  name:'Vegetationsarm',     terrain:TERRAIN.waste,    price:0.05},
  '63': {abbr:'So(Bf)',  name:'Betriebsfläche',     terrain:TERRAIN.waste,    price:0.40},
  '64': {abbr:'GW(Rf)',  name:'Gewässerrand',       terrain:TERRAIN.wetland,  price:0.08},
  '65': {abbr:'So(Vr)',  name:'Verkehrsrand',       terrain:TERRAIN.grass,    price:0.10},
  '72': {abbr:'So(Fh)',  name:'Friedhof',           terrain:TERRAIN.garden,   price:0.20},
  '83': {abbr:'B(Nf)',   name:'Gebäudenebenfläche', terrain:TERRAIN.building, price:0.45},
  '84': {abbr:'So(Ab)',  name:'Abbau/Halde/Deponie',terrain:TERRAIN.waste,    price:0.15},
  '87': {abbr:'So(Fe)',  name:'Fels/Geröll',        terrain:TERRAIN.waste,    price:0.03},
  '88': {abbr:'So(Gl)',  name:'Gletscher',          terrain:TERRAIN.glacier,  price:0.03},
  '92': {abbr:'So(Bahn)',name:'Bahnanlage',         terrain:TERRAIN.road,     price:0.15},
  '95': {abbr:'So(Str)', name:'Straße',             terrain:TERRAIN.road,     price:0.10},
  '96': {abbr:'So(Fz)',  name:'Freizeitfläche',     terrain:TERRAIN.meadow,   price:0.35},
};

// Derived lookups (kept as separate consts — used all over the renderer).
const LANDUSE_TERRAIN = {};   // code → terrain palette
const LANDUSE_NAMES = {};     // code → short German name
const ABBR_MAP = {};          // landuse_summary abbr → {terrain, code, name}
for (const [code, e] of Object.entries(NS_TABLE)) {
  LANDUSE_TERRAIN[code] = e.terrain;
  LANDUSE_NAMES[code] = e.name;
  ABBR_MAP[e.abbr] = {terrain:e.terrain, code, name:e.name};
}
// Player-converted nature reserves are not a BEV code — synthetic entry.
ABBR_MAP['Bio'] = {terrain:TERRAIN.bio, code:'', name:'Naturschutz'};

// Codes whose surface is sealed/paved (roads, rail, parking, forest roads).
const NS_TRAFFIC = new Set(['42','58','92','95']);
// Codes that count as "a building stands here".
const NS_BUILDING = new Set(['41']);
// NS entries are SYMBOL counts, not areas — upstream's own land_prices fix (Aug
// 2026) showed why that matters: a 17.9 ha field carrying three stray
// building/road glyphs was classified as built-up Bauland. Road/rail/building
// symbols are typically thin slivers, so down-weight them when picking a
// parcel's dominant use for terrain colour and pricing.
function nsWeight(code) {
  if (NS_TRAFFIC.has(code)) return 0.25;
  if (NS_BUILDING.has(code) || code === '83') return 0.5;
  return 1;
}

// LiDAR dominant land cover → terrain palette (enhanced mode, real measured cover)
const DOM_TERRAIN = {
  grass:TERRAIN.meadow, tree:TERRAIN.forest, hedge:TERRAIN.garden, shrub:TERRAIN.garden,
  roof:TERRAIN.building, crop:TERRAIN.farm, water:TERRAIN.water, vineyard:TERRAIN.garden,
  garden:TERRAIN.garden, road:TERRAIN.road, parking:TERRAIN.road, path:TERRAIN.road,
  bare_soil:TERRAIN.waste, rock:TERRAIN.waste, fill:TERRAIN.waste, excavation:TERRAIN.waste,
  construction:TERRAIN.waste, tree_loss:TERRAIN.waste,
};
// Impervious classes srtm often mis-reports as a parcel's dominant cover. We never
// use these as the ground-fill color (buildings/roofs render as footprints on top,
// roads come from OSM lines). The server already skips them and returns dom_terrain;
// this set is the client-side fallback when dom_terrain is absent.
const IMPERVIOUS_DOM = new Set(['road','roof','parking','path']);

/** Parse landuse_summary → {dominant:{terrain,code,name}, buildingCount, entries:[{abbr,terrain,count}]} */
function parseLanduseSummary(summary) {
  if (!summary || typeof summary !== 'object') return {dominant:null, buildingCount:0, entries:[]};
  const entries = [];
  let buildingCount = 0;
  for (const [key, count] of Object.entries(summary)) {
    // Extract abbreviation after " - "
    const dashIdx = key.lastIndexOf(' - ');
    const abbr = dashIdx >= 0 ? key.slice(dashIdx + 3) : key;
    let info = ABBR_MAP[abbr];
    if (!info) {
      // Unknown abbr — derive from the German description text. Upstream now
      // emits "Unbekannt - Code NN" for anything outside the BEV table.
      const t = key.toLowerCase();
      if (t.includes('wald') || t.includes('wälder') || t.includes('forst')) info = ABBR_MAP['W'];
      else if (t.includes('acker') || t.includes('äcker') || t.includes('wiese') || t.includes('weide')) info = ABBR_MAP['LN'];
      else if (t.includes('gebäudeneben')) info = ABBR_MAP['B(Nf)'];
      else if (t.includes('gebäude')) info = ABBR_MAP['B(Geb)'];
      else if (t.includes('weingarten') || t.includes('weingärten')) info = ABBR_MAP['WG'];
      else if (t.includes('garten') || t.includes('gärten')) info = ABBR_MAP['GA'];
      else if (t.includes('straß') || t.includes('verkehr') || t.includes('bahn')) info = ABBR_MAP['So(Str)'];
      else if (t.includes('gewässer')) info = ABBR_MAP['GW(f)'];
      else if (t.includes('feucht') || t.includes('sumpf') || t.includes('moor')) info = ABBR_MAP['GW(Fg)'];
      else if (t.includes('alpe') || t.includes('alm')) info = ABBR_MAP['Alpe'];
      else if (t.includes('fels') || t.includes('geröll')) info = ABBR_MAP['So(Fe)'];
      else if (t.includes('gletscher')) info = ABBR_MAP['So(Gl)'];
      else info = {terrain:TERRAIN.grass, code:'', name:abbr};
    }
    entries.push({abbr, terrain:info.terrain, code:info.code, name:info.name, count});
    if (NS_BUILDING.has(info.code)) buildingCount += count;
  }
  // Dominant = highest area-weighted count (see nsWeight)
  let dominant = null, bestW = -1;
  for (const e of entries) {
    const w = e.count * nsWeight(e.code);
    if (w > bestW) { bestW = w; dominant = e; }
  }
  return {dominant, buildingCount, entries};
}

const PLAYER_COLORS = ['#e04040','#4080e0','#e0c040','#a040e0','#40e0a0','#e08040','#e040a0','#40e040'];

// ---- Game State ----
// Canvas type scale — every in-map label uses one of these three so sizes are
// harmonised across treasures, giant trees, GPS, badges (CSS sidebar: 14/16px VT323).
const MAP_FONT = {
  label: '14px VT323, monospace',            // names, distances
  small: '11px VT323, monospace',            // sub-lines (category, units)
  pixel: '9px "Press Start 2P", monospace',  // badges / rewards
};
const G = {
  player: null, session: null,
  playerToken: null,    // rejoin token, sent as X-Player-Token on API calls
  parcels: [],          // from cadastre (point data)
  parcelPolys: [],      // from export/geojson (polygon data for current KGs)
  buildingFootprints: [], // real building footprint polygons from cadastre
  landusePolys: [],     // real landuse polygons (forests, roads, water, etc.)
  luTiles: new Set(),   // viewport-landuse tiles fetched (CAD-1)
  luIds: new Set(),     // landuse polygon dedup keys
  schlaege: [],         // INVEKOS field polygons (FARM-2): {properties:{id,snar_name,crop_group,area_ha,organic}, geometry}
  schlagIds: new Set(),
  schlagTiles: new Set(),
  schlagGen: 0,         // bumps when fields arrive → parcel→field cache revalidates
  schlagYear: 0,
  cropByParcel: {},     // parcel_id → {gen, f|null}
  ezIndex: {},          // kg_code+ez → [parcel features] for quick grouping
  ezHighlight: null,    // {kg, ez} of currently highlighted EZ group
  claimed: [],          // from our DB
  offers: [],           // pending parcel offers
  treasures: [], challenges: [], players: [], chatMsgs: [],
  sse: null,
  // Map view
  cam: { lon: 15.44, lat: 47.07, zoom: 17 },
  drag: { active:false, sx:0, sy:0, slon:0, slat:0 },
  sel: null, // selected parcel feature
  pcolors: {}, pci: 0,
  // Municipality picker state
  pick: { level:'states', state:null, munis:[], cam:{lon:13.3,lat:47.5,zoom:7}, drag:{active:false} },
  selectedMuni: null,
  kgsLoaded: new Set(),
  polyIds: new Set(),        // parcel_ids already in parcelPolys (viewport dedup)
  fpIds: new Set(),          // footprint_ids already in buildingFootprints (viewport dedup)
  vpTiles: new Set(),        // quantized viewport tiles already fetched
  // ---- Enhanced mode (srtm-lidar + OSM + Natura-2000 + land prices) ----
  enhancedKGs: new Set(),   // kg_codes with lidar data available
  enhancedGemeinden: [],    // [{gemeinde_code, gemeinde_name, lon, lat, v2}] deduped
  v2KGs: new Set(),         // subset of enhancedKGs on srtm product 2.1 (richer trees / grids)
  enhancedLoaded: new Set(),// kg_codes whose enhanced data has been fetched
  lidarParcels: {},         // parcel_id → {elev, elevMin, elevMax, slope, aspect, tclass, dom, forestFrac}
  lidarKGTerrain: {},       // kg_code → {emin, emax, tclass}
  lidarBuildingIdx: {},     // grid key → [{lon,lat,stories,roof,h}] for footprint matching
  topTrees: {},             // kg_code → [{height_m, lon, lat}] (flag-filtered server-side)
  topObjects: {},           // kg_code → [{type, height_m, lon, lat}]
  osmLines: {},             // kg_code → [{cat, fclass, major, name, pts:Float64Array}]
  waterParcels: {},         // parcel_id → {fraction, sqm, fclass[]} from osm/geometry?cat=water_parcels
  waterAreas: {},           // kg_code → [{geometry, fclass}] fillable OSM river/lake polygons
  waterKGs: new Set(),      // KGs whose water layers were requested
  n2kSites: {},             // sitecode → {name, habitats, label, geom (GeoJSON), loaded}
  n2kVisible: true,         // layer toggle
  landPrices: {},           // parcel_id → price estimate object (lazy)
  forestValues: {},         // parcel_id → /api/forest-value response (lazy, timber.go)
  osmProx: {},              // parcel_id → OSM proximity object (lazy, null = failed/loading)
  bldgInfo: {},             // footprint_id → building info (lazy, null = loading/failed)
  kgSummaries: {},          // kg_code → summary object (lazy)
  selFp: null,              // tapped building footprint feature (renders section in parcel popup)
  similar: null,            // {refPid, refLon, refLat, data} — active similar-parcels overlay
  similarCache: {},         // "pid:radius" → /api/similar response (client cache)
  similarRadius: 5000,      // selected search radius in m (5/10/20/50 km)
  geo: { watching:false, lon:0, lat:0, acc:0, follow:false, id:null },
  tallUnlocked: false,      // giant trees unlock after first treasure collected
  tallRevealed: false,      // tapping the hint tree starts discovery mode (persisted via tallSeen)
  tallRevealAt: 0,          // timestamp for pop-in animation
  tallSeen: new Set(),      // "Riesen-Chronik": keys of giant trees discovered so far (localStorage per session)
  devTree: null,            // giant tree unlocked via 5-tap "developer mode" on the enhanced badge
  lidarGen: 0,              // bumped when new lidar building data arrives (invalidates footprint matches)
};

// ---- Helpers ----
async function api(method, url, body, apiOpts) {
  const opts = { method, headers: {'Content-Type':'application/json'} };
  // Authenticate as the current player: the server verifies this token
  // against player_id on every mutating endpoint.
  const tok = G.playerToken || getUrlParam('rejoin');
  if (tok) opts.headers['X-Player-Token'] = tok;
  if (body) opts.body = JSON.stringify(body);
  // 202 Accepted = the upstream data product (cadastre KG file / lidar GPKG)
  // is still being pulled from the Zenodo mirror. Our server relays
  // {status:"pending", retry_after_s, progress:{pct,eta_s}, kgs:[...]} plus a
  // Retry-After header, and repeating the identical request converges. Wait
  // as told (bounded) and re-GET; only GETs are idempotent enough for this.
  const budget = (apiOpts && apiOpts.pendingBudgetMs != null) ? apiOpts.pendingBudgetMs : (method === 'GET' ? 45000 : 0);
  const t0 = Date.now();
  for (let attempt = 0; ; attempt++) {
    const r = await fetch(url, opts);
    if (r.status !== 202) return r.json();
    let d = {};
    try { d = await r.json(); } catch (e) {}
    const ra = Math.min(Math.max(+(d.retry_after_s || r.headers.get('Retry-After') || 3), 1), 12) * 1000;
    const elapsed = Date.now() - t0;
    if (elapsed + ra > budget || attempt >= 8) {
      // Out of patience: hand the pending body back, flagged, so callers can
      // schedule their own retry later (fetchKGLayer, loadEnhancedForKGs…).
      d.pending = true; d.status = d.status || 'pending';
      return d;
    }
    pendingNotice(d, url);
    await new Promise(res => setTimeout(res, ra));
  }
}

/** Upstream Zenodo warming state seen by api() — drives the map-loading text. */
let _pendingSeen = 0;
function pendingNotice(d, url) {
  _pendingSeen = Date.now();
  G.pendingUpstream = { at: _pendingSeen, pct: d.progress?.pct, eta: d.progress?.eta_s, zenodo: d.zenodo, kgs: d.kgs || [] };
  updateMapLoadingText();
}
function updateMapLoadingText() {
  const el = document.getElementById('map-loading');
  if (!el) return;
  const p = G.pendingUpstream;
  const fresh = p && Date.now() - p.at < 20000;
  if (!fresh) { el.textContent = tr('⏳ Lade Gelände…'); return; }
  let s = tr('⏳ Kataster wird vom Datenarchiv geholt…');
  if (p.pct > 0) s += ' ' + Math.round(p.pct) + ' %';
  if (p.eta > 0 && p.eta < 600) s += ' · ~' + Math.round(p.eta) + ' s';
  if (p.zenodo && p.zenodo !== 'healthy') s += ' · ' + tr('Archiv langsam');
  el.textContent = s;
}
const GET = url => api('GET', url);
const POST = (url, body) => api('POST', url, body);

function toast(msg, type) {
  const el = document.createElement('div');
  el.className = 'toast' + (type === 'ok' ? ' ok' : type === 'err' ? ' err' : '');
  el.textContent = msg;
  document.getElementById('toast-container').appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

function show(id) {
  document.querySelectorAll('.screen').forEach(s => s.classList.remove('active'));
  document.getElementById('screen-'+id).classList.add('active');
}

function esc(s) { const d=document.createElement('div'); d.textContent=s; return d.innerHTML; }

/** Build invite URL with current camera view encoded as hash params */
function inviteUrl(code) {
  let url = location.origin + '/join/' + code;
  if (G.cam) {
    url += '#v=' + G.cam.lon.toFixed(5) + ',' + G.cam.lat.toFixed(5) + ',' + (Math.round(G.cam.zoom*10)/10);
  }
  return url;
}

/** Parse view hash params from URL (e.g. #v=15.07200,47.06400,18) */
function parseViewHash() {
  const h = location.hash;
  const m = h.match(/v=([\d.]+),([\d.]+),([\d.]+)/);
  if (m) return { lon: parseFloat(m[1]), lat: parseFloat(m[2]), zoom: parseFloat(m[3]) };
  return null;
}

// ================= NAME GENERATOR =================
const _ADJ = [
  'Tapfer','Kühn','Edel','Stolz','Wild','Flink','Mutig','Weise',
  'Stark','Listig','Grimmig','Eisern','Treu','Finster','Feurig',
  'Schnell','Leise','Dunkel','Golden','Silbern','Steinig','Kalt',
  'Schattig','Stürmisch','Sanft','Alt','Jung','Groß','Klein','Mächtig',
];
const _NOUN = [
  'Ritter','Jäger','Bauer','Schmied','Falke','Wolf','Bär','Adler',
  'Fuchs','Hirsch','Löwe','Drache','Rabe','Stein','Berg','Bach',
  'Wald','Turm','Schild','Schwert','Eiche','Linde','Fels','Blitz',
  'Donner','Schatten','Flamme','Frost','Stern','Mond',
];
/** Local fallback only. Prefer suggestFreeName() — the server knows which names
 *  are already taken (there are only ~900 adj+noun combos, so blind picks
 *  collide constantly once a few hundred players exist). */
function randomName() {
  const a = _ADJ[Math.floor(Math.random()*_ADJ.length)];
  const n = _NOUN[Math.floor(Math.random()*_NOUN.length)];
  return a + n + Math.floor(2 + Math.random()*9000);
}

/** Ask the server for a guaranteed-unused name. Falls back to a locally
 *  generated numbered name if the request fails. */
async function suggestFreeName() {
  try {
    const r = await GET('/api/suggest-name');
    if (r && r.name) return r.name;
  } catch(e) { console.error('suggest-name failed', e); }
  return randomName();
}

// ================= URL STATE (no cookies, no localStorage) =================
// State persisted in URL: ?pid=xxx&pname=yyy&rejoin=token&invite=code
function getUrlParam(key) {
  return new URLSearchParams(location.search).get(key);
}
function setUrlParams(obj) {
  const sp = new URLSearchParams(location.search);
  for (const [k,v] of Object.entries(obj)) {
    if (v == null) sp.delete(k); else sp.set(k, v);
  }
  const qs = sp.toString();
  history.replaceState(null, '', location.pathname + (qs ? '?'+qs : '') + location.hash);
}

// ================= WELCOME =================
(async () => {
  const inp = document.getElementById('input-name');
  const err = document.getElementById('welcome-error');

  // Check invite in URL path or param
  const invitePathMatch = location.pathname.match(/\/join\/([^/?]+)/);
  const inviteCode = invitePathMatch?.[1] || getUrlParam('invite');
  if (inviteCode) setUrlParams({invite: inviteCode});

  // If invite code present, fetch session preview and show streamlined join UI
  let invitePreview = null;
  if (inviteCode) {
    try {
      invitePreview = await GET('/api/invite/' + encodeURIComponent(inviteCode));
      if (!invitePreview.error && invitePreview.session) {
        const banner = document.getElementById('invite-banner');
        const creatorName = invitePreview.creator_name || '???';
        const muniName = invitePreview.session.municipality_name || '';
        banner.innerHTML = (window.LANG === 'en'
          ? `⚔️ Joining <b style="color:var(--gold)">${esc(creatorName)}'s</b> game`
          : `⚔️ In <b style="color:var(--gold)">${esc(creatorName)}s</b> Spiel`) +
          (muniName ? `<div class="invite-muni">📍 ${esc(muniName)}</div>` : '');
        banner.style.display = 'block';
        document.getElementById('welcome-buttons-normal').style.display = 'none';
        document.getElementById('welcome-buttons-invite').style.display = '';
      }
    } catch(e) { console.error('invite preview failed', e); }
  }

  // Retrieve saved player from URL
  const savedPid = getUrlParam('pid');
  const savedName = getUrlParam('pname');

  // Pre-fill name: from URL, else a server-checked free suggestion
  inp.value = savedName || '';
  if (!savedName) suggestFreeName().then(n => { if (!inp.value) inp.value = n; });
  document.getElementById('btn-reroll').onclick = async () => {
    const btn = document.getElementById('btn-reroll');
    btn.disabled = true;
    inp.value = await suggestFreeName();
    btn.disabled = false;
    inp.focus();
  };

  if (savedPid && savedName) {
    document.getElementById('quick-rejoin').innerHTML =
      (window.LANG === 'en'
        ? `Last played as <b style="color:var(--gold)">${esc(savedName)}</b> — <a onclick="quickLogin()">Continue ▸</a>`
        : `Zuletzt als <b style="color:var(--gold)">${esc(savedName)}</b> gespielt — <a onclick="quickLogin()">Weiter ▸</a>`);
  }

  // Auto-rejoin: if sid is in URL, go directly to game
  const autoSid = getUrlParam('sid');
  if (savedPid && autoSid) {
    (async () => {
      try {
        const p = await GET('/api/player/'+savedPid);
        if (p.error) return;
        G.player = p;
        const sess = await GET('/api/session/'+autoSid);
        if (sess.error) return;
        G.session = sess;
        show('loading');
        document.getElementById('loading-muni').textContent = '📍 ' + (G.session.municipality_name||'');
        startTipRotation();
        startLoadingCountdown(20);
        await startGameWithLoading();
      } catch(e) { console.error('auto-rejoin failed', e); }
    })();
  }

  async function registerAndProceed(goLucky) {
    let name = inp.value.trim();
    if (!name || name.length < 2) { name = await suggestFreeName(); inp.value = name; }
    let res = await POST('/api/register', {name});
    // Name taken (someone grabbed it between suggestion and submit, or the user
    // typed an existing one): auto-retry once with the server's free suggestion.
    if (res.error && res.suggested) {
      inp.value = res.suggested;
      res = await POST('/api/register', {name: res.suggested});
      if (!res.error) toast('Name war vergeben — du spielst als ' + res.player.name, 'ok');
    }
    if (res.error) { err.textContent=res.error; return null; }
    savePlayer(res.player);
    G.freshPlayer = true; // first ever load → in-game herald intro
    G.playerToken = res.rejoin_token || null;
    setUrlParams({rejoin: res.rejoin_token || null});
    toast('🎉 Servus, ' + res.player.name + '!', 'ok');
    return res.player;
  }

  document.getElementById('btn-register').onclick = async () => {
    const p = await registerAndProceed(false);
    if (p) show('pick');
  };

  document.getElementById('btn-lucky').onclick = async () => {
    const p = await registerAndProceed(true);
    if (!p) return;
    await startLucky();
  };

  // Join via invite button — register, join session, go straight to game
  document.getElementById('btn-join-invite').onclick = async () => {
    const p = await registerAndProceed(false);
    if (!p) return;
    if (!invitePreview?.session) { toast('Einladung ungültig', 'err'); return; }
    try {
      const res = await POST('/api/session/join', {player_id: p.id, invite_code: inviteCode});
      if (res.error) { toast(res.error, 'err'); return; }
      G.session = res.session;
      setUrlParams({invite: null, sid: G.session.id});
      // Go straight to loading
      show('loading');
      document.getElementById('loading-muni').textContent = '📍 ' + (G.session.municipality_name||'');
      startTipRotation();
      startLoadingCountdown(20);
      await startGameWithLoading();
    } catch(e) { toast('Fehler beim Beitreten: ' + e.message, 'err'); }
  };

  inp.addEventListener('keydown', e => {
    if (invitePreview?.session) {
      if (e.key==='Enter') document.getElementById('btn-join-invite').click();
    } else {
      if (e.key==='Enter') document.getElementById('btn-register').click();
    }
  });
})();

window.quickLogin = async function() {
  const id = getUrlParam('pid');
  if (!id) return;
  try {
    const p = await GET('/api/player/'+id);
    if (p.error) { setUrlParams({pid:null,pname:null,rejoin:null}); return; }
    G.player = p;
    const sessions = await GET('/api/player/'+id+'/sessions');
    if (sessions?.length > 0) {
      G.session = sessions[0];
      // Go directly to loading screen
      show('loading');
      document.getElementById('loading-muni').textContent = '📍 ' + (G.session.municipality_name||'') + ' (' + (G.session.municipality_code||'') + ')';
      startTipRotation();
      startLoadingCountdown(20);
      await startGameWithLoading();
    } else {
      show('pick');
    }
  } catch(e) { setUrlParams({pid:null,pname:null,rejoin:null}); }
};

function savePlayer(p) {
  G.player = p;
  setUrlParams({pid: p.id, pname: p.name});
}

// Pick a random municipality and start loading immediately
async function startLucky() {
  show('loading');
  document.getElementById('loading-muni').textContent = '🍀 Zufallsgemeinde wird gewählt...';
  ['ls-session','ls-parcels','ls-kg','ls-treasures','ls-ready'].forEach(id => setLoadStep(id,''));
  startTipRotation();
  startLoadingCountdown(30);
  try {
    // Prefer "enhanced" gemeinden (lidar-processed KGs) ~90% of the time
    let picked = null;
    try {
      if (G.enhancedGemeinden.length === 0) await loadEnhancedRegistry();
      if (G.enhancedGemeinden.length > 0 && Math.random() < 0.9) {
        // Within enhanced, lean (~60%) toward gemeinden with a srtm v2 KG:
        // same UI, but far richer giant-tree data. Invisible to the player.
        const v2 = G.enhancedGemeinden.filter(g => g.v2);
        const pool = (v2.length >= 5 && Math.random() < 0.6) ? v2 : G.enhancedGemeinden;
        const g = pool[Math.floor(Math.random() * pool.length)];
        picked = { code: g.gemeinde_code, name: g.gemeinde_name, lon: g.lon, lat: g.lat, enhanced: true };
      }
    } catch(e) { console.error('enhanced lucky failed:', e); }
    if (!picked) {
      // Fallback: pick a random municipality from the full list
      let all = pickData.allMunis;
      if (!all || all.length === 0) {
        const res = await GET(CAD+'/search/municipalities?list=all&limit=5000&format=json');
        all = (res.data || res || []).filter(m => m.lon && m.lat);
        pickData.allMunis = all;
      }
      if (!all || all.length === 0) throw new Error('Keine Gemeinden geladen');
      const m = all[Math.floor(Math.random() * all.length)];
      picked = { code: m.gemeinde_code || m.code, name: m.name, lon: m.lon, lat: m.lat };
    }
    G.selectedMuni = picked;
    document.getElementById('loading-muni').textContent = '📍 ' + picked.name + ' (' + picked.code + ')' + (picked.enhanced ? ' ✨ Enhanced' : '');
    await startSinglePlayer();
  } catch(e) {
    console.error(e);
    toast('Fehler beim Zufallsstart: ' + e.message, 'err');
    show('welcome');
  }
}

// ================= MUNICIPALITY PICKER (Statistik Austria outlines via /search/municipalities) =================
let pickCanvas, pickCtx;
const pickData = { states: [], munis: [], hover: null };

function initPicker() {
  pickCanvas = document.getElementById('pick-canvas');
  pickCtx = pickCanvas.getContext('2d');
  resizePick();
  window.addEventListener('resize', () => { resizePick(); drawPick(); });
  pickCanvas.addEventListener('mousedown', onPickDown);
  pickCanvas.addEventListener('mousemove', onPickMove);
  pickCanvas.addEventListener('mouseup', onPickUp);
  pickCanvas.addEventListener('mouseleave', onPickUp);
  pickCanvas.addEventListener('wheel', onPickWheel, {passive:false});
  pickCanvas.addEventListener('click', onPickClick);

  // Touch support for mobile
  let pickTouchDist = 0;
  pickCanvas.addEventListener('touchstart', e => {
    if (e.touches.length === 1) {
      e.preventDefault();
      const touch = e.touches[0];
      onPickDown({clientX: touch.clientX, clientY: touch.clientY});
    } else if (e.touches.length === 2) {
      e.preventDefault();
      const dx = e.touches[0].clientX - e.touches[1].clientX;
      const dy = e.touches[0].clientY - e.touches[1].clientY;
      pickTouchDist = Math.sqrt(dx*dx + dy*dy);
      if (G.pick.drag.active) G.pick.drag.wasPinch = true;
    }
  }, {passive: false});

  pickCanvas.addEventListener('touchmove', e => {
    e.preventDefault();
    if (e.touches.length === 1 && G.pick.drag.active) {
      const touch = e.touches[0];
      onPickMove({clientX: touch.clientX, clientY: touch.clientY});
    } else if (e.touches.length === 2 && pickTouchDist > 0) {
      const dx = e.touches[0].clientX - e.touches[1].clientX;
      const dy = e.touches[0].clientY - e.touches[1].clientY;
      const d = Math.sqrt(dx*dx + dy*dy);
      G.pick.cam.zoom += (d/pickTouchDist - 1) * 2;
      G.pick.cam.zoom = Math.max(5, Math.min(14, G.pick.cam.zoom));
      pickTouchDist = d;
      drawPick();
    }
  }, {passive: false});

  pickCanvas.addEventListener('touchend', e => {
    const wasTap = G.pick.drag.active && !G.pick.drag.moved && !G.pick.drag.wasPinch;
    if (wasTap && e.changedTouches && e.changedTouches[0]) {
      const touch = e.changedTouches[0];
      onPickClick({clientX: touch.clientX, clientY: touch.clientY}, true);
    }
    onPickUp();
    if (e.touches.length === 0) {
      pickTouchDist = 0;
      if (G.pick.drag) G.pick.drag.wasPinch = false;
    }
  });

  // Search: municipalities + addresses in parallel (race-guarded)
  const inp = document.getElementById('input-search');
  const dd = document.getElementById('search-results');
  let timer, seq = 0;
  inp.addEventListener('input', () => {
    clearTimeout(timer);
    const q = inp.value.trim();
    if (q.length < 2) { dd.classList.remove('open'); return; }
    timer = setTimeout(async () => {
      const mySeq = ++seq;
      dd.innerHTML = '<div class="search-item"><small>Suche…</small></div>';
      dd.classList.add('open');
      const [muniRes, addrRes] = await Promise.allSettled([
        GET(CAD+'/lookup?q='+encodeURIComponent(q)+'&type=gemeinde&limit=6'),
        GET(CAD+'/search/address_osm?q='+encodeURIComponent(q)+'&limit=4'),
      ]);
      if (mySeq !== seq) return; // stale — newer query in flight
      const munis = muniRes.status==='fulfilled' ? (muniRes.value.data||[]) : [];
      const addrs = addrRes.status==='fulfilled' ? (addrRes.value.data||[]) : [];
      let html = munis.map(m => {
        const enh = G.enhancedGemeinden.some(g => String(g.gemeinde_code) === String(m.code||m.gemeinde_code));
        return `<div class="search-item" data-code="${m.code||m.gemeinde_code}" data-name="${esc(m.name||m.gemeinde_name)}">
          🏘️ ${esc(m.name||m.gemeinde_name)}${enh?' <span style="color:#7ee8fa">✨</span>':''}<br><small>${m.gemeinde_name&&m.gemeinde_name!==m.name?esc(m.gemeinde_name)+' · ':''}${m.code||m.gemeinde_code}</small></div>`;
      }).join('');
      html += addrs.map((a,i) => {
        const l = addrLabel(a);
        return `<div class="search-item" data-lon="${a.lon}" data-lat="${a.lat}" data-name="${esc(l.main)}">
          📍 ${esc(l.main)}${l.sub?'<br><small>'+esc(l.sub)+'</small>':''}</div>`;
      }).join('');
      dd.innerHTML = html || '<div class="search-item">Keine Ergebnisse</div>';
      dd.querySelectorAll('.search-item').forEach(el => {
        el.onclick = () => {
          dd.classList.remove('open');
          if (el.dataset.code) {
            pickMunicipality(el.dataset.code, el.dataset.name);
          } else if (el.dataset.lon) {
            // Address result - find municipality at that point
            findMuniAtPoint(parseFloat(el.dataset.lon), parseFloat(el.dataset.lat), el.dataset.name);
          }
        };
      });
    }, 300);
  });
  inp.addEventListener('keydown', e => {
    if (e.key === 'Escape') { dd.classList.remove('open'); inp.blur(); }
    if (e.key === 'Enter') {
      const first = dd.querySelector('.search-item[data-code],.search-item[data-lon]');
      if (first) first.click();
    }
  });

  document.getElementById('btn-back-pick').onclick = () => {
    G.pick.level = 'states';
    G.pick.cam = {lon:13.3, lat:47.5, zoom:7};
    G.pick.munis = [];
    document.getElementById('btn-back-pick').style.display = 'none';
    document.getElementById('pick-info').classList.remove('show');
    drawPick();
  };

  loadStates();
  // Load enhanced-KG registry so we can glow lidar-enhanced municipalities on the map
  if (G.enhancedGemeinden.length === 0) loadEnhancedRegistry().then(drawPick);
}

async function findMuniAtPoint(lon, lat, name) {
  const res = await GET(CAD+'/search/municipalities?contains_lon='+lon+'&contains_lat='+lat+'&limit=1&format=json');
  const items = res.data || [];
  if (items.length > 0) {
    pickMunicipality(items[0].gemeinde_code, items[0].name);
  } else {
    toast('Keine Gemeinde gefunden','err');
  }
}

async function loadStates() {
  // Load all states by fetching municipalities grouped - we'll draw state outlines
  // For perf, load states one by one as simplified outlines
  const states = ['Burgenland','Kärnten','Niederösterreich','Oberösterreich','Salzburg','Steiermark','Tirol','Vorarlberg','Wien'];
  // First load all munis as points for the overview
  try {
    const res = await GET(CAD+'/search/municipalities?list=all&limit=5000&format=json');
    const all = res.data || [];
    // Group by state and compute bounds
    const byState = {};
    for (const m of all) {
      const s = m.state || '?';
      if (!byState[s]) byState[s] = [];
      byState[s].push(m);
    }
    pickData.allMunis = all;
    pickData.byState = byState;
    drawPick();
  } catch(e) { console.error(e); }
}

async function loadStateMunis(state) {
  toast('Lade '+state+'...','');
  try {
    const res = await GET(CAD+'/search/municipalities?state='+encodeURIComponent(state)+'&limit=600&format=geojson');
    G.pick.munis = res.features || [];
    G.pick.level = 'munis';
    G.pick.state = state;
    // Fit view
    let minLon=Infinity,maxLon=-Infinity,minLat=Infinity,maxLat=-Infinity;
    for (const f of G.pick.munis) {
      const b = geoBounds(f.geometry);
      if (b.w<minLon) minLon=b.w; if (b.e>maxLon) maxLon=b.e;
      if (b.s<minLat) minLat=b.s; if (b.n>maxLat) maxLat=b.n;
    }
    G.pick.cam.lon = (minLon+maxLon)/2;
    G.pick.cam.lat = (minLat+maxLat)/2;
    // Calculate zoom to fit
    const lonRange = maxLon-minLon;
    const zoomFit = Math.log2(360 / lonRange * (pickCanvas.width/800));
    G.pick.cam.zoom = Math.max(8, Math.min(12, zoomFit));
    document.getElementById('btn-back-pick').style.display = '';
    drawPick();
  } catch(e) { console.error(e); toast('Fehler','err'); }
}

function resizePick() {
  if (!pickCanvas) return;
  const r = pickCanvas.parentElement;
  pickCanvas.width = r.clientWidth;
  pickCanvas.height = r.clientHeight - pickCanvas.offsetTop;
}

function pickProject(lon, lat) {
  const cam = G.pick.cam;
  const scale = Math.pow(2, cam.zoom) * 1.8;
  const x = (lon - cam.lon) * scale + pickCanvas.width/2;
  const y = (cam.lat - lat) * scale * 1.35 + pickCanvas.height/2;
  return [x, y];
}

function pickUnproject(x, y) {
  const cam = G.pick.cam;
  const scale = Math.pow(2, cam.zoom) * 1.8;
  const lon = (x - pickCanvas.width/2) / scale + cam.lon;
  const lat = cam.lat - (y - pickCanvas.height/2) / (scale * 1.35);
  return [lon, lat];
}

function drawPick() {
  if (!pickCtx) return;
  const W = pickCanvas.width, H = pickCanvas.height;
  const ctx = pickCtx;

  // Background - dark parchment
  ctx.fillStyle = '#1a1a10';
  ctx.fillRect(0, 0, W, H);

  // Grid
  ctx.strokeStyle = 'rgba(60,55,30,.25)';
  ctx.lineWidth = 0.5;
  for (let gx = -180; gx <= 180; gx += (G.pick.cam.zoom < 9 ? 1 : 0.2)) {
    const [x] = pickProject(gx, 0);
    if (x < 0 || x > W) continue;
    ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, H); ctx.stroke();
  }
  for (let gy = -90; gy <= 90; gy += (G.pick.cam.zoom < 9 ? 1 : 0.2)) {
    const [, y] = pickProject(0, gy);
    if (y < 0 || y > H) continue;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(W, y); ctx.stroke();
  }

  const stateColors = {
    'Burgenland':'#c86040','Kärnten':'#60a060','Niederösterreich':'#6080c0',
    'Oberösterreich':'#c0a040','Salzburg':'#a060a0','Steiermark':'#40a080',
    'Tirol':'#c07050','Vorarlberg':'#5090b0','Wien':'#d0a050'
  };

  // Enhanced (lidar) municipalities: precompute set of codes for glow
  if (!pickData.enhancedCodes || pickData.enhancedCount !== G.enhancedGemeinden.length) {
    pickData.enhancedCodes = new Set(G.enhancedGemeinden.map(g => String(g.gemeinde_code)));
    pickData.enhancedCount = G.enhancedGemeinden.length;
  }
  const glowPulse = 0.55 + Math.sin(Date.now()/600) * 0.25;

  if (G.pick.level === 'states' && pickData.allMunis) {
    // Draw municipality dots colored by state
    for (const m of pickData.allMunis) {
      if (!m.lon || !m.lat) continue;
      const [x, y] = pickProject(m.lon, m.lat);
      if (x < -5 || x > W+5 || y < -5 || y > H+5) continue;
      const isHover = pickData.hoverMuni === m;
      const isEnh = pickData.enhancedCodes.has(String(m.gemeinde_code || m.code));
      if (isEnh) {
        // Cyan glow halo for lidar-enhanced municipalities
        ctx.fillStyle = 'rgba(80,230,255,' + (0.25*glowPulse).toFixed(3) + ')';
        ctx.beginPath(); ctx.arc(x, y, 7, 0, Math.PI*2); ctx.fill();
        ctx.strokeStyle = 'rgba(80,230,255,' + (0.8*glowPulse).toFixed(3) + ')';
        ctx.lineWidth = 1;
        ctx.beginPath(); ctx.arc(x, y, 5, 0, Math.PI*2); ctx.stroke();
      }
      ctx.fillStyle = isHover ? '#ffd700' : (isEnh ? '#a0f0ff' : (stateColors[m.state] || '#888'));
      const sz = isHover ? 5 : (isEnh ? 4 : 3);
      ctx.fillRect(x-sz/2, y-sz/2, sz, sz);
    }
    // State labels
    if (pickData.byState) {
      ctx.font = '10px "Press Start 2P"';
      ctx.textAlign = 'center';
      for (const [state, munis] of Object.entries(pickData.byState)) {
        const clon = munis.reduce((s,m)=>s+(m.lon||0),0)/munis.length;
        const clat = munis.reduce((s,m)=>s+(m.lat||0),0)/munis.length;
        const [x, y] = pickProject(clon, clat);
        ctx.fillStyle = '#000';
        ctx.fillText(state, x+1, y+1);
        ctx.fillStyle = stateColors[state] || '#aaa';
        ctx.fillText(state, x, y);
      }
    }
    // Hover tooltip
    if (pickData.hoverMuni && pickData.hoverPos) {
      const m = pickData.hoverMuni;
      const hx = pickData.hoverPos[0], hy = pickData.hoverPos[1];
      const label = m.name + (m.district_name ? ' · ' + m.district_name : '');
      ctx.font = '16px VT323';
      const tw = ctx.measureText(label).width;
      const px = Math.min(hx + 12, W - tw - 16);
      const py = Math.max(hy - 10, 20);
      ctx.fillStyle = 'rgba(20,16,6,0.9)';
      ctx.fillRect(px - 4, py - 16, tw + 8, 22);
      ctx.strokeStyle = '#d4a843';
      ctx.lineWidth = 1;
      ctx.strokeRect(px - 4, py - 16, tw + 8, 22);
      ctx.fillStyle = '#ffd700';
      ctx.textAlign = 'left';
      ctx.fillText(label, px, py);
      ctx.textAlign = 'center';
    }
  } else if (G.pick.level === 'munis' && G.pick.munis.length) {
    // Draw municipality polygons - Settlers-style terrain fill
    for (const f of G.pick.munis) {
      const isEnh = pickData.enhancedCodes.has(String(f.properties.gemeinde_code || f.properties.code));
      drawMuniPoly(ctx, f, f === pickData.hover, isEnh, glowPulse);
    }
  }

  // Keep glow pulsing while picker is visible
  if (pickData.enhancedCodes.size > 0 && document.getElementById('screen-pick')?.classList.contains('active')) {
    if (!pickData.glowTimer) pickData.glowTimer = setTimeout(() => { pickData.glowTimer = null; drawPick(); }, 120);
  }
}

function drawMuniPoly(ctx, feature, isHover, isEnh, glowPulse) {
  const geom = feature.geometry;
  const rings = geom.type === 'MultiPolygon' ? geom.coordinates.map(p=>p[0]) : [geom.coordinates[0]];

  // Settlers-style fill: earthy greens with variation
  const hash = simpleHash(feature.properties.name || '');
  const baseColors = TERRAIN.grass;
  const color = baseColors[Math.abs(hash) % baseColors.length];

  for (const ring of rings) {
    ctx.beginPath();
    for (let i = 0; i < ring.length; i++) {
      const [x, y] = pickProject(ring[i][0], ring[i][1]);
      i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.fillStyle = isHover ? '#6ab050' : color;
    ctx.globalAlpha = isHover ? 0.9 : 0.7;
    ctx.fill();
    ctx.globalAlpha = 1;
    if (isEnh) {
      // Cyan glow for lidar-enhanced municipalities
      ctx.save();
      ctx.shadowColor = 'rgba(80,230,255,0.9)';
      ctx.shadowBlur = 8 + (glowPulse||0.5) * 8;
      ctx.strokeStyle = 'rgba(80,230,255,' + (0.5 + 0.4*(glowPulse||0.5)).toFixed(2) + ')';
      ctx.lineWidth = 2;
      ctx.stroke();
      ctx.restore();
      ctx.fillStyle = 'rgba(80,230,255,' + (0.05 + 0.05*(glowPulse||0.5)).toFixed(3) + ')';
      ctx.fill();
    }
    ctx.strokeStyle = isHover ? '#ffd700' : '#2a4020';
    ctx.lineWidth = isHover ? 2.5 : 1;
    ctx.stroke();
  }

  // Label
  if (G.pick.cam.zoom >= 9) {
    const b = geoBounds(geom);
    const [cx, cy] = pickProject((b.w+b.e)/2, (b.s+b.n)/2);
    ctx.font = MAP_FONT.label;
    ctx.textAlign = 'center';
    ctx.fillStyle = '#000';
    ctx.fillText(feature.properties.name, cx+1, cy+1);
    ctx.fillStyle = isHover ? '#ffd700' : '#e8dbb5';
    ctx.fillText(feature.properties.name, cx, cy);
  }
}

function simpleHash(s) { let h=0; for(let i=0;i<s.length;i++) h=((h<<5)-h)+s.charCodeAt(i); return h>>>0; }

function centroidOf(ring) {
  let sx=0, sy=0;
  for (const c of ring) { sx += c[0]; sy += c[1]; }
  return [sx/ring.length, sy/ring.length];
}

// ---- Multi-part geometry helpers ----
// Upstream returns MultiPolygon for any parcel with detached parts — very common
// for alpine Gemeindegut / Almen (a single Grundstück split by a ridge or river).
// Around Nauders ~2/3 of the loaded AREA was MultiPolygon, and every renderer /
// hit-test that did `geometry.coordinates[0]` silently treated those as invisible.
// Always go through these instead of indexing coordinates directly.

// Upstream now guarantees the RFC 7946 ring contract (fixed 2026-08-06,
// cadastre feedback #13): coordinates[0] of every part is the exterior ring
// wound CCW, every following ring is an interior hole wound CW, and disjoint
// shells come back as MultiPolygon parts — identical across /export/geojson,
// /spatial/parcels and /parcels/geometry/batch. Before that fix ring[0] was
// often a tiny sliver (84108-3394/1 = [335, 95, 20, 8506967] m²), which made
// huge alpine parcels render fine but be completely unclickable.
//
// We still evaluate ALL rings with the even-odd rule rather than trusting the
// order: it is identical to the contract when the contract holds, it also
// excludes holes properly (ring[0]-only never did), and it degrades gracefully
// if a stale cache or a not-yet-reprocessed KG serves the old ring bag.
// Part order is explicitly NOT part of the contract — never index into parts.

/** @deprecated holes matter now — use geomAllRings() + even-odd. */
function geomOuterRings(g) { return geomAllRings(g); }

/** Every ring (outer + holes) — for filling with the even-odd rule. */
function geomAllRings(g) {
  if (!g) return [];
  if (g.type === 'Polygon') return g.coordinates;
  if (g.type === 'MultiPolygon') { const o = []; for (const p of g.coordinates) for (const r of p) o.push(r); return o; }
  return [];
}

/** Signed area of a ring in squared degrees (sign = winding). */
function ringArea2(r) {
  let a = 0;
  for (let i = 0, n = r.length; i < n; i++) {
    const p = r[i], q = r[(i + 1) % n];
    a += p[0] * q[1] - q[0] * p[1];
  }
  return a / 2;
}

/** Largest ring by area — anchor for labels/sprites/centroids. */
function biggestRing(g) {
  let best = null, ba = -1;
  for (const r of geomAllRings(g)) {
    if (r.length < 3) continue;
    const a = Math.abs(ringArea2(r));
    if (a > ba) { ba = a; best = r; }
  }
  return best;
}

/** Is this an area geometry we can fill? */
function isAreaGeom(g) { return !!g && (g.type === 'Polygon' || g.type === 'MultiPolygon'); }

/** Representative lon/lat for a parcel feature: explicit props, else the
 *  centroid of the largest part, else the raw point coords. */
function featureLonLat(f) {
  const p = f.properties || {};
  if (p.lon != null && p.lat != null) return [p.lon, p.lat];
  const r = biggestRing(f.geometry);
  if (r) return centroidOf(r);
  const c = f.geometry && f.geometry.coordinates;
  return (c && typeof c[0] === 'number') ? [c[0], c[1]] : [null, null];
}

/** Point-in-area across every part, even-odd rule (matches the fill). */
function pipGeom(lon, lat, g) {
  return pipRings(lon, lat, geomAllRings(g));
}

/** Even-odd point-in-rings, for pre-extracted ring arrays. */
function pipRings(lon, lat, rings) {
  let inside = false;
  for (const r of rings) if (pip(lon, lat, r)) inside = !inside;
  return inside;
}

function geoBounds(geom) {
  let w=Infinity,e=-Infinity,s=Infinity,n=-Infinity;
  const processCoord = c => { if(c[0]<w)w=c[0]; if(c[0]>e)e=c[0]; if(c[1]<s)s=c[1]; if(c[1]>n)n=c[1]; };
  const walk = a => { if(typeof a[0]==='number') processCoord(a); else a.forEach(walk); };
  walk(geom.coordinates);
  return {w,e,s,n};
}

// Pick canvas interactions
function onPickDown(ev) {
  G.pick.drag = { active:true, sx:ev.clientX, sy:ev.clientY, slon:G.pick.cam.lon, slat:G.pick.cam.lat, moved:false, wasPinch:false };
  pickCanvas.classList.add('dragging');
}
function onPickMove(ev) {
  if (G.pick.drag.active) {
    const dx = ev.clientX - G.pick.drag.sx;
    const dy = ev.clientY - G.pick.drag.sy;
    if (Math.abs(dx)+Math.abs(dy) > 4) G.pick.drag.moved = true;
    const scale = Math.pow(2, G.pick.cam.zoom) * 1.8;
    G.pick.cam.lon = G.pick.drag.slon - dx / scale;
    G.pick.cam.lat = G.pick.drag.slat + dy / (scale * 1.35);
    drawPick();
  } else if (G.pick.level === 'states' && pickData.allMunis) {
    // Hover detection on municipality dots
    const rect = pickCanvas.getBoundingClientRect();
    const mx = ev.clientX - rect.left, my = ev.clientY - rect.top;
    let best = null, bestD = Infinity;
    for (const m of pickData.allMunis) {
      if (!m.lon || !m.lat) continue;
      const [x, y] = pickProject(m.lon, m.lat);
      const d = Math.abs(x - mx) + Math.abs(y - my);
      if (d < bestD) { bestD = d; best = m; }
    }
    const hit = bestD < 18 ? best : null;
    if (hit !== pickData.hoverMuni) {
      pickData.hoverMuni = hit;
      pickData.hoverPos = hit ? [mx, my] : null;
      pickCanvas.style.cursor = hit ? 'pointer' : 'grab';
      drawPick();
    } else if (hit) {
      pickData.hoverPos = [mx, my];
      drawPick();
    }
  } else if (G.pick.level === 'munis') {
    // Hover detection on polygons
    const rect = pickCanvas.getBoundingClientRect();
    const [lon, lat] = pickUnproject(ev.clientX - rect.left, ev.clientY - rect.top);
    let found = null;
    for (const f of G.pick.munis) {
      if (geoContains(f.geometry, lon, lat)) { found = f; break; }
    }
    if (found !== pickData.hover) { pickData.hover = found; pickCanvas.style.cursor = found ? 'pointer' : 'grab'; drawPick(); }
  }
}
function onPickUp() {
  pickCanvas.classList.remove('dragging');
  G.pick.drag.active = false;
}
function onPickWheel(ev) {
  ev.preventDefault();
  G.pick.cam.zoom += ev.deltaY > 0 ? -0.4 : 0.4;
  G.pick.cam.zoom = Math.max(5, Math.min(14, G.pick.cam.zoom));
  drawPick();
}
function onPickClick(ev, isTouch) {
  if (G.pick.drag.moved) return;
  const rect = pickCanvas.getBoundingClientRect();
  const mx = ev.clientX - rect.left, my = ev.clientY - rect.top;
  const [lon, lat] = pickUnproject(mx, my);

  if (G.pick.level === 'states' && pickData.allMunis) {
    // Find nearest municipality dot
    let best = null, bestD = Infinity;
    for (const m of pickData.allMunis) {
      if (!m.lon || !m.lat) continue;
      const [x, y] = pickProject(m.lon, m.lat);
      const d = Math.abs(x - mx) + Math.abs(y - my);
      if (d < bestD) { bestD = d; best = m; }
    }
    // Use larger hit area for touch (50px) vs mouse (18px)
    const threshold = isTouch ? 50 : 18;
    if (best && bestD < threshold) {
      // Direct click on municipality dot → start game!
      pickMunicipality(best.gemeinde_code || best.code, best.name);
    }
  } else if (G.pick.level === 'munis') {
    for (const f of G.pick.munis) {
      if (geoContains(f.geometry, lon, lat)) {
        // Direct click on polygon → start game!
        const p = f.properties;
        pickMunicipality(p.gemeinde_code, p.name);
        return;
      }
    }
  }
}

function geoContains(geom, lon, lat) {
  const rings = geom.type === 'MultiPolygon' ? geom.coordinates.map(p=>p[0]) : [geom.coordinates[0]];
  for (const ring of rings) { if (pip(lon, lat, ring)) return true; }
  return false;
}
function polyCentroid(ring) {
  let sx=0,sy=0;
  for (const c of ring) { sx+=c[0]; sy+=c[1]; }
  return [sx/ring.length, sy/ring.length];
}
function pip(x, y, poly) {
  let inside = false;
  for (let i=0,j=poly.length-1; i<poly.length; j=i++) {
    const xi=poly[i][0],yi=poly[i][1],xj=poly[j][0],yj=poly[j][1];
    if ((yi>y)!==(yj>y) && x<(xj-xi)*(y-yi)/(yj-yi)+xi) inside=!inside;
  }
  return inside;
}

// showMuniInfo removed — direct click starts the game now

window.pickMunicipality = async function(code, name) {
  G.selectedMuni = { code, name };
  // Try to get coords from already-loaded data
  if (pickData.allMunis) {
    const m = pickData.allMunis.find(m => (m.gemeinde_code||m.code) === code);
    if (m) { G.selectedMuni.lon = m.lon; G.selectedMuni.lat = m.lat; }
  }
  // Fallback: fetch
  if (!G.selectedMuni.lon) {
    try {
      const res = await GET(CAD+'/search/municipalities?q='+encodeURIComponent(name)+'&limit=1&format=json');
      const items = res.data || [];
      if (items.length > 0) { G.selectedMuni.lon = items[0].lon; G.selectedMuni.lat = items[0].lat; }
    } catch(e) {}
  }
  if (!G.selectedMuni.lon) { G.selectedMuni.lon = 13.5; G.selectedMuni.lat = 47.5; }

  // Check pending invite (stored in URL param)
  const inv = getUrlParam('invite');
  if (inv) {
    setUrlParams({invite: null});
    try {
      const res = await POST('/api/session/join', {player_id:G.player.id, invite_code:inv});
      if (res.session) { G.session = res.session; startGameWithLoading(); return; }
    } catch(e) {}
  }

  // Skip lobby, go straight to loading screen
  startSinglePlayer();
};

async function startSinglePlayer() {
  const m = G.selectedMuni;
  G.loadStart = Date.now();
  const alreadyLoading = document.getElementById('screen-loading').classList.contains('active');
  show('loading');
  document.getElementById('loading-muni').textContent = '📍 ' + m.name + ' (' + m.code + ')';
  // Reset all steps
  ['ls-session','ls-parcels','ls-kg','ls-treasures','ls-ready'].forEach(id => setLoadStep(id, ''));
  setLoadStep('ls-session', 'active');
  setLoadProgress(5);
  if (!alreadyLoading) {
    startTipRotation();
    startLoadingCountdown(30);
  }

  // Create session automatically
  const res = await POST('/api/session/create', {
    player_id:G.player.id, name:m.name+' Siedlung',
    municipality_code:m.code, municipality_name:m.name,
    center_lon:m.lon, center_lat:m.lat,
  });
  if (res.error) { toast(res.error,'err'); show('pick'); return; }
  G.session = res.session;
  G.session.invite_code = res.invite_code;
  setLoadStep('ls-session', 'done');
  setLoadProgress(15);
  // Show invite panel on loading screen
  showLoadingInvite(res.invite_code);

  // Now load game data with progress
  await startGameWithLoading();
}

function setLoadStep(id, state) {
  const el = document.getElementById(id);
  if (!el) return;
  el.classList.remove('active','done');
  if (state) el.classList.add(state);
}

let _loadPct = 0;
function setLoadProgress(pct) {
  _loadPct = Math.min(100, pct);
  // Circular ring
  const ring = document.getElementById('ring-fg');
  if (ring) {
    const circumference = 2 * Math.PI * 52; // ~326.7
    ring.style.strokeDashoffset = circumference * (1 - _loadPct / 100);
  }
  const pctEl = document.getElementById('loading-ring-pct');
  if (pctEl) pctEl.textContent = Math.round(_loadPct) + '%';
}
function setLoadSub(text) {
  const el = document.getElementById('loading-sub');
  if (el) el.textContent = text;
}

let tipInterval = null;
function startTipRotation() {
  let idx = 0;
  const tips = document.querySelectorAll('.loading-tip');
  if (tips.length === 0) return;
  tipInterval = setInterval(() => {
    tips[idx].classList.remove('active');
    idx = (idx + 1) % tips.length;
    tips[idx].classList.add('active');
  }, 4000);
}
function stopTipRotation() {
  if (tipInterval) { clearInterval(tipInterval); tipInterval = null; }
}

// Smooth progress animation — interpolates toward target
let _smoothProgressRAF = null;
let _smoothPctCurrent = 0;
function startSmoothProgress() {
  function tick() {
    if (Math.abs(_smoothPctCurrent - _loadPct) > 0.2) {
      _smoothPctCurrent += (_loadPct - _smoothPctCurrent) * 0.08;
      const ring = document.getElementById('ring-fg');
      if (ring) {
        const circumference = 2 * Math.PI * 52;
        ring.style.strokeDashoffset = circumference * (1 - _smoothPctCurrent / 100);
      }
      const pctEl = document.getElementById('loading-ring-pct');
      if (pctEl) pctEl.textContent = Math.round(_smoothPctCurrent) + '%';
    }
    _smoothProgressRAF = requestAnimationFrame(tick);
  }
  tick();
}
function stopSmoothProgress() {
  if (_smoothProgressRAF) { cancelAnimationFrame(_smoothProgressRAF); _smoothProgressRAF = null; }
}

// Legacy compatibility
function startLoadingCountdown(sec) { startSmoothProgress(); }
function stopLoadingCountdown() {
  stopSmoothProgress();
  // Snap to 100%
  setLoadProgress(100);
  setLoadSub('✅ Bereit!');
}

function showLoadingInvite(inviteCode) {
  const panel = document.getElementById('loading-invite');
  const urlEl = document.getElementById('loading-invite-url');
  const copyBtn = document.getElementById('loading-invite-copy');
  if (!panel || !urlEl) return;
  const url = location.origin + '/join/' + inviteCode;
  urlEl.textContent = url;
  copyBtn.onclick = () => {
    navigator.clipboard.writeText(url);
    copyBtn.textContent = '✅ Kopiert!';
    setTimeout(() => { copyBtn.textContent = '📋 Kopieren'; }, 2000);
  };
  panel.style.display = '';
}

// ================= LOBBY =================
{
  document.getElementById('btn-create-session').onclick = async () => {
    const m = G.selectedMuni;
    const res = await POST('/api/session/create', {
      player_id:G.player.id, name:m.name+' Siedlung',
      municipality_code:m.code, municipality_name:m.name,
      center_lon:m.lon, center_lat:m.lat,
    });
    if (res.error) { toast(res.error,'err'); return; }
    G.session = res.session;
    G.session.invite_code = res.invite_code;
    showLobbyWaiting(res.invite_code);
  };

  document.getElementById('btn-join-session').onclick = async () => {
    const code = document.getElementById('input-invite').value.trim();
    if (!code) return;
    const res = await POST('/api/session/join', {player_id:G.player.id, invite_code:code});
    if (res.error) { toast(res.error,'err'); return; }
    G.session = res.session;
    startGame();
  };

  document.getElementById('btn-start').onclick = () => startGame();
}

function showLobbyWaiting(inviteCode) {
  document.getElementById('lobby-create').style.display = 'none';
  document.getElementById('lobby-waiting').style.display = '';
  const url = inviteUrl(inviteCode);
  const box = document.getElementById('invite-url');
  box.textContent = url;
  box.onclick = () => { navigator.clipboard.writeText(inviteUrl(inviteCode)); toast('📋 Kopiert!','ok'); };
  refreshLobby();
}

async function refreshLobby() {
  if (!G.session) return;
  const pl = await GET('/api/session/'+G.session.id+'/players');
  document.getElementById('lobby-players').innerHTML = (pl||[]).map(p => `<li>${esc(p.name)} (${p.coins}🪙)</li>`).join('');
}

// ================= MAIN GAME =================
let gc, gctx, mc, mctx;
let _animFrame = null; // for smooth camera animation

/** Smoothly animate camera to target lon/lat/zoom over durationMs */
function animateCamera(targetLon, targetLat, targetZoom, durationMs) {
  if (_animFrame) cancelAnimationFrame(_animFrame);
  const startLon = G.cam.lon, startLat = G.cam.lat, startZoom = G.cam.zoom;
  const startTime = performance.now();
  durationMs = durationMs || 500;
  function step(now) {
    let t = Math.min(1, (now - startTime) / durationMs);
    // ease-out cubic
    t = 1 - Math.pow(1 - t, 3);
    G.cam.lon = startLon + (targetLon - startLon) * t;
    G.cam.lat = startLat + (targetLat - startLat) * t;
    G.cam.zoom = startZoom + (targetZoom - startZoom) * t;
    render(); renderMini();
    if (t < 1) { _animFrame = requestAnimationFrame(step); }
    else { _animFrame = null; loadMoreParcels(); }
  }
  _animFrame = requestAnimationFrame(step);
}

async function startGameWithLoading() {
  if (!G.loadStart) G.loadStart = Date.now();
  // Show loading screen if not already showing
  if (!document.getElementById('screen-loading').classList.contains('active')) {
    show('loading');
    document.getElementById('loading-muni').textContent = '📍 ' + G.session.municipality_name;
    ['ls-session','ls-parcels','ls-kg','ls-treasures','ls-ready'].forEach(id => setLoadStep(id, ''));
    setLoadStep('ls-session', 'done');
    setLoadProgress(15);
    startTipRotation();
    startLoadingCountdown(25);
    if (G.session?.invite_code) showLoadingInvite(G.session.invite_code);
  }

  // Set camera — use shared view from invite URL hash if present, else municipality center
  const sharedView = parseViewHash();
  if (sharedView) {
    G.cam.lon = sharedView.lon;
    G.cam.lat = sharedView.lat;
    G.cam.zoom = sharedView.zoom;
    history.replaceState(null, '', location.pathname + location.search); // clean hash
  } else {
    G.cam.lon = G.session.center_lon;
    G.cam.lat = G.session.center_lat;
    G.cam.zoom = 17;
  }
  // Track known municipalities so we only toast once per muni
  G.knownMunis = new Set();
  G.knownMunis.add(G.session.municipality_code);
  G.homeMuni = G.session.municipality_name;

  // Kick the enhanced-KG registry now (parallel with parcels) so KG loading
  // knows which KGs are lidar-enhanced and can SKIP the heavy cadastre landuse
  // fetch for them (srtm dominant_type + OSM lines cover the backdrop instead).
  const registryReady = G.enhancedKGs.size ? Promise.resolve() : loadEnhancedRegistry();

  // Step 2: Load parcels
  setLoadStep('ls-parcels', 'active');
  setLoadProgress(15);
  setLoadSub('Parzellen-Punkte werden geladen...');
  await loadParcels();
  await registryReady; // ensure enhancedKGs is populated before KG landuse decisions
  setLoadSub(G.parcels.length + ' Parzellen gefunden');
  setLoadStep('ls-parcels', 'done');
  setLoadProgress(25);

  // Step 3: Load KG polygons — the longest step, with fine-grained progress
  setLoadStep('ls-kg', 'active');
  setLoadProgress(28);
  setLoadSub('Katastralgemeinden werden ermittelt...');
  await fetchKGPolygonsBlocking();
  buildEZIndex();
  // Enhanced mode: fetch lidar/OSM/N2K data in the BACKGROUND (never blocks loading).
  // Registry is already loaded above; just kick the per-KG enhanced fetches.
  loadEnhancedForKGs();
  // Giant trees unlock persists: check if player already found a treasure
  loadTallSeen();
  GET('/api/player/'+G.player.id).then(pl => {
    if (pl && pl.treasures_found > 0) { G.tallUnlocked = true; }
  }).catch(()=>{});
  // If parcels were empty (bbox failed) but we loaded polygon data, synthesize point parcels
  if (G.parcels.length === 0 && G.parcelPolys.length > 0) {
    for (const f of G.parcelPolys) {
      const p = f.properties;
      const c = featureLonLat(f);
      if (c[0] == null) continue;
      G.parcels.push({type:'Feature', properties:{...p, lon:c[0], lat:c[1]}, geometry:{type:'Point', coordinates:c}});
    }
  }
  setLoadSub(G.parcelPolys.length + ' Polygon-Geometrien, ' + G.buildingFootprints.length + ' Gebäude geladen');
  setLoadStep('ls-kg', 'done');
  setLoadProgress(75);

  // Step 4: Load treasures/species, challenges, etc.
  setLoadStep('ls-treasures', 'active');
  setLoadProgress(78);
  setLoadSub('Bedrohte Arten und Schätze werden platziert...');
  await Promise.all([loadClaimed(), loadOffers(), loadTreasures(), loadChallenges(), loadPlayers(), loadBio(), loadChat()]);
  const speciesCount = (G.treasures||[]).filter(t => t.treasure_type === 'species').length;
  setLoadSub(speciesCount + ' seltene Arten versteckt, ' + (G.treasures||[]).length + ' Schätze total');
  setLoadStep('ls-treasures', 'done');
  setLoadProgress(88);

  // Step 5: Render
  setLoadStep('ls-ready', 'active');
  setLoadProgress(92);
  setLoadSub('Karte wird gerendert...');

  // Pre-generate grass pattern
  createGrassPattern();
  loadAustriaBorder(); // background: national outline for the border overlay
  connectSSE();

  setLoadStep('ls-ready', 'done');
  setLoadProgress(100);
  setLoadSub('✅ Bereit — Viel Spaß beim Siedeln!');

  // Brief minimum so the loading screen doesn't flash (was 4s — now get in fast)
  const elapsed = Date.now() - (G.loadStart || 0);
  const minWait = getUrlParam('dev') ? 0 : Math.max(300, 1500 - elapsed);
  if (minWait) await new Promise(r => setTimeout(r, minWait));
  stopTipRotation();
  stopLoadingCountdown();
  show('game');
  setTimeout(() => Herald.start(G.freshPlayer ? 'intro' : 'quest'), 400);

  // Init canvas AFTER showing the game screen (so clientWidth/Height > 0)
  gc = document.getElementById('game-canvas');
  gctx = gc.getContext('2d');
  mc = document.getElementById('mini-canvas');
  mctx = mc.getContext('2d');
  const gt = document.getElementById('game-title');
  gt.textContent = G.session.municipality_name;
  gt.classList.add('kg-link');
  gt.title = 'KG-Übersicht anzeigen';
  gt.onclick = async () => {
    let kg = kgAtCamera();
    if (!kg) {
      // Resolve the KG under the camera via the fast spatial point lookup
      try {
        const r = await GET(CAD + '/spatial/point?lon=' + G.cam.lon + '&lat=' + G.cam.lat + '&attrs_only=1&limit=1');
        kg = r?.data?.parcels?.[0]?.kg_code || null;
      } catch(e) {}
    }
    if (!kg) kg = G.kgsLoaded.values().next().value;
    if (kg) openKGSummary(kg);
    else toast('Noch keine KG-Daten geladen', 'err');
  };
  updateStats();
  resizeGame();
  window.addEventListener('resize', () => { resizeGame(); render(); });
  initGameInput();
  render();
  renderMini();

  document.getElementById('btn-invite').onclick = () => {
    navigator.clipboard.writeText(inviteUrl(G.session.invite_code));
    toast('📋 Einladung kopiert!','ok');
  };

  // Show rejoin link in sidebar — encode as URL with pid param (no localStorage)
  const rejoinParam = getUrlParam('rejoin');
  const rejoinUrl = rejoinParam
    ? location.origin + '/rejoin/' + rejoinParam
    : null;
  if (rejoinUrl) {
    const sec = document.getElementById('sec-rejoin');
    const link = document.getElementById('rejoin-ingame-link');
    sec.style.display = '';
    link.onclick = (e) => { e.preventDefault(); navigator.clipboard.writeText(rejoinUrl); toast('🔑 Wiedereinstiegs-Link kopiert!','ok'); };
  }

  // Show join/invite link in sidebar
  if (G.session && G.session.invite_code) {
    const sec = document.getElementById('sec-rejoin');
    const joinLink = document.getElementById('join-ingame-link');
    sec.style.display = '';
    joinLink.onclick = (e) => { e.preventDefault(); navigator.clipboard.writeText(inviteUrl(G.session.invite_code)); toast('⚔️ Einladungs-Link kopiert!','ok'); };
  }
}

// Legacy startGame for lobby "Spiel starten" button
async function startGame() { await startGameWithLoading(); }

function resizeGame() {
  const wrap = document.getElementById('game-main');
  gc.width = wrap.clientWidth;
  gc.height = wrap.clientHeight;
  mc.width = 180;
  mc.height = 130;
}

// ---- Data loading ----
async function loadParcels() {
  try {
    const r = 0.008;
    const url = CAD+'/spatial/bbox?west='+(G.cam.lon-r)+'&south='+(G.cam.lat-r)+
      '&east='+(G.cam.lon+r)+'&north='+(G.cam.lat+r)+'&layers=parcels&limit=800&format=geojson';
    const data = await GET(url);
    if (data.features) G.parcels = data.features;
    else if (data.data?.parcels) {
      G.parcels = data.data.parcels.map(p => ({
        type:'Feature', properties:p,
        geometry:{type:'Point',coordinates:[p.lon,p.lat]}
      }));
    }
  } catch(e) { console.error(e); }

  // Fallback: if bbox returned nothing, discover KGs from municipality and load polygons directly
  if (G.parcels.length === 0 && G.session.municipality_name) {
    try {
      const res = await GET(CAD+'/search/kg?gemeinde='+encodeURIComponent(G.session.municipality_name)+'&limit=50');
      const kgs = res.data || [];
      for (const kg of kgs) {
        if (kg.kg_code && !G.kgsLoaded.has(kg.kg_code)) {
          G.kgsLoaded.add(kg.kg_code);
          G.municipalityKGs = G.municipalityKGs || [];
          G.municipalityKGs.push(kg.kg_code);
        }
      }
    } catch(e) { console.error('KG lookup fallback failed:', e); }
  }
}

async function loadMoreParcels() {
  const b = viewBounds();
  // Keep the Enhanced badge in sync with the camera even when zoomed too far
  // out to fetch point parcels (below), so it reappears/hides on every pan.
  updateEnhancedBadge();

  // Polygon geometry ALWAYS loads, at every zoom: fetchKGPolygons tiles the
  // viewport itself, so a wide view just means more (smaller) tiles. This used
  // to sit behind the `span > 0.04` guard below, which — because gc.width is in
  // *device* pixels — tripped at zoom ≈15 on a wide/retina screen and made the
  // map silently stop loading when you panned into a new KG.
  fetchKGPolygons().then(() => buildEZIndex()).catch(e => console.error(e));
  loadToponyms().catch(e => console.error(e));
  detectAdjacentMunicipalities();
  checkViewportMunicipality();

  // Point-parcel fallback layer (centroids only) stays gated: it's capped at
  // 800 rows, so over a huge bbox it would return a useless random subset.
  if ((b.e-b.w) > 0.04) return;
  try {
    const url = CAD+'/spatial/bbox?west='+b.w+'&south='+b.s+'&east='+b.e+'&north='+b.n+'&layers=parcels&limit=800&format=geojson';
    const data = await GET(url);
    const feats = data.features || (data.data?.parcels||[]).map(p=>({type:'Feature',properties:p,geometry:{type:'Point',coordinates:[p.lon,p.lat]}}));
    const ids = new Set(G.parcels.map(f=>f.properties.parcel_id));
    let added = 0;
    for (const f of feats) { if (!ids.has(f.properties.parcel_id)) { G.parcels.push(f); added++; } }
    if (added > 0) { render(); renderMini(); }
  } catch(e) { console.error(e); }
}

/** Fetch all pages of a KG layer via /api/kg/{code}?layer=...&page=N.
 *  pagesize 500 keeps each page under the exe.dev ~500KB proxy limit while
 *  cutting round-trips ~2.5× vs the old 200. */
async function fetchKGLayer(kg, layer, pagesize) {
  const features = [];
  let page = 0;
  const ps = pagesize || 500;
  while (true) {
    const data = await GET('/api/kg/'+kg+'?layer='+layer+'&page='+page+'&pagesize='+ps);
    // Still pending after api()'s own wait budget (cold KG, slow Zenodo):
    // throw so the caller can un-mark the KG and retry later, instead of
    // silently recording "no landuse here" for the session.
    if (data.pending) { const e = new Error('pending'); e.pending = true; e.retryAfter = data.retry_after_s || 10; throw e; }
    if (data.error) throw new Error(data.error);
    if (data.features) for (const f of data.features) features.push(f);
    if (!data.has_more) break;
    page++;
  }
  return features;
}

// (The whole-KG landuse streamer loadLanduseBackground() was replaced by the
// per-tile CAD-1 slice loadViewportLanduse(); see loadViewportGeometry.)

/** Fast viewport polygon load. Pulls parcel + footprint geometry for JUST the
 *  given bbox from the server's /api/viewport fast path (upstream R-tree, ~100ms,
 *  ~40KB gzip) instead of loading whole KGs' export/geojson (multi-MB each).
 *  Dedups by parcel_id / footprint_id and by quantized tile so pans are cheap.
 *  Merged features carry the same {properties, geometry} shape the renderer and
 *  EZ index already expect. Returns the number of newly added parcels. */
async function loadViewportGeometry(b, opts) {
  opts = opts || {};
  // Quantize bbox to a ~150m grid; skip if we've already fetched this exact tile.
  const q = v => Math.round(v / 0.002) * 0.002;
  const tileKey = [q(b.w), q(b.s), q(b.e), q(b.n)].map(x => x.toFixed(3)).join(',');
  if (!opts.force && G.vpTiles.has(tileKey)) return { added: 0, ready: true, truncated: false, cached: true };
  G.vpTiles.add(tileKey);
  let data;
  vpBusy(1);
  try {
    data = await GET('/api/viewport?west='+b.w+'&south='+b.s+'&east='+b.e+'&north='+b.n+'&limit='+(opts.limit||6000));
  } catch(e) { console.error('viewport fetch failed', e); G.vpTiles.delete(tileKey); return { added:0, ready:false, truncated:false }; }
  finally { vpBusy(-1); }
  if (!data) { G.vpTiles.delete(tileKey); return { added:0, ready:false, truncated:false }; }
  if (data.pending) { G.vpTiles.delete(tileKey); return { added:0, ready:false, truncated:false, retryAfter: data.retry_after_s }; }
  // If upstream wasn't fully warm yet, allow a later re-fetch of this tile.
  // The server forwards upstream's Zenodo warming state (retry_after_s +
  // per-KG pct/eta) so we can pace the retry and show progress.
  if (data.ready === false) { G.vpTiles.delete(tileKey); if (data.warming) pendingNotice(data.warming, 'viewport'); }
  // Truncated means the tile hit the row limit: some geometry in this bbox was
  // dropped, so let a subdivided re-fetch cover it.
  if (data.truncated) G.vpTiles.delete(tileKey);

  let addedP = 0, needLanduse = false;
  for (const it of (data.parcels||[])) {
    const id = it.parcel_id;
    if (!id || G.polyIds.has(id) || !it.geometry) continue;
    G.polyIds.add(id);
    const { geometry, ...props } = it;
    // Normalize landuse (viewport returns `landuse`, renderer also reads landuse_summary)
    if (props.landuse && !props.landuse_summary) props.landuse_summary = props.landuse;
    G.parcelPolys.push({ type:'Feature', properties: props, geometry });
    addedP++;
    if (props.kg_code) {
      G.kgsLoaded.add(props.kg_code);
      loadWaterForKG(props.kg_code);
      // Non-enhanced KGs get a landuse backdrop; enhanced KGs have lidar dom + OSM.
      if (!G.enhancedKGs.has(props.kg_code)) needLanduse = true;
    }
  }
  // Companion layers for this same tile (CAD-1 viewport landuse slice instead
  // of the ~7 MB whole-KG export; FARM-2 INVEKOS fields). Background, never awaited.
  if (needLanduse) loadViewportLanduse(b);
  loadSchlaege(b);
  for (const it of (data.footprints||[])) {
    const id = it.footprint_id;
    if (!id || G.fpIds.has(id) || !it.geometry) continue;
    G.fpIds.add(id);
    const { geometry, ...props } = it;
    G.buildingFootprints.push({ type:'Feature', properties: props, geometry });
  }
  return { added: addedP, ready: data.ready !== false, truncated: !!data.truncated, retryAfter: data.retry_after_s };
}

/** Generic viewport-sliced companion layer loader (shared by landuse + Schläge).
 *  Dedups by quantized tile + feature key, retries ready:false tiles a few times
 *  while the camera is still nearby. */
async function loadBboxLayer(b, o) {
  const q = v => Math.round(v / 0.002) * 0.002;
  const tileKey = [q(b.w), q(b.s), q(b.e), q(b.n)].map(x => x.toFixed(3)).join(',');
  if (o.tiles.has(tileKey)) return 0;
  o.tiles.add(tileKey);
  let data;
  try { data = await GET(o.url + '?west=' + b.w + '&south=' + b.s + '&east=' + b.e + '&north=' + b.n); }
  catch (e) { o.tiles.delete(tileKey); return 0; }
  if (!data || data.pending || data.ready === false) {
    o.tiles.delete(tileKey);
    const attempt = (o.attempts[tileKey] || 0) + 1;
    if (attempt <= 3) {
      o.attempts[tileKey] = attempt;
      const wait = Math.min(Math.max(+(data && data.retry_after_s) || 4, 3), 30) * 1000 * attempt;
      setTimeout(() => { const v = viewBounds(); if (b.e >= v.w && b.w <= v.e && b.n >= v.s && b.s <= v.n) loadBboxLayer(b, o); }, wait);
    }
    return 0;
  }
  let added = 0;
  for (const it of (data[o.key] || [])) {
    if (!it.geometry || !isAreaGeom(it.geometry)) continue;
    const id = o.idOf(it);
    if (!id || o.ids.has(id)) continue;
    o.ids.add(id);
    const { geometry, ...props } = it;
    o.onFeature(props, geometry, data);
    added++;
  }
  if (added) o.done(added, data);
  return added;
}

/** CAD-1: landuse polygons for one viewport tile (forest, water, roads, fields). */
function loadViewportLanduse(b) {
  loadBboxLayer(b, {
    url: '/api/viewport-landuse', key: 'landuse', tiles: G.luTiles, ids: G.luIds, attempts: (loadViewportLanduse._a = loadViewportLanduse._a || {}),
    idOf: it => { const r = biggestRing(it.geometry); return r && r.length ? it.kg_code + '|' + it.code + '|' + it.area_sqm + '|' + r[0][0] + ',' + r[0][1] : null; },
    onFeature: (props, geometry) => { props.landuse_code = String(props.code || ''); G.landusePolys.push({ type: 'Feature', properties: props, geometry }); },
    done: () => { render(); renderMini(); },
  });
}

/** FARM-2: INVEKOS Schläge — the real crop on every field (AMA, CC BY 4.0). */
function loadSchlaege(b) {
  loadBboxLayer(b, {
    url: '/api/schlaege', key: 'fields', tiles: G.schlagTiles, ids: G.schlagIds, attempts: (loadSchlaege._a = loadSchlaege._a || {}),
    idOf: it => it.id,
    onFeature: (props, geometry, data) => {
      const r = biggestRing(geometry) || [];
      let w = Infinity, s = Infinity, e = -Infinity, n = -Infinity;
      for (const c of r) { if (c[0] < w) w = c[0]; if (c[0] > e) e = c[0]; if (c[1] < s) s = c[1]; if (c[1] > n) n = c[1]; }
      G.schlaege.push({ type: 'Feature', properties: props, geometry, bbox: [w, s, e, n] });
    },
    done: (n, data) => { G.schlagGen++; if (data.year) G.schlagYear = data.year; render(); },
  });
}

// ---- Viewport tiling / retry ----
// Upstream warms parcel+footprint geometry per KG lazily (a cold KG is fetched
// from Zenodo, ~2s) and caps rows per request. Both show up in the response as
// `ready:false` / `truncated:true`. If we ignore them the map just silently
// stops filling in — exactly what happened when panning across a KG border.
// So: tile the viewport, retry not-ready tiles, subdivide truncated ones.

let _vpBusy = 0;
function vpBusy(delta) {
  _vpBusy = Math.max(0, _vpBusy + delta);
  const el = document.getElementById('map-loading');
  if (el) { el.style.display = _vpBusy > 0 ? '' : 'none'; if (_vpBusy > 0) updateMapLoadingText(); }
}

/** Split a bbox into tiles of at most maxSpan degrees, nearest-to-camera first,
 *  capped so a fully zoomed-out view can't fan out into dozens of requests. */
function tileBox(b, maxSpan, maxTiles) {
  const nx = Math.max(1, Math.ceil((b.e - b.w) / maxSpan));
  const ny = Math.max(1, Math.ceil((b.n - b.s) / (maxSpan * 0.72)));
  const dx = (b.e - b.w) / nx, dy = (b.n - b.s) / ny;
  const tiles = [];
  for (let i = 0; i < nx; i++) for (let j = 0; j < ny; j++) {
    const t = { w: b.w + i*dx, e: b.w + (i+1)*dx, s: b.s + j*dy, n: b.s + (j+1)*dy };
    t._d = Math.hypot((t.w+t.e)/2 - G.cam.lon, ((t.s+t.n)/2 - G.cam.lat) / 0.72);
    tiles.push(t);
  }
  tiles.sort((a,z) => a._d - z._d);
  return tiles.slice(0, maxTiles || 12);
}

const _vpRetries = new Map(); // tileKey -> attempts already made

/** Load one tile; if upstream is still warming, retry with backoff (the first
 *  call is what triggers warming upstream, so retries converge). If the tile
 *  was truncated, subdivide it once into quarters. */
async function loadTileResilient(t, depth) {
  depth = depth || 0;
  const key = [t.w,t.s,t.e,t.n].map(x=>x.toFixed(4)).join(',');
  let res = await loadViewportGeometry(t);
  if (res.cached) return 0;
  let added = res.added;
  if (added > 0) { render(); renderMini(); }

  if (!res.ready) {
    const tries = (_vpRetries.get(key) || 0);
    // Upstream keeps downloading the cold KG between polls, so retries
    // converge; a slow Zenodo mirror can take a while though, hence up to 8
    // paced attempts. Pace = upstream's retry_after_s (server already waited
    // ~7s itself), else a growing backoff.
    if (tries < 8) {
      _vpRetries.set(key, tries + 1);
      const wait = res.retryAfter > 0 ? Math.min(res.retryAfter, 15) * 1000 : 1500 * (tries + 1);
      setTimeout(() => {
        // Only retry while the tile is still (roughly) on screen.
        const v = viewBounds();
        if (t.e < v.w - 0.02 || t.w > v.e + 0.02 || t.n < v.s - 0.02 || t.s > v.n + 0.02) return;
        loadTileResilient(t, depth).then(a => { if (a > 0) { buildEZIndex(); loadEnhancedForKGs(); } });
      }, wait);
    }
  } else {
    _vpRetries.delete(key);
  }

  if (res.truncated && depth < 2) {
    const mx = (t.w+t.e)/2, my = (t.s+t.n)/2;
    const quarters = [
      {w:t.w,s:t.s,e:mx,n:my}, {w:mx,s:t.s,e:t.e,n:my},
      {w:t.w,s:my,e:mx,n:t.n}, {w:mx,s:my,e:t.e,n:t.n},
    ];
    for (const qt of quarters) added += await loadTileResilient(qt, depth+1);
  }
  return added;
}

/** Run loaders with bounded concurrency (upstream sees no benefit past ~8). */
async function runPool(items, worker, conc) {
  let i = 0, total = 0;
  const runners = Array.from({length: Math.min(conc||4, items.length)}, async () => {
    while (i < items.length) total += await worker(items[i++]);
  });
  await Promise.all(runners);
  return total;
}

async function fetchKGPolygonsBlocking() {
  // NEW fast path: instead of loading whole KGs' export/geojson (multi-MB each),
  // pull polygon geometry for just the viewport (plus a margin) from the upstream
  // R-tree via /api/viewport (~100ms, ~40KB gzip). One round-trip gets both
  // parcels and building footprints for everything on screen.
  setLoadSub('Geometrien für den sichtbaren Bereich werden geladen...');
  setLoadProgress(35);
  // The canvas isn't sized yet during loading, so derive the box from the camera
  // directly (roughly one screen at the start zoom). ~0.007° ≈ the initial view.
  const rad = 0.007;
  const box = { w: G.cam.lon - rad, s: G.cam.lat - rad*0.72, e: G.cam.lon + rad, n: G.cam.lat + rad*0.72 };
  const added = (await loadViewportGeometry(box, { force: true })).added;
  setLoadProgress(70);
  setLoadSub(`${G.parcelPolys.length} Parzellen, ${G.buildingFootprints.length} Gebäude geladen`);

  // Kick enhanced (lidar/OSM/N2K) fetches for any enhanced KGs now on screen.
  loadEnhancedForKGs();

  // Background: widen ~2.5× so panning outward is already primed.
  const wide = { w: G.cam.lon - rad*2.5, s: G.cam.lat - rad*1.8, e: G.cam.lon + rad*2.5, n: G.cam.lat + rad*1.8 };
  runPool(tileBox(wide, 0.02, 9), t => loadTileResilient(t), 4).then(a => {
    if (a > 0) { buildEZIndex(); loadEnhancedForKGs(); render(); renderMini(); }
  }).catch(()=>{});
}

/** Drop tiles that lie entirely outside Austria — upstream has no cadastre data
 *  there, so those requests are pure latency (and made panning near the border
 *  feel stuck). Conservative: keeps a tile if ANY corner/centre is inside, and
 *  keeps everything until the outline has loaded. */
function tilesInAustria(tiles) {
  if (!G.atBorder) return tiles;
  const keep = tiles.filter(t => {
    const pts = [[t.w,t.s],[t.e,t.s],[t.w,t.n],[t.e,t.n],[(t.w+t.e)/2,(t.s+t.n)/2]];
    return pts.some(p => insideAustria(p[0], p[1]));
  });
  return keep;
}

async function fetchKGPolygons() {
  // Incremental viewport load on pan/zoom. Splits the (padded) current view into
  // ≤0.02° tiles — one request per tile, nearest-to-camera first — and each tile
  // retries while upstream warms / subdivides if it was truncated. Tiling matters
  // at low zoom: a single huge bbox blows past the row limit and comes back
  // `truncated`, which is what made panning look "stuck".
  const b = viewBounds();
  const padX = (b.e - b.w) * 0.25, padY = (b.n - b.s) * 0.25;
  const box = { w: b.w - padX, s: b.s - padY, e: b.e + padX, n: b.n + padY };
  const added = await runPool(tilesInAustria(tileBox(box, 0.02, 12)), t => loadTileResilient(t), 4);
  if (added > 0) { render(); renderMini(); }
  // Enhanced data for any newly-visible enhanced KGs.
  loadEnhancedForKGs();
  // Refresh the badge: it must reappear when panning back into an already-
  // loaded enhanced KG (loadEnhancedForKGs skips those, so it won't re-fire).
  updateEnhancedBadge();
  updateWaterChip();
}

// ================= ENHANCED MODE (lidar terrain, OSM lines, Natura 2000) =================

/** Fetch the enhanced-KG registry (lidar-processed KGs). Cached server-side 15min; refreshed client-side every 10min. */
async function loadEnhancedRegistry() {
  try {
    const res = await GET('/api/enhanced-kgs');
    if (!res || !res.kgs) return;
    G.enhancedKGs = new Set(res.kgs.map(k => k.kg_code));
    const byGem = {};
    G.v2KGs = new Set(res.kgs.filter(k => k.v2).map(k => k.kg_code));
    for (const k of res.kgs) {
      if (!byGem[k.gemeinde_code]) byGem[k.gemeinde_code] = { gemeinde_code: k.gemeinde_code, gemeinde_name: k.gemeinde_name, lon: k.lon, lat: k.lat, v2: false };
      if (k.v2) byGem[k.gemeinde_code].v2 = true;
    }
    G.enhancedGemeinden = Object.values(byGem);
  } catch(e) { console.error('enhanced registry failed:', e); }
}
setInterval(loadEnhancedRegistry, 10*60*1000);

/** Kick off background enhanced-data fetches for loaded KGs that are lidar-processed. Never blocks. */
function loadEnhancedForKGs() {
  for (const kg of G.kgsLoaded) loadWaterForKG(kg);
  for (const kg of G.kgsLoaded) {
    if (G.enhancedLoaded.has(kg)) continue;
    if (!G.enhancedKGs.has(kg)) continue;
    G.enhancedLoaded.add(kg);
    fetchEnhancedKG(kg); // fire & forget
  }
}

/**
 * OSM water for a KG (cadastre feedback #16). BEV NS symbols are counts, not
 * areas, so riverbed parcels (Danube at Dürnstein: 70 ha, water_fraction 0.885)
 * often carry no GW code at all and were painted as land. Upstream now serves
 *  - cat=water_parcels: OSM water ∩ parcel polygons with water_fraction per parcel
 *    (drives terrain fill, landuse name and price via extractLuCode)
 *  - cat=water_area: closed, fillable river/lake (Multi)Polygons (backdrop)
 * Runs for every loaded KG, enhanced or not.
 */
function loadWaterForKG(kg) {
  if (!kg || G.waterKGs.has(kg)) return;
  G.waterKGs.add(kg);
  const fetchParcels = (attempt) => GET('/api/cadastre/osm/geometry?kg='+kg+'&cat=water_parcels&min_fraction=0.05').then(d => {
    let n = 0;
    for (const f of (d.features || [])) {
      const pr = f.properties || {};
      if (!pr.parcel_id || pr.water_fraction == null) continue;
      G.waterParcels[pr.parcel_id] = { fraction: pr.water_fraction, sqm: pr.water_sqm || 0, fclass: pr.fclass || [], name: pr.name || '' };
      n++;
    }
    if (n) { G.lidarGen++; render(); }
    // complete=false → a KG's parcel geometry was still cold upstream; retry.
    if (d.meta?.water_parcels && d.meta.water_parcels.complete === false && attempt < 3) {
      setTimeout(() => fetchParcels(attempt + 1), 4000 * (attempt + 1));
    }
  }).catch(e => console.error('water_parcels failed:', kg, e));
  fetchParcels(0);

  GET('/api/cadastre/osm/geometry?kg='+kg+'&cat=water_area').then(d => {
    const areas = [];
    for (const f of (d.features || [])) {
      if (!f.geometry || !isAreaGeom(f.geometry)) continue;
      areas.push({ geometry: f.geometry, fclass: f.properties?.fclass || 'water' });
    }
    G.waterAreas[kg] = areas;
    if (areas.length) render();
  }).catch(e => console.error('water_area failed:', kg, e));
}

/** Water share of a parcel from OSM (0..1), or null if unknown. */
function waterFraction(pid) {
  const w = G.waterParcels[pid];
  return w ? w.fraction : null;
}

async function fetchEnhancedKG(kg) {
  // 1. LiDAR slim KG data (terrain, buildings, top trees/objects — flags already applied server-side)
  GET('/api/lidar/kg/'+kg).then(d => {
    if (d && d.pending) { // lidar product still warming upstream — try again later
      setTimeout(() => fetchEnhancedKG(kg), Math.min(Math.max(d.retry_after_s || 10, 5), 60) * 1000);
      return;
    }
    if (!d || d.error) return;
    if (d.terrain) G.lidarKGTerrain[kg] = { emin: d.terrain.elevation_min_m, emax: d.terrain.elevation_max_m, tclass: d.terrain.terrain_class, product: d.product_version || 'v1' };
    for (const p of (d.parcels||[])) {
      G.lidarParcels[p.parcel_id] = {
        elev: p.elevation_m, elevMin: p.elevation_min_m, elevMax: p.elevation_max_m,
        slope: p.slope_mean_deg, aspect: p.aspect_dominant, tclass: p.terrain_class,
        dom: p.dominant_type, domTerrain: p.dom_terrain, forestFrac: p.forested_fraction,
        fracs: p.fracs, kg: kg, treeH: p.tree_h || null,
      };
    }
    for (const b of (d.buildings||[])) {
      const key = lidarGridKey(b.lon, b.lat);
      if (!G.lidarBuildingIdx[key]) G.lidarBuildingIdx[key] = [];
      G.lidarBuildingIdx[key].push(b);
    }
    G.topTrees[kg] = (d.top_trees||[]);
    G.topObjects[kg] = (d.top_objects||[]);
    G.lidarGen++;
    render();
    updateEnhancedBadge();
  }).catch(e => console.error('lidar kg failed:', kg, e));

  // 2. OSM roads/rail lines (water areas/parcels come via loadWaterForKG)
  GET('/api/cadastre/osm/geometry?kg='+kg+'&cat=road,water,rail').then(d => {
    const feats = d.features || d.data?.features || [];
    const lines = [];
    for (const f of feats) {
      if (!f.geometry || f.geometry.type !== 'LineString') continue;
      const coords = f.geometry.coordinates;
      const pts = new Float64Array(coords.length * 2);
      for (let i = 0; i < coords.length; i++) { pts[i*2] = coords[i][0]; pts[i*2+1] = coords[i][1]; }
      const pr = f.properties || {};
      lines.push({ cat: pr.cat, fclass: pr.fclass, major: !!pr.major, name: pr.name, pts });
    }
    G.osmLines[kg] = lines;
    render();
  }).catch(e => console.error('osm geometry failed:', kg, e));

  // 3. Natura 2000 sites for this KG
  GET('/api/cadastre/natura2000/kg/'+kg).then(async d => {
    const sites = d.data?.inside_sites || [];
    for (const st of sites) {
      if (G.n2kSites[st.sitecode]) continue;
      G.n2kSites[st.sitecode] = { name: st.sitename, habitats: st.habitats||[], label: st.site_type_label, geom: null };
      try {
        const g = await GET('/api/cadastre/natura2000/site/'+st.sitecode+'?geometry=1');
        const geom = g.data?.geometry || g.geometry;
        if (geom) { G.n2kSites[st.sitecode].geom = geom; render(); }
      } catch(e) { console.error('n2k geometry failed:', st.sitecode, e); }
    }
  }).catch(e => console.error('n2k failed:', kg, e));
}

function lidarGridKey(lon, lat) { return Math.round(lon*2000) + ':' + Math.round(lat*2000); }

/** Tall (lidar landmark) trees inside a parcel polygon. Returns {count, maxH}. */
function tallTreesInParcel(f) {
  const rings = geomOuterRings(f && f.geometry);
  if (!rings.length) return {count:0, maxH:0};
  let count = 0, maxH = 0;
  const b = f._bb || (f._bb = geoBounds(f.geometry));
  for (const t of allTallTrees()) {
    if (t.lon < b.w || t.lon > b.e || t.lat < b.s || t.lat > b.n) continue;
    if (pipRings(t.lon, t.lat, rings)) { count++; if (t.height_m > maxH) maxH = t.height_m; }
  }
  return {count, maxH};
}

/** Find lidar building info near a footprint centroid (~50m grid + neighbors). */
function findLidarBuilding(lon, lat) {
  const gx = Math.round(lon*2000), gy = Math.round(lat*2000);
  let best = null, bestD = 4e-7; // ~ (2e-4 deg)^2 ≈ 20m
  for (let dx = -1; dx <= 1; dx++) for (let dy = -1; dy <= 1; dy++) {
    const arr = G.lidarBuildingIdx[(gx+dx)+':'+(gy+dy)];
    if (!arr) continue;
    for (const b of arr) {
      const d = (b.lon-lon)*(b.lon-lon) + (b.lat-lat)*(b.lat-lat);
      if (d < bestD) { bestD = d; best = b; }
    }
  }
  return best;
}

/** Lazily fetch land price estimate for a parcel; updates popup row when it arrives. */
async function fetchLandPrice(pid) {
  if (pid in G.landPrices) return G.landPrices[pid];
  try {
    const d = await GET('/api/cadastre/land_prices/parcel/'+encodeURIComponent(pid));
    G.landPrices[pid] = (d && !d.error && d.buy_total_eur != null) ? d : null;
  } catch(e) { G.landPrices[pid] = null; }
  return G.landPrices[pid];
}

/** Lazily fetch OSM proximity data (roads, transit, water, settlement) for a parcel. */
async function fetchOsmProx(pid) {
  if (pid in G.osmProx) return G.osmProx[pid];
  G.osmProx[pid] = null; // in-flight guard
  try {
    const d = await GET('/api/cadastre/osm/parcel/' + encodeURIComponent(pid));
    G.osmProx[pid] = (d && !d.error && d.osm) ? d.osm : null;
  } catch(e) { G.osmProx[pid] = null; }
  return G.osmProx[pid];
}

function fmtDist(m) {
  if (m == null) return null;
  return m < 1000 ? Math.round(m) + ' m' : (m/1000).toFixed(1).replace('.', ',') + ' km';
}

// ---- Land-cover composition (srtm fracs), corrected for road/roof bleed ----
const FRAC_LABEL = {
  grass:'Wiese', tree:'Bäume', roof:'Gebäude', crop:'Acker', water:'Wasser',
  road:'Straße', path:'Weg', parking:'Parkplatz', shrub:'Gestrüpp', hedge:'Hecke',
  garden:'Garten', vineyard:'Weingarten', bare_soil:'Offen', rock:'Fels',
  fill:'Schüttung', excavation:'Aushub', construction:'Baustelle', tree_loss:'Rodung',
};
const FRAC_COLOR = {
  grass:'#5a9e3a', tree:'#1e5a1e', roof:'#c8b040', crop:'#a8a040', water:'#2878b8',
  road:'#484848', path:'#6a6658', parking:'#525252', shrub:'#6b8e4a', hedge:'#4e7a3a',
  garden:'#739650', vineyard:'#7b5ea0', bare_soil:'#5a5848', rock:'#8a8878',
  fill:'#625e50', excavation:'#4a4838', construction:'#8a6a4a', tree_loss:'#7a5a38',
};

/**
 * Correct srtm's 1m land-cover fractions using cadastre ground truth:
 * - `roof` is capped at the cadastre built-up ratio (building_area/area); srtm
 *   roof pixels bleed across parcel borders at 1m resolution. No buildings on
 *   record → roof dropped entirely.
 * - `road`/`parking`/`path` are only trusted when the cadastre landuse actually
 *   contains a Verkehr entry; otherwise capped at 5% (adjacent-street bleed).
 * Remaining fractions are renormalized to sum to 1.
 */
function correctedFracs(fracs, p) {
  if (!fracs) return null;
  const out = {};
  for (const [t, f] of Object.entries(fracs)) out[t] = f;
  const parsed = parseLanduseSummary(p.landuse_summary);
  const area = p.area_sqm || 0;
  const barea = p.total_building_area_sqm || 0;
  // Buildings on record? Trust cadastre count, footprint area, OR the
  // landuse summary ("Baufläche (X2)") — point-data props may be missing
  // on polygon parcels.
  const hasBldg = (p.building_count > 0) || barea > 0 || (parsed.buildingCount > 0);
  if (out.roof != null) {
    if (!hasBldg) delete out.roof;
    else if (barea > 0 && area > 0) {
      const builtRatio = Math.min(1, barea / area);
      if (out.roof > builtRatio + 0.05) out.roof = Math.round((builtRatio + 0.05) * 100) / 100;
    }
    // hasBldg but unknown footprint area → keep srtm's roof fraction as-is
  }
  const hasRoadLU = (parsed.entries || []).some(e => NS_TRAFFIC.has(e.code));
  if (!hasRoadLU) {
    let imperv = (out.road || 0) + (out.parking || 0) + (out.path || 0);
    if (imperv > 0.05) {
      const k = 0.05 / imperv;
      for (const t of ['road', 'parking', 'path']) {
        if (out[t] != null) {
          out[t] = Math.round(out[t] * k * 100) / 100;
          if (out[t] < 0.02) delete out[t];
        }
      }
    }
  }
  const sum = Object.values(out).reduce((s, f) => s + f, 0);
  if (sum <= 0.05) return null; // correction ate everything — don't show garbage
  for (const t in out) out[t] = out[t] / sum;
  return out;
}

/**
 * Lidar-measured vegetation breakdown for a parcel, or null when no lidar
 * coverage exists. Distinguishes tall canopy (`tree`) from low woody scrub
 * (`shrub` = shrub + hedge). `wood` is the combined cover used for density.
 * Prefers the corrected srtm land-cover `fracs`; falls back to `forestFrac`
 * (canopy only) when composition is unavailable.
 *   → { tree, shrub, wood }  (each 0..1)  |  null
 */
function parcelVeg(f) {
  const lp = G.lidarParcels[f.properties.parcel_id];
  if (!lp) return null;
  const cf = correctedFracs(lp.fracs, f.properties);
  if (cf) {
    const tree = Math.min(1, cf.tree || 0);
    const shrub = Math.min(1, (cf.shrub || 0) + (cf.hedge || 0));
    const wood = Math.min(1, tree + shrub);
    if (wood > 0) return { tree, shrub, wood };
    if (lp.forestFrac == null) return { tree: 0, shrub: 0, wood: 0 };
  }
  if (lp.forestFrac != null) {
    const t = Math.min(1, Math.max(0, lp.forestFrac));
    return { tree: t, shrub: 0, wood: t };
  }
  return null;
}

/** Compact stacked pixel bar + top-3 legend for a fracs vector. */
function fracsBarHTML(fracs) {
  const entries = Object.entries(fracs).filter(([,f]) => f >= 0.02).sort((a,b) => b[1]-a[1]);
  if (entries.length === 0) return '';
  let seg = '';
  for (const [t, f] of entries) {
    seg += '<i style="width:' + (f*100).toFixed(1) + '%;background:' + (FRAC_COLOR[t]||'#888') + '"></i>';
  }
  let leg = entries.slice(0, 3).map(([t, f]) =>
    '<em><i style="background:' + (FRAC_COLOR[t]||'#888') + '"></i>' + (FRAC_LABEL[t]||t) + ' ' + Math.round(f*100) + '%</em>').join('');
  if (entries.length > 3) leg += '<em style="color:var(--text-dim)">+' + (entries.length-3) + '</em>';
  return '<div class="fracs-bar">' + seg + '</div><div class="fracs-legend">' + leg + '</div>';
}

/** Is the camera currently over an enhanced (lidar-processed) KG? */
/** KG code the camera center currently sits in (via loaded parcel polygons),
 *  or null if the center isn't inside any loaded parcel. */
function kgAtCamera() {
  const lon = G.cam.lon, lat = G.cam.lat;
  let nearestKG = null, nd = Infinity;
  for (const f of G.parcelPolys) {
    const g = f.geometry;
    if (!g) continue;
    const b = geoBounds(g);
    if (lon < b.w || lon > b.e || lat < b.s || lat > b.n) {
      // Track nearest parcel centroid as a fallback for sparse coverage.
      const cx = (b.w + b.e) / 2, cy = (b.s + b.n) / 2;
      const d = (cx - lon) * (cx - lon) + (cy - lat) * (cy - lat);
      if (d < nd && f.properties.kg_code) { nd = d; nearestKG = f.properties.kg_code; }
      continue;
    }
    if (pipGeom(lon, lat, g)) return f.properties.kg_code || null;
  }
  // Only trust the nearest-parcel fallback when it's genuinely close (~120m).
  return nd < 1.2e-6 ? nearestKG : null;
}

function camOverEnhancedKG() {
  const kg = kgAtCamera();
  return !!(kg && G.enhancedKGs.has(kg) && G.lidarKGTerrain[kg]);
}

function updateEnhancedBadge() {
  const el = document.getElementById('enhanced-badge');
  if (!el) return;
  const onEnh = camOverEnhancedKG() && insideAustria(G.cam.lon, G.cam.lat);
  el.style.display = onEnh ? '' : 'none';
  if (onEnh) Herald.hint('enhanced');
  if (G.enhancedKGs.size && G._questEnh !== enhancedLoaded()) { G._questEnh = enhancedLoaded(); renderQuests(); }
  // Entdeckermodus unlocked: tree icon signals "tap = fly to nearest giant tree"
  let txt = G.devTree ? '✨ Enhanced Gelände 🌲' : '✨ Enhanced Gelände';
  if (G.tallUnlocked && G.tallRevealed) {
    const c = giantChronik();
    if (c.total) txt += ' · 🌲 ' + c.seen + '/' + c.total;
  }
  el.textContent = txt;
}

async function loadClaimed() { G.claimed = await GET('/api/session/'+G.session.id+'/parcels') || []; updateParcelCount(); }
async function loadOffers() { try { G.offers = await GET('/api/session/'+G.session.id+'/offers') || []; } catch(e) { G.offers = []; } }

/** Build EZ index from loaded parcel polygons — groups parcels by kg_code + ez */
function buildEZIndex() {
  G.ezIndex = {};
  for (const f of G.parcelPolys) {
    const p = f.properties;
    const ez = p.ez;
    if (!ez) continue;
    const key = p.kg_code + '-EZ' + ez;
    if (!G.ezIndex[key]) G.ezIndex[key] = [];
    G.ezIndex[key].push(f);
  }
  // Also index from point parcels (fallback)
  for (const f of G.parcels) {
    const p = f.properties;
    const ez = p.ez;
    if (!ez) continue;
    const key = p.kg_code + '-EZ' + ez;
    if (!G.ezIndex[key]) G.ezIndex[key] = [];
    // Only add if not already in polys
    const ids = new Set(G.ezIndex[key].map(pf => pf.properties.parcel_id));
    if (!ids.has(p.parcel_id)) G.ezIndex[key].push(f);
  }
}
async function loadTreasures() { G.treasures = await GET('/api/session/'+G.session.id+'/treasures') || []; }
async function updateStatsFromServer() { try { const p = await GET('/api/player/'+G.player.id); if (p && !p.error) { G.player = Object.assign(G.player, p); updateStats(); } } catch(e) {} }
async function loadChallenges() { G.challenges = await GET('/api/session/'+G.session.id+'/challenges?player_id='+G.player.id) || []; renderQuests(); }
async function loadPlayers() { G.players = await GET('/api/session/'+G.session.id+'/players') || []; renderPlayerList(); }
async function loadBio() {
  const b = await GET('/api/session/'+G.session.id+'/biodiversity');
  if (!b) return;
  const pct = b.percent||0;
  document.getElementById('bio-bar').style.width = Math.min(pct,100)+'%';
  document.getElementById('bio-label').textContent = pct.toFixed(1)+'% / 30%';
}
async function loadChat() {
  const msgs = await GET('/api/session/'+G.session.id+'/chat?limit=50&player_id='+encodeURIComponent(G.player.id)) || [];
  G.chatMsgs = msgs.reverse ? msgs.reverse() : msgs;
  renderChat();
  loadChatSafety();
}

function updateStats() {
  if (!G.player) return;
  document.getElementById('s-name').textContent = G.player.name;
  document.getElementById('s-coins').textContent = G.player.coins;
  document.getElementById('s-xp').textContent = G.player.xp;
  document.getElementById('s-level').textContent = Math.floor(G.player.xp/200)+1;
  // Mobile toggle stats
  const stc = document.getElementById('st-coins');
  if (stc) stc.textContent = G.player.coins;
  const stx = document.getElementById('st-xp');
  if (stx) stx.textContent = G.player.xp;
}
function updateParcelCount() {
  const mine = G.claimed.filter(c=>c.player_id===G.player.id);
  document.getElementById('s-parcels').textContent = mine.length;
  const stp = document.getElementById('st-parcels');
  if (stp) stp.textContent = mine.length;
}
function renderPlayerList() {
  document.getElementById('game-players').innerHTML = G.players.map((p,i) => {
    if (!G.pcolors[p.id]) G.pcolors[p.id] = PLAYER_COLORS[G.pci++ % PLAYER_COLORS.length];
    return `<div class="stat"><span style="color:${G.pcolors[p.id]}">■</span> ${esc(p.name)}${p.id===G.player.id?' (du)':''}<b>${p.coins}🪙</b></div>`;
  }).join('');
}
const QUEST_ICONS = {explore:'🗺️',restore:'🌿',treasure:'💎',species:'🦎',tree:'🌲',harvest:'🌾',timber:'🪓'};
/** Any lidar-enhanced KG among the loaded ones? (giant trees only exist there) */
function enhancedLoaded() {
  for (const kg of G.kgsLoaded) if (G.enhancedKGs.has(kg)) return true;
  return false;
}
/** Open quests the player can actually pursue here (Baumriese needs an enhanced KG). */
function visibleQuests() {
  const enh = enhancedLoaded();
  return (G.challenges||[]).filter(c => c.challenge_type !== 'tree' || enh);
}
function renderQuests() {
  document.getElementById('quest-list').innerHTML = visibleQuests().map(c => {
    const icon = QUEST_ICONS[c.challenge_type]||'📜';
    const goal = c.goal || 1, have = Math.min(c.progress || 0, goal);
    const bar = goal > 1 ? `<div class="qp"><i style="width:${Math.round(have/goal*100)}%"></i><span>${have}/${goal}</span></div>` : '';
    const active = Herald.activeQuestId === c.id ? ' active' : '';
    return `<div class="quest-item${active}" role="button" tabindex="0" data-qid="${c.id}" title="${tr('Antippen für Details')}"
        onclick="Herald.brief(${c.id})" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();Herald.brief(${c.id})}">
      <div class="qt">${icon} ${esc(c.title)}</div>
      <div class="qd">${esc(c.description||'')}</div>
      ${bar}<div class="qr">+${c.reward_coins}🪙 +${c.reward_xp}⚡<span class="qgo">▸</span></div></div>`;
  }).join('') || '<div style="font:16px VT323;color:var(--text-dim)">Alle erledigt!</div>';
  Herald.questsChanged();
}

// ---- Quest briefings: what to do, where, and a one-tap action that moves the
// game along (fly to the nearest treasure, open an owned parcel to convert, …).
/** Owned, still-unconverted parcels of the player, nearest to camera first. */
/** O(1) parcel polygon lookup; index rebuilt lazily when G.parcelPolys grows. */
let _polyIdx = null, _polyIdxN = -1;
function polyById(id) {
  if (!_polyIdx || _polyIdxN !== G.parcelPolys.length) {
    _polyIdx = {}; for (const f of G.parcelPolys) _polyIdx[f.properties.parcel_id] = f;
    _polyIdxN = G.parcelPolys.length;
  }
  return _polyIdx[id] || null;
}
function myUnconvertedClaims() {
  return (G.claimed||[]).filter(c => c.player_id === G.player?.id && !c.converted_to)
    .map(c => { const f = G.parcelPolys.find(p => p.properties.parcel_id === c.parcel_id); const ll = f ? featureLonLat(f) : null; return {c, f, ll}; })
    .filter(o => o.ll).sort((a,b) => geoDist(a.ll, [G.cam.lon,G.cam.lat]) - geoDist(b.ll, [G.cam.lon,G.cam.lat]));
}
function geoDist(a, b) { return Math.hypot((a[0]-b[0]) * Math.cos(b[1]*Math.PI/180), a[1]-b[1]) * 111000; }
function nearestTreasure(pred) {
  const cam = [G.cam.lon, G.cam.lat];
  return (G.treasures||[]).filter(t => !t.found_by && (!pred || pred(t)))
    .map(t => ({t, d: geoDist([t.lon,t.lat], cam)})).sort((a,b) => a.d-b.d)[0] || null;
}
function fmtDist(m) { return m >= 1000 ? (m/1000).toFixed(1).replace('.',',') + ' km' : Math.round(m) + ' m'; }
/** Fly + pulse a map marker so the eye lands on the target (see drawQuestPing). */
function questPing(lon, lat, zoom) {
  G.questPing = { lon, lat, t0: performance.now() };
  flyTo(lon, lat, zoom || Math.max(G.cam.zoom, 17));
  const tick = () => { if (!G.questPing) return; render(); if (performance.now() - G.questPing.t0 < 3200) requestAnimationFrame(tick); else { G.questPing = null; render(); } };
  requestAnimationFrame(tick);
}
function drawQuestPing(ctx) {
  const p = G.questPing; if (!p) return;
  const [x, y] = toScreen(p.lon, p.lat);
  const age = (performance.now() - p.t0) / 1000;
  const R = Math.max(60, Math.min(gc.width, gc.height) * 0.16); // radius scales with screen
  ctx.save();
  for (let k = 0; k < 3; k++) {
    const ph = ((age * 0.9) + k / 3) % 1;
    const r = 10 + ph * R;
    ctx.globalAlpha = (1 - ph) * 0.95;
    ctx.lineWidth = 6; ctx.strokeStyle = 'rgba(0,0,0,.55)'; ctx.setLineDash([]);
    ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI*2); ctx.stroke();
    ctx.lineWidth = 3; ctx.strokeStyle = '#ffd23f'; ctx.setLineDash([8, 5]);
    ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI*2); ctx.stroke();
  }
  ctx.globalAlpha = 1; ctx.setLineDash([]);
  const bob = Math.abs(Math.sin(age * 5)) * 8;
  ctx.font = '28px serif'; ctx.textAlign = 'center'; ctx.textBaseline = 'alphabetic';
  ctx.fillText('📍', x, y - 26 - bob);
  ctx.restore();
}

/** Build the briefing lines + action for a quest. */
/** My unconverted crop fields with their stage + position. */
function myFields() {
  const out = [];
  for (const c of G.claimed || []) {
    if (c.player_id !== G.player?.id || c.converted_to || c.landuse !== '48') continue;
    const f = polyById(c.parcel_id);
    if (!f) continue;
    out.push({c, f, ll: featureLonLat(f), fs: fieldStage(f.properties, c)});
  }
  return out;
}
function questBriefing(c) {
  const t = c.title, goal = c.goal || 1, have = Math.min(c.progress || 0, goal);
  const rw = `<span class="rw">+${c.reward_coins}🪙 +${c.reward_xp}⚡</span>`;
  const left = goal - have;
  const prog = goal > 1 ? `\n${tr('Fortschritt')}: <b>${have}/${goal}</b>` : '';
  const brief = { lines: [], act: null };
  const L = (icon, tag, html) => brief.lines.push({ icon, tag, html });
  const icon = QUEST_ICONS[c.challenge_type] || '📜';
  L(icon, tr('Aufgabe'), `<b>${esc(tr(t))}</b>\n${esc(tr(c.description||''))}  ${rw}${prog}`);

  const mine = myUnconvertedClaims();
  if (t === 'Erkunde deine Gemeinde' || t === 'Landvermesser') {
    const coins = (G.player?.coins ?? 0).toLocaleString('de-AT');
    L('🏴', tr('So geht’s'), tr('Tipp auf eine Parzelle am Kartenrand — Wiesen und Wald sind billig, Bauland teuer.') + ` <b>${coins}🪙</b> ` + tr('hast du im Börserl.') + (left > 1 ? `\n${tr('Noch')} <b>${left}</b> ${tr('Parzellen fehlen.')}` : ''));
    // Cheapest unclaimed parcel on screen → jump there.
    const owned = new Set((G.claimed||[]).map(x => x.parcel_id));
    const cheap = DEV.parcelsNear(p => !owned.has(p.parcel_id) && (p.area_sqm||0) > 500 && !(p.building_count > 0), 80)
      .map(p => ({p, price: calcPrice(p.area_sqm||0, extractLuCode('', p), p.building_count||0, p.total_building_area_sqm||0)}))
      .sort((a,b) => a.price - b.price)[0];
    if (cheap) brief.act = { label: `${tr('Günstige Parzelle zeigen')} · ${cheap.price}🪙`, run: async () => { const f = DEV.find(cheap.p.parcel_id); if (!f) return; const [lon, lat] = featureLonLat(f); questPing(lon, lat, Math.max(G.cam.zoom, 17.5)); setTimeout(() => showParcelPopup(f), 850); } };
  } else if (t === 'Schatzsucher') {
    const n = nearestTreasure();
    L('💎', tr('Wo?'), n ? tr('Schatzkisten liegen offen auf der Karte — die nächste ist') + ` <b>${fmtDist(n.d)}</b> ` + tr('entfernt. Zoom hin und tipp sie an.') : tr('Hier liegt gerade kein Schatz. Fahr ein Stück weiter — jede Gemeinde hat welche.'));
    if (n) brief.act = { label: tr('Zum Schatz fliegen') + ` · ${fmtDist(n.d)}`, run: () => questPing(n.t.lon, n.t.lat, 17.5) };
  } else if (t === 'Artenforscher') {
    const n = nearestTreasure(x => x.treasure_type === 'species' || x.treasure_type === 'n2k_species');
    L('🦎', tr('Wo?'), n ? tr('Seltene Arten verstecken sich als 🦎-Marker, oft in Natura-2000-Gebieten (🛡️). Die nächste ist') + ` <b>${fmtDist(n.d)}</b> ` + tr('entfernt.') : tr('Hier ist gerade keine Art bekannt. Schalte 🛡️ Natura 2000 ein und such in Schutzgebieten.'));
    if (n) brief.act = { label: tr('Zur Art fliegen') + ` · ${fmtDist(n.d)}`, run: () => questPing(n.t.lon, n.t.lat, 17.5) };
    else brief.act = { label: tr('Natura 2000 einblenden'), run: () => { DEV.n2k(true); } };
  } else if (t === 'Naturschützer' || t === 'Waldmeister') {
    if (mine.length) {
      L('🌿', tr('So geht’s'), tr('Öffne eine Parzelle, die dir gehört, und tipp auf') + ' <b>🌿 ' + tr('Naturschutz') + '</b>. ' + (mine.length === 1 ? tr('Eine Parzelle wartet schon auf dich.') : tr('Du hast') + ` <b>${mine.length}</b> ` + tr('Parzellen, die noch warten.')) + (left > 1 ? `\n${tr('Noch')} <b>${left}</b> ${tr('Umwandlungen fehlen.')}` : ''));
      brief.act = { label: tr('Meine Parzelle öffnen'), run: () => { const o = mine[0]; questPing(o.ll[0], o.ll[1], Math.max(G.cam.zoom, 17.5)); setTimeout(() => showParcelPopup(o.f), 850); } };
    } else {
      L('🌿', tr('So geht’s'), tr('Dafür brauchst du zuerst Land: Kauf eine Parzelle, öffne sie dann noch einmal und wandle sie um.'));
    }
  } else if (t === 'Erntedank') {
    const fields = myFields();
    const ripe = fields.filter(x => x.fs.stage === 'ripe');
    const soon = fields.filter(x => x.fs.stage !== 'ripe' && x.fs.stage !== 'meadow').sort((a,b) => a.fs.ripeInS - b.fs.ripeInS)[0];
    if (ripe.length) {
      L('🌾', tr('Jetzt!'), tr('Ein Acker von dir ist reif — die 🌾-Marker zeigen ihn. Tipp drauf und ernte, bevor die Bauern es tun.') + (left > 1 ? `\n${tr('Noch')} <b>${left}</b> ${tr('Ernten fehlen.')}` : ''));
      brief.act = { label: tr('Zum reifen Acker'), run: () => { const o = ripe[0]; questPing(o.ll[0], o.ll[1], Math.max(G.cam.zoom, 17)); setTimeout(() => showParcelPopup(o.f), 850); } };
    } else if (soon) {
      L('🌱', tr('Geduld'), tr('Äcker reifen alle 40 Minuten, jeder zu seiner Zeit. Dein nächster ist in') + ` <b>${fmtMin(soon.fs.ripeInS)}</b> ` + tr('reif — dann erscheint ein 🌾-Marker.'));
      brief.act = { label: tr('Zum Acker'), run: () => questPing(soon.ll[0], soon.ll[1], Math.max(G.cam.zoom, 17)) };
    } else {
      L('🌾', tr('So geht’s'), tr('Kauf dir einen Acker (Nutzung „Äcker/Wiesen/Weiden“). Goldene Felder sind gerade reif — ein Kauf zur Erntezeit zahlt sich sofort aus.'));
      const owned = new Set((G.claimed||[]).map(x => x.parcel_id));
      const f = DEV.parcelsNear(p => !owned.has(p.parcel_id) && isCropField(p) && fieldKindFor(p, simpleHash(p.parcel_id)) !== 3 && fieldStage(p, null).stage === 'ripe' && (p.area_sqm||0) > 800, 60)[0];
      if (f) brief.act = { label: tr('Reifen Acker zeigen'), run: async () => { const ff = DEV.find(f.parcel_id); if (!ff) return; const [lon, lat] = featureLonLat(ff); questPing(lon, lat, Math.max(G.cam.zoom, 17)); setTimeout(() => showParcelPopup(ff), 850); } };
    }
  } else if (t === 'Holzknecht' || t === 'Waldhüter') {
    const forests = myForests();
    const ready = forests.filter(x => x.fs.stage === 'baumholz');
    const growing = forests.filter(x => x.fs.stage !== 'baumholz').sort((a,b) => a.fs.readyInS - b.fs.readyInS)[0];
    const verb = t === 'Holzknecht' ? '🪓 ' + tr('Holzernte') : '🌳 ' + tr('Naturwald');
    if (ready.length) {
      L(t === 'Holzknecht' ? '🪓' : '🌳', tr('Jetzt!'), tr('Öffne eine deiner Waldparzellen und tipp auf') + ` <b>${verb}</b>. ` + (t === 'Holzknecht' ? tr('Der Erlös richtet sich nach dem echten Holzvorrat und den aktuellen Holzpreisen.') : tr('Der Wald bleibt dann für immer außer Nutzung — je älter der Bestand, desto mehr ⚡.')) + (left > 1 ? `\n${tr('Noch')} <b>${left}</b> ${tr('fehlen.')}` : ''));
      brief.act = { label: tr('Zum Wald'), run: () => { const o = ready[0]; questPing(o.ll[0], o.ll[1], Math.max(G.cam.zoom, 17)); setTimeout(() => showParcelPopup(o.f), 850); } };
    } else if (growing) {
      L('🌱', tr('Geduld'), tr('Dein Wald wächst nach: Schlag → Jungwuchs → Stangenholz. Hiebsreif in') + ` <b>${fmtMin(growing.fs.readyInS)}</b>.`);
      brief.act = { label: tr('Zum Wald'), run: () => questPing(growing.ll[0], growing.ll[1], Math.max(G.cam.zoom, 17)) };
    } else {
      L('🌲', tr('So geht’s'), tr('Kauf dir eine Waldparzelle (Nutzung „Wald“, dunkelgrün). Wald ist billig — und steht voller Holz.'));
      const owned = new Set((G.claimed||[]).map(x => x.parcel_id));
      const f = DEV.parcelsNear(p => !owned.has(p.parcel_id) && extractLuCode('', p) === '56' && (p.area_sqm||0) > 1500, 60)[0];
      if (f) brief.act = { label: tr('Wald zeigen'), run: async () => { const ff = DEV.find(f.parcel_id); if (!ff) return; const [lon, lat] = featureLonLat(ff); questPing(lon, lat, Math.max(G.cam.zoom, 17)); setTimeout(() => showParcelPopup(ff), 850); } };
    }
  } else if (t === 'Baumriese') {
    if (!G.tallUnlocked) {
      L('🌲', tr('Versteckt'), tr('Riesenbäume zeigen sich erst, wenn du deinen ersten Schatz gefunden hast.'));
      const n = nearestTreasure();
      if (n) brief.act = { label: tr('Zum Schatz fliegen') + ` · ${fmtDist(n.d)}`, run: () => questPing(n.t.lon, n.t.lat, 17.5) };
    } else {
      const trees = (G.tallRevealed ? allTallTrees() : hintTallTrees(12)).map(x => ({x, d: geoDist([x.lon,x.lat],[G.cam.lon,G.cam.lat])})).sort((a,b) => a.d-b.d);
      const n = trees[0];
      L('🌲', tr('Wo?'), n ? (G.tallRevealed ? tr('Kauf die Parzelle, auf der ein Riesenbaum steht. Der nächste') : tr('Goldene Bäume zeigen dir Riesen. Der nächste')) + ` (<b>${Math.round(n.x.height_m)} m</b>) ` + tr('ist') + ` <b>${fmtDist(n.d)}</b> ` + tr('entfernt.') : tr('In dieser Gegend sind noch keine Riesenbäume geladen — fahr ins ✨ Enhanced Gelände.'));
      if (n) brief.act = { label: tr('Zum Baum fliegen') + ` · ${fmtDist(n.d)}`, run: () => questPing(n.x.lon, n.x.lat, 17.5) };
    }
  }
  return brief;
}
function renderChat() {
  const el = document.getElementById('chat-log');
  const blocked = G.blocked || new Set();
  el.innerHTML = G.chatMsgs.filter(m => !m.player_id || !blocked.has(m.player_id)).map(m => {
    if (m.hidden) return `<div class="chat-msg hidden-msg">🚫 Nachricht entfernt</div>`;
    const mine = m.player_id && G.player && m.player_id === G.player.id;
    const act = (!mine && m.player_id) ? `<span class="cm-act">
        <button title="Melden" onclick="openReport(${m.id||0},'${m.player_id}','${esc(m.player_name||'')}')">⚑</button>
        <button title="Blockieren" onclick="blockPlayer('${m.player_id}','${esc(m.player_name||'')}')">🚫</button></span>` : '';
    return `<div class="chat-msg" data-id="${m.id||0}" onclick="this.classList.toggle('touch')"><span class="cn">${esc(m.player_name||'?')}:</span> ${esc(m.message)}${act}</div>`;
  }).join('');
  el.scrollTop = el.scrollHeight;
}

// ---- Chat safety (rules gate, quick phrases, report, block, session mode) ----
async function loadChatSafety() {
  if (!G.chatRules) G.chatRules = await GET('/api/chat/rules') || {rules:[], quick_phrases:[]};
  if (G.player && !G.blocked) {
    const bl = await GET('/api/player/'+G.player.id+'/blocks') || [];
    G.blocked = new Set(Array.isArray(bl) ? bl.map(b=>b.blocked_id) : []);
  }
  applyChatMode(G.session && G.session.chat_mode || 'free');
  const sel = document.getElementById('chat-mode');
  if (G.session && G.session.created_by === G.player.id) {
    sel.style.display = '';
    sel.value = G.session.chat_mode || 'free';
    sel.onchange = async () => {
      const r = await POST('/api/session/'+G.session.id+'/chat-mode', {player_id:G.player.id, mode:sel.value});
      if (r.error) { toast(r.error,'err'); sel.value = G.session.chat_mode||'free'; return; }
      toast(tr('Chat-Modus geändert'),'ok');
    };
  } else sel.style.display = 'none';
  const q = document.getElementById('chat-quick');
  q.innerHTML = (G.chatRules.quick_phrases||[]).map((ph,i) => `<button onclick="sendQuick(${i+1})">${esc(ph)}</button>`).join('');
  renderChat();
}
function applyChatMode(mode) {
  if (G.session) G.session.chat_mode = mode;
  const row = document.getElementById('chat-input-row'), q = document.getElementById('chat-quick'), off = document.getElementById('chat-off-note');
  row.style.display = mode === 'free' ? '' : 'none';
  off.style.display = mode === 'off' ? '' : 'none';
  q.style.display = mode === 'quick' ? '' : (G.quickOpen ? '' : 'none');
  document.getElementById('btn-chat-quick').style.display = mode === 'free' ? '' : 'none';
}
document.getElementById('btn-chat-quick').onclick = (e) => {
  G.quickOpen = !G.quickOpen;
  document.getElementById('chat-quick').style.display = G.quickOpen ? '' : 'none';
  e.currentTarget.classList.toggle('on', G.quickOpen);
  e.currentTarget.setAttribute('aria-pressed', String(G.quickOpen));
};
document.getElementById('btn-chat-safety').onclick = () => showChatRules(false);
function showChatRules(gate) {
  const m = document.getElementById('chat-rules-modal');
  document.getElementById('chat-rules-list').innerHTML = ((G.chatRules||{}).rules||[]).map(r=>`<li>${esc(r)}</li>`).join('');
  document.getElementById('btn-rules-accept').style.display = (gate || !(G.player||{}).chat_rules_accepted) ? '' : 'none';
  m.style.display = '';
}
document.getElementById('btn-rules-close').onclick = () => document.getElementById('chat-rules-modal').style.display = 'none';
document.getElementById('btn-rules-accept').onclick = async () => {
  await POST('/api/chat/accept-rules', {player_id:G.player.id});
  G.player.chat_rules_accepted = 1;
  document.getElementById('chat-rules-modal').style.display = 'none';
  if (G.pendingChat) { const p = G.pendingChat; G.pendingChat = null; postChat(p); }
};
async function postChat(body) {
  body.player_id = G.player.id;
  const r = await POST('/api/session/'+G.session.id+'/chat', body);
  if (r && r.error === 'rules_required') { G.pendingChat = body; showChatRules(true); return; }
  if (r && r.error) { toast(r.error, 'err'); if (body.message && !body.quick) document.getElementById('input-chat').value = body.message; }
}
window.sendQuick = i => postChat({quick:i});
window.blockPlayer = async (pid, name) => {
  if (!confirm(tr('Spieler blockieren? Du siehst dann keine Nachrichten mehr von dieser Person.'))) return;
  await POST('/api/block', {player_id:G.player.id, target_id:pid});
  (G.blocked = G.blocked || new Set()).add(pid);
  renderChat(); toast('🚫 '+name+' '+tr('blockiert'),'ok');
};
const REPORT_REASONS = [['harassment','Beleidigung / Belästigung'],['hate','Hassrede'],['sexual','Sexuelle Inhalte'],['grooming','Fragt nach Alter, Fotos, Treffen oder Kontakt'],['personal','Teilt persönliche Daten'],['spam','Spam / Werbung'],['other','Sonstiges']];
window.openReport = (msgId, pid, name) => {
  G.reportCtx = {message_id:msgId, target_id:pid};
  document.getElementById('report-target').textContent = tr('Spieler: ')+name;
  document.getElementById('report-reasons').innerHTML = REPORT_REASONS.map(([k,l]) => `<label><input type="radio" name="rr" value="${k}"> ${l}</label>`).join('');
  document.getElementById('report-note').value = '';
  document.getElementById('btn-report-send').disabled = true;
  document.querySelectorAll('#report-reasons input').forEach(i => i.onchange = () => document.getElementById('btn-report-send').disabled = false);
  document.getElementById('report-modal').style.display = '';
};
document.getElementById('btn-report-cancel').onclick = () => document.getElementById('report-modal').style.display = 'none';
document.getElementById('btn-report-send').onclick = async () => {
  const reason = (document.querySelector('#report-reasons input:checked')||{}).value; if (!reason) return;
  const r = await POST('/api/report', Object.assign({player_id:G.player.id, session_id:G.session.id, reason, note:document.getElementById('report-note').value}, G.reportCtx));
  document.getElementById('report-modal').style.display = 'none';
  if (r.error) { toast(r.error,'err'); return; }
  (G.blocked = G.blocked || new Set()).add(G.reportCtx.target_id);
  G.chatMsgs.forEach(m => { if (m.id === G.reportCtx.message_id) m.hidden = 1; });
  renderChat(); toast(tr('⚑ Danke für deine Meldung. Der Spieler wurde für dich blockiert.'),'ok');
};

window.tryCompleteQuest = async function(id) {
  const res = await POST('/api/complete-challenge', {player_id:G.player.id, challenge_id:id});
  if (res.error) { toast(res.error,'err'); return; }
  toast('✅ +'+res.coins+'🪙 +'+res.xp+'⚡','ok');
  G.player = res.player; updateStats(); loadChallenges();
};

// Chat
document.getElementById('btn-chat').onclick = sendChat;
document.getElementById('input-chat').addEventListener('keydown', e => { if(e.key==='Enter') sendChat(); });
async function sendChat() {
  const inp = document.getElementById('input-chat');
  const msg = inp.value.trim(); if (!msg||!G.session) return;
  inp.value = '';
  postChat({message:msg});
}

// ---- SSE ----
function connectSSE() {
  if (G.sse) G.sse.close();
  G.sse = new EventSource('/api/session/'+G.session.id+'/events');
  G.sse.onmessage = e => {
    try { handleEvent(JSON.parse(e.data)); } catch(err) {}
  };
  G.sse.onerror = () => setTimeout(connectSSE, 5000);
}
function handleEvent(d) {
  switch(d.type) {
    case 'chat': G.chatMsgs.push({id:d.id,player_id:d.player_id,player_name:d.player,message:d.message}); renderChat(); break;
    case 'chat_hidden': G.chatMsgs.forEach(m => { if (m.id === d.id) m.hidden = 1; }); renderChat(); break;
    case 'chat_refresh': loadChat(); break;
    case 'chat_mode': applyChatMode(d.mode); { const sel=document.getElementById('chat-mode'); if (sel) sel.value=d.mode; } toast(tr('Chat-Modus geändert'),''); break;
    case 'player_joined': toast('⚔️ '+d.player.name+' beigetreten!','ok'); loadPlayers(); break;
    case 'parcel_claimed': toast('🏴 '+d.player+' → '+d.parcel_id,''); loadClaimed().then(()=>render()); break;
    case 'parcel_converted': toast('🌿 '+d.player+' → '+d.convert_to,'ok'); loadClaimed().then(()=>{render();loadBio();}); break;
    case 'parcel_sold': toast('💰 '+d.player+' verkauft',''); loadClaimed().then(()=>render()); break;
    case 'parcel_harvested': if (d.player !== G.player?.name) toast((d.forest ? '🪓 ' : '🌾 ')+d.player+' erntet '+d.coins+'🪙',''); loadClaimed().then(()=>render()); break;
    case 'ez_claimed': toast('\u{1f4cb} '+d.player+' → EZ '+d.ez+' ('+d.count+' Parzellen)',''); loadClaimed().then(()=>render()); break;
    case 'challenge_completed':
      if (d.player === G.player?.name) { toast('🏆 '+tr('Aufgabe erledigt')+': '+tr(d.title||'')+'!','ok'); Herald.completed(d.title); loadChallenges(); updateStatsFromServer(); }
      else toast('🏆 '+d.player+': '+tr(d.title||'Aufgabe'),'');
      break;
    case 'treasures_updated': loadTreasures().then(()=>{ render(); toast('🛡️ Seltene Arten in Natura-2000-Gebieten entdeckt!','ok'); }); break;
    case 'offer_made':
      if (d.seller_id === G.player.id) toast('📨 '+d.buyer+' bietet '+d.offer_price+'🪙 für deine Parzelle!','ok');
      loadOffers().then(()=>{ if(G.sel) showParcelPopup(G.sel); });
      break;
    case 'offer_accepted':
      toast('✅ '+d.buyer+' kauft Parzelle von '+d.seller+' für '+d.offer_price+'🪙','ok');
      Promise.all([loadClaimed(), loadOffers()]).then(()=>{render(); if(G.sel) showParcelPopup(G.sel);});
      // Refresh own player data
      if (d.buyer_id === G.player.id || d.seller_id === G.player.id) {
        GET('/api/player/'+G.player.id).then(p=>{if(!p.error){G.player=p;updateStats();}});
      }
      break;
    case 'offer_rejected':
      if (d.buyer_id === G.player.id) toast('❌ Dein Angebot wurde abgelehnt','err');
      loadOffers().then(()=>{ if(G.sel) showParcelPopup(G.sel); });
      break;
    case 'offer_funds_needed':
      if (d.buyer_id === G.player.id) {
        toast('⚠️ Du brauchst '+d.offer_price+'🪙 aber hast nur '+d.buyer_coins+'🪙 — verkaufe Parzellen!','err');
      }
      break;
  }
}

// ================= MAP RENDERING (Settlers IV Style) =================

function viewBounds() {
  const s = mapScale();
  const hw = gc.width / s / 2;
  const hh = gc.height / s / 2 / 1.35;
  return { w:G.cam.lon-hw, e:G.cam.lon+hw, s:G.cam.lat-hh, n:G.cam.lat+hh };
}
function mapScale() { return Math.pow(2, G.cam.zoom - 14) * 25000; }
function toScreen(lon, lat) {
  const s = mapScale();
  return [(lon-G.cam.lon)*s + gc.width/2, (G.cam.lat-lat)*s*1.35 + gc.height/2];
}
function toGeo(x, y) {
  const s = mapScale();
  return [(x-gc.width/2)/s + G.cam.lon, G.cam.lat - (y-gc.height/2)/(s*1.35)];
}


/** Lambert hillshade factor for a parcel from lidar slope/aspect: −1 (shadow) .. +1 (lit).
 *  Fixed sun from NW (azimuth 315°), elevation 45° — the classic cartographic light. */
const ASPECT_DEG = { N:0, NE:45, E:90, SE:135, S:180, SW:225, W:270, NW:315 };
function hillshade(lp) {
  if (!lp || lp.slope == null) return 0;
  const az = ASPECT_DEG[lp.aspect];
  if (az == null) return 0;
  const sl = lp.slope * Math.PI / 180, a = az * Math.PI / 180;
  const sunAz = 315 * Math.PI / 180, sunEl = 45 * Math.PI / 180;
  // cos of angle between surface normal and sun vector
  const cosI = Math.cos(sunEl) * Math.sin(sl) * Math.cos(a - sunAz) + Math.sin(sunEl) * Math.cos(sl);
  const flat = Math.sin(sunEl);
  return Math.max(-1, Math.min(1, (cosI - flat) / flat));
}

// ---- Two-layer rendering. The static "base" (terrain, parcels, roads,
// buildings, border) is drawn into an offscreen canvas and only redrawn when
// the camera/data signature changes or it gets older than `maxAge`; the
// animated overlay (giants, treasures, GPS, highlights) is drawn on top every
// frame. Before this, the 10 fps tree tick redrew thousands of parcel
// polygons on every frame — the main reason slow phones stuttered.
let _base = null, _baseSig = '', _baseAt = 0;
function baseSignature(W, H) {
  let conv = 0; for (const c of G.claimed) if (c.converted_to) conv++;
  return [G.cam.lon.toFixed(7), G.cam.lat.toFixed(7), G.cam.zoom.toFixed(4), W, H,
    G.parcelPolys.length, G.parcels.length, G.buildingFootprints.length, G.landusePolys.length,
    G.claimed.length, conv, G.lidarGen, G.n2kVisible ? 1 : 0, Object.keys(G.n2kSites).length,
    Object.keys(G.osmLines).length, Object.keys(G.waterAreas || {}).length,
    G.atBorder ? 1 : 0, G.baseGen || 0].join('|');
}
function drawBaseLayers(ctx, W, H, claimMap) {
  // ---- Background terrain ----
  ctx.fillStyle = '#3a6828';
  ctx.fillRect(0, 0, W, H);
  drawGrassTexture(ctx, W, H);

  // ---- Foreign territory (outside Austria — no cadastre data exists there) ----
  drawForeignShading(ctx, W, H);

  // ---- Draw real landuse polygons (forests, water, roads, etc.) ----
  if (G.landusePolys.length > 0) drawLandusePolygons(ctx);

  // ---- Natura 2000 protected-area overlay (enhanced mode) ----
  if (G.n2kVisible) drawN2KOverlay(ctx);

  // ---- OSM water areas (rivers/lakes as closed polygons, feedback #16) ----
  drawWaterAreas(ctx);

  // ---- OSM water lines (enhanced mode) ----
  drawOSMLines(ctx, 'water');

  // ---- Draw parcel polygons (from export/geojson KG data) ----
  if (G.parcelPolys.length > 0) {
    for (const f of G.parcelPolys) {
      drawParcelPoly(ctx, f, claimMap);
    }
  }

  // ---- Draw point parcels (if no polygon available) ----
  const polyIds = new Set(G.parcelPolys.map(f=>f.properties.parcel_id));
  for (const f of G.parcels) {
    if (!polyIds.has(f.properties.parcel_id)) {
      drawParcelPoint(ctx, f, claimMap);
    }
  }

  // ---- OSM roads + rail on top of parcels (enhanced mode) ----
  drawOSMLines(ctx, 'road');
  drawOSMLines(ctx, 'rail');

  // ---- Landuse sprites (crops, flowers, reeds, vines) ----
  drawLanduseSprites(ctx, claimMap);

  // ---- Trees on forest parcels ----
  drawForestSprites(ctx, claimMap);

  // ---- Draw real building footprints ----
  if (G.buildingFootprints.length > 0) drawBuildingFootprints(ctx);
}

function render() {
  if (!gctx) return;
  const ctx = gctx;
  const W = gc.width, H = gc.height;

  // Build claim lookup
  const claimMap = {};
  for (const c of G.claimed) claimMap[c.parcel_id] = c;

  // ---- Static base layer (cached) ----
  const sig = baseSignature(W, H);
  const now = performance.now();
  const maxAge = 1000;
  if (!_base || _baseSig !== sig || now - _baseAt > maxAge) {
    if (!_base || _base.width !== W || _base.height !== H) {
      _base = document.createElement('canvas'); _base.width = W; _base.height = H;
    }
    drawBaseLayers(_base.getContext('2d'), W, H, claimMap);
    _baseSig = sig; _baseAt = now;
  }
  ctx.drawImage(_base, 0, 0);

  // ---- Living nature reserves (waving grass, herbs, fauna) ----
  drawNatureReserves(ctx, claimMap);
  drawForestOverlay(ctx, claimMap);

  // ---- Tallest-tree + landmark markers (enhanced mode) ----
  drawTopLandmarks(ctx);

  // ---- Official place names (BEV DLM Riednamen, Almen, Gipfel, Bäche…) ----
  drawToponyms(ctx);

  // ---- Similar-parcels overlay (below treasures, above parcels) ----
  if (G.similar) drawSimilarParcels(ctx);

  // ---- Treasures ----
  _treasuresOnScreen = 0;
  for (const t of G.treasures) drawTreasure(ctx, t);
  drawRipeMarkers(ctx, claimMap);
  drawCollectFX(ctx);
  if (G.n2kVisible) drawN2KOverlay(ctx, true);

  // ---- GPS position marker ----
  if (G.geo.watching) drawGeoMarker(ctx);

  // ---- EZ group highlight (all parcels in same EZ) ----
  if (G.ezHighlight) drawEZHighlight(ctx);

  // ---- Selected parcel highlight ----
  if (G.sel) drawSelection(ctx, G.sel);

  // ---- Tapped building highlight ----
  if (G.selFp) drawFpHighlight(ctx, G.selFp);

  // ---- Austrian national border (above map content) ----
  drawAustriaBorderLine(ctx);
  updateAbroadBadge();

  // Scale bar
  drawQuestPing(ctx);
  drawScaleBar(ctx, W, H);
}
/** Force the cached base layer to redraw on the next frame. */
function invalidateBase() { G.baseGen = (G.baseGen || 0) + 1; }

let grassPatternCanvas = null;
function createGrassPattern() {
  // Pre-generate a grass pattern tile (much faster than per-pixel)
  const sz = 128;
  grassPatternCanvas = document.createElement('canvas');
  grassPatternCanvas.width = sz;
  grassPatternCanvas.height = sz;
  const pctx = grassPatternCanvas.getContext('2d');
  pctx.fillStyle = '#3a6828';
  pctx.fillRect(0, 0, sz, sz);
  // Dithered noise patches
  const greens = ['#3a6828','#3e6c2c','#366424','#426e30','#34622a','#3c6a2e','#386626','#407030'];
  for (let i = 0; i < 800; i++) {
    const x = (i * 73 + 37) % sz;
    const y = (i * 137 + 91) % sz;
    pctx.fillStyle = greens[i % greens.length];
    pctx.fillRect(x, y, 2 + (i%3), 2 + (i%2));
  }
  // Tiny grass blades
  pctx.strokeStyle = 'rgba(80,160,50,0.3)';
  pctx.lineWidth = 1;
  for (let i = 0; i < 60; i++) {
    const x = (i * 31 + 13) % sz;
    const y = (i * 97 + 47) % sz;
    pctx.beginPath();
    pctx.moveTo(x, y);
    pctx.lineTo(x + (i%3)-1, y - 3 - (i%4));
    pctx.stroke();
  }
}

function drawGrassTexture(ctx, W, H) {
  if (!grassPatternCanvas) createGrassPattern();
  const pat = ctx.createPattern(grassPatternCanvas, 'repeat');
  ctx.fillStyle = pat;
  ctx.fillRect(0, 0, W, H);
}

// ================= REAL LANDUSE POLYGONS =================
// Map landuse_code (BEV NS, corrected Aug 2026) to fill colors (Settlers-style terrain)
const LANDUSE_POLY_COLORS = {
  '40': {fill:'#8aa84a', stroke:'#7a9840'},          // Dauerkulturen/Erwerbsgärten
  '41': {fill:'#d0b848', stroke:'#b09828'},          // Gebäude — yellow
  '42': {fill:'#505050', stroke:'#404040', a:0.7},   // Parkplatz — dark grey
  '48': {fill:'#7ba83c', stroke:'#6a9830'},          // Äcker/Wiesen/Weiden — farmland green
  '52': {fill:'#6b8e4a', stroke:'#5b7e3a'},          // Gärten
  '53': {fill:'#80aa40', stroke:'#709a30'},          // Weingärten
  '54': {fill:'#6a9a5a', stroke:'#5a8a4a'},          // Alpen
  '55': {fill:'#2a5a2a', stroke:'#1a4a1a'},          // Krummholz
  '56': {fill:'#1e5a1e', stroke:'#145014'},          // Wälder
  '57': {fill:'#5a8a4a', stroke:'#4a7a3a'},          // Verbuschte Flächen
  '58': {fill:'#8a7a58', stroke:'#7a6a48', a:0.8},   // Forststraßen — gravel
  '59': {fill:'#2f7fbf', stroke:'#2f7fbf', a:1},     // Fließende Gewässer (= TERRAIN.water)
  '60': {fill:'#2f7fbf', stroke:'#2f7fbf', a:1},     // Stehende Gewässer
  '61': {fill:'#4a8a6a', stroke:'#3a7a5a'},          // Feuchtgebiete
  '62': {fill:'#9a9888', stroke:'#8a8878'},          // Vegetationsarme Flächen
  '63': {fill:'#a09070', stroke:'#907f60'},          // Betriebsflächen
  '64': {fill:'#5a9a7a', stroke:'#4a8a6a'},          // Gewässerrandflächen
  '65': {fill:'#7a9a5a', stroke:'#6a8a4a'},          // Verkehrsrandflächen
  '72': {fill:'#6f8f6f', stroke:'#5f7f5f'},          // Friedhöfe
  '83': {fill:'#c8b060', stroke:'#a89040'},          // Gebäudenebenflächen
  '84': {fill:'#8a8878', stroke:'#7a7868'},          // Abbau/Halden/Deponien
  '87': {fill:'#9a9888', stroke:'#8a8878'},          // Fels/Geröll
  '88': {fill:'#cfe4f2', stroke:'#b8d4e8'},          // Gletscher
  '92': {fill:'#5a5048', stroke:'#4a4038', a:0.85},  // Schienenverkehr
  '95': {fill:'#484848', stroke:'#383838', a:0.8},   // Straßenverkehr — tarmac
  '96': {fill:'#7aaa4a', stroke:'#6a9a3a'},          // Freizeitflächen
};
const LANDUSE_POLY_DEFAULT = {fill:'#5a8a40', stroke:'#4a7a30'};
// Compact Verkehrsfläche (farmyard/courtyard, not a road): light gravel
const LANDUSE_YARD = {fill:'#b0a488', stroke:'#94886c', a:0.7};

function drawLandusePolygons(ctx) {
  const W = gc.width, H = gc.height;
  for (const f of G.landusePolys) {
    const geom = f.geometry;
    if (!geom) continue;
    const code = f.properties.landuse_code || '';
    let colors = LANDUSE_POLY_COLORS[code] || LANDUSE_POLY_DEFAULT;
    // Straßenverkehrsanlagen (95): distinguish real roads (long, thin) from paved
    // farmyards/courtyards (compact blobs). Compact ones drawn as dark tarmac
    // read like flat gray buildings — render them as light gravel instead.
    if (code === '95') {
      if (f._yard === undefined) {
        const r0 = biggestRing(geom) || [[0, 0]];
        let per = 0;
        const latm = 111320, lonm = latm * Math.cos(r0[0][1] * Math.PI / 180);
        for (let i = 1; i < r0.length; i++) {
          const dx = (r0[i][0] - r0[i-1][0]) * lonm, dy = (r0[i][1] - r0[i-1][1]) * latm;
          per += Math.sqrt(dx*dx + dy*dy);
        }
        const a = f.properties.area_sqm || 0;
        // isoperimetric compactness: circle=1, roads ≈ <0.1, yards ≈ >0.2
        f._yard = a > 0 && per > 0 && (4 * Math.PI * a) / (per * per) > 0.22 && a < 20000;
      }
      if (f._yard) colors = LANDUSE_YARD;
    }
    const rings = geom.type === 'MultiPolygon'
      ? geom.coordinates.flatMap(p => p)
      : geom.coordinates;

    // Project first ring to check visibility
    const outerPts = rings[0].map(c => toScreen(c[0], c[1]));
    let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
    for (const pt of outerPts) {
      if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
      if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
    }
    if (maxX < -20 || minX > W+20 || maxY < -20 || minY > H+20) continue;
    // Skip tiny polygons
    if ((maxX-minX) < 2 && (maxY-minY) < 2) continue;

    ctx.beginPath();
    for (let ri = 0; ri < rings.length; ri++) {
      const pts = ri === 0 ? outerPts : rings[ri].map(c => toScreen(c[0], c[1]));
      for (let i = 0; i < pts.length; i++) {
        i === 0 ? ctx.moveTo(pts[i][0], pts[i][1]) : ctx.lineTo(pts[i][0], pts[i][1]);
      }
      ctx.closePath();
    }
    ctx.fillStyle = colors.fill;
    ctx.globalAlpha = colors.a || 0.55;
    ctx.fill();
    ctx.globalAlpha = 1;

    // Subtle stroke for terrain borders
    if ((maxX-minX) > 5 || (maxY-minY) > 5) {
      ctx.strokeStyle = colors.stroke;
      ctx.lineWidth = code === '48' ? 1 : 0.5;  // Roads get thicker border
      ctx.globalAlpha = code === '48' ? 0.6 : 0.4;
      ctx.stroke();
      ctx.globalAlpha = 1;
    }
  }
}

// ================= ENHANCED MODE RENDERING =================

// OSM line styles keyed by cat
const OSM_ROAD_STYLE = {
  motorway:  { w: 5,   color: '#8a7458', center: '#c8b088' },
  primary:   { w: 4,   color: '#8f7a5c', center: '#cbb489' },
  secondary: { w: 3.2, color: '#93805f', center: null },
  tertiary:  { w: 2.6, color: '#96845f', center: null },
  default:   { w: 1.8, color: '#9a8a62', center: null },
};
function osmRoadStyle(fclass) {
  if (!fclass) return OSM_ROAD_STYLE.default;
  if (fclass.startsWith('motorway') || fclass.startsWith('trunk')) return OSM_ROAD_STYLE.motorway;
  if (fclass.startsWith('primary')) return OSM_ROAD_STYLE.primary;
  if (fclass.startsWith('secondary')) return OSM_ROAD_STYLE.secondary;
  if (fclass.startsWith('tertiary')) return OSM_ROAD_STYLE.tertiary;
  return OSM_ROAD_STYLE.default;
}

/** Draw OSM line features of one category (road|water|rail). Culled + major-only below zoom 15. */
function drawOSMLines(ctx, cat) {
  const zoom = G.cam.zoom;
  if (zoom < 13.5) return;
  const majorsOnly = zoom < 15;
  const W = gc.width, H = gc.height;
  const b = viewBounds();
  const pad = 0.002;
  const west = b.w - pad, east = b.e + pad, south = b.s - pad, north = b.n + pad;
  const zs = Math.min(1.6, Math.max(0.5, (zoom - 13) / 4)); // width scale by zoom

  for (const kg in G.osmLines) {
    for (const ln of G.osmLines[kg]) {
      if (ln.cat !== cat) continue;
      // Riverbank/lake outline chunks duplicate the filled water_area polygons
      // (visible as dark diagonals across the Danube) — only streams/rivers
      // without an area polygon still need the centreline.
      if (cat === 'water' && (ln.fclass === 'riverbank' || ln.fclass === 'water' || ln.fclass === 'reservoir' || ln.fclass === 'wetland') && G.waterAreas[kg]) continue;
      if (majorsOnly && cat === 'road' && !ln.major) continue;
      const pts = ln.pts;
      // quick bbox cull using first/last point
      let vis = false;
      for (let i = 0; i < pts.length; i += 2) {
        if (pts[i] > west && pts[i] < east && pts[i+1] > south && pts[i+1] < north) { vis = true; break; }
      }
      if (!vis) continue;

      ctx.beginPath();
      for (let i = 0; i < pts.length; i += 2) {
        const sp = toScreen(pts[i], pts[i+1]);
        i === 0 ? ctx.moveTo(sp[0], sp[1]) : ctx.lineTo(sp[0], sp[1]);
      }

      if (cat === 'water') {
        const isRiver = ln.fclass === 'river' || ln.fclass === 'canal';
        ctx.strokeStyle = isRiver ? '#3a72b0' : '#4a82ba';
        ctx.lineWidth = (isRiver ? 3.5 : 1.6) * zs;
        ctx.globalAlpha = 0.75;
        ctx.lineCap = 'round'; ctx.lineJoin = 'round';
        ctx.stroke();
        ctx.globalAlpha = 1;
      } else if (cat === 'rail') {
        ctx.strokeStyle = '#4a4038';
        ctx.lineWidth = 2 * zs;
        ctx.globalAlpha = 0.8;
        ctx.stroke();
        // cross ties at high zoom
        if (zoom >= 16) {
          ctx.strokeStyle = '#6a5a48';
          ctx.lineWidth = 1;
          for (let i = 0; i < pts.length - 2; i += 2) {
            const a = toScreen(pts[i], pts[i+1]), c = toScreen(pts[i+2], pts[i+3]);
            const dx = c[0]-a[0], dy = c[1]-a[1];
            const len = Math.sqrt(dx*dx+dy*dy);
            if (len < 8) continue;
            const nx = -dy/len, ny = dx/len;
            const nTies = Math.floor(len / 9);
            for (let t = 1; t <= nTies; t++) {
              const mx = a[0] + dx*t/(nTies+1), my = a[1] + dy*t/(nTies+1);
              ctx.beginPath();
              ctx.moveTo(mx - nx*3, my - ny*3);
              ctx.lineTo(mx + nx*3, my + ny*3);
              ctx.stroke();
            }
          }
        }
        ctx.globalAlpha = 1;
      } else { // road — dirt-brown pixel style
        const st = osmRoadStyle(ln.fclass);
        ctx.strokeStyle = st.color;
        ctx.lineWidth = st.w * zs;
        ctx.globalAlpha = 0.85;
        ctx.lineCap = 'round'; ctx.lineJoin = 'round';
        ctx.stroke();
        if (st.center && zoom >= 15) {
          ctx.strokeStyle = st.center;
          ctx.lineWidth = Math.max(0.8, st.w * zs * 0.25);
          ctx.globalAlpha = 0.7;
          ctx.stroke();
        }
        ctx.globalAlpha = 1;
      }
    }
  }
}

/** Natura 2000 protected-area overlay: green hatched polygons + dashed border. */
/** Fillable OSM river/lake polygons (cat=water_area) — below parcels so land
 *  parcels still cover it, water-dominant parcels are painted TERRAIN.water anyway. */
function drawWaterAreas(ctx) {
  const W = gc.width, H = gc.height;
  const b = viewBounds();
  ctx.save();
  ctx.fillStyle = TERRAIN.water[0];
  for (const kg in G.waterAreas) {
    for (const wa of G.waterAreas[kg]) {
      const rings = geomAllRings(wa.geometry);
      ctx.beginPath();
      let vis = false;
      for (const ring of rings) {
        let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
        for (let i = 0; i < ring.length; i++) {
          const sp = toScreen(ring[i][0], ring[i][1]);
          if (sp[0]<minX) minX=sp[0]; if (sp[0]>maxX) maxX=sp[0];
          if (sp[1]<minY) minY=sp[1]; if (sp[1]>maxY) maxY=sp[1];
          i === 0 ? ctx.moveTo(sp[0], sp[1]) : ctx.lineTo(sp[0], sp[1]);
        }
        ctx.closePath();
        if (!(maxX < 0 || minX > W || maxY < 0 || minY > H)) vis = true;
      }
      if (!vis) continue;
      ctx.fill('evenodd');
    }
  }
  ctx.restore();
}

function drawN2KOverlay(ctx, labelsOnly) {
  const W = gc.width, H = gc.height;
  const placed = []; // sites with a visible part (labels pass)
  for (const code in G.n2kSites) {
    const site = G.n2kSites[code];
    if (!site.geom) continue;
    const polys = site.geom.type === 'MultiPolygon' ? site.geom.coordinates : [site.geom.coordinates];
    let labelPt = null, largest = 0;
    for (const poly of polys) {
      const ring = poly[0];
      const pts = [];
      let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
      for (const c of ring) {
        const sp = toScreen(c[0], c[1]);
        pts.push(sp);
        if (sp[0]<minX) minX=sp[0]; if (sp[0]>maxX) maxX=sp[0];
        if (sp[1]<minY) minY=sp[1]; if (sp[1]>maxY) maxY=sp[1];
      }
      if (maxX < -30 || minX > W+30 || maxY < -30 || minY > H+30) continue;
      if (labelsOnly) {
        const a = (maxX-minX)*(maxY-minY);
        if (a > largest) { largest = a; labelPt = [(Math.max(minX,0)+Math.min(maxX,W))/2, (Math.max(minY,0)+Math.min(maxY,H))/2]; }
        continue;
      }
      ctx.beginPath();
      for (let i = 0; i < pts.length; i++) i===0 ? ctx.moveTo(pts[i][0], pts[i][1]) : ctx.lineTo(pts[i][0], pts[i][1]);
      ctx.closePath();
      ctx.fillStyle = 'rgba(40,180,90,0.12)';
      ctx.fill();
      // Hatch lines (clip to polygon)
      ctx.save();
      ctx.clip();
      ctx.strokeStyle = 'rgba(40,180,90,0.18)';
      ctx.lineWidth = 1;
      const step = 14;
      const x0 = Math.max(minX, -30), x1 = Math.min(maxX, W+30);
      const y0 = Math.max(minY, -30), y1 = Math.min(maxY, H+30);
      ctx.beginPath();
      for (let x = x0 - (y1-y0); x < x1; x += step) {
        ctx.moveTo(x, y1);
        ctx.lineTo(x + (y1-y0), y0);
      }
      ctx.stroke();
      ctx.restore();
      // Dashed border
      ctx.beginPath();
      for (let i = 0; i < pts.length; i++) i===0 ? ctx.moveTo(pts[i][0], pts[i][1]) : ctx.lineTo(pts[i][0], pts[i][1]);
      ctx.closePath();
      ctx.strokeStyle = 'rgba(30,160,80,0.7)';
      ctx.lineWidth = 2;
      ctx.setLineDash([8, 5]);
      ctx.stroke();
      ctx.setLineDash([]);
      const a = (maxX-minX)*(maxY-minY);
      if (a > largest) { largest = a; labelPt = [(Math.max(minX,0)+Math.min(maxX,W))/2, (Math.max(minY,0)+Math.min(maxY,H))/2]; }
    }
    // Labels pass: collect the sites on screen; drawn once as a HUD chip below.
    if (labelsOnly && labelPt && G.cam.zoom >= 15) placed.push(site);
  }
  // One combined Natura-2000 chip (sites nest: Wachau ⊃ Wachau-Jauerling ⊃ …),
  // anchored under the search bar instead of floating in the middle of the map
  // where it looked like a garbled watermark and collided with treasures.
  if (labelsOnly && placed.length) {
    const names = placed.map(st => st.name.slice(0, 32) + (st.name.length > 32 ? '…' : ''));
    const label = '🛡️ Natura 2000 · ' + names.join(' · ');
    ctx.save();
    ctx.globalAlpha = 1; ctx.setLineDash([]);
    ctx.font = MAP_FONT.pixel;
    ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
    const tw = ctx.measureText(label).width;
    const pw = Math.min(tw + 24, W - 20), ph = 24;
    const hudShown = [...(document.getElementById('hud-badges')?.children || [])].some(el => el.style.display !== 'none');
    const px = W / 2, py = hudShown ? 102 : 72; // below search bar / HUD badge row
    ctx.fillStyle = 'rgba(10,40,20,0.82)';
    ctx.strokeStyle = 'rgba(125,255,160,0.85)';
    ctx.lineWidth = 1.5;
    ctx.beginPath(); ctx.roundRect(px - pw/2, py - ph/2, pw, ph, 6); ctx.fill(); ctx.stroke();
    ctx.fillStyle = '#c8ffd8';
    ctx.fillText(label, px, py + 1, pw - 16);
    ctx.restore();
  }
}

// ---- Giant-tree index: cached flat list (tallest first) + 0.01° grid for
// viewport culling. Rebuilt only when lidar data changes (G.lidarGen). v2.2
// products carry ~120 giants per KG, so a dozen loaded KGs = ~1500 trees;
// nothing per frame may touch that whole list.
let _tallIdx = null, _tallIdxGen = -1;
function treeKey(t) { return t._k || (t._k = t.lon.toFixed(5) + ',' + t.lat.toFixed(5)); }
function tallIndex() {
  if (_tallIdx && _tallIdxGen === G.lidarGen) return _tallIdx;
  const all = [];
  // Dedupe by key across KGs (a giant near a KG border can arrive from both).
  const keys = new Set();
  for (const kg in G.topTrees) for (const t of (G.topTrees[kg] || [])) { treeKey(t); t._kg = kg; if (keys.has(t._k)) continue; keys.add(t._k); all.push(t); }
  all.sort((a, b) => b.height_m - a.height_m);
  const cells = new Map();
  for (const t of all) {
    const k = Math.floor(t.lon * 100) + ':' + Math.floor(t.lat * 100);
    let c = cells.get(k); if (!c) cells.set(k, c = []);
    c.push(t);
  }
  _tallIdx = { all, cells, maxH: all.length ? all[0].height_m : 0 };
  _tallIdxGen = G.lidarGen;
  return _tallIdx;
}
/** All loaded tall trees, tallest first. Cached — never mutate. */
function allTallTrees() { return tallIndex().all; }
/** Giant trees inside the (padded) viewport, tallest first. */
function tallTreesInView(padFrac) {
  const b = viewBounds();
  const pw = (b.e - b.w) * (padFrac == null ? 0.08 : padFrac), ph = (b.n - b.s) * (padFrac == null ? 0.15 : padFrac);
  const w = b.w - pw, e = b.e + pw, so = b.s - ph, n = b.n + ph;
  const idx = tallIndex(), out = [];
  const gx0 = Math.floor(w * 100), gx1 = Math.floor(e * 100), gy0 = Math.floor(so * 100), gy1 = Math.floor(n * 100);
  if ((gx1 - gx0 + 1) * (gy1 - gy0 + 1) > 4000) {
    for (const t of idx.all) if (t.lon >= w && t.lon <= e && t.lat >= so && t.lat <= n) out.push(t);
    return out;
  }
  for (let gx = gx0; gx <= gx1; gx++) for (let gy = gy0; gy <= gy1; gy++) {
    const c = idx.cells.get(gx + ':' + gy); if (!c) continue;
    for (const t of c) if (t.lon >= w && t.lon <= e && t.lat >= so && t.lat <= n) out.push(t);
  }
  out.sort((a, b) => b.height_m - a.height_m);
  return out;
}

// ---- Riesen-Chronik (discovery): after the hint tree is tapped, giants are
// NOT all shown at once. Trees the player has seen up close (zoom ≥ 15.5, in
// view, within the draw budget) become "discovered"; zoomed out only
// discovered giants render, so the overview grows with exploration — a
// fog-of-war that also bounds the sprite count on slow phones.
const TALL_SEEN_MAX = 4000;
function tallSeenKey() { return 'siedler.giants.' + (G.session ? G.session.id : 'x'); }
function loadTallSeen() {
  try { G.tallSeen = new Set(JSON.parse(localStorage.getItem(tallSeenKey()) || '[]')); }
  catch (e) { G.tallSeen = new Set(); }
  if (G.tallSeen.size) { G.tallRevealed = true; G.tallRevealAt = 0; }
}
let _seenSaveT = 0;
function saveTallSeen() {
  clearTimeout(_seenSaveT);
  _seenSaveT = setTimeout(() => {
    try { localStorage.setItem(tallSeenKey(), JSON.stringify([...G.tallSeen].slice(-TALL_SEEN_MAX))); } catch (e) {}
  }, 800);
}
let _discPending = 0, _discToastT = 0;
function discoverTrees(list, stagger) {
  let n = 0; const now = Date.now();
  for (const t of list) {
    if (G.tallSeen.has(treeKey(t))) continue;
    G.tallSeen.add(t._k); t._seenAt = now + (stagger ? n * 90 : 0); n++;
  }
  if (n) {
    saveTallSeen(); _discPending += n;
    clearTimeout(_discToastT);
    _discToastT = setTimeout(() => {
      const c = giantChronik();
      toast('🌲 +' + _discPending + ' ' + tr('Riesen entdeckt') + ' · ' + tr('Chronik') + ' ' + c.seen + '/' + c.total, 'ok');
      _discPending = 0;
    }, 1800);
    updateEnhancedBadge();
  }
  return n;
}
/** Discovered / loaded giant counts (loaded KGs only). */
function giantChronik() {
  const all = allTallTrees(), seenKeys = new Set();
  for (const t of all) if (G.tallSeen.has(treeKey(t))) seenKeys.add(t._k);
  return { seen: seenKeys.size, total: all.length };
}

// ---- Miraculous tree names: deterministic per tree (seeded by coordinates),
// so "Flüsternde Wolkenwächterin" is always the same tree.
const TREE_NAME_ADJ = ['Ehrwürdige', 'Flüsternde', 'Uralte', 'Schlafende', 'Erwachte',
  'Singende', 'Träumende', 'Wandernde', 'Leuchtende', 'Verwunschene', 'Erhabene',
  'Stille', 'Donnernde', 'Mondbeschienene', 'Sagenhafte', 'Unbeugsame'];
const TREE_NAME_NOUN = ['Wolkenwächter', 'Himmelsgreifer', 'Sturmhüter', 'Waldkönig',
  'Nebelfürst', 'Wurzelweiser', 'Sternenlauscher', 'Riesenherz', 'Donnerwipfel',
  'Morgengrauen', 'Ahnenbaum', 'Bergflüsterer', 'Lichtfänger', 'Windtänzer',
  'Zeitzeuge', 'Kronenträger'];
function treeHash(t) {
  // Stable integer hash from coordinates
  let h = Math.abs(Math.round(t.lon * 1e6) * 31 + Math.round(t.lat * 1e6) * 17);
  return h >>> 0;
}
function giantTreeName(t) {
  const h = treeHash(t);
  const adj = TREE_NAME_ADJ[h % TREE_NAME_ADJ.length];
  const noun = TREE_NAME_NOUN[Math.floor(h / 97) % TREE_NAME_NOUN.length];
  // Feminine noun endings get feminine article feel via '-in' occasionally
  return adj + 'r ' + noun;
}

/** Elevation (m) at a tree's location: point-in-polygon lookup of the parcel
 * it stands in → lidar parcel elevation; falls back to the KG terrain mean. */
function giantTreeElevation(t) {
  for (const f of G.parcelPolys) {
    if (pipGeom(t.lon, t.lat, f.geometry)) {
      const lp = G.lidarParcels[f.properties.parcel_id];
      if (lp && lp.elev != null) return lp.elev;
      break;
    }
  }
  // Fallback: mean of any KG terrain range containing loaded lidar data
  for (const kg in G.topTrees) {
    if ((G.topTrees[kg]||[]).includes(t)) {
      const kt = G.lidarKGTerrain[kg];
      if (kt && kt.emin != null && kt.emax != null) return (kt.emin + kt.emax) / 2;
    }
  }
  return null;
}

/** Rough age estimate for a giant conifer. Growth slows markedly with
 * altitude (shorter season, harsher climate): lowland spruce/fir manage
 * ~30–40cm/yr when young (≈3.2 yr/m); at 1500m+ it's roughly double. */
function giantTreeAge(t) {
  const h = t.height_m;
  const elev = giantTreeElevation(t);
  // yr/m factor: 3.2 below 600m, rising linearly to ~6.5 at 1800m
  let ypm = 3.2;
  if (elev != null && elev > 600) ypm = 3.2 + Math.min(1, (elev - 600) / 1200) * 3.3;
  const base = Math.round(h * ypm + Math.max(0, h - 35) * 4);
  const lo = Math.round(base * 0.85 / 10) * 10;
  const hi = Math.round(base * 1.25 / 10) * 10;
  return { text: lo + '–' + hi + ' Jahre', elev };
}

/** Show the giant tree info popup with height, age and clickable histogram. */
function showTreePopup(tree) {
  const all = allTallTrees();
  // "Nearby" = trees within ~3km of the tapped one (or all if few)
  const mLon = 111320 * Math.cos(tree.lat * Math.PI/180);
  const withD = all.map(t => ({ t, d: Math.hypot((t.lon-tree.lon)*mLon, (t.lat-tree.lat)*110540) }));
  let nearby = withD.filter(o => o.d < 3000).map(o => o.t);
  if (nearby.length < 8) nearby = all;
  // Rank among nearby (1 = tallest)
  const sorted = nearby.slice().sort((a,b) => b.height_m - a.height_m);
  const rank = sorted.findIndex(t => t === tree) + 1;

  document.querySelector('#tree-popup h3').textContent = (tree.broad ? '🌳 ' : '🌲 ') + giantTreeName(tree);
  document.getElementById('tp-height').textContent = tree.height_m + ' m';
  const age = giantTreeAge(tree);
  document.getElementById('tp-age').textContent = age.text +
    (age.elev != null ? ' (auf ' + Math.round(age.elev) + ' m Seehöhe)' : '');
  document.getElementById('tp-rank').textContent = rank > 0 ? rank + '. von ' + nearby.length + ' Riesen in der Nähe' : '-';
  discoverTrees([tree]);
  const chron = giantChronik();
  const tpc = document.getElementById('tp-chron');
  if (tpc) tpc.textContent = chron.seen + ' ' + tr('von') + ' ' + chron.total + ' ' + tr('entdeckt');

  // Histogram: 2m buckets across the nearby height range
  const hs = nearby.map(t => t.height_m);
  const minH = Math.floor(Math.min(...hs) / 2) * 2;
  const maxH = Math.ceil(Math.max(...hs) / 2) * 2;
  const nb = Math.max(1, Math.min(10, (maxH - minH) / 2));
  const step = (maxH - minH) / nb || 1;
  const buckets = Array.from({length: nb}, () => []);
  for (const t of nearby) {
    let bi = Math.floor((t.height_m - minH) / step);
    if (bi >= nb) bi = nb - 1;
    buckets[bi].push(t);
  }
  const maxCount = Math.max(...buckets.map(b => b.length), 1);
  const hist = document.getElementById('tp-hist');
  hist.innerHTML = '';
  buckets.forEach((b, i) => {
    const bar = document.createElement('div');
    bar.className = 'tp-bar';
    const isCur = b.includes(tree);
    if (isCur) bar.classList.add('tp-cur');
    bar.style.height = Math.max(4, Math.round(b.length / maxCount * 70)) + 'px';
    bar.innerHTML = '<div class="tp-count">' + (b.length || '') + '</div>' +
      '<div class="tp-label">' + Math.round(minH + i*step) + 'm</div>';
    bar.title = b.length + ' Baum/Bäume ' + Math.round(minH+i*step) + '–' + Math.round(minH+(i+1)*step) + 'm';
    bar.onclick = (e) => {
      e.stopPropagation();
      if (!b.length) return;
      // Fly to a tree in this bucket — prefer one that isn't the current tree;
      // repeated taps cycle through the bucket.
      bar._idx = ((bar._idx ?? -1) + 1) % b.length;
      let target = b[bar._idx];
      if (target === tree && b.length > 1) { bar._idx = (bar._idx + 1) % b.length; target = b[bar._idx]; }
      flyTo(target.lon, target.lat, Math.max(G.cam.zoom, 16));
      showTreePopup(target);
      toast('🌲 ' + giantTreeName(target) + ' — ' + target.height_m + ' m', 'ok');
    };
    hist.appendChild(bar);
  });

  document.getElementById('parcel-popup').classList.remove('open');
  document.getElementById('ez-popup').classList.remove('open');
  document.getElementById('tree-popup').classList.add('open');
}

/** Giant trees actually drawn in the last frame: [{t, x, y, hint}]. Drives
 * hit testing, the fog hint and animation-tick gating. */
let _drawnTrees = [];
function anyTallTreeOnScreen() { return _drawnTrees.length > 0; }

/** The single "hint" tree shown after unlock but before reveal — the tallest loaded tree. */
function hintTallTree() {
  let best = null;
  for (const t of allTallTrees()) if (!best || t.height_m > best.height_m) best = t;
  return best;
}

/** The top-N tallest loaded trees, shown as hints before reveal (easier to spot). */
function hintTallTrees(n) {
  return allTallTrees().slice(0, n || 5);
}

/** Height (m) of the tallest loaded giant tree — the reference for relative
 *  sizing when zoomed out. Cached, invalidated when new lidar data arrives. */
let _tallMaxH = 0, _tallMaxHGen = -1;
function tallestTreeHeight() { return tallIndex().maxH; }

// ---- Giant tree pixel-art sprite sheets (pre-rendered sway frames) ----
// Two variants: conifer (fir tiers) and broadleaf (round layered canopy) —
// picked per tree from the lidar crown-area/height ratio (t.broad flag).
const GIANT_FRAMES = 8;
let _giantSprites = null;      // conifer frames
let _giantBroadSprites = null; // broadleaf frames
function giantTreeSprites() {
  if (_giantSprites) return _giantSprites;
  _giantSprites = [];
  const p = 3;                 // pixel unit (chunky retro look)
  const CW = 30, CH = 42;      // sprite size in pixel units
  const cx = CW / 2;
  for (let f = 0; f < GIANT_FRAMES; f++) {
    const c = document.createElement('canvas');
    c.width = CW * p; c.height = CH * p;
    const g = c.getContext('2d');
    const ph = (f / GIANT_FRAMES) * Math.PI * 2;
    const px = (ux, uy, col) => { g.fillStyle = col; g.fillRect(Math.round(ux)*p, Math.round(uy)*p, p, p); };
    // Trunk (bottom 8 units), slight sway at top of trunk
    for (let uy = CH-8; uy < CH; uy++) {
      const sw = Math.sin(ph) * 0.3 * ((CH-uy)/8);
      px(cx-1.5+sw, uy, '#4a2f14'); px(cx-0.5+sw, uy, '#6e4a24');
      px(cx+0.5+sw, uy, '#5a3a1a'); if (uy > CH-4) px(cx+1.5+sw, uy, '#4a2f14');
    }
    // Roots
    px(cx-3, CH-1, '#4a2f14'); px(cx+2, CH-1, '#4a2f14');
    // Fir tiers: 5 stacked triangles, upper tiers sway more
    const tiers = [
      {top: 26, h: 9, wBot: 13},
      {top: 20, h: 8, wBot: 11},
      {top: 14, h: 7, wBot: 9},
      {top: 8,  h: 7, wBot: 7},
      {top: 2,  h: 7, wBot: 5},
    ];
    const dark = '#173f1b', mid = '#245c28', lite = '#38843c', top2 = '#4a9848';
    for (let ti = 0; ti < tiers.length; ti++) {
      const T = tiers[ti];
      const swayAmt = Math.sin(ph) * (0.4 + ti * 0.45); // top tiers sway most
      for (let r = 0; r < T.h; r++) {
        const uy = T.top + r;
        const halfW = 1 + (T.wBot - 2) * (r / (T.h - 1)) / 2;
        const off = swayAmt * (1 - r / T.h);
        for (let ux = Math.round(cx - halfW + off); ux <= Math.round(cx + halfW + off); ux++) {
          const rel = (ux - (cx + off)) / (halfW || 1);
          let col = mid;
          if (rel < -0.45) col = dark;                    // left shade
          else if (rel > 0.5) col = lite;                 // right light
          if (ti >= 3 && rel > 0.2 && r < 2) col = top2;  // sun-kissed tips
          // dither
          if (((ux + uy) & 1) === 0 && rel > -0.2 && rel < 0.4) col = (col === mid ? lite : col);
          px(ux, uy, col);
        }
      }
      // Snow/light sparkle pixel on tier edge (animates across frames)
      const sx = cx + Math.sin(ph + ti * 1.3) * (T.wBot/2 - 1);
      px(sx, T.top + 1 + ((f + ti) % 3), '#bfe8a8');
    }
    // Star pixel at the very top (glints)
    if (f % 4 < 2) px(cx + Math.sin(ph)*1.2, 1, '#ffe98a');
    _giantSprites.push(c);
  }
  return _giantSprites;
}

/** Broadleaf giant: thick trunk + big round layered canopy (oak/beech look). */
function giantBroadTreeSprites() {
  if (_giantBroadSprites) return _giantBroadSprites;
  _giantBroadSprites = [];
  const p = 3;
  const CW = 30, CH = 42;      // same canvas as conifer so draw math matches
  const cx = CW / 2;
  for (let f = 0; f < GIANT_FRAMES; f++) {
    const c = document.createElement('canvas');
    c.width = CW * p; c.height = CH * p;
    const g = c.getContext('2d');
    const ph = (f / GIANT_FRAMES) * Math.PI * 2;
    const px = (ux, uy, col) => { g.fillStyle = col; g.fillRect(Math.round(ux)*p, Math.round(uy)*p, p, p); };
    // Trunk (bottom 12 units), broader than the fir, slight sway
    for (let uy = CH-12; uy < CH; uy++) {
      const sw = Math.sin(ph) * 0.25 * ((CH-uy)/12);
      px(cx-2+sw, uy, '#4a2f14'); px(cx-1+sw, uy, '#6e4a24');
      px(cx+sw, uy, '#7a5530'); px(cx+1+sw, uy, '#5a3a1a');
      if (uy > CH-5) { px(cx-3+sw, uy, '#4a2f14'); px(cx+2+sw, uy, '#4a2f14'); }
    }
    // Roots
    px(cx-4, CH-1, '#4a2f14'); px(cx+3, CH-1, '#4a2f14');
    // Branch forks into the canopy
    px(cx-3, CH-13, '#5a3a1a'); px(cx-4, CH-14, '#4a2f14');
    px(cx+2, CH-13, '#5a3a1a'); px(cx+3, CH-14, '#4a2f14');
    // Canopy: stacked overlapping blobs (ellipse rows), upper rows sway more
    const dark = '#1e4d20', mid = '#2e6b30', lite = '#48924a', top2 = '#63b060';
    const cyTop = 3, cyBot = CH-12;           // canopy vertical span
    const cyMid = (cyTop + cyBot) / 2;
    for (let uy = cyTop; uy <= cyBot; uy++) {
      const v = (uy - cyTop) / (cyBot - cyTop);       // 0 top → 1 bottom
      // Round profile: widest just below middle, lumpy edges
      let halfW = 12.5 * Math.sin(Math.PI * Math.min(1, v * 0.92 + 0.06));
      halfW += Math.sin(uy * 2.1 + ph) * 0.9;         // lumpy, sways
      if (halfW < 1.5) continue;
      const off = Math.sin(ph) * (1 - v) * 1.1;       // top sways most
      for (let ux = Math.round(cx - halfW + off); ux <= Math.round(cx + halfW + off); ux++) {
        const rel = (ux - (cx + off)) / (halfW || 1);
        const vv = (uy - cyMid) / ((cyBot - cyTop) / 2);
        let col = mid;
        // light from upper-right, shade lower-left
        const lum = rel * 0.6 - vv * 0.5;
        if (lum > 0.45) col = lite;
        if (lum > 0.8) col = top2;
        if (lum < -0.45) col = dark;
        // leafy dither (hashed — avoids diagonal stripe artifacts)
        const dh = ((ux*73856093) ^ (uy*19349663) ^ (f*83492791)) >>> 0;
        if ((dh % 7) === 0) col = (col === mid ? lite : (col === dark ? mid : col));
        else if ((dh % 11) === 1) col = (col === lite ? mid : (col === mid ? dark : col));
        // ragged edge: skip some rim pixels
        if (Math.abs(rel) > 0.93 && ((ux + uy * 3) % 3) === 0) continue;
        px(ux, uy, col);
      }
    }
    // Highlight sparkles drifting across the crown
    for (let i = 0; i < 3; i++) {
      const sx = cx + Math.sin(ph + i * 2.1) * 8;
      const sy = cyTop + 4 + ((f + i * 3) % 5) + i * 6;
      px(sx, sy, '#a8d890');
    }
    _giantBroadSprites.push(c);
  }
  return _giantBroadSprites;
}

/** Draw one giant tree; size scales with real measured height. */
/** Screen-space hit box of a giant tree sprite — must mirror drawGiantTree's
 *  scale math so taps anywhere on the visible sprite (incl. canopy) register. */
function giantTreeHitBox(t, zoom) {
  const zs = Math.min(1.6, Math.max(0.7, (zoom - 14) / 3));
  const s = zs * (1.0 + t.height_m / 22);
  // sprite canvas is 90×126 px, drawn at s*0.55, anchored at base (y)
  const dw = 90 * s * 0.55, dh = 126 * s * 0.55;
  return { hw: Math.max(30, dw / 2 + 6), up: Math.max(90, dh + 10), down: 20 };
}

/**
 * tier: 2 = full (aura + label + animation per `animate`), 1 = static aura +
 * label, 0 = sprite + shadow only (cheap filler for dense groves). Records the
 * tree in `_drawnTrees` when it lands on screen. Returns true if drawn.
 */
function drawGiantTree(ctx, t, zoom, sway, pop, isHint, animate, maxH, tier) {
  if (animate === undefined) animate = true;
  if (tier === undefined) tier = 2;
  const [x, y] = toScreen(t.lon, t.lat);
  const W = gc.width, H = gc.height;
  if (x < -80 || x > W+80 || y < -140 || y > H+80) return false;
  _drawnTrees.push({ t, x, y, hint: !!isHint });
  // Much taller than normal trees: height drives the scale (30m → ~2.2x, 55m → ~3.4x)
  const zs = Math.min(1.6, Math.max(0.7, (zoom - 14) / 3));
  let s = zs * (1.0 + t.height_m / 22) * pop;
  // When zoomed out, the zs floor makes every giant look the same big size, so
  // shorter ones overlap/merge into the forest and appear to "disappear". Scale
  // each tree relative to the tallest loaded giant so the biggest stand out and
  // shorter ones stay visible but proportionally smaller. Only kicks in below
  // z15 and never shrinks past 45% so nothing vanishes.
  if (maxH && maxH > 0 && zoom < 15) {
    const rel = Math.max(0.45, Math.min(1, t.height_m / maxH));
    const k = Math.min(1, (15 - zoom) / 2); // 0 at z15 → 1 at z13
    s *= (1 - k) + k * rel;
  }
  // Shadow at base
  ctx.fillStyle = 'rgba(0,0,0,0.22)';
  ctx.beginPath(); ctx.ellipse(x, y+2*s, 9*s, 3.2*s, 0, 0, Math.PI*2); ctx.fill();
  // Per-tree phase so auras/labels don't pulse in sync
  const phase = ((t.lon * 7919 + t.lat * 104729) % 6.283) || 0;
  // When not animating, freeze the clock to a per-tree constant so each sprite
  // still gets a distinct (but static) sway frame / aura level. `phase*1000`
  // spreads them across the animation cycles deterministically.
  const now = animate ? Date.now() : phase * 1000;
  if (tier === 0) {
    // filler tier: no aura
  } else if (isHint) {
    // Hint tree: pulsing golden aura
    const pulse = 0.5 + Math.sin(now/400 + phase) * 0.3;
    ctx.fillStyle = 'rgba(255,215,0,' + (0.18*pulse).toFixed(3) + ')';
    ctx.beginPath(); ctx.arc(x, y - 32*s, 22*s, 0, Math.PI*2); ctx.fill();
    ctx.strokeStyle = 'rgba(255,215,0,' + (0.7*pulse).toFixed(2) + ')';
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.arc(x, y - 32*s, 16*s + pulse*4, 0, Math.PI*2); ctx.stroke();
  } else {
    // Revealed giant tree: animated emerald aura + rotating sweep so they stand out
    const pulse = 0.5 + Math.sin(now/500 + phase) * 0.4;
    ctx.fillStyle = 'rgba(80,255,140,' + (0.12*pulse).toFixed(3) + ')';
    ctx.beginPath(); ctx.arc(x, y - 32*s, 20*s, 0, Math.PI*2); ctx.fill();
    ctx.strokeStyle = 'rgba(120,255,160,' + (0.55*pulse).toFixed(2) + ')';
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.arc(x, y - 32*s, 14*s + pulse*4, 0, Math.PI*2); ctx.stroke();
    // Rotating arc sweep around the canopy
    const a0 = (now/700 + phase) % (Math.PI*2);
    ctx.strokeStyle = 'rgba(255,235,120,0.85)';
    ctx.lineWidth = 2.5;
    ctx.beginPath(); ctx.arc(x, y - 32*s, 17*s, a0, a0 + 1.1); ctx.stroke();
    ctx.beginPath(); ctx.arc(x, y - 32*s, 17*s, a0 + Math.PI, a0 + Math.PI + 1.1); ctx.stroke();
  }
  // Pixel-art sprite, animated sway (per-tree phase offset so the forest ripples)
  // Broad-crowned giants (lidar crown area large relative to height → t.broad)
  // get the round broadleaf sprite; the rest the classic fir.
  const sprites = t.broad ? giantBroadTreeSprites() : giantTreeSprites();
  const frame = Math.floor(now / 130 + phase * GIANT_FRAMES / 6.283) % GIANT_FRAMES;
  const sp = sprites[(frame + GIANT_FRAMES) % GIANT_FRAMES];
  const dw = sp.width * s * 0.55, dh = sp.height * s * 0.55;
  const prevSmooth = ctx.imageSmoothingEnabled;
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(sp, x - dw/2, y - dh + 3*s, dw, dh);
  ctx.imageSmoothingEnabled = prevSmooth;
  // Label — bobbing height number with a soft glow pulse
  const glyph = t.broad ? '🌳' : '🌲';
  const label = isHint ? glyph + ' ???'
    : (zoom >= 17 ? glyph + ' ' + giantTreeName(t) + ' · ' + t.height_m + 'm'
                  : glyph + ' ' + t.height_m + 'm');
  if (tier > 0 && (isHint || zoom >= 15.5)) {
    const bob = Math.sin(now/450 + phase) * 3;
    const lp = 0.7 + Math.sin(now/300 + phase) * 0.3;
    const ly = y - dh + 3*s - 8 + bob;
    ctx.font = MAP_FONT.label;
    ctx.textAlign = 'center';
    ctx.fillStyle = 'rgba(0,0,0,0.65)';
    ctx.fillText(label, x+1, ly+1);
    ctx.fillStyle = isHint
      ? 'rgba(255,215,0,' + lp.toFixed(2) + ')'
      : 'rgba(200,255,176,' + lp.toFixed(2) + ')';
    ctx.fillText(label, x, ly);
    ctx.textAlign = 'left';
  }
  return true;
}

/** Total giant sprites per frame (all tiers). Scales with the animation
 *  budget; phones get ~48, desktops up to 128. Cached. */
let _drawBudget = null;
function giantDrawBudget() {
  if (_drawBudget != null) return _drawBudget;
  let b = giantAnimBudget() * 8;
  try { if (window.matchMedia && matchMedia('(pointer: coarse)').matches) b = Math.min(b, 48); } catch (e) {}
  _drawBudget = Math.max(24, Math.min(128, b));
  return _drawBudget;
}

/**
 * How many giant-tree sprites we can afford to animate at once, adapted to the
 * browser/device: each animated tree redraws a pulsing aura + rotating sweep +
 * per-frame sprite swap, so on weak hardware we cap it and draw the rest static.
 * Cached after first call. Honors prefers-reduced-motion, CPU cores, memory,
 * DPR and a coarse mobile check. Non-animated extras still render (just frozen).
 */
let _animBudget = null;
function giantAnimBudget() {
  if (_animBudget != null) return _animBudget;
  let b = 8; // desktop default
  try {
    if (window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches) {
      _animBudget = 1; return 1; // accessibility: animate only the single tallest
    }
    const cores = navigator.hardwareConcurrency || 4;
    const mem = navigator.deviceMemory || 4;            // GB, Chromium only
    const dpr = window.devicePixelRatio || 1;
    const coarse = window.matchMedia && matchMedia('(pointer: coarse)').matches;
    b = Math.round(cores * 1.5);                         // ~scale with CPU
    if (mem <= 2) b = Math.min(b, 4);
    else if (mem <= 4) b = Math.min(b, 8);
    if (coarse) b = Math.min(b, 6);                      // phones/tablets
    if (dpr >= 3) b = Math.min(b, 6);                    // lots of pixels to push
    b = Math.max(3, Math.min(16, b));
  } catch (e) { b = 8; }
  _animBudget = b;
  return b;
}

// ---- Miracle fog hint: when no giant tree is on screen for a few seconds,
// a swirling golden mist gathers at the screen edge pointing toward the
// nearest one. Tapping the mist flies the camera there.
let fogHintSince = 0;
let fogHintPos = null; // {x, y, lon, lat} for tap handling
function drawTallTreeFogHint(ctx) {
  let trees = G.tallRevealed ? allTallTrees() : hintTallTrees(12);
  if (!trees.length) return;
  if (anyTallTreeOnScreen()) { fogHintSince = 0; fogHintPos = null; return; }
  // Discovery mode: the mist is a scout — it leads to giants NOT yet in the
  // Chronik (falls back to any giant once everything loaded is discovered).
  if (G.tallRevealed) {
    const undiscovered = trees.filter(t => !G.tallSeen.has(treeKey(t)));
    if (undiscovered.length) trees = undiscovered;
  }
  const now = Date.now();
  if (!fogHintSince) { fogHintSince = now; fogHintPos = null; return; }
  const age = now - fogHintSince;
  if (age < 3000) { fogHintPos = null; return; }   // patience: let them explore first
  const fade = Math.min(1, (age - 3000) / 1500);
  const W = gc.width, H = gc.height;
  // Nearest tree to screen center
  let best = null, bd = Infinity;
  for (const t of trees) {
    const [x, y] = toScreen(t.lon, t.lat);
    const d = (x - W/2)*(x - W/2) + (y - H/2)*(y - H/2);
    if (d < bd) { bd = d; best = { t, x, y }; }
  }
  if (!best) return;
  // Clamp direction vector to the screen edge (with margin)
  const dx = best.x - W/2, dy = best.y - H/2;
  const k = Math.min(1,
    (W/2 - 70) / Math.max(Math.abs(dx), 1e-9),
    (H/2 - 90) / Math.max(Math.abs(dy), 1e-9));
  // k<1: target is off screen → mist sits at the edge with a chevron.
  // k=1: an undiscovered giant hides right here → mist gathers on the spot.
  const onSpot = k >= 1;
  const ex = W/2 + dx*k, ey = H/2 + dy*k;
  fogHintPos = { x: ex, y: ey, lon: best.t.lon, lat: best.t.lat };
  // Distance in meters (approx equirectangular)
  const mLon = 111320 * Math.cos(G.cam.lat * Math.PI/180);
  const dm = Math.hypot((best.t.lon - G.cam.lon) * mLon, (best.t.lat - G.cam.lat) * 110540);
  const distTxt = onSpot ? '?' : dm >= 1000 ? (dm/1000).toFixed(1) + ' km' : Math.round(dm/10)*10 + ' m';
  // Swirling mist: 3 layered drifting blobs + sparkle motes
  ctx.save();
  ctx.globalAlpha = fade;
  for (let i = 0; i < 3; i++) {
    const wob = now/900 + i * 2.1;
    const bx = ex + Math.sin(wob) * (8 + i*5);
    const by = ey + Math.cos(wob * 1.3) * (5 + i*3);
    const r = 26 + i*10 + Math.sin(now/600 + i) * 4;
    const grad = ctx.createRadialGradient(bx, by, 0, bx, by, r);
    grad.addColorStop(0, 'rgba(255,230,140,' + (0.28 - i*0.07).toFixed(2) + ')');
    grad.addColorStop(1, 'rgba(255,230,140,0)');
    ctx.fillStyle = grad;
    ctx.beginPath(); ctx.arc(bx, by, r, 0, Math.PI*2); ctx.fill();
  }
  // Sparkle motes orbiting the mist
  for (let i = 0; i < 5; i++) {
    const a = now/700 + i * 1.257;
    const mx = ex + Math.cos(a) * (18 + (i%3)*7);
    const my = ey + Math.sin(a * 1.15) * (12 + (i%2)*6);
    const tw = 0.5 + Math.sin(now/180 + i*2) * 0.5;
    ctx.fillStyle = 'rgba(255,245,190,' + (tw*0.9).toFixed(2) + ')';
    ctx.fillRect(mx-1, my-1, 2, 2);
  }
  // Direction chevron pointing outward + pulsing tree glyph
  const ang = Math.atan2(dy, dx);
  const bob = Math.sin(now/350) * 3;
  ctx.translate(ex, ey);
  ctx.rotate(ang);
  if (!onSpot) {
    ctx.fillStyle = 'rgba(255,215,0,0.95)';
    ctx.beginPath();
    ctx.moveTo(34 + bob, 0); ctx.lineTo(22 + bob, -7); ctx.lineTo(22 + bob, 7);
    ctx.closePath(); ctx.fill();
  }
  ctx.rotate(-ang);
  ctx.font = '20px serif';
  ctx.textAlign = 'center';
  ctx.fillText('\uD83C\uDF32', 0, 7);
  // Distance label
  ctx.font = MAP_FONT.label;
  ctx.fillStyle = 'rgba(0,0,0,0.65)';
  ctx.fillText(distTxt, 1, 25);
  ctx.fillStyle = '#ffd700';
  ctx.fillText(distTxt, 0, 24);
  ctx.textAlign = 'left';
  ctx.restore();
}

/** Big landmark tree sprite with subtle sway + height label; banner for tall objects. */
function drawTopLandmarks(ctx) {
  const zoom = G.cam.zoom;
  const W = gc.width, H = gc.height;
  const sway = Math.sin(Date.now() / 1200) * 1.5;

  // Giant trees: locked until first treasure; then hint trees until tapped;
  // then discovery mode (see tallSeen). Everything below is viewport-culled
  // via the grid index and bounded by giantDrawBudget().
  _drawnTrees = [];
  if (G.tallUnlocked) {
    const maxH = tallestTreeHeight();
    if (!G.tallRevealed) {
      // Same hint set at every zoom level so giants stay visible zoomed out.
      for (const hint of hintTallTrees(zoom < 14 ? 3 : 12)) drawGiantTree(ctx, hint, zoom, sway, 1, true, true, maxH);
    } else {
      const inView = tallTreesInView();
      const drawCap = giantDrawBudget();
      let pool;
      if (zoom >= 15.5) {
        // Up close: every giant in view (tallest first, within budget) is
        // drawn — and thereby discovered.
        pool = inView.length > drawCap ? inView.slice(0, drawCap) : inView;
        discoverTrees(pool, true);
      } else {
        // Zoomed out: only discovered giants, tallest first, capped so a
        // canopy of hundreds never buries treasures/markers.
        const cap = Math.min(drawCap, zoom < 13.5 ? 12 : zoom < 14.5 ? 30 : 80);
        pool = [];
        for (const t of inView) { if (G.tallSeen.has(t._k)) { pool.push(t); if (pool.length >= cap) break; } }
      }
      const budget = giantAnimBudget();
      const now = Date.now();
      // Back-to-front by tier so animated champions sit on top of fillers.
      for (let i = pool.length - 1; i >= 0; i--) {
        const t = pool[i];
        const animate = i < budget;
        const tier = i < budget ? 2 : i < budget * 3 ? 1 : 0;
        // Pop-in when a tree enters the Chronik (staggered by rank on reveal).
        let pop = 1;
        if (t._seenAt) {
          const dt = now - t._seenAt;
          if (dt < 0) continue;
          if (dt < 350) { const k = dt / 350; pop = 0.3 + 0.7 * (1 - (1-k)*(1-k)) * (1 + 0.25*Math.sin(k*Math.PI)); }
          else t._seenAt = 0;
        }
        drawGiantTree(ctx, t, zoom, sway, pop, false, animate, maxH, tier);
      }
      // Overflow chip: more giants in view than we draw at this zoom.
      const hidden = (zoom >= 15.5 ? inView.length : 0) - pool.length;
      if (hidden > 0 && zoom < 17) {
        ctx.save();
        ctx.font = MAP_FONT.label; ctx.textAlign = 'right';
        const txt = '🌲 +' + hidden + ' ' + tr('weitere · näher zoomen');
        ctx.fillStyle = 'rgba(0,0,0,0.55)'; ctx.fillText(txt, W - 11, H - 31);
        ctx.fillStyle = '#c8ffb0'; ctx.fillText(txt, W - 12, H - 32);
        ctx.restore();
      }
    }
    drawTallTreeFogHint(ctx);
  }

  // Dev-mode tree (5-tap badge easter egg): always visible once unlocked,
  // even before treasures/reveal. With GPS active, show distance + bearing
  // from the player's real position at the tree base.
  if (G.devTree && !(G.tallUnlocked && G.tallRevealed)) {
    drawGiantTree(ctx, G.devTree, zoom, sway, 1, false);
  }
  if (G.devTree && G.geo.watching && G.geo.lon) {
    drawGeoDistanceAtTree(ctx, G.devTree);
  } else if (G.geo.watching && G.geo.lon && G.tallUnlocked && zoom >= 15) {
    // Kundschafter: with GPS on, the nearest visible giant tree shows walking
    // distance + compass bearing from the player's real position.
    let best = null, bd = Infinity;
    for (const d0 of _drawnTrees) {
      const t = d0.t;
      const d = Math.hypot((t.lon - G.geo.lon) * 0.66, t.lat - G.geo.lat);
      if (d < bd) { bd = d; best = t; }
    }
    if (best && bd < 0.02) drawGeoDistanceAtTree(ctx, best);
  }

  if (zoom < 14) return;

  if (zoom >= 15.5) {
    for (const kg in G.topObjects) {
      for (const o of G.topObjects[kg]) {
        const [x, y] = toScreen(o.lon, o.lat);
        if (x < -30 || x > W+30 || y < -40 || y > H+30) continue;
        // Banner marker: pole + pennant
        ctx.strokeStyle = '#3a2a18';
        ctx.lineWidth = 2;
        ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x, y - 22); ctx.stroke();
        ctx.fillStyle = '#d8b040';
        ctx.beginPath();
        ctx.moveTo(x, y - 22); ctx.lineTo(x + 14, y - 18); ctx.lineTo(x, y - 14);
        ctx.closePath(); ctx.fill();
        if (zoom >= 16.5) {
          ctx.font = MAP_FONT.small;
          ctx.textAlign = 'center';
          const emoji = o.type === 'roof' ? '🏠' : '🗼';
          ctx.fillStyle = 'rgba(0,0,0,0.6)';
          ctx.fillText(emoji + ' ' + o.height_m + 'm', x+1, y - 26 + 1);
          ctx.fillStyle = '#ffe9a0';
          ctx.fillText(emoji + ' ' + o.height_m + 'm', x, y - 26);
          ctx.textAlign = 'left';
        }
      }
    }
  }
}

/** Distance + compass bearing from the GPS position, drawn at a giant
 * tree's base (dev-mode easter egg helper for finding the tree on foot). */
function drawGeoDistanceAtTree(ctx, t) {
  const [x, y] = toScreen(t.lon, t.lat);
  const W = gc.width, H = gc.height;
  if (x < -100 || x > W+100 || y < -140 || y > H+100) return;
  const mLon = 111320 * Math.cos(t.lat * Math.PI/180);
  const dx = (t.lon - G.geo.lon) * mLon;      // east meters
  const dy = (t.lat - G.geo.lat) * 110540;    // north meters
  const dist = Math.hypot(dx, dy);
  const distTxt = dist >= 1000 ? (dist/1000).toFixed(1) + ' km' : Math.round(dist) + ' m';
  const brg = (Math.atan2(dx, dy) * 180 / Math.PI + 360) % 360; // 0=N, cw
  const dirs = window.LANG === 'en' ? ['N','NE','E','SE','S','SW','W','NW'] : ['N','NO','O','SO','S','SW','W','NW'];
  const dir = dirs[Math.round(brg / 45) % 8];
  const label = '📍 ' + distTxt + ' ' + dir;
  ctx.font = MAP_FONT.label;
  ctx.textAlign = 'center';
  const tw = ctx.measureText(label).width;
  const by = y + 14;
  ctx.fillStyle = 'rgba(10,18,10,0.8)';
  ctx.fillRect(x - tw/2 - 14, by - 11, tw + 28, 16);
  ctx.strokeStyle = '#68d0ff'; ctx.lineWidth = 1;
  ctx.strokeRect(x - tw/2 - 14, by - 11, tw + 28, 16);
  ctx.fillStyle = '#aee6ff';
  ctx.fillText(label, x + 6, by + 2);
  // Little arrow rotated to the walking bearing (player → tree)
  ctx.save();
  ctx.translate(x - tw/2 - 4, by - 3);
  ctx.rotate(brg * Math.PI / 180);
  ctx.fillStyle = '#68d0ff';
  ctx.beginPath();
  ctx.moveTo(0, -6); ctx.lineTo(4, 4); ctx.lineTo(0, 2); ctx.lineTo(-4, 4);
  ctx.closePath(); ctx.fill();
  ctx.restore();
  ctx.textAlign = 'left';
}



/** Kundschafter (scout) GPS marker — pixel-art settler with a red-white-red
 *  pennant standing on the player's real position. The accuracy radius is a
 *  dashed gold "Lagerkreis" instead of a Google-Maps blue disc. */
const SCOUT_SPRITE = [
  '....hhh.....',
  '...hhhhh....',
  '...hssss....',
  '...hs.s.....',
  '....sss.....',
  '..ttbbbtt...',
  '.t.bbbbb.t..',
  '.t.bbbbb.t..',
  '...bbbbb....',
  '...ll.ll....',
  '...ll.ll....',
  '..kk...kk...',
];
const SCOUT_COLORS = { h:'#6b3a1e', s:'#f0c8a0', t:'#7a4a2a', b:'#2e6bb5', l:'#5a3a22', k:'#2a1a10' };
function drawGeoMarker(ctx) {
  const g = G.geo;
  if (!g.lon) return;
  const [x, y] = toScreen(g.lon, g.lat);
  const W = gc.width, H = gc.height;
  if (x < -100 || x > W+100 || y < -100 || y > H+100) return;
  const now = Date.now();
  // Accuracy: dashed gold camp circle, slowly rotating
  const s = mapScale();
  const accPx = (g.acc / 111320) * s * 1.35;
  if (accPx > 8 && accPx < 600) {
    ctx.save();
    ctx.setLineDash([6, 5]);
    ctx.lineDashOffset = -(now / 60) % 11;
    ctx.strokeStyle = 'rgba(255,208,80,0.75)';
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.arc(x, y, accPx, 0, Math.PI*2); ctx.stroke();
    ctx.fillStyle = 'rgba(255,208,80,0.06)'; ctx.fill();
    ctx.restore();
  }
  // Ground shadow
  ctx.fillStyle = 'rgba(0,0,0,0.35)';
  ctx.beginPath(); ctx.ellipse(x, y + 2, 10, 4, 0, 0, Math.PI*2); ctx.fill();
  // Sprite (bobbing)
  const px = G.cam.zoom >= 17 ? 3 : 2;
  const bob = Math.round(Math.sin(now / 260) * 1.5);
  const sw = SCOUT_SPRITE[0].length * px, sh = SCOUT_SPRITE.length * px;
  const ox = Math.round(x - sw / 2), oy = Math.round(y - sh + bob);
  ctx.imageSmoothingEnabled = false;
  for (let r = 0; r < SCOUT_SPRITE.length; r++) {
    const row = SCOUT_SPRITE[r];
    for (let c = 0; c < row.length; c++) {
      const col = SCOUT_COLORS[row[c]];
      if (col) { ctx.fillStyle = col; ctx.fillRect(ox + c * px, oy + r * px, px, px); }
    }
  }
  // Pennant pole (right hand) with waving red-white-red flag
  const poleX = ox + 10 * px, poleTop = oy - 10 * px;
  ctx.fillStyle = '#3a2410';
  ctx.fillRect(poleX, poleTop, px, sh + 6 * px);
  const wave = Math.sin(now / 180);
  const fw = 7 * px, fh = px;
  for (let i = 0; i < 3; i++) {
    ctx.fillStyle = i === 1 ? '#f4f0e8' : '#d0342c';
    const skew = Math.round(wave * (i - 1));
    ctx.fillRect(poleX + px, poleTop + i * fh + skew, fw - i * 0, fh);
  }
  ctx.fillStyle = '#ffd050'; ctx.fillRect(poleX - px/2, poleTop - px, px * 2, px); // gold finial
}

// ================= REAL BUILDING FOOTPRINTS =================
const ROOF_COLORS = [
  {roof:'#a05030', wall:'#7a6a58', border:'#6a3020'},  // Classic red-brown
  {roof:'#8a4828', wall:'#6a5a48', border:'#5a2818'},  // Dark terracotta
  {roof:'#b06040', wall:'#8a7a68', border:'#7a4030'},  // Light terracotta
  {roof:'#907060', wall:'#706050', border:'#605040'},  // Gray-brown
  {roof:'#706868', wall:'#585050', border:'#484040'},  // Slate gray
];

function drawBuildingFootprints(ctx) {
  const W = gc.width, H = gc.height;
  const zoom = G.cam.zoom;
  if (zoom < 15) return;
  const enhanced = camOverEnhancedKG(); // hoisted: per-frame, not per-building

  for (const f of G.buildingFootprints) {
    const geom = f.geometry;
    if (!geom || geom.type !== 'Polygon') continue;

    const coords = geom.coordinates[0];
    const pts = coords.map(c => toScreen(c[0], c[1]));

    // Bounding box visibility check
    let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
    for (const pt of pts) {
      if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
      if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
    }
    if (maxX < -10 || minX > W+10 || maxY < -10 || minY > H+10) continue;
    if ((maxX-minX) < 1.5 && (maxY-minY) < 1.5) continue;

    const bw = maxX - minX;
    const bh = maxY - minY;
    const area = bw * bh;
    // Real-world footprint area (m²) — use this for size-based classification
    // (color, roof type) so decisions are zoom-independent. Screen `area`
    // above is only for pixel-visibility gates & gradient sizing.
    const areaM2 = (f.properties && f.properties.area_sqm) || 100;

    // Pick roof style based on building size & hash
    const hash = Math.round(coords[0][0] * 100000) ^ Math.round(coords[0][1] * 100000);
    const colorIdx = (Math.abs(hash) % ROOF_COLORS.length);
    // Large buildings (>400m²) get industrial/slate colors
    const rc = areaM2 > 400 ? ROOF_COLORS[3 + (Math.abs(hash) % 2)] : ROOF_COLORS[colorIdx % 3];

    // 3D roof offset scales with building size — or with REAL lidar height when available
    let roofOff = Math.max(2, Math.min(8, Math.sqrt(area) * 0.12));
    let lidarB = null;
    if (f._lidarGen !== G.lidarGen) {
      // lazy-match lidar building by centroid, cache on feature
      let cx = 0, cy = 0;
      for (const c of coords) { cx += c[0]; cy += c[1]; }
      cx /= coords.length; cy /= coords.length;
      f._lidar = findLidarBuilding(cx, cy);
      f._lidarGen = G.lidarGen;
    }
    lidarB = f._lidar;
    const zs = Math.max(0.5, Math.min(1.4, (zoom - 14) / 3.5));
    if (lidarB && lidarB.stories_est > 0) {
      roofOff = Math.max(2, Math.min(16, lidarB.stories_est * 3 * zs));
    } else if (enhanced) {
      // Default for unmeasured buildings in enhanced KGs: 1–2 stories by footprint size
      const defStories = areaM2 > 250 ? 2 : 1.5;
      roofOff = Math.max(2, Math.min(16, defStories * 3 * zs));
    }

    // Shadow
    ctx.fillStyle = 'rgba(0,0,0,0.15)';
    ctx.beginPath();
    for (let i=0; i<pts.length; i++) {
      const x = pts[i][0]+roofOff*0.6, y = pts[i][1]+roofOff*0.6;
      i===0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.fill();

    // Wall (side) — extrude only viewer-facing edges. The roof is offset
    // straight up in screen space, so an edge shows a wall iff its outward
    // normal points down-screen (normal.y > 0). Anything else is a back wall
    // that would poke out above the roof (the old "awkward" artifacts on
    // rotated / L-shaped buildings). Outward direction depends on winding.
    let sa = 0; // signed area (screen coords, y down): >0 = clockwise
    for (let i=0; i<pts.length; i++) {
      const j = (i+1) % pts.length;
      sa += pts[i][0]*pts[j][1] - pts[j][0]*pts[i][1];
    }
    const wind = sa > 0 ? 1 : -1;
    ctx.fillStyle = rc.wall;
    ctx.beginPath();
    for (let i=0; i<pts.length; i++) {
      const j = (i+1) % pts.length;
      const ex = pts[j][0]-pts[i][0];
      // Screen coords have y down: for a screen-CW ring (sa>0, wind=1) the
      // interior lies left of each directed edge, so the outward normal is
      // (ey,-ex)... in practice: a bottom (viewer-facing) edge runs right→left
      // (ex<0) on a CW ring. Wall visible iff ex*wind < 0.
      if (ex * wind >= -0.01) continue;
      ctx.moveTo(pts[i][0], pts[i][1]);
      ctx.lineTo(pts[j][0], pts[j][1]);
      ctx.lineTo(pts[j][0], pts[j][1]-roofOff);
      ctx.lineTo(pts[i][0], pts[i][1]-roofOff);
      ctx.closePath();
    }
    ctx.fill();
    // Wall shading: darken slightly for depth
    ctx.fillStyle = 'rgba(0,0,0,0.12)';
    ctx.fill();

    // Roof (top face, offset up)
    ctx.fillStyle = rc.roof;
    ctx.beginPath();
    for (let i=0; i<pts.length; i++) {
      const x = pts[i][0], y = pts[i][1]-roofOff;
      i===0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.fill();

    // Roof highlight (subtle light gradient effect)
    ctx.fillStyle = 'rgba(255,255,255,0.08)';
    ctx.fill();

    // Roof border
    ctx.strokeStyle = rc.border;
    ctx.lineWidth = 0.7;
    ctx.stroke();

    // Roof treatment — houses are never empty boxes. Pitched roofs get two
    // shaded planes split along the OBB long axis (clipped to the roof face,
    // so nothing ever draws outside the footprint — the old free-floating
    // ridge line overshot on merged/terraced blocks). Flat roofs get a
    // lighter top with a parapet inset.
    let roofHint = lidarB && lidarB.roof_type_hint;
    if (!roofHint) roofHint = areaM2 > 600 ? 'flat' : 'pitched';
    const props = f.properties || {};
    const rectangular = props.compactness == null || props.compactness >= 0.55;
    if (zoom >= 15.5 && (bw > 6 || bh > 6)) {
      // Roof top-face path (reused for clipping)
      const roofPath = () => {
        ctx.beginPath();
        for (let i=0; i<pts.length; i++) {
          const x = pts[i][0], y = pts[i][1]-roofOff;
          i===0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
        }
        ctx.closePath();
      };
      if (roofHint === 'pitched' && rectangular) {
        // Ridge axis in screen space from the real OBB orientation (fallback:
        // bbox long axis). Then a sharp two-stop gradient perpendicular to it
        // = sunlit plane / shaded plane, clipped to the roof polygon.
        let ux, uy; // ridge direction (screen)
        const cxs = (minX+maxX)/2, cys = (minY+maxY)/2 - roofOff;
        if (props.orientation_deg != null) {
          const th = props.orientation_deg * Math.PI / 180;
          // compass deg → screen: east = +x, north = -y
          ux = Math.sin(th); uy = -Math.cos(th);
        } else {
          if (bw >= bh) { ux = 1; uy = 0; } else { ux = 0; uy = 1; }
        }
        // perpendicular (roof slope direction)
        const px_ = -uy, py_ = ux;
        const halfSpan = Math.max(4, Math.min(bw, bh) * 0.5);
        const g0x = cxs - px_*halfSpan, g0y = cys - py_*halfSpan;
        const g1x = cxs + px_*halfSpan, g1y = cys + py_*halfSpan;
        // Light from upper-left: pick which side is lit by the slope normal
        const lit = (px_ * -0.6 + py_ * -0.8) > 0;
        const grad = ctx.createLinearGradient(g0x, g0y, g1x, g1y);
        const hi = 'rgba(255,240,210,0.22)', lo = 'rgba(20,10,5,0.22)';
        grad.addColorStop(0,     lit ? hi : lo);
        grad.addColorStop(0.48,  lit ? hi : lo);
        grad.addColorStop(0.5,   'rgba(255,245,220,0.30)'); // ridge glint
        grad.addColorStop(0.52,  lit ? lo : hi);
        grad.addColorStop(1,     lit ? lo : hi);
        ctx.save();
        roofPath();
        ctx.clip();
        ctx.fillStyle = grad;
        ctx.fillRect(minX-2, minY-roofOff-2, bw+4, bh+4);
        // Ridge line, drawn inside the clip so it never escapes the roof
        if (zoom >= 16.5) {
          const rl = Math.max(bw, bh); // clip handles the ends
          ctx.strokeStyle = 'rgba(255,235,200,0.5)';
          ctx.lineWidth = 1;
          ctx.beginPath();
          ctx.moveTo(cxs - ux*rl, cys - uy*rl);
          ctx.lineTo(cxs + ux*rl, cys + uy*rl);
          ctx.stroke();
        }
        ctx.restore();
      } else {
        // Flat / irregular roof: lighter top + soft inner parapet shadow
        ctx.save();
        roofPath();
        ctx.clip();
        ctx.fillStyle = 'rgba(200,200,205,0.15)';
        ctx.fillRect(minX-2, minY-roofOff-2, bw+4, bh+4);
        roofPath();
        ctx.strokeStyle = 'rgba(0,0,0,0.25)';
        ctx.lineWidth = 3;
        ctx.stroke();
        ctx.restore();
      }
    }


  }
}

function drawParcelPoly(ctx, f, claimMap) {
  const p = f.properties;
  const geom = f.geometry;
  if (!isAreaGeom(geom)) return;

  const parcelId = p.parcel_id;
  const claim = claimMap[parcelId];
  const terrain = getParcelTerrain(p, claim);

  // Project every ring (a MultiPolygon parcel has several detached parts —
  // common for alpine Gemeindegut split by a ridge; drawing only ring 0 left
  // huge parcels invisible).
  const rings = geomAllRings(geom).map(r => r.map(c => toScreen(c[0], c[1])));
  const pts = rings[0] || [];

  // Check if visible
  let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
  for (const ring of rings) for (const pt of ring) {
    if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
    if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
  }
  if (!rings.length) return;
  if (maxX < -50 || minX > gc.width+50 || maxY < -50 || minY > gc.height+50) return;

  ctx.beginPath();
  for (const ring of rings) {
    for (let i=0; i<ring.length; i++) {
      i===0 ? ctx.moveTo(ring[i][0], ring[i][1]) : ctx.lineTo(ring[i][0], ring[i][1]);
    }
    ctx.closePath();
  }

  // Settlers-style fill with variation
  const hash = simpleHash(parcelId || '');
  const isBiodiversity = claim?.converted_to === 'biodiversity';
  const isForest = claim?.converted_to === 'forest';
  const isWild = claim?.converted_to === 'wildforest';
  const isSchlag = terrain === TERRAIN.schlag || terrain === TERRAIN.regrow;
  const isWater = terrain === TERRAIN.water;
  ctx.fillStyle = terrain[Math.abs(hash) % terrain.length];
  // More transparent when real landuse polys provide terrain backdrop
  // Biodiversity parcels get higher opacity for vibrancy
  ctx.globalAlpha = isWater ? 1
    : isSchlag ? 0.9
    : isBiodiversity
    ? (G.landusePolys.length > 0 ? 0.55 : 0.95)
    : (G.landusePolys.length > 0 ? 0.35 : 0.85);
  ctx.fill();
  ctx.globalAlpha = 1;

  // Worked-field texture (harvest tracks / furrows / mowing swaths) aligned to
  // the parcel's longest edge — see drawFieldPattern.
  if (G.cam.zoom >= 15 && !isWater && !isBiodiversity && !isForest && (maxX - minX) > 18 && (maxY - minY) > 12 &&
      (terrain === TERRAIN.farm || isCropField(p))) {
    const fs = fieldStage(p, claim);
    // stage tint under the texture: fresh soil / young green / golden grain
    const tint = fs.stage === 'ploughed' ? 'rgba(95,62,28,0.45)' : fs.stage === 'growing' ? 'rgba(90,165,55,0.35)'
      : fs.stage === 'ripe' ? 'rgba(235,190,60,0.40)' : null;
    if (tint) { ctx.fillStyle = tint; ctx.fill(); }
    drawFieldPattern(ctx, rings, hash, fs.stage);
  }

  // Enhanced mode: Lambert hillshade from lidar slope + aspect (fixed NW sun),
  // falling back to the elevation-rank tint when the parcel has no slope data.
  if (G.cam.zoom >= 14 && !isWater) {
    const lp = G.lidarParcels[parcelId];
    if (lp && lp.elev != null) {
      const hs = hillshade(lp);
      if (hs < -0.03) {
        ctx.fillStyle = 'rgba(15,20,45,' + Math.min(0.42, -hs * 0.5).toFixed(3) + ')';
        ctx.fill();
      } else if (hs > 0.03) {
        ctx.fillStyle = 'rgba(255,245,210,' + Math.min(0.30, hs * 0.38).toFixed(3) + ')';
        ctx.fill();
      } else {
        const kt = G.lidarKGTerrain[lp.kg];
        if (kt && kt.emax > kt.emin) {
          const n = Math.max(0, Math.min(1, (lp.elev - kt.emin) / (kt.emax - kt.emin)));
          if (n < 0.45) { ctx.fillStyle = 'rgba(10,20,40,' + ((0.45-n) * 0.20).toFixed(3) + ')'; ctx.fill(); }
          else if (n > 0.55) { ctx.fillStyle = 'rgba(255,250,220,' + ((n-0.55) * 0.16).toFixed(3) + ')'; ctx.fill(); }
        }
      }
      // Slope hatching for rugged terrain at high zoom
      if (G.cam.zoom >= 16.5 && lp.tclass && (lp.tclass.includes('steep') || lp.tclass.includes('rugged') || lp.tclass.includes('mountain')) && (maxX-minX) > 14) {
        ctx.save();
        ctx.clip();
        ctx.strokeStyle = 'rgba(60,40,20,0.10)';
        ctx.lineWidth = 1;
        ctx.beginPath();
        for (let hx = minX; hx < maxX + (maxY-minY); hx += 9) {
          ctx.moveTo(hx, minY);
          ctx.lineTo(hx - (maxY-minY), maxY);
        }
        ctx.stroke();
        ctx.restore();
      }
    }
  }

  // Clear-cut / regrowing stand: rutted brown ground (Rückegassen) — stumps,
  // slash and the returning trees live in drawForestOverlay.
  if (isSchlag && G.cam.zoom >= 15 && (maxX - minX) > 18 && (maxY - minY) > 12) drawFieldPattern(ctx, rings, hash, 'schlag');
  // Naturwald: dark mossy border, no glow (the canopy itself is the overlay)
  if (isWild) {
    ctx.save(); ctx.strokeStyle = '#1a6a2a'; ctx.lineWidth = 3; ctx.setLineDash([6, 4]); ctx.stroke(); ctx.setLineDash([]); ctx.restore();
  }
  // Biodiversity: soft green glow + wild-meadow ground mottling (sprites live in
  // the animated overlay, see drawNatureReserves)
  if (isBiodiversity) {
    ctx.fillStyle = 'rgba(60,200,80,0.12)';
    ctx.fill();
    if (G.cam.zoom >= 15 && (maxX - minX) > 18 && (maxY - minY) > 12) drawFieldPattern(ctx, rings, hash, 'wild');
  }

  // Border - thin dark line like terrain boundaries in Settlers
  if (isBiodiversity) {
    // Nature reserve border: thick green dashed line
    ctx.save();
    ctx.strokeStyle = '#2a9a3a';
    ctx.lineWidth = 3;
    ctx.setLineDash([6, 4]);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();
    // Inner glow border
    ctx.strokeStyle = 'rgba(100,230,120,0.35)';
    ctx.lineWidth = 5;
    ctx.stroke();
  } else if (isForest) {
    // Reforested: subtle green border
    ctx.strokeStyle = 'rgba(40,120,50,0.6)';
    ctx.lineWidth = 2.5;
    ctx.stroke();
  }
  // Normal border on top (water↔water boundaries: barely-there light ripple line)
  ctx.strokeStyle = claim ? (G.pcolors[claim.player_id]||'#fff') : (isWater ? 'rgba(200,230,255,0.12)' : 'rgba(20,40,10,0.35)');
  ctx.lineWidth = claim ? 2 : 0.5;
  ctx.stroke();

    // Draw building sprites on parcels with building landuse (only if no real footprints loaded)
  const parsed = parseLanduseSummary(p.landuse_summary);
  if (G.buildingFootprints.length === 0 && parsed.buildingCount > 0 && (maxX - minX) > 8 && (maxY - minY) > 8) {
    const pxArea = (maxX - minX) * (maxY - minY);
    const numBuildings = Math.min(6, Math.max(1, Math.floor(parsed.buildingCount * Math.min(1, pxArea / 3000))));
    const bHash = Math.abs(hash);
    for (let i = 0; i < numBuildings; i++) {
      // Deterministic pseudo-random placement inside parcel
      const t = ((bHash + i * 7919) % 10000) / 10000;
      const u = ((bHash + i * 3571) % 10000) / 10000;
      const bx = minX + (maxX - minX) * (0.15 + t * 0.7);
      const by = minY + (maxY - minY) * (0.15 + u * 0.7);
      if (pip(bx, by, pts)) {
        const big = parsed.buildingCount > 3 && i === 0;
        drawBuilding(ctx, bx, by, big, bHash + i);
      }
    }
  }

  // Claimed: draw player flag
  if (claim) {
    const cx = (minX+maxX)/2, cy = (minY+maxY)/2;
    drawFlag(ctx, cx, cy, G.pcolors[claim.player_id]||'#fff', !!claim.converted_to);
  }
}

/** Draw a pixel-art building at (x,y) */
function drawBuilding(ctx, x, y, large, seed) {
  x = Math.round(x); y = Math.round(y);
  const variant = seed % 4;
  if (large) {
    // Bigger building — church/barn
    ctx.fillStyle = '#6a5a48';
    ctx.fillRect(x-8, y-6, 16, 10);
    ctx.fillStyle = '#8a4a30';
    // Peaked roof
    ctx.beginPath();
    ctx.moveTo(x-9, y-6); ctx.lineTo(x, y-14); ctx.lineTo(x+9, y-6);
    ctx.closePath(); ctx.fill();
    // Window
    ctx.fillStyle = '#e8d880';
    ctx.fillRect(x-2, y-4, 4, 3);
    // Door
    ctx.fillStyle = '#4a3020';
    ctx.fillRect(x-2, y, 4, 4);
  } else if (variant < 2) {
    // Small house
    ctx.fillStyle = '#7a6a58';
    ctx.fillRect(x-5, y-4, 10, 7);
    ctx.fillStyle = '#a05030';
    ctx.beginPath();
    ctx.moveTo(x-6, y-4); ctx.lineTo(x, y-10); ctx.lineTo(x+6, y-4);
    ctx.closePath(); ctx.fill();
    // Window
    ctx.fillStyle = '#e8d880';
    ctx.fillRect(x-2, y-2, 2, 2);
    ctx.fillRect(x+1, y-2, 2, 2);
  } else if (variant === 2) {
    // Flat-roofed building
    ctx.fillStyle = '#808070';
    ctx.fillRect(x-6, y-5, 12, 8);
    ctx.fillStyle = '#606058';
    ctx.fillRect(x-6, y-6, 12, 2);
    ctx.fillStyle = '#e8d880';
    ctx.fillRect(x-4, y-3, 2, 2);
    ctx.fillRect(x+2, y-3, 2, 2);
  } else {
    // Shed
    ctx.fillStyle = '#6a5840';
    ctx.fillRect(x-4, y-3, 8, 6);
    ctx.fillStyle = '#8a6a40';
    ctx.fillRect(x-5, y-5, 10, 3);
  }
  // Shadow
  ctx.fillStyle = 'rgba(0,0,0,0.08)';
  ctx.beginPath();
  ctx.ellipse(x+2, y+4, 6, 2, 0, 0, Math.PI*2);
  ctx.fill();
}

function extractLuCode(lu, p) {
  // OSM water share beats NS symbol counts: a riverbed parcel can carry zero
  // GW glyphs (Danube 12105-1551/1: Alpe/GA/OG/Öd, water_fraction 0.885).
  if (p && p.parcel_id) {
    const wf = waterFraction(p.parcel_id);
    if (wf != null && wf >= 0.5) {
      const fc = G.waterParcels[p.parcel_id].fclass || [];
      return fc.some(c => /riverbank|river|stream|canal|drain/.test(c)) ? '59' : '60';
    }
  }
  // CAD-2: upstream now measures the dominant NS class by polygon *area*
  // (landuse_areas) — that beats any symbol-count weighting below.
  if (p.dominant_ns && NS_TABLE[String(p.dominant_ns)]) return String(p.dominant_ns);
  // landuse_summary is the richest source and shares the weighting logic.
  if (p.landuse_summary) {
    const parsed = parseLanduseSummary(p.landuse_summary);
    if (parsed.dominant && parsed.dominant.code) return parsed.dominant.code;
  }
  // landuse_codes (bbox endpoint) lists one NS code per symbol on the parcel,
  // in arbitrary order — take the weighted MODE, not the first entry, or a
  // single stray road glyph decides the terrain and price of a whole field.
  if (p.landuse_codes) {
    const counts = {};
    let best = '', bestW = 0;
    for (const raw of String(p.landuse_codes).split(',')) {
      const c = raw.trim();
      if (!c) continue;
      counts[c] = (counts[c] || 0) + 1;
      const w = counts[c] * nsWeight(c);
      if (w > bestW) { best = c; bestW = w; }
    }
    if (best) return best;
  }
  if (p.dominant_landuse) return String(p.dominant_landuse);
  // Fallback: try numeric from raw string
  const match = (lu || '').match(/(\d{2})/);
  if (match) return match[1];
  return '';
}

/** Get terrain colors from landuse_summary, returns the dominant terrain color array */
function getParcelTerrain(p, claim) {
  if (claim?.converted_to === 'wildforest') return TERRAIN.wildforest;
  if (claim?.converted_to) return TERRAIN.bio;
  if (claim?.harvested_at && claimIsForest(p, claim)) {
    const st = forestStage(claim).stage;
    if (st === 'schlag') return TERRAIN.schlag;
    if (st === 'jungwuchs' || st === 'stangenholz') return TERRAIN.regrow;
  }
  // OSM water ∩ parcel (feedback #16): water-dominant parcels are water, full stop.
  const wf = waterFraction(p.parcel_id);
  if (wf != null && wf >= 0.5) return TERRAIN.water;
  // Enhanced mode: real measured dominant land cover from lidar beats cadastre landuse
  const lp = G.lidarParcels[p.parcel_id];
  // Use the corrected dominant land cover (impervious road/roof skipped server-side,
  // falls back to #2 natural cover). Buildings are drawn as footprints on top.
  if (lp) {
    const dt = lp.domTerrain || (lp.dom && !IMPERVIOUS_DOM.has(lp.dom) ? lp.dom : null);
    if (dt && DOM_TERRAIN[dt]) return DOM_TERRAIN[dt];
  }
  // Cadastre fallback — same dominant code the popup and price use.
  const luCode = extractLuCode('', p);
  return LANDUSE_TERRAIN[luCode] || TERRAIN.grass;
}

/** Get human-readable landuse name from summary */
function getLanduseName(p) {
  const wf = waterFraction(p.parcel_id);
  if (wf != null && wf >= 0.5) {
    const w = G.waterParcels[p.parcel_id];
    const base = LANDUSE_NAMES[extractLuCode('', p)] || 'Gewässer';
    return '💧 ' + base + (w.name ? ' – ' + w.name : '') + ' (' + Math.round(wf*100) + '% Wasser lt. OSM)';
  }
  // CAD-2: measured m² per NS class → "Äcker, Wiesen oder Weiden 86 %, Gebäude 14 %"
  if (p.landuse_areas && typeof p.landuse_areas === 'object') {
    const tot = Object.values(p.landuse_areas).reduce((a, b) => a + (+b || 0), 0);
    const rows = Object.entries(p.landuse_areas).filter(([c, a]) => +a > 0 && NS_TABLE[c])
      .sort((a, b) => b[1] - a[1]);
    if (tot > 0 && rows.length) {
      return rows.slice(0, 4).map(([c, a], i) => {
        const pct = Math.round(a / tot * 100);
        return NS_TABLE[c].name + (rows.length > 1 && (i > 0 || pct < 100) ? ' ' + Math.max(1, pct) + ' %' : '');
      }).join(', ');
    }
  }
  if (p.landuse_summary) {
    const parsed = parseLanduseSummary(p.landuse_summary);
    if (parsed.entries.length > 0) {
      return parsed.entries
        .slice()
        .sort((a, b) => b.count * nsWeight(b.code) - a.count * nsWeight(a.code))
        .map(e => e.name + (e.count > 1 ? ' (×'+e.count+')' : '')).join(', ');
    }
  }
  const code = extractLuCode('', p);
  return LANDUSE_NAMES[code] || code || '-';
}

function drawParcelPoint(ctx, f, claimMap) {
  const p = f.properties;
  const [x, y] = toScreen(p.lon || f.geometry.coordinates[0], p.lat || f.geometry.coordinates[1]);
  if (x < -30 || x > gc.width+30 || y < -30 || y > gc.height+30) return;

  const area = p.area_sqm || 100;
  const size = Math.max(6, Math.min(40, Math.sqrt(area) * mapScale() / 80000));
  const claim = claimMap[p.parcel_id];
  const terrain = getParcelTerrain(p, claim);
  const hash = simpleHash(p.parcel_id || '');

  // Draw as slightly rotated diamond (isometric feel)
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(0.4); // slight isometric tilt
  ctx.fillStyle = terrain[Math.abs(hash) % terrain.length];
  ctx.globalAlpha = 0.8;
  ctx.fillRect(-size/2, -size/2, size, size);
  ctx.globalAlpha = 1;
  ctx.strokeStyle = claim ? (G.pcolors[claim.player_id]||'#fff') : 'rgba(20,40,10,0.3)';
  ctx.lineWidth = claim ? 2 : 0.5;
  ctx.strokeRect(-size/2, -size/2, size, size);
  ctx.restore();

  if (claim) drawFlag(ctx, x, y-size/2, G.pcolors[claim.player_id]||'#fff', !!claim.converted_to);
}

function drawFlag(ctx, x, y, color, isBio) {
  // Pixel-art flag on pole
  ctx.fillStyle = '#4a3020';
  ctx.fillRect(x-1, y-16, 2, 16);
  ctx.fillStyle = isBio ? '#2ab050' : color;
  ctx.fillRect(x+1, y-16, 10, 7);
  // Banner detail
  ctx.fillStyle = 'rgba(255,255,255,0.4)';
  ctx.fillRect(x+3, y-14, 3, 3);
  if (isBio) {
    // Tiny leaf
    ctx.fillStyle = '#90ff90';
    ctx.fillRect(x+5, y-14, 2, 2);
  }
}

// ================= LANDUSE SPRITES (crops, flowers, reeds, vines, etc.) =================
function drawLanduseSprites(ctx, claimMap) {
  if (G.cam.zoom < 16) return; // Only show at close zoom
  ctx.save();
  for (const f of G.parcelPolys) {
    const p = f.properties;
    const geom = f.geometry;
    if (!isAreaGeom(geom)) continue;
    const claim = claimMap[p.parcel_id];
    const terrain = getParcelTerrain(p, claim);
    const luCode = extractLuCode('', p);
    const area = p.area_sqm || 0;
    if (area < 100) continue;

    const coords = geomOuterRings(geom);
    if (!coords.length) continue;
    const b = geoBounds(geom);
    const [sx1,sy1] = toScreen(b.w, b.n);
    const [sx2,sy2] = toScreen(b.e, b.s);
    if (sx2 < 0 || sx1 > gc.width || sy2 < 0 || sy1 > gc.height) continue;
    if ((sx2-sx1) < 10 || (sy2-sy1) < 10) continue;

    const hash = simpleHash(p.parcel_id || '');
    let spriteType = null;

    // Determine sprite type from landuse
    if (claim?.converted_to === 'biodiversity') spriteType = 'wildflower';   // Brache / Naturschutz
    else if (luCode === '48') spriteType = 'crops';        // Äcker/Wiesen/Weiden
    else if (luCode === '52') spriteType = 'garden';   // Gärten
    else if (luCode === '53') spriteType = 'vineyard'; // Weingärten
    else if (luCode === '54' || luCode === '96') spriteType = 'meadow'; // Alpen, Freizeit
    else if (luCode === '40') continue;                // Dauerkulturen → tree sprites
    else if (terrain === TERRAIN.farm) spriteType = 'crops';
    else if (terrain === TERRAIN.meadow) spriteType = 'meadow';
    else if (terrain === TERRAIN.water) spriteType = 'water';
    else if (terrain === TERRAIN.wetland) spriteType = 'reeds';
    else if (claim?.converted_to === 'biodiversity') spriteType = 'wildflower';
    else continue;

    if (spriteType === 'wildflower') continue;   // living overlay: drawNatureReserves()
    if (spriteType === 'crops') {
      // Settlers-style fields: one motif per parcel (wheat sheaves / haystacks /
      // grass), laid out on a slightly staggered lattice so they read as rows
      // instead of random clutter. Lattice spacing in *screen* px so density is
      // constant across zoom; a parcel-stable phase keeps rows from jumping.
      const fs = fieldStage(p, claim);
      if (fs.stage === 'meadow' || fs.stage === 'stubble') drawSporadicHabitat(ctx, 'crops', b, coords, sx1, sy1, sx2, sy2, hash);
      if (fs.stage === 'ploughed' || fs.stage === 'growing' || fs.stage === 'ripe') continue; // texture carries the stage
      const kind = fs.kind;                        // 0,1 sheaves · 2 haystacks · 3 grass
      const sp = kind === 2 ? 46 : 30;
      const w = sx2 - sx1, h = sy2 - sy1;
      const cols = Math.min(10, Math.max(1, Math.floor(w / sp)));
      const rows = Math.min(10, Math.max(1, Math.floor(h / sp)));
      let n = 0;
      for (let r = 0; r < rows && n < 28; r++) for (let c = 0; c < cols && n < 28; c++) {
        const fx = (c + 0.5 + (r % 2) * 0.5 + ((hash >> (c % 7)) & 1) * 0.15) / cols;
        const fy = (r + 0.5 + ((hash >> (r % 5)) & 1) * 0.15) / rows;
        if (fx > 1 || fy > 1) continue;
        const lon = b.w + (b.e - b.w) * fx, lat = b.n - (b.n - b.s) * fy;
        if (!pipRings(lon, lat, coords)) continue;
        const [sx, sy] = toScreen(lon, lat);
        n++;
        if (kind === 3) drawMeadowSprite(ctx, sx, sy, (hash + r * 3 + c) % 5, hash + r * 31 + c);
        else drawCropSprite(ctx, sx, sy, kind === 2 ? 'haystack' : kind === 1 ? 'maize' : 'sheaf', hash + r * 31 + c);
      }
      continue;
    }
    if (spriteType === 'meadow' || spriteType === 'garden' || spriteType === 'vineyard') drawSporadicHabitat(ctx, spriteType, b, coords, sx1, sy1, sx2, sy2, hash);
    const count = Math.min(14, Math.max(2, Math.floor(area / 600)));
    for (let i = 0; i < count; i++) {
      const t = ((hash + i * 7919) % 10000) / 10000;
      const u = ((hash + i * 3571) % 10000) / 10000;
      const lon = b.w + (b.e - b.w) * (0.1 + t * 0.8);
      const lat = b.s + (b.n - b.s) * (0.1 + u * 0.8);
      if (!pipRings(lon, lat, coords)) continue;
      const [sx, sy] = toScreen(lon, lat);
      const v = (hash + i) % 5;
      switch (spriteType) {
        case 'crops': drawCropSprite(ctx, sx, sy, 'sheaf', hash+i); break;
        case 'meadow': drawMeadowSprite(ctx, sx, sy, v, hash+i); break;
        case 'vineyard': drawVineyardSprite(ctx, sx, sy, v); break;
        case 'garden': drawGardenSprite(ctx, sx, sy, v, hash+i); break;
        case 'water': drawWaterSprite(ctx, sx, sy, v, hash+i); break;
        case 'reeds': drawReedSprite(ctx, sx, sy, v, hash+i); break;
      }
    }
  }
  ctx.restore();
}

// ---- Worked-field patterns ----
// Fields read as *worked land* the way Settlers IV farms did: parallel tractor
// tracks with stubble rows on harvested grain, dark furrows on ploughed land,
// alternating light/dark swaths on mown meadows. One canvas pattern per
// (kind, zoom bucket) is cached; per parcel we only rotate it along the longest
// edge (tracks follow the field, not the screen) with a parcel-stable phase.
// Cost: one clip + one fill per field parcel.
function fieldKind(hash) { return Math.abs(hash) % 4; }   // 0,1,2 crop field (cycle) · 3 meadow (static)

// ---- INVEKOS crops (FARM-2) ----
// Where an AMA Schlag covers the parcel centroid, the *real* crop decides the
// field kind (texture, sprites, meadow-vs-crop) instead of the parcel hash.
// The cycle phase stays hash-based so neighbours still ripen at different
// times. crop_group is the coarse class upstream derives from the SNAR code.
const CROP_GROUPS = {
  getreide: {kind: 0, emoji: '🌾', name: 'Getreide'},
  mais:     {kind: 1, emoji: '🌽', name: 'Mais'},
  sonst:    {kind: 1, emoji: '🌱', name: 'Feldfrucht'},
  obst:     {kind: 2, emoji: '🍎', name: 'Obst'},
  wein:     {kind: 2, emoji: '🍇', name: 'Wein'},
  gruenland:{kind: 3, emoji: '🐄', name: 'Grünland'},
  alm:      {kind: 3, emoji: '🏔️', name: 'Alm'},
  brache:   {kind: 3, emoji: '🌼', name: 'Brache'},
};
const CROP_MEADOW = new Set(['gruenland', 'alm', 'brache']);
/** Schlag feature covering the parcel centroid, or null. Cached per schlagGen. */
function parcelSchlag(p) {
  if (!p || !p.parcel_id || !G.schlaege.length) return null;
  const c = G.cropByParcel[p.parcel_id];
  if (c && (c.f || c.gen === G.schlagGen)) return c.f;
  let lon = p.lon, lat = p.lat;
  if (lon == null || lat == null) { const f = G.sel && G.sel.properties === p ? G.sel : null; if (f) [lon, lat] = featureLonLat(f); }
  let hit = null;
  if (lon != null) {
    for (const f of G.schlaege) {
      const bb = f.bbox;
      if (lon < bb[0] || lon > bb[2] || lat < bb[1] || lat > bb[3]) continue;
      if (pipGeom(lon, lat, f.geometry)) { hit = f; break; }
    }
  }
  G.cropByParcel[p.parcel_id] = {gen: G.schlagGen, f: hit};
  return hit;
}
function fieldKindFor(p, hash) {
  const f = parcelSchlag(p);
  const cg = f && CROP_GROUPS[f.properties.crop_group];
  return cg ? cg.kind : fieldKind(hash);
}
/** "KÖRNERMAIS" → "Körnermais"; "MÄHWIESE/-WEIDE DREI UND MEHR NUTZUNGEN" → "Mähwiese/-Weide drei und mehr Nutzungen" */
function cropName(f) {
  const raw = String(f.properties.snar_name || '').toLowerCase();
  const small = new Set(['und', 'mit', 'ohne', 'drei', 'zwei', 'mehr', 'als', 'oder', 'im', 'in', 'zur', 'für']);
  return raw.split(' ').map((w, i) => (i > 0 && small.has(w)) ? w : w.replace(/(^|[\/-])(\p{L})/gu, (m, a, b) => a + b.toUpperCase())).join(' ');
}
function cropLabel(f) {
  const pr = f.properties, cg = CROP_GROUPS[pr.crop_group] || CROP_GROUPS.sonst;
  let name = cropName(f);
  if (name.length > 26) name = cg.name;
  const ha = pr.area_ha >= 1 ? pr.area_ha.toFixed(1).replace('.', ',') + ' ha' : Math.round(pr.area_ha * 10000) + ' m²';
  return cg.emoji + ' ' + name + ' · ' + ha + (pr.organic ? ' · 🌿 ' + tr('Bio') : '');
}

// ---- Field crop cycle (contract shared with srv/fieldcycle.go) ----
// Every Acker runs ploughed → growing → ripe → stubble on a 40-min real-time
// cycle, phase-shifted by the parcel hash so neighbours ripen at different
// times (~¼ of all fields are ripe at any moment). NPC farmers harvest at the
// end of the ripe window — unless the owner does it first, which is the only
// thing ever stored (claim.harvested_at). Converted fields lie fallow (Brache).
const FIELD_CYCLE_S = 40 * 60, FIELD_GROW_AT = 0.30, FIELD_RIPE_AT = 0.60, FIELD_STUBBLE_AT = 0.85;
/** Avalanche mix so sequential GNRs don't share a phase (mirrors hashMix in fieldcycle.go). */
function hashMix(h) { h = Math.imul(h ^ (h >>> 16), 0x45d9f3b) >>> 0; return (h ^ (h >>> 16)) >>> 0; }
function isCropField(p) { return extractLuCode('', p) === '48'; }
function harvestYield(areaSqm) { return Math.max(5, Math.min(300, Math.round(areaSqm * 0.012))); }
/** @returns {{kind,stage,t,ripeInS,harvested,mine}} stage ∈ fallow|meadow|ploughed|growing|ripe|stubble */
function fieldStage(p, claim, now = Date.now()) {
  const hash = simpleHash(p.parcel_id || '');
  const kind = fieldKindFor(p, hash);
  const mine = !!claim && claim.player_id === G.player?.id;
  if (claim?.converted_to) return {kind, stage: 'fallow', t: 0, ripeInS: 0, harvested: false, mine};
  if (kind === 3) return {kind, stage: 'meadow', t: 0, ripeInS: 0, harvested: false, mine};
  const sec = now / 1000;
  const t = ((sec + (hashMix(hash) % FIELD_CYCLE_S)) % FIELD_CYCLE_S) / FIELD_CYCLE_S;
  const cycleStart = sec - t * FIELD_CYCLE_S;
  const harvested = !!claim?.harvested_at && Date.parse(claim.harvested_at) / 1000 >= cycleStart;
  let stage = t < FIELD_GROW_AT ? 'ploughed' : t < FIELD_RIPE_AT ? 'growing' : t < FIELD_STUBBLE_AT ? 'ripe' : 'stubble';
  if (harvested) stage = 'stubble';
  const ripeT = (t >= FIELD_STUBBLE_AT || harvested ? 1 : 0) + FIELD_RIPE_AT;
  const ripeInS = stage === 'ripe' ? 0 : Math.round((ripeT - t) * FIELD_CYCLE_S);
  const ripeLeftS = stage === 'ripe' ? Math.round((FIELD_STUBBLE_AT - t) * FIELD_CYCLE_S) : 0;
  return {kind, stage, t, ripeInS, ripeLeftS, harvested, mine};
}
function fmtMin(sec) { const m = Math.max(1, Math.ceil(sec / 60)); return m + ' min'; }
function fieldStageLabel(fs) {
  switch (fs.stage) {
    case 'fallow':   return '🌼 ' + tr('Brache');
    case 'meadow':   return '🐄 ' + tr('Weide');
    case 'ploughed': return '🚜 ' + tr('Gepflügt') + ' · ' + tr('reif in') + ' ' + fmtMin(fs.ripeInS);
    case 'growing':  return '🌱 ' + tr('Wächst') + ' · ' + tr('reif in') + ' ' + fmtMin(fs.ripeInS);
    case 'ripe':     return '🌾 ' + tr('Reif!') + ' · ' + tr('noch') + ' ' + fmtMin(fs.ripeLeftS);
    case 'stubble':  return (fs.harvested ? '✅ ' + tr('Geerntet') : '🌾 ' + tr('Abgeerntet')) + ' · ' + tr('nächste Ernte in') + ' ' + fmtMin(fs.ripeInS);
  }
  return '';
}
const _fieldPatCache = {};
function fieldPattern(ctx, kind, k) {
  const key = kind + ':' + k;
  if (_fieldPatCache[key]) return _fieldPatCache[key];
  const P = kind === 2 ? 8 : kind === 3 ? 20 : kind === 4 ? 10 : kind === 5 ? 6 : kind === 6 || kind === 7 ? 48 : 16;   // period in px at k=1
  const c = document.createElement('canvas'); c.width = Math.round(P * k); c.height = Math.round(P * k);
  const g = c.getContext('2d');
  g.scale(k, k);
  if (kind === 7) {
    // clear-cut: churned brown soil, twin skidder ruts (Rückegasse), sawdust
    // and needle litter flecks, darker damp hollows
    let h = 0x7a3f11c9;
    const rnd = () => { h = hashMix(h + 0x9e3779b9); return (h & 0xffff) / 0xffff; };
    for (let i = 0; i < 10; i++) {
      g.fillStyle = rnd() < 0.5 ? `rgba(40,25,10,${0.08 + rnd() * 0.1})` : `rgba(200,170,110,${0.06 + rnd() * 0.06})`;
      const cx = rnd() * P, cy = rnd() * P, rx = 3 + rnd() * 8, ry = 2 + rnd() * 5;
      for (const [ox, oy] of [[0, 0], [P, 0], [-P, 0], [0, P], [0, -P]]) { g.beginPath(); g.ellipse(cx + ox, cy + oy, rx, ry, rnd() * Math.PI, 0, Math.PI * 2); g.fill(); }
    }
    g.fillStyle = 'rgba(45,28,10,0.45)'; g.fillRect(14, 0, 2, P); g.fillRect(20, 0, 2, P);      // ruts
    g.fillStyle = 'rgba(45,28,10,0.18)'; g.fillRect(16, 0, 4, P);
    for (let i = 0; i < 30; i++) { g.fillStyle = rnd() < 0.5 ? 'rgba(225,200,140,0.5)' : 'rgba(50,90,40,0.4)'; g.fillRect(Math.floor(rnd() * P), Math.floor(rnd() * P), 1, 1); }
  } else if (kind === 6) {
    // wild meadow: irregular tussock blotches (darker, taller grass), sun-bleached
    // patches, bare-soil specks — no rows, nothing straight
    let h = 0x2545f491;
    const rnd = () => { h = hashMix(h + 0x9e3779b9); return (h & 0xffff) / 0xffff; };
    for (let i = 0; i < 14; i++) {
      const dark = rnd() < 0.6;
      g.fillStyle = dark ? `rgba(10,50,10,${0.07 + rnd() * 0.09})` : `rgba(230,220,140,${0.06 + rnd() * 0.06})`;
      const cx = rnd() * P, cy = rnd() * P, rx = 3 + rnd() * 9, ry = 2 + rnd() * 6;
      for (const [ox, oy] of [[0, 0], [P, 0], [-P, 0], [0, P], [0, -P]]) {   // wrap so the tile tiles seamlessly
        g.beginPath(); g.ellipse(cx + ox, cy + oy, rx, ry, rnd() * Math.PI, 0, Math.PI * 2); g.fill();
      }
    }
    for (let i = 0; i < 26; i++) { g.fillStyle = rnd() < 0.5 ? 'rgba(90,60,20,0.35)' : 'rgba(200,240,150,0.35)'; g.fillRect(Math.floor(rnd() * P), Math.floor(rnd() * P), 1, 1); }
  } else if (kind === 4) {
    // growing: dark soil rows with a line of young green shoots
    g.fillStyle = 'rgba(70,45,15,0.30)'; g.fillRect(0, 0, 4, P);
    g.fillStyle = 'rgba(120,200,70,0.70)'; g.fillRect(4, 0, 2, P);
    g.fillStyle = 'rgba(60,140,40,0.55)'; g.fillRect(6, 0, 1, P);
    g.fillStyle = 'rgba(190,240,120,0.55)'; for (let y = 1; y < P; y += 3) g.fillRect(4, y, 1, 1);
  } else if (kind === 5) {
    // ripe standing grain: dense golden stalks, umber gaps, pale ear tips
    g.fillStyle = 'rgba(232,190,70,0.55)'; g.fillRect(0, 0, P, P);
    g.fillStyle = 'rgba(120,85,25,0.45)'; g.fillRect(0, 0, 1, P); g.fillRect(3, 0, 1, P);
    g.fillStyle = 'rgba(255,235,150,0.75)'; g.fillRect(1, 0, 1, P); g.fillRect(4, 0, 1, P);
    g.fillStyle = 'rgba(255,250,200,0.8)'; for (let y = 0; y < P; y += 2) g.fillRect(4 + (y % 4 ? 0 : -3), y, 1, 1);
  } else if (kind === 3) {
    // mown meadow: light / dark swath, thin darker seam
    g.fillStyle = 'rgba(255,255,210,0.16)'; g.fillRect(0, 0, P / 2, P);
    g.fillStyle = 'rgba(0,30,0,0.17)'; g.fillRect(P / 2, 0, P / 2, P);
    g.fillStyle = 'rgba(0,20,0,0.28)'; g.fillRect(P / 2 - 0.5, 0, 1, P);
  } else if (kind === 2) {
    // ploughed: furrow shadow + lit crest
    g.fillStyle = 'rgba(70,45,15,0.45)'; g.fillRect(0, 0, 3, P);
    g.fillStyle = 'rgba(215,185,115,0.30)'; g.fillRect(3, 0, 2, P);
    g.fillStyle = 'rgba(70,45,15,0.20)'; g.fillRect(5, 0, 3, P);
  } else {
    // harvested grain: twin tyre tracks, stubble rows between, pale straw sheen
    g.fillStyle = 'rgba(235,215,130,0.22)'; g.fillRect(0, 0, P, P);
    g.fillStyle = 'rgba(55,38,10,0.50)'; g.fillRect(1, 0, 2, P); g.fillRect(6, 0, 2, P);   // tracks
    g.fillStyle = 'rgba(55,38,10,0.22)'; g.fillRect(3, 0, 3, P);                            // between tyres
    g.fillStyle = 'rgba(110,85,25,0.40)';                                                    // stubble rows
    for (let x = 10; x < P; x += 2) g.fillRect(x, 0, 0.8, P);
    g.fillStyle = 'rgba(255,240,170,0.35)'; g.fillRect(9, 0, 1, P);                          // straw glint
  }
  const pat = ctx.createPattern(c, 'repeat');
  _fieldPatCache[key] = {pat, P: P * k};
  return _fieldPatCache[key];
}
function drawFieldPattern(ctx, rings, hash, stage) {
  const ring = rings[0];
  if (!ring || ring.length < 3) return;
  // longest edge → track direction
  let bi = 0, bl = -1;
  for (let i = 0; i < ring.length - 1; i++) {
    const dx = ring[i + 1][0] - ring[i][0], dy = ring[i + 1][1] - ring[i][1];
    const l = dx * dx + dy * dy;
    if (l > bl) { bl = l; bi = i; }
  }
  const ang = Math.atan2(ring[bi + 1][1] - ring[bi][1], ring[bi + 1][0] - ring[bi][0]);
  const z = G.cam.zoom;
  let k = z >= 18 ? 2 : z >= 16.5 ? 1.5 : z >= 15.5 ? 1 : 0.7;
  if (stage === 'wild') k = Math.min(k, 1);   // tussock blotches stay ~5 m wide, never balloon
  if (stage === 'schlag') k = Math.min(k, 1);
  const pk = stage === 'schlag' ? 7 : stage === 'wild' ? 6 : stage === 'meadow' ? 3 : stage === 'ploughed' ? 2 : stage === 'growing' ? 4 : stage === 'ripe' ? 5 : 0;
  const {pat, P} = fieldPattern(ctx, pk, k);
  // Tracks run parallel to the edge: the pattern's stripes are vertical, so
  // rotate by ang (stripe axis = y → edge direction) with a stable phase.
  const m = new DOMMatrix().translate(ring[bi][0], ring[bi][1]).rotate(ang * 180 / Math.PI + 90).translate((Math.abs(hash) % 97) / 97 * P, 0);
  pat.setTransform(m);
  ctx.save();
  ctx.clip();
  ctx.fillStyle = pat;
  ctx.globalAlpha = z >= 16 ? 1 : 0.7;
  ctx.fill();
  ctx.restore();
}

// Pixel-art field props on a unit grid (u px per pixel): a bound wheat sheaf
// with fanned ears, or a golden haystack on a pole. Palette straight from the
// Settlers IV grain fields (deep ochre → straw highlight, umber shadow).
function drawCropSprite(ctx, x, y, kind, seed) {
  const u = G.cam.zoom > 17.5 ? 2 : 1;
  x = Math.round(x); y = Math.round(y);
  const px = (dx, dy, w, h, c) => { ctx.fillStyle = c; ctx.fillRect(x + dx * u, y + dy * u, w * u, h * u); };
  const SH = 'rgba(0,0,0,0.28)', UMB = '#7a5a22', OCH = '#b8902e', GOLD = '#d8b040', STRAW = '#f0d878';
  if (kind === 'haystack') {
    // shadow, dome, pole
    px(-6, 1, 12, 1, SH); px(-4, 2, 8, 1, SH);
    px(-5, -1, 10, 2, UMB);                       // base in shade
    px(-6, -3, 12, 2, OCH); px(-5, -5, 10, 2, OCH);
    px(-4, -7, 8, 2, GOLD); px(-3, -9, 6, 2, GOLD); px(-2, -10, 4, 1, GOLD);
    px(-3, -8, 2, 1, STRAW); px(-1, -10, 2, 1, STRAW); px(-5, -4, 1, 1, STRAW); px(2, -6, 1, 1, STRAW);
    px(-2, -6, 1, 1, UMB); px(3, -4, 1, 1, UMB);  // loose straw shading
    px(0, -13, 1, 4, '#5a4020');                  // pole tip
    if (seed % 3 === 0) { px(5, -2, 2, 1, GOLD); px(-7, -1, 2, 1, GOLD); } // fallen straw
  } else if (kind === 'maize') {
    // maize stook: dried stalks tied upright, two cobs peeking out, husk leaves
    const lean = (seed % 3) - 1;
    const LEAF = '#8aa050', DRY = '#c8b070', COB = '#e8c030', HUSK = '#a89860';
    px(-4, 1, 8, 1, SH);
    px(-2, -8, 4, 9, DRY); px(-1, -11, 2, 3, DRY);
    px(-2, -8, 1, 9, HUSK); px(1, -8, 1, 9, HUSK);
    px(-2, -3, 4, 1, '#5a4020');                  // tie
    px(-4 + lean, -6, 2, 1, LEAF); px(-5 + lean, -7, 1, 1, LEAF);   // leaves
    px(2 + lean, -5, 2, 1, LEAF); px(4 + lean, -6, 1, 1, LEAF);
    px(-3, -7, 1, 3, COB); px(2, -6, 1, 3, COB);  // cobs
    px(0 + lean, -12, 1, 1, HUSK); px(-1 + lean, -13, 1, 1, HUSK); px(1 + lean, -13, 1, 1, HUSK); // tassel
  } else {
    // sheaf: stalk bundle (trapezoid), tie band, fanned ears
    const lean = (seed % 3) - 1;
    px(-4, 1, 8, 1, SH);
    px(-3, -3, 6, 4, OCH); px(-2, -6, 4, 3, OCH); px(-1, -7, 2, 1, OCH);
    px(-3, -3, 1, 4, UMB); px(2, -3, 1, 4, UMB);  // stalk shading
    px(-3, -2, 6, 1, '#5a4020');                  // tie
    px(-1, -6, 1, 3, STRAW);                      // highlight
    // ears fan out
    px(-2 + lean, -10, 1, 3, GOLD); px(-3 + lean, -11, 1, 2, GOLD);
    px(0 + lean, -11, 1, 4, GOLD); px(0 + lean, -12, 1, 1, STRAW);
    px(2 + lean, -10, 1, 3, GOLD); px(3 + lean, -11, 1, 2, GOLD);
    px(-1 + lean, -9, 1, 2, STRAW); px(1 + lean, -9, 1, 2, OCH);
    // awns
    px(-3 + lean, -13, 1, 1, OCH); px(3 + lean, -13, 1, 1, OCH); px(0 + lean, -13, 1, 1, OCH);
  }
}

function drawMeadowSprite(ctx, x, y, v, seed) {
  const s = G.cam.zoom > 17 ? 1.0 : 0.7;
  x = Math.round(x); y = Math.round(y);
  // Grass tufts with occasional small flowers
  // Grass blades
  ctx.strokeStyle = ['#5a9e3a','#4e9234','#62a240','#52963a','#6aaa48'][v];
  ctx.lineWidth = 1;
  for (let j = -2; j <= 2; j++) {
    const ox = x + j * 2.5 * s;
    const h = (5 + (seed+j)%4) * s;
    ctx.beginPath();
    ctx.moveTo(ox, y);
    ctx.quadraticCurveTo(ox + ((seed+j)%3-1)*2*s, y - h*0.6, ox + ((seed+j)%5-2)*s, y - h);
    ctx.stroke();
  }
  // Small flower on some
  if ((seed+v) % 4 === 0) {
    const fc = ['#fff','#f8e0a0','#e8a0c0','#a0c8f0','#f0f080'][(seed)%5];
    ctx.fillStyle = fc;
    ctx.beginPath();
    ctx.arc(x + ((seed%3)-1)*2, y - 8*s, 1.5*s, 0, Math.PI*2);
    ctx.fill();
  }
}

function drawVineyardSprite(ctx, x, y, v) {
  const s = G.cam.zoom > 17 ? 1.2 : 0.8;
  x = Math.round(x); y = Math.round(y);
  // Vine post with green canopy
  ctx.fillStyle = '#6a5030';
  ctx.fillRect(x - 0.5*s, y - 8*s, 1*s, 9*s); // post
  // Wire
  ctx.strokeStyle = '#888';
  ctx.lineWidth = 0.5;
  ctx.beginPath(); ctx.moveTo(x - 6*s, y - 6*s); ctx.lineTo(x + 6*s, y - 6*s); ctx.stroke();
  // Leaves
  ctx.fillStyle = ['#3a8a2a','#4a9a3a','#358a28','#4aa038','#3a8828'][v];
  ctx.beginPath(); ctx.arc(x - 3*s, y - 7*s, 3*s, 0, Math.PI*2); ctx.fill();
  ctx.beginPath(); ctx.arc(x + 2*s, y - 8*s, 2.5*s, 0, Math.PI*2); ctx.fill();
  // Grape clusters
  if (v < 3) {
    ctx.fillStyle = '#6a2878';
    ctx.beginPath(); ctx.arc(x - 2*s, y - 4*s, 1.5*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x - 1*s, y - 3*s, 1.2*s, 0, Math.PI*2); ctx.fill();
  }
}

function drawGardenSprite(ctx, x, y, v, seed) {
  const s = G.cam.zoom > 17 ? 1.0 : 0.7;
  x = Math.round(x); y = Math.round(y);
  // Pick a vegetable type based on seed for diversity
  const vegType = (seed + v) % 8;

  // Soil bed
  ctx.fillStyle = '#5a4a30';
  ctx.fillRect(x - 6*s, y - 1*s, 12*s, 3*s);
  // Soil furrows
  ctx.fillStyle = '#4a3a20';
  ctx.fillRect(x - 6*s, y, 12*s, 0.5*s);

  if (vegType === 0) {
    // Tomatoes — green bush with red/orange fruit
    for (let j = -2; j <= 2; j++) {
      const ox = x + j*3*s;
      ctx.fillStyle = '#3a7a2a';
      ctx.beginPath(); ctx.arc(ox, y - 4*s, 2.2*s, 0, Math.PI*2); ctx.fill();
      // Tomato fruits
      ctx.fillStyle = j%2===0 ? '#e03030' : '#e86020';
      ctx.beginPath(); ctx.arc(ox + 0.5*s, y - 2.5*s, 1.2*s, 0, Math.PI*2); ctx.fill();
    }
    // Stake
    ctx.strokeStyle = '#8a7050'; ctx.lineWidth = 0.8;
    ctx.beginPath(); ctx.moveTo(x, y+1*s); ctx.lineTo(x, y-7*s); ctx.stroke();
  } else if (vegType === 1) {
    // Carrots — feathery green tops, orange root tips peeking out
    for (let j = -2; j <= 2; j++) {
      const ox = x + j*2.5*s;
      // Feathery tops
      ctx.strokeStyle = '#4a9a2a'; ctx.lineWidth = 0.7;
      for (let k = -1; k <= 1; k++) {
        ctx.beginPath(); ctx.moveTo(ox, y - 1*s);
        ctx.lineTo(ox + k*1.5*s, y - 5*s - Math.abs(k)*s);
        ctx.stroke();
      }
      // Orange root tip
      ctx.fillStyle = '#e88020';
      ctx.beginPath(); ctx.moveTo(ox - 0.8*s, y - 0.5*s);
      ctx.lineTo(ox, y + 1.5*s);
      ctx.lineTo(ox + 0.8*s, y - 0.5*s);
      ctx.closePath(); ctx.fill();
    }
  } else if (vegType === 2) {
    // Cabbage/Kohlrabi — round blue-green heads
    for (let j = -2; j <= 1; j++) {
      const ox = x + j*3.5*s + 1.5*s;
      ctx.fillStyle = '#5a9a6a';
      ctx.beginPath(); ctx.arc(ox, y - 2*s, 2.5*s, 0, Math.PI*2); ctx.fill();
      ctx.fillStyle = '#6aaa7a';
      ctx.beginPath(); ctx.arc(ox - 0.5*s, y - 3*s, 1.5*s, 0, Math.PI*2); ctx.fill();
      // Outer leaves
      ctx.fillStyle = '#4a8a5a';
      ctx.beginPath();
      ctx.ellipse(ox + 2*s, y - 1*s, 1.5*s, 2*s, 0.4, 0, Math.PI*2);
      ctx.fill();
    }
  } else if (vegType === 3) {
    // Lettuce/Salat — bright light-green rosettes
    for (let j = -2; j <= 2; j++) {
      const ox = x + j*2.8*s;
      ctx.fillStyle = '#7ac050';
      ctx.beginPath(); ctx.arc(ox, y - 2.5*s, 2*s, 0, Math.PI*2); ctx.fill();
      ctx.fillStyle = '#8ad060';
      ctx.beginPath(); ctx.arc(ox, y - 3.5*s, 1.2*s, 0, Math.PI*2); ctx.fill();
    }
  } else if (vegType === 4) {
    // Pumpkins/Zucchini — large yellow-orange on vine
    // Vine
    ctx.strokeStyle = '#3a7a20'; ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x - 6*s, y - 1*s);
    ctx.quadraticCurveTo(x, y - 3*s, x + 6*s, y - 1*s);
    ctx.stroke();
    // Big leaves
    for (let j = -1; j <= 1; j++) {
      ctx.fillStyle = '#3a8a28';
      ctx.beginPath(); ctx.ellipse(x + j*4*s, y - 3*s, 2*s, 1.5*s, j*0.3, 0, Math.PI*2); ctx.fill();
    }
    // Pumpkins
    ctx.fillStyle = '#e8a020';
    ctx.beginPath(); ctx.ellipse(x - 2*s, y - 1*s, 2.5*s, 1.8*s, 0, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#d09018';
    ctx.beginPath(); ctx.ellipse(x + 3*s, y - 1*s, 2*s, 1.5*s, 0, 0, Math.PI*2); ctx.fill();
    // Stripes on pumpkin
    ctx.strokeStyle = '#c08010'; ctx.lineWidth = 0.5;
    ctx.beginPath(); ctx.moveTo(x-2*s, y-2.8*s); ctx.lineTo(x-2*s, y+0.8*s); ctx.stroke();
  } else if (vegType === 5) {
    // Beans/Peas — climbing up sticks
    for (let j = -1; j <= 1; j++) {
      const ox = x + j*4*s;
      // Stick
      ctx.strokeStyle = '#8a7050'; ctx.lineWidth = 1;
      ctx.beginPath(); ctx.moveTo(ox, y+1*s); ctx.lineTo(ox, y-8*s); ctx.stroke();
      // Winding vine
      ctx.strokeStyle = '#4a8a30'; ctx.lineWidth = 0.8;
      for (let k = 0; k < 4; k++) {
        const ky = y - k*2*s;
        ctx.beginPath();
        ctx.moveTo(ox, ky);
        ctx.quadraticCurveTo(ox + (k%2===0?2:-2)*s, ky - 1*s, ox, ky - 2*s);
        ctx.stroke();
      }
      // Bean pods
      ctx.fillStyle = '#4a9a30';
      ctx.beginPath(); ctx.ellipse(ox + 1.5*s, y - 4*s, 0.8*s, 2*s, 0.3, 0, Math.PI*2); ctx.fill();
    }
  } else if (vegType === 6) {
    // Sunflowers — tall with big yellow heads
    for (let j = -1; j <= 1; j++) {
      const ox = x + j*4*s;
      const h = (8 + (seed+j)%3) * s;
      // Stem
      ctx.strokeStyle = '#4a8a30'; ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.moveTo(ox, y); ctx.lineTo(ox, y - h); ctx.stroke();
      // Leaves on stem
      ctx.fillStyle = '#4a9a28';
      ctx.beginPath(); ctx.ellipse(ox - 2*s, y - h*0.4, 2*s, 1*s, -0.4, 0, Math.PI*2); ctx.fill();
      ctx.beginPath(); ctx.ellipse(ox + 2*s, y - h*0.6, 2*s, 1*s, 0.3, 0, Math.PI*2); ctx.fill();
      // Flower head
      ctx.fillStyle = '#e8c020';
      ctx.beginPath(); ctx.arc(ox, y - h - 1*s, 2.5*s, 0, Math.PI*2); ctx.fill();
      // Center
      ctx.fillStyle = '#8a6020';
      ctx.beginPath(); ctx.arc(ox, y - h - 1*s, 1.2*s, 0, Math.PI*2); ctx.fill();
    }
  } else {
    // Radishes/Beets — small red/pink dots with tiny green tops
    for (let j = -2; j <= 2; j++) {
      const ox = x + j*2.5*s;
      // Green tufts
      ctx.fillStyle = '#4a8a30';
      ctx.beginPath(); ctx.arc(ox, y - 3*s, 1.5*s, 0, Math.PI*2); ctx.fill();
      // Radish body poking out
      ctx.fillStyle = j%2===0 ? '#d03050' : '#c84060';
      ctx.beginPath(); ctx.arc(ox, y - 1*s, 1.3*s, 0, Math.PI*2); ctx.fill();
    }
  }
}

function drawWaterSprite(ctx, x, y, v, seed) {
  const s = G.cam.zoom > 17 ? 1.0 : 0.7;
  x = Math.round(x); y = Math.round(y);
  // Animated ripple rings
  const t = (Date.now() / 1500 + seed * 0.7) % 1;
  ctx.strokeStyle = 'rgba(160,210,255,0.3)';
  ctx.lineWidth = 0.8;
  const r = (3 + t * 8) * s;
  ctx.beginPath();
  ctx.ellipse(x, y, r, r * 0.5, 0, 0, Math.PI*2);
  ctx.stroke();
  // Sparkle
  if (t < 0.3) {
    ctx.fillStyle = 'rgba(255,255,255,0.4)';
    ctx.beginPath(); ctx.arc(x + 2*s, y - 1*s, 1*s, 0, Math.PI*2); ctx.fill();
  }
}

function drawReedSprite(ctx, x, y, v, seed) {
  const s = G.cam.zoom > 17 ? 1.1 : 0.75;
  x = Math.round(x); y = Math.round(y);
  const sway = Math.sin(Date.now()/2000 + seed*0.5) * 1.5 * s;
  // Tall reed stalks
  for (let j = -2; j <= 2; j++) {
    const ox = x + j * 2.5 * s;
    const h = (10 + (seed+j)%5) * s;
    ctx.strokeStyle = '#8a9a50';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(ox, y);
    ctx.quadraticCurveTo(ox + sway*0.5, y - h*0.5, ox + sway, y - h);
    ctx.stroke();
    // Fuzzy cattail top
    if ((seed+j) % 3 === 0) {
      ctx.fillStyle = '#6a4a20';
      ctx.beginPath();
      ctx.ellipse(ox + sway, y - h - 2*s, 1.5*s, 3*s, 0, 0, Math.PI*2);
      ctx.fill();
    }
  }
}

// ---- Nature reserve (Naturschutz / Brache) ----
// A converted parcel reads as a *Blumenwiese* the way Settlers IV drew
// decorative ground: pixel tufts of grass with chunky 3×3 blossoms in a
// parcel-stable palette (2 species per parcel so it looks like one meadow, not
// confetti), sparse structure elements (Hecke, Totholz, Lesesteinhaufen), a
// beehives, ponds, nest boxes, dead snags with hornet nests) and animated pixel
// butterflies. drawSporadicHabitat() sprinkles hives/nest boxes on ordinary land.
// Same px()/u scheme as drawCropSprite (u=1 below zoom 17.5, 2 above).
const WILD_FLOWERS = [
  {petal: '#e83a2a', dark: '#b02418', core: '#2a1a10'},   // Klatschmohn
  {petal: '#3a6ae8', dark: '#2848b8', core: '#1a2a60'},   // Kornblume
  {petal: '#f8f8f0', dark: '#d0d0c0', core: '#f0c020'},   // Margerite
  {petal: '#f0d030', dark: '#c8a020', core: '#a07010'},   // Königskerze / Löwenzahn
  {petal: '#e070b0', dark: '#b8488a', core: '#f0a0d0'},   // Wiesenklee
  {petal: '#9060e0', dark: '#6a40b0', core: '#c0a0f8'},   // Glockenblume
];
const WILD_BUTTERFLIES = [
  {w: '#f08020', e: '#2a1a10'},   // Kleiner Fuchs
  {w: '#f8f0d0', e: '#404040'},   // Kohlweißling
  {w: '#f0d020', e: '#a06010'},   // Zitronenfalter
  {w: '#5090f0', e: '#203070'},   // Bläuling
];
const WILD_GRASS = ['#3e8a2c', '#4c9c36', '#5aac42'], WILD_GRASS_TIP = '#8ccc5a', WILD_SH = 'rgba(0,0,0,0.25)';

function wildPx(ctx, x, y, u) {
  return (dx, dy, w, h, c) => { ctx.fillStyle = c; ctx.fillRect(x + dx * u, y + dy * u, w * u, h * u); };
}
/** 3×3 blossom: cross of petals, shaded lower petals, contrasting core. */
function wildBlossom(px, dx, dy, f, big) {
  if (big) {
    px(dx - 1, dy - 2, 3, 1, f.petal); px(dx - 2, dy - 1, 5, 2, f.petal); px(dx - 1, dy + 1, 3, 1, f.dark);
    px(dx - 2, dy + 1, 1, 1, f.dark); px(dx + 2, dy + 1, 1, 1, f.dark);
    px(dx, dy - 1, 1, 1, f.core); px(dx, dy, 1, 1, f.core);
  } else {
    px(dx, dy - 1, 1, 1, f.petal); px(dx - 1, dy, 3, 1, f.petal); px(dx, dy + 1, 1, 1, f.dark);
    px(dx, dy, 1, 1, f.core);
  }
}
/** Grass tuft with 2–3 blossoms of one species. */
function drawWildTuft(ctx, x, y, u, f, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  const g = WILD_GRASS[seed % 3];
  px(-4, 0, 9, 1, WILD_SH);
  // blades (columns of varying height, light tips)
  const bl = [[-4, 3], [-2, 5], [0, 7], [2, 5], [4, 3], [-3, 2], [3, 2]];
  for (let i = 0; i < bl.length; i++) {
    const h = bl[i][1] + ((seed >> i) & 1);
    px(bl[i][0], -h, 1, h, g); px(bl[i][0], -h, 1, 1, WILD_GRASS_TIP);
  }
  // stems + blossoms
  const lean = (seed % 3) - 1;
  const n = 2 + (seed & 1);
  const stems = [[-3 + lean, 8], [1 + lean, 10], [4 + lean, 7]];
  for (let i = 0; i < n; i++) {
    const [sx, sh] = stems[(i + (seed >> 2)) % 3];
    px(sx, -sh, 1, sh - 4, '#3a7a28');
    wildBlossom(px, sx, -sh - 1, f, i === 0);
  }
}
/** Hedge shrub (Hecke) with berries. */
function drawWildBush(ctx, x, y, u, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  px(-7, 0, 14, 2, WILD_SH); px(-5, 2, 10, 1, WILD_SH);
  px(-6, -3, 12, 3, '#2e6a24'); px(-7, -2, 14, 2, '#2e6a24');
  px(-5, -6, 10, 3, '#3e8a30'); px(-3, -8, 6, 2, '#3e8a30');
  px(-4, -7, 3, 1, '#62b048'); px(-1, -8, 2, 1, '#62b048'); px(-6, -4, 2, 1, '#62b048');
  px(1, -5, 3, 1, '#62b048'); px(-2, -3, 1, 1, '#1e4a18'); px(3, -2, 2, 1, '#1e4a18');
  const berry = seed % 3 === 0 ? '#e02020' : seed % 3 === 1 ? '#301848' : '#f0a020';
  px(-3, -5, 1, 1, berry); px(2, -6, 1, 1, berry); px(0, -3, 1, 1, berry); px(4, -4, 1, 1, berry);
  px(-1, -1, 2, 1, '#5a3a1a');   // stem base
}
/** Totholz: fallen log with mushrooms + moss. */
function drawWildLog(ctx, x, y, u, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  px(-8, 1, 17, 1, WILD_SH);
  px(-8, -3, 16, 4, '#6a4a28'); px(-8, -3, 16, 1, '#8a6838'); px(-8, 0, 16, 1, '#4a3018');
  px(8, -3, 1, 4, '#b89060'); px(8, -2, 1, 2, '#8a6838');   // cut end + rings
  px(-8, -3, 1, 4, '#4a3018');
  px(-5, -2, 1, 1, '#4a3018'); px(1, -1, 2, 1, '#4a3018'); px(4, -2, 1, 1, '#4a3018');  // bark cracks
  px(-3, -3, 3, 1, '#5aa040'); px(3, -3, 2, 1, '#5aa040');   // moss
  const cap = seed % 2 ? '#d83020' : '#c89040';
  px(-6, -4, 1, 1, '#f0e8d0'); px(-7, -5, 3, 1, cap); px(-6, -6, 1, 1, cap);
  if (seed % 2) px(-6, -5, 1, 1, '#f8f0e0');
  px(5, -4, 1, 1, '#f0e8d0'); px(4, -5, 3, 1, cap);
}
/** Lesesteinhaufen: dry-stone pile (habitat for lizards). */
function drawWildStones(ctx, x, y, u, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  px(-6, 1, 12, 1, WILD_SH);
  px(-6, -2, 12, 3, '#7a7a72'); px(-4, -4, 8, 2, '#8a8a80'); px(-2, -5, 4, 1, '#9a9a90');
  px(-5, -3, 2, 1, '#b0b0a6'); px(0, -5, 2, 1, '#b8b8ae'); px(2, -2, 2, 1, '#a8a89e');
  px(-6, 0, 12, 1, '#585850'); px(-1, -2, 1, 1, '#585850'); px(3, -3, 1, 1, '#585850');
  px(-3, -1, 1, 1, '#585850');
  if (seed % 2) { px(-5, -1, 2, 1, '#6aa040'); px(4, -1, 1, 1, '#6aa040'); }  // moss
  if (seed % 3 === 0) { px(1, -6, 3, 1, '#4a8a30'); px(4, -7, 1, 1, '#4a8a30'); }  // Eidechse
}
/** Bienenstock: stacked hive boxes (Zander-Beute) with bees, or a straw skep. */
function drawBeehive(ctx, x, y, u, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  px(-5, 1, 11, 1, WILD_SH);
  if (seed % 3) {
    // box hive on two legs: painted brood box, lighter honey box, roof plate
    const paint = [['#e8c040', '#b89020'], ['#4090d8', '#2a68a8'], ['#e06848', '#b04030'], ['#58a858', '#3a7a3a']][seed % 4];
    px(-3, -1, 1, 2, '#4a3020'); px(2, -1, 1, 2, '#4a3020');
    px(-4, -6, 8, 5, paint[0]); px(3, -6, 1, 5, paint[1]); px(-4, -2, 8, 1, paint[1]);
    px(-4, -10, 8, 4, '#f0e8d0'); px(3, -10, 1, 4, '#c8b890');
    px(-5, -12, 10, 2, '#5a5a58'); px(-5, -12, 10, 1, '#8a8a88');  // roof plate
    px(-2, -2, 4, 1, '#2a1a10');                                     // entrance slit
    px(-3, -8, 1, 1, '#c8b890'); px(0, -5, 1, 1, paint[1]);          // grain
  } else {
    // traditional straw skep on a plank
    px(-6, -1, 12, 1, '#7a5a30'); px(-6, 0, 12, 1, '#5a4020');
    px(-5, -4, 10, 3, '#c8a048'); px(-4, -7, 8, 3, '#d8b458'); px(-3, -9, 6, 2, '#e0c060'); px(-2, -10, 4, 1, '#e8c868');
    px(-5, -3, 10, 1, '#a88030'); px(-4, -6, 8, 1, '#b89040'); px(-3, -8, 6, 1, '#c8a048');  // coil lines
    px(-1, -3, 2, 2, '#2a1a10');                                                            // entrance
  }
  // a few bees buzzing around the entrance
  const t = Date.now() / 300 + seed;
  for (let i = 0; i < 3; i++) {
    const bx = Math.round(Math.sin(t + i * 2.1) * 6 + (i - 1) * 2), by = Math.round(-4 - i * 3 + Math.cos(t * 1.3 + i) * 2);
    px(bx, by, 1, 1, '#f0c020'); px(bx + 1, by, 1, 1, '#201810');
  }
}
/** Tümpel: small pond with reeds and a frog. */
function drawWildPond(ctx, x, y, u, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  px(-7, -1, 14, 4, '#5a7a48'); px(-8, 0, 16, 2, '#5a7a48');            // muddy bank
  px(-6, -1, 12, 3, '#3878b0'); px(-7, 0, 14, 1, '#3878b0'); px(-5, 2, 10, 1, '#2a5a90');
  px(-4, 0, 3, 1, '#78b0e0'); px(2, 1, 2, 1, '#78b0e0');                   // glints
  const r = ((Date.now() / 900 + seed) % 3) | 0;                             // ripple ring
  px(-1 - r, -r, 1, 1, 'rgba(200,230,255,0.5)'); px(1 + r, r, 1, 1, 'rgba(200,230,255,0.5)');
  for (const rx of [-7, -5, 6, 8]) { px(rx, -6, 1, 6, '#4a8a30'); px(rx, -7, 1, 2, '#7a5a30'); }  // reeds
  px(-2, -2, 3, 1, '#48a038'); px(-1, -3, 2, 1, '#48a038'); px(-1, -3, 1, 1, '#f8f8e0');            // frog
  if (seed % 2) { px(3, -1, 2, 1, '#48a038'); px(4, -2, 1, 1, '#f0e040'); }                          // lily
}
/** Nistkasten: bird nesting box on a post, a bird perched on top sometimes. */
function drawNestBox(ctx, x, y, u, seed) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  px(-2, 1, 5, 1, WILD_SH);
  px(0, -9, 1, 10, '#4a3020');
  px(-3, -16, 6, 7, '#8a6838'); px(2, -16, 1, 7, '#5a4020'); px(-3, -10, 6, 1, '#5a4020');
  px(-4, -18, 8, 2, '#4a3a2a'); px(-4, -18, 8, 1, '#6a5a48');    // sloped roof
  px(-1, -14, 2, 2, '#1a1008');                                    // hole
  px(-2, -11, 1, 1, '#c8a060');                                    // perch peg
  if (seed % 2 && Math.sin(Date.now() / 2000 + seed) > -0.3) {     // bird comes and goes
    px(-1, -20, 3, 2, '#3a6ac0'); px(2, -20, 1, 1, '#f0c020'); px(-2, -19, 1, 1, '#3a6ac0'); px(0, -21, 1, 1, '#f8f8f0');
  }
}
/** Toter Baum: bleached snag with broken limbs, woodpecker holes, ivy — and
 *  optionally a hornet colony (Vespa crabro) in a trunk cavity. */
function drawDeadTree(ctx, x, y, u, seed, hornets) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  const T = '#8a7a60', TD = '#5a4a38', TL = '#b8a888';
  px(-5, 1, 11, 1, WILD_SH); px(-3, 2, 6, 1, WILD_SH);
  px(-3, -1, 7, 2, TD); px(-4, 0, 9, 1, TD);                    // root flare
  px(-2, -20, 4, 20, T); px(1, -20, 1, 20, TD); px(-2, -18, 1, 14, TL);   // trunk, shade, light edge
  px(-2, -22, 3, 2, T); px(-1, -24, 1, 2, TD); px(0, -23, 1, 1, T);        // jagged broken top
  // limbs
  px(2, -15, 4, 1, T); px(5, -17, 2, 2, T); px(6, -18, 1, 1, TD);          // right stub
  px(-6, -12, 4, 1, T); px(-7, -14, 2, 2, T); px(-8, -15, 1, 1, TD);       // left stub
  px(2, -9, 3, 1, TD); px(4, -10, 1, 1, TD);                               // short snag
  px(-1, -16, 1, 1, '#1a1008'); px(0, -8, 1, 1, '#1a1008');                // woodpecker holes
  px(-2, -6, 1, 2, TD); px(0, -13, 1, 3, TD);                              // bark cracks
  if (seed % 2) { px(-2, -4, 1, 3, '#3e8a2c'); px(-3, -5, 1, 1, '#3e8a2c'); px(-2, -7, 1, 1, '#5aac42'); }  // ivy
  if (seed % 3 === 0) { px(3, -14, 1, 1, '#2a2a2a'); px(2, -15, 1, 1, '#2a2a2a'); px(4, -15, 1, 1, '#2a2a2a'); }  // crow
  if (hornets) {
    // hollow trunk: big dark cavity, papery comb layers visible at the rim, hornets in and out
    px(-2, -13, 4, 5, '#1a1008'); px(-1, -14, 2, 1, '#1a1008'); px(-1, -8, 2, 1, '#1a1008');
    px(-1, -12, 2, 3, '#a8a090'); px(-1, -11, 2, 1, '#c8c0b0'); px(0, -10, 1, 1, '#2a2018');   // comb
    px(-3, -12, 1, 3, TL); px(2, -12, 1, 3, TD);                                                // worn rim
    const t = Date.now() / 250 + seed;
    for (let i = 0; i < 4; i++) {
      const hx = Math.round(Math.sin(t + i * 1.7) * (4 + i)), hy = Math.round(-11 + Math.cos(t * 1.3 + i * 0.9) * 4);
      px(hx, hy, 1, 1, '#e0a020'); px(hx + 1, hy, 1, 1, '#3a2010');
    }
  }
}
/** One rare habitat feature on an ordinary (non-nature) parcel: beehives on
 *  meadows and fields, a nest box in gardens. ~14% of parcels, hash-stable. */
function drawSporadicHabitat(ctx, spriteType, b, coords, sx1, sy1, sx2, sy2, hash) {
  const m = hashMix(hash ^ 0x9e3779b9);
  if (m % 100 >= 14) return;
  const u = G.cam.zoom > 17.5 ? 2 : 1;
  if ((sx2 - sx1) < 60 * u || (sy2 - sy1) < 40 * u) return;
  // try a few hash-stable spots near the parcel edge (farmers keep hives at the margin)
  for (let i = 0; i < 6; i++) {
    const k = hashMix(m + i * 977);
    const fx = 0.12 + ((k & 255) / 255) * 0.76, fy = 0.12 + (((k >>> 8) & 255) / 255) * 0.76;
    const lon = b.w + (b.e - b.w) * fx, lat = b.n - (b.n - b.s) * fy;
    if (!pipRings(lon, lat, coords)) continue;
    const [sx, sy] = toScreen(lon, lat);
    if (spriteType === 'garden') drawNestBox(ctx, sx, sy, u, m >>> 4);
    else if (spriteType === 'vineyard') drawBeehive(ctx, sx, sy, u, (m >>> 4) | 1);   // box hives between rows
    else {
      // a small row of 2–3 hives
      const n = 2 + ((m >>> 12) & 1);
      for (let j = 0; j < n; j++) drawBeehive(ctx, sx + (j - (n - 1) / 2) * 13 * u, sy + (j % 2) * 2 * u, u, (m >>> 4) + j * 7);
    }
    return;
  }
}
/** Animated pixel butterfly circling (cx,cy). */
function drawWildButterfly(ctx, cx, cy, u, seed) {
  const now = Date.now();
  const t = now / 1400 + seed;
  const bx = Math.round(cx + Math.sin(t) * 9 * u + Math.sin(t * 2.3) * 3 * u);
  const by = Math.round(cy - 14 * u + Math.cos(t * 1.7) * 4 * u);
  const c = WILD_BUTTERFLIES[seed % WILD_BUTTERFLIES.length];
  const open = Math.sin(now / 110 + seed) > 0;
  const px = wildPx(ctx, bx, by, u);
  if (open) {
    px(-3, -1, 2, 2, c.w); px(1, -1, 2, 2, c.w); px(-2, 1, 1, 1, c.w); px(1, 1, 1, 1, c.w);
    px(-3, -1, 1, 1, c.e); px(2, -1, 1, 1, c.e);
  } else {
    px(-1, -2, 1, 3, c.w); px(1, -2, 1, 3, c.w); px(-1, -2, 1, 1, c.e); px(1, -2, 1, 1, c.e);
  }
  px(0, -1, 1, 2, '#201810');
  px(-2, 3, 5, 1, 'rgba(0,0,0,0.12)');  // ground shadow
}

// ================= LIVING NATURE RESERVES (Naturschutz / Brache) =================
// A converted parcel is a *succession meadow*, not a lawn with sprites: the
// Saum (edge band) grows thorny bramble, hawthorn and young trees; the interior
// has clumps of tall herbs (thistle, umbellifer, mullein, teasel, nettle),
// open short-grass patches with molehills and anthills, ponds, dead snags,
// stone piles, logs, beehives. Everything sways in a travelling wind field,
// fauna moves (butterflies, bees, dragonflies, swallows, a hare now and then).
//
// Architecture: drawn as a separate overlay above the cached base layer (like
// treasures) so animation never forces a full base redraw. Per parcel we build
// a *scene* once (geo-space, hash-stable): jittered sampling + clump noise +
// distance-to-edge → item kind; each item has a priority so density stays
// constant in screen px across zoom (LOD without flicker). Device gating:
// natureAnimLevel() 0 = static (prefers-reduced-motion), 1 = light
// (phones: 15 fps, 60% density, fewer fauna), 2 = full (25 fps); plus a
// self-tuning quality knob if a frame gets expensive.
const NATURE = { scenes: new Map(), onScreen: 0, quality: 1, _cost: 0, level: null /* DEV override 0|1|2 */ };

function natureAnimLevel() { if (NATURE.level != null) return NATURE.level; const b = giantAnimBudget(); return b === 1 ? 0 : b <= 6 ? 1 : 2; }
/** Wind field: two travelling gust waves + flutter; -1..1. (x,y screen px, t seconds) */
function windAt(x, y, t) {
  return 0.55 * Math.sin(t * 1.1 + x * 0.010 - y * 0.005) + 0.30 * Math.sin(t * 2.3 + x * 0.028 + y * 0.017) + 0.15 * Math.sin(t * 4.3 + x * 0.06);
}
/** Smooth value noise 0..1 on a unit lattice. */
function vnoise(x, y, seed) {
  const xi = Math.floor(x), yi = Math.floor(y), fx = x - xi, fy = y - yi;
  const h = (i, j) => (hashMix(seed + i * 374761393 + j * 668265263) & 0xffff) / 0xffff;
  const sx = fx * fx * (3 - 2 * fx), sy = fy * fy * (3 - 2 * fy);
  const a = h(xi, yi) + (h(xi + 1, yi) - h(xi, yi)) * sx, b = h(xi, yi + 1) + (h(xi + 1, yi + 1) - h(xi, yi + 1)) * sx;
  return a + (b - a) * sy;
}
function segDist2(px, py, ax, ay, bx, by) {
  const dx = bx - ax, dy = by - ay, l2 = dx * dx + dy * dy || 1e-9;
  let t = ((px - ax) * dx + (py - ay) * dy) / l2; t = t < 0 ? 0 : t > 1 ? 1 : t;
  const ex = ax + t * dx - px, ey = ay + t * dy - py; return ex * ex + ey * ey;
}
// Item kinds. Structural ones surface at low zoom (priority scaled down).
const NK = { GRASS: 0, FLOWER: 1, THISTLE: 2, UMBEL: 3, MULLEIN: 4, TEASEL: 5, NETTLE: 6, BRAMBLE: 7, SAPLING: 8, FERN: 9,
  ANTHILL: 10, MOLEHILL: 11, MUSHROOM: 12, BUSH: 13, LOG: 14, STONES: 15, HIVES: 16, POND: 17, SNAG: 18, NESTBOX: 19 };
const NK_STRUCT = new Set([NK.BRAMBLE, NK.SAPLING, NK.BUSH, NK.LOG, NK.STONES, NK.HIVES, NK.POND, NK.SNAG, NK.NESTBOX, NK.ANTHILL]);

function natureScene(f) {
  const p = f.properties, id = p.parcel_id;
  let sc = NATURE.scenes.get(id);
  if (sc) return sc;
  if (NATURE.scenes.size > 300) NATURE.scenes.clear();
  const geom = f.geometry, rings = geomAllRings(geom), b = geoBounds(geom);
  const lat0 = (b.n + b.s) / 2, mLon = 111320 * Math.cos(lat0 * Math.PI / 180), mLat = 110574;
  const wM = (b.e - b.w) * mLon, hM = (b.n - b.s) * mLat;
  const area = p.area_sqm || wM * hM * 0.6;
  const sp = Math.max(1.5, Math.sqrt(area / 2600));           // ≤ ~2600 candidates
  const hash = simpleHash(id);
  const segs = [];
  for (const r of rings) for (let i = 0; i < r.length - 1; i++) segs.push([(r[i][0] - b.w) * mLon, (r[i][1] - b.s) * mLat, (r[i + 1][0] - b.w) * mLon, (r[i + 1][1] - b.s) * mLat]);
  const edgeDist = (x, y) => { let d = Infinity; for (const s of segs) { const q = segDist2(x, y, s[0], s[1], s[2], s[3]); if (q < d) d = q; } return Math.sqrt(d); };
  const items = [];
  let pondN = 0, snagN = 0, hiveN = 0, pondCand = null;
  const cols = Math.ceil(wM / sp), rows = Math.ceil(hM / sp);
  for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) {
    const m = hashMix(hash + r * 7919 + c * 104729);
    const x = (c + 0.5 + ((m & 255) / 255 - 0.5) * 0.9) * sp, y = (r + 0.5 + (((m >>> 8) & 255) / 255 - 0.5) * 0.9) * sp;
    const lon = b.w + x / mLon, lat = b.s + y / mLat;
    if (!pipRings(lon, lat, rings)) continue;
    const n = vnoise(x / 9, y / 9, hash);                       // clump noise (9 m features)
    const rr = (m >>> 16) % 100, pr = ((m >>> 20) & 1023) / 1023, v = (m >>> 26) & 63;
    const ed = edgeDist(x, y);
    let k, s2 = 0;
    if (ed < 3.5) {                                             // Saum: thorny, woody
      k = rr < 20 ? NK.BRAMBLE : rr < 34 ? NK.SAPLING : rr < 44 ? NK.THISTLE : rr < 52 ? NK.BUSH : NK.GRASS; s2 = 2;
    } else if (n > 0.62) {                                      // tall-herb clump
      k = rr < 28 ? NK.FLOWER : rr < 42 ? NK.UMBEL : rr < 52 ? NK.MULLEIN : rr < 60 ? NK.TEASEL : rr < 69 ? NK.THISTLE : rr < 78 ? NK.NETTLE : rr < 84 ? NK.SAPLING : NK.GRASS; s2 = 2;
    } else if (n < 0.32) {                                      // open, grazed-short
      k = rr < 56 ? NK.GRASS : rr < 76 ? NK.FLOWER : rr < 81 ? NK.MOLEHILL : rr < 85 ? NK.ANTHILL : rr < 88 ? NK.STONES : rr < 91 ? NK.MUSHROOM : NK.GRASS; s2 = rr >= 91 ? 1 : 0;
    } else {
      k = rr < 48 ? NK.GRASS : rr < 68 ? NK.FLOWER : rr < 74 ? NK.THISTLE : rr < 78 ? NK.UMBEL : rr < 82 ? (n > 0.5 ? NK.FERN : NK.NETTLE) : rr < 84 ? NK.LOG : NK.GRASS; s2 = rr >= 84 ? 2 : 1;
    }
    // rare landmarks
    if (rr === 98 && snagN < 2 && ed > 2) { k = NK.SNAG; snagN++; }
    else if (rr === 97 && hiveN < 1 && ed < 10 && ed > 3) { k = NK.HIVES; hiveN++; }
    else if (rr === 99 && pondN < 1 && area > 2500 && ed > 7 && n < 0.4) { k = NK.POND; pondN++; }
    if (k === NK.SAPLING && v % 21 === 0 && ed > 3) k = NK.NESTBOX;
    if (!pondCand || ed > pondCand.ed) pondCand = { i: items.length, ed };
    items.push({ lon, lat, k, v, s2, ph: (m & 1023) / 1023 * Math.PI * 2, pr: NK_STRUCT.has(k) ? pr * 0.25 : k === NK.POND ? 0 : pr, ed, n });
  }
  if (!pondN && area > 4000 && hash % 3 === 0 && pondCand && pondCand.ed > 7) { const it = items[pondCand.i]; it.k = NK.POND; it.pr = 0; pondN = 1; }
  items.sort((a, c) => c.lat - a.lat);                          // painter's order (north first)
  const pond = items.find(it => it.k === NK.POND);
  const flowers = items.filter(it => it.k === NK.FLOWER || it.k === NK.THISTLE || it.k === NK.UMBEL);
  const snags = items.filter(it => it.k === NK.SNAG);
  sc = { items, hash, area, sp, b, mLon, mLat, pond, flowers, snags, wM, hM };
  NATURE.scenes.set(id, sc);
  return sc;
}

// ---- sprites (pixel units × u; w = wind bend at tip in pixel units, may be fractional) ----
/** Swaying blade: 3 stacked segments, bend grows quadratically toward the tip. */
function nBlade(px, x, h, w, c, tip) {
  const h1 = Math.max(1, Math.round(h * 0.4)), h2 = Math.max(1, Math.round(h * 0.3)), h3 = Math.max(1, h - h1 - h2);
  const o2 = Math.round(w * 0.3), o3 = Math.round(w);
  px(x, -h1, 1, h1, c); px(x + o2, -h1 - h2, 1, h2, c); px(x + o3, -h, 1, h3, c);
  if (tip) px(x + o3, -h, 1, 1, tip);
}
const N_GRASS = ['#3e8a2c', '#4c9c36', '#5aac42', '#468f30', '#6ab04a'], N_DRY = ['#b8a860', '#c8b870', '#a89850'];
function nGrass(px, v, s2, w, ph, t) {
  const g = N_GRASS[v % 5];
  const blades = s2 === 0 ? 3 : s2 === 1 ? 4 : 6;
  const base = s2 === 0 ? 3 : s2 === 1 ? 6 : 10;
  px(-blades, 0, blades * 2 + 1, 1, 'rgba(0,0,0,0.18)');
  for (let i = 0; i < blades; i++) {
    const x = (i - (blades - 1) / 2) * 2 + ((v >> i) & 1);
    const h = base + ((v >> (i % 6)) & 3) + (s2 === 2 ? (i % 2) * 2 : 0);
    const dry = s2 === 2 && ((v + i) % 3 === 0);
    // per-blade phase → blades don't move in lockstep
    const wb = w * (0.7 + h / 16) * (1 + 0.2 * Math.sin(t * 5 + ph + i));
    nBlade(px, x, h, wb, dry ? N_DRY[i % 3] : g, dry ? '#e8d890' : '#8ccc5a');
  }
}
function nFlower(px, v, w, ph, t) {
  const f = WILD_FLOWERS[v % WILD_FLOWERS.length], g = N_GRASS[(v >> 2) % 5];
  px(-3, 0, 7, 1, 'rgba(0,0,0,0.18)');
  for (let i = 0; i < 4; i++) nBlade(px, (i - 1.5) * 2, 4 + ((v >> i) & 3), w * 0.8, g, '#8ccc5a');
  const n = 2 + (v & 1);
  for (let i = 0; i < n; i++) {
    const sx = [-2, 1, 3][(i + (v >> 3)) % 3], h = 8 + ((v >> i) & 3);
    const wb = w * (1 + 0.15 * Math.sin(t * 4 + ph + i));
    nBlade(px, sx, h - 3, wb, '#3a7a28');
    wildBlossom(px, sx + Math.round(wb), -h, f, i === 0);
  }
}
function nThistle(px, v, w) {
  px(-3, 0, 7, 1, 'rgba(0,0,0,0.18)');
  const h = 10 + (v & 3), o = Math.round(w);
  nBlade(px, 0, h, w, '#4a8a3a');
  // spiny grey-green leaves (jagged)
  for (const [dy, s] of [[-2, 1], [-5, -1], [-3, -1], [-6, 1]]) { px(s * 1, dy, 2, 1, '#6a9a60'); px(s * 3, dy - 1, 1, 1, '#6a9a60'); }
  px(o - 1, -h - 1, 3, 2, '#5a8a40'); px(o - 1, -h - 1, 1, 1, '#8ab070');           // bulb
  px(o - 1, -h - 3, 3, 2, '#a050c0'); px(o, -h - 4, 1, 1, '#d090e0'); px(o - 2, -h - 3, 1, 1, '#d090e0'); px(o + 2, -h - 3, 1, 1, '#d090e0');
}
function nUmbel(px, v, w) {
  px(-3, 0, 7, 1, 'rgba(0,0,0,0.18)');
  const h = 12 + (v & 3), o = Math.round(w);
  nBlade(px, 0, h, w, '#4c8c34');
  px(-3, -3, 2, 1, '#4c8c34'); px(2, -5, 2, 1, '#4c8c34'); px(-2, -4, 1, 1, '#4c8c34');     // feathery leaves
  px(o - 1, -h - 1, 3, 1, '#7aa860'); px(o - 2, -h - 2, 1, 1, '#7aa860'); px(o + 2, -h - 2, 1, 1, '#7aa860');   // rays
  px(o - 3, -h - 3, 7, 1, '#f4f4ec'); px(o - 2, -h - 4, 5, 1, '#f4f4ec'); px(o - 1, -h - 2, 3, 1, '#e8e8e0');   // umbrella
  if (v & 1) px(o, -h - 3, 1, 1, '#c04040');                                                  // Wilde Möhre dot
}
function nMullein(px, v, w) {
  px(-4, 0, 9, 1, 'rgba(0,0,0,0.2)');
  const h = 12 + (v & 3), o = Math.round(w * 0.6);
  px(-4, -1, 3, 1, '#8ab088'); px(2, -1, 3, 1, '#8ab088'); px(-3, -2, 2, 1, '#a0c0a0'); px(2, -2, 2, 1, '#a0c0a0');   // woolly basal leaves
  px(0, -h + 6, 1, h - 6, '#6a9a60');
  px(o, -h, 2, 7, '#e8c020'); px(o, -h, 1, 1, '#f8e060');
  for (let i = 0; i < 7; i += 2) px(o + (i % 4 ? 1 : 0), -h + i, 1, 1, '#f8e880');
  px(o - 1, -h + 2, 1, 1, '#c8a010'); px(o + 2, -h + 4, 1, 1, '#c8a010');
}
function nTeasel(px, v, w) {
  px(-2, 0, 5, 1, 'rgba(0,0,0,0.18)');
  const h = 11 + (v & 3), o = Math.round(w * 0.7);
  nBlade(px, 0, h, w * 0.7, '#7a8a50');
  px(-3, -4, 3, 1, '#7a8a50'); px(1, -4, 3, 1, '#7a8a50');            // paired leaves (cup)
  px(o - 1, -h - 4, 3, 5, '#9a8a68'); px(o, -h - 5, 1, 1, '#9a8a68'); px(o, -h - 1, 1, 1, '#8a7a58');
  px(o - 1, -h - 2, 3, 1, '#a070b0');                                 // purple flower band
  px(o - 2, -h - 3, 1, 1, '#c8b890'); px(o + 2, -h - 3, 1, 1, '#c8b890'); px(o - 2, -h - 1, 1, 1, '#c8b890'); px(o + 2, -h - 1, 1, 1, '#c8b890');   // spines
}
function nNettle(px, v, w) {
  px(-3, 0, 7, 1, 'rgba(0,0,0,0.2)');
  const o = Math.round(w * 0.5), D = '#2e6a24', L = '#3e8a30';
  px(0, -8, 1, 8, D);
  for (let i = 0; i < 3; i++) { const dy = -3 - i * 2, s = i % 2 ? 1 : -1; px(s * 1 + (i > 0 ? o : 0), dy, 2, 1, L); px(s * 3 + (i > 0 ? o : 0), dy - 1, 1, 1, L); px(s * 2 + (i > 0 ? o : 0), dy + 1, 1, 1, D); }
  px(o - 1, -9, 3, 1, L); px(o, -10, 1, 1, L);
  px(o - 2, -7, 1, 2, '#c8d8b0'); px(o + 2, -6, 1, 2, '#c8d8b0');   // hanging flower strands
}
function nBramble(px, v) {
  px(-6, 0, 13, 1, 'rgba(0,0,0,0.22)');
  px(-5, -3, 11, 3, '#2a5a20'); px(-6, -1, 13, 1, '#2a5a20'); px(-3, -4, 7, 1, '#2a5a20');
  px(-4, -3, 2, 1, '#4a8a3a'); px(1, -4, 2, 1, '#4a8a3a'); px(3, -2, 1, 1, '#4a8a3a');
  // arching canes with thorns
  const C = '#7a3030';
  px(-6, -4, 1, 1, C); px(-5, -5, 2, 1, C); px(-3, -6, 3, 1, C); px(0, -5, 2, 1, C); px(2, -4, 1, 1, C);
  px(3, -5, 1, 1, C); px(4, -6, 2, 1, C); px(6, -5, 1, 1, C); px(7, -3, 1, 2, C);
  px(-4, -6, 1, 1, '#d0c0a0'); px(1, -6, 1, 1, '#d0c0a0'); px(5, -7, 1, 1, '#d0c0a0');   // thorns
  const berries = v % 3 === 0 ? '#181020' : '#c02040';                                    // ripe / unripe
  px(-2, -5, 1, 1, berries); px(-1, -4, 1, 1, berries); px(4, -4, 1, 1, berries); px(-4, -2, 1, 1, berries);
}
function nSapling(px, v, w) {
  const kind = v % 3, o = Math.round(w * 0.6);
  px(-3, 0, 7, 1, 'rgba(0,0,0,0.22)');
  if (kind === 0) {           // birch: white trunk, airy light crown
    px(0, -12, 1, 12, '#e8e8e0'); px(0, -9, 1, 1, '#303030'); px(0, -4, 1, 1, '#303030'); px(0, -7, 1, 1, '#909090');
    px(o - 2, -15, 5, 3, '#8ccc5a'); px(o - 3, -13, 7, 2, '#8ccc5a'); px(o - 1, -16, 3, 1, '#8ccc5a');
    px(o - 2, -14, 1, 1, '#b8e880'); px(o + 1, -15, 1, 1, '#b8e880'); px(o - 3, -12, 2, 1, '#5aac42'); px(o + 2, -12, 2, 1, '#5aac42');
  } else if (kind === 1) {    // oak: brown trunk, dense round crown
    px(0, -8, 1, 8, '#5a3a1a'); px(-1, -1, 3, 1, '#4a2a10');
    px(o - 3, -13, 7, 4, '#2e7a2a'); px(o - 2, -14, 5, 1, '#2e7a2a'); px(o - 4, -11, 9, 2, '#2e7a2a'); px(o - 3, -9, 7, 1, '#2e7a2a');
    px(o - 2, -13, 2, 1, '#5aac42'); px(o + 1, -12, 2, 1, '#5aac42'); px(o - 3, -10, 1, 1, '#1e5a1a'); px(o + 2, -9, 2, 1, '#1e5a1a');
  } else {                    // young spruce: layered triangle
    px(0, -3, 1, 3, '#5a3a1a');
    px(o - 4, -5, 9, 2, '#1e6a2a'); px(o - 3, -7, 7, 2, '#246e30'); px(o - 2, -9, 5, 2, '#2a7a36'); px(o - 1, -11, 3, 2, '#2a7a36'); px(o, -12, 1, 1, '#2a7a36');
    px(o - 4, -5, 2, 1, '#3a9a48'); px(o - 2, -9, 1, 1, '#3a9a48'); px(o, -12, 1, 1, '#8ccc5a'); px(o + 1, -7, 2, 1, '#164a1e');
  }
}
function nFern(px, v, w) {
  px(-4, 0, 9, 1, 'rgba(0,0,0,0.18)');
  const o = Math.round(w * 0.5);
  for (let i = 0; i < 4; i++) {
    const s = i % 2 ? 1 : -1, len = 4 + (i >> 1);
    for (let j = 0; j < len; j++) { px(s * j + (j > 2 ? o : 0), -1 - j - (i >> 1), 1, 1, '#3e8a2c'); if (j % 2) px(s * j + s + (j > 2 ? o : 0), -1 - j - (i >> 1), 1, 1, '#5aac42'); }
  }
  px(0, -2, 1, 2, '#2e6a24');
}
function nAnthill(px, v, t) {
  px(-4, 1, 9, 1, 'rgba(0,0,0,0.2)');
  px(-4, -1, 9, 2, '#7a5a30'); px(-3, -3, 7, 2, '#8a6a38'); px(-1, -4, 3, 1, '#9a7a44');
  px(-2, -2, 1, 1, '#c0a060'); px(1, -3, 1, 1, '#c0a060'); px(3, -1, 1, 1, '#5a3a18');
  for (let i = 0; i < 3; i++) { const a = t * 1.5 + i * 2.1 + v; px(Math.round(Math.cos(a) * (3 + i)), Math.round(-1 + Math.sin(a) * 1.5 - i * 0.6), 1, 1, '#2a1a10'); }
}
function nMolehill(px) { px(-3, 0, 7, 1, 'rgba(0,0,0,0.2)'); px(-3, -1, 7, 1, '#5a3a20'); px(-2, -2, 5, 1, '#6a4a28'); px(-1, -3, 2, 1, '#7a5a30'); px(1, -1, 1, 1, '#3a2010'); }
function nMushroom(px, v) {
  px(-3, 1, 7, 1, 'rgba(0,0,0,0.18)');
  const cap = v % 3 === 0 ? '#d83020' : v % 3 === 1 ? '#c89040' : '#e8d8b0';
  px(-2, -1, 1, 2, '#f0e8d0'); px(-3, -2, 3, 1, cap); px(-2, -3, 1, 1, cap);
  px(2, -1, 1, 2, '#f0e8d0'); px(1, -3, 3, 1, cap); px(2, -4, 1, 1, cap);
  if (v % 3 === 0) { px(-2, -2, 1, 1, '#f8f0e0'); px(2, -3, 1, 1, '#f8f0e0'); }
}
function drawNatureItem(ctx, it, x, y, u, w, t) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  switch (it.k) {
    case NK.GRASS: nGrass(px, it.v, it.s2, w, it.ph, t); break;
    case NK.FLOWER: nFlower(px, it.v, w, it.ph, t); break;
    case NK.THISTLE: nThistle(px, it.v, w); break;
    case NK.UMBEL: nUmbel(px, it.v, w * 1.2); break;
    case NK.MULLEIN: nMullein(px, it.v, w); break;
    case NK.TEASEL: nTeasel(px, it.v, w); break;
    case NK.NETTLE: nNettle(px, it.v, w); break;
    case NK.BRAMBLE: nBramble(px, it.v); break;
    case NK.SAPLING: nSapling(px, it.v, w); break;
    case NK.FERN: nFern(px, it.v, w); break;
    case NK.ANTHILL: nAnthill(px, it.v, t); break;
    case NK.MOLEHILL: nMolehill(px); break;
    case NK.MUSHROOM: nMushroom(px, it.v); break;
    case NK.BUSH: drawWildBush(ctx, x, y, u, it.v); break;
    case NK.LOG: drawWildLog(ctx, x, y, u, it.v); break;
    case NK.STONES: drawWildStones(ctx, x, y, u, it.v); break;
    case NK.HIVES: { const n = 2 + (it.v & 1); for (let j = 0; j < n; j++) drawBeehive(ctx, x + (j - (n - 1) / 2) * 13 * u, y + (j % 2) * 2 * u, u, it.v + j * 7); break; }
    case NK.POND: drawWildPond(ctx, x, y, u, it.v); break;
    case NK.SNAG: drawDeadTree(ctx, x, y, u, it.v, it.v % 3 === 0); break;
    case NK.NESTBOX: drawNestBox(ctx, x, y, u, it.v); break;
  }
}

// ---- fauna ----
function nBee(px, bx, by, t, i) {
  px(bx, by, 1, 1, '#f0c020'); px(bx + 1, by, 1, 1, '#201810'); if (Math.sin(t * 40 + i) > 0) px(bx, by - 1, 2, 1, 'rgba(255,255,255,0.6)');
}
function nDragonfly(ctx, x, y, u, t, seed) {
  const bx = Math.round(x + Math.sin(t * 0.9 + seed) * 12 * u + Math.sin(t * 3.1) * 3 * u), by = Math.round(y - 10 * u + Math.cos(t * 1.4 + seed) * 4 * u);
  const px = wildPx(ctx, bx, by, u), open = Math.sin(t * 30) > 0;
  px(-1, 0, 6, 1, '#20a0c0'); px(4, 0, 1, 1, '#106080');
  if (open) { px(-3, -1, 3, 1, 'rgba(220,240,255,0.7)'); px(-3, 1, 3, 1, 'rgba(220,240,255,0.7)'); px(1, -1, 3, 1, 'rgba(220,240,255,0.7)'); px(1, 1, 3, 1, 'rgba(220,240,255,0.7)'); }
  else { px(-2, -1, 6, 1, 'rgba(220,240,255,0.5)'); }
}
function nSwallow(ctx, x, y, u, t, i) {
  const px = wildPx(ctx, Math.round(x), Math.round(y), u), flap = Math.sin(t * 9 + i) > 0;
  px(-1, 0, 3, 1, '#202838'); px(2, 0, 1, 1, '#f0f0f0');
  if (flap) { px(-4, -1, 3, 1, '#202838'); px(2, -1, 3, 1, '#202838'); } else { px(-4, 1, 3, 1, '#202838'); px(2, 1, 3, 1, '#202838'); }
  px(-3, 0, 1, 1, '#202838'); px(-4, 1, 1, 1, '#202838');   // forked tail
}
function nHare(ctx, x, y, u, t, dir) {
  const px = wildPx(ctx, Math.round(x), Math.round(y), u), hop = Math.abs(Math.sin(t * 6)), d = dir;
  const dy = -Math.round(hop * 3);
  px(-3, 1, 7, 1, 'rgba(0,0,0,0.2)');
  px(-3, dy - 3, 6, 3, '#9a7a58'); px(-3, dy - 4, 4, 1, '#9a7a58'); px(d * 3, dy - 4, 2, 2, '#9a7a58');   // body, head
  px(d * 3, dy - 7, 1, 3, '#9a7a58'); px(d * 4, dy - 7, 1, 3, '#b89a78');                                  // ears
  px(d * 4, dy - 4, 1, 1, '#201810'); px(-d * 3, dy - 3, 1, 1, '#f0f0f0');                                  // eye, tail
  px(-2, dy, 1, 1, '#7a5a38'); px(1, dy, 1, 1, '#7a5a38');
}
function drawNatureFauna(ctx, sc, u, t, lvl, sx1, sy1, sx2, sy2, frac) {
  const shown = sc.items.length * Math.min(1, frac);
  if (lvl >= 1) {
    // butterflies wander between flower clumps
    const nB = Math.min(lvl === 1 ? 4 : 8, Math.max(1, Math.floor(shown / 70)));
    for (let i = 0; i < nB && sc.flowers.length; i++) {
      const a = sc.flowers[(sc.hash + i * 37) % sc.flowers.length];
      const [x, y] = toScreen(a.lon, a.lat);
      drawWildButterfly(ctx, x, y, u, (sc.hash >>> 3) + i * 11);
    }
    // bees at flowers
    const beeN = Math.min(lvl === 1 ? 3 : 8, Math.floor(shown / 90));
    for (let i = 0; i < beeN && sc.flowers.length; i++) {
      const a = sc.flowers[(sc.hash + 13 + i * 53) % sc.flowers.length];
      const [x, y] = toScreen(a.lon, a.lat);
      const px = wildPx(ctx, Math.round(x), Math.round(y), u);
      nBee(px, Math.round(Math.sin(t * 2.2 + i) * 5), Math.round(-9 + Math.cos(t * 3.1 + i * 1.3) * 3), t, i);
    }
    if (sc.pond && lvl === 2) { const [x, y] = toScreen(sc.pond.lon, sc.pond.lat); nDragonfly(ctx, x, y, u, t, sc.hash & 63); }
    // crow on a snag
    for (const sn of sc.snags) if (Math.sin(t * 0.13 + sn.ph) > 0.2) { const [x, y] = toScreen(sn.lon, sn.lat); const px = wildPx(ctx, Math.round(x), Math.round(y), u); px(-3, -23, 3, 1, '#202020'); px(-2, -24, 1, 1, '#202020'); px(-4, -24, 1, 1, '#202020'); }
    // swallows over larger meadows
    if (sc.area > 3000 && lvl === 2) {
      const n = 2 + (sc.hash & 1);
      for (let i = 0; i < n; i++) {
        const ph = ((t * 0.09 + i * 0.37 + (sc.hash % 100) / 100) % 1);
        const x = sx1 + (sx2 - sx1) * ph, y = sy1 + (sy2 - sy1) * (0.3 + 0.4 * Math.sin(ph * 6 + i)) - 30 * u;
        nSwallow(ctx, x, y, u, t, i);
      }
    }
    // a hare crosses every ~45 s
    if (sc.area > 1500) {
      const cyc = (t + (sc.hash % 45)) % 45;
      if (cyc < 5) { const ph = cyc / 5, dir = sc.hash & 1 ? 1 : -1; const x = sx1 + (sx2 - sx1) * (dir > 0 ? ph : 1 - ph), y = sy1 + (sy2 - sy1) * (0.35 + 0.3 * ((sc.hash >>> 5) % 100) / 100); nHare(ctx, x, y, u, t, dir); }
    }
  }
}

function drawNatureReserves(ctx, claimMap) {
  NATURE.onScreen = 0;
  const zoom = G.cam.zoom;
  if (zoom < 15 || !G.parcelPolys.length) return;
  const t0 = performance.now();
  const u = zoom > 19 ? 3 : zoom > 17.5 ? 2 : 1, lvl = natureAnimLevel(), t = lvl ? Date.now() / 1000 : 0;
  const W = gc.width, H = gc.height, cell = 7.5 * u;
  ctx.save();
  for (const f of G.parcelPolys) {
    const claim = claimMap[f.properties.parcel_id];
    if (claim?.converted_to !== 'biodiversity' || !isAreaGeom(f.geometry)) continue;
    const b = geoBounds(f.geometry);
    const [sx1, sy1] = toScreen(b.w, b.n), [sx2, sy2] = toScreen(b.e, b.s);
    if (sx2 < -30 || sx1 > W + 30 || sy2 < -30 || sy1 > H + 30) continue;
    if ((sx2 - sx1) < 8 || (sy2 - sy1) < 6) continue;
    NATURE.onScreen++;
    const sc = natureScene(f);
    const pxPerM = mapScale() / sc.mLon;
    let frac = Math.min(1, (sc.sp * pxPerM / cell) ** 2) * NATURE.quality * (lvl === 1 ? 0.6 : 1);
    for (const it of sc.items) {
      if (it.pr > frac) continue;
      const [x, y] = toScreen(it.lon, it.lat);
      if (x < -20 || x > W + 20 || y < -30 || y > H + 20) continue;
      const w = lvl ? windAt(x, y, t) * 2.2 : 0;
      drawNatureItem(ctx, it, x, y, u, w, t);
    }
    drawNatureFauna(ctx, sc, u, t, lvl, Math.max(sx1, 0), Math.max(sy1, 0), Math.min(sx2, W), Math.min(sy2, H), frac);
  }
  ctx.restore();
  // self-tuning: if the overlay eats > 9 ms, thin it out; recover slowly when cheap
  const cost = performance.now() - t0;
  NATURE._cost = NATURE._cost * 0.8 + cost * 0.2;
  if (NATURE._cost > 9 && NATURE.quality > 0.3) NATURE.quality = Math.max(0.3, NATURE.quality * 0.9);
  else if (NATURE._cost < 4 && NATURE.quality < 1) NATURE.quality = Math.min(1, NATURE.quality * 1.03);
}

// ============================================================================
// FOREST PLOTS — harvest cycle (Holzernte / Kahlschlag) + living overlays for
// clear-cuts and Naturwald (wild forest). Shared contract with timber.go.
// ============================================================================
const FOREST = { schlagMin: 40, jungMin: 90, stangenMin: 150, fullMin: 510, scenes: new Map() };

/** Is this claim/parcel a forest stand? NS 56, or lidar tree cover ≥ 50 % on a
 *  non-crop parcel. Pass the feature when available (lidar lookup). */
function claimIsForest(p, claim) {
  if (claim?.converted_to === 'wildforest') return true;
  const lu = extractLuCode('', p);
  if (lu === '56') return true;
  if (lu === '48') return false;
  const lp = G.lidarParcels[p.parcel_id];
  const t = lp?.fracs?.tree ?? lp?.forestFrac;
  return t != null && t >= 0.5;
}
function isForestParcel(f, claim) { return claimIsForest(f.properties || f, claim); }

/** Stage of a (harvested) forest stand. Mirrors forestPhase() in timber.go. */
function forestStage(claim, now = Date.now()) {
  const ha = claim?.harvested_at ? Date.parse(claim.harvested_at) : NaN;
  if (!claim || isNaN(ha)) return { stage: 'baumholz', t: 1, factor: 1, min: Infinity, nextInS: 0 };
  const min = (now - ha) / 60000, t = Math.min(1, min / FOREST.stangenMin);
  if (min < FOREST.schlagMin) return { stage: 'schlag', t, factor: 0, min, nextInS: (FOREST.schlagMin - min) * 60, readyInS: (FOREST.stangenMin - min) * 60 };
  if (min < FOREST.jungMin) return { stage: 'jungwuchs', t, factor: 0, min, nextInS: (FOREST.jungMin - min) * 60, readyInS: (FOREST.stangenMin - min) * 60 };
  if (min < FOREST.stangenMin) return { stage: 'stangenholz', t, factor: 0, min, nextInS: (FOREST.stangenMin - min) * 60, readyInS: (FOREST.stangenMin - min) * 60 };
  const factor = Math.max(0.5, Math.min(1, 0.5 + 0.5 * (min - FOREST.stangenMin) / (FOREST.fullMin - FOREST.stangenMin)));
  return { stage: 'baumholz', t: 1, factor, min, nextInS: 0, readyInS: 0 };
}
function forestStageLabel(fs) {
  switch (fs.stage) {
    case 'schlag': return '🪓 ' + tr('Kahlschlag') + ' · ' + tr('Jungwuchs in') + ' ' + fmtMin(fs.nextInS);
    case 'jungwuchs': return '🌱 ' + tr('Jungwuchs') + ' · ' + tr('Stangenholz in') + ' ' + fmtMin(fs.nextInS);
    case 'stangenholz': return '🌲 ' + tr('Stangenholz') + ' · ' + tr('erntereif in') + ' ' + fmtMin(fs.nextInS);
  }
  return '🌳 ' + tr('Baumholz') + (fs.factor < 1 ? ' · ' + Math.round(fs.factor * 100) + ' % ' + tr('Wert') : ' · ' + tr('hiebsreif'));
}

/** Lazy timber estimate per parcel (server: /api/forest-value, timber.go). */
async function fetchForestValue(f) {
  const p = f.properties, pid = p.parcel_id;
  if (pid in G.forestValues) return G.forestValues[pid];
  G.forestValues[pid] = null;
  try {
    const d = await GET('/api/forest-value?parcel_id=' + encodeURIComponent(pid) + '&kg=' + encodeURIComponent(p.kg_code || pid.split('-')[0]) +
      '&area=' + (p.area_sqm || 0) + '&lu=' + encodeURIComponent(extractLuCode('', p)) + '&session_id=' + encodeURIComponent(G.session?.id || ''));
    G.forestValues[pid] = (d && !d.error && d.estimate) ? d : null;
  } catch (e) { G.forestValues[pid] = null; }
  return G.forestValues[pid];
}
function fmtEur(v) { return v >= 1e6 ? (v / 1e6).toFixed(2) + ' Mio €' : Math.round(v).toLocaleString('de-AT') + ' €'; }

/** Popup rows for a forest stand (called from renderEnhancedPopupRows). */
function forestPopupRows(fv, claim) {
  const e = fv?.estimate;
  if (!e || !e.is_forest) return [];
  const rows = [];
  const src = e.source === 'v3' ? tr('Einzelbaum-Inventur (ALS)') : e.source === 'lidar' ? tr('ALS-Kronenhöhe') : tr('Nutzungsart');
  let stock = '~' + Math.round(e.vfm).toLocaleString('de-AT') + ' Vfm';
  if (e.vfm_per_ha) stock += ' <span style="color:var(--text-dim)">(' + e.vfm_per_ha + '/ha · Ø ' + e.h_mean_m + ' m' + (e.n_trees ? ' · ' + e.n_trees + ' ' + tr('Bäume') : '') + ')</span>';
  rows.push(['🪵 ' + tr('Holzvorrat'), stock]);
  const sp = e.species || {};
  const mix = [['spruce_fir', tr('Fichte/Tanne')], ['larch', tr('Lärche')], ['pine', tr('Kiefer')], ['broadleaf', tr('Laubholz')]]
    .filter(([k]) => (sp[k] || 0) >= 0.08).sort((a, b) => sp[b[0]] - sp[a[0]]).map(([k, n]) => n + ' ' + Math.round(sp[k] * 100) + '%').join(' · ');
  if (mix) rows.push(['🌲 ' + tr('Bestand'), mix + ' <span style="color:var(--text-dim)">· ' + src + '</span>']);
  const pr = e.prices || {};
  const fs = forestStage(claim);
  const eur = e.net_eur * (claim ? fs.factor : 1);
  rows.push(['💶 ' + tr('Holzerlös'), '<b style="color:var(--gold)">≈ ' + fmtEur(eur) + '</b> <span style="color:var(--text-dim)">' + tr('netto') + ' · ' + Math.round(e.efm) + ' Efm · ' +
    tr('Fichte') + ' ' + Math.round(pr.spruce_eur_efm || 0) + ' €/Efm' + (pr.date ? ' (' + (pr.live ? pr.state + ' ' + pr.date : tr('Richtwert')) + ')' : '') + '</span>']);
  rows.push(['🌍 CO₂', '~' + Math.round(e.co2_t).toLocaleString('de-AT') + ' t ' + tr('im Holz gespeichert')]);
  return rows;
}

// ---- terrain palettes for the stand cycle ----
TERRAIN.schlag = ['#7a6040', '#826848', '#725838', '#8a7050', '#6a5030'];
TERRAIN.regrow = ['#6e7a3c', '#768244', '#667236', '#7e8a4c', '#5e6a30'];
TERRAIN.wildforest = ['#173f17', '#1b451b', '#153b15', '#1e4a1e', '#123612'];

// ---- scene construction ----
// item kinds (forest-specific; NK.* kinds are reused via drawNatureItem)
const FK = { TREE: 100, STUMP: 101, SLASH: 102, POLTER: 103, ROOTPLATE: 104, BLUEBERRY: 105, THORN: 106, HERB: 107 };
// species: 0 spruce 1 fir 2 larch 3 beech 4 oak 5 birch 6 rowan 7 maple
const F_SPECIES_LOW = [0, 1, 3, 3, 4, 5, 7, 3, 0, 6], F_SPECIES_MID = [0, 0, 1, 3, 2, 5, 6, 0, 1, 3], F_SPECIES_HIGH = [0, 0, 2, 2, 6, 1, 0, 2, 0, 5];
const F_PIONEER = [5, 6, 0, 5, 2, 6];   // birch, rowan, spruce, larch on a clear-cut

function forestScene(f, mode) {
  const p = f.properties, id = p.parcel_id, key = id + ':' + mode;
  let sc = FOREST.scenes.get(key);
  if (sc) return sc;
  if (FOREST.scenes.size > 200) FOREST.scenes.clear();
  const geom = f.geometry, rings = geomAllRings(geom), b = geoBounds(geom);
  const lat0 = (b.n + b.s) / 2, mLon = 111320 * Math.cos(lat0 * Math.PI / 180), mLat = 110574;
  const wM = (b.e - b.w) * mLon, hM = (b.n - b.s) * mLat;
  const area = p.area_sqm || wM * hM * 0.6;
  const sp = Math.max(2.2, Math.sqrt(area / 2200));
  const hash = simpleHash(id);
  const lp = G.lidarParcels[id];
  const elev = lp?.elev ?? 600, hMean = lp?.treeH?.mean ?? 20;
  const table = elev > 1200 ? F_SPECIES_HIGH : elev > 750 ? F_SPECIES_MID : F_SPECIES_LOW;
  const veteranBias = hMean > 26 ? 0.15 : hMean < 14 ? -0.2 : 0;
  const segs = [];
  for (const r of rings) for (let i = 0; i < r.length - 1; i++) segs.push([(r[i][0] - b.w) * mLon, (r[i][1] - b.s) * mLat, (r[i + 1][0] - b.w) * mLon, (r[i + 1][1] - b.s) * mLat]);
  const edgeDist = (x, y) => { let d = Infinity; for (const s of segs) { const q = segDist2(x, y, s[0], s[1], s[2], s[3]); if (q < d) d = q; } return Math.sqrt(d); };
  const items = [];
  let polterN = 0, snagN = 0;
  const cols = Math.ceil(wM / sp), rows = Math.ceil(hM / sp);
  for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) {
    const m = hashMix(hash + r * 7919 + c * 104729), m2 = hashMix(m + 0x51ed27);
    const x = (c + 0.5 + ((m & 255) / 255 - 0.5) * 0.9) * sp, y = (r + 0.5 + (((m >>> 8) & 255) / 255 - 0.5) * 0.9) * sp;
    const lon = b.w + x / mLon, lat = b.s + y / mLat;
    if (!pipRings(lon, lat, rings)) continue;
    const n = vnoise(x / 12, y / 12, hash);                     // 12 m stand-structure clumps
    const rr = (m >>> 16) % 100, pr = ((m >>> 20) & 1023) / 1023, v = (m >>> 26) & 63;
    const ed = edgeDist(x, y);
    const it = { lon, lat, v, ph: (m & 1023) / 1023 * Math.PI * 2, pr, ed, n, k: -1, size: 0, spc: table[v % table.length], bend: 0, birth: 0 };
    if (mode === 'wild') {
      if (ed < 3) {                                             // Waldsaum / Waldmantel: thorny
        if (rr < 28) it.k = NK.BRAMBLE; else if (rr < 46) it.k = FK.THORN; else if (rr < 62) { it.k = FK.TREE; it.size = 0; }
        else if (rr < 78) { it.k = FK.TREE; it.size = 1; } else if (rr < 90) it.k = NK.FERN; else it.k = FK.BLUEBERRY;
      } else if (n > 0.62 + veteranBias * -1) {                 // old growth
        if (rr < 50) { it.k = FK.TREE; it.size = 3; } else if (rr < 72) { it.k = FK.TREE; it.size = 2; }
        else if (rr < 77 && snagN < 6) { it.k = NK.SNAG; snagN++; } else if (rr < 84) it.k = NK.LOG; else if (rr < 88) it.k = FK.ROOTPLATE;
        else if (rr < 94) it.k = NK.FERN; else if (rr < 97) it.k = NK.MUSHROOM; else it.k = FK.BLUEBERRY;
      } else if (n < 0.34) {                                    // gap: windthrow + regeneration
        if (rr < 24) { it.k = FK.TREE; it.size = 0; } else if (rr < 44) { it.k = FK.TREE; it.size = 1; }
        else if (rr < 54) it.k = NK.BRAMBLE; else if (rr < 61) it.k = NK.LOG; else if (rr < 66) it.k = FK.ROOTPLATE; else if (rr < 72) it.k = FK.THORN;
        else if (rr < 84) it.k = NK.FERN; else if (rr < 90) it.k = FK.BLUEBERRY; else if (rr < 94) it.k = FK.HERB; else it.k = NK.MUSHROOM;
      } else {                                                  // mixed, all age classes
        if (rr < 34) { it.k = FK.TREE; it.size = 2; } else if (rr < 50) { it.k = FK.TREE; it.size = 1; } else if (rr < 58 + veteranBias * 40) { it.k = FK.TREE; it.size = 3; }
        else if (rr < 64) { it.k = FK.TREE; it.size = 0; } else if (rr < 72) it.k = NK.FERN; else if (rr < 76) it.k = NK.LOG; else if (rr < 78 && snagN < 6) { it.k = NK.SNAG; snagN++; }
        else if (rr < 84) it.k = FK.BLUEBERRY; else if (rr < 88) it.k = NK.MUSHROOM; else if (rr < 91) it.k = NK.STONES; else if (rr < 93) it.k = NK.ANTHILL; else it.k = NK.FERN;
      }
      // pioneer species in gaps and along the edge
      if (it.k === FK.TREE && it.size <= 1 && (n < 0.34 || ed < 3) && v % 3 === 0) it.spc = F_PIONEER[v % F_PIONEER.length];
      if (it.k === FK.TREE && it.size === 3) it.bend = 0.6; else if (it.k === FK.TREE) it.bend = it.size === 0 ? 1.2 : 0.8;
    } else {                                                    // clear-cut
      if (rr < 52) it.k = FK.STUMP; else if (rr < 60) it.k = FK.SLASH; else if (rr < 63) it.k = NK.LOG;
      else if (rr < 65 && ed > 2) { it.k = FK.TREE; it.size = 3; it.bend = 2.6; it.spc = v % 4 === 0 ? 2 : v % 4 === 1 ? 3 : 0; }   // Überhälter (~2 %), wind-bent
      else if (rr < 70 && polterN < 2 && ed > 2 && ed < 9) { it.k = FK.POLTER; polterN++; }
      else if (rr < 84) it.k = FK.STUMP; else if (rr < 90) it.k = NK.GRASS; else it.k = -1;
      if (it.k === NK.GRASS) it.s2 = 0;
      // regrowth: pops in with stand age (birth = cycle fraction 0..1)
      const rr2 = (m2 >>> 16) % 100, pr2 = ((m2 >>> 20) & 1023) / 1023;
      if (it.k === -1 || rr2 < 45) {
        const g = { lon: lon + ((m2 & 255) / 255 - 0.5) * sp * 0.6 / mLon, lat: lat + (((m2 >>> 8) & 255) / 255 - 0.5) * sp * 0.6 / mLat, v: (m2 >>> 26) & 63, ph: it.ph, pr: pr2, ed, n, k: -1, size: 0, bend: 1.2, spc: F_PIONEER[(m2 >>> 3) % F_PIONEER.length], birth: 0 };
        if (rr2 < 38) { g.k = FK.TREE; g.birth = 0.28 + (rr2 / 38) * 0.5; }
        else if (rr2 < 55) { g.k = NK.BRAMBLE; g.birth = 0.3 + ((rr2 - 38) / 17) * 0.4; }
        else if (rr2 < 75) { g.k = FK.HERB; g.birth = 0.15 + ((rr2 - 55) / 20) * 0.4; }     // Schlagflora: Fingerhut, Weidenröschen
        else if (rr2 < 90) { g.k = NK.GRASS; g.s2 = 1; g.birth = 0.1 + ((rr2 - 75) / 15) * 0.5; }
        if (g.k !== -1) items.push(g);
      }
      if (it.k === -1) continue;
    }
    // priority: trees + stumps uniform; ground cover thins first; landmarks stay
    if (it.k === NK.SNAG || it.k === FK.POLTER || it.k === FK.ROOTPLATE || it.k === NK.LOG) it.pr *= 0.4;
    else if (it.k === NK.FERN || it.k === FK.BLUEBERRY || it.k === NK.MUSHROOM || it.k === NK.STONES || it.k === NK.ANTHILL || it.k === NK.GRASS || it.k === FK.HERB) it.pr = 0.3 + it.pr * 0.7;
    else if (it.k === FK.TREE && it.size === 3) it.pr *= 0.7;
    items.push(it);
  }
  items.sort((a, c) => c.lat - a.lat);
  const snags = items.filter(it => it.k === NK.SNAG), stumps = items.filter(it => it.k === FK.STUMP), veterans = items.filter(it => it.k === FK.TREE && it.size === 3);
  sc = { items, hash, area, sp, b, mLon, mLat, wM, hM, snags, stumps, veterans, mode };
  FOREST.scenes.set(key, sc);
  return sc;
}

// ---- sprites (pixel units × u) ----
const F_PAL = {
  0: { d: '#174a22', m: '#1f5c2c', l: '#2e7a3a', trunk: '#4a3018' },   // spruce
  1: { d: '#1c5030', m: '#266a3c', l: '#3a8a50', trunk: '#5a4a38' },   // fir
  2: { d: '#4a7a2a', m: '#6aa040', l: '#9ac860', trunk: '#6a4a28' },   // larch
  3: { d: '#2a6a22', m: '#3e8a2c', l: '#6ab04a', trunk: '#6a6058' },   // beech
  4: { d: '#245a1c', m: '#367a28', l: '#5a9a3a', trunk: '#4a3a20' },   // oak
  5: { d: '#4a8a2c', m: '#6ab04a', l: '#a8dc70', trunk: '#e8e8e0' },   // birch
  6: { d: '#2e6a28', m: '#4a8a34', l: '#7ab84c', trunk: '#6a5a40' },   // rowan
  7: { d: '#3a7a28', m: '#5a9a3a', l: '#9ad060', trunk: '#5a4a30' },   // maple
};
/** Tree: species 0-7, size 0 sapling / 1 young / 2 mature / 3 veteran; lean = trunk-top wind offset (px units). */
function fTree(px, spc, size, lean, v) {
  const P = F_PAL[spc] || F_PAL[0];
  const conifer = spc <= 2;
  const trunkH = [4, 8, 14, 22][size], R = conifer ? [2, 4, 6, 9][size] : [2, 4, 6, 10][size], crownH = conifer ? [6, 12, 20, 32][size] : [4, 8, 12, 20][size];
  const o1 = Math.round(lean * 0.35), o2 = Math.round(lean);
  px(-R, 0, 2 * R + 1, 1, 'rgba(0,0,0,0.25)');
  // trunk in 3 segments (bends toward the tip)
  const tw = size >= 2 ? 2 : 1, h1 = Math.max(1, Math.round(trunkH * 0.4)), h2 = Math.max(1, Math.round(trunkH * 0.3)), h3 = Math.max(1, trunkH - h1 - h2);
  px(0, -h1, tw, h1, P.trunk); px(o1, -h1 - h2, tw, h2, P.trunk); px(o2, -trunkH, tw, h3, P.trunk);
  if (spc === 5) { px(0, -h1 + 1, 1, 1, '#303030'); if (size >= 2) px(o1, -h1 - 1, 1, 1, '#303030'); }   // birch bark
  else if (size >= 2) px(tw - 1, -h1, 1, h1, 'rgba(0,0,0,0.3)');
  const cx = o2 + (tw >> 1), top = -trunkH - crownH;
  if (conifer) {
    // stacked tiers, widening downward; larch airy (gaps between tiers)
    const tiers = size === 0 ? 3 : size === 1 ? 4 : size === 2 ? 6 : 8, th = crownH / tiers;
    for (let i = 0; i < tiers; i++) {
      const y0 = Math.round(top + i * th), w = Math.max(0, Math.round(R * (i + 1) / tiers));
      const hh = spc === 2 ? Math.max(1, Math.round(th * 0.6)) : Math.max(1, Math.round(th));
      px(cx - w, y0, 2 * w + 1, hh, i % 2 ? P.d : P.m);
      px(cx - w, y0, w, 1, P.l);                                  // lit left edge
      if (spc === 2 && size >= 2) px(cx + 1, y0, 1, hh, P.trunk); // larch: stem shows through
    }
    px(cx, top - 1, 1, 2, P.l);
    if (spc === 1 && size >= 2) px(cx - 1, top, 3, 1, P.l);       // fir: flat light top
    if (spc === 0 && size >= 2 && v % 3 === 0) { px(cx - R + 1, top + crownH - 4, 1, 2, '#6a3a20'); px(cx + R - 2, top + crownH - 7, 1, 2, '#6a3a20'); }  // cones
  } else {
    // rounded crown: rows from a circle, lit upper-left, shaded lower-right
    const cy = top + crownH / 2, ry = crownH / 2, rx = R + (spc === 4 ? 1 : 0);
    for (let dy = -ry; dy <= ry; dy++) {
      const w = Math.floor(rx * Math.sqrt(Math.max(0, 1 - (dy / ry) ** 2)));
      if (w < 0) continue;
      const y0 = Math.round(cy + dy);
      px(cx - w, y0, 2 * w + 1, 1, dy > ry * 0.35 ? P.d : P.m);
      if (dy < -ry * 0.2 && w > 1) px(cx - w + 1, y0, Math.max(1, w - 1), 1, P.l);
      if (spc === 4 && (dy + v) % 3 === 0) px(cx + w - 1 - (v % 2), y0, 1, 1, P.l);   // oak: ragged
    }
    if (spc === 6 && size >= 1) { px(cx - 1, Math.round(cy - 1), 1, 1, '#d03020'); px(cx + 2, Math.round(cy + 1), 1, 1, '#d03020'); px(cx, Math.round(cy + 2), 1, 1, '#d03020'); }  // rowan berries
    if (spc === 5 && size >= 2) for (let i = 0; i < 4; i++) px(cx - rx + 1 + ((v + i * 5) % (2 * rx - 1)), Math.round(top + 1 + ((v * 3 + i * 7) % crownH)), 1, 1, 'rgba(255,255,255,0.25)');   // birch: airy
    if (size === 3 && v % 4 === 0) px(cx - rx - 1, Math.round(cy + ry * 0.3), 1, 1, '#2a2a2a');   // a bird in the veteran
  }
}
function fStump(px, v) {
  const big = v % 3 === 0, w = big ? 7 : 5, h = big ? 3 : 2, L = -(w >> 1);
  px(L - 1, 0, w + 2, 1, 'rgba(0,0,0,0.25)');
  px(L, -h, w, h, '#6a4a2a'); px(L + w - 1, -h, 1, h, '#4a3018'); px(L, -h, 1, h, '#8a6a40');
  px(L, -h - 1, w, 1, '#c8a870'); px(L + 1, -h - 1, w - 2, 1, '#e0c890'); px(L + (w >> 1), -h - 1, 1, 1, '#a08050');   // cut face + ring + heart
  if (v % 4 === 1) px(L + w, -1, 2, 1, '#d8c090');                                            // sawdust
  if (v % 5 === 2) { px(L - 1, -1, 1, 1, '#3e8a2c'); px(L + w, -2, 1, 2, '#3e8a2c'); }        // moss
  if (v % 7 === 3) px(L + 1, -h - 2, 1, 1, '#c8b080');                                        // fungus
}
function fSlash(px, v) {
  px(-4, 0, 9, 1, 'rgba(0,0,0,0.18)');
  px(-4, -2, 9, 2, '#5a4020'); px(-3, -3, 7, 1, '#6a4a28'); px(-1, -4, 3, 1, '#6a4a28');
  px(-6, -2, 3, 1, '#4a3018'); px(3, -4, 3, 1, '#4a3018'); px(-2, -5, 1, 2, '#4a3018');
  if (v % 2) { px(-1, -3, 2, 1, '#3a6a2a'); px(2, -2, 2, 1, '#3a6a2a'); } else px(0, -4, 2, 1, '#8a6a30');   // needles / dry twigs
}
function fPolter(px, v) {
  // stacked logs seen end-on: 4 / 3 / 2 rows, cream cut faces with growth rings
  px(-8, 0, 17, 1, 'rgba(0,0,0,0.3)');
  const log = (x, y) => { px(x, y, 3, 3, '#a07840'); px(x + 1, y + 1, 1, 1, '#e8c888'); px(x, y, 1, 1, '#c8a060'); px(x + 2, y + 2, 1, 1, '#7a5830'); };
  for (let i = 0; i < 4; i++) log(-7 + i * 4, -3);
  for (let i = 0; i < 3; i++) log(-5 + i * 4, -6);
  for (let i = 0; i < 2; i++) log(-3 + i * 4, -9);
  px(-8, -2, 1, 3, '#4a3018'); px(8, -2, 1, 3, '#4a3018');   // stakes
  if (v % 2) { px(4, -11, 1, 1, '#2a2a2a'); px(3, -12, 1, 1, '#2a2a2a'); px(5, -12, 1, 1, '#2a2a2a'); }   // crow
}
function fRootplate(px, v) {
  // tipped-over root plate + fallen trunk (Windwurf)
  px(-5, 0, 18, 1, 'rgba(0,0,0,0.22)');
  px(-5, -7, 7, 7, '#5a3a20'); px(-4, -8, 5, 1, '#5a3a20'); px(-5, -6, 1, 5, '#7a5a38'); px(-4, -5, 2, 2, '#3a2010');
  px(-7, -6, 2, 1, '#6a4a28'); px(-6, -9, 1, 2, '#6a4a28'); px(2, -9, 1, 2, '#6a4a28'); px(3, -6, 2, 1, '#6a4a28');   // roots
  px(2, -3, 11, 3, '#8a6a48'); px(2, -3, 11, 1, '#a88a60'); px(2, -1, 11, 1, '#5a4028');                                // trunk
  if (v % 2) { px(5, -4, 2, 1, '#3e8a2c'); px(9, -4, 1, 1, '#3e8a2c'); }                                                // moss
}
function fBlueberry(px, v) {
  px(-3, -2, 7, 2, '#2e6a2e'); px(-2, -3, 5, 1, '#3a7a3a'); px(-3, -2, 1, 1, '#4a8a4a');
  px(-2 + (v % 3), -3, 1, 1, '#3a3a8a'); px(1, -2, 1, 1, '#3a3a8a'); if (v % 2) px(-3, -1, 1, 1, '#4a4aa0');
}
function fThorn(px, v) {
  px(-5, 0, 11, 1, 'rgba(0,0,0,0.2)');
  px(-5, -4, 11, 4, '#3a5a28'); px(-4, -6, 9, 2, '#466a30'); px(-2, -7, 5, 1, '#466a30'); px(-4, -6, 3, 1, '#5a7e3a');
  px(-6, -5, 1, 1, '#8a8a70'); px(5, -6, 1, 1, '#8a8a70'); px(-3, -8, 1, 1, '#8a8a70'); px(2, -8, 1, 1, '#8a8a70');   // thorns
  if (v % 2) { px(-2, -5, 1, 1, '#d03020'); px(2, -3, 1, 1, '#d03020'); } else { px(-1, -4, 1, 1, '#2a2a6a'); px(3, -5, 1, 1, '#2a2a6a'); }   // haws / sloes
}
function fHerb(px, v, w) {
  // Schlagflora: foxglove (pink spike) or fireweed (magenta spike)
  const o = Math.round(w * 0.8), col = v % 2 ? '#d060a0' : '#c04080', pale = v % 2 ? '#f0a0d0' : '#e080b0';
  px(0, -4, 1, 4, '#3e8a2c'); px(o, -8, 1, 4, '#3e8a2c'); px(-2, -2, 2, 1, '#3e8a2c'); px(1, -3, 2, 1, '#3e8a2c');
  px(o - 1, -9, 3, 1, col); px(o - 1, -11, 3, 2, col); px(o, -12, 1, 1, pale); px(o - 1, -10, 1, 1, pale);
}
function drawForestItem(ctx, it, x, y, u, w, t, grow) {
  x = Math.round(x); y = Math.round(y);
  const px = wildPx(ctx, x, y, u);
  switch (it.k) {
    case FK.TREE: {
      let size = it.size;
      if (grow != null) size = grow < 0.4 ? 0 : 1;              // regrowth on a clear-cut
      fTree(px, it.spc, size, w * it.bend, it.v); break;
    }
    case FK.STUMP: fStump(px, it.v); break;
    case FK.SLASH: fSlash(px, it.v); break;
    case FK.POLTER: fPolter(px, it.v); break;
    case FK.ROOTPLATE: fRootplate(px, it.v); break;
    case FK.BLUEBERRY: fBlueberry(px, it.v); break;
    case FK.THORN: fThorn(px, it.v); break;
    case FK.HERB: fHerb(px, it.v, w); break;
    default: drawNatureItem(ctx, it, x, y, u, w, t);
  }
}

// ---- fauna ----
function fDeer(ctx, x, y, u, t, dir) {
  const px = wildPx(ctx, Math.round(x), Math.round(y), u), d = dir, B = '#8a5a30', D = '#5a3a1a';
  px(-4, 0, 9, 1, 'rgba(0,0,0,0.2)');
  px(-3, -6, 7, 3, B); px(-3, -4, 1, 1, D);
  const st = Math.floor(t * 6) % 2;
  px(-3 + st, -3, 1, 3, D); px(2 - st, -3, 1, 3, D); px(-2, -3, 1, 2, D); px(3, -3, 1, 2, D);
  px(3 * d, -9, 2, 3, B); px(4 * d, -10, 2, 2, B); px(4 * d, -11, 1, 1, D); px(3 * d + 1, -10, 1, 1, '#1a1008');
  px(-4 * d, -6, 1, 1, '#f0e8d8');                                  // tail
}
function fWoodpecker(px, t, ph) {
  const peck = Math.sin(t * 18 + ph) > 0.3 ? 1 : 0;
  px(2, -16, 2, 3, '#202020'); px(2, -14, 1, 1, '#f0f0f0'); px(3, -17, 1, 1, '#d02020'); px(3 + peck, -16, 1, 1, '#202020'); px(2, -13, 1, 1, '#d02020');
}
function fJay(ctx, x, y, u, t, i) {
  const px = wildPx(ctx, Math.round(x), Math.round(y), u), fl = Math.sin(t * 9 + i) > 0 ? -1 : 1;
  px(-1, 0, 3, 1, '#c8a080'); px(-3, fl, 2, 1, '#4a80c0'); px(2, fl, 2, 1, '#4a80c0'); px(1, -1, 1, 1, '#202020');
}
function drawForestFauna(ctx, sc, u, t, lvl, sx1, sy1, sx2, sy2) {
  if (lvl < 1) return;
  // woodpecker on a snag
  for (const sn of sc.snags) if (Math.sin(t * 0.09 + sn.ph) > 0) { const [x, y] = toScreen(sn.lon, sn.lat); fWoodpecker(wildPx(ctx, Math.round(x), Math.round(y), u), t, sn.ph); }
  // crows on stumps of a fresh clear-cut
  if (sc.mode === 'schlag') for (let i = 0; i < Math.min(3, sc.stumps.length); i++) {
    const s = sc.stumps[(sc.hash + i * 41) % sc.stumps.length];
    if (Math.sin(t * 0.2 + i * 2) < 0.4) continue;
    const [x, y] = toScreen(s.lon, s.lat), px = wildPx(ctx, Math.round(x), Math.round(y), u);
    px(-1, -5, 3, 1, '#202020'); px(0, -6, 1, 1, '#202020'); px(1 + (Math.sin(t * 5 + i) > 0.5 ? 1 : 0), -5, 1, 1, '#404040');
  }
  if (lvl === 2 && sc.area > 1200) {
    // a deer crosses every ~60 s (edge → edge), a jay flits over the canopy
    const cyc = (t + (sc.hash % 60)) % 60;
    if (cyc < 7) { const ph = cyc / 7, dir = sc.hash & 1 ? 1 : -1; fDeer(ctx, sx1 + (sx2 - sx1) * (dir > 0 ? ph : 1 - ph), sy1 + (sy2 - sy1) * (0.3 + 0.4 * ((sc.hash >>> 5) % 100) / 100), u, t, dir); }
    const jp = (t * 0.07 + (sc.hash % 100) / 100) % 1;
    fJay(ctx, sx1 + (sx2 - sx1) * jp, sy1 + (sy2 - sy1) * (0.25 + 0.3 * Math.sin(jp * 5)) - 26 * u, u, t, sc.hash & 7);
  }
}

/** Living overlay for Naturwald + clear-cut stands. Drawn after drawNatureReserves. */
function drawForestOverlay(ctx, claimMap) {
  const zoom = G.cam.zoom;
  if (zoom < 15 || !G.parcelPolys.length) return;
  const t0 = performance.now();
  const u = zoom > 19 ? 3 : zoom > 17.5 ? 2 : 1, lvl = natureAnimLevel(), t = lvl ? Date.now() / 1000 : 0;
  const W = gc.width, H = gc.height, cell = 9 * u, now = Date.now();
  ctx.save();
  for (const f of G.parcelPolys) {
    const claim = claimMap[f.properties.parcel_id];
    if (!claim || !isAreaGeom(f.geometry)) continue;
    let mode = null, fs = null;
    if (claim.converted_to === 'wildforest') mode = 'wild';
    else if (!claim.converted_to && claim.harvested_at && claimIsForest(f.properties, claim)) { fs = forestStage(claim, now); if (fs.stage !== 'baumholz') mode = 'schlag'; }
    if (!mode) continue;
    const b = geoBounds(f.geometry);
    const [sx1, sy1] = toScreen(b.w, b.n), [sx2, sy2] = toScreen(b.e, b.s);
    if (sx2 < -40 || sx1 > W + 40 || sy2 < -40 || sy1 > H + 40) continue;
    if ((sx2 - sx1) < 8 || (sy2 - sy1) < 6) continue;
    NATURE.onScreen++;
    const sc = forestScene(f, mode);
    const pxPerM = mapScale() / sc.mLon;
    const frac = Math.min(1, (sc.sp * pxPerM / cell) ** 2) * NATURE.quality * (lvl === 1 ? 0.7 : 1);
    for (const it of sc.items) {
      if (it.pr > frac) continue;
      let grow = null;
      if (fs) { if (it.birth) { if (fs.t < it.birth) continue; grow = (fs.t - it.birth) / Math.max(0.05, 1 - it.birth); if (it.k !== FK.TREE) grow = null; } }
      const [x, y] = toScreen(it.lon, it.lat);
      if (x < -30 || x > W + 30 || y < -60 || y > H + 30) continue;
      const w = lvl ? windAt(x, y, t) * 2.2 : 0;
      drawForestItem(ctx, it, x, y, u, w, t, grow);
    }
    drawForestFauna(ctx, sc, u, t, lvl, Math.max(sx1, 0), Math.max(sy1, 0), Math.min(sx2, W), Math.min(sy2, H));
  }
  ctx.restore();
  const cost = performance.now() - t0;
  NATURE._cost = NATURE._cost * 0.8 + cost * 0.2;
  if (NATURE._cost > 9 && NATURE.quality > 0.3) NATURE.quality = Math.max(0.3, NATURE.quality * 0.9);
}

/** My forest stands (for quest briefings). */
function myForests() {
  const out = [];
  for (const c of G.claimed || []) {
    if (c.player_id !== G.player?.id || c.converted_to) continue;
    const f = polyById(c.parcel_id);
    if (!f || !claimIsForest(f.properties, c)) continue;
    out.push({ c, f, ll: featureLonLat(f), fs: forestStage(c) });
  }
  return out;
}

window.doHarvestForest = async function() {
  if (!G.sel) return;
  const p = G.sel.properties;
  const res = await POST('/api/harvest-forest', { session_id: G.session.id, player_id: G.player.id, parcel_id: p.parcel_id });
  if (res.error) { toast(res.error, 'err'); return; }
  const [lon, lat] = featureLonLat(G.sel);
  spawnCollectFX({ lon, lat }, '+' + res.coins + ' 🪙', TREASURE_RARITY.coins);
  toast('🪓 ' + tr('Holz geerntet') + ': ' + Math.round(res.efm) + ' Efm → +' + res.coins + '🪙 +' + res.xp + '⚡', 'ok');
  G.player = res.player; updateStats();
  FOREST.scenes.delete(p.parcel_id + ':schlag');
  delete G.forestValues[p.parcel_id];
  await loadClaimed(); render(); showParcelPopup(G.sel, G.selFp); loadChallenges();
};

function drawForestSprites(ctx, claimMap) {
  // Draw tree sprites on forest, reforested, orchard and scrub parcels
  // Determine tree style per parcel: 'forest' | 'reforested' | 'orchard' | 'krummholz'
  // Minimum lidar-measured wooded fraction to scatter sprites on a parcel whose
  // cadastre/dominant terrain is NOT forest/garden (e.g. a meadow with a copse).
  const WOOD_MIN = 0.15;
  function getTreeStyle(f) {
    const claim = claimMap[f.properties.parcel_id];
    if (claim?.converted_to === 'forest') return 'reforested';
    // Naturwald: the living overlay draws the stand at zoom ≥ 15; below that
    // fall back to the plain forest sprites so it never reads as bare ground.
    if (claim?.converted_to === 'wildforest') return G.cam.zoom < 15 ? 'forest' : null;
    if (claim && !claim.converted_to && claim.harvested_at && claimIsForest(f.properties, claim)) {
      const fs = forestStage(claim);
      if (fs.stage !== 'baumholz') return null;            // stumps / regrowth → drawForestOverlay
      if (fs.factor < 1) return 'young';                    // regrown, not yet full value
    }
    const t = getParcelTerrain(f.properties, claim);
    const veg = parcelVeg(f);
    // Scrub-dominant (low woody cover, little tall canopy) → krummholz sprites,
    // regardless of cadastre terrain. srtm distinguishes shrub/hedge from tree.
    if (veg && veg.wood >= WOOD_MIN && veg.shrub > veg.tree && veg.tree < 0.15) {
      return 'krummholz';
    }
    if (t !== TERRAIN.forest && t !== TERRAIN.garden) {
      // Enhanced mode: trust lidar canopy — a partly-wooded parcel still gets
      // (proportionally sparse) trees even if forest isn't its dominant cover.
      if (veg && veg.wood >= WOOD_MIN) return veg.tree >= 0.1 ? 'forest' : 'krummholz';
      return null;
    }
    // Terrain says forest/garden but lidar says (nearly) treeless → suppress sprites.
    if (veg && veg.wood < 0.05) return null;
    const luCode = extractLuCode('', f.properties);
    if (luCode === '40') return 'orchard';   // Dauerkulturanlagen / Erwerbsgärten
    if (luCode === '55') return 'krummholz'; // Krummholzflächen (Latschen)
    if (luCode === '57') return 'krummholz'; // Verbuschte Flächen
    // Heuristic: large pure-forest parcel with no buildings → plantation likely
    if (t === TERRAIN.forest) {
      const area = f.properties.area_sqm || 0;
      const bc = f.properties.building_count || 0;
      // Large mono parcels (>2ha) with no buildings = plantation look
      if (area > 20000 && bc === 0) return 'plantation';
      return 'forest';
    }
    return null;
  }

  const treePolys = G.parcelPolys.map(f => ({ f, style: getTreeStyle(f) })).filter(x => x.style);

  ctx.save();
  for (const { f, style } of treePolys) {
    const coords = geomOuterRings(f.geometry);
    if (!coords.length) continue;
    const b = geoBounds(f.geometry);
    const [sx1,sy1] = toScreen(b.w, b.n);
    const [sx2,sy2] = toScreen(b.e, b.s);
    if (sx2 < 0 || sx1 > gc.width || sy2 < 0 || sy1 > gc.height) continue;

    const area = f.properties.area_sqm || 1000;
    const hash = simpleHash(f.properties.parcel_id||'');
    // Lidar-measured vegetation (null when no lidar coverage). Scale sprite
    // density by the relevant cover: total woody cover for scrub/krummholz,
    // tall-canopy fraction for real forest. Floor keeps sparse parcels legible.
    const veg = parcelVeg(f);
    let densFrac = null;
    if (veg) densFrac = (style === 'krummholz') ? veg.wood : veg.tree || veg.wood;
    const densMul = densFrac == null ? 1 : Math.max(0.25, Math.min(1, densFrac));

    let treeCount, variantFn;
    if (style === 'orchard') {
      // Orchards: sparser, always fruit tree
      treeCount = Math.min(12, Math.max(2, Math.floor(area / 500)));
      variantFn = (i) => 4;
    } else if (style === 'krummholz') {
      // Krummholz: dense low scrub
      treeCount = Math.min(20, Math.max(3, Math.floor(area / 250)));
      variantFn = (i) => 2;
    } else if (style === 'young') {
      // Regrown stand after a harvest: even-aged young trees, birch pioneers
      treeCount = Math.min(30, Math.max(4, Math.floor(area / 220)));
      const rv = [3, 5, 6, 3, 7, 5, 3, 6];
      variantFn = (i) => rv[(hash + i) % rv.length];
    } else if (style === 'reforested') {
      // Reforested: dense mix of saplings, young firs, birch — vibrant new growth
      treeCount = Math.min(28, Math.max(4, Math.floor(area / 200)));
      const rv = [3, 5, 3, 6, 3, 5, 3, 6, 5, 3]; // heavy on saplings + birch
      variantFn = (i) => rv[(hash + i) % rv.length];
    } else if (style === 'plantation') {
      // Managed forest — very dense, rows of conifers
      treeCount = Math.min(35, Math.max(6, Math.floor(area / 180)));
      variantFn = (i) => (hash + i) % 3 === 0 ? 5 : 7;
    } else {
      // Normal mixed forest: oak, beech, fir, birch — dense Austrian Mischwald
      treeCount = Math.min(30, Math.max(4, Math.floor(area / 250)));
      const v = [0, 1, 5, 6, 1, 0, 5, 1, 0, 6]; // weighted toward deciduous
      variantFn = (i) => v[(hash + i) % v.length];
    }

    // Scale by lidar-measured canopy fraction (skip 'reforested' — that's a
    // game-state look, not a measured natural stand). Always keep ≥1 tree.
    if (style !== 'reforested' && densMul < 1) {
      treeCount = Math.max(1, Math.round(treeCount * densMul));
    }

    // Draw bright green underglow for reforested parcels
    if (style === 'reforested') {
      ctx.beginPath();
      for (const ring of coords) {
        const pts = ring.map(c => toScreen(c[0], c[1]));
        pts.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
        ctx.closePath();
      }
      // Vibrant green overlay with pulse
      const pulse = 0.12 + Math.sin(Date.now()/1200 + hash) * 0.04;
      ctx.fillStyle = `rgba(80,220,60,${pulse})`;
      ctx.fill();
      // Sparkle border to show active growth
      ctx.strokeStyle = 'rgba(120,255,80,0.4)';
      ctx.lineWidth = 2;
      ctx.setLineDash([4, 6]);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    for (let i = 0; i < treeCount; i++) {
      const t = (hash + i * 7919) % 10000 / 10000;
      const u = (hash + i * 3571) % 10000 / 10000;
      const lon = b.w + (b.e - b.w) * t;
      const lat = b.s + (b.n - b.s) * u;
      if (!pipRings(lon, lat, coords)) continue;
      const [tx, ty] = toScreen(lon, lat);
      drawTree(ctx, tx, ty, variantFn(i), hash + i);
    }

    // Draw growth indicators on reforested parcels (small sprouts between trees)
    if (style === 'reforested') {
      const sproutCount = Math.min(8, Math.max(2, Math.floor(area / 1000)));
      for (let i = 0; i < sproutCount; i++) {
        const t = (hash + (i+50) * 6151) % 10000 / 10000;
        const u = (hash + (i+50) * 4337) % 10000 / 10000;
        const lon2 = b.w + (b.e - b.w) * t;
        const lat2 = b.s + (b.n - b.s) * u;
        if (!pipRings(lon2, lat2, coords)) continue;
        const [sx, sy] = toScreen(lon2, lat2);
        drawSprout(ctx, sx, sy, hash + i);
      }
    }
  }
  ctx.restore();
}

function drawSprout(ctx, x, y, seed) {
  // Small bright green sprout — sign of new growth
  const s = G.cam.zoom > 16 ? 1.0 : 0.6;
  const sway = Math.sin(Date.now()/1500 + seed*0.4) * 1.0 * s;
  // Stem
  ctx.strokeStyle = '#4ac838';
  ctx.lineWidth = 1.2;
  ctx.beginPath();
  ctx.moveTo(x, y);
  ctx.quadraticCurveTo(x + sway, y - 5*s, x + sway*0.5, y - 8*s);
  ctx.stroke();
  // Two tiny leaves
  ctx.fillStyle = '#5edc4a';
  ctx.beginPath();
  ctx.ellipse(x + sway*0.5 - 2*s, y - 7*s, 2.5*s, 1.2*s, -0.5, 0, Math.PI*2);
  ctx.fill();
  ctx.fillStyle = '#4ecc3a';
  ctx.beginPath();
  ctx.ellipse(x + sway*0.5 + 2*s, y - 8*s, 2.5*s, 1.2*s, 0.5, 0, Math.PI*2);
  ctx.fill();
  // Tiny dewdrop sparkle
  if ((seed % 3) === 0) {
    ctx.fillStyle = 'rgba(255,255,255,0.6)';
    ctx.beginPath();
    ctx.arc(x + sway*0.5 - 1*s, y - 8.5*s, 0.8*s, 0, Math.PI*2);
    ctx.fill();
  }
}

function drawTree(ctx, x, y, variant, seedOffset) {
  // Settlers IV style trees — warm, chunky, painterly
  // Variants: 0=oak, 1=beech, 2=bush/krummholz, 3=young sapling,
  //           4=fruit tree, 5=fir (Tanne), 6=birch, 7=mixed conifer
  const scale = G.cam.zoom > 16 ? 1.2 : 0.8;
  x = Math.round(x);
  y = Math.round(y);

  const t = (Date.now() / 3000 + (seedOffset||0) * 0.37) % (Math.PI * 2);
  const sway = Math.sin(t) * 0.8 * scale;

  // Shadow
  ctx.fillStyle = 'rgba(0,0,0,0.12)';
  ctx.beginPath();
  ctx.ellipse(x+2, y+1, 6*scale, 2.5*scale, 0.2, 0, Math.PI*2);
  ctx.fill();

  if (variant === 0) {
    // Oak — thick trunk, big lumpy canopy (Settlers IV classic)
    ctx.fillStyle = '#5a3a1a';
    ctx.fillRect(x-1.5*scale, y-5*scale, 3*scale, 6*scale);
    // Main canopy — layered circles for lumpy look
    ctx.fillStyle = '#2a6a22';
    ctx.beginPath(); ctx.arc(x+sway*0.2, y-13*scale, 8*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#348a2c';
    ctx.beginPath(); ctx.arc(x-3*scale+sway*0.3, y-15*scale, 5.5*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#3c9232';
    ctx.beginPath(); ctx.arc(x+4*scale+sway*0.2, y-14*scale, 5*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#449a38';
    ctx.beginPath(); ctx.arc(x+sway*0.4, y-17*scale, 4*scale, 0, Math.PI*2); ctx.fill();
    // Highlight
    ctx.fillStyle = 'rgba(120,200,80,0.2)';
    ctx.beginPath(); ctx.arc(x-2*scale, y-16*scale, 3*scale, 0, Math.PI*2); ctx.fill();
  } else if (variant === 1) {
    // Beech — smooth oval canopy, warm green
    ctx.fillStyle = '#5a3a20';
    ctx.fillRect(x-1*scale, y-4*scale, 2*scale, 5*scale);
    ctx.fillStyle = '#3a8228';
    ctx.beginPath(); ctx.ellipse(x+sway*0.2, y-13*scale, 7*scale, 9*scale, 0, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#4a9438';
    ctx.beginPath(); ctx.ellipse(x-2*scale+sway*0.3, y-15*scale, 5*scale, 6*scale, -0.2, 0, Math.PI*2); ctx.fill();
    // Dappled light
    ctx.fillStyle = 'rgba(140,210,80,0.2)';
    ctx.beginPath(); ctx.arc(x-3*scale, y-16*scale, 2.5*scale, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+2*scale, y-12*scale, 2*scale, 0, Math.PI*2); ctx.fill();
  } else if (variant === 2) {
    // Bush / Krummholz — low, wide, multiple lumps
    ctx.fillStyle = '#4a6a20';
    ctx.beginPath(); ctx.ellipse(x, y-4*scale, 8*scale, 5*scale, 0, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#5a7a28';
    ctx.beginPath(); ctx.arc(x-4*scale, y-6*scale, 4*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#3a5a18';
    ctx.beginPath(); ctx.arc(x+3*scale, y-5*scale, 3.5*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#6a8a30';
    ctx.beginPath(); ctx.arc(x, y-7*scale, 3*scale, 0, Math.PI*2); ctx.fill();
  } else if (variant === 3) {
    // Young sapling — thin, light green, hopeful
    ctx.fillStyle = '#6a4a28';
    ctx.fillRect(x-0.5*scale, y-3*scale, 1*scale, 4*scale);
    ctx.fillStyle = '#48a838';
    ctx.beginPath(); ctx.ellipse(x+sway*0.3, y-10*scale, 4*scale, 6*scale, 0, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#58b848';
    ctx.beginPath(); ctx.arc(x-1*scale+sway*0.4, y-12*scale, 3*scale, 0, Math.PI*2); ctx.fill();
  } else if (variant === 4) {
    // Fruit tree — round, with visible fruit
    ctx.fillStyle = '#7a4a20';
    ctx.fillRect(x-1*scale, y-5*scale, 2*scale, 6*scale);
    ctx.fillStyle = '#3a8a2a';
    ctx.beginPath(); ctx.arc(x+sway*0.2, y-13*scale, 7*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#4a9a38';
    ctx.beginPath(); ctx.arc(x-2*scale, y-15*scale, 5*scale, 0, Math.PI*2); ctx.fill();
    // Fruit — red apples or pinkish blossoms
    const fruitColors = ['#d04040','#e06040','#d05050','#c83838','#e05858'];
    for (let i = 0; i < 4; i++) {
      const fx = x + ((seedOffset+i)*7%11 - 5) * scale;
      const fy = y - (10 + (seedOffset+i)*3%6) * scale;
      ctx.fillStyle = fruitColors[(seedOffset+i)%5];
      ctx.beginPath(); ctx.arc(fx, fy, 1.5*scale, 0, Math.PI*2); ctx.fill();
    }
  } else if (variant === 5) {
    // Fir / Tanne — classic conifer but rounder and warmer than a spruce
    ctx.fillStyle = '#4a2e10';
    ctx.fillRect(x-1*scale, y-3*scale, 2*scale, 4*scale);
    // Rounded triangular tiers with warm dark green
    const tierColors = ['#1a4e1a','#1e5a1e','#226622','#2a7228'];
    const tierW = [5, 7.5, 10, 12];
    const tierH = [5, 6, 7, 6];
    for (let i = 3; i >= 0; i--) {
      const yo = y - 5*scale - i*6*scale;
      ctx.fillStyle = tierColors[i];
      // Rounded triangle using a curved path
      const hw = tierW[i]*scale*0.5;
      const th = tierH[i]*scale;
      ctx.beginPath();
      ctx.moveTo(x+sway*(i*0.1), yo - th);
      ctx.quadraticCurveTo(x + hw*0.3+sway*(i*0.1), yo - th*0.3, x + hw+sway*(i*0.05), yo);
      ctx.quadraticCurveTo(x+sway*(i*0.1), yo + 1*scale, x - hw+sway*(i*0.05), yo);
      ctx.quadraticCurveTo(x - hw*0.3+sway*(i*0.1), yo - th*0.3, x+sway*(i*0.1), yo - th);
      ctx.fill();
    }
    // Snow cap on top (subtle light highlight)
    ctx.fillStyle = 'rgba(140,200,100,0.2)';
    ctx.beginPath(); ctx.arc(x+sway*0.4, y-28*scale, 2.5*scale, 0, Math.PI*2); ctx.fill();
  } else if (variant === 6) {
    // Silver Birch — white trunk, airy light canopy
    ctx.fillStyle = '#d8d4c8';
    ctx.fillRect(x-1*scale+sway*0.05, y-4*scale, 2*scale, 6*scale);
    // Bark marks
    ctx.fillStyle = '#555';
    ctx.fillRect(x-0.8*scale+sway*0.05, y-2*scale, 1.6*scale, 0.8*scale);
    ctx.fillRect(x-0.8*scale+sway*0.05, y-5*scale, 1.6*scale, 0.6*scale);
    // Airy canopy — transparent, warm yellow-green
    ctx.fillStyle = 'rgba(150,200,70,0.6)';
    ctx.beginPath(); ctx.arc(x+sway, y-14*scale, 6*scale, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = 'rgba(170,220,80,0.45)';
    ctx.beginPath(); ctx.arc(x-3*scale+sway, y-12*scale, 4*scale, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+4*scale+sway, y-13*scale, 3.5*scale, 0, Math.PI*2); ctx.fill();
    // Leaf shimmer
    ctx.fillStyle = 'rgba(200,240,100,0.15)';
    ctx.beginPath(); ctx.arc(x-2*scale+sway, y-16*scale, 2.5*scale, 0, Math.PI*2); ctx.fill();
  } else if (variant === 7) {
    // Mixed conifer group — two trees close together, different heights
    const offsets = [-3.5*scale, 3.5*scale];
    const heights = [0.9, 1.1];
    for (let j = 0; j < 2; j++) {
      const ox = x + offsets[j];
      const hs = heights[j];
      // Trunk
      ctx.fillStyle = '#4a3018';
      ctx.fillRect(ox-0.8*scale, y-3*scale, 1.5*scale, 4*scale);
      // Rounded tiers
      const tc = j===0 ? ['#1a4e1a','#1e5820','#226222'] : ['#1e5a22','#266228','#2a6e2e'];
      for (let i = 0; i < 3; i++) {
        const tw = (4 + i*2.5) * scale * hs;
        const th = (4 + i) * scale * hs;
        const yo = y - 5*scale - i*5.5*scale*hs;
        ctx.fillStyle = tc[i];
        ctx.beginPath();
        ctx.moveTo(ox + sway*(i*0.08), yo - th);
        ctx.quadraticCurveTo(ox + tw*0.5, yo, ox - tw*0.5, yo);
        ctx.quadraticCurveTo(ox, yo - th*0.5, ox + sway*(i*0.08), yo - th);
        ctx.fill();
      }
    }
  }
}

function drawTriangle(ctx, cx, top, w, h) {
  ctx.beginPath();
  ctx.moveTo(cx, top);
  ctx.lineTo(cx - w/2, top + h);
  ctx.lineTo(cx + w/2, top + h);
  ctx.closePath();
  ctx.fill();
}

// ================= TREASURE MARKERS =================
// Every treasure shares one "collectible" presentation so it reads on any
// terrain and any screen: ground shadow → rarity glow → translucent medallion
// with a crisp rarity rim → the sprite (bobbing) → orbiting sparkles → name tag
// (zoom ≥ 16.5). Red-list category colours the rim (EN red, VU orange, NT blue,
// LC green); chests are gold, XP gems cyan. N2K bonus finds get a rotating
// dashed gold ring. Sizes are in CSS px and get a 15% boost on touch devices;
// the hit radius is never below 22px (44px target). Collect FX: burst + float.
const TREASURE_RARITY = {
  EN: {rim:'#ff5a4a', glow:'255,90,70',  name:'Stark gefährdet'},
  VU: {rim:'#ffb830', glow:'255,184,48', name:'Gefährdet'},
  NT: {rim:'#6cc4ff', glow:'108,196,255',name:'Potenziell gefährdet'},
  LC: {rim:'#7ee07e', glow:'126,224,126',name:'Nicht gefährdet'},
  coins: {rim:'#ffd24a', glow:'255,210,74', name:'Schatz'},
  xp:    {rim:'#5ee6ff', glow:'94,230,255', name:'Erfahrung'},
  rare_seed:   {rim:'#9be86a', glow:'155,232,106', name:'Seltener Samen'},
  ancient_map: {rim:'#e0c080', glow:'224,192,128', name:'Alte Karte'},
};
let _coarsePointer = null;
function isCoarsePointer() {
  if (_coarsePointer == null) { try { _coarsePointer = !!(window.matchMedia && matchMedia('(pointer: coarse)').matches); } catch (e) { _coarsePointer = false; } }
  return _coarsePointer;
}
function treasureRarity(t) {
  if (t.treasure_type === 'species' || t.treasure_type === 'n2k_species') return TREASURE_RARITY[t.species_category] || TREASURE_RARITY.LC;
  return TREASURE_RARITY[t.treasure_type] || TREASURE_RARITY.coins;
}
// Treasure sprites grow with zoom so they stay findable/tappable at street level.
function treasureScale() {
  const z = G.cam.zoom;
  let s = z > 18 ? 2.2 : z > 17 ? 1.8 : z > 16 ? 1.45 : z > 15 ? 1.2 : 1.05;
  if (isCoarsePointer()) s *= 1.15;
  return s;
}
function treasureHitRadius() { return Math.max(isCoarsePointer() ? 26 : 22, 15 * treasureScale()); }
let _treasuresOnScreen = 0;
let _ripeOnScreen = 0;

// "Ernten!" markers over the player's own ripe fields: a bobbing sheaf on a
// dithered gold ring (same visual grammar as treasures) with a pixel arrow.
// Only own, unharvested, ripe crop fields — NPC fields work themselves.
function drawRipeMarkers(ctx, claimMap) {
  _ripeOnScreen = 0;
  if (!G.player || G.cam.zoom < 14) return;
  const now = Date.now();
  for (const c of G.claimed) {
    if (c.player_id !== G.player.id || c.converted_to || c.landuse !== '48') continue;
    const f = polyById(c.parcel_id);
    const fs = fieldStage({parcel_id: c.parcel_id, landuse_summary: '48'}, c, now);
    if (fs.stage !== 'ripe') continue;
    const ll = f ? featureLonLat(f) : null;
    if (!ll) continue;
    const [x, y] = toScreen(ll[0], ll[1]);
    if (x < -40 || x > gc.width + 40 || y < -40 || y > gc.height + 40) continue;
    _ripeOnScreen++;
    const s = Math.min(1.8, treasureScale());
    const bob = Math.sin(now / 600 + simpleHash(c.parcel_id) % 7) * 2.5 * s;
    ctx.save();
    // dithered ground ring
    const R = 13 * s;
    ctx.fillStyle = 'rgba(255,210,74,0.85)';
    for (let a = 0; a < 40; a++) {
      if (a % 2) continue;
      const ang = a / 40 * Math.PI * 2;
      ctx.fillRect(Math.round(x + Math.cos(ang) * R) - 1, Math.round(y + Math.sin(ang) * R * 0.5) - 1, 2, 2);
    }
    ctx.save(); ctx.translate(0, bob - 4 * s); ctx.scale(s, s); ctx.translate(x / s, y / s);
    drawCropSprite(ctx, 0, 0, 'sheaf', simpleHash(c.parcel_id));
    ctx.restore();
    // bouncing pixel arrow
    const ay = y - 24 * s + bob * 1.4;
    ctx.fillStyle = '#ffd24a';
    ctx.fillRect(Math.round(x) - 1, Math.round(ay) - 6, 2, 5);
    ctx.fillRect(Math.round(x) - 3, Math.round(ay) - 2, 6, 2);
    ctx.fillRect(Math.round(x) - 1, Math.round(ay), 2, 2);
    if (G.cam.zoom >= 15.5) {
      ctx.font = MAP_FONT.pixel; ctx.textAlign = 'center'; ctx.textBaseline = 'bottom';
      const label = tr('Ernten!');
      ctx.fillStyle = '#000'; ctx.fillText(label, Math.round(x) + 1, Math.round(ay) - 8);
      ctx.fillStyle = '#ffd24a'; ctx.fillText(label, Math.round(x), Math.round(ay) - 9);
    }
    ctx.restore();
  }
}
function treasurePhase(t) { return ((t.id || 0) * 0.73) % (Math.PI * 2); }

function drawTreasure(ctx, t) {
  const [x, y] = toScreen(t.lon, t.lat);
  const s = treasureScale();
  const m = 40 * s;
  if (x < -m || x > gc.width + m || y < -m || y > gc.height + m) return;
  _treasuresOnScreen++;
  const time = Date.now();
  const ph = treasurePhase(t);
  const bob = Math.sin(time / 650 + ph) * 2.5 * s;
  const rar = treasureRarity(t);
  const isSpecies = (t.treasure_type === 'species' || t.treasure_type === 'n2k_species') && t.species_name;
  const R = 15 * s;                 // medallion radius
  const cy = y - 6 * s + bob;       // medallion centre (sprite floats above its shadow)

  ctx.save();
  // Settlers-era presentation: no plate, no gradients. The creature/chest
  // stands on the land; underneath lies a *dithered* isometric ring of light
  // (checkerboard pixels in the rarity colour, like the 2000s selection
  // circles), and a small bouncing pixel arrow hovers above — the classic
  // "look here" cue. Everything is snapped to a pixel unit `u`.
  const u = Math.max(1, Math.round(s));
  const xi = Math.round(x), yi = Math.round(y);
  const pulse = 0.5 + 0.5 * Math.sin(time / 800 + ph);

  // Dithered ground ring: ellipse rx=R, ry=0.45R; ring band 3u; checkerboard
  const rx = Math.round(R + 2 * u), ry = Math.round(rx * 0.45);
  const band = 3 * u;
  const spin = Math.floor(time / 250) % 2;             // slowly crawling dither
  ctx.fillStyle = rar.rim;
  for (let py = -ry; py <= ry; py += u) {
    for (let pxx = -rx; pxx <= rx; pxx += u) {
      const d = Math.hypot(pxx / rx, py / ry);          // 0 centre … 1 rim
      const inner = 1 - band / rx;
      if (d > 1 || d < inner) continue;
      const cell = ((pxx / u + py / u + spin) & 1) === 0;
      const edge = d > 1 - u / rx;                       // outer edge row solid
      if (cell || edge) ctx.fillRect(xi + pxx, yi + 3 * u + py, u, u);
    }
  }
  // Soft fill inside the ring (sparse dither, breathes with pulse)
  ctx.fillStyle = 'rgba(' + rar.glow + ',' + (0.25 + pulse * 0.2).toFixed(2) + ')';
  for (let py = -ry; py <= ry; py += 2 * u) {
    for (let pxx = -rx; pxx <= rx; pxx += 2 * u) {
      const d = Math.hypot(pxx / rx, py / ry);
      if (d < 1 - band / rx && (((pxx / u + py / u) >> 1) & 1) === spin) ctx.fillRect(xi + pxx, yi + 3 * u + py, u, u);
    }
  }
  // Contact shadow directly under the sprite
  ctx.fillStyle = 'rgba(0,0,0,0.35)';
  const shw = Math.round(7 * s);
  ctx.fillRect(xi - shw, yi + 2 * u, shw * 2, u); ctx.fillRect(xi - shw + 2 * u, yi + u, shw * 2 - 4 * u, u); ctx.fillRect(xi - shw + 2 * u, yi + 3 * u, shw * 2 - 4 * u, u);

  // Natura-2000 bonus: second, wider gold dither ring rotating the other way
  if (t.treasure_type === 'n2k_species') {
    const rx2 = rx + 5 * u, ry2 = Math.round(rx2 * 0.45);
    ctx.fillStyle = '#ffd84a';
    for (let py = -ry2; py <= ry2; py += u) for (let pxx = -rx2; pxx <= rx2; pxx += u) {
      const d = Math.hypot(pxx / rx2, py / ry2);
      if (d > 1 || d < 1 - 2 * u / rx2) continue;
      if (((pxx / u + py / u + 1 - spin) & 1) === 0) ctx.fillRect(xi + pxx, yi + 3 * u + py, u, u);
    }
  }

  // Bouncing pixel arrow above the sprite (rarity colour, dark outline)
  const ay = Math.round(cy - R - 6 * s - Math.abs(Math.sin(time / 350 + ph)) * 4 * s);
  const arrow = (col, o) => {
    ctx.fillStyle = col;
    ctx.fillRect(xi - 4 * u - o, ay - 6 * u - o, 8 * u + 2 * o, 3 * u + 2 * o);
    ctx.fillRect(xi - 3 * u - o, ay - 3 * u, 6 * u + 2 * o, u + o);
    ctx.fillRect(xi - 2 * u - o, ay - 2 * u, 4 * u + 2 * o, u + o);
    ctx.fillRect(xi - u - o, ay - u, 2 * u + 2 * o, u + o);
  };
  arrow('#1a140c', u); arrow(rar.rim, 0);
  ctx.fillStyle = 'rgba(255,255,255,0.5)'; ctx.fillRect(xi - 4 * u, ay - 6 * u, 8 * u, u);

  // Sprite
  if (isSpecies) drawSpeciesTreasure(ctx, x, y + bob, t, s);
  else drawLootSprite(ctx, xi, Math.round(y - 5 * s + bob), s, t.treasure_type, time, ph);

  // Orbiting sparkles: pixel crosses (1 unit) that twinkle
  for (let i = 0; i < 3; i++) {
    const a = time / 1400 + ph + i * 2.094;
    const tw = 0.5 + 0.5 * Math.sin(time / 180 + i * 1.7 + ph);
    const sx = Math.round(x + Math.cos(a) * (R + 4 * s)), sy = Math.round(cy + Math.sin(a) * (R + 4 * s) * 0.55 - 2 * s);
    ctx.fillStyle = tw > 0.5 ? '#fff8d8' : '#ffd84a';
    ctx.fillRect(sx - u / 2, sy - u / 2, u, u);
    if (tw > 0.35) { ctx.fillRect(sx - u / 2, sy - 2 * u, u, 4 * u); ctx.fillRect(sx - 2 * u, sy - u / 2, 4 * u, u); }
  }

  // Name tag at street level (MAP_FONT.label, like giant-tree labels)
  if (G.cam.zoom >= 16.5) {
    const label = isSpecies ? t.species_german : (rar.name + (t.value ? ' +' + t.value : ''));
    const sub = isSpecies && t.species_category ? t.species_category + ' · ' + rar.name : '';
    ctx.font = MAP_FONT.label;
    ctx.textAlign = 'center'; ctx.textBaseline = 'top';
    const ty = Math.round(y + 8 * s);
    const tw = Math.ceil(ctx.measureText(label).width) + 8;
    ctx.fillStyle = 'rgba(20,16,10,0.78)';
    ctx.fillRect(xi - Math.ceil(tw / 2), ty, tw, sub ? 26 : 15);
    ctx.fillStyle = rar.rim; ctx.fillRect(xi - Math.ceil(tw / 2), ty, tw, 1);
    ctx.fillStyle = 'rgba(0,0,0,0.65)'; ctx.fillText(label, xi + 1, ty + 2);
    ctx.fillStyle = '#fff4d0'; ctx.fillText(label, xi, ty + 1);
    if (sub) {
      ctx.font = MAP_FONT.small;
      ctx.fillStyle = rar.rim; ctx.fillText(sub, xi, ty + 14);
    }
  }
  ctx.restore();
}

// Pixel-art loot sprites (drawn on a virtual 1px grid scaled by s, centred on cx,cy).
function drawLootSprite(ctx, cx, cy, s, type, time, ph) {
  ctx.save();
  ctx.translate(cx, cy);
  ctx.scale(s, s);
  const px = (x, y, w, h, c) => { ctx.fillStyle = c; ctx.fillRect(x, y, w, h); };
  if (type === 'xp') {
    // Floating cyan gem, faceted, with a rotating highlight
    const gl = 0.5 + 0.5 * Math.sin(time / 300 + ph);
    ctx.beginPath(); ctx.moveTo(0, -9); ctx.lineTo(7, -3); ctx.lineTo(4, 7); ctx.lineTo(-4, 7); ctx.lineTo(-7, -3); ctx.closePath();
    ctx.fillStyle = '#1d8fb0'; ctx.fill();
    ctx.beginPath(); ctx.moveTo(0, -9); ctx.lineTo(7, -3); ctx.lineTo(0, -1); ctx.closePath(); ctx.fillStyle = '#7fe8ff'; ctx.fill();
    ctx.beginPath(); ctx.moveTo(0, -9); ctx.lineTo(-7, -3); ctx.lineTo(0, -1); ctx.closePath(); ctx.fillStyle = '#48c6ea'; ctx.fill();
    ctx.beginPath(); ctx.moveTo(-7, -3); ctx.lineTo(0, -1); ctx.lineTo(-4, 7); ctx.closePath(); ctx.fillStyle = '#2aa4cc'; ctx.fill();
    ctx.beginPath(); ctx.moveTo(7, -3); ctx.lineTo(0, -1); ctx.lineTo(4, 7); ctx.closePath(); ctx.fillStyle = '#156f8c'; ctx.fill();
    ctx.beginPath(); ctx.moveTo(0, -1); ctx.lineTo(4, 7); ctx.lineTo(-4, 7); ctx.closePath(); ctx.fillStyle = '#0f5a72'; ctx.fill();
    px(-2, -6, 2, 2, 'rgba(255,255,255,' + (0.5 + gl * 0.5).toFixed(2) + ')');
    px(1, -4, 1, 1, 'rgba(255,255,255,' + (0.3 + gl * 0.6).toFixed(2) + ')');
  } else if (type === 'rare_seed') {
    px(-5, 2, 10, 6, '#7a4a22'); px(-6, 1, 12, 2, '#8f5a2c'); px(-4, 3, 8, 1, '#5e3618');
    px(-1, -6, 2, 8, '#3f9a3a'); px(-5, -5, 4, 3, '#5ec457'); px(1, -8, 4, 3, '#5ec457'); px(-1, -9, 2, 2, '#9be86a');
  } else if (type === 'ancient_map') {
    px(-9, -6, 18, 12, '#d8bf86'); px(-9, -6, 18, 1, '#a88a55'); px(-9, 5, 18, 1, '#a88a55');
    px(-10, -7, 3, 14, '#8a6a3c'); px(7, -7, 3, 14, '#8a6a3c');
    ctx.strokeStyle = '#7a4a22'; ctx.lineWidth = 1; ctx.setLineDash([1.5, 1.5]);
    ctx.beginPath(); ctx.moveTo(-6, 3); ctx.quadraticCurveTo(-1, -6, 5, -2); ctx.stroke(); ctx.setLineDash([]);
    px(4, -3, 2, 2, '#c03030'); px(4.5, -2.5, 1, 1, '#ff8080');
  } else {
    // Wooden treasure chest, iron bands, gold lock; lid cracks open and glints
    const open = Math.max(0, Math.sin(time / 1100 + ph)) * 2.5;
    px(-9, -1, 18, 9, '#5a3416');           // body dark
    px(-8, 0, 16, 7, '#8a5a2e');            // body
    px(-8, 0, 16, 1, '#a9743d');            // top highlight
    px(-8, 6, 16, 1, '#6b4220');            // bottom shade
    px(-9, -1, 2, 9, '#3f3a38'); px(7, -1, 2, 9, '#3f3a38');          // iron corners
    px(-5, -1, 1, 9, '#4a4440'); px(4, -1, 1, 9, '#4a4440');          // straps
    // glint from inside when lid is open
    if (open > 0.4) { px(-7, -1 - open, 14, open + 1, '#ffe27a'); px(-4, -1 - open, 8, 1, '#fff7c0'); }
    // lid (rounded top) — rises with `open`
    ctx.save(); ctx.translate(0, -open);
    px(-9, -6, 18, 5, '#6b4220'); px(-8, -7, 16, 1, '#6b4220'); px(-8, -5, 16, 3, '#9a6634'); px(-7, -6, 14, 1, '#b57d44');
    px(-9, -6, 2, 5, '#3f3a38'); px(7, -6, 2, 5, '#3f3a38'); px(-5, -7, 1, 6, '#4a4440'); px(4, -7, 1, 6, '#4a4440');
    ctx.restore();
    // lock plate + keyhole
    px(-2, -2, 4, 4, '#e8b83a'); px(-2, -2, 4, 1, '#fff0a0'); px(-0.5, -0.5, 1, 1.5, '#4a3010');
    // coin glint on the lock
    const gl = 0.5 + 0.5 * Math.sin(time / 250 + ph);
    if (gl > 0.7) px(1, -2, 1, 1, '#ffffff');
  }
  ctx.restore();
}

// ---- Collect FX: gold burst + floating reward text ----
G.fx = G.fx || [];
function spawnCollectFX(t, text, rar) {
  const [x, y] = toScreen(t.lon, t.lat);
  const parts = [];
  for (let i = 0; i < 16; i++) {
    const a = (i / 16) * Math.PI * 2 + Math.random() * 0.4;
    const v = 60 + Math.random() * 90;
    parts.push({a, vx: Math.cos(a) * v, vy: Math.sin(a) * v * 0.6 - 60, sz: 1.5 + Math.random() * 2.5});
  }
  G.fx.push({lon: t.lon, lat: t.lat, t0: performance.now(), dur: 1400, text, color: rar.rim, glow: rar.glow, parts});
}
function drawCollectFX(ctx) {
  if (!G.fx.length) return;
  const now = performance.now();
  G.fx = G.fx.filter(f => now - f.t0 < f.dur);
  for (const f of G.fx) {
    const [x, y] = toScreen(f.lon, f.lat);
    const k = (now - f.t0) / f.dur;            // 0..1
    const tt = (now - f.t0) / 1000;
    // expanding ring
    ctx.strokeStyle = 'rgba(' + f.glow + ',' + (0.8 * (1 - k)).toFixed(2) + ')';
    ctx.lineWidth = 2;
    const rr = Math.round(10 + k * 60);
    ctx.strokeRect(Math.round(x) - rr, Math.round(y - 6) - rr * 0.6, rr * 2, rr * 1.2);
    // sparks with gravity
    for (const p of f.parts) {
      const px = x + p.vx * tt, py = y - 6 + p.vy * tt + 160 * tt * tt;
      ctx.fillStyle = 'rgba(255,235,140,' + (1 - k).toFixed(2) + ')';
      ctx.fillRect(px - p.sz / 2, py - p.sz / 2, p.sz, p.sz);
    }
    // floating text
    const ease = 1 - Math.pow(1 - Math.min(1, k * 1.3), 3);
    ctx.font = MAP_FONT.pixel;
    ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
    const fy = Math.round(y - 26 - ease * 40);
    ctx.globalAlpha = 1 - k * k;
    ctx.fillStyle = '#000'; ctx.fillText(f.text, Math.round(x) + 1, fy + 1);
    ctx.fillStyle = f.color; ctx.fillText(f.text, Math.round(x), fy);
    ctx.globalAlpha = 1;
  }
}

// Map species names to sprite drawing groups
const SPECIES_SPRITE_MAP = {
  'Lynx lynx':                  'lynx',
  'Barbastella barbastellus':   'bat',
  'Cricetus cricetus':          'hamster',
  'Bison bonasus':              'bison',
  'Gulo gulo':                  'wolverine',
  'Aquila chrysaetos':          'eagle',
  'Bubo bubo':                  'owl',
  'Ciconia nigra':              'stork',
  'Otis tarda':                 'bustard',
  'Tetrao urogallus':           'capercaillie',
  'Coenonympha hero':           'butterfly_brown',
  'Colias chrysotheme':         'butterfly_yellow',
  'Parnassius apollo':          'butterfly_white',
  'Bombina bombina':            'frog',
  'Vipera ursinii':             'snake',
  'Triturus dobrogicus':        'newt',
  'Coenagrion ornatum':         'dragonfly',
  'Cordulegaster heros':        'dragonfly_large',
  'Hucho hucho':                'fish',
  'Acipenser ruthenus':         'sturgeon',
};

// Species sprite only (frame, glow, label are drawn by drawTreasure). `y` is the
// already-bobbed ground anchor; the creature occupies roughly y-17s … y+6s.
function drawSpeciesTreasure(ctx, x, y, t, s) {
  const sprite = SPECIES_SPRITE_MAP[t.species_name] || 'butterfly_white';
  const time = Date.now();
  s = s || treasureScale();
  ctx.save();
  const by = y;

  if (sprite === 'lynx') {
    // Pixel-art lynx face: tufted ears, spotted
    ctx.fillStyle = '#c8a060'; // body
    ctx.beginPath(); ctx.ellipse(x, by - 4*s, 7*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // Ears with tufts
    ctx.fillStyle = '#c8a060';
    ctx.beginPath(); ctx.moveTo(x-5*s, by-8*s); ctx.lineTo(x-3*s, by-14*s); ctx.lineTo(x-1*s, by-8*s); ctx.fill();
    ctx.beginPath(); ctx.moveTo(x+5*s, by-8*s); ctx.lineTo(x+3*s, by-14*s); ctx.lineTo(x+1*s, by-8*s); ctx.fill();
    // Ear tufts (black tips)
    ctx.fillStyle = '#2a2a2a';
    ctx.beginPath(); ctx.moveTo(x-3*s, by-14*s); ctx.lineTo(x-3.5*s, by-17*s); ctx.lineTo(x-2.5*s, by-14*s); ctx.fill();
    ctx.beginPath(); ctx.moveTo(x+3*s, by-14*s); ctx.lineTo(x+3.5*s, by-17*s); ctx.lineTo(x+2.5*s, by-14*s); ctx.fill();
    // Eyes
    ctx.fillStyle = '#e8c840'; ctx.beginPath(); ctx.arc(x-2.5*s, by-5*s, 1.5*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+2.5*s, by-5*s, 1.5*s, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#111'; ctx.beginPath(); ctx.arc(x-2.5*s, by-5*s, 0.7*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+2.5*s, by-5*s, 0.7*s, 0, Math.PI*2); ctx.fill();
    // Nose
    ctx.fillStyle = '#d08080'; ctx.beginPath(); ctx.arc(x, by-2*s, 1*s, 0, Math.PI*2); ctx.fill();
    // Spots
    ctx.fillStyle = '#a08040';
    ctx.beginPath(); ctx.arc(x-4*s, by-2*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+3*s, by-1*s, 0.7*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'bat') {
    // Bat with outstretched wings
    const wingFlap = Math.sin(time / 200) * 0.3;
    ctx.fillStyle = '#3a3040';
    // Body
    ctx.beginPath(); ctx.ellipse(x, by-4*s, 3*s, 4*s, 0, 0, Math.PI*2); ctx.fill();
    // Wings
    ctx.beginPath();
    ctx.moveTo(x-3*s, by-4*s);
    ctx.quadraticCurveTo(x-10*s, by - (8+wingFlap*8)*s, x-12*s, by-2*s);
    ctx.quadraticCurveTo(x-8*s, by+1*s, x-3*s, by-1*s);
    ctx.fill();
    ctx.beginPath();
    ctx.moveTo(x+3*s, by-4*s);
    ctx.quadraticCurveTo(x+10*s, by - (8+wingFlap*8)*s, x+12*s, by-2*s);
    ctx.quadraticCurveTo(x+8*s, by+1*s, x+3*s, by-1*s);
    ctx.fill();
    // Eyes
    ctx.fillStyle = '#e0c040';
    ctx.beginPath(); ctx.arc(x-1.5*s, by-6*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+1.5*s, by-6*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    // Ears
    ctx.fillStyle = '#3a3040';
    ctx.beginPath(); ctx.moveTo(x-1*s, by-8*s); ctx.lineTo(x-2*s, by-12*s); ctx.lineTo(x, by-8*s); ctx.fill();
    ctx.beginPath(); ctx.moveTo(x+1*s, by-8*s); ctx.lineTo(x+2*s, by-12*s); ctx.lineTo(x, by-8*s); ctx.fill();
  } else if (sprite === 'hamster') {
    // Cute round hamster with cheek pouches
    ctx.fillStyle = '#c8963c';
    ctx.beginPath(); ctx.ellipse(x, by-3*s, 6*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // White belly
    ctx.fillStyle = '#f0e8d0';
    ctx.beginPath(); ctx.ellipse(x, by-1*s, 4*s, 3*s, 0, 0, Math.PI*2); ctx.fill();
    // Cheek pouches
    ctx.fillStyle = '#d8a848';
    ctx.beginPath(); ctx.arc(x-4*s, by-3*s, 2.5*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+4*s, by-3*s, 2.5*s, 0, Math.PI*2); ctx.fill();
    // Eyes
    ctx.fillStyle = '#111';
    ctx.beginPath(); ctx.arc(x-2*s, by-5*s, 1*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+2*s, by-5*s, 1*s, 0, Math.PI*2); ctx.fill();
    // Nose
    ctx.fillStyle = '#e08080'; ctx.beginPath(); ctx.arc(x, by-3.5*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    // Ears
    ctx.fillStyle = '#b88838';
    ctx.beginPath(); ctx.arc(x-4*s, by-7*s, 1.5*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+4*s, by-7*s, 1.5*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'bison') {
    // European bison — large, bulky, shaggy
    ctx.fillStyle = '#5a3818';
    // Body
    ctx.beginPath(); ctx.ellipse(x+2*s, by-3*s, 10*s, 6*s, 0, 0, Math.PI*2); ctx.fill();
    // Hump
    ctx.fillStyle = '#4a2e12';
    ctx.beginPath(); ctx.ellipse(x-3*s, by-8*s, 5*s, 4*s, -0.3, 0, Math.PI*2); ctx.fill();
    // Head
    ctx.fillStyle = '#5a3818';
    ctx.beginPath(); ctx.ellipse(x-8*s, by-4*s, 4*s, 3.5*s, 0, 0, Math.PI*2); ctx.fill();
    // Horns
    ctx.strokeStyle = '#d8c898'; ctx.lineWidth = 1.5*s;
    ctx.beginPath(); ctx.arc(x-9*s, by-8*s, 3*s, Math.PI*0.8, Math.PI*1.5); ctx.stroke();
    ctx.beginPath(); ctx.arc(x-7*s, by-8*s, 3*s, Math.PI*1.5, Math.PI*2.2); ctx.stroke();
    // Eye
    ctx.fillStyle = '#111'; ctx.beginPath(); ctx.arc(x-9*s, by-5*s, 0.7*s, 0, Math.PI*2); ctx.fill();
    // Legs
    ctx.fillStyle = '#4a2e12';
    for (const lx of [-2, 3, 6, 9]) ctx.fillRect(x+lx*s-1*s, by+2*s, 2*s, 4*s);
  } else if (sprite === 'wolverine') {
    // Stocky, dark with lighter stripe
    ctx.fillStyle = '#2a2018';
    ctx.beginPath(); ctx.ellipse(x, by-3*s, 8*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // Lighter side stripe
    ctx.fillStyle = '#8a7040';
    ctx.beginPath(); ctx.ellipse(x, by-2*s, 7*s, 2*s, 0, 0, Math.PI*2); ctx.fill();
    // Head
    ctx.fillStyle = '#2a2018';
    ctx.beginPath(); ctx.ellipse(x-6*s, by-4*s, 3.5*s, 3*s, 0, 0, Math.PI*2); ctx.fill();
    // Eyes
    ctx.fillStyle = '#c0a030'; ctx.beginPath(); ctx.arc(x-7*s, by-5*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    // Legs
    ctx.fillStyle = '#2a2018';
    for (const lx of [-3, 0, 3, 5]) ctx.fillRect(x+lx*s-0.8*s, by+1*s, 1.6*s, 3*s);
  } else if (sprite === 'eagle') {
    // Golden eagle soaring
    const wingAngle = Math.sin(time / 400) * 0.15;
    ctx.fillStyle = '#5a3818';
    // Body
    ctx.beginPath(); ctx.ellipse(x, by-4*s, 4*s, 3*s, 0, 0, Math.PI*2); ctx.fill();
    // Wings
    ctx.beginPath();
    ctx.moveTo(x-4*s, by-4*s);
    ctx.quadraticCurveTo(x-10*s, by-(10+wingAngle*5)*s, x-14*s, by-6*s);
    ctx.lineTo(x-12*s, by-3*s);
    ctx.quadraticCurveTo(x-8*s, by-2*s, x-4*s, by-3*s);
    ctx.fill();
    ctx.beginPath();
    ctx.moveTo(x+4*s, by-4*s);
    ctx.quadraticCurveTo(x+10*s, by-(10+wingAngle*5)*s, x+14*s, by-6*s);
    ctx.lineTo(x+12*s, by-3*s);
    ctx.quadraticCurveTo(x+8*s, by-2*s, x+4*s, by-3*s);
    ctx.fill();
    // Head
    ctx.fillStyle = '#c8a040';
    ctx.beginPath(); ctx.ellipse(x, by-7*s, 2.5*s, 2*s, 0, 0, Math.PI*2); ctx.fill();
    // Beak
    ctx.fillStyle = '#e8c030';
    ctx.beginPath(); ctx.moveTo(x, by-7*s); ctx.lineTo(x, by-4.5*s); ctx.lineTo(x+1.5*s, by-6.5*s); ctx.fill();
    // Eye
    ctx.fillStyle = '#111'; ctx.beginPath(); ctx.arc(x-0.8*s, by-7.5*s, 0.6*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'owl') {
    // Eurasian Eagle-Owl: big round head, orange eyes, tufts
    ctx.fillStyle = '#8a6830';
    ctx.beginPath(); ctx.ellipse(x, by-3*s, 6*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // Facial disc
    ctx.fillStyle = '#c8a868';
    ctx.beginPath(); ctx.ellipse(x, by-6*s, 5*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // Ear tufts
    ctx.fillStyle = '#7a5828';
    ctx.beginPath(); ctx.moveTo(x-3*s, by-10*s); ctx.lineTo(x-4*s, by-15*s); ctx.lineTo(x-1*s, by-10*s); ctx.fill();
    ctx.beginPath(); ctx.moveTo(x+3*s, by-10*s); ctx.lineTo(x+4*s, by-15*s); ctx.lineTo(x+1*s, by-10*s); ctx.fill();
    // Big orange eyes
    ctx.fillStyle = '#e88020';
    ctx.beginPath(); ctx.arc(x-2*s, by-6*s, 2*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+2*s, by-6*s, 2*s, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#111';
    ctx.beginPath(); ctx.arc(x-2*s, by-6*s, 1*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+2*s, by-6*s, 1*s, 0, Math.PI*2); ctx.fill();
    // Beak
    ctx.fillStyle = '#555';
    ctx.beginPath(); ctx.moveTo(x-1*s, by-4*s); ctx.lineTo(x, by-2*s); ctx.lineTo(x+1*s, by-4*s); ctx.fill();
    // Breast feather marks
    ctx.strokeStyle = '#6a4820'; ctx.lineWidth = 0.5;
    for (let i = 0; i < 5; i++) {
      const fy = by - 1*s + i*1.5*s;
      ctx.beginPath(); ctx.moveTo(x-2*s, fy); ctx.lineTo(x, fy+0.8*s); ctx.lineTo(x+2*s, fy); ctx.stroke();
    }
  } else if (sprite === 'stork') {
    // Black stork — dark body, red beak, long legs
    // Legs
    ctx.strokeStyle = '#d04030'; ctx.lineWidth = 1.5*s;
    ctx.beginPath(); ctx.moveTo(x-2*s, by+1*s); ctx.lineTo(x-2*s, by+8*s); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(x+2*s, by+1*s); ctx.lineTo(x+2*s, by+8*s); ctx.stroke();
    // Body
    ctx.fillStyle = '#1a1a2a';
    ctx.beginPath(); ctx.ellipse(x, by-3*s, 6*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // White belly
    ctx.fillStyle = '#e8e0d8';
    ctx.beginPath(); ctx.ellipse(x, by, 4*s, 2.5*s, 0, 0, Math.PI*2); ctx.fill();
    // Head
    ctx.fillStyle = '#1a1a2a';
    ctx.beginPath(); ctx.arc(x, by-9*s, 2.5*s, 0, Math.PI*2); ctx.fill();
    // Beak
    ctx.fillStyle = '#d04030';
    ctx.beginPath(); ctx.moveTo(x+2.5*s, by-9*s); ctx.lineTo(x+7*s, by-8.5*s); ctx.lineTo(x+2.5*s, by-8*s); ctx.fill();
    // Eye
    ctx.fillStyle = '#d03020'; ctx.beginPath(); ctx.arc(x+0.5*s, by-9.5*s, 0.6*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'bustard') {
    // Great bustard — large, puffed chest
    ctx.fillStyle = '#b8963c';
    ctx.beginPath(); ctx.ellipse(x, by-2*s, 7*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // Neck
    ctx.fillStyle = '#888890';
    ctx.fillRect(x-1.5*s, by-10*s, 3*s, 6*s);
    // Head
    ctx.fillStyle = '#888890';
    ctx.beginPath(); ctx.arc(x, by-12*s, 2.5*s, 0, Math.PI*2); ctx.fill();
    // Whiskers
    ctx.strokeStyle = '#a08040'; ctx.lineWidth = 0.8;
    ctx.beginPath(); ctx.moveTo(x+2*s, by-12*s); ctx.lineTo(x+6*s, by-14*s); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(x+2*s, by-11*s); ctx.lineTo(x+5*s, by-10*s); ctx.stroke();
    // Eye & beak
    ctx.fillStyle = '#111'; ctx.beginPath(); ctx.arc(x-0.5*s, by-12.5*s, 0.5*s, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#a0a068';
    ctx.beginPath(); ctx.moveTo(x-2*s, by-12*s); ctx.lineTo(x-5*s, by-11.5*s); ctx.lineTo(x-2*s, by-11*s); ctx.fill();
    // Legs
    ctx.strokeStyle = '#8a7a60'; ctx.lineWidth = 1.5*s;
    ctx.beginPath(); ctx.moveTo(x-3*s, by+2*s); ctx.lineTo(x-3*s, by+7*s); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(x+3*s, by+2*s); ctx.lineTo(x+3*s, by+7*s); ctx.stroke();
  } else if (sprite === 'capercaillie') {
    // Western capercaillie — dark body, red eyebrow, fan tail
    ctx.fillStyle = '#1a2a1a';
    ctx.beginPath(); ctx.ellipse(x, by-3*s, 7*s, 5*s, 0, 0, Math.PI*2); ctx.fill();
    // Fan tail
    ctx.fillStyle = '#2a3a2a';
    for (let i = -3; i <= 3; i++) {
      ctx.beginPath();
      ctx.moveTo(x+5*s, by-2*s);
      ctx.lineTo(x+12*s + i*s, by - 5*s + Math.abs(i)*s);
      ctx.lineTo(x+5*s, by);
      ctx.fill();
    }
    // Head
    ctx.fillStyle = '#1a2a1a';
    ctx.beginPath(); ctx.arc(x-5*s, by-7*s, 2.5*s, 0, Math.PI*2); ctx.fill();
    // Red eyebrow
    ctx.fillStyle = '#e02020';
    ctx.beginPath(); ctx.ellipse(x-5*s, by-9*s, 2*s, 1*s, 0, 0, Math.PI*2); ctx.fill();
    // Beak
    ctx.fillStyle = '#c8c8a0';
    ctx.beginPath(); ctx.moveTo(x-7*s, by-7*s); ctx.lineTo(x-10*s, by-6.5*s); ctx.lineTo(x-7*s, by-6*s); ctx.fill();
    // Eye
    ctx.fillStyle = '#111'; ctx.beginPath(); ctx.arc(x-5*s, by-7.5*s, 0.6*s, 0, Math.PI*2); ctx.fill();
    // Legs
    ctx.strokeStyle = '#6a6050'; ctx.lineWidth = 1.2*s;
    ctx.beginPath(); ctx.moveTo(x-2*s, by+2*s); ctx.lineTo(x-2*s, by+6*s); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(x+2*s, by+2*s); ctx.lineTo(x+2*s, by+6*s); ctx.stroke();
  } else if (sprite.startsWith('butterfly_')) {
    // Butterflies — color varies by species
    const colors = {
      'butterfly_brown':  ['#8a6030','#a07838','#6a4820'],
      'butterfly_yellow': ['#e8c820','#f0d840','#d0b018'],
      'butterfly_white':  ['#f0e8e0','#e8e0d0','#d8d0c0'],
      'butterfly_ring':   ['#c89838','#a88028','#e8b848'],
    };
    const cols = colors[sprite] || colors.butterfly_white;
    const wingFlap = Math.abs(Math.sin(time / 250 + x * 0.1));
    const wingW = (6 + wingFlap * 4) * s;
    const wingH = 6 * s;
    // Upper wings
    ctx.fillStyle = cols[0];
    ctx.beginPath(); ctx.ellipse(x - wingW*0.6, by - 5*s, wingW, wingH, -0.3, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(x + wingW*0.6, by - 5*s, wingW, wingH, 0.3, 0, Math.PI*2); ctx.fill();
    // Lower wings (smaller)
    ctx.fillStyle = cols[1];
    ctx.beginPath(); ctx.ellipse(x - wingW*0.5, by - 1*s, wingW*0.7, wingH*0.6, -0.4, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(x + wingW*0.5, by - 1*s, wingW*0.7, wingH*0.6, 0.4, 0, Math.PI*2); ctx.fill();
    // Wing patterns (eye spots for apollo/ring)
    if (sprite === 'butterfly_white' || sprite === 'butterfly_ring') {
      ctx.fillStyle = '#d04040';
      ctx.beginPath(); ctx.arc(x - wingW*0.4, by - 5*s, 1.5*s, 0, Math.PI*2); ctx.fill();
      ctx.beginPath(); ctx.arc(x + wingW*0.4, by - 5*s, 1.5*s, 0, Math.PI*2); ctx.fill();
    }
    // Body
    ctx.fillStyle = cols[2];
    ctx.beginPath(); ctx.ellipse(x, by - 3*s, 1.5*s, 4*s, 0, 0, Math.PI*2); ctx.fill();
    // Antennae
    ctx.strokeStyle = '#333'; ctx.lineWidth = 0.6;
    ctx.beginPath(); ctx.moveTo(x-0.5*s, by-7*s); ctx.quadraticCurveTo(x-3*s, by-12*s, x-4*s, by-13*s); ctx.stroke();
    ctx.beginPath(); ctx.moveTo(x+0.5*s, by-7*s); ctx.quadraticCurveTo(x+3*s, by-12*s, x+4*s, by-13*s); ctx.stroke();
    ctx.fillStyle = '#333';
    ctx.beginPath(); ctx.arc(x-4*s, by-13*s, 0.5*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+4*s, by-13*s, 0.5*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'frog') {
    // Bombina bombina — green frog with orange belly
    ctx.fillStyle = '#4a8a30';
    ctx.beginPath(); ctx.ellipse(x, by-2*s, 6*s, 4*s, 0, 0, Math.PI*2); ctx.fill();
    // Orange belly peeking
    ctx.fillStyle = '#e87030';
    ctx.beginPath(); ctx.ellipse(x, by+1*s, 4*s, 1.5*s, 0, 0, Math.PI*2); ctx.fill();
    // Eyes (big, bulging)
    ctx.fillStyle = '#c8e040';
    ctx.beginPath(); ctx.arc(x-3*s, by-5*s, 2*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+3*s, by-5*s, 2*s, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#111';
    ctx.beginPath(); ctx.arc(x-3*s, by-5*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+3*s, by-5*s, 0.8*s, 0, Math.PI*2); ctx.fill();
    // Dark spots
    ctx.fillStyle = '#2a6a18';
    ctx.beginPath(); ctx.arc(x-2*s, by-1*s, 1*s, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x+3*s, by-2*s, 0.8*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'snake') {
    // Vipera ursinii — zigzag pattern
    const sway2 = Math.sin(time / 800 + y * 0.01) * 2 * s;
    ctx.strokeStyle = '#7a7a60'; ctx.lineWidth = 3*s;
    ctx.beginPath();
    ctx.moveTo(x - 8*s, by);
    for (let i = 0; i < 6; i++) {
      ctx.quadraticCurveTo(
        x + (-5 + i*3)*s + sway2*(i%2===0?1:-1),
        by - (i%2===0 ? 3 : -1)*s,
        x + (-3 + i*3)*s,
        by - 1*s
      );
    }
    ctx.stroke();
    // Zigzag dorsal pattern
    ctx.strokeStyle = '#3a3a30'; ctx.lineWidth = 1.5*s;
    ctx.beginPath();
    ctx.moveTo(x - 7*s, by-1*s);
    for (let i = 0; i < 5; i++) {
      ctx.lineTo(x + (-5+i*3)*s, by - (i%2===0?3:0)*s);
    }
    ctx.stroke();
    // Head
    ctx.fillStyle = '#7a7a60';
    ctx.beginPath(); ctx.ellipse(x+8*s, by-1*s, 2*s, 1.5*s, 0, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#111';
    ctx.beginPath(); ctx.arc(x+9*s, by-2*s, 0.5*s, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'newt') {
    // Donau-Kammmolch — dark with orange belly, crested
    ctx.fillStyle = '#2a3a2a';
    ctx.beginPath(); ctx.ellipse(x, by-2*s, 8*s, 3*s, 0, 0, Math.PI*2); ctx.fill();
    // Orange belly
    ctx.fillStyle = '#e87030';
    ctx.beginPath(); ctx.ellipse(x, by, 6*s, 1.5*s, 0, 0, Math.PI*2); ctx.fill();
    // Crest (dorsal)
    ctx.fillStyle = '#1a2a1a';
    ctx.beginPath();
    for (let i = 0; i < 6; i++) {
      ctx.lineTo(x + (-4+i*2)*s, by - (i%2===0?5:3)*s);
    }
    ctx.closePath(); ctx.fill();
    // Head
    ctx.fillStyle = '#2a3a2a';
    ctx.beginPath(); ctx.ellipse(x-7*s, by-2*s, 2.5*s, 2*s, 0, 0, Math.PI*2); ctx.fill();
    ctx.fillStyle = '#111'; ctx.beginPath(); ctx.arc(x-8*s, by-3*s, 0.5*s, 0, Math.PI*2); ctx.fill();
    // Tail
    ctx.strokeStyle = '#2a3a2a'; ctx.lineWidth = 2*s;
    ctx.beginPath();
    ctx.moveTo(x+8*s, by-2*s);
    ctx.quadraticCurveTo(x+12*s, by-4*s, x+14*s, by-2*s);
    ctx.stroke();
  } else if (sprite === 'dragonfly' || sprite === 'dragonfly_large') {
    // Dragonfly — long body, 4 transparent wings
    const isLarge = sprite === 'dragonfly_large';
    const ds = isLarge ? 1.3 : 1.0;
    const wingBeat = Math.sin(time / 150) * 0.2;
    // Body
    ctx.fillStyle = isLarge ? '#2a5a3a' : '#3080c0';
    ctx.beginPath(); ctx.ellipse(x, by-2*s*ds, 2*s*ds, 8*s*ds, Math.PI*0.5, 0, Math.PI*2); ctx.fill();
    // Head
    ctx.beginPath(); ctx.arc(x-8*s*ds, by-2*s*ds, 2*s*ds, 0, Math.PI*2); ctx.fill();
    // Wings (transparent, iridescent)
    ctx.fillStyle = 'rgba(180,220,255,0.35)';
    ctx.beginPath(); ctx.ellipse(x-2*s*ds, by-(5+wingBeat*3)*s*ds, 7*s*ds, 2.5*s*ds, -0.2, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(x-2*s*ds, by+(1+wingBeat*3)*s*ds, 7*s*ds, 2.5*s*ds, 0.2, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(x+1*s*ds, by-(4+wingBeat*2)*s*ds, 5*s*ds, 2*s*ds, -0.15, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(x+1*s*ds, by+(0+wingBeat*2)*s*ds, 5*s*ds, 2*s*ds, 0.15, 0, Math.PI*2); ctx.fill();
    // Wing veins
    ctx.strokeStyle = 'rgba(100,160,200,0.3)'; ctx.lineWidth = 0.3;
    ctx.beginPath(); ctx.moveTo(x-8*s*ds, by-5*s*ds); ctx.lineTo(x+4*s*ds, by-5*s*ds); ctx.stroke();
    // Eyes
    ctx.fillStyle = isLarge ? '#80c060' : '#60a0e0';
    ctx.beginPath(); ctx.arc(x-9*s*ds, by-3*s*ds, 1.2*s*ds, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.arc(x-9*s*ds, by-1*s*ds, 1.2*s*ds, 0, Math.PI*2); ctx.fill();
  } else if (sprite === 'fish' || sprite === 'sturgeon') {
    // Fish / Sturgeon in water
    const swim = Math.sin(time / 500 + x * 0.02) * 3 * s;
    const isSturgeon = sprite === 'sturgeon';
    // Water ripple under
    ctx.strokeStyle = 'rgba(100,160,220,0.3)'; ctx.lineWidth = 1;
    ctx.beginPath(); ctx.ellipse(x, by + 3*s, 10*s, 2*s, 0, 0, Math.PI*2); ctx.stroke();
    // Body
    ctx.fillStyle = isSturgeon ? '#6a7a8a' : '#8a5a3a';
    ctx.beginPath();
    ctx.ellipse(x + swim*0.3, by - 2*s, isSturgeon ? 10*s : 8*s, 3*s, 0, 0, Math.PI*2);
    ctx.fill();
    // Belly
    ctx.fillStyle = isSturgeon ? '#b0b8c0' : '#d8c8a0';
    ctx.beginPath(); ctx.ellipse(x + swim*0.3, by, isSturgeon ? 8*s : 6*s, 1.5*s, 0, 0, Math.PI*2); ctx.fill();
    // Tail fin
    ctx.fillStyle = isSturgeon ? '#5a6a7a' : '#7a4a2a';
    ctx.beginPath();
    ctx.moveTo(x + (isSturgeon?10:8)*s + swim*0.3, by - 2*s);
    ctx.lineTo(x + (isSturgeon?15:12)*s + swim, by - 5*s);
    ctx.lineTo(x + (isSturgeon?15:12)*s + swim, by + 1*s);
    ctx.closePath(); ctx.fill();
    // Eye
    ctx.fillStyle = '#111';
    ctx.beginPath(); ctx.arc(x - (isSturgeon?8:6)*s + swim*0.2, by - 3*s, 0.7*s, 0, Math.PI*2); ctx.fill();
    // Sturgeon: bony scutes (plates along body)
    if (isSturgeon) {
      ctx.fillStyle = '#8a9aaa';
      for (let i = 0; i < 5; i++) {
        ctx.beginPath();
        ctx.arc(x + (-6+i*3.5)*s + swim*0.3, by - 3.5*s, 1*s, 0, Math.PI*2);
        ctx.fill();
      }
      // Barbels (whiskers)
      ctx.strokeStyle = '#6a7a8a'; ctx.lineWidth = 0.5;
      ctx.beginPath(); ctx.moveTo(x-10*s, by-1*s); ctx.lineTo(x-13*s, by+1*s); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(x-9*s, by-0.5*s); ctx.lineTo(x-12*s, by+2*s); ctx.stroke();
    }
    // Huchen: red spots
    if (!isSturgeon) {
      ctx.fillStyle = '#c04040';
      for (let i = 0; i < 4; i++) {
        ctx.beginPath();
        ctx.arc(x + (-4+i*3)*s + swim*0.3, by - 2*s, 0.6*s, 0, Math.PI*2);
        ctx.fill();
      }
    }
  }

  ctx.restore();
}

/** Gold pulsing outline around the tapped building footprint. */
function drawFpHighlight(ctx, fp) {
  const g = fp.geometry;
  if (!g || g.type !== 'Polygon') return;
  const pts = g.coordinates[0].map(c => toScreen(c[0], c[1]));
  let onScreen = false;
  for (const pt of pts) {
    if (pt[0] > -50 && pt[0] < gc.width + 50 && pt[1] > -50 && pt[1] < gc.height + 50) { onScreen = true; break; }
  }
  if (!onScreen) return;
  const pulse = 0.55 + 0.35 * Math.sin(performance.now() / 300);
  ctx.save();
  ctx.beginPath();
  pts.forEach((pt, i) => i === 0 ? ctx.moveTo(pt[0], pt[1]) : ctx.lineTo(pt[0], pt[1]));
  ctx.closePath();
  ctx.strokeStyle = 'rgba(255,204,64,' + pulse.toFixed(2) + ')';
  ctx.lineWidth = 2.5;
  ctx.setLineDash([5, 3]);
  ctx.stroke();
  ctx.restore();
  // Keep the pulse animating while a building is selected
  if (!G._fpPulse) {
    G._fpPulse = true;
    requestAnimationFrame(() => { G._fpPulse = false; if (G.selFp) render(); });
  }
}

function drawEZHighlight(ctx) {
  const key = G.ezHighlight.kg + '-EZ' + G.ezHighlight.ez;
  const parcels = G.ezIndex[key] || [];
  if (parcels.length < 2) return;
  const selId = G.sel?.properties?.parcel_id;
  ctx.save();
  // Subtle static glow — no pulsing
  const alpha = 0.12;
  for (const f of parcels) {
    if (f.properties.parcel_id === selId) continue; // skip the selected one (drawn separately)
    if (isAreaGeom(f.geometry)) {
      const rings = geomAllRings(f.geometry).map(r => r.map(c => toScreen(c[0], c[1])));
      const pts = rings[0] || [];
      // Quick bounds check
      let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
      for (const ring of rings) for (const pt of ring) {
        if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
        if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
      }
      if (!rings.length) continue;
      if (maxX < -50 || minX > gc.width+50 || maxY < -50 || minY > gc.height+50) continue;
      ctx.beginPath();
      for (const ring of rings) {
        ring.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
        ctx.closePath();
      }
      // Subtle whitish glow fill
      ctx.fillStyle = 'rgba(220,220,240,' + alpha + ')';
      ctx.fill();
      // Soft white border
      ctx.strokeStyle = 'rgba(255,255,255,0.35)';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([6, 4]);
      ctx.stroke();
      ctx.setLineDash([]);
    } else {
      const p = f.properties;
      const [x, y] = toScreen(p.lon || f.geometry.coordinates[0], p.lat || f.geometry.coordinates[1]);
      if (x < -30 || x > gc.width+30 || y < -30 || y > gc.height+30) continue;
      const sz = Math.max(8, Math.min(30, Math.sqrt(p.area_sqm||100) * mapScale() / 80000));
      ctx.fillStyle = 'rgba(220,220,240,' + alpha + ')';
      ctx.fillRect(x-sz/2, y-sz/2, sz, sz);
      ctx.strokeStyle = 'rgba(255,255,255,0.35)';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([6, 4]);
      ctx.strokeRect(x-sz/2, y-sz/2, sz, sz);
      ctx.setLineDash([]);
    }
  }
  ctx.restore();
}

function drawSelection(ctx, f) {
  const p = f.properties;
  ctx.save();
  ctx.strokeStyle = '#ffd700';
  ctx.lineWidth = 3;
  ctx.setLineDash([6, 4]);
  ctx.shadowColor = '#ffd700';
  ctx.shadowBlur = 8;

  if (isAreaGeom(f.geometry)) {
    ctx.beginPath();
    for (const ring of geomAllRings(f.geometry)) {
      const pts = ring.map(c => toScreen(c[0], c[1]));
      pts.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
      ctx.closePath();
    }
    ctx.stroke();
  } else {
    const [x,y] = toScreen(p.lon||f.geometry.coordinates[0], p.lat||f.geometry.coordinates[1]);
    const sz = Math.max(12, Math.sqrt(p.area_sqm||100) * mapScale() / 60000);
    ctx.strokeRect(x-sz/2-3, y-sz/2-3, sz+6, sz+6);
  }
  ctx.restore();
}

// ================= AUSTRIA NATIONAL BORDER =================
// Cadastre data stops at the state border, so a viewport straddling it looks
// "broken" (large empty green area) unless we say WHY. We ship a simplified
// ADM0 outline (srv/static/austria.json, geoBoundaries/BEV, ~12k vertices,
// DP-simplified to 0.0002° ≈ 20 m) and dim + hatch everything outside it, plus
// a red-white-red border line. Same outline is drawn on the minimap.

G.atBorder = null;      // [ring, ...] lon/lat outer rings
let _atBorderTried = false;

async function loadAustriaBorder() {
  if (_atBorderTried) return;
  _atBorderTried = true;
  try {
    const r = await fetch('/static/austria.json?v=1');
    const d = await r.json();
    G.atBorder = d.rings || [];
    render(); renderMini();
  } catch(e) { console.error('austria border load failed', e); }
}

/** Is a lon/lat inside Austria? Null-safe: true while the outline is loading. */
function insideAustria(lon, lat) {
  if (!G.atBorder) return true;
  return pipRings(lon, lat, G.atBorder);
}

/** Foreign-territory shading + national border line. Drawn right after the
 *  grass backdrop so parcels/landuse sit on top of the shading, and the border
 *  line again on top of everything (drawAustriaBorderLine). */
function drawForeignShading(ctx, W, H) {
  if (!G.atBorder) return;
  const v = viewBounds();
  // Skip entirely if the view is comfortably inside the country (cheap check:
  // no ring segment intersects the padded viewport and the centre is inside).
  if (!borderNearView(v) && insideAustria(G.cam.lon, G.cam.lat)) return;

  ctx.save();
  // Even-odd: whole canvas minus Austria = foreign land.
  ctx.beginPath();
  ctx.rect(0, 0, W, H);
  for (const ring of G.atBorder) {
    let started = false;
    for (const c of ring) {
      const [x, y] = toScreen(c[0], c[1]);
      started ? ctx.lineTo(x, y) : (ctx.moveTo(x, y), started = true);
    }
    ctx.closePath();
  }
  ctx.fillStyle = 'rgba(28,44,20,0.55)';
  ctx.fill('evenodd');

  // Diagonal hatch over the foreign side to read as "no data here".
  ctx.clip('evenodd');
  ctx.strokeStyle = 'rgba(0,0,0,0.16)';
  ctx.lineWidth = 2;
  ctx.beginPath();
  for (let x = -H; x < W + H; x += 14) { ctx.moveTo(x, 0); ctx.lineTo(x + H, H); }
  ctx.stroke();
  ctx.restore();
}

/** Rough test: does any border vertex fall in (a padded) current view? */
function borderNearView(v) {
  const px = (v.e - v.w) * 0.5, py = (v.n - v.s) * 0.5;
  const w = v.w - px, e = v.e + px, s = v.s - py, n = v.n + py;
  for (const ring of G.atBorder) {
    for (const c of ring) {
      if (c[0] >= w && c[0] <= e && c[1] >= s && c[1] <= n) return true;
    }
  }
  return false;
}

/** Show the "outside Austria" banner when the view centre is across the border.
 *  Without it, the shaded/hatched foreign area reads as a loading failure. */
function updateAbroadBadge() {
  const el = document.getElementById('abroad-badge');
  if (!el) return;
  const abroad = !!G.atBorder && !insideAustria(G.cam.lon, G.cam.lat);
  el.style.display = abroad ? '' : 'none';
  // The two badges share the same slot — don't stack them.
  const eb = document.getElementById('enhanced-badge');
  if (eb && abroad) eb.style.display = 'none';
  const wc = document.getElementById('water-chip');
  if (wc && abroad) wc.style.display = 'none';
}

/** Red-white-red national border line, drawn above the map content. */
function drawAustriaBorderLine(ctx) {
  if (!G.atBorder) return;
  const v = viewBounds();
  if (!borderNearView(v)) return;
  ctx.save();
  for (const pass of [{ c: 'rgba(160,20,30,0.85)', w: 6 }, { c: 'rgba(255,255,255,0.9)', w: 2 }]) {
    ctx.beginPath();
    for (const ring of G.atBorder) {
      let started = false;
      for (const c of ring) {
        const [x, y] = toScreen(c[0], c[1]);
        started ? ctx.lineTo(x, y) : (ctx.moveTo(x, y), started = true);
      }
      ctx.closePath();
    }
    ctx.strokeStyle = pass.c;
    ctx.lineWidth = pass.w;
    ctx.lineJoin = 'round';
    ctx.stroke();
  }
  // Label the foreign side once, near the screen edge closest to the border.
  ctx.restore();
}

function drawScaleBar(ctx, W, H) {
  const s = mapScale();
  const mPerDeg = 111320 * Math.cos(G.cam.lat * Math.PI/180);
  let barM = 100, barPx = (barM / mPerDeg) * s;
  if (barPx < 40) { barM = 500; barPx = (barM/mPerDeg)*s; }
  if (barPx < 40) { barM = 1000; barPx = (barM/mPerDeg)*s; }
  if (barPx > 250) { barM = 20; barPx = (barM/mPerDeg)*s; }

  const x = 20, y = H - 25;
  ctx.fillStyle = '#000'; ctx.fillRect(x-1,y-1,barPx+2,6);
  ctx.fillStyle = '#fff'; ctx.fillRect(x,y,barPx,4);
  ctx.fillStyle = '#000'; ctx.fillRect(x,y,barPx/2,4);
  ctx.font = MAP_FONT.small; ctx.fillStyle = '#fff';
  ctx.fillText(barM>=1000?(barM/1000)+'km':barM+'m', x+barPx+6, y+4);
}

// ---- Minimap ----
function renderMini() {
  if (!mctx) return;
  mctx.fillStyle = '#1a2a10';
  mctx.fillRect(0, 0, 180, 130);

  const all = G.parcelPolys.length > 0 ? G.parcelPolys : G.parcels;
  if (!all.length) return;

  let minLon=Infinity,maxLon=-Infinity,minLat=Infinity,maxLat=-Infinity;
  for (const f of all) {
    const p = f.properties;
    const lon = p.lon || f.geometry.coordinates?.[0];
    const lat = p.lat || f.geometry.coordinates?.[1];
    if (lon) { minLon=Math.min(minLon,lon); maxLon=Math.max(maxLon,lon); }
    if (lat) { minLat=Math.min(minLat,lat); maxLat=Math.max(maxLat,lat); }
  }

  const pad = 5;
  const sw = 170, sh = 120;
  const lr = maxLon-minLon||0.01, ar = maxLat-minLat||0.01;
  const sc = Math.min(sw/lr, sh/ar);

  const cm = {};
  for (const c of G.claimed) cm[c.parcel_id] = c;

  for (const f of all) {
    const p = f.properties;
    const lon = p.lon || f.geometry.coordinates?.[0];
    const lat = p.lat || f.geometry.coordinates?.[1];
    if (!lon || !lat) continue;
    const mx = pad + (lon-minLon)*sc;
    const my = pad + (maxLat-lat)*sc;
    const cl = cm[p.parcel_id];
    const t = getParcelTerrain(p, cl);
    mctx.fillStyle = cl ? (G.pcolors[cl.player_id]||t[0]) : t[0];
    mctx.fillRect(mx-1, my-1, 3, 3);
  }

  // ---- Austrian border on the minimap ----
  // Clipped to the minimap extent; makes it obvious when the play area butts
  // against the state border (no cadastre data on the other side).
  if (G.atBorder) {
    mctx.save();
    mctx.beginPath();
    mctx.rect(0, 0, 180, 130);
    mctx.clip();
    const mspan = Math.max(lr, ar);
    mctx.strokeStyle = 'rgba(255,120,120,0.85)';
    mctx.lineWidth = 1.5;
    for (const ring of G.atBorder) {
      // Skip rings entirely outside the minimap window (cheap bbox test).
      let started = false, any = false;
      mctx.beginPath();
      for (const c of ring) {
        if (Math.abs(c[0] - minLon) > lr + mspan || Math.abs(c[1] - minLat) > ar + mspan) { started = false; continue; }
        const mx = pad + (c[0] - minLon) * sc, my = pad + (maxLat - c[1]) * sc;
        started ? mctx.lineTo(mx, my) : (mctx.moveTo(mx, my), started = true);
        any = true;
      }
      if (any) mctx.stroke();
    }
    mctx.restore();
  }

  // Viewport rect
  const vb = viewBounds();
  const vx1 = pad+(vb.w-minLon)*sc, vy1 = pad+(maxLat-vb.n)*sc;
  const vx2 = pad+(vb.e-minLon)*sc, vy2 = pad+(maxLat-vb.s)*sc;
  mctx.strokeStyle = '#ffd700'; mctx.lineWidth = 2;
  mctx.strokeRect(vx1, vy1, vx2-vx1, vy2-vy1);
}

// ---- Game Input ----
let loadTimer;
function initGameInput() {
  gc.addEventListener('mousedown', e => {
    G.drag = { active:true, sx:e.clientX, sy:e.clientY, slon:G.cam.lon, slat:G.cam.lat, moved:false };
    G.geo.follow = false; // manual pan disables GPS follow-mode
    gc.classList.add('dragging');
  });
  gc.addEventListener('mousemove', e => {
    if (!G.drag.active) return;
    const dx = e.clientX - G.drag.sx, dy = e.clientY - G.drag.sy;
    if (Math.abs(dx)+Math.abs(dy)>3) G.drag.moved = true;
    const s = mapScale();
    G.cam.lon = G.drag.slon - dx/s;
    G.cam.lat = G.drag.slat + dy/(s*1.35);
    render(); renderMini();
  });
  gc.addEventListener('mouseup', () => {
    gc.classList.remove('dragging');
    if (G.drag.active && G.drag.moved) {
      clearTimeout(loadTimer);
      loadTimer = setTimeout(loadMoreParcels, 600);
    }
    G.drag.active = false;
  });
  gc.addEventListener('mouseleave', () => { gc.classList.remove('dragging'); G.drag.active=false; });
  gc.addEventListener('wheel', e => {
    e.preventDefault();
    G.cam.zoom += e.deltaY > 0 ? -0.4 : 0.4;
    G.cam.zoom = Math.max(13, Math.min(20, G.cam.zoom));
    render(); renderMini();
    clearTimeout(loadTimer);
    loadTimer = setTimeout(loadMoreParcels, 600);
  }, {passive:false});
  gc.addEventListener('click', onGameClick);

  // Touch
  let touchDist = 0;
  gc.addEventListener('touchstart', e => {
    if (e.touches.length===1) {
      e.preventDefault();
      G.drag = {active:true,sx:e.touches[0].clientX,sy:e.touches[0].clientY,slon:G.cam.lon,slat:G.cam.lat,moved:false,wasPinch:false};
      G.geo.follow = false; // manual pan disables GPS follow-mode
    } else if (e.touches.length===2) {
      const dx=e.touches[0].clientX-e.touches[1].clientX, dy=e.touches[0].clientY-e.touches[1].clientY;
      touchDist = Math.sqrt(dx*dx+dy*dy);
      // Mark that we started a pinch gesture
      if (G.drag.active) G.drag.wasPinch = true;
    }
  }, {passive:false});
  gc.addEventListener('touchmove', e => {
    e.preventDefault();
    if (e.touches.length===1 && G.drag.active) {
      const dx=e.touches[0].clientX-G.drag.sx, dy=e.touches[0].clientY-G.drag.sy;
      if(Math.abs(dx)+Math.abs(dy)>3) G.drag.moved=true;
      const s=mapScale();
      G.cam.lon=G.drag.slon-dx/s; G.cam.lat=G.drag.slat+dy/(s*1.35);
      render();
    } else if (e.touches.length===2 && touchDist>0) {
      const dx=e.touches[0].clientX-e.touches[1].clientX, dy=e.touches[0].clientY-e.touches[1].clientY;
      const d=Math.sqrt(dx*dx+dy*dy);
      G.cam.zoom += (d/touchDist-1)*2;
      G.cam.zoom = Math.max(13,Math.min(20,G.cam.zoom));
      touchDist=d; render();
    }
  }, {passive:false});
  gc.addEventListener('touchend', (e) => {
    const wasTap = G.drag.active && !G.drag.moved && !G.drag.wasPinch;
    const wasPinch = G.drag.wasPinch;
    G.drag.active=false;
    clearTimeout(loadTimer);
    loadTimer=setTimeout(loadMoreParcels,600);
    // Trigger click logic for taps (touch without drag or pinch)
    if (wasTap && e.changedTouches && e.changedTouches[0]) {
      const touch = e.changedTouches[0];
      // Create a synthetic event with clientX/clientY for onGameClick
      onGameClick({clientX: touch.clientX, clientY: touch.clientY});
    }
    // Reset pinch zoom tracking when all touches end
    if (e.touches.length === 0) {
      touchDist = 0;
    }
  });

  // Zoom buttons
  document.getElementById('btn-zoomin').onclick = () => { G.cam.zoom=Math.min(20,G.cam.zoom+0.5); render(); renderMini(); };
  document.getElementById('btn-zoomout').onclick = () => { G.cam.zoom=Math.max(13,G.cam.zoom-0.5); render(); renderMini(); };
  document.getElementById('btn-gearth').onclick = () => {
    // Open Google Maps satellite view at current camera position
    // Map game zoom (13-20) to Google Maps zoom: game z13→GM z13, game z20→GM z18
    const gmZoom = Math.round(13 + (G.cam.zoom - 13) * 5/7);
    const url = 'https://www.google.com/maps/@'+G.cam.lat.toFixed(6)+','+G.cam.lon.toFixed(6)+','+gmZoom+'z/data=!3m1!1e3';
    window.open(url, '_blank');
  };

  // Share this exact viewport: invite link with #v= camera hash
  document.getElementById('btn-share').onclick = async () => {
    const code = G.session && G.session.invite_code;
    if (!code) { toast('Kein Einladungscode verfügbar', 'err'); return; }
    const url = inviteUrl(code);
    // Prefer native share sheet on mobile, clipboard otherwise
    if (navigator.share && /Mobi|Android/i.test(navigator.userAgent)) {
      try {
        await navigator.share({ title: 'Siedler Österreich', text: tr('Komm zu mir auf die Karte!'), url });
        return;
      } catch(e) { if (e.name === 'AbortError') return; /* fall through to clipboard */ }
    }
    try {
      await navigator.clipboard.writeText(url);
      toast('🔗 Link zu dieser Ansicht kopiert — einfach weiterschicken!', 'ok');
    } catch(e) {
      prompt(tr('Link kopieren:'), url);
    }
  };

  // Natura-2000 layer toggle
  const n2kBtn = document.getElementById('btn-n2k');
  n2kBtn.onclick = () => {
    G.n2kVisible = !G.n2kVisible;
    n2kBtn.classList.toggle('off', !G.n2kVisible);
    toast(G.n2kVisible ? '🛡️ Schutzgebiete sichtbar' : '🛡️ Schutzgebiete ausgeblendet', '');
    render();
  };

  // Place-name (Flurnamen) layer toggle
  const topoBtn = document.getElementById('btn-topo');
  if (topoBtn) {
    topoBtn.classList.toggle('off', !G.topoVisible);
    topoBtn.onclick = () => {
      G.topoVisible = !G.topoVisible;
      localStorage.setItem('topoVisible', G.topoVisible ? '1' : '0');
      topoBtn.classList.toggle('off', !G.topoVisible);
      toast(G.topoVisible ? '🏷️ ' + tr('Flurnamen sichtbar') : '🏷️ ' + tr('Flurnamen ausgeblendet'), '');
      render();
    };
  }

  // Data attribution chip (CC BY 4.0 / ODbL): tap to expand, tap outside / Esc to close
  const attrib = document.getElementById('map-attrib');
  const attribBtn = document.getElementById('map-attrib-toggle');
  if (attrib && attribBtn) {
    const setOpen = (o) => { attrib.classList.toggle('open', o); attribBtn.setAttribute('aria-expanded', String(o)); };
    attribBtn.onclick = (e) => { e.stopPropagation(); setOpen(!attrib.classList.contains('open')); };
    document.addEventListener('pointerdown', (e) => { if (attrib.classList.contains('open') && !attrib.contains(e.target)) setOpen(false); });
    document.addEventListener('keydown', (e) => { if (e.key === 'Escape') setOpen(false); });
  }

  // "Developer mode": 7 quick taps on the Enhanced-Gelände badge (like
  // Android's build-number easter egg) unlock the giant tree nearest to the
  // viewport center and fly to it.
  const enhBadge = document.getElementById('enhanced-badge');
  if (enhBadge) {
    const flyToNearestTree = (unlockMsg) => {
      const trees = allTallTrees();
      if (!trees.length) { toast('🌲 Noch keine Riesenbäume geladen …', 'err'); return; }
      const mLon = 111320 * Math.cos(G.cam.lat * Math.PI/180);
      let best = null, bd = Infinity;
      for (const t of trees) {
        const d = Math.hypot((t.lon - G.cam.lon) * mLon, (t.lat - G.cam.lat) * 110540);
        if (d < bd) { bd = d; best = t; }
      }
      G.devTree = best;
      updateEnhancedBadge();
      flyTo(best.lon, best.lat, Math.max(G.cam.zoom, 16.5));
      toast((unlockMsg ? '🔓 Entdeckermodus: ' : '🌲 Nächster Riesenbaum: ') + giantTreeName(best) + ' (' + best.height_m + ' m)' + (unlockMsg ? ' freigeschaltet!' : ''), 'ok');
      render();
    };
    let devTaps = 0, devTapAt = 0;
    enhBadge.onclick = () => {
      // Already unlocked: single tap flies to the giant tree nearest the viewport
      if (G.devTree) { flyToNearestTree(false); return; }
      const now = Date.now();
      if (now - devTapAt > 2500) devTaps = 0;   // taps must be quick
      devTapAt = now;
      devTaps++;
      if (devTaps < 7) {
        const left = 7 - devTaps;
        if (devTaps >= 2) toast('✨ Noch ' + left + (left === 1 ? ' Tap' : ' Taps') + ' …', '');
        return;
      }
      devTaps = 0;
      flyToNearestTree(true);
    };
  }

  // GPS "show my location" (mobile flagship feature; requires HTTPS)
  if ('geolocation' in navigator) {
    const gpsBtn = document.getElementById('btn-gps');
    gpsBtn.style.display = '';
    gpsBtn.onclick = () => {
      if (G.geo.watching) {
        // First tap while active: re-center current location (e.g. after manual
        // pan disabled follow-mode). Only a second tap when already centered
        // actually turns GPS off.
        const [gx, gy] = toScreen(G.geo.lon, G.geo.lat);
        const centered = G.geo.follow && gc &&
          Math.abs(gx - gc.width/2) < 40 && Math.abs(gy - gc.height/2) < 40;
        if (!centered && G.geo.lon) {
          G.geo.follow = true;
          flyTo(G.geo.lon, G.geo.lat, Math.max(G.cam.zoom, 17));
          toast('📍 Auf Standort zentriert — nochmal tippen zum Ausschalten', '');
          return;
        }
        navigator.geolocation.clearWatch(G.geo.id);
        G.geo.watching = false; G.geo.follow = false; G.geo.id = null;
        gpsBtn.classList.remove('active');
        toast('📍 Standort aus', '');
        render();
        return;
      }
      gpsBtn.classList.add('active');
      toast('📍 Standort wird ermittelt…', '');
      let firstFix = true;
      G.geo.id = navigator.geolocation.watchPosition(pos => {
        G.geo.watching = true;
        G.geo.lon = pos.coords.longitude;
        G.geo.lat = pos.coords.latitude;
        G.geo.acc = pos.coords.accuracy || 0;
        const inAT = G.geo.lat > 46.3 && G.geo.lat < 49.1 && G.geo.lon > 9.5 && G.geo.lon < 17.2;
        if (firstFix) {
          firstFix = false;
          if (inAT) {
            G.geo.follow = true;
            flyTo(G.geo.lon, G.geo.lat, Math.max(G.cam.zoom, 17));
          } else {
            toast('📍 Außerhalb Österreichs — Position wird nicht angezeigt', 'err');
          }
        } else if (G.geo.follow && inAT) {
          G.cam.lon = G.geo.lon; G.cam.lat = G.geo.lat;
        }
        render();
      }, err => {
        gpsBtn.classList.remove('active');
        G.geo.watching = false;
        toast('📍 Standort nicht verfügbar: ' + (err.message||''), 'err');
      }, { enableHighAccuracy: false, maximumAge: 5000, timeout: 20000 });
    };
  }

  // Keyboard
  document.addEventListener('keydown', e => {
    if (e.target.tagName==='INPUT') {
      // ESC closes search dropdown and blurs
      if (e.key==='Escape' && e.target.id==='game-search-input') {
        document.getElementById('game-search-results').classList.remove('open');
        e.target.blur();
      }
      return;
    }
    if (e.key==='c'||e.key==='C') document.getElementById('input-chat').focus();
    if (e.key==='/') { e.preventDefault(); const si=document.getElementById('game-search-input'); if(si) si.focus(); }
  });

  // In-game address search
  initGameSearch();
}

// ================= FLY-TO ANIMATION =================
let flyAnim = null;
function flyTo(lon, lat, zoom) {
  if (flyAnim) cancelAnimationFrame(flyAnim);
  const start = { lon:G.cam.lon, lat:G.cam.lat, zoom:G.cam.zoom };
  const t0 = performance.now();
  const dur = 800;
  function step(now) {
    const t = Math.min((now - t0) / dur, 1);
    // Ease in-out cubic
    const e = t < 0.5 ? 4*t*t*t : 1-Math.pow(-2*t+2,3)/2;
    G.cam.lon = start.lon + (lon - start.lon) * e;
    G.cam.lat = start.lat + (lat - start.lat) * e;
    G.cam.zoom = start.zoom + (zoom - start.zoom) * e;
    render(); renderMini();
    if (t < 1) { flyAnim = requestAnimationFrame(step); }
    else { flyAnim = null; clearTimeout(loadTimer); loadTimer = setTimeout(loadMoreParcels, 300); }
  }
  flyAnim = requestAnimationFrame(step);
}

// ================= IN-GAME ADDRESS SEARCH =================

/** Short two-line label for an OSM address result. */
function addrLabel(a) {
  if (a._topo) {
    const t = a._topo;
    const km = t.distance_m != null ? (t.distance_m < 950 ? Math.round(t.distance_m / 50) * 50 + ' m' : (t.distance_m / 1000).toFixed(1) + ' km') : '';
    const sub = [topoKindLabel(t), t.gemeinde_name, km].filter(Boolean).join(' · ');
    return { main: topoIcon(t) + ' ' + t.name + (t.elevation_m ? ' (' + t.elevation_m + ' m)' : ''), sub };
  }
  const ad = a.address || {};
  const parts = (a.display_name || '').split(', ');
  // First segment is the most specific (POI/house number/street)
  let main = parts.slice(0, 2).join(', ');
  if (ad.road) {
    main = ad.road + (ad.house_number ? ' ' + ad.house_number : '');
    if (parts[0] && parts[0] !== ad.road && parts[0] !== ad.house_number) main = parts[0] + ' · ' + main;
  }
  const place = ad.municipality || ad.city || ad.town || ad.village || a.nearest_kg?.gemeinde_name || '';
  const sub = [ad.postcode, place, ad.state].filter(Boolean).join(' · ');
  return { main: main || a.display_name || '?', sub };
}

/** Zoom level so the result's bbox fills a sensible part of the screen. */
function zoomForResult(a) {
  if (a._topo) return topoZoom(a._topo);
  const b = a.bbox;
  if (b && b.east > b.west) {
    const span = Math.max(b.east - b.west, (b.north - b.south) * 1.5, 1e-5);
    // mapScale: screenpx = spanLon * 2^(z-14)*25000 → solve for z
    const px = (gc ? gc.width : 900) * 0.7;
    let z = Math.log2(px / (span * 25000)) + 14;
    return Math.max(13, Math.min(18.5, z));
  }
  // House-number results get closer than street/place results
  return a.address?.house_number ? 18 : 16.5;
}

function initGameSearch() {
  const inp = document.getElementById('game-search-input');
  const dd = document.getElementById('game-search-results');
  if (!inp || !dd) return;
  let timer, seq = 0, items = [], hi = -1;

  const pick = (a) => {
    if (!a) return;
    dd.classList.remove('open');
    inp.value = addrLabel(a).main;
    inp.blur();
    flyTo(parseFloat(a.lon), parseFloat(a.lat), zoomForResult(a));
  };
  const renderDD = () => {
    if (!items.length) { dd.innerHTML = '<div class="search-item"><small>Keine Ergebnisse</small></div>'; return; }
    dd.innerHTML = items.map((a, i) => {
      const l = addrLabel(a);
      return `<div class="search-item${i===hi?' hi':''}" data-idx="${i}">${esc(l.main)}${l.sub?'<br><small>'+esc(l.sub)+'</small>':''}</div>`;
    }).join('');
    dd.querySelectorAll('.search-item[data-idx]').forEach(el => {
      el.onmousedown = e => { e.preventDefault(); pick(items[+el.dataset.idx]); };
    });
  };

  inp.addEventListener('input', () => {
    clearTimeout(timer);
    const q = inp.value.trim();
    if (q.length < 2) { dd.classList.remove('open'); items = []; return; }
    timer = setTimeout(async () => {
      const mySeq = ++seq;
      dd.innerHTML = '<div class="search-item"><small>Suche…</small></div>';
      dd.classList.add('open');
      try {
        const [res, topo] = await Promise.all([
          GET(CAD+'/search/address_osm?q='+encodeURIComponent(q)+'&limit=6').catch(() => ({data: []})),
          searchToponyms(q, 4),
        ]);
        if (mySeq !== seq) return; // stale response — a newer query is in flight
        // Official BEV names (Almen, Rieden, Gipfel, Höfe) rank above OSM when
        // they match well; weaker fuzzy hits go below the addresses.
        const strong = topo.filter(t => t._topo.score >= 0.85), weak = topo.filter(t => t._topo.score < 0.85);
        items = [...strong, ...(res.data || []), ...weak].slice(0, 8);
        hi = items.length ? 0 : -1;
        renderDD();
      } catch(e) {
        if (mySeq !== seq) return;
        dd.innerHTML = '<div class="search-item"><small>Fehler bei der Suche</small></div>';
      }
    }, 300);
  });
  inp.addEventListener('keydown', e => {
    if (e.key === 'Escape') { dd.classList.remove('open'); inp.blur(); return; }
    if (!items.length) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); hi = (hi + 1) % items.length; renderDD(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); hi = (hi - 1 + items.length) % items.length; renderDD(); }
    else if (e.key === 'Enter') { e.preventDefault(); pick(items[hi >= 0 ? hi : 0]); }
  });
  inp.addEventListener('focus', () => { if (items.length) dd.classList.add('open'); });
  // Close dropdown on outside click
  document.addEventListener('click', e => {
    if (!e.target.closest('#game-search')) dd.classList.remove('open');
  });
}

// ================= ADJACENT MUNICIPALITY DETECTION =================
G.knownMunis = new Set(); // Track municipality names we've seen parcels from
G.homeMuni = null; // The session's home municipality name

function detectAdjacentMunicipalities() {
  if (!G.session) return;
  if (!G.homeMuni) G.homeMuni = G.session.municipality_name;

  // Check KG names for new municipalities
  // KG codes have a prefix that maps to municipalities; we track by kg_code prefix (first 5 digits = gemeinde)
  const newKGs = new Set();
  for (const f of G.parcels) {
    const kg = f.properties.kg_code;
    if (kg && !G.knownMunis.has(kg)) {
      G.knownMunis.add(kg);
      newKGs.add(kg);
    }
  }
  // Also check polygon data
  for (const f of G.parcelPolys) {
    const kg = f.properties.kg_code;
    if (kg && !G.knownMunis.has(kg)) {
      G.knownMunis.add(kg);
      newKGs.add(kg);
    }
  }
}

async function checkViewportMunicipality() {
  // Reverse-geocode the view centre to spot municipality crossings.
  // Quantize to a ~110m grid: this fires on every pan, and the server caches by
  // exact query string, so raw float coords meant a cache MISS (i.e. a real
  // upstream round-trip) on literally every pan. Quantized, panning around the
  // same area is served from cache.
  const b = viewBounds();
  const qz = v => (Math.round(v / 0.001) * 0.001).toFixed(3);
  const centerLon = qz((b.w + b.e) / 2);
  const centerLat = qz((b.s + b.n) / 2);
  if (G._muniCheckKey === centerLon+','+centerLat) return;
  G._muniCheckKey = centerLon+','+centerLat;
  try {
    const res = await GET(CAD+'/search/municipalities?contains_lon='+centerLon+'&contains_lat='+centerLat+'&limit=1&format=json');
    const items = res.data || [];
    if (items.length > 0) {
      const muniName = items[0].name || items[0].gemeinde_name;
      if (muniName && G.homeMuni && muniName !== G.homeMuni && muniName !== G._lastMuniToast) {
        G._lastMuniToast = muniName;
        showMuniCrossingToast(muniName);
      } else if (muniName === G.homeMuni) {
        G._lastMuniToast = null;
        hideMuniCrossingToast();
      }
    }
  } catch(e) { /* ignore */ }
}

let muniToastTimer;
function showMuniCrossingToast(name) {
  const el = document.getElementById('muni-toast');
  if (!el) return;
  el.innerHTML = window.LANG === 'en'
    ? '\uD83D\uDDFA\uFE0F Leaving <span class="muni-name">'+esc(G.homeMuni)+'</span> — loading parcels from <span class="muni-name">'+esc(name)+'</span>'
    : '\uD83D\uDDFA\uFE0F Du verlässt <span class="muni-name">'+esc(G.homeMuni)+'</span> — Parzellen aus <span class="muni-name">'+esc(name)+'</span> werden geladen';
  el.classList.add('show');
  clearTimeout(muniToastTimer);
  muniToastTimer = setTimeout(() => el.classList.remove('show'), 5000);
}

function hideMuniCrossingToast() {
  const el = document.getElementById('muni-toast');
  if (el) el.classList.remove('show');
}

function onGameClick(e) {
  if (G.drag.moved) return;
  const rect = gc.getBoundingClientRect();
  const x = e.clientX - rect.left, y = e.clientY - rect.top;
  const [lon, lat] = toGeo(x, y);

  // Check treasures first
  for (const t of G.treasures) {
    const [tx, ty] = toScreen(t.lon, t.lat);
    const hr = treasureHitRadius();
    if (Math.hypot(tx-x, ty-(y+6*treasureScale())) < hr) { claimTreasure(t); return; }
  }

  // Similar-parcel markers (before parcel hit-testing — they sit on top)
  const simHit = hitSimilarMarker(x, y);
  if (simHit) { openSimilarResult(simHit); return; }

  // Similar-parcels edge arrow: fly to the nearest off-screen result that way
  if (G.similar && G._simEdgeArrows) {
    for (const a of G._simEdgeArrows) {
      if (Math.abs(a.ex - x) < 30 && Math.abs(a.ey - y) < 30) {
        let best = null, bestD = Infinity;
        for (const r of G.similar.data.results) {
          const [sx, sy] = toScreen(r.lon, r.lat);
          if (sx >= 0 && sx <= gc.width && sy >= 0 && sy <= gc.height) continue; // on-screen
          const ang = Math.atan2(sy - gc.height/2, sx - gc.width/2);
          let dAng = Math.abs(ang - a.ang);
          if (dAng > Math.PI) dAng = 2*Math.PI - dAng;
          if (dAng < 0.5 && r.distance_m < bestD) { bestD = r.distance_m; best = r; }
        }
        if (best) { flyTo(best.lon, best.lat, Math.max(G.cam.zoom, 15)); return; }
      }
    }
  }

  // Miracle fog hint: tapping the mist flies to the nearest giant tree
  if (fogHintPos && Math.abs(fogHintPos.x - x) < 45 && Math.abs(fogHintPos.y - y) < 45) {
    flyTo(fogHintPos.lon, fogHintPos.lat, Math.max(G.cam.zoom, 15.5));
    toast('✨ Der Nebel führt dich zu einem Riesenbaum...', 'ok');
    return;
  }

  // Giant trees: only what was actually drawn last frame is tappable.
  if (G.tallUnlocked) {
    let hit = null, hitD = Infinity;
    for (const d of _drawnTrees) {
      const hb = giantTreeHitBox(d.t, G.cam.zoom);
      if (Math.abs(d.x-x) < hb.hw && d.y-y > -hb.down && d.y-y < hb.up) {
        const dd = Math.abs(d.x-x) + Math.abs(d.y-y - hb.up/2);
        if (dd < hitD) { hitD = dd; hit = d; }
      }
    }
    if (hit && hit.hint && !G.tallRevealed) {
      // Hint tree tapped: discovery mode begins with the giants in sight.
      G.tallRevealed = true;
      G.tallRevealAt = Date.now();
      const inView = tallTreesInView().slice(0, giantDrawBudget());
      if (!inView.includes(hit.t)) inView.unshift(hit.t);
      discoverTrees(inView, true);
      const total = allTallTrees().length;
      toast('🌲 ' + tr('Riesenbaum entdeckt!') + ' ' + inView.length + ' ' + tr('Riesen in Sicht — erkunde das Land und finde alle') + ' ' + total + '. ' + tr('Grundstücke mit Riesenbäumen bringen Bonus-XP!'), 'ok');
      render();
      return;
    }
    if (hit && G.tallRevealed) { showTreePopup(hit.t); return; }
  }

  // Dev-mode tree (5-tap badge easter egg) is drawn even before reveal
  if (G.devTree) {
    const [tx, ty] = toScreen(G.devTree.lon, G.devTree.lat);
    const hb = giantTreeHitBox(G.devTree, G.cam.zoom);
    if (Math.abs(tx-x) < hb.hw && ty-y > -hb.down && ty-y < hb.up) { showTreePopup(G.devTree); return; }
  }

  // Building footprint under the tap? (buildings only render at zoom>=15)
  // IMPORTANT: a building tap still selects the underlying PARCEL — the
  // building details render as an extra section inside the parcel popup, so
  // densely built parcels stay fully clickable/buyable.
  let fpHit = null;
  if (G.cam.zoom >= 15) {
    for (const f of G.buildingFootprints) {
      const g = f.geometry;
      if (!g || g.type !== 'Polygon') continue;
      const b = f._bb || (f._bb = geoBounds(g));
      if (lon < b.w || lon > b.e || lat < b.s || lat > b.n) continue;
      if (pip(lon, lat, g.coordinates[0])) { fpHit = f; break; }
    }
  }

  // Check polygon parcels
  for (const f of G.parcelPolys) {
    if (pipGeom(lon, lat, f.geometry)) {
      showParcelPopup(f, fpHit); return;
    }
  }

  // Check point parcels
  let best=null, bestD=Infinity;
  for (const f of G.parcels) {
    const p=f.properties;
    const plon=p.lon||f.geometry.coordinates[0], plat=p.lat||f.geometry.coordinates[1];
    const d=Math.abs(plon-lon)+Math.abs(plat-lat);
    if (d<bestD && d<0.0005) { bestD=d; best=f; }
  }
  if (best) { showParcelPopup(best, fpHit); return; }

  document.getElementById('parcel-popup').classList.remove('open');
  document.getElementById('ez-popup').classList.remove('open');
  document.getElementById('tree-popup').classList.remove('open');
  resetPopupPosition('parcel-popup');
  resetPopupPosition('ez-popup');
  resetPopupPosition('tree-popup');
  G.sel = null; G.selFp = null; G.ezHighlight = null; render();
}

function showParcelPopup(f, tappedFp) {
  G.sel = f;
  G.selFp = tappedFp || null;
  const p = f.properties;
  const pid = p.parcel_id;

  // Auto-hide the giant tree info popup when a parcel is tapped.
  const treePop = document.getElementById('tree-popup');
  if (treePop && treePop.classList.contains('open')) {
    treePop.classList.remove('open');
    resetPopupPosition('tree-popup');
  }

  // Keep camera stable on parcel tap (no zoom jumps — important on mobile).
  // Only nudge the view if the parcel is off-screen (e.g. re-opened programmatically).
  const [pLon, pLat] = featureLonLat(f);
  if (gc) {
    const [sx, sy] = toScreen(pLon, pLat);
    const m = 40; // margin
    if (sx < m || sx > gc.width - m || sy < m || sy > gc.height - m) {
      animateCamera(pLon, pLat, G.cam.zoom, 350); // pan only, keep zoom
    }
  }
  // Enrich polygon data with point data (has building_count, total_building_area_sqm, landuse_codes)
  const pointF = G.parcels.find(pf => pf.properties.parcel_id === pid);
  if (pointF) {
    for (const [k,v] of Object.entries(pointF.properties)) {
      if (!(k in p) || p[k] === undefined || p[k] === null) p[k] = v;
    }
  }
  const claim = G.claimed.find(c=>c.parcel_id===pid);
  const owner = claim ? G.players.find(pl=>pl.id===claim.player_id) : null;
  const luCode = extractLuCode('', p);
  const area = p.area_sqm||0;
  const bldgCount = p.building_count || 0;
  const bldgArea = p.total_building_area_sqm || 0;
  const price = calcPrice(area, luCode, bldgCount, bldgArea);

  document.getElementById('pp-title').textContent = '📍 ' + (p.gnr || pid);
  document.getElementById('pp-id').textContent = pid;
  const kgEl = document.getElementById('pp-kg');
  if (p.kg_code) {
    kgEl.innerHTML = `<span class="pp-ez-link" onclick="openKGSummary('${p.kg_code}')">${esc(p.kg_name || p.kg_code)} ▸</span>`;
  } else {
    kgEl.textContent = p.kg_name || '-';
  }
  const ez = p.ez || '';
  document.getElementById('pp-ez').textContent = ez ? 'EZ ' + ez : '-';
  renderFlurRow(f);
  document.getElementById('pp-area').textContent = area>10000?(area/10000).toFixed(2)+' ha':Math.round(area)+' m²';
  document.getElementById('pp-use').textContent = getLanduseName(p);
  // Density label based on built-up ratio
  let densityLabel = 'Keine';
  if (area > 0 && (bldgCount > 0 || bldgArea > 0)) {
    const ratio = bldgArea / area;
    const bc = bldgCount ? ' (' + bldgCount + ' Geb.)' : '';
    if (ratio > 0.3) densityLabel = '🏙️ Dicht' + bc;
    else if (ratio > 0.05) densityLabel = '🏡 Mittel' + bc;
    else if (ratio > 0.001) densityLabel = '🌾 Gering' + bc;
    else densityLabel = '🌾 Minimal';
  }
  document.getElementById('pp-density').textContent = densityLabel;
  document.getElementById('pp-owner').textContent = owner ? owner.name : 'Frei';
  const fieldEl = document.getElementById('pp-field'), fieldL = document.getElementById('pp-field-l');
  if (isCropField(p)) {
    fieldEl.style.display = fieldL.style.display = '';
    fieldL.textContent = tr('Feld');
    fieldEl.textContent = fieldStageLabel(fieldStage(p, claim));
  } else if (claim && isForestParcel(G.sel, claim) && (claim.harvested_at || claim.converted_to === 'wildforest')) {
    fieldEl.style.display = fieldL.style.display = '';
    fieldL.textContent = tr('Wald');
    fieldEl.textContent = claim.converted_to === 'wildforest' ? '🌳 ' + tr('Naturwald') + ' · ' + tr('außer Nutzung') : forestStageLabel(forestStage(claim));
  } else { fieldEl.style.display = fieldL.style.display = 'none'; }
  {
    // FARM-2: the real crop on this field (AMA INVEKOS Schlag under the parcel centroid)
    const cropEl = document.getElementById('pp-crop'), cropL = document.getElementById('pp-crop-l');
    const sf = cropEl ? parcelSchlag(p) : null;
    if (sf) {
      cropEl.style.display = cropL.style.display = '';
      cropL.textContent = tr('Anbau');
      cropEl.textContent = cropLabel(sf);
      cropEl.title = 'INVEKOS ' + (G.schlagYear || '') + ' · AMA, CC BY 4.0';
    } else if (cropEl) cropEl.style.display = cropL.style.display = 'none';
  }
  document.getElementById('pp-price').textContent = claim ? (claim.player_id===G.player.id?'Dein Besitz':'Besetzt') : price+' 🪙';

  renderBuildingRows(tappedFp);
  renderEnhancedPopupRows(pid, price);
  renderSimilarPopupRows(pid);

  const act = document.getElementById('pp-actions');
  act.innerHTML = '';
  if (!claim) {
    act.innerHTML = `<button class="btn btn-primary btn-small" onclick="doClaim()">🏴 Kaufen (${price}🪙)</button>`;
  } else if (claim.player_id === G.player.id && !claim.converted_to) {
    // My parcel — show harvest/convert/sell + any incoming offers
    let html = '';
    if (isCropField(p)) {
      const fs = fieldStage(p, claim);
      if (fs.stage === 'ripe') html += `<button class="btn btn-gold btn-small" onclick="doHarvest()">🌾 Ernten (+${harvestYield(area)}🪙)</button>`;
    }
    if (isForestParcel(G.sel, claim)) {
      // Forest stand: harvest the timber (coins now, stand regrows) or set it
      // aside as Naturwald (XP, permanent). Values come from /api/forest-value.
      const fs = forestStage(claim), fv = G.forestValues[pid], e = fv?.estimate;
      const coinsNow = e ? Math.max(5, Math.round(e.coins * fs.factor)) : null;
      if (fs.stage === 'baumholz') html += `<button class="btn btn-gold btn-small" onclick="doHarvestForest()">🪓 ${tr('Holzernte')} (${coinsNow != null ? '+' + coinsNow + '🪙' : '…'})</button>`;
      else html += `<span style="font:16px VT323;color:var(--text-dim);width:100%">${forestStageLabel(fs)}</span>`;
      html += `<button class="btn btn-primary btn-small" onclick="doConvert('wildforest')" ${fs.stage !== 'baumholz' ? 'disabled title="' + tr('Der Wald muss erst nachwachsen') + '"' : ''}>🌳 ${tr('Naturwald')} (+${e ? e.wild_xp : '…'}⚡)</button>`;
      if (!(pid in G.forestValues)) fetchForestValue(G.sel).then(() => { if (G.sel && G.sel.properties.parcel_id === pid) showParcelPopup(G.sel, G.selFp); });
    } else {
      html += `
      <button class="btn btn-primary btn-small" onclick="doConvert('biodiversity')">🌿 ${isCropField(p) ? tr('Brache') : tr('Naturschutz')}</button>
      <button class="btn btn-secondary btn-small" onclick="doConvert('forest')">🌳 Aufforsten</button>`;
    }
    html += `<button class="btn btn-danger btn-small" onclick="doSell(${claim.id})">💰 Verkaufen</button>`;
    // Show incoming offers for this parcel
    const incomingOffers = (G.offers||[]).filter(o => o.parcel_id === pid && o.seller_id === G.player.id && o.status === 'pending');
    if (incomingOffers.length > 0) {
      html += `<div style="width:100%;margin-top:8px;border-top:1px solid var(--panel-border);padding-top:8px">`;
      html += `<span style="font:8px var(--font-pixel);color:var(--gold)">📨 Kaufangebote:</span>`;
      for (const o of incomingOffers) {
        html += `<div style="display:flex;align-items:center;gap:6px;margin-top:4px;font:18px VT323;color:var(--text)">`;
        html += `<span>${esc(o.buyer_name)}: ${o.offer_price}🪙</span>`;
        html += `<button class="btn btn-primary btn-small" style="padding:3px 8px;font-size:7px" onclick="doRespondOffer(${o.id},true)">✓</button>`;
        html += `<button class="btn btn-danger btn-small" style="padding:3px 8px;font-size:7px" onclick="doRespondOffer(${o.id},false)">✗</button>`;
        html += `</div>`;
      }
      html += `</div>`;
    }
    act.innerHTML = html;
  } else if (claim.player_id === G.player.id) {
    const convLabel = claim.converted_to === 'wildforest' ? '🌳 ' + tr('Naturwald') : claim.converted_to === 'biodiversity' ? (isCropField(p) ? tr('Naturschutz') + ' · ' + tr('Brache') : tr('Naturschutz')) : claim.converted_to === 'forest' ? tr('Aufforstung') : claim.converted_to;
    act.innerHTML = `<span style="font:18px VT323;color:var(--green-light)">✅ ${convLabel}</span>`;
  } else {
    // Someone else's parcel — offer to buy
    const myOffer = (G.offers||[]).find(o => o.parcel_id === pid && o.buyer_id === G.player.id && o.status === 'pending');
    if (myOffer) {
      act.innerHTML = `<span style="font:18px VT323;color:var(--gold)">📨 Angebot: ${myOffer.offer_price}🪙 (wartet)</span>`;
    } else {
      const suggestedPrice = Math.round(price * 1.5);
      act.innerHTML = `
        <div style="width:100%">
          <span style="font:8px var(--font-pixel);color:var(--text-dim);display:block;margin-bottom:4px">Kaufangebot an ${esc(owner.name)}:</span>
          <div style="display:flex;gap:6px;align-items:stretch">
            <input type="number" id="offer-price-input" value="${suggestedPrice}" min="10" max="99999" 
              style="flex:1;padding:6px 8px;font:20px VT323;background:var(--bg);color:var(--text-bright);border:2px solid var(--panel-border);width:80px">
            <button class="btn btn-gold btn-small" onclick="doMakeOffer()">📨 Anbieten</button>
          </div>
        </div>`;
    }
  }

  // Similar parcels search (cadastre R-tree + srtm terrain matching)
  act.innerHTML += `<div class="similar-row">
    <button class="btn btn-secondary btn-small" id="pp-similar-btn" onclick="findSimilarParcels()">🔍 Ähnliche Parzellen</button>
    <span class="similar-radius" id="pp-similar-radius">${[5000,10000,20000,50000].map(r =>
      `<button class="sim-r${r===G.similarRadius?' on':''}" onclick="setSimilarRadius(${r})">${r/1000}</button>`).join('')}<i>km</i></span>
  </div>`;
  // Lazy count: prefetch the current radius in background, show "(N)" when it lands
  prefetchSimilarCount(pid);

  // EZ link — make the EZ field clickable to open separate EZ popup
  const ezEl = document.getElementById('pp-ez');
  if (ez && p.kg_code) {
    const ezKey = p.kg_code + '-EZ' + ez;
    const ezParcels = G.ezIndex[ezKey] || [];
    if (ezParcels.length > 1) {
      ezEl.innerHTML = `<span class="pp-ez-link" onclick="openEZPopup('${p.kg_code}','${ez}')">EZ ${ez} ▸ (${ezParcels.length} Parzellen)</span>`;
      G.ezHighlight = {kg: p.kg_code, ez: ez};
    } else {
      ezEl.textContent = ez ? 'EZ ' + ez : '-';
      G.ezHighlight = null;
    }
  } else {
    ezEl.textContent = '-';
    G.ezHighlight = null;
  }

  // Close EZ popup if open (will reopen if user clicks link)
  document.getElementById('ez-popup').classList.remove('open');

  document.getElementById('parcel-popup').classList.add('open');
  // Reset inline position so CSS handles it (mobile vs desktop)
  const pp = document.getElementById('parcel-popup');
  if (!pp.dataset.userMoved) {
    pp.style.left = ''; pp.style.bottom = '';
    pp.style.right = ''; pp.style.top = '';
  }
  render();
}

// ---- Building tap: extra section inside the parcel popup ----

/** Lazily fetch merged building info (cadastre metrics + parcel links + addresses). */
async function fetchBuildingInfo(fpId, lon, lat) {
  if (fpId in G.bldgInfo) return G.bldgInfo[fpId];
  G.bldgInfo[fpId] = null; // in-flight guard
  try {
    const d = await GET('/api/building-info?fp=' + encodeURIComponent(fpId) + '&lon=' + lon + '&lat=' + lat);
    G.bldgInfo[fpId] = (d && !d.error) ? d : null;
  } catch(e) { G.bldgInfo[fpId] = null; }
  return G.bldgInfo[fpId];
}

// Footprint ns_code → label. Footprints carry BEV NS codes too; in practice
// only 41 (Gebäude) and 83 (Gebäudenebenfläche) occur on building polygons.
const NS_NAMES = {'41':'Gebäude','42':'Parkplatz','83':'Gebäudenebenfläche'};

// ---- Collapsible popup sections (pixel-art headers) ----
G.ppSec = { bldg: true, env: window.innerWidth >= 768 }; // remembered per session
function ppSecSync(name) {
  const sec = document.getElementById('pp-sec-' + name);
  if (!sec) return;
  sec.classList.toggle('open', !!G.ppSec[name]);
}
document.querySelectorAll('.pp-sec-h').forEach(btn => {
  btn.onclick = () => {
    const n = btn.dataset.sec;
    G.ppSec[n] = !G.ppSec[n];
    ppSecSync(n);
  };
});

/** Render the tapped building's section in the parcel popup.
 *  Instant rows come from data already on the client (footprint metrics from
 *  the viewport payload + lidar height match); addresses arrive lazily. */
function renderBuildingRows(fp) {
  const sec = document.getElementById('pp-sec-bldg');
  const box = document.getElementById('pp-bldg');
  if (!box) return;
  if (!fp) { sec.style.display = 'none'; box.innerHTML = ''; return; }

  const p = fp.properties || {};
  const fpId = p.footprint_id;
  const rows = [];

  // Size: real footprint area + oriented dims (already in viewport payload)
  if (p.area_sqm) {
    let dims = '';
    if (p.obb_length_m && p.obb_width_m) dims = ' · ' + Math.round(p.obb_length_m) + '×' + Math.round(p.obb_width_m) + ' m';
    rows.push(['📏 Grundfläche', Math.round(p.area_sqm) + ' m²' + dims]);
  }
  if (p.ns_code && NS_NAMES[p.ns_code]) rows.push(['🏷️ Typ', NS_NAMES[p.ns_code]]);

  // LiDAR height/stories/roof — match by centroid like the renderer does
  let cx = 0, cy = 0, ring = fp.geometry && fp.geometry.type === 'Polygon' ? fp.geometry.coordinates[0] : null;
  if (ring) {
    for (const c of ring) { cx += c[0]; cy += c[1]; }
    cx /= ring.length; cy /= ring.length;
    const lb = findLidarBuilding(cx, cy);
    if (lb && lb.max_height_m) {
      let h = '≈ ' + Math.round(lb.max_height_m) + ' m';
      if (lb.stories_est > 0) h += ' · ' + lb.stories_est + ' Etage' + (lb.stories_est > 1 ? 'n' : '');
      rows.push(['📐 Höhe (LiDAR)', h]);
      if (lb.roof_type_hint) rows.push(['🏠 Dach', lb.roof_type_hint === 'flat' ? 'Flachdach' : 'Steildach']);
    }
  }
  if (p.orientation_axis) rows.push(['🧭 Ausrichtung', p.orientation_axis]);

  box.innerHTML = '<div class="pp-grid">' +
    rows.map(([k,v]) => '<span>'+k+'</span><b>'+v+'</b>').join('') +
    '</div><div class="pp-bldg-lazy" id="pp-bldg-lazy"></div>';
  sec.style.display = '';
  ppSecSync('bldg');

  // Lazy: addresses + multi-parcel span from the server aggregate
  if (!fpId) return;
  const lon = (p.lon != null ? p.lon : cx), lat = (p.lat != null ? p.lat : cy);
  fetchBuildingInfo(fpId, lon, lat).then(info => {
    // Popup may have moved on to another selection meanwhile
    if (!G.selFp || G.selFp.properties.footprint_id !== fpId) return;
    if (!info) return;
    const lazy = document.getElementById('pp-bldg-lazy');
    if (!lazy) return;
    let html = '';
    if (info.addresses && info.addresses.length) {
      html += '<div class="pp-bldg-addrs">📫 ' + info.addresses.map(esc).join('<br>📫 ') + '</div>';
    }
    if (info.parcels && info.parcels.length > 1) {
      html += '<div class="pp-bldg-span">⚠️ Gebäude erstreckt sich über ' + info.parcels.length + ' Parzellen</div>';
    }
    lazy.innerHTML = html;
  });
}

// ---- KG summary popup (tap the KG name in the parcel popup) ----

async function openKGSummary(kg) {
  const pop = document.getElementById('kg-popup');
  const body = document.getElementById('kg-body');
  document.getElementById('kg-title').textContent = '🏘️ KG ' + kg;
  body.innerHTML = '<div class="kg-loading">Lädt…</div>';
  pop.classList.add('open');
  let d = G.kgSummaries[kg];
  if (!d) {
    // Retry transient upstream failures (server answers 503 with a retryable
    // error, vs 404 for a genuinely unknown code) rather than flashing
    // "Keine Daten verfügbar" at the player for a perfectly valid KG.
    for (let attempt = 0; attempt < 3; attempt++) {
      if (attempt > 0) await new Promise(r => setTimeout(r, 600 * attempt));
      try { d = await GET('/api/kg-summary/' + encodeURIComponent(kg)); } catch(e) { d = null; }
      if (d && !d.error) break;
      if (d && d.error === 'unknown KG code') break; // permanent — don't retry
      if (!pop.classList.contains('open')) return;  // player closed it
    }
    if (d && !d.error) G.kgSummaries[kg] = d;
  }
  if (!d || d.error) {
    body.innerHTML = '<div class="kg-loading">' +
      (d && d.error === 'unknown KG code' ? 'Keine Daten verfügbar'
        : 'Daten momentan nicht erreichbar — bitte nochmal antippen') + '</div>';
    return;
  }

  document.getElementById('kg-title').textContent = '🏘️ ' + (d.kg_name || 'KG ' + kg);
  const rows = [];
  if (d.gemeinde_name) rows.push(['Gemeinde', esc(d.gemeinde_name)]);
  rows.push(['KG-Code', kg]);
  if (d.area_ha != null) rows.push(['Fläche', d.area_ha >= 100 ? Math.round(d.area_ha) + ' ha' : d.area_ha.toFixed(1) + ' ha']);
  if (d.parcels != null) rows.push(['Parzellen', d.parcels.toLocaleString('de-AT')]);
  if (d.buildings != null) rows.push(['Gebäude', d.buildings.toLocaleString('de-AT')]);
  if (d.avg_area_sqm != null) rows.push(['Ø Parzelle', Math.round(d.avg_area_sqm) + ' m²']);
  if (d.elev_min != null && d.elev_max != null) {
    const tl = {level:'eben', nearly_level:'fast eben', 'nearly level':'fast eben', gentle:'sanft', undulating:'wellig', moderate:'mäßig', hilly:'hügelig', steep:'steil', mountainous:'gebirgig', rugged:'schroff', 'slightly rugged':'leicht schroff'};
    rows.push(['⛰️ Seehöhe', Math.round(d.elev_min) + '–' + Math.round(d.elev_max) + ' m' + (d.terrain_class ? ' · ' + (tl[d.terrain_class]||tl[String(d.terrain_class).replace(/_/g,' ')]||d.terrain_class) : '')]);
  }
  if (d.tallest_tree_m) rows.push(['🌲 Höchster Baum', d.tallest_tree_m + ' m' + (d.giant_trees ? ' (' + d.giant_trees + ' Riesen)' : '')]);
  if (d.n2k_parcels > 0) rows.push(['🛡️ Natura 2000', d.n2k_parcels.toLocaleString('de-AT') + ' Parzellen' + (d.n2k_sites && d.n2k_sites.length ? '<br><i class="kg-dim">' + d.n2k_sites.map(esc).join(', ') + '</i>' : '')]);
  if (d.legal_refs > 0) rows.push(['⚖️ Rechtsbezüge', d.legal_refs + (d.legal_contexts && d.legal_contexts.length ? ' · ' + d.legal_contexts.slice(0,3).map(esc).join(', ') : '')]);

  // My claims in this KG (client-side, free)
  const mine = G.claimed.filter(c => c.kg_code === kg && c.player_id === G.player.id);
  if (mine.length) {
    const ha = mine.reduce((s,c) => s + (c.area_sqm||0), 0) / 10000;
    rows.push(['🏴 Dein Besitz', mine.length + ' Parzellen · ' + (ha >= 1 ? ha.toFixed(1) + ' ha' : Math.round(ha*10000) + ' m²')]);
  }

  let html = '<div class="pp-grid">' + rows.map(([k,v]) => '<span>'+k+'</span><b>'+v+'</b>').join('') + '</div>';

  // Landuse breakdown as a compact bar
  if (d.landuse && d.landuse.length && d.parcels > 0) {
    const total = d.landuse.reduce((s,e) => s + (e.count||0), 0);
    let seg = '', leg = '';
    for (const e of d.landuse) {
      const fr = (e.count||0) / total;
      if (fr < 0.02) continue;
      const col = (LANDUSE_POLY_COLORS[e.code] && LANDUSE_POLY_COLORS[e.code].fill) || '#888';
      // Prefer our short German NS name; upstream labels like
      // "Äcker, Wiesen oder Weiden - LN" are too long for the legend.
      const nm = (NS_TABLE[String(e.code)] && NS_TABLE[String(e.code)].name)
        || (e.name||'').split(' - ')[0].split(' (')[0];
      seg += '<i style="width:' + (fr*100).toFixed(1) + '%;background:' + col + '"></i>';
      if (leg.split('<em').length <= 4) leg += '<em><i style="background:' + col + '"></i>' + esc(nm) + ' ' + Math.round(fr*100) + '%</em>';
    }
    html += '<div class="kg-lu-title">Nutzung (nach Parzellenzahl)</div><div class="fracs-bar">' + seg + '</div><div class="fracs-legend">' + leg + '</div>';
  }
  if (d.enhanced) html += '<div class="kg-enh">✨ Enhanced — LiDAR-Geländedaten aktiv</div>';
  html += '<div class="kg-lu-title"><span class="pp-ez-link" onclick="openDossier(\'' + kg + '\')">📖 ' + tr('Gemeinde-Chronik') + ' ▸</span> <span class="kg-dim">' + tr('Wasser · Wald · Höfe') + '</span></div>';
  body.innerHTML = html;
}

/** Enhanced-mode rows in the parcel popup: elevation, slope, vegetation, market value, Natura 2000. */
function renderEnhancedPopupRows(pid, gamePrice) {
  const box = document.getElementById('pp-enhanced');
  const sec = document.getElementById('pp-sec-env');
  if (!box) return;
  const rows = [];
  const moreRows = [];  // secondary rows, rendered after the primary ones

  const lp = G.lidarParcels[pid];
  if (lp) {
    if (lp.elev != null) {
      let range = '';
      if (lp.elevMin != null && lp.elevMax != null && (lp.elevMax - lp.elevMin) >= 1) {
        range = ' <span style="color:var(--text-dim)">(' + Math.round(lp.elevMin) + '–' + Math.round(lp.elevMax) + 'm)</span>';
      }
      rows.push(['⛰️ Höhe', Math.round(lp.elev) + ' m' + range]);
    }
    if (lp.slope != null) {
      const arrows = {N:'↑',NE:'↗',E:'→',SE:'↘',S:'↓',SW:'↙',W:'←',NW:'↖'};
      const tlabels = {level:'eben', nearly_level:'fast eben', 'nearly level':'fast eben', gentle:'sanft', undulating:'wellig', moderate:'mäßig', hilly:'hügelig', steep:'steil', rugged:'schroff', 'slightly rugged':'leicht schroff', mountainous:'gebirgig'};
      rows.push(['⛰️ Hang', lp.slope.toFixed(1) + '° ' + (arrows[lp.aspect]||'') + (lp.tclass ? ' · ' + (tlabels[lp.tclass]||tlabels[String(lp.tclass).replace(/_/g,' ')]||lp.tclass) : '')]);
    }
    // Land-cover composition: 1m-resolution srtm fracs, corrected against
    // cadastre building/landuse data (roof + road bleed). Falls back to the
    // plain dominant-type row when no fracs are available.
    const cf = correctedFracs(lp.fracs, G.sel?.properties || {});
    if (cf) {
      rows.push(['🌿 Bewuchs', fracsBarHTML(cf)]);
    } else {
      const domShown = lp.domTerrain || lp.dom;
      if (domShown) {
        const domDE = {grass:'Wiese', tree:'Baumbestand', roof:'Bebaut', crop:'Acker', water:'Wasser', bare:'Offen', bare_soil:'Offen', road:'Straße', shrub:'Gestrüpp', hedge:'Hecke', garden:'Garten', vineyard:'Weingarten'};
        let veg = domDE[domShown] || domShown;
        if (lp.forestFrac != null && lp.forestFrac > 0.02) veg += ' · ' + Math.round(lp.forestFrac*100) + '% Wald';
        moreRows.push(['🌿 Bewuchs', veg]);
      }
    }
  }

  // Giant-tree bonus (only after reveal)
  if (G.tallRevealed && G.sel) {
    const tt = tallTreesInParcel(G.sel);
    if (tt.count > 0) {
      const bonus = Math.min(300, tt.count*40 + Math.floor(tt.maxH));
      rows.push(['🌲 Riesenbaum', tt.count + '× (max ' + tt.maxH + 'm) — <b style="color:var(--gold)">+' + bonus + '⚡ Bonus</b>']);
    }
  }

  // Timber stock + harvest value for forest stands (lazy; timber.go)
  if (G.sel && isForestParcel(G.sel, G.claimed?.find(c => c.parcel_id === pid))) {
    const claim = G.claimed?.find(c => c.parcel_id === pid);
    if (pid in G.forestValues) { if (G.forestValues[pid]) for (const r of forestPopupRows(G.forestValues[pid], claim)) rows.push(r); }
    else fetchForestValue(G.sel).then(() => { if (G.sel && G.sel.properties.parcel_id === pid) showParcelPopup(G.sel, G.selFp); });   // re-render rows + action buttons
  }

  // Natura 2000: is parcel inside a loaded site polygon?
  const [pLon, pLat] = G.sel ? featureLonLat(G.sel) : [null, null];
  if (pLon != null) {
    for (const code in G.n2kSites) {
      const st = G.n2kSites[code];
      if (!st.geom) continue;
      if (geoContains(st.geom, pLon, pLat)) {
        const habEmoji = (st.habitats||[]).map(h => ({forest:'🌲',meadow:'🦋',floodplain:'💧',water:'💧',bog:'🌿'}[h]||'🌿')).join('');
        rows.push(['🛡️ Natura 2000', esc(st.name.slice(0,36)) + ' ' + habEmoji]);
        break;
      }
    }
  }

  // OSM proximity rows (lazy-loaded; skip when not yet fetched)
  const osmRows = [];
  const osm = G.osmProx[pid];
  if (osm) {
    const nm = (n) => n ? ' <span style="color:var(--text-dim)">' + esc(String(n).slice(0,24)) + '</span>' : '';
    if (osm.dist_road_m != null) osmRows.push(['🛣️ Straße', fmtDist(osm.dist_road_m) + nm(osm.road_name) + (osm.road_on_parcel ? ' <span style="color:var(--text-dim)">(am Grundstück)</span>' : '')]);
    if (osm.dist_transit_m != null) osmRows.push(['🚌 Öffi', fmtDist(osm.dist_transit_m) + nm(osm.transit_name)]);
    if (osm.dist_train_station_m != null) osmRows.push(['🚉 Bahnhof', fmtDist(osm.dist_train_station_m) + nm(osm.train_station_name)]);
    if (osm.dist_water_m != null) osmRows.push(['💧 Gewässer', fmtDist(osm.dist_water_m) + nm(osm.water_name)]);
    if (osm.dist_settlement_m != null) osmRows.push(['🏘️ Ort', fmtDist(osm.dist_settlement_m) + nm(osm.settlement_name)]);
    if (osm.remoteness != null) {
      const r = osm.remoteness;
      const lbl = r < 20 ? 'zentral' : r < 45 ? 'gut erschlossen' : r < 70 ? 'ländlich' : 'abgelegen';
      osmRows.push(['🧭 Lage', Math.round(r) + '/100 <span style="color:var(--text-dim)">' + lbl + '</span>']);
    }
    for (const row of osmRows) moreRows.push(row);
  }

  const renderRows = () => {
    const list = rows.concat(moreRows);
    if (list.length === 0 && !(pid in G.landPrices)) { sec.style.display = 'none'; return; }
    let html = '';
    for (const [k, v] of list) html += '<span>' + k + '</span><b>' + v + '</b>';
    // Market value row (lazy loaded)
    const mv = G.landPrices[pid];
    if (mv) {
      // Upstream (Aug 2026) derives `class` from the AREA split of the parcel when
      // available (class_source="area") and then also reports a blended total
      // across the actual landuse mix — prefer it; the single-class total can be
      // wildly off on mixed parcels (a field with one shed glyph priced as Bauland).
      const total = mv.buy_total_blended_eur != null ? mv.buy_total_blended_eur : mv.buy_total_eur;
      const eur = total >= 1e6 ? (total/1e6).toFixed(2) + ' Mio €' : Math.round(total).toLocaleString('de-AT') + ' €';
      const cls = {bauland_built:'Bauland (bebaut)', bauland_zoned:'Bauland', ackerland:'Ackerland', gruenland:'Grünland', wald:'Wald', other:'Sonstig'}[mv.class] || mv.class;
      const share = (mv.class_source === 'area' && mv.class_share != null && mv.class_share < 0.95)
        ? ' ' + Math.round(mv.class_share*100) + '%' : '';
      const approx = mv.class_source === 'symbol' ? '≈ ' : '';
      html += '<span>💶 Marktwert</span><b style="color:var(--gold)">' + approx + eur + ' <span style="color:var(--text-dim)">(' + cls + share + ')</span></b>';
      if (gamePrice > 0) {
        const perSqm = (mv.area_sqm > 0 ? total / mv.area_sqm : mv.buy_eur_per_sqm) || 0;
        html += '<span></span><b style="color:var(--text-dim);font-size:14px">Spielpreis: ' + gamePrice + '🪙 · ' + Math.round(perSqm) + ' €/m² echt</b>';
      }
    }
    box.innerHTML = html;
    sec.style.display = html ? '' : 'none';
    if (html) ppSecSync('env');
  };

  renderRows();

  // Lazy market value fetch (only for enhanced... actually land_prices covers most of AT — always try)
  if (!(pid in G.landPrices)) {
    fetchLandPrice(pid).then(() => {
      if (G.sel && G.sel.properties.parcel_id === pid) renderRows();
    });
  }

  // Lazy OSM proximity fetch (first call per KG can be slow upstream — never blocks)
  if (!(pid in G.osmProx)) {
    fetchOsmProx(pid).then((o) => {
      if (o && G.sel && G.sel.properties.parcel_id === pid) renderEnhancedPopupRows(pid, gamePrice);
    });
  }
}

// ================= SIMILAR PARCELS (cadastre R-tree + srtm terrain) =================

/** Fit camera to a bbox with margin; zoom clamped to [13, maxZoom]. */
function fitBBox(minLon, minLat, maxLon, maxLat, maxZoom) {
  const cLon = (minLon + maxLon) / 2, cLat = (minLat + maxLat) / 2;
  const spanLon = Math.max(maxLon - minLon, 1e-5) * 1.25;
  const spanLat = Math.max(maxLat - minLat, 1e-5) * 1.25;
  const s = Math.min(gc.width / spanLon, gc.height / (spanLat * 1.35));
  let zoom = Math.log2(s / 25000) + 14;
  zoom = Math.max(13, Math.min(maxZoom || 20, zoom));
  flyTo(cLon, cLat, zoom);
}

function similarQueryFor(f) {
  const p = f.properties;
  const [pLon, pLat] = featureLonLat(f);
  return { pid: p.parcel_id, pLon, pLat, params: new URLSearchParams({
    parcel_id: p.parcel_id, lon: pLon, lat: pLat,
    area: p.area_sqm || 0,
    bcount: p.building_count || 0,
    barea: p.total_building_area_sqm || 0,
  })};
}

async function fetchSimilar(f, radius) {
  const { pid, params } = similarQueryFor(f);
  const key = pid + ':' + radius;
  if (G.similarCache[key]) return G.similarCache[key];
  params.set('radius', radius);
  const d = await GET('/api/similar?' + params.toString());
  if (d && !d.error && d.results) G.similarCache[key] = d;
  return d;
}

function similarBtnLabel(pid) {
  const cached = G.similarCache[pid + ':' + G.similarRadius];
  const n = cached ? ' (' + cached.results.length + ')' : '';
  return '🔍 Ähnliche Parzellen' + n;
}

/** Background-prefetch the similar count for the popup button label. Only for
 *  fast radii (≤10km) — 20/50km can take many seconds cold, don't waste that. */
function prefetchSimilarCount(pid) {
  const btn = document.getElementById('pp-similar-btn');
  if (btn) btn.textContent = similarBtnLabel(pid);
  if (G.similarRadius > 10000) return;
  const key = pid + ':' + G.similarRadius;
  if (G.similarCache[key] || !G.sel || G.sel.properties.parcel_id !== pid) return;
  fetchSimilar(G.sel, G.similarRadius).then(() => {
    if (G.sel && G.sel.properties.parcel_id === pid) {
      const b = document.getElementById('pp-similar-btn');
      if (b && !b.disabled) b.textContent = similarBtnLabel(pid);
    }
  }).catch(()=>{});
}

window.setSimilarRadius = function setSimilarRadius(r) {
  G.similarRadius = r;
  const span = document.getElementById('pp-similar-radius');
  if (span) for (const b of span.querySelectorAll('.sim-r')) b.classList.toggle('on', b.textContent === String(r/1000));
  if (G.sel) prefetchSimilarCount(G.sel.properties.parcel_id);
  // If an overlay for this parcel is showing, re-run with the new radius
  if (G.similar && G.sel && G.similar.refPid === G.sel.properties.parcel_id) findSimilarParcels();
};

window.findSimilarParcels = async function findSimilarParcels() {
  if (!G.sel) return;
  const f = G.sel;
  const { pid, pLon, pLat } = similarQueryFor(f);
  const radius = G.similarRadius;
  const km = radius / 1000 + ' km';
  const btn = document.getElementById('pp-similar-btn');
  if (btn) { btn.disabled = true; btn.textContent = radius > 10000 ? '⏳ Suche… (' + km + ', dauert etwas)' : '⏳ Suche ähnliche Parzellen…'; }
  G.similar = null; render();
  try {
    const d = await fetchSimilar(f, radius);
    if (!d || d.error || !d.results) throw new Error(d && d.error || 'no results');
    if (d.results.length === 0) {
      toast('🔍 Keine ähnlichen Parzellen im Umkreis von ' + km + ' gefunden', 'err');
      return;
    }
    G.similar = { refPid: pid, refLon: pLon, refLat: pLat, data: d };
    const chip = document.getElementById('btn-similar-clear');
    if (chip) chip.style.display = '';
    // Zoom out to fit all results + reference
    let minLon = pLon, maxLon = pLon, minLat = pLat, maxLat = pLat;
    for (const r of d.results) {
      minLon = Math.min(minLon, r.lon); maxLon = Math.max(maxLon, r.lon);
      minLat = Math.min(minLat, r.lat); maxLat = Math.max(maxLat, r.lat);
    }
    fitBBox(minLon, minLat, maxLon, maxLat, 16);
    let msg = '🔍 ' + d.results.length + ' ähnliche Parzellen im Umkreis von ' + km + ' (von ' + (d.candidates || '?') + ' Kandidaten)';
    if (d.lidar_terms) msg += ' · mit LiDAR-Geländeabgleich ✨';
    toast(msg, 'ok');
  } catch(e) {
    toast('🔍 Ähnlichkeitssuche fehlgeschlagen', 'err');
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = similarBtnLabel(pid); }
  }
};

window.clearSimilar = function clearSimilar() {
  G.similar = null;
  const chip = document.getElementById('btn-similar-clear');
  if (chip) chip.style.display = 'none';
  const simBox = document.getElementById('pp-sim');
  if (simBox) simBox.style.display = 'none';
  render();
};

/** Similarity score breakdown in the parcel popup — shown when the selected
 *  parcel is one of the active similar-search results (or the reference).
 *  This is the mobile-friendly "score detail": tapping a teal diamond opens
 *  the parcel popup, which now explains WHY it matched. */
function renderSimilarPopupRows(pid) {
  const box = document.getElementById('pp-sim');
  if (!box) return;
  box.style.display = 'none';
  box.innerHTML = '';
  if (!G.similar) return;
  if (pid === G.similar.refPid) {
    box.innerHTML = '<span>🔍 Vergleich</span><b style="color:var(--gold)">Referenzparzelle</b>';
    box.style.display = '';
    return;
  }
  const r = G.similar.data.results.find(x => x.parcel_id === pid);
  if (!r) return;
  const bar = (v) => {
    const pct = Math.round(Math.max(0, Math.min(1, v)) * 100);
    return '<span class="sim-bar"><i class="' + (v > 0.75 ? 'hi' : '') + '" style="width:' + pct + '%"></i></span>' + pct + '%';
  };
  const labels = { size:'📏 Größe', landuse:'🌾 Nutzung', building:'🏗️ Bebauung', terrain:'⛰️ Gelände', composition:'🌿 Bewuchs' };
  let html = '<span>🔍 Ähnlichkeit</span><b class="sim-score">' + Math.round(r.score * 100) + '% · ' + fmtDist(r.distance_m) + ' entfernt</b>';
  const order = ['size','landuse','building','terrain','composition'];
  for (const k of order) {
    if (r.parts && r.parts[k] != null) html += '<span>' + labels[k] + '</span><b>' + bar(r.parts[k]) + '</b>';
  }
  html += '<span></span><b style="font:14px VT323;color:var(--text-dim)"><a href="#" onclick="flyToSimilarRef();return false" style="color:var(--gold)">→ zur Referenzparzelle</a></b>';
  box.innerHTML = html;
  box.style.display = '';
}

window.flyToSimilarRef = function flyToSimilarRef() {
  if (!G.similar) return;
  flyTo(G.similar.refLon, G.similar.refLat, Math.max(G.cam.zoom, 17));
  const pid = G.similar.refPid;
  setTimeout(() => {
    const f = G.parcelPolys.find(pf => pf.properties.parcel_id === pid) ||
              G.parcels.find(pf => pf.properties.parcel_id === pid);
    if (f) showParcelPopup(f);
  }, 700);
};

/** Pulsing pixel-art diamond markers for similar-parcel results + gold reference marker. */
function drawSimilarParcels(ctx) {
  if (!G.similar) return;
  const d = G.similar.data;
  const pulse = 0.75 + Math.sin(Date.now() / 350) * 0.25;
  const showLabel = G.cam.zoom >= 16;

  // Off-screen results → edge arrows (dedup per border cell so 40 results at
  // 50 km don't stack). Zoom min is 13 (~8 km viewport), so large radii rely on these.
  const edgeCells = {};
  for (const r of d.results) {
    const [x, y] = toScreen(r.lon, r.lat);
    if (x < -30 || x > gc.width + 30 || y < -30 || y > gc.height + 30) {
      // clamp position to screen border (with margin)
      const m = 22;
      const cx = gc.width / 2, cy = gc.height / 2;
      let dx = x - cx, dy = y - cy;
      const k = Math.min((cx - m) / Math.abs(dx || 1e-9), (cy - m) / Math.abs(dy || 1e-9));
      const ex = cx + dx * k, ey = cy + dy * k;
      const cell = Math.round(ex / 60) + ':' + Math.round(ey / 60);
      if (edgeCells[cell]) { edgeCells[cell].n++; if (r.distance_m < edgeCells[cell].dist) edgeCells[cell].dist = r.distance_m; continue; }
      edgeCells[cell] = { ex, ey, ang: Math.atan2(dy, dx), n: 1, dist: r.distance_m, score: r.score };
      continue;
    }
    const sc = Math.max(0, Math.min(1, (r.score - 0.5) / 0.5)); // 0.5..1 → 0..1
    const sz = (7 + sc * 6) * pulse;
    ctx.globalAlpha = 0.55 + sc * 0.45;
    // chunky diamond — dark outline + teal fill + bright core
    ctx.beginPath();
    ctx.moveTo(x, y - sz); ctx.lineTo(x + sz, y); ctx.lineTo(x, y + sz); ctx.lineTo(x - sz, y); ctx.closePath();
    ctx.fillStyle = '#0d5c63';
    ctx.fill();
    ctx.lineWidth = 2;
    ctx.strokeStyle = '#083b40';
    ctx.stroke();
    const isz = sz * 0.55;
    ctx.beginPath();
    ctx.moveTo(x, y - isz); ctx.lineTo(x + isz, y); ctx.lineTo(x, y + isz); ctx.lineTo(x - isz, y); ctx.closePath();
    ctx.fillStyle = sc > 0.6 ? '#4de8dc' : '#2ab5ac';
    ctx.fill();
    if (showLabel) {
      ctx.font = MAP_FONT.pixel;
      ctx.textAlign = 'center';
      const lbl = Math.round(r.score * 100) + '%';
      ctx.fillStyle = '#062d30';
      ctx.fillText(lbl, x + 1, y - sz - 5);
      ctx.fillStyle = '#7ff5eb';
      ctx.fillText(lbl, x, y - sz - 6);
    }
    ctx.globalAlpha = 1;
  }

  // Edge arrows for off-screen results (tap = fly toward them)
  G._simEdgeArrows = [];
  for (const cell in edgeCells) {
    const a = edgeCells[cell];
    ctx.save();
    ctx.translate(a.ex, a.ey);
    ctx.rotate(a.ang);
    ctx.globalAlpha = 0.9;
    // chunky triangle arrow
    ctx.beginPath();
    ctx.moveTo(10 * pulse, 0); ctx.lineTo(-6, -8); ctx.lineTo(-6, 8); ctx.closePath();
    ctx.fillStyle = '#4de8dc';
    ctx.fill();
    ctx.lineWidth = 2;
    ctx.strokeStyle = '#083b40';
    ctx.stroke();
    ctx.restore();
    // count + distance label, offset toward screen center
    const lx = a.ex - Math.cos(a.ang) * 26, ly = a.ey - Math.sin(a.ang) * 26;
    ctx.font = MAP_FONT.pixel;
    ctx.textAlign = 'center';
    const lbl = (a.n > 1 ? a.n + '× ' : '') + (a.dist >= 1000 ? Math.round(a.dist/1000) + 'km' : Math.round(a.dist) + 'm');
    ctx.fillStyle = '#062d30';
    ctx.fillText(lbl, lx + 1, ly + 4);
    ctx.fillStyle = '#7ff5eb';
    ctx.fillText(lbl, lx, ly + 3);
    ctx.globalAlpha = 1;
    G._simEdgeArrows.push(a);
  }

  // Reference parcel: gold marker
  const [rx, ry] = toScreen(G.similar.refLon, G.similar.refLat);
  if (rx > -30 && rx < gc.width + 30 && ry > -30 && ry < gc.height + 30) {
    const sz = 11 * pulse;
    ctx.beginPath();
    ctx.moveTo(rx, ry - sz); ctx.lineTo(rx + sz, ry); ctx.lineTo(rx, ry + sz); ctx.lineTo(rx - sz, ry); ctx.closePath();
    ctx.fillStyle = '#8a6a1a';
    ctx.fill();
    ctx.lineWidth = 2;
    ctx.strokeStyle = '#4d3a0c';
    ctx.stroke();
    const isz = sz * 0.55;
    ctx.beginPath();
    ctx.moveTo(rx, ry - isz); ctx.lineTo(rx + isz, ry); ctx.lineTo(rx, ry + isz); ctx.lineTo(rx - isz, ry); ctx.closePath();
    ctx.fillStyle = '#ffd34d';
    ctx.fill();
    if (showLabel) {
      ctx.font = MAP_FONT.pixel;
      ctx.textAlign = 'center';
      ctx.fillStyle = '#3a2c08';
      ctx.fillText('REF', rx + 1, ry - sz - 5);
      ctx.fillStyle = '#ffe9a8';
      ctx.fillText('REF', rx, ry - sz - 6);
    }
  }
}

/** Tap on a similar-parcel marker: fly there and open its parcel popup. */
function hitSimilarMarker(x, y) {
  if (!G.similar) return null;
  let best = null, bestD = Infinity;
  for (const r of G.similar.data.results) {
    const [sx, sy] = toScreen(r.lon, r.lat);
    const dd = Math.abs(sx - x) + Math.abs(sy - y);
    if (dd < 22 && dd < bestD) { bestD = dd; best = r; }
  }
  return best;
}

function openSimilarResult(r) {
  flyTo(r.lon, r.lat, Math.max(G.cam.zoom, 17));
  const tryOpen = (attempt) => {
    const f = G.parcelPolys.find(pf => pf.properties.parcel_id === r.parcel_id) ||
              G.parcels.find(pf => pf.properties.parcel_id === r.parcel_id);
    if (f) { showParcelPopup(f); return; }
    if (attempt < 8) setTimeout(() => tryOpen(attempt + 1), 700); // viewport loading will fetch it
  };
  setTimeout(() => tryOpen(0), 900);
}

window.openEZPopup = function openEZPopup(kgCode, ez) {
  const ezKey = kgCode + '-EZ' + ez;
  const ezParcels = G.ezIndex[ezKey] || [];
  if (ezParcels.length < 1) return;

  const claimMap = {};
  for (const c of G.claimed) claimMap[c.parcel_id] = c;
  const totalArea = ezParcels.reduce((s, pf) => s + (pf.properties.area_sqm || 0), 0);
  const unclaimed = ezParcels.filter(pf => !claimMap[pf.properties.parcel_id]);
  const myCount = ezParcels.filter(pf => claimMap[pf.properties.parcel_id]?.player_id === G.player.id).length;
  const areaStr = totalArea > 10000 ? (totalArea/10000).toFixed(2)+' ha' : Math.round(totalArea)+' m\u00b2';

  document.getElementById('ez-title').textContent = '\ud83d\udccb EZ ' + ez + ' \u2014 ' + kgCode;
  document.getElementById('ez-stats').innerHTML = `
    <span>Parzellen</span><b>${ezParcels.length} (${unclaimed.length} frei)</b>
    <span>Gesamtfl\u00e4che</span><b>${areaStr}</b>
    <span>Dein Besitz</span><b>${myCount} / ${ezParcels.length}</b>`;

  const ezAct = document.getElementById('ez-actions');
  ezAct.innerHTML = '';
  if (unclaimed.length > 0) {
    let totalPrice = 0;
    for (const pf of unclaimed) {
      const pp = pf.properties;
      totalPrice += calcPrice(pp.area_sqm||0, extractLuCode('',pp), pp.building_count||0, pp.total_building_area_sqm||0);
    }
    const discountedPrice = Math.round(totalPrice * 0.8);
    const savings = totalPrice - discountedPrice;
    ezAct.innerHTML = `<button class="btn btn-gold btn-small" style="width:100%" onclick="doClaimEZ('${kgCode}','${ez}')">📋 Ganze EZ kaufen: ${discountedPrice}🪙 <span style='font-size:14px;color:#2a2'>(-20% = -${savings}🪙)</span></button>`;
  }

  G.ezHighlight = {kg: kgCode, ez: ez};

  // Calculate bounds of all EZ parcels and zoom to fit
  let minLon = Infinity, maxLon = -Infinity, minLat = Infinity, maxLat = -Infinity;
  for (const pf of ezParcels) {
    const p = pf.properties;
    if (isAreaGeom(pf.geometry)) {
      const b = geoBounds(pf.geometry);
      minLon = Math.min(minLon, b.w); maxLon = Math.max(maxLon, b.e);
      minLat = Math.min(minLat, b.s); maxLat = Math.max(maxLat, b.n);
    } else {
      // Point geometry
      const lon = p.lon || pf.geometry.coordinates[0];
      const lat = p.lat || pf.geometry.coordinates[1];
      minLon = Math.min(minLon, lon);
      maxLon = Math.max(maxLon, lon);
      minLat = Math.min(minLat, lat);
      maxLat = Math.max(maxLat, lat);
    }
  }
  // Calculate center and zoom to fit all parcels
  const centerLon = (minLon + maxLon) / 2;
  const centerLat = (minLat + maxLat) / 2;
  const lonRange = maxLon - minLon;
  const latRange = maxLat - minLat;
  // Canvas dimensions for zoom calculation (accounting for popups on mobile)
  const canvasWidth = gc.width;
  const canvasHeight = gc.height;
  // Calculate zoom level to fit the bounds with some padding
  const lonZoom = Math.log2(360 / (lonRange * 1.5) * (canvasWidth / 800));
  const latZoom = Math.log2(180 / (latRange * 1.5) * (canvasHeight / 600));
  let targetZoom = Math.max(15, Math.min(19, Math.min(lonZoom, latZoom)));
  // Stability: zooming OUT to fit the EZ is fine, but never zoom IN by more than
  // ~1 level from where the user is — keeps the map calm on phones.
  targetZoom = Math.min(targetZoom, G.cam.zoom + 1);
  // Animate smoothly to show the entire EZ
  animateCamera(centerLon, centerLat, targetZoom, 650);

  const popup = document.getElementById('ez-popup');
  popup.classList.add('open');
  // Position to the right of parcel popup if not manually moved
  if (!popup.dataset.userMoved) {
    // Reset to CSS defaults; on desktop only, position to the right of parcel popup
    popup.style.left = ''; popup.style.top = '';
    popup.style.right = ''; popup.style.bottom = '';
    if (window.innerWidth >= 768) {
      const ppEl = document.getElementById('parcel-popup');
      const ppRect = ppEl.getBoundingClientRect();
      popup.style.left = (ppRect.right + 12) + 'px';
      popup.style.bottom = '16px';
    }
  }
  render();
}

function calcPrice(area, lu, buildingCount, totalBuildingArea) {
  // Base price/m² from the BEV NS table — keep in sync with nsPricePerSqm() in server.go.
  const ppm = (NS_TABLE[lu] && NS_TABLE[lu].price) || 0.15;
  // Density multiplier: built-up ratio drives price up/down
  let densityMult = 1.0;
  if (area > 0 && totalBuildingArea > 0) {
    const builtRatio = totalBuildingArea / area; // 0..1+
    // urban dense (ratio>0.3) → 2x, suburban (0.05-0.3) → 1-2x, rural (<0.01) → 0.5x
    if (builtRatio > 0.3) densityMult = 2.0;
    else if (builtRatio > 0.05) densityMult = 1.0 + (builtRatio - 0.05) / 0.25;
    else densityMult = 0.5 + builtRatio / 0.05 * 0.5;
  } else if (buildingCount === 0) {
    densityMult = 0.5; // no buildings at all → cheap rural land
  }
  return Math.max(10, Math.min(5000, Math.round(area * ppm * densityMult)));
}

window.doClaim = async function() {
  if (!G.sel) return;
  const p = G.sel.properties;
  const tt = G.tallRevealed ? tallTreesInParcel(G.sel) : {count:0, maxH:0};
  const res = await POST('/api/claim-parcel', {
    session_id:G.session.id, player_id:G.player.id,
    parcel_id:p.parcel_id, kg_code:p.kg_code||'', gnr:p.gnr||'',
    ez:p.ez||'',
    area_sqm:p.area_sqm||0, landuse:extractLuCode('',p),
    building_count:p.building_count||0, total_building_area:p.total_building_area_sqm||0,
    tall_tree_count:tt.count, tall_tree_max_h:tt.maxH,
  });
  if (res.error) { toast(res.error,'err'); return; }
  if (res.tall_bonus_xp > 0) {
    toast('🏴 Gekauft für '+res.price+'🪙! 🌲 Riesenbaum-Bonus: +'+res.tall_bonus_xp+'⚡','ok');
  } else {
    const fl = parcelFlur(G.sel);
    toast('🏴 Gekauft für '+res.price+'🪙!' + (fl && fl.t.layer === 'ried' ? ' 🌾 ' + tr('Ried') + ' „' + fl.label + '“' : fl && fl.inside ? ' · ' + fl.label : ''),'ok');
  }
  G.player = res.player; updateStats();
  await loadClaimed(); render(); showParcelPopup(G.sel); loadChallenges();
  Herald.hint('first_claim');
  if (isCropField(p) && fieldKindFor(p, simpleHash(p.parcel_id)) !== 3) Herald.hint('first_field');
};

window.doHarvest = async function() {
  if (!G.sel) return;
  const p = G.sel.properties;
  const sf = parcelSchlag(p);
  const res = await POST('/api/harvest-parcel', {session_id:G.session.id, player_id:G.player.id, parcel_id:p.parcel_id, crop_group: sf ? sf.properties.crop_group : ''});
  if (res.error) { toast(res.error,'err'); return; }
  const [lon, lat] = featureLonLat(G.sel);
  spawnCollectFX({lon, lat}, '+' + res.coins + ' 🪙', TREASURE_RARITY.coins);
  toast('🌾 ' + tr('Geerntet') + ': +' + res.coins + '🪙 +' + res.xp + '⚡','ok');
  G.player = res.player; updateStats();
  await loadClaimed(); render(); showParcelPopup(G.sel); loadChallenges();
};

window.doConvert = async function(to) {
  if (!G.sel) return;
  const res = await POST('/api/convert-parcel', {
    session_id:G.session.id, player_id:G.player.id,
    parcel_id:G.sel.properties.parcel_id, convert_to:to,
  });
  if (res.error) { toast(res.error,'err'); return; }
  if (to === 'wildforest') { FOREST.scenes.delete(G.sel.properties.parcel_id + ':wild'); toast('🌳 ' + tr('Naturwald') + '! ~' + Math.round(res.co2_t || 0) + ' t CO₂ ' + tr('bleiben im Wald') + ' · +' + res.xp_reward + '⚡', 'ok'); }
  else toast('🌿 Umgewandelt! +'+res.xp_reward+'⚡','ok');
  G.player = res.player; updateStats();
  await loadClaimed(); await loadBio(); render(); showParcelPopup(G.sel); loadChallenges();
};

window.doSell = async function(claimId) {
  const res = await POST('/api/sell-parcel', {session_id:G.session.id, player_id:G.player.id, claim_id:claimId});
  if (res.error) { toast(res.error,'err'); return; }
  toast('💰 Verkauft für '+res.sell_price+'🪙','ok');
  G.player = res.player; updateStats();
  await loadClaimed(); render();
  document.getElementById('parcel-popup').classList.remove('open');
  document.getElementById('ez-popup').classList.remove('open');
  resetPopupPosition('parcel-popup'); resetPopupPosition('ez-popup');
  G.sel=null; G.ezHighlight=null;
};

window.doClaimEZ = async function(kgCode, ez) {
  const ezKey = kgCode + '-EZ' + ez;
  const ezParcels = G.ezIndex[ezKey] || [];
  const claimMap = {};
  for (const c of G.claimed) claimMap[c.parcel_id] = c;
  const unclaimed = ezParcels.filter(pf => !claimMap[pf.properties.parcel_id]);
  if (unclaimed.length === 0) { toast('Alle Parzellen dieser EZ sind bereits vergeben','err'); return; }

  const parcels = unclaimed.map(pf => {
    const pp = pf.properties;
    return {
      parcel_id: pp.parcel_id,
      gnr: pp.gnr || '',
      area_sqm: pp.area_sqm || 0,
      landuse: extractLuCode('', pp),
      building_count: pp.building_count || 0,
      total_building_area: pp.total_building_area_sqm || 0,
    };
  });

  const res = await POST('/api/claim-ez', {
    session_id: G.session.id, player_id: G.player.id,
    kg_code: kgCode, ez: ez, parcels: parcels,
  });
  if (res.error) { toast(res.error, 'err'); return; }
  toast('\u{1f4cb} EZ ' + ez + ': ' + res.claimed_count + ' Parzellen ('+res.discount+' gespart!)', 'ok');
  G.player = res.player; updateStats();
  await loadClaimed(); render();
  if (G.sel) showParcelPopup(G.sel);
  loadChallenges();
};

window.doMakeOffer = async function() {
  if (!G.sel) return;
  const priceInput = document.getElementById('offer-price-input');
  if (!priceInput) return;
  const offerPrice = parseInt(priceInput.value);
  if (!offerPrice || offerPrice < 10) { toast('Mindestangebot: 10 Münzen','err'); return; }
  if (offerPrice > G.player.coins) { toast('Nicht genug Münzen! Du hast '+G.player.coins+'🪙','err'); return; }
  const p = G.sel.properties;
  const res = await POST('/api/offer-parcel', {
    session_id: G.session.id,
    buyer_id: G.player.id,
    parcel_id: p.parcel_id,
    offer_price: offerPrice,
  });
  if (res.error) { toast(res.error, 'err'); return; }
  toast('📨 Angebot gesendet: '+offerPrice+'🪙', 'ok');
  await loadOffers();
  showParcelPopup(G.sel);
};

window.doRespondOffer = async function(offerId, accept) {
  const res = await POST('/api/offer-respond', {
    offer_id: offerId,
    player_id: G.player.id,
    accept: accept,
  });
  if (res.error) { toast(res.error, 'err'); return; }
  if (accept) {
    toast('✅ Angebot angenommen! Parzelle verkauft.', 'ok');
    if (res.seller) G.player = res.seller;
  } else {
    toast('❌ Angebot abgelehnt.', 'ok');
  }
  updateStats();
  await Promise.all([loadClaimed(), loadOffers()]);
  render();
  if (G.sel) showParcelPopup(G.sel);
};

async function claimTreasure(t) {
  const res = await POST('/api/claim-treasure', {player_id:G.player.id, treasure_id:t.id});
  if (res.error) { toast(res.error,'err'); return; }
  if ((res.type === 'species' || res.type === 'n2k_species') && res.species_german) {
    const catLabels = {'EN':'Stark gefährdet','VU':'Gefährdet','NT':'Potenziell gefährdet','LC':'Nicht gefährdet'};
    const catEmoji = {'EN':'🔴','VU':'🟠','NT':'🔵','LC':'🟢'};
    const n2k = res.type === 'n2k_species' ? '🛡️ Natura-2000-Bonus! ' : '';
    toast(`🦎 ${n2k}Artenfund: ${res.species_german} (${res.species_name})\n${catEmoji[res.species_category]||''} ${catLabels[res.species_category]||res.species_category} — +${res.value}🪙`, 'ok');
  } else {
    const emoji = {xp:'⚡',rare_seed:'🌱',ancient_map:'🗺️',coins:'🪙'}[res.type]||'🪙';
    toast('💎 Schatz! +'+res.value+emoji,'ok');
  }
  G.player = res.player; updateStats();
  spawnCollectFX(t, '+' + res.value + (res.type === 'xp' ? ' XP' : ' 🪙'), treasureRarity(t));
  G.treasures = G.treasures.filter(tr=>tr.id!==t.id);
  if (!G.tallUnlocked && enhancedLoaded()) setTimeout(() => Herald.hint('trees_unlocked'), 1200);
  // First treasure unlocks the giant trees (enhanced mode)
  if (!G.tallUnlocked) {
    G.tallUnlocked = true;
    if (allTallTrees().length > 0) {
      setTimeout(() => toast('🌲 Gerücht: Irgendwo da steht ein Riesenbaum... Find ihn und tipp ihn an!', 'ok'), 1200);
    }
  }
  render(); loadChallenges();
}

document.getElementById('popup-close').onclick = () => {
  document.getElementById('parcel-popup').classList.remove('open');
  resetPopupPosition('parcel-popup');
  G.sel=null; G.selFp=null; render();
};

document.getElementById('kg-popup-close').onclick = () => {
  document.getElementById('kg-popup').classList.remove('open');
  resetPopupPosition('kg-popup');
};

document.getElementById('tree-popup-close').onclick = () => {
  document.getElementById('tree-popup').classList.remove('open');
  resetPopupPosition('tree-popup');
};

document.getElementById('ez-popup-close').onclick = () => {
  document.getElementById('ez-popup').classList.remove('open');
  resetPopupPosition('ez-popup');
  G.ezHighlight=null; render();
};

// ---- Draggable popups ----
(function initDraggablePopups() {
  const handles = document.querySelectorAll('.popup-drag-handle');
  handles.forEach(handle => {
    let startX, startY, startLeft, startTop;
    function onMouseDown(e) {
      const targetId = handle.dataset.dragTarget;
      const popup = document.getElementById(targetId);
      if (!popup) return;
      e.preventDefault();
      const rect = popup.getBoundingClientRect();
      startX = e.clientX; startY = e.clientY;
      startLeft = rect.left; startTop = rect.top;
      // Switch from bottom positioning to top positioning for dragging
      popup.style.left = rect.left + 'px';
      popup.style.top = rect.top + 'px';
      popup.style.bottom = 'auto';
      popup.style.right = 'auto';
      popup.dataset.userMoved = '1';

      function onMouseMove(e) {
        const dx = e.clientX - startX, dy = e.clientY - startY;
        popup.style.left = Math.max(0, startLeft + dx) + 'px';
        popup.style.top = Math.max(0, startTop + dy) + 'px';
      }
      function onMouseUp() {
        document.removeEventListener('mousemove', onMouseMove);
        document.removeEventListener('mouseup', onMouseUp);
      }
      document.addEventListener('mousemove', onMouseMove);
      document.addEventListener('mouseup', onMouseUp);
    }
    handle.addEventListener('mousedown', onMouseDown);

    // Touch support
    handle.addEventListener('touchstart', e => {
      const targetId = handle.dataset.dragTarget;
      const popup = document.getElementById(targetId);
      if (!popup || !e.touches[0]) return;
      e.preventDefault();
      const touch = e.touches[0];
      const rect = popup.getBoundingClientRect();
      startX = touch.clientX; startY = touch.clientY;
      startLeft = rect.left; startTop = rect.top;
      popup.style.left = rect.left + 'px';
      popup.style.top = rect.top + 'px';
      popup.style.bottom = 'auto';
      popup.style.right = 'auto';
      popup.dataset.userMoved = '1';

      function onTouchMove(e) {
        if (!e.touches[0]) return;
        const dx = e.touches[0].clientX - startX, dy = e.touches[0].clientY - startY;
        popup.style.left = Math.max(0, startLeft + dx) + 'px';
        popup.style.top = Math.max(0, startTop + dy) + 'px';
      }
      function onTouchEnd() {
        document.removeEventListener('touchmove', onTouchMove);
        document.removeEventListener('touchend', onTouchEnd);
      }
      document.addEventListener('touchmove', onTouchMove, {passive:false});
      document.addEventListener('touchend', onTouchEnd);
    }, {passive:false});
  });
})();

// Reset popup positions when closing
function resetPopupPosition(id) {
  const el = document.getElementById(id);
  if (el) {
    delete el.dataset.userMoved;
    el.style.left = '';
    el.style.top = '';
    el.style.bottom = '';
    el.style.right = '';
  }
}

// Smooth treasure bob/sparkle + collect FX: ~25fps only while a treasure is
// actually on screen (or FX are playing); respects prefers-reduced-motion.
(function treasureAnimLoop() {
  requestAnimationFrame(treasureAnimLoop);
  if (!document.getElementById('screen-game').classList.contains('active')) return;
  const natureLive = NATURE.onScreen > 0 && natureAnimLevel() > 0;
  if (!(_treasuresOnScreen > 0 || _ripeOnScreen > 0 || G.fx.length || natureLive)) return;
  const now = performance.now();
  const step = natureAnimLevel() === 1 ? 66 : isCoarsePointer() ? 50 : 40;
  if (now - (treasureAnimLoop._last || 0) < step) return;
  if (giantAnimBudget() === 1 && !G.fx.length) return; // reduced motion → static (800ms tick below)
  treasureAnimLoop._last = now;
  render();
})();
// Field-cycle countdown in the open parcel popup + button appearing when ripe
setInterval(() => {
  if (!G.sel || !document.getElementById('parcel-popup').classList.contains('open')) return;
  const p = G.sel.properties; if (!isCropField(p)) return;
  const claim = G.claimed.find(c => c.parcel_id === p.parcel_id);
  const fs = fieldStage(p, claim);
  const el = document.getElementById('pp-field'); if (el) el.textContent = fieldStageLabel(fs);
  const hasBtn = !!document.querySelector('#pp-actions [onclick="doHarvest()"]');
  if (fs.mine && !claim.converted_to && (fs.stage === 'ripe') !== hasBtn) showParcelPopup(G.sel, G.selFp);
}, 5000);
// Sparkle animation for treasures, top-tree sway + GPS pulse
setInterval(() => {
  if (document.getElementById('screen-game').classList.contains('active') &&
      (G.treasures.length>0 || G.geo.watching || Object.keys(G.topTrees).length>0)) render();
}, 800);
// Faster pulse when GPS marker is active (still cheap: only when watching)
// Also drives the giant-tree hint pulse and the reveal pop-in animation.
setInterval(() => {
  if (!document.getElementById('screen-game').classList.contains('active')) return;
  // Animate when a giant tree is on screen, or while the miracle fog hint
  // is gathering/visible (no tree on screen → fog timer runs in render).
  // Only when something tree-related is actually animating: giants drawn in the
  // last frame, the fog scout gathering, or a fresh reveal pop-in.
  const treeAnim = G.tallUnlocked && (_drawnTrees.length > 0 || (fogHintSince > 0 && allTallTrees().length > 0) ||
    Date.now() - G.tallRevealAt < 3000);
  if (G.geo.watching || treeAnim) render();
}, 100);

// Auto-refresh
setInterval(async () => {
  if (!G.session||!G.player) return;
  try { const p = await GET('/api/player/'+G.player.id); if(!p.error){G.player=p;updateStats(); if(p.treasures_found>0) G.tallUnlocked=true;} } catch(e){}
}, 15000);

// ---- Init picker when shown ----
const pickObs = new MutationObserver(() => {
  if (document.getElementById('screen-pick').classList.contains('active') && !pickCanvas) initPicker();
});
pickObs.observe(document.getElementById('screen-pick'), {attributes:true, attributeFilter:['class']});

// ================= DEV / SCREENSHOT HELPERS =================
// Scripting API for automated screenshots & QA (no UI). Usage from devtools /
// browser automation, e.g.:
//   await DEV.goto(15.5205, 48.3955, 17.5)          // move camera, wait for tiles
//   await DEV.parcel('12105-68/3')                   // select parcel + open popup
//   DEV.ez('12105', 430); DEV.kg('12105'); DEV.tree(0)
//   DEV.chrome(false)                                // hide search/badges/attrib
//   DEV.state()                                      // compact JSON of G
//   DEV.warp(45); DEV.freeze()                       // shift/freeze the game clock (field & forest cycles)
//   DEV.mock('12105-68/3', {to:'biodiversity'})      // local-only claim for overlay shots
//   DEV.fields('ripe'); await DEV.harvest(id)         // crop cycle QA
//   DEV.forests(); await DEV.forest(id)               // forest stands + timber estimate
//   DEV.nature(); DEV.nature({level:2, quality:1})    // living-overlay perf/state
// Also honoured on load: URL ?dev=1 skips the min. loading-screen dwell time,
// and #v=lon,lat,zoom (existing) sets the initial camera.
window.DEV = {
  /** Toponyms: DEV.topo() → counts; DEV.topo('Wunderburg') → fly to best local match. */
  async topo(q) {
    if (!q) {
      const by = {}; for (const t of G.toponyms) by[t.layer] = (by[t.layer] || 0) + 1;
      return { loaded: G.toponyms.length, shown: (G.topoShown || []).length, visible: G.topoVisible, by };
    }
    const r = await searchToponyms(q, 1);
    if (!r.length) return null;
    flyTo(r[0].lon, r[0].lat, topoZoom(r[0]._topo)); return r[0]._topo;
  },
  /** Wait until no viewport tiles are in flight (or timeout). */
  idle(timeout = 15000) {
    return new Promise(res => {
      const t0 = Date.now();
      (function chk() {
        if (_vpBusy === 0 && Date.now() - t0 > 250) return res(true);
        if (Date.now() - t0 > timeout) return res(false);
        setTimeout(chk, 100);
      })();
    });
  },
  /** Move camera instantly, trigger loading, wait for idle, render. */
  async goto(lon, lat, zoom, settleMs = 400) {
    if (flyAnim) { cancelAnimationFrame(flyAnim); flyAnim = null; }
    if (lon != null) G.cam.lon = lon;
    if (lat != null) G.cam.lat = lat;
    if (zoom != null) G.cam.zoom = Math.max(13, Math.min(20, zoom));
    render(); renderMini();
    await loadMoreParcels();
    await this.idle();
    await new Promise(r => setTimeout(r, settleMs));
    render(); renderMini();
    return this.state();
  },
  /** Close every popup and clear selection. */
  closeAll() {
    for (const id of ['parcel-popup','ez-popup','kg-popup','tree-popup']) {
      const el = document.getElementById(id); if (!el) continue;
      el.classList.remove('open'); try { resetPopupPosition(id); } catch(e) {}
    }
    document.getElementById('game-search-results').innerHTML = '';
    G.sel = null; G.selFp = null; G.ezHighlight = null; render();
  },
  /** Find a loaded polygon feature by parcel_id. */
  find(parcelId) { return G.parcelPolys.find(f => f.properties.parcel_id === parcelId) || null; },
  /** Select a parcel by id (or feature) and open its popup; optionally center on it. */
  async parcel(idOrFeature, center = false, zoom) {
    const f = typeof idOrFeature === 'string' ? this.find(idOrFeature) : idOrFeature;
    if (!f) return null;
    if (center) { const [lon, lat] = featureLonLat(f); await this.goto(lon, lat, zoom || G.cam.zoom); }
    showParcelPopup(f);
    await new Promise(r => setTimeout(r, 1200)); // lazy rows (lidar/osm/price)
    render();
    return f.properties;
  },
  /** Open the EZ (folio) popup + gold highlight. */
  ez(kg, ez) { openEZPopup(String(kg), Number(ez)); render(); return G.ezIndex[kg + '-EZ' + ez]?.length || 0; },
  /** Open the KG statistics popup. */
  kg(kg) { return openKGSummary(String(kg)); },
  /** Open the giant-tree popup for the n-th tallest loaded tree (unlocks/reveals). */
  tree(n = 0) {
    G.tallUnlocked = true; G.tallRevealed = true;
    const t = allTallTrees()[n];
    if (t) { showTreePopup(t); render(); }
    return t || null;
  },
  /** Unlock/reveal state for giant trees: 'locked' | 'hint' | 'revealed'. */
  /** 'locked' | 'hint' | 'revealed' (discover what's in view) | 'all' (discover every loaded giant) | 'reset' (forget Chronik) */
  trees(mode) {
    if (mode === 'reset') { G.tallSeen = new Set(); saveTallSeen(); for (const t of allTallTrees()) t._seenAt = 0; G.tallRevealed = false; render(); return giantChronik(); }
    G.tallUnlocked = mode !== 'locked'; G.tallRevealed = mode === 'revealed' || mode === 'all';
    if (mode === 'revealed') discoverTrees(tallTreesInView().slice(0, giantDrawBudget()), true);
    if (mode === 'all') discoverTrees(allTallTrees());
    render(); return giantChronik();
  },
  /** Candidate EZs on screen with n..m parcels (for finding a nice farm folio). */
  ezCandidates(min = 4, max = 20) {
    const out = [];
    for (const [k, v] of Object.entries(G.ezIndex)) {
      if (v.length < min || v.length > max) continue;
      const area = v.reduce((s, f) => s + (f.properties.area_sqm || 0), 0);
      const b = v.filter(f => (f.properties.building_count || 0) > 0).length;
      out.push({ key: k, n: v.length, area_ha: +(area / 1e4).toFixed(2), withBuildings: b });
    }
    return out.sort((a, b) => b.n - a.n);
  },
  /** Loaded parcels near the camera matching a predicate on properties. */
  parcelsNear(pred, limit = 20) {
    const [cx, cy] = [gc.width / 2, gc.height / 2];
    return G.parcelPolys.map(f => { const [x, y] = toScreen(...featureLonLat(f)); return { f, d: Math.hypot(x - cx, y - cy) }; })
      .filter(o => o.d < Math.max(gc.width, gc.height) && (!pred || pred(o.f.properties)))
      .sort((a, b) => a.d - b.d).slice(0, limit)
      .map(o => Object.assign({ d: Math.round(o.d) }, o.f.properties));
  },
  /** Quest briefing: DEV.quest() lists quests; DEV.quest(id|title) opens the herald briefing; DEV.quest(id, true) also runs its action. */
  async quest(idOrTitle, act = false) {
    if (idOrTitle === undefined) return (G.challenges||[]).map(c => ({ id: c.id, title: c.title, type: c.challenge_type, progress: c.progress, goal: c.goal, visible: visibleQuests().includes(c) }));
    const c = (G.challenges||[]).find(x => x.id === idOrTitle || x.title === idOrTitle); if (!c) return null;
    Herald.brief(c.id);
    await new Promise(r => setTimeout(r, 200));
    const b = questBriefing(c);
    if (act && Herald.action) { Herald.runAction(); await new Promise(r => setTimeout(r, 1200)); await this.idle(); render(); }
    return { id: c.id, lines: b.lines.map(l => l.html.replace(/<[^>]+>/g, '')), action: b.act ? b.act.label : null };
  },
  /** Herald (typewriter hint box): DEV.herald('intro'|'quest'|'off') or DEV.herald('hint','first_claim'). */
  herald(mode, key) {
    if (mode === 'off') return Herald.dismiss();
    if (mode === 'hint') { Herald.seen.delete(key); return Herald.hint(key); }
    Herald.reset(); Herald.start(mode || 'intro');
  },
  /** Show/hide non-map chrome (search, badges, attribution, loading hint). */
  chrome(on) {
    for (const id of ['game-search','hud-badges','abroad-badge','map-attrib','map-loading','muni-toast','herald','flow-chip'])
      { const el = document.getElementById(id); if (el) el.style.visibility = on ? '' : 'hidden'; }
  },
  /** Sidebar (desktop) show/hide — more map for hero shots. */
  sidebar(on) {
    const el = document.getElementById('sidebar');
    if (el) el.style.display = on ? '' : 'none';
    resizeGame(); render();
  },
  // ---- Game clock (Date.now shim): freeze for deterministic frames, warp to
  // fast-forward the 40-min field cycle / forest regrowth. Display-only — the
  // server keeps real time, so harvest calls may still be refused.
  _realNow: null, _offsetMs: 0, _frozenAt: null,
  _installClock() {
    if (this._realNow) return;
    this._realNow = Date.now; const self = this;
    Date.now = () => (self._frozenAt != null ? self._frozenAt : self._realNow()) + self._offsetMs;
  },
  /** Freeze/unfreeze sprite animations (deterministic frames). */
  freeze(on = true) { this._installClock(); this._frozenAt = on ? this._realNow() : null; render(); return this.clock(); },
  /** Shift the game clock by N minutes (negative allowed). DEV.warp(0) resets. Clears scene caches. */
  warp(min = 0) {
    this._installClock(); this._offsetMs = min * 60000;
    FOREST.scenes.clear(); NATURE.scenes.clear(); render();
    if (G.sel && document.getElementById('parcel-popup').classList.contains('open')) showParcelPopup(G.sel, G.selFp);
    return this.clock();
  },
  clock() { return { offsetMin: this._offsetMs / 60000, frozen: this._frozenAt != null, now: new Date(Date.now()).toISOString() }; },
  _claimOf(pid) { return (G.claimed || []).find(c => c.parcel_id === pid) || null; },
  /** Local-only claim (no server, no coins) so overlays can be shot without buying:
   *  DEV.mock(id, {to:'biodiversity'|'wildforest'|null, harvestedMinAgo:12, owner:false}); DEV.mock(null) reloads real claims. */
  async mock(parcelId, o = {}) {
    if (parcelId === null) { await loadClaimed(); NATURE.scenes.clear(); FOREST.scenes.clear(); render(); return G.claimed.length; }
    const f = this.find(parcelId); if (!f) return null;
    const p = f.properties;
    G.claimed = (G.claimed || []).filter(c => c.parcel_id !== parcelId);
    const c = { id: -1 - G.claimed.length, session_id: G.session?.id, player_id: o.owner === false ? -1 : G.player?.id, parcel_id: parcelId,
      kg_code: p.kg_code, gnr: p.gnr, ez: p.ez, area_sqm: p.area_sqm || 0, landuse: extractLuCode('', p), purchase_price: 0,
      converted_to: o.to || null, claimed_at: new Date(Date.now() - 3600e3).toISOString(),
      harvested_at: o.harvestedMinAgo != null ? new Date(Date.now() - o.harvestedMinAgo * 60000).toISOString() : null,
      harvests: o.harvestedMinAgo != null ? 1 : 0, _mock: true };
    G.claimed.push(c);
    NATURE.scenes.delete(parcelId); FOREST.scenes.delete(parcelId + ':wild'); FOREST.scenes.delete(parcelId + ':schlag');
    updateParcelCount(); render();
    return c;
  },
  /** Crop fields near the camera with their cycle stage. DEV.fields('ripe'|'growing'|'ploughed'|'stubble'|'meadow'|'fallow'). */
  fields(stage, limit = 20) {
    return this.parcelsNear(p => isCropField(p) && (!stage || fieldStage(p, this._claimOf(p.parcel_id)).stage === stage), limit)
      .map(p => { const fs = fieldStage(p, this._claimOf(p.parcel_id)); return Object.assign({ stage: fs.stage, t: +fs.t.toFixed(3), ripeInMin: Math.ceil(fs.ripeInS / 60), mine: fs.mine, kind: fs.kind }, p); });
  },
  /** Forest stands near the camera (NS 56 or lidar canopy ≥ 50 %) with owner + regrowth stage. */
  forests(limit = 20, onlyMine = false) {
    return this.parcelsNear(p => { const c = this._claimOf(p.parcel_id); return claimIsForest(p, c) && (!onlyMine || c?.player_id === G.player?.id); }, limit)
      .map(p => { const c = this._claimOf(p.parcel_id), fs = forestStage(c); const fv = G.forestValues[p.parcel_id]?.estimate;
        return Object.assign({ mine: c?.player_id === G.player?.id, converted: c?.converted_to || null, stage: fs.stage, factor: +fs.factor.toFixed(2), nextInMin: Math.ceil(fs.nextInS / 60), vfm: fv ? Math.round(fv.vfm) : undefined, source: fv?.source }, p); });
  },
  /** Select a forest parcel, fetch its timber estimate (/api/forest-value) and return the estimate. */
  async forest(parcelId, center = false) {
    const f = typeof parcelId === 'string' ? this.find(parcelId) : parcelId || G.sel; if (!f) return null;
    await this.parcel(f, center);
    const fv = await fetchForestValue(f);
    showParcelPopup(f, G.selFp); render();
    const c = this._claimOf(f.properties.parcel_id);
    return { parcel_id: f.properties.parcel_id, isForest: claimIsForest(f.properties, c), stage: forestStage(c), estimate: fv?.estimate || null, cached: fv?.cached };
  },
  /** Harvest the given/selected parcel for real (field → /api/harvest-parcel, forest → /api/harvest-forest). Returns coin delta. */
  async harvest(parcelId) {
    if (parcelId) await this.parcel(parcelId); if (!G.sel) return null;
    const before = G.player?.coins ?? 0, p = G.sel.properties;
    if (claimIsForest(p, this._claimOf(p.parcel_id))) await doHarvestForest(); else await doHarvest();
    await new Promise(r => setTimeout(r, 300)); render();
    return { parcel_id: p.parcel_id, coins: (G.player?.coins ?? 0) - before, claim: this._claimOf(p.parcel_id) };
  },
  /** Convert the given/selected owned parcel for real: 'biodiversity' | 'forest' | 'wildforest'. */
  async convert(parcelId, to = 'biodiversity') {
    if (parcelId) await this.parcel(parcelId); if (!G.sel) return null;
    await doConvert(to); await new Promise(r => setTimeout(r, 300)); render();
    return this._claimOf(G.sel.properties.parcel_id);
  },
  /** Living overlays (Naturschutz / Naturwald / Schlag): stats, or set {level:0|1|2|null, quality:0.3..1, clear:true}. */
  nature(o) {
    if (o) {
      if ('level' in o) NATURE.level = o.level;
      if ('quality' in o) NATURE.quality = Math.max(0.3, Math.min(1, o.quality));
      if (o.clear) { NATURE.scenes.clear(); FOREST.scenes.clear(); }
      render();
    }
    const items = k => { const h = {}; for (const [id, sc] of k.scenes) for (const it of sc.items) h[it.k] = (h[it.k] || 0) + 1; return h; };
    return { onScreen: NATURE.onScreen, level: natureAnimLevel(), forcedLevel: NATURE.level, quality: +NATURE.quality.toFixed(2), costMs: +NATURE._cost.toFixed(2),
      natureScenes: NATURE.scenes.size, forestScenes: FOREST.scenes.size, natureKinds: items(NATURE), forestKinds: items(FOREST), animBudget: giantAnimBudget() };
  },
  /** Item-kind histogram of one parcel's procedural scene (nature reserve, 'wild' or 'schlag' forest). */
  scene(parcelId, mode) {
    const f = this.find(parcelId); if (!f) return null;
    const sc = mode ? forestScene(f, mode) : natureScene(f);
    const names = Object.fromEntries(Object.entries(Object.assign({}, NK, FK)).map(([k, v]) => [v, k]));
    const h = {}; for (const it of sc.items) { const n = names[it.k] || it.k; h[n] = (h[n] || 0) + 1; }
    return { parcel_id: parcelId, mode: mode || 'nature', items: sc.items.length, spacing_m: sc.sp, kinds: h };
  },
  /** Show the loading screen frozen at a given progress (for screenshots).
   *  DEV.loading(62, 'Dürnstein (12105)') ; DEV.loading(false) returns to game. */
  loading(pct, muni) {
    if (pct === false) { show('game'); stopTipRotation(); resizeGame(); render(); return; }
    show('loading');
    if (muni) document.getElementById('loading-muni').textContent = '📍 ' + muni;
    document.getElementById('loading-sub').textContent = '';
    const steps = ['ls-session','ls-parcels','ls-kg','ls-treasures','ls-ready'];
    const doneN = Math.floor(pct / 100 * steps.length);
    steps.forEach((id, i) => setLoadStep(id, i < doneN ? 'done' : i === doneN ? 'active' : ''));
    setLoadProgress(pct);
    startTipRotation();
  },
  /** Fake a GPS fix (shows the blue marker + enables tree distance/bearing). */
  gps(lon, lat, acc = 12) {
    G.geo.watching = true; G.geo.follow = false;
    G.geo.lon = lon; G.geo.lat = lat; G.geo.acc = acc;
    const b = document.getElementById('btn-gps'); if (b) { b.style.display = ''; b.classList.add('active'); }
    render();
  },
  /** Run the similar-parcels search for the selected (or given) parcel and wait. */
  async similar(parcelId, radius = 5000) {
    if (parcelId) await this.parcel(parcelId);
    G.similarRadius = radius;
    await findSimilarParcels();
    await this.idle();
    render();
    return G.similar ? G.similar.data.results.length : 0;
  },
  /** Treasures still on the map (unclaimed), nearest to camera first. */
  treasures() {
    return G.treasures.filter(t => !t.found_by).map(t => {
      const d = Math.hypot((t.lon - G.cam.lon) * 0.68, t.lat - G.cam.lat) * 111000;
      return { id: t.id, type: t.treasure_type, value: t.value, lon: t.lon, lat: t.lat, d: Math.round(d), species: t.species_name || null };
    }).sort((a, b) => a.d - b.d);
  },
  /** Fly to a treasure (nearest by default) and optionally claim it. */
  async treasure(id, claim = false, zoom = 17.5) {
    const list = this.treasures();
    const t = id ? G.treasures.find(x => x.id === id) : (list[0] && G.treasures.find(x => x.id === list[0].id));
    if (!t) return null;
    await this.goto(t.lon, t.lat, zoom);
    if (claim) { await claimTreasure(t); await new Promise(r => setTimeout(r, 800)); render(); }
    return t;
  },
  /** Open the popup for the n-th nearest building footprint (building info rows). */
  async building(n = 0) {
    const [cx, cy] = [gc.width / 2, gc.height / 2];
    const fps = G.buildingFootprints.map(fp => { const [x, y] = toScreen(...featureLonLat(fp)); return { fp, d: Math.hypot(x - cx, y - cy) }; })
      .sort((a, b) => a.d - b.d);
    const o = fps[n]; if (!o) return null;
    const [lon, lat] = featureLonLat(o.fp);
    const f = G.parcelPolys.find(p => isAreaGeom(p.geometry) && pipGeom(lon, lat, p.geometry));
    if (!f) return null;
    showParcelPopup(f, o.fp);
    await new Promise(r => setTimeout(r, 1200));
    render();
    return o.fp.properties;
  },
  /** Mobile bottom sheet: expand/collapse the sidebar. */
  sheet(open) {
    const el = document.getElementById('sidebar'); if (!el) return;
    el.classList.toggle('expanded', !!open);
  },
  /** Toggle the N2K overlay. */
  n2k(on) { G.n2kVisible = !!on; const b = document.getElementById('btn-n2k'); if (b) b.classList.toggle('off', !on); render(); },
  /** Compact state snapshot. */
  state() {
    return {
      cam: { lon: +G.cam.lon.toFixed(5), lat: +G.cam.lat.toFixed(5), zoom: +G.cam.zoom.toFixed(2) },
      polys: G.parcelPolys.length, fps: G.buildingFootprints.length, claimed: G.claimed.length,
      treasures: G.treasures.length, tall: G.tallUnlocked, revealed: G.tallRevealed,
      trees: allTallTrees().length, busy: _vpBusy, coins: G.player?.coins, xp: G.player?.xp,
      popups: ['parcel-popup','ez-popup','kg-popup','tree-popup'].filter(id => document.getElementById(id)?.classList.contains('open')),
      sel: G.sel?.properties?.parcel_id || null,
      clock: this._realNow ? this.clock() : null, natureOnScreen: NATURE.onScreen, mocks: (G.claimed||[]).filter(c => c._mock).length,
    };
  },
};

// ================= HERALD — typewriter quest/hint dialogue =================
// RPG-style dialogue box over the map. Fresh players get a 3-line intro that
// ends on their first quest; everyone gets quest-completion beats and a few
// one-shot contextual hints. Click/tap: finish line → next line → dismiss.
// No storage (no cookies policy): "seen" lives for this page load only.
const Herald = {
  el: null, seen: new Set(), lines: [], idx: 0, timer: null, typing: null, mode: null, hideTimer: null,
  reduced: window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches,

  init() {
    if (this.el) return;
    this.el = document.getElementById('herald');
    if (!this.el) return;
    this.el.addEventListener('click', e => { if (e.target.id !== 'herald-close') this.advance(); });
    document.getElementById('herald-close').onclick = e => { e.stopPropagation(); this.dismiss(true); };
    document.getElementById('herald-act').onclick = e => { e.stopPropagation(); this.runAction(); };
    document.addEventListener('keydown', e => {
      if (!this.el.classList.contains('show')) return;
      if (e.key === 'Escape') this.dismiss(true);
      else if ((e.key === 'Enter' || e.key === ' ') && !/INPUT|TEXTAREA/.test(document.activeElement?.tagName)) { e.preventDefault(); this.advance(); }
    });
    // Hide while a popup is open (mobile: they share the bottom edge).
    setInterval(() => {
      if (!this.el.classList.contains('show')) return;
      const busy = !!document.querySelector('.popup.open');
      this.el.style.visibility = busy ? 'hidden' : '';
    }, 300);
  },
  reset() { this.seen.clear(); this.dismiss(); },

  /** Quest tapped in the sidebar → detailed briefing with a one-tap action. */
  activeQuestId: null, action: null,
  brief(id) {
    this.init(); if (!this.el) return;
    const c = (G.challenges||[]).find(x => x.id === id); if (!c) return;
    if (this.activeQuestId === id && this.el.classList.contains('show')) { this.dismiss(true); return; } // toggle
    const b = questBriefing(c);
    this.dismiss(); // clean slate, no chaining onto hints
    this.activeQuestId = id;
    document.querySelectorAll('.quest-item').forEach(q => q.classList.toggle('active', +q.dataset.qid === id));
    // Mobile: the sheet covers the herald — collapse it so the briefing is visible.
    const sb = document.getElementById('sidebar'); if (sb && innerWidth <= 768) sb.classList.remove('expanded');
    this.action = b.act || null;
    this.play(b.lines, 'brief');
  },
  runAction() {
    const a = this.action; if (!a) return;
    this.dismiss(true);
    try { a.run(); } catch (e) { console.warn('quest action', e); }
  },

  /** Top open quest (respecting enhanced visibility). */
  topQuest() { return visibleQuests()[0] || null; },
  questLine(c, tag) {
    return {
      tag: tag || tr('Aufgabe'), icon: QUEST_ICONS[c.challenge_type] || '📜',
      html: `<b>${esc(tr(c.title))}</b>\n${esc(tr(c.description || ''))}  <span class="rw">+${c.reward_coins}🪙 +${c.reward_xp}⚡</span>`,
    };
  },

  start(mode) {
    this.init(); if (!this.el) return;
    const q = this.topQuest();
    if (mode === 'intro' && !this.seen.has('intro')) {
      this.seen.add('intro');
      const muni = esc(G.session?.municipality_name || 'Österreich');
      const name = esc(G.player?.name || '');
      const lines = [
        { icon:'🏰', tag: tr('Servus'), html: tr('Servus in') + ` <b>${muni}</b>, ${name}!\n` + tr('Alles hier ist echt — jede Parzelle stammt aus dem österreichischen Kataster.') },
        { icon:'🏴', tag: tr('So geht’s'), html: tr('Tipp auf eine Parzelle und kauf sie dir.') + ` <b>${(G.player?.coins ?? 10000).toLocaleString('de-AT')}🪙</b> ` + tr('hast du im Börserl.') + '\n' + tr('Was dir gehört, kannst du in 🌿 Naturschutz umwandeln — Ziel: 30 % der Gemeinde.') },
        { icon:'💎', tag: tr('Unterwegs'), html: tr('Halt die Augen offen nach 💎 Schätzen und 🦎 seltenen Arten der Roten Liste — beides bringt Münzen und XP.') },
      ];
      if (q) lines.push(this.questLine(q, tr('Deine erste Aufgabe')));
      this.play(lines, 'intro');
    } else if (q && G.freshPlayer && !this.seen.has('quest0')) {
      this.seen.add('quest0');
      this.play([this.questLine(q)], 'quest');
    }
  },

  /** One-shot contextual hints. */
  hint(key) {
    this.init(); if (!this.el || this.seen.has(key)) return;
    if (this.mode === 'intro' && this.el.classList.contains('show')) return; // don't interrupt the intro
    const H = {
      first_claim: { icon:'🌿', tag: tr('Tipp'), html: tr('Dein erstes Stückerl Land! Mach es noch einmal auf und wandle es in') + ' <b>🌿 ' + tr('Naturschutz') + '</b> ' + tr('um — das bringt XP und zählt zum 30 %-Ziel.') },
      first_field: { icon:'🌾', tag: tr('Dein Acker'), html: tr('Äcker reifen alle 40 Minuten — jeder zu seiner Zeit. Ist deiner golden, zeigt ein 🌾-Marker: ernten bringt Münzen. Wartest du zu lang, ernten die Bauern. Oder lass ihn als') + ' <b>🌿 ' + tr('Brache') + '</b> ' + tr('liegen — das zählt zum Naturschutz.') },
      trees_unlocked: { icon:'🌲', tag: tr('Freigeschaltet'), html: tr('Riesenbäume sichtbar! Goldene Bäume zeigen dir, wo sie stehen. Kauf dir eine Parzelle mit so einem Riesen für die Aufgabe') + ' <b>' + tr('Baumriese') + '</b>.' },
      drought: { icon:'☀️', tag: tr('Dürre'), html: tr('Das Grundwasser steht hier') + ' <b>' + fmtSigma(G.drought?.sigma || 0) + '</b> ' + tr('unter normal — deine Felder tragen nur') + ' <b>×' + ((G.dossiers[G.drought?.kg]?.game?.yield_factor) ?? 0.6).toFixed(1).replace('.', ',') + '</b>. ' + tr('Ein 🕳️ Brunnen schützt, Brache zählt zum Naturschutz.') + ' <span class="pp-ez-link" onclick="openDossier(null,\'water\')">📖 ' + tr('Chronik') + '</span>' },
      enhanced: { icon:'✨', tag: tr('Enhanced Gelände'), html: tr('Da gibt’s echte Baumhöhen aus Laserscans — und versteckte Riesenbäume. Find zuerst einen Schatz, dann siehst du sie.') },
    };
    if (!H[key]) return;
    if (key === 'enhanced' && (!G.freshPlayer || G.tallUnlocked)) { this.seen.add(key); return; }
    this.seen.add(key);
    this.play([H[key]], 'hint', 9000);
  },

  /** Quest completed → celebrate, then reveal the next one. */
  completed(title) {
    this.init(); if (!this.el) return;
    const lines = [{ icon:'🏆', tag: tr('Passt!'), html: '<b>' + esc(tr(title || 'Aufgabe')) + '</b> ✔', cls:'done' }];
    // Next quest gets appended once loadChallenges() refreshed — see questsChanged().
    this._awaitNext = true;
    this.play(lines, 'done', 7000);
  },
  questsChanged() {
    if (!this._awaitNext) return;
    this._awaitNext = false;
    const q = this.topQuest();
    if (q) { this.lines.push(this.questLine(q, tr('Nächste Aufgabe'))); this.renderDots(); }
  },

  play(lines, mode, autoHide) {
    // Something already on screen → chain the new lines onto it instead of clobbering.
    if (this.el.classList.contains('show') && this.lines.length) {
      this.lines.push(...lines); this.autoHide = autoHide || this.autoHide; this.renderDots();
      if (this.el.classList.contains('ready') && this.idx === this.lines.length - lines.length - 1) { // was idle on its last line
        clearTimeout(this.hideTimer); clearTimeout(this.timer);
        this.timer = setTimeout(() => this.advance(), 1800);
      }
      return;
    }
    clearTimeout(this.hideTimer); this.stopTyping();
    this.lines = lines; this.idx = 0; this.mode = mode; this.autoHide = autoHide || 0;
    this.el.className = 'herald show' + (mode === 'quest' ? ' quest' : '');
    this.showLine();
  },
  renderDots() {
    const d = document.getElementById('herald-dots');
    d.innerHTML = this.lines.length > 1 ? this.lines.map((_, i) => `<i class="${i <= this.idx ? 'on' : ''}"></i>`).join('') : '';
  },
  showLine() {
    const L = this.lines[this.idx]; if (!L) return this.dismiss();
    this.el.classList.remove('ready');
    this.el.classList.toggle('done', !!L.cls);
    document.getElementById('herald-avatar').textContent = L.icon || '🏰';
    document.getElementById('herald-tag').textContent = L.tag || '';
    this.renderDots();
    document.getElementById('herald-act').style.display = 'none';
    this.type(L.html, () => {
      this.el.classList.add('ready');
      const last = this.idx >= this.lines.length - 1;
      if (last && this.mode === 'brief' && this.action) { const a = document.getElementById('herald-act'); a.textContent = this.action.label; a.style.display = ''; }
      const dwell = Math.min(9000, 2600 + L.html.replace(/<[^>]+>/g, '').length * 45);
      // Auto-advance through multi-line sequences; quests linger, hints fade.
      // (while a popup hides us, timers re-arm instead of firing unseen)
      const later = (fn, ms) => setTimeout(() => document.querySelector('.popup.open') ? (this[fn === 'adv' ? 'timer' : 'hideTimer'] = later(fn, 3000)) : (fn === 'adv' ? this.advance() : this.dismiss()), ms);
      if (!last) this.timer = later('adv', dwell);
      else if (this.autoHide) this.hideTimer = later('hide', Math.max(this.autoHide, dwell));
      else if (this.mode === 'intro' || this.mode === 'quest') this.hideTimer = later('hide', 25000);
      else if (this.mode === 'brief') this.hideTimer = later('hide', 40000);
    });
  },
  /** Typewriter over an HTML string: tags appear whole, text char by char. */
  type(html, done) {
    const t = document.getElementById('herald-text');
    t.classList.remove('typed');
    if (this.reduced) { t.innerHTML = html; t.classList.add('typed'); done(); return; }
    const tokens = html.match(/<[^>]+>|&[a-z#0-9]+;|[\s\S]/gu) || [];
    let i = 0, out = '';
    const step = () => {
      if (i >= tokens.length) { t.innerHTML = out; t.classList.add('typed'); this.typing = null; done(); return; }
      const tk = tokens[i++]; out += tk;
      t.innerHTML = out + '<span class="cur"></span>';
      let d = 24;
      if (tk[0] === '<' || tk[0] === '&') d = 0;
      else if (/[.!?]/.test(tk)) d = 260; else if (/[,;:—]/.test(tk)) d = 120; else if (tk === '\n') d = 200;
      else d = 18 + Math.random() * 22;
      this.typing = setTimeout(step, d);
    };
    step();
    this._finish = () => { this.stopTyping(); t.innerHTML = html; t.classList.add('typed'); done(); };
  },
  stopTyping() { clearTimeout(this.typing); clearTimeout(this.timer); this.typing = null; this._finish = null; },
  /** Click: finish typing → next line → dismiss. */
  advance() {
    if (this.typing) { const f = this._finish; this._finish = null; f && f(); return; }
    clearTimeout(this.timer); clearTimeout(this.hideTimer);
    this.idx++;
    if (this.idx < this.lines.length) this.showLine(); else this.dismiss();
  },
  dismiss(user) {
    if (!this.el) return;
    this.stopTyping(); clearTimeout(this.hideTimer);
    this.el.classList.remove('show', 'ready');
    if (user && this.mode === 'intro') this.seen.add('quest0');
    if (this.mode === 'brief') { this.activeQuestId = null; document.querySelectorAll('.quest-item.active').forEach(q => q.classList.remove('active')); }
    this.action = null; const ab = document.getElementById('herald-act'); if (ab) ab.style.display = 'none';
    this.mode = null; this._awaitNext = false; this.lines = []; this.idx = 0;
  },
};

// ================= TOPONYMS — BEV DLM Geographische Namen =================
// Official Austrian place names (Riednamen, Almen, Höfe, Gipfel, Bäche, Kapellen…)
// drawn as hand-lettered map labels in Settlers style. Data: cadastre
// /spatial/bbox?layers=toponyms (BEV DLM 7000 NAMEN, CC BY 4.0, ±50 m points).
// Loaded per 0.04° grid tile on camera idle, deduped by BEV GLOBALID.
G.toponyms = [];            // all loaded rows
G.topoIds = new Set();      // ids already loaded
G.topoTiles = new Set();    // grid tiles fetched
G.topoVisible = localStorage.getItem('topoVisible') !== '0';
G.topoRied = null;          // last placed label set (for hit-testing / DEV)
let _topoBusy = 0, _topoFadeRAF = null;
const TOPO_TILE = 0.04;

/** Label class → { minZoom, prio, style }. Higher prio wins collisions. */
function topoClass(t) {
  const fc = t.f_code, L = t.layer;
  if (L === 'siedlung') {
    if (fc === 7101) return { z: 12, prio: 100, kind: 'town', size: 12 };
    if (fc === 7102) return { z: 12, prio: 95,  kind: 'town', size: 10 };
    if (fc === 7103) return { z: 13, prio: 90,  kind: 'town', size: 8 };
    if (fc === 7104) return { z: 14.5, prio: 70, kind: 'town', size: 7 };
    if (t.abandoned)  return { z: 16.5, prio: 20, kind: 'ruinhof' };
    return { z: 16, prio: 40, kind: 'hof' };                     // 7111 Einzelhäuser / Almhütten
  }
  if (L === 'gelaende') {
    if (fc === 7302) return { z: 13, prio: 85, kind: 'peak' };
    if (fc === 7301) return { z: 13, prio: 80, kind: 'range' };
    if (fc === 7303) return { z: 14.5, prio: 60, kind: 'range' };
    if (fc === 7304) return { z: 14.5, prio: 60, kind: 'pass' };
    return { z: 14.5, prio: 55, kind: 'valley' };                // 7305 Tal
  }
  if (L === 'gewaesser') {
    if (fc === 7501 || fc === 7511) return { z: 12, prio: 88, kind: 'water', size: 18 };
    if (fc === 7502 || fc === 7512) return { z: 13.5, prio: 75, kind: 'water', size: 16 };
    if (fc === 7513) return { z: 14.5, prio: 65, kind: 'water', size: 15 };
    return { z: 15.5, prio: 45, kind: 'water', size: 14 };       // Bäche
  }
  if (L === 'gletscher') return { z: 13, prio: 82, kind: 'ice' };
  if (L === 'gebiet')    return { z: 14, prio: 58, kind: 'area' };
  if (L === 'ried')      return { z: 15, prio: 30, kind: 'ried' };
  if (L === 'sonstige')  return { z: 15.5, prio: 50, kind: 'poi' };
  return null;
}

/** Objektart code (sonstige) → tiny pictogram. */
const TOPO_POI_ICON = {
  '2301': '⛪', '2213': '🍺', '2216': '🛖', '2212': '⛺', '2313': '✝', '2321': '🏰', '2323': '🏚️',
  '2324': '🗼', '2331': '🗿', '2222': '🕳️', '2221': '⛏️', '2114': '🪵', '2441': '✚', '2421': '🦌',
  '2405': '🏟️', '2404': '🏟️', '2431': '🎭', '2603': '📌', '7603': '📌',
};
function topoPoiIcon(t) { const c = String(t.objektart || '').slice(0, 4); return TOPO_POI_ICON[c] || '📌'; }
function topoKindLabel(t) {
  const c = topoClass(t); if (!c) return '';
  switch (c.kind) {
    case 'town': return t.f_name;
    case 'hof': return /alm|alpe|hütte/i.test(t.name) ? tr('Almhütte') : tr('Hof');
    case 'ruinhof': return tr('aufgelassener Hof');
    case 'peak': return tr('Gipfel'); case 'range': return t.f_name; case 'pass': return tr('Übergang');
    case 'valley': return tr('Tal'); case 'water': return t.f_name; case 'ice': return tr('Gletscher');
    case 'area': return tr('Gebiet'); case 'ried': return tr('Riedname'); case 'poi': return (t.art || t.objektart || '').replace(/^\d+\s*/, '');
  }
  return '';
}

/** Fetch toponyms for the (padded) viewport as 0.04° grid tiles. */
async function loadToponyms() {
  if (!gc || !G.session) return;
  const v = viewBounds();
  const pad = (v.e - v.w) * 0.25;
  const b = { w: v.w - pad, e: v.e + pad, s: v.s - pad * 0.72, n: v.n + pad * 0.72 };
  const x0 = Math.floor(b.w / TOPO_TILE), x1 = Math.floor(b.e / TOPO_TILE);
  const y0 = Math.floor(b.s / TOPO_TILE), y1 = Math.floor(b.n / TOPO_TILE);
  const tiles = [];
  for (let x = x0; x <= x1; x++) for (let y = y0; y <= y1; y++) {
    const k = x + ',' + y;
    if (G.topoTiles.has(k)) continue;
    const cx = (x + 0.5) * TOPO_TILE, cy = (y + 0.5) * TOPO_TILE;
    if (typeof insideAustria === 'function' && !insideAustria(cx, cy)) continue;
    tiles.push({ k, x, y, d: Math.hypot(cx - G.cam.lon, (cy - G.cam.lat) / 0.72) });
  }
  if (!tiles.length) return;
  tiles.sort((a, b) => a.d - b.d);
  const batch = tiles.slice(0, 12);
  _topoBusy++;
  try {
    await Promise.all(batch.map(async t => {
      G.topoTiles.add(t.k);
      const w = (t.x * TOPO_TILE).toFixed(3), e = ((t.x + 1) * TOPO_TILE).toFixed(3);
      const s = (t.y * TOPO_TILE).toFixed(3), n = ((t.y + 1) * TOPO_TILE).toFixed(3);
      try {
        const d = await GET(CAD + `/spatial/bbox?west=${w}&south=${s}&east=${e}&north=${n}&layers=toponyms&limit=3000`);
        const rows = d?.data?.toponyms || [];
        let added = 0;
        for (const r of rows) {
          if (!r.id || G.topoIds.has(r.id) || !r.name) continue;
          G.topoIds.add(r.id); r._cls = topoClass(r); r._t0 = 0;
          G.toponyms.push(r); added++;
        }
        if (added) render();
      } catch (err) { G.topoTiles.delete(t.k); console.warn('toponyms tile failed', t.k, err); }
    }));
  } finally { _topoBusy--; }
}

/** Nearest toponym of given kinds to (lon,lat) within maxM metres. */
function nearestToponym(lon, lat, maxM, filter) {
  const kx = 111320 * Math.cos(lat * Math.PI / 180), ky = 110540;
  let best = null, bd = Infinity;
  for (const t of G.toponyms) {
    if (filter && !filter(t)) continue;
    const d = Math.hypot((t.lon - lon) * kx, (t.lat - lat) * ky);
    if (d < bd) { bd = d; best = t; }
  }
  return best && bd <= maxM ? { t: best, d: bd } : null;
}

/** Flur context for a parcel: a toponym INSIDE the polygon (Hof, Kapelle, Gipfel…)
 *  or the nearest Riedname within 400 m. Returns {label, sub, t} or null. */
function parcelFlur(f) {
  if (!f || !G.toponyms.length) return null;
  const [lon, lat] = featureLonLat(f);
  const g = f.geometry;
  if (isAreaGeom(g)) {
    let inside = null;
    for (const t of G.toponyms) {
      if (t.layer === 'ried' || t.layer === 'gebiet' || (t.layer === 'gelaende' && t.f_code !== 7302)) continue;
      if (Math.abs(t.lon - lon) > 0.03 || Math.abs(t.lat - lat) > 0.02) continue;
      if (pipGeom(t.lon, t.lat, g) && (!inside || t._cls.prio > inside._cls.prio)) inside = t;
    }
    if (inside) return { label: inside.name, sub: topoKindLabel(inside), t: inside, inside: true };
  }
  const r = nearestToponym(lon, lat, 400, t => t.layer === 'ried');
  if (r) return { label: r.t.name, sub: tr('Riedname') + (r.d > 60 ? ' · ~' + Math.round(r.d / 10) * 10 + ' m' : ''), t: r.t };
  const a = nearestToponym(lon, lat, 800, t => t.layer === 'gebiet' || (t.layer === 'siedlung' && t.f_code === 7111));
  if (a) return { label: a.t.name, sub: topoKindLabel(a.t) + ' · ~' + Math.round(a.d / 10) * 10 + ' m', t: a.t };
  return null;
}

// ---- Rendering ----
function _topoFont(kind, size) {
  switch (kind) {
    case 'town':  return `${size}px "Press Start 2P", monospace`;
    case 'peak': case 'range': case 'pass': return 'bold 15px VT323, monospace';
    case 'ice':   return 'bold 15px VT323, monospace';
    case 'valley': case 'area': return 'italic 15px VT323, monospace';
    case 'water': return `italic ${size}px VT323, monospace`;
    case 'ried':  return 'italic 15px VT323, monospace';
    case 'hof': case 'ruinhof': return '14px VT323, monospace';
    default:      return MAP_FONT.label;
  }
}
function _topoColor(kind) {
  switch (kind) {
    case 'town':  return '#fff3c4';
    case 'peak': case 'range': case 'pass': return '#f2e6d0';
    case 'ice':   return '#dff6ff';
    case 'water': return '#a9dcff';
    case 'valley': case 'area': return 'rgba(255,240,200,0.85)';
    case 'ried':  return 'rgba(255,236,190,0.78)';
    case 'hof':   return '#ffe9a0';
    case 'ruinhof': return 'rgba(200,190,170,0.75)';
    default:      return '#ffe9a0';
  }
}
function _topoText(t, kind) {
  if (kind === 'peak') return '▲ ' + t.name + (t.elevation_m ? ' ' + t.elevation_m : '');
  if (kind === 'pass') return '⌒ ' + t.name + (t.elevation_m ? ' ' + t.elevation_m : '');
  if (kind === 'ice') return '❄ ' + t.name;
  if (kind === 'area' || kind === 'range') return t.name.toUpperCase();
  if (kind === 'hof') return '⌂ ' + t.name;
  if (kind === 'ruinhof') return '† ' + t.name;
  if (kind === 'poi') return topoPoiIcon(t) + ' ' + t.name;
  return t.name;
}

function drawToponyms(ctx) {
  if (!G.topoVisible || !G.toponyms.length) return;
  const zoom = G.cam.zoom, W = gc.width, H = gc.height;
  const now = performance.now();
  const cands = [];
  for (const t of G.toponyms) {
    const c = t._cls; if (!c || zoom < c.z) { t._t0 = 0; continue; }
    const [x, y] = toScreen(t.lon, t.lat);
    if (x < -120 || x > W + 120 || y < -30 || y > H + 30) { t._t0 = 0; continue; }
    cands.push({ t, c, x, y });
  }
  // Priority first, then tie-break: bigger things first, then top-to-bottom
  cands.sort((a, b) => b.c.prio - a.c.prio || a.y - b.y);
  const placed = [];
  const budget = zoom < 14 ? 40 : zoom < 15.5 ? 90 : 160;
  let fading = false;
  ctx.save();
  ctx.textBaseline = 'middle';
  ctx.lineJoin = 'round';
  const shown = [];
  // Name dedup: the register carries the same name several times for one
  // place (Stadt "Dürnstein" + Schloss "Dürnstein" + Burgruine "Dürnstein";
  // a Ried split over two KGs). Once a name is on screen, a second copy only
  // shows if it is far away (and never for the same layer within 600 px).
  const byName = new Map(); // norm name → [{x,y,layer}]
  const norm = n => n.toLowerCase().replace(/[^a-zäöüß0-9]/g, '');
  // Fixed obstacles: treasures, the selected parcel's centre, giant-tree hint.
  const obstacles = [];
  for (const tz of G.treasures) { const [ox, oy] = toScreen(tz.lon, tz.lat); obstacles.push({ x: ox - 16, y: oy - 20, w: 32, h: 36 }); }
  const pad = 3;
  const collides = (bx, by, w, h) => {
    for (const p of placed) if (bx < p.x + p.w + pad && bx + w + pad > p.x && by < p.y + p.h + pad && by + h + pad > p.y) return true;
    for (const p of obstacles) if (bx < p.x + p.w && bx + w > p.x && by < p.y + p.h && by + h > p.y) return true;
    return bx < 2 || by < 2 || bx + w > W - 2 || by + h > H - 2;
  };
  for (const cd of cands) {
    if (placed.length >= budget) break;
    const { t, c } = cd;
    const kind = c.kind;
    const size = c.size || 0;
    const nk = norm(t.name);
    const prev = byName.get(nk);
    if (prev) {
      let dup = false;
      for (const q of prev) {
        const d = Math.hypot(q.x - cd.x, q.y - cd.y);
        // POI named after the town it stands in (Schloss/Ruine "Dürnstein")
        // never repeats the town label; otherwise proximity-based.
        const townPoi = (q.layer === 'siedlung' && t.layer === 'sonstige') || (q.layer === 'sonstige' && t.layer === 'siedlung');
        if (townPoi || d < 400 || (q.layer === t.layer && d < 700)) { dup = true; break; }
      }
      if (dup) { t._t0 = 0; continue; }
    }
    ctx.font = _topoFont(kind, size);
    const spaced = kind === 'town' || kind === 'area' || kind === 'range' || kind === 'ried';
    if ('letterSpacing' in ctx) ctx.letterSpacing = spaced ? (kind === 'town' ? '1px' : '2px') : '0px';
    const text = _topoText(t, kind);
    const m = ctx.measureText(text);
    const w = m.width + 8, h = (kind === 'town' ? size * 1.6 : 16) + 4;
    // Candidate anchors, in preference order. Area-like names (Ried, Gebiet,
    // Tal, Gewässer) want to sit ON their point; point features (towns,
    // peaks, Höfe, POIs) sit above it and fall back to the right / below /
    // left so a crowded spot still gets its name instead of nothing.
    const areaLike = kind === 'ried' || kind === 'area' || kind === 'valley' || kind === 'water';
    const up = kind === 'town' ? h / 2 + 8 : 11;
    const offs = areaLike
      ? [[0, 0], [0, -h], [0, h], [w / 2 + 6, 0], [-w / 2 - 6, 0]]
      : [[0, -up], [w / 2 + 8, -2], [0, up + 2], [-w / 2 - 8, -2], [w / 2 + 8, -up], [-w / 2 - 8, -up]];
    let cx = 0, cy = 0, ok = false;
    for (const [dx, dy] of offs) {
      cx = cd.x + dx; cy = cd.y + dy;
      if (!collides(cx - w / 2, cy - h / 2, w, h)) { ok = true; break; }
    }
    if (!ok) { t._t0 = 0; continue; }
    placed.push({ x: cx - w / 2, y: cy - h / 2, w, h });
    if (!prev) byName.set(nk, [{ x: cd.x, y: cd.y, layer: t.layer }]); else prev.push({ x: cd.x, y: cd.y, layer: t.layer });
    if (!t._t0) t._t0 = now;
    const k = Math.min(1, (now - t._t0) / 420);
    if (k < 1) fading = true;
    shown.push({ t, kind, text, x: cx, y: cy, alpha: k, dx: cx - cd.x, dy: cy - cd.y });
  }
  // Draw in two passes so outlines never cut through neighbouring glyphs.
  for (const s of shown) {
    ctx.font = _topoFont(s.kind, s.t._cls.size || 0);
    if ('letterSpacing' in ctx) ctx.letterSpacing = (s.kind === 'town') ? '1px' : (s.kind === 'area' || s.kind === 'range' || s.kind === 'ried') ? '2px' : '0px';
    ctx.textAlign = 'center';
    ctx.globalAlpha = s.alpha * (s.kind === 'ried' ? 0.9 : 1);
    // Pixel-art style: hard offset shadow + dark outline, no blur
    ctx.strokeStyle = s.kind === 'water' ? 'rgba(10,30,60,0.85)' : 'rgba(30,18,6,0.85)';
    ctx.lineWidth = s.kind === 'town' ? 3 : 2.5;
    ctx.strokeText(s.text, s.x, s.y);
    ctx.fillStyle = _topoColor(s.kind);
    ctx.fillText(s.text, s.x, s.y);
    if (s.kind !== 'town' && s.kind !== 'ried' && s.kind !== 'area' && s.kind !== 'valley' && s.kind !== 'water' && Math.abs(s.dx) > 4) {
      // Label pushed sideways: tiny pixel leader dot at the true position
      ctx.fillStyle = 'rgba(30,18,6,0.85)'; ctx.fillRect(s.x - s.dx - 2, s.y - s.dy - 2, 4, 4);
      ctx.fillStyle = _topoColor(s.kind); ctx.fillRect(s.x - s.dx - 1, s.y - s.dy - 1, 2, 2);
    }
    if (s.kind === 'town') {
      // Small pennant tick under settlement names — the Settlers "town sign"
      const tw = ctx.measureText(s.text).width;
      ctx.fillStyle = 'rgba(30,18,6,0.85)'; ctx.fillRect(s.x - tw / 2, s.y + (s.t._cls.size || 8) * 0.8 + 1, tw, 2);
      ctx.fillStyle = '#d8b040'; ctx.fillRect(s.x - tw / 2, s.y + (s.t._cls.size || 8) * 0.8, tw, 1);
    }
  }
  ctx.restore();
  if ('letterSpacing' in ctx) ctx.letterSpacing = '0px';
  ctx.globalAlpha = 1;
  G.topoShown = shown;
  if (fading && !_topoFadeRAF) {
    _topoFadeRAF = requestAnimationFrame(() => { _topoFadeRAF = null; render(); });
  }
}

/** Toponym search results for the in-game search (near the camera first). */
async function searchToponyms(q, limit) {
  try {
    const d = await GET(CAD + `/toponyms/search?q=${encodeURIComponent(q)}&near=${G.cam.lon.toFixed(3)},${G.cam.lat.toFixed(3)}&radius=60000&limit=${limit || 5}`);
    return (d?.data || []).map(t => ({ _topo: t, lon: t.lon, lat: t.lat, display_name: t.name }));
  } catch (e) { return []; }
}
function topoZoom(t) {
  const c = topoClass(t) || { kind: 'poi' };
  switch (c.kind) {
    case 'town': return t.f_code === 7101 ? 14.5 : t.f_code === 7102 ? 15 : 15.5;
    case 'peak': case 'range': case 'pass': case 'ice': case 'area': return 14.5;
    case 'water': return t.f_code <= 7502 || t.f_code === 7511 ? 14 : 15.5;
    case 'ried': return 16.2;
    default: return 17;
  }
}
function topoIcon(t) {
  const c = topoClass(t) || {};
  switch (c.kind) {
    case 'town': return '🏘️'; case 'hof': return '🏠'; case 'ruinhof': return '🏚️'; case 'peak': return '⛰️';
    case 'range': return '🏔️'; case 'pass': return '🪧'; case 'valley': return '🌄'; case 'water': return '💧';
    case 'ice': return '❄️'; case 'area': return '🌿'; case 'ried': return '🌾'; case 'poi': return topoPoiIcon(t);
  }
  return '📌';
}

/** "Flur" row in the parcel popup: the official Riedname / Hof / Gipfel this
 *  parcel belongs to. Hidden when nothing is known nearby. */
function renderFlurRow(f) {
  const lab = document.getElementById('pp-flur-l'), val = document.getElementById('pp-flur');
  if (!lab || !val) return;
  const fl = parcelFlur(f);
  if (!fl) { lab.style.display = 'none'; val.style.display = 'none'; return; }
  lab.style.display = ''; val.style.display = '';
  lab.textContent = fl.t.layer === 'ried' ? tr('Ried') : tr('Flur');
  val.innerHTML = `<span class="pp-flur-name" title="${esc(tr('BEV Geographische Namen'))}">${topoIcon(fl.t)} ${esc(fl.label)}</span>` +
    (fl.sub ? `<small class="pp-flur-sub">${esc(fl.sub)}</small>` : '');
}

// ================= WATER & GEMEINDE-CHRONIK (gw / holz / farm siblings) =================
// Real-data mechanics surfaced in the game: the KG dossier (GET /api/dossier/{kg})
// drives the "Grundwasser heute" HUD chip, the 3-tab Chronik panel and the
// drought/Förderung numbers in the field popup. Everything here is lazy and
// background — nothing blocks loading.
G.dossiers = {};        // kg → dossier | 'loading' | {error}
G.drought = null;       // drought block of the KG under the camera
G.waterKG = null;       // sticky KG code the chip refers to
G.gwPoints = [];        // GW-2 Messstellen (points, no geometry)
G.gwPointIds = new Set(); G.gwTiles = new Set(); G.gwAttempts = {};
G.gwVisible = localStorage.getItem('gwVisible') !== '0';
G.wpZones = []; G.wpIds = new Set(); G.wpTiles = new Set();   // GW-5 Wasserschutzgebiete
G.wellQuotes = {};      // parcel_id → /api/well-quote
G.fieldEco = {};        // parcel_id → {t, d} (/api/field-economy, 60 s)
G.flow = null;          // GW-6 active Wassertropfen-Reise
G.gwi = null;           // GW-7 {kgs:{kg:[gwi,cat]}}
G.stationHist = {};     // station id → /api/water/station response

const WATER_STATUS = {
  normal:   { de: 'normal',        cls: 'st-normal' },
  low:      { de: 'niedrig',       cls: 'st-low' },
  very_low: { de: 'sehr niedrig',  cls: 'st-very_low' },
  high:     { de: 'hoch',          cls: 'st-high' },
};
const GWI_CAT = { good: ['gut', '#5ad06a'], watch: ['beobachten', '#e8a83a'], stressed: ['belastet', '#e05040'] };
const CDI_COLORS = ['#2e6a3a', '#b8a03a', '#d07a30', '#c83a2a'];   // 0 none · 1 watch · 2 warning · 3 alert
const MONTHS_DE = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D'];

function fmtSigma(s) { return (s > 0 ? '+' : s < 0 ? '−' : '') + Math.abs(s).toFixed(1).replace('.', ',') + 'σ'; }
function fmtNum(v, d = 0) { return (+v || 0).toLocaleString('de-AT', { maximumFractionDigits: d, minimumFractionDigits: d }); }
function fmtPct(v) { return Math.round((+v || 0) * 100) + ' %'; }
function fmtHa(ha) { return ha >= 100 ? fmtNum(ha) + ' ha' : fmtNum(ha, 1) + ' ha'; }
function padKG(kg) { kg = String(kg || ''); return kg.length === 4 ? '0' + kg : kg; }

/** Lazy dossier fetch; resolves to the dossier (or null). Retries once on 202/pending. */
async function loadDossier(kg) {
  kg = padKG(kg); if (!kg) return null;
  const cur = G.dossiers[kg];
  if (cur && cur !== 'loading') return cur.error ? null : cur;
  if (cur === 'loading') return new Promise(res => { (loadDossier._w[kg] = loadDossier._w[kg] || []).push(res); });
  G.dossiers[kg] = 'loading';
  let d = null;
  try { d = await api('GET', '/api/dossier/' + kg, null, { pendingBudgetMs: 12000 }); } catch (e) { d = null; }
  if (!d || d.pending) { delete G.dossiers[kg]; setTimeout(() => loadDossier(kg).then(() => updateWaterChip()), 20000); d = null; }
  else if (d.error) { G.dossiers[kg] = { error: d.error }; d = null; }
  else G.dossiers[kg] = d;
  (loadDossier._w[kg] || []).forEach(r => r(d)); delete loadDossier._w[kg];
  return d;
}
loadDossier._w = {};

/** KG under the camera — sticky so the chip doesn't flicker between parcels. */
function currentWaterKG() {
  const kg = kgAtCamera();
  if (kg) G.waterKG = padKG(kg);
  return G.waterKG;
}

/** GW-4 HUD chip + sidebar row: today's groundwater status of the KG under the camera. */
function updateWaterChip() {
  const chip = document.getElementById('water-chip'), sbRow = document.getElementById('sb-chronik'), sbVal = document.getElementById('sb-drought');
  if (!chip) return;
  const kg = currentWaterKG();
  const abroad = !!G.atBorder && !insideAustria(G.cam.lon, G.cam.lat);
  if (!kg || abroad) { chip.style.display = 'none'; return; }
  const d = G.dossiers[kg];
  if (!d || d === 'loading') { if (!d) loadDossier(kg).then(() => updateWaterChip()); return; }
  if (d.error || !d.drought) { chip.style.display = 'none'; return; }
  const dr = d.drought, w = d.water || {}, now = w.now || {};
  G.drought = Object.assign({ kg }, dr);
  const st = WATER_STATUS[dr.status] || null;
  const known = dr.known !== false && st;
  const stLabel = known ? tr(st.de) : tr('keine Messung');
  const sigma = known && now.sigma != null ? fmtSigma(now.sigma) : '';
  let txt = '💧 ' + stLabel + (sigma ? ' ' + sigma : '');
  if (dr.level >= 1 && dr.label) txt += ' · ' + tr(dr.label);
  chip.className = 'water-chip ' + (known ? st.cls : 'st-unknown') + (dr.level >= 2 ? ' pulse' : '');
  chip.innerHTML = esc(txt) + (known && d.game ? '<span class="wc-sub">' + tr('Ernte') + ' ×' + (d.game.yield_factor || 1).toFixed(1).replace('.', ',') + '</span>' : '');
  chip.title = tr('Grundwasser heute') + ' · ' + esc(d.kg_name || kg) + (now.as_of ? ' · ' + now.as_of : '') + ' — ' + tr('Gemeinde-Chronik öffnen');
  chip.style.display = '';
  if (sbRow) {
    sbRow.style.display = '';
    sbVal.textContent = stLabel + (sigma ? ' ' + sigma : '');
    sbVal.className = known ? st.cls : '';
  }
  // Herald: first time a drought (level ≥ 2) shows up under the camera.
  if (dr.level >= 2 && known) Herald.hint('drought');
  // N2K chip in drawN2KOverlay sits below the badge row — refresh so it doesn't overlap.
  if (G.n2kVisible && Object.keys(G.n2kSites).length) render();
}

// ---- Chronik panel ----
G.dossierTab = 'water';
G.dossierKG = null;
window.openDossier = async function(kg, tab) {
  kg = padKG(kg || currentWaterKG() || (G.claimed.find(c => c.player_id === G.player?.id) || {}).kg_code);
  if (tab) G.dossierTab = tab;
  const pop = document.getElementById('dossier-popup'), body = document.getElementById('dossier-body');
  if (!kg) { toast(tr('Noch keine Katastralgemeinde geladen — zoom näher ran.'), 'err'); return; }
  G.dossierKG = kg;
  document.getElementById('kg-popup').classList.remove('open');
  document.getElementById('station-popup').classList.remove('open');
  pop.classList.add('open');
  if (innerWidth <= 768) { const sb = document.getElementById('sidebar'); if (sb) sb.classList.remove('expanded'); }
  renderDossierTabs();
  const cached = G.dossiers[kg];
  if (!cached || cached === 'loading') body.innerHTML = '<div class="kg-loading">' + tr('Chronik wird aufgeschlagen…') + '</div>';
  const d = await loadDossier(kg);
  if (G.dossierKG !== kg) return;
  document.getElementById('dossier-title').textContent = '📖 ' + (d && (d.gemeinde_name || d.kg_name) ? (d.gemeinde_name || d.kg_name) : 'KG ' + kg);
  if (!d) { body.innerHTML = '<div class="kg-loading">' + tr('Chronik gerade nicht erreichbar — bitte nochmal antippen') + '</div>'; return; }
  renderDossier(d);
};
function renderDossierTabs() {
  document.querySelectorAll('#dossier-tabs button').forEach(b => b.classList.toggle('on', b.dataset.tab === G.dossierTab));
}
document.querySelectorAll('#dossier-tabs button').forEach(b => b.onclick = () => { G.dossierTab = b.dataset.tab; renderDossierTabs(); const d = G.dossiers[G.dossierKG]; if (d && d !== 'loading' && !d.error) renderDossier(d); });
document.getElementById('dossier-popup-close').onclick = () => { document.getElementById('dossier-popup').classList.remove('open'); resetPopupPosition('dossier-popup'); };
document.getElementById('station-popup-close').onclick = () => { document.getElementById('station-popup').classList.remove('open'); resetPopupPosition('station-popup'); };

function ppRows(rows) { return '<div class="pp-grid">' + rows.map(([k, v]) => '<span>' + k + '</span><b>' + v + '</b>').join('') + '</div>'; }
/** ≤24 pixel bars. vals:[{v,label,cls}], max auto. */
function pxChart(vals, opts) {
  opts = opts || {};
  const n = Math.min(24, vals.length), arr = vals.slice(-n);
  const max = Math.max(1e-9, ...arr.map(x => Math.abs(x.v || 0)));
  let h = '<div class="px-chart">';
  for (const x of arr) {
    const pct = Math.max(2, Math.round(Math.abs(x.v || 0) / max * 100));
    h += '<i class="' + (x.cls || '') + '" style="height:' + pct + '%" title="' + esc(x.label + ': ' + (opts.fmt ? opts.fmt(x.v) : fmtNum(x.v, 1))) + '"></i>';
  }
  h += '</div><div class="px-axis"><span>' + esc(String(arr[0].label)) + '</span>' + (opts.mid ? '<span>' + esc(opts.mid) + '</span>' : '') + '<span>' + esc(String(arr[arr.length - 1].label)) + '</span></div>';
  return h;
}
function segBar(parts) {   // [{f, color, name}]
  let seg = '', leg = '';
  for (const p of parts) { if (p.f < 0.01) continue; seg += '<i style="width:' + (p.f * 100).toFixed(1) + '%;background:' + p.color + '"></i>'; leg += '<em><i style="background:' + p.color + '"></i>' + esc(p.name) + ' ' + Math.round(p.f * 100) + '%</em>'; }
  return '<div class="fracs-bar">' + seg + '</div><div class="fracs-legend">' + leg + '</div>';
}

function renderDossier(d) {
  const body = document.getElementById('dossier-body');
  const tab = G.dossierTab, g = d.game || {};
  let html = '';
  if (tab === 'water') {
    const w = d.water, dr = d.drought || {}, now = (w && w.now) || {};
    if (!w) html += '<div class="kg-loading">' + tr('Keine Grundwasserdaten für diese Gemeinde.') + '</div>';
    else {
      const st = WATER_STATUS[now.status];
      const cat = GWI_CAT[w.gwi_category] || ['?', '#888'];
      const rows = [];
      rows.push(['💧 ' + tr('Heute'), st ? '<span class="' + st.cls.replace('st-', 'wc-') + '" style="color:inherit">' + tr(st.de) + (now.sigma != null ? ' · ' + fmtSigma(now.sigma) : '') + '</span>' + (now.trend_30d_cm != null ? ' <span class="kg-dim">' + (now.trend_30d_cm > 0 ? '↗' : now.trend_30d_cm < 0 ? '↘' : '→') + ' ' + Math.abs(now.trend_30d_cm) + ' cm/30 d</span>' : '') : '<span class="kg-dim">' + tr('keine Live-Messung') + '</span>']);
      rows.push(['🧭 ' + tr('Wasserstress'), '<span style="color:' + cat[1] + '">' + tr(cat[0]) + '</span> <span class="kg-dim">GWI ' + (w.gwi != null ? w.gwi.toFixed(2) : '–') + (w.gw_trend_m_decade != null ? ' · ' + (w.gw_trend_m_decade > 0 ? '+' : '') + w.gw_trend_m_decade.toFixed(2) + ' m/10 J' : '') + '</span>']);
      if (w.no3_mg_l != null) rows.push(['🧪 Nitrat', fmtNum(w.no3_mg_l, 1) + ' mg/l <span class="kg-dim">' + (w.no3_mg_l >= 50 ? tr('über Grenzwert') : w.no3_mg_l >= 25 ? tr('erhöht') : tr('unauffällig')) + '</span>']);
      if (dr.p_drought_year != null) rows.push(['☀️ ' + tr('Dürre-Risiko'), fmtPct(dr.p_drought_year) + ' <span class="kg-dim">' + tr('der Jahre') + (w.drought && w.drought.worst_year ? ' · ' + tr('schlimmstes') + ' ' + w.drought.worst_year : '') + '</span>']);
      if (w.stations != null) rows.push(['📏 ' + tr('Messstellen'), w.stations + (w.gw_body ? ' <span class="kg-dim">· ' + esc(w.gw_body) + '</span>' : '')]);
      html += ppRows(rows);
      // Season calendar
      const sp = (w.drought && w.drought.season_profile) || [];
      if (sp.length === 12) {
        const m = new Date().getMonth();
        html += '<div class="ds-title"><span>' + tr('Dürre-Kalender') + '</span><span>' + tr('Ø Klasse pro Monat') + '</span></div><div class="season-cal">';
        for (let i = 0; i < 12; i++) { const c = Math.min(3, Math.max(0, Math.round(sp[i]))); html += '<i class="' + (i === m ? 'now' : '') + '" style="background:' + CDI_COLORS[c] + '" title="' + esc(MONTHS_DE[i] + ': ' + sp[i].toFixed(2)) + '">' + MONTHS_DE[i] + '</i>'; }
        html += '</div>';
      }
      const hist = w.history || [];
      if (hist.length >= 3) {
        html += '<div class="ds-title"><span>' + tr('Trockenheit') + ' (CDI)</span><span>' + hist[0].year + '–' + hist[hist.length - 1].year + '</span></div>';
        html += pxChart(hist.map(h => ({ v: h.cdi, label: h.year, cls: h.cdi >= 1.5 ? 'bad' : h.cdi >= 1 ? 'warn' : '' })), { fmt: v => v.toFixed(2) });
      }
      html += '<div class="ds-rule">🎮 ' + tr('Ernte heute') + ' <b>×' + (g.yield_factor != null ? g.yield_factor.toFixed(1).replace('.', ',') : '1,0') + '</b> · 🕳️ ' + tr('Brunnen schützt') + ' <b>' + fmtPct(g.well_protection || 0) + '</b>' + (dr.level >= 1 ? '<br>' + tr('Dürre: Felder tragen weniger — Brunnen und Brache lohnen sich.') : '') + '</div>';
      html += '<div class="ds-actions"><button class="btn btn-secondary btn-small" onclick="startFlow()">💧 ' + tr('Weg des Wassers') + '</button>' +
        '<label><input type="checkbox" id="ds-gw-toggle" ' + (G.gwVisible ? 'checked' : '') + ' onchange="setGwVisible(this.checked)"> 📏 ' + tr('Messstellen') + '</label></div>';
      html += '<div class="ds-src">groundwater-at · eHYD, Copernicus EDO, WISE · CC BY 4.0' + (now.as_of ? ' · ' + now.as_of : '') + '</div>';
    }
  } else if (tab === 'forest') {
    const f = d.forest;
    if (!f) html += '<div class="kg-loading">' + tr('Keine Walddaten für diese Gemeinde.') + '</div>';
    else {
      const rows = [];
      if (f.forest_area_ha != null) rows.push(['🌲 ' + tr('Waldfläche'), fmtHa(f.forest_area_ha)]);
      rows.push(['🪓 ' + tr('Verlust'), (f.loss_ha_latest != null ? fmtNum(f.loss_ha_latest, 1) + ' ha ' + tr('letztes Jahr') : '–') + (f.loss_total_ha != null ? ' <span class="kg-dim">· ' + fmtNum(f.loss_total_ha, 0) + ' ha ' + tr('seit 2001') + '</span>' : '')]);
      if (f.harvest_efm != null) rows.push(['🪵 ' + tr('Ernte'), fmtNum(f.harvest_efm) + ' Efm' + (f.harvest_value_eur ? ' <span class="kg-dim">≈ ' + fmtEur(f.harvest_value_eur) + '</span>' : '')]);
      if (f.co2_t != null) rows.push(['🌍 CO₂', fmtNum(f.co2_t) + ' t ' + tr('gebunden/Jahr') + (f.net_flux_tco2e_ha != null ? ' <span class="kg-dim">(' + fmtNum(f.net_flux_tco2e_ha, 0) + ' t/ha)</span>' : '')]);
      if (f.price_spruce) rows.push(['💶 ' + tr('Fichte'), f.price_spruce + ' €/Fm' + (f.state ? ' <span class="kg-dim">' + esc(f.state) + '</span>' : '')]);
      html += ppRows(rows);
      const hist = f.history || [];
      if (hist.length >= 3) {
        const mx = Math.max(...hist.map(h => h.loss_ha || 0));
        html += '<div class="ds-title"><span>' + tr('Waldverlust') + ' ha/Jahr</span><span>' + hist[0].year + '–' + hist[hist.length - 1].year + '</span></div>';
        html += pxChart(hist.map(h => ({ v: h.loss_ha || 0, label: h.year, cls: h.loss_ha >= mx * 0.75 ? 'bad' : h.loss_ha >= mx * 0.4 ? 'warn' : '' })), { fmt: v => fmtNum(v, 1) + ' ha' });
      }
      html += '<div class="ds-rule">🎮 🌳 ' + tr('Naturwald') + ': <b>120⚡ + Holzvorrat/10</b> — ' + tr('je dichter der Bestand, desto mehr XP; zählt zum 30-%-Ziel.') + '</div>';
      html += '<div class="ds-src">holzeinschlag-at · Hansen GFC, BFW, Statistik Austria · CC BY 4.0' + (f.as_of ? ' · ' + f.as_of : '') + '</div>';
    }
  } else {
    const fm = d.farm;
    if (!fm) html += '<div class="kg-loading">' + tr('Keine Förderdaten für diese Gemeinde.') + '</div>';
    else {
      const rows = [];
      if (fm.recipients_n != null) rows.push(['🚜 ' + tr('Betriebe'), fmtNum(fm.recipients_n) + (fm.eligible_ha_total ? ' <span class="kg-dim">· ' + fmtHa(fm.eligible_ha_total) + '</span>' : '')]);
      if (fm.total_eur != null) rows.push(['💶 ' + tr('Förderung'), fmtEur(fm.total_eur) + (fm.eur_per_recipient_median ? ' <span class="kg-dim">· Ø ' + fmtNum(fm.eur_per_recipient_median) + ' €/Hof</span>' : '')]);
      if (fm.eur_per_ha_median != null) rows.push(['📐 ' + tr('Median'), fmtNum(fm.eur_per_ha_median) + ' €/ha']);
      if (fm.organic_share != null) rows.push(['🌿 Bio', fmtPct(fm.organic_share)]);
      if (fm.mountain_share != null) rows.push(['⛰️ ' + tr('Bergbauern'), fmtPct(fm.mountain_share)]);
      html += ppRows(rows);
      const mix = fm.archetype_mix || {};
      const AR = { bergbauer: ['Bergbauer', '#7a9a5a'], bio_bergbauer: ['Bio-Bergbauer', '#5ad06a'], bio: ['Bio', '#3fb850'], ackerbau: ['Ackerbau', '#d8b04a'], gruenland: ['Grünland', '#4a9848'], vieh: ['Viehhaltung', '#b07050'], wein: ['Wein', '#a050a0'], obst: ['Obst', '#e07040'], ohne_flaeche: ['ohne Fläche', '#888'], sonst: ['sonstige', '#777'] };
      const parts = Object.entries(mix).sort((a, b) => b[1] - a[1]).map(([k, f]) => ({ f, name: (AR[k] || [k.replace(/_/g, ' ')])[0], color: (AR[k] || [0, '#' + (simpleHash(k) & 0xffffff).toString(16).padStart(6, '0')])[1] }));
      if (parts.length) html += '<div class="ds-title"><span>' + tr('Hoftypen') + '</span></div>' + segBar(parts);
      const tm = (fm.top_measures || []).slice(0, 3);
      if (tm.length) html += '<div class="ds-title"><span>' + tr('Top-Maßnahmen') + '</span></div><div class="pp-grid" style="font-size:16px">' + tm.map(m => '<span>' + Math.round(m.share * 100) + ' %</span><b>' + esc(String(m.name).replace(/\s*\(Artikel.*$/, '').slice(0, 60)) + '</b>').join('') + '</div>';
      const hist = fm.history || [];
      if (hist.length >= 2) html += '<div class="ds-title"><span>' + tr('Förderung') + ' €/Jahr</span></div>' + pxChart(hist.map(h => ({ v: h.total_eur, label: h.year, cls: h.year === fm.as_of ? 'cur' : '' })), { fmt: fmtEur });
      html += '<div class="ds-rule">🎮 🏛 ' + tr('Förderung') + ': <b>' + fmtNum(g.subsidy_per_ha || 0, 1) + '🪙/ha</b> ' + tr('pro Ernte') + ' — ' + tr('Wiesen holen sie alle 40 Minuten ab, Bio-Schläge kriegen mehr.') + '</div>';
      html += '<div class="ds-src">farm-subsidies · AMA Transparenzdatenbank ' + (fm.as_of || '') + ' · CC BY 4.0</div>';
    }
  }
  body.innerHTML = html;
}
