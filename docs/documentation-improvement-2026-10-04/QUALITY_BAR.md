# Documentation improvement quality bar

Audience: maintainers. Scope: acceptance for this clarity and teaching pass.

Text and visuals take priority. Use concrete stories and everyday explanations;
include numeric examples only when they clarify time or fixture behavior.

Scope: reader-facing edits stay inside `documentation/`; the plan, quality bar,
and review records live in this working folder under `docs/`. Runtime code,
fixtures, root entrypoints, and validation tools remain read-only authorities.
The existing public release review requirements still apply; this editorial pass
records separate self-review lenses and does not claim independent reviewer sign-off.

| Gate | Required evidence |
| --- | --- |
| Learning path | Home → learning index → purpose → worked flow → time/state → reasoning → effects → design choices; every step has Next reads |
| Understanding | A reader can answer all ten questions in the current review record using linked public pages |
| Plain language | Each new concept starts with a concrete explanation; technical names follow; acronyms are expanded at first use |
| Progressive diagrams | First diagram has at most four nodes; new learning diagrams have at most six; detailed maps are introduced only after the basic model |
| Diagram meaning | Each diagram has a question, text equivalent, and source reference; shapes/labels carry meaning without color |
| Diagram legibility | Static overview is inspected at a 720 px reading width; all public Mermaid diagrams are rendered and inspected in the browser; labels fit and SVG has title/description |
| Page focus | Each new learning page answers one primary question and stays under 180 lines |
| Source alignment | Numeric examples are labeled as illustrations; fixture thresholds and operational claims link to current source |
| Runnable proof | Build, validate, replay twice with fresh temporary databases, compare history hashes, and run the documented local live batch |
| Regression evidence | Run existing focused tests for described stream, cognition, episode, policy, action, and replay behavior |
| Navigation | Existing docs-check passes; new pages/assets are reachable; internal heading links are reviewed |
| Round discipline | Record findings → fix → review → validate → commit; no unresolved P0/P1/P2 finding in this pass at handoff |

Do not use a diagram to imply that every event creates a Situation, every
version starts an episode, or every Intent becomes an effect. Show those gates
in the explanation before introducing the detailed implementation map.

## Completion rule

All gates must pass and all recorded P0/P1/P2 findings must be closed. Preserve
existing accurate reference material. No runtime or validation-tool edits are
part of this pass. Independent release approval remains a separate gate.

## Core concept completion gate

For the follow-up, require a connected explanation of domain identities,
state/publication, attention/reasoning, proposals/effects, and ownership/recovery.
Each concept group must have a plain meaning, a domain example or purpose,
an implementation owner, and a stated current limit where the design exceeds
the implemented model. Validate the reader questions in the Round 4 record.
Keep the motor story as the anchor; avoid presenting a schema field as an
enforced capability merely because the field exists.

## Wording gate

Lead with what the reader needs to understand or do. Prefer concrete actors
and verbs to dense noun lists. Explain essential technical terms at first use
when the page introduces them; use exact contract names where precision matters.
Retain explicit limits and requirement words. Do not simplify wording into a
stronger guarantee. Verify that runnable examples, protocol identifiers, and
diagram definitions are unchanged for a prose-only pass.
