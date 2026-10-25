import { CHAPTERS } from './story.js';
import { GENERATED } from './timeline.generated.js';
import { clamp } from './util.js';

const byId = Object.fromEntries(GENERATED.chapters.map((c) => [c.id, c]));

export const chapters = CHAPTERS.map((c, i) => {
  const g = byId[c.id];
  return { ...c, index: i, start: g.start, dur: g.duration, end: g.start + g.duration, audio: g.audio };
});
export const TOTAL = GENERATED.total;
export const captions = GENERATED.captions;
export const ch = Object.fromEntries(chapters.map((c) => [c.id, c]));

const cues = {};
for (const c of captions) (cues[c.chapter] ??= [])[c.i] = c;

// The spoken sentence `i` of a chapter: { start, end, text }. Scene beats are
// scheduled on these, so picture and voice stay locked when the narration changes.
export function cue(id, i) {
  const c = cues[id]?.[i];
  if (!c) throw new Error(`no narration cue ${id}[${i}]`);
  return c;
}

// The moment a word is spoken inside a sentence, estimated from its character
// position (a sentence is read at an even pace, so this lands within a syllable or two).
export function word(id, i, needle, nth = 0) {
  const c = cue(id, i);
  let at = -1;
  for (let k = 0; k <= nth; k++) at = c.text.indexOf(needle, at + 1);
  if (at < 0) throw new Error(`"${needle}" is not in ${id}[${i}]`);
  return c.start + (at / c.text.length) * (c.end - c.start);
}

export function chapterAt(T) {
  const t = clamp(T, 0, TOTAL - 1e-6);
  return chapters.find((c) => t >= c.start && t < c.end) ?? chapters.at(-1);
}

export function captionAt(T) {
  return captions.find((c) => T >= c.start - 0.05 && T < c.end + 0.35) ?? null;
}
