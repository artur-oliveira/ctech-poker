import {createReadStream} from 'node:fs';
import {stat} from 'node:fs/promises';
import {createServer} from 'node:http';
import {extname, resolve, sep} from 'node:path';
import next from 'next';

const hostname = process.env.OG_PREVIEW_HOST || '0.0.0.0';
const port = Number(process.env.OG_PREVIEW_PORT || 3003);
const workspace = resolve(import.meta.dirname, '../..');
const publicRoot = resolve(workspace, 'public');
const app = next({dev: true, dir: import.meta.dirname, hostname, port});
const handle = app.getRequestHandler();

const contentTypes = {
  '.ico': 'image/x-icon',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.webp': 'image/webp'
};

await app.prepare();

const server = createServer(async (request, response) => {
  const pathname = decodeURIComponent(new URL(request.url || '/', `http://${hostname}`).pathname);
  const assetPath = resolve(publicRoot, `.${pathname}`);
  const isPublicPath = assetPath === publicRoot || assetPath.startsWith(`${publicRoot}${sep}`);

  if (isPublicPath) {
    try {
      const asset = await stat(assetPath);
      if (asset.isFile()) {
        response.writeHead(200, {
          'Content-Type': contentTypes[extname(assetPath)] || 'application/octet-stream',
          'Content-Length': asset.size,
          'Cache-Control': 'no-store'
        });
        createReadStream(assetPath).pipe(response);
        return;
      }
    } catch {
      // Let Next render its normal not-found response.
    }
  }

  await handle(request, response);
});

server.listen(port, hostname, () => {
  process.stdout.write(`OG preview ready at http://127.0.0.1:${port}\n`);
});
