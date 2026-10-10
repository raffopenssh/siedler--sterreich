# Siedler Österreich

Multiplayer browser game on real Austrian cadastre data (BEV, CC BY 4.0): explore, claim parcels, convert land
to nature reserves — Settlers IV pixel-art look. Go + SQLite backend, vanilla-JS canvas frontend. Nightly we
report cadastre-change digests to umfeld-at (`docs/contrib.md`).

Live: https://siedler-oesterreich.exe.xyz:8000/

- Agent/developer guide: `AGENTS.md` (rules, mental model, repo map)
- Topic docs: `docs/README.md`
- Build & deploy: `go build -o siedler ./cmd/srv/ && sudo systemctl restart srv`
