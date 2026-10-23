import * as THREE from 'three';
import { PLANES, EVENT_TYPES } from '../story.js';

export const COLORS = {
  ink: 0x050912,
  floor: 0x0a1226,
  panel: 0x111a30,
  metal: 0x2a3550,
  steel: 0x8b97b3,
  white: 0xf1f5f9,
  danger: 0xff5470,
  gold: 0xffd166,
  ok: 0x34d399,
  plane: Object.fromEntries(Object.entries(PLANES).map(([k, v]) => [k, new THREE.Color(v.color)])),
  type: Object.fromEntries(Object.entries(EVENT_TYPES).map(([k, v]) => [k, new THREE.Color(v.color)])),
  phase: { candidate: new THREE.Color('#6f8cc7'), watch: new THREE.Color('#f5b94a'), cooling: new THREE.Color('#ff7a59') },
};

export const mat = {
  body: (c = COLORS.metal, o = {}) => new THREE.MeshStandardMaterial({ color: c, roughness: 0.55, metalness: 0.3, ...o }),
  matte: (c, o = {}) => new THREE.MeshStandardMaterial({ color: c, roughness: 0.9, metalness: 0.05, ...o }),
  glow: (c, i = 1.4, o = {}) => new THREE.MeshStandardMaterial({ color: c, emissive: c, emissiveIntensity: i, roughness: 0.4, metalness: 0, ...o }),
  glass: (c, op = 0.14, o = {}) => new THREE.MeshStandardMaterial({ color: c, transparent: true, opacity: op, roughness: 0.15, metalness: 0.1, side: THREE.DoubleSide, depthWrite: false, ...o }),
  line: (c, op = 0.9) => new THREE.LineBasicMaterial({ color: c, transparent: true, opacity: op }),
  add: (c, op = 0.5) => new THREE.MeshBasicMaterial({ color: c, transparent: true, opacity: op, blending: THREE.AdditiveBlending, depthWrite: false, side: THREE.DoubleSide }),
};

export function box(w, h, d, material, x = 0, y = 0, z = 0) {
  const m = new THREE.Mesh(new THREE.BoxGeometry(w, h, d), material);
  m.position.set(x, y, z);
  return m;
}

export function cyl(rt, rb, h, material, x = 0, y = 0, z = 0, seg = 32) {
  const m = new THREE.Mesh(new THREE.CylinderGeometry(rt, rb, h, seg), material);
  m.position.set(x, y, z);
  return m;
}

export function edges(mesh, color, op = 0.9) {
  const l = new THREE.LineSegments(new THREE.EdgesGeometry(mesh.geometry, 20), mat.line(color, op));
  l.position.copy(mesh.position);
  l.rotation.copy(mesh.rotation);
  l.scale.copy(mesh.scale);
  return l;
}

export function roundedSlab(w, d, h, r = 0.6) {
  const s = new THREE.Shape();
  const x = -w / 2, y = -d / 2;
  s.moveTo(x + r, y); s.lineTo(x + w - r, y); s.quadraticCurveTo(x + w, y, x + w, y + r);
  s.lineTo(x + w, y + d - r); s.quadraticCurveTo(x + w, y + d, x + w - r, y + d);
  s.lineTo(x + r, y + d); s.quadraticCurveTo(x, y + d, x, y + d - r);
  s.lineTo(x, y + r); s.quadraticCurveTo(x, y, x + r, y);
  const g = new THREE.ExtrudeGeometry(s, { depth: h, bevelEnabled: false, curveSegments: 6 });
  g.rotateX(-Math.PI / 2);
  return g;
}

const spriteCache = new Map();
export function glowTexture() {
  if (spriteCache.has('glow')) return spriteCache.get('glow');
  const c = document.createElement('canvas');
  c.width = c.height = 128;
  const g = c.getContext('2d');
  const r = g.createRadialGradient(64, 64, 0, 64, 64, 64);
  r.addColorStop(0, 'rgba(255,255,255,1)');
  r.addColorStop(0.25, 'rgba(255,255,255,0.45)');
  r.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = r;
  g.fillRect(0, 0, 128, 128);
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  spriteCache.set('glow', t);
  return t;
}

export function glowSprite(color, size = 3, op = 0.8) {
  const s = new THREE.Sprite(new THREE.SpriteMaterial({ map: glowTexture(), color, transparent: true, opacity: op, blending: THREE.AdditiveBlending, depthWrite: false }));
  s.scale.setScalar(size);
  return s;
}

export function textPlane(lines, { w = 512, h = 320, scale = 0.012, bg = 'rgba(10,18,38,0.92)', border = '#4cc9f0', font = 'Inter, ui-sans-serif, system-ui, sans-serif' } = {}) {
  const c = document.createElement('canvas');
  c.width = w; c.height = h;
  const g = c.getContext('2d');
  g.fillStyle = bg;
  g.beginPath(); g.roundRect(4, 4, w - 8, h - 8, 22); g.fill();
  g.lineWidth = 4; g.strokeStyle = border; g.stroke();
  let y = 18;
  for (const line of lines) {
    const size = line.size ?? 30;
    g.font = `${line.weight ?? 600} ${size}px ${font}`;
    g.fillStyle = line.color ?? '#e6edf7';
    g.textBaseline = 'top';
    g.fillText(line.text, 28, y + 8);
    y += size * 1.35 + (line.gap ?? 4);
  }
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  t.anisotropy = 8;
  const m = new THREE.Mesh(new THREE.PlaneGeometry(w * scale, h * scale), new THREE.MeshBasicMaterial({ map: t, transparent: true, side: THREE.DoubleSide, depthWrite: false }));
  return m;
}
