import { launch, evaluate } from './chrome.mjs';
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync, copyFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const { GENERATED } = await import('../src/timeline.generated.js');
const POSTER_T = GENERATED.captions.find((c) => c.chapter === 'action' && c.i === 4).end + 0.5;
const root = join(here, '..');
const frames = resolve(process.argv[2] ?? join(tmpdir(), 'agentic-stream-film-frames'));
const FPS = 30;
const W = 1920, H = 1080;
const encodeOnly = process.env.ENCODE_ONLY === '1';
const only = process.env.ONLY ? process.env.ONLY.split(',').map(Number) : null;
const probe = (file) => Number(execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', file]).toString());

async function capture() {
  const c = await launch({ width: W, height: H });
  await c.cdp.send('Runtime.enable');
  await c.cdp.send('Page.enable');
  await c.cdp.send('Emulation.setDeviceMetricsOverride', { width: W, height: H, deviceScaleFactor: 1, mobile: false });
  await c.cdp.send('Page.navigate', { url: `file://${join(root, 'world.html')}?film=1&w=${W}&h=${H}` });
  await new Promise((r) => setTimeout(r, 2500));
  const total = await evaluate(c.cdp, 'window.__total');
  const n = Math.round(total * FPS);
  const start = Date.now();
  for (let i = 0; i < n; i++) {
    if (only && !only.includes(i)) continue;
    await evaluate(c.cdp, `window.__seek(${(i / FPS).toFixed(4)})`);
    const shot = await c.cdp.send('Page.captureScreenshot', { format: 'jpeg', quality: 94 });
    writeFileSync(join(frames, `f${String(i).padStart(5, '0')}.jpg`), Buffer.from(shot.data, 'base64'));
    if (i % 150 === 0) console.log(`frame ${i}/${n} (${((Date.now() - start) / 1000).toFixed(0)} s)`);
  }
  await c.close();
}

function encode(total) {
  const out = join(root, 'film/agentic-stream-explained.mp4');
  const fadeOut = (total - 1).toFixed(2);
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-framerate', String(FPS), '-i', join(frames, 'f%05d.jpg'), '-i', join(root, 'film/narration.m4a'),
    '-vf', `fade=t=in:st=0:d=0.8,fade=t=out:st=${fadeOut}:d=1`, '-af', `afade=t=out:st=${fadeOut}:d=1`,
    '-c:v', 'libx264', '-preset', 'slow', '-crf', '27', '-maxrate', '2800k', '-bufsize', '5600k', '-x264-params', 'aq-mode=3',
    '-pix_fmt', 'yuv420p', '-c:a', 'aac', '-b:a', '160k', '-shortest', '-movflags', '+faststart', out]);
  copyFileSync(join(frames, `f${String(Math.round(POSTER_T * FPS)).padStart(5, '0')}.jpg`), join(root, 'film/poster.jpg'));
  console.log('wrote', out);
}

mkdirSync(frames, { recursive: true });
if (!encodeOnly) await capture();
if (!only) encode(probe(join(root, 'film/narration.m4a')));
