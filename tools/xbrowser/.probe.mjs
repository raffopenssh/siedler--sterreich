import fs from 'node:fs';
import { chromium, firefox, webkit } from 'playwright';
const S = JSON.parse(fs.readFileSync('.session.json','utf8'));
const BASE='http://localhost:8000';
const engines = { chromium, firefox, webkit };
const which = (process.argv[2]||'chromium,firefox,webkit').split(',');
const form = process.argv[3]||'mobile';
const code = fs.readFileSync(process.argv[4]||'/tmp/probe-body.js','utf8');
for (const name of which) {
  const browser = await engines[name].launch();
  const ctx = await browser.newContext(form==='mobile'
    ? { viewport:{width:390,height:844}, deviceScaleFactor:2, isMobile:name!=='firefox', hasTouch:true }
    : { viewport:{width:1440,height:900} });
  const page = await ctx.newPage();
  const errs=[]; page.on('pageerror', e=>errs.push(e.message)); page.on('console', m=>{ if(m.type()==='error') errs.push(m.text()); });
  await page.goto(`${BASE}/?lang=de&dev=1&pid=${S.pid}&pname=${encodeURIComponent(S.pname)}&rejoin=${S.token}&sid=${S.sid}#v=${S.lon},${S.lat},16`, { waitUntil:'load', timeout:90000 });
  await page.waitForFunction(() => typeof G!=='undefined' && G.session && document.getElementById('screen-game').classList.contains('active'), null, {timeout:120000});
  await page.evaluate(() => DEV.idle(20000));
  const r = await page.evaluate(`(async()=>{${code}})()`);
  console.log(name, JSON.stringify(r, null, 0), errs.length?errs:'');
  await page.screenshot({ path:`/tmp/probe-${name}-${form}.png` });
  await browser.close();
}
