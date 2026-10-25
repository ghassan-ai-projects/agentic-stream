import * as THREE from 'three';
import { RING_ORDER, PLANES } from '../story.js';
import { COLORS } from './theme.js';
import { R, STEP, angleOf, half, ringPoint } from './layout.js';

const railShader = (color) => new THREE.ShaderMaterial({
  transparent: true, depthWrite: false, blending: THREE.AdditiveBlending,
  uniforms: { uT: { value: 0 }, uColor: { value: color }, uAmp: { value: 1 } },
  vertexShader: `varying vec2 vUv; void main(){ vUv = uv; gl_Position = projectionMatrix*modelViewMatrix*vec4(position,1.0); }`,
  fragmentShader: `varying vec2 vUv; uniform float uT; uniform vec3 uColor; uniform float uAmp;
    void main(){ float b = smoothstep(0.55,0.95,fract(vUv.x*34.0 - uT*0.9)); float edge = smoothstep(0.5,0.0,abs(vUv.y-0.5));
      gl_FragColor = vec4(uColor*(0.35+1.4*b)*uAmp, (0.30+0.55*b)*edge*uAmp); }`,
});

export function createRails() {
  const group = new THREE.Group();
  const mats = [];
  const arc = (a0, a1, y, radius, color, tube = 0.16) => {
    const pts = [];
    for (let k = 0; k <= 40; k++) {
      const u = k / 40;
      pts.push(ringPoint(a0 + (a1 - a0) * u, y + (y > 1 ? Math.sin(Math.PI * u) * 2.2 : 0), radius));
    }
    const m = railShader(color);
    mats.push(m);
    group.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 80, tube, 8), m));
  };
  for (let i = 0; i < RING_ORDER.length - 1; i++) {
    const from = RING_ORDER[i];
    const planeKey = ['bench', 'gateway', 'log'].includes(from) ? 'evidence' : null;
    const color = COLORS.plane[planeKey ?? colorOf(from)];
    arc(angleOf(from) + half, angleOf(RING_ORDER[i + 1]) - half, 0.28, R, color);
  }
  arc(angleOf('action') + half, angleOf('gateway') - half + Math.PI * 2, 0.28, R + 7.5, COLORS.plane.effect, 0.1);
  return { group, update(T) { mats.forEach((m) => { m.uniforms.uT.value = T; }); } };
}

function colorOf(id) {
  const planes = { engine: 'stream', situation: 'stream', scheduler: 'cognition', episode: 'cognition', validator: 'governance', policy: 'governance', action: 'effect' };
  return planes[id] ?? 'evidence';
}
