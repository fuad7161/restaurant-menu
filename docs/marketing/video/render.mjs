// Renders scenes.html to PNG frames with headless Firefox (WebDriver BiDi), one frame at a time.
// See README.md for the full steps. Usage:
//   node render.mjs <page url> <frames dir> [fps=30] [seconds=20]
import { mkdirSync, writeFileSync } from 'node:fs';

const [url, out, fpsArg, secArg] = process.argv.slice(2);
const fps = +(fpsArg || 30), seconds = +(secArg || 20);
mkdirSync(out, { recursive: true });

const ws = new WebSocket('ws://127.0.0.1:19222/session');
let id = 0;
const pending = new Map();
ws.onmessage = (m) => { const d = JSON.parse(m.data); if (d.id && pending.has(d.id)) { pending.get(d.id)(d); pending.delete(d.id); } };
await new Promise((r) => (ws.onopen = r));
const send = (method, params = {}) => new Promise((res, rej) => {
  const n = ++id;
  pending.set(n, (d) => (d.type === 'error' ? rej(new Error(method + ': ' + d.message)) : res(d.result)));
  ws.send(JSON.stringify({ id: n, method, params }));
});

try {
  await send('session.new', { capabilities: {} });
  const ctx = (await send('browsingContext.getTree', {})).contexts[0].context;
  // 540 × 960 CSS pixels at 2× = a 1080 × 1920 video
  await send('browsingContext.setViewport', { context: ctx, viewport: { width: 540, height: 960 }, devicePixelRatio: 2 });
  await send('browsingContext.navigate', { context: ctx, url, wait: 'complete' });
  const js = async (e) => {
    const r = await send('script.evaluate', { expression: e, target: { context: ctx }, awaitPromise: true });
    if (r.type === 'exception') throw new Error(r.exceptionDetails.text);
    return r.result.value;
  };
  await js('document.fonts.ready.then(() => true)');
  const total = Math.round(fps * seconds);
  for (let f = 0; f < total; f++) {
    await js(`setTime(${(f * 1000) / fps})`);
    const shot = await send('browsingContext.captureScreenshot', { context: ctx });
    writeFileSync(`${out}/${String(f).padStart(4, '0')}.png`, Buffer.from(shot.data, 'base64'));
    if (f % fps === 0) process.stdout.write(`${f / fps}s `);
  }
  console.log(`\n${total} frames in ${out}`);
} finally {
  await send('session.end', {}).catch(() => {});
  process.exit(0);
}
