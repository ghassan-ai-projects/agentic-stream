# Bugfix Prompt

Fix bugs root-cause first.

Process:

1. Reproduce or isolate the failing behavior.
2. Identify the actual cause in code, config, docs, or automation.
3. Add or update a failing test first when production behavior is involved, if feasible.
4. Apply the smallest fix that addresses the cause, not just the symptom.
5. Check for nearby regressions.
6. Validate with the narrowest commands that prove the fix.

Do not patch around unclear behavior without stating the assumption or gap.
