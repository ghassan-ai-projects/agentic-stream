import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut } from '../../util.js';
import { events, logBeats } from '../schedule.js';
import { ch } from '../../timeline.js';

const Y = 0.56;
const X0 = -1.9;
const DX = 0.135;

export function createLog() {
  const g = stationGroup('log', 'evidence', 16, 14);
  const gateRing = new THREE.Mesh(new THREE.TorusGeometry(2.1, 0.11, 12, 56), new THREE.MeshBasicMaterial({ color: COLORS.plane.evidence.clone().multiplyScalar(1.6) }));
  gateRing.rotation.y = Math.PI / 2;
  gateRing.position.set(-4.6, Y + 2.3, 0);
  g.add(gateRing);
  for (const z of [-2.2, 2.2]) g.add(box(0.5, 1.9, 0.5, mat.body(0x1a2440, { metalness: 0.6 }), -4.6, Y + 0.95, z));
  const scan = new THREE.Mesh(new THREE.PlaneGeometry(4.0, 0.12), new THREE.MeshBasicMaterial({ color: 0x4cc9f0, transparent: true, opacity: 0.8, side: THREE.DoubleSide, blending: THREE.AdditiveBlending, depthWrite: false }));
  scan.rotation.y = Math.PI / 2;
  scan.position.set(-4.6, Y + 2.3, 0);
  g.add(scan);
  const flash = glowSprite(0x34d399, 7, 0);
  flash.position.set(-4.6, Y + 2.3, 0);
  g.add(flash);

  const trough = box(9.2, 0.5, 3.0, mat.body(0x0b1328, { roughness: 0.8 }), 2.6, Y + 0.25, 0);
  g.add(trough);
  g.add(box(9.4, 0.06, 0.12, new THREE.MeshBasicMaterial({ color: COLORS.plane.evidence.clone().multiplyScalar(1.4) }), 2.6, Y + 0.52, 1.55));
  g.add(box(9.4, 0.06, 0.12, new THREE.MeshBasicMaterial({ color: COLORS.plane.evidence.clone().multiplyScalar(1.4) }), 2.6, Y + 0.52, -1.55));
  const slabs = new THREE.InstancedMesh(new THREE.BoxGeometry(0.11, 1, 2.4), new THREE.MeshBasicMaterial({ color: 0xffffff }), events.length);
  slabs.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  const col = new THREE.Color();
  events.forEach((e, i) => slabs.setColorAt(i, col.copy(COLORS.type[e.type]).multiplyScalar(0.9)));
  slabs.frustumCulled = false;
  g.add(slabs);
  const head = glowSprite(0xffffff, 3, 0.0);
  g.add(head);

  const bin = new THREE.Group();
  bin.add(box(3.2, 0.9, 2.4, mat.glass(0xff5470, 0.18), 0, 0.45, 0));
  bin.add(edges(box(3.2, 0.9, 2.4, mat.glass(0xff5470)), 0xff5470, 0.9));
  bin.position.set(3.6, Y, 4.6);
  g.add(bin);
  const bad = new THREE.Mesh(new THREE.OctahedronGeometry(0.42, 0), mat.glow(0xff5470, 2));
  g.add(bad);

  g.updateMatrixWorld(true);
  const ports = { in: g.localToWorld(new THREE.Vector3(-4.6, Y + 2.3, 0)), out: g.localToWorld(new THREE.Vector3(7.2, Y + 1.4, 0)), top: g.localToWorld(new THREE.Vector3(2.6, Y + 2.6, 0)) };
  const slotWorld = (i) => g.localToWorld(new THREE.Vector3(X0 + i * DX, Y + 0.9, 0));
  const o = new THREE.Object3D();
  const badAt = logBeats.bad;

  function update(T, tags) {
    scan.position.y = Y + 2.3 + Math.sin(T * 9) * 1.7;
    let pass = 0;
    for (const e of events) pass = Math.max(pass, decay(T, e.atLog, 0.22));
    flash.material.opacity = pass * 0.8;
    const badPulse = pulse(T, badAt + 0.2, 0.25);
    gateRing.material.color.copy(COLORS.plane.evidence).lerp(new THREE.Color(0x34d399), pass).lerp(new THREE.Color(0xff5470), badPulse).multiplyScalar(1.5 + pass * 1.2 + badPulse * 1.5);

    let landed = 0;
    events.forEach((e, i) => {
      const u = clamp((T - e.landed) / 0.5);
      if (u > 0) landed++;
      const h = 0.25 + 1.55 * easeOut(u) * (e.type === 'heartbeat' ? 0.6 : 1);
      o.position.set(X0 + i * DX, Y + 0.5 + (u > 0 ? h / 2 : -3), 0);
      o.scale.set(1, u > 0 ? h : 0.0001, 1);
      o.updateMatrix();
      slabs.setMatrixAt(i, o.matrix);
    });
    slabs.instanceMatrix.needsUpdate = true;
    head.position.set(X0 + landed * DX, Y + 2.2, 0);
    head.material.opacity = landed > 0 && landed < 61 ? 0.55 : 0;

    const u = clamp((T - badAt) / 2.4);
    if (u > 0 && u < 1.9) {
      const from = new THREE.Vector3(-4.6, Y + 2.3, 0), to = new THREE.Vector3(3.6, Y + 0.9, 4.6);
      const k = smooth(u);
      bad.position.copy(from).lerp(to, k); bad.position.y += Math.sin(Math.PI * k) * 2.2;
      bad.scale.setScalar(clamp(1.3 - Math.max(0, T - badAt - 3.5) * 0.2, 0.6, 1.3)); bad.rotation.set(T * 3, T * 2, 0);
    } else bad.scale.setScalar(0.0001);

    setPlatformEnergy(g, window01(T, ch.log.start - 2, ch.log.end + 1, 1.5) * 0.8);
    const near = window01(T, ch.log.start, ch.log.end, 1.0);
    tags.put('log-name', g.localToWorld(new THREE.Vector3(0, 7.4, 4)), 'Ingress + event log<small>append-only, in arrival order</small>', near * 0.95, 'name evidence');
    tags.put('log-count', g.localToWorld(new THREE.Vector3(2.6, Y + 3.3, 0)), `<b>${landed}</b> appended · <b>0</b> quarantined<small>${landed >= 61 ? 'every one kept its event time' : 'validated against its schema first'}</small>`, window01(T, ch.log.start + 1.8, ch.log.end + 0.3, 0.6) * 0.95, 'note');
    tags.put('log-bad', g.localToWorld(new THREE.Vector3(3.6, Y + 2.6, 4.6)), 'illustrative: malformed input<small>quarantined and kept as data, never passed on</small>', window01(T, badAt + 1.0, ch.log.end - 0.2, 0.4) * 0.95, 'note warn');
    tags.put('log-scan', g.localToWorld(new THREE.Vector3(-4.6, Y + 5.0, 0)), 'schema check<small>identity and payload</small>', window01(T, Math.max(logBeats.scan - 0.4, ch.log.start + 1.8), logBeats.bad + 0.2, 0.5) * 0.9, 'note');
    tags.put('log-evidence', g.localToWorld(new THREE.Vector3(6.6, Y + 4.4, -1.5)), 'evidence, never an instruction<small>nothing in a payload can trigger an action</small>', window01(T, logBeats.evidence, ch.log.end + 0.3, 0.4) * 0.95, 'note gold');
  }
  return { id: 'log', group: g, ports, slotWorld, update };
}
