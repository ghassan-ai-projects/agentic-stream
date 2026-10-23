# Visual guide

An animated way to understand Agentic Stream: a narrated film, an interactive 3D
world and four small labs. All of it follows one real recorded run of the
real-world-sensor experiment, from a warming sensor to a verified effect. In that
run a simulator played the sensor bench and a device emulator answered commands;
no physical hardware was involved.

Open [index.html](index.html) in a browser. It works from the file system, with
no server and no network.

## What is here

| Page | What it gives you |
| --- | --- |
| [index.html](index.html) | The film, the five-plane map, the labs, a ten-word vocabulary and an honest "what is proven" box. |
| [world.html](world.html) | The 3D world: eleven stations on a ring, driven by one timeline. Play the narrated tour, scrub, orbit, click any station for what it does, why it exists, its numbers from the run and its owning module. |
| [labs/time.html](labs/time.html) | Event time, the watermark, windows and late data. |
| [labs/situation.html](labs/situation.html) | Hysteresis, minimum duration and immutable Situation versions. |
| [labs/scheduler.html](labs/scheduler.html) | The reasoning gate, replaying the 22 real evaluations. |
| [labs/authority.html](labs/authority.html) | Validator, policy, outbox, dispatch and the three answers that come back. |
| `film/` | The film's captions (`captions.vtt`) and poster. The rendered `agentic-stream-explained.mp4` and the narration track are git-ignored (60 MB); rebuild them with `tools/render-film.mjs`. |

## Where the numbers come from

Nothing is typed into a page by hand. `tools/extract-run-data.mjs` reads the
recorded run (its SQLite database and its exported artifact) and writes
`src/data/run-data.json`. Pages and the film read that file. Illustrative
moments (a quarantined malformed input, a late reading, an intent outside the
catalog) are labelled as illustrative in the picture, and the R2 fan path with
human approval is labelled as designed, not part of this run.

## Rebuild

Needs Node 22.5 or later, [rolldown](https://rolldown.rs), Three.js 0.186, ffmpeg
and Google Chrome. The narration voice is ElevenLabs `eleven-v4` (voice `george`)
through OpenRouter: `narrate.mjs` reads `OPENROUTER_SPEECH_API_KEY` from the
environment or the repository `.env`, and caches each synthesized sentence in
`tools/.tts-cache/` (git-ignored), so only changed sentences are sent again.
`TTS=say` uses the macOS voice instead.

```bash
cd documentation/visual-guide/tools && npm install
node extract-run-data.mjs <stream.db copy> <run artifact dir>
node narrate.mjs
./build.sh
node make-pages.mjs
node render-film.mjs <frames dir>
```

The narration script lives in `src/story.js` (`CHAPTERS`, one sentence per line,
with an optional pause after it). `narrate.mjs` synthesizes each sentence on its
own, trims and measures it, and writes the exact start and end of every sentence
to `src/timeline.generated.js`. Scene beats are scheduled on those cues
(`cue(chapter, sentence)` and `word(chapter, sentence, text)` in
`src/timeline.js`, used by `src/world/schedule.js` and `src/world/camera.js`), so
picture and voice stay locked when the narration changes. `render-film.mjs`
drives Chrome frame by frame through `window.__seek(t)`, which makes the film
reproducible.

## Quality

The pass criteria are in [BAR.md](BAR.md). `tools/audit.mjs` checks label overlap
across the whole timeline and `tools/perf.mjs` measures frame cost.

## Next reads

- [How a reading becomes a result](../learn/how-it-works.md)
- [Runtime architecture overview](../architecture/overview.md)
- [The ten product invariants](../architecture/invariants.md)
- [Run the predictive-maintenance walkthrough](../guides/predictive-maintenance.md)
