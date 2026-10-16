# Food Map

Free and subsidised meals near you (Delhi, Bangalore). Static site + PocketBase. No login.

- `web/` static site, deployed on Vercel (Root Directory `web`, env `API_URL=https://api.crizah.space`)
- `db/places.json` the data; `db/seed.mjs` syncs it into PocketBase
- `infra/` old Terraform, delete after `terraform destroy`

## Run locally
```bash
./pocketbase serve                       # http://127.0.0.1:8090 (create the superuser once)
PB_EMAIL=... PB_PASSWORD=... node db/seed.mjs
cd web && cp .env.example .env && npm run dev   # http://localhost:3000/locate
```

## Update data
Edit `db/places.json`, push, then on the server (or your laptop with `PB_URL=https://api.crizah.space`):
```bash
git pull && PB_EMAIL=... PB_PASSWORD=... node db/seed.mjs
```

## PocketBase notes
- The seed creates the `places` collection: list/view public, no create/update/delete.
- Allow the site's origin for CORS (`pocketbase serve --origins=https://crizah.space`; default is `*`).
- Turn on rate limiting in the admin UI: Settings → Application → Rate limiting.
