"""Regression tests for the executable hysteresis design oracle."""

import unittest

from hysteresis_reference import BUDGET, bands, implies


class HysteresisReferenceTests(unittest.TestCase):
    def test_evidence_only_string_has_one_value_band(self):
        model = {
            "inputs": {
                "mode_input": {
                    "value_type": "string",
                    "enum": ["idle", "running"],
                }
            },
            "facts": {"mode": {"latest": "mode_input"}},
        }

        self.assertEqual(
            bands("mode", {}, {}, model),
            [("unreferenced", None), ("unavailable", None)],
        )

    def test_cross_product_budget_is_checked_before_enumeration(self):
        literals = list(range(32))
        nums = {"a": literals, "b": literals, "c": literals}
        model = {"inputs": {}, "facts": {"a": {}, "b": {}, "c": {}}}
        expression = {
            "all": [
                {"gte": [{"fact": "a"}, {"number": 0}]},
                {"gte": [{"fact": "b"}, {"number": 0}]},
                {"gte": [{"fact": "c"}, {"number": 0}]},
            ]
        }

        result, combinations, counterexample = implies(
            expression,
            expression,
            nums,
            {},
            model,
            budget=BUDGET,
        )

        self.assertIsNone(result)
        self.assertGreater(combinations, BUDGET)
        self.assertIsNone(counterexample)


if __name__ == "__main__":
    unittest.main()
