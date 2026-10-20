import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { createStage } from './world/stage.js';
import { createRails } from './world/rails.js';
import { createTags, auditTags, fallbacks } from './world/tags.js';
import { cameraAt, stationView } from './world/camera.js';
import { createBench } from './world/stations/bench.js';
import { createGateway } from './world/stations/gateway.js';
import { createLog } from './world/stations/log.js';
import { createEngine } from './world/stations/engine.js';
import { createSituation } from './world/stations/situation.js';
import { createScheduler } from './world/stations/scheduler.js';
import { createEpisode } from './world/stations/episode.js';
import { createTamoz } from './world/stations/tamoz.js';
import { createValidator } from './world/stations/validator.js';
import { createPolicy } from './world/stations/policy.js';
import { createAction } from './world/stations/action.js';
import { createStore } from './world/stations/store.js';
import { createPackets } from './world/packets.js';
import { createLoopComet } from './world/loop.js';
import { createUI, createOverlay } from './ui.js';
import { TOTAL, ch, cue } from './timeline.js';
import { RING_ORDER, STATIONS } from './story.js';
import { stationPoint } from './world/layout.js';
import { ramp, window01 } from './util.js';

const params = new URLSearchParams(location.search);
const film = params.get('film') === '1';
if (film) document.documentElement.classList.add('film');
const canvas = document.getElementById('stage');
const stage = createStage(canvas);
const rails = createRails();
const tags = createTags(document.getElementById('tags'));
const store = createStore();
const bench = createBench();
const gateway = createGateway();
const log = createLog();
const engine = createEngine();
const situation = createSituation(engine);
const scheduler = createScheduler();
const episode = createEpisode(scheduler);
const tamoz = createTamoz();
const validator = createValidator();
const policy = createPolicy();
const action = createAction(bench, gateway, log);
const packets = createPackets(bench, gateway, log);
const comet = createLoopComet();
const stations = [bench, gateway, log, engine, situation, scheduler, episode, tamoz, validator, policy, action];
const byId = Object.fromEntries(stations.map((s) => [s.id, s]));

stage.scene.add(rails.group, store.group, packets.group, situation.tokens, action.flight, comet.group);
stations.forEach((s) => stage.scene.add(s.group));

const overview = RING_ORDER;
const hits = stations.map((s) => s.group.userData.hit);
let size = { w: 0, h: 0 };
let free = null;
let controls = null;

function fit() {
  const w = film ? Number(params.get('w') ?? 1920) : innerWidth;
  const h = film ? Number(params.get('h') ?? 1080) : innerHeight;
  if (w === size.w && h === size.h) return;
  size = { w, h };
  stage.resize(w, h, film ? 1 : undefined);
}

const LIGHT0 = cue('intro', 0).start + 0.4;
const LIGHT_STEP = (cue('intro', 1).end - LIGHT0) / (RING_ORDER.length + 1);
function boostFor(T) {
  const b = {};
  const lit = (k) => 0.55 * ramp(T, LIGHT0 + k * LIGHT_STEP, LIGHT0 + (k + 1) * LIGHT_STEP) * (1 - ramp(T, ch.intro.end - 1.0, ch.intro.end + 0.5));
  RING_ORDER.forEach((id, k) => { b[id] = lit(k) + 0.5 * comet.energy(T, id); });
  b.tamoz = lit(6);
  return b;
}

const filmOverlay = film ? createOverlay(document.getElementById('ui')) : null;
const ui = film ? null : createUI({ stations: byId, hits, canvas, camera: stage.camera, seek: (T) => seek(T), onFree: (on, id) => setFree(on, id) });

function setFree(on, id) {
  if (on) {
    if (!controls) { controls = new OrbitControls(stage.camera, canvas); controls.enableDamping = false; controls.maxPolarAngle = Math.PI * 0.49; controls.minDistance = 6; controls.maxDistance = 260; controls.addEventListener('change', () => seek(window.__T)); }
    const target = id ? stationView(id) : { pos: stage.camera.position.clone(), target: new THREE.Vector3(0, 2, 0) };
    stage.camera.position.copy(target.pos);
    controls.target.copy(target.target);
    controls.enabled = true;
    controls.update();
    free = true;
  } else { free = false; if (controls) controls.enabled = false; }
  seek(window.__T);
}

function seek(T) {
  fit();
  window.__T = T;
  if (!free) {
    const cam = cameraAt(T);
    stage.camera.position.copy(cam.pos);
    if (stage.camera.fov !== cam.fov) { stage.camera.fov = cam.fov; stage.camera.updateProjectionMatrix(); }
    stage.camera.lookAt(cam.target);
  }
  const panel = film ? null : document.querySelector('.panel:not(.closed)');
  const shift = panel && innerWidth > 980 ? panel.getBoundingClientRect().width / 2 : 0;
  if (shift) stage.camera.setViewOffset(size.w, size.h, shift, 0, size.w, size.h); else stage.camera.clearViewOffset();
  stage.camera.updateProjectionMatrix();
  stage.camera.updateMatrixWorld();
  rails.update(T);
  tags.begin();
  const boost = boostFor(T);
  stations.forEach((s) => { s.group.userData.boost = boost[s.id] ?? 0; });
  stations.forEach((s) => s.update(T, tags));
  store.update(T, tags);
  packets.update(T);
  comet.update(T);
  ringLabels(T);
  stage.frame(T);
  tags.end(stage.camera, size.w, size.h);
  ui?.sync(T);
  filmOverlay?.sync(T);
}

function ringLabels(T) {
  const v = window01(T, 1.2, ch.intro.end - 0.3, 0.6) + window01(T, ch.outro.start + 1.2, 1e9, 0.8);
  overview.forEach((id) => {
    const s = STATIONS[id];
    const p = stationPoint(id);
    p.y = 8.2 + (id === 'tamoz' ? 1 : 0);
    tags.put(`ov-${id}`, p, s.name, Math.min(1, v) * 0.92, `name mini ${s.plane}`);
  });
}

window.__seek = seek;
window.__audit = () => auditTags(size.w, size.h);
window.__tagFallbacks = fallbacks;
window.__total = TOTAL;
window.__setFree = setFree;
addEventListener('resize', () => seek(window.__T ?? 0));
window.__T = Number(params.get('t') ?? 0);
seek(window.__T);
ui?.start();
