# Screenshot recipes (Content Vienna 2026 deck)

All shots use the Raffael/Dürnstein session. Output goes to `/tmp/shots/NN_*.png`
(copy from `/tmp/shelley-screenshots/<id>.png` after `browser screenshot`).

## Setup

```
browser emulate_custom width=1920 height=1080 device_scale_factor=2   # desktop → 3840×2160
browser emulate_device iphone_14                                      # mobile  → 1170×2532
browser navigate (timeout 60s):
http://localhost:8000/?lang=de&dev=1&pid=c39355caaff8a4518f37d1dcb750398d&pname=Raffael&rejoin=a8a8d0e9b383e44e1e9001fa281ba9700fe76ef6809d4be6&sid=c09d88bc0f77ef6efa90f71e372580d8&nc=<bump>#v=<lon>,<lat>,<zoom>
```
`nc=` is a cache-buster for index.html (browser caches it, `?v=` bump alone isn't enough).
Then `await new Promise(r=>setTimeout(r,3000))` before the first `DEV.*` call.

Key places (Dürnstein, KG 12105):
- Town + Stift + Danube: `15.5205,48.3955,17.5`
- Giant-tree stand west of Stift: `15.5152,48.3951,18.2`
- Farm EZ 430: `DEV.ez('12105', 430)`; EZ parcel via `DEV.parcel(...)`

## Shots (browser eval, one-liners)

| # | file | commands |
|---|------|----------|
| 01 | Startbildschirm | navigate `http://localhost:8000/?lang=de` (fresh, no pid) |
| 02 | Gemeindeauswahl | welcome → click "Spielen"; picker map (enhanced KGs glow cyan) |
| 03 | Spielkarte Dürnstein | `await DEV.goto(15.5205,48.3955,17.5); DEV.freeze()` |
| 04 | Parzellen-Info Gebäude | `await DEV.goto(15.5205,48.3955,18); await DEV.building(0); DEV.freeze()` |
| 05 | EZ Sammelkauf | `await DEV.goto(15.5205,48.3955,17.5); const p=DEV.ezCandidates(4,20)[0]; await DEV.parcel(p.parcel_id)` → EZ popup via "▸" (`document.querySelector('#pp-ez-link')?.click()`) |
| 06 | Naturschutz | converted parcels near town: `await DEV.goto(15.5218,48.3945,17.8); DEV.freeze()` |
| 07 | Schätze / N2K | `await DEV.treasure(null,false,17.5); DEV.n2k(true); DEV.freeze()` (pick species treasure via `DEV.treasures()`) |
| 08 | Riesenbaum-Hinweis | `DEV.trees('hint'); await DEV.goto(15.5152,48.3951,17); DEV.freeze()` |
| 09 | Baum-Histogramm | `DEV.trees('revealed'); DEV.tree(0)` |
| 10 | KG-Statistik | `DEV.kg('12105')` |
| 11 | Gebäude-Info LiDAR | `await DEV.goto(15.5205,48.3955,18.5); await DEV.building(0)` |
| 12 | Ähnliche Parzellen | `await DEV.similar('12105-<pid>', 5000); await DEV.goto(lon,lat,14.5)` |
| 13 | Ladebildschirm | `DEV.loading(62, 'Dürnstein (12105)')` … `DEV.loading(false)` |
| 14 | Kundschafter GPS | `await DEV.goto(15.5152,48.3951,18.2); DEV.trees('revealed'); DEV.gps(15.5141,48.3959,8); DEV.freeze(); render()` |

Chrome control: `DEV.chrome(false)` hides HUD, `DEV.sidebar(false)`, `DEV.closeAll()`,
`DEV.freeze()` stops animations (call `DEV.freeze(false)` before `goto`).

## Mobile (iphone_14)
Same URLs; `DEV.sheet(true)` expands the bottom sheet; `DEV.chrome(true)`.
Planned: 03m Karte, 04m Parzellen-Popup, 07m Schatz, 14m GPS.

## Data-source pages (for "the world is real" slides)
- https://kataster.bev.gv.at/ (Grundstück Dürnstein, e.g. search "Dürnstein 12105")
- https://www.statistik.at/atlas/ or https://www.statistik.at/services/tools/services/regionales/regionalstatistische-rastereinheiten
- https://srtm-lidar-at.exe.xyz:8000/ (Query Explorer, KG 12105)
- https://cadastre-process-api.exe.xyz/api/v1/docs (API docs)
- https://www.data.gv.at/ (ALS DGM/DOM Austria)

## Batch re-shoot without polluting agent context (CDP)

`tools/cdp.py` + `tools/run_desktop.py` / `tools/run_mobile.py` drive the
headless browser tab straight over the DevTools protocol (`pip install
websocket-client`). Find the port with `ss -ltnp | grep headless-shell`, then
`python3 tools/run_desktop.py <port>` → `/tmp/shots2/desktop/*.png` (3840×2160)
and `python3 tools/run_mobile.py <port>` → `/tmp/shots2/mobile/*.png`
(1170×2532). Screenshots are written to disk only — nothing enters the chat.
`Page.captureScreenshot` needs an explicit `clip.scale=dpr`, and the script
sets `Emulation.setDeviceMetricsOverride` itself.

Lessons from the 2026-09-04 run:
- 05: `#pp-ez-link` is a class, not an id — use `DEV.ez('12105', 430)` directly
  (call it twice: once before and once after `goto`, the first call may race
  the EZ index).
- 07: pick `DEV.treasures().filter(t=>t.type=='n2k_species')` and zoom 16.2,
  otherwise the frame is featureless meadow.
- 12: use the fixed parcel `12105-.45/2`; `parcelsNear()` is screen-radius
  limited and returns nothing on a phone viewport.
- Deck: `python-pptx`, 16:9, desktop image fills the slide, mobile shot 86%
  height right-aligned on a dark rounded frame.

## Resolved
- Danube grey (glitch #8) — water renders blue since 2026-09; 03/06/12/14 re-shot.
