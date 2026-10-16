# Needs db/pocketbase (download from pocketbase.io/docs). Its data lives in db/pb_data.
# Credentials come from the command line: make seed PB_EMAIL=you@x.com PB_PASSWORD=...
PB_URL ?= http://127.0.0.1:8090

.PHONY: serve superuser seed web

serve:
	cd db && ./pocketbase serve

superuser:
	cd db && ./pocketbase superuser upsert "$(PB_EMAIL)" "$(PB_PASSWORD)"

# Run while `make serve` is up. Stop it (ctrl+c) before committing db/pb_data/data.db.
seed:
	cd db && PB_URL=$(PB_URL) PB_EMAIL="$(PB_EMAIL)" PB_PASSWORD="$(PB_PASSWORD)" node seed.mjs

web:
	cd web && npm run dev
