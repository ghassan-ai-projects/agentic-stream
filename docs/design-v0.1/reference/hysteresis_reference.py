"""Reference implementation of SITUATION_MODEL_DESIGN.md sections 4.1 and 6.1.

Stage 1 (disjunct matching): normalize enter and stay into disjunctions of terms.
For each enter term, look for a single stay term it implies, enumerating only the
bands of the facts those two terms mention. Sound but incomplete.

Stage 2 (full enumeration): only if stage 1 fails, and only within budget.
"""

import itertools
import json
import sys

BUDGET = 100_000


def walk(expr, fn):
    if "all" in expr or "any" in expr:
        for sub in expr.get("all", expr.get("any")):
            walk(sub, fn)
    elif "not" in expr:
        walk(expr["not"], fn)
    else:
        op = next(iter(expr))
        left, right = expr[op]
        fn(op, left["fact"], right)


def literals(model):
    nums, strs = {}, {}

    def collect(op, fact, right):
        if "number" in right:
            nums.setdefault(fact, set()).add(right["number"])
        else:
            strs.setdefault(fact, set()).add(right["string"])

    for st in model["states"]:
        for key in ("enter", "stay"):
            if key in st:
                walk(st[key], collect)
    return ({f: sorted(v) for f, v in nums.items()},
            {f: sorted(v) for f, v in strs.items()})


def string_enum(fact, model):
    """The enum of a latest-over-string fact, or None if the fact is numeric."""
    spec = model["facts"].get(fact, {})
    if "latest" not in spec:
        return None
    return model["inputs"][spec["latest"]].get("enum")


def bands(fact, nums, strs, model):
    """Section 4.1: open intervals separated by point bands at each literal,
    plus a distinguished unavailable band. Strings band on their value.

    Referenced strings band on their enum value. Any evidence-only fact has one
    unreferenced value band regardless of type."""
    if fact not in nums and fact not in strs:
        return [("unreferenced", None), ("unavailable", None)]

    enum = string_enum(fact, model)
    if enum is not None:
        vals = sorted(set(strs.get(fact, [])) | set(enum))
        return [("s", v) for v in vals] + [("unavailable", None)]
    ls = nums.get(fact, [])
    if not ls:
        return [("unreferenced", None), ("unavailable", None)]
    out = [("i", ls[0] - 1.0)]
    for i, l in enumerate(ls):
        out.append(("p", l))
        nxt = ls[i + 1] if i + 1 < len(ls) else None
        out.append(("i", (l + nxt) / 2 if nxt else l + 1.0))
    out.append(("unavailable", None))
    return out


def evaluate(expr, assign):
    if "all" in expr:
        return all(evaluate(s, assign) for s in expr["all"])
    if "any" in expr:
        return any(evaluate(s, assign) for s in expr["any"])
    if "not" in expr:
        return not evaluate(expr["not"], assign)
    op = next(iter(expr))
    fact = expr[op][0]["fact"]
    lit = expr[op][1]
    kind, val = assign[fact]
    if kind == "unavailable":
        return False                                   # section 2.4
    if "string" in lit:
        return (val == lit["string"]) if op == "eq" else (val != lit["string"])
    r = lit["number"]
    return {"gte": val >= r, "gt": val > r, "lte": val <= r,
            "lt": val < r, "eq": val == r, "neq": val != r}[op]


def facts_in(expr):
    seen = set()
    walk(expr, lambda op, f, r: seen.add(f))
    return seen


def terms(expr):
    return expr["any"] if "any" in expr else [expr]


def implies(a, b, nums, strs, model, budget=BUDGET):
    involved = sorted(facts_in(a) | facts_in(b))
    domains = [bands(f, nums, strs, model) for f in involved]
    size = 1
    for d in domains:
        size *= len(d)
    if size > budget:
        return None, size, None
    for combo in itertools.product(*domains):
        assign = dict(zip(involved, combo))
        if evaluate(a, assign) and not evaluate(b, assign):
            return False, size, assign
    return True, size, None


def check(path):
    with open(path, encoding="utf-8") as model_file:
        model = json.load(model_file)
    nums, strs = literals(model)
    ok = True
    print(f"\n{model['id']}@{model['version']}")
    for f in sorted(set(nums) | set(strs)):
        print(f"    bands  {f:24s} {len(bands(f, nums, strs, model)):3d}")
    for st in model["states"]:
        if "stay" not in st:
            print(f"    state  {st['id']:24s} no stay condition")
            continue
        et, stt = terms(st["enter"]), terms(st["stay"])
        worst, unmatched = 0, []
        for i, e in enumerate(et):
            matched = False
            for s in stt:
                held, size, _ = implies(e, s, nums, strs, model)
                worst = max(worst, size)
                if held is True:
                    matched = True
                    break
            if not matched:
                unmatched.append(i)
        if not unmatched:
            print(f"    state  {st['id']:24s} PROVED stage 1 - {len(et)} enter term(s), "
                  f"largest sub-problem {worst:,} combinations")
            continue
        held, size, ce = implies(st["enter"], st["stay"], nums, strs, model)
        if held is True:
            print(f"    state  {st['id']:24s} PROVED stage 2 over {size:,} combinations")
        elif held is None:
            ok = False
            print(f"    state  {st['id']:24s} hysteresis_uncheckable - stage 1 failed on "
                  f"enter term(s) {unmatched}; stage 2 needs {size:,} > {BUDGET:,}")
        else:
            ok = False
            print(f"    state  {st['id']:24s} REJECT - counterexample {ce}")
    return ok


def band_label(fact, value, nums, strs, model):
    """Band name per section 4.1: i0, p1, i1, p2, i2, ... pk, ik."""
    if fact not in nums and fact not in strs:
        return "unreferenced"
    if string_enum(fact, model) is not None:
        return "s:" + value
    ls = nums.get(fact, [])
    if not ls:
        return "i0"
    for j, l in enumerate(ls, 1):
        if value == l:
            return f"p{j}"
        if value < l:
            return f"i{j - 1}"
    return f"i{len(ls)}"


if __name__ == "__main__":
    all_ok = True
    for p in sys.argv[1:]:
        all_ok &= check(p)
    print("\nRESULT:", "proved" if all_ok else "REJECTED")
    sys.exit(0 if all_ok else 1)
