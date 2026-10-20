import { RUN } from '../story.js';
import { ch, cue, word } from '../timeline.js';
import { clamp } from '../util.js';

// Every beat of the film is placed on the narration: cue(chapter, sentence) and
// word(chapter, sentence, text) give the moment a sentence or a word is spoken.
const SPAN = RUN.events.spanSeconds;
const TYPE_ORDER = ['temp', 'humidity', 'ambient', 'fan_tach', 'heartbeat'];
const spread = (a, b, n) => Array.from({ length: n }, (_, k) => (n === 1 ? a : a + (k * (b - a)) / (n - 1)));

export const TRAVEL = { toGateway: 2.0, dwell: 0.5, toLog: 3.2, drop: 0.8 };
const FLIGHT = TRAVEL.toGateway + TRAVEL.dwell + TRAVEL.toLog + TRAVEL.drop;
const L0 = cue('gateway', 0).start + 0.4;
const L1 = cue('log', 0).end - FLIGHT - 0.2;

export const events = RUN.events.series.map((e, i) => {
  const launch = L0 + (e.t / SPAN) * (L1 - L0) + TYPE_ORDER.indexOf(e.type) * 0.09;
  const atGateway = launch + TRAVEL.toGateway;
  const leaveGateway = atGateway + TRAVEL.dwell;
  const atLog = leaveGateway + TRAVEL.toLog;
  return { i, ...e, launch, atGateway, leaveGateway, atLog, landed: atLog + TRAVEL.drop };
});

export const gatewayBeats = { envelope: word('gateway', 1, 'event'), envelopeEnd: ch.gateway.end + 0.6 };

export const logBeats = { scan: cue('log', 0).start, bad: cue('log', 1).start + 0.1, evidence: cue('log', 2).start };

export const engine = (() => {
  const t0 = cue('engine', 0).start + 0.2;
  const t1 = cue('engine', 3).end + 0.3;
  const clock = (T) => clamp((T - t0) / (t1 - t0)) * SPAN;
  const filmAt = (eventSeconds) => t0 + (eventSeconds / SPAN) * (t1 - t0);
  const late = cue('engine', 2).start + 0.1;
  return {
    t0, t1, clock, filmAt, flight: 2.6,
    watermark: cue('engine', 1).start, late, lateEvent: Math.max(30, Math.round(clock(late) - 45)),
    window: word('engine', 3, 'window'), features: word('engine', 3, 'operators'), same: cue('engine', 4).start,
  };
})();

export const versions = RUN.versions.map((v) => {
  const emit = engine.filmAt(v.t) + (v.completeness === 'on_time' ? 0.45 : 0);
  return { ...v, emit, land: emit + engine.flight };
});

export const situationBeats = {
  record: cue('situation', 0).start,
  rule: cue('situation', 1).start,
  candidate: word('situation', 2, 'candidate'),
  watch: word('situation', 2, 'watch'),
  cooling: word('situation', 2, 'cooling'),
  pairs: cue('situation', 3).start,
  seal: cue('situation', 4).start,
  edit: cue('situation', 4).start + 1.2,
};

export const scheduler = (() => {
  const gates = {};
  spread(cue('scheduler', 1).start + 0.3, cue('scheduler', 1).end + 0.4, 4).forEach((t, k) => { gates[k + 1] = t; });
  gates[5] = cue('scheduler', 2).start + 0.2;
  gates[6] = cue('scheduler', 2).end + 0.35;
  gates[7] = cue('scheduler', 3).start + 0.2;
  spread(cue('scheduler', 4).start + 0.2, cue('scheduler', 4).end + 0.3, 15).forEach((t, k) => { gates[k + 8] = t; });
  const items = RUN.triggers.map((t) => {
    const gate = gates[t.v];
    const verdict = t.outcome === 'admitted' ? 'queue' : t.score >= t.threshold ? 'nothing-new' : 'condition-false';
    return { ...t, gate, verdict, decided: gate + 1.2, material: t.outcome === 'admitted' };
  });
  const v5 = items[4];
  const v7 = items[6];
  const release = cue('scheduler', 5).start + 0.5;
  return {
    s0: cue('scheduler', 0).start, items, queueV5: v5.decided, coalesce: v7.decided + 0.5, queueV7: v7.decided,
    release, arrive: Math.max(release + 3.0, ch.episode.start + 0.9),
  };
})();

export const episode = (() => {
  const request = cue('episode', 3).start + 0.4;
  const thinkEnd = cue('episode', 4).end - 0.5;
  const answer = thinkEnd;
  const decision = answer + 0.9;
  return {
    t: ch.episode.start, roomIn: ch.episode.start, admit: scheduler.arrive,
    rebind: cue('episode', 1).start + 0.4, rebound: word('episode', 1, 'twenty-two') + 0.4,
    attempt: cue('episode', 2).start + 0.1, budget: word('episode', 2, 'budget'), fence: word('episode', 2, 'fence'),
    request, thinkStart: request + 1.0, evidence: [cue('episode', 4).start + 0.1, cue('episode', 4).start + 1.4],
    thinkEnd, answer, decision, door: decision + 0.3, leave: decision + 0.6,
  };
})();

export const validator = (() => {
  const t = ch.validator.start;
  const c3 = cue('validator', 3);
  const checks = spread(c3.start - 0.3, c3.end + 0.5, 4);
  const pass = checks[3] + 0.6;
  return {
    t, arrive: cue('validator', 0).start + 1.0, decisionCard: cue('validator', 0).start, intentCard: cue('validator', 1).start,
    catalog: cue('validator', 2).start, reject: word('validator', 2, 'An unknown') - 0.2, checks, pass, leave: pass + 0.5,
  };
})();

export const policy = (() => {
  const c0 = cue('policy', 0);
  const rings = spread(c0.start + 0.4, c0.end + 1.0, 6);
  return {
    t: ch.policy.start, arrive: ch.policy.start + 1.0, rings, approved: word('policy', 1, 'approved'),
    r2: cue('policy', 2).start + 0.1, r2sign: word('policy', 2, 'signed') + 0.3, designed: cue('policy', 3).start,
    leave: ch.policy.end - 1.4,
  };
})();

export const action = (() => {
  const t = ch.action.start;
  const lease = cue('action', 1).end - 0.4;
  const send = cue('action', 2).start + 0.1;
  const atGateway = send + 2.0;
  const atBench = atGateway + 1.0;
  const led = Math.max(atBench + 0.15, word('action', 2, 'indicator'));
  const verified = word('action', 4, 'verified') - 0.2;
  return {
    t, appear: t + 0.3, stamp: word('action', 0, 'idempotency'), outbox: word('action', 0, 'outbox'), retry: word('action', 1, 'retry'),
    lease, send, atGateway, atBench, led,
    answers: cue('action', 3).start, receipt: word('action', 3, 'receipt'), result: word('action', 3, 'result'), state: word('action', 3, 'state query'),
    verified, feedback: verified + 0.6, feedbackLand: verified + 3.6,
  };
})();

export const outroBeats = { counts: cue('outro', 0).start, record: cue('outro', 1).start, loop: cue('outro', 2).start - 0.3 };

export const counters = (T) => ({
  events: events.filter((e) => T >= e.landed).length,
  versions: versions.filter((v) => T >= v.land).length,
  episodes: T >= episode.admit ? 1 : 0,
  decisions: T >= validator.decisionCard + 0.3 ? 1 : 0,
  commands: T >= action.stamp ? 1 : 0,
  verified: T >= action.verified ? 1 : 0,
});

export const phaseAt = (T) => {
  const landed = versions.filter((v) => T >= v.land);
  return landed.length ? landed.at(-1) : null;
};
