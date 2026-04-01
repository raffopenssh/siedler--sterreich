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
  building: ['#8b7058','#907560','#856b52','#957a65','#7e654c'],
  road:     ['#787870','#808078','#707068','#888880','#686860'],
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

const PLAYER_COLORS = ['#e04040','#4080e0','#e0c040','#a040e0','#40e0a0','#e08040','#e040a0','#40e040'];

// ---- Game State ----
const G = {
  player: null, session: null,
  parcels: [],          // from cadastre (point data)
  parcelPolys: [],      // from export/geojson (polygon data for current KGs)
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

// ================= WELCOME =================
{
  const inp = document.getElementById('input-name');
  const err = document.getElementById('welcome-error');
  const saved = localStorage.getItem('pid');
  const savedName = localStorage.getItem('pname');

  if (saved && savedName) {
    document.getElementById('quick-rejoin').innerHTML =
      `Zuletzt: <a onclick="quickLogin()">${esc(savedName)}</a> — <a onclick="quickLogin()">Weiterspielen</a>`;
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

function geoBounds(geom) {
  let w=Infinity,e=-Infinity,s=Infinity,n=-Infinity;
  const processCoord = c => { if(c[0]<w)w=c[0]; if(c[0]>e)e=c[0]; if(c[1]<s)s=c[1]; if(c[1]>n)n=c[1]; };
  const walk = a => { if(typeof a[0]==='number') processCoord(a); else a.forEach(walk); };
  walk(geom.coordinates);
  return {w,e,s,n};
}

// Pick canvas interactions
function onPickDown(ev) {
  G.pick.drag = { active:true, sx:ev.clientX, sy:ev.clientY, slon:G.pick.cam.lon, slat:G.pick.cam.lat, moved:false };
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
function onPickClick(ev) {
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
    if (best && bestD < 18) {
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
  const url = location.origin + '/join/' + inviteCode;
  const box = document.getElementById('invite-url');
  box.textContent = url;
  box.onclick = () => { navigator.clipboard.writeText(url); toast('📋 Kopiert!','ok'); };
  refreshLobby();
}

async function refreshLobby() {
  if (!G.session) return;
  const pl = await GET('/api/session/'+G.session.id+'/players');
  document.getElementById('lobby-players').innerHTML = (pl||[]).map(p => `<li>${esc(p.name)} (${p.coins}🪙)</li>`).join('');
}

// ================= MAIN GAME =================
let gc, gctx, mc, mctx;

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

  // Set camera to municipality center BEFORE loading parcels
  G.cam.lon = G.session.center_lon;
  G.cam.lat = G.session.center_lat;
  G.cam.zoom = 17;

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
    navigator.clipboard.writeText(location.origin+'/join/'+G.session.invite_code);
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
    fetchKGPolygons();
  } catch(e) { console.error(e); }
}

async function fetchKGPolygonsBlocking() {
  // Find KG codes from loaded parcels and fetch real polygon geometries
  const kgs = new Set();
  for (const f of G.parcels) {
    const kg = f.properties.kg_code;
    if (kg && !G.kgsLoaded.has(kg)) kgs.add(kg);
  }
  const total = kgs.size || 1;
  let done = 0;
  const promises = [];
  for (const kg of kgs) {
    G.kgsLoaded.add(kg);
    promises.push(
      GET(CAD+'/export/geojson?kg='+kg+'&layers=parcels').then(data => {
        if (data.features) {
          for (const f of data.features) G.parcelPolys.push(f);
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
      const data = await GET(CAD+'/export/geojson?kg='+kg+'&layers=parcels');
      if (data.features) {
        for (const f of data.features) G.parcelPolys.push(f);
        render();
        renderMini();
      }
    } catch(e) { console.error('KG fetch failed:', kg, e); }
  }
}

async function loadClaimed() { G.claimed = await GET('/api/session/'+G.session.id+'/parcels') || []; updateParcelCount(); }
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
}
function updateParcelCount() {
  const mine = G.claimed.filter(c=>c.player_id===G.player.id);
  document.getElementById('s-parcels').textContent = mine.length;
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
  // Settlers IV has lush green grass with variation
  ctx.fillStyle = '#3a6828';
  ctx.fillRect(0, 0, W, H);

  // Grass texture - noise pattern
  const s = mapScale();
  const b = viewBounds();
  drawGrassTexture(ctx, W, H);

  // Build claim lookup
  const claimMap = {};
  for (const c of G.claimed) claimMap[c.parcel_id] = c;

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

  // ---- Treasures ----
  for (const t of G.treasures) drawTreasure(ctx, t);

  // ---- Selected parcel highlight ----
  if (G.sel) drawSelection(ctx, G.sel);

  // ---- Trees on forest parcels ----
  drawForestSprites(ctx, claimMap);

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

function drawParcelPoly(ctx, f, claimMap) {
  const p = f.properties;
  const geom = f.geometry;
  if (!geom || geom.type !== 'Polygon') return;

  const coords = geom.coordinates[0];
  const parcelId = p.parcel_id;
  const claim = claimMap[parcelId];
  const lu = p.landuse_summary ? Object.keys(p.landuse_summary)[0] : '';
  const luCode = extractLuCode(lu, p);
  const terrain = (claim?.converted_to) ? TERRAIN.bio : (LANDUSE_TERRAIN[luCode] || TERRAIN.grass);

  // Project coordinates
  const pts = coords.map(c => toScreen(c[0], c[1]));

  // Check if visible
  const minX = Math.min(...pts.map(p=>p[0]));
  const maxX = Math.max(...pts.map(p=>p[0]));
  const minY = Math.min(...pts.map(p=>p[1]));
  const maxY = Math.max(...pts.map(p=>p[1]));
  if (maxX < -50 || minX > gc.width+50 || maxY < -50 || minY > gc.height+50) return;

  ctx.beginPath();
  for (let i=0; i<pts.length; i++) {
    i===0 ? ctx.moveTo(pts[i][0], pts[i][1]) : ctx.lineTo(pts[i][0], pts[i][1]);
  }
  ctx.closePath();

  // Settlers-style fill with variation
  const hash = simpleHash(parcelId || '');
  ctx.fillStyle = terrain[Math.abs(hash) % terrain.length];
  ctx.globalAlpha = 0.85;
  ctx.fill();
  ctx.globalAlpha = 1;

  // Border - thin dark line like terrain boundaries in Settlers
  ctx.strokeStyle = claim ? (G.pcolors[claim.player_id]||'#fff') : 'rgba(20,40,10,0.4)';
  ctx.lineWidth = claim ? 2 : 0.7;
  ctx.stroke();

  // Claimed: draw player flag
  if (claim) {
    const cx = (minX+maxX)/2, cy = (minY+maxY)/2;
    drawFlag(ctx, cx, cy, G.pcolors[claim.player_id]||'#fff', !!claim.converted_to);
  }
}

function extractLuCode(lu, p) {
  // Try to get a numeric code from various property formats
  if (p.landuse_codes) {
    const first = p.landuse_codes.split(',')[0].trim();
    if (first) return first;
  }
  if (p.dominant_landuse) return p.dominant_landuse;
  // Try parsing from summary key
  const match = lu.match(/(\d{2})/);
  if (match) return match[1];
  // Parse from landuse_summary text keys
  const st = p.landuse_summary ? Object.keys(p.landuse_summary).join(' ') : lu;
  if (st.includes('Wald') || st.includes(' W')) return '56';
  if (st.includes('Wiese') || st.includes('LN(W)')) return '52';
  if (st.includes('Acker') || st.includes('LN(A)')) return '50';
  if (st.includes('Weide') || st.includes('LN(Hu)')) return '53';
  if (st.includes('Baufläche') || st.includes('B(bf)') || st.includes('B(')  ) return '42';
  if (st.includes('Gebäude') || st.includes('Geb(')) return '43';
  if (st.includes('Garten') || st.includes('Ga')) return '62';
  if (st.includes('Verkehr') || st.includes('Straß') || st.includes('V(')) return '48';
  if (st.includes('Gewässer') || st.includes('Bach') || st.includes('See') || st.includes('Fl(')) return '70';
  if (st.includes('Alpe') || st.includes('Alm')) return '55';
  if (st.includes('Weingarten') || st.includes('WG')) return '60';
  if (st.includes('Fels') || st.includes('Geröll') || st.includes('Ödland') || st.includes('Fe')) return '85';
  if (st.includes('Sumpf') || st.includes('Moor')) return '83';
  if (st.includes('Grünland') || st.includes('LN')) return '52';
  return '';
}

function drawParcelPoint(ctx, f, claimMap) {
  const p = f.properties;
  const [x, y] = toScreen(p.lon || f.geometry.coordinates[0], p.lat || f.geometry.coordinates[1]);
  if (x < -30 || x > gc.width+30 || y < -30 || y > gc.height+30) return;

  const area = p.area_sqm || 100;
  const size = Math.max(6, Math.min(40, Math.sqrt(area) * mapScale() / 80000));
  const claim = claimMap[p.parcel_id];
  const luCode = (p.landuse_codes||'').split(',')[0].trim() || p.dominant_landuse || '';
  const terrain = (claim?.converted_to) ? TERRAIN.bio : (LANDUSE_TERRAIN[luCode] || TERRAIN.grass);
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
    const lu = extractLuCode('', f.properties);
    return lu === '56' || lu === '57' || lu === '58';
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
    const luCode = (p.landuse_codes||'').split(',')[0] || '';
    const t = cl?.converted_to ? TERRAIN.bio : (LANDUSE_TERRAIN[luCode]||TERRAIN.grass);
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
      G.drag = {active:true,sx:e.touches[0].clientX,sy:e.touches[0].clientY,slon:G.cam.lon,slat:G.cam.lat,moved:false};
    } else if (e.touches.length===2) {
      const dx=e.touches[0].clientX-e.touches[1].clientX, dy=e.touches[0].clientY-e.touches[1].clientY;
      touchDist = Math.sqrt(dx*dx+dy*dy);
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
  gc.addEventListener('touchend', () => { G.drag.active=false; clearTimeout(loadTimer); loadTimer=setTimeout(loadMoreParcels,600); });

  // Zoom buttons
  document.getElementById('btn-zoomin').onclick = () => { G.cam.zoom=Math.min(20,G.cam.zoom+0.5); render(); renderMini(); };
  document.getElementById('btn-zoomout').onclick = () => { G.cam.zoom=Math.max(13,G.cam.zoom-0.5); render(); renderMini(); };

  // Keyboard
  document.addEventListener('keydown', e => {
    if (e.target.tagName==='INPUT') return;
    if (e.key==='c'||e.key==='C') document.getElementById('input-chat').focus();
  });
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
  G.sel = null; render();
}

function showParcelPopup(f) {
  G.sel = f;
  const p = f.properties;
  const pid = p.parcel_id;
  const claim = G.claimed.find(c=>c.parcel_id===pid);
  const owner = claim ? G.players.find(pl=>pl.id===claim.player_id) : null;
  const luCode = extractLuCode('', p);
  const area = p.area_sqm||0;
  const price = calcPrice(area, luCode);

  document.getElementById('pp-title').textContent = '📍 ' + (p.gnr || pid);
  document.getElementById('pp-id').textContent = pid;
  document.getElementById('pp-kg').textContent = p.kg_name || p.kg_code || '-';
  document.getElementById('pp-area').textContent = area>10000?(area/10000).toFixed(2)+' ha':Math.round(area)+' m²';
  document.getElementById('pp-use').textContent = LANDUSE_NAMES[luCode] || luCode || '-';
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

  document.getElementById('parcel-popup').classList.add('open');
  render();
}

function calcPrice(area, lu) {
  let ppm = 0.15;
  if (lu?.startsWith('4')) ppm = 0.5;
  if (lu==='48') ppm = 0.1;
  if (lu==='56') ppm = 0.2;
  if (lu==='52') ppm = 0.3;
  return Math.max(10, Math.min(5000, Math.round(area * ppm)));
}

window.doClaim = async function() {
  if (!G.sel) return;
  const p = G.sel.properties;
  const res = await POST('/api/claim-parcel', {
    session_id:G.session.id, player_id:G.player.id,
    parcel_id:p.parcel_id, kg_code:p.kg_code||'', gnr:p.gnr||'',
    area_sqm:p.area_sqm||0, landuse:extractLuCode('',p),
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
  document.getElementById('parcel-popup').classList.remove('open'); G.sel=null;
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
  document.getElementById('parcel-popup').classList.remove('open'); G.sel=null; render();
};

// Sparkle animation
setInterval(() => {
  if (G.treasures.length>0 && document.getElementById('screen-game').classList.contains('active')) render();
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
