# Data licensing check — cadastre-process-api / Siedler Österreich (2026-09-04)

Question: is reuse of BEV cadastre data via cadastre-process-api.exe.xyz legally sound?

**Short answer: yes.** The DKM / Kataster datasets are published by BEV as Open
Government Data under **CC BY 4.0** (data.bev.gv.at metadata: "Für dieses Produkt
gilt die Standardlizenz CC-BY-4.0"; BEV confirmed to the OSM community in 2023
that the kataster.bev.gv.at service is CC BY 4.0). kataster.bev.gv.at itself
shows no reuse text because the license lives in the dataset metadata on
data.bev.gv.at, not on the viewer.

CC BY 4.0 permits copying, transformation (segmentation, R-tree API, game), and
commercial use. Obligations:

1. **Attribution**: "Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen"
   (+ year/Stichtag if known).
2. **License link**: https://creativecommons.org/licenses/by/4.0/
3. **Indicate changes**: our data is re-projected, simplified, re-assembled and
   enriched → state "bearbeitet / modified".
4. No implication of BEV endorsement.

Notes / caveats:
- BEV holds copyright + sui generis database rights (§76c ff UrhG); the CC BY
  grant is exactly what licenses them, so no separate agreement is needed.
- Owner data is not in the open datasets and not in our API → no GDPR issue.
  EZ numbers are public cadastre attributes (also shown on kataster.bev.gv.at).
- ALS 1 m DTM/DSM (geoland/BEV) and BEV orthophoto are also CC BY 4.0 — same
  attribution pattern; verify the Stichtag/version string per dataset.
- OSM data is ODbL, not CC BY: attribution "© OpenStreetMap contributors" and,
  if OSM-derived fields are merged into a redistributed database (e.g. Zenodo
  dumps with OSM proximity fields), share-alike (ODbL) applies to that database.
  Keep OSM-derived layers in separate files or note ODbL for them.
- Zenodo derivatives published as CC BY 4.0 are compatible with the BEV source.

Recommended attribution line (grant doc §1.5 + in-app footer):
"Datenquelle: BEV – Bundesamt für Eich- und Vermessungswesen, Kataster & ALS,
CC BY 4.0 (bearbeitet) · © OpenStreetMap contributors (ODbL)"
