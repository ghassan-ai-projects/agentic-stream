import { DatabaseSync } from 'node:sqlite';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const [dbPath, artifactDir, outPath] = process.argv.slice(2);
if (!dbPath || !artifactDir) {
  console.error('usage: extract-run-data.mjs <stream.db copy> <artifact dir> [out.json]');
  process.exit(2);
}
const out = outPath ?? join(dirname(fileURLToPath(import.meta.url)), '../src/data/run-data.json');
const db = new DatabaseSync(dbPath, { readOnly: true });
const all = (sql) => db.prepare(sql).all();
const one = (sql) => db.prepare(sql).get();
const json = (name) => JSON.parse(readFileSync(join(artifactDir, name), 'utf8'));
const jsonl = (name) => readFileSync(join(artifactDir, name), 'utf8').trim().split('\n').map((l) => JSON.parse(l));
const text = (blob) => JSON.parse(Buffer.from(blob).toString('utf8'));
const short = (s, n = 12) => String(s).replace(/^sha256:/, '').slice(0, n);

const events = all('select event_type, event_time, payload_json from event_log order by position').map((e) => ({
  type: e.event_type, at: Date.parse(e.event_time), payload: text(e.payload_json),
}));
const t0 = events[0].at;
const sec = (iso) => Math.round((Date.parse(iso) - t0) / 1000);
const valueOf = (e) => e.payload.celsius ?? e.payload.rpm ?? e.payload.percent ?? e.payload.relative_humidity ?? null;

const versions = all('select version, phase, severity, completeness, event_horizon, watermark, snapshot_json from situation_versions order by version').map((v) => {
  const snap = text(v.snapshot_json);
  return {
    v: v.version, phase: v.phase, severity: v.severity, completeness: v.completeness, t: sec(v.event_horizon),
    watermarkLagSeconds: Math.round((Date.parse(v.event_horizon) - Date.parse(v.watermark)) / 1000),
    tempMean: Number(snap.facts['facts.temp_mean'].toFixed(2)), tempSlope: Number(snap.facts['facts.temp_slope'].toFixed(1)),
    evidenceCount: snap.evidence.length,
  };
});
const situation = text(one('select snapshot_json from situation_versions where version = 22').snapshot_json);
const evalRows = all('select situation_version v, outcome, score, threshold, reasons_json, evaluated_at from trigger_evaluations order by situation_version');
const w0 = Date.parse(evalRows[0].evaluated_at);
const triggers = evalRows.map((r) => ({
  v: r.v, outcome: r.outcome, score: r.score, threshold: r.threshold, reason: text(r.reasons_json)[0], wallMs: Date.parse(r.evaluated_at) - w0,
}));
const sched = all('select situation_version v, status, priority, not_before, created_at, updated_at from scheduler_items order by created_at').map((s) => ({
  v: s.v, status: s.status, priority: s.priority, debounceSeconds: Math.round((Date.parse(s.not_before) - Date.parse(s.created_at)) / 1000),
  createdMs: Date.parse(s.created_at) - w0, notBeforeMs: Date.parse(s.not_before) - w0, updatedMs: Date.parse(s.updated_at) - w0,
}));
const ep = one('select * from episodes');
const decision = jsonl('decisions.jsonl')[0];
const intent = decision.raw_json.intents[0];
const policyEval = one('select result, reason, situation_version, policy_digest from policy_evaluations');
const command = jsonl('commands.jsonl')[0];
const device = jsonl('device-results.jsonl')[0];
const verdict = json('verdict.json');
const metrics = json('metrics.json');
const manifest = json('manifest.json');

const first = one('select event_id, source, event_time, ingested_at, schema_version from event_log order by position limit 1');
const clock = (iso) => iso.slice(11, 19);
const counts = {};
for (const e of events) counts[e.type] = (counts[e.type] ?? 0) + 1;
const tables = ['event_log', 'situation_versions', 'trigger_evaluations', 'scheduler_items', 'episodes', 'decisions', 'intents', 'policy_evaluations', 'commands', 'outcomes', 'verifications'];

const data = {
  run: {
    id: 'round-007-simulator-pass2',
    kind: 'simulator-fed joined run (Streams Simulator -> Agentic Stream -> Tamoz -> device emulator)',
    specDigest: short(manifest.spec_digest), policyDigest: short(manifest.policy_digest),
    capabilityDigest: short(manifest.device.capability_digest),
    rows: Object.fromEntries(tables.map((t) => [t, one(`select count(*) c from ${t}`).c])),
    verdict: verdict.verdict ?? metrics.verdict ?? 'pass',
    blindSpots: manifest.known_blind_spots,
    zeroTolerance: metrics.zero_tolerance,
  },
  events: {
    total: events.length, byType: counts, spanSeconds: sec(new Date(events.at(-1).at).toISOString()),
    series: events.map((e) => ({ type: e.type.replace('zone.', '').replace('.observed', ''), t: Math.round((e.at - t0) / 1000), value: valueOf(e) })),
    sample: { ...events[0].payload, event_type: events[0].type },
    envelope: { id: first.event_id, source: first.source, schemaVersion: first.schema_version, eventTime: clock(first.event_time), ingestedAt: clock(first.ingested_at), arrivalLagSeconds: Math.round((Date.parse(first.ingested_at) - Date.parse(first.event_time)) / 1000) },
  },
  situation: { type: situation.situation_type, entity: situation.entity, partition: situation.partition_id },
  versions,
  triggers,
  scheduler: sched,
  episode: {
    fence: ep.current_fence, rebinds: ep.stale_rebind_count, status: ep.lifecycle_status, executor: ep.executor_name,
    dispatchPolicy: ep.dispatch_policy, snapshotVersion: ep.situation_version,
    wallMillis: Date.parse(ep.ended_at) - Date.parse(ep.started_at), acceptedMs: Date.parse(ep.accepted_at) - w0,
  },
  decision: {
    confidence: decision.raw_json.confidence, hypothesis: decision.raw_json.primary_hypothesis, summary: decision.raw_json.summary,
    factsUsed: decision.raw_json.facts_used.map((f) => f.evidence.replace('fact:facts.', '')),
    snapshotDigest: short(decision.raw_json.snapshot_digest),
  },
  intent: { type: intent.type, risk: intent.risk_class, parameters: { state: intent.parameters.state, entity_id: intent.parameters.entity_id, situation_version: intent.parameters.situation_version }, digest: short(intent.intent_digest) },
  policy: { result: policyEval.result, reason: policyEval.reason, version: policyEval.situation_version },
  command: { status: command.status, route: command.effector_route, target: command.normalized_target, idempotencyKey: short(command.command_json.idempotency_key) },
  device: {
    receipt: device.provider_result_json.receipt.accepted, result: device.provider_result_json.result.status,
    detail: device.provider_result_json.result.detail, observed: device.observed_effect_json.feedback.observed_state,
    source: device.observed_effect_json.source, reconciliation: device.reconciliation_status,
  },
};
mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, JSON.stringify(data, null, 2) + '\n');
console.log('wrote', out, data.events.total, 'events', data.versions.length, 'versions');
