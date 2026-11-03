import { RUN, COUNTS } from '../story.js';
import { rng, fmt } from '../util.js';
import { $, s, h, plural, wrap, keepFocus, Clock, transport, control } from './kit.js';

const W = 1080, H = 540;
const PLOT = { x: 70, y: 60, w: 640, h: 250 };
const DUR = 720;
const ENTER = RUN.spec.enterCelsius;
const holdOf = (from, to) => RUN.spec.transitions.find((t) => t.from === from && t.to === to).minDurationSeconds;
const state = { noise: 1.2, exit: RUN.spec.exitCelsius, minDur: holdOf('candidate', 'watch'), recHold: holdOf('recovering', 'resolved'), seed: 7, mode: 'synthetic', picked: null };
const PHASE = { candidate: '#6f8cc7', watch: '#f5b94a', cooling: '#ff7a59', open: '#ff7a59', recovering: '#4cc9f0', resolved: '#2a3a63', quiet: '#2a3a63', pending: '#f5b94a' };
const PICK = 'Click a version slab to inspect it.';

const base = (t) => (t < 200 ? 27 + (t / 200) * 6 : t < 380 ? 33 : Math.max(25.5, 33 - ((t - 380) / 180) * 7));

function signal() {
  const r = rng(state.seed);
  let n = 0;
  return Array.from({ length: DUR / 2 + 1 }, (_, i) => { n = n * 0.55 + (r() - 0.5) * 2; return { t: i * 2, v: base(i * 2) + n * state.noise }; });
}

function step(m, p) {
  const above = p.v > ENTER;
  if (m.st === 'quiet' && above) Object.assign(m, { st: 'pending', since: p.t });
  else if (m.st === 'pending') confirmEntry(m, p, above);
  else if (m.st === 'open' && p.v <= state.exit) { publish(m, 'recovering', p); m.since = p.t; }
  else if (m.st === 'recovering') confirmExit(m, p);
}

function confirmEntry(m, p, above) {
  if (!above) m.st = 'quiet';
  else if (p.t - m.since >= state.minDur) publish(m, 'open', p);
}

function confirmExit(m, p) {
  if (p.v > state.exit) m.since = null;
  else if (m.since == null) m.since = p.t;
  else if (p.t - m.since >= state.recHold) publish(m, 'resolved', p, 'quiet');
}

function publish(m, phase, p, next = phase) {
  m.st = next;
  m.versions.push({ v: m.versions.length + 1, phase, t: p.t, mean: p.v });
}

function machines(sig) {
  const m = { st: 'quiet', since: 0, versions: [] };
  const naive = sig.map((p) => ({ t: p.t, on: p.v > ENTER }));
  const spec = sig.map((p) => { step(m, p); return { t: p.t, st: m.st }; });
  const flips = (arr) => arr.reduce((n, x, i) => n + (i && x.on !== arr[i - 1].on ? 1 : 0), 0);
  return { naive, spec, versions: m.versions, naiveFlips: (t) => flips(naive.filter((x) => x.t <= t)), specFlips: (t) => m.versions.filter((v) => v.t <= t).length };
}

const phaseChanges = RUN.versions.filter((v, i) => i && v.phase !== RUN.versions[i - 1].phase).length;
const svg = s('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Signal chart, two state machines and a stack of published versions' });
$('#stage').append(svg);
const chips = $('#chips');
const detail = h('div', { class: 'chip info', style: 'min-width:320px' }, PICK);
let clock;

const X = (t) => PLOT.x + (t / DUR) * PLOT.w;
const Yv = (v) => PLOT.y + PLOT.h - ((v - 24) / 12) * PLOT.h;

function render(p) { keepFocus(svg, () => draw(p)); }

function draw(p) {
  const sig = signal();
  const M = machines(sig);
  svg.replaceChildren();
  const add = (n) => svg.append(n);
  const rec = state.mode === 'recorded';
  header(add, rec);
  grid(add, rec);
  if (rec) recordedChart(add); else syntheticChart(add, sig, M, p);
  stack(add, rec ? RUN.versions.map((v) => ({ v: v.v, phase: v.phase, t: v.t, mean: v.tempMean, rec: v })) : M.versions.filter((v) => v.t <= p), rec);
  const nf = M.naiveFlips(p), sf = M.specFlips(p);
  chips.replaceChildren(...(rec ? recordedChips() : [h('div', { class: 'chip bad' }, [h('b', {}, plural(nf, 'flip')), h('small', {}, 'naive single threshold')]), h('div', { class: 'chip good' }, [h('b', {}, plural(sf, 'version')), h('small', {}, 'spec rule: enter, hold, recover, hold')])]), detail);
}

function header(add, rec) {
  add(s('text', { x: PLOT.x, y: 28, class: 'big' }, rec ? 'Recorded run: mean temperature at each published version' : 'Signal and two ways to read it'));
  add(s('text', { x: PLOT.x, y: 46, class: 'small mute' }, rec ? `${plural(COUNTS.versions, 'version')}, two per half-minute slide: provisional, then on-time` : 'synthetic temperature with noise, seconds on the x axis'));
}

function grid(add, rec) {
  for (let v = 24; v <= 36; v += 4) { add(s('line', { x1: PLOT.x, x2: PLOT.x + PLOT.w, y1: Yv(v), y2: Yv(v), stroke: '#22305a' })); add(s('text', { x: PLOT.x - 10, y: Yv(v) + 4, class: 'small mute', 'text-anchor': 'end' }, `${v}°`)); }
  const exit = rec ? RUN.spec.exitCelsius : state.exit;
  add(s('line', { x1: PLOT.x, x2: PLOT.x + PLOT.w, y1: Yv(ENTER), y2: Yv(ENTER), stroke: '#f5b94a', 'stroke-dasharray': '6 5', 'stroke-width': 1.6 }));
  add(s('text', { x: PLOT.x + PLOT.w + 6, y: Yv(ENTER) + 4, class: 'small', style: 'fill:#f5b94a' }, `enter ${ENTER}°`));
  add(s('line', { x1: PLOT.x, x2: PLOT.x + PLOT.w, y1: Yv(exit), y2: Yv(exit), stroke: '#4cc9f0', 'stroke-dasharray': '6 5', 'stroke-width': 1.6 }));
  add(s('text', { x: PLOT.x + PLOT.w + 6, y: Yv(exit) + 4, class: 'small', style: `fill:#4cc9f0` }, `exit ${exit}°`));
}

function syntheticChart(add, sig, M, p) {
  const pts = sig.filter((q) => q.t <= p).map((q) => `${X(q.t).toFixed(1)},${Yv(q.v).toFixed(1)}`);
  if (pts.length > 1) add(s('polyline', { points: pts.join(' '), fill: 'none', stroke: '#ff7a59', 'stroke-width': 2.2, 'stroke-linejoin': 'round' }));
  add(s('line', { x1: X(p), x2: X(p), y1: PLOT.y, y2: PLOT.y + PLOT.h, stroke: '#fff', 'stroke-width': 1.6, opacity: 0.8 }));
  stateBar(add, 'Naive: one threshold at ' + ENTER + '°', 350, M.naive, p, (c) => (c.on ? PHASE.open : PHASE.quiet));
  stateBar(add, `Spec rule: enter above ${ENTER}° held ${state.minDur} s; leave at ${state.exit}° held ${state.recHold} s`, 410, M.spec, p, (c) => PHASE[c.st]);
  [['quiet', 'quiet'], ['pending', 'pending (above, not yet held)'], ['open', 'open'], ['recovering', 'recovering']].forEach(([k, t], i) => { add(s('rect', { x: PLOT.x + i * 170 + (i > 1 ? 70 : 0), y: 452, width: 12, height: 12, rx: 3, fill: PHASE[k] })); add(s('text', { x: PLOT.x + 18 + i * 170 + (i > 1 ? 70 : 0), y: 463, class: 'small mute' }, t)); });
}

function stateBar(add, label, y, cells, p, color) {
  add(s('text', { x: PLOT.x, y: y - 8, class: 'mid' }, label));
  add(s('rect', { x: PLOT.x, y, width: PLOT.w, height: 26, rx: 6, fill: '#101a38' }));
  cells.filter((c) => c.t <= p).forEach((c) => add(s('rect', { x: X(c.t), y, width: Math.max(1, X(c.t + 2) - X(c.t) + 0.5), height: 26, fill: color(c) })));
}

function recordedChart(add) {
  const last = RUN.versions.length - 1;
  const pts = RUN.versions.map((v, i) => [PLOT.x + (i / last) * PLOT.w, Yv(v.tempMean), v]);
  add(s('polyline', { points: pts.map((q) => `${q[0].toFixed(1)},${q[1].toFixed(1)}`).join(' '), fill: 'none', stroke: '#ffffff44', 'stroke-width': 1.6 }));
  pts.forEach(([x, y, v]) => add(s('circle', { cx: x, cy: y, r: v.completeness === 'on_time' ? 7 : 5, fill: v.completeness === 'on_time' ? PHASE[v.phase] : 'transparent', stroke: PHASE[v.phase], 'stroke-width': 2 })));
  add(s('text', { x: PLOT.x, y: 350, class: 'mid' }, 'Filled dot: on-time version. Hollow dot: provisional version.'));
  const first = RUN.versions[0], end = RUN.versions.at(-1);
  const story = `Mean rose from ${fmt(first.tempMean, 2)} to ${fmt(end.tempMean, 2)} °C at ${fmt(first.tempSlope)} °C/h. The phase changed ${phaseChanges === 2 ? 'only twice' : `${phaseChanges} times`}. The mean was already above the enter line at version 1, so it never crossed ${ENTER}° and never fell to ${RUN.spec.exitCelsius}°: the occurrence opened at once as candidate, and the changes to watch and cooling came from the slope rule after holds of ${holdOf('candidate', 'watch')} s and ${holdOf('watch', 'cooling')} s.`;
  wrap(story, 84).forEach((t, i) => add(s('text', { x: PLOT.x, y: 374 + i * 16, class: 'small mute' }, t)));
  ['candidate', 'watch', 'cooling'].forEach((k, i) => { add(s('rect', { x: PLOT.x + i * 170, y: 452, width: 12, height: 12, rx: 3, fill: PHASE[k] })); add(s('text', { x: PLOT.x + 18 + i * 170, y: 463, class: 'small mute' }, k)); });
}

function stack(add, list, rec) {
  const stackX = 800, stackBottom = 440;
  add(s('text', { x: stackX, y: 28, class: 'big' }, 'Published versions'));
  add(s('text', { x: stackX, y: 46, class: 'small mute' }, 'immutable once published'));
  const sh = rec ? 14 : 28;
  list.forEach((v, i) => {
    const y = stackBottom - (i + 1) * (sh + 3);
    const picked = state.picked === v.v;
    const g = s('g', { style: 'cursor:pointer', tabindex: 0, role: 'button', 'aria-label': `version ${v.v}`, 'aria-pressed': picked ? 'true' : 'false', 'data-focus': `v${v.v}` });
    g.append(s('rect', { x: stackX, y, width: 230, height: sh, rx: 5, fill: PHASE[v.phase] ?? PHASE.quiet, stroke: picked ? '#fff' : '#ffffff33', 'stroke-width': picked ? 2.4 : 1, opacity: rec && v.rec.completeness === 'provisional' ? 0.55 : 1 }));
    if (!rec || sh >= 20) g.append(s('text', { x: stackX + 12, y: y + sh / 2 + 5, class: 'mid', style: 'fill:#06101f;font-weight:700' }, `v${v.v} · ${v.phase}`));
    g.onclick = () => { state.picked = v.v; render(clock.t); showDetail(v, rec); };
    g.onkeydown = (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); g.onclick(); } };
    add(g);
  });
  if (!list.length) add(s('text', { x: stackX, y: stackBottom - 10, class: 'small mute' }, 'No version yet: nothing has held long enough.'));
}

function recordedChips() {
  return [h('div', { class: 'chip info' }, [h('b', {}, plural(COUNTS.versions, 'version')), h('small', {}, `${COUNTS.phaseCounts.candidate} candidate · ${COUNTS.phaseCounts.watch} watch · ${COUNTS.phaseCounts.cooling} cooling`)]), h('div', { class: 'chip good' }, [h('b', {}, plural(phaseChanges, 'phase change')), h('small', {}, 'the rest are provisional → on-time pairs')])];
}

function showDetail(v, rec) {
  const next = (rec ? COUNTS.versions : machines(signal()).versions.length) + 1;
  detail.replaceChildren(...(rec
    ? [h('b', {}, `Version ${v.v} · ${v.rec.phase} · severity ${v.rec.severity}`), h('small', {}, `${v.rec.completeness.replace('_', '-')} · mean ${fmt(v.rec.tempMean, 2)} °C · slope ${fmt(v.rec.tempSlope)} °C/h · ${v.rec.evidenceCount} readings behind it`)]
    : [h('b', {}, `Version ${v.v} · ${v.phase} at ${v.t} s`), h('small', {}, `temperature ${fmt(v.mean, 1)} °C when it published`)]),
  h('button', { class: 'btn', style: 'margin-top:8px', onclick: () => { detail.querySelector('.refuse').textContent = `Refused. Published versions never change. A correction would publish version ${next}.`; } }, 'Try to edit it'),
  h('small', { class: 'refuse', style: 'color:#ff8aa0', role: 'status' }, ''));
}

clock = new Clock({ duration: DUR, speed: 1, onFrame: (t) => { render(t); sync(); } });
const sync = transport($('#transport'), clock, { label: 'Play' });
const ctl = $('#ctl');
const redraw = () => render(clock.t);
control(ctl, { id: 'noise', label: 'Noise', min: 0, max: 3, step: 0.1, value: state.noise, unit: ' °C', format: (v) => v.toFixed(1), onInput: (v) => { state.noise = v; redraw(); } });
control(ctl, { id: 'exit', label: 'Exit threshold', min: 24, max: 30, step: 0.5, value: state.exit, unit: ' °C', format: (v) => v.toFixed(1), onInput: (v) => { state.exit = v; redraw(); } });
control(ctl, { id: 'mind', label: 'Hold to enter', min: 0, max: 120, step: 5, value: state.minDur, unit: ' s', onInput: (v) => { state.minDur = v; redraw(); } });
control(ctl, { id: 'rec', label: 'Hold to leave', min: 0, max: 180, step: 10, value: state.recHold, unit: ' s', onInput: (v) => { state.recHold = v; redraw(); } });
ctl.append(h('button', { class: 'btn', onclick: () => { state.seed = Math.floor(Math.random() * 1e6); redraw(); } }, 'Re-roll noise'));
const modeBtn = h('button', { class: 'btn', 'aria-pressed': 'false', onclick: () => {
  const recorded = state.mode !== 'recorded';
  state.mode = recorded ? 'recorded' : 'synthetic';
  modeBtn.textContent = recorded ? 'Back to the synthetic signal' : 'Show the recorded run';
  modeBtn.setAttribute('aria-pressed', String(recorded));
  state.picked = null;
  detail.textContent = PICK;
  redraw();
} }, 'Show the recorded run');
ctl.append(modeBtn);
clock.seek(DUR);
