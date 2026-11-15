import { readFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { matchRoute } from './router.js';
import { notFound } from './routes.js';

const host = '127.0.0.1';
const port = Number(process.argv[2] ?? 8080);
const files = new Map([
  ['/styles.css', 'text/css'],
  ['/app.js', 'text/javascript'],
  ['/layout.js', 'text/javascript'],
  ['/router.js', 'text/javascript'],
  ['/routes.js', 'text/javascript'],
  ['/views.js', 'text/javascript'],
]);

createServer(async (request, response) => {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405, { Allow: 'GET, HEAD' }).end();
    return;
  }

  const [pathname] = request.url.split('?');
  const assetType = files.get(pathname);
  const accept = request.headers.accept ?? '*/*';
  const acceptsHtml = accept.includes('text/html') || accept === '*/*';
  if (!assetType && !acceptsHtml) {
    response.writeHead(404).end();
    return;
  }

  const file = assetType ? pathname.slice(1) : 'index.html';
  const status = assetType || matchRoute(pathname).route !== notFound ? 200 : 404;
  try {
    const body = await readFile(new URL(file, import.meta.url));
    response.writeHead(status, { 'Content-Type': `${assetType ?? 'text/html'}; charset=utf-8` });
    response.end(request.method === 'HEAD' ? undefined : body);
  } catch {
    response.writeHead(500).end('Could not read the requested file.');
  }
}).listen(port, host, () => {
  console.log(`ExamPrep: http://${host}:${port}/`);
});
