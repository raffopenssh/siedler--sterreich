# game.js Full Code Analysis

File: `/home/exedev/siedler/srv/static/game.js` — 2396 lines

---

## 1. Google Maps Link Generation (Lines 1982-1988)

```javascript
// Line 1982-1988
document.getElementById('btn-gearth').onclick = () => {
  // Open Google Maps satellite view at current camera position
  // Map game zoom (13-20) to Google Maps distance: z13≈8000m, z20≈50m
  const dist = Math.round(8000 / Math.pow(2, G.cam.zoom - 13));
  const url = 'https://www.google.com/maps/@'+G.cam.lat.toFixed(6)+','+G.cam.lon.toFixed(6)+','+dist+'m/data=!3m1!1e3';
  window.open(url, '_blank');
};
```

This is the **only** Google Maps reference. It's on a button `btn-gearth` in the zoom controls area. It converts the game zoom (13–20) to a Google Maps distance parameter and opens satellite view (`!3m1!1e3`).

---

## 2. LANDUSE_TERRAIN and LANDUSE_POLY_COLORS Maps

### TERRAIN base colors (Lines 10-22)

```javascript
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
```

### LANDUSE_TERRAIN (Lines 24-34) — maps numeric landuse codes → TERRAIN color arrays

```javascript
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
```

### LANDUSE_NAMES (Lines 36-42)

```javascript
const LANDUSE_NAMES = {
  '40':'Baugrün','41':'Baufläche','42':'Gebäude','43':'Keller','44':'Ruine','45':'Gewächshaus',
  '48':'Verkehr','50':'Acker','51':'Acker','52':'Wiese','53':'Weide','54':'Grünland',
  '55':'Alpe','56':'Wald','57':'Krummholz','58':'Wald','60':'Weingarten','62':'Garten',
  '63':'Obstgarten','70':'Gewässer','71':'Bach','72':'See','73':'Fluss',
  '80':'Ödland','83':'Sumpf','84':'Gletscher','85':'Fels',
};
```

### ABBR_MAP (Lines 45-75) — maps landuse_summary abbreviations → {terrain, code, name}

```javascript
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
```

### LANDUSE_POLY_COLORS (Lines 1480-1502) — for real landuse polygon rendering

```javascript
const LANDUSE_POLY_COLORS = {
  '40': {fill:'#5a8a40', stroke:'#4a7a30'},     // Baufläche begrünt
  '42': {fill:'#8b7058', stroke:'#6a5040'},     // Gebäude
  '48': {fill:'#b0a898', stroke:'#908880', a:0.7},  // Verkehr (roads)
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
  '92': {fill:'#6a9a5a', stroke:'#5a8a4a'},     // Hochalm
  '96': {fill:'#2888c8', stroke:'#1878b8', a:0.8}, // Gewässer — vivid blue
};
const LANDUSE_POLY_DEFAULT = {fill:'#5a8a40', stroke:'#4a7a30'};
```

---

## 3. Tree/Forest Sprite Drawing Code

### drawForestSprites (Lines 1665-1696)

```javascript
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
```

### drawTree (Lines 1698-1743)

```javascript
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
```

### drawTriangle helper (Lines 1745-1752)

```javascript
function drawTriangle(ctx, cx, top, w, h) {
  ctx.beginPath();
  ctx.moveTo(cx, top);
  ctx.lineTo(cx - w/2, top + h);
  ctx.lineTo(cx + w/2, top + h);
  ctx.closePath();
  ctx.fill();
}
```

---

## 4. EZ Highlight Drawing Code

### drawEZHighlight (Lines 1781-1827)

```javascript
function drawEZHighlight(ctx) {
  const key = G.ezHighlight.kg + '-EZ' + G.ezHighlight.ez;
  const parcels = G.ezIndex[key] || [];
  if (parcels.length < 2) return;
  const selId = G.sel?.properties?.parcel_id;
  ctx.save();
  // Pulse animation based on time
  const pulse = 0.3 + 0.15 * Math.sin(Date.now() / 400);
  for (const f of parcels) {
    if (f.properties.parcel_id === selId) continue; // skip the selected one
    if (f.geometry.type === 'Polygon') {
      const pts = f.geometry.coordinates[0].map(c => toScreen(c[0], c[1]));
      let minX=Infinity, maxX=-Infinity, minY=Infinity, maxY=-Infinity;
      for (const pt of pts) {
        if (pt[0]<minX) minX=pt[0]; if (pt[0]>maxX) maxX=pt[0];
        if (pt[1]<minY) minY=pt[1]; if (pt[1]>maxY) maxY=pt[1];
      }
      if (maxX < -50 || minX > gc.width+50 || maxY < -50 || minY > gc.height+50) continue;
      ctx.beginPath();
      pts.forEach((pt,i) => i===0 ? ctx.moveTo(pt[0],pt[1]) : ctx.lineTo(pt[0],pt[1]));
      ctx.closePath();
      // Fill with semi-transparent gold
      ctx.fillStyle = 'rgba(212,168,67,' + pulse + ')';
      ctx.fill();
      // Gold dashed border
      ctx.strokeStyle = '#d4a843';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 3]);
      ctx.stroke();
      ctx.setLineDash([]);
    } else {
      const p = f.properties;
      const [x, y] = toScreen(p.lon || f.geometry.coordinates[0], p.lat || f.geometry.coordinates[1]);
      if (x < -30 || x > gc.width+30 || y < -30 || y > gc.height+30) continue;
      const sz = Math.max(8, Math.min(30, Math.sqrt(p.area_sqm||100) * mapScale() / 80000));
      ctx.fillStyle = 'rgba(212,168,67,' + pulse + ')';
      ctx.fillRect(x-sz/2, y-sz/2, sz, sz);
      ctx.strokeStyle = '#d4a843';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 3]);
      ctx.strokeRect(x-sz/2, y-sz/2, sz, sz);
      ctx.setLineDash([]);
    }
  }
  ctx.restore();
}
```

### Pulse animation timer (Lines 1981-1984)

```javascript
// Sparkle animation + EZ pulse
setInterval(() => {
  if (document.getElementById('screen-game').classList.contains('active') &&
      (G.treasures.length>0 || G.ezHighlight)) render();
}, 800);
```

---

## 5. Parcel Selection Code

### G.sel state (Line 137)

```javascript
sel: null, // selected parcel feature
```

### Click handler — onGameClick (Lines 1744-1775 / actual lines 2144-2175)

```javascript
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
  G.sel = null; G.ezHighlight = null; render();
}
```

### drawSelection (Lines 1829-1850)

```javascript
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
```

### Popup close button (Lines 1976-1978)

```javascript
document.getElementById('popup-close').onclick = () => {
  document.getElementById('parcel-popup').classList.remove('open'); G.sel=null; G.ezHighlight=null; render();
};
```

---

## 6. Tooltip/Panel Code — showParcelPopup (Lines 1777-1874 / actual 2177-2274)

```javascript
function showParcelPopup(f) {
  G.sel = f;
  const p = f.properties;
  const pid = p.parcel_id;
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

  // EZ info panel
  const ezInfo = document.getElementById('pp-ez-info');
  const ezStats = document.getElementById('pp-ez-stats');
  const ezAct = document.getElementById('pp-ez-actions');
  if (ez && p.kg_code) {
    const ezKey = p.kg_code + '-EZ' + ez;
    const ezParcels = G.ezIndex[ezKey] || [];
    if (ezParcels.length > 1) {
      const claimMap = {};
      for (const c of G.claimed) claimMap[c.parcel_id] = c;
      const totalArea = ezParcels.reduce((s, pf) => s + (pf.properties.area_sqm || 0), 0);
      const unclaimed = ezParcels.filter(pf => !claimMap[pf.properties.parcel_id]);
      const myCount = ezParcels.filter(pf => claimMap[pf.properties.parcel_id]?.player_id === G.player.id).length;
      const areaStr = totalArea > 10000 ? (totalArea/10000).toFixed(2)+' ha' : Math.round(totalArea)+' m²';
      ezStats.innerHTML = `
        <span>Parzellen</span><b>${ezParcels.length} (${unclaimed.length} frei)</b>
        <span>Gesamtfläche</span><b>${areaStr}</b>
        <span>Dein Besitz</span><b>${myCount} / ${ezParcels.length}</b>`;
      ezAct.innerHTML = '';
      if (unclaimed.length > 0) {
        let totalPrice = 0;
        for (const pf of unclaimed) {
          const pp = pf.properties;
          totalPrice += calcPrice(pp.area_sqm||0, extractLuCode('',pp), pp.building_count||0, pp.total_building_area_sqm||0);
        }
        const discountedPrice = Math.round(totalPrice * 0.8);
        const savings = totalPrice - discountedPrice;
        ezAct.innerHTML = `<button class="btn btn-gold btn-small" style="margin-top:8px;width:100%" onclick="doClaimEZ('${p.kg_code}','${ez}')">📋 Ganze EZ kaufen: ${discountedPrice}🪙 <span style='font-size:14px;color:#2a2'>(−20% = −${savings}🪙)</span></button>`;
      }
      ezInfo.style.display = '';
      // Set EZ highlight for rendering
      G.ezHighlight = {kg: p.kg_code, ez: ez};
    } else {
      ezInfo.style.display = 'none';
      G.ezHighlight = null;
    }
  } else {
    ezInfo.style.display = 'none';
    G.ezHighlight = null;
  }

  document.getElementById('parcel-popup').classList.add('open');
  render();
}
```

The popup targets these DOM elements:
- `parcel-popup` (container, toggled via `.open` class)
- `popup-close` (close button)
- `pp-title`, `pp-id`, `pp-kg`, `pp-ez`, `pp-area`, `pp-use`, `pp-density`, `pp-owner`, `pp-price`
- `pp-actions` (buy/convert/sell buttons)
- `pp-ez-info`, `pp-ez-stats`, `pp-ez-actions` (EZ group panel)

---

## 7. Smooth Zoom/Animation Code

### flyTo animation (Lines 2008-2027 — the FLY-TO ANIMATION section)

```javascript
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
```

Used by the in-game search (when you click a search result, it calls `flyTo(parseFloat(a.lon), parseFloat(a.lat), 18)`).

**Note**: There is NO smooth zoom on scroll wheel — wheel zoom is instant:
```javascript
// Line 1940-1947
gc.addEventListener('wheel', e => {
  e.preventDefault();
  G.cam.zoom += e.deltaY > 0 ? -0.4 : 0.4;
  G.cam.zoom = Math.max(13, Math.min(20, G.cam.zoom));
  render(); renderMini();
  clearTimeout(loadTimer);
  loadTimer = setTimeout(loadMoreParcels, 600);
}, {passive:false});
```

Zoom buttons are also instant:
```javascript
// Lines 1979-1981
document.getElementById('btn-zoomin').onclick = () => { G.cam.zoom=Math.min(20,G.cam.zoom+0.5); render(); renderMini(); };
document.getElementById('btn-zoomout').onclick = () => { G.cam.zoom=Math.max(13,G.cam.zoom-0.5); render(); renderMini(); };
```

---

## 8. The render() Function — Full Draw Order (Lines 1389-1438)

```javascript
function render() {
  if (!gctx) return;
  const ctx = gctx;
  const W = gc.width, H = gc.height;

  // ---- 1. Background terrain ----
  ctx.fillStyle = '#3a6828';
  ctx.fillRect(0, 0, W, H);
  drawGrassTexture(ctx, W, H);              // Pre-generated tiled grass pattern

  // Build claim lookup
  const claimMap = {};
  for (const c of G.claimed) claimMap[c.parcel_id] = c;

  // ---- 2. Real landuse polygons (forests, water, roads, etc.) ----
  if (G.landusePolys.length > 0) drawLandusePolygons(ctx);

  // ---- 3. Parcel polygons (from export/geojson KG data) ----
  if (G.parcelPolys.length > 0) {
    for (const f of G.parcelPolys) {
      drawParcelPoly(ctx, f, claimMap);
    }
  }

  // ---- 4. Point parcels (fallback if no polygon) ----
  const polyIds = new Set(G.parcelPolys.map(f=>f.properties.parcel_id));
  for (const f of G.parcels) {
    if (!polyIds.has(f.properties.parcel_id)) {
      drawParcelPoint(ctx, f, claimMap);
    }
  }

  // ---- 5. Trees on forest parcels ----
  drawForestSprites(ctx, claimMap);

  // ---- 6. Real building footprints ----
  if (G.buildingFootprints.length > 0) drawBuildingFootprints(ctx);

  // ---- 7. Treasures ----
  for (const t of G.treasures) drawTreasure(ctx, t);

  // ---- 8. EZ group highlight (all parcels in same EZ) ----
  if (G.ezHighlight) drawEZHighlight(ctx);

  // ---- 9. Selected parcel highlight ----
  if (G.sel) drawSelection(ctx, G.sel);

  // ---- 10. Scale bar ----
  drawScaleBar(ctx, W, H);
}
```

### Draw order summary:
1. Green background fill (`#3a6828`)
2. Tiled grass texture pattern (128×128 pre-generated canvas)
3. Real landuse polygons (`G.landusePolys`) — forests, water, roads etc. at alpha 0.55
4. Parcel polygons (`G.parcelPolys`) — at alpha 0.35 (with landuse) or 0.85 (without)
5. Point parcels (`G.parcels`) — only for parcels lacking polygon geometry
6. Forest tree sprites — scattered on TERRAIN.forest parcels
7. Real building footprints — 3D extruded with roof/walls/shadow/windows/doors
8. Treasure chests — animated sparkles
9. EZ highlight — pulsing gold overlay on sibling parcels
10. Selection highlight — gold dashed stroke with glow
11. Scale bar — bottom-left

---

## Additional Key Functions

### getParcelTerrain (Lines 1601-1608)
```javascript
function getParcelTerrain(p, claim) {
  if (claim?.converted_to) return TERRAIN.bio;
  if (p.landuse_summary) {
    const parsed = parseLanduseSummary(p.landuse_summary);
    if (parsed.dominant) return parsed.dominant.terrain;
  }
  const luCode = extractLuCode('', p);
  return LANDUSE_TERRAIN[luCode] || TERRAIN.grass;
}
```

### calcPrice (Lines 1876-1894)
```javascript
function calcPrice(area, lu, buildingCount, totalBuildingArea) {
  let ppm = 0.15;
  if (lu?.startsWith('4')) ppm = 0.5;
  if (lu==='48') ppm = 0.1;
  if (lu==='56') ppm = 0.2;
  if (lu==='52') ppm = 0.3;
  if (lu?.startsWith('7') || lu?.startsWith('8')) ppm = 0.05;
  let densityMult = 1.0;
  if (area > 0 && totalBuildingArea > 0) {
    const builtRatio = totalBuildingArea / area;
    if (builtRatio > 0.3) densityMult = 2.0;
    else if (builtRatio > 0.05) densityMult = 1.0 + (builtRatio - 0.05) / 0.25;
    else densityMult = 0.5 + builtRatio / 0.05 * 0.5;
  } else if (buildingCount === 0) {
    densityMult = 0.5;
  }
  return Math.max(10, Math.min(5000, Math.round(area * ppm * densityMult)));
}
```

### mapScale and projection (Lines 1379-1387)
```javascript
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
```

### Game input binding (Lines 1917-2006)
Covers: mousedown/move/up drag panning, wheel zoom, touch (1-finger pan, 2-finger pinch zoom), keyboard shortcuts (C=chat, /=search, Escape=close), click → `onGameClick`.
