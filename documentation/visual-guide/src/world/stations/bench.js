import * as THREE from 'three';
import { COLORS, mat, box, cyl, edges, glowSprite } from '../theme.js';
import { stationGroup, setPlatformEnergy, tube, dot } from '../common.js';
import { clamp, lerp, ramp, smooth, window01 } from '../../util.js';
import { events, action } from '../schedule.js';
import { ch } from '../../timeline.js';
import { fmt } from '../../util.js';

const Y = 0.56;
const temps = events.filter((e) => e.type === 'temp');

export function tempAt(T) {
  if (T <= temps[0].launch) return temps[0].value;
  for (let i = 1; i < temps.length; i++) {
    if (T < temps[i].launch) return lerp(temps[i - 1].value, temps[i].value, smooth((T - temps[i - 1].launch) / (temps[i].launch - temps[i - 1].launch)));
  }
  return temps.at(-1).value;
}

function dotTexture(cols, rows, bg, fg) {
  const c = document.createElement('canvas');
  c.width = 512; c.height = 320;
  const g = c.getContext('2d');
  g.fillStyle = bg; g.fillRect(0, 0, 512, 320);
  g.fillStyle = fg;
  for (let i = 0; i < cols; i++) for (let j = 0; j < rows; j++) g.fillRect(14 + i * (484 / cols), 14 + j * (292 / rows), 6, 6);
  const t = new THREE.CanvasTexture(c); t.colorSpace = THREE.SRGBColorSpace; return t;
}

export function createBench() {
  const g = stationGroup('bench', 'evidence', 22, 17);
  const m = {};

  const breadboard = box(5.6, 0.34, 3.4, new THREE.MeshStandardMaterial({ map: dotTexture(26, 12, '#e8ecf3', '#8b97b3'), roughness: 0.7 }), -7, Y + 0.17, 2.6);
  g.add(breadboard);
  const dht = new THREE.Group();
  const dhtMat = new THREE.MeshStandardMaterial({ color: 0x2f6df0, roughness: 0.5, metalness: 0.1, emissive: 0x000000 });
  dht.add(box(1.7, 2.2, 1.1, dhtMat, 0, 1.2, 0));
  for (let i = 0; i < 5; i++) dht.add(box(0.07, 1.5, 0.05, mat.body(0x0b2b78), -0.55 + i * 0.28, 1.45, 0.58));
  for (let i = 0; i < 3; i++) dht.add(box(0.1, 0.7, 0.1, mat.body(0xcfd6e4, { metalness: 0.9, roughness: 0.3 }), -0.5 + i * 0.5, 0.1, 0));
  dht.position.set(-7, Y + 0.34, 2.6);
  g.add(dht);
  m.dht = dhtMat;
  const heat = new THREE.PointLight(0xffa660, 0, 12, 1.8);
  heat.position.set(-7, Y + 2.2, 3.4);
  g.add(heat);
  const heatSprite = glowSprite(0xff7a40, 7, 0);
  heatSprite.position.set(-7, Y + 2.0, 3.0);
  g.add(heatSprite);
  const waves = [0, 1, 2].map(() => {
    const w = new THREE.Mesh(new THREE.TorusGeometry(1, 0.035, 8, 48), mat.add(0xff8a5a, 0));
    w.rotation.x = Math.PI / 2;
    g.add(w);
    return w;
  });

  const therm = new THREE.Group();
  therm.add(cyl(0.5, 0.5, 6.6, mat.glass(0x9ad7ff, 0.2), 0, 3.3, 0, 24));
  const bulb = new THREE.Mesh(new THREE.SphereGeometry(0.78, 20, 16), mat.glow(0xff5a3a, 1.2));
  bulb.position.y = 0.78;
  therm.add(bulb);
  const fill = cyl(0.26, 0.26, 1, mat.glow(0xff6a4a, 1.6), 0, 0.5, 0, 16);
  therm.add(fill);
  for (let i = 0; i <= 8; i++) therm.add(box(0.5, 0.04, 0.04, mat.body(0x9fb2d6), 0.72, 1.4 + i * 0.62, 0));
  therm.position.set(-9.4, Y, -3.6);
  g.add(therm);

  const board = new THREE.Group();
  board.add(box(8.8, 0.24, 4.5, mat.body(0x0a5c4b, { roughness: 0.45, metalness: 0.25 }), 0, 0.12, 0));
  board.add(box(8.9, 0.04, 4.6, new THREE.MeshBasicMaterial({ color: 0x1a9d7c }), 0, 0.01, 0));
  for (const z of [-1.9, 1.9]) board.add(box(8.0, 0.5, 0.3, mat.matte(0x0b0e16), -0.1, 0.4, z));
  const pins = new THREE.InstancedMesh(new THREE.BoxGeometry(0.07, 0.3, 0.07), mat.body(0xe6c36a, { metalness: 0.9, roughness: 0.3 }), 80);
  const o = new THREE.Object3D();
  for (let i = 0; i < 40; i++) for (const [k, z] of [[0, -1.9], [1, 1.9]]) { o.position.set(-3.8 + i * 0.2, 0.8, z); o.updateMatrix(); pins.setMatrixAt(i * 2 + k, o.matrix); }
  board.add(pins);
  board.add(box(1.9, 0.22, 1.9, mat.matte(0x090b12), 0.4, 0.35, 0));
  board.add(box(1.5, 0.12, 1.5, mat.body(0x1a2133), 0.4, 0.5, 0));
  board.add(box(1.9, 1.1, 1.6, mat.body(0xaab4c8, { metalness: 0.95, roughness: 0.28 }), 3.6, 0.7, -0.9));
  board.add(box(1.7, 0.9, 1.5, mat.matte(0x0d0f16), -3.7, 0.55, 0.9));
  board.add(cyl(0.22, 0.22, 0.3, mat.body(0xc0394d), 2.2, 0.4, 1.4, 16));
  board.position.set(-0.5, Y + 0.45, -1.2);
  g.add(board);
  const led = new THREE.Mesh(new THREE.SphereGeometry(0.34, 20, 16), new THREE.MeshStandardMaterial({ color: 0x331015, emissive: 0xff3b53, emissiveIntensity: 0, roughness: 0.3 }));
  led.position.set(2.1, Y + 1.2, 0.2);
  g.add(led);
  const ledGlow = glowSprite(0xff3b53, 6, 0);
  ledGlow.position.copy(led.position);
  g.add(ledGlow);
  const ledLight = new THREE.PointLight(0xff3b53, 0, 18, 1.5);
  ledLight.position.copy(led.position).add(new THREE.Vector3(0, 1.4, 1));
  g.add(ledLight);

  const wireCol = [0xe0364d, 0x10131c, 0xf2c94c];
  wireCol.forEach((c, i) => {
    const a = new THREE.Vector3(-6.2 + i * 0.5, Y + 0.7, 3.2);
    const b = new THREE.Vector3(-3.3 + i * 0.4, Y + 1.5, 0.6);
    g.add(tube([a, a.clone().lerp(b, 0.5).add(new THREE.Vector3(0, 1.1, 0.6)), b], 0.07, mat.matte(c)));
  });

  const fan = new THREE.Group();
  fan.add(box(4.2, 4.2, 1.0, mat.body(0x1a2133, { roughness: 0.6 }), 0, 0, 0));
  fan.add(edges(box(4.2, 4.2, 1.0, mat.body(0x1a2133)), 0x5b6b95, 0.7));
  const rotor = new THREE.Group();
  rotor.add(cyl(0.55, 0.55, 0.8, mat.body(0x2d3a5c), 0, 0, 0, 20));
  rotor.children[0].rotation.x = Math.PI / 2;
  for (let i = 0; i < 7; i++) {
    const blade = box(0.7, 1.55, 0.07, mat.body(0x7f95c4, { roughness: 0.4 }), 0, 1.25, 0);
    const holder = new THREE.Group();
    blade.rotation.z = 0.0; blade.rotation.y = 0.45;
    holder.add(blade);
    holder.rotation.z = (i / 7) * Math.PI * 2;
    rotor.add(holder);
  }
  rotor.position.z = 0.55;
  fan.add(rotor);
  fan.position.set(7.4, Y + 2.3, 1.2);
  g.add(fan);
  g.add(box(2.4, 0.26, 2.2, mat.body(0x1a2133), 7.4, Y + 0.13, 1.2));
  g.add(box(1.0, 2.0, 0.5, mat.body(0x1a2133), 7.4, Y + 1.1, 1.0));
  const driver = box(1.6, 0.3, 0.8, mat.matte(0x090b12), 4.2, Y + 0.3, 4.4);
  g.add(box(2.2, 0.16, 1.4, mat.body(0x0a5c4b), 4.2, Y + 0.08, 4.4), driver);
  [[0xf2c94c, 4.0], [0x3b82f6, 4.4]].forEach(([c, x]) => {
    const a = new THREE.Vector3(x - 0.3, Y + 0.5, 4.2);
    const b = new THREE.Vector3(x + 2.6, Y + 1.6, 1.4);
    g.add(tube([a, a.clone().lerp(b, 0.5).add(new THREE.Vector3(0, 1.2, 0.4)), b], 0.06, mat.matte(c)));
  });

  g.updateMatrixWorld(true);
  const port = {
    sensor: g.localToWorld(new THREE.Vector3(-7, Y + 3.6, 2.6)),
    led: g.localToWorld(led.position.clone()),
    board: g.localToWorld(new THREE.Vector3(-0.5, Y + 1.6, -1.2)),
    usb: g.localToWorld(new THREE.Vector3(3.1, Y + 1.7, -2.1)),
    therm: g.localToWorld(new THREE.Vector3(-9.4, Y + 7.6, -3.6)),
    fan: g.localToWorld(new THREE.Vector3(7.4, Y + 5.2, 1.2)),
  };

  function update(T, tags) {
    const temp = tempAt(T);
    const k = clamp((temp - 31) / 5.3);
    const started = ramp(T, ch.gateway.start, ch.gateway.start + 2.5);
    m.dht.emissive.setRGB(1.0, 0.28, 0.12).multiplyScalar(k * 0.55 * started);
    heat.intensity = k * 9 * started;
    heatSprite.material.opacity = k * 0.55 * started;
    waves.forEach((w, i) => {
      const ph = ((T * 0.42 + i / 3) % 1 + 1) % 1;
      w.position.set(-7, Y + 2.3 + ph * 2.6, 2.6);
      w.scale.setScalar(0.5 + ph * 1.6);
      w.material.opacity = (1 - ph) * 0.5 * k * started;
    });
    const level = clamp((temp - 28) / 10);
    fill.scale.y = 0.2 + level * 5.6;
    fill.position.y = 0.78 + (fill.scale.y) / 2 - 0.2;
    const on = ramp(T, action.led, action.led + 0.35);
    led.material.emissiveIntensity = on * 3.2;
    led.material.color.setHex(on > 0.5 ? 0xff6075 : 0x331015);
    ledGlow.material.opacity = on * 0.9;
    ledLight.intensity = on * 40;
    rotor.rotation.z = 0;
    setPlatformEnergy(g, window01(T, ch.gateway.start, ch.gateway.end, 1) * 0.7 + window01(T, action.led - 1, action.led + 5, 1) * 0.5);

    const near = window01(T, ch.gateway.start - 1, ch.gateway.end + 1, 1.5);
    tags.put('bench-name', g.localToWorld(new THREE.Vector3(0, 8.4, 4)), 'Sensor bench<small>played by a simulator in this run</small>', near * 0.95, 'name evidence');
    tags.put('bench-temp', port.therm, `${fmt(temp, 1)}<i> °C</i>`, window01(T, ch.gateway.start + 0.4, ch.gateway.end + 2, 1) , 'value hot');
    tags.put('bench-led', g.localToWorld(new THREE.Vector3(2.1, Y + 3.2, 0.2)), `LED <b>${on > 0.5 ? 'ON' : 'off'}</b><small>led-01</small>`, window01(T, action.led - 0.8, action.answers + 0.2, 0.3) * 0.95, on > 0.5 ? 'note on' : 'note');
  }

  return { id: 'bench', group: g, port, update };
}
