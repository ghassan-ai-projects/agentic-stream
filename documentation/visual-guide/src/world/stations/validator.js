import * as THREE from 'three';
import { COLORS, mat, box, edges, glowSprite, textPlane } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, easeInOut, lerp } from '../../util.js';
import { RUN } from '../../story.js';
import { validator, episode } from '../schedule.js';
import { ch } from '../../timeline.js';
import { angleOf, PORT, arcPoint } from '../layout.js';

const Y = 0.56;
const CHECKS = [
  { x: -2.4, label: 'type is in the catalog', dy: 0 },
  { x: 0.2, label: 'parameters fit the schema', dy: 2.0 },
  { x: 2.8, label: 'only model-writable fields', dy: 0 },
  { x: 5.4, label: 'grounded in evidence', dy: 2.0 },
];
const GREEN = new THREE.Color(0x34d399);

const CAT = [['install_watch_condition', 'R0'], ['set_indicator', 'R1'], ['select_thermal_mode', 'R2']];
const CATALOG = `<div class="cardx"><h4>Intent catalog · written in the spec</h4>${CAT.map(([n, r]) => `<div class="kv${n === RUN.intent.type ? ' ok' : ''}"><span>${r}</span><b>${n}</b></div>`).join('')}<div class="foot">the only actions a decision may propose</div></div>`;
const D = RUN.decision;
const DECISION = (intent) => `<div class="cardx"><h4>Decision · from Tamoz</h4><div class="hyp">${D.hypothesis}</div><div class="kv"><span>confidence</span><b>${D.confidence}</b></div><div class="kv"><span>snapshot</span><b>v${RUN.episode.snapshotVersion} · ${D.snapshotDigest}…</b></div><div class="kv"><span>facts cited</span><b>${D.factsUsed.length}: ${D.factsUsed.join(', ')}</b></div><div class="kv hot${intent ? '' : ' dim'}"><span>intent</span><b>${RUN.intent.type} { state: "${RUN.intent.parameters.state}" } · ${RUN.intent.risk}</b></div></div>`;

export function createValidator() {
  const g = stationGroup('validator', 'governance', 17, 13);
  const green = COLORS.plane.governance;
  g.add(box(14, 0.16, 2.6, mat.body(0x0c1429), 0.2, Y + 0.2, 0));

  const frames = CHECKS.map((c) => {
    const grp = new THREE.Group();
    const m = new THREE.Mesh(new THREE.PlaneGeometry(3.2, 4.4), mat.add(0x93a3c0, 0.1));
    m.rotation.y = Math.PI / 2;
    const e = new THREE.LineSegments(new THREE.EdgesGeometry(new THREE.PlaneGeometry(3.2, 4.4)), mat.line(0x93a3c0, 0.9));
    e.rotation.y = Math.PI / 2;
    grp.add(m, e);
    grp.position.set(c.x, Y + 2.8, 0);
    g.add(grp);
    return { m, e };
  });

  const catalog = new THREE.Group();
  const entries = [
    { name: 'install_watch_condition', risk: 'R0' },
    { name: 'set_indicator', risk: 'R1' },
    { name: 'select_thermal_mode', risk: 'R2' },
  ].map((c, i) => {
    const tp = textPlane([{ text: c.name, size: 36, weight: 700 }, { text: `risk ${c.risk}  ·  from the spec`, size: 26, color: '#93a3c0', weight: 500 }], { w: 560, h: 140, scale: 0.0088, border: '#34d399' });
    tp.position.set(0, i * 1.45, 0);
    catalog.add(tp);
    return { ...c, tp };
  });
  catalog.position.set(-5.6, Y + 1.2, -4.6);
  catalog.rotation.y = 0;
  g.add(catalog);

  const good = textPlane([
    { text: `Decision · ${RUN.decision.hypothesis}`, size: 26, color: '#f5b94a', weight: 700 },
    { text: `intent  ${RUN.intent.type}`, size: 32, color: '#ffffff', weight: 800 },
    { text: `{ state: "${RUN.intent.parameters.state}" }  ${RUN.intent.risk}`, size: 30, color: '#34d399', weight: 700 },
  ], { w: 600, h: 190, scale: 0.0105, border: '#f5b94a' });
  g.add(good);
  const bad = textPlane([
    { text: 'intent  open_valve', size: 32, color: '#ffffff', weight: 800 },
    { text: 'not in the catalog', size: 28, color: '#ff8aa0', weight: 600 },
  ], { w: 600, h: 150, scale: 0.0105, border: '#ff5470', bg: 'rgba(40,10,18,0.92)' });
  g.add(bad);
  const stop = glowSprite(0xff5470, 6, 0);
  g.add(stop);

  g.updateMatrixWorld(true);
  const ports = { in: g.localToWorld(new THREE.Vector3(-8, Y + 3.0, 0)), out: g.localToWorld(new THREE.Vector3(8.2, Y + 2.2, 0)) };
  const aEp = angleOf('episode');
  const aVal = angleOf('validator');

  const holdX = -4.4;
  function update(T, tags) {
    let pos;
    const flyFrom = episode.leave + 0.6;
    if (T < flyFrom) pos = null;
    else if (T < validator.arrive) {
      const u = easeInOut(clamp((T - flyFrom) / (validator.arrive - flyFrom)));
      pos = g.worldToLocal(arcPoint(aEp + PORT * 1.4, aVal - PORT * 1.2, u, 1.4).clone());
      pos.y = Math.max(pos.y, Y + 3.0);
    } else {
      const toHold = easeInOut(clamp((T - validator.arrive) / 1.6));
      const through = easeInOut(clamp((T - (validator.checks[0] - 0.5)) / (validator.checks[3] - validator.checks[0] + 0.8)));
      const out = easeInOut(clamp((T - validator.pass) / (validator.leave + 1.0 - validator.pass)));
      pos = new THREE.Vector3(lerp(-8, holdX, toHold) + through * (1.2 - holdX) + out * 7.4, Y + 3.0, 0);
    }
    if (pos && T < validator.leave + 1.0) { good.position.copy(pos); good.scale.setScalar(1); good.visible = true; } else good.visible = false;

    const rj = clamp((T - validator.reject) / 2.8);
    if (rj > 0 && rj < 1) {
      const x = rj < 0.5 ? lerp(-8.4, -3.0, easeOut(rj * 2)) : lerp(-3.0, -7.2, easeOut((rj - 0.5) * 2));
      bad.position.set(x, Y + 2.2 + Math.sin(rj * Math.PI * 4) * 0.12 * (rj > 0.5 ? 1 : 0), 2.2);
      bad.visible = true;
      stop.position.set(-3.1, Y + 2.2, 2.2);
      stop.material.opacity = pulse(rj, 0.5, 0.12) * 0.9;
    } else { bad.visible = false; stop.material.opacity = 0; }

    frames.forEach((f, i) => {
      const done = ramp(T, validator.checks[i], validator.checks[i] + 0.3);
      const c = new THREE.Color(0x93a3c0).lerp(GREEN, done);
      f.e.material.color.copy(c).multiplyScalar(1 + done * 0.6); f.m.material.color.copy(c); f.m.material.opacity = 0.1 + done * 0.22 + decay(T, validator.checks[i], 0.4) * 0.3;
    });
    const hit = clamp(ramp(T, validator.t + 1.0, validator.t + 2.0));
    entries.forEach((en) => {
      const on = en.name === RUN.intent.type;
      const lit = on ? ramp(T, validator.catalog + 0.3, validator.catalog + 0.9) : 0;
      en.tp.material.opacity = 0.55 + lit * 0.45;
      en.tp.scale.setScalar(1 + lit * 0.08);
    });

    const act = window01(T, episode.leave, ch.validator.end, 1.0);
    setPlatformEnergy(g, act * 0.9);
    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    const near = window01(T, ch.validator.start, validator.decisionCard + 0.3, 0.6) + window01(T, validator.reject + 3.0, ch.validator.end - 0.4, 0.6);
    tags.put('val-name', P(0, Y + 7.6, 5.0), 'Decision validator<small>pure check · no side effects</small>', near * 0.95, 'name governance');
    CHECKS.forEach((c, i) => {
      tags.put(`val-c${i}`, P(c.x, Y + 5.6 + c.dy, 0), `${T >= validator.checks[i] ? '✓ ' : ''}${c.label}`, window01(T, validator.reject + 3.0, validator.leave + 0.6, 0.4) * (T >= validator.checks[i] ? 0.97 : 0.7), T >= validator.checks[i] ? 'note ok' : 'note');
    });
    tags.put('val-dec', good.visible ? g.localToWorld(good.position.clone().add(new THREE.Vector3(0, 1.4, 0))) : P(-6, Y + 4, 0), DECISION(T >= validator.intentCard - 0.2), window01(T, validator.decisionCard + 0.2, validator.catalog + 0.6, 0.5), 'card cognition');
    tags.put('val-cat', P(-5.6, Y + 5.6, -4.6), CATALOG, window01(T, validator.catalog - 0.1, validator.reject + 3.0, 0.5), 'card governance');
    tags.put('val-bad', P(-6.0, Y + 4.8, 0), 'illustrative: refused<small>an intent outside the catalog is not a proposal</small>', window01(T, validator.reject + 0.5, validator.reject + 3.0, 0.4) * 0.95, 'note warn');
    tags.put('val-ok', P(7.0, Y + 4.7, 0), `accepted<small>${RUN.intent.type} · ${RUN.intent.risk} · state "${RUN.intent.parameters.state}"</small>`, window01(T, validator.pass - 0.2, ch.validator.end + 0.6, 0.4) * 0.95, 'note ok');
  }
  return { id: 'validator', group: g, ports, update };
}
