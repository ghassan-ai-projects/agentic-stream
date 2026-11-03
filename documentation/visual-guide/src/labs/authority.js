import { RUN } from '../story.js';
import { lerp, easeInOut } from '../util.js';
import { $, s, h, wrap, Clock, transport, select, toggle } from './kit.js';

const W = 1080, H = 500;
const INTENTS = {
  indicator: { id: 'set_indicator', args: '{ state: "alert" }', risk: 'R1', recorded: true, readback: true },
  fan: { id: 'select_thermal_mode', args: '{ mode: "bounded_cooling" }', risk: 'R2', recorded: false, readback: true },
  watch: { id: 'install_watch_condition', args: '{ metric: "condition_score" }', risk: 'R0', recorded: false, readback: true },
  ticket: { id: 'create_maintenance_ticket', args: '{ hypothesis: "bearing_wear" }', risk: 'R1', recorded: false, readback: false },
  valve: { id: 'open_valve', args: '{ position: 100 }', risk: 'R3?', recorded: false },
};
const state = { intent: 'indicator', stale: false, stop: false, epoch: false, approval: 'granted', ceiling: 'R2', lost: false, refused: false, noKey: false };
const GATES = [
  { name: 'Validator', sub: 'decision is well formed', color: '#34d399' },
  { name: 'Policy', sub: 'allowed right now?', color: '#34d399' },
  { name: 'Outbox and lease', sub: 'record before sending', color: '#fb7185' },
  { name: 'Dispatch to device', sub: 'gateway and board', color: '#fb7185' },
  { name: 'Outcome', sub: 'prove what happened', color: '#ffd166' },
];
const GX = (i) => 52 + i * 204;
const GT = [1.2, 3.2, 5.0, 6.8, 9.0];

function run() {
  const I = INTENTS[state.intent];
  const steps = [];
  const rank = { R0: 0, R1: 1, R2: 2 };
  const stop = (gate, why, record, tone = 'bad') => { steps.push({ gate, ok: false, why, record, tone }); return { final: { tone, title: why, text: record.text } }; };
  if (I.id === 'open_valve') return { steps, ...stop(0, 'Refused: not in the catalog', { table: 'decisions', text: 'rejected: intent type is not in the spec catalog' }) };
  if (rank[I.risk] > rank[state.ceiling]) return { steps, ...stop(0, `Refused: ${I.risk} is above the ${state.ceiling} ceiling`, { table: 'decisions', text: `rejected: risk ${I.risk} exceeds the episode ceiling ${state.ceiling}` }) };
  steps.push({ gate: 0, ok: true, why: 'catalog, schema, writable fields, evidence', record: { table: 'decisions', text: 'accepted, bound to the snapshot digest' } });
  if (state.stale) return { steps, ...stop(1, 'Stale: superseded', { table: 'policy_evaluations', text: 'stale: situation_version_stale, a material change superseded this intent' }) };
  if (state.epoch) return { steps, ...stop(1, 'Fenced: owner changed', { table: 'policy_evaluations', text: 'no record: the old owner\'s write is fenced out' }) };
  if (state.stop) return { steps, ...stop(1, 'Denied: operator stop', { table: 'policy_evaluations', text: 'denied: interlock_not_ready' }) };
  if (I.risk === 'R2') {
    if (state.approval !== 'granted') return { steps, ...stop(1, state.approval === 'pending' ? 'Waiting for a human' : 'Denied: approval not granted', { table: 'approvals', text: state.approval === 'pending' ? 'requested: waiting for a signed approval' : `${state.approval}: no command is created` }, state.approval === 'pending' ? 'warn' : 'bad') };
    steps.push({ gate: 1, ok: true, why: 'R2: signed human approval, then re-checked', record: { table: 'approvals', text: 'approved by a human' } });
  } else steps.push({ gate: 1, ok: true, why: `${I.risk}: automatic_r0_r1`, record: { table: 'policy_evaluations', text: `approved, reason ${I.recorded ? RUN.policy.reason : 'automatic_r0_r1'}` } });
  steps.push({ gate: 2, ok: true, why: 'idempotency key and outbox row', record: { table: 'commands + outbox', text: `prepared, ${I.recorded ? `key ${RUN.command.idempotencyKey}…` : 'with a stable idempotency key'}, written before sending` } });
  if (state.refused) return refusedOutcome(steps);
  if (state.lost) return lostOutcome(steps, I);
  steps.push({ gate: 3, ok: true, why: 'receipt accepted · result executed', record: { table: 'outcomes', text: I.recorded ? `succeeded, "${RUN.device.detail}"` : 'succeeded' } });
  if (!I.readback) return { steps: [...steps, noReadback(false)], final: manualReview(false) };
  steps.push({ gate: 4, ok: true, why: 'the state query agrees', record: { table: 'verifications', text: `observed: ${observed(I)}` } });
  return { steps, final: { tone: 'good', title: 'Executed and verified', text: `query_state: ${observed(I)}. Verification: ${I.recorded ? RUN.device.reconciliation : 'verified'}.` } };
}

const observed = (I) => (I.recorded ? `${RUN.device.observed.target} energized, value ${RUN.device.observed.value}` : 'the output matches what was commanded');

function refusedOutcome(steps) {
  steps.push({ gate: 3, ok: false, tone: 'bad', why: 'refused: no effect happened', record: { table: 'outcomes', text: 'failed: refused by the node, the reason is in the provider result' } });
  steps.push({ gate: 4, ok: false, tone: 'bad', why: 'refuted: nothing to verify', record: { table: 'verifications', text: 'refuted: a refused or failed command has a definite outcome and no effect to verify' } });
  return { steps, final: { tone: 'bad', title: 'Refused, verification refuted', text: 'The device said no, so the outcome is known and nothing is retried blindly. The verification closes as refuted instead of waiting forever, so the run does not count it as unresolved.' } };
}

function lostOutcome(steps, I) {
  steps.push({ gate: 3, ok: true, why: 'sent; the reply never came back', record: { table: 'outcomes', text: 'unknown: no result received' }, lost: true });
  if (state.noKey) {
    steps.push({ gate: 4, ok: false, why: 'a second effect happened', record: { table: 'outcomes', text: 'second request sent without the first key' }, tone: 'bad' });
    return { steps, final: { tone: 'bad', title: 'Duplicate effect', text: 'Without a stable key, a blind retry is a second request. For effects like opening a ticket that means two.' }, dup: true };
  }
  if (!I.readback) return { steps: [...steps, noReadback(true)], final: manualReview(true), reconciled: true };
  steps.push({ gate: 4, ok: true, why: 'reconciled with query_state', record: { table: 'verifications', text: `observed after reconciliation: ${observed(I)}` } });
  return { steps, final: { tone: 'good', title: 'Unknown, then reconciled', text: 'The system does not retry blindly. It waits for reconciliation, and query_state shows the output is on: verified without a second effect.' }, reconciled: true };
}

const noReadback = (unknown) => ({ gate: 4, ok: true, tone: 'warn', why: 'manual review: nothing to read back', record: { table: 'outcomes', text: `manual_review: ${unknown ? 'the outcome is unknown' : 'it succeeded'} and no reading of the device could confirm it; the empty read-back is sealed as evidence` } });

const manualReview = (unknown) => ({ tone: 'warn', title: 'Manual review', text: `${unknown ? 'The reply was lost' : 'The device reported success'}, but this command changes nothing the host can read back, so no later reading could settle it. It concludes as manual review for an operator at once, and does not hold up later commands to the device.` });

const svg = s('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'A proposed action moving through validator, policy, outbox, dispatch and outcome' });
$('#stage').append(svg);
const chips = $('#chips');
const TONE = { good: '#34d399', bad: '#ff5470', warn: '#f5b94a' };

function render(t) {
  const R = run();
  const I = INTENTS[state.intent];
  svg.replaceChildren();
  const add = (n) => svg.append(n);
  add(s('text', { x: 52, y: 30, class: 'big' }, `Proposal: ${I.id} ${I.args}`));
  add(s('text', { x: 52, y: 50, class: 'small mute' }, `risk class ${I.risk}${I.recorded ? ' · this is the intent in the recorded run' : ' · what-if, not in the recorded run'}`));
  const stopped = R.steps.find((x) => !x.ok);
  const last = stopped ? stopped.gate : 4;
  GATES.forEach((g, i) => {
    const reached = t >= GT[i] - 0.6;
    const step = R.steps.find((x) => x.gate === i);
    const done = step && t >= GT[i];
    const bad = step && !step.ok && done;
    const col = done ? (step.tone ? TONE[step.tone] : '#34d399') : '#3a4c80';
    add(s('rect', { x: GX(i), y: 100, width: 176, height: 130, rx: 16, fill: 'rgba(11,18,38,.92)', stroke: col, 'stroke-width': done ? 2.6 : 1.4, opacity: i > last ? 0.35 : 1 }));
    add(s('text', { x: GX(i) + 88, y: 132, class: 'big', 'text-anchor': 'middle' }, g.name));
    add(s('text', { x: GX(i) + 88, y: 152, class: 'small mute', 'text-anchor': 'middle' }, g.sub));
    if (done) {
      add(s('circle', { cx: GX(i) + 88, cy: 188, r: 15, fill: col }));
      add(s('text', { x: GX(i) + 88, y: 194, class: 'big', 'text-anchor': 'middle', style: 'fill:#06101f' }, bad ? '✕' : step.tone === 'warn' ? '!' : '✓'));
      const lines = wrap(step.why, 24);
      lines.forEach((l, k) => add(s('text', { x: GX(i) + 88, y: 252 + k * 15, class: 'small', 'text-anchor': 'middle', style: `fill:${bad ? col : '#cdd8ec'}` }, l)));
    }
    if (i < 4) add(s('path', { d: `M ${GX(i) + 180} 165 h 20`, stroke: '#3a4c80', 'stroke-width': 2, fill: 'none', 'marker-end': 'url(#arr)' }));
  });
  const defs = s('defs', {}, [s('marker', { id: 'arr', viewBox: '0 0 10 10', refX: 8, refY: 5, markerWidth: 6, markerHeight: 6, orient: 'auto' }, [s('path', { d: 'M0 1 L9 5 L0 9', fill: 'none', stroke: '#3a4c80', 'stroke-width': 1.6 })])]);
  svg.prepend(defs);

  const tokenX = (() => {
    let x = GX(0) - 40;
    for (const k of [0, 1, 2, 3, 4]) { if (t >= GT[k] - 0.8) x = GX(k) + 88; }
    const seg = GT.findIndex((g) => t < g - 0.8);
    if (seg > 0) { const a = GX(seg - 1) + 88, b = GX(seg) + 88; const u = easeInOut((t - (GT[seg - 1] - 0.8)) / (GT[seg] - GT[seg - 1])); x = lerp(a, b, u); }
    else if (seg === 0) { x = lerp(GX(0) - 40, GX(0) + 88, easeInOut(t / (GT[0] - 0.8))); }
    return Math.min(x, GX(last) + 88);
  })();
  const ended = t >= GT[stopped ? stopped.gate : 4] + 0.3;
  add(s('circle', { cx: tokenX, cy: 82, r: 11, fill: ended && stopped ? TONE[stopped.tone] : '#ffd166' }));
  add(s('circle', { cx: tokenX, cy: 82, r: 20, fill: 'none', stroke: '#ffd16655', 'stroke-width': 2 }));

  if (R.steps.find((x) => x.lost) && t >= GT[3] + 0.4) {
    add(s('text', { x: GX(3) + 88, y: 290, class: 'mid', 'text-anchor': 'middle', style: 'fill:#ff8aa0' }, '✉ reply lost'));
  }
  const finalT = R.final ? (stopped ? GT[stopped.gate] + 0.6 : GT[4]) : GT[4];
  if (R.final && t >= finalT) {
    const f = R.final;
    add(s('rect', { x: 52, y: 340, width: 976, height: 110, rx: 16, fill: 'rgba(11,18,38,.95)', stroke: TONE[f.tone], 'stroke-width': 2.4 }));
    add(s('text', { x: 78, y: 378, class: 'big', style: `fill:${TONE[f.tone]};font-size:22px` }, f.title));
    wrap(f.text, 110).forEach((l, k) => add(s('text', { x: 78, y: 406 + k * 20, class: 'mid' }, l)));
    if (!stopped && !R.dup && I.readback) {
      ['receipt: accepted', `result: ${RUN.device.result}`, `query_state: ${RUN.device.observed.target} energized`].forEach((x, k) => { const on = t >= GT[4] + 0.4 + k * 0.7 || R.reconciled; add(s('text', { x: 640 + (k === 2 ? 0 : 0), y: 378 + k * 22, class: 'mid', style: `fill:${k === 2 ? '#ffd166' : '#cdd8ec'};opacity:${on ? 1 : 0.25}` }, `${k + 1}  ${x}`)); });
    }
  }
  chips.replaceChildren(...R.steps.filter((x) => t >= GT[x.gate]).map((x) => h('div', { class: `chip ${x.lost ? 'warn' : x.tone ?? 'good'}` }, [h('b', {}, x.record.table), h('small', {}, x.record.text)])));
}

const clock = new Clock({ duration: 11, speed: 1, onFrame: (t) => { render(t); sync(); } });
const sync = transport($('#transport'), clock, { label: 'Send it' });
const ctl = $('#ctl');
const restart = () => { clock.seek(0); clock.play(); };
select(ctl, { id: 'intent', label: 'Proposal', options: [['indicator', 'set_indicator · R1 (recorded run)'], ['fan', 'select_thermal_mode · R2 (the fan)'], ['watch', 'install_watch_condition · R0'], ['ticket', 'create_maintenance_ticket · R1 (nothing to read back)'], ['valve', 'open_valve (not in the catalog)']], value: 'indicator', onInput: (v) => { state.intent = v; restart(); } });
select(ctl, { id: 'ceil', label: 'Episode risk ceiling', options: [['R1', 'R1'], ['R2', 'R2']], value: 'R2', onInput: (v) => { state.ceiling = v; restart(); } });
select(ctl, { id: 'appr', label: 'Human approval (R2)', options: [['granted', 'granted'], ['pending', 'not yet'], ['denied', 'denied'], ['expired', 'expired']], value: 'granted', onInput: (v) => { state.approval = v; restart(); } });
toggle(ctl, { id: 'stale', label: 'A material change superseded it', onInput: (v) => { state.stale = v; restart(); } });
toggle(ctl, { id: 'stop', label: 'Operator stop engaged', onInput: (v) => { state.stop = v; restart(); } });
toggle(ctl, { id: 'epoch', label: 'Another runtime took ownership', onInput: (v) => { state.epoch = v; restart(); } });
toggle(ctl, { id: 'lost', label: 'The device reply is lost', onInput: (v) => { state.lost = v; restart(); } });
toggle(ctl, { id: 'refused', label: 'The device refuses or fails the command', onInput: (v) => { state.refused = v; restart(); } });
toggle(ctl, { id: 'nokey', label: 'Blind retry without the key (needs "reply lost")', onInput: (v) => { state.noKey = v; restart(); } });
clock.seek(11);
