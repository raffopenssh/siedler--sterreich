# Glitch log (screenshot session 2026-09-04)
1. Sidebar Aufgaben: "Erkunde deine Gemeinde – Kaufe deine erste Parzelle" and "Naturschützer" still open although player owns 13 parcels / converted 7. Challenges not auto-completed on rejoin/claim? ✅ fixed 2026-09-26: handleGetChallenges sweeps autoCompleteChallenges on every fetch.
2. Species-treasure labels (Luchs, Mopsfledermaus, N2K) very small/low contrast at zoom 17. ✅ fixed: pill-backed name tag (drawTreasure).
3. N2K species treasure (Rotbauchunke) placed at 15.5222/48.4087 = on the Danube / area with no parcel polygons loaded → floats on empty green. Treasure placement should snap to a land parcel (or the viewport loader doesn't cover river KGs). ✅ fixed 2026-09-26: generateN2KTreasures prefers parcels ≤ 3 ha (site_parcels is area-desc → river beds first), dedupes positions.
4. N2K site label ("Wachau - Jauerling") at z17 is a faint, near-unreadable dark-on-green watermark; garbled look.
5. Session center for Dürnstein (48.4139) is 2 km north of town in forest — treasures cluster there, not near the village. Center should prefer the settlement (OSM place node) over the Gemeinde centroid. ✅ fixed 2026-09-26: settlementCenter() snaps the session centre to the OSM place (city/town/village ≤ 8 km) on create.
6. Giant-tree name labels overlap/clip each other in dense stands (Dürnstein Stift slope); need label collision avoidance or show only nearest/tallest label. ✅ fixed 2026-09-26: labelSlotFree() collision pre-pass (tallest wins), labels flushed above all sprites (flushTreeLabels).
7. Tree labels (dark grey on dark forest green) hard to read at z18; same style issue as N2K label before fix. ✅ fixed 2026-09-26: dark pill + rim behind giant-tree tags.
8. Danube at Dürnstein renders grey: riverbed parcel 12105-1551/1 (70 ha) has no GW NS symbol (Alpe/GA/OG/Öd…) and lidar dom_terrain=bare_soil (0.87). Not fixable client-side without flooding land (OSM riverbank chunks arrive bbox-cut and don't close). Reported upstream: cadastre feedback #16, srtm-lidar feedback #10 (correct_type→water). Re-check after upstream fix.
9. GPS marker was a generic blue Google dot — replaced by pixel-art Kundschafter with red-white-red pennant + dashed gold accuracy ring.

# Glitch log (water/Chronik frontend session 2026-09-25, KG 63307 Gaisfeld)
Screenshots: docs/screenshots/water/ (JPEG, ≤1600 px)
10. Parcel popup "KG" row shows the bare code ("63307 ▸") when the viewport parcel row carries no `kg_name` — pre-existing; Chronik title fixes it via the dossier's gemeinde_name, popup row should do the same (lookup G.dossiers[kg]). ✅ fixed 2026-09-26: kgName()/ensureKGName() (lazy /lookup?type=kg) — popup shows "Gaisfeld 63307 ▸".
11. Wassertropfen-Reise: terrain tiles stream behind the droplet (~1.5 km/s), so the last kilometres before the border are often bare green; loadMoreParcels() is throttled to 2.5 s during follow. Acceptable for a "journey", but a per-KG prewarm along the reach chain would fix it (upstream `POST /prewarm` with the KGs under the flowpath). ✅ fixed 2026-09-26: handleWaterFlowpath prewarms the KGs under the reach chain (kgsAlongPath → POST /prewarm, once per path).
12. MERIT-Basins reach geometry is coarse (120 vertices for 65 km) and sits 50–300 m off the OSM river line at zoom ≥ 15 — visible as a straight cyan line crossing the meandering Kainach. Cosmetic; snapping to OSM water lines client-side would be expensive.
13. Muni-crossing toast ("Du verlässt …") fires on every KG change during the follow-mode journey and is wider than the viewport on 1600 px screens (pre-existing width issue). ✅ fixed 2026-09-26: no muni toast while G.flow; toast wraps at ≤ 92 vw.
14. Co-located stations (gw + wq at one point, e.g. Gaisfeld Bl 4010) fan out horizontally by 12 px; at zoom 15–16 the second sprite can sit on the neighbouring parcel. ✅ fixed 2026-09-26: stations stack upward below zoom 17.
15. Meadow Förderung on a 1 231 m² Weide is 2🪙 per 40 min — correct from eur_per_ha_median but reads as a joke reward; consider a 5🪙 floor server-side. ✅ fixed 2026-09-26: subsidyCoins floor 5🪙.
16. `DEV.sidebar(false)` leaves the minimap black until the next renderMini() — DEV only. ✅ fixed 2026-09-26: DEV.sidebar re-renders the minimap.
17. Herald drought hint fires for the *arrival* KG when a journey ends in another Dürre KG; suppressed while G.flow is active, but the first pan afterwards still triggers it (one-shot, so only once per session). ✅ fixed 2026-09-26: hint suppressed 20 s after the journey ends (G._flowEndedAt).

# Glitch log (LID-3 / HOLZ-3 / CAD-5 pass 2026-09-26, KG 63307 Gaisfeld)
18. srtm `/query/buildings` `stories_est` is ridge-height/3 (16.8 m farmhouse → "6 Etagen"); we derive storeys from `mean_height_m`/2.9 instead (loadBuildings).
19. Still open: #8 (Danube riverbed parcel grey — upstream), #12 superseded by river snapping.

# Glitch log (provider-migration browser soak 2026-10-03, Dürnstein 12105)
Screenshots: docs/screenshots/migration/ (JPEG ≤1600 px). Server-side findings: docs/subagent-report-2026-10-03b.md.
20. srtm landmark banners (pennant + "🏠 45m") clustered 9× on Stift Dürnstein and overprinted into "229m"; every object was a *roof* that the isometric footprints already show. ✅ removed the layer (data stays in G.topObjects for popups/inspect).
21. Three INVEKOS Hofstellen in one wine-village yard drew three overlapping "Hofstelle" labels. ✅ one label per slot (labelSlotFree).
22. Natura-2000 chip listed "Kamp- und Kremstal" twice (SPA + SAC share a name) and ran off the phone screen under the button column. ✅ deduped; on phones shortens to "first +N" → "Natura 2000 (N)", centred on the free area.
23. Mitspieler row showed stale coins (own 10000 after +250 treasure; others never updated). ✅ own row reads G.player, list refreshes ≤3 s after economy SSE events.
24. Treasure claim had no SSE broadcast → other tabs kept the chest and got a 409. ✅ `treasure_claimed` event removes it.
25. Price shown 387🪙, charged 386 (Go truncated, JS rounded). ✅ Go uses math.Round.
26. Well row said "schützt 25 %" while the dig toast said 75 % (hard-coded fallback). ✅ uses the parcel's well quote / dossier rule / server default 0.75.
27. Offers could be made on converted (protected) parcels whose owner has no sell/accept UI → unanswerable offers. ✅ server 400 + popup shows "🛡️ Geschützt".
28. EZ popup stayed at "8 frei · 0/8" after a bulk buy. ✅ re-opened after claim-ez.
29. In-game toponym search ranked "Loiben" 58 km away above Loibenberg 1.5 km (upstream exact-name first). ✅ results ≤10 km first, by distance.
30. KG summary showed the raw RIS key "nature_protection". ✅ German labels (LEGAL_CTX).
31. Cadastre outage (breaker open) left an empty green map with no explanation after the one toast. ✅ `#map-loading` chip "⚠️ Kataster nicht erreichbar – neuer Versuch in N s" until the breaker window ends; tiles refill automatically on recovery (verified by stopping bevdirect-serve).
32. Minimap framed the bbox of *all* loaded parcels → two specks after a 10 km jump. ✅ camera-centred window.
33. `DEV.trees()` without a mode reset `tallRevealed` (QA footgun). ✅ read-only without args.
34. URL-based rejoin left `G.playerToken` null (links built from it broke). ✅ token copied from the URL on quickLogin.
35. Open: giant/apex trees can stand inside the Danube polygon (riverbank apex on the riverbed parcel 12105-1551/1) — a real lidar tree on the bank, cosmetic.
36. Open: Herald sometimes announces a quest ("Nächste Aufgabe: Naturschützer") that was just auto-completed by the same action (queued before the completion event). Cosmetic.
37. Verified, not a glitch: Bauflächen parcels (gnr ".65/1" etc.) hug their house, so footprints look "parcel-shaped" — the BEV resolver's footprints are the DKM NS-41 polygons (ratio 0.2–0.7 of the parcel, obb 10×4.6 m matches a 10×4.6 m drawn building) and the srtm `/buildings/bbox` lidar roofs sit on exactly the same centroids.
39. Tile-shaped gap in the Danube (lighter triangle, no parcel under it). Cause: bevdirect assembles each 0.02° cell with a 0.004° pad; a parcel running past it is `complete:false` in that cell and complete in the neighbour, and the client's "first copy wins" dedup kept the clipped one. ✅ `loadViewportGeometry()` upgrades in place (complete copy wins, then larger area; clears `_bb`/scene caches, re-assigns apex trees, rebuilds the parcel grid). Server: cells report `incomplete` (count); hot-cell entries now carry a hard expiry (they used to be LRU-touched forever).

