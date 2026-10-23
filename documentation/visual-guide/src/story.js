import run from './data/run-data.json' with { type: 'json' };

export const RUN = run;

export const PLANES = {
  evidence: { name: 'Evidence', color: '#4cc9f0', blurb: 'Observations enter, are checked and are kept.' },
  stream: { name: 'Stream', color: '#9b8cff', blurb: 'Deterministic state built from evidence in event time.' },
  cognition: { name: 'Cognition', color: '#f5b94a', blurb: 'Decides whether reasoning is useful, then runs a bounded episode.' },
  governance: { name: 'Governance', color: '#34d399', blurb: 'Validates the proposal and re-checks authority.' },
  effect: { name: 'Effect', color: '#fb7185', blurb: 'Performs the permitted change and proves what happened.' },
};

export const EVENT_TYPES = {
  temp: { label: 'temperature', color: '#ff7a59' },
  humidity: { label: 'humidity', color: '#5aa9ff' },
  ambient: { label: 'ambient', color: '#6ee7b7' },
  fan_tach: { label: 'fan tach', color: '#c4a1ff' },
  heartbeat: { label: 'heartbeat', color: '#f1f5f9' },
};

export const INVARIANTS = [
  'Raw events are evidence, never executable instructions.',
  'Event time, watermark, completeness, and late-data status are explicit.',
  'A Situation version is immutable after publication.',
  'Deterministic state changes are serial per virtual partition.',
  'Every episode is bound to one immutable Situation snapshot and finite budget.',
  'A model can read evidence and propose typed Intents; it cannot execute effects.',
  'Policy revalidates every Intent against current state immediately before dispatch.',
  'Cross-boundary work uses stable identities, inbox/outbox records, and idempotency.',
  'Replay never performs external effects unless an explicit, separate simulation mode is selected.',
  'Every admitted, deferred, coalesced, rejected, canceled, and expired cognitive opportunity is explainable from durable records.',
];

const phaseCounts = run.versions.reduce((m, v) => ({ ...m, [v.phase]: (m[v.phase] ?? 0) + 1 }), {});
const ignored = run.triggers.filter((t) => t.outcome === 'ignored').length;
const admitted = run.triggers.filter((t) => t.outcome === 'admitted').length;
const coalesced = run.scheduler.filter((s) => s.status === 'coalesced').length;
const temps = run.events.series.filter((e) => e.type === 'temp');
const tempFirst = temps[0].value;
const tempLast = temps.at(-1).value;
const f = (n, d = 1) => Number(n).toFixed(d);

export const COUNTS = { events: run.events.total, versions: run.versions.length, ignored, admitted, coalesced, phaseCounts, episodes: run.run.rows.episodes };

export const STATIONS = {
  bench: {
    name: 'Sensor bench', plane: 'evidence', ring: 0,
    owner: 'Outside this repository: firmware, wiring, simulator',
    what: 'A temperature sensor, an Arduino board with firmware, an indicator LED and a fan driver. In the recorded run a simulator plays this bench over the same contract the board uses.',
    why: 'Hardware is the final proof. The simulator lets every software gate pass first, with nothing to damage.',
    real: [
      `${run.events.total} events from five signals: temperature, humidity, ambient, fan tach, heartbeat`,
      `Temperature climbed from ${f(tempFirst)} to ${f(tempLast, 2)} °C in ${temps.at(-1).t} s of event time`,
      'Fan tach stayed at 0 rpm: nothing had acted yet',
    ],
    invariants: [1],
    failsSafe: 'On the bench, the board enforces its own limits and a lease, so even a valid command cannot exceed what the firmware allows.',
  },
  gateway: {
    name: 'Gateway', plane: 'evidence', ring: 1,
    owner: 'Gateway program (outside this repository); live ingress socket (internal/ingress); device-gateway link (internal/device)',
    what: 'On the bench, the gateway is the only program that opens the board\'s serial port. It validates frames, keeps device time and a quality flag, turns each reading into an event, and passes governed commands back down as typed device records.',
    why: 'A garbled or hostile value must never be able to reach a command. One careful pipe, no decisions.',
    real: [
      `Emulated in this run: the simulator produced the events (source ${run.events.envelope.source}) and a device emulator answered behind the device-gateway socket`,
      `Every event carries an id, type, source, entity ${run.situation.entity.id}, event time and payload`,
      'On the bench, commands are allow-listed by policy digest and capability digest before they reach the board',
    ],
    invariants: [1],
    failsSafe: 'On the bench, bad frames are quarantined, not forwarded, and a command whose digests are not allow-listed is refused.',
  },
  log: {
    name: 'Ingress and event log', plane: 'evidence', ring: 2,
    owner: 'internal/ingress, internal/eventlog',
    what: 'Checks each event\'s identity and payload against its schema, recognises duplicates by identity plus payload digest, and appends accepted events in arrival order with their event time preserved. Invalid input is quarantined as data.',
    why: 'One append-only source of truth makes the stream auditable and replayable, and keeps untrusted input from becoming anything but evidence.',
    real: [
      `${run.run.rows.event_log} events appended, 0 quarantined`,
      `All mapped to virtual partition ${run.situation.partition}, entity ${run.situation.entity.id}`,
    ],
    invariants: [1, 2, 8],
    failsSafe: 'Re-using an event identity with a different payload is rejected; the same payload is a harmless duplicate.',
  },
  engine: {
    name: 'Stream engine', plane: 'stream', ring: 3,
    owner: 'internal/engine, internal/operators',
    what: 'Applies the SituationSpec in event time. A sliding five-minute window selects readings, operators compute features (mean, slope, missing heartbeat), and a watermark says how far time has progressed.',
    why: 'Results must depend on when things happened, not on when packets arrived, so the same input always produces the same state.',
    real: [
      `Window 5 min, slide 30 s; watermark trailed the newest reading by ${run.versions[0].watermarkLagSeconds} s`,
      `Mean temperature ${f(run.versions[0].tempMean, 2)} → ${f(run.versions.at(-1).tempMean, 2)} °C, slope ${f(run.versions[0].tempSlope)} °C/h`,
      `The first reading happened at ${run.events.envelope.eventTime} and arrived at ${run.events.envelope.ingestedAt} UTC (the run was replayed fast); event time kept the order`,
    ],
    invariants: [2, 4, 9],
    failsSafe: 'A late reading is applied by its event time and never pulls the watermark back; the spec declares what to do about it.',
  },
  situation: {
    name: 'Situation', plane: 'stream', ring: 4,
    owner: 'internal/situations',
    what: `The durable record of one condition of one thing: ${run.situation.type} for ${run.situation.entity.id}. Phases move only when rules hold long enough. Every publication is an immutable, numbered version.`,
    why: 'It separates what the world looks like (deterministic) from what to do about it (judgment), and gives every later step a stable thing to point at.',
    real: [
      `${run.versions.length} versions: ${phaseCounts.candidate} candidate, ${phaseCounts.watch} watch, ${phaseCounts.cooling} cooling`,
      'Each 30 s slide published a provisional version, then an on-time version',
      'Version 22 is the snapshot the reasoner read',
    ],
    invariants: [3, 2],
    failsSafe: 'Nothing can edit a published version. A correction publishes a new one.',
  },
  scheduler: {
    name: 'Cognitive scheduler', plane: 'cognition', ring: 5,
    owner: 'internal/cognition, internal/episodeledger',
    what: 'Scores every new version against declared triggers. It can ignore it, queue it with a delay, replace older queued work for the same trigger, or admit an episode within capacity and expiry limits.',
    why: 'Reasoning is slow, costly and not repeatable. It should run when a change matters, not on every event.',
    real: [
      `${run.triggers.length} versions evaluated: ${ignored} ignored, ${admitted} passed the trigger`,
      `Version 5 queued (${run.scheduler[0].debounceSeconds} s debounce), then replaced by version 7 (${coalesced} coalesced), which waited ${run.scheduler[1].debounceSeconds} s for the cooldown`,
      `Result: ${run.run.rows.episodes} episode for ${run.versions.length} versions, no model call per event`,
    ],
    invariants: [10],
    failsSafe: 'Every outcome, ignored included, has a durable record with its reason, so you can always ask why nothing happened.',
  },
  episode: {
    name: 'Episode', plane: 'cognition', ring: 6,
    owner: 'internal/episodes, internal/episodeledger',
    what: 'One bounded reasoning session: an immutable snapshot, an objective, allowed read tools, an intent catalog, a deadline and a budget. Each attempt carries a fence number so stale output can be rejected.',
    why: 'Bounding the room bounds the risk. The reasoner cannot wander, spend without limit, or act on yesterday\'s world.',
    real: [
      `Admitted for version 7, rebound once to the newest snapshot (version ${run.episode.snapshotVersion}) before the attempt started`,
      `Fence ${run.episode.fence}; executor ${run.episode.executor}; dispatch policy ${run.episode.dispatchPolicy}`,
      `The attempt concluded in ${run.episode.wallMillis} ms`,
    ],
    invariants: [5, 6],
    failsSafe: 'If a material change supersedes the episode, its attempt is canceled and any late output is rejected by its fence.',
  },
  tamoz: {
    name: 'Tamoz, the reasoner', plane: 'cognition', ring: null,
    owner: 'Separate worker process (worker protocol, internal/executor)',
    what: 'The reasoner. It runs outside the deterministic core, in its own worker process, reached over a versioned protocol on a private socket. It reads evidence through scoped read-only tools and returns a typed Decision.',
    why: 'Judgment is the one part that is not deterministic. Keeping it in a separate process with no credentials keeps it advisory.',
    real: [
      `Answered: ${run.decision.hypothesis}, confidence ${run.decision.confidence}`,
      `Used ${run.decision.factsUsed.length} facts: ${run.decision.factsUsed.join(', ')}`,
    ],
    invariants: [6],
    failsSafe: 'It has no effector and no production credentials. A bad answer is at worst a rejected proposal.',
  },
  validator: {
    name: 'Decision validator', plane: 'governance', ring: 7,
    owner: 'internal/decisions',
    what: 'A pure check of the Decision: structure, episode and snapshot identity, intent type in the catalog, parameters inside the catalog schema, only model-writable fields set, and every claim grounded in evidence.',
    why: 'The model proposes in a typed language the spec authored. Anything outside that language is not a proposal, it is noise.',
    real: [
      `Intent: ${run.intent.type} {state: "${run.intent.parameters.state}"}, risk ${run.intent.risk}`,
      `Bound to snapshot digest ${run.decision.snapshotDigest}…`,
    ],
    invariants: [1, 6],
    failsSafe: 'Preset fields stay preset-authored; the model cannot invent a target, a duty cycle or a longer duration.',
  },
  policy: {
    name: 'Policy', plane: 'governance', ring: 8,
    owner: 'internal/policy, internal/approvalledger',
    what: 'Re-evaluates the intent against the current durable state immediately before dispatch: still fresh, condition still open, risk class, approval route, interlock, owner epoch, rate limit.',
    why: 'Time passes between a proposal and an action. Permission has to be true now, not just when the proposal was made.',
    real: [
      `Result: ${run.policy.result}, reason ${run.policy.reason}, at version ${run.policy.version}`,
      `Policy digest ${run.run.policyDigest}… binds the decision to this exact rule set`,
      'Designed, not in this run: an R2 intent such as the fan waits for a signed human approval',
    ],
    invariants: [7, 8],
    failsSafe: 'An operator stop, a new runtime owner or a superseded intent all turn an earlier approval into a refusal.',
  },
  action: {
    name: 'Action dispatch', plane: 'effect', ring: 9,
    owner: 'internal/actions, internal/device, internal/authority',
    what: 'Turns an approved intent into a command with a stable idempotency key, records it in an outbox before sending, leases it to the dispatcher, then records the outcome and verifies it.',
    why: 'A lost reply must not turn into a blind second attempt, and "the device said OK" is not the same as "it happened".',
    real: [
      `Command ${run.command.route} → ${run.command.target}, idempotency key ${run.command.idempotencyKey}…`,
      `Receipt accepted, result ${run.device.result} ("${run.device.detail}")`,
      `query_state: ${run.device.observed.target} energized, value ${run.device.observed.value}; verification ${run.device.reconciliation}`,
      `Run verdict ${run.run.verdict}; every zero-tolerance counter is 0`,
    ],
    invariants: [7, 8],
    failsSafe: 'An unknown outcome is recorded as unknown and reconciled with evidence. It is never retried blindly.',
  },
  store: {
    name: 'Durable store', plane: 'stream', ring: null,
    owner: 'internal/storage (SQLite in WAL mode)',
    what: 'Every plane hands work to the next through records in one durable store: events, versions, evaluations, episodes, decisions, intents, commands, outcomes.',
    why: 'Records survive a restart, make every choice explainable, and let replay rebuild the same history without touching the world.',
    real: Object.entries(run.run.rows).map(([k, v]) => `${k}: ${v}`),
    invariants: [8, 9, 10],
    failsSafe: 'Replay opens its own isolated database with effects disabled.',
  },
};

export const RING_ORDER = ['bench', 'gateway', 'log', 'engine', 'situation', 'scheduler', 'episode', 'validator', 'policy', 'action'];

const L = (text, hold = 0) => ({ text, hold });

const SCRIPT = [
  {
    id: 'intro', title: 'One run, end to end', stations: [], lead: 1.2,
    lines: [
      L('Agentic Stream turns sensor readings into safe, governed action.', 0.3),
      L('Here is one real recorded run, with a simulator playing the sensor and the device.', 0.4),
      L('Let\'s follow it from a warming sensor to a verified effect.', 0.8),
    ],
  },
  {
    id: 'gateway', title: 'Readings become events', stations: ['bench', 'gateway'],
    lines: [
      L('A temperature sensor is warming up.', 0.95),
      L('At the edge, each reading becomes an event, with an identity and the time it really happened.', 2.0),
    ],
  },
  {
    id: 'log', title: 'Evidence is kept, never obeyed', stations: ['log'],
    lines: [
      L('Ingress checks each event against its schema, then appends it to a log that is never rewritten.', 0.95),
      L('Malformed input would be quarantined.', 0.95),
      L('An event is evidence, never an instruction.', 1.0),
    ],
  },
  {
    id: 'engine', title: 'Event time and windows', stations: ['engine'],
    lines: [
      L('The stream engine works in event time: when a reading happened, not when it arrived.', 0.5),
      L('A watermark trails the newest reading by fifteen seconds, and marks how far time has safely advanced.', 0.5),
      L('A late reading still lands at its true time.', 0.95),
      L('A five-minute window slides every thirty seconds, and operators turn it into features: the mean, and the slope.', 0.6),
      L('Same input, same features.', 1.0),
    ],
  },
  {
    id: 'situation', title: 'A Situation with versions', stations: ['situation'],
    lines: [
      L('Features update a Situation: the durable record of one condition.', 0.4),
      L('A phase changes only when its rule holds for a minimum time.', 0.4),
      L('After a minute, it moved from candidate to watch. Thirty seconds later, to cooling, meaning cooling is needed.', 0.8),
      L('Each window step publishes a provisional version, then an on-time one.', 0.6),
      L('Twenty-two versions, and none can be edited.', 1.4),
    ],
  },
  {
    id: 'scheduler', title: 'Thinking is rationed', stations: ['scheduler'],
    lines: [
      L('Reasoning is slow and costly, so a deterministic scheduler decides when it is worth it.', 0.4),
      L('The four candidate versions fail the trigger.', 0.9),
      L('Version five passes, and is queued.', 0.9),
      L('Version seven scores higher, and replaces it.', 0.95),
      L('The other sixteen bring nothing new, so they are ignored, each with a recorded reason.', 0.8),
      L('Twenty-two versions, one episode. No model call per event.', 1.4),
    ],
  },
  {
    id: 'episode', title: 'A sealed room for reasoning', stations: ['episode', 'tamoz'],
    lines: [
      L('Admission opens one episode: a bounded reasoning session.', 0.4),
      L('It is rebound to the newest snapshot, version twenty-two, and that snapshot never changes.', 0.8),
      L('A deadline, a budget, and a fence number keep it bounded.', 0.8),
      L('Tamoz, the reasoner, runs in a separate worker process.', 0.4),
      L('It reads evidence, and can only propose. It cannot act.', 1.8),
    ],
  },
  {
    id: 'validator', title: 'A typed proposal', stations: ['validator'],
    lines: [
      L('Tamoz returns a typed decision: a rising over-temperature, at ninety-six percent confidence.', 0.4),
      L('It proposes one intent: set the indicator to alert.', 0.8),
      L('The validator accepts only what the spec\'s catalog allows. An unknown intent would be refused.', 0.8),
      L('This one passes every check.', 1.4),
    ],
  },
  {
    id: 'policy', title: 'Authority is re-checked', stations: ['policy'],
    lines: [
      L('Policy checks again, against the current state, right before dispatch.', 0.95),
      L('The indicator is low risk, so it is approved automatically.', 0.8),
      L('A fan, class R2, would wait for a signed human approval.', 0.4),
      L('That path is designed, but was not part of this run.', 1.2),
    ],
  },
  {
    id: 'action', title: 'One command, three answers', stations: ['action', 'gateway', 'bench'],
    lines: [
      L('The command gets a stable idempotency key, and is written to an outbox before it is sent.', 0.4),
      L('So a crash cannot lose it, and a retry is recognised as the same request.', 0.6),
      L('It goes out through the device gateway, and the indicator switches on.', 0.9),
      L('Three answers come back: a receipt, a result, and a state query.', 0.8),
      L('Only when the observed state matches is the effect verified.', 1.4),
    ],
  },
  {
    id: 'outro', title: 'The loop closes', stations: [],
    lines: [
      L('Sixty-one events, twenty-two versions, one episode, one decision, one verified effect.', 0.6),
      L('Every step left a durable record, so every outcome can be explained.', 0.6),
      L('And new readings keep flowing into the same loop.', 3.0),
    ],
  },
];

export const CHAPTERS = SCRIPT.map((c) => ({ ...c, narration: c.lines.map((l) => l.text).join(' ') }));

export const LABS = [
  { href: 'labs/time.html', title: 'Time and windows', blurb: 'Why a late reading cannot rewrite history. Drag the arrival order, watch the watermark.' },
  { href: 'labs/situation.html', title: 'Situation and hysteresis', blurb: 'Enter and leave thresholds, minimum durations, and why versions never change.' },
  { href: 'labs/scheduler.html', title: 'The reasoning gate', blurb: 'Score, threshold, debounce, cooldown, coalescing. Replay the real 22 versions.' },
  { href: 'labs/authority.html', title: 'From proposal to proof', blurb: 'Decision, intent, policy, command, and the three answers that come back.' },
];
