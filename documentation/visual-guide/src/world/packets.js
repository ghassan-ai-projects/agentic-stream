import * as THREE from 'three';
import { COLORS, glowSprite } from './theme.js';
import { events, TRAVEL } from './schedule.js';
import { angleOf, ringPoint, PORT, arcPoint } from './layout.js';
import { clamp, smooth, easeInOut } from '../util.js';

const geo = new THREE.OctahedronGeometry(0.6, 0);

export function createPackets(bench, gateway, log) {
  const group = new THREE.Group();
  const mesh = new THREE.InstancedMesh(geo, new THREE.MeshBasicMaterial({ color: 0xffffff }), events.length);
  mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  mesh.frustumCulled = false;
  const tmp = new THREE.Color();
  events.forEach((e, i) => mesh.setColorAt(i, tmp.copy(COLORS.type[e.type]).multiplyScalar(0.95)));
  group.add(mesh);
  const glows = events.map((e) => { const s = glowSprite(COLORS.type[e.type], 2.2, 0); group.add(s); return s; });
  const o = new THREE.Object3D();
  const aBench = angleOf('bench');
  const aGate = angleOf('gateway');
  const aLog = angleOf('log');
  const A = new THREE.Vector3();
  const B = new THREE.Vector3();
  const C = new THREE.Vector3();

  function position(e, T, out) {
    if (T < e.launch || T >= e.landed) return false;
    if (T < e.atGateway) {
      const u = easeInOut((T - e.launch) / (e.atGateway - e.launch));
      A.copy(bench.port.sensor); B.copy(gateway.ports.in);
      C.copy(A).lerp(B, 0.5); C.y += 5.5;
      out.set(0, 0, 0).addScaledVector(A, (1 - u) * (1 - u)).addScaledVector(C, 2 * (1 - u) * u).addScaledVector(B, u * u);
      return true;
    }
    if (T < e.leaveGateway) {
      const u = (T - e.atGateway) / (e.leaveGateway - e.atGateway);
      out.copy(gateway.ports.in).lerp(gateway.ports.out, u);
      return true;
    }
    if (T < e.atLog) {
      const u = (T - e.leaveGateway) / (e.atLog - e.leaveGateway);
      const p = arcPoint(aGate + PORT, aLog - PORT, easeInOut(u), 0.8);
      out.copy(p); out.y = 1.8 + Math.sin(Math.PI * u) * 0.5;
      return true;
    }
    const u = smooth((T - e.atLog) / (e.landed - e.atLog));
    A.copy(log.ports.in); B.copy(log.slotWorld(e.i));
    out.copy(A).lerp(B, u); out.y += Math.sin(Math.PI * u) * 0.8;
    return true;
  }

  const p = new THREE.Vector3();
  function update(T) {
    events.forEach((e, i) => {
      const live = position(e, T, p);
      if (live) {
        const s = 1;
        o.position.copy(p); o.scale.setScalar(s); o.rotation.set(T * 2 + i, T * 1.4, 0);
        glows[i].position.copy(p); glows[i].material.opacity = 0.32;
      } else { o.position.set(0, -50, 0); o.scale.setScalar(0.0001); glows[i].material.opacity = 0; }
      o.updateMatrix();
      mesh.setMatrixAt(i, o.matrix);
    });
    mesh.instanceMatrix.needsUpdate = true;
  }
  return { group, update };
}
