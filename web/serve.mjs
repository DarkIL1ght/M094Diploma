import { readFile, realpath, stat } from 'node:fs/promises';
import { createServer } from 'node:http';
import { resolve, relative, isAbsolute, extname, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { matchRoute } from './router.js';
import { notFound } from './routes.js';

const host = '127.0.0.1';
const port = Number(process.argv[2] ?? 8080);
const root = await realpath(fileURLToPath(new URL('.', import.meta.url)));
const mimeTypes = new Map([
  ['.html', 'text/html; charset=utf-8'],
  ['.js', 'text/javascript; charset=utf-8'],
  ['.mjs', 'text/javascript; charset=utf-8'],
  ['.css', 'text/css; charset=utf-8'],
  ['.json', 'application/json; charset=utf-8'],
  ['.svg', 'image/svg+xml'],
  ['.png', 'image/png'],
  ['.jpg', 'image/jpeg'],
  ['.jpeg', 'image/jpeg'],
  ['.gif', 'image/gif'],
  ['.webp', 'image/webp'],
  ['.avif', 'image/avif'],
  ['.ico', 'image/x-icon'],
  ['.woff', 'font/woff'],
  ['.woff2', 'font/woff2'],
]);

function isInsideRoot(path) {
  const fromRoot = relative(root, path);
  return fromRoot !== '' && fromRoot !== '..' &&
    !fromRoot.startsWith(`..${sep}`) && !isAbsolute(fromRoot);
}

function decodePath(pathname) {
  let path;
  try {
    path = decodeURIComponent(pathname);
  } catch (error) {
    if (error instanceof URIError) return null;
    throw error;
  }

  if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\') ||
      path.includes(':') || /[\x00-\x1f\x7f]/.test(path)) return null;
  if (path.split('/').some((part) => part.startsWith('.') || /[. ]$/.test(part))) return null;
  return path;
}

async function readStaticFile(path) {
  const candidate = resolve(root, `.${path}`);
  if (!isInsideRoot(candidate)) return null;

  try {
    const file = await realpath(candidate);
    if (!isInsideRoot(file) || !(await stat(file)).isFile()) return null;
    return await readFile(file);
  } catch (error) {
    if (['ENOENT', 'ENOTDIR', 'ELOOP'].includes(error.code)) return null;
    throw error;
  }
}

const server = createServer(async (request, response) => {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405, { Allow: 'GET, HEAD' }).end();
    return;
  }

  const [pathname] = request.url.split('?');
  const path = decodePath(pathname);
  if (path === null) {
    response.writeHead(404).end();
    return;
  }

  const extension = extname(path).toLowerCase();
  const assetType = path.endsWith('/') ? undefined : mimeTypes.get(extension);
  const accept = request.headers.accept ?? '*/*';
  const acceptsHtml = accept.includes('text/html') || accept === '*/*';
  const knownRoute = matchRoute(pathname).route !== notFound;

  try {
    let body = assetType ? await readStaticFile(path) : null;
    let type = assetType;
    let status = 200;
    if (body === null) {
      if (!acceptsHtml || (extension && !accept.includes('text/html'))) {
        response.writeHead(404).end();
        return;
      }

      body = await readStaticFile('/index.html');
      if (body === null) {
        response.writeHead(500).end('Missing application entry point.');
        return;
      }
      type = mimeTypes.get('.html');
      status = knownRoute ? 200 : 404;
    }

    response.writeHead(status, { 'Content-Type': type, 'X-Content-Type-Options': 'nosniff' });
    response.end(request.method === 'HEAD' ? undefined : body);
  } catch {
    response.writeHead(500).end('Could not read the requested file.');
  }
});

server.listen(port, host, () => {
  console.log(`ExamPrep: http://${host}:${server.address().port}/`);
});
