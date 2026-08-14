import {spawn} from 'node:child_process';
import {mkdir, writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {OG_PREVIEWS} from '../src/lib/ogPreviews.ts';

const origin = process.env.OG_CAPTURE_ORIGIN || 'http://127.0.0.1:3003';
const captureAll = process.argv.includes('--all');
const captureGuideOnly = process.argv.includes('--guide');
const tableID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';

const ogCaptures = OG_PREVIEWS.map(preview => ({
    ...preview,
    output: resolve('public/og', `${preview.slug}.webp`),
    width: 1200,
    height: 630,
    prepare: preview.slug === 'table' ? 'join-table' : undefined
}));

const guideCaptures = [
    {slug: 'lobby', route: '/lobby'},
    {slug: 'buyin', route: `/table?id=${tableID}&scenario=pre_flop`},
    {slug: 'create-room', route: '/lobby', prepare: 'open-private-room'},
    {slug: 'table-preflop', route: `/table?id=${tableID}&scenario=pre_flop`, prepare: 'join-table'},
    {slug: 'table-flop', route: `/table?id=${tableID}&scenario=flop`, prepare: 'join-table'},
    {slug: 'table-showdown', route: `/table?id=${tableID}&scenario=showdown`, prepare: 'join-table'}
].map(capture => ({
    ...capture,
    title: capture.slug,
    output: resolve('public/guide', `${capture.slug}.webp`),
    width: 1280,
    height: 800
}));

const captures = captureAll ? [...guideCaptures, ...ogCaptures] : captureGuideOnly ? guideCaptures : ogCaptures;
const chrome = spawn('google-chrome', [
    '--headless=new',
    '--no-sandbox',
    '--disable-gpu',
    '--hide-scrollbars',
    '--disable-dev-shm-usage',
    '--remote-debugging-port=9223',
    '--window-size=1920,1280',
    'about:blank'
], {stdio: 'ignore'});

const delay = ms => new Promise(resolveDelay => setTimeout(resolveDelay, ms));

async function browserSocket() {
    for (let attempt = 0; attempt < 50; attempt += 1) {
        try {
            const response = await fetch('http://127.0.0.1:9223/json/version');
            if (response.ok) return (await response.json()).webSocketDebuggerUrl;
        } catch {
        }
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

async function evaluate(send, expression) {
    const result = await send('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true});
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || 'Browser evaluation failed.');
    return result.result?.value;
}

async function waitFor(send, expression, label) {
    for (let attempt = 0; attempt < 120; attempt += 1) {
        if (await evaluate(send, expression)) return;
        await delay(100);
    }
    throw new Error(`Timed out waiting for ${label}.`);
}

async function prepareCapture(send, action) {
    if (action === 'open-private-room') {
        await waitFor(send,
            `Array.from(document.querySelectorAll('button')).some(node => node.textContent?.includes('Mesa privada'))`,
            'the private-room button');
        await evaluate(send,
            `Array.from(document.querySelectorAll('button')).find(node => node.textContent?.includes('Mesa privada'))?.click()`);
        await waitFor(send, `Boolean(document.querySelector('[role="dialog"]'))`, 'the private-room dialog');
    }
    if (action === 'join-table') {
        await waitFor(send,
            `Array.from(document.querySelectorAll('button')).some(node => node.textContent?.includes('Entrar com'))`,
            'the buy-in action');
        await evaluate(send,
            `Array.from(document.querySelectorAll('button')).find(node => node.textContent?.includes('Entrar com'))?.click()`);
        await waitFor(send, `Boolean(document.querySelector('.game'))`, 'the live table');
    }
}

try {
    await mkdir(resolve('public/og'), {recursive: true});
    await mkdir(resolve('public/guide'), {recursive: true});
    const browser = cdp(await browserSocket());
    const {targetId} = await browser.send('Target.createTarget', {url: 'about:blank'});
    const {sessionId} = await browser.send('Target.attachToTarget', {targetId, flatten: true});
    const send = (method, params = {}) => browser.send(method, params, sessionId);
    await send('Page.enable');

    for (const capture of captures) {
        await send('Emulation.setDeviceMetricsOverride', {
            width: capture.width,
            height: capture.height,
            deviceScaleFactor: 1,
            mobile: false
        });
        await send('Page.navigate', {url: `${origin}${capture.route}`});
        await delay(1200);
        await prepareCapture(send, capture.prepare);
        await delay(capture.prepare === 'join-table' ? 2500 : 500);
        await evaluate(send, `(() => {
      document.querySelectorAll('.mock-controls, nextjs-portal').forEach(node => node.remove());
      const style = document.createElement('style');
      style.id = 'capture-stability';
      style.textContent = '*, *::before, *::after { animation-delay: 0s !important; animation-duration: 1ms !important; transition: none !important; }';
      document.head.append(style);
      document.documentElement.style.scrollBehavior = 'auto';
      window.scrollTo(0, 0);
      return true;
    })()`);
        const {data} = await send('Page.captureScreenshot', {
            format: 'webp',
            quality: 100,
            fromSurface: true,
            captureBeyondViewport: false
        });
        await writeFile(capture.output, Buffer.from(data, 'base64'));
        process.stdout.write(`Captured ${capture.output.replace(`${resolve('.')}/`, '')} from ${capture.route}\n`);
    }
    browser.close();
} finally {
    chrome.kill('SIGTERM');
}
