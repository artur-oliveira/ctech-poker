import {spawn} from 'node:child_process';
import {mkdir, writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';

const origin = process.env.OG_CAPTURE_ORIGIN || 'http://127.0.0.1:3003';
const outputDir = resolve('public/og');
const routes = [
  'home', 'guide', 'poker-rules', 'profile', 'lobby', 'table', 'hands', 'hand-history',
  'hand-replay', 'shared-hand', 'leaderboard', 'achievements'
].map(slug => [slug, `/${slug}`]);

const chrome = spawn('google-chrome', [
  '--headless=new',
  '--no-sandbox',
  '--disable-gpu',
  '--hide-scrollbars',
  '--disable-dev-shm-usage',
  '--remote-debugging-port=9223',
  '--window-size=1200,630',
  'about:blank'
], {stdio: 'ignore'});

const delay = ms => new Promise(resolveDelay => setTimeout(resolveDelay, ms));

async function browserSocket() {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    try {
      const response = await fetch('http://127.0.0.1:9223/json/version');
      if (response.ok) return (await response.json()).webSocketDebuggerUrl;
    } catch {}
    await delay(100);
  }
  throw new Error('Chrome DevTools did not become ready.');
}

function cdp(socketUrl) {
  const socket = new WebSocket(socketUrl);
  let nextId = 0;
  const pending = new Map();
  socket.onmessage = event => {
    const message = JSON.parse(event.data);
    const request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id);
    if (message.error) request.reject(new Error(message.error.message));
    else request.resolve(message.result);
  };
  const ready = new Promise((resolveReady, rejectReady) => {
    socket.onopen = resolveReady;
    socket.onerror = rejectReady;
  });
  return {
    async send(method, params = {}, sessionId) {
      await ready;
      const id = ++nextId;
      socket.send(JSON.stringify({id, method, params, ...(sessionId ? {sessionId} : {})}));
      return new Promise((resolveRequest, rejectRequest) => pending.set(id, {
        resolve: resolveRequest,
        reject: rejectRequest
      }));
    },
    close: () => socket.close()
  };
}

try {
  await mkdir(outputDir, {recursive: true});
  const browser = cdp(await browserSocket());
  const {targetId} = await browser.send('Target.createTarget', {url: 'about:blank'});
  const {sessionId} = await browser.send('Target.attachToTarget', {targetId, flatten: true});
  const send = (method, params = {}) => browser.send(method, params, sessionId);
  await send('Page.enable');
  await send('Emulation.setDeviceMetricsOverride', {
    width: 1200,
    height: 630,
    deviceScaleFactor: 1,
    mobile: false
  });

  for (const [slug, path] of routes) {
    await send('Page.navigate', {url: `${origin}${path}`});
    await delay(500);
    await send('Runtime.evaluate', {
      expression: `document.querySelectorAll('.mock-controls, nextjs-portal').forEach(node => node.remove())`
    });
    const {data} = await send('Page.captureScreenshot', {
      format: 'png',
      fromSurface: true,
      captureBeyondViewport: false
    });
    await writeFile(resolve(outputDir, `${slug}.png`), Buffer.from(data, 'base64'));
    process.stdout.write(`Captured public/og/${slug}.png\n`);
  }
  browser.close();
} finally {
  chrome.kill('SIGTERM');
}
