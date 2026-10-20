import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, fmt } from '../../util.js';
import { RUN } from '../../story.js';
import { events, engine, versions } from '../schedule.js';
import { ch } from '../../timeline.js';

const Y = 0.56;
const LANES = ['temp', 'humidity', 'ambient', 'fan_tach', 'heartbeat'];
const LANE_Z = [-4.4, -2.2, 0, 2.2, 4.4];
const XR = 6.8;
const SC = 0.04;
const WIN = 300 * SC;
const SPAN = RUN.events.spanSeconds;

export function createEngine() {
  const g = stationGroup('engine', 'stream', 19, 16);
  const violet = COLORS.plane.stream;
  LANES.forEach((l, i) => {
    g.add(box(17, 0.04, 0.1, new THREE.MeshBasicMaterial({ color: COLORS.type[l].clone().multiplyScalar(0.55) }), 0, Y + 0.05, LANE_Z[i]));
  });

  const dots = new THREE.InstancedMesh(new THREE.SphereGeometry(0.26, 14, 10), new THREE.MeshBasicMaterial({ color: 0xffffff }), events.length + 1);
  dots.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  dots.frustumCulled = false;
  const col = new THREE.Color();
  events.forEach((e, i) => dots.setColorAt(i, col.copy(COLORS.type[e.type]).multiplyScalar(0.95)));
  dots.setColorAt(events.length, col.set(0xff5470));
  g.add(dots);
  const lateGlow = glowSprite(0xff5470, 3.4, 0);
  g.add(lateGlow);

  const winGeo = new THREE.BoxGeometry(WIN, 2.8, 10.6);
  const win = new THREE.Mesh(winGeo, mat.glass(violet, 0.09));
  const winEdges = new THREE.LineSegments(new THREE.EdgesGeometry(winGeo), mat.line(violet.clone().multiplyScalar(1.4), 0.95));
  const winGroup = new THREE.Group();
  winGroup.add(win, winEdges);
  winGroup.position.set(XR - WIN / 2, Y + 1.5, 0);
  g.add(winGroup);

  const wm = new THREE.Mesh(new THREE.PlaneGeometry(10.6, 3.2), mat.add(0xffd166, 0.22));
  wm.rotation.y = Math.PI / 2;
  wm.position.set(XR - 15 * SC, Y + 1.7, 0);
  g.add(wm);
  const head = new THREE.Mesh(new THREE.PlaneGeometry(10.6, 3.2), mat.add(0xffffff, 0.2));
  head.rotation.y = Math.PI / 2;
  head.position.set(XR, Y + 1.7, 0);
  g.add(head);

  const ops = [
    { id: 'mean', x: -4.6, color: 0xff7a59 },
    { id: 'slope', x: 0, color: 0xffb36b },
    { id: 'heartbeat', x: 4.6, color: 0xf1f5f9 },
  ].map((o) => {
    const grp = new THREE.Group();
    grp.add(box(2.6, 1.6, 2.2, mat.body(0x1a2440, { metalness: 0.6 }), 0, 0.8, 0));
    grp.add(edges(box(2.6, 1.6, 2.2, mat.body(0)), violet, 0.9).translateY(0.8));
    const core = new THREE.Mesh(new THREE.SphereGeometry(0.55, 20, 16), mat.glow(o.color, 0.4));
    core.position.y = 2.1;
    grp.add(core);
    grp.position.set(o.x, Y, -6.3);
    g.add(grp);
    return { ...o, core, grp };
  });
  const feed = ops.map((o) => {
    const line = new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(o.x, Y + 2.1, -6.3), new THREE.Vector3(o.x, Y + 1.5, -4.2)]), mat.line(violet, 0.0));
    g.add(line);
    return line;
  });

  g.updateMatrixWorld(true);
  const ports = { feature: g.localToWorld(new THREE.Vector3(7.2, Y + 3.2, -6.3)), top: g.localToWorld(new THREE.Vector3(0, Y + 3.4, 0)) };
  const o3 = new THREE.Object3D();
  const lateAt = engine.late;

  function update(T, tags) {
    const e = engine.clock(T);
    const eStart = e <= 0.01;
    events.forEach((ev, i) => {
      const x = XR - (e - ev.t) * SC;
      const vis = e >= ev.t && !eStart;
      const pop = vis ? easeOut((e - ev.t) / 6) : 0;
      o3.position.set(x, Y + 0.45, LANE_Z[LANES.indexOf(ev.type)]);
      const inWin = e - ev.t <= 300;
      o3.scale.setScalar(vis ? (0.35 + pop * 0.65) * (inWin ? 1 : 0.6) : 0.0001);
      o3.updateMatrix();
      dots.setMatrixAt(i, o3.matrix);
    });
    const lt = clamp((T - lateAt) / 1.6);
    const lateE = engine.lateEvent;
    o3.position.set(XR - (engine.clock(T) - lateE) * SC, Y + 0.45 + (1 - easeOut(lt)) * 5, LANE_Z[2] + 1.1);
    o3.scale.setScalar(T >= lateAt && T < lateAt + 4.6 ? 1.7 : 0.0001);
    o3.updateMatrix();
    dots.setMatrixAt(events.length, o3.matrix);
    lateGlow.position.copy(o3.position);
    lateGlow.material.opacity = T >= lateAt && T < lateAt + 4.6 ? 0.75 : 0;
    dots.instanceMatrix.needsUpdate = true;

    const wmFocus = window01(T, engine.watermark, engine.late + 3.5, 0.6);
    wm.material.opacity = 0.16 + wmFocus * 0.3 + pulse(T, engine.watermark + 0.4, 0.5) * 0.3;
    let slidePulse = 0;
    versions.forEach((v) => { if (v.completeness === 'provisional') slidePulse = Math.max(slidePulse, decay(T, v.emit, 0.35)); });
    winEdges.material.color.copy(violet).multiplyScalar(1.2 + slidePulse * 1.4);
    win.material.opacity = 0.07 + slidePulse * 0.1;
    ops.forEach((op, i) => {
      op.core.material.emissiveIntensity = 0.3 + (op.id === 'heartbeat' ? 0.5 : slidePulse * 2.4);
      feed[i].material.opacity = (op.id === 'heartbeat' ? 0.2 : slidePulse) * 0.9;
    });
    const active = window01(T, engine.t0 - 0.5, engine.t1 + 1.5, 0.5);
    setPlatformEnergy(g, active * 0.9);

    const cur = versions.filter((v) => T >= v.emit).at(-1);
    const near = window01(T, ch.engine.start, engine.same, 1.0);
    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    tags.put('engine-name', P(0, 8.2, 5), 'Stream engine<small>event time · windows · operators</small>', near * 0.95, 'name stream');
    tags.put('engine-clock', P(XR + 0.4, Y + 4.6, 4.6), `event time <b>${Math.round(e)} s</b><small>when each reading happened</small>`, window01(T, engine.t0 + 0.3, engine.watermark, 0.5) * 0.95, 'note');
    tags.put('engine-wm', P(XR - 15 * SC - 0.2, Y + 3.6, -4.9), `watermark<small>newest reading − ${RUN.versions[0].watermarkLagSeconds} s</small>`, window01(T, engine.watermark - 0.2, engine.t1 + 1.5, 0.6) * 0.95, 'note gold');
    tags.put('engine-late', P(XR - (engine.clock(lateAt + 1) - lateE) * SC, Y + 2.8, 3.2), 'illustrative: a late reading<small>placed at its event time · the watermark never moves back</small>', window01(T, lateAt + 0.4, lateAt + 4.6, 0.5) * 0.95, 'note warn');
    tags.put('engine-win', P(XR - WIN / 2, Y + 3.9, 0), `window · last 5 min<small>slides every 30 s · event time ${Math.round(e)} s</small>`, window01(T, engine.window - 0.2, engine.t1 + 2, 0.6) * 0.95, 'note');
    tags.put('engine-feat', P(0, Y + 4.4, -6.3), cur ? `mean <b>${fmt(cur.tempMean, 2)} °C</b> · slope <b>${fmt(cur.tempSlope, 1)} °C/h</b><small>features from the window</small>` : '', window01(T, engine.features - 0.2, ch.engine.end + 0.4, 0.5) * (cur ? 0.95 : 0), 'note');
    tags.put('engine-same', P(-5.5, Y + 3.2, 4.8), 'deterministic<small>same input, same features</small>', window01(T, engine.same, ch.engine.end + 0.2, 0.4) * 0.95, 'note ok');
  }
  return { id: 'engine', group: g, ports, update };
}
