import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');

const run = JSON.parse(readFileSync(join(root, 'src/data/run-data.json'), 'utf8'));
const { GENERATED } = await import('../src/timeline.generated.js');
const ONES = ['zero', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten', 'eleven', 'twelve', 'thirteen', 'fourteen', 'fifteen', 'sixteen', 'seventeen', 'eighteen', 'nineteen'];
const TENS = ['', '', 'twenty', 'thirty', 'forty', 'fifty', 'sixty', 'seventy', 'eighty', 'ninety'];
const words = (n) => (n < 20 ? ONES[n] : `${TENS[Math.floor(n / 10)]}${n % 10 ? `-${ONES[n % 10]}` : ''}`);
const cap = (t) => t[0].toUpperCase() + t.slice(1);
const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;
const { spec } = run;
const V = run.versions.length;
const temps = run.events.series.filter((e) => e.type === 'temp').length;
const [firstQueued, secondQueued] = run.scheduler;
const firstAdmitted = run.triggers.find((t) => t.outcome === 'admitted');
const episodeDelay = Math.round((run.episode.acceptedMs - firstAdmitted.wallMs) / 1000);
const lastWallSeconds = Math.max(...run.triggers.map((t) => t.wallMs)) / 1000;
const minutes = (seconds) => `${seconds / 60}-minute`;
const hold = (from, to) => spec.transitions.find((t) => t.from === from && t.to === to).minDurationSeconds;

const NAV = [['time', 'Time'], ['situation', 'Situation'], ['scheduler', 'Reasoning gate'], ['authority', 'Authority']];

const LABS = {
  time: {
    title: 'Time and windows', kicker: 'Lab 1 · Stream plane', h1: 'A late reading cannot rewrite history',
    lede: 'Sensors send readings. Networks reorder them. The runtime keeps two clocks apart: when something happened (event time) and when it was heard about. Delay a reading and watch the watermark, the window and the result react.',
    cards: [
      ['Reading the picture', 'Two lanes, two clocks', `${cap(words(temps))} temperature readings from the recorded run sit on two lanes. The top lane is arrival order, the runtime's own clock. The bottom lane is when each one really happened. A line joins them once a reading arrives.`],
      ['Key idea', 'The watermark is a promise', 'The watermark is the newest event time seen minus a lag. When it passes the end of a window, the window closes and emits a result. A reading that arrives afterwards is late, and the spec says what to do with it.'],
      ['Why it matters', 'Same input, same state', 'If results followed arrival order, a slow network would change the answer. Event time makes the state a function of the evidence alone, which is what makes deterministic replay possible.'],
    ],
    terms: [
      ['Event time', 'When the sensor took the reading. Windows and results are placed by it.'],
      ['Watermark', `The runtime's promise that nothing older than this event time is still expected. Here: the newest event time seen minus ${spec.watermarkLagSeconds} s.`],
      ['Allowed lateness', `How long after a window closes a late reading may still correct it (${spec.allowedLatenessSeconds} s here).`],
      ['on_time, corrected', 'on_time: the result was emitted when the window closed. corrected: a later reading inside the allowed lateness produced a new result.'],
    ],
    note: `The readings, their event times, the ${minutes(spec.windowSeconds)} window, the ${spec.watermarkLagSeconds} s watermark lag and the ${spec.allowedLatenessSeconds} s allowed lateness come from the recorded run and its spec. The network delay and the +6 °C spike are what-ifs you control; the recorded readings were not late.`,
    src: [['Time and changing state', '../../learn/time-and-state.md'], ['Stream mechanics', '../../design/stream-processing.md']],
  },
  situation: {
    title: 'Situation and hysteresis', kicker: 'Lab 2 · Stream plane', h1: 'A condition changes state, not its mind every second',
    lede: 'A noisy sensor crosses a threshold again and again. A Situation should not flap with it. Compare a naive single threshold with a spec-style rule (separate enter and exit thresholds, so the line it must cross to leave is not the line it crossed to enter, plus a minimum duration), and see why every change becomes an immutable, numbered version.',
    cards: [
      ['Reading the picture', 'Two machines, one signal', 'One synthetic signal drives both state machines. The strips under the chart show each machine\'s state over time and count its flips. Each flip of the spec machine publishes a version into the stack.'],
      ['Key idea', 'Hysteresis and minimum duration', `Enter above ${spec.enterCelsius} °C, but only leave at or below ${spec.exitCelsius} °C, and only believe a new state after it has held for a minimum duration. Small wobbles stop mattering. This lab applies the hold to leaving as well as entering, as the spec's rules do.`],
      ['Why it matters', 'Versions are facts, not drafts', 'A published version never changes. A correction publishes a new one, so you can always reconstruct what the system believed at the time, and every later step can point at exactly that version.'],
    ],
    terms: [
      ['Hysteresis', 'Using one threshold to enter a state and a different one to leave it, so a signal hovering near a line cannot flip the state back and forth.'],
      ['Minimum duration', 'How long a condition must keep holding before the state changes. A brief spike does not count.'],
      ['quiet, pending, open, recovering', `The lab's four states. The spec's own phases are finer: candidate, watch, cooling, recovering, resolved. Quiet is "no occurrence". Pending is the candidate phase waiting out its ${hold('candidate', 'watch')} s hold. Open stands for watch and cooling. Recovering waits ${hold('recovering', 'resolved')} s at or below the exit threshold before resolving.`],
      ['Provisional, on-time', 'Each window step publishes an early (provisional) version, then an on-time one when the window closes.'],
    ],
    note: `The signal is synthetic, for illustration. The ${spec.enterCelsius} °C enter and ${spec.exitCelsius} °C exit thresholds and the holds (${hold('candidate', 'watch')} s to watch, ${hold('watch', 'cooling')} s to cooling, ${hold('recovering', 'resolved')} s to resolve) come from the recorded run's spec. Use the button to show the ${V} versions of the recorded run instead. In that run the window's mean was already above ${spec.enterCelsius} °C at its first version, so the enter threshold is never crossed and the mean never falls to the exit threshold: the run shows only the slope-driven phase changes.`,
    src: [['What a Situation represents', '../../learn/domain-model.md'], ['Time and changing state', '../../learn/time-and-state.md']],
  },
  scheduler: {
    title: 'The reasoning gate', kicker: 'Lab 3 · Cognition plane', h1: `${cap(words(V))} versions, ${words(run.run.rows.episodes)} ${run.run.rows.episodes === 1 ? 'episode' : 'episodes'}`,
    lede: `Reasoning is slow, costly and not repeatable, so a deterministic scheduler decides when it is worth starting. Replay the ${V} real evaluations, then change the rules and the sensor's pace to see how the gate behaves.`,
    cards: [
      ['Reading the picture', 'Evaluations, queue, episodes', 'The top row is every version with its score and outcome. The middle row is queued work with its not-before time. The bottom row is the episodes that actually started.'],
      ['Key idea', 'Score, newness, then timing', 'A version must score above the threshold and be materially new. Queued work is delayed by a debounce, spaced by a cooldown, and replaced by newer work for the same trigger.'],
      ['Why it matters', 'Rationed attention', 'Every outcome, ignored ones included, is a durable record with a reason. You can always answer the question "why did nothing happen?".'],
    ],
    terms: [
      ['Debounce', 'A short wait before queued work may start, so a burst of similar versions collapses into the latest one.'],
      ['Cooldown', 'The minimum gap between two admissions for the same trigger.'],
      ['Replaced (coalesced)', 'Queued work for a trigger is swapped for newer work instead of running twice.'],
      ['Material change', 'A version is worth attention only if something real changed: the phase, the severity, or the hypothesis.'],
    ],
    note: `This is a simplified model of the scheduler. With the recorded settings it reproduces the recorded run: ${V} evaluations, version ${firstQueued.v} replaced by version ${secondQueued.v}, one episode about ${episodeDelay} s after the first admitted evaluation. The recorded run was fast-forwarded, so all ${V} versions arrived within ${lastWallSeconds.toFixed(1)} s. For a live sensor, ${spec.slideSeconds} s apart, the model predicts a second episode.`,
    src: [['When an agent should reason', '../../learn/reasoning.md'], ['Cognition mechanics', '../../design/cognition.md']],
  },
  authority: {
    title: 'From proposal to proof', kicker: 'Lab 4 · Governance and effect planes', h1: 'A proposal is not permission, and "OK" is not proof',
    lede: 'Send a proposed action down the real path: validator, policy, outbox and dispatch, device, verification. Flip the conditions of the world and watch where it stops, what record is written, and what a lost reply does.',
    cards: [
      ['Reading the picture', 'Gates, records, outcome', 'Each gate either passes the proposal or stops it and writes a durable record with a reason. The chips under the picture are those records, in order.'],
      ['Key idea', 'Check again, right before acting', 'Policy is evaluated against current state at dispatch. A superseded intent is marked stale, an engaged operator stop is denied, and a runtime that lost ownership is fenced from writing.'],
      ['Why it matters', 'Unknown is a real answer', 'If a reply is lost, the system records an unknown outcome and reconciles with evidence. A stable idempotency key lets a provider that honours it recognise a repeat as the same request. A command with nothing to read back ends in manual review, and a refused command closes its verification as refuted.'],
    ],
    terms: [
      ['Outbox', 'A durable record written before a command is sent, so a crash cannot lose it and a retry finds it.'],
      ['Fence', 'A number that rises with each owner of the runtime. A write from an older owner carries a stale number and is refused.'],
      ['Idempotency key', 'A stable id on a command. A provider that honours it treats a repeat as the same request.'],
      ['Receipt, result, query_state', 'The three answers: the device accepted it, the device says what happened, and a separate read of the output\'s state.'],
      ['Refuted, manual review', 'Refuted: the command was refused or failed, so no effect exists to verify. Manual review: the outcome is unknown and nothing could ever be read back, so a person decides.'],
    ],
    note: 'The gate order, the risk classes (R0, R1, R2), the human approval rule for R2, and the three answers (receipt, result, query_state) follow the design and the recorded run. The failure switches are what-ifs; the recorded run passed every gate. The ticket proposal is the rotating-machinery example\'s create_maintenance_ticket.',
    src: [['From proposal to effect', '../../learn/safe-actions.md'], ['Decision and action mechanics', '../../design/decisions-and-actions.md'], ['Security model', '../../architecture/security-model.md']],
  },
};

for (const [name, p] of Object.entries(LABS)) {
  const nav = NAV.map(([k, t]) => `<a href="${k}.html"${k === name ? ' aria-current="page"' : ''}>${t}</a>`).join('');
  const cards = p.cards.map(([tag, h, body]) => `<div class="card"><div class="tag">${tag}</div><h3>${h}</h3><p>${body}</p></div>`).join('');
  const terms = p.terms.map(([t, d]) => `<div><dt>${t}</dt><dd>${d}</dd></div>`).join('');
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
<div class="chips" id="chips"></div>
<div class="controls"><div class="transport" id="transport"></div><div class="group" id="ctl"></div></div>
<div class="cards">${cards}</div>
<section class="terms" aria-label="Words used in this lab"><h2>Words used here</h2><dl>${terms}</dl></section>
<div class="note">${p.note}</div>
<footer class="src">Read more<ul>${src}</ul></footer>
</main>
<script src="../dist/lab-${name}.js"></script>
</body>
</html>
`);
}
console.log('wrote', Object.keys(LABS).length, 'lab pages');

const FILM = 'film/agentic-stream-explained.mp4';
const totalSeconds = Math.round(GENERATED.total);
const filmLength = `${Math.floor(totalSeconds / 60)} min ${totalSeconds % 60} s`;
const esc = (t) => String(t).replace(/&/g, '&amp;').replace(/</g, '&lt;');
const PLANES = [
  ['evidence', '#4cc9f0', 'Evidence', 'What happened?', 'Observations enter, are checked against a schema and are kept in an append-only log.', 'Never decides what an observation means, never triggers an action.', ['bench', 'gateway', 'log']],
  ['stream', '#9b8cff', 'Stream', 'What is changing?', 'Event-time windows, features and the versioned Situation, built deterministically.', 'Never calls a model, never touches a device.', ['engine', 'situation']],
  ['cognition', '#f5b94a', 'Cognition', 'Is thinking worth it, and what do we propose?', 'A scheduler that rations reasoning, and bounded episodes where a reasoner reads evidence and proposes: the Tamoz model worker in this run, or, by design, an approved playbook rule that answers a recurring case with no model call.', 'Never mutates stream state, never executes a command.', ['scheduler', 'episode']],
  ['governance', '#34d399', 'Governance', 'Is this proposal valid and permitted now?', 'Pure validation against the spec\'s catalog, then policy re-checked against current state.', 'Never reasons, never calls a provider.', ['validator', 'policy']],
  ['effect', '#fb7185', 'Effect', 'Did the permitted change really happen?', 'Idempotent commands, an outbox, the device boundary and verification of the outcome.', 'Never turns a decision into a different intent.', ['action']],
];
const BLURB = { time: 'Delay a reading and watch the watermark, the window and the result react.', situation: 'Compare a naive threshold with hysteresis and a minimum duration, then inspect immutable versions.', scheduler: `Replay the ${V} real evaluations and change the rules that decide when to think.`, authority: 'Flip the conditions of the world and watch where a proposal stops, and what a lost reply does.' };
const NAMES = { bench: 'Sensor bench', gateway: 'Gateway', log: 'Ingress and event log', engine: 'Stream engine', situation: 'Situation', scheduler: 'Cognitive scheduler', episode: 'Episode', validator: 'Decision validator', policy: 'Policy', action: 'Action dispatch' };
const GLOSSARY = [
  ['Event', 'One observation with an identity, a source, an event time and a typed payload. Evidence, never an instruction.'],
  ['Event time and watermark', 'Event time is when something happened. The watermark says how far time has progressed, so windows know when they can close.'],
  ['Window and operator', `A window selects readings (here: the last ${spec.windowSeconds / 60} minutes, sliding every ${spec.slideSeconds} s). An operator turns them into a feature such as a mean or a slope.`],
  ['Situation and version', 'The durable record of one condition of one thing. Each publication is a numbered version that never changes afterwards.'],
  ['Trigger and scheduler', 'Declared rules that say when a version deserves reasoning. The scheduler can ignore, queue, replace or admit it.'],
  ['Episode and fence', 'One bounded reasoning session over one immutable snapshot, with a deadline and a budget. Each attempt carries a fence number, so output from a stale attempt is rejected.'],
  ['Decision and Intent', 'The reasoner\'s typed answer, and the typed action it proposes. A proposal, not a permission.'],
  ['Policy and approval', 'Rules re-checked against current state just before dispatch. R2 intents wait for a signed human approval; R3 and R4 are refused.'],
  ['Command and idempotency key', 'The governed request that may be dispatched. A retry carries the same key, so a device that honours it recognises the repeat instead of acting twice.'],
  ['Outcome and verification', 'What the device reported, and a separate state query of what the output is now. Only a matching observation verifies the effect. "Unknown" is a legal outcome.'],
];

const strip = [[run.events.total, 'events'], [V, 'Situation versions'], [run.run.rows.episodes, 'episode'], [run.run.rows.decisions, 'decision'], [run.run.rows.commands, 'command'], [run.run.rows.verifications, 'verified effect']];
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
<ol class="strip" aria-label="The recorded run in numbers">${strip.map(([n, l]) => `<li><b>${n}</b><span>${l}</span></li>`).join('')}</ol>
<p class="fine">Recorded run <code>${run.run.id}</code>: ${esc(run.run.kind)}. Verdict: <b>${run.run.verdict}</b>, every zero-tolerance counter is ${Object.values(run.run.zeroTolerance).every((n) => n === 0) ? '0' : 'NOT 0'}. "Verified effect" here means the device emulator that executed the command also answered the state query; no independent sensor confirmed it.</p>
</section>

<section id="film" class="block">
<div class="kicker">The film</div>
<h2>The whole path, step by step</h2>
<div class="video"><video controls preload="metadata" poster="film/poster.jpg" playsinline><source src="${FILM}" type="video/mp4"><track kind="captions" src="film/captions.vtt" srclang="en" label="English" default>Your browser cannot play this video. Open <a href="${FILM}">the MP4</a> directly.</video><p class="film-missing" hidden>The film is not built here: <code>${FILM}</code> is git-ignored because it is large. Render it with <code>tools/render-film.mjs</code> (see the README), or explore the <a href="world.html">3D world</a>, which shows the same scene live.</p></div>
<p class="fine">${filmLength}, narrated, with captions. The same scene is live in the 3D world, where you can pause, scrub, orbit and open any station.</p>
</section>

<section class="block">
<div class="kicker">The map</div>
<h2>Five planes, each with one job</h2>
<p class="lede small">Work moves through five planes. Each owns one question and is forbidden from doing the others' jobs. That separation is the safety design. The grouping is this guide's own; the architecture overview lists seven planes.</p>
<div class="planes">${PLANES.map(([k, c, n, q, owns, never, st]) => `<article class="plane" style="--c:${c}"><h3>${n}</h3><div class="q">${q}</div><p>${owns}</p><p class="never"><b>Never:</b> ${never}</p><div class="stlinks">${st.map((id) => `<a href="world.html?station=${id}">${NAMES[id]}</a>`).join('')}</div></article>`).join('')}</div>
</section>

<section class="block">
<div class="kicker">Hands on</div>
<h2>Four labs</h2>
<div class="labs">${NAV.map(([k, t], i) => `<a class="lab" href="labs/${k}.html"><span class="n">${i + 1}</span><h3>${LABS[k].h1}</h3><p>${BLURB[k]}</p><span class="go">Open the lab →</span></a>`).join('')}</div>
</section>

<section class="block">
<div class="kicker">Vocabulary</div>
<h2>The words that carry the design</h2>
<dl class="gloss">${GLOSSARY.map(([t, d]) => `<div><dt>${t}</dt><dd>${d}</dd></div>`).join('')}</dl>
</section>

<section class="block honest">
<div class="kicker">What is proven</div>
<h2>Be precise about what this run shows</h2>
<ul>
<li><b>Proven in the recorded run:</b> events to Situation, scheduler, one episode, a typed decision, policy approval, one governed command (<code>set_indicator</code>, risk R1), device receipt and result, and a verified state check.</li>
<li><b>Not physical hardware:</b> a simulator played the sensor bench and a device emulator answered commands behind the gateway socket. The 3D bench and gateway show the designed physical setup, labelled as emulated. The emulator that executed the command also answered the state query, so the verification shows the pipeline's logic, not an independent physical measurement.</li>
<li><b>Designed, not part of this run:</b> the fan path (<code>select_thermal_mode</code>, risk R2) with signed human approval. It is shown in the film and the 3D world and labelled as such.</li>
<li><b>Digests are not reproducible from the repository:</b> the run's spec digest (<code>${run.run.specDigest}…</code>) and policy digest (<code>${run.run.policyDigest}…</code>) come from the run's own spec copy. The <code>examples/real-world-sensor</code> specs compile to other digests, so the digests tie these pages to that run's artifact; they cannot be recomputed from the checked-in files.</li>
<li><b>Not used in this run:</b> an approved playbook rule answering a recurring case in the in-process rule executor with no model call (<code>internal/playbook</code>, <code>internal/executor/rule</code>; design in <code>documentation/design/rule-promotion.md</code>). Here the model worker answered.</li>
<li><b>Known blind spots recorded with the run:</b></li>
</ul>
<ul class="spots">${run.run.blindSpots.map((b) => `<li>${esc(b)}</li>`).join('')}</ul>
</section>
<footer class="src">Source run: <code>${run.run.id}</code> (id read from the ${run.run.idFrom}). Regenerate: <code>tools/extract-run-data.mjs</code> reads the recorded run and its exported artifact (<code>internal/runartifact</code>), <code>tools/narrate.mjs</code> builds narration, captions and timing, <code>tools/build.sh</code> bundles the pages, <code>tools/render-film.mjs</code> renders the film. See <a href="README.md">README</a> and <a href="BAR.md">the quality bar</a>.</footer>
</main>
<script>
(function () {
  var video = document.querySelector('.video video');
  var note = document.querySelector('.film-missing');
  function showNote() { video.hidden = true; note.hidden = false; }
  function check() { if (video.error || video.networkState === HTMLMediaElement.NETWORK_NO_SOURCE) showNote(); }
  video.addEventListener('error', showNote, true);
  addEventListener('load', function () { setTimeout(check, 400); });
  check();
})();
</script>
</body>
</html>
`;
writeFileSync(join(root, 'index.html'), indexHtml);
console.log('wrote index.html');
