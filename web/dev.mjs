// Local dev server for dist/: builds, then serves with /locate -> locate.html.
//   WEB_PORT  port to serve on (default 3000)
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { dirname, extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from './build.mjs';

const root = dirname(fileURLToPath(import.meta.url));
const dist = join(root, 'dist');
const port = Number(process.env.WEB_PORT || 3000);
const types = { '.html': 'text/html; charset=utf-8', '.css': 'text/css', '.js': 'text/javascript', '.png': 'image/png', '.svg': 'image/svg+xml' };

try { const r = build(); console.log(`built dist/ (${r.files} files), API_URL=${r.apiUrl}`); }
catch (e) { console.error(`build failed: ${e.message}`); process.exit(1); }

createServer(async (req, res) => {
  let path = decodeURIComponent(new URL(req.url, 'http://x').pathname);
  if (path === '/') path = '/index.html';
  else if (!extname(path)) path += '.html'; // clean URLs, like Vercel
  const file = normalize(join(dist, path));
  if (!file.startsWith(dist + '/')) { res.writeHead(403).end(); return; }
  try {
    const body = await readFile(file);
    res.writeHead(200, { 'Content-Type': types[extname(file)] || 'application/octet-stream', 'X-Content-Type-Options': 'nosniff' }).end(body);
  } catch {
    res.writeHead(404, { 'Content-Type': 'text/plain' }).end('not found');
  }
}).listen(port, () => console.log(`web on http://localhost:${port}`));
