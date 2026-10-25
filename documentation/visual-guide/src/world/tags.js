import * as THREE from 'three';

const v = new THREE.Vector3();
const OBSTACLES = ['.hud', '.chip', '.brand', '.legend'];

export function auditTags(w, h) {
  const out = [];
  const boxes = [];
  for (const el of document.querySelectorAll('.wtag')) {
    if (el.style.visibility === 'hidden' || Number(el.style.opacity) < 0.35) continue;
    boxes.push({ name: 'tag:' + el.textContent.slice(0, 28), r: el.getBoundingClientRect() });
  }
  for (const sel of [...OBSTACLES, '.caption.on', '.panel:not(.closed)', '.controls']) {
    const el = document.querySelector(sel);
    if (el) boxes.push({ name: sel, r: el.getBoundingClientRect(), fixed: true });
  }
  const area = (a, b) => Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left)) * Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
  for (let i = 0; i < boxes.length; i++) {
    const a = boxes[i];
    if (!a.fixed && (a.r.left < -2 || a.r.top < -2 || a.r.right > w + 2 || a.r.bottom > h + 2)) out.push('clipped ' + a.name);
    for (let j = i + 1; j < boxes.length; j++) {
      const b = boxes[j];
      if (a.fixed && b.fixed) continue;
      if (area(a.r, b.r) > 60) out.push(a.name + ' x ' + b.name);
    }
  }
  return out;
}

const overlap = (a, b, m = 0) => a.x < b.x + b.w + m && a.x + a.w + m > b.x && a.y < b.y + b.h + m && a.y + a.h + m > b.y;

export const fallbacks = { dropped: 0, snapped: 0 };

export function createTags(root) {
  const els = new Map();
  const offsets = new Map();
  let frame = new Map();
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('class', 'leaders');
  root.parentNode.insertBefore(svg, root);

  function obstacles(w, h) {
    const list = [];
    for (const sel of OBSTACLES) {
      const el = document.querySelector(sel);
      if (!el) continue;
      const r = el.getBoundingClientRect();
      if (r.width) list.push({ x: r.left, y: r.top, w: r.width, h: r.height });
    }
    const panel = document.querySelector('.panel:not(.closed)');
    if (panel) { const r = panel.getBoundingClientRect(); list.push({ x: r.left - 8, y: 0, w: r.width + 8, h }); }
    const film = document.documentElement.classList.contains('film');
    const reserve = film ? 190 : 170;
    return { list, bottom: h - reserve, top: film ? 96 : 100 };
  }

  return {
    begin() { frame = new Map(); },
    put(key, pos, html, vis = 1, kind = 'tag') { if (vis > 0.02) frame.set(key, { pos: pos.clone ? pos.clone() : pos, html, vis, kind }); },
    end(camera, w, h) {
      for (const [key, el] of els) if (!frame.has(key)) { el.style.opacity = '0'; el.style.visibility = 'hidden'; offsets.delete(key); }
      const placed = [];
      const { list, bottom, top } = obstacles(w, h);
      const items = [];
      for (const [key, t] of frame) {
        v.copy(t.pos).project(camera);
        const off = v.z > 1 || Math.abs(v.x) > 1.4 || Math.abs(v.y) > 1.4;
        let el = els.get(key);
        if (!el) { el = document.createElement('div'); el.className = `wtag ${t.kind}`; root.appendChild(el); els.set(key, el); }
        if (el.__html !== t.html) { el.innerHTML = t.html; el.__html = t.html; el.__w = el.offsetWidth; el.__h = el.offsetHeight; }
        if (!el.__w) { el.__w = el.offsetWidth; el.__h = el.offsetHeight; }
        if (off) { el.style.visibility = 'hidden'; continue; }
        items.push({ key, t, el, ax: (v.x + 1) / 2 * w, ay: (1 - v.y) / 2 * h });
      }
      const order = (it) => (/\b(name)\b/.test(it.t.kind) ? 0 : /\b(value|truthbox|card)\b/.test(it.t.kind) ? 1 : 2);
      items.sort((a, b) => order(a) - order(b));
      svg.setAttribute('width', w); svg.setAttribute('height', h);
      let lines = '';
      for (const it of items) {
        const { el, t } = it;
        const bw = el.__w, bh = el.__h;
        const base = { x: it.ax - bw / 2, y: it.ay - bh - 10, w: bw, h: bh };
        let best = null;
        const free = (r) => r.x >= 10 && r.x + r.w <= w - 10 && r.y >= top && r.y + r.h <= bottom && !list.some((o) => overlap(r, o, 6)) && !placed.some((p) => overlap(r, p, 8));
        if (free(base)) best = base;
        else {
          let bestD = Infinity;
          for (let dy = -9; dy <= 9; dy++) for (let dx = -12; dx <= 12; dx++) {
            const r = { x: base.x + dx * 36, y: base.y + dy * (bh * 0.55 + 6), w: bw, h: bh };
            if (!free(r)) continue;
            const d = dx * dx * 0.55 + dy * dy * 1.2;
            if (d < bestD) { bestD = d; best = r; }
          }
        }
        if (!best) { el.style.visibility = 'hidden'; offsets.delete(it.key); fallbacks.dropped++; continue; }
        const prev = offsets.get(it.key);
        const target = { x: best.x, y: best.y };
        const cur = prev ? { x: prev.x + (target.x - prev.x) * 0.45, y: prev.y + (target.y - prev.y) * 0.45 } : target;
        if (prev && Math.hypot(target.x - prev.x, target.y - prev.y) < 0.6) { cur.x = target.x; cur.y = target.y; }
        const curBox = { ...cur, w: bw, h: bh };
        if (placed.some((p) => overlap(curBox, p, 2)) || list.some((o) => overlap(curBox, o, 2))) { cur.x = target.x; cur.y = target.y; fallbacks.snapped++; }
        offsets.set(it.key, cur);
        placed.push({ x: cur.x, y: cur.y, w: bw, h: bh });
        el.style.visibility = 'visible';
        el.style.opacity = String(t.vis);
        el.style.transform = `translate(${cur.x.toFixed(1)}px, ${cur.y.toFixed(1)}px)`;
        const cx = Math.min(Math.max(it.ax, cur.x + 10), cur.x + bw - 10);
        const cy = it.ay > cur.y + bh ? cur.y + bh : cur.y;
        if (Math.hypot(it.ax - cx, it.ay - cy) > 14 && t.vis > 0.3) lines += `<line x1="${it.ax.toFixed(1)}" y1="${it.ay.toFixed(1)}" x2="${cx.toFixed(1)}" y2="${cy.toFixed(1)}" opacity="${(t.vis * 0.7).toFixed(2)}"/><circle cx="${it.ax.toFixed(1)}" cy="${it.ay.toFixed(1)}" r="3.2" opacity="${t.vis.toFixed(2)}"/>`;
      }
      svg.innerHTML = lines;
    },
  };
}
