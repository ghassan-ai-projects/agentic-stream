# Slow tests: static investigation

A read-only investigation made before rounds 14 and 16 (2026-10-09). The causes
are unverified: measure before acting. Line numbers refer to commit 4bb694c5.

```text
Cross-cutting:
1. cmd operator tests: runOperatorCommand -> storage.Open (operator.go:35) cold-migrates 33 migrations per invocation; replay opens storage.OpenFresh (internal/replay/internal/transport/database.go:26) cold. Fix in tests: pre-seed --db with storagetest.Open then Close.
2. spec.CompileFile re-compiles the embedded schema (spec/internal/domain/compiler.go:18, compiler_schema.go:14-24) and new CEL env (cel.go:33-35) per compile; situations.go:175 and cognition/internal/domain/evaluation.go:43 build more. Candidate: cache compiled schema / CEL env via sync.OnceValues (production perf change, behavior-preserving).
3. replay app/run.go:125-137 RunNTimes runs n replays sequentially, each cold.
4. Live cmd tests wait on wall-clock spec gates: debounce 5s, cooldown 20s (examples/real-world-sensor/zone-thermal-sim.situation.yaml:218-219), slide 30s, maxOutOfOrderness 15s. Experiment live feed workerDelay 5s (experiment_live_feed_test.go:27, experiment_standins_test.go:54); time.Sleep(1s) at experiment_e2e_test.go:369.
Per test fixes:
- TestRunRepeatProvesDeterminism (19s): 3 sequential cold replays (main_test.go:144, run_command.go:76).
- TestNotificationsPrune/TestInterlockTripAndClear/TestPrincipalsApply/TestCommandsListAndResolve (~8s each): cold --db each invocation; pre-seed via storagetest.Open.
- replay TestThermalChamberReplayIsDeterministic 34s: RunNTimes(3) sequential (thermal_chamber_test.go:135).
- replay shadow tests (replay_test.go:296-321, :251, :265, bench_spec_test.go:24/:34): independent replays sequential -> parallel subtests.
- storagetest TestTemplateIsBuiltOnceAndReusedFromItsDirectory (storagetest_test.go:94-107): builds a cold template in fresh TempDir; reuse shared dir and assert reuse via modtime/inode.
- Production candidate: OpenFresh copy a migrated template instead of running migrations (storage.go:37-49).
```
