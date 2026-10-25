import * as THREE from 'three';
import { COLORS, mat, box, cyl, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy } from '../common.js';
import { clamp, window01, decay, pulse } from '../../util.js';
import { events, action, gatewayBeats } from '../schedule.js';
import { RUN } from '../../story.js';
import { ch } from '../../timeline.js';

const Y = 0.56;

const S = RUN.events.sample;
const E = RUN.events.envelope;
const ENVELOPE = `<div class="cardx"><h4>The first reading, as an event</h4><div class="kv"><span>id</span><b>${E.id}</b></div><div class="kv"><span>source</span><b>${E.source}</b></div><div class="kv"><span>type</span><b>${S.event_type}</b></div><div class="kv"><span>entity</span><b>${RUN.situation.entity.id}</b></div><div class="kv"><span>payload</span><b>${S.celsius} °C · ${S.quality} · seq ${S.seq}</b></div><div class="kv hot"><span>event time</span><b>${E.eventTime} UTC</b></div><div class="kv"><span>arrived</span><b>${E.ingestedAt} UTC</b></div></div>`;

export function createGateway() {
  const g = stationGroup('gateway', 'evidence', 15, 12);
  g.add(box(6.4, 2.8, 4.6, mat.body(0x1a2440, { roughness: 0.4, metalness: 0.6 }), 0, Y + 1.4, 0));
  const topEdge = new THREE.LineSegments(new THREE.EdgesGeometry(new THREE.BoxGeometry(6.4, 2.8, 4.6)), mat.line(COLORS.plane.evidence, 0.9)); topEdge.position.set(0, Y + 1.4, 0); g.add(topEdge);
  const strip = new THREE.Mesh(new THREE.PlaneGeometry(4.4, 0.5), new THREE.MeshBasicMaterial({ color: 0x4cc9f0 }));
  strip.position.set(0, Y + 1.6, 2.32);
  g.add(strip);
  const port = cyl(0.7, 0.7, 1.2, mat.body(0xaab4c8, { metalness: 0.9, roughness: 0.3 }), -3.6, Y + 1.6, 0, 20);
  port.rotation.z = Math.PI / 2;
  g.add(port);
  const pipe = cyl(0.55, 0.55, 1.6, mat.body(0x2d3a5c, { metalness: 0.7 }), 3.8, Y + 1.6, 0, 20);
  pipe.rotation.z = Math.PI / 2;
  g.add(pipe);

  const rings = [-5.2, 5.2].map((x) => {
    const r = new THREE.Mesh(new THREE.TorusGeometry(1.5, 0.09, 12, 48), new THREE.MeshBasicMaterial({ color: COLORS.plane.evidence.clone().multiplyScalar(1.5) }));
    r.rotation.y = Math.PI / 2;
    r.position.set(x, Y + 1.6, 0);
    g.add(r);
    return r;
  });
  const gate = glowSprite(0x34d399, 6, 0);
  gate.position.set(-5.2, Y + 1.6, 0);
  g.add(gate);
  const returnPipe = new THREE.Mesh(new THREE.TorusGeometry(1.5, 0.09, 12, 48), new THREE.MeshBasicMaterial({ color: COLORS.plane.effect.clone().multiplyScalar(1.5) }));
  returnPipe.rotation.y = Math.PI / 2;
  returnPipe.position.set(0, Y + 5.6, 0);
  returnPipe.scale.setScalar(0.001);
  g.add(returnPipe);

  g.updateMatrixWorld(true);
  const ports = {
    in: g.localToWorld(new THREE.Vector3(-5.2, Y + 1.6, 0)),
    out: g.localToWorld(new THREE.Vector3(5.2, Y + 1.6, 0)),
    top: g.localToWorld(new THREE.Vector3(0, Y + 3.4, 0)),
    commandIn: g.localToWorld(new THREE.Vector3(-1.5, Y + 6.2, 0)),
    commandOut: g.localToWorld(new THREE.Vector3(-4, Y + 2.2, 0)),
  };

  function update(T, tags) {
    let flash = 0;
    for (const e of events) flash = Math.max(flash, decay(T, e.atGateway, 0.28));
    const cmd = pulse(T, action.atGateway, 0.5);
    gate.material.opacity = flash * 0.85;
    rings.forEach((r) => { r.material.color.copy(COLORS.plane.evidence).lerp(new THREE.Color(0x34d399), flash).multiplyScalar(1.4 + flash * 1.8); });
    returnPipe.scale.setScalar(0.001 + cmd * 1);
    strip.material.color.setHex(0x4cc9f0).lerp(new THREE.Color(0xfb7185), cmd);
    const energy = window01(T, ch.gateway.start, ch.log.start + 4, 1.2) * 0.65 + cmd * 0.8;
    setPlatformEnergy(g, energy);

    const count = events.filter((e) => T >= e.atGateway).length;
    const near = window01(T, ch.gateway.start, ch.gateway.end - 0.2, 1.0);
    tags.put('gateway-name', g.localToWorld(new THREE.Vector3(0, 6.2, 3)), `Gateway<small>the edge · emulated in this run · ${count} events</small>`, near * 0.95, 'name evidence');
    tags.put('gateway-env', g.localToWorld(new THREE.Vector3(0, Y + 9.5, -1)), ENVELOPE, window01(T, gatewayBeats.envelope, gatewayBeats.envelopeEnd, 0.5), 'card evidence');
    tags.put('gateway-cmd', g.localToWorld(new THREE.Vector3(0, Y + 7.2, 0)), 'command to the device<small>a typed record, bound to its policy</small>', window01(T, action.atGateway - 0.2, action.atBench + 0.6, 0.4) * 0.95, 'note cmd');
  }
  return { id: 'gateway', group: g, ports, update };
}
