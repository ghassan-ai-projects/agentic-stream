import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, easeInOut, lerp } from '../../util.js';
import { COUNTS, RUN } from '../../story.js';
import { scheduler } from '../schedule.js';
import { ch } from '../../timeline.js';

const Y = 0.56;
const BELT_Y = Y + 0.55;
const X_IN = -8.6;
const X1 = -2.0;
const X2 = 1.6;
const X_Q = 6.0;
const SC = 0.045;
const BIN_A = new THREE.Vector3(-1.2, Y + 0.1, 5.0);
const BIN_B = new THREE.Vector3(1.8, Y + 0.1, -5.2);

const pile = (base, k) => new THREE.Vector3(base.x + ((k % 4) - 1.5) * 0.95, base.y + Math.floor(k / 8) * 0.28 + 0.15, base.z + (Math.floor(k / 4) % 2 - 0.5) * 1.0);

export function createScheduler() {
  const g = stationGroup('scheduler', 'cognition', 22, 15);
  const amber = COLORS.plane.cognition;
  g.add(box(18.6, 0.3, 2.2, mat.body(0x0c1429, { roughness: 0.8 }), -0.2, Y + 0.3, 0));
  g.add(box(18.6, 0.05, 0.1, new THREE.MeshBasicMaterial({ color: amber.clone().multiplyScalar(0.9) }), -0.2, Y + 0.47, 1.1));
  g.add(box(18.6, 0.05, 0.1, new THREE.MeshBasicMaterial({ color: amber.clone().multiplyScalar(0.9) }), -0.2, Y + 0.47, -1.1));

  const beamY = BELT_Y + 35 * SC;
  for (const z of [-1.6, 1.6]) g.add(box(0.28, 3.2, 0.28, mat.body(0x1a2440, { metalness: 0.6 }), X1, Y + 1.6, z));
  const beam = box(0.16, 0.16, 3.4, new THREE.MeshBasicMaterial({ color: amber.clone().multiplyScalar(2.0) }), X1, beamY, 0);
  g.add(beam);
  for (const [v, label] of [[0, 0], [35, 1], [60, 0]]) g.add(box(0.5, 0.04, 0.05, mat.body(0x9fb2d6), X1 - 1.2, BELT_Y + v * SC, 1.6));
  const portal = new THREE.Mesh(new THREE.TorusGeometry(1.7, 0.1, 12, 48), new THREE.MeshBasicMaterial({ color: COLORS.plane.stream.clone().multiplyScalar(1.5) }));
  portal.rotation.y = Math.PI / 2;
  portal.position.set(X2, Y + 1.8, 0);
  g.add(portal);

  const lane = box(6.2, 0.18, 2.4, mat.glass(amber, 0.22), X_Q + 0.6, Y + 0.38, 0);
  g.add(lane, edges(lane, amber, 0.9));
  const binMat = mat.glass(0x93a3c0, 0.12);
  for (const b of [BIN_A, BIN_B]) {
    const bx = box(4.6, 0.5, 2.8, binMat, b.x, Y + 0.25, b.z);
    g.add(bx, edges(bx, 0x93a3c0, 0.6));
  }

  const tokenGeo = new THREE.BoxGeometry(0.86, 0.22, 0.86);
  const tokens = new THREE.InstancedMesh(tokenGeo, new THREE.MeshBasicMaterial({ color: 0xffffff }), scheduler.items.length);
  const cols = new THREE.InstancedMesh(new THREE.BoxGeometry(0.16, 1, 0.16), mat.glow(0xffb84d, 1.5), scheduler.items.length);
  tokens.frustumCulled = cols.frustumCulled = false;
  tokens.instanceMatrix.setUsage(THREE.DynamicDrawUsage); cols.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  const phaseOf = (v) => (v <= 4 ? 'candidate' : v <= 6 ? 'watch' : 'cooling');
  scheduler.items.forEach((it, i) => tokens.setColorAt(i, COLORS.phase[phaseOf(it.v)].clone().multiplyScalar(1.1)));
  g.add(tokens, cols);

  const timerGeo = (p) => new THREE.RingGeometry(1.05, 1.3, 48, 1, Math.PI / 2, -Math.PI * 2 * Math.max(p, 0.001));
  const timer = new THREE.Mesh(timerGeo(0.5), mat.add(amber, 0.9));
  timer.rotation.x = -Math.PI / 2;
  timer.position.set(X_Q + 2.4, Y + 0.5, 0);
  g.add(timer);
  const timerBase = new THREE.Mesh(new THREE.RingGeometry(1.05, 1.3, 48), mat.add(amber, 0.15));
  timerBase.rotation.x = -Math.PI / 2;
  timerBase.position.copy(timer.position);
  g.add(timerBase);
  const spark = glowSprite(0xffb84d, 5, 0);
  g.add(spark);

  g.updateMatrixWorld(true);
  const ports = { in: g.localToWorld(new THREE.Vector3(X_IN, BELT_Y + 1, 0)), out: g.localToWorld(new THREE.Vector3(9.6, BELT_Y + 0.6, 0)), release: new THREE.Vector3(9.4, BELT_Y, 0) };
  const o = new THREE.Object3D();
  const items = scheduler.items;
  const cnt = { a: 0, b: 0 };
  const rank = {};
  let ka = 0, kb = 0;
  items.forEach((it) => { if (it.verdict === 'condition-false') rank[it.v] = ka++; if (it.verdict === 'nothing-new') rank[it.v] = kb++; });

  function tokenPose(it, T) {
    const enter = it.gate - 1.4;
    if (T < enter) return null;
    const s1 = it.gate;
    const s2 = it.gate + 0.7;
    if (T < s1) return new THREE.Vector3(lerp(X_IN, X1, easeInOut((T - enter) / 1.4)), BELT_Y + 0.12, 0);
    if (it.verdict === 'condition-false') {
      const u = clamp((T - s1) / 0.9);
      return new THREE.Vector3(X1, BELT_Y + 0.12, 0).lerp(pile(BIN_A, rank[it.v]), easeInOut(u)).add(new THREE.Vector3(0, Math.sin(Math.PI * u) * 0.9, 0));
    }
    if (T < s2) return new THREE.Vector3(lerp(X1, X2, (T - s1) / 0.7), BELT_Y + 0.12, 0);
    if (it.verdict === 'nothing-new') {
      const u = clamp((T - s2) / 0.9);
      return new THREE.Vector3(X2, BELT_Y + 0.12, 0).lerp(pile(BIN_B, rank[it.v]), easeInOut(u)).add(new THREE.Vector3(0, Math.sin(Math.PI * u) * 0.9, 0));
    }
    const slotX = it.v === 5 ? X_Q - 0.2 : X_Q + 1.2;
    const u = clamp((T - s2) / 1.0);
    const base = new THREE.Vector3(lerp(X2, slotX, easeInOut(u)), BELT_Y + 0.12, 0);
    if (it.v === 5 && T > scheduler.coalesce) {
      const k = clamp((T - scheduler.coalesce) / 0.9);
      base.x = lerp(slotX, X_Q + 1.2, easeInOut(k));
    }
    if (it.v === 7 && T > scheduler.release) {
      const k = clamp((T - scheduler.release) / 1.2);
      base.x = lerp(slotX, 9.4, easeInOut(k));
    }
    return base;
  }

  function update(T, tags) {
    let ign = 0, adm = 0, coal = 0;
    items.forEach((it, i) => {
      const p = tokenPose(it, T);
      if (!p || (it.v === 7 && T > scheduler.release + 1.3)) { o.position.set(0, -40, 0); o.scale.setScalar(0.0001); o.updateMatrix(); tokens.setMatrixAt(i, o.matrix); cols.setMatrixAt(i, o.matrix); return; }
      let s = 1;
      if (it.v === 5 && T > scheduler.coalesce) s = Math.max(0.0001, 1 - clamp((T - scheduler.coalesce) / 0.9));
      o.position.copy(p); o.scale.setScalar(s); o.rotation.set(0, 0, 0); o.updateMatrix();
      tokens.setMatrixAt(i, o.matrix);
      const h = Math.max(it.score, 1) * SC * (it.gate > T - 0.001 || it.verdict === 'queue' || true ? 1 : 1);
      const showCol = T < it.decided + 0.2 || it.verdict === 'queue';
      o.position.set(p.x, p.y + 0.11 + (h * s) / 2, p.z); o.scale.set(s, showCol ? h * s : 0.0001, s); o.updateMatrix();
      cols.setMatrixAt(i, o.matrix);
      if (T >= it.decided) { if (it.verdict === 'queue') adm++; else ign++; }
    });
    if (T >= scheduler.coalesce) coal = 1;
    tokens.instanceMatrix.needsUpdate = true; cols.instanceMatrix.needsUpdate = true;

    const hit = Math.max(...items.map((it) => decay(T, it.gate, 0.18)));
    beam.material.color.copy(amber).multiplyScalar(2.0 + hit * 2.5);
    const hit2 = Math.max(...items.filter((it) => it.verdict !== 'condition-false').map((it) => decay(T, it.gate + 0.6, 0.18)));
    portal.material.color.copy(COLORS.plane.stream).multiplyScalar(1.5 + hit2 * 2);

    const v5on = T > scheduler.queueV5 - 0.5 && T < scheduler.coalesce;
    const v7on = T > scheduler.queueV7 && T < scheduler.release;
    let progress = 0;
    if (v5on) progress = clamp((T - (scheduler.queueV5 - 0.5)) / ((scheduler.coalesce - scheduler.queueV5 + 0.5) * 2.2));
    if (v7on || T >= scheduler.coalesce && T < scheduler.release) progress = clamp((T - scheduler.coalesce) / (scheduler.release - scheduler.coalesce));
    timer.geometry.dispose(); timer.geometry = timerGeo(progress);
    timer.material.opacity = (v5on || (T >= scheduler.coalesce && T < scheduler.release + 0.4)) ? 0.9 : 0;
    timerBase.material.opacity = timer.material.opacity * 0.2;
    spark.position.set(X_Q + 0.5, Y + 1.0, 0);
    spark.material.opacity = pulse(T, scheduler.coalesce + 0.15, 0.3) * 0.9;

    const act = window01(T, ch.scheduler.start - 1, ch.scheduler.end, 1.2);
    setPlatformEnergy(g, act * 0.9);

    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    const near = window01(T, ch.scheduler.start, ch.scheduler.end - 0.4, 1.0);
    tags.put('sch-name', P(-3, 8.2, -2), `Cognitive scheduler<small>${T < items[3].decided ? 'deterministic · no model call' : `${COUNTS.versions} versions → ${ign} ignored · ${coal} replaced · ${T >= scheduler.release ? 1 : 0} episode`}</small>`, near * 0.95, 'name cognition');
    const cur = items.filter((it) => T >= it.gate - 0.25 && T < it.gate + (it.v >= 8 ? 0.4 : 1.1)).at(-1);
    if (cur) tags.put('sch-cur', P(-6.5, BELT_Y + 3.6, 0), `<b>v${cur.v}</b> · score <b>${cur.score}</b><small>${cur.reason}</small>`, 0.95, 'note');
    const firstB = items.find((it) => it.verdict === 'nothing-new');
    const nB = items.filter((it) => it.verdict === 'nothing-new').length;
    const nA = items.filter((it) => it.verdict === 'condition-false').length;
    const gateOn = window01(T, ch.scheduler.start + 0.5, items[7].gate, 0.5) * 0.9;
    tags.put('sch-g1', P(X1, Y + 3.6, 1.6), `trigger holds and score ≥ ${items[0].threshold}?`, gateOn, 'note');
    tags.put('sch-g2', P(X2, Y + 4.0, -1.6), 'something new?<small>a material change</small>', gateOn * window01(T, scheduler.items[4].gate, 1e9, 0.4), 'note');
    tags.put('sch-binA', P(BIN_A.x, Y + 1.8, BIN_A.z), `ignored · trigger false<small>${items.filter((it) => it.verdict === 'condition-false' && T >= it.decided).length} of ${nA}</small>`, window01(T, items[0].gate, ch.scheduler.end - 0.6, 0.5) * 0.9, 'note');
    tags.put('sch-binB', P(BIN_B.x + 3.5, Y + 1.2, BIN_B.z - 1.5), `ignored · nothing new<small>${items.filter((it) => it.verdict === 'nothing-new' && T >= it.decided).length} of ${nB}</small>`, window01(T, firstB.gate + 0.6, ch.scheduler.end - 0.6, 0.5) * 0.9, 'note');
    const [q5, q7] = RUN.scheduler;
    tags.put('sch-q', P(X_Q + 3.0, Y + 3.2, 0), T < scheduler.coalesce ? `queued · v${q5.v}<small>debounce ${q5.debounceSeconds} s</small>` : T < scheduler.release ? `v${q7.v} replaced v${q5.v}<small>coalesced · waits ${q7.debounceSeconds} s (cooldown)</small>` : `admitted<small>one episode, for v${q7.v}</small>`, window01(T, scheduler.queueV5 - 0.3, scheduler.arrive, 0.4) * 0.95, 'note cognition');
  }
  return { id: 'scheduler', group: g, ports, update };
}
