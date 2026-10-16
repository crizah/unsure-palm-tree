// Syncs db/places.json into PocketBase (creates the collection if missing,
// upserts by name, deletes rows no longer in the file). Needs Node 18+.
//   PB_URL=https://api.crizah.space PB_EMAIL=you@x.com PB_PASSWORD=... node db/seed.mjs
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const { PB_URL = 'http://127.0.0.1:8090', PB_EMAIL, PB_PASSWORD } = process.env;
if (!PB_EMAIL || !PB_PASSWORD) { console.error('set PB_EMAIL and PB_PASSWORD (superuser)'); process.exit(1); }
const places = JSON.parse(readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'places.json'), 'utf8'));

let token = '';
async function pb(method, path, body) {
  const res = await fetch(PB_URL + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token && { Authorization: token }) },
    body: body && JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status} ${await res.text()}`);
  return res.status === 204 ? null : res.json();
}

token = (await pb('POST', '/api/collections/_superusers/auth-with-password', { identity: PB_EMAIL, password: PB_PASSWORD })).token;

const text = (name) => ({ name, type: 'text' });
const num = (name) => ({ name, type: 'number' });
try {
  await pb('GET', '/api/collections/places');
} catch {
  // Public read-only: list/view open, create/update/delete left null (superuser only).
  await pb('POST', '/api/collections', {
    name: 'places', type: 'base', listRule: '', viewRule: '',
    fields: ['name', 'city', 'type', 'area', 'address', 'timings', 'cost', 'geo_precision'].map(text).concat(num('lat'), num('lng')),
  });
  console.log('created collection "places"');
}

const existing = new Map();
for (let page = 1, total = 1; page <= total; page++) {
  const r = await pb('GET', `/api/collections/places/records?perPage=200&page=${page}`);
  total = r.totalPages;
  for (const rec of r.items) existing.set(rec.name, rec.id);
}

let created = 0, updated = 0;
for (const p of places) {
  const id = existing.get(p.name);
  existing.delete(p.name);
  if (id) { await pb('PATCH', `/api/collections/places/records/${id}`, p); updated++; }
  else { await pb('POST', '/api/collections/places/records', p); created++; }
}
for (const id of existing.values()) await pb('DELETE', `/api/collections/places/records/${id}`);
console.log(`created ${created}, updated ${updated}, deleted ${existing.size}`);
