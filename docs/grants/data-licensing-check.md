# Data licensing check — Siedler Österreich & upstream services (updated 2026-09-04)

Question: is reuse of BEV cadastre / LiDAR / auxiliary data via our upstream
services legally sound, and do the services themselves disclose licensing?

**Short answer: yes — every service we call now publishes machine-readable
licensing and we only use endpoints with open, commercial-use-compatible terms.**

## Upstream services — licence disclosure (verified 2026-09-04)

| Service | Where licensing is disclosed | Derivative licence |
|---|---|---|
| cadastre-process-api.exe.xyz | `GET /api/v1/license` (table of all sources + obligations), `meta.license` on every JSON envelope / GeoJSON export, `Link: <…>; rel="license"` + `X-Data-Attribution` headers on every `/api/` response, "License & Attribution" section in `/api/v1/docs/llm.txt` | CC BY 4.0 (code MIT) |
| srtm-lidar-at.exe.xyz | `GET /api/v1/attribution` (JSON / `?format=text`), `attribution` key in every KG JSON, `gpkg_metadata` table in every GPKG, Zenodo deposit notes; "Licence & Attribution" section in `/api/v1/docs/llm.txt` | CC BY 4.0; OSM-derived layers ODbL |

## Sources actually consumed by Siedler Österreich

| Source | Licence | Used via | Attribution |
|---|---|---|---|
| BEV Kataster (DKM, Grundstücksverzeichnis, Nutzungsflächen, footprints) | CC BY 4.0 (OGD) | cadastre `/spatial/*`, `/export/*`, `/search/*` | "Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen, CC BY 4.0, bearbeitet" |
| BEV ALS DTM/DSM 1 m, DOP orthophoto | CC BY 4.0 (OGD) | srtm-lidar `/kg/*`, `/attribution` | same, + Stichtag from `/attribution` |
| Statistik Austria (Gemeinden, Boden-/Baulandpreise) | CC BY 4.0 | cadastre `/search/municipalities`, `/lookup`, `/land_prices/*` | "Datenquelle: STATISTIK AUSTRIA" |
| EEA Natura 2000 | EEA standard re-use policy (attribution) | cadastre `/natura2000/*` | "Source: European Environment Agency (EEA), Natura 2000 data" |
| RIS Landesrecht | CC BY 4.0 | cadastre `/legal/kg/*` | "Datenquelle: RIS, Bundeskanzleramt Österreich" |
| ESA WorldCover 2021 v200, Copernicus Sentinel | CC BY 4.0 / Copernicus terms | srtm-lidar land-cover classes (`dom_terrain`) | "© ESA WorldCover project 2021 / Contains modified Copernicus Sentinel data" |
| Hansen GFC-2024 v1.12 | CC BY 4.0 | srtm-lidar forest fractions | "Hansen/UMD/Google/USGS/NASA" |
| OpenStreetMap (Geofabrik AT extract, Nominatim) | **ODbL 1.0** (share-alike) | cadastre `/osm/*`, `osm{}` block, `/search/address_osm`; srtm-lidar infrastructure layer | "© OpenStreetMap contributors" |
| geoBoundaries gbOpen / BEV ADM0 outline (`srv/static/austria.json`) | CC BY 4.0 / CC BY-SA | shipped static file | "geoBoundaries / BEV" |

**Not used** (non-commercial terms, flagged as legacy upstream): `/search/gadm`
(GADM v4.1) and WDPA. The municipality picker uses `/search/municipalities`
(Statistik Austria) — the "GADM" mention in older game.js comments is stale.

## Why this is sound

CC BY 4.0 permits copying, transformation (segmentation, R-tree API, game) and
commercial use. Obligations, all met in-app (welcome footer, in-map "© BEV ·
OSM" chip with expandable rows, Impressum) and on every Zenodo record:

1. **Attribution** to BEV (+ Stichtag where known) and the auxiliary sources.
2. **Licence link** https://creativecommons.org/licenses/by/4.0/
3. **Indicate changes** — "bearbeitet / modified" (re-projected, re-assembled,
   simplified, enriched).
4. **No implied endorsement** by BEV or any provider.

Notes / caveats:
- BEV holds copyright + sui generis database rights (§76c ff UrhG); the CC BY
  grant licenses exactly those, so no separate agreement is needed. BEV
  confirmed CC BY 4.0 for kataster.bev.gv.at to the OSM community in 2023.
- Owner (Eigentümer) data is not in the open datasets and not served by any
  upstream → no GDPR exposure. EZ numbers are public cadastre attributes.
- OSM is ODbL, not CC BY. We only *display* OSM-derived layers; we do not
  redistribute a merged database. Upstream keeps `osm{}` fields out of Zenodo
  dumps and GPKG/GeoJSON exports so share-alike never attaches to the CC BY
  cadastre derivatives.
- Natura 2000 (EEA) is attribution-only, no share-alike — fine for commercial use.
- Zenodo derivatives published as CC BY 4.0 are compatible with all sources above.

Recommended attribution line (grant doc §1.5 + in-app footer):
"Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen, Kataster & ALS,
CC BY 4.0 (bearbeitet) · Statistik Austria (CC BY 4.0) · EEA Natura 2000 ·
© ESA WorldCover 2021 · © OpenStreetMap contributors (ODbL)"
