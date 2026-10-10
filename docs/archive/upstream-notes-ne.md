# Notes for the upstream maintainers (bevdirect-serve, umfeld-at ne_cells) — 2026-10-04

Observations from running the NE epoch-report pipeline (`tools/ne-report`, see `ne-report.md`)
against bevdirect-serve **v0.2.1** and the frozen `ne_cells` reference package (umfeld commit
3f26b3b, algo `ne-cells-2`). Not yet sent — we have no peer token / issue channel for umfeld;
file these when one exists.

## bevdirect-serve v0.2.1

1. **`"footprints": null` for an empty layer.** A cell with no building footprints (e.g.
   05007 cell i=840, j=2402) serialises the layer as `null` instead of `[]` (Go nil slice).
   Same risk for `parcels`/`landuse` in principle. Consumers that do `doc.get("footprints", [])`
   get `None` → iteration `TypeError`. Suggested fix: initialise the slices (`make([]T, 0)`) or
   `omitempty` + document the absence. We normalise `null → []` locally before writing the cell
   (`ne_report.py`, `null_layers_normalised` in the meta file); the record digest is unaffected
   (63330 reproduces the contract digest bit-exactly).

## umfeld-at `ne_cells` (frozen reference `ne-cells-2`)

2. **`canon._one_bevdirect` is not null-safe** — see (1): `doc.get("footprints", [])` should be
   `doc.get("footprints") or []` (likewise for the other layers). Since the package is frozen for
   digest stability this is a *canon-neutral* change: `None` and `[]` produce the same records,
   so the digest of every existing build stays identical.
3. **`ne_cells build` ignores `truncated`.** It refuses to build on `ready:false` but accepts a
   cell document with `truncated:true` (bevdirect caps very dense cells), which yields a
   silently incomplete chunk digest. Our driver checks both before handing cells to the build;
   the reference should at least warn, ideally refuse unless `--allow-truncated`.
4. Minor: our `/api/warm/status` now exposes `v24_kgs`; `run.sh` prefers it over the built-in list.

## srtm-lidar-at `/api/v1/cells`

5. Nothing blocking. `format=columns` documents are 0.05–1 MB per 0.02° cell, 30–200 ms; 404 for
   unprocessed KGs carries `kgs_missing` + `retry_after_s`, which we honour (negative cache 1 h).
