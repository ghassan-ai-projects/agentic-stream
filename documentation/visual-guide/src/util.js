export const clamp = (x, a = 0, b = 1) => Math.min(b, Math.max(a, x));
export const lerp = (a, b, t) => a + (b - a) * t;
export const smooth = (x) => { const t = clamp(x); return t * t * (3 - 2 * t); };
export const easeInOut = (x) => { const t = clamp(x); return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2; };
export const easeOut = (x) => 1 - Math.pow(1 - clamp(x), 3);
export const ramp = (T, a, b) => smooth((T - a) / (b - a));
export const window01 = (T, a, b, fade = 0.4) => ramp(T, a, a + fade) * (1 - ramp(T, b - fade, b));
export const pulse = (T, at, width = 0.6) => Math.exp(-Math.pow((T - at) / width, 2));
export const decay = (T, at, tau = 0.5) => (T < at ? 0 : Math.exp(-(T - at) / tau));

export function rng(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export const fmt = (n, d = 1) => Number(n).toFixed(d);
