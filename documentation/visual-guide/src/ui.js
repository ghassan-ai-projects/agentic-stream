import * as THREE from 'three';
import { chapters, chapterAt, captionAt, TOTAL } from './timeline.js';
import { STATIONS, PLANES, INVARIANTS, EVENT_TYPES, COUNTS } from './story.js';
import { counters } from './world/schedule.js';
import { clamp } from './util.js';

const setOn = (btn, on) => { btn.classList.toggle('on', on); btn.setAttribute('aria-pressed', String(on)); };
const el = (tag, cls, html) => { const e = document.createElement(tag); if (cls) e.className = cls; if (html != null) e.innerHTML = html; return e; };
const mmssWords = (s) => { const t = Math.round(s); return `${Math.floor(t / 60)} min ${t % 60} s`; };
const mmss = (s) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;
const ICON = {
  play: '<svg viewBox="0 0 24 24" width="22" height="22"><path d="M8 5v14l11-7z" fill="currentColor"/></svg>',
  pause: '<svg viewBox="0 0 24 24" width="22" height="22"><path d="M7 5h4v14H7zm6 0h4v14h-4z" fill="currentColor"/></svg>',
  prev: '<svg viewBox="0 0 24 24" width="20" height="20"><path d="M6 5h2v14H6zm3.5 7L19 5v14z" fill="currentColor"/></svg>',
  next: '<svg viewBox="0 0 24 24" width="20" height="20"><path d="M16 5h2v14h-2zM5 5l9.5 7L5 19z" fill="currentColor"/></svg>',
  replay: '<svg viewBox="0 0 24 24" width="22" height="22"><path d="M12 5V2L7 6.5 12 11V8a5 5 0 1 1-5 5H5a7 7 0 1 0 7-8z" fill="currentColor"/></svg>',
  cc: '<svg viewBox="0 0 24 24" width="20" height="20"><path d="M5 5h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zm3.2 9.4c.5.3 1 .5 1.7.5.8 0 1.3-.3 1.7-.8l.9.9c-.7.8-1.6 1.2-2.7 1.2-1.9 0-3.1-1.3-3.1-3.2s1.3-3.2 3.1-3.2c1.1 0 2 .4 2.7 1.200l-.9.9c-.4-.5-.9-.8-1.7-.8-1 0-1.800.700-1.800 1.900s.7 1.900 1.800 1.900zm7.300 0c.5.3 1 .5 1.700.5.8 0 1.300-.3 1.700-.8l.900.900c-.7.800-1.600 1.200-2.700 1.200-1.900 0-3.100-1.300-3.100-3.200s1.300-3.200 3.100-3.200c1.100 0 2 .4 2.700 1.200l-.9.9c-.4-.5-.9-.8-1.700-.8-1 0-1.800.7-1.800 1.900s.7 1.900 1.800 1.900z" fill="currentColor"/></svg>',
  sound: '<svg viewBox="0 0 24 24" width="20" height="20"><path d="M4 9v6h4l5 4V5L8 9zm11.500 3a4.500 4.500 0 0 0-2.500-4v8a4.500 4.500 0 0 0 2.500-4z" fill="currentColor"/></svg>',
  orbit: '<svg viewBox="0 0 24 24" width="20" height="20"><path d="M12 3a9 9 0 1 0 9 9h-2a7 7 0 1 1-7-7zm0 5 4 4-4 4-4-4z" fill="currentColor"/></svg>',
  info: '<svg viewBox="0 0 24 24" width="20" height="20"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm1 15h-2v-6h2zm0-8h-2V7h2z" fill="currentColor"/></svg>',
};


export function createOverlay(root, ccOn = () => true) {
  const brand = el('div', 'brand', '<b>Agentic Stream</b><span>one run, end to end</span>');
  const chip = el('div', 'chip');
  const hud = el('div', 'hud');
  const hudItems = [
    ['events', 'events', COUNTS.events], ['versions', 'Situation versions', COUNTS.versions], ['episodes', 'episode', 1],
    ['decisions', 'decision', 1], ['commands', 'command', 1], ['verified', 'verified effect', 1],
  ];
  const hudCells = hudItems.map(([k, label, max], i) => {
    const c = el('div', 'hcell', `<b data-k="${k}">0</b><span>${label}</span>`);
    hud.appendChild(c);
    if (i < hudItems.length - 1) hud.appendChild(el('div', 'harrow', '›'));
    return { k, c, b: c.querySelector('b'), max };
  });
  const cap = el('div', 'caption');
  const legend = el('div', 'legend', Object.entries(EVENT_TYPES).map(([k, v]) => `<i style="--c:${v.color}"></i>${v.label}`).join(''));
  root.append(brand, chip, hud, legend, cap);
  let chipChapter = null;
  function sync(T) {
    const c = chapterAt(T);
    if (chipChapter !== c.id) { chipChapter = c.id; chip.innerHTML = `<b>${c.index + 1}</b><span>${c.title}</span>`; }
    const k = counters(T);
    hudCells.forEach((h) => { const v = Math.min(k[h.k], h.max); if (h.b.textContent !== String(v)) h.b.textContent = v; h.c.classList.toggle('done', v >= h.max); h.c.classList.toggle('live', v > 0 && v < h.max); });
    const cp = captionAt(T);
    const on = Boolean(ccOn() && cp);
    const text = on ? cp.text : '';
    if (cap.textContent !== text) cap.textContent = text;
    cap.classList.toggle('on', on);
  }
  return { cap, sync };
}

export function createUI({ stations, hits, canvas, camera, seek, onFree }) {
  const root = document.getElementById('ui');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const state = { T: 0, playing: false, speed: 1, cc: true, voice: false, free: false, picked: null, last: 0, ended: false };

  const overlayParts = createOverlay(root, () => state.cc);
  const { cap } = overlayParts;

  const bar = el('div', 'controls');
  const bPlay = el('button', 'btn primary', ICON.play); bPlay.title = 'Play / pause (space)'; bPlay.setAttribute('aria-label', 'Play or pause');
  const bPrev = el('button', 'btn', ICON.prev); bPrev.title = 'Previous chapter (←)'; bPrev.setAttribute('aria-label', 'Previous chapter');
  const bNext = el('button', 'btn', ICON.next); bNext.title = 'Next chapter (→)'; bNext.setAttribute('aria-label', 'Next chapter');
  const time = el('div', 'time', '0:00 / ' + mmss(TOTAL));
  const track = el('div', 'track'); track.setAttribute('role', 'slider'); track.setAttribute('aria-label', 'Timeline'); track.setAttribute('aria-valuemin', '0'); track.setAttribute('aria-valuemax', String(Math.round(TOTAL))); track.tabIndex = 0;
  const segs = chapters.map((c) => { const s = el('div', 'seg'); s.style.flex = String(c.dur); s.title = `${c.index + 1}. ${c.title}`; s.appendChild(el('i')); track.appendChild(s); return s; });
  const playhead = el('div', 'playhead'); track.appendChild(playhead);
  const bSpeed = el('button', 'btn txt', '1×'); bSpeed.title = 'Speed (S)';
  const bCC = el('button', 'btn', ICON.cc); bCC.title = 'Captions (C)'; bCC.setAttribute('aria-label', 'Captions');
  const bVoice = el('button', 'btn', ICON.sound); bVoice.title = 'Narration (V)'; bVoice.setAttribute('aria-label', 'Narration');
  const bFree = el('button', 'btn txt', 'Explore'); bFree.title = 'Free camera (E)';
  const bPanel = el('button', 'btn', ICON.info); bPanel.title = 'Guide panel (G)'; bPanel.setAttribute('aria-label', 'Guide panel');
  bSpeed.setAttribute('aria-label', 'Speed');
  bFree.setAttribute('aria-label', 'Free camera');
  [bCC, bVoice, bFree, bPanel].forEach((b) => setOn(b, false));
  setOn(bCC, true);
  bar.append(bPlay, bPrev, bNext, time, track, bSpeed, bCC, bVoice, bFree, bPanel);

  const panel = el('aside', 'panel');
  panel.id = 'guide-panel';
  panel.setAttribute('aria-label', 'Guide panel');
  bPanel.setAttribute('aria-controls', panel.id);
  if (reduced) { bPlay.title = 'Next chapter step (space)'; bPlay.setAttribute('aria-label', 'Next chapter step'); }
  const panelBody = el('div', 'pbody');
  panel.appendChild(panelBody);
  const hint = el('div', 'hint', 'Space play · ← → chapter · E explore · drag to orbit · click a station');
  const overlay = el('div', 'start', `<div class="card" role="dialog" aria-modal="true" aria-labelledby="start-title"><h1 id="start-title">Agentic Stream, end to end</h1><p>One real recorded run, from a warming sensor to a verified effect, in ${mmssWords(TOTAL)}. ${reduced ? 'Reduced motion is on, so the tour steps one chapter at a time instead of playing.' : 'Press play for the narrated tour, or explore the 3D world yourself.'}</p><p class="missing" hidden>The narration track is not built in this checkout (it is large and git-ignored), so the tour runs silently with captions. See the README to render it.</p><div class="row"><button class="big" id="go">${reduced ? '▶ Start the tour' : '▶ Play the tour'}</button><button class="big ghost" id="explore">Explore freely</button></div><small>${COUNTS.events} events · ${COUNTS.versions} Situation versions · 1 episode · 1 verified effect. Every number comes from the recorded run.</small></div>`);
  root.append(bar, panel, hint, overlay);

  const audio = new Audio('film/narration.m4a');
  audio.preload = 'auto';
  audio.addEventListener('error', () => { bVoice.style.display = 'none'; state.voice = false; setOn(bVoice, false); overlay.querySelector('.missing').hidden = false; });

  function setPanel(open) {
    panel.classList.toggle('closed', !open);
    panel.inert = !open;
    setOn(bPanel, open);
    bPanel.setAttribute('aria-expanded', String(open));
  }
  setPanel(innerWidth >= 1500);

  function setStart(open) {
    overlay.classList.toggle('gone', !open);
    overlay.inert = !open;
    bar.inert = open;
    if (!open) return;
    panel.inert = true;
    overlay.querySelector('#go').focus();
  }
  function closeStart() {
    if (overlay.classList.contains('gone')) return;
    setStart(false);
    panel.inert = panel.classList.contains('closed');
    bPlay.focus();
  }

  function renderPanel(T) {
    const c = chapterAt(T);
    const ids = state.picked ? [state.picked] : c.stations;
    const key = (state.picked ?? c.id) + '|' + ids.join();
    if (panelBody.__key === key) return;
    panelBody.__key = key;
    const head = state.picked ? `<div class="kicker">Station</div><h2>${STATIONS[state.picked].name}</h2>` : `<div class="kicker">Chapter ${c.index + 1} of ${chapters.length}</div><h2>${c.title}</h2><p class="narr">${c.narration}</p>`;
    const cards = ids.map((id) => stationCard(id)).join('') || `<div class="card2"><p>Eleven stations, one loop. Click any station in the scene, or pick one below.</p></div>`;
    const list = Object.entries(STATIONS).map(([id, s]) => `<button class="pick ${state.picked === id ? 'on' : ''}" data-st="${id}" style="--c:${PLANES[s.plane].color}">${s.name}</button>`).join('');
    const legendHtml = Object.values(PLANES).map((p) => `<span style="--c:${p.color}"><i></i>${p.name}</span>`).join('');
    panelBody.innerHTML = `${head}${cards}<div class="sub">All stations</div><div class="picks">${list}</div><div class="sub">Planes</div><div class="planes">${legendHtml}</div><details class="inv"><summary>The ten invariants</summary><ol>${INVARIANTS.map((t) => `<li>${t}</li>`).join('')}</ol></details>${state.picked ? '<button class="link" data-act="tour">← back to the tour</button>' : ''}`;
  }

  function stationCard(id) {
    const s = STATIONS[id];
    const col = PLANES[s.plane].color;
    return `<section class="card2" style="--c:${col}"><div class="pill"><i></i>${PLANES[s.plane].name} plane</div><h3>${s.name}</h3>
      <dl><dt>What it does</dt><dd>${s.what}</dd><dt>Why it exists</dt><dd>${s.why}</dd><dt>In this run</dt><dd><ul>${s.real.map((r) => `<li>${r}</li>`).join('')}</ul></dd><dt>If something goes wrong</dt><dd>${s.failsSafe}</dd><dt>Owner</dt><dd class="mono">${s.owner}</dd></dl>
      <div class="invs">${s.invariants.map((n) => `<span title="${INVARIANTS[n - 1]}">invariant ${n}</span>`).join('')}</div></section>`;
  }

  panel.addEventListener('click', (e) => {
    const b = e.target.closest('[data-st]');
    if (b) { pick(b.dataset.st); return; }
    if (e.target.closest('[data-act="tour"]')) { state.picked = null; setFree(false); renderPanel(state.T); }
  });

  function pick(id) {
    state.picked = id;
    setFree(true, id);
    pause();
    panelBody.__key = null;
    setPanel(true);
    renderPanel(state.T);
  }

  function setFree(on, id) {
    state.free = on;
    setOn(bFree, on);
    bFree.textContent = on ? 'Resume tour' : 'Explore';
    onFree(on, id);
    hint.classList.toggle('show', on);
  }

  function pause() { state.playing = false; bPlay.innerHTML = state.ended ? ICON.replay : ICON.play; audio.pause(); }
  function play() {
    if (state.T >= TOTAL - 0.05) { state.T = 0; }
    if (state.free) { state.picked = null; setFree(false); }
    closeStart();
    if (reduced) { stepChapter(1); return; }
    state.playing = true; state.ended = false; bPlay.innerHTML = ICON.pause; state.last = performance.now();
    if (state.voice) { audio.currentTime = state.T; audio.play().catch(() => {}); }
  }
  function jump(T) {
    state.T = clamp(T, 0, TOTAL - 0.001); state.ended = false;
    if (state.voice) audio.currentTime = state.T;
    seek(state.T);
  }
  const settled = (c) => c.start + c.dur * 0.92;
  function chapterJump(dir) {
    const c = chapterAt(state.T);
    const target = dir > 0 ? chapters[Math.min(chapters.length - 1, c.index + 1)] : (state.T - c.start > 2 ? c : chapters[Math.max(0, c.index - 1)]);
    if (state.free) { state.picked = null; setFree(false); }
    jump(reduced ? settled(target) : target.start + 0.01);
  }
  function stepChapter(dir) {
    const c = chapterAt(state.T);
    if (dir > 0 && state.T < settled(c) - 0.05) { if (state.free) { state.picked = null; setFree(false); } jump(settled(c)); } else chapterJump(dir);
  }

  bPlay.onclick = () => (state.playing ? pause() : play());
  bPrev.onclick = () => chapterJump(-1);
  bNext.onclick = () => chapterJump(1);
  bSpeed.onclick = () => { state.speed = state.speed === 1 ? 1.5 : state.speed === 1.5 ? 0.5 : 1; bSpeed.textContent = state.speed + '×'; audio.playbackRate = state.speed; };
  bCC.onclick = () => { state.cc = !state.cc; setOn(bCC, state.cc); };
  bVoice.onclick = () => { state.voice = !state.voice; setOn(bVoice, state.voice); if (state.voice && state.playing) { audio.currentTime = state.T; audio.play().catch(() => {}); } else audio.pause(); };
  bFree.onclick = () => { if (state.free) { state.picked = null; setFree(false); panelBody.__key = null; } else { setFree(true); pause(); } };
  bPanel.onclick = () => { setPanel(panel.classList.contains('closed')); seek(state.T); };
  overlay.querySelector('#go').onclick = () => { if (!reduced && bVoice.style.display !== 'none') { state.voice = true; setOn(bVoice, true); } play(); };
  overlay.querySelector('#explore').onclick = () => { closeStart(); jump(chapters.find((c) => c.id === 'outro').start + 3); setFree(true); };

  const scrub = (ev) => { const r = track.getBoundingClientRect(); jump(clamp((ev.clientX - r.left) / r.width) * TOTAL); };
  track.addEventListener('pointerdown', (ev) => { track.setPointerCapture(ev.pointerId); scrub(ev); track.onpointermove = scrub; });
  track.addEventListener('pointerup', () => { track.onpointermove = null; });
  track.addEventListener('keydown', (ev) => {
    if (ev.key !== 'ArrowRight' && ev.key !== 'ArrowLeft') return;
    ev.stopPropagation();
    jump(state.T + (ev.key === 'ArrowRight' ? 2 : -2));
  });

  addEventListener('keydown', (ev) => {
    if (ev.target.closest?.('input,textarea') || ev.ctrlKey || ev.metaKey || ev.altKey) return;
    const k = ev.key.toLowerCase();
    if (k === ' ' && ev.target.closest?.('button')) return;
    if (k === ' ') { ev.preventDefault(); bPlay.onclick(); }
    else if (k === 'arrowright') chapterJump(1);
    else if (k === 'arrowleft') chapterJump(-1);
    else if (k === 'c') bCC.onclick();
    else if (k === 'v') bVoice.onclick();
    else if (k === 'e') bFree.onclick();
    else if (k === 'g') bPanel.onclick();
    else if (k === 's') bSpeed.onclick();
    else if (/^[1-9]$/.test(k)) jump(chapters[Number(k) - 1].start + 0.01);
    else if (k === '0') jump(0);
  });

  const ray = new THREE.Raycaster();
  const ndc = new THREE.Vector2();
  let down = null;
  canvas.addEventListener('pointerdown', (e) => { down = { x: e.clientX, y: e.clientY }; });
  canvas.addEventListener('pointerup', (e) => {
    if (!down || Math.hypot(e.clientX - down.x, e.clientY - down.y) > 5) return;
    ndc.set((e.clientX / innerWidth) * 2 - 1, -(e.clientY / innerHeight) * 2 + 1);
    ray.setFromCamera(ndc, camera);
    const h = ray.intersectObjects(hits, false)[0];
    if (h) pick(h.object.userData.station);
  });
  canvas.addEventListener('pointermove', (e) => {
    ndc.set((e.clientX / innerWidth) * 2 - 1, -(e.clientY / innerHeight) * 2 + 1);
    ray.setFromCamera(ndc, camera);
    canvas.style.cursor = ray.intersectObjects(hits, false).length ? 'pointer' : 'default';
  });

  function frame(now) {
    if (state.playing) {
      const dt = Math.min(0.1, (now - state.last) / 1000);
      state.last = now;
      state.T += dt * state.speed;
      if (state.T >= TOTAL) { state.T = TOTAL; state.ended = true; pause(); }
      seek(state.T);
    }
    requestAnimationFrame(frame);
  }

  function sync(T) {
    state.T = T;
    const c = chapterAt(T);
    time.textContent = `${mmss(T)} / ${mmss(TOTAL)}`;
    playhead.style.left = (T / TOTAL * 100).toFixed(3) + '%';
    segs.forEach((s, i) => { const cc = chapters[i]; s.style.setProperty('--p', String(clamp((T - cc.start) / cc.dur))); s.classList.toggle('now', i === c.index); });
    overlayParts.sync(T);
    if (!state.free || !state.picked) renderPanel(T);
    track.setAttribute('aria-valuenow', String(Math.round(T)));
    track.setAttribute('aria-valuetext', `${mmss(T)} of ${mmss(TOTAL)}, chapter ${c.index + 1}: ${c.title}`);
  }

  function start() {
    requestAnimationFrame(frame);
    renderPanel(0);
    setStart(true);
    const want = new URLSearchParams(location.search).get('station');
    if (want && STATIONS[want]) { closeStart(); const c = chapters.find((x) => x.stations.includes(want)); if (c) jump(c.start + Math.min(c.dur - 1, 7)); pick(want); }
  }
  return { start, sync };
}
