#!/usr/bin/env python3
r"""
scripts/generate_ablation_figure.py
Generates Figure 10 for Section 4.9 (Ablation Study: Graph-Aware vs Frequency-Only under Cache Contention).
Outputs both Indonesian and English publication-grade figures dynamically computed from raw k6 JSON runs.
Zero em-dash tolerance.
"""
from pathlib import Path
import json
import glob
import shutil
import matplotlib
matplotlib.use('Agg')
import matplotlib.patches as mpatches
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np
from scipy.stats import mannwhitneyu

BASE = Path(__file__).resolve().parent.parent
FIGS_ID = BASE / "figures"
FIGS_ID_SUB = BASE / "figures" / "id"
FIGS_EN = BASE / "figures" / "en"
DRAFT_FIGS_ID = BASE / "draft" / "figures"
DRAFT_FIGS_EN = BASE / "draft" / "en" / "figures"

for d in [FIGS_ID, FIGS_ID_SUB, FIGS_EN]:
    d.mkdir(parents=True, exist_ok=True)
if (BASE / 'draft').exists():
    DRAFT_FIGS_ID.mkdir(parents=True, exist_ok=True)
    DRAFT_FIGS_EN.mkdir(parents=True, exist_ok=True)

plt.rcParams.update({
    'font.family': 'serif',
    'font.size': 10,
    'axes.titlesize': 10.5,
    'axes.labelsize': 10,
    'xtick.labelsize': 9,
    'ytick.labelsize': 9,
    'legend.fontsize': 8.5,
    'figure.dpi': 300,
    'savefig.dpi': 300,
    'savefig.bbox': 'tight',
    'savefig.pad_inches': 0.1,
    'mathtext.fontset': 'cm',
    'mathtext.default': 'regular',
})

COLOR_LFU = '#FF9800'    # Orange (consistent with static/baseline across Fig 3-8)
COLOR_GRAPH = '#2196F3'  # Blue (consistent with proposed ZTA Cache across Fig 3-8)

ablation_dir = BASE / "load-testing" / "results" / "ablation"
lfu_files = sorted(glob.glob(str(ablation_dir / "*_lfu_rep*.json")))
graph_files = sorted(glob.glob(str(ablation_dir / "*_graph_rep*.json")))

def load_metrics(files):
    h2, e2e, direct, med = [], [], [], []
    for f in files:
        with open(f, "r", encoding="utf-8") as fp:
            d = json.load(fp)
        m = d.get("metrics", {})
        h2.append(float(m.get("multihop_latency", {}).get("p(99)")))
        e2e.append(float(m.get("http_req_duration", {}).get("p(99)")))
        direct.append(float(m.get("direct_latency", {}).get("p(99)")))
        med.append(float(m.get("http_req_duration", {}).get("med")))
    return np.array(h2), np.array(e2e), np.array(direct), np.array(med)

lfu_hop2, lfu_e2e, lfu_direct, lfu_med = load_metrics(lfu_files)
graph_hop2, graph_e2e, graph_direct, graph_med = load_metrics(graph_files)
n_runs = len(lfu_hop2)

def compute_ablation_stats(lfu_arr, graph_arr):
    """Compute median, % delta, 1-sided MWU (H1: Graph < LFU), and Cliff's delta (LFU - Graph dominance)."""
    med_lfu = float(np.median(lfu_arr))
    med_graph = float(np.median(graph_arr))
    pct_delta = ((med_graph - med_lfu) / med_lfu) * 100.0
    u_graph, p_1s = mannwhitneyu(graph_arr, lfu_arr, alternative='less', method='exact')
    n1, n2 = len(lfu_arr), len(graph_arr)
    u_lfu = n1 * n2 - u_graph
    cliffs_d = (u_lfu - u_graph) / (n1 * n2)
    ad = abs(cliffs_d)
    if ad < 0.147:
        eff = "negligible"
    elif ad < 0.330:
        eff = "small"
    elif ad < 0.474:
        eff = "medium"
    else:
        eff = "large"
    return med_lfu, med_graph, pct_delta, u_graph, p_1s, cliffs_d, eff

def draw_bracket(ax, x1, x2, y, h, text, fontsize=7.8, color='#1E293B', lw=1.1):
    ax.plot([x1, x1, x2, x2], [y - h, y, y, y - h], lw=lw, color=color)
    ax.text((x1 + x2) * 0.5, y + h * 0.35, text, ha='center', va='bottom',
            fontsize=fontsize, color=color, linespacing=1.25)

def thousands_id_formatter(x, pos):
    return f'{int(round(x)):,}'.replace(',', '.')

def thousands_en_formatter(x, pos):
    return f'{int(round(x)):,}'

def fmt_num(val, decimals=1, lang="id"):
    s = f"{val:.{decimals}f}"
    return s.replace('.', ',') if lang == "id" else s

def fmt_stat_bracket(lfu_arr, graph_arr, lang="id"):
    med_lfu, med_graph, pct_delta, u_g, p_1s, d_val, eff = compute_ablation_stats(lfu_arr, graph_arr)
    sign_str = "+" if pct_delta > 0 else "\u2212"
    pct_str = fmt_num(abs(pct_delta), 1, lang)
    p_str = fmt_num(p_1s, 3, lang)
    d_sign = "+" if d_val > 0 else "\u2212"
    d_str = fmt_num(abs(d_val), 2, lang)
    if lang == "id":
        p_tex = p_str.replace(',', '{,}')
        d_tex = d_str.replace(',', '{,}')
        pct_tex = pct_str.replace(',', '{,}')
        return (
            rf"$\mathbf{{{sign_str}{pct_tex}\%}}$ ($U = {u_g:.0f},\ p = {p_tex}$)" + "\n" +
            rf"$\delta = {d_sign}{d_tex}$ ({eff})"
        )
    else:
        return (
            rf"$\mathbf{{{sign_str}{pct_str}\%}}$ ($U = {u_g:.0f},\ p = {p_str}$)" + "\n" +
            rf"$\delta = {d_sign}{d_str}$ ({eff})"
        )

def render_figure(lang="id", out_paths=None):
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(9.6, 4.6), dpi=300)

    if lang == "id":
        labels_a = ['LFU\n(Hop-2)', 'Graph-Aware\n(Hop-2)', 'LFU\n(End-to-End)', 'Graph-Aware\n(End-to-End)']
        ylabel_1 = r'Latensi $p_{99}$ (ms)'
        title_1 = f'(a) Transaksi Multi-Hop East-West ($n = {n_runs}$)'
        labels_b = ['LFU\n(Direct $p_{99}$)', 'Graph-Aware\n(Direct $p_{99}$)', 'LFU\n(E2E Median)', 'Graph-Aware\n(E2E Median)']
        ylabel_2 = 'Latensi (ms)'
        title_2 = f'(b) Transaksi Langsung ($p_{{99}}$) & E2E Median ($n = {n_runs}$)'
        sup_title = rf'Studi Ablasi Multi-Hop pada Persaingan Kapasitas Cache ($K_{{\mathrm{{local}}}} = 2,\ n = {n_runs}$)'
        leg_lfu = 'Frequency-Only (LFU, $w = 1{,}0$)'
        leg_graph = 'Graph-Aware (Usulan, $w = 0{,}7$)'
        y_fmt = thousands_id_formatter
    else:
        labels_a = ['LFU\n(Hop-2)', 'Graph-Aware\n(Hop-2)', 'LFU\n(End-to-End)', 'Graph-Aware\n(End-to-End)']
        ylabel_1 = r'$p_{99}$ Latency (ms)'
        title_1 = f'(a) East-West Multi-Hop Transactions ($n = {n_runs}$)'
        labels_b = ['LFU\n(Direct $p_{99}$)', 'Graph-Aware\n(Direct $p_{99}$)', 'LFU\n(E2E Median)', 'Graph-Aware\n(E2E Median)']
        ylabel_2 = 'Latency (ms)'
        title_2 = f'(b) Direct Call ($p_{{99}}$) & E2E Median ($n = {n_runs}$)'
        sup_title = rf'Multi-Hop Ablation Study Under Cache Contention ($K_{{\mathrm{{local}}}} = 2,\ n = {n_runs}$)'
        leg_lfu = 'Frequency-Only (LFU, $w = 1.0$)'
        leg_graph = 'Graph-Aware (Proposed, $w = 0.7$)'
        y_fmt = thousands_en_formatter

    data_a = [lfu_hop2, graph_hop2, lfu_e2e, graph_e2e]
    colors_pair = [COLOR_LFU, COLOR_GRAPH, COLOR_LFU, COLOR_GRAPH]

    # ── Panel A ──
    bp1 = ax1.boxplot(data_a, tick_labels=labels_a, patch_artist=True, widths=0.48,
                      showfliers=False, medianprops=dict(color='black', linewidth=1.8))
    for patch, color in zip(bp1['boxes'], colors_pair):
        patch.set_facecolor(color)
        patch.set_alpha(0.72)

    rng = np.random.default_rng(42)
    for i, d in enumerate(data_a, start=1):
        jitter = rng.normal(0, 0.045, size=len(d))
        ax1.scatter(np.full(len(d), i) + jitter, d, color='#1E293B', alpha=0.75, s=22, zorder=4, edgecolors='white', linewidth=0.4)
        med_v = np.median(d)
        med_lbl = fmt_num(med_v, 1, lang).replace(',', '{,}')
        ax1.text(i, np.min(d) - 55, rf'$\tilde{{x}}={med_lbl}$', ha='center', va='top', fontsize=7.3, color='#334155')

    ax1.set_ylabel(ylabel_1)
    ax1.set_title(title_1, pad=10)
    ax1.grid(True, linestyle='--', alpha=0.35)
    ax1.set_ylim(1080, 2320)
    ax1.yaxis.set_major_formatter(ticker.FuncFormatter(y_fmt))
    ax1.axvline(x=2.5, color='#CBD5E1', linestyle=':', linewidth=1.2)

    draw_bracket(ax1, 1, 2, 2110, 32, fmt_stat_bracket(lfu_hop2, graph_hop2, lang), fontsize=7.6)
    draw_bracket(ax1, 3, 4, 1710, 32, fmt_stat_bracket(lfu_e2e, graph_e2e, lang), fontsize=7.6)

    # ── Panel B ──
    data_b = [lfu_direct, graph_direct, lfu_med, graph_med]
    bp2 = ax2.boxplot(data_b, tick_labels=labels_b, patch_artist=True, widths=0.48,
                      showfliers=False, medianprops=dict(color='black', linewidth=1.8))
    for patch, color in zip(bp2['boxes'], colors_pair):
        patch.set_facecolor(color)
        patch.set_alpha(0.72)

    for i, d in enumerate(data_b, start=1):
        jitter = rng.normal(0, 0.045, size=len(d))
        ax2.scatter(np.full(len(d), i) + jitter, d, color='#1E293B', alpha=0.75, s=22, zorder=4, edgecolors='white', linewidth=0.4)
        med_v = np.median(d)
        med_lbl = fmt_num(med_v, 1, lang).replace(',', '{,}')
        offset = -48 if i <= 2 else -42
        ax2.text(i, np.min(d) + offset, rf'$\tilde{{x}}={med_lbl}$', ha='center', va='top', fontsize=7.3, color='#334155')

    ax2.set_ylabel(ylabel_2)
    ax2.set_title(title_2, pad=10)
    ax2.grid(True, linestyle='--', alpha=0.35)
    ax2.set_ylim(240, 1420)
    ax2.yaxis.set_major_formatter(ticker.FuncFormatter(y_fmt))
    ax2.axvline(x=2.5, color='#CBD5E1', linestyle=':', linewidth=1.2)

    draw_bracket(ax2, 1, 2, 1255, 28, fmt_stat_bracket(lfu_direct, graph_direct, lang), fontsize=7.6)
    draw_bracket(ax2, 3, 4, 480, 28, fmt_stat_bracket(lfu_med, graph_med, lang), fontsize=7.6)

    # Shared Legend
    patch_lfu = mpatches.Patch(facecolor=COLOR_LFU, alpha=0.72, edgecolor='black', label=leg_lfu)
    patch_graph = mpatches.Patch(facecolor=COLOR_GRAPH, alpha=0.72, edgecolor='black', label=leg_graph)
    fig.legend(handles=[patch_lfu, patch_graph], loc='upper center', ncol=2,
               bbox_to_anchor=(0.5, 0.96), frameon=True, facecolor='#F8FAFC', edgecolor='#CBD5E1')

    fig.suptitle(sup_title, fontsize=11, fontweight='bold', y=1.02)
    fig.tight_layout(rect=[0, 0, 1, 0.91])

    for p in out_paths:
        fig.savefig(p)
        print(f"Generated {p} successfully.")
    plt.close(fig)

if __name__ == "__main__":
    id_paths = [FIGS_ID / "fig10-ablation-multihop-boxplot.png", FIGS_ID_SUB / "fig10-ablation-multihop-boxplot.png"]
    en_paths = [FIGS_EN / "fig10-ablation-multihop-boxplot.png"]
    if (BASE / 'draft').exists():
        id_paths.append(DRAFT_FIGS_ID / "fig10-ablation-multihop-boxplot.png")
        en_paths.append(DRAFT_FIGS_EN / "fig10-ablation-multihop-boxplot.png")
    render_figure("id", id_paths)
    render_figure("en", en_paths)

    # Sync to bundles if present
    for bundle_name in ["predictive-authorization-caching-zero-trust-mesh", "core_codebase_bundle"]:
        b = BASE / bundle_name
        if b.exists():
            for rel_dir, src_file in [
                ("figures", FIGS_ID / "fig10-ablation-multihop-boxplot.png"),
                ("figures/id", FIGS_ID / "fig10-ablation-multihop-boxplot.png"),
                ("figures/en", FIGS_EN / "fig10-ablation-multihop-boxplot.png"),
                ("draft/figures", FIGS_ID / "fig10-ablation-multihop-boxplot.png"),
                ("draft/en/figures", FIGS_EN / "fig10-ablation-multihop-boxplot.png"),
                ("paper/figures", FIGS_ID / "fig10-ablation-multihop-boxplot.png"),
                ("paper/figures/en", FIGS_EN / "fig10-ablation-multihop-boxplot.png"),
            ]:
                target_dir = b / rel_dir
                if target_dir.exists():
                    shutil.copy2(src_file, target_dir / "fig10-ablation-multihop-boxplot.png")
            if (b / "scripts").exists():
                shutil.copy2(Path(__file__), b / "scripts" / "generate_ablation_figure.py")
