#!/usr/bin/env python3
"""
scripts/test_analyze_ablation.py
Unit Test Suite untuk Modul Analisis Statistik Ablasi (analyze_ablation_results.py)

Memvalidasi:
1. Akurasi kalkulasi ukuran efek Cliff's Delta pada kondisi batas (boundary conditions).
2. Estimasi Confidence Interval Bootstrap untuk selisih median.
3. Uji hipotesis Mann-Whitney U dan perhitungan reduksi persentase.
4. Integritas sintaks output Markdown, LaTeX, dan CSV.
"""

import os
import tempfile
import unittest
from pathlib import Path
import numpy as np

# Import modul analisis yang diuji
from analyze_ablation_results import (
    cliffs_delta,
    bootstrap_median_diff_ci,
    analyze_metric_group,
    generate_markdown_report,
    generate_latex_table,
    generate_csv_summary,
)


class TestAblationStatisticalAnalysis(unittest.TestCase):
    def test_cliffs_delta_all_greater(self):
        """Uji Cliff's Delta saat seluruh elemen kelompok 1 lebih besar daripada kelompok 2 (delta = 1.0)."""
        a = [10.0, 11.0, 12.0, 13.0]
        b = [1.0, 2.0, 3.0, 4.0]
        d, mag = cliffs_delta(a, b)
        self.assertAlmostEqual(d, 1.0, places=5)
        self.assertEqual(mag, "Large")

    def test_cliffs_delta_all_lesser(self):
        """Uji Cliff's Delta saat seluruh elemen kelompok 1 lebih kecil daripada kelompok 2 (delta = -1.0)."""
        a = [1.0, 2.0, 3.0]
        b = [5.0, 6.0, 7.0]
        d, mag = cliffs_delta(a, b)
        self.assertAlmostEqual(d, -1.0, places=5)
        self.assertEqual(mag, "Large")

    def test_cliffs_delta_identical(self):
        """Uji Cliff's Delta saat kedua kelompok identik (delta = 0.0)."""
        a = [5.0, 5.0, 5.0]
        b = [5.0, 5.0, 5.0]
        d, mag = cliffs_delta(a, b)
        self.assertAlmostEqual(d, 0.0, places=5)
        self.assertEqual(mag, "Negligible")

    def test_cliffs_delta_empty(self):
        """Uji penanganan sampel kosong."""
        d, mag = cliffs_delta([], [1.0, 2.0])
        self.assertEqual(d, 0.0)
        self.assertEqual(mag, "Undefined")

    def test_bootstrap_median_diff_ci(self):
        """Uji perhitungan Confidence Interval Bootstrap untuk selisih median."""
        lfu = [10.0, 10.2, 9.8, 10.5, 10.1]
        graph = [4.0, 4.2, 3.9, 4.1, 4.0]
        actual_diff, low, high = bootstrap_median_diff_ci(lfu, graph, n_bootstraps=500, seed=42)

        self.assertAlmostEqual(actual_diff, 6.1, places=1)
        self.assertGreater(low, 5.0)
        self.assertLess(high, 7.0)
        self.assertTrue(low <= actual_diff <= high)

    def test_analyze_metric_group_statistically_significant(self):
        """Uji pengujian inferensial saat Graph-Aware secara signifikan lebih cepat daripada LFU."""
        # LFU mengalami cache miss berulang pada hop-2 (latensi ~12 ms)
        lfu = [12.4, 13.1, 12.8, 13.5, 12.9]
        # Graph-Aware mengalami cache hit lokal pada hop-2 (latensi ~4.2 ms)
        graph = [4.1, 4.3, 4.0, 4.5, 4.2]

        res = analyze_metric_group(lfu, graph, "Multi-Hop Latency")

        self.assertEqual(res["metric"], "Multi-Hop Latency")
        self.assertGreater(res["pct_reduction"], 60.0)
        self.assertLess(res["mwu_p_onesided"], 0.05)
        self.assertEqual(res["cliffs_delta"], 1.0)
        self.assertEqual(res["effect_size"], "Large")
        self.assertGreater(res["ci_lower"], 7.0)

    def test_generators_output_validity(self):
        """Uji bahwa generator output menghasilkan berkas non-kosong dengan format yang benar."""
        sample_results = [
            {
                "metric": "Multi-Hop Latency (Hop-2) [ms]",
                "lfu_med": 12.90,
                "lfu_mean": 12.94,
                "lfu_std": 0.41,
                "graph_med": 4.20,
                "graph_mean": 4.22,
                "graph_std": 0.19,
                "pct_reduction": 67.4,
                "mwu_u": 0.0,
                "mwu_p_twosided": 0.0079,
                "mwu_p_onesided": 0.0040,
                "cliffs_delta": 1.000,
                "effect_size": "Large",
                "ci_diff": 8.70,
                "ci_lower": 7.90,
                "ci_upper": 9.40,
                "shapiro_lfu_p": 0.89,
                "shapiro_graph_p": 0.92,
            }
        ]

        with tempfile.TemporaryDirectory() as tmpdir:
            tmppath = Path(tmpdir)
            md_path = tmppath / "report.md"
            tex_path = tmppath / "table.tex"
            csv_path = tmppath / "table.csv"

            generate_markdown_report(sample_results, 5, md_path)
            generate_latex_table(sample_results, tex_path)
            generate_csv_summary(sample_results, csv_path)

            self.assertTrue(md_path.exists())
            self.assertTrue(tex_path.exists())
            self.assertTrue(csv_path.exists())

            with open(md_path, "r", encoding="utf-8") as f:
                md_content = f.read()
                self.assertIn("Multi-Hop Latency", md_content)
                self.assertIn("67.4%", md_content)
                self.assertNotIn("\u2014", md_content)  # Zero em-dash check

            with open(tex_path, "r", encoding="utf-8") as f:
                tex_content = f.read()
                self.assertIn(r"\begin{table*}", tex_content)
                self.assertIn("67.4\\%", tex_content)
                self.assertNotIn("\u2014", tex_content)  # Zero em-dash check

            with open(csv_path, "r", encoding="utf-8") as f:
                csv_content = f.read()
                self.assertIn("pct_reduction", csv_content)
                self.assertIn("67.4", csv_content)


if __name__ == "__main__":
    unittest.main()
