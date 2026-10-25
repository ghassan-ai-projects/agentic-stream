import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const NAV = [['time', 'Time'], ['situation', 'Situation'], ['scheduler', 'Reasoning gate'], ['authority', 'Authority']];

const LABS = {
  time: {
    title: 'Time and windows', kicker: 'Lab 1 · Stream plane', h1: 'A late reading cannot rewrite history',
    lede: 'Sensors send readings. Networks reorder them. The runtime keeps two clocks apart: when something happened (event time) and when it was heard about. Delay a reading and watch the watermark, the window and the result react.',
    cards: [
      ['Reading the picture', 'Two lanes, two clocks', 'Twelve temperature readings from the recorded run sit on two lanes. The top lane is arrival order, the runtime\'s own clock. The bottom lane is when each one really happened. A line joins them once a reading arrives.'],
      ['Key idea', 'The watermark is a promise', 'The watermark is the newest event time seen minus a lag. When it passes the end of a window, the window closes and emits a result. A reading that arrives afterwards is late, and the spec says what to do with it.'],
      ['Why it matters', 'Same input, same state', 'If results followed arrival order, a slow network would change the answer. Event time makes the state a function of the evidence alone, which is what makes deterministic replay possible.'],
    ],
    note: 'The readings, their event times, the 5-minute window, the 15 s watermark lag and the 30 s allowed lateness come from the recorded run and the experiment spec. The network delay and the +6 °C spike are what-ifs you control; the recorded readings were not late.',
    src: [['Time and changing state', '../../learn/time-and-state.md'], ['Stream mechanics', '../../design/stream-processing.md']],
  },
  situation: {
    title: 'Situation and hysteresis', kicker: 'Lab 2 · Stream plane', h1: 'A condition changes state, not its mind every second',
    lede: 'A noisy sensor crosses a threshold again and again. A Situation should not flap with it. Compare a naive single threshold with a spec-style rule (separate enter and exit thresholds, plus a minimum duration), and see why every change becomes an immutable, numbered version.',
    cards: [
      ['Reading the picture', 'Two machines, one signal', 'One synthetic signal drives both state machines. The strips under the chart show each machine\'s state over time and count its flips. Each flip of the spec machine publishes a version into the stack.'],
      ['Key idea', 'Hysteresis and minimum duration', 'Enter above 30 °C, but only leave at or below 28 °C, and only believe a new state after it has held for a minimum duration. Small wobbles stop mattering.'],
      ['Why it matters', 'Versions are facts, not drafts', 'A published version never changes. A correction publishes a new one, so you can always reconstruct what the system believed at the time, and every later step can point at exactly that version.'],
    ],
    note: 'The signal is synthetic, for illustration. The 30 °C enter and 28 °C exit thresholds and the minimum duration default come from the experiment\'s bench spec (examples/real-world-sensor). Use the button to show the 22 versions of the recorded run instead.',
    src: [['What a Situation represents', '../../learn/domain-model.md'], ['Time and changing state', '../../learn/time-and-state.md']],
  },
  scheduler: {
    title: 'The reasoning gate', kicker: 'Lab 3 · Cognition plane', h1: 'Twenty-two versions, one episode',
    lede: 'Reasoning is slow, costly and not repeatable, so a deterministic scheduler decides when it is worth starting. Replay the 22 real evaluations, then change the rules and the sensor\'s pace to see how the gate behaves.',
    cards: [
      ['Reading the picture', 'Evaluations, queue, episodes', 'The top row is every version with its score and outcome. The middle row is queued work with its not-before time. The bottom row is the episodes that actually started.'],
      ['Key idea', 'Score, newness, then timing', 'A version must score above the threshold and be materially new. Queued work is delayed by a debounce, spaced by a cooldown, and replaced by newer work for the same trigger.'],
      ['Why it matters', 'Rationed attention', 'Every outcome, ignored ones included, is a durable record with a reason. You can always answer the question "why did nothing happen?".'],
    ],
    note: 'This is a simplified model of the scheduler. With the recorded settings it reproduces the recorded run: 22 evaluations, version 5 replaced by version 7, one episode about 21 s after the first admitted evaluation. The recorded run was fast-forwarded, so versions arrived a fraction of a second apart. For a live sensor, 30 s apart, the model predicts a second episode.',
    src: [['When an agent should reason', '../../learn/reasoning.md'], ['Cognition mechanics', '../../design/cognition.md']],
  },
  authority: {
    title: 'From proposal to proof', kicker: 'Lab 4 · Governance and effect planes', h1: 'A proposal is not permission, and "OK" is not proof',
    lede: 'Send a proposed action down the real path: validator, policy, outbox and dispatch, device, verification. Flip the conditions of the world and watch where it stops, what record is written, and what a lost reply does.',
    cards: [
      ['Reading the picture', 'Gates, records, outcome', 'Each gate either passes the proposal or stops it and writes a durable record with a reason. The chips under the picture are those records, in order.'],
      ['Key idea', 'Check again, right before acting', 'Policy is evaluated against current state at dispatch. A superseded intent, an operator stop or a new runtime owner can turn an earlier yes into a no.'],
      ['Why it matters', 'Unknown is a real answer', 'If a reply is lost, the system records an unknown outcome and reconciles with evidence. A stable idempotency key means a retry cannot cause a second effect.'],
    ],
    note: 'The gate order, the risk classes (R0, R1, R2), the human approval rule for R2, and the three answers (receipt, result, query_state) follow the design and the recorded run. The failure switches are what-ifs; the recorded run passed every gate.',
    src: [['From proposal to effect', '../../learn/safe-actions.md'], ['Decision and action mechanics', '../../design/decisions-and-actions.md'], ['Security model', '../../architecture/security-model.md']],
  },
};

for (const [name, p] of Object.entries(LABS)) {
  const nav = NAV.map(([k, t]) => `<a href="${k}.html"${k === name ? ' aria-current="page"' : ''}>${t}</a>`).join('');
  const cards = p.cards.map(([tag, h, body]) => `<div class="card"><div class="tag">${tag}</div><h3>${h}</h3><p>${body}</p></div>`).join('');
  const src = p.src.map(([t, u]) => `<li><a href="${u}">${t}</a></li>`).join('');
  writeFileSync(join(root, `labs/${name}.html`), `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${p.title} — Agentic Stream</title>
<link rel="stylesheet" href="lab.css">
</head>
<body>
<header class="top"><a class="brand" href="../index.html">Agentic Stream</a>
<nav aria-label="Visual guide"><a href="../index.html">Home</a><a href="../world.html">3D world</a>${nav}</nav></header>
<main>
<div class="kicker">${p.kicker}</div>
<h1>${p.h1}</h1>
<p class="lede">${p.lede}</p>
<div class="stage" id="stage"></div>
<div class="chips" id="chips" aria-live="polite"></div>
<div class="controls"><div class="transport" id="transport"></div><div class="group" id="ctl"></div></div>
<div class="cards">${cards}</div>
<div class="note">${p.note}</div>
<footer class="src">Read more<ul>${src}</ul></footer>
</main>
<script src="../dist/lab-${name}.js"></script>
</body>
</html>
`);
}
console.log('wrote', Object.keys(LABS).length, 'lab pages');

const run = JSON.parse((await import('node:fs')).readFileSync(join(root, 'src/data/run-data.json'), 'utf8'));
const { GENERATED } = await import('../src/timeline.generated.js');
const FILM = 'film/agentic-stream-explained.mp4';
const filmLength = `${Math.floor(GENERATED.total / 60)} min ${Math.round(GENERATED.total % 60)} s`;
const esc = (t) => String(t).replace(/&/g, '&amp;').replace(/</g, '&lt;');
const PLANES = [
  ['evidence', '#4cc9f0', 'Evidence', 'What happened?', 'Observations enter, are checked against a schema and are kept in an append-only log.', 'Never decides what an observation means, never triggers an action.', ['gateway', 'log']],
  ['stream', '#9b8cff', 'Stream', 'What is changing?', 'Event-time windows, features and the versioned Situation, built deterministically.', 'Never calls a model, never touches a device.', ['engine', 'situation']],
  ['cognition', '#f5b94a', 'Cognition', 'Is thinking worth it, and what do we propose?', 'A scheduler that rations reasoning, and bounded episodes where the reasoner reads evidence and proposes.', 'Never mutates stream state, never executes a command.', ['scheduler', 'episode']],
  ['governance', '#34d399', 'Governance', 'Is this proposal valid and permitted now?', 'Pure validation against the spec\'s catalog, then policy re-checked against current state.', 'Never reasons, never calls a provider.', ['validator', 'policy']],
  ['effect', '#fb7185', 'Effect', 'Did the permitted change really happen?', 'Idempotent commands, an outbox, the device boundary and verification of the outcome.', 'Never turns a decision into a different intent.', ['action', 'bench']],
];
const BLURB = { time: 'Delay a reading and watch the watermark, the window and the result react.', situation: 'Compare a naive threshold with hysteresis and a minimum duration, then inspect immutable versions.', scheduler: 'Replay the 22 real evaluations and change the rules that decide when to think.', authority: 'Flip the conditions of the world and watch where a proposal stops, and what a lost reply does.' };
const NAMES = { bench: 'Sensor bench', gateway: 'Gateway', log: 'Ingress and event log', engine: 'Stream engine', situation: 'Situation', scheduler: 'Cognitive scheduler', episode: 'Episode', validator: 'Decision validator', policy: 'Policy', action: 'Action dispatch' };
const GLOSSARY = [
  ['Event', 'One observation with an identity, a source, an event time and a typed payload. Evidence, never an instruction.'],
  ['Event time and watermark', 'Event time is when something happened. The watermark says how far time has progressed, so windows know when they can close.'],
  ['Window and operator', 'A window selects readings (here: the last 5 minutes, sliding every 30 s). An operator turns them into a feature such as a mean or a slope.'],
  ['Situation and version', 'The durable record of one condition of one thing. Each publication is a numbered version that never changes afterwards.'],
  ['Trigger and scheduler', 'Declared rules that say when a version deserves reasoning. The scheduler can ignore, queue, replace or admit it.'],
  ['Episode and fence', 'One bounded reasoning session over one immutable snapshot, with a deadline and a budget. The fence rejects output from a stale attempt.'],
  ['Decision and Intent', 'The reasoner\'s typed answer, and the typed action it proposes. A proposal, not a permission.'],
  ['Policy and approval', 'Rules re-checked against current state just before dispatch. Higher-risk intents wait for a signed human approval.'],
  ['Command and idempotency key', 'The governed request that may be dispatched. A retry carries the same key, so a device that honours it recognises the repeat instead of acting twice.'],
  ['Outcome and verification', 'What the device reported, and a separate state query of what the output is now. Only a matching observation verifies the effect. "Unknown" is a legal outcome.'],
];

const strip = [['events', run.events.total, 'events'], ['versions', run.versions.length, 'Situation versions'], ['episodes', run.run.rows.episodes, 'episode'], ['decisions', run.run.rows.decisions, 'decision'], ['commands', run.run.rows.commands, 'command'], ['verified', run.run.rows.verifications, 'verified effect']];
const indexHtml = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Agentic Stream — visual guide</title>
<meta name="description" content="A narrated film, an interactive 3D world and four labs that explain how Agentic Stream turns evidence into governed action, using one real recorded run.">
<link rel="stylesheet" href="labs/lab.css">
<link rel="stylesheet" href="index.css">
</head>
<body>
<header class="top"><a class="brand" href="index.html">Agentic Stream</a>
<nav aria-label="Visual guide"><a href="index.html" aria-current="page">Home</a><a href="world.html">3D world</a>${NAV.map(([k, t]) => `<a href="labs/${k}.html">${t}</a>`).join('')}</nav></header>
<main>
<section class="hero">
<div class="kicker">Visual guide</div>
<h1>Follow one decision from a warming sensor to a verified effect</h1>
<p class="lede">Agentic Stream keeps track of a condition, thinks only when thinking is worth it, and acts only when it is permitted. This guide explains every step using one real recorded run. Watch it, walk through it in 3D, then play with the rules.</p>
<div class="cta"><a class="big" href="#film">Watch the film (${filmLength})</a><a class="big ghost" href="world.html">Explore the 3D world</a></div>
<ol class="strip" aria-label="The recorded run in numbers">${strip.map(([, n, l], i) => `<li><b>${n}</b><span>${l}</span></li>`).join('')}</ol>
<p class="fine">Recorded run <code>${run.run.id}</code>: ${esc(run.run.kind)}. Verdict: <b>${run.run.verdict}</b>, every zero-tolerance counter is 0.</p>
</section>

<section id="film" class="block">
<div class="kicker">The film</div>
<h2>The whole path, step by step</h2>
<div class="video"><video controls preload="metadata" poster="film/poster.jpg" playsinline><source src="${FILM}" type="video/mp4"><track kind="captions" src="film/captions.vtt" srclang="en" label="English" default>Your browser cannot play this video. Open <a href="${FILM}">the MP4</a> directly.</video></div>
<p class="fine">${filmLength}, narrated, with captions. The same scene is live in the 3D world, where you can pause, scrub, orbit and open any station.</p>
</section>

<section class="block">
<div class="kicker">The map</div>
<h2>Five planes, each with one job</h2>
<p class="lede small">Work moves through five planes. Each owns one question and is forbidden from doing the others' jobs. That separation is the safety design.</p>
<div class="planes">${PLANES.map(([k, c, n, q, owns, never, st]) => `<article class="plane" style="--c:${c}"><h3>${n}</h3><div class="q">${q}</div><p>${owns}</p><p class="never"><b>Never:</b> ${never}</p><div class="stlinks">${st.map((id) => `<a href="world.html?station=${id}">${NAMES[id]}</a>`).join('')}</div></article>`).join('')}</div>
</section>

<section class="block">
<div class="kicker">Hands on</div>
<h2>Four labs</h2>
<div class="labs">${NAV.map(([k, t], i) => `<a class="lab" href="labs/${k}.html"><span class="n">${i + 1}</span><h3>${LABS[k].h1}</h3><p>${BLURB[k]}</p><span class="go">Open the lab →</span></a>`).join('')}</div>
</section>

<section class="block">
<div class="kicker">Vocabulary</div>
<h2>Ten words that carry the design</h2>
<dl class="gloss">${GLOSSARY.map(([t, d]) => `<div><dt>${t}</dt><dd>${d}</dd></div>`).join('')}</dl>
</section>

<section class="block honest">
<div class="kicker">What is proven</div>
<h2>Be precise about what this run shows</h2>
<ul>
<li><b>Proven in the recorded run:</b> events to Situation, scheduler, one episode, a typed decision, policy approval, one governed command (<code>set_indicator</code>, risk R1), device receipt and result, and a verified state check.</li>
<li><b>Not physical hardware:</b> a simulator played the sensor bench and a device emulator answered commands behind the gateway socket. The 3D bench and gateway show the designed physical setup, labelled as emulated.</li>
<li><b>Designed, not part of this run:</b> the fan path (<code>select_thermal_mode</code>, risk R2) with signed human approval. It is shown in the film and the 3D world and labelled as such.</li>
<li><b>Known blind spots recorded with the run:</b></li>
</ul>
<ul class="spots">${run.run.blindSpots.map((b) => `<li>${esc(b)}</li>`).join('')}</ul>
</section>
<footer class="src">Regenerate: <code>tools/extract-run-data.mjs</code> reads the recorded run, <code>tools/narrate.mjs</code> builds narration, captions and timing, <code>tools/build.sh</code> bundles the pages, <code>tools/render-film.mjs</code> renders the film. See <a href="README.md">README</a> and <a href="BAR.md">the quality bar</a>.</footer>
</main>
</body>
</html>
`;
writeFileSync(join(root, 'index.html'), indexHtml);
console.log('wrote index.html');
