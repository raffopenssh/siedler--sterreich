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
    url += '#v=' + G.cam.lon.toFixed(5) + ',' + G.cam.lat.toFixed(5) + ',' + G.cam.zoom;
  }
  return url;
}

/** Parse view hash params from URL (e.g. #v=15.07200,47.06400,18) */
function parseViewHash() {
  const h = location.hash;
  const m = h.match(/v=([\d.]+),([\d.]+),(\d+)/);
  if (m) return { lon: parseFloat(m[1]), lat: parseFloat(m[2]), zoom: parseInt(m[3]) };
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

// ================= WELCOME =================
{
  const inp = document.getElementById('input-name');
  const err = document.getElementById('welcome-error');
  const saved = localStorage.getItem('pid');
  const savedName = localStorage.getItem('pname');

  // Pre-fill with a random name suggestion
  if (!saved) inp.value = randomName();
  document.getElementById('btn-reroll').onclick = () => { inp.value = randomName(); inp.focus(); };

  if (saved && savedName) {
    document.getElementById('quick-rejoin').innerHTML =
      `Zuletzt: <a onclick="quickLogin()">${esc(savedName)}</a> — <a onclick="quickLogin()">Weiterspielen</a>`;
    // Load active session to show invite link
    GET('/api/player/'+saved+'/sessions').then(sessions => {
      if (sessions?.length > 0) {
        const s = sessions[0];
        const iUrl = location.origin + '/join/' + s.invite_code;
        document.getElementById('quick-rejoin').innerHTML =
          `<div>Zuletzt: <a onclick="quickLogin()">${esc(savedName)}</a> — <a onclick="quickLogin()">Weiterspielen</a></div>` +
          `<div class="invite-share">`+
            `<span class="invite-label">⚔️ Freunde einladen:</span> `+
            `<span class="invite-link" onclick="navigator.clipboard.writeText('${iUrl}');this.textContent='📋 Kopiert!';setTimeout(()=>this.textContent='${iUrl}',2000)">${iUrl}</span>`+
          `</div>`;
      }
    }).catch(()=>{});
  }

  // Check invite in URL
  const inviteMatch = location.pathname.match(/\/join\/(.+)/);
  if (inviteMatch) localStorage.setItem('invite', inviteMatch[1]);

  document.getElementById('btn-register').onclick = async () => {
    const name = inp.value.trim();
    if (name.length < 2) { err.textContent='Mindestens 2 Zeichen!'; return; }
    const res = await POST('/api/register', {name});
    if (res.error) { err.textContent=res.error; return; }
    savePlayer(res.player);
    // Store rejoin URL for later (shown in-game sidebar)
    localStorage.setItem('rejoin_url', location.origin + res.rejoin_url);
    toast('🎉 Willkommen, ' + res.player.name + '!', 'ok');
    show('pick');
  };

  document.getElementById('btn-login').onclick = async () => {
    const name = inp.value.trim();
    if (!name) { err.textContent='Name eingeben!'; return; }
    const res = await POST('/api/login', {name});
    if (res.error) { err.textContent=res.error; return; }
    savePlayer(res.player);
    const sessions = await GET('/api/player/'+res.player.id+'/sessions');
    if (sessions?.length > 0) { G.session = sessions[0]; startGame(); }
    else show('pick');
  };

  inp.addEventListener('keydown', e => { if(e.key==='Enter') document.getElementById('btn-register').click(); });
}

window.quickLogin = async function() {
  const id = localStorage.getItem('pid');
  if (!id) return;
  try {
    const p = await GET('/api/player/'+id);
    if (p.error) { localStorage.clear(); return; }
    G.player = p;
    const sessions = await GET('/api/player/'+id+'/sessions');
    if (sessions?.length > 0) { G.session = sessions[0]; startGame(); }
    else show('pick');
  } catch(e) { localStorage.clear(); }
};

function savePlayer(p) {
  G.player = p;
  localStorage.setItem('pid', p.id);
  localStorage.setItem('pname', p.name);
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

  if (G.pick.level === 'states' && pickData.allMunis) {
    // Draw municipality dots colored by state
    for (const m of pickData.allMunis) {
      if (!m.lon || !m.lat) continue;
      const [x, y] = pickProject(m.lon, m.lat);
      if (x < -5 || x > W+5 || y < -5 || y > H+5) continue;
      const isHover = pickData.hoverMuni === m;
      ctx.fillStyle = isHover ? '#ffd700' : (stateColors[m.state] || '#888');
      const sz = isHover ? 5 : 3;
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
      drawMuniPoly(ctx, f, f === pickData.hover);
    }
  }
}

function drawMuniPoly(ctx, feature, isHover) {
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

function simpleHash(s) { let h=0; for(let i=0;i<s.length;i++) h=((h<<5)-h)+s.charCodeAt(i); return h; }

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

  // Check pending invite
  const inv = localStorage.getItem('invite');
  if (inv) {
    localStorage.removeItem('invite');
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
  show('loading');
  document.getElementById('loading-muni').textContent = '📍 ' + m.name + ' (' + m.code + ')';
  // Reset all steps
  ['ls-session','ls-parcels','ls-kg','ls-treasures','ls-ready'].forEach(id => setLoadStep(id, ''));
  setLoadStep('ls-session', 'active');
  setLoadProgress(5);
  startTipRotation();

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

  // Now load game data with progress
  await startGameWithLoading();
}

function setLoadStep(id, state) {
  const el = document.getElementById(id);
  if (!el) return;
  el.classList.remove('active','done');
  if (state) el.classList.add(state);
}

function setLoadProgress(pct) {
  const bar = document.getElementById('loading-bar');
  if (bar) bar.style.width = Math.min(100, pct) + '%';
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

  // Step 2: Load parcels
  setLoadStep('ls-parcels', 'active');
  setLoadProgress(20);
  await loadParcels();
  setLoadStep('ls-parcels', 'done');
  setLoadProgress(40);

  // Step 3: Load KG polygons
  setLoadStep('ls-kg', 'active');
  setLoadProgress(45);
  await fetchKGPolygonsBlocking();
  buildEZIndex();
  // If parcels were empty (bbox failed) but we loaded polygon data, synthesize point parcels
  if (G.parcels.length === 0 && G.parcelPolys.length > 0) {
    for (const f of G.parcelPolys) {
      const p = f.properties;
      const c = polyCentroid(f.geometry.coordinates[0]);
      G.parcels.push({type:'Feature', properties:{...p, lon:c[0], lat:c[1]}, geometry:{type:'Point', coordinates:c}});
    }
  }
  setLoadStep('ls-kg', 'done');
  setLoadProgress(65);

  // Step 4: Load treasures, challenges, etc.
  setLoadStep('ls-treasures', 'active');
  setLoadProgress(70);
  await Promise.all([loadClaimed(), loadTreasures(), loadChallenges(), loadPlayers(), loadBio(), loadChat()]);
  setLoadStep('ls-treasures', 'done');
  setLoadProgress(85);

  // Step 5: Render
  setLoadStep('ls-ready', 'active');
  setLoadProgress(90);

  // Pre-generate grass pattern
  createGrassPattern();
  connectSSE();

  setLoadStep('ls-ready', 'done');
  setLoadProgress(100);

  // Ensure loading screen shows for at least 4s so users can read tips
  const elapsed = Date.now() - (G.loadStart || 0);
  const minWait = Math.max(800, 4000 - elapsed);
  await new Promise(r => setTimeout(r, minWait));
  stopTipRotation();
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

  // Show rejoin link in sidebar if available
  const rejoinUrl = localStorage.getItem('rejoin_url');
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

/** Fetch all pages of a KG layer via /api/kg/{code}?layer=...&page=N */
async function fetchKGLayer(kg, layer) {
  const features = [];
  let page = 0;
  while (true) {
    const data = await GET('/api/kg/'+kg+'?layer='+layer+'&page='+page+'&pagesize=200');
    if (data.features) for (const f of data.features) features.push(f);
    if (!data.has_more) break;
    page++;
  }
  return features;
}

async function fetchKGPolygonsBlocking() {
  // Find KG codes from loaded parcels and fetch real polygon geometries
  const kgs = new Set();
  for (const f of G.parcels) {
    const kg = f.properties.kg_code;
    if (kg && !G.kgsLoaded.has(kg)) kgs.add(kg);
  }
  // Also include KGs discovered via municipality fallback
  if (G.municipalityKGs) {
    for (const kg of G.municipalityKGs) kgs.add(kg);
    G.municipalityKGs = null;
  }
  const total = kgs.size || 1;
  let done = 0;
  const promises = [];
  for (const kg of kgs) {
    G.kgsLoaded.add(kg);
    promises.push(
      Promise.all([
        fetchKGLayer(kg, 'parcels'),
        fetchKGLayer(kg, 'building_footprints'),
        fetchKGLayer(kg, 'landuse'),
      ]).then(([parcels, footprints, landuse]) => {
        for (const f of parcels) G.parcelPolys.push(f);
        for (const f of footprints) G.buildingFootprints.push(f);
        for (const f of landuse) {
          if (f.geometry?.type === 'Polygon' || f.geometry?.type === 'MultiPolygon') {
            G.landusePolys.push(f);
          }
        }
        done++;
        setLoadProgress(45 + Math.floor((done/total)*20));
      }).catch(e => { console.error('KG fetch failed:', kg, e); done++; })
    );
  }
  await Promise.all(promises);
}

async function fetchKGPolygons() {
  // Incremental version for panning
  const kgs = new Set();
  for (const f of G.parcels) {
    const kg = f.properties.kg_code;
    if (kg && !G.kgsLoaded.has(kg)) kgs.add(kg);
  }
  for (const kg of kgs) {
    G.kgsLoaded.add(kg);
    try {
      const [parcels, footprints, landuse] = await Promise.all([
        fetchKGLayer(kg, 'parcels'),
        fetchKGLayer(kg, 'building_footprints'),
        fetchKGLayer(kg, 'landuse'),
      ]);
      for (const f of parcels) G.parcelPolys.push(f);
      for (const f of footprints) G.buildingFootprints.push(f);
      for (const f of landuse) {
        if (f.geometry?.type === 'Polygon' || f.geometry?.type === 'MultiPolygon') {
          G.landusePolys.push(f);
        }
      }
      render();
      renderMini();
    } catch(e) { console.error('KG fetch failed:', kg, e); }
  }
}

async function loadClaimed() { G.claimed = await GET('/api/session/'+G.session.id+'/parcels') || []; updateParcelCount(); }

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

  // ---- Trees on forest parcels ----
  drawForestSprites(ctx, claimMap);

  // ---- Draw real building footprints ----
  if (G.buildingFootprints.length > 0) drawBuildingFootprints(ctx);

  // ---- Treasures ----
  for (const t of G.treasures) drawTreasure(ctx, t);

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

    // 3D roof offset scales with building size
    const roofOff = Math.max(2, Math.min(8, Math.sqrt(area) * 0.12));

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

    // Windows (if building is large enough)
    if (bw > 12 && bh > 10 && zoom >= 17) {
      const cx = (minX+maxX)/2;
      const cy = (minY+maxY)/2 - roofOff;
      ctx.fillStyle = '#e8d880';
      const ws = Math.max(1.5, Math.min(3, bw*0.06));
      const gap = ws * 3;
      const count = Math.min(5, Math.floor(bw / (gap)));
      const startX = cx - (count-1)*gap/2;
      for (let i=0; i<count; i++) {
        ctx.fillRect(Math.round(startX + i*gap - ws/2), Math.round(cy - ws/2), ws, ws);
      }
    }

    // Door on larger buildings
    if (bw > 14 && bh > 12 && zoom >= 17) {
      ctx.fillStyle = '#3a2818';
      const dw = Math.max(2, Math.min(3.5, bw*0.08));
      const dh = dw * 1.4;
      ctx.fillRect(Math.round((minX+maxX)/2 - dw/2), Math.round(maxY - roofOff - dh - 1), dw, dh);
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
  ctx.fillStyle = terrain[Math.abs(hash) % terrain.length];
  // More transparent when real landuse polys provide terrain backdrop
  ctx.globalAlpha = G.landusePolys.length > 0 ? 0.35 : 0.85;
  ctx.fill();
  ctx.globalAlpha = 1;

  // Border - thin dark line like terrain boundaries in Settlers
  ctx.strokeStyle = claim ? (G.pcolors[claim.player_id]||'#fff') : 'rgba(20,40,10,0.35)';
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

function drawForestSprites(ctx, claimMap) {
  // Draw little tree sprites on forest parcels
  const polys = G.parcelPolys.filter(f => {
    const t = getParcelTerrain(f.properties, claimMap[f.properties.parcel_id]);
    return t === TERRAIN.forest;
  });

  ctx.save();
  for (const f of polys) {
    const coords = f.geometry.coordinates[0];
    const b = geoBounds(f.geometry);
    const [sx1,sy1] = toScreen(b.w, b.n);
    const [sx2,sy2] = toScreen(b.e, b.s);
    if (sx2 < 0 || sx1 > gc.width || sy2 < 0 || sy1 > gc.height) continue;

    // Scatter trees
    const area = f.properties.area_sqm || 1000;
    const treeCount = Math.min(15, Math.max(2, Math.floor(area / 500)));
    const hash = simpleHash(f.properties.parcel_id||'');

    for (let i = 0; i < treeCount; i++) {
      const t = (hash + i * 7919) % 10000 / 10000;
      const u = (hash + i * 3571) % 10000 / 10000;
      const lon = b.w + (b.e - b.w) * t;
      const lat = b.s + (b.n - b.s) * u;
      if (!pip(lon, lat, coords)) continue;
      const [tx, ty] = toScreen(lon, lat);
      drawTree(ctx, tx, ty, (hash+i) % 3);
    }
  }
  ctx.restore();
}

function drawTree(ctx, x, y, variant) {
  // Settlers IV style conifer - layered triangles
  const scale = G.cam.zoom > 16 ? 1.2 : 0.8;
  x = Math.round(x);
  y = Math.round(y);

  // Shadow
  ctx.fillStyle = 'rgba(0,0,0,0.15)';
  ctx.beginPath();
  ctx.ellipse(x+3, y+2, 5*scale, 2*scale, 0, 0, Math.PI*2);
  ctx.fill();

  // Trunk
  ctx.fillStyle = '#5a3a20';
  ctx.fillRect(x-1*scale, y-4*scale, 2*scale, 5*scale);

  if (variant === 0) {
    // Pine tree - dark pointed
    ctx.fillStyle = '#1a5a1a';
    drawTriangle(ctx, x, y-18*scale, 8*scale, 8*scale);
    ctx.fillStyle = '#226622';
    drawTriangle(ctx, x, y-13*scale, 10*scale, 8*scale);
    ctx.fillStyle = '#2a7a2a';
    drawTriangle(ctx, x, y-8*scale, 12*scale, 8*scale);
  } else if (variant === 1) {
    // Deciduous - round
    ctx.fillStyle = '#2a7a2a';
    ctx.beginPath();
    ctx.arc(x, y-12*scale, 7*scale, 0, Math.PI*2);
    ctx.fill();
    ctx.fillStyle = '#3a8a3a';
    ctx.beginPath();
    ctx.arc(x-2, y-14*scale, 5*scale, 0, Math.PI*2);
    ctx.fill();
  } else {
    // Bush
    ctx.fillStyle = '#3a8a3a';
    ctx.beginPath();
    ctx.arc(x, y-6*scale, 6*scale, 0, Math.PI*2);
    ctx.fill();
    ctx.fillStyle = '#4a9a4a';
    ctx.beginPath();
    ctx.arc(x+2, y-8*scale, 4*scale, 0, Math.PI*2);
    ctx.fill();
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
  resetPopupPosition('parcel-popup');
  resetPopupPosition('ez-popup');
  G.sel = null; G.ezHighlight = null; render();
}

function showParcelPopup(f) {
  G.sel = f;
  const p = f.properties;
  const pid = p.parcel_id;

  // Smooth zoom to parcel (center on it, zoom to ~18 if further out)
  const pLon = p.lon || (f.geometry.type === 'Polygon' ? centroidOf(f.geometry.coordinates[0])[0] : f.geometry.coordinates[0]);
  const pLat = p.lat || (f.geometry.type === 'Polygon' ? centroidOf(f.geometry.coordinates[0])[1] : f.geometry.coordinates[1]);
  const targetZoom = Math.max(G.cam.zoom, 17.5);
  animateCamera(pLon, pLat, targetZoom, 400);
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

  const act = document.getElementById('pp-actions');
  act.innerHTML = '';
  if (!claim) {
    act.innerHTML = `<button class="btn btn-primary btn-small" onclick="doClaim()">🏴 Kaufen (${price}🪙)</button>`;
  } else if (claim.player_id === G.player.id && !claim.converted_to) {
    act.innerHTML = `
      <button class="btn btn-primary btn-small" onclick="doConvert('biodiversity')">🌿 Naturschutz</button>
      <button class="btn btn-secondary btn-small" onclick="doConvert('forest')">🌳 Aufforsten</button>
      <button class="btn btn-danger btn-small" onclick="doSell(${claim.id})">💰 Verkaufen</button>`;
  } else if (claim.player_id === G.player.id) {
    act.innerHTML = `<span style="font:18px VT323;color:var(--green-light)">✅ ${claim.converted_to}</span>`;
  }

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
  // Position parcel popup at bottom-left
  const pp = document.getElementById('parcel-popup');
  if (!pp.dataset.userMoved) {
    pp.style.left = '16px'; pp.style.bottom = '16px';
    pp.style.right = ''; pp.style.top = '';
  }
  render();
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
  const targetZoom = Math.max(15, Math.min(19, Math.min(lonZoom, latZoom)));
  // Animate to show entire EZ
  animateCamera(centerLon, centerLat, targetZoom, 500);

  const popup = document.getElementById('ez-popup');
  popup.classList.add('open');
  // Position to the right of parcel popup if not manually moved
  if (!popup.dataset.userMoved) {
    const ppEl = document.getElementById('parcel-popup');
    const ppRect = ppEl.getBoundingClientRect();
    // On mobile, position at top; on desktop, position to the right
    if (window.innerWidth < 768) {
      popup.style.left = '8px';
      popup.style.top = '60px';
      popup.style.right = ''; popup.style.bottom = '';
    } else {
      popup.style.left = (ppRect.right + 12) + 'px';
      popup.style.bottom = '16px';
      popup.style.right = ''; popup.style.top = '';
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
  const res = await POST('/api/claim-parcel', {
    session_id:G.session.id, player_id:G.player.id,
    parcel_id:p.parcel_id, kg_code:p.kg_code||'', gnr:p.gnr||'',
    ez:p.ez||'',
    area_sqm:p.area_sqm||0, landuse:extractLuCode('',p),
    building_count:p.building_count||0, total_building_area:p.total_building_area_sqm||0,
  });
  if (res.error) { toast(res.error,'err'); return; }
  toast('🏴 Gekauft für '+res.price+'🪙!','ok');
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

async function claimTreasure(t) {
  const res = await POST('/api/claim-treasure', {player_id:G.player.id, treasure_id:t.id});
  if (res.error) { toast(res.error,'err'); return; }
  const emoji = {xp:'⚡',rare_seed:'🌱',ancient_map:'🗺️',coins:'🪙'}[res.type]||'🪙';
  toast('💎 Schatz! +'+res.value+emoji,'ok');
  G.player = res.player; updateStats();
  G.treasures = G.treasures.filter(tr=>tr.id!==t.id);
  render(); loadChallenges();
}

document.getElementById('popup-close').onclick = () => {
  document.getElementById('parcel-popup').classList.remove('open');
  document.getElementById('ez-popup').classList.remove('open');
  resetPopupPosition('parcel-popup');
  resetPopupPosition('ez-popup');
  G.sel=null; G.ezHighlight=null; render();
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
  if (el) { delete el.dataset.userMoved; }
}

// Sparkle animation for treasures
setInterval(() => {
  if (document.getElementById('screen-game').classList.contains('active') &&
      G.treasures.length>0) render();
}, 800);

// Auto-refresh
setInterval(async () => {
  if (!G.session||!G.player) return;
  try { const p = await GET('/api/player/'+G.player.id); if(!p.error){G.player=p;updateStats();} } catch(e){}
}, 15000);

// ---- Init picker when shown ----
const pickObs = new MutationObserver(() => {
  if (document.getElementById('screen-pick').classList.contains('active') && !pickCanvas) initPicker();
});
pickObs.observe(document.getElementById('screen-pick'), {attributes:true, attributeFilter:['class']});
