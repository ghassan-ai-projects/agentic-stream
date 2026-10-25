import * as THREE from 'three';
import { ch, chapters, cue } from '../timeline.js';
import { engine as EN, episode as EP, action as AC, policy as PO, scheduler as SC } from './schedule.js';
import { angleOf, stationPoint, outward, tangent, ringPoint, R } from './layout.js';
import { easeInOut } from '../util.js';

const Y = new THREE.Vector3(0, 1, 0);

function view(id, { out = 18, up = 9, side = 0, ahead = 0, tUp = 2, tOut = 0, fov = 42 } = {}) {
  const a = angleOf(id);
  const P = stationPoint(id);
  const o = outward(a), t = tangent(a);
  return {
    pos: P.clone().addScaledVector(o, out).addScaledVector(t, side).addScaledVector(Y, up),
    target: P.clone().addScaledVector(t, ahead).addScaledVector(Y, tUp).addScaledVector(o, tOut),
    fov,
  };
}

function mid(idA, idB, k = 0.5, { out = 22, up = 11, side = 0, tUp = 2, tOut = 0, fov = 42, extra = 0 } = {}) {
  const a = angleOf(idA) + (angleOf(idB) - angleOf(idA) + (idB === 'bench' ? Math.PI * 2 : 0)) * k;
  const P = ringPoint(a, 0, R);
  const o = outward(a), t = tangent(a);
  return {
    pos: P.clone().addScaledVector(o, out + extra).addScaledVector(t, side).addScaledVector(Y, up),
    target: P.clone().addScaledVector(t, 0).addScaledVector(Y, tUp).addScaledVector(o, tOut),
    fov,
  };
}

const raw = (pos, target, fov = 42) => ({ pos: new THREE.Vector3(...pos), target: new THREE.Vector3(...target), fov });

const C = (id, i, edge = 'start') => cue(id, i)[edge];
const plan = {
  intro: [[0, raw([-24, 86, 112], [0, 0, 22], 42)], [C('intro', 2, 'end'), raw([26, 80, 108], [0, 0, 22], 42)]],
  gateway: [
    [ch.gateway.start + 2.0, mid('bench', 'gateway', 0.42, { out: 38, up: 19, tUp: 3.2 })],
    [C('gateway', 1) + 0.4, mid('bench', 'gateway', 0.62, { out: 34, up: 17, tUp: 4.0 })],
  ],
  log: [[ch.log.start + 2.0, view('log', { out: 25, up: 12.5, ahead: 1.4, tUp: 2.6 })], [C('log', 2), view('log', { out: 24, up: 12, ahead: 2.6, tUp: 2.8 })]],
  engine: [
    [ch.engine.start + 1.8, view('engine', { out: 25, up: 14.5, ahead: 0.5, tUp: 2.6 })],
    [EN.watermark + 0.6, view('engine', { out: 22, up: 12.5, ahead: 2.6, tUp: 2.2 })],
    [EN.features, view('engine', { out: 25, up: 15, ahead: 1.0, tUp: 2.6 })],
    [EN.same + 0.4, mid('engine', 'situation', 0.5, { out: 36, up: 21, tUp: 4.5 })],
  ],
  situation: [[ch.situation.start + 2.0, view('situation', { out: 31, up: 12, tUp: 6.0, ahead: 0 })], [C('situation', 4) - 0.4, view('situation', { out: 26, up: 9, tUp: 4.0, ahead: 0.6 })]],
  scheduler: [
    [ch.scheduler.start + 2.2, view('scheduler', { out: 21, up: 11.5, ahead: -0.6, tUp: 0.6, tOut: 0.6 })],
    [SC.items[4].gate + 0.4, view('scheduler', { out: 20, up: 11, ahead: 2.0, tUp: 0.6, tOut: 0.4 })],
    [SC.release + 0.4, view('scheduler', { out: 27, up: 16, ahead: 3.4, tUp: 1.6 }), 2.2],
  ],
  episode: [
    [ch.episode.start + 1.8, view('episode', { out: 27, up: 15, side: 1, tUp: 3.0, tOut: -2, ahead: 0 })],
    [EP.request - 0.6, view('episode', { out: 33, up: 21, side: 0, tUp: 3.0, tOut: -8, ahead: -5 })],
    [EP.answer + 0.4, view('episode', { out: 30, up: 18, side: 2, tUp: 3.0, tOut: -4, ahead: 0 })],
  ],
  validator: [[ch.validator.start + 1.8, view('validator', { out: 24, up: 12, ahead: -1.0, tUp: 3.2 })], [C('validator', 3) - 0.6, view('validator', { out: 23, up: 12, ahead: 1.0, tUp: 3.2 })]],
  policy: [[ch.policy.start + 1.6, view('policy', { out: 24, up: 13, ahead: 0, tUp: 3.2, side: 0 })], [PO.r2 + 0.6, view('policy', { out: 19, up: 10.5, ahead: -1.0, tUp: 3.4, tOut: -3.0 })]],
  action: [
    [ch.action.start + 1.4, view('action', { out: 26, up: 13, ahead: 0.5, tUp: 2.8 })],
    [AC.send + 0.2, mid('action', 'bench', 0.62, { out: 36, up: 23, tUp: 3.5 })],
    [AC.atBench + 0.2, mid('bench', 'gateway', 0.4, { out: 35, up: 16, tUp: 3.5 })],
    [AC.verified + 0.4, mid('bench', 'gateway', 0.3, { out: 44, up: 26, tUp: 3.0 })],
  ],
  outro: [[cue('outro', 0).start + 0.6, mid('bench', 'gateway', 0.4, { out: 48, up: 30, tUp: 2.5 })], [cue('outro', 1).start + 0.4, raw([0, 98, 120], [0, 0, 18], 42)]],
};

const MOVE = 2.6;
const ARRIVE = 3.2;
const entries = chapters.flatMap((c) => plan[c.id].map(([t, p, move], k) => ({ t, p, move: move ?? (k === 0 ? ARRIVE : MOVE) })));
entries.sort((a, b) => a.t - b.t);
const keys = [entries[0]];
for (const e of entries.slice(1)) {
  const prev = keys.at(-1);
  const leave = e.t - e.move;
  if (leave > prev.t + 0.05) keys.push({ t: leave, p: prev.p });
  keys.push(e);
}

const tmp = { pos: new THREE.Vector3(), target: new THREE.Vector3(), fov: 42 };

export function cameraAt(T, out = tmp) {
  let i = keys.findIndex((k) => k.t > T);
  if (i === -1) i = keys.length;
  const a = keys[Math.max(0, i - 1)], b = keys[Math.min(keys.length - 1, i)];
  const u = b.t === a.t ? 1 : easeInOut((T - a.t) / (b.t - a.t));
  out.pos.copy(a.p.pos).lerp(b.p.pos, u);
  out.target.copy(a.p.target).lerp(b.p.target, u);
  out.pos.y += Math.sin(T * 0.23) * 0.25;
  out.pos.x += Math.sin(T * 0.17) * 0.35;
  out.fov = a.p.fov + (b.p.fov - a.p.fov) * u;
  return out;
}

export function stationView(id) { return view(id, { out: 19, up: 10, tUp: 2 }); }
