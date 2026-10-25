import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite, textPlane } from '../theme.js';
import { stationGroup, setPlatformEnergy, tube } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, easeInOut, lerp } from '../../util.js';
import { RUN } from '../../story.js';
import { episode, scheduler } from '../schedule.js';
import { ch } from '../../timeline.js';
import { angleOf, ringPoint, PORT, arcPoint } from '../layout.js';

const Y = 0.56;
const RW = 7.2, RH = 5.2, RD = 7.2;
const BRIDGE_END = -19.5;

export function createEpisode(sched) {
  const g = stationGroup('episode', 'cognition', 17, 15);
  const amber = COLORS.plane.cognition;
  const room = new THREE.Group();
  room.position.set(0, Y, 0);
  g.add(room);
  const glass = new THREE.Mesh(new THREE.BoxGeometry(RW, RH, RD), mat.glass(amber, 0.08));
  glass.position.y = RH / 2;
  room.add(glass);
  const frameMat = new THREE.MeshBasicMaterial({ color: amber.clone().multiplyScalar(1.6) });
  const bar = (w, h, d, x, y, z) => room.add(box(w, h, d, frameMat, x, y, z));
  for (const sx of [-1, 1]) for (const sz of [-1, 1]) bar(0.14, RH, 0.14, sx * RW / 2, RH / 2, sz * RD / 2);
  for (const sy of [0.07, RH]) { for (const sz of [-1, 1]) bar(RW, 0.14, 0.14, 0, sy, sz * RD / 2); for (const sx of [-1, 1]) bar(0.14, 0.14, RD, sx * RW / 2, sy, 0); }
  const door = new THREE.Mesh(new THREE.PlaneGeometry(RD - 0.4, RH - 0.4), mat.add(amber, 0.28));
  door.rotation.y = Math.PI / 2;
  door.position.set(RW / 2 + 0.02, RH / 2, 0);
  room.add(door);
  const lockBar = box(0.2, RH - 0.5, 0.3, frameMat, RW / 2 + 0.05, RH / 2, 0);
  room.add(lockBar);

  const snapCol = new THREE.Color();
  const snap = new THREE.Mesh(new THREE.BoxGeometry(1.9, 1.9, 1.9), new THREE.MeshStandardMaterial({ color: 0xff7a59, emissive: 0xff7a59, emissiveIntensity: 0.8, transparent: true, opacity: 0.85, roughness: 0.3 }));
  const snapEdges = new THREE.LineSegments(new THREE.EdgesGeometry(snap.geometry), mat.line(0xffd0c2, 1));
  const snapGrp = new THREE.Group();
  snapGrp.add(snap, snapEdges);
  snapGrp.position.set(0, 2.2, 0);
  room.add(snapGrp);
  const snapGlow = glowSprite(0xff7a59, 6, 0.0);
  snapGlow.position.copy(snapGrp.position);
  room.add(snapGlow);
  const floorRing = new THREE.Mesh(new THREE.RingGeometry(2.2, 2.4, 48), mat.add(amber, 0.5));
  floorRing.rotation.x = -Math.PI / 2;
  floorRing.position.y = 0.05;
  room.add(floorRing);

  const gauges = [0, 1, 2, 3].map((i) => {
    const gx = -2.55 + i * 1.7;
    const base = new THREE.Mesh(new THREE.RingGeometry(0.5, 0.66, 40), mat.add(amber, 0.18));
    base.position.set(gx, 3.8, -RD / 2 + 0.1);
    const fill = new THREE.Mesh(new THREE.RingGeometry(0.5, 0.66, 40, 1, Math.PI / 2, -Math.PI * 2 * 0.001), mat.add(amber, 0.95));
    fill.position.copy(base.position);
    room.add(base, fill);
    return fill;
  });
  const evidencePorts = [-1.4, 1.4].map((z) => {
    const p = cyl(0.28, 0.28, 0.9, mat.body(0x4cc9f0, { metalness: 0.5 }), -RW / 2 - 0.2, 1.4, z, 16);
    p.rotation.z = Math.PI / 2;
    room.add(p);
    return p;
  });

  const bridgeMat = new THREE.ShaderMaterial({
    transparent: true, depthWrite: false, blending: THREE.AdditiveBlending,
    uniforms: { uT: { value: 0 }, uColor: { value: amber } },
    vertexShader: `varying vec2 vUv; void main(){ vUv = uv; gl_Position = projectionMatrix*modelViewMatrix*vec4(position,1.0); }`,
    fragmentShader: `varying vec2 vUv; uniform float uT; uniform vec3 uColor; void main(){ float b = smoothstep(0.6,0.95,fract(vUv.x*22.0 - uT*0.6)); gl_FragColor = vec4(uColor*(0.4+1.0*b), 0.35+0.4*b); }`,
  });
  const bridgePts = [new THREE.Vector3(0, Y + 2.4, -RD / 2), new THREE.Vector3(-3.4, Y + 3.8, -11), new THREE.Vector3(-6.6, Y + 2.8, BRIDGE_END)];
  g.add(tube(bridgePts, 0.12, bridgeMat, 40));
  const bridgeCurve = new THREE.CatmullRomCurve3(bridgePts);

  const reqPulse = glowSprite(0xffd166, 3.2, 0);
  const tool = [glowSprite(0x4cc9f0, 2.4, 0), glowSprite(0x4cc9f0, 2.4, 0)];
  const answer = glowSprite(0xffd166, 3.2, 0);
  g.add(reqPulse, answer, ...tool);

  const card = textPlane([
    { text: 'Decision', size: 40, color: '#f5b94a', weight: 800 },
    { text: `${RUN.decision.hypothesis}`, size: 28, color: '#e6edf7' },
    { text: `confidence ${RUN.decision.confidence}  ·  snapshot v${RUN.episode.snapshotVersion}`, size: 26, color: '#93a3c0' },
    { text: `intent  ${RUN.intent.type}`, size: 28, color: '#ffffff', weight: 700 },
    { text: `{ state: "${RUN.intent.parameters.state}" }   ${RUN.intent.risk}`, size: 28, color: '#34d399', weight: 700 },
  ], { w: 640, h: 330, scale: 0.0078, border: '#f5b94a' });
  card.position.set(0, Y + 3, 0);
  g.add(card);

  const token = new THREE.Mesh(new THREE.BoxGeometry(0.86, 0.22, 0.86), mat.glow(COLORS.phase.cooling, 1.4));
  g.add(token);

  g.updateMatrixWorld(true);
  const ports = { in: g.localToWorld(new THREE.Vector3(-8.2, Y + 1.6, 0)), out: g.localToWorld(new THREE.Vector3(8.2, Y + 2.4, 0)), bridge: g.localToWorld(new THREE.Vector3(-6.6, Y + 2.8, BRIDGE_END)) };
  const o = new THREE.Object3D();
  const gaugeLevels = [0.34, 0.2, 0.28, 0.15];
  const arcGeo = (p) => new THREE.RingGeometry(0.5, 0.66, 40, 1, Math.PI / 2, -Math.PI * 2 * Math.max(p, 0.001));
  const aSch = angleOf('scheduler');
  const aEp = angleOf('episode');

  function update(T, tags) {
    bridgeMat.uniforms.uT.value = T;
    const vis = ramp(T, episode.roomIn - 1.5, episode.roomIn + 0.8);
    room.visible = true;
    glass.material.opacity = 0.05 + vis * 0.08;
    frameMat.color.copy(amber).multiplyScalar(0.8 + vis * 1.0 + pulse(T, episode.attempt, 0.4) * 1.2);
    const open = ramp(T, episode.door, episode.door + 0.8);
    door.material.opacity = 0.28 * (1 - open) + 0.03;
    lockBar.scale.y = 1 - open;
    lockBar.visible = open < 0.98;

    const rebound = T >= episode.rebound;
    const rb = ramp(T, episode.rebind, episode.rebound);
    snapCol.copy(COLORS.phase.cooling);
    snap.material.emissiveIntensity = 0.6 + pulse(T, episode.rebind + 0.6, 0.35) * 1.8 + (rebound ? 0.3 : 0);
    snap.material.opacity = 0.85 * ramp(T, episode.t - 1.0, episode.t + 0.8);
    snapEdges.material.opacity = ramp(T, episode.t - 1.0, episode.t + 0.8);
    snapGlow.material.opacity = (0.3 + pulse(T, episode.rebind + 0.6, 0.4) * 0.7) * ramp(T, episode.t - 1.0, episode.t + 0.8);
    snapGrp.rotation.y = T * 0.5;
    snapGrp.position.y = 2.2 + Math.sin(T * 1.3) * 0.12;
    const a = ramp(T, episode.attempt, episode.attempt + 2.0);
    gauges.forEach((fill, i) => { fill.geometry.dispose(); fill.geometry = arcGeo(a * gaugeLevels[i] * (1 + 0.25 * Math.sin(T * 0.8 + i))); fill.material.opacity = 0.95 * vis; });

    const bp = (u) => bridgeCurve.getPoint(clamp(u));
    const rq = clamp((T - episode.request) / 1.4);
    reqPulse.position.copy(bp(rq)); reqPulse.material.opacity = rq > 0 && rq < 1 ? 0.95 : 0;
    episode.evidence.forEach((te, k) => {
      const u = clamp((T - te) / 1.6);
      const out = u < 0.5 ? u * 2 : 2 - u * 2;
      tool[k].position.copy(bp(1 - out)).add(new THREE.Vector3(0, 0, 0)); tool[k].material.opacity = u > 0 && u < 1 ? 0.9 : 0;
    });
    const an = clamp((T - episode.answer) / 1.2);
    answer.position.copy(bp(1 - an)); answer.material.opacity = an > 0 && an < 1 ? 0.95 : 0;

    const cardIn = ramp(T, episode.answer + 1.0, episode.decision + 0.3);
    const cardOut = clamp((T - episode.leave) / 1.4);
    card.position.set(lerp(0, 8.2, easeInOut(cardOut)), Y + 2.6 + (cardOut > 0 ? Math.sin(Math.PI * cardOut) * 0.6 : 0), lerp(0, 0.5, cardOut));
    card.rotation.y = Math.PI / 2 * 0 + 0;
    card.scale.setScalar(Math.max(0.0001, cardIn) * 0.78 * (cardOut >= 1 ? 0.0001 : 1));
    card.material.opacity = cardIn;

    const flight = clamp((T - scheduler.release) / (scheduler.arrive - scheduler.release));
    const flightPos = (() => {
      if (flight <= 0) return null;
      if (flight >= 1) return flight >= 1 && T < episode.rebind + 0.05 ? new THREE.Vector3(0, 0, 0) : null;
      const p = arcPoint(aSch + PORT * 1.3, aEp - PORT * 1.1, easeInOut(flight), 1.2);
      return g.worldToLocal(p.clone());
    })();
    if (flightPos) { token.position.copy(flightPos); token.scale.setScalar(1); token.visible = true; token.position.y = Math.max(Y + 0.9, flightPos.y); }
    else if (T >= scheduler.arrive && T < episode.rebind + 0.5) { token.position.set(lerp(-8.2, 0, easeInOut((T - scheduler.arrive) / (episode.rebind - scheduler.arrive + 0.3))), Y + 2.2, 0); token.visible = true; }
    else token.visible = false;

    const act = window01(T, scheduler.release, ch.episode.end, 1.0);
    setPlatformEnergy(g, act * 0.9);

    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    const near = window01(T, ch.episode.start, episode.decision - 0.3, 0.8);
    tags.put('ep-name', P(0, Y + RH + 3.4, 5.0), 'Episode<small>one bounded reasoning session</small>', near * 0.95, 'name cognition');
    tags.put('ep-admit', P(-5.2, Y + 6.4, 2.0), `admitted for v${RUN.scheduler.at(-1).v}<small>one episode, one trigger</small>`, window01(T, episode.admit - 0.3, episode.rebind, 0.4) * 0.95, 'note cognition');
    tags.put('ep-snap', P(0, Y + 5.2, 0), rebound ? `snapshot <b>v${RUN.episode.snapshotVersion}</b><small>immutable · rebound from v${RUN.scheduler.at(-1).v}</small>` : `snapshot <b>v${RUN.scheduler.at(-1).v}</b><small>the version it was admitted on</small>`, window01(T, episode.rebind - 0.6, episode.attempt + 0.4, 0.5) * 0.95, 'note');
    tags.put('ep-rebind', P(0, Y + 7.6, 0), `rebound to the newest snapshot<small>before the attempt starts · same identity and budget</small>`, window01(T, episode.rebind, episode.attempt, 0.4) * 0.95, 'note');
    tags.put('ep-budget', P(-RW / 2 - 1.0, Y + 0.6, RD / 2), 'deadline and budget<small>time · model calls · tokens · tool calls</small>', window01(T, episode.attempt, episode.request + 0.5, 0.5) * 0.95, 'note');
    tags.put('ep-fence', P(RW / 2 + 0.3, Y + 4.6, 2.9), `fence <b>${RUN.episode.fence}</b><small>output from a stale attempt is rejected</small>`, window01(T, episode.fence - 0.2, episode.request + 0.5, 0.5) * 0.95, 'note');
    tags.put('ep-tools', P(-RW / 2 - 1.4, Y + 3.4, 0), 'read-only evidence tools', window01(T, episode.evidence[0] - 0.4, episode.thinkEnd, 0.4) * 0.92, 'note');
    tags.put('ep-dec', P(RW / 2 + 1.5, Y + 6.6, 0), `typed decision returned<small>bound to fence ${RUN.episode.fence} and snapshot v${RUN.episode.snapshotVersion}</small>`, window01(T, episode.decision - 0.2, ch.episode.end + 0.4, 0.4) * 0.95, 'note cognition');
    tags.put('ep-time', P(-3.4, Y + 5.6, -11), `attempt concluded in ${RUN.episode.wallMillis} ms<small>recorded run</small>`, window01(T, episode.answer + 0.2, episode.leave + 1.2, 0.4) * 0.92, 'note');
  }
  return { id: 'episode', group: g, ports, update };
}
