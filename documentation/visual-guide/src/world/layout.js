import * as THREE from 'three';
import { RING_ORDER } from '../story.js';

export const R = 44;
export const HALF = 7.4;
export const STEP = (Math.PI * 2) / RING_ORDER.length;
export const TAMOZ_LOCAL = new THREE.Vector3(-13, 0, -21);

export const angleOf = (id) => RING_ORDER.indexOf(id) * STEP;
export const ringPoint = (a, y = 0, r = R) => new THREE.Vector3(r * Math.sin(a), y, r * Math.cos(a));
export const outward = (a) => new THREE.Vector3(Math.sin(a), 0, Math.cos(a));
export const tangent = (a) => new THREE.Vector3(Math.cos(a), 0, -Math.sin(a));
export const half = HALF / R;
export const PORT = 5.2 / R;

export function stationPoint(id, local = new THREE.Vector3()) {
  if (id === 'tamoz') return localToWorld(angleOf('episode'), R, TAMOZ_LOCAL.clone().add(local));
  if (id === 'store') return new THREE.Vector3(0, 0, 0).add(local);
  return localToWorld(angleOf(id), R, local);
}

export function localToWorld(a, r, local) {
  const o = outward(a), t = tangent(a);
  return ringPoint(a, 0, r).addScaledVector(t, local.x).addScaledVector(new THREE.Vector3(0, 1, 0), local.y).addScaledVector(o, local.z);
}

export function placeStation(group, id) {
  const a = id === 'tamoz' ? angleOf('episode') : angleOf(id);
  group.position.copy(id === 'tamoz' ? localToWorld(a, R, TAMOZ_LOCAL) : ringPoint(a, 0, R));
  group.rotation.y = a;
  return group;
}

export function arcPoint(a0, a1, u, lift = 1.6, r = R) {
  const a = a0 + (a1 - a0) * u;
  return ringPoint(a, lift * Math.sin(Math.PI * u) + 0.9, r);
}
