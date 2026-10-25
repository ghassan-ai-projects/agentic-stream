import { launch, evaluate } from './chrome.mjs';
import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const [page, out, size = '1280x900', script = '', wait = '600'] = process.argv.slice(2);
const [w, h] = size.split('x').map(Number);
const c = await launch({ width: w, height: h });
const logs = [];
c.cdp.on((m) => {
  if (m.method === 'Runtime.exceptionThrown') logs.push('EXC ' + (m.params.exceptionDetails.exception?.description ?? m.params.exceptionDetails.text).slice(0, 400));
  if (m.method === 'Runtime.consoleAPICalled' && ['error', 'warning'].includes(m.params.type)) logs.push(m.params.type.toUpperCase() + ' ' + m.params.args.map((a) => a.value ?? a.description).join(' ').slice(0, 300));
  if (m.method === 'Network.loadingFailed') logs.push('NETFAIL ' + m.params.errorText + ' ' + (m.params.canceled ? 'canceled' : ''));
});
await c.cdp.send('Runtime.enable'); await c.cdp.send('Page.enable'); await c.cdp.send('Network.enable');
await c.cdp.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 600 });
await c.cdp.send('Page.navigate', { url: page.startsWith('http') ? page : `file://${resolve(page)}` });
await new Promise((r) => setTimeout(r, 1500));
if (script) await evaluate(c.cdp, script);
await new Promise((r) => setTimeout(r, Number(wait)));
const m = await c.cdp.send('Page.getLayoutMetrics');
const full = process.env.FULL === '1';
const clip = full ? { x: 0, y: 0, width: w, height: Math.ceil(m.cssContentSize.height), scale: 1 } : undefined;
const s = await c.cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: full, clip });
writeFileSync(out, Buffer.from(s.data, 'base64'));
const overflow = await evaluate(c.cdp, 'document.documentElement.scrollWidth > document.documentElement.clientWidth + 1');
console.log(logs.length ? logs.join('\n') : 'no console errors', overflow ? '| HORIZONTAL OVERFLOW' : '');
await c.close();
