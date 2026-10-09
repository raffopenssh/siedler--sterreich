# i18n tooling — DE canonical, EN via `srv/static/i18n.js`

German markup/strings are canonical. `i18n.js` translates at runtime for `?lang=en`
(or a non-German browser): a DOM walker + MutationObserver for everything in the
document, `tr(s)` for canvas labels / toasts / innerHTML built in game.js. Server
messages stay German and get an exact entry. Dictionary = `I18N_EXACT` (whole text
node must match, trimmed) + `I18N_RX` (composites with numbers/names; the regex
must match the *whole* text node, so build the string first and call `tr()` on
the result rather than on fragments).

## The three checks (run all three before you commit UI text)

| what | command | catches |
|---|---|---|
| **static** | `node tools/i18n/audit.js srv/static/game.js` | string literals in game.js that no entry/pattern translates (also useful on `index.html`-sized templates; `tools/i18n/has.js 'A\|B'` checks single keys) |
| **runtime, scripted** | `tools/xb.sh --i18n` (≈ 6 min, chromium desktop + phone, every xbrowser scene) · `tools/xb.sh --i18n --lang=de` for English leaks on the German side | what a player actually sees: visible DOM text + placeholder/title/aria-label in the wrong language **and every `tr()` miss** (canvas labels, toasts) per scene → console, `tools/xbrowser/out/i18n-leaks.txt` (leak → scenes), report `…:8765/report.html` (`N i18n` flags) |
| **runtime, by hand** | in the game with `?lang=en&dev=1`: `DEV.i18n()` (prints + returns `{dom, misses}`; `DEV.i18n(true)` resets the miss log) — or paste `tools/i18n/sweep.js` into any page without DEV | the state you are looking at right now |

`window.i18nSweep({reset})` is what both runtime paths call (defined in i18n.js, both
languages). Proper nouns (Gemeinde/KG/player names from `G`) are filtered; `#chat-log`
and anything inside `.i18n-user` are user content and skipped. The "looks German /
looks English" heuristics are the `DE_RX`/`EN_RX` word lists in i18n.js — extend them
when a leak slips through, and keep `sweep.js` in sync.

## Fixing a leak

1. DOM text → exact entry in `I18N_EXACT` (whole trimmed node). Composite with numbers →
   `I18N_RX` pattern anchored `^…$`; prefer a function replacement that looks sub-terms
   up in `I18N_EXACT` (see the landuse-share list and crop-row patterns).
2. Canvas / toast / `textContent` built in game.js → wrap the *final* string in `tr()`
   (`tr(n + ' Parzellen')`, not `n + ' ' + tr('Parzellen')`), then add the pattern.
3. Server message (`srv/*.go`) → leave it German, add the exact entry.
4. English leaking into German (`--lang=de`) → the literal is an upstream label or a
   mapping-table value; map it to German in game.js (e.g. the terrain `tl` tables).
5. `node --check` both files, bump `i18n.js?v=` (and `game.js?v=`) in `index.html`,
   `go build`, restart, re-run the check that found it.

New xbrowser scenes (`tools/xbrowser/scenes.mjs`) automatically take part in the audit —
add one for every new popup/overlay so the next `--i18n` run sees it.
