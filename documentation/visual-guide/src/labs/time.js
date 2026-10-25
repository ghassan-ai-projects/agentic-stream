import { RUN } from '../story.js';
import { $, s, h, clamp, lerp, ease, fmt, Clock, transport, control, select, toggle } from './kit.js';

const temps = RUN.events.series.filter((e) => e.type === 'temp').map((e) => ({ e: e.t, v: e.value }));
const WM_LAG = 15;
const WIN_END = 300;
const LATENESS = 30;
const MAXP = 420;
const X = (t) => 60 + (t / MAXP) * 960;
const Y_ARR = 130;
const Y_EVT = 330;
const state = { sel: 4, delay: 210, policy: 'correct_and_reconsider', spike: true };

const jitter = (i) => 1.2 + ((i * 37) % 11) / 8;

function readings() {
  return temps.map((r, i) => ({
    i, e: r.e, v: i === state.sel && state.spike ? r.v + 6 : r.v,
    a: r.e + jitter(i) + (i === state.sel ? state.delay : 0), delayed: i === state.sel,
  }));
}

function analyse() {
  const rs = readings();
  const byArrival = [...rs].sort((x, y) => x.a - y.a);
  let maxE = -Infinity, closedAt = Infinity;
  const wmAt = new Map();
  for (const r of byArrival) {
    maxE = Math.max(maxE, r.e);
    wmAt.set(r.i, maxE - WM_LAG);
    if (closedAt === Infinity && maxE - WM_LAG >= WIN_END) closedAt = r.a;
  }
  const inWin = rs.filter((r) => r.e < WIN_END);
  const onTime = inWin.filter((r) => r.a <= closedAt);
  const late = inWin.filter((r) => r.a > closedAt).map((r) => ({ ...r, within: wmAt.get(r.i) <= WIN_END + LATENESS }));
  const mean = (a) => (a.length ? a.reduce((n, r) => n + r.v, 0) / a.length : NaN);
  const keepsLate = (l) => l.within && (state.policy === 'correct' || state.policy === 'correct_and_reconsider');
  const final = [...onTime, ...late.filter(keepsLate)];
  return {
    rs, closedAt, onTime, late, wmAt,
    emitted: mean(onTime), final: mean(final), truth: mean(inWin), naive: mean(rs.filter((r) => r.a < WIN_END && r.e < WIN_END)),
    outcome: late.length === 0 ? 'on_time' : late.every((l) => !l.within) ? 'too late' : keepsLate(late[0]) ? 'corrected' : state.policy === 'keep' ? 'kept in history' : 'dropped and audited',
  };
}

const svg = s('svg', { viewBox: '0 0 1080 470', role: 'img', 'aria-label': 'Readings arriving out of order on the runtime clock, placed by their event time' });
$('#stage').append(svg);
const chips = $('#chips');

function render(p) {
  const A = analyse();
  const wm = (() => { let m = -Infinity; for (const r of A.rs) if (r.a <= p) m = Math.max(m, r.e); return m === -Infinity ? null : m - WM_LAG; })();
  const closed = p >= A.closedAt;
  svg.replaceChildren();
  const add = (n) => svg.append(n);
  add(s('text', { x: 60, y: 30, class: 'big' }, 'What the runtime sees: arrival order'));
  add(s('text', { x: 60, y: 50, class: 'small mute' }, 'its own clock, in seconds since the run started'));
  add(s('text', { x: 60, y: 214, class: 'big' }, 'What actually happened: event time'));
  add(s('text', { x: 60, y: 234, class: 'small mute' }, 'when the sensor measured it. Windows, the watermark and results live here.'));
  for (let t = 0; t <= MAXP; t += 60) {
    add(s('line', { x1: X(t), x2: X(t), y1: Y_ARR - 26, y2: Y_ARR + 26, stroke: '#2a3a63', 'stroke-width': 1 }));
    add(s('text', { x: X(t), y: Y_ARR + 44, class: 'small mute', 'text-anchor': 'middle' }, t));
    add(s('line', { x1: X(t), x2: X(t), y1: Y_EVT - 26, y2: Y_EVT + 26, stroke: '#2a3a63', 'stroke-width': 1 }));
    add(s('text', { x: X(t), y: Y_EVT + 44, class: 'small mute', 'text-anchor': 'middle' }, t));
  }
  add(s('line', { x1: X(0), x2: X(MAXP), y1: Y_ARR, y2: Y_ARR, stroke: '#3a4c80', 'stroke-width': 2 }));
  add(s('line', { x1: X(0), x2: X(MAXP), y1: Y_EVT, y2: Y_EVT, stroke: '#3a4c80', 'stroke-width': 2 }));

  const winColor = closed ? '#9b8cff' : '#6b5fd0';
  add(s('rect', { x: X(0), y: Y_EVT - 62, width: X(WIN_END) - X(0), height: 124, rx: 10, fill: closed ? 'rgba(155,140,255,.16)' : 'rgba(155,140,255,.07)', stroke: winColor, 'stroke-width': closed ? 2.4 : 1.4, 'stroke-dasharray': closed ? '0' : '6 5' }));
  add(s('text', { x: X(0) + 6, y: Y_EVT + 84, class: 'mid', 'text-anchor': 'start', style: 'fill:#c9c1ff' }, closed ? `window [0, 300 s) closed at ${fmt(A.closedAt, 0)} s` : 'window [0, 300 s) is open'));
  add(s('rect', { x: X(WIN_END), y: Y_EVT - 62, width: X(WIN_END + LATENESS) - X(WIN_END), height: 124, fill: 'rgba(245,185,74,.10)', stroke: '#f5b94a', 'stroke-dasharray': '4 4' }));
  add(s('text', { x: X(WIN_END + LATENESS / 2), y: Y_EVT + 84, class: 'small', 'text-anchor': 'middle', fill: '#f5b94a' }, 'allowed lateness'));

  if (wm != null) {
    add(s('line', { x1: X(wm), x2: X(wm), y1: Y_EVT - 78, y2: Y_EVT + 70, stroke: '#ffd166', 'stroke-width': 2.4, 'stroke-dasharray': '7 5' }));
    add(s('text', { x: X(wm), y: Y_EVT - 84, class: 'mid', 'text-anchor': 'middle', fill: '#ffd166' }, `watermark ${fmt(Math.max(wm, 0), 0)} s`));
  }
  add(s('line', { x1: X(p), x2: X(p), y1: Y_ARR - 40, y2: Y_ARR + 30, stroke: '#fff', 'stroke-width': 2.4 }));
  add(s('text', { x: X(p), y: Y_ARR - 46, class: 'small', 'text-anchor': 'middle' }, `now ${fmt(p, 0)} s`));

  for (const r of A.rs) {
    const known = r.a <= p;
    const col = r.delayed ? (state.spike ? '#ff5470' : '#4cc9f0') : '#ff7a59';
    add(s('circle', { cx: X(r.e), cy: Y_EVT, r: 11, fill: known ? col : 'transparent', stroke: col, 'stroke-width': 2, opacity: known ? 1 : 0.6 }));
    if (known) {
      const u = ease((p - r.a) / 1.2);
      const path = `M ${X(r.a)} ${Y_ARR + 11} C ${X(r.a)} ${Y_ARR + 80}, ${X(r.e)} ${Y_EVT - 90}, ${X(r.e)} ${Y_EVT - 12}`;
      add(s('path', { d: path, fill: 'none', stroke: col, 'stroke-width': r.delayed ? 2 : 1.2, opacity: 0.18 + 0.4 * (1 - u) + (r.delayed ? 0.3 : 0) }));
      add(s('circle', { cx: X(r.a), cy: Y_ARR, r: 11, fill: col }));
      add(s('text', { x: X(r.a), y: Y_ARR + 4, class: 'small', 'text-anchor': 'middle', fill: '#10131c', style: 'fill:#10131c;font-weight:700' }, r.i + 1));
    }
    add(s('text', { x: X(r.e), y: Y_EVT + 4, class: 'small', 'text-anchor': 'middle', style: `fill:${known ? '#10131c' : '#c9d6ee'};font-weight:700` }, r.i + 1));
    if (r.delayed) add(s('text', { x: X(r.e), y: Y_EVT + 28, class: 'small', 'text-anchor': 'middle', fill: '#ff8aa0', style: 'fill:#ff8aa0' }, `reading ${r.i + 1}: sent at ${r.e} s`));
  }
  const dr = A.rs[state.sel];
  if (p >= dr.a && dr.a > A.closedAt) {
    const l = A.late[0];
    const tone = l.within ? '#f5b94a' : '#ff5470';
    add(s('rect', { x: X(dr.a) - 150, y: Y_ARR - 110, width: 300, height: 46, rx: 10, fill: 'rgba(8,13,28,.95)', stroke: tone }));
    add(s('text', { x: X(dr.a), y: Y_ARR - 91, class: 'mid', 'text-anchor': 'middle', style: `fill:${tone};font-weight:700` }, `reading ${dr.i + 1} arrived after the window closed`));
    add(s('text', { x: X(dr.a), y: Y_ARR - 73, class: 'small', 'text-anchor': 'middle' }, l.within ? 'inside the allowed lateness' : 'beyond the allowed lateness'));
  }

  const pol = { drop: 'audit and drop', keep: 'keep in history', correct: 'publish a correction', correct_and_reconsider: 'correct and reconsider' }[state.policy];
  const cls = A.outcome === 'on_time' ? 'good' : A.outcome === 'corrected' ? 'info' : 'warn';
  const afterClose = p >= A.closedAt;
  const arrivedLate = A.late.length && p >= A.late[0].a;
  chips.replaceChildren(
    h('div', { class: 'chip info' }, [h('b', {}, wm == null ? '–' : `${fmt(Math.max(wm, 0), 0)} s`), h('small', {}, 'watermark: newest event time minus 15 s')]),
    h('div', { class: 'chip good' }, [h('b', {}, `${fmt(A.truth, 2)} °C`), h('small', {}, 'mean of window [0, 300 s) by event time, all readings')]),
    h('div', { class: 'chip bad' }, [h('b', {}, `${fmt(A.naive, 2)} °C`), h('small', {}, `same window grouped by arrival time (wrong by ${fmt(Math.abs(A.naive - A.truth), 2)})`)]),
    h('div', { class: `chip ${afterClose ? cls : ''}` }, [h('b', {}, !afterClose ? 'window open' : arrivedLate ? `${A.outcome}` : `on_time · emitted ${fmt(A.emitted, 2)} °C`), h('small', {}, `policy: ${pol}${arrivedLate && A.outcome === 'corrected' ? ` · result now ${fmt(A.final, 2)} °C` : ''}`)]),
  );
}

const clock = new Clock({ duration: MAXP, speed: 1, onFrame: (t) => { render(t); sync(); } });
clock.duration = MAXP;
const tr = $('#transport');
const sync = transport(tr, clock, { label: 'Play' });
const ctl = $('#ctl');
select(ctl, { id: 'sel', label: 'Delay reading', options: temps.map((r, i) => [i, `#${i + 1} (sent at ${r.e} s)`]), value: 4, onInput: (v) => { state.sel = Number(v); render(clock.t); } });
control(ctl, { id: 'delay', label: 'Network delay', min: 0, max: 300, step: 5, value: 210, unit: ' s', onInput: (v) => { state.delay = v; render(clock.t); } });
select(ctl, { id: 'policy', label: 'Late policy', options: [['drop', 'audit and drop'], ['keep', 'keep in history'], ['correct', 'publish a correction'], ['correct_and_reconsider', 'correct and reconsider']], value: state.policy, onInput: (v) => { state.policy = v; render(clock.t); } });
toggle(ctl, { id: 'spike', label: 'What-if: the delayed reading is a +6 °C spike', value: true, onInput: (v) => { state.spike = v; render(clock.t); } });
clock.seek(262);
