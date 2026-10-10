# Known cosmetic issues (open)

Fixed glitches live in git history / `lessons.md`. Append new ones here; delete when fixed.

- (#4) Open? N2K site label ("Wachau - Jauerling") at z17 is a faint, near-unreadable dark-on-green watermark; garbled look.
- (#8) Danube at Dürnstein renders grey: riverbed parcel 12105-1551/1 (70 ha) has no GW NS symbol (Alpe/GA/OG/Öd…) and lidar dom_terrain=bare_soil (0.87). Not fixable client-side without flooding land (OSM riverbank chunks arrive bbox-cut and don't close). Reported upstream: cadastre feedback #16, srtm-lidar feedback #10 (correct_type→water). Re-check after upstream fix.
- (#35) giant/apex trees can stand inside the Danube polygon (riverbank apex on the riverbed parcel 12105-1551/1) — a real lidar tree on the bank, cosmetic.
- (#36) Herald sometimes announces a quest ("Nächste Aufgabe: Naturschützer") that was just auto-completed by the same action (queued before the completion event). Cosmetic.
