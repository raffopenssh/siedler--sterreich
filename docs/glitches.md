# Glitch log (screenshot session 2026-09-04)
1. Sidebar Aufgaben: "Erkunde deine Gemeinde – Kaufe deine erste Parzelle" and "Naturschützer" still open although player owns 13 parcels / converted 7. Challenges not auto-completed on rejoin/claim?
2. Species-treasure labels (Luchs, Mopsfledermaus, N2K) very small/low contrast at zoom 17.
3. N2K species treasure (Rotbauchunke) placed at 15.5222/48.4087 = on the Danube / area with no parcel polygons loaded → floats on empty green. Treasure placement should snap to a land parcel (or the viewport loader doesn't cover river KGs).
4. N2K site label ("Wachau - Jauerling") at z17 is a faint, near-unreadable dark-on-green watermark; garbled look.
5. Session center for Dürnstein (48.4139) is 2 km north of town in forest — treasures cluster there, not near the village. Center should prefer the settlement (OSM place node) over the Gemeinde centroid.
6. Giant-tree name labels overlap/clip each other in dense stands (Dürnstein Stift slope); need label collision avoidance or show only nearest/tallest label.
7. Tree labels (dark grey on dark forest green) hard to read at z18; same style issue as N2K label before fix.
8. Danube at Dürnstein renders grey: riverbed parcel 12105-1551/1 (70 ha) has no GW NS symbol (Alpe/GA/OG/Öd…) and lidar dom_terrain=bare_soil (0.87). Not fixable client-side without flooding land (OSM riverbank chunks arrive bbox-cut and don't close). Reported upstream: cadastre feedback #16, srtm-lidar feedback #10 (correct_type→water). Re-check after upstream fix.
9. GPS marker was a generic blue Google dot — replaced by pixel-art Kundschafter with red-white-red pennant + dashed gold accuracy ring.

# Glitch log (water/Chronik frontend session 2026-09-25, KG 63307 Gaisfeld)
Screenshots: docs/screenshots/water/ (JPEG, ≤1600 px)
10. Parcel popup "KG" row shows the bare code ("63307 ▸") when the viewport parcel row carries no `kg_name` — pre-existing; Chronik title fixes it via the dossier's gemeinde_name, popup row should do the same (lookup G.dossiers[kg]).
11. Wassertropfen-Reise: terrain tiles stream behind the droplet (~1.5 km/s), so the last kilometres before the border are often bare green; loadMoreParcels() is throttled to 2.5 s during follow. Acceptable for a "journey", but a per-KG prewarm along the reach chain would fix it (upstream `POST /prewarm` with the KGs under the flowpath).
12. MERIT-Basins reach geometry is coarse (120 vertices for 65 km) and sits 50–300 m off the OSM river line at zoom ≥ 15 — visible as a straight cyan line crossing the meandering Kainach. Cosmetic; snapping to OSM water lines client-side would be expensive.
13. Muni-crossing toast ("Du verlässt …") fires on every KG change during the follow-mode journey and is wider than the viewport on 1600 px screens (pre-existing width issue).
14. Co-located stations (gw + wq at one point, e.g. Gaisfeld Bl 4010) fan out horizontally by 12 px; at zoom 15–16 the second sprite can sit on the neighbouring parcel.
15. Meadow Förderung on a 1 231 m² Weide is 2🪙 per 40 min — correct from eur_per_ha_median but reads as a joke reward; consider a 5🪙 floor server-side.
16. `DEV.sidebar(false)` leaves the minimap black until the next renderMini() — DEV only.
17. Herald drought hint fires for the *arrival* KG when a journey ends in another Dürre KG; suppressed while G.flow is active, but the first pan afterwards still triggers it (one-shot, so only once per session).
