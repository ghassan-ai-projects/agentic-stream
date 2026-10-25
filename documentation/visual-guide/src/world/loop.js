import * as THREE from 'three';
import { glowSprite } from './theme.js';
import { outroBeats } from './schedule.js';
import { ringPoint, R, angleOf, STEP } from './layout.js';
import { RING_ORDER } from '../story.js';
import { clamp, easeInOut } from '../util.js';

const TRAIL = 9;

export function createLoopComet() {
  const group = new THREE.Group();
  const sprites = Array.from({ length: TRAIL }, (_, i) => { const s = glowSprite(i === 0 ? 0xffffff : 0x9fd8ff, 5.5 - i * 0.45, 0); group.add(s); return s; });
  const t0 = outroBeats.loop;
  const dur = 6.5;
  const angleAt = (T) => clamp((T - t0) / dur) * Math.PI * 2;

  function update(T) {
    const a = angleAt(T);
    const live = T >= t0 && T <= t0 + dur + 0.3;
    sprites.forEach((s, i) => {
      s.position.copy(ringPoint(a - i * 0.045, 1.6, R));
      s.material.opacity = live ? (1 - i / TRAIL) * 0.9 : 0;
    });
  }
  function energy(T, id) {
    if (T < t0 || T > t0 + dur + 2.5) return 0;
    const sa = angleOf(id);
    const a = angleAt(T);
    const d = ((a - sa) % (Math.PI * 2) + Math.PI * 2) % (Math.PI * 2);
    return d < 1.2 ? Math.exp(-d * 2.4) : 0;
  }
  return { group, update, energy };
}
