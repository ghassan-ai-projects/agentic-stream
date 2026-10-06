# Duration ubiquitous language

| Term | Meaning | Code name | Authored as |
| --- | --- | --- | --- |
| Duration string | The runtime's textual duration: any `time.ParseDuration` unit plus whole days. | `Parse` | `15m`, `6h`, `30d` |
| Day | A fixed 24 hours, with no calendar or daylight-saving meaning. Must be positive and must not overflow `time.Duration`. | `parseDays` | `d` suffix |

Spec fields such as `maxOutOfOrderness`, `reopenCooldown` and `wallTime` all
use this one format; nothing else parses durations.
