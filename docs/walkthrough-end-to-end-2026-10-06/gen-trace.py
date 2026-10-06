#!/usr/bin/env python3
"""Write the 60-minute degradation trace used by the walkthrough: one vibration,
temperature, current and heartbeat event per minute for motor-17, with vibration
held at 6.2 mm/s and temperature climbing 0.2 degrees per minute."""
import datetime
import json
import sys

out = sys.argv[1] if len(sys.argv) > 1 else "/tmp/walk-trace.jsonl"
t0 = datetime.datetime(2026, 1, 1, tzinfo=datetime.timezone.utc)
fmt = "%Y-%m-%dT%H:%M:%SZ"
rows = []
for minute in range(60):
    when = t0 + datetime.timedelta(minutes=minute)

    def event(kind, data):
        return {
            "id": f"walk-{kind.split('.')[1][:4]}-{minute:03d}", "type": kind, "schema_version": "1.0",
            "tenant_id": "default", "source": "walkthrough", "partition_key": "motor-17",
            "entity": {"type": "motor", "id": "motor-17"}, "event_time": when.strftime(fmt),
            "ingested_at": (when + datetime.timedelta(seconds=1)).strftime(fmt),
            "classification": "internal", "data": data,
        }

    rows.append(event("motor.vibration.observed", {"rms_mm_s": 6.2}))
    rows.append(event("motor.temperature.observed", {"celsius": 60 + minute * 0.2}))
    rows.append(event("motor.current.observed", {"amps": 12.0}))
    rows.append(event("motor.heartbeat.observed", {}))
with open(out, "w", encoding="utf-8") as handle:
    handle.write("\n".join(json.dumps(r, separators=(",", ":")) for r in rows) + "\n")
print(len(rows), "events ->", out)
