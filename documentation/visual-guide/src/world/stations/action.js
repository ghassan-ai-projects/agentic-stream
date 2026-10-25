import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite, textPlane } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, ramp, window01, decay, pulse, smooth, easeOut, easeInOut, lerp } from '../../util.js';
import { RUN } from '../../story.js';
import { action, policy } from '../schedule.js';
import { ch } from '../../timeline.js';
import { angleOf, PORT, arcPoint, ringPoint, R, STEP } from '../layout.js';

const Y = 0.56;
const GOLD = new THREE.Color(0xffd166);

export function createAction(bench, gateway, log) {
  const g = stationGroup('action', 'effect', 18, 14);
  const coral = COLORS.plane.effect;
  g.add(box(16, 0.16, 2.4, mat.body(0x0c1429), 0, Y + 0.2, 0));

  const outbox = new THREE.Group();
  const tray = box(5.0, 0.5, 2.4, mat.glass(coral, 0.16), 0, 0.25, 0);
  outbox.add(tray, edges(tray, coral, 0.9));
  const slab = box(2.2, 0.3, 1.5, mat.glow(coral, 1.0), 0, 0.5, 0);
  outbox.add(slab);
  outbox.position.set(0, Y, 4.0);
  g.add(outbox);

  const lease = new THREE.Mesh(new THREE.TorusGeometry(1.4, 0.07, 10, 40), new THREE.MeshBasicMaterial({ color: coral.clone().multiplyScalar(1.8) }));
  lease.rotation.y = Math.PI / 2;
  lease.position.set(2.8, Y + 2.1, 0);
  g.add(lease);

  const ledger = [0, 1, 2].map((i) => {
    const colr = [0x5aa9ff, 0xffb84d, 0xffd166][i];
    const m = box(2.6, 0.3, 1.5, mat.glow(colr, 0.15), 6.0, Y + 0.3 + i * 0.42, -4.2);
    g.add(m);
    return m;
  });

  const cmd = new THREE.Group();
  cmd.add(new THREE.Mesh(new THREE.CapsuleGeometry(0.5, 1.7, 8, 20), mat.glow(0xfb7185, 1.1)));
  cmd.children[0].rotation.z = Math.PI / 2;
  const cmdGlow = glowSprite(0xfb7185, 5, 0.55);
  cmd.add(cmdGlow);
  cmd.visible = false;

  const fb = new THREE.Mesh(new THREE.OctahedronGeometry(0.5, 0), mat.glow(0xffd166, 2.2));
  const fbGlow = glowSprite(0xffd166, 3.6, 0);
  fb.visible = false;

  const aAct = angleOf('action');
  const aGate = angleOf('gateway') + Math.PI * 2;
  const aBench = Math.PI * 2;
  const aLog = angleOf('log') + Math.PI * 2;
  const RR = R + 7.5;
  const v = new THREE.Vector3();
  const w = new THREE.Vector3();

  g.updateMatrixWorld(true);
  const start = g.localToWorld(new THREE.Vector3(2.8, Y + 2.1, 0));

  function commandPos(T, out) {
    if (T < action.appear) return false;
    if (T < action.send) {
      const u = easeOut(ramp(T, action.appear, action.appear + 0.9));
      const x = lerp(-8.6, -2.6, u);
      const lift = ramp(T, action.lease, action.send) * 1.0;
      out.copy(g.localToWorld(v.set(T < action.lease ? x : lerp(-2.6, 2.8, easeInOut(ramp(T, action.lease, action.send))), Y + 2.1 + lift * 0.5, 0)));
      return true;
    }
    if (T < action.atGateway) {
      const u = easeInOut((T - action.send) / (action.atGateway - action.send));
      const a = lerp(aAct + PORT, aGate - PORT * 0.4, u);
      out.copy(ringPoint(a, 3.2 + Math.sin(Math.PI * u) * 4.5, lerp(R - 0.0, RR, Math.sin(Math.PI * Math.min(1, u * 1.0)))));
      out.copy(ringPoint(a, 3.0 + Math.sin(Math.PI * u) * 4.6, RR - (1 - Math.sin(Math.PI * u)) * 7.0));
      return true;
    }
    if (T < action.atBench) {
      const u = easeInOut((T - action.atGateway) / (action.atBench - action.atGateway));
      v.copy(gateway.ports.commandIn); w.copy(bench.port.led).add(new THREE.Vector3(0, 0.8, 0));
      out.copy(v).lerp(w, u); out.y += Math.sin(Math.PI * u) * 1.0;
      return true;
    }
    if (T < action.led + 1.2) { out.copy(bench.port.led).add(new THREE.Vector3(0, 0.8, 0)); return true; }
    return false;
  }

  function update(T, tags) {
    const ok = commandPos(T, cmd.position);
    cmd.visible = ok;
    const shrink = T > action.atBench ? 1 - ramp(T, action.led, action.led + 1.2) : 1;
    cmd.scale.setScalar(Math.max(0.0001, shrink));
    cmd.rotation.y = T < action.send ? 0 : -0.2;
    const ob = ramp(T, action.outbox, action.outbox + 0.9);
    slab.position.y = 0.5 + (1 - easeOut(ob)) * 3;
    slab.material.emissiveIntensity = ob > 0 ? 0.6 + decay(T, action.outbox + 0.6, 0.5) * 1.6 : 0;
    slab.visible = ob > 0.001;
    const le = ramp(T, action.lease, action.lease + 0.4) * (1 - ramp(T, action.send + 0.2, action.send + 0.8));
    lease.scale.setScalar(Math.max(0.001, le));
    lease.visible = le > 0.01;

    const answers = [action.receipt, action.result, action.state];
    ledger.forEach((m, i) => { const on = ramp(T, answers[i], answers[i] + 0.4); m.material.emissiveIntensity = 0.15 + on * (i === 2 ? 2.4 : 1.4); });

    const fu = clamp((T - action.feedback) / (action.feedbackLand - action.feedback));
    if (fu > 0 && fu < 1) {
      const u = easeInOut(fu);
      v.copy(bench.port.led); w.set(0, 2.2, 0);
      fb.position.copy(v).lerp(w, u); fb.position.y += Math.sin(Math.PI * u) * 6.0;
      fb.visible = true; fbGlow.position.copy(fb.position); fbGlow.material.opacity = 0.8; fb.rotation.set(T * 3, T * 2, 0);
    } else { fb.visible = false; fbGlow.material.opacity = 0; }

    const act = window01(T, policy.leave, ch.action.end + 1, 1.2);
    setPlatformEnergy(g, act * 0.9);
    const P = (x, y, z) => g.localToWorld(new THREE.Vector3(x, y, z));
    const near = window01(T, ch.action.start, action.outbox, 0.6);
    tags.put('act-name', P(0, Y + 6.8, 5.2), 'Action dispatch<small>command · outbox · lease · outcome</small>', near * 0.95, 'name effect');
    tags.put('act-key', P(-2.6, Y + 4.0, 0), `idempotency key<small>${RUN.command.idempotencyKey}… · one logical request</small>`, window01(T, action.stamp - 0.1, action.lease, 0.4) * 0.95, 'note');
    tags.put('act-out', P(0, Y + 2.3, 4.0), 'outbox: written before sending<small>a crash cannot lose it</small>', window01(T, action.outbox, action.retry, 0.4) * 0.95, 'note');
    tags.put('act-retry', P(-6.2, Y + 2.6, 3.2), 'a retry reuses the key<small>a repeat is answered from the receipt cache</small>', window01(T, action.retry - 0.1, action.lease, 0.4) * 0.95, 'note');
    tags.put('act-lease', P(2.8, Y + 4.4, 0), 'leased<small>one dispatcher owns it, re-checks, sends</small>', window01(T, action.lease, action.send + 0.8, 0.3) * 0.95, 'note');

    const mid = bench.port.board.clone().lerp(gateway.ports.top, 0.5).add(new THREE.Vector3(0, 9.6, 6));
    const row = (i, on, html, cls = '') => `<div class="tr ${cls}" style="opacity:${0.25 + 0.75 * on}"><i>${i}</i>${html}</div>`;
    const r1 = ramp(T, action.receipt, action.receipt + 0.4);
    const r2 = ramp(T, action.result, action.result + 0.4);
    const r3 = ramp(T, action.state, action.state + 0.4);
    const verifiedNow = T >= action.verified;
    const html = `<div class="truths"><h4>Three answers come back</h4>${row(1, r1, 'receipt <b>accepted</b><small>the device took the command</small>')}${row(2, r2, `result <b>${RUN.device.result}</b> · “${RUN.device.detail}”<small>the device says it is done</small>`)}${row(3, r3, `state query <b>${RUN.device.observed.target} energized · value ${RUN.device.observed.value}</b><small>asked again: what is the output now?</small>`, 'gold')}<div class="tr ${verifiedNow ? 'gold' : ''}" style="opacity:${verifiedNow ? 1 : 0.2}"><i>✓</i>verification <b>${RUN.device.reconciliation}</b><small>the observed state matches the command</small></div></div>`;
    tags.put('act-truths', mid, html, window01(T, action.answers - 0.3, ch.outro.start + 0.4, 0.3), 'truthbox');
    tags.put('act-fb', new THREE.Vector3(0, 3.0, 6.0), 'observed state stored as evidence<small>with the command, outcome and verification</small>', window01(T, action.feedbackLand - 0.6, ch.outro.start + 1.0, 0.4) * 0.95, 'note gold');
  }
  return { id: 'action', group: g, flight: new THREE.Group().add(cmd, fb, fbGlow), update };
}
