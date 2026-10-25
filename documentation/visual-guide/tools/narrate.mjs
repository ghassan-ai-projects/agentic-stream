import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CHAPTERS } from '../src/story.js';

// Each sentence is synthesized on its own, trimmed of edge silence and measured,
// so every caption cue (and every scene beat scheduled on it) is exact. Sentences
// are joined with a short natural gap plus the optional hold the script asks for.
const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const ENGINE = process.env.TTS ?? 'openrouter';
const MODEL = process.env.TTS_MODEL ?? 'elevenlabs/eleven-v4';
const VOICE = process.env.VOICE ?? (ENGINE === 'openrouter' ? 'george' : 'Samantha');
const RATE = process.env.RATE ?? '180';
const CACHE = join(here, '.tts-cache');
const SR = 48000;
const LEAD = 0.7;
const GAP = 0.45;
const TAIL = 0.5;
mkdirSync(CACHE, { recursive: true });
const work = mkdtempSync(join(tmpdir(), 'vg-narr-'));

function openrouterKey() {
  if (process.env.OPENROUTER_SPEECH_API_KEY) return process.env.OPENROUTER_SPEECH_API_KEY;
  const env = readFileSync(join(root, '../../.env'), 'utf8');
  const m = env.match(/OPENROUTER_SPEECH_API_KEY\s*=\s*["']?([^\s"']+)/);
  if (!m) throw new Error('OPENROUTER_SPEECH_API_KEY not found in the environment or the repository .env');
  return m[1];
}

async function synthesize(text) {
  const hash = createHash('sha256').update(`${ENGINE}|${MODEL}|${VOICE}|${ENGINE === 'openrouter' ? '' : RATE}|${text}`).digest('hex').slice(0, 24);
  const cached = join(CACHE, `${hash}.${ENGINE === 'openrouter' ? 'mp3' : 'aiff'}`);
  if (existsSync(cached)) return cached;
  if (ENGINE !== 'openrouter') { execFileSync('say', ['-v', VOICE, '-r', RATE, '-o', cached, '--', text]); return cached; }
  const res = await fetch('https://openrouter.ai/api/v1/audio/speech', {
    method: 'POST',
    headers: { Authorization: `Bearer ${openrouterKey()}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ model: MODEL, input: text, voice: VOICE, response_format: 'mp3' }),
  });
  if (!res.ok) throw new Error(`speech request failed: ${res.status} ${(await res.text()).slice(0, 200)}`);
  writeFileSync(cached, Buffer.from(await res.arrayBuffer()));
  return cached;
}

const trim = 'silenceremove=start_periods=1:start_threshold=-48dB:start_silence=0.02,areverse,silenceremove=start_periods=1:start_threshold=-48dB:start_silence=0.02,areverse';
function pcm(file) {
  const out = join(work, createHash('sha1').update(file).digest('hex') + '.f32');
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', file, '-af', trim, '-ac', '1', '-ar', String(SR), '-f', 'f32le', out]);
  const b = readFileSync(out);
  return new Float32Array(b.buffer, b.byteOffset, b.byteLength / 4);
}

const rows = [];
let cursor = 0;
for (const c of CHAPTERS) {
  const start = cursor;
  let t = start + (c.lead ?? LEAD);
  const cues = [];
  for (const [i, line] of c.lines.entries()) {
    if (i < c.lines.length - 1 && GAP + (line.hold ?? 0) > 1.4) throw new Error(`${c.id}[${i}]: a pause inside a chapter must stay under 1.5 s`);
    const samples = pcm(await synthesize(line.text));
    const dur = samples.length / SR;
    cues.push({ chapter: c.id, i, start: t, end: t + dur, text: line.text, samples });
    t += dur + GAP + (line.hold ?? 0);
  }
  const speech = cues.at(-1).end - cues[0].start;
  const duration = Math.max(c.minSeconds ?? 0, t - GAP - start + TAIL);
  rows.push({ id: c.id, start, duration, audio: speech, cues });
  cursor = start + duration;
}
const total = cursor;

const mix = new Float32Array(Math.ceil(total * SR));
for (const r of rows) for (const q of r.cues) mix.set(q.samples, Math.round(q.start * SR));
const raw = join(work, 'mix.f32');
writeFileSync(raw, Buffer.from(mix.buffer));
mkdirSync(join(root, 'film'), { recursive: true });
execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'f32le', '-ar', String(SR), '-ac', '1', '-i', raw,
  '-af', 'loudnorm=I=-16:TP=-2:LRA=9,alimiter=limit=0.84:level=false', '-ar', String(SR), '-ac', '2', '-c:a', 'aac', '-b:a', '160k', join(root, 'film/narration.m4a')]);

const captions = rows.flatMap((r) => r.cues.map(({ samples, ...q }) => q));
const stamp = (s) => { const ms = Math.round(s * 1000); const p = (n, w = 2) => String(n).padStart(w, '0'); return `${p(Math.floor(ms / 3600000))}:${p(Math.floor(ms / 60000) % 60)}:${p(Math.floor(ms / 1000) % 60)}.${p(ms % 1000, 3)}`; };
writeFileSync(join(root, 'film/captions.vtt'), 'WEBVTT\n\n' + captions.map((c, i) => `${i + 1}\n${stamp(c.start)} --> ${stamp(c.end)}\n${c.text}\n`).join('\n'));

const r3 = (n) => Number(n.toFixed(3));
const generated = {
  engine: ENGINE, model: ENGINE === 'openrouter' ? MODEL : 'macOS say', voice: VOICE, total: r3(total),
  chapters: rows.map((r) => ({ id: r.id, start: r3(r.start), duration: r3(r.duration), audio: r3(r.audio) })),
  captions: captions.map((c) => ({ ...c, start: r3(c.start), end: r3(c.end) })),
};
writeFileSync(join(root, 'src/timeline.generated.js'), `export const GENERATED = ${JSON.stringify(generated, null, 2)};\n`);
rmSync(work, { recursive: true, force: true });
console.log(`${ENGINE} ${ENGINE === 'openrouter' ? MODEL : ''} voice ${VOICE}: ${rows.length} chapters, ${captions.length} cues, total ${total.toFixed(1)} s`);
for (const r of rows) console.log(r.id.padEnd(10), 'start', r.start.toFixed(1).padStart(6), 'speech', r.audio.toFixed(1).padStart(5), 'dur', r.duration.toFixed(1));
