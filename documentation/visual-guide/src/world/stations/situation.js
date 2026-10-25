import * as THREE from 'three';
import { COLORS, mat, box, edges, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, lerp } from '../../util.js';
import { RUN, COUNTS } from '../../story.js';
import { versions, engine, situationBeats as SB } from '../schedule.js';
import { ch } from '../../timeline.js';
import { arcPoint, angleOf, PORT } from '../layout.js';

const Y = 0.56;
const DY = 0.5;
const GOLD = new THREE.Color(0xffd166);
const PH = (phase) => { const vs = versions.filter((v) => v.phase === phase); return { a: vs[0].v, b: vs.at(-1).v, n: vs.length, mid: (vs[0].v + vs.at(-1).v) / 2 - 1 }; };
const CAND = PH('candidate'), WATCH = PH('watch'), COOL = PH('cooling');

export function createSituation(engineStation) {
  const g = stationGroup('situation', 'stream', 14, 14);
  const slabGeo = new THREE.BoxGeometry(5.4, 0.38, 3.6);
  const slabs = versions.map((v) => {
    const color = COLORS.phase[v.phase];
    const m = new THREE.MeshStandardMaterial({ color, emissive: color, emissiveIntensity: 0.55, roughness: 0.35, metalness: 0.2, transparent: true, opacity: 1 });
    const mesh = new THREE.Mesh(slabGeo, m);
    const line = new THREE.LineSegments(new THREE.EdgesGeometry(slabGeo), mat.line(color.clone().multiplyScalar(1.5), 1));
    const grp = new THREE.Group();
    grp.add(mesh, line);
    g.add(grp);
    return { v, grp, mesh, line, color };
  });
  const tokenGeo = new THREE.OctahedronGeometry(0.46, 0);
  const tokens = versions.map((v) => {
    const t = new THREE.Mesh(tokenGeo, mat.glow(COLORS.phase[v.phase].clone().lerp(COLORS.plane.stream, 0.4), 1.8));
    g.parent;
    return t;
  });
  const tokenGroup = new THREE.Group();
  tokens.forEach((t) => tokenGroup.add(t));

  const cursor = new THREE.Mesh(new THREE.ConeGeometry(0.4, 1.3, 16), mat.glow(0xff5470, 1.8));
  cursor.rotation.x = Math.PI;
  g.add(cursor);
  const rejectFlash = glowSprite(0xff5470, 6, 0);
  g.add(rejectFlash);

  g.updateMatrixWorld(true);
  const slabPos = (i) => new THREE.Vector3(0, Y + 0.3 + i * DY, 0);
  const ports = { top: g.localToWorld(new THREE.Vector3(0, Y + 12.5, 0)), in: g.localToWorld(new THREE.Vector3(-3, Y + 4.5, 0)), out: g.localToWorld(new THREE.Vector3(5.5, Y + 4.5, 0)) };
  const src = engineStation.ports.feature;
  const aEng = angleOf('engine');
  const aSit = angleOf('situation');
  const editAt = SB.edit;
  const sealStart = SB.seal;
  const focusOf = (T, phase) => (phase === 'candidate' ? window01(T, SB.candidate - 0.2, SB.watch, 0.3) : phase === 'watch' ? window01(T, SB.watch - 0.1, SB.cooling, 0.3) : window01(T, SB.cooling - 0.1, SB.pairs, 0.3));

  function update(T, tags) {
    tokens.forEach((t, i) => {
      const v = versions[i];
      const u = (T - v.emit) / (v.land - v.emit);
      if (u <= 0 || u >= 1) { t.scale.setScalar(0.0001); return; }
      const dest = g.localToWorld(slabPos(i).add(new THREE.Vector3(0, 2.4, 0)));
      const k = smooth(u);
      t.position.copy(src).lerp(dest, k);
      t.position.y += Math.sin(Math.PI * k) * 5;
      t.scale.setScalar(1);
      t.rotation.set(T * 3, T * 2, 0);
    });
    let count = 0;
    slabs.forEach((s, i) => {
      const v = s.v;
      const u = clamp((T - v.land) / 0.45);
      const visible = u > 0;
      if (visible) count++;
      const drop = (1 - easeOut(u)) * 2.2;
      s.grp.position.copy(slabPos(i)).add(new THREE.Vector3(0, drop, 0));
      s.grp.visible = visible;
      const sealed = ramp(T, sealStart + i * 0.06, sealStart + i * 0.06 + 0.3);
      const prov = v.completeness === 'provisional';
      const pairBeat = window01(T, SB.pairs, SB.seal, 0.4) * (prov ? 1 : 0);
      s.mesh.material.opacity = (prov ? 0.55 - pairBeat * 0.35 : 0.96) * clamp(u * 2);
      s.grp.scale.set(1 - pairBeat * 0.1, 1, 1 - pairBeat * 0.1);
      s.mesh.material.emissiveIntensity = 0.45 + decay(T, v.land, 0.5) * 1.2 + focusOf(T, v.phase) * 1.3 - pairBeat * 0.3;
      s.line.material.color.copy(s.color).multiplyScalar(1.4).lerp(GOLD, sealed * 0.9);
      s.line.material.opacity = prov ? 0.7 : 1;
    });

    const latest = versions.filter((v) => T >= v.land).at(-1);
    const act = window01(T, engine.features - 1, ch.situation.end, 1.2);
    setPlatformEnergy(g, act * 0.9);

    const e = clamp((T - editAt) / 2.2);
    const hit = T > editAt + 0.9 ? decay(T, editAt + 0.9, 0.5) : 0;
    const target = g.localToWorld(slabPos(2).add(new THREE.Vector3(0, 0.2, 2.1)));
    cursor.position.copy(slabPos(2)).add(new THREE.Vector3(0.2, 0, 3.6 + (1 - Math.min(1, e * 1.4)) * 4 - Math.sin(Math.max(0, (T - editAt - 0.9)) * 6) * 0.3 * (T > editAt + 0.9 ? 1 : 0)));
    cursor.position.y += 1.4;
    cursor.scale.setScalar(T >= editAt && T < editAt + 3.4 ? 1 : 0.0001);
    rejectFlash.position.copy(slabPos(2)).add(new THREE.Vector3(0, 0.3, 2.2));
    rejectFlash.material.opacity = hit * 0.9;

    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    const near = window01(T, ch.situation.start, ch.situation.end - 0.4, 1.0);
    tags.put('sit-name', P(0, Y + 14.2, 0), `Situation<small>${RUN.situation.type} · ${RUN.situation.entity.id}</small>`, near * 0.95, 'name stream');
    tags.put('sit-latest', P(3.4, Y + 0.3 + (Math.max(count, 1) - 1) * DY + 0.9, 1.8), latest ? `<b>v${latest.v}</b> · ${latest.phase}<small>${latest.completeness.replace('_', '-')} · severity ${latest.severity}</small>` : '', window01(T, engine.features, ch.situation.end, 0.5) * (latest ? 0.95 : 0), 'note');
    const at = (i) => Y + 0.3 + i * DY;
    const lit = (on) => 0.6 + 0.35 * on;
    tags.put('sit-cand', P(-4.4, at(CAND.mid), 1.8), `candidate<small>v${CAND.a}–${CAND.b} · opened, not yet held</small>`, window01(T, SB.candidate - 0.3, ch.situation.end - 0.3, 0.5) * lit(focusOf(T, 'candidate')), 'note phase-candidate');
    tags.put('sit-watch', P(-4.4, at(WATCH.mid), 1.8), `watch<small>v${WATCH.a}–${WATCH.b} · held for a minute</small>`, window01(T, SB.watch - 0.2, ch.situation.end - 0.3, 0.5) * lit(focusOf(T, 'watch')), 'note phase-watch');
    tags.put('sit-cool', P(-4.4, at(COOL.mid), 1.8), `cooling<small>v${COOL.a}–${COOL.b} · cooling is needed</small>`, window01(T, SB.cooling - 0.2, ch.situation.end - 0.3, 0.5) * lit(focusOf(T, 'cooling')), 'note phase-cooling');
    tags.put('sit-hyst', P(4.8, Y + 5.6, 0), 'a phase moves only when its rule<br>has held for a minimum time<small>enter and leave rules differ (hysteresis)</small>', window01(T, SB.rule, SB.pairs, 0.5) * 0.95, 'note');
    tags.put('sit-pairs', P(4.6, Y + 7.2, 0), 'each window step, twice<small>provisional first (faint), then on-time (solid)</small>', window01(T, SB.pairs - 0.1, SB.seal + 0.2, 0.4) * 0.95, 'note');
    tags.put('sit-edit', P(1.2, Y + 2.4, 3.8), 'edit refused<small>a published version never changes</small>', window01(T, editAt + 0.9, ch.situation.end - 0.1, 0.3) * 0.97, 'note warn');
  }
  return { id: 'situation', group: g, ports, tokens: tokenGroup, update };
}
