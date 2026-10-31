#!/usr/bin/env python3
"""
scripts/generate_ablation_figure.py
Generates Figure 10 for Section 4.9 (Ablation Study: Graph-Aware vs Frequency-Only under Cache Contention).
Outputs both Indonesian and English figures.
Reads raw empirical JSON results from load-testing/results/ablation/.
Zero em-dash tolerance.
"""
from pathlib import Path
import json
import glob
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import numpy as np

BASE = Path(__file__).resolve().parent.parent
FIGS_ID = BASE / "figures"
FIGS_EN = BASE / "figures" / "en"
FIGS_ID.mkdir(parents=True, exist_ok=True)
FIGS_EN.mkdir(parents=True, exist_ok=True)

plt.rcParams.update({
    'font.family': 'serif',
    'font.size': 10,
    'axes.titlesize': 11,
    'axes.labelsize': 10,
    'xtick.labelsize': 9,
    'ytick.labelsize': 9,
    'legend.fontsize': 9,
    'figure.dpi': 300,
    'savefig.dpi': 300,
    'savefig.bbox': 'tight',
    'savefig.pad_inches': 0.1,
    'mathtext.fontset': 'cm',
    'mathtext.default': 'regular',
})

# Load empirical data
ablation_dir = BASE / "load-testing" / "results" / "ablation"
lfu_files = sorted(glob.glob(str(ablation_dir / "*_lfu_rep*.json")))
graph_files = sorted(glob.glob(str(ablation_dir / "*_graph_rep*.json")))

def load_metrics(files):
    h2, e2e, direct, med = [], [], [], []
    for f in files:
        with open(f, "r", encoding="utf-8") as fp:
            d = json.load(fp)
        m = d.get("metrics", {})
        h2.append(m.get("multihop_latency", {}).get("p(99)"))
        e2e.append(m.get("http_req_duration", {}).get("p(99)"))
        direct.append(m.get("direct_latency", {}).get("p(99)"))
        med.append(m.get("http_req_duration", {}).get("med"))
    return h2, e2e, direct, med

lfu_hop2, lfu_e2e, lfu_direct, lfu_med = load_metrics(lfu_files)
graph_hop2, graph_e2e, graph_direct, graph_med = load_metrics(graph_files)

data_a = [lfu_hop2, graph_hop2, lfu_e2e, graph_e2e]
colors_a = ['#e74c3c', '#2ecc71', '#e67e22', '#27ae60']

data_b = [lfu_direct, graph_direct, lfu_med, graph_med]
colors_b = ['#3498db', '#2980b9', '#9b59b6', '#8e44ad']

def render_figure(lang="id", out_path=None):
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4.5))

    if lang == "id":
        labels_a = ['LFU\n(Hop-2)', 'Graph-Aware\n(Hop-2)', 'LFU\n(End-to-End)', 'Graph-Aware\n(End-to-End)']
        ylabel_1 = 'Latensi p99 [ms]'
        title_1 = '(a) Distribusi Latensi Transaksi Multi-Hop'
        labels_b = ['LFU\n(Direct)', 'Graph-Aware\n(Direct)', 'LFU\n(Median)', 'Graph-Aware\n(Median)']
        ylabel_2 = 'Latensi [ms]'
        title_2 = '(b) Latensi Direct (p99) & End-to-End (Median)'
        pct_h2 = r'$\mathbf{-6{,}4\%}$' + '\n' + r'$(\delta = +0{,}36)$'
        pct_e2e = r'$\mathbf{-4{,}5\%}$' + '\n' + r'$(\delta = +0{,}36)$'
        note_direct = r'$\mathbf{-2{,}5\%}$'
        note_med = r'$\Delta \approx 8\text{ ms}$'
    else:
        labels_a = ['LFU\n(Hop-2)', 'Graph-Aware\n(Hop-2)', 'LFU\n(End-to-End)', 'Graph-Aware\n(End-to-End)']
        ylabel_1 = 'p99 Latency [ms]'
        title_1 = '(a) Multi-Hop Latency Distribution'
        labels_b = ['LFU\n(Direct)', 'Graph-Aware\n(Direct)', 'LFU\n(Median)', 'Graph-Aware\n(Median)']
        ylabel_2 = 'Latency [ms]'
        title_2 = '(b) Direct (p99) & End-to-End (Median) Latency'
        pct_h2 = r'$\mathbf{-6.4\%}$' + '\n' + r'$(\delta = +0.36)$'
        pct_e2e = r'$\mathbf{-4.5\%}$' + '\n' + r'$(\delta = +0.36)$'
        note_direct = r'$\mathbf{-2.5\%}$'
        note_med = r'$\Delta \approx 8\text{ ms}$'

    # Subplot A
    bp1 = ax1.boxplot(data_a, tick_labels=labels_a, patch_artist=True, widths=0.55,
                      medianprops=dict(color='black', linewidth=1.8))

    for patch, color in zip(bp1['boxes'], colors_a):
        patch.set_facecolor(color)
        patch.set_alpha(0.7)

    np.random.seed(42)
    for i, d in enumerate(data_a, start=1):
        jitter = np.random.normal(0, 0.04, size=len(d))
        ax1.plot(i + jitter, d, 'k.', alpha=0.6, markersize=7)

    ax1.set_ylabel(ylabel_1)
    ax1.set_title(title_1)
    ax1.grid(True, linestyle='--', alpha=0.4)
    ax1.set_ylim(1100, 2200)

    ax1.annotate(pct_h2,
                 xy=(1.5, 1750), xytext=(1.5, 1950),
                 ha='center', fontsize=9,
                 arrowprops=dict(arrowstyle='->', lw=1.2, color='#2c3e50'))

    ax1.annotate(pct_e2e,
                 xy=(3.5, 1480), xytext=(3.5, 1750),
                 ha='center', fontsize=9,
                 arrowprops=dict(arrowstyle='->', lw=1.2, color='#2c3e50'))

    # Subplot B
    bp2 = ax2.boxplot(data_b, tick_labels=labels_b, patch_artist=True, widths=0.55,
                      medianprops=dict(color='black', linewidth=1.8))

    for patch, color in zip(bp2['boxes'], colors_b):
        patch.set_facecolor(color)
        patch.set_alpha(0.7)

    for i, d in enumerate(data_b, start=1):
        jitter = np.random.normal(0, 0.04, size=len(d))
        ax2.plot(i + jitter, d, 'k.', alpha=0.6, markersize=7)

    ax2.set_ylabel(ylabel_2)
    ax2.set_title(title_2)
    ax2.grid(True, linestyle='--', alpha=0.4)
    ax2.set_ylim(250, 1350)

    ax2.annotate(note_direct,
                 xy=(1.5, 1050), xytext=(1.5, 1180),
                 ha='center', fontsize=9,
                 arrowprops=dict(arrowstyle='->', lw=1.2, color='#2c3e50'))

    ax2.annotate(note_med,
                 xy=(3.5, 410), xytext=(3.5, 600),
                 ha='center', fontsize=9,
                 arrowprops=dict(arrowstyle='->', lw=1.2, color='#2c3e50'))

    fig.tight_layout()
    fig.savefig(out_path)
    plt.close(fig)
    print(f"Generated {out_path} successfully.")

if __name__ == "__main__":
    render_figure("id", FIGS_ID / "fig10-ablation-multihop-boxplot.png")
    render_figure("en", FIGS_EN / "fig10-ablation-multihop-boxplot.png")
