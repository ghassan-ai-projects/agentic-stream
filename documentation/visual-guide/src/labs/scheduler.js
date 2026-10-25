import { RUN, COUNTS } from '../story.js';
import { $, s, h, clamp, fmt, Clock, transport, control, select, toggle } from './kit.js';

const W = 1080, H = 530;
const state = { pace: 'recorded', threshold: 35, debounce: 5, cooldown: 20, material: true };
const SLIDE = { x: 60, w: 960 };
const COL = { condition: '#4a5a86', nothing: '#6b86c9', queued: '#f5b94a', episode: '#34d399', replaced: '#ff7a59' };

const versions = RUN.triggers.map((t) => ({
  v: t.v, score: t.score, wall: t.wallMs / 1000, live: RUN.versions[t.v - 1].t + (RUN.versions[t.v - 1].completeness === 'on_time' ? 0.3 : 0),
  material: t.outcome === 'admitted', gateOpen: t.reason !== 'trigger condition false',
}));

function simulate() {
  const when = (x) => (state.pace === 'recorded' ? x.wall : x.live);
  const rows = [];
  const items = [];
  let pending = null;
  let lastAdmit = -Infinity;
  const episodes = [];
  const release = (upTo) => {
    if (pending && pending.notBefore <= upTo) { pending.status = 'admitted'; pending.endsAt = pending.notBefore; episodes.push({ v: pending.v, at: pending.notBefore }); pending = null; }
  };
  for (const x of versions) {
    const p = when(x);
    release(p);
    let outcome;
    if (!x.gateOpen) outcome = 'condition';
    else if (x.score < state.threshold) outcome = 'condition';
    else if (state.material && !x.material) outcome = 'nothing';
    else {
      outcome = 'queued';
      const notBefore = Math.max(p + state.debounce, lastAdmit + state.cooldown);
      if (pending) { pending.status = 'coalesced'; pending.endsAt = p; pending.by = x.v; }
      pending = { v: x.v, created: p, notBefore, status: 'pending', endsAt: null };
      items.push(pending);
      lastAdmit = p;
    }
    rows.push({ ...x, p, outcome });
  }
  release(Infinity);
  return { rows, items, episodes };
}

const svg = s('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Evaluations of 22 versions, queued work and episodes started' });
$('#stage').append(svg);
const chips = $('#chips');

function render(f) {
  const sim = simulate();
  const tMax = Math.max(state.pace === 'recorded' ? 25 : 380, ...sim.items.map((i) => i.notBefore + 3));
  const T = f * tMax;
  const X = (t) => SLIDE.x + (t / tMax) * SLIDE.w;
  svg.replaceChildren();
  const add = (n) => svg.append(n);
  add(s('text', { x: 60, y: 28, class: 'big' }, 'Every new version is scored'));
  add(s('text', { x: 60, y: 46, class: 'small mute' }, `bar height is the score; the line is the threshold (${state.threshold}). Columns are versions 1 to 22.`));
  const colX = (i) => SLIDE.x + 20 + (i / 21) * (SLIDE.w - 40);
  const scoreY = (sc) => 190 - (sc / 70) * 110;
  add(s('line', { x1: 60, x2: 1020, y1: 190, y2: 190, stroke: '#2a3a63' }));
  add(s('line', { x1: 60, x2: 1020, y1: scoreY(state.threshold), y2: scoreY(state.threshold), stroke: '#f5b94a', 'stroke-dasharray': '6 5' }));
  sim.rows.forEach((r, i) => {
    const done = T >= r.p;
    const col = r.outcome === 'queued' ? COL.queued : r.outcome === 'nothing' ? COL.nothing : COL.condition;
    const hgt = Math.max(3, 190 - scoreY(r.score));
    add(s('rect', { x: colX(i) - 10, y: 190 - hgt, width: 20, height: hgt, rx: 4, fill: done ? col : 'transparent', stroke: col, 'stroke-width': 1.5, opacity: done ? 1 : 0.35 }));
    add(s('text', { x: colX(i), y: 210, class: 'small mute', 'text-anchor': 'middle' }, r.v));
  });
  const legend = [['condition', 'trigger not met or below threshold'], ['nothing', 'nothing materially new'], ['queued', 'queued for an episode']];
  legend.forEach(([k, t], i) => { add(s('rect', { x: 60 + i * 300, y: 224, width: 12, height: 12, rx: 3, fill: COL[k] })); add(s('text', { x: 78 + i * 300, y: 235, class: 'small mute' }, t)); });

  add(s('text', { x: 60, y: 282, class: 'big' }, 'Queued work waits'));
  add(s('text', { x: 60, y: 300, class: 'small mute' }, `until its not-before time: debounce ${state.debounce} s, and cooldown ${state.cooldown} s after the previous admission. Newer work for the same trigger replaces older work.`));
  const axisY = 470;
  add(s('line', { x1: SLIDE.x, x2: SLIDE.x + SLIDE.w, y1: axisY, y2: axisY, stroke: '#3a4c80', 'stroke-width': 2 }));
  const step = tMax > 100 ? 60 : 5;
  for (let t = 0; t <= tMax; t += step) { add(s('line', { x1: X(t), x2: X(t), y1: axisY - 4, y2: axisY + 4, stroke: '#3a4c80' })); add(s('text', { x: X(t), y: axisY + 20, class: 'small mute', 'text-anchor': 'middle' }, `${t} s`)); }
  add(s('text', { x: SLIDE.x + SLIDE.w, y: axisY + 40, class: 'small mute', 'text-anchor': 'end' }, state.pace === 'recorded' ? 'wall-clock seconds in the recorded run (versions arrived within 1.6 s)' : 'wall-clock seconds for a live sensor (a slide every 30 s)'));
  sim.items.forEach((it, k) => {
    const y = 345 + k * 44;
    if (y > 440) return;
    const end = it.status === 'pending' ? it.notBefore : it.endsAt;
    const x0 = X(it.created), x1 = X(Math.min(Math.max(it.created, Math.min(T, end)), end));
    const col = it.status === 'coalesced' ? COL.replaced : COL.queued;
    add(s('rect', { x: x0, y, width: Math.max(10, X(end) - x0), height: 18, rx: 5, fill: 'transparent', stroke: col, 'stroke-dasharray': '4 4', opacity: 0.6 }));
    if (T >= it.created) add(s('rect', { x: x0, y, width: Math.max(10, x1 - x0), height: 18, rx: 5, fill: col, opacity: 0.85 }));
    const label = it.status === 'coalesced' ? `v${it.v} was replaced by v${it.by} before its not-before time` : `v${it.v} waits, then starts an episode at ${fmt(it.notBefore, 1)} s`;
    if (T >= it.created) add(s('text', { x: Math.max(x0, 66), y: y - 6, class: 'mid', style: `fill:${col}` }, T >= end ? label : `v${it.v} waiting`));
    if (it.status === 'coalesced' && T >= it.endsAt) add(s('text', { x: X(it.endsAt) + 6, y: y + 14, class: 'mid', style: `fill:${col}` }, '✕'));
    if (it.status === 'admitted' && T >= it.notBefore) add(s('path', { d: `M ${X(it.notBefore)} ${y - 4} l 9 13 l -9 13 z`, fill: COL.episode }));
  });
  add(s('line', { x1: X(T), x2: X(T), y1: 318, y2: axisY + 6, stroke: '#fff', 'stroke-width': 2, opacity: 0.8 }));
  if (state.pace === 'recorded' && state.threshold === 35 && state.debounce === 5 && state.cooldown === 20 && state.material) {
    const rx = X(RUN.episode.acceptedMs / 1000);
    add(s('line', { x1: rx, x2: rx, y1: 322, y2: axisY, stroke: '#34d399', 'stroke-dasharray': '3 4', opacity: 0.8 }));
    add(s('text', { x: rx + 6, y: 456, class: 'small', style: 'fill:#34d399' }, `recorded: accepted at ${fmt(RUN.episode.acceptedMs / 1000, 1)} s`));
  }

  const shown = sim.rows.filter((r) => T >= r.p);
  const ignored = shown.filter((r) => r.outcome !== 'queued').length;
  const queued = shown.filter((r) => r.outcome === 'queued').length;
  const replaced = sim.items.filter((i) => i.status === 'coalesced' && T >= i.endsAt).length;
  const eps = sim.episodes.filter((e) => T >= e.at).length;
  const recorded = state.pace === 'recorded' && state.threshold === 35 && state.debounce === 5 && state.cooldown === 20 && state.material;
  const match = recorded && sim.episodes.length === 1 && sim.episodes[0].v === 7 && sim.items[0].status === 'coalesced';
  chips.replaceChildren(
    h('div', { class: 'chip' }, [h('b', {}, `${shown.length} of ${COUNTS.versions}`), h('small', {}, 'versions evaluated')]),
    h('div', { class: 'chip' }, [h('b', {}, `${ignored}`), h('small', {}, 'ignored (each with a recorded reason)')]),
    h('div', { class: 'chip warn' }, [h('b', {}, `${queued} queued · ${replaced} replaced`), h('small', {}, 'before any model runs')]),
    h('div', { class: 'chip good' }, [h('b', {}, `${eps} of ${sim.episodes.length} episodes started`), h('small', {}, 'final count for these settings')]),
    h('div', { class: `chip ${recorded ? (match ? 'good' : 'bad') : 'info'}` }, [h('b', {}, recorded ? (match ? 'matches the recorded run' : 'does NOT match the recorded run') : 'what-if'), h('small', {}, recorded ? 'recorded: 22 evaluations, v5 replaced by v7, 1 episode' : 'settings differ from the recorded run')]),
  );
}

const clock = new Clock({ duration: 18, speed: 1, onFrame: (t) => { render(t / 18); sync(); } });
const sync = transport($('#transport'), clock, { label: 'Replay' });
const ctl = $('#ctl');
select(ctl, { id: 'pace', label: 'Sensor pace', options: [['recorded', 'recorded replay (fast-forward)'], ['live', 'live sensor (30 s slides)']], value: 'recorded', onInput: (v) => { state.pace = v; render(clock.t / 18); } });
control(ctl, { id: 'thr', label: 'Threshold', min: 0, max: 70, step: 1, value: 35, onInput: (v) => { state.threshold = v; render(clock.t / 18); } });
control(ctl, { id: 'deb', label: 'Debounce', min: 0, max: 30, step: 1, value: 5, unit: ' s', onInput: (v) => { state.debounce = v; render(clock.t / 18); } });
control(ctl, { id: 'cool', label: 'Cooldown', min: 0, max: 60, step: 1, value: 20, unit: ' s', onInput: (v) => { state.cooldown = v; render(clock.t / 18); } });
toggle(ctl, { id: 'mat', label: 'Require a material change', value: true, onInput: (v) => { state.material = v; render(clock.t / 18); } });
clock.seek(18);
