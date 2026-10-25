import { launch, evaluate } from './chrome.mjs';
import { createHash } from 'node:crypto';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(fileURLToPath(new URL('.', import.meta.url)), '..');
const frames = (process.argv[2] ?? '30,30.0333,30.0667,75,75.0333,110,110.0333').split(',').map(Number);

async function run() {
  const c = await launch({ width: 1920, height: 1080 });
  await c.cdp.send('Runtime.enable'); await c.cdp.send('Page.enable');
  await c.cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
  await c.cdp.send('Page.navigate', { url: `file://${join(root, 'world.html')}?film=1&w=1920&h=1080` });
  await new Promise((r) => setTimeout(r, 2200));
  const hashes = [];
  for (const t of frames) {
    await evaluate(c.cdp, `window.__seek(${t})`);
    const s = await c.cdp.send('Page.captureScreenshot', { format: 'png' });
    hashes.push(createHash('sha256').update(s.data).digest('hex').slice(0, 12));
  }
  await c.close();
  return hashes;
}
const a = await run();
const b = await run();
frames.forEach((t, i) => console.log(`${a[i] === b[i] ? 'SAME' : 'DIFF'}  t=${t}  ${a[i]} ${b[i]}`));
process.exit(a.every((h, i) => h === b[i]) ? 0 : 1);
