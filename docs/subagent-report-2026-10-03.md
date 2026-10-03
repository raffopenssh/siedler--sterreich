# Subagent report — metrics, soak test, agent docs (2026-10-03)

## Commits (mine)
- a5a0303  Add GET /api/metrics (srv/metrics.go; warm.go: warmStatusMap(); server.go: 2 lines)
- 37f4cb1  tools/soak.sh + tools/soakload/main.go (+ .gitignore: tools/soakload/soakload)
- b1806f4  Rewrite srv/agent_inspect.go on the migrated sources
- cb2d58c  Agent docs + Impressum: srv/agent.go (llmGameMD only), srv/discover.go (openapi),
           srv/llmahead.go, srv/static/impressum.html, srv/static/imprint.html

## server.go hunk (only change)
+	mux.HandleFunc("GET /api/metrics", s.handleMetrics) // ops snapshot (metrics.go)
-	return http.ListenAndServe(addr, securityHeaders(s.maintenanceMiddleware(rateLimitMiddleware(gzipMiddleware(mux)))))
+	return http.ListenAndServe(addr, securityHeaders(s.maintenanceMiddleware(rateLimitMiddleware(gzipMiddleware(metricsMiddleware(mux))))))

## Soak run (50 users, 60 s, 2-core VM, after your 8d04f7a)
cold Weißkirchen i. Stmk (62048), warm St. Gallenkirch (80120), lucky Knittelfeld (62041)
total 5066 req, 5xx 0, breaker-503 17 (farm /api/schlaege), conn-errors 0, viewport ready:false 58
viewport HIT: n=539 p50 885 ms p95 2405 ms max 4208 ms  → FAIL (< 1500 ms)
viewport all: warm role p50 989 / p95 11083 ms (78 % HIT), lucky p50 1167 / p95 9512 (88 % HIT)
POST /api/session/create p50 3.8–6.3 s (!), /api/similar MISS p50 2.4–3.1 s p95 5–6.9 s
GET /api/session/{id}/parcels p95 570–780 ms (pure SQLite) — box saturated (load avg 4.4 on 2 cores)
api_cache 354 → 374 MB (+19 MB); bevdirect 934 → 940 MB of 1024 MB MemoryMax; prefetch_queue 256
At 20 users / 30 s: viewport HIT p50 81–106 ms, p95 374–470 ms (PASS), all other assertions PASS.

## Findings (server side) — for you to decide
- Throughput: at 50 concurrent users the whole server degrades uniformly (cheap DB routes p95 ~0.7 s,
  HIT viewport ~0.9 s). Likely CPU: 1 MB cached cell JSON re-read from SQLite + gzip level 5 per request
  on 2 cores. Candidates: cache gzipped bodies, serve cells from the in-memory cellMem, lower gzip level.
- POST /api/session/create is slow even warm (3.8–6.3 s under load, 2.6 s cold Gemeinde at 20 users,
  50–80 ms warm at 20 users) — settlementCenter()/N2K treasures/warm enqueue on the request path?
- farm-subsidies (/api/schlaege) returned HTTP 503 under ~10 req/s of cold cells → 7 relayed 5xx
  then breaker OPEN 25 s. Repeated in every run (10–26 breaker 503s).
- bevdirect-serve sits at 93–95 % of MemoryMax (940/1024 MB) with cells_cached pinned at 160 and
  prefetch_queue growing (68 → 152 → 256) during runs; worth watching for OOM kills.
- cellstore.ezParcels() sends the whole KG bbox to bevdirect /ez → bevdirect rejects it
  ("lon,lat (or west..north) required", bbox > 0.045°) so it always returns ok=false. Inspect uses
  /ez?kg&ez&lon&lat directly.
- /api/agent/look on a cold cell: fetchAgentParcels returns 202 but handleAgentLook relays it as
  jsonErr "cadastre service unavailable, retry shortly" with no Retry-After / retry_after_s.
- /api/lucky centre for St. Gallenkirch (9.9703, 46.9864) is the KG bbox centre, 3.8 km from the
  village; look radius 300 m there returns 0 parcels. Lucky pool was 1 Gemeinde most of the day.
- Cached viewport row for 90107-2019/2 says building_count 0 / total_building_area 881, while
  bevdirect /ez now returns building_count 2 for the same parcel (stale cell vs live).
- /api/similar and agent look/claim/municipality responses carry no BEV `notice`; suggest
  `"notice": bevNotice` in agentAttribution (agent.go) and in similarJSON.
- gw /llm/parcel/{pid} answers 404 no_data for every parcel I tried → stationOnParcel() (claim
  Pegelwart bonus) is effectively dead; inspect uses /llm/points in the parcel bbox + PIP instead.
- /api/metrics also saw 5xx=4 during the final run that soakload did not (your browser session?).
- queue_len in /api/warm/status now reports the full queue (was capped at the 40-row preview).
