# 7. The Learning Loop: Self-Improvement as Disciplined Text Gardening

This is the chapter the repository's reputation rests on, so it opens by correcting expectations. Hermes Agent's "self-improvement" is real, closed-loop, and on by default — and narrower than the phrase suggests. The system turns conversational experience into bounded, human-readable Markdown artifacts (skills, memory entries, a user model), injects them into future prompts, and gardens them over time; it never mutates code, weights, or its own runtime prompts. The narrowness is the safety story, not a gap.

## 7.1 The Honest Frame

### 7.1.1 Self-organizing, not self-modifying

The organizing claim is insight 3, and the taxonomy dimension's verdict states it verbatim: **self-organizing, not self-modifying**. Three absences define the mechanism, and each is load-bearing:

- **No in-loop fitness measurement.** Nothing in the main repository scores whether a learned skill improved anything. Quality is prompt-mediated — it rests on the reviewing model's judgment plus the review prompt's anti-capture rules — and so degrades gracefully with model quality rather than failing loudly.
- **No weight updates.** There is no training path in the runtime; the RL layer that once existed was amputated in one PR in May 2026 (chapter 9). "Learning" here means the filesystem changes.
- **No runtime prompt self-modification.** The agent cannot rewrite its system prompt, tool descriptions, or review prompts. Learned artifacts re-enter context only through sanctioned injection points — the skills index and the memory snapshot — and only at session boundaries, because mid-session mutation would break chapter 5's prefix-cache invariant.

What remains as the mutation surface is deliberately small: whitelisted, bounded, human-readable Markdown writes — `SKILL.md` packages under `~/.hermes/skills/`, `MEMORY.md` and `USER.md` under `$HERMES_HOME/memories/` — capped at 100 KB per skill and 1 MiB per file, written through tools carrying provenance and ownership guards ([tools/skill_manager_tool.py:488–489](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/skill_manager_tool.py#L488)). Safety is achieved by *constriction*: the system does not police a general self-modification surface, it declines to have one — insight 5's outermost ring made behavioral. The absences are why the loop may run unattended by default; a system that measured fitness and rewrote itself in-loop would need exactly the human-review gates this project reserves for its external companion (§7.4.1).

## 7.2 The Background Reflection Fork

![Figure 3: The closed learning loop — epilogue cadences fire a cache-parity reflection fork whose artifacts re-enter the next session's prompt; the Curator gardens them; fitness-measured evolution is external and PR-gated.](/mnt/agents/output/diagrams/03_learning_loop.png)

Figure 3 traces the circuit: two cadence counters fire a forked second agent after response delivery; the fork writes memory and skill artifacts through a whitelisted tool surface; the artifacts re-enter the next session as frozen prompt data; the Curator decays them; and the only fitness-measured optimization happens outside the repository, landing as human-reviewed PRs.

### 7.2.1 Cadences: two counters, one fire condition

Reflection is scheduled by two counters, not a timer. The **memory nudge** counts user turns: `_turns_since_memory` increments per turn context and fires at `_memory_nudge_interval` (default **10 user turns**, `memory.nudge_interval`), rehydrated from persisted history across restarts ([agent/turn_context.py:554–561](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/turn_context.py#L554); rehydration :520–526). The **skill nudge** counts tool-calling iterations: `_iters_since_skill` increments per iteration ([agent/conversation_loop.py:889–891](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/conversation_loop.py#L889)) and is checked in the finalizer against `_skill_nudge_interval` (default **10 tool iterations**, `skills.creation_nudge_interval`), resetting on fire ([agent/turn_finalizer.py:576–581](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/turn_finalizer.py#L576)). The metrics are complementary: turns approximate elapsed experience; iterations approximate *difficulty* — ten tool calls means a non-trivial procedure was likely just discovered.

The fire condition matters as much as the cadences: the review spawns only when `final_response and not interrupted and (review_memory or review_skills)` — **after response delivery, only on successful uninterrupted turns** (`agent/turn_finalizer.py:591–601`), per the code comment, "so it never competes with the user's task for model attention." An interrupted turn produces no learning, which quietly keeps error transcripts out of the skill mine. The spawn is best-effort — a failed review can never fail a turn — and lands on a daemon thread named `bg-review` (`run_agent.py:1688–1714`), so a wedged review cannot hold the process open.

### 7.2.2 The fork: a second AIAgent with its capabilities subtracted

`_run_review_in_thread` ([agent/background_review.py:617–953](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/background_review.py#L617)) constructs a **full second `AIAgent`**, not a one-shot prompt. The weight is the point: the reviewer can `skill_view` the exact skill it intends to patch before writing, which is what makes the read-before-write guard (§7.4.2) enforceable at all. The fork inherits the parent's live runtime — provider, model, base URL, credentials — because re-running credential resolution fails for OAuth-only and session-scoped setups (background_review.py:659–665). Then the hardening begins, all of it capability subtraction (insight 4), never instruction:

- A **thread-scoped tool whitelist** restricts the fork to memory and skill tools; everything else is denied at dispatch (background_review.py:819–835).
- **Dangerous-command approvals auto-deny** on the worker thread — a blocking `input()` would deadlock against the parent's TUI (#15216), at background_review.py:637–644:

```python
def _bg_review_auto_deny(command, description, **kwargs):
    logger.warning(
        "Background review auto-denied dangerous command: %s (%s)",
        command, description,
    )
    return "deny"
try:
    _set_approval_callback(_bg_review_auto_deny)
```

- **`_persist_disabled = True`** hard-stops every session-DB write path (background_review.py:743–754). The comment names this "the curator-takeover root cause": because the fork shares the parent's `session_id` (for cache warmth, §7.2.3), without persistence isolation its review prompt would land in the user's real session, and the next live turn would re-read it as a standing instruction and "become" the curator. Session-identity sharing for cache economics opened a prompt-injection channel from the agent's own reflection machinery; the fix removed the write capability rather than filtering the text.
- **Compression is disabled** (background_review.py:796–807): a compression race won by the single-lifecycle fork would rotate the parent's session into a child the gateway never adopts (#38727). The fork's own nudges are zeroed — no recursive reviews — and stdout is silenced thread-locally (#55769).

The review prompts complete the design. The memory prompt is conservative — save persona, preferences, expectations, else "say 'Nothing to save.' and stop" (background_review.py:170–179). The skill prompt is deliberately bias-to-act: "Be ACTIVE — most sessions produce at least one skill update… A pass that does nothing is a missed learning opportunity, not a neutral outcome" (background_review.py:181–369). The system *wants* sediment and then pays the Curator to fight the bloat (§7.3.2). Counterweighting the bias is an anti-capture list (background_review.py:260–275): never capture environment-dependent failures, transient errors, one-off narratives, or negative tool claims, because "these harden into refusals the agent cites against itself for months." That sentence explains why prompt-mediated learning needs editorial rules: a learned artifact is a future prompt, and a false negative claim in it is a self-inflicted capability loss.

### 7.2.3 Cache-parity pins: reflection priced at ~26% less

A full agent loop (up to 16 iterations) every ten turns would naively double token spend on those turns [INFERRED from the ~26% figure cited at background_review.py:772–774]. It does not, because the fork is engineered to hit the parent's warm provider prefix cache — insight 1 appearing inside the learning loop. On the same-model path the fork inherits the parent's cached system prompt verbatim and pins every other value that could perturb the byte prefix, at background_review.py:779–789 (defensive comment elided):

```python
if not _routed:
    review_agent._cached_system_prompt = agent._cached_system_prompt
    # ... pin session_start + session_id so any re-render path
    # still produces byte-identical output ...
    review_agent.session_start = agent.session_start
review_agent.session_id = agent.session_id
```

The comment cites issue #25322 / PR #17276 and a measured **~26% end-to-end cost reduction on Sonnet 4.5** — a single-source, in-code figure, quoted as such. The pins are defensive in depth: the cached-prompt assignment already short-circuits the rebuild path; the `session_start`/`session_id` pins guarantee parity "even if a future code path bypasses the cache." Routing review to a cheaper auxiliary model (`auxiliary.background_review.{provider,model}`) makes the parent's cache useless — wrong cache key — so the routed fork instead replays a compact digest (last 24 messages verbatim plus a synthetic summary) rather than the full transcript (background_review.py:34–43, 122–163). The cloner's lesson: cache parity is what makes per-ten-turn reflection *economically viable at all*, and it doubles as a constraint — learned artifacts live in the system prompt, so writes take effect only at session boundaries, which is precisely chapter 6's frozen-snapshot behavior.

### 7.2.4 /learn, the learning graph, and closing the loop

The background cadence is opportunistic; `/learn` is the deliberate path. `build_learn_prompt(user_request)` builds one foreground instruction: gather named sources — directories, URLs, the current conversation, pasted notes — and author exactly one `SKILL.md` via `skill_manage action=create` ([agent/learn_prompt.py:99–150](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/learn_prompt.py#L99)). There is "no separate distillation engine and no model-tool footprint" (learn_prompt.py:18–22): distillation is the same loop with a different prompt. The embedded authoring standards (learn_prompt.py:30–96) enforce house style as hard rules — description ≤60 characters because the prompt index truncates at 60, `author: Hermes` literally for privacy, "NEVER invent flags, paths, or APIs" — because a learned skill that lies about an interface is worse than none.

Visibility is a feature, not an afterthought. `agent/learning_graph.py` builds a presentation graph directly over on-disk state: skill nodes carry provenance, use counts, lifecycle state, and declared `related_skills` edges; `MEMORY.md`/`USER.md` split on `§` separators into memory "cards" linked to skills by lexical overlap (learning_graph.py:156–168, 227–245), filtered to learned artifacts with real signal (learning_graph.py:262–267). User-initiated mutations (`hermes journey delete|edit`, TUI, GUI) operate on the same node IDs — delete means *archive* for skills, atomic rewrite for memory — and every mutation clears the skills prompt cache so the next session reflects it ([agent/learning_mutations.py:200–206](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/learning_mutations.py#L200)).

The loop closes through the prompt, never through code: artifacts written at turn *N* sit on disk until the next session assembles its system prompt — skills index into the stable tier, memory snapshot into the volatile tier (chapters 5–6) — and only then alter behavior. Closure is real but **session-deferred**; the deferral is the cache invariant's price, paid willingly.

## 7.3 Verification and the Curator

### 7.3.1 The verification ledger: the only in-loop verifier

The "no verifier" claim of §7.1.1 has one exception, and its scope deserves precision. `agent/verification_evidence.py` is a deliberately passive SQLite ledger — it "never decides to run a suite, never blocks completion" — classifying terminal-tool results into lint/typecheck/build/test events with canonical-command equivalence (`pytest` ≡ `python -m pytest` ≡ `uv run pytest`) and per-workspace edit tracking ([agent/verification_evidence.py:1–6, 177–197](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/verification_evidence.py#L1)). `agent/verification_stop.py` consumes it: when the model tries to end a turn after editing verifiable code — docs and prose filtered out, so SKILL.md edits never demand tests — without fresh passing evidence, it injects a synthetic user message demanding the verification command, bounded to two attempts per turn, default-on for CLI/TUI/desktop and default-off for messaging surfaces (verification_stop.py:24–72, 135–170, 245–310). Synthetic scaffolding is stripped from persisted history so it cannot poison resumed transcripts (turn_finalizer.py:50–66).

The scope discipline is the point. Verify-on-stop improves *truthfulness within a turn* — unverified completion claims become structurally difficult — and thereby improves the signal quality of the transcripts the fork later mines. It cannot measure whether a learned skill is good. Cross-turn artifact quality remains unverified by construction; the ledger is a floor, not a fitness function.

### 7.3.2 The Curator: deterministic decay, optional judgment

The Curator (`agent/curator.py`, 2,016 lines) is the lifecycle manager chapter 6 forward-pointed. Scheduling is inactivity-triggered, not cron: `should_run_now` gates on enabled-and-not-paused, `last_run_at` older than `interval_hours` (default **168 h**), an idle gate of **2 h** at the call site, and — unusually — a fresh install seeds `last_run_at = now`, so the **first run defers a full interval** rather than mutating a library it has never seen ([agent/curator.py:233–283](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/curator.py#L233); cadence figures are Medium-tier: two readers, one source).

Phase 1 is a deterministic state machine with no LLM: `apply_automatic_transitions` walks agent-created skills by last real activity — unused 30 days → `stale`, 90 days → `archive`, used again while stale → reactivated (curator.py:305–383, predicate at :370–381). The guards encode operational respect: pinned skills skipped, cron-referenced skills skipped (a paused cron job's skill is in use by definition), never-used skills granted a grace floor ("absence of evidence, not evidence of staleness"), hub-installed skills never pruned, and — the load-bearing invariant — **archive only, never auto-delete** (curator.py:17). Before every mutating pass the run takes a tar.gz snapshot of the skills tree plus `cron/jobs.json`, kept five-deep and undoable via `hermes curator rollback` — a rollback that is itself undoable (`agent/curator_backup.py:1–40`). The snapshot is best-effort by deliberate reasoning, at curator.py:1544–1551:

```python
# Pre-mutation snapshot — best-effort, never blocks the run. A
# failed snapshot logs at debug and continues (the alternative is
# that a transient disk issue silently disables curator forever,
# which is worse). Users who want to require snapshots can disable
# curator entirely until they can fix disk space.
try:
    from agent import curator_backup
    snap = curator_backup.snapshot_skills(reason="pre-curator-run")
```

Phase 2 is the honest tell about LLM judgment here: an umbrella-building consolidation pass ("If you end the pass with fewer than 10 archives, you stopped too early") that merges prefix clusters, demotes siblings to support files, and emits a structured YAML summary (curator.py:417–568) — and it is **off by default** (`DEFAULT_CONSOLIDATE = False`, curator.py:78). The maintainers treat aggressive LLM consolidation as risky enough to gate. Decay is deterministic; qualitative judgment is optional. That split is the Curator's thesis.

## 7.4 Fitness Measurement Lives Elsewhere

### 7.4.1 The self-evolution companion: optimization as a PR, never a commit

The only fitness-measured optimization in the ecosystem is a separate repository, `NousResearch/hermes-agent-self-evolution`, which "operates ON hermes-agent — not part of it," requires zero changes in the agent repo, and lands every result as a human-reviewed pull request.[^15^][^16^] Its primary engine is DSPy with GEPA — reflective Genetic-Pareto prompt evolution, an ICLR 2026 Oral method that reads execution traces to learn *why* candidates fail[^18^] — with MIPROv2 as fallback; no GPU training, a reported ~$2–10 per run in API calls.[^15^][^16^] Phase 1, implemented as of June 2026, optimizes `SKILL.md` files against synthetic or session-mined eval sets with LLM-as-judge rubrics, behind constraint gates: full test suite green, skills ≤15 KB, cache compatibility (no mid-conversation prompt mutation), semantic preservation, benchmark gates, and a "done when" of ≥10% score increase with a diff that "reads sensibly to a human."[^16^] Phases 2–5 (tool descriptions, prompt sections, tool *code*, continuous loop) are planned; the code phase carries the strictest rule — "every line of evolved code reviewed before merge."[^15^]

The architectural relationship summarizes the chapter: the main repo's loop is *experience → markdown*; the companion is *markdown → measurably better markdown* — "the existing Curator prunes unused skills; self-evolution *improves* the ones you keep."[^16^] Together: create (background review, /learn) → maintain (Curator) → evolve (GEPA PRs). The project trusts autonomous improvement exactly up to the point where fitness becomes measurable, and not one step further: everything with an objective score passes through human review. [INFERRED] The split reads as a deliberate trust boundary rather than an accident of repo organization, though no source states the motivation.

### 7.4.2 The guardrail stack and residual risks

The chapter's transferable content is the guardrail stack — each guard with its enforcement point and the failure it prevents:

| Guard | Enforcement point | Failure mode prevented |
|---|---|---|
| Thread-scoped tool whitelist (memory/skill only) | [agent/background_review.py:819–835](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/background_review.py#L819) | Reflection fork invoking shell, file, or network tools unsupervised |
| Persistence isolation (`_persist_disabled`) | agent/background_review.py:743–754 | "Curator-takeover": fork prompt injected into the real session, re-read as a standing instruction |
| Compression disabled; `_end_session_on_close=False` | agent/background_review.py:790–807 | Fork winning a compression race or finalizing the parent's live session (#38727) |
| Dangerous-command auto-deny | agent/background_review.py:637–644 | Blocking `input()` deadlock against the parent TUI (#15216) |
| Provenance ContextVar + ownership guards | tools/skill_manager_tool.py:296–395; tools/skill_provenance.py | Autonomous writes to pinned, bundled, hub-installed, or user-owned skills |
| Read-before-write preflight | tools/skill_manager_tool.py:399–427 | Blind patches against unread or drifted skill content |
| Size caps (100 KB / 1 MiB) | tools/skill_manager_tool.py:170–171, 488–489 | Unbounded artifact growth inflating every future system prompt |
| Archive-never-delete + snapshot + rollback | agent/curator.py:17, 1544–1562; agent/curator_backup.py | Irrecoverable loss from a bad autonomous pass |
| Opt-in staging (`write_approval`) and scan (`guard_agent_created`) | tools/skill_manager_tool.py:1279–1339, 94–141 | Unreviewed or dangerous-pattern agent text entering the prompt — both **off by default** |
| Anti-capture prompt rules | agent/background_review.py:260–275 | Negative tool claims hardening "into refusals the agent cites against itself for months" |

The stack's shape matters more than any row. Every always-on guard is a *capability* guard — whitelist, persistence isolation, archive-only, caps — enforced in code at the toolset layer (insight 4), while every *judgment* guard — content scanning, human approval, LLM consolidation — is opt-in. The default posture is therefore structurally safe and editorially permissive: the system cannot easily hurt itself, but nothing by default checks whether what it learned is *good*. Read-before-write is enforceable only because the reviewer is a full agent able to call `skill_view` — the practical argument for the heavyweight fork. The provenance flag is the keystone: it lets the Curator's decay apply to agent sediment while never touching user property, and it must exist before the first autonomous write, because it cannot be retrofitted.

Residual risks, assessed neutrally:

- **Skill-quality drift (medium).** Bias-to-act prompting with no fitness signal accumulates plausible-but-wrong procedures; the Curator prunes by *use*, not *correctness*. A weak review model degrades output gracefully — noise, not corruption — but noise compounds in a prompt loaded every session.
- **Ledger as sole verifier (medium).** Verify-on-stop covers code edits only; learned Markdown bypasses it by design. A systematically wrong skill faces no in-loop challenge until a human notices or the external pipeline scores it.
- **Default-off judgment gates (medium in untrusted environments).** `guard_agent_created` and `write_approval` are opt-in; deployments ingesting adversarial content should flip both and accept the review burden.
- **Prompt-mediated learning ceiling (low, inherent).** The loop can only learn what transcript review can see; it cannot discover improvements requiring measurement. A scope limit, not a defect — but do not expect the in-repo loop to move task success rates measurably.
- **Cache-parity fragility (low).** The pins couple the fork to prompt-assembly internals; a future change that perturbs the prefix silently forfeits the ~26% saving — the code's own defensive comments anticipate this.

### 7.4.3 Clone notes

> **Clone notes.** The learning fork is stage 8 of the staged roadmap (~400 LOC: cadence counters, a whitelisted second-agent fork with `_persist_disabled`, curator-lite stale/archive transitions with snapshot/rollback) and the first omission candidate — the loop runs full turns without it, and a time-pressed clone should start with `/learn`-style foreground distillation only. If the fork is kept, the guardrail stack is non-negotiable: whitelist, persistence isolation, provenance, archive-never-delete, and snapshot/rollback are the difference between "self-improvement" and an unsupervised agent writing its own future prompts. Skip the cache-parity pins if the target provider lacks prefix caching (the routed-model digest path is the honest fallback), and flip `write_approval` and `guard_agent_created` on by default if the clone ingests untrusted content. Budget honestly: without cache parity, reflection roughly doubles token spend on review turns [INFERRED from the ~26% figure cited at background_review.py:772–774] — the cadence counters, not the fork, are where the cost knob lives.
