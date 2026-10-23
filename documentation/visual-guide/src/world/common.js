import * as THREE from 'three';
import { COLORS, mat, roundedSlab, glowSprite, box } from './theme.js';
import { placeStation } from './layout.js';

export function platform(planeKey, w = 14, d = 15, { glow = 0.5 } = {}) {
  const g = new THREE.Group();
  const c = COLORS.plane[planeKey];
  const base = new THREE.Mesh(roundedSlab(w, d, 0.5, 0.9), mat.body(0x0e1730, { roughness: 0.7, metalness: 0.35 }));
  g.add(base);
  const top = new THREE.Mesh(roundedSlab(w - 0.8, d - 0.8, 0.06, 0.7), mat.body(0x16213f, { roughness: 0.5, metalness: 0.3 }));
  top.position.y = 0.5;
  g.add(top);
  const rimMat = new THREE.MeshBasicMaterial({ color: c.clone().multiplyScalar(1.25) });
  const t = 0.09;
  g.add(box(w - 1.8, t, t, rimMat, 0, 0.55, d / 2 - 0.35), box(w - 1.8, t, t, rimMat, 0, 0.55, -d / 2 + 0.35));
  g.add(box(t, t, d - 1.8, rimMat, w / 2 - 0.35, 0.55, 0), box(t, t, d - 1.8, rimMat, -w / 2 + 0.35, 0.55, 0));
  const halo = new THREE.Mesh(new THREE.PlaneGeometry(w * 1.7, d * 1.7), mat.add(c, 0.0));
  halo.material.map = haloTexture();
  halo.material.opacity = glow * 0.2;
  halo.rotation.x = -Math.PI / 2;
  halo.position.y = 0.02;
  g.add(halo);
  g.userData.rimMat = rimMat;
  g.userData.halo = halo;
  g.userData.planeColor = c;
  return g;
}

let haloTex;
function haloTexture() {
  if (haloTex) return haloTex;
  const c = document.createElement('canvas');
  c.width = c.height = 256;
  const g = c.getContext('2d');
  const r = g.createRadialGradient(128, 128, 10, 128, 128, 128);
  r.addColorStop(0, 'rgba(255,255,255,0.55)');
  r.addColorStop(0.55, 'rgba(255,255,255,0.12)');
  r.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = r; g.fillRect(0, 0, 256, 256);
  haloTex = new THREE.CanvasTexture(c);
  haloTex.colorSpace = THREE.SRGBColorSpace;
  return haloTex;
}

export function stationGroup(id, planeKey, w, d) {
  const g = new THREE.Group();
  g.userData.station = id;
  placeStation(g, id);
  const p = platform(planeKey, w, d);
  g.add(p);
  g.userData.platform = p;
  const hit = new THREE.Mesh(new THREE.BoxGeometry(w, 9, d), new THREE.MeshBasicMaterial({ visible: false }));
  hit.position.y = 4.5;
  hit.userData.station = id;
  g.add(hit);
  g.userData.hit = hit;
  return g;
}

export function setPlatformEnergy(group, e) {
  e = Math.min(1.2, e + (group.userData.boost ?? 0));
  const p = group.userData.platform;
  p.userData.rimMat.color.copy(p.userData.planeColor).multiplyScalar(0.9 + e * 1.1);
  p.userData.halo.material.opacity = 0.06 + e * 0.22;
}

export function tube(points, radius, material, seg = 48) {
  return new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(points), seg, radius, 8), material);
}

export function dot(color, r = 0.2, intensity = 2) {
  return new THREE.Mesh(new THREE.SphereGeometry(r, 16, 12), mat.glow(color, intensity));
}

export { glowSprite };
