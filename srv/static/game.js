// ============================================================
//  SIEDLER ÖSTERREICH — Game Engine
//  Isometric Settlers IV style
// ============================================================
'use strict';

const CAD = '/api/cadastre';

// ---- Colors inspired by Settlers IV ----
const TERRAIN = {
  grass:    ['#4a8a30','#528e35','#5a9238','#4e8c32','#468828'],
  forest:   ['#1e5a1e','#245e22','#2a6228','#1c581c','#286026'],
  water:    ['#2878b8','#3080c0','#2570a8','#3888c8','#2068a0'],
  farm:     ['#a8a040','#b0a848','#a09838','#b8b050','#989030'],
  meadow:   ['#5a9e3a','#62a240','#52963a','#6aaa48','#4e9234'],
  building: ['#c8b040','#d0b848','#bca838','#d8c050','#b4a030'],
  road:     ['#484848','#505050','#444444','#525252','#404040'],
  garden:   ['#6b8e4a','#739650','#638644','#7b9e58','#5b7e3e'],
  wetland:  ['#3a7a5a','#428260','#327254','#4a8a68','#2a6a4e'],
  waste:    ['#5a5848','#625e50','#525040','#6a6658','#4a4838'],
  bio:      ['#2aaa4a','#32b252','#22a242','#3aba5a','#1a9a3a'],
};

const LANDUSE_TERRAIN = {
  '40':TERRAIN.garden,'41':TERRAIN.road,'42':TERRAIN.building,'43':TERRAIN.building,
  '44':TERRAIN.waste,'45':TERRAIN.garden,'46':TERRAIN.garden,
  '48':TERRAIN.road,'49':TERRAIN.road,
  '50':TERRAIN.farm,'51':TERRAIN.farm,'52':TERRAIN.meadow,'53':TERRAIN.meadow,'54':TERRAIN.meadow,
  '55':TERRAIN.meadow,'56':TERRAIN.forest,'57':TERRAIN.forest,'58':TERRAIN.forest,
  '60':TERRAIN.garden,'61':TERRAIN.garden,'62':TERRAIN.garden,'63':TERRAIN.garden,
  '70':TERRAIN.water,'71':TERRAIN.water,'72':TERRAIN.water,'73':TERRAIN.water,
  '80':TERRAIN.waste,'81':TERRAIN.waste,'83':TERRAIN.wetland,'84':TERRAIN.water,
  '85':TERRAIN.waste,'90':TERRAIN.road,'91':TERRAIN.waste,'97':TERRAIN.waste,
};

const LANDUSE_NAMES = {
  '40':'Baugrün','41':'Baufläche','42':'Gebäude','43':'Keller','44':'Ruine','45':'Gewächshaus',
  '48':'Verkehr','50':'Acker','51':'Acker','52':'Wiese','53':'Weide','54':'Grünland',
  '55':'Alpe','56':'Wald','57':'Krummholz','58':'Wald','60':'Weingarten','62':'Garten',
  '63':'Obstgarten','70':'Gewässer','71':'Bach','72':'See','73':'Fluss',
  '80':'Ödland','83':'Sumpf','84':'Gletscher','85':'Fels',
};

// Map abbreviations from landuse_summary keys (e.g. "B(bf)") to terrain type + numeric code
const ABBR_MAP = {
  'B(bf)':  {terrain:TERRAIN.building, code:'41', name:'Baufläche'},
  'B(Geb)': {terrain:TERRAIN.building, code:'42', name:'Gebäude'},
  'B(Ga)':  {terrain:TERRAIN.garden,   code:'62', name:'Garten'},
  'B(Ghs)': {terrain:TERRAIN.building, code:'45', name:'Gewächshaus'},
  'B(Ke)':  {terrain:TERRAIN.building, code:'43', name:'Keller'},
  'B(Ru)':  {terrain:TERRAIN.waste,    code:'44', name:'Ruine'},
  'LN(W)':  {terrain:TERRAIN.meadow,   code:'52', name:'Wiese'},
  'LN(A)':  {terrain:TERRAIN.farm,     code:'50', name:'Acker'},
  'LN(Hu)': {terrain:TERRAIN.meadow,   code:'53', name:'Weide'},
  'LN(EW)': {terrain:TERRAIN.meadow,   code:'54', name:'Grünland'},
  'LN':     {terrain:TERRAIN.meadow,   code:'52', name:'Grünland'},
  'W':      {terrain:TERRAIN.forest,   code:'56', name:'Wald'},
  'Alpe':   {terrain:TERRAIN.meadow,   code:'55', name:'Alpe'},
  'V(Str)': {terrain:TERRAIN.road,     code:'48', name:'Straße'},
  'V(Weg)': {terrain:TERRAIN.road,     code:'48', name:'Weg'},
  'V(Pl)':  {terrain:TERRAIN.road,     code:'48', name:'Platz'},
  'V(Bahn)':{terrain:TERRAIN.road,     code:'48', name:'Bahn'},
  'V(Brü)': {terrain:TERRAIN.road,     code:'48', name:'Brücke'},
  'Ga':     {terrain:TERRAIN.garden,   code:'62', name:'Garten'},
  'WG':     {terrain:TERRAIN.garden,   code:'60', name:'Weingarten'},
  'Ob':     {terrain:TERRAIN.garden,   code:'63', name:'Obstgarten'},
  'So':     {terrain:TERRAIN.waste,    code:'80', name:'Sonstige'},
  'Q':      {terrain:TERRAIN.water,    code:'70', name:'Quelle'},
  'Fl(St)': {terrain:TERRAIN.water,    code:'73', name:'Fluss'},
  'Fl(B)':  {terrain:TERRAIN.water,    code:'71', name:'Bach'},
  'See':    {terrain:TERRAIN.water,    code:'72', name:'See'},
  'Fe':     {terrain:TERRAIN.waste,    code:'85', name:'Fels'},
  'Moor':   {terrain:TERRAIN.wetland,  code:'83', name:'Moor'},
  'Bio':    {terrain:TERRAIN.bio,      code:'52', name:'Naturschutz'},
};

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
      // Fuzzy match: try prefix
      for (const [k, v] of Object.entries(ABBR_MAP)) {
        if (abbr.startsWith(k) || abbr.includes(k)) { info = v; break; }
      }
    }
    if (!info) {
      // Guess from the full description text
      const t = key.toLowerCase();
      if (t.includes('wald') || t.includes(' w ')) info = ABBR_MAP['W'];
      else if (t.includes('wiese')) info = ABBR_MAP['LN(W)'];
      else if (t.includes('acker')) info = ABBR_MAP['LN(A)'];
      else if (t.includes('baufläche') || t.includes('gebäude')) info = ABBR_MAP['B(bf)'];
      else if (t.includes('garten')) info = ABBR_MAP['Ga'];
      else if (t.includes('verkehr') || t.includes('straß')) info = ABBR_MAP['V(Str)'];
      else if (t.includes('gewässer') || t.includes('bach') || t.includes('see')) info = ABBR_MAP['Q'];
      else if (t.includes('alpe') || t.includes('alm')) info = ABBR_MAP['Alpe'];
      else if (t.includes('sumpf') || t.includes('moor')) info = ABBR_MAP['Moor'];
      else if (t.includes('fels') || t.includes('geröll')) info = ABBR_MAP['Fe'];
      else info = {terrain:TERRAIN.grass, code:'', name:abbr};
    }
    entries.push({abbr, terrain:info.terrain, code:info.code, name:info.name, count});
    if (info.code === '41' || info.code === '42' || info.code === '43' || info.code === '45') {
      buildingCount += count;
    }
  }
  // Dominant = highest count
  let dominant = entries.length > 0 ? entries[0] : null;
  for (const e of entries) { if (!dominant || e.count > dominant.count) dominant = e; }
  return {dominant, buildingCount, entries};
}

const PLAYER_COLORS = ['#e04040','#4080e0','#e0c040','#a040e0','#40e0a0','#e08040','#e040a0','#40e040'];

// ---- Game State ----
const G = {
  player: null, session: null,
  parcels: [],          // from cadastre (point data)
  parcelPolys: [],      // from export/geojson (polygon data for current KGs)
  buildingFootprints: [], // real building footprint polygons from cadastre
  landusePolys: [],     // real landuse polygons (forests, roads, water, etc.)
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
  enhancedGemeinden: [],    // [{gemeinde_code, gemeinde_name, lon, lat}] deduped
  enhancedLoaded: new Set(),// kg_codes whose enhanced data has been fetched
  lidarParcels: {},         // parcel_id → {elev, elevMin, elevMax, slope, aspect, tclass, dom, forestFrac}
  lidarKGTerrain: {},       // kg_code → {emin, emax, tclass}
  lidarBuildingIdx: {},     // grid key → [{lon,lat,stories,roof,h}] for footprint matching
  topTrees: {},             // kg_code → [{height_m, lon, lat}] (flag-filtered server-side)
  topObjects: {},           // kg_code → [{type, height_m, lon, lat}]
  osmLines: {},             // kg_code → [{cat, fclass, major, name, pts:Float64Array}]
  n2kSites: {},             // sitecode → {name, habitats, label, geom (GeoJSON), loaded}
  n2kVisible: true,         // layer toggle
  landPrices: {},           // parcel_id → price estimate object (lazy)
  osmProx: {},              // parcel_id → OSM proximity object (lazy, null = failed/loading)
  similar: null,            // {refPid, refLon, refLat, data} — active similar-parcels overlay
  similarCache: {},         // "pid:radius" → /api/similar response (client cache)
  similarRadius: 5000,      // selected search radius in m (5/10/20/50 km)
  geo: { watching:false, lon:0, lat:0, acc:0, follow:false, id:null },
  tallUnlocked: false,      // giant trees unlock after first treasure collected
  tallRevealed: false,      // tapping the hint tree reveals all giant trees
  tallRevealAt: 0,          // timestamp for pop-in animation
  devTree: null,            // giant tree unlocked via 5-tap "developer mode" on the enhanced badge
  lidarGen: 0,              // bumped when new lidar building data arrives (invalidates footprint matches)
};

// ---- Helpers ----
async function api(method, url, body) {
  const opts = { method, headers: {'Content-Type':'application/json'} };
  if (body) opts.body = JSON.stringify(body);
  const r = await fetch(url, opts);
  return r.json();
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
function randomName() {
  const a = _ADJ[Math.floor(Math.random()*_ADJ.length)];
  const n = _NOUN[Math.floor(Math.random()*_NOUN.length)];
  return a + n;
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
        banner.innerHTML = `⚔️ In <b style="color:var(--gold)">${esc(creatorName)}s</b> Spiel` +
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

  // Pre-fill name: from URL or random suggestion
  inp.value = savedName || '';
  document.getElementById('btn-reroll').onclick = () => { inp.value = randomName(); inp.focus(); };

  if (savedPid && savedName) {
    document.getElementById('quick-rejoin').innerHTML =
      `Zuletzt als <b style="color:var(--gold)">${esc(savedName)}</b> gespielt — <a onclick="quickLogin()">Weiter ▸</a>`;
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
    if (!name || name.length < 2) { name = randomName(); inp.value = name; }
    const res = await POST('/api/register', {name});
    if (res.error) { err.textContent=res.error; return null; }
    savePlayer(res.player);
    setUrlParams({rejoin: res.rejoin_url ? res.rejoin_url.split('/').pop() : null});
    toast('🎉 Willkommen, ' + res.player.name + '!', 'ok');
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
        const g = G.enhancedGemeinden[Math.floor(Math.random() * G.enhancedGemeinden.length)];
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

// ================= MUNICIPALITY PICKER (Real GADM outlines) =================
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

  // Search
  const inp = document.getElementById('input-search');
  const dd = document.getElementById('search-results');
  let timer;
  inp.addEventListener('input', () => {
    clearTimeout(timer);
    const q = inp.value.trim();
    if (q.length < 2) { dd.classList.remove('open'); return; }
    timer = setTimeout(async () => {
      // Try municipality lookup first, then address
      const res = await GET(CAD+'/lookup?q='+encodeURIComponent(q)+'&type=gemeinde&limit=10');
      const items = res.data || [];
      if (!items.length) {
        // Try address
        const addr = await GET(CAD+'/search/address_osm?q='+encodeURIComponent(q)+'&limit=5');
        const addrItems = addr.data || [];
        dd.innerHTML = addrItems.map(a =>
          `<div class="search-item" data-lon="${a.lon}" data-lat="${a.lat}" data-name="${esc(a.display_name)}">
            ${esc(a.display_name)}<br><small>${a.address?.municipality||''}</small></div>`
        ).join('') || '<div class="search-item">Keine Ergebnisse</div>';
      } else {
        dd.innerHTML = items.map(m =>
          `<div class="search-item" data-code="${m.code||m.gemeinde_code}" data-name="${esc(m.name||m.gemeinde_name)}">
            ${esc(m.name||m.gemeinde_name)}<br><small>${m.gemeinde_name!==m.name?m.gemeinde_name+' · ':'' }${m.code||m.gemeinde_code}</small></div>`
        ).join('');
      }
      dd.classList.add('open');
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
    }, 350);
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
    ctx.font = '14px VT323';
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
  GET('/api/player/'+G.player.id).then(pl => {
    if (pl && pl.treasures_found > 0) { G.tallUnlocked = true; }
  }).catch(()=>{});
  // If parcels were empty (bbox failed) but we loaded polygon data, synthesize point parcels
  if (G.parcels.length === 0 && G.parcelPolys.length > 0) {
    for (const f of G.parcelPolys) {
      const p = f.properties;
      const c = polyCentroid(f.geometry.coordinates[0]);
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
  connectSSE();

  setLoadStep('ls-ready', 'done');
  setLoadProgress(100);
  setLoadSub('✅ Bereit — Viel Spaß beim Siedeln!');

  // Brief minimum so the loading screen doesn't flash (was 4s — now get in fast)
  const elapsed = Date.now() - (G.loadStart || 0);
  const minWait = Math.max(300, 1500 - elapsed);
  await new Promise(r => setTimeout(r, minWait));
  stopTipRotation();
  stopLoadingCountdown();
  show('game');

  // Init canvas AFTER showing the game screen (so clientWidth/Height > 0)
  gc = document.getElementById('game-canvas');
  gctx = gc.getContext('2d');
  mc = document.getElementById('mini-canvas');
  mctx = mc.getContext('2d');
  document.getElementById('game-title').textContent = G.session.municipality_name;
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
  if ((b.e-b.w) > 0.04) return;
  try {
    const url = CAD+'/spatial/bbox?west='+b.w+'&south='+b.s+'&east='+b.e+'&north='+b.n+'&layers=parcels&limit=800&format=geojson';
    const data = await GET(url);
    const feats = data.features || (data.data?.parcels||[]).map(p=>({type:'Feature',properties:p,geometry:{type:'Point',coordinates:[p.lon,p.lat]}}));
    const ids = new Set(G.parcels.map(f=>f.properties.parcel_id));
    let added = 0;
    for (const f of feats) { if (!ids.has(f.properties.parcel_id)) { G.parcels.push(f); added++; } }
    if (added > 0) { render(); renderMini(); }
    // Also fetch polygon data for any new KGs
    fetchKGPolygons().then(() => buildEZIndex());
    // Check for adjacent municipality crossings
    detectAdjacentMunicipalities();
    checkViewportMunicipality();
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
    if (data.features) for (const f of data.features) features.push(f);
    if (!data.has_more) break;
    page++;
  }
  return features;
}

/** Stream the landuse backdrop for a KG in the background (never blocks loading).
 *  Landuse is only a visual backdrop — in enhanced mode terrain comes from lidar
 *  dominant-type and roads/water from OSM, so it need not gate the game.
 *
 *  For ENHANCED (lidar) KGs we skip the cadastre landuse layer entirely: it's the
 *  single heaviest payload (~7MB vs ~0.85MB parcels / ~2MB footprints), and srtm's
 *  per-parcel dominant_type land cover + OSM road/water lines already provide a
 *  richer, measured backdrop. This is the biggest load-time win. */
function loadLanduseBackground(kg) {
  if (G.enhancedKGs.has(kg)) return; // lidar dom + OSM cover the backdrop — skip the 7MB fetch
  if (!G.landuseKGs) G.landuseKGs = new Set();
  if (G.landuseKGs.has(kg)) return; // already streamed
  G.landuseKGs.add(kg);
  fetchKGLayer(kg, 'landuse').then(landuse => {
    let added = 0;
    for (const f of landuse) {
      if (f.geometry?.type === 'Polygon' || f.geometry?.type === 'MultiPolygon') {
        G.landusePolys.push(f); added++;
      }
    }
    if (added > 0) { render(); renderMini(); }
  }).catch(e => console.error('landuse bg fetch failed:', kg, e));
}

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
  if (!opts.force && G.vpTiles.has(tileKey)) return 0;
  G.vpTiles.add(tileKey);
  let data;
  try {
    data = await GET('/api/viewport?west='+b.w+'&south='+b.s+'&east='+b.e+'&north='+b.n+'&limit='+(opts.limit||6000));
  } catch(e) { console.error('viewport fetch failed', e); G.vpTiles.delete(tileKey); return 0; }
  if (!data) { G.vpTiles.delete(tileKey); return 0; }
  // If upstream wasn't fully warm yet, allow a later re-fetch of this tile.
  if (data.ready === false) G.vpTiles.delete(tileKey);

  let addedP = 0;
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
      // Non-enhanced KGs still get their landuse polygon backdrop streamed in
      // (viewport endpoint carries parcels+footprints only). Enhanced KGs skip it.
      if (!G.enhancedKGs.has(props.kg_code)) loadLanduseBackground(props.kg_code);
    }
  }
  for (const it of (data.footprints||[])) {
    const id = it.footprint_id;
    if (!id || G.fpIds.has(id) || !it.geometry) continue;
    G.fpIds.add(id);
    const { geometry, ...props } = it;
    G.buildingFootprints.push({ type:'Feature', properties: props, geometry });
  }
  return addedP;
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
  const added = await loadViewportGeometry(box, { force: true });
  setLoadProgress(70);
  setLoadSub(`${G.parcelPolys.length} Parzellen, ${G.buildingFootprints.length} Gebäude geladen`);

  // Kick enhanced (lidar/OSM/N2K) fetches for any enhanced KGs now on screen.
  loadEnhancedForKGs();

  // Background: widen ~2.5× so panning outward is already primed.
  const wide = { w: G.cam.lon - rad*2.5, s: G.cam.lat - rad*1.8, e: G.cam.lon + rad*2.5, n: G.cam.lat + rad*1.8 };
  loadViewportGeometry(wide).then(a => {
    if (a > 0) { buildEZIndex(); loadEnhancedForKGs(); render(); renderMini(); }
  }).catch(()=>{});
}

async function fetchKGPolygons() {
  // Incremental viewport load on pan/zoom. Grabs polygon geometry for the current
  // view (padded) from the /api/viewport fast path; the tile-dedup in
  // loadViewportGeometry keeps repeat pans cheap.
  const b = viewBounds();
  const padX = (b.e - b.w) * 0.3, padY = (b.n - b.s) * 0.3;
  const box = { w: b.w - padX, s: b.s - padY, e: b.e + padX, n: b.n + padY };
  const added = await loadViewportGeometry(box);
  if (added > 0) { render(); renderMini(); }
  // Enhanced data for any newly-visible enhanced KGs.
  loadEnhancedForKGs();
}

// ================= ENHANCED MODE (lidar terrain, OSM lines, Natura 2000) =================

/** Fetch the enhanced-KG registry (lidar-processed KGs). Cached server-side 15min; refreshed client-side every 10min. */
async function loadEnhancedRegistry() {
  try {
    const res = await GET('/api/enhanced-kgs');
    if (!res || !res.kgs) return;
    G.enhancedKGs = new Set(res.kgs.map(k => k.kg_code));
    const byGem = {};
    for (const k of res.kgs) {
      if (!byGem[k.gemeinde_code]) byGem[k.gemeinde_code] = { gemeinde_code: k.gemeinde_code, gemeinde_name: k.gemeinde_name, lon: k.lon, lat: k.lat };
    }
    G.enhancedGemeinden = Object.values(byGem);
  } catch(e) { console.error('enhanced registry failed:', e); }
}
setInterval(loadEnhancedRegistry, 10*60*1000);

/** Kick off background enhanced-data fetches for loaded KGs that are lidar-processed. Never blocks. */
function loadEnhancedForKGs() {
  for (const kg of G.kgsLoaded) {
    if (G.enhancedLoaded.has(kg)) continue;
    if (!G.enhancedKGs.has(kg)) continue;
    G.enhancedLoaded.add(kg);
    fetchEnhancedKG(kg); // fire & forget
  }
}

async function fetchEnhancedKG(kg) {
  // 1. LiDAR slim KG data (terrain, buildings, top trees/objects — flags already applied server-side)
  GET('/api/lidar/kg/'+kg).then(d => {
    if (!d || d.error) return;
    if (d.terrain) G.lidarKGTerrain[kg] = { emin: d.terrain.elevation_min_m, emax: d.terrain.elevation_max_m, tclass: d.terrain.terrain_class };
    for (const p of (d.parcels||[])) {
      G.lidarParcels[p.parcel_id] = {
        elev: p.elevation_m, elevMin: p.elevation_min_m, elevMax: p.elevation_max_m,
        slope: p.slope_mean_deg, aspect: p.aspect_dominant, tclass: p.terrain_class,
        dom: p.dominant_type, domTerrain: p.dom_terrain, forestFrac: p.forested_fraction,
        fracs: p.fracs, kg: kg,
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

  // 2. OSM roads/water/rail lines
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
  if (!f || f.geometry?.type !== 'Polygon') return {count:0, maxH:0};
  const ring = f.geometry.coordinates[0];
  let count = 0, maxH = 0;
  for (const t of allTallTrees()) {
    if (pip(t.lon, t.lat, ring)) { count++; if (t.height_m > maxH) maxH = t.height_m; }
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
  const hasRoadLU = (parsed.entries || []).some(e => e.terrain === TERRAIN.road || e.code === '48' || e.code === '90');
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
function camOverEnhancedKG() {
  for (const kg of G.kgsLoaded) {
    if (!G.enhancedKGs.has(kg) || !G.lidarKGTerrain[kg]) continue;
    return true; // approximation: any loaded+enhanced KG data present
  }
  return false;
}

function updateEnhancedBadge() {
  const el = document.getElementById('enhanced-badge');
  if (!el) return;
  el.style.display = camOverEnhancedKG() ? '' : 'none';
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
  const msgs = await GET('/api/session/'+G.session.id+'/chat?limit=50') || [];
  G.chatMsgs = msgs.reverse ? msgs.reverse() : msgs;
  renderChat();
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
function renderQuests() {
  document.getElementById('quest-list').innerHTML = (G.challenges||[]).map(c => {
    const icon = {explore:'🗺️',restore:'🌿',treasure:'💎'}[c.challenge_type]||'📜';
    return `<div class="quest-item" onclick="tryCompleteQuest(${c.id})">
      <div class="qt">${icon} ${esc(c.title)}</div>
      <div class="qd">${esc(c.description||'')}</div>
      <div class="qr">+${c.reward_coins}🪙 +${c.reward_xp}⚡</div></div>`;
  }).join('') || '<div style="font:16px VT323;color:var(--text-dim)">Alle erledigt!</div>';
}
function renderChat() {
  const el = document.getElementById('chat-log');
  el.innerHTML = G.chatMsgs.map(m => `<div class="chat-msg"><span class="cn">${esc(m.player_name||'?')}:</span> ${esc(m.message)}</div>`).join('');
  el.scrollTop = el.scrollHeight;
}

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
  await POST('/api/session/'+G.session.id+'/chat', {player_id:G.player.id, message:msg});
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
    case 'chat': G.chatMsgs.push({player_name:d.player,message:d.message}); renderChat(); break;
    case 'player_joined': toast('⚔️ '+d.player.name+' beigetreten!','ok'); loadPlayers(); break;
    case 'parcel_claimed': toast('🏴 '+d.player+' → '+d.parcel_id,''); loadClaimed().then(()=>render()); break;
    case 'parcel_converted': toast('🌿 '+d.player+' → '+d.convert_to,'ok'); loadClaimed().then(()=>{render();loadBio();}); break;
    case 'parcel_sold': toast('💰 '+d.player+' verkauft',''); loadClaimed().then(()=>render()); break;
    case 'ez_claimed': toast('\u{1f4cb} '+d.player+' → EZ '+d.ez+' ('+d.count+' Parzellen)',''); loadClaimed().then(()=>render()); break;
    case 'challenge_completed': toast('🏆 '+d.player+' Aufgabe!','ok'); break;
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

function render() {
  if (!gctx) return;
  const ctx = gctx;
  const W = gc.width, H = gc.height;

  // ---- Background terrain ----
  ctx.fillStyle = '#3a6828';
  ctx.fillRect(0, 0, W, H);
  drawGrassTexture(ctx, W, H);

  // Build claim lookup
  const claimMap = {};
  for (const c of G.claimed) claimMap[c.parcel_id] = c;

  // ---- Draw real landuse polygons (forests, water, roads, etc.) ----
  if (G.landusePolys.length > 0) drawLandusePolygons(ctx);

  // ---- Natura 2000 protected-area overlay (enhanced mode) ----
  if (G.n2kVisible) drawN2KOverlay(ctx);

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

  // ---- Tallest-tree + landmark markers (enhanced mode) ----
  drawTopLandmarks(ctx);

  // ---- Similar-parcels overlay (below treasures, above parcels) ----
  if (G.similar) drawSimilarParcels(ctx);

  // ---- Treasures ----
  for (const t of G.treasures) drawTreasure(ctx, t);

  // ---- GPS position marker ----
  if (G.geo.watching) drawGeoMarker(ctx);

  // ---- EZ group highlight (all parcels in same EZ) ----
  if (G.ezHighlight) drawEZHighlight(ctx);

  // ---- Selected parcel highlight ----
  if (G.sel) drawSelection(ctx, G.sel);

  // Scale bar
  drawScaleBar(ctx, W, H);
}

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
// Map landuse_code to fill colors (Settlers-style terrain)
const LANDUSE_POLY_COLORS = {
  '40': {fill:'#c8b040', stroke:'#a89830'},     // Baufläche begrünt — yellow
  '41': {fill:'#d0b848', stroke:'#b09828'},     // Baufläche — yellow
  '42': {fill:'#c0a838', stroke:'#a09028'},     // Gebäude — yellow
  '43': {fill:'#b8a030', stroke:'#988820'},     // Keller — yellow
  '48': {fill:'#404040', stroke:'#303030', a:0.8},  // Verkehr (roads) — dark grey tarmac
  '52': {fill:'#5a9e3a', stroke:'#4a8e2a'},     // Wiese
  '53': {fill:'#62a240', stroke:'#52923a'},     // Weide
  '56': {fill:'#1e5a1e', stroke:'#145014'},     // Wald
  '57': {fill:'#2a5a2a', stroke:'#1a4a1a'},     // Krummholz
  '58': {fill:'#6a9a5a', stroke:'#5a8a4a'},     // Alpe
  '59': {fill:'#7a7860', stroke:'#6a6850'},     // Ödland
  '60': {fill:'#4a8a6a', stroke:'#3a7a5a'},     // Sumpf
  '61': {fill:'#5aa83a', stroke:'#4a982a'},     // Grünland gemäht
  '62': {fill:'#c8b858', stroke:'#a89838', a:0.65}, // Acker — golden/brown
  '63': {fill:'#80aa40', stroke:'#709a30'},     // Weingarten
  '64': {fill:'#6b8e4a', stroke:'#5b7e3a'},     // Gartenanlage
  '65': {fill:'#7aaa4a', stroke:'#6a9a3a'},     // Obstgarten
  '72': {fill:'#3090d0', stroke:'#2080c0', a:0.8}, // Quelle — bluer
  '83': {fill:'#9a9888', stroke:'#8a8878'},     // Fels
  '84': {fill:'#8a8878', stroke:'#7a7868'},     // Geröll
  '90': {fill:'#484848', stroke:'#383838', a:0.8}, // Verkehrsfläche — dark grey
  '91': {fill:'#505050', stroke:'#404040', a:0.7}, // Parkplatz — dark grey
  '92': {fill:'#6a9a5a', stroke:'#5a8a4a'},     // Hochalm
  '96': {fill:'#2888c8', stroke:'#1878b8', a:0.8}, // Gewässer — vivid blue
};
const LANDUSE_POLY_DEFAULT = {fill:'#5a8a40', stroke:'#4a7a30'};

function drawLandusePolygons(ctx) {
  const W = gc.width, H = gc.height;
  for (const f of G.landusePolys) {
    const geom = f.geometry;
    if (!geom) continue;
    const code = f.properties.landuse_code || '';
    const colors = LANDUSE_POLY_COLORS[code] || LANDUSE_POLY_DEFAULT;
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
function drawN2KOverlay(ctx) {
  const W = gc.width, H = gc.height;
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
    // Label at zoom >= 15
    if (labelPt && G.cam.zoom >= 15) {
      const hab = site.habitats || [];
      const habEmoji = hab.map(h => ({forest:'🌲',meadow:'🦋',floodplain:'💧',water:'💧',bog:'🌿',rock:'⛰️',alpine:'⛰️'}[h]||'🌿')).join('');
      ctx.font = '10px "Press Start 2P", monospace';
      ctx.textAlign = 'center';
      ctx.fillStyle = 'rgba(0,40,10,0.75)';
      const label = '🛡️ ' + site.name.slice(0, 40) + (site.name.length > 40 ? '…' : '');
      ctx.fillText(label, labelPt[0]+1, labelPt[1]+1);
      ctx.fillStyle = '#7dffa0';
      ctx.fillText(label, labelPt[0], labelPt[1]);
      if (habEmoji) {
        ctx.font = '14px sans-serif';
        ctx.fillText(habEmoji, labelPt[0], labelPt[1] + 18);
      }
      ctx.textAlign = 'left';
    }
  }
}

/** All loaded tall trees as a flat list. */
function allTallTrees() {
  const out = [];
  for (const kg in G.topTrees) for (const t of G.topTrees[kg]) out.push(t);
  return out;
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
    if (f.geometry.type === 'Polygon' && pip(t.lon, t.lat, f.geometry.coordinates[0])) {
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

  document.querySelector('#tree-popup h3').textContent = '🌲 ' + giantTreeName(tree);
  document.getElementById('tp-height').textContent = tree.height_m + ' m';
  const age = giantTreeAge(tree);
  document.getElementById('tp-age').textContent = age.text +
    (age.elev != null ? ' (auf ' + Math.round(age.elev) + ' m Seehöhe)' : '');
  document.getElementById('tp-rank').textContent = rank > 0 ? rank + '. von ' + nearby.length + ' Riesen in der Nähe' : '-';

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

/** Cheap check: is any giant tree (visible per current mode) inside the viewport? */
function anyTallTreeOnScreen() {
  if (!gc) return false;
  const W = gc.width, H = gc.height;
  const cand = !G.tallRevealed ? hintTallTrees(G.cam.zoom < 14 ? 3 : 12)
             : (G.cam.zoom < 14 ? hintTallTrees(6) : allTallTrees());
  for (const t of cand) {
    const [x, y] = toScreen(t.lon, t.lat);
    if (x > -80 && x < W+80 && y > -140 && y < H+80) return true;
  }
  return false;
}

/** The single "hint" tree shown after unlock but before reveal — the tallest loaded tree. */
function hintTallTree() {
  let best = null;
  for (const t of allTallTrees()) if (!best || t.height_m > best.height_m) best = t;
  return best;
}

/** The top-N tallest loaded trees, shown as hints before reveal (easier to spot). */
function hintTallTrees(n) {
  return allTallTrees().sort((a,b) => b.height_m - a.height_m).slice(0, n || 5);
}

// ---- Giant tree pixel-art sprite sheet (pre-rendered sway frames) ----
const GIANT_FRAMES = 8;
let _giantSprites = null; // [{c:canvas}] per frame
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

/** Draw one giant tree; size scales with real measured height. */
function drawGiantTree(ctx, t, zoom, sway, pop, isHint) {
  const [x, y] = toScreen(t.lon, t.lat);
  const W = gc.width, H = gc.height;
  if (x < -80 || x > W+80 || y < -140 || y > H+80) return;
  // Much taller than normal trees: height drives the scale (30m → ~2.2x, 55m → ~3.4x)
  const zs = Math.min(1.6, Math.max(0.7, (zoom - 14) / 3));
  const s = zs * (1.0 + t.height_m / 22) * pop;
  // Shadow at base
  ctx.fillStyle = 'rgba(0,0,0,0.22)';
  ctx.beginPath(); ctx.ellipse(x, y+2*s, 9*s, 3.2*s, 0, 0, Math.PI*2); ctx.fill();
  // Per-tree phase so auras/labels don't pulse in sync
  const phase = ((t.lon * 7919 + t.lat * 104729) % 6.283) || 0;
  const now = Date.now();
  if (isHint) {
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
  const sprites = giantTreeSprites();
  const frame = Math.floor(now / 130 + phase * GIANT_FRAMES / 6.283) % GIANT_FRAMES;
  const sp = sprites[(frame + GIANT_FRAMES) % GIANT_FRAMES];
  const dw = sp.width * s * 0.55, dh = sp.height * s * 0.55;
  const prevSmooth = ctx.imageSmoothingEnabled;
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(sp, x - dw/2, y - dh + 3*s, dw, dh);
  ctx.imageSmoothingEnabled = prevSmooth;
  // Label — bobbing height number with a soft glow pulse
  const label = isHint ? '🌲 ???'
    : (zoom >= 17 ? '🌲 ' + giantTreeName(t) + ' · ' + t.height_m + 'm'
                  : '🌲 ' + t.height_m + 'm');
  if (isHint || zoom >= 15.5) {
    const bob = Math.sin(now/450 + phase) * 3;
    const lp = 0.7 + Math.sin(now/300 + phase) * 0.3;
    const ly = y - dh + 3*s - 8 + bob;
    ctx.font = '13px VT323, monospace';
    ctx.textAlign = 'center';
    ctx.fillStyle = 'rgba(0,0,0,0.65)';
    ctx.fillText(label, x+1, ly+1);
    ctx.fillStyle = isHint
      ? 'rgba(255,215,0,' + lp.toFixed(2) + ')'
      : 'rgba(200,255,176,' + lp.toFixed(2) + ')';
    ctx.fillText(label, x, ly);
    ctx.textAlign = 'left';
  }
}

// ---- Miracle fog hint: when no giant tree is on screen for a few seconds,
// a swirling golden mist gathers at the screen edge pointing toward the
// nearest one. Tapping the mist flies the camera there.
let fogHintSince = 0;
let fogHintPos = null; // {x, y, lon, lat} for tap handling
function drawTallTreeFogHint(ctx) {
  const trees = G.tallRevealed ? allTallTrees() : hintTallTrees(12);
  if (!trees.length) return;
  if (anyTallTreeOnScreen()) { fogHintSince = 0; fogHintPos = null; return; }
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
  const k = Math.min(
    (W/2 - 70) / Math.max(Math.abs(dx), 1e-9),
    (H/2 - 90) / Math.max(Math.abs(dy), 1e-9));
  const ex = W/2 + dx*k, ey = H/2 + dy*k;
  fogHintPos = { x: ex, y: ey, lon: best.t.lon, lat: best.t.lat };
  // Distance in meters (approx equirectangular)
  const mLon = 111320 * Math.cos(G.cam.lat * Math.PI/180);
  const dm = Math.hypot((best.t.lon - G.cam.lon) * mLon, (best.t.lat - G.cam.lat) * 110540);
  const distTxt = dm >= 1000 ? (dm/1000).toFixed(1) + ' km' : Math.round(dm/10)*10 + ' m';
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
  ctx.fillStyle = 'rgba(255,215,0,0.95)';
  ctx.beginPath();
  ctx.moveTo(34 + bob, 0); ctx.lineTo(22 + bob, -7); ctx.lineTo(22 + bob, 7);
  ctx.closePath(); ctx.fill();
  ctx.rotate(-ang);
  ctx.font = '20px serif';
  ctx.textAlign = 'center';
  ctx.fillText('\uD83C\uDF32', 0, 7);
  // Distance label
  ctx.font = '13px VT323, monospace';
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

  // Giant trees: locked until first treasure; then only the hint tree until tapped.
  // Always show at least the single tallest hint even when fully zoomed out.
  if (G.tallUnlocked) {
    if (!G.tallRevealed) {
      // Show more hint trees so giants are easier to spot; still at least one
      // when fully zoomed out.
      const n = zoom < 14 ? 3 : 12;
      for (const hint of hintTallTrees(n)) drawGiantTree(ctx, hint, zoom, sway, 1, true);
    } else if (zoom < 14) {
      // Zoomed out: show the tallest handful so they stay locatable (>=1)
      for (const t of hintTallTrees(6)) drawGiantTree(ctx, t, zoom, sway, 1, false);
    } else {
      // Pop-in animation after reveal (staggered by height rank)
      const trees = allTallTrees().sort((a,b) => b.height_m - a.height_m);
      const dt = Date.now() - G.tallRevealAt;
      for (let i = 0; i < trees.length; i++) {
        const t0 = i * 90;
        if (dt < t0) continue;
        const k = Math.min(1, (dt - t0) / 350);
        const pop = k < 1 ? 0.3 + 0.7 * (1 - (1-k)*(1-k)) * (1 + 0.25*Math.sin(k*Math.PI)) : 1;
        drawGiantTree(ctx, trees[i], zoom, sway, pop, false);
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
          ctx.font = '11px VT323, monospace';
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
  const dirs = ['N','NO','O','SO','S','SW','W','NW'];
  const dir = dirs[Math.round(brg / 45) % 8];
  const label = '📍 ' + distTxt + ' ' + dir;
  ctx.font = '13px VT323, monospace';
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

/** Pulsing blue GPS dot + accuracy circle. */
function drawGeoMarker(ctx) {
  const g = G.geo;
  if (!g.lon) return;
  const [x, y] = toScreen(g.lon, g.lat);
  const W = gc.width, H = gc.height;
  if (x < -100 || x > W+100 || y < -100 || y > H+100) return;
  // Accuracy circle (meters → px): 1°lat ≈ 111320m
  const s = mapScale();
  const accPx = (g.acc / 111320) * s * 1.35;
  if (accPx > 8 && accPx < 600) {
    ctx.fillStyle = 'rgba(60,140,255,0.12)';
    ctx.strokeStyle = 'rgba(60,140,255,0.35)';
    ctx.lineWidth = 1;
    ctx.beginPath(); ctx.arc(x, y, accPx, 0, Math.PI*2); ctx.fill(); ctx.stroke();
  }
  // Pulse ring
  const pulse = (Date.now() % 1600) / 1600;
  ctx.strokeStyle = 'rgba(60,140,255,' + (0.6 * (1-pulse)).toFixed(2) + ')';
  ctx.lineWidth = 2;
  ctx.beginPath(); ctx.arc(x, y, 8 + pulse * 16, 0, Math.PI*2); ctx.stroke();
  // Dot
  ctx.fillStyle = '#fff';
  ctx.beginPath(); ctx.arc(x, y, 7, 0, Math.PI*2); ctx.fill();
  ctx.fillStyle = '#2a7ae0';
  ctx.beginPath(); ctx.arc(x, y, 5, 0, Math.PI*2); ctx.fill();
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

    // Pick roof style based on building size & hash
    const hash = Math.round(coords[0][0] * 100000) ^ Math.round(coords[0][1] * 100000);
    const colorIdx = (Math.abs(hash) % ROOF_COLORS.length);
    // Large buildings (>600px area) get industrial/slate colors
    const rc = area > 600 ? ROOF_COLORS[3 + (Math.abs(hash) % 2)] : ROOF_COLORS[colorIdx % 3];

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
      const defStories = area > 400 ? 2 : 1.5;
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

    // Wall (side) — draw extruded shape for visible edges
    ctx.fillStyle = rc.wall;
    ctx.beginPath();
    for (let i=0; i<pts.length; i++) {
      const j = (i+1) % pts.length;
      if (pts[i][1] >= pts[j][1] - 0.5) {
        ctx.moveTo(pts[i][0], pts[i][1]);
        ctx.lineTo(pts[j][0], pts[j][1]);
        ctx.lineTo(pts[j][0], pts[j][1]-roofOff);
        ctx.lineTo(pts[i][0], pts[i][1]-roofOff);
        ctx.closePath();
      }
    }
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

    // Lidar roof type: pitched → ridge line along long axis; flat → lighter plain top
    // Buildings without a measured roof type default to pitched (typical Austrian roofs);
    // large footprints default to flat (industrial/commercial).
    let roofHint = lidarB && lidarB.roof_type_hint;
    if (!roofHint && enhanced) roofHint = area > 900 ? 'flat' : 'pitched';
    if (roofHint && zoom >= 16 && (bw > 8 || bh > 8)) {
      if (roofHint === 'pitched') {
        ctx.strokeStyle = 'rgba(255,235,200,0.45)';
        ctx.lineWidth = 1.2;
        ctx.beginPath();
        if (bw >= bh) {
          ctx.moveTo(minX + bw*0.18, (minY+maxY)/2 - roofOff);
          ctx.lineTo(maxX - bw*0.18, (minY+maxY)/2 - roofOff);
        } else {
          ctx.moveTo((minX+maxX)/2, minY + bh*0.18 - roofOff);
          ctx.lineTo((minX+maxX)/2, maxY - bh*0.18 - roofOff);
        }
        ctx.stroke();
      } else if (roofHint === 'flat') {
        ctx.fillStyle = 'rgba(180,180,190,0.18)';
        ctx.fill();
      }
    }


  }
}

function drawParcelPoly(ctx, f, claimMap) {
  const p = f.properties;
  const geom = f.geometry;
  if (!geom || geom.type !== 'Polygon') return;

  const coords = geom.coordinates[0];
  const parcelId = p.parcel_id;
  const claim = claimMap[parcelId];
  const terrain = getParcelTerrain(p, claim);

  // Project coordinates
  const pts = coords.map(c => toScreen(c[0], c[1]));

  // Check if visible
  let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
  for (const pt of pts) {
    if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
    if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
  }
  if (maxX < -50 || minX > gc.width+50 || maxY < -50 || minY > gc.height+50) return;

  ctx.beginPath();
  for (let i=0; i<pts.length; i++) {
    i===0 ? ctx.moveTo(pts[i][0], pts[i][1]) : ctx.lineTo(pts[i][0], pts[i][1]);
  }
  ctx.closePath();

  // Settlers-style fill with variation
  const hash = simpleHash(parcelId || '');
  const isBiodiversity = claim?.converted_to === 'biodiversity';
  const isForest = claim?.converted_to === 'forest';
  ctx.fillStyle = terrain[Math.abs(hash) % terrain.length];
  // More transparent when real landuse polys provide terrain backdrop
  // Biodiversity parcels get higher opacity for vibrancy
  ctx.globalAlpha = isBiodiversity
    ? (G.landusePolys.length > 0 ? 0.55 : 0.95)
    : (G.landusePolys.length > 0 ? 0.35 : 0.85);
  ctx.fill();
  ctx.globalAlpha = 1;

  // Enhanced mode: subtle per-parcel elevation tint (lidar) — cheap terrain shading
  if (G.cam.zoom >= 14) {
    const lp = G.lidarParcels[parcelId];
    if (lp && lp.elev != null) {
      const kt = G.lidarKGTerrain[lp.kg];
      if (kt && kt.emax > kt.emin) {
        const n = Math.max(0, Math.min(1, (lp.elev - kt.emin) / (kt.emax - kt.emin)));
        // low = slightly darker (valley shadow), high = slightly lighter (sunlit)
        if (n < 0.45) {
          ctx.fillStyle = 'rgba(10,20,40,' + ((0.45-n) * 0.28).toFixed(3) + ')';
          ctx.fill();
        } else if (n > 0.55) {
          ctx.fillStyle = 'rgba(255,250,220,' + ((n-0.55) * 0.22).toFixed(3) + ')';
          ctx.fill();
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

  // Biodiversity: soft green glow overlay
  if (isBiodiversity) {
    const pulse = 0.12 + Math.sin(Date.now() / 2000 + Math.abs(hash) * 0.1) * 0.04;
    ctx.fillStyle = `rgba(60,200,80,${pulse})`;
    ctx.fill();
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
  // Normal border on top
  ctx.strokeStyle = claim ? (G.pcolors[claim.player_id]||'#fff') : 'rgba(20,40,10,0.35)';
  ctx.lineWidth = claim ? 2 : 0.5;
  ctx.stroke();

  // Biodiversity: extra wildflower/butterfly sprites and sparkle particles
  if (isBiodiversity && (maxX - minX) > 6 && (maxY - minY) > 6) {
    const zoom = G.cam.zoom;
    const absHash = Math.abs(hash);
    // At zoom >= 16, draw extra wildflowers and butterflies inside the parcel
    if (zoom >= 16) {
      const pxArea = (maxX - minX) * (maxY - minY);
      const extraCount = Math.min(12, Math.max(3, Math.floor(pxArea / 400)));
      for (let i = 0; i < extraCount; i++) {
        const t = ((absHash + i * 6197) % 10000) / 10000;
        const u = ((absHash + i * 4253) % 10000) / 10000;
        const sx = minX + (maxX - minX) * (0.1 + t * 0.8);
        const sy = minY + (maxY - minY) * (0.1 + u * 0.8);
        if (!pip(sx, sy, pts)) continue;
        // Tiny wildflowers
        const flColors = ['#e8e040','#e060a0','#a060e0','#60a0e8','#e08040','#ff7070','#70e070'];
        const fc = flColors[(absHash + i) % flColors.length];
        const fs = zoom > 17 ? 1.0 : 0.65;
        // Stem
        ctx.strokeStyle = '#4a8a2a';
        ctx.lineWidth = 0.6;
        ctx.beginPath();
        ctx.moveTo(sx, sy);
        ctx.lineTo(sx + ((absHash+i)%3-1)*fs, sy - 5*fs);
        ctx.stroke();
        // Flower
        ctx.fillStyle = fc;
        ctx.beginPath();
        ctx.arc(sx + ((absHash+i)%3-1)*fs, sy - 5*fs - 1*fs, 1.3*fs, 0, Math.PI*2);
        ctx.fill();
        // Extra butterfly every 4th sprite
        if (i % 4 === 0) {
          const bt = (Date.now() / 900 + absHash + i) % (Math.PI*2);
          const bx = sx + Math.sin(bt) * 4*fs;
          const by = sy - 10*fs + Math.cos(bt*1.3) * 2*fs;
          const wing = Math.abs(Math.sin(Date.now()/180 + absHash + i)) * 2*fs + 0.8*fs;
          ctx.fillStyle = flColors[(absHash + i + 3) % flColors.length];
          ctx.beginPath(); ctx.ellipse(bx - wing*0.4, by, wing, 0.8*fs, -0.3, 0, Math.PI*2); ctx.fill();
          ctx.beginPath(); ctx.ellipse(bx + wing*0.4, by, wing, 0.8*fs, 0.3, 0, Math.PI*2); ctx.fill();
        }
      }
    }
    // Animated sparkle particles (nature magic / healing) — visible at any zoom
    const sparkCount = Math.min(5, Math.max(2, Math.floor((maxX-minX)*(maxY-minY) / 800)));
    for (let i = 0; i < sparkCount; i++) {
      const phase = (Date.now() / 1500 + absHash * 0.3 + i * 1.7) % 1.0; // 0..1 cycle
      const t = ((absHash + i * 8831) % 10000) / 10000;
      const sx = minX + (maxX - minX) * (0.15 + t * 0.7);
      const baseY = maxY - (maxY - minY) * 0.15;
      const sy = baseY - phase * (maxY - minY) * 0.8;
      const sparkAlpha = phase < 0.2 ? phase / 0.2 : phase > 0.8 ? (1 - phase) / 0.2 : 1.0;
      const sparkSize = 1.0 + Math.sin(Date.now() / 300 + i) * 0.4;
      ctx.fillStyle = `rgba(200,255,180,${(sparkAlpha * 0.7).toFixed(2)})`;
      ctx.beginPath();
      ctx.arc(sx + Math.sin(Date.now() / 700 + i * 2) * 2, sy, sparkSize, 0, Math.PI*2);
      ctx.fill();
      // Sparkle cross
      ctx.strokeStyle = `rgba(220,255,200,${(sparkAlpha * 0.5).toFixed(2)})`;
      ctx.lineWidth = 0.4;
      ctx.beginPath();
      ctx.moveTo(sx - sparkSize*1.5, sy);
      ctx.lineTo(sx + sparkSize*1.5, sy);
      ctx.moveTo(sx, sy - sparkSize*1.5);
      ctx.lineTo(sx, sy + sparkSize*1.5);
      ctx.stroke();
    }
  }

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
  // Use landuse_codes if available (from bbox spatial endpoint)
  if (p.landuse_codes) {
    const first = p.landuse_codes.split(',')[0].trim();
    if (first) return first;
  }
  if (p.dominant_landuse) return p.dominant_landuse;
  // Parse from landuse_summary using our proper parser
  if (p.landuse_summary) {
    const parsed = parseLanduseSummary(p.landuse_summary);
    if (parsed.dominant) return parsed.dominant.code;
  }
  // Fallback: try numeric from raw string
  const match = (lu || '').match(/(\d{2})/);
  if (match) return match[1];
  return '';
}

/** Get terrain colors from landuse_summary, returns the dominant terrain color array */
function getParcelTerrain(p, claim) {
  if (claim?.converted_to) return TERRAIN.bio;
  // Enhanced mode: real measured dominant land cover from lidar beats cadastre landuse
  const lp = G.lidarParcels[p.parcel_id];
  // Use the corrected dominant land cover (impervious road/roof skipped server-side,
  // falls back to #2 natural cover). Buildings are drawn as footprints on top.
  if (lp) {
    const dt = lp.domTerrain || (lp.dom && !IMPERVIOUS_DOM.has(lp.dom) ? lp.dom : null);
    if (dt && DOM_TERRAIN[dt]) return DOM_TERRAIN[dt];
  }
  if (p.landuse_summary) {
    const parsed = parseLanduseSummary(p.landuse_summary);
    if (parsed.dominant) return parsed.dominant.terrain;
  }
  const luCode = extractLuCode('', p);
  return LANDUSE_TERRAIN[luCode] || TERRAIN.grass;
}

/** Get human-readable landuse name from summary */
function getLanduseName(p) {
  if (p.landuse_summary) {
    const parsed = parseLanduseSummary(p.landuse_summary);
    if (parsed.entries.length > 0) {
      return parsed.entries.map(e => e.name + (e.count > 1 ? ' (×'+e.count+')' : '')).join(', ');
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
    if (!geom || geom.type !== 'Polygon') continue;
    const claim = claimMap[p.parcel_id];
    const terrain = getParcelTerrain(p, claim);
    const luCode = extractLuCode('', p);
    const area = p.area_sqm || 0;
    if (area < 100) continue;

    const coords = geom.coordinates[0];
    const b = geoBounds(geom);
    const [sx1,sy1] = toScreen(b.w, b.n);
    const [sx2,sy2] = toScreen(b.e, b.s);
    if (sx2 < 0 || sx1 > gc.width || sy2 < 0 || sy1 > gc.height) continue;
    if ((sx2-sx1) < 10 || (sy2-sy1) < 10) continue;

    const hash = simpleHash(p.parcel_id || '');
    let spriteType = null;

    // Determine sprite type from landuse
    if (luCode === '50' || luCode === '51') spriteType = 'crops';
    else if (luCode === '52' || luCode === '53' || luCode === '54') spriteType = 'meadow';
    else if (luCode === '60') spriteType = 'vineyard';
    else if (luCode === '62') spriteType = 'garden';
    else if (luCode === '63') continue; // orchards handled by tree sprites
    else if (terrain === TERRAIN.farm) spriteType = 'crops';
    else if (terrain === TERRAIN.meadow && luCode !== '55') spriteType = 'meadow';
    else if (terrain === TERRAIN.water) spriteType = 'water';
    else if (terrain === TERRAIN.wetland) spriteType = 'reeds';
    else if (claim?.converted_to === 'biodiversity') spriteType = 'wildflower';
    else continue;

    const count = Math.min(14, Math.max(2, Math.floor(area / 600)));
    for (let i = 0; i < count; i++) {
      const t = ((hash + i * 7919) % 10000) / 10000;
      const u = ((hash + i * 3571) % 10000) / 10000;
      const lon = b.w + (b.e - b.w) * (0.1 + t * 0.8);
      const lat = b.s + (b.n - b.s) * (0.1 + u * 0.8);
      if (!pip(lon, lat, coords)) continue;
      const [sx, sy] = toScreen(lon, lat);
      const v = (hash + i) % 5;
      switch (spriteType) {
        case 'crops': drawCropSprite(ctx, sx, sy, v, hash+i); break;
        case 'meadow': drawMeadowSprite(ctx, sx, sy, v, hash+i); break;
        case 'vineyard': drawVineyardSprite(ctx, sx, sy, v); break;
        case 'garden': drawGardenSprite(ctx, sx, sy, v, hash+i); break;
        case 'water': drawWaterSprite(ctx, sx, sy, v, hash+i); break;
        case 'reeds': drawReedSprite(ctx, sx, sy, v, hash+i); break;
        case 'wildflower': drawWildflowerSprite(ctx, sx, sy, v, hash+i); break;
      }
    }
  }
  ctx.restore();
}

function drawCropSprite(ctx, x, y, v, seed) {
  const s = G.cam.zoom > 17 ? 1.2 : 0.8;
  x = Math.round(x); y = Math.round(y);
  // Wheat/grain stalks in rows
  const colors = ['#c8a830','#d0b038','#b89828','#d8b840','#c0a028'];
  const stalkColor = colors[v];
  for (let j = -2; j <= 2; j++) {
    const ox = x + j * 3 * s;
    // Stalk
    ctx.strokeStyle = '#8a7a30';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(ox, y);
    ctx.lineTo(ox + (seed%3-1)*0.5, y - 10*s);
    ctx.stroke();
    // Grain head
    ctx.fillStyle = stalkColor;
    ctx.fillRect(ox - 1*s, y - 12*s, 2*s, 4*s);
    // Awns (tiny lines at top)
    ctx.strokeStyle = stalkColor;
    ctx.beginPath();
    ctx.moveTo(ox, y - 12*s);
    ctx.lineTo(ox - 1.5*s, y - 14*s);
    ctx.moveTo(ox, y - 12*s);
    ctx.lineTo(ox + 1.5*s, y - 14*s);
    ctx.stroke();
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

function drawWildflowerSprite(ctx, x, y, v, seed) {
  const s = G.cam.zoom > 17 ? 1.0 : 0.7;
  x = Math.round(x); y = Math.round(y);
  // Dense wildflower patch (biodiversity)
  const flowerColors = ['#e8e040','#e060a0','#a060e0','#60a0e8','#e08040','#ff7070','#70e070'];
  // Stems
  for (let j = -3; j <= 3; j++) {
    const ox = x + j * 2 * s;
    const h = (6 + (seed+j)%4) * s;
    ctx.strokeStyle = '#4a8a2a';
    ctx.lineWidth = 0.8;
    ctx.beginPath();
    ctx.moveTo(ox, y);
    ctx.lineTo(ox + ((seed+j)%3-1)*s, y - h);
    ctx.stroke();
    // Flower head
    ctx.fillStyle = flowerColors[(seed+j+v) % flowerColors.length];
    ctx.beginPath();
    ctx.arc(ox + ((seed+j)%3-1)*s, y - h - 1*s, 1.5*s, 0, Math.PI*2);
    ctx.fill();
  }
  // Butterfly on some (animated)
  if (v === 0) {
    const bt = (Date.now() / 800 + seed) % (Math.PI*2);
    const bx = x + Math.sin(bt) * 5*s;
    const by = y - 12*s + Math.cos(bt*1.5) * 2*s;
    const wing = Math.abs(Math.sin(Date.now()/200 + seed)) * 2*s + 1*s;
    ctx.fillStyle = flowerColors[(seed+2) % flowerColors.length];
    ctx.beginPath(); ctx.ellipse(bx - wing*0.5, by, wing, 1*s, -0.3, 0, Math.PI*2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(bx + wing*0.5, by, wing, 1*s, 0.3, 0, Math.PI*2); ctx.fill();
  }
}

function drawForestSprites(ctx, claimMap) {
  // Draw tree sprites on forest, reforested, orchard and scrub parcels
  // Determine tree style per parcel: 'forest' | 'reforested' | 'orchard' | 'krummholz'
  function getTreeStyle(f) {
    const claim = claimMap[f.properties.parcel_id];
    if (claim?.converted_to === 'forest') return 'reforested';
    const t = getParcelTerrain(f.properties, claim);
    if (t !== TERRAIN.forest && t !== TERRAIN.garden) return null;
    const luCode = extractLuCode('', f.properties);
    if (luCode === '63') return 'orchard';   // Obstgarten
    if (luCode === '57') return 'krummholz'; // Krummholz/Latschen
    if (luCode === '58') return 'plantation'; // Aufforstung / plantation
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
    const coords = f.geometry.coordinates[0];
    const b = geoBounds(f.geometry);
    const [sx1,sy1] = toScreen(b.w, b.n);
    const [sx2,sy2] = toScreen(b.e, b.s);
    if (sx2 < 0 || sx1 > gc.width || sy2 < 0 || sy1 > gc.height) continue;

    const area = f.properties.area_sqm || 1000;
    const hash = simpleHash(f.properties.parcel_id||'');

    let treeCount, variantFn;
    if (style === 'orchard') {
      // Orchards: sparser, always fruit tree
      treeCount = Math.min(12, Math.max(2, Math.floor(area / 500)));
      variantFn = (i) => 4;
    } else if (style === 'krummholz') {
      // Krummholz: dense low scrub
      treeCount = Math.min(20, Math.max(3, Math.floor(area / 250)));
      variantFn = (i) => 2;
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

    // Draw bright green underglow for reforested parcels
    if (style === 'reforested') {
      const pts = coords.map(c => toScreen(c[0], c[1]));
      ctx.beginPath();
      pts.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
      ctx.closePath();
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
      if (!pip(lon, lat, coords)) continue;
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
        if (!pip(lon2, lat2, coords)) continue;
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

function drawTreasure(ctx, t) {
  const [x, y] = toScreen(t.lon, t.lat);
  if (x < -20 || x > gc.width+20 || y < -20 || y > gc.height+20) return;

  if ((t.treasure_type === 'species' || t.treasure_type === 'n2k_species') && t.species_name) {
    // Natura-2000 rare-species treasures get a pulsing gold ring
    if (t.treasure_type === 'n2k_species') {
      const pulse = (Date.now() % 2000) / 2000;
      ctx.strokeStyle = 'rgba(255,210,60,' + (0.8 * (1-pulse)).toFixed(2) + ')';
      ctx.lineWidth = 2.5;
      ctx.beginPath(); ctx.arc(x, y-4, 12 + pulse * 14, 0, Math.PI*2); ctx.stroke();
      ctx.strokeStyle = 'rgba(255,230,120,0.5)';
      ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.arc(x, y-4, 12, 0, Math.PI*2); ctx.stroke();
    }
    drawSpeciesTreasure(ctx, x, y, t);
  } else {
    drawChestTreasure(ctx, x, y, t);
  }
}

function drawChestTreasure(ctx, x, y, t) {
  const frame = Math.floor(Date.now() / 400) % 3;
  // Chest body
  ctx.fillStyle = '#6b4020';
  ctx.fillRect(x-7, y-3, 14, 9);
  ctx.fillStyle = '#8b5530';
  ctx.fillRect(x-6, y-2, 12, 7);
  // Lid
  ctx.fillStyle = '#7b4828';
  ctx.fillRect(x-7, y-6, 14, 4);
  // Gold trim
  ctx.fillStyle = '#d4a843';
  ctx.fillRect(x-7, y-3, 14, 1);
  ctx.fillRect(x-1, y-6, 2, 9);
  // Sparkles
  ctx.fillStyle = '#fff';
  const sx = [x-10, x+8, x-6, x+10][frame];
  const sy = [y-10, y-8, y-12, y-6][frame];
  ctx.fillRect(sx, sy, 2, 2);
  ctx.fillRect(sx-1, sy+1, 1, 1);
  ctx.fillRect(sx+2, sy+1, 1, 1);
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

function drawSpeciesTreasure(ctx, x, y, t) {
  const sprite = SPECIES_SPRITE_MAP[t.species_name] || 'butterfly_white';
  const time = Date.now();
  const s = G.cam.zoom > 16 ? 1.4 : 1.0;
  const bob = Math.sin(time / 600 + x * 0.01) * 2;

  ctx.save();

  // Soft glow circle underneath
  const glowPulse = 0.3 + Math.sin(time / 800) * 0.1;
  const catColor = {'EN':'rgba(220,60,60,','VU':'rgba(220,160,40,','NT':'rgba(100,180,220,','LC':'rgba(100,200,100,'};
  const gBase = catColor[t.species_category] || 'rgba(200,200,100,';
  ctx.fillStyle = gBase + glowPulse + ')';
  ctx.beginPath(); ctx.arc(x, y + 2, 12*s, 0, Math.PI*2); ctx.fill();

  const by = y + bob;

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

  // Species label at higher zoom
  if (G.cam.zoom >= 17 && t.species_german) {
    ctx.fillStyle = 'rgba(0,0,0,0.6)';
    ctx.font = `${8*s}px "VT323", monospace`;
    ctx.textAlign = 'center';
    ctx.fillText(t.species_german, x, by + 14*s);
    // Red list category badge
    const catColors = {'EN':'#d04040','VU':'#e0a020','NT':'#60a0d0','LC':'#60b060'};
    const cc = catColors[t.species_category] || '#888';
    ctx.fillStyle = cc;
    ctx.fillText(t.species_category, x, by + 22*s);
  }

  ctx.restore();
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
    if (f.geometry.type === 'Polygon') {
      const pts = f.geometry.coordinates[0].map(c => toScreen(c[0], c[1]));
      // Quick bounds check
      let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
      for (const pt of pts) {
        if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
        if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
      }
      if (maxX < -50 || minX > gc.width+50 || maxY < -50 || minY > gc.height+50) continue;
      ctx.beginPath();
      pts.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
      ctx.closePath();
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

  if (f.geometry.type === 'Polygon') {
    const pts = f.geometry.coordinates[0].map(c => toScreen(c[0], c[1]));
    ctx.beginPath();
    pts.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
    ctx.closePath();
    ctx.stroke();
  } else {
    const [x,y] = toScreen(p.lon||f.geometry.coordinates[0], p.lat||f.geometry.coordinates[1]);
    const sz = Math.max(12, Math.sqrt(p.area_sqm||100) * mapScale() / 60000);
    ctx.strokeRect(x-sz/2-3, y-sz/2-3, sz+6, sz+6);
  }
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
  ctx.font = '12px VT323'; ctx.fillStyle = '#fff';
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
        await navigator.share({ title: 'Siedler Österreich', text: 'Komm zu mir auf die Karte!', url });
        return;
      } catch(e) { if (e.name === 'AbortError') return; /* fall through to clipboard */ }
    }
    try {
      await navigator.clipboard.writeText(url);
      toast('🔗 Link zu dieser Ansicht kopiert — einfach weiterschicken!', 'ok');
    } catch(e) {
      prompt('Link kopieren:', url);
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

  // "Developer mode": 5 quick taps on the Enhanced-Gelände badge (like
  // Android's build-number easter egg) unlock the giant tree nearest to the
  // viewport center and fly to it.
  const enhBadge = document.getElementById('enhanced-badge');
  if (enhBadge) {
    let devTaps = 0, devTapAt = 0;
    enhBadge.onclick = () => {
      const now = Date.now();
      if (now - devTapAt > 2500) devTaps = 0;   // taps must be quick
      devTapAt = now;
      devTaps++;
      if (devTaps < 5) {
        const left = 5 - devTaps;
        if (devTaps >= 2) toast('✨ Noch ' + left + (left === 1 ? ' Tap' : ' Taps') + ' …', '');
        return;
      }
      devTaps = 0;
      const trees = allTallTrees();
      if (!trees.length) { toast('🌲 Noch keine Riesenbäume geladen …', 'err'); return; }
      const mLon = 111320 * Math.cos(G.cam.lat * Math.PI/180);
      let best = null, bd = Infinity;
      for (const t of trees) {
        const d = Math.hypot((t.lon - G.cam.lon) * mLon, (t.lat - G.cam.lat) * 110540);
        if (d < bd) { bd = d; best = t; }
      }
      G.devTree = best;
      flyTo(best.lon, best.lat, Math.max(G.cam.zoom, 16.5));
      toast('🔓 Entdeckermodus: ' + giantTreeName(best) + ' (' + best.height_m + ' m) freigeschaltet!', 'ok');
      render();
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
function initGameSearch() {
  const inp = document.getElementById('game-search-input');
  const dd = document.getElementById('game-search-results');
  if (!inp || !dd) return;
  let timer;
  inp.addEventListener('input', () => {
    clearTimeout(timer);
    const q = inp.value.trim();
    if (q.length < 2) { dd.classList.remove('open'); return; }
    timer = setTimeout(async () => {
      try {
        const res = await GET(CAD+'/search/address_osm?q='+encodeURIComponent(q)+'&limit=5');
        const items = res.data || [];
        if (!items.length) {
          dd.innerHTML = '<div class="search-item"><small>Keine Ergebnisse</small></div>';
        } else {
          dd.innerHTML = items.map((a,i) =>
            `<div class="search-item" data-idx="${i}">`+
            esc(a.display_name)+'<br><small>'+(a.address?.municipality||a.address?.city||a.address?.town||'')+'</small></div>'
          ).join('');
          dd.querySelectorAll('.search-item').forEach(el => {
            el.onclick = () => {
              const idx = parseInt(el.dataset.idx);
              const a = items[idx];
              if (!a) return;
              dd.classList.remove('open');
              inp.value = a.display_name || '';
              inp.blur();
              flyTo(parseFloat(a.lon), parseFloat(a.lat), 18);
            };
          });
        }
        dd.classList.add('open');
      } catch(e) { console.error('Search error:', e); }
    }, 350);
  });
  // ESC and blur
  inp.addEventListener('keydown', e => {
    if (e.key==='Escape') { dd.classList.remove('open'); inp.blur(); }
    if (e.key==='Enter') {
      const first = dd.querySelector('.search-item[data-idx]');
      if (first) first.click();
    }
  });
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
  // Check corners of viewport for municipality crossings
  const b = viewBounds();
  const centerLon = (b.w + b.e) / 2;
  const centerLat = (b.s + b.n) / 2;
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
  el.innerHTML = '\uD83D\uDDFA\uFE0F Du verlässt <span class="muni-name">'+esc(G.homeMuni)+'</span> — Parzellen aus <span class="muni-name">'+esc(name)+'</span> werden geladen';
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
    if (Math.abs(tx-x)<15 && Math.abs(ty-y)<15) { claimTreasure(t); return; }
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

  // Hint giant tree: tapping it reveals ALL giant trees
  if (G.tallUnlocked && !G.tallRevealed) {
    for (const hint of hintTallTrees(5)) {
      const [tx, ty] = toScreen(hint.lon, hint.lat);
      if (Math.abs(tx-x) < 30 && ty-y > -20 && ty-y < 90) {
        G.tallRevealed = true;
        G.tallRevealAt = Date.now();
        const n = allTallTrees().length;
        toast('🌲 Riesenbaum entdeckt! ' + n + ' Riesenbäume sind nun sichtbar — Grundstücke mit Riesenbäumen bringen Bonus-XP!', 'ok');
        render();
        return;
      }
    }
  }

  // Revealed giant tree: tap opens info popup with height, age + histogram
  if (G.tallUnlocked && G.tallRevealed) {
    let hitTree = null, hitD = Infinity;
    for (const t of allTallTrees()) {
      const [tx, ty] = toScreen(t.lon, t.lat);
      if (Math.abs(tx-x) < 30 && ty-y > -20 && ty-y < 90) {
        const d = Math.abs(tx-x) + Math.abs(ty-y-35);
        if (d < hitD) { hitD = d; hitTree = t; }
      }
    }
    if (hitTree) { showTreePopup(hitTree); return; }
  }

  // Check polygon parcels
  for (const f of G.parcelPolys) {
    if (f.geometry.type==='Polygon' && pip(lon, lat, f.geometry.coordinates[0])) {
      showParcelPopup(f); return;
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
  if (best) { showParcelPopup(best); return; }

  document.getElementById('parcel-popup').classList.remove('open');
  document.getElementById('ez-popup').classList.remove('open');
  document.getElementById('tree-popup').classList.remove('open');
  resetPopupPosition('parcel-popup');
  resetPopupPosition('ez-popup');
  resetPopupPosition('tree-popup');
  G.sel = null; G.ezHighlight = null; render();
}

function showParcelPopup(f) {
  G.sel = f;
  const p = f.properties;
  const pid = p.parcel_id;

  // Keep camera stable on parcel tap (no zoom jumps — important on mobile).
  // Only nudge the view if the parcel is off-screen (e.g. re-opened programmatically).
  const pLon = p.lon || (f.geometry.type === 'Polygon' ? centroidOf(f.geometry.coordinates[0])[0] : f.geometry.coordinates[0]);
  const pLat = p.lat || (f.geometry.type === 'Polygon' ? centroidOf(f.geometry.coordinates[0])[1] : f.geometry.coordinates[1]);
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
  document.getElementById('pp-kg').textContent = p.kg_name || p.kg_code || '-';
  const ez = p.ez || '';
  document.getElementById('pp-ez').textContent = ez ? 'EZ ' + ez : '-';
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
  document.getElementById('pp-price').textContent = claim ? (claim.player_id===G.player.id?'Dein Besitz':'Besetzt') : price+' 🪙';

  renderEnhancedPopupRows(pid, price);

  const act = document.getElementById('pp-actions');
  act.innerHTML = '';
  if (!claim) {
    act.innerHTML = `<button class="btn btn-primary btn-small" onclick="doClaim()">🏴 Kaufen (${price}🪙)</button>`;
  } else if (claim.player_id === G.player.id && !claim.converted_to) {
    // My parcel — show convert/sell + any incoming offers
    let html = `
      <button class="btn btn-primary btn-small" onclick="doConvert('biodiversity')">🌿 Naturschutz</button>
      <button class="btn btn-secondary btn-small" onclick="doConvert('forest')">🌳 Aufforsten</button>
      <button class="btn btn-danger btn-small" onclick="doSell(${claim.id})">💰 Verkaufen</button>`;
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
    act.innerHTML = `<span style="font:18px VT323;color:var(--green-light)">✅ ${claim.converted_to}</span>`;
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

/** Enhanced-mode rows in the parcel popup: elevation, slope, vegetation, market value, Natura 2000. */
function renderEnhancedPopupRows(pid, gamePrice) {
  const box = document.getElementById('pp-enhanced');
  const moreBtn = document.getElementById('pp-more');
  if (!box) return;
  const isMobile = window.innerWidth < 768;
  const rows = [];      // always visible
  const moreRows = [];  // behind "Mehr ▸" on mobile

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
      rows.push(['⛰️ Hang', lp.slope.toFixed(1) + '° ' + (arrows[lp.aspect]||'') + (lp.tclass ? ' · ' + (tlabels[lp.tclass]||lp.tclass) : '')]);
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

  // Natura 2000: is parcel inside a loaded site polygon?
  const pLon = G.sel?.properties?.lon || (G.sel?.geometry?.type === 'Polygon' ? centroidOf(G.sel.geometry.coordinates[0])[0] : null);
  const pLat = G.sel?.properties?.lat || (G.sel?.geometry?.type === 'Polygon' ? centroidOf(G.sel.geometry.coordinates[0])[1] : null);
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
    const showMore = !isMobile || box.dataset.expanded === '1';
    const list = showMore ? rows.concat(moreRows) : rows;
    if (list.length === 0 && !(pid in G.landPrices)) { box.style.display = 'none'; moreBtn.style.display = 'none'; return; }
    let html = '';
    for (const [k, v] of list) html += '<span>' + k + '</span><b>' + v + '</b>';
    // Market value row (lazy loaded)
    const mv = G.landPrices[pid];
    if (mv) {
      const eur = mv.buy_total_eur >= 1e6 ? (mv.buy_total_eur/1e6).toFixed(2) + ' Mio €' : Math.round(mv.buy_total_eur).toLocaleString('de-AT') + ' €';
      const cls = {bauland_built:'Bauland (bebaut)', bauland_zoned:'Bauland', ackerland:'Ackerland', gruenland:'Grünland', wald:'Wald', other:'Sonstig'}[mv.class] || mv.class;
      html += '<span>💶 Marktwert</span><b style="color:var(--gold)">' + eur + ' <span style="color:var(--text-dim)">(' + cls + ')</span></b>';
      if (showMore && gamePrice > 0) {
        html += '<span></span><b style="color:var(--text-dim);font-size:14px">Spielpreis: ' + gamePrice + '🪙 · ' + Math.round(mv.buy_eur_per_sqm) + ' €/m² echt</b>';
      }
    }
    box.innerHTML = html;
    box.style.display = html ? '' : 'none';
    moreBtn.style.display = (isMobile && moreRows.length > 0 && box.dataset.expanded !== '1') ? '' : 'none';
  };

  box.dataset.expanded = '';
  moreBtn.onclick = () => { box.dataset.expanded = '1'; renderRows(); };
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
  const pLon = p.lon || (f.geometry.type === 'Polygon' ? centroidOf(f.geometry.coordinates[0])[0] : f.geometry.coordinates[0]);
  const pLat = p.lat || (f.geometry.type === 'Polygon' ? centroidOf(f.geometry.coordinates[0])[1] : f.geometry.coordinates[1]);
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
  render();
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
      ctx.font = '10px "Press Start 2P", monospace';
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
    ctx.font = '9px "Press Start 2P", monospace';
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
      ctx.font = '10px "Press Start 2P", monospace';
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
    if (pf.geometry.type === 'Polygon') {
      // Get bounds from polygon coordinates
      for (const coord of pf.geometry.coordinates[0]) {
        minLon = Math.min(minLon, coord[0]);
        maxLon = Math.max(maxLon, coord[0]);
        minLat = Math.min(minLat, coord[1]);
        maxLat = Math.max(maxLat, coord[1]);
      }
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
  let ppm = 0.15;
  if (lu?.startsWith('4')) ppm = 0.5;
  if (lu==='48') ppm = 0.1;
  if (lu==='56') ppm = 0.2;
  if (lu==='52') ppm = 0.3;
  if (lu?.startsWith('7') || lu?.startsWith('8')) ppm = 0.05;
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
  } else toast('🏴 Gekauft für '+res.price+'🪙!','ok');
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
  toast('🌿 Umgewandelt! +'+res.xp_reward+'⚡','ok');
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
  G.treasures = G.treasures.filter(tr=>tr.id!==t.id);
  // First treasure unlocks the giant trees (enhanced mode)
  if (!G.tallUnlocked) {
    G.tallUnlocked = true;
    if (allTallTrees().length > 0) {
      setTimeout(() => toast('🌲 Gerücht: Irgendwo hier steht ein Riesenbaum... Finde und tippe ihn an!', 'ok'), 1200);
    }
  }
  render(); loadChallenges();
}

document.getElementById('popup-close').onclick = () => {
  document.getElementById('parcel-popup').classList.remove('open');
  resetPopupPosition('parcel-popup');
  G.sel=null; render();
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
  const treeAnim = G.tallUnlocked && Object.keys(G.topTrees).length > 0;
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
