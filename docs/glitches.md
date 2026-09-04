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
