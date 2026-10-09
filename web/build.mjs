// Builds the static site into dist/. No dependencies.
//   API_URL  public origin of the API, e.g. https://api.example.com (required)
// Real environment variables win over web/.env, so Vercel's project env works.
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

function loadDotEnv(path) {
  if (!existsSync(path)) return;
  for (const raw of readFileSync(path, 'utf8').split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const i = line.indexOf('=');
    if (i < 1) throw new Error(`${path}: expected KEY=VALUE, got "${line}"`);
    const k = line.slice(0, i).trim().replace(/^export\s+/, '');
    let v = line.slice(i + 1).trim();
    if (/^(["']).*\1$/.test(v)) v = v.slice(1, -1);
    if (!(k in process.env)) process.env[k] = v;
  }
}

function origin(key) {
  const v = (process.env[key] || '').trim();
  if (!v) throw new Error(`${key} is required (set it in web/.env or the Vercel project env)`);
  let u;
  try { u = new URL(v); } catch { throw new Error(`${key} "${v}" is not a valid URL`); }
  if (!['http:', 'https:'].includes(u.protocol) || (u.pathname !== '/' && u.pathname !== '') || u.search || u.hash || u.username) {
    throw new Error(`${key} "${v}" must be an origin like https://api.example.com (scheme + host, no path)`);
  }
  return u.origin;
}

export function build() {
  loadDotEnv(join(root, '.env'));
  const apiUrl = origin('API_URL');

  const csp = (extra) => [
    "default-src 'none'",
    "script-src 'self'",
    "style-src 'self' 'unsafe-inline'", // Leaflet sets inline positioning styles
    `img-src 'self' data: ${extra.img}`.trim(),
    `connect-src ${extra.connect}`.trim(),
    "base-uri 'none'",
    "form-action 'none'",
  ].join('; ');

  const pageCsp = {
    'index.html': csp({ img: '', connect: "'none'" }),
    'locate.html': csp({ img: 'https://tile.openstreetmap.org', connect: apiUrl }),
  };

  const src = join(root, 'src'), dist = join(root, 'dist');
  rmSync(dist, { recursive: true, force: true });
  mkdirSync(dist, { recursive: true });
  cpSync(src, dist, { recursive: true });

  for (const [name, policy] of Object.entries(pageCsp)) {
    const f = join(dist, name);
    const html = readFileSync(f, 'utf8')
      .replaceAll('{{CSP}}', policy)
      .replaceAll('{{API_URL}}', apiUrl);
    if (html.includes('{{')) throw new Error(`${name}: unreplaced placeholder`);
    writeFileSync(f, html);
  }
  return { apiUrl, files: count(dist) };
}

function count(dir) {
  return readdirSync(dir).reduce((n, f) => {
    const p = join(dir, f);
    return n + (statSync(p).isDirectory() ? count(p) : 1);
  }, 0);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    const { apiUrl, files } = build();
    console.log(`built dist/ (${files} files), API_URL=${apiUrl}`);
  } catch (e) {
    console.error(`build failed: ${e.message}`);
    process.exit(1);
  }
}
