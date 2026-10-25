import { launch, evaluate } from './chrome.mjs';
import { writeFileSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';

const [page, times, outDir, size = '1600x900'] = process.argv.slice(2);
const [w, h] = size.split('x').map(Number);
mkdirSync(outDir, { recursive: true });
const c = await launch({ width: w, height: h });
const logs = [];
c.cdp.on((m) => {
  if (m.method === 'Runtime.exceptionThrown') logs.push('EXC ' + (m.params.exceptionDetails.exception?.description ?? m.params.exceptionDetails.text).slice(0, 400));
  if (m.method === 'Runtime.consoleAPICalled' && ['error', 'warning'].includes(m.params.type)) logs.push(m.params.type.toUpperCase() + ' ' + m.params.args.map((a) => a.value ?? a.description).join(' ').slice(0, 300));
});
await c.cdp.send('Runtime.enable');
await c.cdp.send('Page.enable');
await c.cdp.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false });
const sep = page.includes('?') ? '&' : '?';
await c.cdp.send('Page.navigate', { url: `file://${resolve(page)}${sep}film=1&w=${w}&h=${h}` });
await new Promise((r) => setTimeout(r, 1800));
for (const t of times.split(',')) {
  for (let k = 0; k < 10; k++) await evaluate(c.cdp, `window.__seek(${Number(t)})`);
  await new Promise((r) => setTimeout(r, 120));
  const s = await c.cdp.send('Page.captureScreenshot', { format: 'png' });
  writeFileSync(join(outDir, `t${String(t).replace('.', '_')}.png`), Buffer.from(s.data, 'base64'));
}
console.log(logs.length ? logs.join('\n') : 'no console errors');
await c.close();
