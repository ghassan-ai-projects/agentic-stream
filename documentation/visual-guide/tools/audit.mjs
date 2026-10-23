import { launch, evaluate } from './chrome.mjs';
import { resolve } from 'node:path';

const [page, size = '1920x1080', step = '1'] = process.argv.slice(2);
const interactive = process.env.INTERACTIVE === '1';
const [w, h] = size.split('x').map(Number);
const c = await launch({ width: w, height: h });
await c.cdp.send('Runtime.enable'); await c.cdp.send('Page.enable');
await c.cdp.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false });
await c.cdp.send('Page.navigate', { url: `file://${resolve(page)}${interactive ? '' : `?film=1&w=${w}&h=${h}`}` });
await new Promise((r) => setTimeout(r, 1800));
if (interactive) await evaluate(c.cdp, "document.querySelector('.start').classList.add('gone')");
const total = await evaluate(c.cdp, 'window.__total');
const bad = [];
for (let t = 0; t < total; t += Number(step)) {
  await evaluate(c.cdp, `window.__seek(${t})`);
  const issues = await evaluate(c.cdp, 'window.__audit()');
  if (issues.length) bad.push(`${t.toFixed(1)}s: ${issues.join(' | ')}`);
}
console.log(bad.length ? bad.join('\n') : `clean at ${size} across ${total.toFixed(0)} s`);
console.log(`${bad.length} timestamps with issues`);
await c.close();
