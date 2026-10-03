#!/usr/bin/env node
// Cross-browser state walker: drives the game through every screen / popup /
// overlay / animation in Chromium, Firefox and WebKit (Safari engine), desktop
// and phone, and writes tools/xbrowser/out/report.html with side-by-side
// screenshots, per-engine checks (JS errors, font readiness, clipped labels,
// off-screen chrome, dead animations) and a pixel mismatch vs Chromium.
//
//   node xb.mjs                      # all engines, desktop + mobile
//   node xb.mjs --engines=webkit --form=mobile --only=trees,nature
//   BASE=http://localhost:8000 node xb.mjs
import fs from 'node:fs';
import path from 'node:path';
import { chromium, firefox, webkit } from 'playwright';
import pixelmatch from 'pixelmatch';
import { PNG } from 'pngjs';
import { SCENES, PRE_SCENES } from './scenes.mjs';

const args = Object.fromEntries(process.argv.slice(2).map(a => { const m = a.match(/^--([^=]+)(?:=(.*))?$/); return m ? [m[1], m[2] ?? true] : [a, true]; }));
const BASE = process.env.BASE || 'http://localhost:8000';
const ENGINES = String(args.engines || 'chromium,firefox,webkit').split(',');
const FORMS = String(args.form || 'desktop,mobile').split(',');
const ONLY = args.only ? String(args.only).split(',') : null;
const OUT = path.resolve('out'); fs.mkdirSync(OUT, { recursive: true });
const engines = { chromium, firefox, webkit };

// ---- session bootstrap (one QA player + session, reused across runs) ----
async function session() {
  const f = path.resolve('.session.json');
  if (fs.existsSync(f)) { const s = JSON.parse(fs.readFileSync(f, 'utf8')); const r = await fetch(`${BASE}/api/session/${s.sid}`); if (r.ok) return s; }
  const reg = await (await fetch(`${BASE}/api/register`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: 'XB' + Math.random().toString(36).slice(2, 6) }) })).json();
  const lucky = await (await fetch(`${BASE}/api/lucky`)).json();
  const cs = await (await fetch(`${BASE}/api/session/create`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Player-Token': reg.rejoin_token },
    body: JSON.stringify({ player_id: reg.player.id, name: 'XB', municipality_code: lucky.gemeinde_code, municipality_name: lucky.name, center_lon: lucky.lon, center_lat: lucky.lat }) })).json();
  const s = { pid: reg.player.id, pname: reg.player.name, token: reg.rejoin_token, sid: cs.session.id, invite: cs.invite_code, lon: cs.session.center_lon, lat: cs.session.center_lat, muni: lucky.name };
  fs.writeFileSync(f, JSON.stringify(s, null, 1)); return s;
}

// ---- in-page checks (serialised into the page) ----
const CHECKS = `(() => {
  const vis = el => { const r = el.getBoundingClientRect(); const cs = getComputedStyle(el); return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none' && +cs.opacity > 0.05; };
  const pathOf = el => { const p = []; for (let e = el; e && e !== document.body && p.length < 4; e = e.parentElement) p.unshift(e.tagName.toLowerCase() + (e.id ? '#' + e.id : e.classList.length ? '.' + [...e.classList].slice(0, 2).join('.') : '')); return p.join('>'); };
  const out = { clipped: [], offscreen: [], fonts: {}, screen: null };
  out.screen = [...document.querySelectorAll('.screen.active')].map(e => e.id).join(',');
  // viewport sanity: a scrolled document / offset visual viewport shifts fixed chrome in screenshots
  const vv = window.visualViewport; const sbt = document.getElementById('sb-toggle');
  out.view = { ih: innerHeight, iw: innerWidth, vvh: vv ? Math.round(vv.height) : null, vvTop: vv ? Math.round(vv.offsetTop) : null, scrollY: Math.round(scrollY), docH: document.documentElement.scrollHeight,
    toggle: sbt ? [Math.round(sbt.getBoundingClientRect().top), Math.round(sbt.getBoundingClientRect().height)] : null };
  for (const fam of ['VT323', '"Press Start 2P"']) out.fonts[fam] = document.fonts.check('12px ' + fam);
  const W = innerWidth, H = innerHeight;
  const all = [...document.querySelectorAll('body *')].filter(vis);
  for (const el of all) {
    const cs = getComputedStyle(el);
    const hasText = [...el.childNodes].some(n => n.nodeType === 3 && n.textContent.trim());
    if (hasText && el.tagName !== 'TEXTAREA' && el.tagName !== 'INPUT' && el.tagName !== 'SELECT' && el.tagName !== 'OPTION') {
      const ox = el.scrollWidth - el.clientWidth, oy = el.scrollHeight - el.clientHeight;
      const scrolls = /auto|scroll/.test(cs.overflowX + cs.overflowY);
      const ell = cs.textOverflow === 'ellipsis';
      const hid = cs.overflow === 'hidden' && ox <= 4 && oy <= 3; /* deliberate clip (rotated chevron) */
      if (!scrolls && !ell && !hid && (ox > 2 || oy > 3) && el.clientWidth > 0) out.clipped.push({ el: pathOf(el), ox, oy, w: el.clientWidth, h: el.clientHeight, text: el.textContent.trim().slice(0, 40) });
    }
    if (/fixed|absolute/.test(cs.position) && (el.id || /popup|chip|hud|modal|toast|btn|sheet|herald|minimap|attrib/.test(el.className))) {
      const r = el.getBoundingClientRect();
      // parked bottom sheet (translateY) and anything inside a scroll container are reachable by design
      if (el.id === 'sidebar' && cs.transform !== 'none') continue;
      let scroller = null; for (let a = el.parentElement; a && a !== document.body; a = a.parentElement) { const o = getComputedStyle(a); if (/auto|scroll/.test(o.overflowY + o.overflowX)) { scroller = a; break; } }
      if (scroller) continue;
      if (r.width > 20 && r.height > 10 && (r.left < -1 || r.top < -1 || r.right > W + 1 || r.bottom > H + 1) && !el.closest('.screen:not(.active)'))
        out.offscreen.push({ el: pathOf(el), rect: [r.left, r.top, r.right, r.bottom].map(Math.round) });
    }
  }
  out.clipped = out.clipped.slice(0, 12); out.offscreen = out.offscreen.slice(0, 12);
  return out;
})()`;

// Canvas liveness: two downscaled samples of the game canvas, N ms apart.
const SAMPLE = `(async (ms) => {
  const c = document.getElementById('game-canvas'); if (!c || !c.width) return null;
  const sw = 240, sh = Math.round(c.height / c.width * 240);
  const o = document.createElement('canvas'); o.width = sw; o.height = sh; const g = o.getContext('2d', { willReadFrequently: true });
  const grab = () => { g.drawImage(c, 0, 0, sw, sh); return g.getImageData(0, 0, sw, sh).data; };
  const a = grab(); await new Promise(r => setTimeout(r, ms)); const b = grab();
  let diff = 0, nonEmpty = 0;
  for (let i = 0; i < a.length; i += 4) {
    if (Math.abs(a[i] - b[i]) + Math.abs(a[i+1] - b[i+1]) + Math.abs(a[i+2] - b[i+2]) > 24) diff++;
    if (a[i+3] > 0) nonEmpty++;
  }
  return { diffPct: +(100 * diff / (sw * sh)).toFixed(2), paintedPct: +(100 * nonEmpty / (sw * sh)).toFixed(1) };
})`;

const results = {};   // scene → engine-form → {shot, checks, errors, info}
const sleep = ms => new Promise(r => setTimeout(r, ms));

for (const form of FORMS) for (const name of ENGINES) {
  const tag = `${name}-${form}`;
  const S = await session();
  const browser = await engines[name].launch();
  const ctxOpts = form === 'mobile'
    ? { viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: name !== 'firefox', hasTouch: true, userAgent: name === 'webkit' ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1' : undefined }
    : { viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 };
  const ctx = await browser.newContext(ctxOpts);
  const page = await ctx.newPage();
  let errors = [];
  page.on('console', m => { if (m.type() === 'error') { const u = (m.location() || {}).url || ''; errors.push('console: ' + m.text().slice(0, 200) + (u ? ' @ ' + u.replace(BASE, '').slice(0, 120) : '')); } });
  page.on('pageerror', e => errors.push('PAGEERROR: ' + String(e.message).slice(0, 300)));
  // SSE streams are cancelled/reconnected on navigation and server restarts — not a page error
  page.on('requestfailed', r => { if (!/favicon|hillshade|\/events(\?|$)/.test(r.url())) errors.push('netfail: ' + r.url().replace(BASE, '').slice(0, 120) + ' ' + (r.failure()?.errorText || '')); });

  const run = async (sc, list) => {
    if (ONLY && !ONLY.some(o => sc.id.includes(o))) return;
    if (sc.forms && !sc.forms.includes(form)) return;
    errors = [];
    const rec = { tag, engine: name, form, errors: [], checks: null, info: null, shot: `${sc.id}--${tag}.png`, ms: 0 };
    const t0 = Date.now();
    try {
      rec.info = await sc.run({ page, S, BASE, form, engine: name, sleep }) ?? null;
      if (sc.anim) rec.anim = await page.evaluate(`(${SAMPLE})(350)`);
      await page.screenshot({ path: path.join(OUT, rec.shot), animations: 'allow' });
      rec.checks = await page.evaluate(CHECKS);
    } catch (e) { rec.fail = String(e.message).split('\n')[0].slice(0, 300); try { await page.screenshot({ path: path.join(OUT, rec.shot) }); } catch {} }
    rec.ms = Date.now() - t0;
    rec.errors = errors.slice(0, 10);
    (results[sc.id] ||= { title: sc.title, anim: !!sc.anim, by: {} }).by[tag] = rec;
    console.log(`${tag.padEnd(16)} ${sc.id.padEnd(22)} ${rec.fail ? 'FAIL ' + rec.fail : 'ok'} ${rec.anim ? 'anim=' + rec.anim.diffPct + '%' : ''} ${rec.errors.length ? 'errs=' + rec.errors.length : ''} ${rec.checks?.clipped?.length ? 'clipped=' + rec.checks.clipped.length : ''} ${rec.checks?.offscreen?.length ? 'offscreen=' + rec.checks.offscreen.length : ''} ${rec.ms}ms`);
  };
  for (const sc of PRE_SCENES) await run(sc);
  // game session: one load, then walk all in-game states
  await page.goto(`${BASE}/?lang=de&dev=1&pid=${S.pid}&pname=${encodeURIComponent(S.pname)}&rejoin=${S.token}&sid=${S.sid}#v=${S.lon},${S.lat},16`, { waitUntil: 'load', timeout: 90000 });
  try {
    await page.waitForFunction(() => typeof G !== 'undefined' && G.session && document.getElementById('screen-game').classList.contains('active'), null, { timeout: 120000 });
    await page.evaluate(() => DEV.idle(20000));
  } catch (e) { console.log(tag, 'game did not open:', e.message.slice(0, 200)); }
  for (const sc of SCENES) await run(sc);
  await browser.close();
}

// ---- pixel mismatch vs chromium (same form) ----
function mismatch(a, b) {
  try {
    const A = PNG.sync.read(fs.readFileSync(a)), B = PNG.sync.read(fs.readFileSync(b));
    if (A.width !== B.width || A.height !== B.height) return null;
    const n = pixelmatch(A.data, B.data, null, A.width, A.height, { threshold: 0.25 });
    return +(100 * n / (A.width * A.height)).toFixed(1);
  } catch { return null; }
}
for (const [id, r] of Object.entries(results)) for (const [tag, rec] of Object.entries(r.by)) {
  const ref = r.by[`chromium-${rec.form}`];
  if (ref && ref !== rec) rec.mismatch = mismatch(path.join(OUT, ref.shot), path.join(OUT, rec.shot));
}

// ---- report ----
fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify(results, null, 1));
const tags = [...new Set(Object.values(results).flatMap(r => Object.keys(r.by)))];
const esc = s => String(s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
let html = `<!doctype html><meta charset=utf-8><title>Siedler cross-browser report</title><style>
body{font:13px/1.4 system-ui;margin:16px;background:#111;color:#ddd}h1{font-size:18px}table{border-collapse:collapse}td,th{border:1px solid #333;padding:4px;vertical-align:top}
img{max-width:460px;display:block;background:#000}.m img{max-width:200px}.bad{color:#f66;font-weight:600}.warn{color:#fc6}.ok{color:#6c6}pre{white-space:pre-wrap;font-size:11px;max-width:460px;margin:2px 0;color:#bbb}
.sum td{font-size:12px}a{color:#9cf}</style><h1>Siedler Österreich — cross-browser state walk (${new Date().toISOString().slice(0, 16)})</h1>`;
html += `<h2>Summary</h2><table class=sum><tr><th>scene</th>${tags.map(t => `<th>${t}</th>`).join('')}</tr>`;
for (const [id, r] of Object.entries(results)) {
  html += `<tr><td><a href="#${id}">${esc(r.title)}</a></td>` + tags.map(t => {
    const x = r.by[t]; if (!x) return '<td>–</td>';
    const flags = [];
    if (x.fail) flags.push(`<span class=bad>FAIL</span>`);
    if (x.errors.length) flags.push(`<span class=bad>${x.errors.length} err</span>`);
    if (x.checks && !Object.values(x.checks.fonts).every(Boolean)) flags.push(`<span class=bad>fonts</span>`);
    if (x.checks?.clipped?.length) flags.push(`<span class=warn>${x.checks.clipped.length} clipped</span>`);
    if (x.checks?.offscreen?.length) flags.push(`<span class=warn>${x.checks.offscreen.length} offscreen</span>`);
    if (r.anim && x.anim && x.anim.diffPct < 0.05) flags.push(`<span class=bad>dead anim</span>`);
    if (x.mismatch != null && x.mismatch > 35) flags.push(`<span class=warn>Δ${x.mismatch}%</span>`);
    return `<td>${flags.join(' ') || '<span class=ok>ok</span>'}</td>`;
  }).join('') + '</tr>';
}
html += '</table>';
for (const [id, r] of Object.entries(results)) {
  const rows = tags.filter(t => r.by[t]);
  html += `<h2 id="${id}">${esc(r.title)} <small>(${id})</small></h2><table><tr>${rows.map(t => `<th>${t}${r.by[t].mismatch != null ? ` · Δ${r.by[t].mismatch}% vs chromium` : ''}</th>`).join('')}</tr><tr>`;
  for (const t of rows) {
    const x = r.by[t];
    html += `<td class="${x.form === 'mobile' ? 'm' : ''}"><a href="${x.shot}"><img loading=lazy src="${x.shot}"></a>`;
    if (x.fail) html += `<pre class=bad>${esc(x.fail)}</pre>`;
    if (x.anim) html += `<pre>anim diff ${x.anim.diffPct}% · painted ${x.anim.paintedPct}%</pre>`;
    if (x.info) html += `<pre>${esc(JSON.stringify(x.info)).slice(0, 300)}</pre>`;
    for (const e of x.errors) html += `<pre class=bad>${esc(e)}</pre>`;
    if (x.checks) { for (const c of x.checks.clipped) html += `<pre class=warn>clipped ${esc(c.el)} +${c.ox}×${c.oy}px (${c.w}×${c.h}) “${esc(c.text)}”</pre>`; for (const c of x.checks.offscreen) html += `<pre class=warn>offscreen ${esc(c.el)} ${c.rect}</pre>`; if (!Object.values(x.checks.fonts).every(Boolean)) html += `<pre class=bad>fonts ${esc(JSON.stringify(x.checks.fonts))}</pre>`; }
    html += '</td>';
  }
  html += '</tr></table>';
}
fs.writeFileSync(path.join(OUT, 'report.html'), html);
console.log('report:', path.join(OUT, 'report.html'));
