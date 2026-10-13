"""Fixture test for scripts/perf-record.py.

Run: python3 -m unittest discover -s scripts/tests -p 'test_*.py'
"""
import importlib.util
import io
import json
import os
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("perf_record", os.path.join(HERE, "..", "perf-record.py"))
perf_record = importlib.util.module_from_spec(spec)
spec.loader.exec_module(perf_record)

FIXTURE = os.path.join(HERE, "testdata", "perf-record", "pair.jsonl")


def load():
    with open(FIXTURE) as f:
        return [json.loads(l) for l in f if l.strip()]


class PerfRecordTest(unittest.TestCase):
    def rows(self):
        rows = perf_record.build_rows(load(), "aaaa", "cccc", "box1", "2026-10-12T00:00:00Z", "ff",
                                      host_local={"threads": 32, "kernel": "6.17"})
        return {(r["scenario"], r["metric"]): r for r in rows}

    def test_scenarios_and_metrics(self):
        got = set(self.rows())
        self.assertEqual(got, {
            ("random/A", "turns_per_cpu_s"),
            ("bot-autopay/A", "turns_per_cpu_s"),
            ("clone", "us_per_op_cpu_gc"),
            ("clone", "alloc_bytes_per_op"),
            ("sampler/A", "sims_per_cpu_s"),
            ("corpus", "heap_live_mb"),
        })  # az100's cand errored, so it has no pair and no row

    def test_ratio_is_median_of_paired_reps(self):
        r = self.rows()[("random/A", "turns_per_cpu_s")]
        self.assertEqual(r["ratios"], [1.1, 1.2])
        self.assertAlmostEqual(r["ratio_to_anchor"], 1.15)
        self.assertEqual(r["reps"], 2)
        self.assertAlmostEqual(r["value"], 1150.0)
        self.assertAlmostEqual(r["anchor_value"], 1000.0)
        self.assertTrue(r["higher_is_better"])

    def test_lower_is_better_keeps_raw_ratio(self):
        r = self.rows()[("clone", "us_per_op_cpu_gc")]
        self.assertFalse(r["higher_is_better"])
        self.assertAlmostEqual(r["ratio_to_anchor"], 0.8)
        h = self.rows()[("corpus", "heap_live_mb")]
        self.assertFalse(h["higher_is_better"])
        # Paired per (row, pair, rep) across every row: two random reps at
        # 36/40 and three other pairs at 40/40 -> median 1.0.
        self.assertEqual(h["reps"], 5)
        self.assertAlmostEqual(h["ratio_to_anchor"], 1.0)

    def test_schema_fields(self):
        r = self.rows()[("sampler/A", "sims_per_cpu_s")]
        for k in ("schema", "commit", "commit_date", "run_date", "anchor", "scenario", "metric", "unit",
                  "higher_is_better", "value", "anchor_value", "ratio_to_anchor", "reps", "ratios", "host", "harness"):
            self.assertIn(k, r)
        self.assertEqual(r["schema"], 1)
        self.assertEqual(r["host"], {"id": "box1", "cpu": "Test CPU", "threads": 32, "kernel": "6.17",
                                     "go": "go1.25.1", "gomaxprocs": 1, "pin": "9", "gogc": "default",
                                     "load1_med": 30.0})
        self.assertEqual(r["harness"], {"tool": "enginebench-pair", "secs": 8.0, "bin_sha256": "ff"})
        self.assertTrue(r["run_date"].endswith("Z"))

    def test_commit_mismatch_warns(self):
        buf = io.StringIO()
        self.assertTrue(perf_record.check_commit(load(), "cccc1234", out=buf))
        self.assertFalse(perf_record.check_commit(load(), "dddd", out=buf))
        self.assertIn("warning", buf.getvalue())


if __name__ == "__main__":
    unittest.main()
