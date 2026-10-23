import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, window01, pulse, ramp, rng } from '../../util.js';
import { episode } from '../schedule.js';
import { ch } from '../../timeline.js';

const Y = 0.56;

export function createTamoz() {
  const g = stationGroup('tamoz', 'cognition', 14, 12);
  const amber = COLORS.plane.cognition;
  const knot = new THREE.Group();
  const ico = new THREE.IcosahedronGeometry(2.4, 1);
  knot.add(new THREE.LineSegments(new THREE.WireframeGeometry(ico), mat.line(amber, 0.55)));
  const pos = ico.attributes.position;
  const nodes = [];
  const seen = new Set();
  for (let i = 0; i < pos.count; i++) {
    const key = [pos.getX(i), pos.getY(i), pos.getZ(i)].map((n) => n.toFixed(2)).join();
    if (seen.has(key)) continue;
    seen.add(key);
    const n = new THREE.Mesh(new THREE.SphereGeometry(0.16, 12, 10), mat.glow(0xffd166, 1.2));
    n.position.set(pos.getX(i), pos.getY(i), pos.getZ(i));
    knot.add(n);
    nodes.push(n);
  }
  const core = new THREE.Mesh(new THREE.SphereGeometry(0.95, 24, 20), mat.glow(0xffb84d, 1.0, { transparent: true, opacity: 0.9 }));
  knot.add(core);
  const halo = glowSprite(0xffb84d, 9, 0.3);
  knot.add(halo);
  knot.position.set(0, Y + 3.8, 0);
  g.add(knot);

  const cageMat = new THREE.LineDashedMaterial({ color: 0xf5b94a, dashSize: 0.5, gapSize: 0.35, transparent: true, opacity: 0.8 });
  const cage = new THREE.LineSegments(new THREE.EdgesGeometry(new THREE.BoxGeometry(12.2, 8.8, 10.2)), cageMat);
  cage.computeLineDistances();
  cage.position.y = Y + 4.4;
  g.add(cage);
  const rand = rng(11);
  const flashers = nodes.map(() => rand() * 10);

  g.updateMatrixWorld(true);
  function update(T, tags) {
    const think = ramp(T, episode.thinkStart, episode.thinkStart + 0.6) * (1 - ramp(T, episode.thinkEnd, episode.thinkEnd + 0.6));
    knot.rotation.y = T * 0.25;
    knot.rotation.x = Math.sin(T * 0.3) * 0.2;
    nodes.forEach((n, i) => { n.material.emissiveIntensity = 0.5 + think * (1.6 + Math.sin(T * 6 + flashers[i]) * 1.2); });
    core.material.emissiveIntensity = 0.5 + think * 2.2;
    core.scale.setScalar(1 + think * 0.12 * Math.sin(T * 9));
    halo.material.opacity = 0.18 + think * 0.5;
    const act = window01(T, episode.request - 0.5, episode.answer + 1.5, 0.8);
    setPlatformEnergy(g, 0.25 + act * 0.8);
    const near = window01(T, episode.request - 0.6, ch.episode.end - 0.4, 0.8);
    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    tags.put('tamoz-name', P(0, Y + 10.2, 0), 'Tamoz · the reasoner<small>separate worker process · private socket</small>', near * 0.95, 'name cognition');
    tags.put('tamoz-lock', P(0, Y + 0.5, 5.6), 'no effectors · no credentials<small>reads evidence, proposes, nothing else</small>', window01(T, episode.evidence[0] + 0.6, ch.episode.end - 0.2, 0.6) * 0.95, 'note warn');
  }
  return { id: 'tamoz', group: g, update };
}
