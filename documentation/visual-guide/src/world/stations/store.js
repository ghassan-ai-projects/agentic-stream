import * as THREE from 'three';
import { COLORS, mat, box, cyl, glowSprite } from '../theme.js';
import { clamp, window01, smooth, easeOut } from '../../util.js';
import { RUN } from '../../story.js';
import { events, versions, scheduler, episode, validator, policy, action } from '../schedule.js';
import { ch } from '../../timeline.js';
import { RING_ORDER } from '../../story.js';
import { stationPoint } from '../layout.js';

const writes = [
  ...events.filter((e, i) => i % 4 === 0).map((e) => ({ st: 'log', t: e.landed, c: 0x4cc9f0 })),
  ...versions.map((v) => ({ st: 'situation', t: v.land, c: 0x9b8cff })),
  ...scheduler.items.filter((it) => it.verdict === 'queue').map((it) => ({ st: 'scheduler', t: it.decided, c: 0xf5b94a })),
  { st: 'episode', t: episode.admit, c: 0xf5b94a }, { st: 'episode', t: episode.decision, c: 0xf5b94a },
  { st: 'validator', t: validator.pass, c: 0x34d399 }, { st: 'policy', t: policy.approved, c: 0x34d399 },
  { st: 'action', t: action.stamp, c: 0xfb7185 }, { st: 'action', t: action.outbox, c: 0xfb7185 },
  { st: 'action', t: action.receipt, c: 0xfb7185 }, { st: 'action', t: action.result, c: 0xfb7185 }, { st: 'action', t: action.verified, c: 0xffd166 },
];

export function createStore() {
  const g = new THREE.Group();
  const stack = [0, 1, 2].map((i) => {
    const d = cyl(5.2, 5.2, 0.55, mat.body(0x0e1a38, { metalness: 0.5, roughness: 0.4 }), 0, 0.45 + i * 0.95, 0, 56);
    g.add(d);
    const rim = new THREE.Mesh(new THREE.TorusGeometry(5.2, 0.05, 8, 80), new THREE.MeshBasicMaterial({ color: COLORS.plane.stream.clone().multiplyScalar(1.4) }));
    rim.rotation.x = Math.PI / 2;
    rim.position.set(0, 0.75 + i * 0.95, 0);
    g.add(rim);
    return rim;
  });
  const beam = cyl(0.12, 0.12, 14, mat.add(0x9b8cff, 0.5), 0, 8.5, 0, 12);
  g.add(beam);
  const halo = glowSprite(0x9b8cff, 24, 0.22);
  halo.position.y = 2;
  g.add(halo);

  const lineMat = new THREE.LineBasicMaterial({ color: 0x7f8cff, transparent: true, opacity: 0.12 });
  const lines = RING_ORDER.map((id) => {
    const p = stationPoint(id);
    const geo = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(p.x * 0.88, 0.2, p.z * 0.88), new THREE.Vector3(0, 0.2, 0)]);
    const l = new THREE.Line(geo, lineMat);
    g.add(l);
    return l;
  });
  const pulses = writes.map((w) => { const s = glowSprite(w.c, 1.8, 0); g.add(s); return s; });
  const starts = writes.map((w) => { const p = stationPoint(w.st); return new THREE.Vector3(p.x * 0.86, 0.5, p.z * 0.86); });

  function update(T, tags) {
    writes.forEach((w, i) => {
      const u = clamp((T - w.t) / 1.5);
      const s = pulses[i];
      if (u > 0 && u < 1) {
        s.position.copy(starts[i]).lerp(new THREE.Vector3(0, 1.6, 0), smooth(u)); s.material.opacity = 0.85 * (1 - u * u);
      } else s.material.opacity = 0;
    });
    const showAll = window01(T, 0, ch.intro.end, 1.0) + window01(T, ch.outro.start, 1e9, 1.0);
    lineMat.opacity = 0.10 + Math.min(1, showAll) * 0.28;
    beam.material.opacity = 0.18 + Math.min(1, showAll) * 0.4;
    const ev = window01(T, 0.0, ch.intro.end, 1.0) + window01(T, ch.outro.start + 0.5, 1e9, 1.0);
    tags.put('store-name', new THREE.Vector3(0, 0.2, 8.5), `Durable store<small>${Object.values(RUN.run.rows).reduce((a, b) => a + b, 0)} ledger rows in this run · one SQLite file</small>`, Math.min(1, ev) * 0.92, 'name mini stream');
  }
  return { group: g, update };
}
