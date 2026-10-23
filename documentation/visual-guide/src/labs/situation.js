import { RUN, COUNTS } from '../story.js';
import { $, s, h, clamp, fmt, Clock, transport, control, toggle } from './kit.js';

const W = 1080, H = 540;
const PLOT = { x: 70, y: 60, w: 640, h: 250 };
const DUR = 600;
const ENTER = 30;
const state = { noise: 1.2, exit: 28, minDur: 60, seed: 7, mode: 'synthetic', picked: null };
const PHASE = { candidate: '#6f8cc7', watch: '#f5b94a', cooling: '#ff7a59', open: '#ff7a59', quiet: '#2a3a63', pending: '#f5b94a' };

function rand(seed) { let a = seed >>> 0; return () => { a = (a + 0x6d2b79f5) >>> 0; let t = a; t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296; }; }
const base = (t) => (t < 200 ? 27 + (t / 200) * 6 : t < 380 ? 33 : 33 - ((t - 380) / 180) * 7);

function signal() {
  const r = rand(state.seed);
  let n = 0;
  return Array.from({ length: DUR / 2 + 1 }, (_, i) => { n = n * 0.55 + (r() - 0.5) * 2; return { t: i * 2, v: base(i * 2) + n * state.noise }; });
}

function machines(sig) {
  const naive = []; const spec = [];
  let ns = false, st = 'quiet', since = 0;
  const versions = [];
  for (const p of sig) {
    const above = p.v > ENTER;
    naive.push({ t: p.t, on: above });
    if (st === 'quiet' && above) { st = 'pending'; since = p.t; }
    else if (st === 'pending') { if (!above) st = 'quiet'; else if (p.t - since >= state.minDur) { st = 'open'; versions.push({ v: versions.length + 1, phase: 'open', t: p.t, mean: p.v }); } }
    else if (st === 'open' && p.v <= state.exit) { st = 'quiet'; versions.push({ v: versions.length + 1, phase: 'closed', t: p.t, mean: p.v }); }
    spec.push({ t: p.t, st });
  }
  const flips = (arr, f) => arr.reduce((n, x, i) => n + (i && f(x) !== f(arr[i - 1]) ? 1 : 0), 0);
  return { naive, spec, versions, naiveFlips: (t) => flips(naive.filter((x) => x.t <= t), (x) => x.on), specFlips: (t) => versions.filter((v) => v.t <= t).length };
}

const svg = s('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Signal chart, two state machines and a stack of published versions' });
$('#stage').append(svg);
const chips = $('#chips');
const detail = h('div', { class: 'chip info', style: 'min-width:320px' }, 'Click a version slab to inspect it.');
let clock;

const X = (t) => PLOT.x + (t / DUR) * PLOT.w;
const Yv = (v) => PLOT.y + PLOT.h - ((v - 24) / 12) * PLOT.h;

function render(p) {
  const sig = signal();
  const M = machines(sig);
  svg.replaceChildren();
  const add = (n) => svg.append(n);
  const rec = state.mode === 'recorded';
  add(s('text', { x: PLOT.x, y: 28, class: 'big' }, rec ? 'Recorded run: mean temperature at each published version' : 'Signal and two ways to read it'));
  add(s('text', { x: PLOT.x, y: 46, class: 'small mute' }, rec ? `${COUNTS.versions} versions, two per half-minute slide: provisional, then on-time` : 'synthetic temperature with noise, seconds on the x axis'));
  for (let v = 24; v <= 36; v += 4) { add(s('line', { x1: PLOT.x, x2: PLOT.x + PLOT.w, y1: Yv(v), y2: Yv(v), stroke: '#22305a' })); add(s('text', { x: PLOT.x - 10, y: Yv(v) + 4, class: 'small mute', 'text-anchor': 'end' }, `${v}°`)); }
  if (!rec) {
    add(s('line', { x1: PLOT.x, x2: PLOT.x + PLOT.w, y1: Yv(ENTER), y2: Yv(ENTER), stroke: '#f5b94a', 'stroke-dasharray': '6 5', 'stroke-width': 1.6 }));
    add(s('text', { x: PLOT.x + PLOT.w + 6, y: Yv(ENTER) + 4, class: 'small', style: 'fill:#f5b94a' }, 'enter 30°'));
    add(s('line', { x1: PLOT.x, x2: PLOT.x + PLOT.w, y1: Yv(state.exit), y2: Yv(state.exit), stroke: '#4cc9f0', 'stroke-dasharray': '6 5', 'stroke-width': 1.6 }));
    add(s('text', { x: PLOT.x + PLOT.w + 6, y: Yv(state.exit) + 4, class: 'small', style: 'fill:#4cc9f0' }, `exit ${state.exit}°`));
    const pts = sig.filter((q) => q.t <= p).map((q) => `${X(q.t).toFixed(1)},${Yv(q.v).toFixed(1)}`);
    if (pts.length > 1) add(s('polyline', { points: pts.join(' '), fill: 'none', stroke: '#ff7a59', 'stroke-width': 2.2, 'stroke-linejoin': 'round' }));
    add(s('line', { x1: X(p), x2: X(p), y1: PLOT.y, y2: PLOT.y + PLOT.h, stroke: '#fff', 'stroke-width': 1.6, opacity: 0.8 }));
    const bar = (label, y, cells, color) => {
      add(s('text', { x: PLOT.x, y: y - 8, class: 'mid' }, label));
      add(s('rect', { x: PLOT.x, y, width: PLOT.w, height: 26, rx: 6, fill: '#101a38' }));
      cells.filter((c) => c.t <= p).forEach((c, i, arr) => { const w = X(c.t + 2) - X(c.t); add(s('rect', { x: X(c.t), y, width: Math.max(1, w + 0.5), height: 26, fill: color(c) })); });
    };
    bar('Naive: one threshold at 30°', 350, M.naive, (c) => (c.on ? PHASE.open : PHASE.quiet));
    bar('Spec rule: enter above 30° held for the minimum duration, exit at the exit threshold', 410, M.spec, (c) => PHASE[c.st]);
    [['quiet', 'quiet'], ['pending', 'pending (above, not yet held)'], ['open', 'open']].forEach(([k, t], i) => { add(s('rect', { x: PLOT.x + i * 230, y: 452, width: 12, height: 12, rx: 3, fill: PHASE[k] })); add(s('text', { x: PLOT.x + 18 + i * 230, y: 463, class: 'small mute' }, t)); });
  } else {
    const pts = RUN.versions.map((v, i) => [PLOT.x + (i / 21) * PLOT.w, Yv(v.tempMean), v]);
    add(s('polyline', { points: pts.map((q) => `${q[0].toFixed(1)},${q[1].toFixed(1)}`).join(' '), fill: 'none', stroke: '#ffffff44', 'stroke-width': 1.6 }));
    pts.forEach(([x, y, v]) => add(s('circle', { cx: x, cy: y, r: v.completeness === 'on_time' ? 7 : 5, fill: v.completeness === 'on_time' ? PHASE[v.phase] : 'transparent', stroke: PHASE[v.phase], 'stroke-width': 2 })));
    add(s('text', { x: PLOT.x, y: 350, class: 'mid' }, 'Filled dot: on-time version. Hollow dot: provisional version.'));
    add(s('text', { x: PLOT.x, y: 374, class: 'small mute' }, `Mean rose from ${fmt(RUN.versions[0].tempMean, 2)} to ${fmt(RUN.versions.at(-1).tempMean, 2)} °C at ${fmt(RUN.versions[0].tempSlope)} °C/h. The phase changed only twice.`));
    [['candidate', 'candidate'], ['watch', 'watch'], ['cooling', 'cooling']].forEach(([k, t], i) => { add(s('rect', { x: PLOT.x + i * 170, y: 400, width: 12, height: 12, rx: 3, fill: PHASE[k] })); add(s('text', { x: PLOT.x + 18 + i * 170, y: 411, class: 'small mute' }, t)); });
  }

  const stackX = 800, stackBottom = 440;
  add(s('text', { x: stackX, y: 28, class: 'big' }, 'Published versions'));
  add(s('text', { x: stackX, y: 46, class: 'small mute' }, 'immutable once published'));
  const list = rec ? RUN.versions.map((v) => ({ v: v.v, phase: v.phase, t: v.t, mean: v.tempMean, rec: v })) : M.versions.filter((v) => v.t <= p);
  const sh = rec ? 14 : 28;
  list.forEach((v, i) => {
    const y = stackBottom - (i + 1) * (sh + 3);
    const picked = state.picked === v.v;
    const g = s('g', { style: 'cursor:pointer', tabindex: 0, role: 'button', 'aria-label': `version ${v.v}` });
    g.append(s('rect', { x: stackX, y, width: 230, height: sh, rx: 5, fill: PHASE[v.phase === 'closed' ? 'quiet' : v.phase] ?? PHASE.quiet, stroke: picked ? '#fff' : '#ffffff33', 'stroke-width': picked ? 2.4 : 1, opacity: rec && v.rec.completeness === 'provisional' ? 0.55 : 1 }));
    if (!rec || sh >= 20) g.append(s('text', { x: stackX + 12, y: y + sh / 2 + 5, class: 'mid', style: 'fill:#06101f;font-weight:700' }, `v${v.v} · ${v.phase}`));
    g.onclick = () => { state.picked = v.v; render(clock.t); showDetail(v, rec); };
    g.onkeydown = (e) => { if (e.key === 'Enter') g.onclick(); };
    add(g);
  });
  if (!list.length) add(s('text', { x: stackX, y: stackBottom - 10, class: 'small mute' }, 'No version yet: nothing has held long enough.'));

  const nf = M.naiveFlips(p), sf = M.specFlips(p);
  chips.replaceChildren(
    ...(rec ? [h('div', { class: 'chip info' }, [h('b', {}, `${COUNTS.versions} versions`), h('small', {}, `${COUNTS.phaseCounts.candidate} candidate · ${COUNTS.phaseCounts.watch} watch · ${COUNTS.phaseCounts.cooling} cooling`)]), h('div', { class: 'chip good' }, [h('b', {}, '2 phase changes'), h('small', {}, 'the rest are provisional → on-time pairs')])]
      : [h('div', { class: 'chip bad' }, [h('b', {}, `${nf} flips`), h('small', {}, 'naive single threshold')]), h('div', { class: 'chip good' }, [h('b', {}, `${sf} versions`), h('small', {}, 'spec rule: enter, hold, exit')])]),
    detail,
  );
}

function showDetail(v, rec) {
  detail.replaceChildren(...(rec
    ? [h('b', {}, `Version ${v.v} · ${v.rec.phase} · severity ${v.rec.severity}`), h('small', {}, `${v.rec.completeness.replace('_', '-')} · mean ${fmt(v.rec.tempMean, 2)} °C · slope ${fmt(v.rec.tempSlope)} °C/h · ${v.rec.evidenceCount} readings behind it`)]
    : [h('b', {}, `Version ${v.v} · ${v.phase} at ${v.t} s`), h('small', {}, `temperature ${fmt(v.mean, 1)} °C when it published`)]), h('button', { class: 'btn', style: 'margin-top:8px', onclick: () => { detail.dataset.refused = '1'; detail.querySelector('.refuse').textContent = `Refused. Published versions never change. A correction would publish version ${(rec ? COUNTS.versions : machines(signal()).versions.length) + 1}.`; } }, 'Try to edit it'), h('small', { class: 'refuse', style: 'color:#ff8aa0' }, ''));
}

clock = new Clock({ duration: DUR, speed: 1, onFrame: (t) => { render(t); sync(); } });
const sync = transport($('#transport'), clock, { label: 'Play' });
const ctl = $('#ctl');
control(ctl, { id: 'noise', label: 'Noise', min: 0, max: 3, step: 0.1, value: state.noise, unit: ' °C', format: (v) => v.toFixed(1), onInput: (v) => { state.noise = v; render(clock.t); } });
control(ctl, { id: 'exit', label: 'Exit threshold', min: 24, max: 30, step: 0.5, value: state.exit, unit: ' °C', format: (v) => v.toFixed(1), onInput: (v) => { state.exit = v; render(clock.t); } });
control(ctl, { id: 'mind', label: 'Minimum duration', min: 0, max: 120, step: 5, value: state.minDur, unit: ' s', onInput: (v) => { state.minDur = v; render(clock.t); } });
ctl.append(h('button', { class: 'btn', onclick: () => { state.seed = Math.floor(Math.random() * 1e6); render(clock.t); } }, 'Re-roll noise'));
const modeBtn = h('button', { class: 'btn', onclick: () => { state.mode = state.mode === 'recorded' ? 'synthetic' : 'recorded'; modeBtn.textContent = state.mode === 'recorded' ? 'Back to the synthetic signal' : 'Show the recorded run'; state.picked = null; detail.textContent = 'Click a version slab to inspect it.'; render(clock.t); } }, 'Show the recorded run');
ctl.append(modeBtn);
clock.seek(DUR);
