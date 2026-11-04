import { clamp } from '../util.js';

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
const NS = 'http://www.w3.org/2000/svg';

export function s(tag, attrs = {}, children = []) {
  const e = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs)) if (v != null) e.setAttribute(k, v);
  for (const c of [].concat(children)) if (c != null) e.append(c.nodeType ? c : document.createTextNode(String(c)));
  return e;
}

export function h(tag, attrs = {}, children = []) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) { if (k === 'class') e.className = v; else if (k === 'html') e.innerHTML = v; else if (k.startsWith('on')) e.addEventListener(k.slice(2), v); else if (v != null) e.setAttribute(k, v); }
  for (const c of [].concat(children)) if (c != null) e.append(c.nodeType ? c : document.createTextNode(String(c)));
  return e;
}

export const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

export function wrap(text, n) {
  const lines = [];
  let cur = '';
  for (const w of text.split(' ')) { if ((cur + ' ' + w).trim().length > n) { lines.push(cur); cur = w; } else cur = (cur + ' ' + w).trim(); }
  if (cur) lines.push(cur);
  return lines;
}

export function keepFocus(root, rebuild) {
  const id = document.activeElement?.closest?.('[data-focus]')?.dataset.focus;
  rebuild();
  if (id) root.querySelector(`[data-focus="${id}"]`)?.focus({ preventScroll: true });
}

const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches;

export class Clock {
  constructor({ duration, onFrame, speed = 1 }) {
    Object.assign(this, { duration, onFrame, speed, t: 0, playing: false, last: 0 });
    this.loop = this.loop.bind(this);
    requestAnimationFrame(this.loop);
  }
  loop(now) {
    if (this.playing) {
      const dt = Math.min(0.1, (now - this.last) / 1000);
      this.t = Math.min(this.duration, this.t + dt * this.speed);
      if (this.t >= this.duration) this.playing = false;
      this.onFrame(this.t, this);
    }
    this.last = now;
    requestAnimationFrame(this.loop);
  }
  play() {
    if (reducedMotion) { this.seek(this.duration); return; }
    if (this.t >= this.duration) this.t = 0;
    this.playing = true; this.last = performance.now(); this.onFrame(this.t, this); }
  pause() { this.playing = false; this.onFrame(this.t, this); }
  seek(t) { this.t = clamp(t, 0, this.duration); this.onFrame(this.t, this); }
}

export function transport(root, clock, { label = 'Play' } = {}) {
  const play = h('button', { class: 'btn primary' }, label);
  const reset = h('button', { class: 'btn' }, 'Reset');
  const scrub = h('input', { type: 'range', min: 0, max: 1000, value: 0, 'aria-label': 'Timeline' });
  const speed = h('select', { 'aria-label': 'Speed' }, [h('option', { value: 0.5 }, '0.5×'), h('option', { value: 1, selected: '' }, '1×'), h('option', { value: 2 }, '2×')]);
  play.onclick = () => (clock.playing ? clock.pause() : clock.play());
  reset.onclick = () => { clock.pause(); clock.seek(0); };
  scrub.oninput = () => { const t = (scrub.value / 1000) * clock.duration; clock.pause(); clock.seek(t); };
  speed.onchange = () => { clock.speed = Number(speed.value); };
  root.append(play, reset, scrub, speed);
  const sync = () => { play.textContent = clock.playing ? 'Pause' : clock.t >= clock.duration ? 'Replay' : label; scrub.value = String(Math.round((clock.t / clock.duration) * 1000)); };
  return sync;
}

export function control(root, { id, label, min, max, step = 1, value, unit = '', format = (v) => v, onInput }) {
  const out = h('output', { for: id }, format(value) + unit);
  const input = h('input', { id, type: 'range', min, max, step, value });
  input.oninput = () => { out.textContent = format(Number(input.value)) + unit; onInput(Number(input.value)); };
  root.append(h('label', { class: 'ctl', for: id }, [h('span', {}, label), input, out]));
  return input;
}

export function select(root, { id, label, options, value, onInput }) {
  const sel = h('select', { id }, options.map(([v, t]) => h('option', { value: v, ...(v === value ? { selected: '' } : {}) }, t)));
  sel.onchange = () => onInput(sel.value);
  root.append(h('label', { class: 'ctl', for: id }, [h('span', {}, label), sel]));
  return sel;
}

export function toggle(root, { id, label, value = false, onInput }) {
  const box = h('input', { id, type: 'checkbox', ...(value ? { checked: '' } : {}) });
  box.onchange = () => onInput(box.checked);
  root.append(h('label', { class: 'ctl check', for: id }, [box, h('span', {}, label)]));
  return box;
}
