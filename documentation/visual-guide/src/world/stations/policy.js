import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite, textPlane } from '../theme.js';
import { stationGroup, setPlatformEnergy, tube } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, easeInOut, lerp } from '../../util.js';
import { RUN } from '../../story.js';
import { policy, validator } from '../schedule.js';
import { ch } from '../../timeline.js';
import { angleOf, PORT, arcPoint } from '../layout.js';

const Y = 0.56;
const GREEN = new THREE.Color(0x34d399);
const RINGS = [
  { x: -6.0, label: 'still fresh?' },
  { x: -3.6, label: 'condition open?' },
  { x: -1.2, label: 'risk ≤ ceiling?' },
  { x: 1.2, label: 'approval needed?' },
  { x: 3.6, label: 'interlock · owner?' },
  { x: 6.0, label: 'rate limit?' },
];

export function createPolicy() {
  const g = stationGroup('policy', 'governance', 18, 14);
  const green = COLORS.plane.governance;
  g.add(box(16, 0.16, 2.4, mat.body(0x0c1429), 0, Y + 0.2, 0));
  const rings = RINGS.map((r) => {
    const m = new THREE.Mesh(new THREE.TorusGeometry(2.0, 0.13, 12, 56), new THREE.MeshBasicMaterial({ color: 0x93a3c0 }));
    m.rotation.y = Math.PI / 2;
    m.position.set(r.x, Y + 2.6, 0);
    g.add(m);
    return m;
  });

  const podium = new THREE.Group();
  podium.add(cyl(1.5, 1.7, 0.7, mat.body(0x1a2440, { metalness: 0.5 }), 0, 0.35, 0, 24));
  const body = cyl(0.42, 0.5, 1.6, mat.glass(0xc9d6ee, 0.5), 0, 1.5, 0, 16);
  const head = new THREE.Mesh(new THREE.SphereGeometry(0.42, 18, 14), mat.glass(0xc9d6ee, 0.55));
  head.position.y = 2.65;
  podium.add(body, head);
  podium.position.set(-1.0, Y, -4.9);
  g.add(podium);
  const seal = new THREE.Mesh(new THREE.TorusGeometry(0.9, 0.07, 10, 40), new THREE.MeshBasicMaterial({ color: 0xffd166 }));
  seal.rotation.x = Math.PI / 2;
  seal.position.set(-1.0, Y + 1.4, -4.9);
  g.add(seal);
  const ribbonPts = [new THREE.Vector3(-3.4, Y + 2.2, -1.0), new THREE.Vector3(-2.6, Y + 3.6, -3.0), new THREE.Vector3(-1.0, Y + 3.9, -4.4)];
  const ribbon = tube(ribbonPts, 0.05, new THREE.MeshBasicMaterial({ color: 0xffd166, transparent: true, opacity: 0.5 }), 24);
  g.add(ribbon);
  const ribbonCurve = new THREE.CatmullRomCurve3(ribbonPts);
  const ghost = textPlane([{ text: 'select_thermal_mode', size: 34, weight: 800 }, { text: 'R2 · the fan', size: 28, color: '#ffd166', weight: 700 }], { w: 560, h: 150, scale: 0.0088, border: '#ffd166', bg: 'rgba(30,24,8,0.82)' });
  g.add(ghost);

  const card = textPlane([
    { text: `intent  ${RUN.intent.type}`, size: 44, color: '#ffffff', weight: 800 },
    { text: `{ state: "${RUN.intent.parameters.state}" }  ${RUN.intent.risk}`, size: 40, color: '#34d399', weight: 700 },
  ], { w: 760, h: 180, scale: 0.0095, border: '#34d399' });
  g.add(card);
  const stamp = glowSprite(0x34d399, 7, 0);
  g.add(stamp);

  g.updateMatrixWorld(true);
  const ports = { in: g.localToWorld(new THREE.Vector3(-8.6, Y + 2.0, 0)), out: g.localToWorld(new THREE.Vector3(8.6, Y + 2.0, 0)) };
  const aVal = angleOf('validator');
  const aPol = angleOf('policy');
  const aAct = angleOf('action');

  function update(T, tags) {
    let pos;
    const toAction = clamp((T - (policy.leave + 0.8)) / (ch.action.start + 0.4 - policy.leave - 0.8));
    const flyFrom = validator.leave + 0.6;
    if (T < flyFrom) pos = null;
    else if (T < policy.arrive) {
      const u = easeInOut(clamp((T - flyFrom) / (policy.arrive - flyFrom)));
      pos = g.worldToLocal(arcPoint(aVal + PORT * 1.4, aPol - PORT * 1.2, u, 1.4).clone());
      pos.y = Math.max(pos.y, Y + 2.6);
    } else if (T < policy.leave + 0.8) {
      const xs = [-8.6, ...RINGS.map((r) => r.x), 7.2];
      const ts = [policy.arrive, ...policy.rings, policy.rings[5] + 0.8];
      let x = xs.at(-1);
      for (let k = 0; k < ts.length - 1; k++) if (T < ts[k + 1]) { x = lerp(xs[k], xs[k + 1], easeInOut((T - ts[k]) / (ts[k + 1] - ts[k]))); break; }
      if (T < ts[0]) x = xs[0];
      const out = easeInOut(clamp((T - policy.leave) / 0.8));
      pos = new THREE.Vector3(x + out * 3.0, Y + 3.9, 2.4);
    } else if (toAction < 1) {
      pos = g.worldToLocal(arcPoint(aPol + PORT * 1.3, aAct - PORT * 1.2, easeInOut(toAction), 1.4).clone());
      pos.y = Math.max(pos.y, Y + 2.6);
    } else pos = null;
    if (pos) { card.position.copy(pos); card.visible = true; } else card.visible = false;
    card.scale.setScalar(0.95);

    rings.forEach((r, i) => {
      const done = ramp(T, policy.rings[i], policy.rings[i] + 0.25);
      const flash = decay(T, policy.rings[i], 0.35);
      r.material.color.set(0x93a3c0).lerp(GREEN, done).multiplyScalar(1 + flash * 2.2);
    });
    stamp.position.set(7.2, Y + 2.2, 0);
    stamp.material.opacity = pulse(T, policy.approved + 0.1, 0.45) * 0.9;

    const gu = clamp((T - policy.r2) / 2.2);
    if (gu > 0 && T < ch.policy.end - 0.2) {
      const p = ribbonCurve.getPoint(easeInOut(Math.min(1, gu)));
      ghost.position.copy(p).add(new THREE.Vector3(0, 1.3, 0.8));
      ghost.scale.setScalar(1.05);
      ghost.visible = true;
      ghost.material.opacity = 0.72 * (1 - ramp(T, ch.policy.end - 1.0, ch.policy.end - 0.2));
    } else ghost.visible = false;
    const waiting = 0.35 + 0.3 * (0.5 + 0.5 * Math.sin((T - policy.r2) * 3.2)) * ramp(T, policy.r2sign, policy.r2sign + 0.5);
    seal.material.color.set(0x5a4a1e).lerp(new THREE.Color(0xffd166), waiting);
    seal.scale.setScalar(0.001 + window01(T, policy.r2 - 0.4, ch.policy.end, 0.4));
    seal.visible = seal.scale.x > 0.01;
    podium.visible = true;

    const act = window01(T, validator.leave, ch.policy.end + 1, 1.0);
    setPlatformEnergy(g, act * 0.9);
    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    const near = window01(T, ch.policy.start, policy.r2 - 0.2, 0.6);
    tags.put('pol-name', P(0, Y + 7.8, 5.2), 'Policy<small>checks against the state of the world right now</small>', near * 0.95, 'name governance');
    RINGS.forEach((r, i) => tags.put(`pol-r${i}`, P(r.x, Y + (i % 2 ? 6.3 : 4.9), 0), r.label, window01(T, policy.rings[i] - 0.3, policy.r2, 0.35) * 0.9, 'note'));
    tags.put('pol-ok', P(7.0, Y + 5.6, 0), `approved<small>${RUN.policy.reason} · v${RUN.policy.version}</small>`, window01(T, policy.approved - 0.2, ch.policy.end + 0.4, 0.4) * 0.97, 'note ok');
    tags.put('pol-r2', P(-1.0, Y + 6.4, -4.9), `${T >= policy.designed - 0.2 ? '<b>designed, not in this run</b>' : 'R2 · the fan'}<small>an R2 intent waits for a signed human approval</small>`, window01(T, policy.r2 + 0.4, ch.policy.end - 0.1, 0.4) * 0.95, 'note gold');
  }
  return { id: 'policy', group: g, ports, update };
}
